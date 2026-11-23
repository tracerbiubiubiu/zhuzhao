package riskremediation

import (
	"context"
	"errors"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

const handoffAction = "remediation_handoff"

type HandoffCommand struct {
	Command
	DefinitionVersion int64
	SuccessorUserID   int64
	Reason            string
	Items             []HandoffItem
}

type HandoffItem struct {
	LinkID                  int64
	ExpectedLinkRevision    int64
	ExpectedRiskRevision    int64
	ExpectedProposalVersion int64
}

// HandoffScope contains the locked group and ticket identity for the owning
// adapter's successor check. The adapter returns the new live dispatch key.
type HandoffScope struct {
	OwnerID, ManagementOrgID, TargetOrgID int64
	CreatorID, BoundReviewerID            int64
	FindingKey, TypeCode, ApprovedScope   string
	Status                                string
}

func (s *Service) HandoffTx(ctx context.Context, tx pgx.Tx, c HandoffCommand, check func(context.Context, pgx.Tx, HandoffScope) (string, error)) (Result, error) {
	if err := s.guardAccess(ctx, tx); err != nil {
		return Result{}, err
	}
	if check == nil {
		return Result{}, errcode.ErrServiceUnavailable
	}
	c.Reason = strings.TrimSpace(c.Reason)
	if c.DefinitionVersion <= 0 || c.SuccessorUserID <= 0 || c.TicketBinding != "" || c.Reason == "" || len(c.Reason) > 2000 || strings.ContainsRune(c.Reason, 0) || len(c.Items) == 0 || len(c.Items) > 20 {
		return Result{}, errcode.ErrInvalidParams
	}
	items := make(map[int64]HandoffItem, len(c.Items))
	for _, i := range c.Items {
		if i.LinkID <= 0 || items[i.LinkID].LinkID != 0 || i.ExpectedLinkRevision <= 0 || i.ExpectedRiskRevision <= 0 || i.ExpectedProposalVersion < 0 {
			return Result{}, errcode.ErrInvalidParams
		}
		items[i.LinkID] = i
	}
	c.Items = append([]HandoffItem(nil), c.Items...)
	sort.Slice(c.Items, func(i, j int) bool { return c.Items[i].LinkID < c.Items[j].LinkID })
	return s.withTicketCommand(ctx, tx, c.Command, handoffAction, c, func(ctx context.Context, tx pgx.Tx, sc *scope) (Result, error) {
		var group HandoffScope
		err := tx.QueryRow(ctx, `SELECT g.owner_user_id,g.finding_key,g.management_org_id,g.target_org_id,g.type_code,g.approved_scope,t.status,t.created_by,COALESCE(t.risk_retest_reviewer_id,0)
			FROM risk_dispatch_groups g JOIN tickets t ON t.id=g.ticket_id
			WHERE g.id=$1 AND g.ended_at IS NULL AND t.definition_version=$2 AND g.definition_version=$2`, sc.group, c.DefinitionVersion).
			Scan(&group.OwnerID, &group.FindingKey, &group.ManagementOrgID, &group.TargetOrgID, &group.TypeCode, &group.ApprovedScope, &group.Status, &group.CreatorID, &group.BoundReviewerID)
		if errors.Is(err, pgx.ErrNoRows) {
			return Result{}, errcode.ErrConflict
		}
		if err != nil {
			return Result{}, storageError(err)
		}
		if (group.Status != "pending_verify" && group.Status != "in_progress") || !advance(sc.revision) || len(sc.order) > 20 {
			return Result{}, errcode.ErrConflict
		}
		if err := s.callbacks.Authorize(ctx, tx, Access{Action: handoffAction, ActorID: c.ActorID, TicketID: c.TicketID}); err != nil {
			return Result{}, err
		}
		bindingChanged := group.BoundReviewerID != c.SuccessorUserID
		var changesOwner bool
		submissions := make(map[int64]*int64, len(items))
		for _, id := range sc.order {
			l := sc.links[id]
			if l.state == "resolved" {
				continue
			}
			r, item := sc.risks[l.risk], items[id]
			if err := s.callbacks.Authorize(ctx, tx, sc.access(handoffAction, c.ActorID, id)); err != nil {
				return Result{}, err
			}
			if item.ExpectedLinkRevision != l.revision || item.ExpectedRiskRevision != r.revision || item.ExpectedProposalVersion != l.proposal ||
				!advance(l.revision) || !advance(r.revision) || r.owner <= 0 {
				return Result{}, errcode.ErrConflict
			}
			changesOwner = changesOwner || r.owner != c.SuccessorUserID
			submission, err := handoffSubmission(ctx, tx, group.Status, l, r)
			if err != nil {
				return Result{}, err
			}
			submissions[id] = submission
		}
		if len(submissions) != len(items) || !bindingChanged || (!changesOwner && group.CreatorID == c.SuccessorUserID && group.OwnerID == c.SuccessorUserID) {
			return Result{}, errcode.ErrConflict
		}
		groupKey, err := check(ctx, tx, group)
		if err != nil {
			return Result{}, err
		}
		return s.persistHandoff(ctx, tx, sc, c, group, groupKey, submissions)
	})
}

func handoffSubmission(ctx context.Context, tx pgx.Tx, ticketStatus string, l *linkState, r *riskState) (*int64, error) {
	if ticketStatus == "in_progress" && (l.state == "open" || l.state == "rejected") && r.status == "remediating" {
		return nil, nil
	}
	pendingAcceptance := l.state == "pending_acceptance" && r.status == "remediating"
	if l.proposal <= 0 || (!pendingAcceptance && (l.state != "pending_verify" || r.status != "pending_verify")) {
		return nil, errcode.ErrConflict
	}
	var submission, owner, baseline int64
	err := tx.QueryRow(ctx, `SELECT s.id,s.owner_at_submission,s.risk_revision FROM risk_remediation_submissions s
		WHERE s.link_id=$1 AND s.risk_id=$2 AND s.proposal_version=$3 AND s.resolution='fixed'
		AND NOT EXISTS (SELECT 1 FROM risk_remediation_reviews v WHERE v.submission_id=s.id)`, l.id, r.id, l.proposal).Scan(&submission, &owner, &baseline)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, errcode.ErrConflict
	}
	if err != nil {
		return nil, storageError(err)
	}
	if pendingAcceptance {
		return handoffAcceptanceProposal(ctx, tx, l, r, submission, owner, baseline)
	}
	if owner == r.owner && baseline == r.revision {
		return &submission, nil
	}
	allowed, err := carriedRetestProposal(ctx, tx, l, r, submission)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, errcode.ErrConflict
	}
	return &submission, nil
}

