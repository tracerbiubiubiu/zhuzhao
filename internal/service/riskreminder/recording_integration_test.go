//go:build integration

// 分派/接单记录面专项红绿（真实 PG + 完全合成可控钟）：已锁票下代次 1/MAX+1、
// 固定截止、同键重放只回原行（同键只看当前代次）、仅当前代次+本人+合法实际接单边、
// 刷新/读/通知不构成接单、同事务精确读（单连接池）、锁边界（读不取票锁/写按序排在票锁后）、
// 故障随事务回滚零残留。
package riskreminder

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

// configuredDispatch 模拟分派调用方：守卫事务 + 票行锁内执行适配器；
// 适配器错误作为业务事务失败原样上抛（守卫回滚——零残留）。
func configuredDispatch(t *testing.T, svc *Service, f *fixture, ticketID, assigneeID, actorID int64, asOf time.Time) (*Assignment, bool, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var a *Assignment
	var recorded bool
	guardErr := repository.WithIAMCommandGuard(ctx, testPool, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT id FROM tickets WHERE id=$1 FOR UPDATE`, ticketID); err != nil {
			return err
		}
		var runErr error
		a, recorded, runErr = svc.RecordConfiguredAssignment(ctx, tx, ticketID, assigneeID, actorID, asOf)
		return runErr
	})
	return a, recorded, guardErr
}

// configuredAccept 模拟接单调用方：守卫事务 + 票行锁内先落接单边的票态写入
// （mutate），再执行适配器；错误原样上抛（守卫回滚——零残留）。
func configuredAccept(t *testing.T, svc *Service, f *fixture, ticketID, acceptorID int64, asOf time.Time, mutate func(context.Context, pgx.Tx) error) (*Assignment, bool, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var a *Assignment
	var recorded bool
	guardErr := repository.WithIAMCommandGuard(ctx, testPool, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT id FROM tickets WHERE id=$1 FOR UPDATE`, ticketID); err != nil {
			return err
		}
		if mutate != nil {
			if err := mutate(ctx, tx); err != nil {
				return err
			}
		}
		var runErr error
		a, recorded, runErr = svc.RecordConfiguredAcceptance(ctx, tx, ticketID, acceptorID, asOf)
		return runErr
	})
	return a, recorded, guardErr
}

// acceptOnEdge 接单边的票态写入（assigned→in_progress），在调用方事务内落。
func acceptOnEdge(ticketID int64) func(context.Context, pgx.Tx) error {
	return func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE tickets SET status='in_progress' WHERE id=$1`, ticketID)
		return err
	}
}

func countAssignments(t *testing.T, ticketID int64) int64 {
	t.Helper()
	var n int64
	mustScan(t, testPool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM riskreminder_assignments WHERE ticket_id=$1`, ticketID), &n)
	return n
}

func readGeneration(t *testing.T, ticketID, generation int64) *Assignment {
	t.Helper()
	var a Assignment
	mustScan(t, testPool.QueryRow(context.Background(), `SELECT `+assignmentCols+`
		FROM riskreminder_assignments WHERE ticket_id=$1 AND generation=$2`, ticketID, generation),
		&a.ID, &a.TicketID, &a.Generation, &a.AssigneeID, &a.DispatchedBy,
		&a.DispatchedAt, &a.AcceptDueAt, &a.RuleID, &a.RuleVersion, &a.AcceptedAt, &a.AcceptedBy)
	return &a
}

