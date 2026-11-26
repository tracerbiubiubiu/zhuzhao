package riskdispatch

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
)

// TargetMapping risk_dispatch_targets 行投影（本包唯一写主的派单目标配置）。
type TargetMapping struct {
	ApprovedScope string
	Revision      int64
	Enabled       bool
}

// lockTargetMapping 组内共享锁读目标映射；无行/停用返回不可派原因。
func lockTargetMapping(ctx context.Context, tx pgx.Tx, mgmtOrgID int64, target Target) (TargetMapping, error) {
	var mapping TargetMapping
	err := tx.QueryRow(ctx, `
		SELECT approved_scope, revision, enabled
		FROM risk_dispatch_targets
		WHERE management_org_id = $1 AND target_org_id = $2 AND type_code = $3
		FOR SHARE
	`, mgmtOrgID, target.OrganizationID, target.TypeCode).Scan(
		&mapping.ApprovedScope, &mapping.Revision, &mapping.Enabled)
	if errors.Is(err, pgx.ErrNoRows) {
		return TargetMapping{}, fmt.Errorf("dispatch target mapping missing: %w", errcode.ErrForbidden)
	}
	if err != nil {
		return TargetMapping{}, fmt.Errorf("read dispatch target mapping: %w", err)
	}
	if !mapping.Enabled {
		return TargetMapping{}, fmt.Errorf("dispatch target mapping disabled: %w", errcode.ErrForbidden)
	}
	return mapping, nil
}

// readTargetMapping 预览用无锁读（无行不视为错误，交分组器给逐项原因）。
func readTargetMapping(ctx context.Context, db Reader, mgmtOrgID int64, target Target) (TargetMapping, bool, error) {
	var mapping TargetMapping
	err := db.QueryRow(ctx, `
		SELECT approved_scope, revision, enabled
		FROM risk_dispatch_targets
		WHERE management_org_id = $1 AND target_org_id = $2 AND type_code = $3
	`, mgmtOrgID, target.OrganizationID, target.TypeCode).Scan(
		&mapping.ApprovedScope, &mapping.Revision, &mapping.Enabled)
	if errors.Is(err, pgx.ErrNoRows) {
		return TargetMapping{}, false, nil
	}
	if err != nil {
		return TargetMapping{}, false, fmt.Errorf("read dispatch target mapping: %w", err)
	}
	return mapping, true, nil
}

// advisoryGroupKey 组身份（finding_key+owner）的稳定 advisory 键；
// 同组并发 apply 在进入行锁前先在此串行化（详设 §4.3 group 稳定键）。
func advisoryGroupKey(findingKey string, ownerUserID int64) int64 {
	sum := sha256.Sum256([]byte("risk_dispatch_group:" + findingKey + ":" + fmt.Sprint(ownerUserID)))
	return int64(binary.BigEndian.Uint64(sum[:8]) & (1<<63 - 1))
}

// LockHandoffGroupIdentities serializes handoff with dispatch on both owners.
// Acquire the same keys in stable order before either command locks group rows.
func LockHandoffGroupIdentities(ctx context.Context, tx pgx.Tx, findingKey string, previous, successor int64) error {
	a, b := advisoryGroupKey(findingKey, previous), advisoryGroupKey(findingKey, successor)
	if a > b {
		a, b = b, a
	}
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, a); err != nil {
		return err
	}
	if a == b {
		return nil
	}
	_, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, b)
	return err
}

// LiveGroup 存活在办组行（同一漏洞同一责任人最多一张在办票）。
type LiveGroup struct {
	ID                 int64
	GroupKey           string
	TicketID           int64
	TicketRefSnapshot  string
	BusinessDueAt      time.Time
	OwnerOrgAtDispatch int64
	ActiveLinks        int64
}

