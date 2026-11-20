//go:build integration

// B1-2 存量收口（2026-11-20 审计续批）：字典类型删除 × 工单字段规范 `dict:<code>`
// 引用互斥。
//
// 写侧（ReplaceTypeFields）：工单类型 CAS 事务内按稳定排序为所有新规范引用取
// dict_types 父行 FOR KEY SHARE，锁内确认存在并持有至提交；
// 删除侧（DeleteTypeByCode）：同事务先锁当前父行 FOR UPDATE，下一独立语句以
// 锁后新快照重扫规范引用，再按既有 ID/version/92304/404/409 边界删除锁定行
// （避免单语句内「先取快照后等锁」读到锁前旧引用集）。
//
// 不变量：select/radio/multi_select 规范的 `dict:<code>` 引用不指向已删除类型
// （零悬空引用）。处理当前三类规范引用；历史歧义项（其他 field_type 的残留
// dict: 形态、多元素/具体选项列表）不在守卫范围。动态公式与字典子项集合版本
// 政策后置（不在本批）。
package repository_test

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

// uniqueSuffix 测试数据唯一后缀（与 service 包同约定：完整 UnixNano，无截断回绕）。
func uniqueSuffix() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

// ─── 夹具 ───

func createDictTypeForMutex(t *testing.T, repo *repository.DictRepo, code string) (int64, int64) {
	t.Helper()
	dt := &model.DictType{Code: code, Name: "互斥测试字典", Enabled: true}
	require.NoError(t, repo.CreateType(context.Background(), dt))
	return dt.ID, dt.Version
}

func insertTicketTypeForMutex(t *testing.T, code string) (int64, int) {
	t.Helper()
	var id int64
	var version int
	err := testPool.QueryRow(context.Background(), `
		INSERT INTO ticket_types (code, name, states, transitions)
		VALUES ($1, $1, '["open","closed"]', '{}')
		RETURNING id, version`, code).Scan(&id, &version)
	require.NoError(t, err)
	return id, version
}

func cleanupDictMutexFixture(t *testing.T, dictCode, typeCode string) {
	t.Helper()
	_, err := testPool.Exec(context.Background(),
		`DELETE FROM ticket_type_fields WHERE type_code = $1`, typeCode)
	require.NoError(t, err)
	_, err = testPool.Exec(context.Background(),
		`DELETE FROM ticket_types WHERE code = $1`, typeCode)
	require.NoError(t, err)
	_, err = testPool.Exec(context.Background(),
		`DELETE FROM dict_types WHERE code = $1`, dictCode)
	require.NoError(t, err)
}

func countDictSpecRefs(t *testing.T, code string) int64 {
	t.Helper()
	var n int64
	require.NoError(t, testPool.QueryRow(context.Background(), `
		SELECT COUNT(*) FROM ticket_type_fields f,
		jsonb_array_elements_text(f.field_options) e
		WHERE f.field_type IN ('select','radio','multi_select') AND e = 'dict:' || $1`,
		code).Scan(&n))
	return n
}

func dictTypeAlive(t *testing.T, code string) bool {
	t.Helper()
	var alive bool
	require.NoError(t, testPool.QueryRow(context.Background(),
		`SELECT EXISTS(SELECT 1 FROM dict_types WHERE code = $1)`, code).Scan(&alive))
	return alive
}

func dictRefSelectField(code string) model.TicketTypeField {
	return model.TicketTypeField{
		FieldKey:     "sev",
		FieldLabel:   "级别",
		FieldType:    "select",
		FieldOptions: json.RawMessage(`["dict:` + code + `"]`),
		SortOrder:    1,
	}
}

// ─── 顺序正反例 ───

// 写侧建立引用后，删除侧两种形态（legacy code-only / target CAS）都必须被
// 锁后重扫拒绝且字典存活；摘除引用后按既有 ID/version 边界可删。
func TestDictFieldRefMutex_ReplaceRefBlocksDelete(t *testing.T) {
	ctx := context.Background()
	ticketRepo := repository.NewTicketRepo(testPool)
	dictRepo := repository.NewDictRepo(testPool)

	code := "dfm" + uniqueSuffix()
	tcode := "dfmt" + uniqueSuffix()
	dictID, _ := createDictTypeForMutex(t, dictRepo, code)
	ttID, ttVer := insertTicketTypeForMutex(t, tcode)
	defer cleanupDictMutexFixture(t, code, tcode)

	fields := []model.TicketTypeField{dictRefSelectField(code)}
	require.NoError(t, ticketRepo.ReplaceTypeFields(ctx, tcode, fields, &ttVer, ttID),
		"引用现存字典的规范 replace 须成功")
	require.Equal(t, int64(1), countDictSpecRefs(t, code))

	require.ErrorIs(t, dictRepo.DeleteTypeByCode(ctx, code), repository.ErrDictTypeInUse,
		"legacy 形态删除被引用字典须拒")
	require.ErrorIs(t, dictRepo.DeleteTypeByCode(ctx, code,
		repository.DictTypeDeleteTarget{ID: dictID, Version: 1}), repository.ErrDictTypeInUse,
		"target CAS 形态删除被引用字典须拒")
	require.True(t, dictTypeAlive(t, code), "被拒删的字典必须存活")

	// 摘除引用后可删（守卫不误伤、CAS 边界保留）
	_, err := testPool.Exec(ctx, `DELETE FROM ticket_type_fields WHERE type_code = $1`, tcode)
	require.NoError(t, err)
	require.NoError(t, dictRepo.DeleteTypeByCode(ctx, code,
		repository.DictTypeDeleteTarget{ID: dictID, Version: 1}))
	require.False(t, dictTypeAlive(t, code))
}

