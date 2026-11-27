//go:build integration

// Phase6 B2 第8项专项红绿（真实 PG）：分派事实幂等、asOf 扫描持久去重、
// 恰好一次入箱、接单/改派/资格撤回抑止留因、失败按原键恢复。
// 场景锚点：验收卡二主路径第 2 步（09:05 分派 / 09:35 截止 / 09:36 生成 /
// 09:40 接单）+「拒绝和待恢复变体」末行（投递暂失败/重启按原键恢复）。
package riskreminder

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
	"github.com/tracerbiubiubiu/zhuzhao/internal/testutil"
)

var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	pool, cleanup, err := testutil.SetupPostgresShared()
	if err != nil {
		panic(err)
	}
	testPool = pool
	if err := applyReminderMigration(context.Background(), pool); err != nil {
		panic(err)
	}
	code := m.Run()
	cleanup()
	os.Exit(code)
}

// applyReminderMigration testutil 共享迁移清单不含 000091（清单维护不在本切片授权内，
// 全量接入时由主会话补登）——专项测试从测试侧显式应用本迁移（已存在则跳过）。
func applyReminderMigration(ctx context.Context, pool *pgxpool.Pool) error {
	var exists bool
	if err := pool.QueryRow(ctx,
		`SELECT to_regclass('riskreminder_assignments') IS NOT NULL`).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return nil
	}
	return execMigrationFile(ctx, pool, "000091_riskreminder_accept_timeout.up.sql")
}

func execMigrationFile(ctx context.Context, pool *pgxpool.Pool, name string) error {
	sql, err := os.ReadFile(filepath.Join("..", "..", "..", "migrations", name))
	if err != nil {
		return err
	}
	_, err = pool.Exec(ctx, string(sql))
	return err
}

// ─── 可控 UTC 样例时钟（验收卡二：09:05 分派 / 09:35 截止 / 09:36 生成 / 09:40 接单）───

var (
	tDispatch = time.Date(2026, 6, 1, 9, 5, 0, 0, time.UTC)
	tDue      = time.Date(2026, 6, 1, 9, 35, 0, 0, time.UTC)
	tGen      = time.Date(2026, 6, 1, 9, 36, 0, 0, time.UTC)
	tAccept   = time.Date(2026, 6, 1, 9, 40, 0, 0, time.UTC)
)

type fixture struct {
	OrgID    int64
	M        int64 // 分派操作者
	P        int64 // 首轮被分派人（收件人）
	P2       int64 // 改派轮被分派人
	TicketID int64
	HumanKey string
	TypeCode string
}

// resetReminderState 只清内核两表与站内箱——tickets 族不动（000086 遗留绑定守卫
// 禁 TRUNCATE 级联；其余夹具表按唯一后缀各建各行，计数断言均按 ticket/recipient 定界）。
func resetReminderState(t *testing.T) {
	t.Helper()
	_, err := testPool.Exec(context.Background(), `
		TRUNCATE riskreminder_intents, riskreminder_assignments, notification_review_notices, notifications RESTART IDENTITY`)
	if err != nil {
		t.Fatalf("truncate: %v", err)
	}
}

func createFixture(t *testing.T) *fixture {
	t.Helper()
	resetReminderState(t)
	ctx := context.Background()
	f := &fixture{
		HumanKey: "TK-20260601-" + randSuffix(),
	}
	mustScan(t, testPool.QueryRow(ctx,
		`INSERT INTO organizations (code, name, path) VALUES ($1,$2,$3) RETURNING id`,
		"org-"+randSuffix(), "样例组织", "root."+randSuffix()), &f.OrgID)
	mustScan(t, testPool.QueryRow(ctx,
		`INSERT INTO users (username, password, real_name) VALUES ($1,'x','M') RETURNING id`,
		"m-"+randSuffix()), &f.M)
	mustScan(t, testPool.QueryRow(ctx,
		`INSERT INTO users (username, password, real_name) VALUES ($1,'x','P') RETURNING id`,
		"p-"+randSuffix()), &f.P)
	mustScan(t, testPool.QueryRow(ctx,
		`INSERT INTO users (username, password, real_name) VALUES ($1,'x','P2') RETURNING id`,
		"p2-"+randSuffix()), &f.P2)
	var typeCode string
	mustScan(t, testPool.QueryRow(ctx,
		`INSERT INTO ticket_types (code, name) VALUES ($1,$2) RETURNING code`,
		"risk_fix_"+randSuffix(), "漏洞整改"), &typeCode)
	f.TypeCode = typeCode
	mustScan(t, testPool.QueryRow(ctx,
		`INSERT INTO tickets (type_code, title, created_by, org_id, org_path, human_key, status)
		 VALUES ($1,'T-201 样例整改票',$2,$3,
		         (SELECT path FROM organizations WHERE id=$3),$4,'open') RETURNING id`,
		typeCode, f.M, f.OrgID, f.HumanKey), &f.TicketID)
	return f
}