const liveGroupColumns = `
	g.id, COALESCE(g.retest_group_key, g.group_key), g.ticket_id, g.ticket_ref_snapshot,
	g.business_due_at, COALESCE(g.owner_org_at_dispatch, 0),
	(SELECT COUNT(*) FROM risk_ticket_links l WHERE l.group_id = g.id AND l.ended_at IS NULL)
`

func scanLiveGroup(row pgx.Row) (LiveGroup, error) {
	var group LiveGroup
	err := row.Scan(&group.ID, &group.GroupKey, &group.TicketID, &group.TicketRefSnapshot,
		&group.BusinessDueAt, &group.OwnerOrgAtDispatch, &group.ActiveLinks)
	if errors.Is(err, pgx.ErrNoRows) {
		return LiveGroup{}, nil
	}
	if err != nil {
		return LiveGroup{}, fmt.Errorf("read live dispatch group: %w", err)
	}
	return group, nil
}

// readLiveGroup 预览用无锁读。
func readLiveGroup(ctx context.Context, db Reader, findingKey string, ownerUserID int64) (LiveGroup, error) {
	return scanLiveGroup(db.QueryRow(ctx, `
		SELECT `+liveGroupColumns+`
		FROM risk_dispatch_groups g
		WHERE g.finding_key = $1 AND g.owner_user_id = $2 AND g.ended_at IS NULL
		LIMIT 1
	`, findingKey, ownerUserID))
}

// lockLiveGroup apply 用行锁读。
func lockLiveGroup(ctx context.Context, tx pgx.Tx, findingKey string, ownerUserID int64) (LiveGroup, error) {
	return scanLiveGroup(tx.QueryRow(ctx, `
		SELECT `+liveGroupColumns+`
		FROM risk_dispatch_groups g
		WHERE g.finding_key = $1 AND g.owner_user_id = $2 AND g.ended_at IS NULL
		LIMIT 1
		FOR UPDATE OF g
	`, findingKey, ownerUserID))
}

// insertDispatchGroupTx 新建组行并保存派单当时责任人/部门/到期事实。
func insertDispatchGroupTx(ctx context.Context, tx pgx.Tx, findingKey string, ownerUserID int64,
	mgmtOrgID int64, target Target, scope string, groupKey string, ticket TicketOutcome,
	operatorUserID, ownerOrgID int64, due time.Time) (int64, error) {
	var id int64
	err := tx.QueryRow(ctx, `
		INSERT INTO risk_dispatch_groups
			(finding_key, owner_user_id, owner_org_at_dispatch, management_org_id, target_org_id,
			 type_code, definition_version, approved_scope, group_key, ticket_id,
			 ticket_ref_snapshot, operator_user_id, business_due_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		RETURNING id
	`, findingKey, ownerUserID, ownerOrgID, mgmtOrgID, target.OrganizationID, target.TypeCode,
		target.DefinitionVersion, scope, groupKey, ticket.ID, ticket.Ref,
		operatorUserID, due.UTC()).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("save dispatch group: %w", err)
	}
	return id, nil
}

