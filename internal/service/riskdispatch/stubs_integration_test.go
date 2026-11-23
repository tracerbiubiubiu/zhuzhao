//go:build integration

package riskdispatch

// 各域窄接口的合成实现（仅测试编排语义，不复制任何域的既有服务逻辑）；
// 生产实现归 risk / ticket / user-org 各域属主，见 dispatch-handoff.md §4。

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
)

// ---------- 授权 stub：allowlist 决定 actor 对具名风险的派单资格 ----------

type stubAuthorizer struct {
	allowed  map[string]bool // "actor|riskID" → true
	denyCall int             // 第 N 次调用起拒绝（0=不启用；模拟预检后撤权的 TOCTOU）
	calls    int
}

func (s *stubAuthorizer) AuthorizeDispatch(_ context.Context, _ Reader, actorID int64,
	_ DispatchScope, riskIDs []int64, _ Target) error {
	s.calls++
	if s.denyCall > 0 && s.calls >= s.denyCall {
		return errcode.ErrForbidden
	}
	for _, id := range riskIDs {
		if !s.allowed[fmt.Sprintf("%d|%d", actorID, id)] {
			return errcode.ErrForbidden
		}
	}
	return nil
}

// ---------- 事实 stub：可见性 = allowlist；锁内升序读现势 ----------

type stubFacts struct {
	visible     map[string]bool // "actor|riskID" → true
	basisByRisk map[int64]string
}

func (s *stubFacts) CurrentDispatchFacts(ctx context.Context, db Reader, actorID int64,
	expected []RiskExpected) ([]RiskFact, error) {
	ids := make([]int64, 0, len(expected))
	for _, item := range expected {
		if s.visible[fmt.Sprintf("%d|%d", actorID, item.ID)] {
			ids = append(ids, item.ID)
		}
	}
	if len(ids) == 0 {
		return nil, nil
	}
	return s.readFacts(ctx, db, ids, false)
}

func (s *stubFacts) LockDispatchFacts(ctx context.Context, tx pgx.Tx, ids []int64) ([]RiskFact, error) {
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return s.readFacts(ctx, tx, ids, true)
}

func (s *stubFacts) readFacts(ctx context.Context, db Reader, ids []int64, forUpdate bool) ([]RiskFact, error) {
	placeholders := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
		args[i] = id
	}
	sql := `
		SELECT id, revision, human_key, status, COALESCE(owner_user_id, 0), org_id,
		       COALESCE(affected_org_id, 0), COALESCE(level, ''), COALESCE(deadline, 'epoch'::timestamptz),
		       COALESCE(finding_key, ''), COALESCE(asset_provider, '') || ':' || COALESCE(asset_external_id, ''),
		       title
		FROM risk_items WHERE id IN (` + strings.Join(placeholders, ",") + `)`
	if forUpdate {
		sql += ` ORDER BY id FOR UPDATE`
	} else {
		sql += ` ORDER BY id`
	}
	rows, err := db.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var facts []RiskFact
	for rows.Next() {
		var fact RiskFact
		var level string
		var deadline time.Time
		if err := rows.Scan(&fact.ID, &fact.Revision, &fact.HumanKey, &fact.Status, &fact.OwnerID,
			&fact.ManagementOrgID, &fact.AffectedOrgID, &level, &deadline,
			&fact.FindingKey, &fact.AssetKey, &fact.Title); err != nil {
			return nil, err
		}
		fact.Priority = map[string]int{"urgent": 1, "high": 2, "medium": 3, "low": 4}[level]
		fact.Deadline = deadline.UTC()
		fact.DispatchBasis = "synthetic-basis-v1"
		if basis, ok := s.basisByRisk[fact.ID]; ok {
			fact.DispatchBasis = basis
		}
		facts = append(facts, fact)
	}
	return facts, rows.Err()
}

// ---------- 落账 stub：CAS 翻转状态 + 同事务追加 risk_events ----------

type stubCommit struct{}

