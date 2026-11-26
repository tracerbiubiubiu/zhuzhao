package risk

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

// 合成来源上报（POST /risk-items/report；详设 §2.1/§4 + 生命周期合同 §8.1）。
// 先收件后应用；重复 event 同内容只应用一次；同键/等序异内容冲突隔离；
// 旧序号 stale 保留收据；缺失/范围/映射不可判定 held；来源身份与范围显式绑定。

const (
	maxReportBatch   = 100
	obsKindObserved  = "observed"
	obsKindWithdrawn = "withdrawn"
	obsKindDeleted   = "deleted"
)

// observationFingerprint 观察规范指纹（哈希输入：固定字段序 + 键排序 attributes）。
type observationFingerprint struct {
	SourceRef      string                    `json:"source_ref"`
	SourceItemKey  string                    `json:"source_item_key"`
	EventID        string                    `json:"event_id"`
	SourceSequence string                    `json:"source_sequence"`
	ObservedAt     string                    `json:"observed_at"`
	ValidUntil     string                    `json:"valid_until,omitempty"`
	Kind           string                    `json:"kind"`
	AssetRef       *model.RiskAssetRef       `json:"asset_ref,omitempty"`
	FindingClass   string                    `json:"finding_class,omitempty"`
	FindingKey     string                    `json:"finding_key,omitempty"`
	Port           *int                      `json:"port,omitempty"`
	SourceSeverity *model.RiskSourceSeverity `json:"source_severity,omitempty"`
	ConclusionRef  string                    `json:"conclusion_ref,omitempty"`
	EvidenceRefs   []model.RiskEvidenceRef   `json:"evidence_refs,omitempty"`
	MappingRef     string                    `json:"mapping_ref"`
	Attributes     any                       `json:"attributes,omitempty"`
}

// Report 合成来源上报（单条观察由 handler 归一为批量入口）。
func (s *Service) Report(ctx context.Context, req model.RiskReportRequest, actor int64) (*model.RiskReportResult, error) {
	return s.report(ctx, req, actor, "")
}

// ReportFromSource uses the identity bound to the verified feed credential.
func (s *Service) ReportFromSource(ctx context.Context, req model.RiskReportRequest, sourceRef string) (*model.RiskReportResult, error) {
	if sourceRef == "" {
		return nil, errcode.ErrNoPermission
	}
	return s.report(ctx, req, 0, sourceRef)
}

