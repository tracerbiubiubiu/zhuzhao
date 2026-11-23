package riskremediation

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
)

// ticketCommand acquires the operation identity before the group. Replays check
// current read qualification, but never require an already-consumed flow edge.
func (s *Service) ticketCommand(ctx context.Context, tx pgx.Tx, c Command, command string, input any, run func(context.Context, pgx.Tx, *scope) (Result, error)) (Result, error) {
	if err := s.guard(ctx, tx); err != nil {
		return Result{}, err
	}
	if err := validCommand(c, true); err != nil {
		return Result{}, err
	}
	return s.atomic(ctx, tx, func(ctx context.Context, tx pgx.Tx) (Result, error) {
		if err := s.callbacks.Admit(ctx, tx, Access{Action: command, ActorID: c.ActorID, TicketID: c.TicketID}); err != nil {
			return Result{}, err
		}
		if err := operationLock(ctx, tx, c); err != nil {
			return Result{}, err
		}
		if res, found, err := s.tryReplay(ctx, tx, c, command, input); found || err != nil {
			return res, err
		}
		if err := s.callbacks.Prelock(ctx, tx, Access{Action: command, ActorID: c.ActorID, TicketID: c.TicketID}); err != nil {
			return Result{}, err
		}
		sc, err := lockScope(ctx, tx, c.TicketID, false)
		if err != nil {
			return Result{}, err
		}
		if err = s.authorize(ctx, tx, sc, "receipt", c.ActorID); err != nil {
			return Result{}, err
		}
		if sc.revision != c.ExpectedTicketRevision {
			return Result{}, errcode.ErrConflict
		}
		return run(ctx, tx, sc)
	})
}

