package repository

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
)

type B3Execution struct {
	model.TicketTaskRun
	TicketID            int64
	ActorID             int64
	TransitionRoles     []string
	ActionIndex         int
	Payload             json.RawMessage
	ExecutionGeneration int64
	RemoteStarted       bool
}

type B3Store struct{ db *pgxpool.Pool }

func NewB3Store(db *pgxpool.Pool) *B3Store { return &B3Store{db: db} }

func (s *B3Store) Stage(ctx context.Context, tx pgx.Tx, run B3Execution) error {
	var ticketID, revision, actorID *int64
	var definition, node *string
	var actionIndex *int
	if run.TicketID != 0 {
		ticketID, revision, actorID = &run.TicketID, &run.TicketRevision, &run.ActorID
		definition, node, actionIndex = &run.DefinitionHash, &run.Node, &run.ActionIndex
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO command_receipts
		    (operation_id, task_id, action, ticket_id, ticket_revision, definition_hash, node, action_index, actor_id, payload, transition_roles)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,COALESCE($11::text[], ARRAY[]::text[]))
	`, run.OperationID, run.TaskID, run.Action, ticketID, revision, definition, node, actionIndex, actorID, run.Payload, run.TransitionRoles)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, "INSERT INTO business_intent_outbox (operation_id) VALUES ($1)", run.OperationID)
	return err
}

func (s *B3Store) Execution(ctx context.Context, taskID string) (B3Execution, error) {
	var run B3Execution
	err := IAMDB(ctx, s.db).QueryRow(ctx, `
		SELECT operation_id, task_id, action, COALESCE(ticket_id,0), COALESCE(ticket_revision,0),
		       COALESCE(definition_hash,''), COALESCE(node,''), COALESCE(action_index,0), COALESCE(actor_id,0),
		       payload, status, COALESCE(reason,''), result, created_at, completed_at, execution_generation, remote_started, transition_roles
		FROM command_receipts WHERE task_id=$1
	`, taskID).Scan(&run.OperationID, &run.TaskID, &run.Action, &run.TicketID, &run.TicketRevision,
		&run.DefinitionHash, &run.Node, &run.ActionIndex, &run.ActorID,
		&run.Payload, &run.Status, &run.Reason, &run.Result, &run.CreatedAt, &run.CompletedAt, &run.ExecutionGeneration, &run.RemoteStarted, &run.TransitionRoles)
	return run, err
}

// Complete changes only the original pending operation. Replayed delivery may
// recover the winner's persisted result without changing it.
func (s *B3Store) Complete(ctx context.Context, run B3Execution, status, reason string, result json.RawMessage) (bool, error) {
	tag, err := IAMDB(ctx, s.db).Exec(ctx, `
		UPDATE command_receipts
		SET status=$3, reason=NULLIF($4,''), result=$5, completed_at=now(), execution_until=NULL
		WHERE operation_id=$1 AND task_id=$2 AND status='pending' AND execution_generation=$6 AND execution_until > clock_timestamp()
	`, run.OperationID, run.TaskID, status, reason, result, run.ExecutionGeneration)
	return tag.RowsAffected() == 1, err
}

func (s *B3Store) ClaimExecution(ctx context.Context, run *B3Execution) error {
	err := s.db.QueryRow(ctx, `
		UPDATE command_receipts
		SET execution_generation=execution_generation+1, execution_until=now()+interval '20 seconds'
		WHERE task_id=$1 AND status='pending' AND (execution_until IS NULL OR execution_until <= now())
		RETURNING execution_generation, remote_started
	`, run.TaskID).Scan(&run.ExecutionGeneration, &run.RemoteStarted)
	return err
}

func (s *B3Store) ReleaseExecution(ctx context.Context, run B3Execution) error {
	_, err := s.db.Exec(ctx, `
		UPDATE command_receipts SET execution_until=NULL
		WHERE task_id=$1 AND execution_generation=$2 AND status='pending'
	`, run.TaskID, run.ExecutionGeneration)
	return err
}

func (s *B3Store) MarkRemoteStarted(ctx context.Context, run B3Execution) error {
	tag, err := IAMDB(ctx, s.db).Exec(ctx, `
		UPDATE command_receipts SET remote_started=true
		WHERE task_id=$1 AND execution_generation=$2 AND status='pending' AND execution_until > clock_timestamp()
	`, run.TaskID, run.ExecutionGeneration)
	if err == nil && tag.RowsAffected() != 1 {
		return pgx.ErrNoRows
	}
	return err
}

type B3Delivery struct {
	OperationID   string
	TaskID        string
	Action        string
	Attempt       int
	Generation    int64
	Accepted      bool
	RemoteStarted bool
	CreatedAt     time.Time
}

func (s *B3Store) Claim(ctx context.Context) (B3Delivery, error) {
	var delivery B3Delivery
	err := s.db.QueryRow(ctx, `
		WITH due AS (
		    SELECT o.operation_id FROM business_intent_outbox o
		    JOIN command_receipts r USING (operation_id)
		    WHERE r.status='pending'
		      AND o.next_attempt_at <= now() AND (o.lease_until IS NULL OR o.lease_until <= now())
		    ORDER BY o.next_attempt_at, o.operation_id LIMIT 1 FOR UPDATE OF o SKIP LOCKED
		)
		UPDATE business_intent_outbox o
		SET lease_generation=o.lease_generation+1, lease_until=now()+interval '25 seconds', attempts=o.attempts+1
		FROM due JOIN command_receipts r USING (operation_id)
		WHERE o.operation_id=due.operation_id
		RETURNING o.operation_id, r.task_id, r.action, o.attempts, o.lease_generation,
		          o.accepted_at IS NOT NULL, r.remote_started, r.created_at
	`).Scan(&delivery.OperationID, &delivery.TaskID, &delivery.Action, &delivery.Attempt, &delivery.Generation, &delivery.Accepted, &delivery.RemoteStarted, &delivery.CreatedAt)
	return delivery, err
}

func (s *B3Store) DeliveryResult(ctx context.Context, delivery B3Delivery, accepted bool) error {
	delay := min(delivery.Attempt, 6) * 5
	if accepted || delivery.Accepted {
		delay = 30
	}
	_, err := s.db.Exec(ctx, `
		UPDATE business_intent_outbox
		SET accepted_at=CASE WHEN $3 THEN COALESCE(accepted_at,now()) ELSE accepted_at END,
		    lease_until=NULL, next_attempt_at=now()+$4 * interval '1 second'
		WHERE operation_id=$1 AND lease_generation=$2
	`, delivery.OperationID, delivery.Generation, accepted, delay)
	return err
}

func (s *B3Store) TicketRuns(ctx context.Context, ticketID int64) ([]model.TicketTaskRun, error) {
	rows, err := IAMDB(ctx, s.db).Query(ctx, `
		SELECT r.operation_id, r.task_id, r.action, r.ticket_revision, r.definition_hash, r.node,
		       CASE WHEN r.status='pending' AND o.accepted_at IS NOT NULL AND r.reason IS NULL THEN 'queued' ELSE r.status END,
		       COALESCE(r.reason,''),
		       CASE WHEN r.action='activelist.document.update' AND r.status='succeeded' THEN
		           jsonb_build_object('document_revision', r.result->'document_revision', 'noop', r.result->'noop')
		           ELSE r.result END,
		       r.created_at, r.completed_at
		FROM command_receipts r JOIN business_intent_outbox o USING (operation_id)
		WHERE r.ticket_id=$1
		ORDER BY r.ticket_revision DESC, r.action_index LIMIT 50
	`, ticketID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (model.TicketTaskRun, error) {
		var run model.TicketTaskRun
		err := row.Scan(&run.OperationID, &run.TaskID, &run.Action, &run.TicketRevision, &run.DefinitionHash,
			&run.Node, &run.Status, &run.Reason, &run.Result, &run.CreatedAt, &run.CompletedAt)
		return run, err
	})
}

func (s *B3Store) RecordPendingReason(ctx context.Context, run B3Execution, reason string) error {
	_, err := s.db.Exec(ctx, `
		UPDATE command_receipts SET reason=$3
		WHERE task_id=$1 AND execution_generation=$2 AND status='pending'
	`, run.TaskID, run.ExecutionGeneration, reason)
	return err
}

// HasHistory keeps the protected read surface in place while dispatch is paused.
func (s *B3Store) HasHistory(ctx context.Context) (bool, error) {
	var installed bool
	if err := s.db.QueryRow(ctx, "SELECT to_regclass('command_receipts') IS NOT NULL").Scan(&installed); err != nil || !installed {
		return false, err
	}
	var present bool
	err := s.db.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM command_receipts)").Scan(&present)
	return present, err
}