// An acceptance request advances both revisions. Only a continuous chain of
// those requests can bridge from the original, unreviewed fixed proposal (or
// its last handoff) to the current acceptance revisions. Other edits conflict.
func handoffAcceptanceProposal(ctx context.Context, tx pgx.Tx, l *linkState, r *riskState, submission, submitterOwner, baseline int64) (*int64, error) {
	if submitterOwner != r.owner {
		baseline = 0
	}
	var matching bool
	err := tx.QueryRow(ctx, `WITH base AS (
			SELECT GREATEST($6::bigint,COALESCE(MAX(risk_revision),0)) AS revision
			FROM risk_retest_handoff_items WHERE link_id=$1 AND submission_id=$5 AND successor_user_id=$4
			  AND risk_revision<$2 AND link_revision<$3 AND $2-risk_revision=$3-link_revision
		)
		SELECT b.revision>0 AND b.revision<$2 AND $2-b.revision<$3 AND (
			SELECT COUNT(DISTINCT a.risk_revision) FROM risk_acceptances a WHERE a.risk_id=$7 AND a.link_id=$1
			AND a.applicant_id=$4 AND a.risk_revision>b.revision AND a.risk_revision<=$2 AND a.link_revision<=$3
			AND $2-a.risk_revision=$3-a.link_revision AND a.status IN ('pending','superseded','expired')
		)=$2-b.revision FROM base b`, l.id, r.revision, l.revision, r.owner, submission, baseline, r.id).Scan(&matching)
	if err != nil {
		return nil, storageError(err)
	}
	if !matching {
		return nil, errcode.ErrConflict
	}
	return &submission, nil
}

