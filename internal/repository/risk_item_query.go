package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// 风险域授权读取查询（L2 过滤先行，再筛选/计数——详设 §2.1）。
// 不可见与不存在统一 ErrRiskNotFound（404 同形，详设 §7）。

// RiskListFilter 列表筛选（服务层已归一化后的受控面）。
type RiskListFilter struct {
	Status        string
	Level         string
	OwnerUserID   int64 // 0 = 不过滤
	AffectedOrgID int64 // 0 = 不过滤
	SourceRef     string
	Q             string
	Sort          string
	Limit         int
	Offset        int
}

const riskItemViewSelect = `ri.id, ri.human_key, ri.family_id, ri.status, ri.closure_kind, ri.revision,
ri.org_id, ri.affected_org_id, ri.owner_user_id, ri.level, ri.deadline, ri.init_deadline, ri.treatment,
ri.title, ri.description, ri.system_ref, ri.finding_key, ri.asset_provider, ri.asset_external_id,
ri.evidence_refs, ri.attributes, ri.created_by,
COALESCE(current_source.mirror->'source_severity', 'null'::jsonb)`

// Match each source item to its applied watermark before choosing a source
// across the risk's mirrors. Observation time alone cannot order an ordered feed.
const riskCurrentSourceJoin = `
	LEFT JOIN LATERAL (
		SELECT o.ingest_id, o.source_ref, o.source_item_key, o.source_sequence, o.observed_at,
			sr.mirror, sr.content_hash, sr.mapping_ref
		FROM risk_observations o
		JOIN risk_source_records sr ON sr.source_ref = o.source_ref AND sr.source_item_key = o.source_item_key
			AND sr.content_hash = o.content_hash
		WHERE o.risk_id = ri.id AND o.processing_status = 'applied'
		ORDER BY o.observed_at DESC, o.ingest_id DESC
		LIMIT 1
	) current_source ON TRUE`

func scanRiskItemWithSeverity(row pgx.Row, extra ...any) (*RiskItemRow, []byte, error) {
	var r RiskItemRow
	var severity []byte
	fields := []any{&r.ID, &r.HumanKey, &r.FamilyID, &r.Status, &r.ClosureKind, &r.Revision,
		&r.OrgID, &r.AffectedOrgID, &r.OwnerUserID, &r.Level, &r.Deadline, &r.InitDeadline,
		&r.Treatment, &r.Title, &r.Description, &r.SystemRef,
		&r.FindingKey, &r.AssetProvider, &r.AssetExternalID, &r.EvidenceRefs, &r.Attributes, &r.CreatedBy,
		&severity}
	err := row.Scan(append(fields, extra...)...)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil, ErrRiskNotFound
	}
	if err != nil {
		return nil, nil, err
	}
	return &r, severity, nil
}

// riskListWhere 组装 L2 谓词与受控筛选；返回 WHERE 片段与追加参数。
func riskListWhere(filter RiskListFilter, actor int64) (string, []any) {
	where := []string{riskVisiblePredicate("$1")}
	args := []any{actor}
	if filter.Status != "" {
		args = append(args, filter.Status)
		where = append(where, fmt.Sprintf(`ri.status = $%d`, len(args)))
	}
	if filter.Level != "" {
		args = append(args, filter.Level)
		where = append(where, fmt.Sprintf(`ri.level = $%d`, len(args)))
	}
	if filter.OwnerUserID > 0 {
		args = append(args, filter.OwnerUserID)
		where = append(where, fmt.Sprintf(`ri.owner_user_id = $%d`, len(args)))
	}
	if filter.AffectedOrgID > 0 {
		args = append(args, filter.AffectedOrgID)
		where = append(where, fmt.Sprintf(`ri.affected_org_id = $%d`, len(args)))
	}
	if filter.SourceRef != "" {
		args = append(args, filter.SourceRef)
		where = append(where, fmt.Sprintf(`f.fingerprint_scope = $%d`, len(args)))
	}
	if filter.Q != "" {
		args = append(args, "%"+filter.Q+"%")
		where = append(where, fmt.Sprintf(`ri.title ILIKE $%d`, len(args)))
	}
	return "WHERE " + strings.Join(where, " AND "), args
}

