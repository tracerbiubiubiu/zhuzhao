//go:build integration

// 并发正确性专项红绿（真实 PG + 可控屏障）：票面改派与投递提交全序化（票→意图锁序）、扫描同键排除防最老 N 条霸占配额、失败条目健康
// 优先防饥饿、提交确认不明按原意图读回报真实结果、时间精度幂等。
// 场景锚点：risk-module-detail §6 D15（撤权抑止旧待发、投递重试用原收件人
// 原键）+ §14-3（发送前核现势收件人资格）。
package riskreminder

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

// waitCh 带超时收通道（屏障用例防悬挂；超时即失败并说明等的是哪一环）。
func waitCh[T any](t *testing.T, ch <-chan T, d time.Duration, what string) T {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-time.After(d):
		t.Fatalf("timed out waiting for %s", what)
		var zero T
		return zero
	}
}

// assignTicket 落票面主责事实（读 assigned_to 判资格的用例先 arrange 这一步；
// 撤权用例的撤销动作即改此列——业务事务对 tickets 的 IAM 变更）。
func assignTicket(t *testing.T, ticketID, assignee int64) {
	t.Helper()
	if _, err := testPool.Exec(context.Background(),
		`UPDATE tickets SET assigned_to=$1 WHERE id=$2`, assignee, ticketID); err != nil {
		t.Fatalf("assign ticket: %v", err)
	}
}

// ─── 用例 ───

// 票面改派先于投递提交完成时，收件箱不得入箱（D15 撤权抑止）。
// 可控屏障：投递按「撤权前」快照读出资格后停住；撤权（票面主责转移，业务事务
// 对 tickets 的 IAM 变更）尝试先提交。不变量只有一条：
//
//	不存在「撤权先于投递提交完成，而收件箱仍入箱」。
//
// 分支一（撤权先提交成功）：投递放行后必须零入箱、不得报告已送达；
// 分支二（共同守卫挡住撤权）：撤权事务的撤销语句必须亲见投递已提交
// （witness=intent 已终态），投递先于撤权入箱属合法线性序。
func TestReassignmentCommitBeforeDeliveryCommitNeverInboxes(t *testing.T) {
	f := createFixture(t)
	assignTicket(t, f.TicketID, f.P) // 主责事实在先——checker 按票面判资格
	recordAssignment(t, f, 1, f.P, tDue)
	ctx := context.Background()

	once := &sync.Once{}
	checkerReached := make(chan struct{})
	allowChecker := make(chan struct{})
	release := func() { once.Do(func() { close(allowChecker) }) }
	svc := newService(func(ctx context.Context, tx pgx.Tx, ticketID, assigneeID int64) (bool, string, error) {
		var assignedTo *int64
		if err := tx.QueryRow(ctx, `SELECT assigned_to FROM tickets WHERE id=$1`, ticketID).Scan(&assignedTo); err != nil {
			return false, "", err
		}
		eligible := assignedTo != nil && *assignedTo == assigneeID
		close(checkerReached) // 资格已按当前快照读出——此后到提交前是易损窗口
		<-allowChecker
		return eligible, "", nil
	})
	if _, err := svc.GenerateDue(ctx, tGen, 10); err != nil {
		t.Fatalf("GenerateDue: %v", err)
	}

	type dResult struct {
		report *DeliveryReport
		err    error
	}
	dCh := make(chan dResult, 1)
	go func() {
		report, err := svc.DeliverDue(ctx, tGen, 10)
		dCh <- dResult{report, err}
	}()
	<-checkerReached

	type revResult struct {
		witness string // 撤销语句执行时亲见的意图状态（顺序见证）
		err     error
	}
	revCh := make(chan revResult, 1)
	go func() {
		tx, err := testPool.Begin(ctx)
		if err != nil {
			revCh <- revResult{"", err}
			return
		}
		if _, err := tx.Exec(ctx,
			`UPDATE tickets SET assigned_to=$1 WHERE id=$2`, f.P2, f.TicketID); err != nil {
			_ = tx.Rollback(ctx)
			revCh <- revResult{"", err}
			return
		}
		// 顺序见证：撤销语句真正执行到时读意图状态——pending 说明撤权语句先于
		// 投递提交到达（投递还拿着旧快照），delivered 说明投递已先提交。
		var status string
		if err := tx.QueryRow(ctx,
			`SELECT status FROM riskreminder_intents WHERE ticket_id=$1 AND generation=1`,
			f.TicketID).Scan(&status); err != nil {
			_ = tx.Rollback(ctx)
			revCh <- revResult{"", err}
			return
		}
		err = tx.Commit(ctx)
		revCh <- revResult{status, err}
	}()

	select {
	case r := <-revCh:
		if r.err != nil {
			t.Fatalf("revocation tx failed: %v", r.err)
		}
		// 分支一：撤权先于投递提交完成。契约要求零入箱。
		if r.witness != StatusPending {
			t.Fatalf("revocation committed first but witnessed intent %q, want pending", r.witness)
		}
		release()
		res := waitCh(t, dCh, 5*time.Second, "delivery after committed revocation")
		if res.err != nil {
			t.Fatalf("DeliverDue: %v", res.err)
		}
		if len(res.report.Delivered) != 0 {
			t.Fatalf("revocation committed before delivery commit yet report delivered %+v", res.report.Delivered)
		}
		if got := inboxCount(t, f.P); got != 0 {
			t.Fatalf("IAM 撤权先于投递提交，收件箱仍入箱 %d 条（撤权抑止失效）", got)
		}
	case <-time.After(2 * time.Second):
		// 分支二：共同 IAM 守卫挡住撤权——投递未提交前撤权无法提交。
		// 放行投递，撤权必然全序在后，其撤销语句应亲见投递已提交。
		release()
		res := waitCh(t, dCh, 5*time.Second, "delivery under guard")
		if res.err != nil {
			t.Fatalf("DeliverDue: %v", res.err)
		}
		if len(res.report.Delivered) != 1 {
			t.Fatalf("delivery linearized before revocation should deliver, got %+v", res.report)
		}
		r := waitCh(t, revCh, 5*time.Second, "revocation after delivery commit")
		if r.err != nil {
			t.Fatalf("revocation tx failed: %v", r.err)
		}
		if r.witness != StatusDelivered {
			t.Fatalf("revocation statement should observe delivered intent (投递先提交), got %q", r.witness)
		}
		if got := inboxCount(t, f.P); got != 1 {
			t.Fatalf("inbox rows = %d, want 1 (投递先于撤权的合法线性序)", got)
		}
	}
}