// 引用不存在字典的 replace：锁内确认存在必须拒绝（400），且 CAS 递增随事务
// 回滚——不留字段行、父类型版本不变；字典随后建成即可正常替换（不误伤）。
func TestDictFieldRefMutex_ReplaceMissingDictRejected(t *testing.T) {
	ctx := context.Background()
	ticketRepo := repository.NewTicketRepo(testPool)
	dictRepo := repository.NewDictRepo(testPool)

	missing := "dfmx" + uniqueSuffix()
	tcode := "dfmt" + uniqueSuffix()
	ttID, ttVer := insertTicketTypeForMutex(t, tcode)
	defer cleanupDictMutexFixture(t, missing, tcode)

	fields := []model.TicketTypeField{dictRefSelectField(missing)}
	err := ticketRepo.ReplaceTypeFields(ctx, tcode, fields, &ttVer, ttID)
	require.Error(t, err, "锁内确认字典存在：引用缺失字典的 replace 须拒")
	var biz *errcode.Error
	require.ErrorAs(t, err, &biz)
	require.Equal(t, errcode.ErrInvalidParams.Code, biz.Code)

	var version int
	require.NoError(t, testPool.QueryRow(ctx,
		`SELECT version FROM ticket_types WHERE code = $1`, tcode).Scan(&version))
	require.Equal(t, ttVer, version, "CAS 递增须随事务回滚")
	var fieldRows int64
	require.NoError(t, testPool.QueryRow(ctx,
		`SELECT COUNT(*) FROM ticket_type_fields WHERE type_code = $1`, tcode).Scan(&fieldRows))
	require.Zero(t, fieldRows, "拒绝的 replace 不得残留字段行")

	// 字典随后建成即可替换成功
	createDictTypeForMutex(t, dictRepo, missing)
	require.NoError(t, ticketRepo.ReplaceTypeFields(ctx, tcode, fields, &version, ttID))
	require.Equal(t, int64(1), countDictSpecRefs(t, missing))
}

// 删除侧既有边界保留：409（ID/version 不匹配）、404（不存在）、命中即删。
func TestDictFieldRefMutex_DeleteBoundariesPreserved(t *testing.T) {
	ctx := context.Background()
	dictRepo := repository.NewDictRepo(testPool)

	code := "dfm" + uniqueSuffix()
	dictID, _ := createDictTypeForMutex(t, dictRepo, code)
	defer cleanupDictMutexFixture(t, code, code)

	require.ErrorIs(t, dictRepo.DeleteTypeByCode(ctx, code,
		repository.DictTypeDeleteTarget{ID: dictID + 1, Version: 1}), repository.ErrDictDeleteConflict)
	require.ErrorIs(t, dictRepo.DeleteTypeByCode(ctx, code,
		repository.DictTypeDeleteTarget{ID: dictID, Version: 2}), repository.ErrDictDeleteConflict)
	require.True(t, dictTypeAlive(t, code), "409 后字典必须存活")

	require.ErrorIs(t, dictRepo.DeleteTypeByCode(ctx, "dfmnope"+uniqueSuffix()),
		repository.ErrDictTypeNotFound)

	require.NoError(t, dictRepo.DeleteTypeByCode(ctx, code,
		repository.DictTypeDeleteTarget{ID: dictID, Version: 1}))
	require.False(t, dictTypeAlive(t, code))
}

// 存量的非规范选项保留原拒删语义，移除该配置后仍可正常删除。
func TestDictFieldRefMutex_HistoricalAmbiguousReferenceGuarded(t *testing.T) {
	ctx := context.Background()
	dictRepo := repository.NewDictRepo(testPool)
	code := "dfm" + uniqueSuffix()
	tcode := "dfmt" + uniqueSuffix()
	createDictTypeForMutex(t, dictRepo, code)
	_, _ = insertTicketTypeForMutex(t, tcode)
	defer cleanupDictMutexFixture(t, code, tcode)

	_, err := testPool.Exec(ctx, `
		INSERT INTO ticket_type_fields (type_code, field_key, field_label, field_type, field_options, required, sort_order)
		VALUES ($1, 'amb_in', '历史字段', 'input', $2::jsonb, false, 1),
		       ($1, 'amb_list', '纯值选项', 'select', '["a","b"]', false, 2)`, tcode, `["dict:`+code+`"]`)
	require.NoError(t, err)
	require.ErrorIs(t, dictRepo.DeleteTypeByCode(ctx, code), repository.ErrDictTypeInUse)
	require.True(t, dictTypeAlive(t, code))
	_, err = testPool.Exec(ctx, `DELETE FROM ticket_type_fields WHERE type_code=$1 AND field_key='amb_in'`, tcode)
	require.NoError(t, err)
	require.NoError(t, dictRepo.DeleteTypeByCode(ctx, code))
	require.False(t, dictTypeAlive(t, code))
}

