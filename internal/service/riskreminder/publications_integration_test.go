//go:build integration

// 发布读取面专项红绿（真实 PG + 完全合成可控钟）：三态合同（有效发布/无条目/
// 失配可见错误）、发布键经真实 BoundTicketDefinitionTx 验哈希、绑定 resolver 的
// 运行 job 端到端恰好一次投递。
package riskreminder

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

type publicationFixture struct {
	*fixture
	Ref  model.TicketDefinitionRef
	Role string
}

// publicationMeta 构造标准发布条目（待接单状态=assigned）。
func publicationMeta(ruleID string, ruleVersion, minutes int64) map[string]any {
	return map[string]any{
		"assigned": map[string]any{
			"first_accept_reminder": map[string]any{
				"rule_id": ruleID, "rule_version": ruleVersion, "accept_within_minutes": minutes,
			},
		},
	}
}

// newPublicationFixture 钉版 v1 定义（transition_meta 由用例给定）、票面钉版且
// 处于待接单状态，收件人 P 具备投递所需的现势路由资格。
func newPublicationFixture(t *testing.T, meta map[string]any) *publicationFixture {
	return newPublicationDefinitionWithHash(t, meta, "")
}

// newPublicationDefinitionWithHash 以给定 definition_hash 插入钉版 v1
// （hashOverride 非空时存储失配哈希——发布键失配夹具；发布行不可 UPDATE，
// 失配只能在 INSERT 侧构造）。
func newPublicationDefinitionWithHash(t *testing.T, meta map[string]any, hashOverride string) *publicationFixture {
	t.Helper()
	f := &publicationFixture{fixture: createFixture(t)}
	f.Role = "pub_role_" + randSuffix()
	ctx := context.Background()
	snapshot, err := json.Marshal(map[string]any{
		"type": map[string]any{
			"code": f.TypeCode, "name": "synthetic publication",
			"states":          []string{"open", "assigned", "in_progress", "closed"},
			"transitions":     map[string][]string{"open": {"assigned"}, "assigned": {"in_progress"}, "in_progress": {"closed"}},
			"transition_meta": meta, "default_sla_hours": 48, "has_custom_fields": false,
		},
		"fields": []any{},
	})
	require.NoError(t, err)
	f.Ref = model.TicketDefinitionRef{TypeCode: f.TypeCode, DefinitionVersion: 1}
	var row pgx.Row
	if hashOverride != "" {
		row = testPool.QueryRow(ctx, `INSERT INTO ticket_type_definitions(type_code,definition_version,definition_hash,schema_format_version,snapshot,source_draft_revision,published_by)
			VALUES($1,1,$2::text,1,$3,1,$4) RETURNING definition_hash`, f.TypeCode, hashOverride, snapshot, f.M)
	} else {
		row = testPool.QueryRow(ctx, `INSERT INTO ticket_type_definitions(type_code,definition_version,definition_hash,schema_format_version,snapshot,source_draft_revision,published_by)
			VALUES($1,1,ticket_definition_snapshot_hash($2::jsonb),1,$2,1,$3) RETURNING definition_hash`, f.TypeCode, snapshot, f.M)
	}
	require.NoError(t, row.Scan(&f.Ref.DefinitionHash))
	_, err = testPool.Exec(ctx, `UPDATE ticket_types SET current_published_version=1 WHERE code=$1`, f.TypeCode)
	require.NoError(t, err)
	_, err = testPool.Exec(ctx, `UPDATE tickets SET assigned_to=$2,status='assigned',definition_version=1 WHERE id=$1`, f.TicketID, f.P)
	require.NoError(t, err)
	addVisibleRiskLink(t, f.fixture, f.Role)
	require.NoError(t, testPool.QueryRow(ctx, `INSERT INTO roles(code,name,priority) VALUES($1,'synthetic publication role',100) RETURNING code`, f.Role).Scan(new(string)))
	_, err = testPool.Exec(ctx, `INSERT INTO user_roles(user_id,role_id) SELECT $1,id FROM roles WHERE code=$2`, f.P, f.Role)
	require.NoError(t, err)
	_, err = testPool.Exec(ctx, `INSERT INTO casbin_rule(ptype,v0,v1,v2) VALUES('p',$1,'/api/v1/tickets','GET'),('p',$1,'/api/v1/tickets/:id','GET'),('p',$1,'/api/v1/tickets/transition','POST'),('p',$1,'/api/v1/risk-items/remediation-accept-assignment','POST')`, "role::"+f.Role)
	require.NoError(t, err)
	return f
}

// readPolicyOnTx 纯读面：三态读取走调用方给定事务。
func readPolicyOnTx(t *testing.T, ticketID int64) (*AcceptancePolicy, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	tx, err := testPool.Begin(ctx)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback(ctx) }()
	return BoundAcceptancePolicyTx(ctx, testPool, tx, ticketID)
}