// 分派记录：代次 1/MAX+1、固定截止、发布参数落事实；同键重放（同事实）只回原行；
// 与历史代次巧合同事实的重分派是新动作；非法输入与裸事务拒绝。
func TestConfiguredAssignmentGenerationsAndReplay(t *testing.T) {
	f := newPublicationFixture(t, publicationMeta("first_accept", 3, 30))
	svc := newService(nil)

	a, recorded, err := configuredDispatch(t, svc, f.fixture, f.TicketID, f.P, f.M, tDispatch)
	require.NoError(t, err)
	require.True(t, recorded)
	require.EqualValues(t, 1, a.Generation)
	require.Equal(t, tDue.UTC(), a.AcceptDueAt.UTC())
	require.Equal(t, tDispatch.UTC(), a.DispatchedAt.UTC())
	require.Equal(t, "first_accept", a.RuleID)
	require.EqualValues(t, 3, a.RuleVersion)
	require.Equal(t, f.M, a.DispatchedBy)

	// 同键重放：完全同事实重提 → 原行原时刻，不新增动作
	a2, recorded2, err := configuredDispatch(t, svc, f.fixture, f.TicketID, f.P, f.M, tDispatch)
	require.NoError(t, err)
	require.False(t, recorded2)
	require.Equal(t, a.ID, a2.ID)
	require.Equal(t, a.DispatchedAt.UTC(), a2.DispatchedAt.UTC())
	require.EqualValues(t, 1, countAssignments(t, f.TicketID))

	// 不同 asOf 的重分派 → 新动作新代次（截止随新分派时刻固定）
	t2 := tDispatch.Add(time.Hour)
	a3, recorded3, err := configuredDispatch(t, svc, f.fixture, f.TicketID, f.P, f.M, t2)
	require.NoError(t, err)
	require.True(t, recorded3)
	require.EqualValues(t, 2, a3.Generation)
	require.Equal(t, t2.Add(30*time.Minute).UTC(), a3.AcceptDueAt.UTC())
	require.EqualValues(t, 2, countAssignments(t, f.TicketID))

	// 同键重放只看当前代次：再提 gen1 事实（非当前代次）= 新动作 gen3
	a4, recorded4, err := configuredDispatch(t, svc, f.fixture, f.TicketID, f.P, f.M, tDispatch)
	require.NoError(t, err)
	require.True(t, recorded4)
	require.EqualValues(t, 3, a4.Generation)
	require.EqualValues(t, 3, countAssignments(t, f.TicketID))

	// 非法输入（无空变量）：零 asOf/零 ID → ErrInvalidParams
	_, _, err = configuredDispatch(t, svc, f.fixture, f.TicketID, f.P, f.M, time.Time{})
	require.ErrorIs(t, err, errcode.ErrInvalidParams)
	_, _, err = configuredDispatch(t, svc, f.fixture, f.TicketID, 0, f.M, tDispatch)
	require.ErrorIs(t, err, errcode.ErrInvalidParams)

	// 裸事务（无 IAM 守卫）拒绝，零残留
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	tx, err := testPool.Begin(ctx)
	require.NoError(t, err)
	_, _, err = svc.RecordConfiguredAssignment(ctx, tx, f.TicketID, f.P, f.M, tDispatch)
	require.ErrorIs(t, err, errcode.ErrNoPermission)
	require.NoError(t, tx.Rollback(ctx))
	require.EqualValues(t, 3, countAssignments(t, f.TicketID))
}

// 无有效发布条目 → (nil,false,nil) 零提醒事实；条目存在但钉版哈希失配/未钉版/
// 条目损坏 → 可见错误，事务回滚零残留（回调故障不留半状态）。
func TestConfiguredAssignmentPublicationGates(t *testing.T) {
	t.Run("absent publication records nothing", func(t *testing.T) {
		f := newPublicationFixture(t, map[string]any{})
		svc := newService(nil)
		a, recorded, err := configuredDispatch(t, svc, f.fixture, f.TicketID, f.P, f.M, tDispatch)
		require.NoError(t, err)
		require.Nil(t, a)
		require.False(t, recorded)
		require.Zero(t, countAssignments(t, f.TicketID))
	})
	t.Run("tampered hash fails visibly with zero residue", func(t *testing.T) {
		f := newPublicationDefinitionWithHash(t, publicationMeta("first_accept", 1, 30), "tampered")
		svc := newService(nil)
		a, recorded, err := configuredDispatch(t, svc, f.fixture, f.TicketID, f.P, f.M, tDispatch)
		require.ErrorIs(t, err, errcode.ErrServiceUnavailable)
		require.Nil(t, a)
		require.False(t, recorded)
		require.Zero(t, countAssignments(t, f.TicketID)) // 失败随事务回滚，不留半状态
	})
	t.Run("broken entry fails visibly with zero residue", func(t *testing.T) {
		f := newPublicationFixture(t, publicationMeta("first_accept", 0, 30))
		svc := newService(nil)
		_, _, err := configuredDispatch(t, svc, f.fixture, f.TicketID, f.P, f.M, tDispatch)
		require.Error(t, err)
		require.NotErrorIs(t, err, ErrReminderPolicyAbsent)
		require.Zero(t, countAssignments(t, f.TicketID))
	})
}

