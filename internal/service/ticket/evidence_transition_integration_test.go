//go:build integration

package ticket

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

func TestD02MigrationRevisionTracksBusinessChanges(t *testing.T) {
	ctx := context.Background()
	tx, err := testPool.Begin(ctx)
	require.NoError(t, err)
	defer tx.Rollback(ctx)
	schema := "d02_migration_" + uniqueSuffix()
	_, err = tx.Exec(ctx, `CREATE SCHEMA `+schema+`; SET LOCAL search_path TO `+schema+`, public;
CREATE TABLE tickets (LIKE public.tickets INCLUDING DEFAULTS INCLUDING INDEXES);
ALTER TABLE tickets DROP COLUMN IF EXISTS revision;
INSERT INTO tickets (id, type_code, title, status, created_by, org_id, org_path)
VALUES (1, 'synthetic', 'old ticket', 'open', 1, 1, 'root');`)
	require.NoError(t, err)
	up, err := os.ReadFile("../../../migrations/000087_ticket_evidence_transition.up.sql")
	require.NoError(t, err)
	_, err = tx.Exec(ctx, string(up))
	require.NoError(t, err)
	checkRevision := func(want int64, write string) {
		t.Helper()
		var got int64
		require.NoError(t, tx.QueryRow(ctx, write+` RETURNING revision`).Scan(&got))
		require.Equal(t, want, got)
	}
	checkRevision(1, `UPDATE tickets SET title=title WHERE id=1`)
	checkRevision(1, `UPDATE tickets SET revision=99,updated_at=now(),status_changed_at=now(),sla_due_at=now() WHERE id=1`)
	checkRevision(2, `UPDATE tickets SET assigned_to=2 WHERE id=1`)
	checkRevision(3, `UPDATE tickets SET title='edited',priority=2,custom_data='{"count":1}' WHERE id=1`)
	checkRevision(3, `UPDATE tickets SET custom_data='{"count":1.0}' WHERE id=1`)
	checkRevision(4, `UPDATE tickets SET status='assigned',definition_version=1 WHERE id=1`)
	checkRevision(5, `UPDATE tickets SET org_path='root.moved' WHERE id=1`)
	down, err := os.ReadFile("../../../migrations/000087_ticket_evidence_transition.down.sql")
	require.NoError(t, err)
	_, err = tx.Exec(ctx, `SAVEPOINT empty_receipts;
		INSERT INTO ticket_evidence_transition_receipts
		(ticket_id,actor_id,idempotency_key,request_hash,result_status,result_revision,result_definition_version)
		VALUES (1,2,'synthetic-key',decode(repeat('01',32),'hex'),'assigned',5,1);`)
	require.NoError(t, err)
	_, err = tx.Exec(ctx, `DELETE FROM tickets WHERE id=1`)
	require.NoError(t, err)
	var detached bool
	require.NoError(t, tx.QueryRow(ctx, `SELECT ticket_id IS NULL FROM ticket_evidence_transition_receipts
		WHERE actor_id=2 AND idempotency_key='synthetic-key'`).Scan(&detached))
	require.True(t, detached, "deleting a ticket must not free a used operation key")
	_, err = tx.Exec(ctx, string(down))
	require.ErrorContains(t, err, "retain receipts and revision")
	_, err = tx.Exec(ctx, `ROLLBACK TO SAVEPOINT empty_receipts`)
	require.NoError(t, err)
	_, err = tx.Exec(ctx, string(down))
	require.NoError(t, err)
	var status string
	require.NoError(t, tx.QueryRow(ctx, `SELECT status FROM tickets WHERE id=1`).Scan(&status))
	require.Equal(t, "assigned", status)
}

