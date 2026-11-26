//go:build integration

package manageracceptance_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMockDirectoryDepartmentOwnersCLI(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "directory-sync-mock")
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	build := exec.CommandContext(ctx, "go", "build", "-o", binary, "../../../../cmd/directory-sync-mock")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build real mock directory CLI: %v\n%s", err, output)
	}
	db := directoryCLIIntegrationDB(t)
	cli := managerCLI{binary: binary, databaseURL: db.Config().ConnString()}

	const scope, otherScope = "synthetic-owner-scope", "synthetic-other-owner-scope"
	execSQL(t, db, `INSERT INTO directory_mock_scopes(scope_ref, success_baseline, success_cursor)
		VALUES ($1, 'synthetic-baseline', 'synthetic-cursor'), ($2, 'synthetic-other', 'synthetic-other-cursor')`, scope, otherScope)
	bindEmployee(t, db, scope, "owner-a", "mock-directory", 2, 1)
	bindEmployee(t, db, scope, "owner-b", "mock-directory", 3, 1)
	bindEmployee(t, db, scope, "owner-inactive", "mock-directory", 2, 0)
	bindEmployee(t, db, scope, "owner-local", "local", 1, 1)
	bindEmployee(t, db, scope, "owner-other-tenant", "mock-directory", 2, 1)
	bindEmployee(t, db, otherScope, "owner-other-scope", "mock-directory", 2, 1)
	execSQL(t, db, `UPDATE users SET tenant_id=2 WHERE external_id='owner-other-tenant'`)

	execSQL(t, db, `INSERT INTO directory_mock_departments(scope_ref, external_id, display_name, source_revision)
		VALUES ($1, 'owner-dept', 'Owner dept', 1)`, scope)
	execSQL(t, db, `WITH department AS (
		INSERT INTO organizations(code, name, path, source, external_id, owner_user_ids)
		VALUES ('owner-dept', 'Owner dept', 'owner_dept', 'mock-directory', 'owner-dept',
			ARRAY(SELECT id FROM users WHERE external_id='owner-local')) RETURNING id
	)
	INSERT INTO directory_mock_department_bindings(scope_ref, external_id, org_id)
	SELECT $1, 'owner-dept', id FROM department`, scope)
	execSQL(t, db, `INSERT INTO user_orgs(user_id, org_id, is_primary, org_member_role, ticket_scope, source)
		SELECT u.id, o.id, true, 'member', 'assigned', 'local'
		FROM users u CROSS JOIN organizations o WHERE u.external_id='owner-local' AND o.code='owner-dept'`)
	execSQL(t, db, `WITH local_role AS (
		INSERT INTO roles(code, name, priority) VALUES ('synthetic-owner-role', 'Synthetic owner role', 20)
		RETURNING id
	)
	INSERT INTO user_roles(user_id, role_id)
	SELECT u.id, r.id FROM users u CROSS JOIN local_role r WHERE u.external_id='owner-local'`)
	execSQL(t, db, `INSERT INTO org_roles(org_id, role_id)
	SELECT o.id, r.id FROM organizations o CROSS JOIN roles r
	WHERE o.code='owner-dept' AND r.code='synthetic-owner-role'`)

	const operation = "owners-shared-op"
	const managerRequest = `{"provider_ref":"mock-directory","scope_ref":"synthetic-owner-scope","operation_id":"owners-shared-op","employee_external_id":"owner-a","identity_generation":2,"manager_external_id":"owner-b","manager_identity_generation":3}`
	managerBefore, _ := cli.run(t, managerRequest, 0, "resolve-manager")
	const transferRequest = `{"provider_ref":"mock-directory","scope_ref":"synthetic-owner-scope","operation_id":"owners-shared-op","employee_external_id":"owner-a","identity_generation":2,"from_department_external_id":"owner-dept","to_department_external_id":"owner-unmapped-dept","source_revision":12}`
	transferBefore, _ := cli.run(t, transferRequest, 3, "transfer-primary")
	snapshot := businessSnapshot(t, db)

	makeInput := func(operationID, department, candidates string) string {
		return `{"provider_ref":"mock-directory","scope_ref":"` + scope + `","operation_id":"` + operationID +
			`","department_external_id":"` + department + `","candidates":` + candidates + `}`
	}
	const first = `{"employee_external_id":"owner-a","identity_generation":2}`
	const second = `{"employee_external_id":"owner-b","identity_generation":3}`
	submit := func(t *testing.T, operationID, department, candidates string, exit int, status, problem string) ([]byte, []ownerCandidateReceipt) {
		t.Helper()
		out, stderr := cli.run(t, makeInput(operationID, department, candidates), exit, "resolve-department-owners")
		var receipt struct {
			RunID                string                  `json:"run_id"`
			ScopeRef             string                  `json:"scope_ref"`
			OperationID          string                  `json:"operation_id"`
			DepartmentExternalID string                  `json:"department_external_id"`
			Status               string                  `json:"status"`
			Problem              string                  `json:"problem"`
			Candidates           []ownerCandidateReceipt `json:"candidates"`
		}
		if err := json.Unmarshal(out, &receipt); err != nil || stderr != "" || receipt.RunID == "" ||
			receipt.Status != status || receipt.Problem != problem || receipt.ScopeRef != scope ||
			receipt.OperationID != operationID || receipt.DepartmentExternalID != department {
			t.Fatalf("department owner receipt: stdout=%q stderr=%q err=%v", out, stderr, err)
		}
		recovered, stderr := cli.run(t, "", 0, "department-owner-receipt", scope, operationID)
		if !bytes.Equal(out, recovered) || stderr != "" {
			t.Fatalf("recovered department owner receipt: stdout=%q stderr=%q want=%q", recovered, stderr, out)
		}
		return out, receipt.Candidates
	}

	var original []byte
	t.Run("resolves_multiple_explicit_people_without_granting_authority", func(t *testing.T) {
		out, ids := submit(t, operation, "owner-dept", "["+first+","+second+"]", 0, "resolved", "")
		original = out
		if len(ids) != 2 || ids[0] != (ownerCandidateReceipt{"owner-a", 2, "resolved", ""}) ||
			ids[1] != (ownerCandidateReceipt{"owner-b", 3, "resolved", ""}) {
			t.Fatalf("expected both bound users, got %+v", ids)
		}
		assertBusinessUnchanged(t, db, snapshot)
	})
	t.Run("distinguishes_missing_and_invalid_candidates", func(t *testing.T) {
		cases := []struct {
			name, department, candidates, status, problem, candidateStatus, candidateProblem string
		}{
			{"empty", "owner-dept", "[]", "unavailable", "owners_not_supplied", "", ""},
			{"inactive", "owner-dept", `[{"employee_external_id":"owner-inactive","identity_generation":2}]`, "unavailable", "no_viable_owner", "invalid", "owner_link_invalid"},
			{"stale", "owner-dept", `[{"employee_external_id":"owner-a","identity_generation":3}]`, "unavailable", "no_viable_owner", "invalid", "owner_link_invalid"},
			{"local", "owner-dept", `[{"employee_external_id":"owner-local","identity_generation":1}]`, "unavailable", "no_viable_owner", "invalid", "owner_link_invalid"},
			{"other-tenant", "owner-dept", `[{"employee_external_id":"owner-other-tenant","identity_generation":2}]`, "unavailable", "no_viable_owner", "invalid", "owner_link_invalid"},
			{"other-scope", "owner-dept", `[{"employee_external_id":"owner-other-scope","identity_generation":2}]`, "unavailable", "no_viable_owner", "missing", "owner_not_registered"},
			{"missing-dept", "owner-unmapped-dept", "[" + first + "]", "unavailable", "department_unresolved", "", ""},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				_, ids := submit(t, "owners-"+tc.name, tc.department, tc.candidates, 3, tc.status, tc.problem)
				if tc.candidateStatus == "" {
					if len(ids) != 0 {
						t.Fatalf("unexpected candidate results %+v", ids)
					}
				} else if len(ids) != 1 || ids[0].Status != tc.candidateStatus || ids[0].Problem != tc.candidateProblem {
					t.Fatalf("unexpected candidate results %+v", ids)
				}
			})
		}
		assertBusinessUnchanged(t, db, snapshot)
	})
	t.Run("keeps_viable_owner_among_invalid_candidates", func(t *testing.T) {
		_, results := submit(t, "owners-mixed", "owner-dept",
			"["+first+","+`{"employee_external_id":"owner-inactive","identity_generation":2}]`, 0, "resolved", "")
		if len(results) != 2 || results[0] != (ownerCandidateReceipt{"owner-a", 2, "resolved", ""}) ||
			results[1] != (ownerCandidateReceipt{"owner-inactive", 2, "invalid", "owner_link_invalid"}) {
			t.Fatalf("unexpected mixed candidate results %+v", results)
		}
		assertBusinessUnchanged(t, db, snapshot)
	})
	t.Run("historical_replay_conflict_and_namespace", func(t *testing.T) {
		execSQL(t, db, `UPDATE users SET status=0 WHERE external_id='owner-a'`)
		staleSnapshot := businessSnapshot(t, db)
		out, _ := submit(t, operation, "owner-dept", "["+first+","+second+"]", 0, "resolved", "")
		if !bytes.Equal(out, original) {
			t.Fatalf("historical replay got %q want %q", out, original)
		}
		for _, check := range []struct {
			input      string
			args       []string
			diagnostic string
		}{
			{makeInput(operation, "owner-dept", "["+second+"]"), []string{"resolve-department-owners"}, "mock_department_owner_content_conflict"},
			{"", []string{"department-owner-receipt", otherScope, operation}, "mock_department_owner_receipt_not_found"},
			{makeInput("duplicate-owner", "owner-dept", "["+first+","+first+"]"), []string{"resolve-department-owners"}, "invalid_mock_department_owner_resolution"},
			{strings.Replace(makeInput("wrong-provider", "owner-dept", "[]"), "mock-directory", "local", 1), []string{"resolve-department-owners"}, "invalid_mock_department_owner_resolution"},
		} {
			out, stderr := cli.run(t, check.input, 2, check.args...)
			if len(out) != 0 || stderr != check.diagnostic {
				t.Fatalf("rejected command %v: stdout=%q stderr=%q", check.args, out, stderr)
			}
		}
		managerAfter, _ := cli.run(t, "", 0, "manager-receipt", scope, operation)
		transferAfter, _ := cli.run(t, "", 0, "transfer-receipt", scope, operation)
		if !bytes.Equal(managerAfter, managerBefore) || !bytes.Equal(transferAfter, transferBefore) {
			t.Fatalf("shared public ID changed manager or transfer receipts")
		}
		assertBusinessUnchanged(t, db, staleSnapshot)
	})
}

type ownerCandidateReceipt struct {
	EmployeeExternalID string `json:"employee_external_id"`
	IdentityGeneration int64  `json:"identity_generation"`
	Status             string `json:"status"`
	Problem            string `json:"problem"`
}