// 接单记录：合法实际接单边（assigned→in_progress）+ 本人 + 当前代次才落事实；
// 票仍在待接单状态（刷新/读/通知模拟）→ ErrNotAcceptanceEdge；非本人拒绝；
// 重放返回原行原时刻（票继续流转亦然）；旧代次不记、当前代次唯一落事实。
func TestConfiguredAcceptanceRecordsOnlyLegalSelfEdge(t *testing.T) {
	f := newPublicationFixture(t, publicationMeta("first_accept", 1, 30))
	svc := newService(nil)
	_, recorded, err := configuredDispatch(t, svc, f.fixture, f.TicketID, f.P, f.M, tDispatch)
	require.NoError(t, err)
	require.True(t, recorded)
	_, err = svc.GenerateDue(context.Background(), tGen, 10) // 09:36 生成待发意图
	require.NoError(t, err)

	// 刷新/读/通知模拟：票仍在 assigned（无接单边）→ 显式拒绝，不记接单
	_, _, err = configuredAccept(t, svc, f.fixture, f.TicketID, f.P, tAccept, nil)
	require.ErrorIs(t, err, ErrNotAcceptanceEdge)
	list := intentsOf(t, f.fixture)
	require.Len(t, list, 1)
	require.Equal(t, StatusPending, list[0].Status) // 意图未受影响
	require.Zero(t, countAssignmentsAccepted(t, f.TicketID))

	// 非本人（P2）即便沿合法边也拒绝
	_, _, err = configuredAccept(t, svc, f.fixture, f.TicketID, f.P2, tAccept, acceptOnEdge(f.TicketID))
	require.ErrorIs(t, err, errcode.ErrNoPermission)
	require.Zero(t, countAssignmentsAccepted(t, f.TicketID))

	// 本人 + 合法实际接单边 → 记录当前代次接单事实并抑止待发
	a, recorded, err := configuredAccept(t, svc, f.fixture, f.TicketID, f.P, tAccept, acceptOnEdge(f.TicketID))
	require.NoError(t, err)
	require.True(t, recorded)
	require.Equal(t, tAccept.UTC(), a.AcceptedAt.UTC())
	require.Equal(t, f.P, *a.AcceptedBy)
	list = intentsOf(t, f.fixture)
	require.Len(t, list, 1)
	require.Equal(t, StatusSuppressed, list[0].Status)
	require.NotNil(t, list[0].SuppressReason)
	require.Equal(t, ReasonAccepted, *list[0].SuppressReason)
	require.Zero(t, inboxCount(t, f.P)) // 抑止非投递——零入箱

	// 重放：原时间事实唯一，不覆盖；票继续流转到 closed 后重放仍回原行
	a2, recorded2, err := configuredAccept(t, svc, f.fixture, f.TicketID, f.P, tAccept.Add(5*time.Minute), nil)
	require.NoError(t, err)
	require.False(t, recorded2)
	require.Equal(t, tAccept.UTC(), a2.AcceptedAt.UTC())
	_, err = testPool.Exec(context.Background(), `UPDATE tickets SET status='closed' WHERE id=$1`, f.TicketID)
	require.NoError(t, err)
	a3, recorded3, err := configuredAccept(t, svc, f.fixture, f.TicketID, f.P, tAccept.Add(6*time.Minute), nil)
	require.NoError(t, err)
	require.False(t, recorded3)
	require.Equal(t, a.ID, a3.ID)
	require.Equal(t, tAccept.UTC(), a3.AcceptedAt.UTC())
}

// 仅当前代次：改派后旧代次被分派人不得接单；新代次接单只落当前代次事实。
func TestConfiguredAcceptanceCurrentGenerationOnly(t *testing.T) {
	f := newPublicationFixture(t, publicationMeta("first_accept", 1, 30))
	svc := newService(nil)
	_, _, err := configuredDispatch(t, svc, f.fixture, f.TicketID, f.P, f.M, tDispatch)
	require.NoError(t, err)
	t2 := tDispatch.Add(time.Hour)
	_, _, err = configuredDispatch(t, svc, f.fixture, f.TicketID, f.P2, f.M, t2)
	require.NoError(t, err)
	// 模拟改派后的票面事实：主责已换 P2、票回待接单
	_, err = testPool.Exec(context.Background(), `UPDATE tickets SET assigned_to=$2, status='assigned' WHERE id=$1`, f.TicketID, f.P2)
	require.NoError(t, err)

	// 旧代次被分派人 P 在合法边上接单 → 非当前代次本人，拒绝
	_, _, err = configuredAccept(t, svc, f.fixture, f.TicketID, f.P, tAccept, acceptOnEdge(f.TicketID))
	require.ErrorIs(t, err, errcode.ErrNoPermission)

	// 新代次本人 P2 接单 → 只记 gen2
	asOf := t2.Add(30 * time.Minute)
	a, recorded, err := configuredAccept(t, svc, f.fixture, f.TicketID, f.P2, asOf, acceptOnEdge(f.TicketID))
	require.NoError(t, err)
	require.True(t, recorded)
	require.EqualValues(t, 2, a.Generation)
	require.Equal(t, asOf.UTC(), a.AcceptedAt.UTC())
	require.Nil(t, readGeneration(t, f.TicketID, 1).AcceptedAt) // 旧代次事实不动
	require.NotNil(t, readGeneration(t, f.TicketID, 2).AcceptedAt)
}