func TestD02EvidenceTransactionAndReplay(t *testing.T) {
	ctx := context.Background()
	var migrated bool
	require.NoError(t, testPool.QueryRow(ctx, `SELECT to_regclass('public.ticket_evidence_transition_receipts') IS NOT NULL`).Scan(&migrated))
	if !migrated {
		up, err := os.ReadFile("../../../migrations/000087_ticket_evidence_transition.up.sql")
		require.NoError(t, err)
		_, err = testPool.Exec(ctx, string(up))
		require.NoError(t, err)
	}
	env := newB53Env(t) // Real PG, local object metadata and a stub object client.
	svc := env.svc
	code := "evidence_" + uniqueSuffix()
	_, err := svc.CreateTicketType(ctx, &model.CreateTicketTypeRequest{Code: code, Name: code})
	require.NoError(t, err)
	_, err = svc.ReplaceTicketTypeFields(ctx, code, &model.ReplaceTypeFieldsRequest{Fields: []model.TicketTypeFieldInput{
		{FieldKey: "plan", FieldLabel: "Plan", FieldType: "input"},
		{FieldKey: "files", FieldLabel: "Files", FieldType: "file", FieldOptions: json.RawMessage(`["image/png"]`)},
	}})
	require.NoError(t, err)
	_, err = svc.UpdateTicketType(ctx, code, &model.UpdateTicketTypeRequest{TransitionMeta: json.RawMessage(`{
		"assigned":{"sla_due_hours":2},
		"open>assigned":{"collect_fields":[{"key":"plan","required":true},{"key":"files","required":true}],"validator":"custom_data['plan'] == 'fixed'"}
	}`)})
	require.NoError(t, err)
	def := publishD01(t, svc, code, env.u1)
	boundFile := env.confirmedCreate(t, env.u1, "attachment_create")
	freshFile := env.confirmedCreate(t, env.u1, "attachment_create")
	otherActorFile := env.confirmedCreate(t, env.u2, "attachment_create")
	create := func(keys ...string) *model.Ticket {
		t.Helper()
		req := env.createReq(keys)
		req.TypeCode = code
		created, err := svc.Create(ctx, req, env.u1)
		require.NoError(t, err)
		return created
	}
	ticket := create(boundFile)
	otherTicketFile := env.confirmedCreate(t, env.u1, "attachment_create")
	otherTicket := create(otherTicketFile)
	operationKey := "d02-" + uniqueSuffix()
	command := func(id, revision, version int64, key, plan string, files ...string) evidenceTransitionRequest {
		t.Helper()
		patch, err := json.Marshal(map[string]any{"plan": plan, "files": files})
		require.NoError(t, err)
		cmd, err := parseEvidenceTransitionRequest(&model.TransitionTicketRequest{
			ID: id, To: StatusAssigned, Comment: "synthetic submission", IdempotencyKey: key,
			ExpectedRevision: json.RawMessage(fmt.Sprintf(`"%d"`, revision)), DefinitionVersion: json.RawMessage(fmt.Sprintf(`"%d"`, version)),
			CustomDataPatch: patch,
		})
		require.NoError(t, err)
		return cmd
	}
	lock := func(id int64) (pgx.Tx, *model.Ticket, *model.TicketDefinitionSnapshot) {
		t.Helper()
		tx, err := testPool.Begin(ctx)
		require.NoError(t, err)
		t.Cleanup(func() { _ = tx.Rollback(ctx) })
		_, err = tx.Exec(ctx, `SELECT code FROM ticket_types WHERE code=$1 FOR SHARE`, code)
		require.NoError(t, err)
		before, err := svc.ticketRepo.GetByIDForUpdateTx(ctx, tx, id)
		require.NoError(t, err)
		bound, err := svc.ticketRepo.GetTicketDefinitionTx(ctx, tx, before.TypeCode, *before.DefinitionVersion)
		require.NoError(t, err)
		snapshot, err := decodePublishedDefinition(bound)
		require.NoError(t, err)
		return tx, before, snapshot
	}
	assertFacts := func(status string, revision int64, attachments, events, receipts int) {
		t.Helper()
		var gotStatus string
		var gotRevision int64
		var gotAttachments, gotEvents, gotReceipts int
		require.NoError(t, testPool.QueryRow(ctx, `SELECT status, revision,
			(SELECT count(*) FROM ticket_attachments WHERE ticket_id=t.id),
			(SELECT count(*) FROM ticket_events WHERE ticket_id=t.id AND action IN ($2,$3)),
			(SELECT count(*) FROM ticket_evidence_transition_receipts WHERE ticket_id=t.id)
			FROM tickets t WHERE id=$1`, ticket.ID, EventStatusChanged, EventComment).
			Scan(&gotStatus, &gotRevision, &gotAttachments, &gotEvents, &gotReceipts))
		require.Equal(t, status, gotStatus)
		require.Equal(t, revision, gotRevision)
		require.Equal(t, attachments, gotAttachments)
		require.Equal(t, events, gotEvents)
		require.Equal(t, receipts, gotReceipts)
	}
	for _, invalid := range []struct {
		cmd  evidenceTransitionRequest
		code int
	}{
		{command(ticket.ID, 2, def.DefinitionVersion, operationKey, "fixed", freshFile), errcode.ErrConcurrentModification.Code},
		{command(ticket.ID, 1, def.DefinitionVersion+1, operationKey, "fixed", freshFile), errcode.ErrConcurrentModification.Code},
		{command(ticket.ID, 1, def.DefinitionVersion, operationKey, "wrong plan", freshFile), errcode.ErrInvalidParams.Code},
		{command(ticket.ID, 1, def.DefinitionVersion, operationKey, "fixed", otherActorFile), errcode.ErrInvalidParams.Code},
		{command(ticket.ID, 1, def.DefinitionVersion, operationKey, "fixed", otherTicketFile), errcode.ErrInvalidParams.Code},
	} {
		tx, before, snapshot := lock(ticket.ID)
		_, err := svc.applyEvidenceTransitionTx(ctx, tx, invalid.cmd, before, snapshot, env.u1)
		requireErrCode(t, err, invalid.code)
		require.NoError(t, tx.Rollback(ctx))
		assertFacts(StatusOpen, 1, 1, 0, 0)
	}
	cmd := command(ticket.ID, 1, def.DefinitionVersion, operationKey, "fixed", boundFile, freshFile)
	tx, before, snapshot := lock(ticket.ID)
	_, err = svc.applyEvidenceTransitionTx(ctx, tx, cmd, before, snapshot, env.u1)
	require.NoError(t, err)
	// Simulate the caller's next mandatory transactional write failing.
	_, err = tx.Exec(ctx, `SELECT 1/0`)
	require.Error(t, err)
	require.ErrorIs(t, tx.Commit(ctx), pgx.ErrTxCommitRollback)
	assertFacts(StatusOpen, 1, 1, 0, 0)
	var purpose string
	require.NoError(t, testPool.QueryRow(ctx, `SELECT purpose FROM file_objects WHERE bucket='b53' AND object_key=$1`, freshFile).Scan(&purpose))
	require.Equal(t, "attachment_create", purpose)

	tx, before, snapshot = lock(ticket.ID)
	result, err := svc.applyEvidenceTransitionTx(ctx, tx, cmd, before, snapshot, env.u1)
	require.NoError(t, err)
	require.NoError(t, tx.Commit(ctx))
	assertFacts(StatusAssigned, 2, 2, 2, 1)
	var data json.RawMessage
	var entered, deadline time.Time
	require.NoError(t, testPool.QueryRow(ctx, `SELECT custom_data,status_changed_at,sla_due_at FROM tickets WHERE id=$1`, ticket.ID).Scan(&data, &entered, &deadline))
	require.JSONEq(t, string(cmd.patch), string(data))
	require.True(t, entered.Equal(*result.StatusChangedAt))
	require.Equal(t, 2*time.Hour, deadline.Sub(entered))

	// Advance the ticket after the successful operation, then retry the original.
	_, err = testPool.Exec(ctx, `UPDATE tickets SET status='in_progress',status_changed_at=statement_timestamp() WHERE id=$1`, ticket.ID)
	require.NoError(t, err)
	tx, _, _ = lock(ticket.ID)
	replayed, err := svc.findEvidenceTransitionReplayTx(ctx, tx, cmd, env.u1)
	require.NoError(t, err)
	require.NotNil(t, replayed)
	require.Equal(t, result, *replayed)
	changed := command(ticket.ID, 1, def.DefinitionVersion, operationKey, "changed", boundFile, freshFile)
	_, err = svc.findEvidenceTransitionReplayTx(ctx, tx, changed, env.u1)
	require.ErrorIs(t, err, errcode.ErrConflict)
	notActorsReceipt, err := svc.findEvidenceTransitionReplayTx(ctx, tx, cmd, env.u2)
	require.NoError(t, err)
	require.Nil(t, notActorsReceipt)
	require.NoError(t, tx.Commit(ctx))
	tx, _, _ = lock(otherTicket.ID)
	crossTarget := command(otherTicket.ID, 1, def.DefinitionVersion, operationKey, "fixed", boundFile, freshFile)
	_, err = svc.findEvidenceTransitionReplayTx(ctx, tx, crossTarget, env.u1)
	require.ErrorIs(t, err, errcode.ErrConflict)
	// The repository uniqueness guard also rejects a writer racing a lookup.
	err = svc.ticketRepo.RecordTicketEvidenceTransitionReceiptTx(ctx, tx, otherTicket.ID, env.u1, operationKey, crossTarget.hash[:], repository.TicketEvidenceTransitionResult{Status: StatusAssigned, Revision: 2, DefinitionVersion: def.DefinitionVersion})
	require.ErrorIs(t, err, errcode.ErrConflict)
	require.NoError(t, tx.Rollback(ctx))
	assertFacts(StatusInProgress, 3, 2, 2, 1)
}
