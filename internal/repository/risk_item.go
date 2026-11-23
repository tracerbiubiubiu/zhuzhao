package repository

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
)

// Phase 6 B2 第 6 项（D08/D09）风险登记与核实域仓储。
// DDL SSOT = migrations/000089_risk_registration；详设 risk-module-detail §3。
// L2 读谓词 = owner ∨ 主锚管理组织成员 ∨ 副锚受影响组织成员（详设 §7）；
// L3 管理范围 = 管理组织祖先链上的 admin/owner 成员资格。

// RiskItemRow 风险周期行（risk_items）。
type RiskItemRow struct {
	ID              int64
	HumanKey        string
	FamilyID        int64
	Status          string
	ClosureKind     *string
	Revision        int64
	OrgID           int64
	AffectedOrgID   *int64
	OwnerUserID     *int64
	Level           *string
	Deadline        *time.Time
	InitDeadline    *time.Time
	Treatment       *string
	Title           string
	Description     *string
	SystemRef       *string
	FindingKey      *string
	AssetProvider   *string
	AssetExternalID *string
	EvidenceRefs    []byte
	Attributes      []byte
	CreatedBy       int64
}

// RiskSourceRow 合成来源登记行（risk_sources）。
type RiskSourceRow struct {
	SourceRef        string
	DisplayName      string
	FingerprintScope string
	KeySchemaVersion int
	Ordered          bool
	MappingRef       string
	AllowedOrgIDs    []int64
	DefaultOrgID     *int64
	Enabled          bool
}

// RiskObservationRow 接入台账行（risk_observations）。
type RiskObservationRow struct {
	IngestID         string
	SourceRef        string
	EventID          string
	SourceItemKey    string
	SourceSequence   *string
	ObservedAt       time.Time
	ContentHash      []byte
	MappingRef       string
	Kind             string
	ProcessingStatus string
	ResultKind       string
	RiskID           *int64
	SafeReason       *string
	ReceivedAt       time.Time
}

// RiskOperationRow durable 操作收据行（risk_operations）。
type RiskOperationRow struct {
	OperationID    string
	RequestHash    []byte
	Command        string
	ResultStatus   string
	ResultRiskID   *int64
	ResultRevision *int64
	CreatedAt      time.Time
}

const riskItemColumns = `id, human_key, family_id, status, closure_kind, revision, org_id, affected_org_id,
owner_user_id, level, deadline, init_deadline, treatment, title, description, system_ref,
finding_key, asset_provider, asset_external_id, evidence_refs, attributes, created_by`