// 锁序证明：业务事务持票锁（共同 IAM 守卫）期间，投递必须被挡在票锁上——
// 到不了资格检查、也绝不能先锁意图行（票→意图取锁；intent 锁后补票锁即死锁向）。
func TestDeliveryLocksTicketBeforeIntent(t *testing.T) {
	f := createFixture(t)
	recordAssignment(t, f, 1, f.P, tDue)
	ctx := context.Background()
	svc := newService(func(ctx context.Context, tx pgx.Tx, ticketID, assigneeID int64) (bool, string, error) {
		return true, "", nil
	})
	if _, err := svc.GenerateDue(ctx, tGen, 10); err != nil {
		t.Fatalf("GenerateDue: %v", err)
	}
	var intentID int64
	mustScan(t, testPool.QueryRow(ctx,
		`SELECT id FROM riskreminder_intents WHERE ticket_id=$1`, f.TicketID), &intentID)

	// 业务事务：先持票锁（真实接线里分派/接单/撤权都持此锁后写 riskreminder_*）
	holder, err := testPool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin holder: %v", err)
	}
	defer func() { _ = holder.Rollback(ctx) }()
	if _, err := holder.Exec(ctx,
		`SELECT id FROM tickets WHERE id=$1 FOR UPDATE`, f.TicketID); err != nil {
		t.Fatalf("holder lock: %v", err)
	}

	once := &sync.Once{}
	allowChecker := make(chan struct{})
	release := func() { once.Do(func() { close(allowChecker) }) }
	defer release()
	// checker 屏障：把投递停在「已过意图锁、未提交」的窗口里——若锁序违规，
	// 投递此刻正拿着意图行锁等屏障；锁序正确则它根本到不了这里（挡在票锁）。
	svcBlocked := newService(func(ctx context.Context, tx pgx.Tx, ticketID, assigneeID int64) (bool, string, error) {
		<-allowChecker
		return true, "", nil
	})

	type dResult struct {
		report *DeliveryReport
		err    error
	}
	dCh := make(chan dResult, 1)
	go func() {
		report, err := svcBlocked.DeliverDue(ctx, tGen, 10)
		dCh <- dResult{report, err}
	}()

	// 给投递一个调度窗口：若锁序违规（先锁意图），此刻意图行已被它锁上
	time.Sleep(300 * time.Millisecond)

	// NOWAIT 探针：票锁被业务事务持有时，意图行必须无锁——
	// 投递必须仍挡在共同守卫（票锁）上，证明取锁顺序为票→意图。
	var probe int64
	err = testPool.QueryRow(ctx,
		`SELECT id FROM riskreminder_intents WHERE id=$1 FOR UPDATE NOWAIT`, intentID).Scan(&probe)
	if err != nil {
		t.Fatalf("票锁被业务事务持有时意图行已被加锁——锁序违规（意图先于票）: %v", err)
	}
	if got := inboxCount(t, f.P); got != 0 {
		t.Fatalf("守卫被持期间投递已入箱 %d 条——未取守卫就投递", got)
	}
	if err := holder.Rollback(ctx); err != nil {
		t.Fatalf("holder rollback: %v", err)
	}
	release()
	res := waitCh(t, dCh, 5*time.Second, "delivery after guard release")
	if res.err != nil {
		t.Fatalf("DeliverDue: %v", res.err)
	}
	if len(res.report.Delivered) != 1 {
		t.Fatalf("delivery after guard release = %+v", res.report)
	}
	if got := inboxCount(t, f.P); got != 1 {
		t.Fatalf("inbox rows = %d, want 1", got)
	}
}