// 无发布 → 接单 no-op；有发布但无分派事实（调用序错误）→ ErrNotFound。
func TestConfiguredAcceptanceGates(t *testing.T) {
	t.Run("absent publication is a no-op", func(t *testing.T) {
		f := newPublicationFixture(t, map[string]any{})
		svc := newService(nil)
		a, recorded, err := configuredAccept(t, svc, f.fixture, f.TicketID, f.P, tAccept, acceptOnEdge(f.TicketID))
		require.NoError(t, err)
		require.Nil(t, a)
		require.False(t, recorded)
		require.Zero(t, countAssignments(t, f.TicketID))
	})
	t.Run("acceptance without assignment is not found", func(t *testing.T) {
		f := newPublicationFixture(t, publicationMeta("first_accept", 1, 30))
		svc := newService(nil)
		_, _, err := configuredAccept(t, svc, f.fixture, f.TicketID, f.P, tAccept, acceptOnEdge(f.TicketID))
		require.ErrorIs(t, err, errcode.ErrNotFound)
	})
	t.Run("unguarded tx rejected", func(t *testing.T) {
		f := newPublicationFixture(t, publicationMeta("first_accept", 1, 30))
		svc := newService(nil)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		tx, err := testPool.Begin(ctx)
		require.NoError(t, err)
		_, _, err = svc.RecordConfiguredAcceptance(ctx, tx, f.TicketID, f.P, tAccept)
		require.ErrorIs(t, err, errcode.ErrNoPermission)
		require.NoError(t, tx.Rollback(ctx))
	})
}