func scanRiskItemRow(row pgx.Row) (*RiskItemRow, error) {
	var r RiskItemRow
	err := row.Scan(&r.ID, &r.HumanKey, &r.FamilyID, &r.Status, &r.ClosureKind, &r.Revision,
		&r.OrgID, &r.AffectedOrgID, &r.OwnerUserID, &r.Level, &r.Deadline, &r.InitDeadline,
		&r.Treatment, &r.Title, &r.Description, &r.SystemRef,
		&r.FindingKey, &r.AssetProvider, &r.AssetExternalID, &r.EvidenceRefs, &r.Attributes, &r.CreatedBy)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrRiskNotFound
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// riskVisiblePredicate L2 读可见谓词（参数占位符由调用方给定点位，如 "$2"）。
// 别名约定：ri=risk_items、go=管理组织、ao=受影响组织。
func riskVisiblePredicate(actorParam string) string {
	return `(
		ri.owner_user_id = ` + actorParam + `
		OR EXISTS (
			SELECT 1 FROM user_orgs m
			JOIN organizations mo ON mo.id = m.org_id
			WHERE m.user_id = ` + actorParam + `
			  AND (` + OrgSourceActiveSQL("mo") + `)
			  AND (m.expires_at IS NULL OR m.expires_at > NOW())
			  AND (go.path <@ mo.path OR ao.path <@ mo.path)
		)
	)`
}

// riskManagePredicate L3 主锚管理范围谓词（参数占位符由调用方给定点位）。
func riskManagePredicate(actorParam string) string {
	return `EXISTS (
		SELECT 1 FROM user_orgs m
		JOIN organizations mo ON mo.id = m.org_id
		WHERE m.user_id = ` + actorParam + `
		  AND (` + OrgSourceActiveSQL("mo") + `)
		  AND (m.expires_at IS NULL OR m.expires_at > NOW())
		  AND m.org_member_role IN ('admin','owner')
		  AND go.path <@ mo.path
	)`
}

// GetRiskSource 读取来源登记（不存在返回 ErrRiskSourceUnknown）。
func GetRiskSource(ctx context.Context, q IAMDBTX, sourceRef string) (*RiskSourceRow, error) {
	row := q.QueryRow(ctx, `SELECT source_ref, display_name, fingerprint_scope, key_schema_version,
		ordered, mapping_ref, allowed_org_ids, default_org_id, enabled
		FROM risk_sources WHERE source_ref=$1`, sourceRef)
	var r RiskSourceRow
	err := row.Scan(&r.SourceRef, &r.DisplayName, &r.FingerprintScope, &r.KeySchemaVersion,
		&r.Ordered, &r.MappingRef, &r.AllowedOrgIDs, &r.DefaultOrgID, &r.Enabled)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrRiskSourceUnknown
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// UpsertFamilyTx 首插用 ON CONFLICT 取得同 family，再锁行（详设 §4 第 3 条）。
func UpsertFamilyTx(ctx context.Context, tx pgx.Tx, scope, canonicalKeyHash string, schemaVersion int) (int64, error) {
	var id int64
	err := tx.QueryRow(ctx, `INSERT INTO risk_families(fingerprint_scope, canonical_key_hash, key_schema_version)
		VALUES($1,$2,$3)
		ON CONFLICT (fingerprint_scope, canonical_key_hash, key_schema_version) DO NOTHING
		RETURNING id`, scope, canonicalKeyHash, schemaVersion).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		err = tx.QueryRow(ctx, `SELECT id FROM risk_families
			WHERE fingerprint_scope=$1 AND canonical_key_hash=$2 AND key_schema_version=$3 FOR UPDATE`,
			scope, canonicalKeyHash, schemaVersion).Scan(&id)
	}
	if err != nil {
		return 0, fmt.Errorf("upsert risk family: %w", err)
	}
	return id, nil
}

// FindItemIDByFamilyTx 家族现有周期（本轮不建回归代）。
func FindItemIDByFamilyTx(ctx context.Context, tx pgx.Tx, familyID int64) (int64, bool, error) {
	var id int64
	err := tx.QueryRow(ctx, `SELECT id FROM risk_items WHERE family_id=$1 ORDER BY id LIMIT 1`, familyID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return id, true, nil
}

// LockObservedRiskItemsTx locks existing cycles in dispatch order. The caller
// has already serialized each supplied family key with the source advisory lock.
func LockObservedRiskItemsTx(ctx context.Context, tx pgx.Tx, scope string, schemaVersion int, keys []string) error {
	rows, err := tx.Query(ctx, `
		SELECT r.id FROM risk_items r
		JOIN risk_families f ON f.id = r.family_id
		WHERE f.fingerprint_scope = $1 AND f.key_schema_version = $2
			AND f.canonical_key_hash = ANY($3)
			AND r.id = (SELECT min(first.id) FROM risk_items first WHERE first.family_id = f.id)
		ORDER BY r.id FOR UPDATE OF r`, scope, schemaVersion, keys)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
	}
	return rows.Err()
}

// InsertRiskItemTx 落风险周期行（human_key 由服务层生成后传入）。
func InsertRiskItemTx(ctx context.Context, tx pgx.Tx, r *RiskItemRow) (int64, error) {
	var id int64
	err := tx.QueryRow(ctx, `INSERT INTO risk_items
		(human_key, family_id, status, revision, org_id, affected_org_id, owner_user_id,
		 level, deadline, init_deadline, treatment, title, description, system_ref,
		 finding_key, asset_provider, asset_external_id, evidence_refs, attributes, created_by)
		VALUES ($1,$2,$3,1,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19)
		RETURNING id`,
		r.HumanKey, r.FamilyID, r.Status, r.OrgID, r.AffectedOrgID, r.OwnerUserID,
		r.Level, r.Deadline, r.InitDeadline, r.Treatment, r.Title, r.Description, r.SystemRef,
		r.FindingKey, r.AssetProvider, r.AssetExternalID, r.EvidenceRefs, r.Attributes, r.CreatedBy).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("insert risk item: %w", err)
	}
	return id, nil
}

