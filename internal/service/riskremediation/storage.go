package riskremediation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

type riskState struct {
	id, org, owner, revision int64
	status                   string
}
type linkState struct {
	id, risk, revision, proposal, resolutionVersion int64
	state                                           string
	resolution                                      *string
	resolvedRevision, effectiveOwner                *int64
}
type scope struct {
	group, ticket, revision int64
	closedHistory           bool
	risks                   map[int64]*riskState
	links                   map[int64]*linkState
	order                   []int64
}

func (s *Service) guard(ctx context.Context, tx pgx.Tx) error {
	if err := s.guardAccess(ctx, tx); err != nil {
		return err
	}
	if s.callbacks.TicketPolicy == nil || s.callbacks.Evidence == nil || s.callbacks.ApplyTicket == nil {
		return errcode.ErrServiceUnavailable
	}
	return nil
}

func (s *Service) guardAccess(ctx context.Context, tx pgx.Tx) error {
	if s == nil || s.pool == nil || tx == nil || !repository.IAMGuardActive(ctx) || repository.IAMDB(ctx, s.pool) != tx {
		return errcode.ErrServiceUnavailable
	}
	if s.callbacks.Admit == nil || s.callbacks.Prelock == nil || s.callbacks.Authorize == nil {
		return errcode.ErrServiceUnavailable
	}
	return nil
}
func validCommand(c Command, ticket bool) error {
	if c.ActorID <= 0 || strings.TrimSpace(c.OperationID) == "" || len(c.OperationID) > 128 || (ticket && (c.TicketID <= 0 || c.ExpectedTicketRevision <= 0)) || (!ticket && (c.TicketID < 0 || (c.TicketID == 0) != (c.ExpectedTicketRevision == 0))) {
		return errcode.ErrInvalidParams
	}
	if c.TicketBinding != "" {
		raw, err := hex.DecodeString(c.TicketBinding)
		if err != nil || len(raw) != sha256.Size || hex.EncodeToString(raw) != c.TicketBinding {
			return errcode.ErrInvalidParams
		}
	}
	return nil
}
func validEvidence(refs []EvidenceRef, required bool) bool {
	if len(refs) > 50 || (required && len(refs) == 0) {
		return false
	}
	for _, r := range refs {
		if (r.Kind != "reference" && r.Kind != "file") || strings.TrimSpace(r.Ref) == "" || len(r.Ref) > 1000 || strings.TrimSpace(r.Version) == "" || len(r.Version) > 200 {
			return false
		}
	}
	return true
}
func advance(v int64) bool { return v > 0 && v < math.MaxInt64 }
func storageError(err error) error {
	if err == nil {
		return nil
	}
	return errcode.ErrServiceUnavailable
}

