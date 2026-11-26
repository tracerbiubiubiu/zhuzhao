//go:build integration

package manageracceptance_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestMockDirectoryRetirementPreviewCLI(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "directory-sync-mock")
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	build := exec.CommandContext(ctx, "go", "build", "-o", binary, "../../../../cmd/directory-sync-mock")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build real mock directory CLI: %v\n%s", err, output)
	}
	db := directoryCLIIntegrationDB(t)
	cli := managerCLI{binary: binary, databaseURL: db.Config().ConnString()}
	const scope = "synthetic-retirement-scope"

	execSQL(t, db, `INSERT INTO directory_mock_scopes(scope_ref) VALUES ($1), ('synthetic-retirement-other')`, scope)
	execSQL(t, db, `INSERT INTO directory_mock_departments(scope_ref, external_id, display_name, source_revision)
		VALUES ($1, 'retire-source', 'Source', 2), ($1, 'retire-target', 'Target', 3),
			   ('synthetic-retirement-other', 'retire-cross-scope', 'Other', 4)`, scope)
	execSQL(t, db, `WITH depts AS (
		INSERT INTO organizations(code, name, path, source, external_id, ticket_visibility)
		VALUES ('retire-source', 'Source', 'retire_source', 'mock-directory', 'retire-source', 'entity_transparent_read'),
			   ('retire-target', 'Target', 'retire_target', 'mock-directory', 'retire-target', 'project_isolated'),
			   ('retire-cross-scope', 'Cross scope', 'retire_other', 'mock-directory', 'retire-cross-scope', 'entity_transparent_read')
		RETURNING id, external_id
	)
	INSERT INTO directory_mock_department_bindings(scope_ref, external_id, org_id)
	SELECT d.scope_ref, d.external_id, o.id FROM depts o JOIN directory_mock_departments d USING(external_id)
	WHERE d.scope_ref IN ($1, 'synthetic-retirement-other')`, scope)
	execSQL(t, db, `INSERT INTO organizations(code, name, path, parent_id, source, is_virtual)
		SELECT 'retire-virtual', 'Retirement virtual group', 'retire_source.virtual', id, 'local', true
		FROM organizations WHERE code='retire-source'`)
	bindEmployee(t, db, scope, "retire-assignee", "mock-directory", 1, 1)
	bindEmployee(t, db, scope, "retire-local", "local", 1, 1)
	execSQL(t, db, `INSERT INTO user_orgs(user_id, org_id, org_member_role, ticket_scope, source)
		SELECT u.id, o.id, 'admin', 'group', 'local' FROM users u CROSS JOIN organizations o
		WHERE u.external_id='retire-local' AND o.code='retire-virtual'`)
	execSQL(t, db, `WITH local_role AS (
		INSERT INTO roles(code, name, priority) VALUES ('synthetic-retire-role', 'Synthetic retirement role', 20) RETURNING id
	)
	INSERT INTO org_roles(org_id, role_id)
	SELECT o.id, r.id FROM organizations o CROSS JOIN local_role r WHERE o.code='retire-source'`)
	execSQL(t, db, `INSERT INTO tickets(type_code, title, status, created_by, assigned_to, org_id, org_path)
		SELECT 'incident', v.title, v.status, u.id, u.id, o.id, o.path
		FROM (VALUES ('retire-inflight', 'in_progress'), ('retire-archived', 'closed')) v(title,status)
		CROSS JOIN users u CROSS JOIN organizations o
		WHERE u.external_id='retire-assignee' AND o.code='retire-virtual'`)
	execSQL(t, db, `INSERT INTO ticket_events(ticket_id, user_id, action, to_value)
		SELECT id, assigned_to, 'status_changed', 'closed' FROM tickets WHERE title='retire-archived'`)

	ticketSnapshot := func() string {
		t.Helper()
		var state string
		if err := db.QueryRow(t.Context(), `SELECT jsonb_build_object(
			'tickets', (SELECT jsonb_agg(t ORDER BY t.id) FROM tickets t),
			'events', (SELECT jsonb_agg(e ORDER BY e.id) FROM ticket_events e)
		)::text`).Scan(&state); err != nil {
			t.Fatal(err)
		}
		return state
	}
	sourceSnapshot, sourceTicketSnapshot := businessSnapshot(t, db), ticketSnapshot()
	assertUnchanged := func(iam, tickets string) {
		t.Helper()
		assertBusinessUnchanged(t, db, iam)
		if ticketSnapshot() != tickets {
			t.Fatal("preview changed tickets or ticket history")
		}
	}
	makeInput := func(operation, target, mapping string) string {
		return `{"provider_ref":"mock-directory","scope_ref":"` + scope + `","operation_id":"` + operation +
			`","source_department_external_id":"retire-source","target_department_external_id":"` + target +
			`","approved_mapping_ref":"` + mapping + `"}`
	}
	submit := func(input string, exit int) ([]byte, retirementReceipt) {
		t.Helper()
		out, stderr := cli.run(t, input, exit, "preview-department-retirement")
		var receipt retirementReceipt
		if err := json.Unmarshal(out, &receipt); err != nil || stderr != "" ||
			len(receipt.PreviewHash) != 64 || receipt.RunID == "" || receipt.OperationID == "" {
			t.Fatalf("retirement preview: stdout=%q stderr=%q err=%v", out, stderr, err)
		}
		return out, receipt
	}

	const mapping = "synthetic-approved-map"
	input := makeInput("retire-plan", "retire-target", mapping)
	original, receipt := submit(input, 0)
	if receipt.Status != "previewed" || receipt.ApprovedMappingRef != mapping ||
		receipt.SourceExternalID != "retire-source" || receipt.TargetExternalID != "retire-target" ||
		receipt.Snapshot.Source.SourceRevision != 2 || receipt.Snapshot.Target.SourceRevision != 3 ||
		receipt.Snapshot.Source.TicketVisibility != "entity_transparent_read" ||
		receipt.Snapshot.Target.TicketVisibility != "project_isolated" {
		t.Fatalf("unexpected retirement mapping/visibility %+v", receipt)
	}
	if len(receipt.Snapshot.VirtualGroups) != 1 || len(receipt.Snapshot.InFlightTickets) != 1 ||
		len(receipt.Snapshot.LocalMemberships) != 1 || len(receipt.Snapshot.OrgRoles) != 1 {
		t.Fatalf("unexpected affected items %+v", receipt.Snapshot)
	}
	group := receipt.Snapshot.VirtualGroups[0]
	ticket := receipt.Snapshot.InFlightTickets[0]
	membership := receipt.Snapshot.LocalMemberships[0]
	if group.Source != "local" || group.ParentID != receipt.Snapshot.Source.ID ||
		ticket.OrgID != group.ID || ticket.Status != "in_progress" || ticket.Revision != 1 ||
		membership.OrgID != group.ID || membership.Role != "admin" || membership.Scope != "group" ||
		receipt.Snapshot.OrgRoles[0].OrgID != receipt.Snapshot.Source.ID {
		t.Fatalf("unexpected item details %+v", receipt.Snapshot)
	}
	assertUnchanged(sourceSnapshot, sourceTicketSnapshot)

	for _, check := range []struct{ operation, target, mapping, problem string }{
		{"retire-no-mapping", "retire-target", "", "mapping_unavailable"},
		{"retire-no-target", "", mapping, "target_department_unresolved"},
		{"retire-cross-scope-target", "retire-cross-scope", mapping, "target_department_unresolved"},
		{"retire-self-target", "retire-source", mapping, "target_department_unresolved"},
	} {
		_, pending := submit(makeInput(check.operation, check.target, check.mapping), 3)
		if pending.Status != "pending" || pending.Problem != check.problem || pending.Snapshot.Target.ID != 0 {
			t.Fatalf("wanted pending %q, got %+v", check.problem, pending)
		}
	}
	assertUnchanged(sourceSnapshot, sourceTicketSnapshot)

	_, sameObservation := submit(makeInput("retire-same-facts", "retire-target", mapping), 0)
	if sameObservation.PreviewHash != receipt.PreviewHash {
		t.Fatal("same mapping and facts changed preview hash")
	}
	_, remapped := submit(makeInput("retire-new-mapping", "retire-target", "other-mapping"), 0)
	if remapped.PreviewHash == receipt.PreviewHash {
		t.Fatal("different mapping reused original preview hash")
	}
	execSQL(t, db, `UPDATE tickets SET status='pending_verify' WHERE id=$1`, ticket.ID)
	changedIAM, changedTickets := businessSnapshot(t, db), ticketSnapshot()
	_, changed := submit(makeInput("retire-new-plan", "retire-target", mapping), 0)
	if changed.PreviewHash == receipt.PreviewHash || changed.Snapshot.InFlightTickets[0].Revision != 2 {
		t.Fatalf("changed ticket was not reflected in new preview %+v", changed)
	}
	lookedUp, stderr := cli.run(t, "", 0, "department-retirement-receipt", scope, "retire-plan")
	replay, _ := submit(input, 0)
	if stderr != "" || !bytes.Equal(lookedUp, original) || !bytes.Equal(replay, original) {
		t.Fatal("new observation changed prior retirement receipt")
	}
	out, stderr := cli.run(t, makeInput("retire-plan", "retire-target", "other-mapping"), 2, "preview-department-retirement")
	if len(out) != 0 || stderr != "mock_department_retirement_content_conflict" {
		t.Fatalf("changed mapping reused original operation: stdout=%q stderr=%q", out, stderr)
	}
	assertUnchanged(changedIAM, changedTickets)

	execSQL(t, db, `UPDATE organizations SET status=0 WHERE code='retire-target'`)
	inactiveIAM := businessSnapshot(t, db)
	_, inactive := submit(makeInput("retire-inactive-target", "retire-target", mapping), 3)
	if inactive.Status != "pending" || inactive.Problem != "target_department_unresolved" || inactive.Snapshot.Target.ID != 0 {
		t.Fatalf("inactive target was not held: %+v", inactive)
	}
	assertUnchanged(inactiveIAM, changedTickets)
}