func riskListOrder(sort string) string {
	switch sort {
	case "deadline":
		return "ORDER BY ri.deadline NULLS LAST, ri.id"
	case "level":
		return `ORDER BY CASE ri.level WHEN 'urgent' THEN 1 WHEN 'high' THEN 2 WHEN 'medium' THEN 3 WHEN 'low' THEN 4 ELSE 5 END, ri.id`
	default:
		return "ORDER BY ri.created_at DESC, ri.id DESC"
	}
}

// ListRiskItems 授权列表（固定字段排序加 ID 稳定次序）。
func ListRiskItems(ctx context.Context, q IAMDBTX, filter RiskListFilter, actor int64) ([]RiskItemRow, [][]byte, error) {
	where, args := riskListWhere(filter, actor)
	sql := `SELECT ` + riskItemViewSelect + `
		FROM risk_items ri
		JOIN organizations go ON go.id = ri.org_id
		LEFT JOIN organizations ao ON ao.id = ri.affected_org_id
		JOIN risk_families f ON f.id = ri.family_id
		` + riskCurrentSourceJoin + ` ` + where + ` ` + riskListOrder(filter.Sort) + `
		LIMIT $` + fmt.Sprint(len(args)+1) + ` OFFSET $` + fmt.Sprint(len(args)+2)
	args = append(args, filter.Limit, filter.Offset)
	rows, err := q.Query(ctx, sql, args...)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	var items []RiskItemRow
	var severities [][]byte
	for rows.Next() {
		r, sev, err := scanRiskItemWithSeverity(rows)
		if err != nil {
			return nil, nil, err
		}
		items = append(items, *r)
		severities = append(severities, sev)
	}
	return items, severities, rows.Err()
}

// CountRiskItems 同谓词计数（不泄露无权风险存在——同一谓词）。
func CountRiskItems(ctx context.Context, q IAMDBTX, filter RiskListFilter, actor int64) (int64, error) {
	where, args := riskListWhere(filter, actor)
	sql := `SELECT count(*) FROM risk_items ri
		JOIN organizations go ON go.id = ri.org_id
		LEFT JOIN organizations ao ON ao.id = ri.affected_org_id
		JOIN risk_families f ON f.id = ri.family_id
		` + where
	var total int64
	err := q.QueryRow(ctx, sql, args...).Scan(&total)
	return total, err
}

// GetRiskItemVisible 单条授权读取（不可见与不存在同 404）。
// 返回行、来源严重性镜像与镜像对应的已应用观察。
func GetRiskItemVisible(ctx context.Context, q IAMDBTX, id, actor int64) (*RiskItemRow, []byte, *RiskObservationRow, error) {
	row := q.QueryRow(ctx, `SELECT `+riskItemViewSelect+`
		, current_source.ingest_id, current_source.source_ref, current_source.source_item_key,
		current_source.source_sequence, current_source.observed_at
		FROM risk_items ri
		JOIN organizations go ON go.id = ri.org_id
		LEFT JOIN organizations ao ON ao.id = ri.affected_org_id
		`+riskCurrentSourceJoin+`
		WHERE ri.id=$1 AND `+riskVisiblePredicate("$2"), id, actor)
	var ingestID, sourceRef, sourceItemKey, sourceSequence *string
	var observedAt *time.Time
	r, severity, err := scanRiskItemWithSeverity(row, &ingestID, &sourceRef, &sourceItemKey, &sourceSequence, &observedAt)
	if err != nil {
		return nil, nil, nil, err
	}
	if ingestID == nil {
		return r, severity, nil, nil
	}
	obs := &RiskObservationRow{
		IngestID: *ingestID, SourceRef: *sourceRef, SourceItemKey: *sourceItemKey,
		SourceSequence: sourceSequence, ObservedAt: *observedAt,
	}
	return r, severity, obs, nil
}