// lockScope stabilizes membership with the dispatch group before enumerating
// risk IDs. All bridge writers must lock that same group before topology edits.
// Organizations and pinned definitions have already been locked by Prelock.
func lockScope(ctx context.Context, tx pgx.Tx, ticketID int64, includeClosedHistory bool) (*scope, error) {
	var group int64
	var ended bool
	err := tx.QueryRow(ctx, `SELECT id, ended_at IS NOT NULL FROM risk_dispatch_groups WHERE ticket_id=$1 FOR UPDATE`, ticketID).Scan(&group, &ended)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, errcode.ErrNotFound
	}
	if err != nil {
		return nil, storageError(err)
	}
	closedHistory := ended && includeClosedHistory
	rows, err := tx.Query(ctx, `SELECT id,risk_id FROM risk_ticket_links WHERE group_id=$1 AND (ended_at IS NULL OR ($2 AND end_reason='ticket_closed')) ORDER BY id`, group, closedHistory)
	if err != nil {
		return nil, storageError(err)
	}
	type pair struct{ link, risk int64 }
	pairs, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (pair, error) { var p pair; err := r.Scan(&p.link, &p.risk); return p, err })
	if err != nil {
		return nil, storageError(err)
	}
	riskIDs := make([]int64, 0, len(pairs))
	for _, p := range pairs {
		riskIDs = append(riskIDs, p.risk)
	}
	sc := &scope{group: group, ticket: ticketID, closedHistory: closedHistory, risks: map[int64]*riskState{}, links: map[int64]*linkState{}}
	rows, err = tx.Query(ctx, `SELECT id,org_id,COALESCE(owner_user_id,0),revision,status FROM risk_items WHERE id=ANY($1) ORDER BY id FOR UPDATE`, riskIDs)
	if err != nil {
		return nil, storageError(err)
	}
	risks, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (*riskState, error) {
		r := new(riskState)
		err := row.Scan(&r.id, &r.org, &r.owner, &r.revision, &r.status)
		return r, err
	})
	if err != nil {
		return nil, storageError(err)
	}
	for _, r := range risks {
		sc.risks[r.id] = r
	}
	err = tx.QueryRow(ctx, `SELECT revision FROM tickets WHERE id=$1 FOR UPDATE`, ticketID).Scan(&sc.revision)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, errcode.ErrNotFound
	}
	if err != nil {
		return nil, storageError(err)
	}
	rows, err = tx.Query(ctx, `SELECT id,risk_id,revision,proposal_version,resolution_version,remediation_state,resolution,resolved_risk_revision,effective_owner_id FROM risk_ticket_links WHERE group_id=$1 AND (ended_at IS NULL OR ($2 AND end_reason='ticket_closed')) ORDER BY id FOR UPDATE`, group, closedHistory)
	if err != nil {
		return nil, storageError(err)
	}
	links, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (*linkState, error) {
		l := new(linkState)
		err := row.Scan(&l.id, &l.risk, &l.revision, &l.proposal, &l.resolutionVersion, &l.state, &l.resolution, &l.resolvedRevision, &l.effectiveOwner)
		return l, err
	})
	if err != nil {
		return nil, storageError(err)
	}
	for _, l := range links {
		sc.links[l.id] = l
		sc.order = append(sc.order, l.id)
	}
	return sc, nil
}
func (s *Service) authorize(ctx context.Context, tx pgx.Tx, sc *scope, action string, actor int64) error {
	for _, id := range sc.order {
		l := sc.links[id]
		r := sc.risks[l.risk]
		if r == nil {
			return errcode.ErrConflict
		}
		if err := s.callbacks.Authorize(ctx, tx, Access{Action: action, ActorID: actor, TicketID: sc.ticket, RiskID: r.id, OrgID: r.org, OwnerID: r.owner, LinkID: l.id}); err != nil {
			return err
		}
	}
	return nil
}
func (sc *scope) access(action string, actor, link int64) Access {
	l := sc.links[link]
	r := sc.risks[l.risk]
	return Access{Action: action, ActorID: actor, TicketID: sc.ticket, RiskID: r.id, OrgID: r.org, OwnerID: r.owner, LinkID: l.id}
}
func (sc *scope) terminal(ctx context.Context, tx pgx.Tx) (bool, error) {
	if len(sc.order) == 0 {
		var detached bool
		err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM risk_ticket_links WHERE group_id=$1)
			AND NOT EXISTS(SELECT 1 FROM risk_ticket_links WHERE group_id=$1 AND
				(ended_at IS NULL OR btrim(COALESCE(end_reason,''))='' OR end_reason='ticket_closed'))`, sc.group).Scan(&detached)
		return detached, storageError(err)
	}
	for _, id := range sc.order {
		l := sc.links[id]
		r := sc.risks[l.risk]
		if l.state != "resolved" || l.resolvedRevision == nil || *l.resolvedRevision != r.revision || l.effectiveOwner == nil || *l.effectiveOwner != r.owner || l.resolution == nil {
			return false, nil
		}
		if (*l.resolution == "fixed" && r.status != "closed") || (*l.resolution == "accepted" && r.status != "accepted") {
			return false, nil
		}
	}
	var blocked bool
	err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM risk_acceptances a JOIN risk_ticket_links l ON l.risk_id=a.risk_id
		WHERE l.group_id=$1 AND (l.ended_at IS NULL OR ($2 AND l.end_reason='ticket_closed')) AND a.status='pending')`, sc.group, sc.closedHistory).Scan(&blocked)
	if err != nil {
		return false, storageError(err)
	}
	var expired bool
	err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM risk_items r JOIN risk_ticket_links l ON l.risk_id=r.id
		WHERE l.group_id=$1 AND (l.ended_at IS NULL OR ($2 AND l.end_reason='ticket_closed'))
			AND l.resolution='accepted' AND (r.accepted_until IS NULL OR r.accepted_until<=clock_timestamp()))`, sc.group, sc.closedHistory).Scan(&expired)
	return !blocked && !expired, storageError(err)
}
func (s *Service) replay(ctx context.Context, tx pgx.Tx, c Command, command string, input any) (Result, bool, error) {
	raw, err := json.Marshal(input)
	if err != nil {
		return Result{}, false, errcode.ErrInvalidParams
	}
	hash := sha256.Sum256(raw)
	// Stable operation lock also handles retries that mention another ticket;
	// it precedes group locks in command entry, see operationLock below.
	var oldHash []byte
	var oldCommand string
	err = tx.QueryRow(ctx, `SELECT request_hash,command FROM risk_operations WHERE actor_id=$1 AND operation_id=$2`, c.ActorID, c.OperationID).Scan(&oldHash, &oldCommand)
	if errors.Is(err, pgx.ErrNoRows) {
		return Result{}, false, nil
	}
	if err != nil {
		return Result{}, false, storageError(err)
	}
	if string(oldHash) != string(hash[:]) || oldCommand != command {
		return Result{}, false, errcode.ErrConflict
	}
	var result Result
	var body []byte
	err = tx.QueryRow(ctx, `SELECT result FROM risk_remediation_receipts WHERE actor_id=$1 AND operation_id=$2`, c.ActorID, c.OperationID).Scan(&body)
	if err != nil {
		return result, false, errcode.ErrConflict
	}
	if json.Unmarshal(body, &result) != nil {
		return result, false, errcode.ErrServiceUnavailable
	}
	return result, true, nil
}
func operationLock(ctx context.Context, tx pgx.Tx, c Command) error {
	_, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 920092))`, jsonKey(c.ActorID, c.OperationID))
	return storageError(err)
}
func jsonKey(actor int64, op string) string { b, _ := json.Marshal([]any{actor, op}); return string(b) }
func saveReceipt(ctx context.Context, tx pgx.Tx, c Command, command string, input any, result Result) error {
	raw, _ := json.Marshal(input)
	hash := sha256.Sum256(raw)
	body, _ := json.Marshal(result)
	_, err := tx.Exec(ctx, `INSERT INTO risk_operations(actor_id,operation_id,request_hash,command,result_status) VALUES($1,$2,$3,$4,'completed')`, c.ActorID, c.OperationID, hash[:], command)
	if err != nil {
		return storageError(err)
	}
	_, err = tx.Exec(ctx, `INSERT INTO risk_remediation_receipts(actor_id,operation_id,ticket_id,result) VALUES($1,$2,NULLIF($3,0),$4)`, c.ActorID, c.OperationID, c.TicketID, body)
	return storageError(err)
}
func recordFact(ctx context.Context, tx pgx.Tx, c Command, riskID, revision, linkID, resolutionVersion int64, kind string, payload any) error {
	return recordFactPayloads(ctx, tx, c, riskID, revision, linkID, resolutionVersion, kind, payload, payload)
}