// 扫描配额：已有同键意图的分派（含终态）必须先被排除，LIMIT 只数新意图——
// 否则最老 N 条已键控分派霸占配额，更晚到期的其他票永远生成不了。
// （注意不能用同票新旧代次造「老条目」——最新代次过滤本就排除它们，测不到该缺陷。）
func TestGenerateDueSkipsAlreadyKeyedNotHoggingQuota(t *testing.T) {
	f := createFixture(t)
	ctx := context.Background()
	// T1（最早到期）：已按原键生成并因资格撤回抑止成终态（assignment 未接单、意图已终态）
	recordAssignment(t, f, 1, f.P, tDue)
	svcInelig := newService(func(ctx context.Context, tx pgx.Tx, ticketID, assigneeID int64) (bool, string, error) {
		return false, "eligibility:assignee_moved", nil
	})
	if _, err := svcInelig.GenerateDue(ctx, tGen, 10); err != nil {
		t.Fatalf("GenerateDue(T1): %v", err)
	}
	if _, err := svcInelig.DeliverDue(ctx, tGen, 10); err != nil {
		t.Fatalf("DeliverDue(T1): %v", err)
	}
	// T2（更晚到期，无最新代次竞争）：尚无意图，等这次扫描生成
	t2 := addTicket(t, f)
	due2 := tDue.Add(time.Hour)
	recordAssignmentOn(t, f, t2, 1, f.P2, due2)

	// limit=1 的扫描：唯一新意图必须是 T2 的（T1 已有同键——排除，不占槽）
	svc := newService(nil)
	intents, err := svc.GenerateDue(ctx, due2.Add(time.Minute), 1)
	if err != nil {
		t.Fatalf("GenerateDue: %v", err)
	}
	if len(intents) != 1 {
		t.Fatalf("scan with limit=1 produced %d intents, want exactly T2's 1（最老已键控分派霸占了配额）", len(intents))
	}
	if intents[0].TicketID != t2 || intents[0].RecipientID != f.P2 {
		t.Fatalf("scan produced ticket %d for recipient %d, want T2 for P2",
			intents[0].TicketID, intents[0].RecipientID)
	}
	var total int
	mustScan(t, testPool.QueryRow(ctx, `SELECT COUNT(*) FROM riskreminder_intents`), &total)
	if total != 2 {
		t.Fatalf("total intents = %d, want 2 (T1 suppressed + T2 pending)", total)
	}
}