// insertDispatchLinkTx 追加一条活跃风险关联（唯一活跃约束兜底防重）。
func insertDispatchLinkTx(ctx context.Context, tx pgx.Tx, riskID, groupID, ownerUserID int64) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO risk_ticket_links (risk_id, group_id, owner_at_link)
		VALUES ($1, $2, $3)
	`, riskID, groupID, ownerUserID)
	if err != nil {
		return fmt.Errorf("save dispatch link: %w", err)
	}
	return nil
}

// activeLinkTicket 查风险当前活跃关联所在票；无活跃关联返回 ok=false。
func activeLinkTicket(ctx context.Context, db Reader, riskID int64) (ticketID int64, ok bool, err error) {
	err = db.QueryRow(ctx, `
		SELECT g.ticket_id FROM risk_ticket_links l
		JOIN risk_dispatch_groups g ON g.id = l.group_id
		WHERE l.risk_id = $1 AND l.ended_at IS NULL
		LIMIT 1
	`, riskID).Scan(&ticketID)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("read active risk link: %w", err)
	}
	return ticketID, true, nil
}

// GroupReceiptItem 回执内的逐项结果。
type GroupReceiptItem struct {
	RiskID int64            `json:"risk_id"`
	Result ItemResult       `json:"result"`
	Reason string           `json:"reason,omitempty"`
	Ticket *TicketReference `json:"ticket,omitempty"`
}

// TicketReference 原票引用（仅在当前主体仍获权时投影；不可读时整体省略，
// 不给「存在但被隐藏」的暗示——详设 §7）。
type TicketReference struct {
	ID  int64  `json:"id,string"`
	Ref string `json:"ref"`
}

// viewTicket 当前主体仍获权才返回引用（ref 为桥内已存快照），否则 nil。
func (d *Dispatcher) viewTicket(ctx context.Context, actorUserID, ticketID int64, ref string) *TicketReference {
	if ticketID <= 0 || d.deps.TicketViews == nil {
		return nil
	}
	if !d.deps.TicketViews.CanViewTicket(ctx, actorUserID, ticketID) {
		return nil
	}
	return &TicketReference{ID: ticketID, Ref: ref}
}

// GroupReceipt 已终结组的不可变回执（completed 或 failed）。
type GroupReceipt struct {
	GroupOperationID   string             `json:"group_operation_id"`
	Status             string             `json:"status"`
	TicketID           int64              `json:"ticket_id"`
	TicketRef          string             `json:"ticket_ref"`
	TicketCreated      bool               `json:"ticket_created"`
	OwnerUserID        int64              `json:"owner_user_id,string"`
	OwnerOrgAtDispatch int64              `json:"owner_org_at_dispatch,string"`
	Priority           int                `json:"priority"`
	BusinessDueAt      time.Time          `json:"business_due_at"`
	Items              []GroupReceiptItem `json:"items"`
	CompletedAt        time.Time          `json:"completed_at"`
}

// saveGroupReceiptTx 在持有 parent 行锁时写入终态回执。
func saveGroupReceiptTx(ctx context.Context, tx pgx.Tx, actorID int64, operationID, groupKey string, receipt GroupReceipt) error {
	body, err := json.Marshal(receipt)
	if err != nil {
		return fmt.Errorf("encode dispatch group receipt: %w", err)
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO risk_dispatch_group_receipts (actor_id, operation_id, group_key, result)
		VALUES ($1, $2, $3, $4)
	`, actorID, operationID, groupKey, body)
	if err != nil {
		return fmt.Errorf("save dispatch group receipt: %w", err)
	}
	return nil
}

// readGroupReceipt 原号重投读回执；无行返回 ok=false。
func readGroupReceipt(ctx context.Context, db Reader, actorID int64, operationID, groupKey string) (GroupReceipt, bool, error) {
	var body []byte
	err := db.QueryRow(ctx, `
		SELECT result FROM risk_dispatch_group_receipts
		WHERE actor_id = $1 AND operation_id = $2 AND group_key = $3
	`, actorID, operationID, groupKey).Scan(&body)
	if errors.Is(err, pgx.ErrNoRows) {
		return GroupReceipt{}, false, nil
	}
	if err != nil {
		return GroupReceipt{}, false, fmt.Errorf("read dispatch group receipt: %w", err)
	}
	var receipt GroupReceipt
	if err := json.Unmarshal(body, &receipt); err != nil {
		return GroupReceipt{}, false, fmt.Errorf("decode dispatch group receipt: %w", err)
	}
	return receipt, true, nil
}

// deriveGroupOperationID 派生组操作号：parent + plan_hash + group_key 确定性派生（详设 §5.1）。
func deriveGroupOperationID(parentOperationID, planHash, groupKey string) string {
	sum := sha256.Sum256([]byte(parentOperationID + ":" + planHash + ":" + groupKey))
	return fmt.Sprintf("%x", sum)
}