func (stubCommit) CommitDispatched(ctx context.Context, tx pgx.Tx, items []DispatchCommitItem) error {
	for _, item := range items {
		tag, err := tx.Exec(ctx, `
			UPDATE risk_items SET status = 'remediating', revision = revision + 1, updated_at = now()
			WHERE id = $1 AND revision = $2
		`, item.RiskID, item.ExpectedRevision)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return errcode.ErrConcurrentModification
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO risk_events (risk_id, aggregate_revision, event_kind, ordinal, actor_id, operation_id, payload)
			VALUES ($1, $2, 'dispatch', 0, $3, $4, jsonb_build_object('ticket_id', $5::bigint, 'group_key', $6::text))
		`, item.RiskID, item.ExpectedRevision+1, item.ActorID, item.OperationID, item.TicketID, item.GroupKey); err != nil {
			return err
		}
	}
	return nil
}

// ---------- 工单桥 stub：冻结三段合同的最小形状（type/org 锁序 + created_by=owner） ----------

type stubTicketCreator struct {
	prepares int
	locks    int
	creates  int
}

type stubPreparation struct {
	req   *model.CreateTicketRequest
	actor int64
}

type stubLocked struct {
	prep  *stubPreparation
	orgID int64
}

func (s *stubTicketCreator) PrepareRiskDispatchCreate(_ context.Context,
	req *model.CreateTicketRequest, actorUserID int64) (TicketPreparation, error) {
	s.prepares++
	// 冻结合同：规范正版本 + 64 位小写 hex 摘要；assigned_to 一律禁止
	if req.DefinitionVersion == "" || req.DefinitionHash == "" {
		return nil, errcode.ErrInvalidParams
	}
	if !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(req.DefinitionHash) {
		return nil, errcode.ErrInvalidParams
	}
	if req.AssignedTo != nil {
		return nil, errcode.ErrInvalidParams
	}
	return &stubPreparation{req: req, actor: actorUserID}, nil
}

func (s *stubTicketCreator) LockRiskDispatchCreateTx(ctx context.Context, tx pgx.Tx,
	prepared TicketPreparation) (LockedTicketCreate, error) {
	s.locks++
	prep, ok := prepared.(*stubPreparation)
	if !ok || prep == nil {
		return nil, errcode.ErrInvalidParams
	}
	// 冻结锁序：type → org
	if _, err := tx.Exec(ctx, `SELECT 1 FROM ticket_types WHERE code = $1 FOR SHARE`, prep.req.TypeCode); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `SELECT 1 FROM organizations WHERE id = $1 FOR UPDATE`, prep.req.OrgID); err != nil {
		return nil, err
	}
	return &stubLocked{prep: prep, orgID: prep.req.OrgID}, nil
}

func (s *stubTicketCreator) UpdateRiskDispatchPriorityTx(ctx context.Context, tx pgx.Tx, ticket, actor int64, priority int) error {
	tag, err := tx.Exec(ctx, `UPDATE tickets SET priority=$1,revision=revision+1 WHERE id=$2 AND priority<>$1`, priority, ticket)
	if err != nil || tag.RowsAffected() == 0 {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO ticket_events(ticket_id,user_id,action) VALUES($1,$2,'updated')`, ticket, actor)
	return err
}

func (s *stubTicketCreator) CreateRiskDispatchTicketTx(ctx context.Context, tx pgx.Tx,
	locked LockedTicketCreate, ownerUserID int64, link RiskDispatchLinkWriter) (TicketOutcome, error) {
	s.creates++
	lk, ok := locked.(*stubLocked)
	if !ok || lk == nil {
		return TicketOutcome{}, errcode.ErrInvalidParams
	}
	req := lk.prep.req
	var ticketID int64
	var orgPath string
	if err := tx.QueryRow(ctx, `SELECT path FROM organizations WHERE id = $1`, lk.orgID).Scan(&orgPath); err != nil {
		return TicketOutcome{}, err
	}
	if err := tx.QueryRow(ctx, `
		INSERT INTO tickets (type_code, title, description, priority, status, created_by, org_id, org_path)
		VALUES ($1, $2, $3, $4, 'open', $5, $6, $7)
		RETURNING id
	`, req.TypeCode, req.Title, req.Description, req.Priority,
		ownerUserID, lk.orgID, orgPath).Scan(&ticketID); err != nil {
		return TicketOutcome{}, err
	}
	ref := fmt.Sprintf("SYN-%d", ticketID)
	if _, err := tx.Exec(ctx, `UPDATE tickets SET human_key = $1 WHERE id = $2`, ref, ticketID); err != nil {
		return TicketOutcome{}, err
	}
	// created 历史 actor=真实操作者，created_by=owner（桥合同两账分离）
	if _, err := tx.Exec(ctx, `
		INSERT INTO ticket_events (ticket_id, user_id, action, to_value)
		VALUES ($1, $2, 'created', 'open')
	`, ticketID, lk.prep.actor); err != nil {
		return TicketOutcome{}, err
	}
	// link 回调先于 file 绑定；回调失败必须整体回滚
	if err := link(ctx, tx, ticketID); err != nil {
		return TicketOutcome{}, err
	}
	return TicketOutcome{ID: ticketID, Ref: ref}, nil
}

