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

	const mapping = "synthetic-approved-map"
	execSQL(t, db, `INSERT INTO directory_mock_transfer_runs(scope_ref, operation_id, run_id, input_hash, receipt)
		SELECT $1, 'retirement-mapping/' || m.ref, gen_random_uuid(), repeat('0',64), jsonb_build_object(
			'approved', m.ref <> 'revoked-mapping', 'source_department_external_id', m.source, 'target_department_external_id', m.target)
		FROM (VALUES ($2::text,'retire-source','retire-target'), ('other-mapping','retire-source','retire-target'),
			('wrong-source-map','different-source','retire-target'), ('wrong-target-map','retire-source','different-target'),
			('cross-scope-map','retire-source','retire-cross-scope'), ('self-map','retire-source','retire-source'),
			('revoked-mapping','retire-source','retire-target')) m(ref,source,target)`, scope, mapping)
	execSQL(t, db, `INSERT INTO directory_mock_transfer_runs(scope_ref, operation_id, run_id, input_hash, receipt)
		VALUES ('synthetic-retirement-other', 'retirement-mapping/wrong-scope-map', gen_random_uuid(), repeat('0',64),
			'{"approved":true,"source_department_external_id":"retire-source","target_department_external_id":"retire-target"}')`)

	ticketSnapshot := func(t *testing.T) string {
		t.Helper()
		var state string
		if err := db.QueryRow(t.Context(), `SELECT jsonb_build_object(
			'tickets', (SELECT jsonb_agg(t ORDER BY t.id) FROM tickets t),
			'events', (SELECT jsonb_agg(e ORDER BY e.id) FROM ticket_events e),
			'risks', (SELECT jsonb_agg(r ORDER BY r.id) FROM risk_items r)
		)::text`).Scan(&state); err != nil {
			t.Fatal(err)
		}
		return state
	}
	sourceSnapshot, sourceTicketSnapshot := businessSnapshot(t, db), ticketSnapshot(t)
	assertUnchanged := func(t *testing.T, iam, tickets string) {
		t.Helper()
		assertBusinessUnchanged(t, db, iam)
		if ticketSnapshot(t) != tickets {
			t.Fatal("preview changed tickets or ticket history")
		}
	}
	makeInput := func(operation, target, mapping string) string {
		return `{"provider_ref":"mock-directory","scope_ref":"` + scope + `","operation_id":"` + operation +
			`","source_department_external_id":"retire-source","target_department_external_id":"` + target +
			`","approved_mapping_ref":"` + mapping + `"}`
	}
	submit := func(t *testing.T, input string, exit int) ([]byte, retirementReceipt) {
		t.Helper()
		out, stderr := cli.run(t, input, exit, "preview-department-retirement")
		var receipt retirementReceipt
		if err := json.Unmarshal(out, &receipt); err != nil || stderr != "" ||
			len(receipt.PreviewHash) != 64 || receipt.RunID == "" || receipt.OperationID == "" {
			t.Fatalf("retirement preview: stdout=%q stderr=%q err=%v", out, stderr, err)
		}
		return out, receipt
	}

	input := makeInput("retire-plan", "retire-target", mapping)
	original, receipt := submit(t, input, 0)
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
	assertUnchanged(t, sourceSnapshot, sourceTicketSnapshot)

	for _, check := range []struct{ operation, target, mapping, problem string }{
		{"retire-no-mapping", "retire-target", "", "mapping_unavailable"},
		{"retire-no-target", "", mapping, "target_department_unresolved"},
		{"retire-cross-scope-target", "retire-cross-scope", "cross-scope-map", "target_department_unresolved"},
		{"retire-self-target", "retire-source", "self-map", "target_department_unresolved"},
	} {
		_, pending := submit(t, makeInput(check.operation, check.target, check.mapping), 3)
		if pending.Status != "pending" || pending.Problem != check.problem || pending.Snapshot.Target.ID != 0 {
			t.Fatalf("wanted pending %q, got %+v", check.problem, pending)
		}
	}
	assertUnchanged(t, sourceSnapshot, sourceTicketSnapshot)

	t.Run("rejects_arbitrary_or_mismatched_approval", func(t *testing.T) {
		for _, ref := range []string{"unknown-mapping", "wrong-source-map", "wrong-target-map", "wrong-scope-map", "revoked-mapping"} {
			_, pending := submit(t, makeInput("retire-"+ref, "retire-target", ref), 3)
			if pending.Status != "pending" || pending.Problem != "mapping_unavailable" || pending.Snapshot.Target.ID != 0 {
				t.Fatalf("unregistered approval %s gained a target: %+v", ref, pending)
			}
		}
		assertUnchanged(t, sourceSnapshot, sourceTicketSnapshot)
	})
	_, sameObservation := submit(t, makeInput("retire-same-facts", "retire-target", mapping), 0)
	if sameObservation.PreviewHash != receipt.PreviewHash {
		t.Fatal("same mapping and facts changed preview hash")
	}
	_, remapped := submit(t, makeInput("retire-new-mapping", "retire-target", "other-mapping"), 0)
	if remapped.PreviewHash == receipt.PreviewHash {
		t.Fatal("different mapping reused original preview hash")
	}
	execSQL(t, db, `UPDATE tickets SET status='pending_verify' WHERE id=$1`, ticket.ID)
	changedIAM, changedTickets := businessSnapshot(t, db), ticketSnapshot(t)
	_, changed := submit(t, makeInput("retire-new-plan", "retire-target", mapping), 0)
	if changed.PreviewHash == receipt.PreviewHash || changed.Snapshot.InFlightTickets[0].Revision != 2 {
		t.Fatalf("changed ticket was not reflected in new preview %+v", changed)
	}
	lookedUp, stderr := cli.run(t, "", 0, "department-retirement-receipt", scope, "retire-plan")
	replay, _ := submit(t, input, 0)
	if stderr != "" || !bytes.Equal(lookedUp, original) || !bytes.Equal(replay, original) {
		t.Fatal("new observation changed prior retirement receipt")
	}
	out, stderr := cli.run(t, makeInput("retire-plan", "retire-target", "other-mapping"), 2, "preview-department-retirement")
	if len(out) != 0 || stderr != "mock_department_retirement_content_conflict" {
		t.Fatalf("changed mapping reused original operation: stdout=%q stderr=%q", out, stderr)
	}
	assertUnchanged(t, changedIAM, changedTickets)

	t.Run("tracks_new_unassigned_work_and_current_risks", func(t *testing.T) {
		execSQL(t, db, `INSERT INTO tickets(type_code, title, status, created_by, org_id, org_path)
			SELECT 'incident', 'retire-unassigned', 'open', u.id, o.id, o.path
			FROM organizations o CROSS JOIN users u
			WHERE o.code='retire-virtual' AND u.external_id='retire-assignee'`)
		unassignedFacts := ticketSnapshot(t)
		_, unassigned := submit(t, makeInput("retire-with-unassigned", "retire-target", mapping), 0)
		if unassigned.PreviewHash == changed.PreviewHash || len(unassigned.Snapshot.InFlightTickets) != 2 {
			t.Errorf("unassigned work was omitted from preview: %+v", unassigned.Snapshot)
		}
		assertUnchanged(t, changedIAM, unassignedFacts)
		execSQL(t, db, `WITH family AS (
			INSERT INTO risk_families(fingerprint_scope, canonical_key_hash)
			VALUES ('synthetic-retirement', repeat('9',64)) RETURNING id
		)
		INSERT INTO risk_items(human_key, family_id, status, org_id, affected_org_id, title, created_by)
		SELECT v.key, f.id, v.status, target.id, affected.id, v.key, u.id
		FROM family f CROSS JOIN (VALUES ('retire-risk-current','assessed'),('retire-risk-closed','closed')) v(key,status)
		CROSS JOIN organizations target CROSS JOIN organizations affected CROSS JOIN users u
		WHERE target.code='retire-target' AND affected.code='retire-virtual' AND u.external_id='retire-assignee'`)
		riskFacts := ticketSnapshot(t)
		_, risk := submit(t, makeInput("retire-with-risk", "retire-target", mapping), 0)
		if risk.PreviewHash == unassigned.PreviewHash || len(risk.Snapshot.InFlightRisks) != 1 ||
			risk.Snapshot.InFlightRisks[0].AffectedOrgID != group.ID || risk.Snapshot.InFlightRisks[0].Revision != 1 {
			t.Fatalf("current affected risk missing from preview: %+v", risk.Snapshot)
		}
		assertUnchanged(t, changedIAM, riskFacts)
	})

	execSQL(t, db, `UPDATE organizations SET status=0 WHERE code='retire-target'`)
	inactiveIAM, inactiveFacts := businessSnapshot(t, db), ticketSnapshot(t)
	_, inactive := submit(t, makeInput("retire-inactive-target", "retire-target", mapping), 3)
	if inactive.Status != "pending" || inactive.Problem != "target_department_unresolved" || inactive.Snapshot.Target.ID != 0 {
		t.Fatalf("inactive target was not held: %+v", inactive)
	}
	assertUnchanged(t, inactiveIAM, inactiveFacts)
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
		InFlightRisks []struct {
			AffectedOrgID int64 `json:"affected_org_id"`
			Revision      int64 `json:"revision"`
		} `json:"in_flight_risks"`
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