func TestBoundPublicationThreeStates(t *testing.T) {
	t.Run("published", func(t *testing.T) {
		f := newPublicationFixture(t, publicationMeta("first_accept", 3, 30))
		policy, err := readPolicyOnTx(t, f.TicketID)
		require.NoError(t, err)
		require.Equal(t, f.Ref, policy.DefinitionRef) // 发布键=type_code+钉版版本+真实哈希
		require.Equal(t, "first_accept", policy.RuleID)
		require.EqualValues(t, 3, policy.RuleVersion)
		require.Equal(t, "assigned", policy.AwaitingState)
		require.Equal(t, 30*time.Minute, policy.AcceptWithin)
	})
	t.Run("absent", func(t *testing.T) {
		f := newPublicationFixture(t, map[string]any{})
		_, err := readPolicyOnTx(t, f.TicketID)
		require.ErrorIs(t, err, ErrReminderPolicyAbsent)
	})
	t.Run("resolver maps absent to nil policy", func(t *testing.T) {
		f := newPublicationFixture(t, map[string]any{})
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		require.NoError(t, repository.WithIAMCommandGuard(ctx, testPool, func(ctx context.Context, tx pgx.Tx) error {
			policy, err := NewBoundPolicyResolver(testPool)(ctx, tx, f.TicketID)
			require.NoError(t, err)
			require.Nil(t, policy) // checker 收敛为 ErrPolicyUnavailable——留 pending 可恢复
			return nil
		}))
	})
	t.Run("tampered hash is visible failure not absent", func(t *testing.T) {
		f := newPublicationDefinitionWithHash(t, publicationMeta("first_accept", 1, 30), "tampered")
		_, err := readPolicyOnTx(t, f.TicketID)
		require.ErrorIs(t, err, errcode.ErrServiceUnavailable)
		require.NotErrorIs(t, err, ErrReminderPolicyAbsent)
	})
	t.Run("unpinned version is visible failure", func(t *testing.T) {
		f := newPublicationFixture(t, publicationMeta("first_accept", 1, 30))
		_, err := testPool.Exec(context.Background(), `UPDATE tickets SET definition_version=NULL WHERE id=$1`, f.TicketID)
		require.NoError(t, err)
		_, err = readPolicyOnTx(t, f.TicketID)
		require.ErrorIs(t, err, errcode.ErrServiceUnavailable)
	})
	t.Run("malformed entries are visible failures", func(t *testing.T) {
		// 发布行不可变且存储图校验先行，域级损坏（非对象 transition_meta）无法经 DB
		// 到达解析面——该防御分支由 publications_unit_test 覆盖；此处只测可达分支。
		for name, meta := range map[string]map[string]any{
			"empty_entry":   {"assigned": map[string]any{"first_accept_reminder": map[string]any{}}},
			"zero_version":  publicationMeta("first_accept", 0, 30),
			"zero_minutes":  publicationMeta("first_accept", 1, 0),
			"empty_rule_id": publicationMeta("", 1, 30),
			"duplicated":    {"assigned": publicationMeta("r", 1, 30)["assigned"], "open": publicationMeta("r", 1, 30)["assigned"]},
			"unknown_state": {"waiting": map[string]any{"first_accept_reminder": map[string]any{"rule_id": "r", "rule_version": 1, "accept_within_minutes": 30}}},
		} {
			t.Run(name, func(t *testing.T) {
				f := newPublicationFixture(t, meta)
				_, err := readPolicyOnTx(t, f.TicketID)
				require.Error(t, err)
				require.NotErrorIs(t, err, ErrReminderPolicyAbsent)
			})
		}
	})
	t.Run("missing ticket", func(t *testing.T) {
		f := newPublicationFixture(t, publicationMeta("first_accept", 1, 30))
		_, err := readPolicyOnTx(t, f.TicketID+1000000)
		require.ErrorIs(t, err, errcode.ErrNotFound)
	})
}