type retirementReceipt struct {
	RunID              string `json:"run_id"`
	OperationID        string `json:"operation_id"`
	Status             string `json:"status"`
	Problem            string `json:"problem"`
	PreviewHash        string `json:"preview_hash"`
	ApprovedMappingRef string `json:"approved_mapping_ref"`
	SourceExternalID   string `json:"source_department_external_id"`
	TargetExternalID   string `json:"target_department_external_id"`
	Snapshot           struct {
		Source, Target struct {
			ID               int64  `json:"id"`
			SourceRevision   int64  `json:"source_revision"`
			TicketVisibility string `json:"ticket_visibility"`
		}
		VirtualGroups []struct {
			ID       int64  `json:"id"`
			ParentID int64  `json:"parent_id"`
			Source   string `json:"source"`
		} `json:"virtual_groups"`
		InFlightTickets []struct {
			ID       int64  `json:"id"`
			OrgID    int64  `json:"org_id"`
			Revision int64  `json:"revision"`
			Status   string `json:"status"`
		} `json:"in_flight_tickets"`
		LocalMemberships []struct {
			OrgID int64  `json:"org_id"`
			Role  string `json:"org_member_role"`
			Scope string `json:"ticket_scope"`
		} `json:"local_memberships"`
		OrgRoles []struct {
			OrgID int64 `json:"org_id"`
		} `json:"org_roles"`
	} `json:"snapshot"`
}