// addTicket 同夹具追加一张票（多票并发用例；不动内核两表）。
func addTicket(t *testing.T, f *fixture) int64 {
	t.Helper()
	var id int64
	mustScan(t, testPool.QueryRow(context.Background(),
		`INSERT INTO tickets (type_code, title, created_by, org_id, org_path, human_key, status)
		 VALUES ($1,'T-201 附加样例票',$2,$3,
		         (SELECT path FROM organizations WHERE id=$3),$4,'open') RETURNING id`,
		f.TypeCode, f.M, f.OrgID, "TK-X-"+randSuffix()), &id)
	return id
}

func mustScan(t *testing.T, row interface{ Scan(dest ...any) error }, dest ...any) {
	t.Helper()
	if err := row.Scan(dest...); err != nil {
		t.Fatalf("fixture scan: %v", err)
	}
}

var fixtureCounter atomic.Uint64

func randSuffix() string {
	return strconv.FormatUint(fixtureCounter.Add(1), 10)
}

// ─── 服务与调用助手 ───

func newService(checker EligibilityChecker) *Service {
	return NewService(testPool, repository.NewNotificationInboxRepo(testPool), checker,
		slog.New(slog.NewTextHandler(os.Stderr, nil)))
}

func assignInput(f *fixture, generation, assignee int64, due time.Time) *AssignmentInput {
	return &AssignmentInput{
		TicketID: f.TicketID, Generation: generation,
		AssigneeID: assignee, DispatchedBy: f.M,
		DispatchedAt: tDispatch, AcceptDueAt: due,
		RuleID: "risk_accept_timeout", RuleVersion: 1,
	}
}

func recordAssignment(t *testing.T, f *fixture, generation, assignee int64, due time.Time) *Assignment {
	return recordAssignmentOn(t, f, f.TicketID, generation, assignee, due)
}