func carriedRetestProposal(ctx context.Context, tx pgx.Tx, l *linkState, r *riskState, submission int64) (bool, error) {
	var allowed bool
	err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM risk_retest_handoff_items
		WHERE link_id=$1 AND link_revision=$2 AND risk_revision=$3 AND successor_user_id=$4 AND submission_id=$5)`,
		l.id, l.revision, r.revision, r.owner, submission).Scan(&allowed)
	return allowed, storageError(err)
}

func (s *Service) persistHandoff(ctx context.Context, tx pgx.Tx, sc *scope, c HandoffCommand, group HandoffScope, groupKey string, submissions map[int64]*int64) (Result, error) {
	_, err := tx.Exec(ctx, `UPDATE risk_dispatch_groups SET owner_at_dispatch=COALESCE(owner_at_dispatch,owner_user_id),
		owner_user_id=$2,retest_group_key=$3 WHERE id=$1`, sc.group, c.SuccessorUserID, groupKey)
	var constraint *pgconn.PgError
	if errors.As(err, &constraint) && constraint.Code == "23505" && constraint.ConstraintName == "risk_dispatch_one_live_personal_group" {
		return Result{}, errcode.ErrConflict
	}
	if err != nil {
		return Result{}, storageError(err)
	}
	res := Result{TicketID: sc.ticket, DefinitionVersion: c.DefinitionVersion, Status: group.Status, SuccessorUserID: c.SuccessorUserID,
		Items: make([]ItemResult, 0, len(sc.order))}
	for _, id := range sc.order {
		l := sc.links[id]
		r := sc.risks[l.risk]
		if l.state == "resolved" {
			res.Items = append(res.Items, itemResult(l, r, "preserved"))
			continue
		}
		previousOwner := r.owner
		retireAcceptance := l.state == "pending_acceptance"
		if retireAcceptance {
			r.status = "pending_verify"
		}
		if err := tx.QueryRow(ctx, `UPDATE risk_items SET owner_user_id=$2,revision=revision+1,status=$4,updated_at=clock_timestamp()
			WHERE id=$1 AND revision=$3 RETURNING revision`, r.id, c.SuccessorUserID, r.revision, r.status).Scan(&r.revision); err != nil {
			return Result{}, storageError(err)
		}
		r.owner = c.SuccessorUserID
		if retireAcceptance {
			if err := persistLink(ctx, tx, l, r, "pending_verify", nil); err != nil {
				return Result{}, err
			}
			pending, err := recoverPendingAcceptance(ctx, tx, c.Command, r, l)
			if err != nil {
				return Result{}, err
			}
			if pending {
				return Result{}, errcode.ErrConflict
			}
		} else if err := tx.QueryRow(ctx, `UPDATE risk_ticket_links SET revision=revision+1 WHERE id=$1 AND revision=$2 RETURNING revision`, l.id, l.revision).Scan(&l.revision); err != nil {
			return Result{}, storageError(err)
		}
		_, err := tx.Exec(ctx, `INSERT INTO risk_retest_handoff_items(link_id,link_revision,risk_revision,submission_id,from_owner_id,from_group_owner_id,successor_user_id,actor_id,operation_id,reason)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, l.id, l.revision, r.revision, submissions[id], previousOwner, group.OwnerID, c.SuccessorUserID, c.ActorID, c.OperationID, c.Reason)
		if err != nil {
			return Result{}, storageError(err)
		}
		facts := map[string]int64{"previous_owner_id": previousOwner, "new_owner_id": c.SuccessorUserID, "proposal_version": l.proposal}
		audit := struct {
			Reason          string `json:"reason"`
			PreviousOwnerID int64  `json:"previous_owner_id"`
			NewOwnerID      int64  `json:"new_owner_id"`
			ProposalVersion int64  `json:"proposal_version"`
		}{c.Reason, previousOwner, c.SuccessorUserID, l.proposal}
		if err := recordFactPayloads(ctx, tx, c.Command, r.id, r.revision, l.id, l.resolutionVersion, "retest_handoff", audit, facts); err != nil {
			return Result{}, err
		}
		res.Items = append(res.Items, itemResult(l, r, "applied"))
	}
	err = tx.QueryRow(ctx, `UPDATE tickets SET risk_retest_reviewer_id=$3,updated_at=clock_timestamp()
		WHERE id=$1 AND revision=$2 RETURNING revision`, sc.ticket, sc.revision, c.SuccessorUserID).Scan(&res.TicketRevision)
	if err != nil {
		return Result{}, storageError(err)
	}
	if res.TicketRevision != sc.revision+1 {
		return Result{}, errcode.ErrConflict
	}
	if err := repository.NewTicketRepo(s.pool).CreateEventTx(ctx, tx, &model.TicketEvent{TicketID: sc.ticket, UserID: c.ActorID, Action: "risk_retest_handoff"}); err != nil {
		return Result{}, storageError(err)
	}
	_, err = tx.Exec(ctx, `UPDATE risk_remediation_intents SET ticket_revision=$1 WHERE actor_id=$2 AND operation_id=$3 AND ticket_id=$4`, res.TicketRevision, c.ActorID, c.OperationID, c.TicketID)
	if err != nil {
		return Result{}, storageError(err)
	}
	if err := saveReceipt(ctx, tx, c.Command, handoffAction, c, res); err != nil {
		return Result{}, err
	}
	return res, nil
}
