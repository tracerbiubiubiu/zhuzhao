package riskworkflow

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service/riskremediation"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service/ticket"
)

// ReviewNotice identifies the original remediation submission and its reviewer.
type ReviewNotice struct {
	TicketID, TicketRevision, RecipientID int64
	OperationID                           string
}

// ReviewNoticeRecorder joins a review notice to the remediation transaction.
type ReviewNoticeRecorder func(context.Context, pgx.Tx, ReviewNotice) error

// SetReviewNoticeRecorder wires durable notification recording before serving requests.
func (s *Service) SetReviewNoticeRecorder(record ReviewNoticeRecorder) *Service {
	s.reviewNoticeRecorder = record
	return s
}

func (s *Service) recordReviewNotices(ctx context.Context, tx pgx.Tx, operation string, revision int64, effect riskremediation.TicketEffect) error {
	links := make([]int64, 0, len(effect.Items))
	for _, item := range effect.Items {
		if item.Result == "pending_confirmation" {
			links = append(links, item.LinkID)
		}
	}
	if len(links) == 0 {
		return nil
	}
	rows, err := tx.Query(ctx, `
		SELECT DISTINCT r.owner_user_id
		FROM tickets t
		JOIN risk_dispatch_groups g ON g.ticket_id=t.id AND g.ended_at IS NULL
		JOIN risk_ticket_links l ON l.group_id=g.id AND l.ended_at IS NULL
		JOIN risk_items r ON r.id=l.risk_id
		WHERE t.id=$1 AND t.status='pending_verify' AND t.revision=$2
		  AND l.id=ANY($3) AND l.remediation_state='pending_verify'
		  AND r.status='pending_verify' AND r.owner_user_id>0
		ORDER BY r.owner_user_id`, effect.TicketID, revision, links)
	if err != nil {
		return errcode.ErrServiceUnavailable
	}
	recipients, err := pgx.CollectRows(rows, pgx.RowTo[int64])
	if err != nil {
		return errcode.ErrServiceUnavailable
	}
	for _, recipient := range recipients {
		notice := ReviewNotice{TicketID: effect.TicketID, TicketRevision: revision, RecipientID: recipient, OperationID: operation}
		if err := s.reviewNoticeRecorder(ctx, tx, notice); err != nil {
			return errcode.ErrServiceUnavailable
		}
	}
	return nil
}

// CheckReviewNotice revalidates a previously recorded reviewer notice inside the
// caller's IAM transaction. Only an outstanding item of the original receipt
// qualifies. Unrelated ticket revisions and partial reviews can leave it valid.
func CheckReviewNotice(ctx context.Context, tx pgx.Tx, ticketService *ticket.Service, modelText string, notice ReviewNotice) (bool, string, error) {
	if !notice.valid() {
		return false, "", errcode.ErrInvalidParams
	}
	if err := ticketService.CheckRiskReviewNoticeAuthorizationTx(ctx, tx, notice.RecipientID, notice.TicketID, modelText); err != nil {
		if errors.Is(err, errcode.ErrTicketInvalidTransition) {
			return false, "review_no_longer_pending", nil
		}
		if reviewNoticeReadDenied(err) {
			return false, "recipient_unauthorized", nil
		}
		return false, "", err
	}
	pending, err := repository.TxBool(ctx, tx, `
		SELECT EXISTS (
			SELECT 1
			FROM risk_remediation_receipts receipt
			JOIN risk_operations op USING (actor_id, operation_id)
			JOIN tickets t ON t.id=receipt.ticket_id AND t.status='pending_verify'
			JOIN risk_dispatch_groups g ON g.ticket_id=t.id AND g.ended_at IS NULL
			JOIN risk_ticket_links l ON l.group_id=g.id AND l.ended_at IS NULL
			JOIN risk_items r ON r.id=l.risk_id AND r.owner_user_id=$4 AND r.status='pending_verify'
			WHERE receipt.ticket_id=$1 AND receipt.operation_id=$2 AND receipt.result->>'TicketRevision'=$3
			  AND op.command='remediation_finish' AND op.result_status='completed'
			  AND l.remediation_state='pending_verify'
			  AND receipt.result->'Items' @> jsonb_build_array(jsonb_build_object(
			    'LinkID',l.id,'LinkRevision',l.revision,'RiskID',r.id,'RiskRevision',r.revision,
			    'ProposalVersion',l.proposal_version,'Result','pending_confirmation'))
		)`, notice.TicketID, notice.OperationID, strconv.FormatInt(notice.TicketRevision, 10), notice.RecipientID)
	if err != nil {
		return false, "", fmt.Errorf("check pending review notice: %w", err)
	}
	if !pending {
		return false, "review_no_longer_pending", nil
	}
	visible, err := repository.CanReadActiveTicketRisks(ctx, tx, notice.TicketID, notice.RecipientID)
	if err != nil {
		return false, "", fmt.Errorf("check review notice risk scope: %w", err)
	}
	if !visible {
		return false, "recipient_unauthorized", nil
	}
	return true, "", nil
}