// 失败饥饿：刚试败的条目（backoff 窗口内）不得每轮队头压制健康条目——
// 健康优先；健康耗尽后原键照常恢复（恢复语义不回退）。
func TestDeliverPrefersHealthyOverRecentlyFailed(t *testing.T) {
	f := createFixture(t)
	t1 := f.TicketID
	t2 := addTicket(t, f)
	ctx := context.Background()
	now := time.Now().UTC()
	recordAssignmentOn(t, f, t1, 1, f.P, now.Add(-time.Hour))
	recordAssignmentOn(t, f, t2, 1, f.P, now.Add(-30*time.Minute))

	boom := errors.New("synthetic t1 failure")
	failingT1 := func(ctx context.Context, tx pgx.Tx, ticketID, assigneeID int64) (bool, string, error) {
		if ticketID == t1 {
			return false, "", boom
		}
		return true, "", nil
	}
	svc := newService(failingT1)
	if _, err := svc.GenerateDue(ctx, now, 10); err != nil {
		t.Fatalf("GenerateDue: %v", err)
	}

	// 轮1（limit=1）：最老的 t1 被拾取并试败留痕
	rep1, err := svc.DeliverDue(ctx, now, 1)
	if err != nil {
		t.Fatalf("DeliverDue#1: %v", err)
	}
	if len(rep1.Failed) != 1 || rep1.Failed[0].IntentID == 0 {
		t.Fatalf("round1 report = %+v, want t1 failed", rep1)
	}
	var t1Intent int64
	mustScan(t, testPool.QueryRow(ctx,
		`SELECT id FROM riskreminder_intents WHERE ticket_id=$1`, t1), &t1Intent)
	if rep1.Failed[0].IntentID != t1Intent {
		t.Fatalf("round1 failed intent = %d, want t1's %d", rep1.Failed[0].IntentID, t1Intent)
	}

	// 轮2（limit=1，立即）：t1 在 backoff 窗口内——健康优先必须投 t2
	rep2, err := svc.DeliverDue(ctx, time.Now().UTC(), 1)
	if err != nil {
		t.Fatalf("DeliverDue#2: %v", err)
	}
	if len(rep2.Delivered) != 1 || rep2.Delivered[0].TicketID != t2 {
		t.Fatalf("round2 report = %+v, want t2 delivered (健康优先，t1 刚试败不压制)", rep2)
	}
	var t1Attempts int
	mustScan(t, testPool.QueryRow(ctx,
		`SELECT attempt_count FROM riskreminder_intents WHERE id=$1`, t1Intent), &t1Attempts)
	if t1Attempts != 1 {
		t.Fatalf("t1 attempt_count = %d, want 1 (t1 本轮不应被重试)", t1Attempts)
	}

	// 轮3（limit=1）：健康耗尽——t1 按原键照常恢复重试（仍失败、attempt 累计）
	rep3, err := svc.DeliverDue(ctx, time.Now().UTC(), 1)
	if err != nil {
		t.Fatalf("DeliverDue#3: %v", err)
	}
	if len(rep3.Failed) != 1 || rep3.Failed[0].IntentID != t1Intent {
		t.Fatalf("round3 report = %+v, want t1 retried by same key", rep3)
	}
	mustScan(t, testPool.QueryRow(ctx,
		`SELECT attempt_count FROM riskreminder_intents WHERE id=$1`, t1Intent), &t1Attempts)
	if t1Attempts != 2 {
		t.Fatalf("t1 attempt_count = %d, want 2", t1Attempts)
	}
}