// 同事务边界（单连接池）：适配器在同守卫事务的未提交票态上判定接单边并落事实；
// 任何隐藏池内第二连接都会自死锁——测试超时即失败。
func TestRecordingRunsOnExactGuardTransaction(t *testing.T) {
	f := newPublicationFixture(t, publicationMeta("first_accept", 1, 30))
	arrange := newService(nil)
	_, _, err := configuredDispatch(t, arrange, f.fixture, f.TicketID, f.P, f.M, tDispatch)
	require.NoError(t, err)
	_, err = arrange.GenerateDue(context.Background(), tGen, 10)
	require.NoError(t, err)

	cfg := testPool.Config()
	cfg.MaxConns = 1
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	require.NoError(t, err)
	defer pool.Close()
	svc := NewService(pool, repository.NewNotificationInboxRepo(pool), nil, slog.New(slog.NewTextHandler(os.Stderr, nil)))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	require.NoError(t, repository.WithIAMCommandGuard(ctx, pool, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT id FROM tickets WHERE id=$1 FOR UPDATE`, f.TicketID); err != nil {
			return err
		}
		// 同事务未提交票态：assigned→in_progress——适配器必须在本事务上读到
		if _, err := tx.Exec(ctx, `UPDATE tickets SET status='in_progress' WHERE id=$1`, f.TicketID); err != nil {
			return err
		}
		a, recorded, err := svc.RecordConfiguredAcceptance(ctx, tx, f.TicketID, f.P, tAccept)
		if err != nil {
			return err
		}
		if !recorded {
			return errors.New("same-tx acceptance edge not recorded")
		}
		if !a.AcceptedAt.UTC().Equal(tAccept) {
			return errors.New("same-tx accepted_at drift")
		}
		return nil
	}))
	a := readGeneration(t, f.TicketID, 1)
	require.NotNil(t, a.AcceptedAt)
	require.Equal(t, tAccept.UTC(), a.AcceptedAt.UTC())
	list := intentsOf(t, f.fixture)
	require.Len(t, list, 1)
	require.Equal(t, StatusSuppressed, list[0].Status)
}

// 锁边界：读取路径不取票锁（持票锁者不阻塞读取判定）；提醒写入按票→提醒行锁序
// 排在持票者之后（提醒行 INSERT 的 FK 键共享检查等待票锁释放——写入不得越过票锁）。
func TestConfiguredRecordingLockBoundary(t *testing.T) {
	t.Run("read path takes no ticket lock", func(t *testing.T) {
		f := newPublicationFixture(t, map[string]any{}) // 无发布——适配器只读即可判定
		svc := newService(nil)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		holder, err := testPool.Begin(ctx)
		require.NoError(t, err)
		defer func() { _ = holder.Rollback(ctx) }()
		_, err = holder.Exec(ctx, `SELECT id FROM tickets WHERE id=$1 FOR UPDATE`, f.TicketID)
		require.NoError(t, err)

		var a *Assignment
		var recorded bool
		done := make(chan error, 1)
		go func() {
			done <- repository.WithIAMCommandGuard(ctx, testPool, func(ctx context.Context, tx pgx.Tx) error {
				var runErr error
				a, recorded, runErr = svc.RecordConfiguredAssignment(ctx, tx, f.TicketID, f.P, f.M, tDispatch)
				return runErr
			})
		}()
		select {
		case err := <-done: // 持票锁期间完成——读取路径未取票锁
			require.NoError(t, err)
		case <-ctx.Done():
			t.Fatal("adapter read blocked on caller ticket lock — lock boundary violated")
		}
		require.Nil(t, a)
		require.False(t, recorded)
	})

	t.Run("reminder write serializes behind ticket lock", func(t *testing.T) {
		f := newPublicationFixture(t, publicationMeta("first_accept", 1, 30))
		svc := newService(nil)
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		holder, err := testPool.Begin(ctx)
		require.NoError(t, err)
		defer func() { _ = holder.Rollback(ctx) }()
		var holderPID int32
		require.NoError(t, holder.QueryRow(ctx, `SELECT pg_backend_pid()::int4`).Scan(&holderPID))
		_, err = holder.Exec(ctx, `SELECT id FROM tickets WHERE id=$1 FOR UPDATE`, f.TicketID)
		require.NoError(t, err)

		adapterPID := make(chan int32, 1)
		done := make(chan error, 1)
		var recorded bool
		go func() {
			done <- repository.WithIAMCommandGuard(ctx, testPool, func(ctx context.Context, tx pgx.Tx) error {
				var pid int32
				if err := tx.QueryRow(ctx, `SELECT pg_backend_pid()::int4`).Scan(&pid); err != nil {
					return err
				}
				adapterPID <- pid
				// 调用方票锁（合同第一步）：持票者未释放前不得进入，更不得写提醒行
				if _, err := tx.Exec(ctx, `SELECT id FROM tickets WHERE id=$1 FOR UPDATE`, f.TicketID); err != nil {
					return err
				}
				var runErr error
				_, recorded, runErr = svc.RecordConfiguredAssignment(ctx, tx, f.TicketID, f.P, f.M, tDispatch)
				return runErr
			})
		}()
		var writerPID int32
		select {
		case writerPID = <-adapterPID:
		case <-ctx.Done():
			t.Fatal("adapter never entered its ticket-locked section")
		}
		// 票锁未释放前适配器进不了票锁之后任何一步（提醒行写入更在其后）——
		// 票→提醒行锁序实证：提醒事实不得越过票锁
		require.Eventually(t, func() bool {
			var blocked bool
			err := testPool.QueryRow(ctx,
				`SELECT $1::int = ANY(pg_blocking_pids($2))`, holderPID, writerPID).Scan(&blocked)
			return err == nil && blocked
		}, 3*time.Second, 10*time.Millisecond)
		require.NoError(t, holder.Commit(ctx)) // 票锁释放 → 适配器继续完成
		require.NoError(t, waitCh(t, done, 5*time.Second, "adapter completion"))
		require.True(t, recorded)
		require.EqualValues(t, 1, countAssignments(t, f.TicketID))
	})
}

func countAssignmentsAccepted(t *testing.T, ticketID int64) int64 {
	t.Helper()
	var n int64
	mustScan(t, testPool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM riskreminder_assignments WHERE ticket_id=$1 AND accepted_at IS NOT NULL`, ticketID), &n)
	return n
}
