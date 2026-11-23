package risk

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/tracerbiubiubiu/zhuzhao/internal/middleware"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

// WithDispatchTransaction gives the risk and ticket writes the same IAM ordering proof.
func (s *Service) WithDispatchTransaction(ctx context.Context, actorID int64, write func(context.Context, pgx.Tx) error) error {
	const singlePath = "/api/v1/risk-items/dispatch"
	const batchPath = "/api/v1/risk-items/batch-dispatch"
	const planPath = "/api/v1/risk-items/dispatch-plan"
	path := singlePath
	if purpose, ok := middleware.IAMRequestFromContext(ctx); ok {
		switch purpose.Path {
		case batchPath, planPath:
			path = purpose.Path
		}
	}
	return s.withHTTPCommand(ctx, path, actorID, func(ctx context.Context, parent pgx.Tx) error {
		// A caller may handle our error and still commit its outer guard. Keep
		// failed previews, group writes and projections within their own child.
		return repository.WithIAMSavepoint(ctx, s.db, parent, write)
	})
}

func (s *Service) AuthorizeDispatch(ctx context.Context, reader repository.RiskDispatchReader, actorID int64,
	scope model.RiskDispatchScope, ids []int64, _ model.RiskDispatchTarget) error {
	q, ok := reader.(repository.IAMDBTX)
	if !ok {
		return errcode.ErrServiceUnavailable
	}
	if actorID <= 0 || len(ids) == 0 || (scope != model.RiskDispatchBulk && scope != model.RiskDispatchSingle) {
		return errcode.ErrForbidden
	}
	for _, id := range ids {
		item, _, _, err := repository.GetRiskItemVisible(ctx, q, id, actorID)
		if errors.Is(err, ErrRiskNotFound) {
			return errcode.ErrForbidden
		}
		if err != nil {
			return err
		}
		if scope == model.RiskDispatchSingle && len(ids) == 1 && item.OwnerUserID != nil && *item.OwnerUserID == actorID {
			continue
		}
		allowed, err := repository.CanManageRiskItem(ctx, q, id, actorID)
		if err != nil {
			return err
		}
		if !allowed {
			return errcode.ErrForbidden
		}
	}
	return nil
}

func (s *Service) CurrentDispatchFacts(ctx context.Context, reader repository.RiskDispatchReader, actorID int64,
	expected []model.RiskDispatchExpected) ([]model.RiskDispatchFact, error) {
	q, ok := reader.(repository.IAMDBTX)
	if !ok {
		return nil, errcode.ErrServiceUnavailable
	}
	facts := make([]model.RiskDispatchFact, 0, len(expected))
	for _, item := range expected {
		row, _, _, err := repository.GetRiskItemVisible(ctx, q, item.ID, actorID)
		if errors.Is(err, ErrRiskNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		facts = append(facts, dispatchFact(row))
	}
	return facts, nil
}

func (s *Service) LockDispatchFacts(ctx context.Context, tx pgx.Tx, ids []int64) ([]model.RiskDispatchFact, error) {
	ordered := slices.Clone(ids)
	slices.Sort(ordered)
	facts := make([]model.RiskDispatchFact, 0, len(ordered))
	for _, id := range ordered {
		row, err := repository.LockRiskItemTx(ctx, tx, id)
		if err != nil {
			return nil, err
		}
		facts = append(facts, dispatchFact(row))
	}
	return facts, nil
}

func (s *Service) CommitDispatched(ctx context.Context, tx pgx.Tx, items []model.RiskDispatchCommitItem) error {
	for _, item := range items {
		next, err := repository.MarkRiskDispatchedTx(ctx, tx, item.RiskID, item.ExpectedRevision)
		if err != nil {
			return err
		}
		payload := jsonBytes(map[string]any{
			"ticket_id": fmt.Sprint(item.TicketID), "ticket_ref": item.TicketRef, "group_key": item.GroupKey,
		})
		if err := repository.InsertEventTx(ctx, tx, item.RiskID, next, "dispatch", 0, item.ActorID, item.OperationID, payload); err != nil {
			return err
		}
	}
	return nil
}

func dispatchFact(row *repository.RiskItemRow) model.RiskDispatchFact {
	fact := model.RiskDispatchFact{
		ID: row.ID, Revision: row.Revision, HumanKey: row.HumanKey, Status: row.Status,
		ManagementOrgID: row.OrgID, FindingKey: derefStr(row.FindingKey),
		Title: row.Title, Description: derefStr(row.Description), SystemRef: derefStr(row.SystemRef),
	}
	if row.OwnerUserID != nil {
		fact.OwnerID = *row.OwnerUserID
	}
	if row.AffectedOrgID != nil {
		fact.AffectedOrgID = *row.AffectedOrgID
	}
	if row.Deadline != nil {
		fact.Deadline = *row.Deadline
	}
	if row.AssetProvider != nil && row.AssetExternalID != nil {
		fact.AssetKey = fmt.Sprintf("%q:%q", *row.AssetProvider, *row.AssetExternalID)
		fact.AssetRef = &model.RiskAssetRef{Provider: *row.AssetProvider, ExternalID: *row.AssetExternalID}
	}
	switch derefStr(row.Level) {
	case "urgent":
		fact.Priority = 1
	case "high":
		fact.Priority = 2
	case "medium":
		fact.Priority = 3
	case "low":
		fact.Priority = 4
	}
	return fact
}