// 提交确认不明：Commit 报错时不得径直宣称未送达——按原意图读回后报真实结果。
// (a) 真提交已到服务器、调用方收到不确定错误 → 读回 delivered，报告已送达、
//
//	不得写失败留证（attempt_count 不增）；
//
// (b) 提交未发出（上下文取消）→ 读回仍 pending，按失败留证且证据带上下文取消
//
//	（errors.Is 可辨），可按原键恢复恰好入箱一次。
func TestCommitAmbiguityReportsTruthByReadback(t *testing.T) {
	ctx := context.Background()

	// (a) 提交成功但确认不明
	f := createFixture(t)
	recordAssignment(t, f, 1, f.P, tDue)
	svc := newService(func(ctx context.Context, tx pgx.Tx, ticketID, assigneeID int64) (bool, string, error) {
		return true, "", nil
	})
	if _, err := svc.GenerateDue(ctx, tGen, 10); err != nil {
		t.Fatalf("GenerateDue: %v", err)
	}
	faultOnce := true
	svc.runDeliveryTx = func(ctx context.Context, fn func(context.Context, pgx.Tx) error) error {
		if err := repository.WithIAMCommandGuard(ctx, testPool, fn); err != nil {
			return err
		}
		if faultOnce {
			faultOnce = false
			return context.Canceled
		} // only the first business commit loses its response
		return nil
	}
	report, err := svc.DeliverDue(ctx, tGen, 10)
	if err != nil {
		t.Fatalf("DeliverDue(ambiguous commit): %v", err)
	}
	if len(report.Delivered) != 1 {
		t.Fatalf("提交已成功仅确认不明——读回必须报真实结果 delivered, got %+v", report)
	}
	if len(report.Failed) != 0 {
		t.Fatalf("已送达意图不得进失败留证, got %+v", report.Failed)
	}
	if got := inboxCount(t, f.P); got != 1 {
		t.Fatalf("inbox rows = %d, want 1", got)
	}
	var attempts, delivered int
	mustScan(t, testPool.QueryRow(ctx,
		`SELECT attempt_count, (status='delivered')::int FROM riskreminder_intents WHERE ticket_id=$1`, f.TicketID),
		&attempts, &delivered)
	if delivered != 1 || attempts != 0 {
		t.Fatalf("delivered intent bookkeeping wrong: delivered=%d attempt_count=%d (不得留证失败)", delivered, attempts)
	}

	// (b) 提交未发出（上下文取消）——失败留证区分取消、可按原键恢复
	f2 := createFixture(t)
	recordAssignment(t, f2, 1, f2.P, tDue)
	svc2 := newService(func(ctx context.Context, tx pgx.Tx, ticketID, assigneeID int64) (bool, string, error) {
		return true, "", nil
	})
	if _, err := svc2.GenerateDue(ctx, tGen, 10); err != nil {
		t.Fatalf("GenerateDue: %v", err)
	}
	faultOnce2 := true
	svc2.runDeliveryTx = func(ctx context.Context, fn func(context.Context, pgx.Tx) error) error {
		return repository.WithIAMCommandGuard(ctx, testPool, func(ctx context.Context, tx pgx.Tx) error {
			if err := fn(ctx, tx); err != nil {
				return err
			}
			if faultOnce2 {
				faultOnce2 = false
				return context.Canceled
			} // owning transaction rolls back before commit
			return nil
		})
	}
	report2, err := svc2.DeliverDue(ctx, tGen, 10)
	if err != nil {
		t.Fatalf("DeliverDue(cancel commit): %v", err)
	}
	if len(report2.Failed) != 1 {
		t.Fatalf("未提交的取消应按失败留证, got %+v", report2)
	}
	if !errors.Is(report2.Failed[0].Err, context.Canceled) {
		t.Fatalf("失败证据必须可辨上下文取消, got %v", report2.Failed[0].Err)
	}
	if got := inboxCount(t, f2.P); got != 0 {
		t.Fatalf("inbox rows = %d, want 0 (提交未发出不得入箱)", got)
	}
	var status string
	var attempts2 int
	mustScan(t, testPool.QueryRow(ctx,
		`SELECT status, attempt_count FROM riskreminder_intents WHERE ticket_id=$1`, f2.TicketID),
		&status, &attempts2)
	if status != StatusPending || attempts2 != 1 {
		t.Fatalf("failure bookkeeping wrong: status=%s attempts=%d, want pending/1", status, attempts2)
	}
	// 恢复：原键重投恰好一次
	svcOK := newService(func(ctx context.Context, tx pgx.Tx, ticketID, assigneeID int64) (bool, string, error) {
		return true, "", nil
	})
	rep3, err := svcOK.DeliverDue(ctx, tGen, 10)
	if err != nil {
		t.Fatalf("DeliverDue(recover): %v", err)
	}
	if got := inboxCount(t, f2.P); got != 1 || len(rep3.Delivered) != 1 {
		t.Fatalf("recovery report=%+v inbox=%d, want delivered once", rep3, got)
	}
}

// 时间精度幂等：PG timestamptz 微秒精度——带纳秒分量的接单期限重试同代次
// 必须幂等返回原行（入库与比较都按微秒收敛），不得误判事实漂移。
func TestAcceptDuePrecisionIdempotent(t *testing.T) {
	f := createFixture(t)
	due := tDue.Add(567 * time.Nanosecond) // 纳秒分量：PG 只存微秒
	a1 := recordAssignment(t, f, 1, f.P, due)
	if a1.ID == 0 {
		t.Fatal("assignment not created")
	}
	a2 := recordAssignment(t, f, 1, f.P, due) // 同事实重试（同纳秒值）
	if a2.ID != a1.ID {
		t.Fatalf("nano-precision retry drifted: first id %d, retry id %d (want idempotent)", a1.ID, a2.ID)
	}
	// 截断后的事实必须仍落库一致：重扫生成的意图 due_at 与期限相等
	svc := newService(nil)
	intents, err := svc.GenerateDue(context.Background(), due.Add(time.Minute), 10)
	if err != nil {
		t.Fatalf("GenerateDue: %v", err)
	}
	if len(intents) != 1 || !intents[0].DueAt.UTC().Equal(due.Truncate(time.Microsecond)) {
		t.Fatalf("intent due_at precision wrong: %+v", intents)
	}
}