func recordFactPayloads(ctx context.Context, tx pgx.Tx, c Command, riskID, revision, linkID, resolutionVersion int64, kind string, event, intent any) error {
	eventBody, _ := json.Marshal(event)
	_, err := tx.Exec(ctx, `INSERT INTO risk_events(risk_id,aggregate_revision,event_kind,ordinal,actor_id,operation_id,payload) VALUES($1,$2,$3,0,$4,$5,$6)`, riskID, revision, kind, c.ActorID, c.OperationID, eventBody)
	if err != nil {
		return storageError(err)
	}
	intentBody, _ := json.Marshal(intent)
	_, err = tx.Exec(ctx, `INSERT INTO risk_remediation_intents(risk_id,link_id,ticket_id,aggregate_revision,resolution_version,event_kind,operation_id,actor_id,payload) VALUES($1,NULLIF($2,0),NULLIF($3,0),$4,$5,$6,$7,$8,$9)`, riskID, linkID, c.TicketID, revision, resolutionVersion, kind, c.OperationID, c.ActorID, intentBody)
	return storageError(err)
}

// atomic protects the caller even if it handles an error and then commits its
// transaction: a failed callback/history/receipt cannot leave half a command.
func (s *Service) atomic(ctx context.Context, tx pgx.Tx, fn func(context.Context, pgx.Tx) (Result, error)) (Result, error) {
	var result Result
	var callbackErr error
	err := repository.WithIAMSavepoint(ctx, s.pool, tx, func(ctx context.Context, sub pgx.Tx) error {
		result, callbackErr = fn(ctx, sub)
		return callbackErr
	})
	if callbackErr != nil {
		return Result{}, callbackErr
	}
	if err != nil {
		return Result{}, storageError(err)
	}
	return result, nil
}

