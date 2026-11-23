package risk

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

// 授权读取：列表/详情/历史/来源收件回执（L2 过滤先行——详设 §2.1/§7）。

var validStatuses = map[string]bool{
	"new": true, "assessed": true, "remediating": true,
	"pending_verify": true, "accepted": true, "closed": true,
}

func normalizePage(page, pageSize int) (int, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		return 0, 0, errcode.ErrInvalidParams
	}
	return page, pageSize, nil
}

// List 授权列表（先授权过滤再筛选/计数；固定字段排序加 ID 稳定次序）。
func (s *Service) List(ctx context.Context, q model.RiskListQuery, actor int64) (*model.RiskListResult, error) {
	if q.Status != "" && !validStatuses[q.Status] {
		return nil, errcode.ErrInvalidParams
	}
	if q.Level != "" && !validLevels[q.Level] {
		return nil, errcode.ErrInvalidParams
	}
	switch q.Sort {
	case "", "created_at", "deadline", "level":
	default:
		return nil, errcode.ErrInvalidParams
	}
	if q.OwnerUserID != nil && *q.OwnerUserID <= 0 {
		return nil, errcode.ErrInvalidParams
	}
	if q.AffectedOrgID != nil && *q.AffectedOrgID <= 0 {
		return nil, errcode.ErrInvalidParams
	}
	page, pageSize, err := normalizePage(q.Page, q.PageSize)
	if err != nil {
		return nil, err
	}
	filter := repository.RiskListFilter{
		Status: q.Status, Level: q.Level, Q: q.Q, Sort: q.Sort,
		Limit: pageSize, Offset: (page - 1) * pageSize,
	}
	if q.OwnerUserID != nil {
		filter.OwnerUserID = *q.OwnerUserID
	}
	if q.AffectedOrgID != nil {
		filter.AffectedOrgID = *q.AffectedOrgID
	}
	items, severities, err := repository.ListRiskItems(ctx, s.db, filter, actor)
	if err != nil {
		return nil, err
	}
	total, err := repository.CountRiskItems(ctx, s.db, filter, actor)
	if err != nil {
		return nil, err
	}
	views := make([]model.RiskView, 0, len(items))
	for i := range items {
		views = append(views, viewFromRow(&items[i], severities[i]))
	}
	return &model.RiskListResult{Items: views, Total: total, Page: page, PageSize: pageSize}, nil
}

// Detail 授权详情（RiskView + 有界扩展；不可见与不存在同 404）。
func (s *Service) Detail(ctx context.Context, id, actor int64) (*model.RiskDetailView, error) {
	if id <= 0 {
		return nil, errcode.ErrInvalidParams
	}
	row, severityJSON, obs, err := repository.GetRiskItemVisible(ctx, s.db, id, actor)
	if err != nil {
		return nil, err
	}
	detail := &model.RiskDetailView{
		View:         viewFromRow(row, severityJSON),
		Description:  derefStr(row.Description),
		SystemRef:    derefStr(row.SystemRef),
		EvidenceRefs: bytesOr(row.EvidenceRefs, []byte("[]")),
		Attributes:   bytesOr(row.Attributes, []byte("{}")),
	}
	if obs != nil {
		summary := map[string]any{
			"source_ref":      obs.SourceRef,
			"source_item_key": obs.SourceItemKey,
			"observed_at":     obs.ObservedAt,
			"ingest_id":       obs.IngestID,
		}
		if obs.SourceSequence != nil {
			summary["source_sequence"] = *obs.SourceSequence
		}
		detail.SourceSummary = jsonBytes(summary)
	} else {
		detail.SourceSummary = jsonBytes(nil)
	}
	return detail, nil
}

// History 风险核实/处置历史（先 L2 后分页）。
func (s *Service) History(ctx context.Context, id int64, page, pageSize int, actor int64) (*model.RiskHistoryResult, error) {
	if id <= 0 {
		return nil, errcode.ErrInvalidParams
	}
	page, pageSize, err := normalizePage(page, pageSize)
	if err != nil {
		return nil, err
	}
	rows, total, err := repository.ListRiskEvents(ctx, s.db, id, actor, pageSize, (page-1)*pageSize)
	if err != nil {
		return nil, err
	}
	items := make([]model.RiskEventView, 0, len(rows))
	for _, r := range rows {
		items = append(items, model.RiskEventView{
			ID: r.ID, RiskID: r.RiskID, AggregateRevision: r.AggregateRevision,
			EventKind: r.EventKind, Ordinal: r.Ordinal, ActorID: r.ActorID,
			OccurredAt: r.OccurredAt, OperationID: r.OperationID, Payload: projectHistoryPayload(ctx, actor, r.EventKind, r.Payload, s.tickets),
		})
	}
	return &model.RiskHistoryResult{Items: items, Total: total, Page: page, PageSize: pageSize}, nil
}

// IngestReceipt 来源收件回执受权查询（丢响应后按原 ingest_id 核查；不返回越权 risk_ref）。
func (s *Service) IngestReceipt(ctx context.Context, ingestID string, actor int64) (*model.RiskIngestReceiptView, error) {
	if ingestID == "" {
		return nil, errcode.ErrInvalidParams
	}
	obs, err := repository.GetIngestReceiptAuthorized(ctx, s.db, ingestID, actor)
	if err != nil {
		return nil, err
	}
	view := &model.RiskIngestReceiptView{
		IngestID:   obs.IngestID,
		SourceRef:  obs.SourceRef,
		EventID:    obs.EventID,
		Result:     obs.ResultKind,
		Processing: obs.ProcessingStatus,
		ReceivedAt: obs.ReceivedAt,
	}
	if obs.SafeReason != nil {
		view.SafeReason = *obs.SafeReason
	}
	if obs.RiskID != nil {
		var ref model.RiskRef
		if err := s.db.QueryRow(ctx,
			`SELECT id, human_key, revision FROM risk_items WHERE id=$1`, *obs.RiskID).
			Scan(&ref.ID, &ref.HumanKey, &ref.Revision); err != nil {
			return nil, fmt.Errorf("receipt risk ref: %w", err)
		}
		view.RiskRef = &ref
	}
	return view, nil
}

func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func bytesOr(b, fallback []byte) json.RawMessage {
	if len(b) == 0 || bytes.Equal(bytes.TrimSpace(b), []byte("null")) {
		return fallback
	}
	return json.RawMessage(b)
}

// OperationResult returns a receipt only while its original actor may read the risk.
func (s *Service) OperationResult(ctx context.Context, operationID string, actorID int64) (*model.RiskReceipt, error) {
	if actorID <= 0 || len(operationID) == 0 || len(operationID) > 128 {
		return nil, ErrRiskNotFound
	}
	row, err := repository.GetOperationTx(ctx, repository.IAMDB(ctx, s.db), actorID, operationID)
	if err != nil {
		return nil, err
	}
	if row.ResultRiskID == nil {
		return nil, ErrRiskNotFound
	}
	if _, err := s.Detail(ctx, *row.ResultRiskID, actorID); err != nil {
		return nil, err
	}
	result := receiptFromRow(row, true)
	return &result, nil
}