func (s *Service) report(ctx context.Context, req model.RiskReportRequest, actor int64, verifiedSource string) (*model.RiskReportResult, error) {
	obs := req.Observations
	if len(obs) == 0 {
		return nil, errcode.ErrInvalidParams
	}
	if len(obs) > maxReportBatch {
		return nil, ErrRiskBatchTooLarge
	}
	// 形状校验在任何写入之前：整批 400/413/403 且零接入效果（详设 §2.1）。
	sourceRef := obs[0].SourceRef
	if verifiedSource != "" && sourceRef != verifiedSource {
		return nil, errcode.ErrNoPermission
	}
	for i := range obs {
		o := &obs[i]
		if o.SourceRef == "" || o.SourceRef != sourceRef {
			return nil, errcode.ErrInvalidParams // 单批次限一来源
		}
		if o.SourceItemKey == "" || o.EventID == "" || o.MappingRef == "" || o.ObservedAt.IsZero() {
			return nil, errcode.ErrInvalidParams
		}
		if o.ValidUntil != nil && !o.ValidUntil.After(o.ObservedAt) {
			return nil, errcode.ErrInvalidParams
		}
		switch o.ObservationKind {
		case "", obsKindObserved, obsKindWithdrawn, obsKindDeleted:
		default:
			return nil, errcode.ErrInvalidParams
		}
		if o.SourceSequence != "" {
			if err := validateCanonicalSequence(o.SourceSequence); err != nil {
				return nil, errcode.ErrInvalidParams
			}
		}
	}

	// The existing number allocator uses the pool. Reserve numbers outside the transaction.
	reservedKeys := make([]string, len(obs))
	for i, observation := range obs {
		if observation.ObservationKind == "" || observation.ObservationKind == obsKindObserved {
			key, err := s.humanKeys.Next(ctx)
			if err != nil {
				return nil, fmt.Errorf("reserve risk number: %w", err)
			}
			reservedKeys[i] = key
		}
	}
	var result *model.RiskReportResult
	err := s.withHTTPCommand(ctx, riskReportPath, actor, func(ctx context.Context, tx pgx.Tx) error {
		src, err := repository.GetRiskSource(ctx, tx, sourceRef)
		if err != nil {
			return err
		}
		if !src.Enabled {
			return ErrRiskSourceUnknown
		}
		if src.DefaultOrgID == nil {
			return ErrRiskSourceScope
		}
		if verifiedSource != "" && !containsInt64(src.AllowedOrgIDs, *src.DefaultOrgID) {
			return ErrRiskSourceScope
		}
		if verifiedSource == "" {
			allowed, err := repository.TxBool(ctx, tx, `SELECT `+orgManageOnOrgSQL()+`
				FROM organizations go WHERE go.id=$1`, *src.DefaultOrgID, actor)
			if err != nil {
				return err
			}
			if !allowed {
				return errcode.ErrNoPermission
			}
		}
		// 并发首报确定性（外审阻断③）：同 (fingerprint_scope, source_item_key)
		// 的并发批次先按稳定键升序取事务级 advisory 锁再进应用路径——
		// 首报无镜像行可锁，advisory 锁先于一切行锁串行同键事务，
		// 锁内（重）读到的等序/旧序判定不再依赖先到者的提交时机，也不产生锁环。
		for _, key := range sourceItemLockKeys(src.FingerprintScope, obs) {
			if err := repository.AdvisoryLockSourceItemTx(ctx, tx, key); err != nil {
				return err
			}
		}
		if len(obs) > 1 {
			families := make([]string, len(obs))
			for i := range obs {
				families[i] = canonicalKeyHex(src.FingerprintScope, obs[i].SourceItemKey)
			}
			if err := repository.LockObservedRiskItemsTx(ctx, tx, src.FingerprintScope, src.KeySchemaVersion, families); err != nil {
				return err
			}
		}
		items := make([]model.RiskReportItem, 0, len(obs))
		for i := range obs {
			item, err := s.applyObservation(ctx, tx, src, obs[i], actor, reservedKeys[i])
			if err != nil {
				return err
			}
			items = append(items, item)
		}
		result = &model.RiskReportResult{BatchReceipt: uuid.NewString(), Items: items}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (s *Service) applyObservation(ctx context.Context, tx pgx.Tx, src *repository.RiskSourceRow,
	o model.FindingObservationV1, actor int64, humanKey string) (model.RiskReportItem, error) {
	item := model.RiskReportItem{EventID: o.EventID}
	kind := o.ObservationKind
	if kind == "" {
		kind = obsKindObserved
	}
	fp := observationFingerprint{
		SourceRef: o.SourceRef, SourceItemKey: o.SourceItemKey, EventID: o.EventID,
		SourceSequence: o.SourceSequence, ObservedAt: o.ObservedAt.UTC().Format(time.RFC3339Nano),
		Kind: kind, AssetRef: o.AssetRef, FindingClass: o.FindingClass, FindingKey: o.FindingKey,
		Port: o.Port, SourceSeverity: o.SourceSeverity, ConclusionRef: o.ConclusionRef,
		EvidenceRefs: o.EvidenceRefs, MappingRef: o.MappingRef, Attributes: normalizeJSON(o.Attributes),
	}
	if o.ValidUntil != nil {
		fp.ValidUntil = o.ValidUntil.UTC().Format(time.RFC3339Nano)
	}
	hash := canonicalHash(fp)
	var seq *string
	if o.SourceSequence != "" {
		s := o.SourceSequence
		seq = &s
	}
	// ① 同 event 重放：先查后插——同内容 duplicate（只应用一次），异内容冲突隔离。
	existing, err := repository.GetObservationByEventTx(ctx, tx, o.SourceRef, o.EventID)
	switch {
	case err == nil:
		return replayOrQuarantineEvent(ctx, tx, existing, hash, o, seq, item)
	case errors.Is(err, repository.ErrRiskNotFound):
		// 新事件，继续收件。
	default:
		return item, err
	}
	// ② 等序冲突判据（外审阻断③收口）：同 (source,item,sequence) 台账占用即
	// 异 event 同序输入——stale 也落台账行，故判据对台账而非仅镜像 latest；
	// 锁内读取不依赖先到者提交时机，原观察与已应用镜像不可变，
	// 冲突输入只落隔离记录（唯一索引兜底并发）。
	var occupied *repository.RiskObservationRow
	if seq != nil {
		row, err := repository.GetObservationBySequenceTx(ctx, tx, o.SourceRef, o.SourceItemKey, *seq)
		if err != nil && !errors.Is(err, repository.ErrRiskNotFound) {
			return item, err
		}
		occupied = row
	}
	if occupied != nil {
		return quarantineEqualSequence(ctx, tx, occupied, o, seq, hash, item)
	}
	// ③ 落收件行（stale 也落——保留收据不覆盖镜像）。savepoint 包住插入：
	// 唯一约束兜底（详设 §4）可按现势重走重放/判序，23505 不中止命令事务。
	sp, err := tx.Begin(ctx)
	if err != nil {
		return item, fmt.Errorf("observation savepoint: %w", err)
	}
	row, inserted, err := repository.InsertObservationTx(ctx, sp, newIngestID(), o.SourceRef,
		o.EventID, o.SourceItemKey, seq, o.ObservedAt, hash, o.MappingRef, kind)
	if err != nil {
		_ = sp.Rollback(ctx)
		// 唯一约束兜底：并发同 event 或同 (item,sequence)——按现势重走
		// 重放/判序一次，不覆写已验原文。
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			raced, rErr := repository.GetObservationByEventTx(ctx, tx, o.SourceRef, o.EventID)
			if rErr == nil {
				return replayOrQuarantineEvent(ctx, tx, raced, hash, o, seq, item)
			}
			if seq == nil {
				return item, err
			}
			seqRow, sErr := repository.GetObservationBySequenceTx(ctx, tx, o.SourceRef, o.SourceItemKey, *seq)
			if sErr != nil {
				return item, sErr
			}
			return quarantineEqualSequence(ctx, tx, seqRow, o, seq, hash, item)
		}
		return item, err
	}
	if err := sp.Commit(ctx); err != nil {
		return item, err
	}
	if !inserted {
		// 同 (source,event) 已有台账行：按现势重放/判冲突，绝不二次应用。
		return replayOrQuarantineEvent(ctx, tx, row, hash, o, seq, item)
	}
	item.IngestID = row.IngestID
	// ④ 不可判定 → held（缺失/映射/范围分别可区别；零业务效果）。
	hold := func(reason string) (model.RiskReportItem, error) {
		if err := repository.ApplyObservationTx(ctx, tx, row.IngestID, "held", "held", nil, reason); err != nil {
			return item, err
		}
		item.Result, item.Processing, item.SafeReason = "held", "held", reason
		return item, nil
	}
	if src.Ordered && seq == nil {
		return hold("missing_sequence")
	}
	if o.MappingRef != src.MappingRef {
		return hold("mapping_not_approved")
	}
	if src.DefaultOrgID == nil {
		return hold("org_unassigned")
	}
	if !containsInt64(src.AllowedOrgIDs, *src.DefaultOrgID) {
		return hold(fmt.Sprintf("org_not_authorized:%d", *src.DefaultOrgID))
	}
	// ⑤ 旧序号：stale，保留收据、不覆盖镜像、不建周期。
	latest, recordExists, err := repository.LockSourceRecordTx(ctx, tx, o.SourceRef, o.SourceItemKey)
	if err != nil {
		return item, err
	}
	if recordExists && latest != nil && seq != nil && compareCanonicalSequence(*seq, *latest) < 0 {
		if err := repository.ApplyObservationTx(ctx, tx, row.IngestID, "held", "stale",
			nil, "stale_sequence:"+*latest); err != nil {
			return item, err
		}
		item.Result, item.Processing, item.SafeReason = "stale", "held", "stale_sequence:"+*latest
		return item, nil
	}
	// 应用：family 稳定键 → 周期 → 镜像（§4 锁序：family 稳定键 → risk ID）。
	canonicalKeyHash := canonicalKeyHex(src.FingerprintScope, o.SourceItemKey)
	familyID, err := repository.UpsertFamilyTx(ctx, tx, src.FingerprintScope, canonicalKeyHash, src.KeySchemaVersion)
	if err != nil {
		return item, err
	}
	itemID, hasItem, err := repository.FindItemIDByFamilyTx(ctx, tx, familyID)
	if err != nil {
		return item, err
	}
	if hasItem {
		locked, err := repository.LockRiskItemTx(ctx, tx, itemID)
		if err != nil {
			return item, err
		}
		ordinal, err := repository.NextEventOrdinalTx(ctx, tx, itemID, locked.Revision, "source_observed")
		if err != nil {
			return item, err
		}
		if err := repository.InsertEventTx(ctx, tx, itemID, locked.Revision, "source_observed",
			ordinal, actor, "", jsonBytes(map[string]any{
				"source_ref": o.SourceRef, "event_id": o.EventID, "kind": kind,
			})); err != nil {
			return item, err
		}
	} else if kind == obsKindObserved {
		newRow := &repository.RiskItemRow{
			HumanKey: humanKey, FamilyID: familyID, Status: "new",
			OrgID: *src.DefaultOrgID, Title: deriveObservationTitle(o),
			Description:  nilIfEmpty(o.ConclusionRef),
			FindingKey:   nilIfEmpty(o.FindingKey),
			EvidenceRefs: jsonArrayBytes(o.EvidenceRefs),
			Attributes:   jsonBytes(normalizeJSON(o.Attributes)),
			CreatedBy:    actor,
		}
		if o.AssetRef != nil {
			newRow.AssetProvider = &o.AssetRef.Provider
			newRow.AssetExternalID = &o.AssetRef.ExternalID
		}
		itemID, err = repository.InsertRiskItemTx(ctx, tx, newRow)
		if err != nil {
			return item, err
		}
		if err := repository.InsertEventTx(ctx, tx, itemID, 1, "created", 0, actor, "",
			jsonBytes(map[string]any{
				"origin": "source", "source_ref": o.SourceRef, "event_id": o.EventID, "kind": kind,
			})); err != nil {
			return item, err
		}
		hasItem = true
	}
	mirror := jsonBytes(fp)
	if recordExists {
		err = repository.UpdateSourceRecordTx(ctx, tx, o.SourceRef, o.SourceItemKey,
			seq, o.ObservedAt, hash, o.MappingRef, mirror)
	} else {
		err = repository.InsertSourceRecordTx(ctx, tx, o.SourceRef, o.SourceItemKey,
			seq, o.ObservedAt, hash, o.MappingRef, mirror)
	}
	if err != nil {
		return item, err
	}
	var riskIDPtr *int64
	if hasItem {
		riskIDPtr = &itemID
	}
	if err := repository.ApplyObservationTx(ctx, tx, row.IngestID, "applied", "accepted", riskIDPtr, ""); err != nil {
		return item, err
	}
	item.Result, item.Processing = "accepted", "applied"
	if hasItem {
		ref, err := riskRefByIDTx(ctx, tx, itemID)
		if err != nil {
			return item, err
		}
		item.RiskRef = ref
	}
	return item, nil
}

func canonicalKeyHex(scope, itemKey string) string {
	sum := sha256.Sum256([]byte(scope + "\n" + itemKey))
	return hex.EncodeToString(sum[:])
}

// sourceItemLockKeys 批次的来源项锁键（去重后升序——固定获取顺序防批间交叉等待）。
func sourceItemLockKeys(scope string, obs []model.FindingObservationV1) []int64 {
	seen := make(map[int64]bool, len(obs))
	keys := make([]int64, 0, len(obs))
	for i := range obs {
		k := repository.SourceItemLockKey(scope, obs[i].SourceItemKey)
		if !seen[k] {
			seen[k] = true
			keys = append(keys, k)
		}
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	return keys
}

// replayOrQuarantineEvent 同 (source,event) 的重放判定：
// 同内容 → duplicate（只应用一次，返回原引用）；异内容 → 冲突隔离落库。
func replayOrQuarantineEvent(ctx context.Context, tx pgx.Tx, row *repository.RiskObservationRow,
	hash []byte, o model.FindingObservationV1, seq *string, item model.RiskReportItem) (model.RiskReportItem, error) {
	if bytes.Equal(row.ContentHash, hash) {
		item.IngestID = row.IngestID
		item.Result = "duplicate"
		item.Processing = row.ProcessingStatus
		if row.RiskID != nil {
			ref, err := riskRefByIDTx(ctx, tx, *row.RiskID)
			if err != nil {
				return item, err
			}
			item.RiskRef = ref
		}
		return item, nil
	}
	conflictID, err := repository.InsertConflictTx(ctx, tx, row.IngestID, o.SourceRef,
		o.EventID, o.SourceItemKey, seq, hash, "event_hash_mismatch")
	if err != nil {
		return item, err
	}
	item.IngestID = row.IngestID
	item.Result = "quarantined"
	item.Processing = "held"
	item.SafeReason = fmt.Sprintf("conflict:%d", conflictID)
	return item, nil
}

// quarantineEqualSequence 等序冲突：同 (source,item,sequence) 已被占用的输入
// ——原观察与已应用镜像不可变，冲突输入只落隔离记录（不覆写已验原文）；
// 同冲突重投返回原引用。
func quarantineEqualSequence(ctx context.Context, tx pgx.Tx, original *repository.RiskObservationRow,
	o model.FindingObservationV1, seq *string, hash []byte, item model.RiskReportItem) (model.RiskReportItem, error) {
	conflictID, err := repository.InsertConflictTx(ctx, tx, original.IngestID, o.SourceRef,
		o.EventID, o.SourceItemKey, seq, hash, "sequence_hash_mismatch")
	if err != nil {
		return item, err
	}
	item.IngestID = original.IngestID
	item.Result = "quarantined"
	item.Processing = "held"
	item.SafeReason = fmt.Sprintf("conflict:%d", conflictID)
	return item, nil
}

func containsInt64(list []int64, v int64) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

func deriveObservationTitle(o model.FindingObservationV1) string {
	switch {
	case o.FindingKey != "" && o.AssetRef != nil && o.AssetRef.ExternalID != "":
		return fmt.Sprintf("%s @ %s", o.FindingKey, o.AssetRef.ExternalID)
	case o.FindingKey != "":
		return o.FindingKey
	default:
		return o.SourceItemKey
	}
}

func riskRefByIDTx(ctx context.Context, tx pgx.Tx, id int64) (*model.RiskRef, error) {
	var ref model.RiskRef
	err := tx.QueryRow(ctx, `SELECT id, human_key, revision FROM risk_items WHERE id=$1`, id).
		Scan(&ref.ID, &ref.HumanKey, &ref.Revision)
	if err != nil {
		return nil, err
	}
	return &ref, nil
}
