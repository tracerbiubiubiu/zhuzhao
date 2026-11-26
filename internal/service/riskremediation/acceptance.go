package riskremediation

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
)

type acceptanceState struct {
	id, risk, applicant, owner, riskRevision, revision int64
	link, linkRevision                                 *int64
	validUntil                                         time.Time
	status                                             string
}

// acceptanceScope locks the same complete ticket scope as finish/review. A
// standalone request may not silently ignore an active link on another ticket.
func acceptanceScope(ctx context.Context, tx pgx.Tx, c Command, riskID int64) (*scope, *riskState, *linkState, error) {
	if c.TicketID > 0 {
		sc, err := lockScope(ctx, tx, c.TicketID, false)
		if err != nil {
			return nil, nil, nil, err
		}
		r := sc.risks[riskID]
		if r == nil {
			return nil, nil, nil, errcode.ErrConflict
		}
		for _, id := range sc.order {
			l := sc.links[id]
			if l.risk == riskID {
				return sc, r, l, nil
			}
		}
		return nil, nil, nil, errcode.ErrConflict
	}
	r := new(riskState)
	err := tx.QueryRow(ctx, `SELECT id,org_id,COALESCE(owner_user_id,0),revision,status FROM risk_items WHERE id=$1 FOR UPDATE`, riskID).Scan(&r.id, &r.org, &r.owner, &r.revision, &r.status)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil, nil, errcode.ErrNotFound
	}
	if err != nil {
		return nil, nil, nil, storageError(err)
	}
	var active bool
	err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM risk_ticket_links WHERE risk_id=$1 AND ended_at IS NULL)`, riskID).Scan(&active)
	if err != nil {
		return nil, nil, nil, storageError(err)
	}
	if active {
		return nil, nil, nil, errcode.ErrConflict
	}
	return nil, r, nil, nil
}
func acceptanceAccess(action string, c Command, r *riskState, l *linkState) Access {
	a := Access{Action: action, ActorID: c.ActorID, TicketID: c.TicketID, RiskID: r.id, OrgID: r.org, OwnerID: r.owner}
	if l != nil {
		a.LinkID = l.id
	}
	return a
}

// recoverPendingAcceptance is called only after the new command's current
// authorization, evidence and CAS checks. Invalidation and the replacement
// effect share its savepoint: failed or stale commands cannot retire a request.
// The replacement advances risk/link revisions; this helper preserves the old
// application and advances only its own revision, history and durable intent.
func recoverPendingAcceptance(ctx context.Context, tx pgx.Tx, c Command, r *riskState, l *linkState) (bool, error) {
	a := acceptanceState{}
	err := tx.QueryRow(ctx, `SELECT id,risk_id,applicant_id,owner_at_request,risk_revision,revision,link_id,link_revision,valid_until,status FROM risk_acceptances WHERE risk_id=$1 AND status='pending' FOR UPDATE`, r.id).Scan(&a.id, &a.risk, &a.applicant, &a.owner, &a.riskRevision, &a.revision, &a.link, &a.linkRevision, &a.validUntil, &a.status)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, storageError(err)
	}
	status := ""
	sameLink := l == nil && a.link == nil
	if l != nil && a.link != nil && a.linkRevision != nil {
		sameLink = l.id == *a.link && l.revision == *a.linkRevision && l.state == "pending_acceptance"
	}
	if r.owner != a.owner || r.revision != a.riskRevision || !sameLink {
		status = "superseded"
	} else {
		var expired bool
		if err = tx.QueryRow(ctx, `SELECT $1::timestamptz<=clock_timestamp()`, a.validUntil).Scan(&expired); err != nil {
			return false, storageError(err)
		}
		if !expired {
			return true, nil
		}
		status = "expired"
	}
	if !advance(a.revision) {
		return false, errcode.ErrConflict
	}
	_, err = tx.Exec(ctx, `UPDATE risk_acceptances SET status=$2,revision=revision+1,updated_at=clock_timestamp() WHERE id=$1`, a.id, status)
	if err != nil {
		return false, storageError(err)
	}
	var linkID, resolutionVersion int64
	if l != nil {
		linkID, resolutionVersion = l.id, l.resolutionVersion
	}
	err = recordFact(ctx, tx, c, r.id, r.revision, linkID, resolutionVersion, "acceptance_"+status, map[string]any{"acceptance_id": a.id, "acceptance_revision": a.revision + 1, "status": status})
	return false, err
}

func (s *Service) RequestAcceptanceTx(ctx context.Context, tx pgx.Tx, c AcceptanceRequest) (Result, error) {
	if err := s.guard(ctx, tx); err != nil {
		return Result{}, err
	}
	if err := validCommand(c.Command, false); err != nil {
		return Result{}, err
	}
	if c.RiskID <= 0 || c.ExpectedRiskRevision <= 0 || strings.TrimSpace(c.Reason) == "" || len(c.Reason) > 16000 || c.ValidUntil.IsZero() || (c.TicketID > 0 && (c.LinkID <= 0 || c.ExpectedLinkRevision <= 0)) || (c.TicketID == 0 && (c.LinkID != 0 || c.ExpectedLinkRevision != 0)) {
		return Result{}, errcode.ErrInvalidParams
	}
	return s.atomic(ctx, tx, func(ctx context.Context, tx pgx.Tx) (Result, error) {
		if err := s.callbacks.Admit(ctx, tx, Access{Action: "accept_request", ActorID: c.ActorID, TicketID: c.TicketID, RiskID: c.RiskID}); err != nil {
			return Result{}, err
		}
		if err := operationLock(ctx, tx, c.Command); err != nil {
			return Result{}, err
		}
		if res, found, err := s.tryReplay(ctx, tx, c.Command, "accept_request", c); found || err != nil {
			return res, err
		}
		if err := s.callbacks.Prelock(ctx, tx, Access{Action: "accept_request", ActorID: c.ActorID, TicketID: c.TicketID, RiskID: c.RiskID}); err != nil {
			return Result{}, err
		}
		sc, r, l, err := acceptanceScope(ctx, tx, c.Command, c.RiskID)
		if err != nil {
			return Result{}, err
		}
		if err = s.callbacks.Authorize(ctx, tx, acceptanceAccess("receipt", c.Command, r, l)); err != nil {
			return Result{}, err
		}
		if c.ActorID != r.owner {
			return Result{}, errcode.ErrNoPermission
		}
		if err = s.callbacks.Authorize(ctx, tx, acceptanceAccess("accept_request", c.Command, r, l)); err != nil {
			return Result{}, err
		}
		if r.revision != c.ExpectedRiskRevision || !advance(r.revision) || (r.status != "assessed" && r.status != "remediating" && r.status != "pending_verify") {
			return Result{}, errcode.ErrConflict
		}
		if l != nil && (l.id != c.LinkID || l.revision != c.ExpectedLinkRevision || sc.revision != c.ExpectedTicketRevision || l.state == "resolved" || !advance(l.revision)) {
			return Result{}, errcode.ErrConflict
		}
		var future bool
		err = tx.QueryRow(ctx, `SELECT $1::timestamptz>clock_timestamp()`, c.ValidUntil).Scan(&future)
		if err != nil {
			return Result{}, storageError(err)
		}
		if !future {
			return Result{}, errcode.ErrInvalidParams
		}
		if sc != nil {
			if _, err = s.callbacks.TicketPolicy(ctx, tx, TicketCheck{Action: "accept_request", ActorID: c.ActorID, TicketID: c.TicketID, ExpectedRevision: sc.revision}); err != nil {
				return Result{}, err
			}
		}
		// Independence precedes every pending write, including recovery: a
		// creator-only scope must be rejected with zero writes instead of opening
		// an application nobody else could ever decide. The decision rechecks
		// identity, current qualification and every version at its own instant.
		if s.callbacks.IndependentApprover == nil {
			return Result{}, errcode.ErrServiceUnavailable
		}
		if err = s.callbacks.IndependentApprover(ctx, tx, acceptanceAccess("accept_independence", c.Command, r, l)); err != nil {
			return Result{}, err
		}
		pending, err := recoverPendingAcceptance(ctx, tx, c.Command, r, l)
		if err != nil {
			return Result{}, err
		}
		if pending {
			return Result{}, errcode.ErrConflict
		}
		status := r.status
		if l != nil {
			status = "remediating"
		}
		if err = setRisk(ctx, tx, r, status); err != nil {
			return Result{}, err
		}
		var linkID, linkRevision *int64
		if l != nil {
			if err = persistLink(ctx, tx, l, r, "pending_acceptance", nil); err != nil {
				return Result{}, err
			}
			linkID = &l.id
			linkRevision = &l.revision
		}
		res := Result{AcceptanceRevision: 1, AcceptanceStatus: "pending"}
		err = tx.QueryRow(ctx, `INSERT INTO risk_acceptances(risk_id,link_id,applicant_id,owner_at_request,risk_revision,link_revision,reason,valid_until) VALUES($1,$2,$3,$3,$4,$5,$6,$7) RETURNING id`, r.id, linkID, c.ActorID, r.revision, linkRevision, c.Reason, c.ValidUntil).Scan(&res.AcceptanceID)
		if err != nil {
			return Result{}, storageError(err)
		}
		item := ItemResult{RiskID: r.id, RiskRevision: r.revision, Result: "pending_confirmation"}
		if l != nil {
			item = itemResult(l, r, "pending_confirmation")
		}
		res.Items = []ItemResult{item}
		if err = recordFact(ctx, tx, c.Command, r.id, r.revision, item.LinkID, item.ResolutionVersion, "acceptance_requested", map[string]any{"acceptance_id": res.AcceptanceID, "item": item}); err != nil {
			return Result{}, err
		}
		if sc != nil {
			return s.complete(ctx, tx, sc, c.Command, "accept_request", c, res)
		}
		if err = saveReceipt(ctx, tx, c.Command, "accept_request", c, res); err != nil {
			return Result{}, err
		}
		return res, nil
	})
}

func (s *Service) DecideAcceptanceTx(ctx context.Context, tx pgx.Tx, c AcceptanceDecision) (Result, error) {
	if err := s.guard(ctx, tx); err != nil {
		return Result{}, err
	}
	if err := validCommand(c.Command, false); err != nil {
		return Result{}, err
	}
	if c.AcceptanceID <= 0 || c.ExpectedAcceptanceRevision <= 0 || c.ExpectedRiskRevision <= 0 || (c.Decision != "approve" && c.Decision != "reject") || strings.TrimSpace(c.Reason) == "" || len(c.Reason) > 16000 {
		return Result{}, errcode.ErrInvalidParams
	}
	return s.atomic(ctx, tx, func(ctx context.Context, tx pgx.Tx) (Result, error) {
		if err := s.callbacks.Admit(ctx, tx, Access{Action: "accept_confirm", ActorID: c.ActorID, TicketID: c.TicketID}); err != nil {
			return Result{}, err
		}
		if err := operationLock(ctx, tx, c.Command); err != nil {
			return Result{}, err
		}
		if res, found, err := s.tryReplay(ctx, tx, c.Command, "accept_confirm", c); found || err != nil {
			return res, err
		}
		var riskID int64
		err := tx.QueryRow(ctx, `
			SELECT a.risk_id FROM risk_acceptances a
			LEFT JOIN risk_ticket_links l ON l.id = a.link_id
			LEFT JOIN risk_dispatch_groups g ON g.id = l.group_id
			WHERE a.id=$1 AND (($2 = 0 AND a.link_id IS NULL) OR g.ticket_id = $2)`, c.AcceptanceID, c.TicketID).Scan(&riskID)
		if errors.Is(err, pgx.ErrNoRows) {
			return Result{}, errcode.ErrNotFound
		}
		if err != nil {
			return Result{}, storageError(err)
		}
		if err := s.callbacks.Prelock(ctx, tx, Access{Action: "accept_confirm", ActorID: c.ActorID, TicketID: c.TicketID, RiskID: riskID}); err != nil {
			return Result{}, err
		}
		sc, r, l, err := acceptanceScope(ctx, tx, c.Command, riskID)
		if err != nil {
			return Result{}, err
		}
		if err = s.callbacks.Authorize(ctx, tx, acceptanceAccess("receipt", c.Command, r, l)); err != nil {
			return Result{}, err
		}
		a := acceptanceState{}
		err = tx.QueryRow(ctx, `SELECT id,risk_id,applicant_id,owner_at_request,risk_revision,revision,link_id,link_revision,valid_until,status FROM risk_acceptances WHERE id=$1 FOR UPDATE`, c.AcceptanceID).Scan(&a.id, &a.risk, &a.applicant, &a.owner, &a.riskRevision, &a.revision, &a.link, &a.linkRevision, &a.validUntil, &a.status)
		if err != nil {
			return Result{}, storageError(err)
		}
		if c.ActorID == a.applicant {
			return Result{}, errcode.ErrNoPermission
		}
		if err = s.callbacks.Authorize(ctx, tx, acceptanceAccess("accept_confirm", c.Command, r, l)); err != nil {
			return Result{}, err
		}
		if a.status != "pending" || a.revision != c.ExpectedAcceptanceRevision || r.revision != c.ExpectedRiskRevision || r.revision != a.riskRevision || r.owner != a.owner || !advance(r.revision) || !advance(a.revision) {
			return Result{}, errcode.ErrConflict
		}
		if (l == nil) != (a.link == nil) || (l != nil && (l.id != *a.link || l.revision != *a.linkRevision || sc.revision != c.ExpectedTicketRevision || l.state != "pending_acceptance" || !advance(l.revision))) {
			return Result{}, errcode.ErrConflict
		}
		var future bool
		err = tx.QueryRow(ctx, `SELECT $1::timestamptz>clock_timestamp()`, a.validUntil).Scan(&future)
		if err != nil {
			return Result{}, storageError(err)
		}
		if !future {
			return Result{}, errcode.ErrConflict
		}
		if sc != nil {
			if _, err = s.callbacks.TicketPolicy(ctx, tx, TicketCheck{Action: "accept_confirm", ActorID: c.ActorID, TicketID: c.TicketID, ExpectedRevision: sc.revision}); err != nil {
				return Result{}, err
			}
		}
		status, state, result, acceptStatus := "accepted", "resolved", "applied", "approved"
		accepted := "accepted"
		var effective *string = &accepted
		if c.Decision == "reject" {
			status = "assessed"
			if l != nil {
				status = "remediating"
			}
			state = "rejected"
			result = "rejected"
			acceptStatus = "rejected"
			effective = nil
		}
		if err = setRisk(ctx, tx, r, status); err != nil {
			return Result{}, err
		}
		_, err = tx.Exec(ctx, `UPDATE risk_items SET accepted_until=CASE WHEN $2::text='accepted' THEN $3::timestamptz ELSE NULL END WHERE id=$1`, r.id, status, a.validUntil)
		if err != nil {
			return Result{}, storageError(err)
		}
		if l != nil {
			if err = persistLink(ctx, tx, l, r, state, effective); err != nil {
				return Result{}, err
			}
		}
		// This final write is the decision's effective instant. Its single clock
		// value both guards validity and supplies decided_at, after callbacks and
		// risk/link writes; crossing the deadline rolls the whole command back.
		err = tx.QueryRow(ctx, `WITH decision AS MATERIALIZED (SELECT clock_timestamp() AS at)
			UPDATE risk_acceptances a SET status=$2,revision=revision+1,decided_by=$3,decided_at=decision.at,decided_reason=$4,updated_at=decision.at
			FROM decision WHERE a.id=$1 AND a.status='pending' AND a.revision=$5 AND a.valid_until>decision.at RETURNING a.id`, a.id, acceptStatus, c.ActorID, c.Reason, a.revision).Scan(&a.id)
		if errors.Is(err, pgx.ErrNoRows) {
			return Result{}, errcode.ErrConflict
		}
		if err != nil {
			return Result{}, storageError(err)
		}
		item := ItemResult{RiskID: r.id, RiskRevision: r.revision, Result: result}
		if l != nil {
			item = itemResult(l, r, result)
		}
		res := Result{AcceptanceID: a.id, AcceptanceRevision: a.revision + 1, AcceptanceStatus: acceptStatus, Items: []ItemResult{item}}
		if err = recordFact(ctx, tx, c.Command, r.id, r.revision, item.LinkID, item.ResolutionVersion, "acceptance_decided", map[string]any{"acceptance_id": a.id, "status": acceptStatus, "item": item}); err != nil {
			return Result{}, err
		}
		if sc != nil {
			return s.complete(ctx, tx, sc, c.Command, "accept_confirm", c, res)
		}
		if err = saveReceipt(ctx, tx, c.Command, "accept_confirm", c, res); err != nil {
			return Result{}, err
		}
		return res, nil
	})
}

// ReworkAcceptanceTx is the controlled continuation for one approved acceptance
// whose validity has lapsed on an active ticket. A currently qualified manager
// returns exactly that item to submittable remediation. The original approval
// record, its evidence trail, and every sibling conclusion are preserved; the
// kernel never reopens a closed dispatch scope. Linked flows only.
func (s *Service) ReworkAcceptanceTx(ctx context.Context, tx pgx.Tx, c AcceptanceRework) (Result, error) {
	if err := s.guard(ctx, tx); err != nil {
		return Result{}, err
	}
	if err := validCommand(c.Command, true); err != nil {
		return Result{}, err
	}
	if c.RiskID <= 0 || c.ExpectedRiskRevision <= 0 || c.LinkID <= 0 || c.ExpectedLinkRevision <= 0 || c.AcceptanceID <= 0 || c.ExpectedAcceptanceRevision <= 0 || strings.TrimSpace(c.Reason) == "" || len(c.Reason) > 16000 {
		return Result{}, errcode.ErrInvalidParams
	}
	return s.atomic(ctx, tx, func(ctx context.Context, tx pgx.Tx) (Result, error) {
		if err := s.callbacks.Admit(ctx, tx, Access{Action: "accept_rework", ActorID: c.ActorID, TicketID: c.TicketID, RiskID: c.RiskID}); err != nil {
			return Result{}, err
		}
		if err := operationLock(ctx, tx, c.Command); err != nil {
			return Result{}, err
		}
		if res, found, err := s.tryReplay(ctx, tx, c.Command, "accept_rework", c); found || err != nil {
			return res, err
		}
		if err := s.callbacks.Prelock(ctx, tx, Access{Action: "accept_rework", ActorID: c.ActorID, TicketID: c.TicketID, RiskID: c.RiskID}); err != nil {
			return Result{}, err
		}
		sc, r, l, err := acceptanceScope(ctx, tx, c.Command, c.RiskID)
		if err != nil {
			return Result{}, err
		}
		if err = s.callbacks.Authorize(ctx, tx, acceptanceAccess("receipt", c.Command, r, l)); err != nil {
			return Result{}, err
		}
		if err = s.callbacks.Authorize(ctx, tx, acceptanceAccess("accept_rework", c.Command, r, l)); err != nil {
			return Result{}, err
		}
		if sc.groupEnded || (sc.ticketStatus != "in_progress" && sc.ticketStatus != "pending_verify") {
			return Result{}, errcode.ErrConflict
		}
		if l == nil || l.id != c.LinkID || r.revision != c.ExpectedRiskRevision ||
			l.revision != c.ExpectedLinkRevision ||
			l.state != "resolved" || l.resolution == nil || *l.resolution != "accepted" ||
			sc.revision != c.ExpectedTicketRevision || r.status != "accepted" ||
			!advance(r.revision) || !advance(l.revision) {
			return Result{}, errcode.ErrConflict
		}
		// Match the original approval to the currently effective expiry. Approval
		// advances the live revisions past the application's stored baselines.
		var acceptanceRevision int64
		err = tx.QueryRow(ctx, `
			SELECT a.revision FROM risk_acceptances a JOIN risk_items r ON r.id=a.risk_id
			WHERE a.id=$1 AND r.id=$2 AND a.link_id=$3 AND a.status='approved'
			  AND r.accepted_until=a.valid_until AND r.accepted_until<=clock_timestamp()
			FOR UPDATE OF a`, c.AcceptanceID, r.id, l.id).Scan(&acceptanceRevision)
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && acceptanceRevision != c.ExpectedAcceptanceRevision) {
			return Result{}, errcode.ErrConflict
		}
		if err != nil {
			return Result{}, storageError(err)
		}
		if _, err = s.callbacks.TicketPolicy(ctx, tx, TicketCheck{Action: "accept_rework", ActorID: c.ActorID, TicketID: c.TicketID, ExpectedRevision: sc.revision}); err != nil {
			return Result{}, err
		}
		if err = setRisk(ctx, tx, r, "remediating"); err != nil {
			return Result{}, err
		}
		_, err = tx.Exec(ctx, `UPDATE risk_items SET accepted_until=NULL WHERE id=$1`, r.id)
		if err != nil {
			return Result{}, storageError(err)
		}
		if err = persistLink(ctx, tx, l, r, "open", nil); err != nil {
			return Result{}, err
		}
		item := itemResult(l, r, "reworked")
		res := Result{AcceptanceID: c.AcceptanceID, AcceptanceRevision: acceptanceRevision, AcceptanceStatus: "reworked", Items: []ItemResult{item}}
		if err = recordFact(ctx, tx, c.Command, r.id, r.revision, item.LinkID, item.ResolutionVersion, "acceptance_reworked", map[string]any{"acceptance_id": c.AcceptanceID, "acceptance_revision": acceptanceRevision, "reason": c.Reason, "item": item}); err != nil {
			return Result{}, err
		}
		return s.complete(ctx, tx, sc, c.Command, "accept_rework", c, res)
	})
}