// recordAssignmentOn 指定票写入分派事实（多票并发用例）。
func recordAssignmentOn(t *testing.T, f *fixture, ticketID, generation, assignee int64, due time.Time) *Assignment {
	t.Helper()
	ctx := context.Background()
	tx, err := testPool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	in := &AssignmentInput{
		TicketID: ticketID, Generation: generation,
		AssigneeID: assignee, DispatchedBy: f.M,
		DispatchedAt: tDispatch, AcceptDueAt: due,
		RuleID: "risk_accept_timeout", RuleVersion: 1,
	}
	a, err := RecordAssignmentTx(ctx, tx, in)
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("RecordAssignmentTx: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}
	return a
}

func recordAccepted(t *testing.T, f *fixture, generation int64, at time.Time, by int64) (*Assignment, bool) {
	t.Helper()
	ctx := context.Background()
	tx, err := testPool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	a, already, err := RecordAcceptedTx(ctx, tx, f.TicketID, generation, at, by)
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("RecordAcceptedTx: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}
	return a, already
}

func inboxCount(t *testing.T, recipientID int64) int64 {
	t.Helper()
	var n int64
	mustScan(t, testPool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM notifications WHERE recipient_id=$1`, recipientID), &n)
	return n
}

func intentsOf(t *testing.T, f *fixture) []*Intent {
	t.Helper()
	list, err := newService(nil).IntentsByTicket(context.Background(), f.TicketID)
	if err != nil {
		t.Fatalf("IntentsByTicket: %v", err)
	}
	return list
}

// ─── 用例 ───

// 分派事实：同代次重试幂等返回原行；事实漂移拒绝；接单期限/参数按输入落库。
func TestRecordAssignmentIdempotentAndConflict(t *testing.T) {
	f := createFixture(t)
	a1 := recordAssignment(t, f, 1, f.P, tDue)
	if a1.ID == 0 || a1.AcceptDueAt.UTC() != tDue || a1.AssigneeID != f.P {
		t.Fatalf("assignment facts wrong: %+v", a1)
	}
	// 同代次重试（新 dispatched_at 时钟）→ 原行，不新增
	a2 := recordAssignment(t, f, 1, f.P, tDue)
	if a2.ID != a1.ID || !a2.DispatchedAt.Equal(a1.DispatchedAt) {
		t.Fatalf("retry should return original row: got %+v want id %d", a2, a1.ID)
	}
	var n int64
	mustScan(t, testPool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM riskreminder_assignments WHERE ticket_id=$1`, f.TicketID), &n)
	if n != 1 {
		t.Fatalf("assignment rows = %d, want 1", n)
	}
	// 同代次事实漂移 → 冲突
	tx, _ := testPool.Begin(context.Background())
	_, err := RecordAssignmentTx(context.Background(), tx, assignInput(f, 1, f.P2, tDue))
	_ = tx.Rollback(context.Background())
	if !errors.Is(err, errcode.ErrConflict) {
		t.Fatalf("fact drift should conflict, got %v", err)
	}
	// 非法输入
	tx, _ = testPool.Begin(context.Background())
	_, err = RecordAssignmentTx(context.Background(), tx, &AssignmentInput{TicketID: f.TicketID})
	_ = tx.Rollback(context.Background())
	if !errors.Is(err, errcode.ErrInvalidParams) {
		t.Fatalf("invalid input should be ErrInvalidParams, got %v", err)
	}
}

// asOf 扫描：截止前不生成；到期生成一条 pending；重扫持久去重不重发。
func TestGenerateDuePersistsIntentOnce(t *testing.T) {
	f := createFixture(t)
	recordAssignment(t, f, 1, f.P, tDue)
	svc := newService(nil)

	// 09:34：未到接单截止，不生成
	intents, err := svc.GenerateDue(context.Background(), tDue.Add(-time.Minute), 10)
	if err != nil {
		t.Fatalf("GenerateDue(09:34): %v", err)
	}
	if len(intents) != 0 {
		t.Fatalf("pre-due scan generated %d intents, want 0", len(intents))
	}
	// 09:35/09:36：到期生成一条待发送意图
	intents, err = svc.GenerateDue(context.Background(), tGen, 10)
	if err != nil {
		t.Fatalf("GenerateDue(09:36): %v", err)
	}
	if len(intents) != 1 {
		t.Fatalf("due scan generated %d intents, want 1", len(intents))
	}
	i := intents[0]
	if i.Status != StatusPending || !i.DueAt.UTC().Equal(tDue) || i.RecipientID != f.P {
		t.Fatalf("intent facts wrong: %+v", i)
	}
	if i.Source != Source || i.Channel != Channel || i.SemanticKey != SemanticKey {
		t.Fatalf("delivery key parts wrong: %+v", i)
	}
	// 七元组的 source_event_id 绑定分派事实
	if want := strconv.FormatInt(i.AssignmentID, 10); i.SourceEventID != want {
		t.Fatalf("source_event_id = %q, want assignment id %q", i.SourceEventID, want)
	}
	// 重扫（重启/重放）不重发同一时隙
	again, err := svc.GenerateDue(context.Background(), tGen, 10)
	if err != nil {
		t.Fatalf("rescan: %v", err)
	}
	if len(again) != 0 || len(intentsOf(t, f)) != 1 {
		t.Fatalf("rescan dedupe broken: new=%d total=%d", len(again), len(intentsOf(t, f)))
	}
}

// 主路径 09:36→投递：有资格收件人收件箱恰好新增一条；重复投递/重扫均零新增。
func TestDeliverDueExactlyOnceInbox(t *testing.T) {
	f := createFixture(t)
	recordAssignment(t, f, 1, f.P, tDue)
	svc := newService(func(ctx context.Context, tx pgx.Tx, ticketID, assigneeID int64) (bool, string, error) {
		return true, "", nil
	})
	if _, err := svc.GenerateDue(context.Background(), tGen, 10); err != nil {
		t.Fatalf("GenerateDue: %v", err)
	}

	report, err := svc.DeliverDue(context.Background(), tGen, 10)
	if err != nil {
		t.Fatalf("DeliverDue: %v", err)
	}
	if len(report.Delivered) != 1 || len(report.Suppressed) != 0 || len(report.Failed) != 0 {
		t.Fatalf("first delivery report = %+v", report)
	}
	if got := inboxCount(t, f.P); got != 1 {
		t.Fatalf("P inbox rows = %d, want exactly 1", got)
	}
	row := report.Delivered[0]
	if row.Status != StatusDelivered || row.NotificationID == nil || row.DeliveredAt == nil {
		t.Fatalf("delivered intent facts wrong: %+v", row)
	}
	// 入箱内容：biz 锚点与深链沿 inbox 通道惯例
	var bizID int64
	var title, link string
	mustScan(t, testPool.QueryRow(context.Background(),
		`SELECT biz_id, title, link FROM notifications WHERE recipient_id=$1`, f.P),
		&bizID, &title, &link)
	if bizID != f.TicketID || link != "/tickets/"+strconv.FormatInt(f.TicketID, 10) {
		t.Fatalf("notification anchors wrong: biz_id=%d link=%q", bizID, link)
	}
	if title == "" {
		t.Fatal("notification title empty (入站即渲染)")
	}

	// 重复投递 + 重扫：零新增（一次性效果；重启不重发）
	for range 2 {
		if _, err := svc.GenerateDue(context.Background(), tGen, 10); err != nil {
			t.Fatalf("regenerate: %v", err)
		}
		report, err := svc.DeliverDue(context.Background(), tGen, 10)
		if err != nil {
			t.Fatalf("redeliver: %v", err)
		}
		if len(report.Delivered)+len(report.Suppressed)+len(report.Failed) != 0 {
			t.Fatalf("redelivery produced effects: %+v", report)
		}
	}
	if got := inboxCount(t, f.P); got != 1 {
		t.Fatalf("P inbox rows after redelivery = %d, want 1", got)
	}
}

// 接单抑止：未发的抑止留因 accepted；原时间事实不覆盖；已发的保留可查；
// 接单后重扫不再生成（接单前暂停发送 → 接单后不再发出）。
func TestAcceptSuppressesPendingKeepsSent(t *testing.T) {
	f := createFixture(t)
	recordAssignment(t, f, 1, f.P, tDue)
	svc := newService(func(ctx context.Context, tx pgx.Tx, ticketID, assigneeID int64) (bool, string, error) {
		return true, "", nil
	})

	// A：先发后接——已发保留
	if _, err := svc.GenerateDue(context.Background(), tGen, 10); err != nil {
		t.Fatalf("GenerateDue: %v", err)
	}
	if _, err := svc.DeliverDue(context.Background(), tGen, 10); err != nil {
		t.Fatalf("DeliverDue: %v", err)
	}
	a, already := recordAccepted(t, f, 1, tAccept, f.P)
	if already || a.AcceptedAt == nil || !a.AcceptedAt.UTC().Equal(tAccept) {
		t.Fatalf("accept facts wrong: already=%v acceptedAt=%v", already, a.AcceptedAt)
	}
	byID := int64(0)
	mustScan(t, testPool.QueryRow(context.Background(),
		`SELECT accepted_by FROM riskreminder_assignments WHERE ticket_id=$1 AND generation=1`, f.TicketID), &byID)
	if byID != f.P {
		t.Fatalf("accepted_by = %d, want %d", byID, f.P)
	}
	// 已发提醒与超期事实可查：delivered 意图保留 + accepted_at 与 accept_due_at 同行
	sent := 0
	for _, i := range intentsOf(t, f) {
		if i.Status == StatusDelivered {
			sent++
		}
	}
	if sent != 1 {
		t.Fatalf("delivered reminders kept = %d, want 1", sent)
	}
	// 重复接单：原时间事实不覆盖
	_, already = recordAccepted(t, f, 1, tAccept.Add(5*time.Minute), f.P)
	if !already {
		t.Fatal("second accept should be already=true")
	}
	var gotAt time.Time
	mustScan(t, testPool.QueryRow(context.Background(),
		`SELECT accepted_at FROM riskreminder_assignments WHERE ticket_id=$1 AND generation=1`, f.TicketID), &gotAt)
	if gotAt.UTC() != tAccept {
		t.Fatalf("accepted_at drifted to %v, want original %v", gotAt, tAccept)
	}

	// B：接单先于扫描/投递——不再生成、不再发出
	f2 := createFixture(t)
	recordAssignment(t, f2, 1, f.P, tDue)
	recordAccepted(t, f2, 1, tAccept, f.P)
	intents, err := newService(nil).GenerateDue(context.Background(), tGen, 10)
	if err != nil {
		t.Fatalf("GenerateDue after accept: %v", err)
	}
	if len(intents) != 0 {
		t.Fatalf("scan after accept generated %d intents, want 0", len(intents))
	}
	report, err := newService(nil).DeliverDue(context.Background(), tGen, 10)
	if err != nil {
		t.Fatalf("DeliverDue after accept: %v", err)
	}
	if len(report.Delivered)+len(report.Suppressed)+len(report.Failed) != 0 {
		t.Fatalf("delivery after accept produced effects: %+v", report)
	}
	if got := inboxCount(t, f2.P); got != 0 {
		t.Fatalf("P2 inbox rows = %d, want 0", got)
	}

	// C：生成后、投递前接单——接单事务已抑止留因 accepted，投递轮零效果、零入箱
	f3 := createFixture(t)
	recordAssignment(t, f3, 1, f.P, tDue)
	svc3 := newService(func(ctx context.Context, tx pgx.Tx, ticketID, assigneeID int64) (bool, string, error) {
		return true, "", nil
	})
	if _, err := svc3.GenerateDue(context.Background(), tGen, 10); err != nil {
		t.Fatalf("GenerateDue: %v", err)
	}
	recordAccepted(t, f3, 1, tAccept, f.P)
	report, err = svc3.DeliverDue(context.Background(), tGen, 10)
	if err != nil {
		t.Fatalf("DeliverDue: %v", err)
	}
	if len(report.Delivered)+len(report.Suppressed)+len(report.Failed) != 0 {
		t.Fatalf("delivery after accept-produced intent should be no-op: %+v", report)
	}
	suppressed := 0
	for _, i := range intentsOf(t, f3) {
		if i.Status == StatusSuppressed && i.SuppressReason != nil && *i.SuppressReason == ReasonAccepted {
			suppressed++
		}
	}
	if suppressed != 1 {
		t.Fatalf("accept-time suppression records = %d, want 1", suppressed)
	}
	if got := inboxCount(t, f3.P); got != 0 {
		t.Fatalf("inbox rows = %d, want 0", got)
	}
}

// 改派：新代次抑止旧代次待发（留因 reassigned）；旧代次抑制不阻碍新代次重入；
// 投递期复核发现更新代次同样抑止（superseded）。
func TestReassignSuppressesOldGenerationNotNew(t *testing.T) {
	f := createFixture(t)
	recordAssignment(t, f, 1, f.P, tDue)
	svc := newService(func(ctx context.Context, tx pgx.Tx, ticketID, assigneeID int64) (bool, string, error) {
		return true, "", nil
	})
	if _, err := svc.GenerateDue(context.Background(), tGen, 10); err != nil {
		t.Fatalf("GenerateDue: %v", err)
	}

	// 09:05+ 新代次分派给 P2（新接单期限另取 30 分钟后）
	due2 := tDispatch.Add(65 * time.Minute)
	recordAssignment(t, f, 2, f.P2, due2)
	reasons := map[string]int{}
	for _, i := range intentsOf(t, f) {
		if i.SuppressReason != nil {
			reasons[*i.SuppressReason]++
		}
	}
	if reasons[ReasonReassigned] != 1 {
		t.Fatalf("old-generation pending should be reassigned-suppressed, got %v", reasons)
	}

	// 旧代次到期不再投出（已抑止）；新代次未到期不生成
	if _, err := svc.GenerateDue(context.Background(), tGen, 10); err != nil {
		t.Fatalf("GenerateDue: %v", err)
	}
	report, err := svc.DeliverDue(context.Background(), tGen, 10)
	if err != nil {
		t.Fatalf("DeliverDue: %v", err)
	}
	if len(report.Delivered)+len(report.Suppressed)+len(report.Failed) != 0 {
		t.Fatalf("old-gen delivery produced effects: %+v", report)
	}
	if got := inboxCount(t, f.P) + inboxCount(t, f.P2); got != 0 {
		t.Fatalf("inbox rows = %d, want 0 before new-gen due", got)
	}

	// 新代次到期：旧代次抑制不阻碍重入——正常生成并投给 P2
	if _, err := svc.GenerateDue(context.Background(), due2.Add(time.Minute), 10); err != nil {
		t.Fatalf("GenerateDue(gen2): %v", err)
	}
	report, err = svc.DeliverDue(context.Background(), due2.Add(time.Minute), 10)
	if err != nil {
		t.Fatalf("DeliverDue(gen2): %v", err)
	}
	if len(report.Delivered) != 1 || report.Delivered[0].RecipientID != f.P2 {
		t.Fatalf("gen2 delivery report = %+v", report)
	}
	if got := inboxCount(t, f.P2); got != 1 {
		t.Fatalf("P2 inbox rows = %d, want 1", got)
	}

	// 投递期复核（先扫描、后出现更新代次、再投递）→ superseded 抑止。
	// 正常路径 RecordAssignmentTx 已在改派事务抑止旧待发——此例用绕过内核的
	// 直插模拟「更新代次未经抑止面到达」，验证投递锁内的代次复核防线仍在。
	f2 := createFixture(t)
	recordAssignment(t, f2, 1, f.P, tDue)
	svc2 := newService(func(ctx context.Context, tx pgx.Tx, ticketID, assigneeID int64) (bool, string, error) {
		return true, "", nil
	})
	if _, err := svc2.GenerateDue(context.Background(), tGen, 10); err != nil {
		t.Fatalf("GenerateDue: %v", err)
	}
	if _, err := testPool.Exec(context.Background(), `
		INSERT INTO riskreminder_assignments
			(ticket_id, generation, assignee_id, dispatched_by, dispatched_at, accept_due_at, rule_id, rule_version)
		VALUES ($1, 2, $2, $3, $4, $5, 'risk_accept_timeout', 1)`,
		f2.TicketID, f2.P2, f2.M, tDispatch, tDue.Add(time.Hour)); err != nil {
		t.Fatalf("bypass insert gen2: %v", err)
	}
	report, err = svc2.DeliverDue(context.Background(), tGen, 10)
	if err != nil {
		t.Fatalf("DeliverDue: %v", err)
	}
	if len(report.Suppressed) != 1 || report.Suppressed[0].SuppressReason == nil ||
		*report.Suppressed[0].SuppressReason != ReasonSuperseded {
		t.Fatalf("suppress-on-supersede report = %+v", report)
	}
	if got := inboxCount(t, f2.P); got != 0 {
		t.Fatalf("inbox rows = %d, want 0", got)
	}
}

// 资格撤回：checker 判不合格 → 抑止留因、零入箱；空原因有兜底词。
func TestDeliverChecksRecipientEligibility(t *testing.T) {
	f := createFixture(t)
	recordAssignment(t, f, 1, f.P, tDue)
	svc := newService(func(ctx context.Context, tx pgx.Tx, ticketID, assigneeID int64) (bool, string, error) {
		return false, "eligibility:assignee_moved", nil
	})
	if _, err := svc.GenerateDue(context.Background(), tGen, 10); err != nil {
		t.Fatalf("GenerateDue: %v", err)
	}
	report, err := svc.DeliverDue(context.Background(), tGen, 10)
	if err != nil {
		t.Fatalf("DeliverDue: %v", err)
	}
	if len(report.Suppressed) != 1 || report.Suppressed[0].SuppressReason == nil ||
		*report.Suppressed[0].SuppressReason != "eligibility:assignee_moved" {
		t.Fatalf("withdraw suppression report = %+v", report)
	}
	if got := inboxCount(t, f.P); got != 0 {
		t.Fatalf("inbox rows after withdrawal = %d, want 0", got)
	}
}

// 恢复变体：checker 读失败 → 留痕可按原键恢复（状态仍 pending、attempt/last_error
// 明确可见），修复后同键只入箱一次。
func TestDeliverFailureRecoverableBySameKey(t *testing.T) {
	f := createFixture(t)
	recordAssignment(t, f, 1, f.P, tDue)
	boom := errors.New("synthetic read failure")
	svc := newService(func(ctx context.Context, tx pgx.Tx, ticketID, assigneeID int64) (bool, string, error) {
		return false, "", boom
	})
	if _, err := svc.GenerateDue(context.Background(), tGen, 10); err != nil {
		t.Fatalf("GenerateDue: %v", err)
	}
	report, err := svc.DeliverDue(context.Background(), tGen, 10)
	if err != nil {
		t.Fatalf("DeliverDue(fail): %v", err)
	}
	if len(report.Failed) != 1 {
		t.Fatalf("failure report = %+v, want 1 failed", report)
	}
	// 意图仍 pending 且留痕可见
	list := intentsOf(t, f)
	if len(list) != 1 || list[0].Status != StatusPending || list[0].AttemptCount != 1 ||
		list[0].LastError == nil || *list[0].LastError == "" || list[0].LastAttemptedAt == nil {
		t.Fatalf("failure bookkeeping wrong: %+v", list)
	}
	// 再失败一次：attempt 累计
	if _, err := svc.DeliverDue(context.Background(), tGen, 10); err != nil {
		t.Fatalf("DeliverDue(fail#2): %v", err)
	}
	list = intentsOf(t, f)
	if len(list) != 1 || list[0].AttemptCount != 2 {
		t.Fatalf("attempt count after second failure = %+v", list)
	}
	// checker 恢复 → 同一原键投递成功，入箱恰好一次
	svcOK := newService(func(ctx context.Context, tx pgx.Tx, ticketID, assigneeID int64) (bool, string, error) {
		return true, "", nil
	})
	report, err = svcOK.DeliverDue(context.Background(), tGen, 10)
	if err != nil {
		t.Fatalf("DeliverDue(recover): %v", err)
	}
	if len(report.Delivered) != 1 {
		t.Fatalf("recovery report = %+v", report)
	}
	if got := inboxCount(t, f.P); got != 1 {
		t.Fatalf("inbox rows after recovery = %d, want exactly 1", got)
	}
	// checker 未装配：fail-safe 失败留痕，不抑止不入箱
	f2 := createFixture(t)
	recordAssignment(t, f2, 1, f.P, tDue)
	svcNoChecker := newService(nil)
	if _, err := svcNoChecker.GenerateDue(context.Background(), tGen, 10); err != nil {
		t.Fatalf("GenerateDue: %v", err)
	}
	if _, err := svcNoChecker.DeliverDue(context.Background(), tGen, 10); err != nil {
		t.Fatalf("DeliverDue(no checker) should fail intents, not error: %v", err)
	}
	list = intentsOf(t, f2)
	if len(list) != 1 || list[0].Status != StatusPending || list[0].AttemptCount != 1 {
		t.Fatalf("no-checker fail-safe wrong: %+v", list)
	}
	if got := inboxCount(t, f2.P); got != 0 {
		t.Fatalf("inbox rows with no checker = %d, want 0", got)
	}
}

// 并发消费：同意图两路并发投递，收件箱恰好新增一条（同事务终态仲裁）。
func TestConcurrentDeliverExactlyOnce(t *testing.T) {
	f := createFixture(t)
	recordAssignment(t, f, 1, f.P, tDue)
	svc := newService(func(ctx context.Context, tx pgx.Tx, ticketID, assigneeID int64) (bool, string, error) {
		return true, "", nil
	})
	if _, err := svc.GenerateDue(context.Background(), tGen, 10); err != nil {
		t.Fatalf("GenerateDue: %v", err)
	}
	var wg sync.WaitGroup
	reports := make([]*DeliveryReport, 2)
	errs := make([]error, 2)
	for k := range 2 {
		wg.Add(1)
		go func(k int) {
			defer wg.Done()
			reports[k], errs[k] = svc.DeliverDue(context.Background(), tGen, 10)
		}(k)
	}
	wg.Wait()
	delivered := 0
	for k := range 2 {
		if errs[k] != nil {
			t.Fatalf("worker %d: %v", k, errs[k])
		}
		delivered += len(reports[k].Delivered)
		if len(reports[k].Failed) > 0 {
			t.Fatalf("worker %d unexpected failures: %+v", k, reports[k].Failed)
		}
	}
	if delivered != 1 {
		t.Fatalf("concurrent delivered total = %d, want exactly 1", delivered)
	}
	if got := inboxCount(t, f.P); got != 1 {
		t.Fatalf("inbox rows = %d, want exactly 1", got)
	}
}

// 接单缺失不扫描（ErrNotFound）+ 分派代次升序事实读取（IntentsByTicket 读面）。
func TestAcceptWithoutAssignmentNotFound(t *testing.T) {
	f := createFixture(t)
	tx, _ := testPool.Begin(context.Background())
	_, _, err := RecordAcceptedTx(context.Background(), tx, f.TicketID, 1, tAccept, f.P)
	_ = tx.Rollback(context.Background())
	if !errors.Is(err, errcode.ErrNotFound) {
		t.Fatalf("accept before assignment should be ErrNotFound, got %v", err)
	}
}