// CanReadActiveTicketRisks applies the risk detail predicate to every active
// link. The caller holds the ticket lock for a live workflow command.
func CanReadActiveTicketRisks(ctx context.Context, q IAMDBTX, ticketID, actorID int64) (bool, error) {
	return TxBool(ctx, q, `SELECT count(*) > 0 AND COALESCE(bool_and(COALESCE(`+riskVisiblePredicate("$2")+`,false)),false)
		FROM risk_dispatch_groups dg
		JOIN risk_ticket_links l ON l.group_id=dg.id AND l.ended_at IS NULL
		JOIN risk_items ri ON ri.id=l.risk_id
		JOIN organizations go ON go.id=ri.org_id
		LEFT JOIN organizations ao ON ao.id=ri.affected_org_id
		WHERE dg.ticket_id=$1 AND dg.ended_at IS NULL`, ticketID, actorID)
}

// ListRiskEvents 风险历史（先复用 L2 谓词确认可读，再分页）。
func ListRiskEvents(ctx context.Context, q IAMDBTX, riskID, actor int64, limit, offset int) ([]RiskEventRow, int64, error) {
	visible, err := TxBool(ctx, q, `SELECT EXISTS(SELECT 1 FROM risk_items ri
		JOIN organizations go ON go.id = ri.org_id
		LEFT JOIN organizations ao ON ao.id = ri.affected_org_id
		WHERE ri.id=$1 AND `+riskVisiblePredicate("$2")+`)`, riskID, actor)
	if err != nil {
		return nil, 0, err
	}
	if !visible {
		return nil, 0, ErrRiskNotFound
	}
	var total int64
	if err := q.QueryRow(ctx, `SELECT count(*) FROM risk_events WHERE risk_id=$1`, riskID).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := q.Query(ctx, `SELECT id, risk_id, aggregate_revision, event_kind, ordinal,
		actor_id, occurred_at, COALESCE(operation_id,''), payload
		FROM risk_events WHERE risk_id=$1 ORDER BY occurred_at DESC, id DESC
		LIMIT $2 OFFSET $3`, riskID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var items []RiskEventRow
	for rows.Next() {
		var ev RiskEventRow
		if err := rows.Scan(&ev.ID, &ev.RiskID, &ev.AggregateRevision, &ev.EventKind, &ev.Ordinal,
			&ev.ActorID, &ev.OccurredAt, &ev.OperationID, &ev.Payload); err != nil {
			return nil, 0, err
		}
		items = append(items, ev)
	}
	return items, total, rows.Err()
}

// RiskEventRow 风险事件行。
type RiskEventRow struct {
	ID                int64
	RiskID            int64
	AggregateRevision int64
	EventKind         string
	Ordinal           int
	ActorID           int64
	OccurredAt        time.Time
	OperationID       string
	Payload           []byte
}

// GetIngestReceiptAuthorized 来源收件回执受权查询：
// 已回链风险的复用风险 L2 谓词；未回链的按来源 allowed_org 组织可见性判定（详设 §2.1）。
func GetIngestReceiptAuthorized(ctx context.Context, q IAMDBTX, ingestID string, actor int64) (*RiskObservationRow, error) {
	obs, err := scanObservationRow(q.QueryRow(ctx, `SELECT `+riskObservationColumns+`
		FROM risk_observations WHERE ingest_id=$1`, ingestID))
	if err != nil {
		return nil, err
	}
	if obs.RiskID != nil {
		visible, err := TxBool(ctx, q, `SELECT EXISTS(SELECT 1 FROM risk_items ri
			JOIN organizations go ON go.id = ri.org_id
			LEFT JOIN organizations ao ON ao.id = ri.affected_org_id
			WHERE ri.id=$1 AND `+riskVisiblePredicate("$2")+`)`, *obs.RiskID, actor)
		if err != nil {
			return nil, err
		}
		if visible {
			return obs, nil
		}
		return nil, ErrRiskNotFound
	}
	allowed, err := TxBool(ctx, q, `SELECT EXISTS(
		SELECT 1 FROM risk_sources s
		JOIN organizations so ON so.id = ANY(s.allowed_org_ids)
		JOIN user_orgs m ON m.org_id = ANY(s.allowed_org_ids)
		JOIN organizations mo ON mo.id = m.org_id
		WHERE s.source_ref=$1 AND m.user_id=$2
		  AND (`+OrgSourceActiveSQL("mo")+`)
		  AND (m.expires_at IS NULL OR m.expires_at > NOW())
		  AND so.path <@ mo.path)`, obs.SourceRef, actor)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, ErrRiskNotFound
	}
	return obs, nil
}