// CheckReviewNoticeReceipt checks whether the stored recipient can still see
// the original ticket and risks. It only authorizes historical status metadata.
func CheckReviewNoticeReceipt(ctx context.Context, tx pgx.Tx, ticketService *ticket.Service, modelText string, notice ReviewNotice) (bool, string, error) {
	if !notice.valid() {
		return false, "", errcode.ErrInvalidParams
	}
	if err := ticketService.CheckRiskReviewNoticeReceiptAuthorizationTx(ctx, tx, notice.RecipientID, notice.TicketID, modelText); err != nil {
		if reviewNoticeReadDenied(err) {
			return false, "recipient_unauthorized", nil
		}
		return false, "", err
	}
	rows, err := tx.Query(ctx, `
		SELECT DISTINCT l.risk_id
		FROM risk_remediation_receipts receipt
		JOIN risk_operations op USING (actor_id, operation_id)
		JOIN risk_dispatch_groups g ON g.ticket_id=receipt.ticket_id
		JOIN risk_ticket_links l ON l.group_id=g.id
		WHERE receipt.ticket_id=$1 AND receipt.operation_id=$2 AND receipt.result->>'TicketRevision'=$3
		  AND op.command='remediation_finish' AND op.result_status='completed'
		  AND receipt.result->'Items' @> jsonb_build_array(jsonb_build_object(
		    'LinkID',l.id,'RiskID',l.risk_id,'Result','pending_confirmation'))`, notice.TicketID, notice.OperationID, strconv.FormatInt(notice.TicketRevision, 10))
	if err != nil {
		return false, "", fmt.Errorf("read review receipt scope: %w", err)
	}
	risks, err := pgx.CollectRows(rows, pgx.RowTo[int64])
	if err != nil {
		return false, "", fmt.Errorf("read review receipt risks: %w", err)
	}
	if len(risks) == 0 {
		return false, "recipient_unauthorized", nil
	}
	for _, risk := range risks {
		_, _, _, err := repository.GetRiskItemVisible(ctx, tx, risk, notice.RecipientID)
		if errors.Is(err, repository.ErrRiskNotFound) {
			return false, "recipient_unauthorized", nil
		}
		if err != nil {
			return false, "", fmt.Errorf("read review receipt risk scope: %w", err)
		}
	}
	return true, "", nil
}

func (n ReviewNotice) valid() bool {
	return n.TicketID > 0 && n.TicketRevision > 0 && n.RecipientID > 0 && n.OperationID != ""
}

func reviewNoticeReadDenied(err error) bool {
	return errors.Is(err, errcode.ErrNoPermission) || errors.Is(err, errcode.ErrNoRoles) || errors.Is(err, errcode.ErrTicketNotFound)
}
