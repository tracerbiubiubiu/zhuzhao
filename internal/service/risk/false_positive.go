package risk

import (
	"bytes"
	"context"
	"math"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

const (
	riskFalsePositivePath    = "/api/v1/risk-items/false-positive"
	riskSuppressRemovePath   = "/api/v1/risk-suppressions/remove"
	falsePositiveCommand     = "false_positive"
	removeSuppressionCommand = "remove_suppression"
)

// FalsePositive records an authorized unlinked risk closure and a bounded suppression atomically.
func (s *Service) FalsePositive(ctx context.Context, req model.RiskFalsePositiveRequest, actor int64) (*model.RiskSuppressionActionResult, error) {
	req.Reason = strings.TrimSpace(req.Reason)
	if !validSuppressionInput(req.ID, actor, req.OperationID, req.Reason) || req.SuppressionValidUntil.IsZero() {
		return nil, errcode.ErrInvalidParams
	}
	expected, err := parseExpectedRevision(req.ExpectedRevision, false)
	if err != nil {
		return nil, err
	}
	req.SuppressionValidUntil = req.SuppressionValidUntil.UTC()
	hash := canonicalHash(struct {
		Command string
		model.RiskFalsePositiveRequest
	}{falsePositiveCommand, req})

	var result *model.RiskSuppressionActionResult
	err = s.withHTTPCommand(ctx, riskFalsePositivePath, actor, func(ctx context.Context, tx pgx.Tx) error {
		receipt, replayed, err := repository.ClaimOperationTx(ctx, tx, actor, req.OperationID, hash, falsePositiveCommand)
		if err != nil {
			return err
		}
		if replayed {
			result, err = replaySuppressionAction(ctx, tx, actor, receipt, hash, falsePositiveCommand)
			return err
		}
		row, err := lockManagedSuppressionRisk(ctx, tx, req.ID, actor)
		if err != nil {
			return err
		}
		if row.Revision != expected {
			return errcode.ErrConcurrentModification
		}
		if row.Status != "new" && row.Status != "assessed" {
			return ErrRiskInvalidState
		}
		linked, err := repository.RiskHasActiveLinkTx(ctx, tx, row.ID)
		if err != nil {
			return err
		}
		if linked {
			return ErrRiskInvalidState
		}
		window, err := repository.GetRiskSuppressionWindowTx(ctx, tx, row.FamilyID)
		if err != nil {
			return err
		}
		if window != nil && window.Active {
			return ErrRiskInvalidState
		}
		ref, err := repository.CreateRiskSuppressionTx(ctx, tx, row.FamilyID, row.ID, actor, req.SuppressionValidUntil, req.Reason)
		if err != nil {
			return err
		}
		closed, err := repository.CloseRiskFalsePositiveTx(ctx, tx, row.ID, expected)
		if err != nil {
			return err
		}
		event := jsonBytes(map[string]any{"closure_kind": "false_positive"})
		if err := repository.InsertEventTx(ctx, tx, closed.ID, closed.Revision, "false_positive_closed", 0, actor, req.OperationID, event); err != nil {
			return err
		}
		result, err = completeSuppressionAction(ctx, tx, actor, req.OperationID, "closed", closed, ref)
		return err
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// ListSuppressions pages current and historical suppression records under primary-anchor management scope.
func (s *Service) ListSuppressions(ctx context.Context, actor int64, page, pageSize int) (*model.RiskSuppressionList, error) {
	if actor <= 0 || page < 1 || pageSize < 1 || pageSize > 100 || page-1 > math.MaxInt/pageSize {
		return nil, errcode.ErrInvalidParams
	}
	items, total, err := repository.ListRiskSuppressions(ctx, repository.IAMDB(ctx, s.db), actor, pageSize, (page-1)*pageSize)
	if err != nil {
		return nil, err
	}
	return &model.RiskSuppressionList{Items: items, Total: total, Page: page, PageSize: pageSize}, nil
}

// RemoveSuppression ends an active suppression without changing the historical risk cycle.
func (s *Service) RemoveSuppression(ctx context.Context, req model.RiskSuppressionRemoveRequest, actor int64) (*model.RiskSuppressionActionResult, error) {
	req.Reason = strings.TrimSpace(req.Reason)
	if !validSuppressionInput(req.ID, actor, req.OperationID, req.Reason) {
		return nil, errcode.ErrInvalidParams
	}
	expected, err := parseExpectedRevision(req.ExpectedRevision, false)
	if err != nil {
		return nil, err
	}
	hash := canonicalHash(struct {
		Command string
		model.RiskSuppressionRemoveRequest
	}{removeSuppressionCommand, req})

	var result *model.RiskSuppressionActionResult
	err = s.withHTTPCommand(ctx, riskSuppressRemovePath, actor, func(ctx context.Context, tx pgx.Tx) error {
		receipt, replayed, err := repository.ClaimOperationTx(ctx, tx, actor, req.OperationID, hash, removeSuppressionCommand)
		if err != nil {
			return err
		}
		if replayed {
			result, err = replaySuppressionAction(ctx, tx, actor, receipt, hash, removeSuppressionCommand)
			return err
		}
		riskID, err := repository.GetRiskIDForSuppressionTx(ctx, tx, req.ID)
		if err != nil {
			return err
		}
		row, err := lockManagedSuppressionRisk(ctx, tx, riskID, actor)
		if err != nil {
			return err
		}
		ref, err := repository.EndRiskSuppressionTx(ctx, tx, req.ID, expected, actor, req.Reason)
		if err != nil {
			return err
		}
		event := jsonBytes(map[string]any{"active": false})
		if err := repository.InsertEventTx(ctx, tx, row.ID, row.Revision, "suppression_removed", 0, actor, req.OperationID, event); err != nil {
			return err
		}
		result, err = completeSuppressionAction(ctx, tx, actor, req.OperationID, "removed", row, ref)
		return err
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func validSuppressionInput(id, actor int64, operationID, reason string) bool {
	return id > 0 && actor > 0 && strings.TrimSpace(operationID) != "" && len(operationID) <= 128 && reason != "" && len(reason) <= 2000
}

func lockManagedSuppressionRisk(ctx context.Context, tx pgx.Tx, riskID, actor int64) (*repository.RiskItemRow, error) {
	if err := repository.LockRiskFamilyByRiskTx(ctx, tx, riskID); err != nil {
		return nil, err
	}
	return repository.RiskItemAuthorizedTx(ctx, tx, riskID, actor, true)
}

func completeSuppressionAction(ctx context.Context, tx pgx.Tx, actor int64, op, status string, risk *repository.RiskItemRow, ref model.RiskSuppressionRef) (*model.RiskSuppressionActionResult, error) {
	if err := repository.CompleteOperationTx(ctx, tx, actor, op, status, &risk.ID, &risk.Revision); err != nil {
		return nil, err
	}
	receipt, err := repository.GetOperationTx(ctx, tx, actor, op)
	if err != nil {
		return nil, err
	}
	return suppressionActionResult(risk, ref, receipt, false), nil
}

func replaySuppressionAction(ctx context.Context, tx pgx.Tx, actor int64, receipt *repository.RiskOperationRow, hash []byte, command string) (*model.RiskSuppressionActionResult, error) {
	if receipt.ResultRiskID == nil || receipt.ResultRevision == nil {
		return nil, ErrRiskNotFound
	}
	row, err := repository.RiskItemAuthorizedTx(ctx, tx, *receipt.ResultRiskID, actor, true)
	if err != nil {
		return nil, err
	}
	if receipt.Command != command || !bytes.Equal(receipt.RequestHash, hash) {
		return nil, ErrRiskOperationConflict
	}
	ref, err := repository.GetSuppressionByRiskTx(ctx, tx, row.ID)
	if err != nil {
		return nil, err
	}
	// Ending the suppression advances its own revision. The original close receipt
	// always reports the creation revision.
	if command == falsePositiveCommand {
		ref.Revision = 1
	}
	return suppressionActionResult(row, ref, receipt, true), nil
}

func suppressionActionResult(risk *repository.RiskItemRow, ref model.RiskSuppressionRef, receipt *repository.RiskOperationRow, replayed bool) *model.RiskSuppressionActionResult {
	riskRef := riskRefFromRow(risk)
	riskRef.Revision = *receipt.ResultRevision
	return &model.RiskSuppressionActionResult{
		Risk: riskRef, Status: receipt.ResultStatus, Suppression: ref, Receipt: receiptFromRow(receipt, replayed),
	}
}