// LockRiskItemTx 锁定风险行（IAM 守卫之后、按稳定键→risk ID 锁序进入）。
func LockRiskItemTx(ctx context.Context, tx pgx.Tx, id int64) (*RiskItemRow, error) {
	return scanRiskItemRow(tx.QueryRow(ctx,
		`SELECT `+riskItemColumns+` FROM risk_items ri WHERE ri.id=$1 FOR UPDATE`, id))
}

// RiskItemAuthorizedTx 行锁内读取风险行并判定写入口授权（详设 §7）：
// 先 L2 可见性——不可见与不存在同 404（不泄露对象存在性）；
// 再 L3——可见而缺当前资格才 403。requireManage=true 仅主锚管理范围；
// false = 管理范围 ∨ owner。
func RiskItemAuthorizedTx(ctx context.Context, tx pgx.Tx, id, actor int64, requireManage bool) (*RiskItemRow, error) {
	r, err := LockRiskItemTx(ctx, tx, id)
	if err != nil {
		return nil, err
	}
	var visible, manage bool
	err = tx.QueryRow(ctx, `SELECT `+riskVisiblePredicate("$2")+`, `+riskManagePredicate("$2")+`
		FROM risk_items ri
		JOIN organizations go ON go.id = ri.org_id
		LEFT JOIN organizations ao ON ao.id = ri.affected_org_id
		WHERE ri.id=$1`, id, actor).Scan(&visible, &manage)
	if err != nil {
		return nil, fmt.Errorf("risk write authorization: %w", err)
	}
	if !visible {
		return nil, ErrRiskNotFound
	}
	if !manage && (requireManage || r.OwnerUserID == nil || *r.OwnerUserID != actor) {
		return nil, errcode.ErrNoPermission
	}
	return r, nil
}

// AssessRiskItemTx 核实/评估：CAS 推进 revision；deadline 只在显式给出时覆盖，首次写入即 init_deadline。
func AssessRiskItemTx(ctx context.Context, tx pgx.Tx, id int64, expectedRevision int64,
	level, treatment string, deadline *time.Time) (*RiskItemRow, error) {
	row := tx.QueryRow(ctx, `UPDATE risk_items SET
		status='assessed', level=$3, treatment=$4,
		deadline=COALESCE($5, deadline), init_deadline=COALESCE(init_deadline, $5),
		revision=revision+1, updated_at=NOW()
		WHERE id=$1 AND revision=$2 AND status IN ('new','assessed')
		RETURNING `+riskItemColumns, id, expectedRevision, level, treatment, deadline)
	r, err := scanRiskItemRow(row)
	if errors.Is(err, ErrRiskNotFound) {
		return nil, errcode.ErrConcurrentModification
	}
	return r, err
}

// AssignRiskItemTx 责任确认：CAS 推进 revision（主锚管理者专属，服务层先验管理范围）。
func AssignRiskItemTx(ctx context.Context, tx pgx.Tx, id int64, expectedRevision, ownerUserID int64) (*RiskItemRow, error) {
	row := tx.QueryRow(ctx, `UPDATE risk_items SET
		owner_user_id=$3, revision=revision+1, updated_at=NOW()
		WHERE id=$1 AND revision=$2 AND status <> 'closed'
		RETURNING `+riskItemColumns, id, expectedRevision, ownerUserID)
	r, err := scanRiskItemRow(row)
	if errors.Is(err, ErrRiskNotFound) {
		return nil, errcode.ErrConcurrentModification
	}
	return r, err
}

// UserActive 指派目标必须是启用中的账号。
func UserActive(ctx context.Context, q IAMDBTX, userID int64) (bool, error) {
	return TxBool(ctx, q, `SELECT EXISTS(SELECT 1 FROM users WHERE id=$1 AND status=1 AND deleted_at IS NULL)`, userID)
}