// 既有保守引用谓词保留：三类规范内元素精确匹配（含多元素混合形态）仍拦截删除。
func TestDictFieldRefMutex_ConservativeMultiElementDictFormGuarded(t *testing.T) {
	ctx := context.Background()
	dictRepo := repository.NewDictRepo(testPool)

	code := "dfm" + uniqueSuffix()
	tcode := "dfmt" + uniqueSuffix()
	createDictTypeForMutex(t, dictRepo, code)
	_, _ = insertTicketTypeForMutex(t, tcode)
	defer cleanupDictMutexFixture(t, code, tcode)

	_, err := testPool.Exec(ctx, `
		INSERT INTO ticket_type_fields (type_code, field_key, field_label, field_type, field_options, required, sort_order)
		VALUES ($1, 'amb_mix', '混合', 'select', $2::jsonb, false, 1)`,
		tcode, `["dict:`+code+`","x"]`)
	require.NoError(t, err)
	require.Equal(t, int64(1), countDictSpecRefs(t, code))

	require.ErrorIs(t, dictRepo.DeleteTypeByCode(ctx, code), repository.ErrDictTypeInUse,
		"三类规范内 dict: 元素（含多元素混合形态）按既有保守谓词须拒删")
	require.True(t, dictTypeAlive(t, code))
}

// ─── 并发互斥（每方向 8 轮变异） ───

// replace（FOR KEY SHARE 写引用）× delete（FOR UPDATE 删类型）双向交错——
// 无论谁先认领，不变量恒成立：三类规范的 dict:<code> 引用零悬空。
//   - replace 先提交 → delete 锁后重扫看到引用 → 92304 拒删（字典存活）；
//   - delete 先提交 → replace 锁内确认存在失败 → 400 拒写（引用不落库）。
func TestDictFieldRefMutex_ReplaceDeleteConcurrency(t *testing.T) {
	ctx := context.Background()
	ticketRepo := repository.NewTicketRepo(testPool)
	dictRepo := repository.NewDictRepo(testPool)

	const roundsEach = 8
	idx := 0
	for _, replaceFirst := range []bool{true, false} {
		for r := 0; r < roundsEach; r++ {
			code := fmt.Sprintf("dfmr%d", idx)
			tcode := fmt.Sprintf("dfmtr%d", idx)
			idx++

			dictID, _ := createDictTypeForMutex(t, dictRepo, code)
			ttID, ttVer := insertTicketTypeForMutex(t, tcode)

			fields := []model.TicketTypeField{dictRefSelectField(code)}
			target := repository.DictTypeDeleteTarget{ID: dictID, Version: 1}
			roundCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
			var repErr, delErr error
			replaceFn := func() { repErr = ticketRepo.ReplaceTypeFields(roundCtx, tcode, fields, &ttVer, ttID) }
			deleteFn := func() { delErr = dictRepo.DeleteTypeByCode(roundCtx, code, target) }
			var wg sync.WaitGroup
			wg.Add(2)
			start := make(chan struct{})
			launch := func() {
				<-start
				if replaceFirst {
					replaceFn()
				} else {
					deleteFn()
				}
				wg.Done()
			}
			launch2 := func() {
				<-start
				if replaceFirst {
					deleteFn()
				} else {
					replaceFn()
				}
				wg.Done()
			}
			go launch()
			go launch2()
			close(start)
			wg.Wait()
			cancel()

			refs := countDictSpecRefs(t, code)
			alive := dictTypeAlive(t, code)

			// 不变量：三类规范引用存在 ⇒ 字典存活（零悬空）
			require.True(t, refs == 0 || alive,
				"第 %d 轮（replaceFirst=%v）出现悬空 dict 引用（replace=%v delete=%v）",
				idx-1, replaceFirst, repErr, delErr)

			if repErr == nil {
				require.ErrorIs(t, delErr, repository.ErrDictTypeInUse,
					"引用已提交，delete 锁后重扫必须拒删（第 %d 轮 replaceFirst=%v）", idx-1, replaceFirst)
				require.True(t, alive)
				require.Equal(t, int64(1), refs)
			} else {
				require.NoError(t, delErr,
					"replace 已被拒（引用未落库），delete 必须成功（第 %d 轮 replaceFirst=%v）", idx-1, replaceFirst)
				require.False(t, alive)
				require.Zero(t, refs)
				var biz *errcode.Error
				require.ErrorAs(t, repErr, &biz)
				require.Equal(t, errcode.ErrInvalidParams.Code, biz.Code)
			}

			cleanupDictMutexFixture(t, code, tcode)
		}
	}
}