// ---------- 读权/主体/定义 stub ----------

type stubTicketViews struct {
	visible map[int64]bool
	all     bool
}

func (s stubTicketViews) CanViewTicket(_ context.Context, _, ticketID int64) bool {
	return s.all || s.visible[ticketID]
}

type stubOwners struct{}

func (stubOwners) LockOwnerFact(ctx context.Context, tx pgx.Tx, userID int64) (OwnerFact, error) {
	var fact OwnerFact
	var status int16
	err := tx.QueryRow(ctx, `SELECT status FROM users WHERE id = $1 FOR SHARE`, userID).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return OwnerFact{UserID: userID, Employed: false}, nil
	}
	if err != nil {
		return OwnerFact{}, err
	}
	fact.Employed = status == 1
	err = tx.QueryRow(ctx, `
		SELECT org_id FROM user_orgs WHERE user_id = $1 AND is_primary LIMIT 1
	`, userID).Scan(&fact.PrimaryOrgID)
	if errors.Is(err, pgx.ErrNoRows) {
		fact.PrimaryOrgID = 0
	} else if err != nil {
		return OwnerFact{}, err
	}
	return fact, nil
}

type stubDefinitions struct{ published map[string]DefinitionRef }

func (s stubDefinitions) CurrentPublished(_ context.Context, _ Reader, typeCode string) (DefinitionRef, error) {
	ref, ok := s.published[typeCode]
	if !ok {
		return DefinitionRef{}, errcode.ErrNotFound
	}
	return ref, nil
}

// ---------- 夹具 ----------

type dispatchFixture struct {
	actorID   int64
	ownerA    int64
	ownerB    int64
	orgID     int64 // 风险管理组织
	targetID  int64 // 派单目标组织
	ownerAOrg int64
	ownerBOrg int64
	familyID  int64
	typeCode  string
	target    Target
}

func setupDispatchFixture(t *testing.T, db *pgxpool.Pool, tag string) dispatchFixture {
	t.Helper()
	ctx := context.Background()
	f := dispatchFixture{typeCode: "synthetic_remediation_" + tag}

	var orgPath string
	require.NoError(t, db.QueryRow(ctx, `
		INSERT INTO organizations (code, name, path) VALUES ($1, $2, $3) RETURNING id, path
	`, "rd_org_"+tag, "RD org "+tag, "rd_org_"+tag).Scan(&f.orgID, &orgPath))
	_ = orgPath
	require.NoError(t, db.QueryRow(ctx, `
		INSERT INTO risk_families (fingerprint_scope, canonical_key_hash)
		VALUES ($1, $2) RETURNING id
	`, "rd_"+tag, tag).Scan(&f.familyID))
	require.NoError(t, db.QueryRow(ctx, `
		INSERT INTO organizations (code, name, path) VALUES ($1, $2, $3) RETURNING id
	`, "rd_target_"+tag, "RD target "+tag, "rd_target_"+tag).Scan(&f.targetID))
	execMustNoError(t, db, ctx, `
		INSERT INTO ticket_types (code, name) VALUES ($1, $2) ON CONFLICT DO NOTHING
	`, f.typeCode, "RD type "+tag)

	newUser := func(name string, status int16) int64 {
		var id int64
		require.NoError(t, db.QueryRow(ctx, `
			INSERT INTO users (username, password, real_name, status) VALUES ($1, 'x', $2, $3) RETURNING id
		`, name+"_"+tag, name, status).Scan(&id))
		return id
	}
	f.actorID = newUser("actor", 1)
	f.ownerA = newUser("ownerA", 1)
	f.ownerB = newUser("ownerB", 1)
	require.NoError(t, db.QueryRow(ctx, `
		INSERT INTO organizations (code, name, path) VALUES ($1, $2, $3) RETURNING id
	`, "rd_orga_"+tag, "orgA", "rd_orga_"+tag).Scan(&f.ownerAOrg))
	require.NoError(t, db.QueryRow(ctx, `
		INSERT INTO organizations (code, name, path) VALUES ($1, $2, $3) RETURNING id
	`, "rd_orgb_"+tag, "orgB", "rd_orgb_"+tag).Scan(&f.ownerBOrg))
	execMustNoError(t, db, ctx, `
		INSERT INTO user_orgs (user_id, org_id, is_primary) VALUES ($1, $2, true), ($3, $4, true)
	`, f.ownerA, f.ownerAOrg, f.ownerB, f.ownerBOrg)

	f.target = Target{
		OrganizationID:    f.targetID,
		TypeCode:          f.typeCode,
		DefinitionVersion: 1,
		DefinitionHash:    strings.Repeat("a", 64),
	}
	execMustNoError(t, db, ctx, `
		INSERT INTO risk_dispatch_targets
			(management_org_id, target_org_id, type_code, approved_scope, revision, enabled)
		VALUES ($1, $2, $3, 'scope-1', 1, true)
	`, f.orgID, f.targetID, f.typeCode)
	return f
}