// ClaimOperationTx 认领 durable 操作收据身份（result 待回填）；同号已存在时返回原行与 replayed=true。
func ClaimOperationTx(ctx context.Context, tx pgx.Tx, actorID int64, operationID string,
	requestHash []byte, command string) (*RiskOperationRow, bool, error) {
	tag, err := tx.Exec(ctx, `INSERT INTO risk_operations
		(actor_id, operation_id, request_hash, command, result_status)
		VALUES($1,$2,$3,$4,'pending') ON CONFLICT (actor_id, operation_id) DO NOTHING`,
		actorID, operationID, requestHash, command)
	if err != nil {
		return nil, false, fmt.Errorf("claim risk operation: %w", err)
	}
	if tag.RowsAffected() == 1 {
		return &RiskOperationRow{OperationID: operationID, RequestHash: requestHash,
			Command: command, ResultStatus: "pending", CreatedAt: time.Now()}, false, nil
	}
	existing, getErr := GetOperationTx(ctx, tx, actorID, operationID)
	if getErr != nil {
		return nil, false, getErr
	}
	return existing, true, nil
}

// CompleteOperationTx 回填收据结果坐标（与业务写同事务提交）。
func CompleteOperationTx(ctx context.Context, tx pgx.Tx, actorID int64, operationID string,
	resultStatus string, riskID *int64, resultRevision *int64) error {
	_, err := tx.Exec(ctx, `UPDATE risk_operations SET
		result_status=$3, result_risk_id=$4, result_revision=$5
		WHERE actor_id=$1 AND operation_id=$2`,
		actorID, operationID, resultStatus, riskID, resultRevision)
	return err
}