func (s *Service) FinishTx(ctx context.Context, tx pgx.Tx, c FinishCommand) (Result, error) {
	if len(c.Items) == 0 || len(c.Items) > 20 {
		return Result{}, errcode.ErrInvalidParams
	}
	seen := map[int64]bool{}
	for _, i := range c.Items {
		if i.LinkID <= 0 || seen[i.LinkID] || i.ExpectedLinkRevision <= 0 || i.ExpectedRiskRevision <= 0 || (i.Resolution != "fixed" && i.Resolution != "accept") || strings.TrimSpace(i.Measures) == "" || len(i.Measures) > 16000 || !validEvidence(i.Evidence, true) {
			return Result{}, errcode.ErrInvalidParams
		}
		seen[i.LinkID] = true
	}
	return s.ticketCommand(ctx, tx, c.Command, "remediation_finish", c, func(ctx context.Context, tx pgx.Tx, sc *scope) (Result, error) {
		policy, err := s.callbacks.TicketPolicy(ctx, tx, TicketCheck{Action: "finish", ActorID: c.ActorID, TicketID: c.TicketID, ExpectedRevision: sc.revision})
		if err != nil {
			return Result{}, err
		}
		// Preflight is read-only: stale items may return conflicts while other
		// items commit, so attachment bindings belong only to effective items.
		for _, i := range c.Items {
			l := sc.links[i.LinkID]
			if l == nil {
				return Result{}, errcode.ErrConflict
			}
			if err = s.callbacks.Authorize(ctx, tx, sc.access("finish", c.ActorID, l.id)); err != nil {
				return Result{}, err
			}
			if err = s.callbacks.Evidence(ctx, tx, EvidenceCheck{Access: sc.access("finish", c.ActorID, l.id), Mode: EvidenceValidate, Evidence: i.Evidence}); err != nil {
				return Result{}, err
			}
		}
		res := Result{Items: make([]ItemResult, 0, len(c.Items))}
		for _, i := range c.Items {
			l := sc.links[i.LinkID]
			r := sc.risks[l.risk]
			if l.revision != i.ExpectedLinkRevision || r.revision != i.ExpectedRiskRevision || l.state == "resolved" || l.state == "pending_verify" || r.owner <= 0 || (r.status != "remediating" && r.status != "pending_verify") || !advance(r.revision) || !advance(l.revision) || l.proposal == math.MaxInt64 {
				res.Items = append(res.Items, itemResult(l, r, "conflict"))
				continue
			}
			pending, err := recoverPendingAcceptance(ctx, tx, c.Command, r, l)
			if err != nil {
				return Result{}, err
			}
			if pending {
				res.Items = append(res.Items, itemResult(l, r, "conflict"))
				continue
			}
			if err = s.callbacks.Evidence(ctx, tx, EvidenceCheck{Access: sc.access("finish", c.ActorID, l.id), Mode: EvidenceBind, Evidence: i.Evidence}); err != nil {
				return Result{}, err
			}
			status, state, result := "pending_verify", "pending_verify", "pending_confirmation"
			var effective *string
			if i.Resolution == "accept" {
				status = "remediating"
				state = "open"
			} else if !policy.ReviewGate {
				status = "closed"
				state = "resolved"
				fixed := "fixed"
				effective = &fixed
				result = "applied"
			}
			if err = setRisk(ctx, tx, r, status); err != nil {
				return Result{}, err
			}
			l.proposal++
			body, _ := json.Marshal(i.Evidence)
			_, err = tx.Exec(ctx, `INSERT INTO risk_remediation_submissions(link_id,risk_id,proposal_version,risk_revision,owner_at_submission,actor_id,resolution,measures,evidence_refs) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, l.id, r.id, l.proposal, r.revision, r.owner, c.ActorID, i.Resolution, i.Measures, body)
			if err != nil {
				return Result{}, storageError(err)
			}
			if err = persistLink(ctx, tx, l, r, state, effective); err != nil {
				return Result{}, err
			}
			item := itemResult(l, r, result)
			res.Items = append(res.Items, item)
			if err = recordFact(ctx, tx, c.Command, r.id, r.revision, l.id, l.resolutionVersion, "remediation_submitted", item); err != nil {
				return Result{}, err
			}
		}
		return s.complete(ctx, tx, sc, c.Command, "remediation_finish", c, res)
	})
}

func (s *Service) ReviewTx(ctx context.Context, tx pgx.Tx, c ReviewCommand) (Result, error) {
	if len(c.Items) == 0 || len(c.Items) > 20 {
		return Result{}, errcode.ErrInvalidParams
	}
	seen := map[int64]bool{}
	for _, i := range c.Items {
		if i.LinkID <= 0 || seen[i.LinkID] || i.ExpectedLinkRevision <= 0 || i.ExpectedRiskRevision <= 0 || i.ExpectedProposalVersion <= 0 || (i.Decision != "approve" && i.Decision != "reject") || strings.TrimSpace(i.Note) == "" || len(i.Note) > 16000 || !validEvidence(i.Evidence, false) {
			return Result{}, errcode.ErrInvalidParams
		}
		seen[i.LinkID] = true
	}
	return s.ticketCommand(ctx, tx, c.Command, "remediation_review", c, func(ctx context.Context, tx pgx.Tx, sc *scope) (Result, error) {
		_, err := s.callbacks.TicketPolicy(ctx, tx, TicketCheck{Action: "review", ActorID: c.ActorID, TicketID: c.TicketID, ExpectedRevision: sc.revision})
		if err != nil {
			return Result{}, err
		}
		for _, i := range c.Items {
			l := sc.links[i.LinkID]
			if l == nil {
				return Result{}, errcode.ErrConflict
			}
			r := sc.risks[l.risk]
			if c.ActorID != r.owner {
				return Result{}, errcode.ErrNoPermission
			}
			if err = s.callbacks.Authorize(ctx, tx, sc.access("review", c.ActorID, l.id)); err != nil {
				return Result{}, err
			}
			if err = s.callbacks.Evidence(ctx, tx, EvidenceCheck{Access: sc.access("review", c.ActorID, l.id), Mode: EvidenceValidate, Evidence: i.Evidence}); err != nil {
				return Result{}, err
			}
		}
		res := Result{Items: make([]ItemResult, 0, len(c.Items))}
		for _, i := range c.Items {
			l := sc.links[i.LinkID]
			r := sc.risks[l.risk]
			awaitingReview := l.state == "pending_verify" && r.status == "pending_verify"
			returnExpired := i.Decision == "reject" && l.state == "pending_acceptance" && r.status == "remediating"
			if l.revision != i.ExpectedLinkRevision || r.revision != i.ExpectedRiskRevision || l.proposal != i.ExpectedProposalVersion ||
				(!awaitingReview && !returnExpired) || !advance(r.revision) || !advance(l.revision) {
				res.Items = append(res.Items, itemResult(l, r, "conflict"))
				continue
			}
			var submission, owner, baseline int64
			var proposed string
			var original []byte
			err = tx.QueryRow(ctx, `SELECT id,owner_at_submission,risk_revision,resolution,evidence_refs FROM risk_remediation_submissions WHERE link_id=$1 AND proposal_version=$2`, l.id, l.proposal).Scan(&submission, &owner, &baseline, &proposed, &original)
			if err != nil {
				return Result{}, storageError(err)
			}
			matchesProposal := baseline == r.revision || (returnExpired && baseline < r.revision)
			if owner != r.owner || !matchesProposal || proposed != "fixed" {
				res.Items = append(res.Items, itemResult(l, r, "conflict"))
				continue
			}
			if returnExpired {
				// An acceptance request advances risk and link revisions. The
				// current owner may return the old proposal to remediation after
				// the pending acceptance expires or becomes superseded. This
				// path cannot approve a proposal against its earlier revision.
				var present bool
				err = tx.QueryRow(ctx, `SELECT EXISTS(
					SELECT 1 FROM risk_acceptances WHERE risk_id=$1 AND link_id=$2 AND status='pending'
				)`, r.id, l.id).Scan(&present)
				if err != nil {
					return Result{}, storageError(err)
				}
				if !present {
					res.Items = append(res.Items, itemResult(l, r, "conflict"))
					continue
				}
				pending, err := recoverPendingAcceptance(ctx, tx, c.Command, r, l)
				if err != nil {
					return Result{}, err
				}
				if pending {
					res.Items = append(res.Items, itemResult(l, r, "conflict"))
					continue
				}
			}
			var refs []EvidenceRef
			if json.Unmarshal(original, &refs) != nil {
				return Result{}, errcode.ErrServiceUnavailable
			}
			if err = s.callbacks.Evidence(ctx, tx, EvidenceCheck{Access: sc.access("review", c.ActorID, l.id), Mode: EvidenceValidate, Historical: true, Evidence: refs}); err != nil {
				return Result{}, err
			}
			if err = s.callbacks.Evidence(ctx, tx, EvidenceCheck{Access: sc.access("review", c.ActorID, l.id), Mode: EvidenceBind, Evidence: i.Evidence}); err != nil {
				return Result{}, err
			}
			state, status, result := "resolved", "closed", "applied"
			fixed := "fixed"
			var effective *string = &fixed
			if i.Decision == "reject" {
				state = "rejected"
				status = "remediating"
				result = "rejected"
				effective = nil
			}
			if err = setRisk(ctx, tx, r, status); err != nil {
				return Result{}, err
			}
			if err = persistLink(ctx, tx, l, r, state, effective); err != nil {
				return Result{}, err
			}
			body, _ := json.Marshal(i.Evidence)
			if body == nil || string(body) == "null" {
				body = []byte("[]")
			}
			_, err = tx.Exec(ctx, `INSERT INTO risk_remediation_reviews(submission_id,decision,actor_id,note,evidence_refs,risk_revision,link_revision) VALUES($1,$2,$3,$4,$5,$6,$7)`, submission, i.Decision, c.ActorID, i.Note, body, r.revision, l.revision)
			if err != nil {
				return Result{}, storageError(err)
			}
			item := itemResult(l, r, result)
			res.Items = append(res.Items, item)
			if err = recordFact(ctx, tx, c.Command, r.id, r.revision, l.id, l.resolutionVersion, "remediation_reviewed", item); err != nil {
				return Result{}, err
			}
		}
		return s.complete(ctx, tx, sc, c.Command, "remediation_review", c, res)
	})
}
func (s *Service) complete(ctx context.Context, tx pgx.Tx, sc *scope, c Command, command string, input any, res Result) (Result, error) {
	changed := false
	for _, i := range res.Items {
		if i.Result != "conflict" {
			changed = true
		}
	}
	if changed {
		var err error
		res, err = s.finishTicket(ctx, tx, sc, c, command, res)
		if err != nil {
			return Result{}, err
		}
	} else {
		res.TicketID = sc.ticket
		res.TicketRevision = sc.revision
		var err error
		res.TerminalAllowed, err = sc.terminal(ctx, tx)
		if err != nil {
			return Result{}, err
		}
	}
	if err := saveReceipt(ctx, tx, c, command, input, res); err != nil {
		return Result{}, err
	}
	return res, nil
}

func (s *Service) ValidateTerminalTx(ctx context.Context, tx pgx.Tx, actorID, ticketID, expectedRevision int64) error {
	if err := s.guard(ctx, tx); err != nil {
		return err
	}
	if actorID <= 0 || ticketID <= 0 || expectedRevision <= 0 {
		return errcode.ErrInvalidParams
	}
	if err := s.callbacks.Admit(ctx, tx, Access{Action: "terminal", ActorID: actorID, TicketID: ticketID}); err != nil {
		return err
	}
	if err := s.callbacks.Prelock(ctx, tx, Access{Action: "terminal", ActorID: actorID, TicketID: ticketID}); err != nil {
		return err
	}
	sc, err := lockScope(ctx, tx, ticketID, true)
	if err != nil {
		return err
	}
	if sc.revision != expectedRevision {
		return errcode.ErrConflict
	}
	if err = s.callbacks.Authorize(ctx, tx, Access{Action: "terminal", ActorID: actorID, TicketID: ticketID}); err != nil {
		return err
	}
	if err = s.authorize(ctx, tx, sc, "terminal", actorID); err != nil {
		return err
	}
	ready, err := sc.terminal(ctx, tx)
	if err != nil {
		return err
	}
	if !ready {
		return errcode.ErrConflict
	}
	return nil
}

func (s *Service) ReceiptTx(ctx context.Context, tx pgx.Tx, actorID int64, operationID string) (Result, error) {
	if err := s.guard(ctx, tx); err != nil {
		return Result{}, err
	}
	c := Command{ActorID: actorID, OperationID: operationID}
	if err := validCommand(c, false); err != nil {
		return Result{}, err
	}
	// Admission precedes any operation/object lookup, including missing IDs.
	if err := s.callbacks.Admit(ctx, tx, Access{Action: "receipt", ActorID: actorID}); err != nil {
		return Result{}, err
	}
	var body []byte
	err := tx.QueryRow(ctx, `SELECT result FROM risk_remediation_receipts WHERE actor_id=$1 AND operation_id=$2`, actorID, operationID).Scan(&body)
	if errors.Is(err, pgx.ErrNoRows) {
		return Result{}, errcode.ErrNotFound
	}
	if err != nil {
		return Result{}, storageError(err)
	}
	var res Result
	if json.Unmarshal(body, &res) != nil {
		return Result{}, errcode.ErrServiceUnavailable
	}
	if err = s.authorizeReceipt(ctx, tx, actorID, res); err != nil {
		return Result{}, err
	}
	return res, nil
}

// Query and replay share the immutable original read set. Current unrelated
// active links are neither authorization inputs nor part of the risk lock set.
func (s *Service) authorizeReceipt(ctx context.Context, tx pgx.Tx, actorID int64, res Result) error {
	if err := s.callbacks.Prelock(ctx, tx, receiptAccess(actorID, res)); err != nil {
		return err
	}
	if res.TicketID > 0 {
		// Preserve group -> original risks -> ticket -> original links order.
		// The current group may no longer exist; original ticket read is required.
		rows, err := tx.Query(ctx, `SELECT id FROM risk_dispatch_groups WHERE ticket_id=$1 FOR UPDATE`, res.TicketID)
		if err != nil {
			return storageError(err)
		}
		if _, err = pgx.CollectRows(rows, pgx.RowTo[int64]); err != nil {
			return storageError(err)
		}
	}
	if err := lockOriginalRisks(ctx, tx, res); err != nil {
		return err
	}
	if res.TicketID > 0 {
		var ticketID int64
		err := tx.QueryRow(ctx, `SELECT id FROM tickets WHERE id=$1 FOR UPDATE`, res.TicketID).Scan(&ticketID)
		if errors.Is(err, pgx.ErrNoRows) {
			return errcode.ErrNotFound
		}
		if err != nil {
			return storageError(err)
		}
		if err = s.callbacks.Authorize(ctx, tx, Access{Action: "receipt", ActorID: actorID, TicketID: ticketID}); err != nil {
			return err
		}
	}
	linkIDs := make([]int64, 0, len(res.Items))
	for _, item := range res.Items {
		if item.LinkID > 0 {
			linkIDs = append(linkIDs, item.LinkID)
		}
	}
	rows, err := tx.Query(ctx, `SELECT id FROM risk_ticket_links WHERE id=ANY($1) ORDER BY id FOR UPDATE`, linkIDs)
	if err != nil {
		return storageError(err)
	}
	if _, err = pgx.CollectRows(rows, pgx.RowTo[int64]); err != nil {
		return storageError(err)
	}
	return s.authorizeReceiptItems(ctx, tx, actorID, res)
}

// Original receipt members remain protected even after they leave active scope.
func (s *Service) authorizeReceiptItems(ctx context.Context, tx pgx.Tx, actorID int64, res Result) error {
	for _, i := range res.Items {
		var r riskState
		err := tx.QueryRow(ctx, `SELECT org_id,COALESCE(owner_user_id,0) FROM risk_items WHERE id=$1`, i.RiskID).Scan(&r.org, &r.owner)
		if errors.Is(err, pgx.ErrNoRows) {
			return errcode.ErrNotFound
		}
		if err != nil {
			return storageError(err)
		}
		if err = s.callbacks.Authorize(ctx, tx, Access{Action: "receipt", ActorID: actorID, TicketID: res.TicketID, RiskID: i.RiskID, OrgID: r.org, OwnerID: r.owner, LinkID: i.LinkID}); err != nil {
			return err
		}
	}
	return nil
}

func receiptRiskIDs(res Result) []int64 {
	ids := make([]int64, 0, len(res.Items))
	for _, i := range res.Items {
		ids = append(ids, i.RiskID)
	}
	return ids
}
func lockOriginalRisks(ctx context.Context, tx pgx.Tx, res Result) error {
	rows, err := tx.Query(ctx, `SELECT id FROM risk_items WHERE id=ANY($1) ORDER BY id FOR UPDATE`, receiptRiskIDs(res))
	if err != nil {
		return storageError(err)
	}
	_, err = pgx.CollectRows(rows, pgx.RowTo[int64])
	return storageError(err)
}

// A replay restores its original identity before current topology validation.
// Original risk locks are included before the ticket, even for detached links.
func (s *Service) tryReplay(ctx context.Context, tx pgx.Tx, c Command, command string, input any) (Result, bool, error) {
	res, found, err := s.replay(ctx, tx, c, command, input)
	if err != nil || !found {
		return res, found, err
	}
	if err = s.authorizeReceipt(ctx, tx, c.ActorID, res); err != nil {
		return Result{}, false, err
	}
	return res, true, nil
}

func receiptAccess(actorID int64, res Result) Access {
	a := Access{Action: "receipt", ActorID: actorID, TicketID: res.TicketID, RiskIDs: receiptRiskIDs(res)}
	if len(a.RiskIDs) == 1 {
		a.RiskID = a.RiskIDs[0]
	}
	return a
}