// 运行 job 接法：NewRuntime + NewBoundPolicyResolver（真实存储发布，非合成策略）
// 端到端——适配器分派 → 截止前零投递 → 09:36 恰好一次入箱 → 重跑零新增。
func TestRuntimeJobDeliversOnceWithBoundResolver(t *testing.T) {
	f := newPublicationFixture(t, publicationMeta("first_accept", 1, 30))
	svc, job, err := NewRuntime(testPool, repository.NewNotificationInboxRepo(testPool), eligibilityModel,
		NewBoundPolicyResolver(testPool), func() time.Time { return tGen }, nil)
	require.NoError(t, err)

	a, recorded, err := configuredDispatch(t, svc, f.fixture, f.TicketID, f.P, f.M, tDispatch)
	require.NoError(t, err)
	require.True(t, recorded)
	require.EqualValues(t, 1, a.Generation)
	require.Equal(t, tDue.UTC(), a.AcceptDueAt.UTC()) // 截止=分派时刻+发布 30 分钟，不取票面 SLA

	// 09:34：未到截止——零生成零投递
	before, err := job.RunOnce(context.Background(), tDue.Add(-time.Minute))
	require.NoError(t, err)
	require.Empty(t, before.Scan.Generated)
	require.Empty(t, before.Delivery.Delivered)
	require.Zero(t, inboxCount(t, f.P))

	// 09:36：生成 + 投递恰好一次
	atDue, err := job.RunOnce(context.Background(), tGen)
	require.NoError(t, err)
	require.Len(t, atDue.Scan.Generated, 1)
	require.Len(t, atDue.Delivery.Delivered, 1)
	require.EqualValues(t, 1, inboxCount(t, f.P))

	// 重跑（重启/重扫）：原键去重，零新增
	again, err := job.RunOnce(context.Background(), tGen)
	require.NoError(t, err)
	require.Empty(t, again.Scan.Generated)
	require.Empty(t, again.Delivery.Delivered)
	require.Empty(t, again.Delivery.Suppressed)
	require.Empty(t, again.Delivery.Failed)
	require.EqualValues(t, 1, inboxCount(t, f.P))
}

// 发布键失配（钉版行哈希失配——INSERT 侧构造，发布行不可变）经绑定 resolver
// 在分派与扫描两侧都可见；修复（钉到内容一致的正确 v2）后同一原键恢复投递。
func TestRuntimeBoundResolverKeepsIntentRecoverable(t *testing.T) {
	meta := publicationMeta("first_accept", 1, 30)
	f := newPublicationDefinitionWithHash(t, meta, "tampered")
	svc, job, err := NewRuntime(testPool, repository.NewNotificationInboxRepo(testPool), eligibilityModel,
		NewBoundPolicyResolver(testPool), func() time.Time { return tGen }, nil)
	require.NoError(t, err)

	// 失配下分派：可见错误、零残留（不伪记分派事实）
	_, _, err = configuredDispatch(t, svc, f.fixture, f.TicketID, f.P, f.M, tDispatch)
	require.ErrorIs(t, err, errcode.ErrServiceUnavailable)
	require.Zero(t, countAssignments(t, f.TicketID))

	// 内核直落一份与发布内容一致的分派事实（模拟源失配前已入账），并生成待发
	require.NoError(t, repository.WithIAMCommandGuard(context.Background(), testPool, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT id FROM tickets WHERE id=$1 FOR UPDATE`, f.TicketID); err != nil {
			return err
		}
		_, err := RecordAssignmentTx(ctx, tx, &AssignmentInput{
			TicketID: f.TicketID, Generation: 1, AssigneeID: f.P, DispatchedBy: f.M,
			DispatchedAt: tDispatch, AcceptDueAt: tDue, RuleID: "first_accept", RuleVersion: 1,
		})
		return err
	}))

	// 失配源下运行 job：扫描资格失败可见，意图留 pending 带失败留痕——按原键可恢复
	run, err := job.RunOnce(context.Background(), tGen)
	require.Error(t, err) // 失配不可伪装为成功 tick
	require.Len(t, run.Scan.Failed, 1)
	list := intentsOf(t, f.fixture)
	require.Len(t, list, 1)
	require.Equal(t, StatusPending, list[0].Status)

	// 修复 = 钉到内容一致、哈希正确的 v2（发布行不可变，修复以新版本承载：
	// v1 快照内容本身正确，仅存储哈希失配）
	_, err = testPool.Exec(context.Background(), `INSERT INTO ticket_type_definitions(type_code,definition_version,definition_hash,schema_format_version,snapshot,source_draft_revision,published_by)
		SELECT type_code,2,ticket_definition_snapshot_hash(snapshot),schema_format_version,snapshot,source_draft_revision,published_by
		FROM ticket_type_definitions WHERE type_code=$1 AND definition_version=1`, f.TypeCode)
	require.NoError(t, err)
	_, err = testPool.Exec(context.Background(), `UPDATE tickets SET definition_version=2 WHERE id=$1`, f.TicketID)
	require.NoError(t, err)

	recovered, err := job.RunOnce(context.Background(), tGen)
	require.NoError(t, err)
	require.Len(t, recovered.Delivery.Delivered, 1)
	require.EqualValues(t, 1, inboxCount(t, f.P))
	after := intentsOf(t, f.fixture)
	require.Len(t, after, 1)
	require.Equal(t, StatusDelivered, after[0].Status)
}