// GetOperationTx 读取原收据。
func GetOperationTx(ctx context.Context, q IAMDBTX, actorID int64, operationID string) (*RiskOperationRow, error) {
	row := q.QueryRow(ctx, `SELECT operation_id, request_hash, command, result_status, result_risk_id, result_revision, created_at
		FROM risk_operations WHERE actor_id=$1 AND operation_id=$2`, actorID, operationID)
	var r RiskOperationRow
	err := row.Scan(&r.OperationID, &r.RequestHash, &r.Command, &r.ResultStatus, &r.ResultRiskID, &r.ResultRevision, &r.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrRiskNotFound
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// NextEventOrdinalTx 同 (risk, revision, kind) 的下一个 ordinal（唯一约束兜底并发）。
func NextEventOrdinalTx(ctx context.Context, tx pgx.Tx, riskID, aggregateRevision int64, kind string) (int, error) {
	var n int
	err := tx.QueryRow(ctx, `SELECT count(*) FROM risk_events
		WHERE risk_id=$1 AND aggregate_revision=$2 AND event_kind=$3`, riskID, aggregateRevision, kind).Scan(&n)
	if err != nil {
		return 0, err
	}
	return n, nil
}

// InsertEventTx 追加风险域事件（与业务变更同事务）。
func InsertEventTx(ctx context.Context, tx pgx.Tx, riskID, aggregateRevision int64,
	kind string, ordinal int, actorID int64, operationID string, payload []byte) error {
	_, err := tx.Exec(ctx, `INSERT INTO risk_events
		(risk_id, aggregate_revision, event_kind, ordinal, actor_id, occurred_at, operation_id, payload)
		VALUES($1,$2,$3,$4,$5,NOW(),$6,$7)`,
		riskID, aggregateRevision, kind, ordinal, actorID, pointerOrNil(operationID), payload)
	if err != nil {
		return fmt.Errorf("insert risk event: %w", err)
	}
	return nil
}

// InsertObservationTx 先收件；同 (source,event) 冲突时返回既有行与 inserted=false。
func InsertObservationTx(ctx context.Context, tx pgx.Tx, ingestID, sourceRef,
	eventID, sourceItemKey string, sourceSequence *string, observedAt time.Time,
	contentHash []byte, mappingRef, kind string) (*RiskObservationRow, bool, error) {
	row := tx.QueryRow(ctx, `INSERT INTO risk_observations
		(ingest_id, source_ref, event_id, source_item_key, source_sequence, observed_at, content_hash, mapping_ref, kind)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)
		ON CONFLICT (source_ref, event_id) DO NOTHING
		RETURNING ingest_id, source_ref, event_id, source_item_key, source_sequence, observed_at,
			content_hash, mapping_ref, kind, processing_status, result_kind, risk_id, safe_reason, received_at`,
		ingestID, sourceRef, eventID, sourceItemKey, sourceSequence, observedAt, contentHash, mappingRef, kind)
	var r RiskObservationRow
	err := row.Scan(&r.IngestID, &r.SourceRef, &r.EventID, &r.SourceItemKey, &r.SourceSequence,
		&r.ObservedAt, &r.ContentHash, &r.MappingRef, &r.Kind, &r.ProcessingStatus, &r.ResultKind,
		&r.RiskID, &r.SafeReason, &r.ReceivedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		existing, getErr := GetObservationByEventTx(ctx, tx, sourceRef, eventID)
		return existing, false, getErr
	}
	if err != nil {
		return nil, false, err
	}
	return &r, true, nil
}

// GetObservationByEventTx 同唯一键的既有观察行。
func GetObservationByEventTx(ctx context.Context, q IAMDBTX, sourceRef, eventID string) (*RiskObservationRow, error) {
	return scanObservationRow(q.QueryRow(ctx, `SELECT `+riskObservationColumns+`
		FROM risk_observations WHERE source_ref=$1 AND event_id=$2`, sourceRef, eventID))
}

// GetObservationBySequenceTx 占据该 (source, item, sequence) 的已收观察行。
func GetObservationBySequenceTx(ctx context.Context, q IAMDBTX, sourceRef, itemKey, sequence string) (*RiskObservationRow, error) {
	return scanObservationRow(q.QueryRow(ctx, `SELECT `+riskObservationColumns+`
		FROM risk_observations WHERE source_ref=$1 AND source_item_key=$2 AND source_sequence=$3`,
		sourceRef, itemKey, sequence))
}

const riskObservationColumns = `ingest_id, source_ref, event_id, source_item_key, source_sequence,
	observed_at, content_hash, mapping_ref, kind, processing_status, result_kind, risk_id, safe_reason, received_at`

func scanObservationRow(row pgx.Row) (*RiskObservationRow, error) {
	var r RiskObservationRow
	err := row.Scan(&r.IngestID, &r.SourceRef, &r.EventID, &r.SourceItemKey, &r.SourceSequence,
		&r.ObservedAt, &r.ContentHash, &r.MappingRef, &r.Kind, &r.ProcessingStatus, &r.ResultKind,
		&r.RiskID, &r.SafeReason, &r.ReceivedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrRiskNotFound
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// ApplyObservationTx 观察应用收尾：处理状态/结果/回链风险与 safe_reason。
func ApplyObservationTx(ctx context.Context, tx pgx.Tx, ingestID, processingStatus, resultKind string,
	riskID *int64, safeReason string) error {
	_, err := tx.Exec(ctx, `UPDATE risk_observations SET
		processing_status=$2, result_kind=$3, risk_id=$4, safe_reason=NULLIF($5,'')
		WHERE ingest_id=$1`, ingestID, processingStatus, resultKind, riskID, safeReason)
	return err
}

// LockSourceRecordTx 行锁读取来源镜像；无记录返回 (nil,false,nil)（首观察随后插入）。
func LockSourceRecordTx(ctx context.Context, tx pgx.Tx, sourceRef, sourceItemKey string) (*string, bool, error) {
	var latest *string
	err := tx.QueryRow(ctx, `SELECT latest_sequence FROM risk_source_records
		WHERE source_ref=$1 AND source_item_key=$2 FOR UPDATE`, sourceRef, sourceItemKey).Scan(&latest)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("lock risk source record: %w", err)
	}
	return latest, true, nil
}

// SourceItemLockKey 来源项稳定键的事务级 advisory 锁键
// （sha256(scope+"\n"+itemKey) 前 8 字节大端有符号——同键恒同锁）。
func SourceItemLockKey(scope, itemKey string) int64 {
	sum := sha256.Sum256([]byte(scope + "\n" + itemKey))
	return int64(binary.BigEndian.Uint64(sum[:8]))
}

// AdvisoryLockSourceItemTx 取来源项事务级 advisory 锁（详设 §4 显式锁口径）。
// 并发首报按稳定键先串行：锁释放前其他同键事务进不了应用路径，
// 锁内读到的镜像/序号即现势；批次须按键升序获取防交叉等待。
func AdvisoryLockSourceItemTx(ctx context.Context, tx pgx.Tx, key int64) error {
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, key); err != nil {
		return fmt.Errorf("advisory lock source item: %w", err)
	}
	return nil
}

// InsertSourceRecordTx 首观察建立镜像行。
func InsertSourceRecordTx(ctx context.Context, tx pgx.Tx, sourceRef, sourceItemKey string,
	sequence *string, observedAt time.Time, contentHash []byte, mappingRef string, mirror []byte) error {
	_, err := tx.Exec(ctx, `INSERT INTO risk_source_records
		(source_ref, source_item_key, latest_sequence, observed_at, content_hash, mapping_ref, mirror)
		VALUES($1,$2,$3,$4,$5,$6,$7)
		ON CONFLICT (source_ref, source_item_key) DO NOTHING`,
		sourceRef, sourceItemKey, sequence, observedAt, contentHash, mappingRef, mirror)
	if err != nil {
		return fmt.Errorf("insert risk source record: %w", err)
	}
	return nil
}

// UpdateSourceRecordTx 正向推进镜像（只在确认新观察时调用；stale 不覆盖）。
func UpdateSourceRecordTx(ctx context.Context, tx pgx.Tx, sourceRef, sourceItemKey string,
	sequence *string, observedAt time.Time, contentHash []byte, mappingRef string, mirror []byte) error {
	_, err := tx.Exec(ctx, `UPDATE risk_source_records SET
		latest_sequence=$3, observed_at=$4, content_hash=$5, mapping_ref=$6, mirror=$7,
		revision=revision+1, updated_at=NOW()
		WHERE source_ref=$1 AND source_item_key=$2`,
		sourceRef, sourceItemKey, sequence, observedAt, contentHash, mappingRef, mirror)
	return err
}

// InsertConflictTx 冲突隔离落库；同冲突重投返回原 id（unique 兜底）。
func InsertConflictTx(ctx context.Context, tx pgx.Tx, originalIngestID, sourceRef, eventID,
	sourceItemKey string, sourceSequence *string, contentHash []byte, conflictKind string) (int64, error) {
	var id int64
	err := tx.QueryRow(ctx, `INSERT INTO risk_ingest_conflicts
		(original_ingest_id, source_ref, event_id, source_item_key, source_sequence, content_hash, conflict_kind)
		VALUES($1,$2,$3,$4,$5,$6,$7)
		ON CONFLICT (source_ref, event_id, content_hash) DO NOTHING RETURNING id`,
		originalIngestID, sourceRef, eventID, sourceItemKey, sourceSequence, contentHash, conflictKind).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		err = tx.QueryRow(ctx, `SELECT id FROM risk_ingest_conflicts
			WHERE source_ref=$1 AND event_id=$2 AND content_hash=$3`,
			sourceRef, eventID, contentHash).Scan(&id)
	}
	if err != nil {
		return 0, fmt.Errorf("insert risk ingest conflict: %w", err)
	}
	return id, nil
}

func pointerOrNil(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// TxBool 单值布尔查询辅助（SQL 内 $1.. 依次绑定 args）。
func TxBool(ctx context.Context, q IAMDBTX, sql string, args ...any) (bool, error) {
	var b bool
	if err := q.QueryRow(ctx, sql, args...).Scan(&b); err != nil {
		return false, err
	}
	return b, nil
}

// CanManageRiskItem reports current primary-scope authority for a visible item.
func CanManageRiskItem(ctx context.Context, q IAMDBTX, id, actorID int64) (bool, error) {
	return TxBool(ctx, q, `SELECT `+riskManagePredicate("$2")+`
		FROM risk_items ri JOIN organizations go ON go.id = ri.org_id WHERE ri.id=$1`, id, actorID)
}

func MarkRiskDispatchedTx(ctx context.Context, tx pgx.Tx, id, revision int64) (int64, error) {
	var next int64
	err := tx.QueryRow(ctx, `
		UPDATE risk_items SET status = 'remediating', revision = revision + 1, updated_at = now()
		WHERE id = $1 AND revision = $2 AND status IN ('new', 'assessed')
		RETURNING revision
	`, id, revision).Scan(&next)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, errcode.ErrConcurrentModification
	}
	return next, err
}