func (f dispatchFixture) newRisk(t *testing.T, db *pgxpool.Pool, tag, finding, asset string,
	owner int64, level string, deadline time.Time, status string) int64 {
	t.Helper()
	ctx := context.Background()
	var riskID int64
	require.NoError(t, db.QueryRow(ctx, `
		INSERT INTO risk_items (human_key, family_id, status, revision, org_id, owner_user_id,
			level, deadline, title, finding_key, asset_provider, asset_external_id, created_by)
		VALUES ($1, $2, $3, 1, $4, $5, $6, $7, $8, $9, 'cmdb', $10, $11)
		RETURNING id
	`, "RD-"+tag+"-"+asset, f.familyID, status, f.orgID, owner, level, deadline,
		"risk "+asset, finding, asset, f.actorID).Scan(&riskID))
	return riskID
}

func requireRiskRevision(t *testing.T, db *pgxpool.Pool, riskID int64) int64 {
	t.Helper()
	var revision int64
	require.NoError(t, db.QueryRow(context.Background(),
		`SELECT revision FROM risk_items WHERE id = $1`, riskID).Scan(&revision))
	return revision
}

func requireRiskStatus(t *testing.T, db *pgxpool.Pool, riskID int64) string {
	t.Helper()
	var status string
	require.NoError(t, db.QueryRow(context.Background(),
		`SELECT status FROM risk_items WHERE id = $1`, riskID).Scan(&status))
	return status
}

func countRows(t *testing.T, db *pgxpool.Pool, query string, args ...any) int64 {
	t.Helper()
	var count int64
	require.NoError(t, db.QueryRow(context.Background(), query, args...).Scan(&count))
	return count
}

func execMustNoError(t *testing.T, db *pgxpool.Pool, ctx context.Context, query string, args ...any) {
	t.Helper()
	_, err := db.Exec(ctx, query, args...)
	require.NoError(t, err)
}

// newTestDispatcher 组装被测服务（全部窄接口均为合成实现）。
func newTestDispatcher(db *pgxpool.Pool, authorizer *stubAuthorizer, facts *stubFacts,
	tickets *stubTicketCreator, views stubTicketViews, definitions map[string]DefinitionRef) *Dispatcher {
	return NewDispatcher(DispatcherDeps{
		DB:          db,
		Tx:          poolTxOpener{db: db},
		Authorizer:  authorizer,
		Facts:       facts,
		Commit:      stubCommit{},
		Tickets:     tickets,
		TicketViews: views,
		Owners:      stubOwners{},
		Definitions: stubDefinitions{published: definitions},
	})
}

type poolTxOpener struct{ db *pgxpool.Pool }

func (o poolTxOpener) WithDispatchTransaction(ctx context.Context, _ int64, work func(context.Context, pgx.Tx) error) error {
	tx, err := o.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := work(ctx, tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