func (s *Service) finishTicket(ctx context.Context, tx pgx.Tx, sc *scope, c Command, action string, res Result) (Result, error) {
	ready, err := sc.terminal(ctx, tx)
	if err != nil {
		return Result{}, err
	}
	res.TerminalAllowed = ready
	rev, err := s.callbacks.ApplyTicket(ctx, tx, TicketEffect{TicketCheck: TicketCheck{Action: action, ActorID: c.ActorID, TicketID: c.TicketID, ExpectedRevision: sc.revision}, TerminalAllowed: ready, Items: res.Items})
	if err != nil {
		return Result{}, err
	}
	var actual int64
	var status string
	err = tx.QueryRow(ctx, `SELECT revision,status FROM tickets WHERE id=$1`, sc.ticket).Scan(&actual, &status)
	if err != nil {
		return Result{}, storageError(err)
	}
	if actual != rev || rev < sc.revision {
		return Result{}, errcode.ErrConflict
	}
	if status == "closed" {
		stillReady, err := sc.terminal(ctx, tx)
		if err != nil {
			return Result{}, err
		}
		if !ready || !stillReady {
			return Result{}, errcode.ErrConflict
		}
		if err := endClosedDispatchScope(ctx, tx, sc.group, res.Items); err != nil {
			return Result{}, err
		}
	}
	res.TicketID = sc.ticket
	res.TicketRevision = rev
	_, err = tx.Exec(ctx, `UPDATE risk_remediation_intents SET ticket_revision=$1 WHERE actor_id=$2 AND operation_id=$3 AND ticket_id=$4`, rev, c.ActorID, c.OperationID, c.TicketID)
	return res, storageError(err)
}

// endClosedDispatchScope releases active uniqueness only after the owning
// workflow has really closed the ticket. The original group and links remain
// readable as historical facts. All rows are already locked by lockScope.
func endClosedDispatchScope(ctx context.Context, tx pgx.Tx, groupID int64, results []ItemResult) error {
	tag, err := tx.Exec(ctx, `
		UPDATE risk_dispatch_groups SET ended_at = clock_timestamp()
		WHERE id = $1 AND ended_at IS NULL
	`, groupID)
	if err != nil {
		return storageError(err)
	}
	if tag.RowsAffected() != 1 {
		return errcode.ErrConflict
	}
	rows, err := tx.Query(ctx, `
		UPDATE risk_ticket_links l
		SET ended_at = g.ended_at, end_reason = 'ticket_closed', revision = l.revision + 1
		FROM risk_dispatch_groups g
		WHERE l.group_id = g.id AND g.id = $1 AND l.ended_at IS NULL
		RETURNING l.id,l.revision
	`, groupID)
	if err != nil {
		return storageError(err)
	}
	defer rows.Close()
	for rows.Next() {
		var id, revision int64
		if err := rows.Scan(&id, &revision); err != nil {
			return storageError(err)
		}
		for i := range results {
			if results[i].LinkID == id {
				results[i].LinkRevision = revision
				break
			}
		}
	}
	return storageError(rows.Err())
}

func setRisk(ctx context.Context, tx pgx.Tx, r *riskState, status string) error {
	if !advance(r.revision) {
		return errcode.ErrConflict
	}
	_, err := tx.Exec(ctx, `UPDATE risk_items SET revision=revision+1,status=$2::varchar,closure_kind=CASE WHEN $2::varchar='closed' THEN 'fixed' ELSE NULL END,closed_at=CASE WHEN $2::varchar='closed' THEN clock_timestamp() ELSE NULL END,updated_at=clock_timestamp() WHERE id=$1`, r.id, status)
	if err != nil {
		return storageError(err)
	}
	r.revision++
	r.status = status
	return nil
}
func persistLink(ctx context.Context, tx pgx.Tx, l *linkState, r *riskState, state string, resolution *string) error {
	if !advance(l.revision) {
		return errcode.ErrConflict
	}
	if state == "resolved" {
		if l.resolutionVersion == math.MaxInt64 {
			return errcode.ErrConflict
		}
		l.resolutionVersion++
		l.resolvedRevision = &r.revision
		l.effectiveOwner = &r.owner
	} else {
		l.resolvedRevision = nil
		l.effectiveOwner = nil
	}
	l.state = state
	l.resolution = resolution
	l.revision++
	_, err := tx.Exec(ctx, `UPDATE risk_ticket_links SET revision=$2,proposal_version=$3,resolution_version=$4,remediation_state=$5,resolution=$6,resolved_risk_revision=$7,effective_owner_id=$8,effective_at=CASE WHEN $5='resolved' THEN clock_timestamp() ELSE NULL END WHERE id=$1`, l.id, l.revision, l.proposal, l.resolutionVersion, l.state, l.resolution, l.resolvedRevision, l.effectiveOwner)
	return storageError(err)
}
func itemResult(l *linkState, r *riskState, status string) ItemResult {
	return ItemResult{LinkID: l.id, RiskID: r.id, LinkRevision: l.revision, RiskRevision: r.revision, ProposalVersion: l.proposal, ResolutionVersion: l.resolutionVersion, Result: status}
}
func pendingAcceptance(ctx context.Context, tx pgx.Tx, risk int64) (bool, error) {
	var p bool
	err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM risk_acceptances WHERE risk_id=$1 AND status='pending')`, risk).Scan(&p)
	return p, storageError(err)
}
