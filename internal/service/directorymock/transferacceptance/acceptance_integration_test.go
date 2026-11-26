//go:build integration

package transferacceptance_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
	"github.com/tracerbiubiubiu/zhuzhao/internal/testutil"
)

func TestMockDirectoryTransferCLI(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "directory-sync-mock")
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	build := exec.CommandContext(ctx, "go", "build", "-o", binary, "../../../../cmd/directory-sync-mock")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build real mock directory CLI: %v\n%s", err, output)
	}
	db, terminate, err := testutil.SetupPostgresShared()
	if err != nil {
		t.Fatalf("start isolated PostgreSQL: %v", err)
	}
	t.Cleanup(terminate)
	cli := transferCLI{binary: binary, databaseURL: db.Config().ConnString()}

	if !t.Run("missing receipt", func(t *testing.T) {
		output, stderr := cli.run(t, "", 2, "transfer-receipt", "synthetic-absent", "synthetic-absent")
		if len(output) != 0 || stderr != "mock_transfer_receipt_not_found" {
			t.Fatalf("lookup missing transfer receipt: stdout=%q stderr=%q", output, stderr)
		}
	}) {
		return
	}

	fixture := seedTransferFixture(t, db)
	input := transferInput{
		ProviderRef: "mock-directory", ScopeRef: transferScope,
		OperationID: "move-to-b", EmployeeExternalID: "employee-transfer",
		IdentityGeneration: 1, FromDepartmentExternalID: "dept-a",
		ToDepartmentExternalID: "dept-b", SourceRevision: 12,
	}
	t.Run("unresolved target holds without advancing business state", func(t *testing.T) {
		request := input
		request.OperationID = "move-to-unresolved"
		request.ToDepartmentExternalID = "dept-unresolved"
		before := transferBusinessSnapshot(t, db)
		held := cli.submit(t, request, 3)
		assertTransferReceipt(t, held, transferReceipt{
			ScopeRef: transferScope, OperationID: request.OperationID, Status: "held",
			SourceRevision: 12, Problem: "target_department_unresolved",
		})
		if fresh := cli.lookup(t, request.OperationID); fresh != held {
			t.Fatalf("fresh process retrieved %+v, want %+v", fresh, held)
		}
		if replay := cli.submit(t, request, 3); replay != held {
			t.Fatalf("held replay returned %+v, want %+v", replay, held)
		}
		if after := transferBusinessSnapshot(t, db); after != before {
			t.Fatal("unresolved target or held replay mutated business state")
		}
		assertTransferReceiptStored(t, db, held)
		assertSourceRevision(t, db, 11)
	})

	t.Run("receipt write failure rolls back the appointment and source revision", func(t *testing.T) {
		before := transferBusinessSnapshot(t, db)
		if _, err := db.Exec(t.Context(), `
			CREATE FUNCTION fail_synthetic_transfer_receipt() RETURNS trigger LANGUAGE plpgsql AS $$
			BEGIN
				RAISE EXCEPTION 'synthetic receipt write failure';
			END;
			$$;
			CREATE TRIGGER fail_synthetic_transfer_receipt BEFORE INSERT ON directory_mock_transfer_runs
			FOR EACH ROW EXECUTE FUNCTION fail_synthetic_transfer_receipt();
		`); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if _, err := db.Exec(context.Background(), `
				DROP TRIGGER fail_synthetic_transfer_receipt ON directory_mock_transfer_runs;
				DROP FUNCTION fail_synthetic_transfer_receipt();
			`); err != nil {
				t.Error(err)
			}
		})
		output, stderr := cli.run(t, input.encode(t), 1, "transfer-primary")
		if len(output) != 0 || stderr == "" {
			t.Fatalf("receipt write failure: stdout=%q stderr=%q", output, stderr)
		}
		output, stderr = cli.run(t, "", 2, "transfer-receipt", transferScope, input.OperationID)
		if len(output) != 0 || stderr != "mock_transfer_receipt_not_found" {
			t.Fatalf("lookup rolled-back receipt: stdout=%q stderr=%q", output, stderr)
		}
		if after := transferBusinessSnapshot(t, db); after != before {
			t.Fatal("failed receipt write mutated business state")
		}
	})

	t.Run("unadvanced revision cannot move the employee", func(t *testing.T) {
		request := input
		request.OperationID = "stale-move-to-b"
		request.SourceRevision = 11
		before := transferBusinessSnapshot(t, db)
		held := cli.submit(t, request, 3)
		assertTransferReceipt(t, held, transferReceipt{
			ScopeRef: transferScope, OperationID: request.OperationID, Status: "held",
			SourceRevision: 11, Problem: "source_revision_not_advanced",
		})
		if after := transferBusinessSnapshot(t, db); after != before {
			t.Fatal("unadvanced revision mutated business state")
		}
		assertTransferReceiptStored(t, db, held)
	})

	t.Run("department role grants hold the transfer", func(t *testing.T) {
		cases := []struct {
			name    string
			orgID   int64
			problem string
		}{
			{"source", fixture.orgA, "source_membership_role_grant"},
			{"target", fixture.orgB, "target_membership_role_grant"},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				var roleID int64
				if err := db.QueryRow(t.Context(), `INSERT INTO roles (code, name, priority)
					VALUES ('synthetic_department_role', 'Synthetic Department Role', 30) RETURNING id`).
					Scan(&roleID); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() {
					ctx := context.Background()
					if _, err := db.Exec(ctx, `DELETE FROM org_roles WHERE org_id = $1 AND role_id = $2`, tc.orgID, roleID); err != nil {
						t.Error(err)
					}
					if _, err := db.Exec(ctx, `DELETE FROM roles WHERE id = $1`, roleID); err != nil {
						t.Error(err)
					}
				})
				if _, err := db.Exec(t.Context(), `INSERT INTO org_roles (org_id, role_id) VALUES ($1, $2)`,
					tc.orgID, roleID); err != nil {
					t.Fatal(err)
				}
				roles, err := repository.NewRoleRepo(db).GetEffectiveRoleCodes(t.Context(), fixture.user)
				hasDeptRole := slices.Contains(roles, "synthetic_department_role")
				if want := tc.name == "source"; err != nil || hasDeptRole != want {
					t.Fatalf("department role effective before transfer = %t, want %t: %v", hasDeptRole, want, err)
				}
				before := transferBusinessSnapshot(t, db)
				request := input
				request.OperationID = "move-with-" + tc.name + "-role"
				held := cli.submit(t, request, 3)
				assertTransferReceipt(t, held, transferReceipt{
					ScopeRef: transferScope, OperationID: request.OperationID, Status: "held",
					SourceRevision: 12, Problem: tc.problem,
				})
				if fresh := cli.lookup(t, request.OperationID); fresh != held {
					t.Fatalf("fresh process retrieved %+v, want %+v", fresh, held)
				}
				if after := transferBusinessSnapshot(t, db); after != before {
					t.Fatal("held department role transfer mutated business state")
				}
				assertTransferReceiptStored(t, db, held)
			})
		}
	})

	t.Run("local membership writes cannot displace the managed primary", func(t *testing.T) {
		orgs := repository.NewOrgRepo(db)
		ctx := t.Context()
		cases := []struct {
			name string
			run  func() error
		}{
			{"AddMember", func() error { return orgs.AddMember(ctx, fixture.localOrg, fixture.user, true) }},
			{"AddMemberWithRole", func() error {
				return orgs.AddMemberWithRole(ctx, fixture.localOrg, fixture.user, true, "member", "assigned")
			}},
			{"RemoveMember", func() error { return orgs.RemoveMember(ctx, fixture.orgA, fixture.user) }},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				before := transferBusinessSnapshot(t, db)
				err := tc.run()
				var businessErr *errcode.Error
				if !errors.As(err, &businessErr) || businessErr.Code != errcode.ErrForbidden.Code {
					t.Fatalf("local write = %v, want forbidden", err)
				}
				if after := transferBusinessSnapshot(t, db); after != before {
					t.Fatal("forbidden local membership write mutated business state")
				}
			})
		}
	})

	t.Run("moves primary and current branch while preserving local appointments", func(t *testing.T) {
		orgs := repository.NewOrgRepo(db)
		assertBranchAccess(t, orgs, fixture.orgA, fixture.user, true)
		assertBranchAccess(t, orgs, fixture.orgB, fixture.user, false)
		localBefore := transferLocalSnapshot(t, db, fixture)
		assertLocalEffectiveRole(t, db, fixture.user)

		applied := cli.submit(t, input, 0)
		assertTransferReceipt(t, applied, transferReceipt{
			ScopeRef: transferScope, OperationID: input.OperationID, Status: "applied", SourceRevision: 12,
		})
		assertTransferReceiptStored(t, db, applied)
		assertSourceRevision(t, db, 12)
		assertBranchAccess(t, orgs, fixture.orgA, fixture.user, false)
		assertBranchAccess(t, orgs, fixture.orgB, fixture.user, true)
		assertLocalEffectiveRole(t, db, fixture.user)
		if after := transferLocalSnapshot(t, db, fixture); after != localBefore {
			t.Fatal("transfer mutated the local appointment, direct grant, or assigned role")
		}

		var primaryOrg int64
		var source, role, scope string
		var expires bool
		err := db.QueryRow(t.Context(), `SELECT org_id, source, org_member_role, ticket_scope, expires_at IS NOT NULL
			FROM user_orgs WHERE user_id = $1 AND is_primary`, fixture.user).
			Scan(&primaryOrg, &source, &role, &scope, &expires)
		if err != nil || primaryOrg != fixture.orgB || source != "mock-directory" ||
			role != "member" || scope != "assigned" || expires {
			t.Fatalf("primary membership: org=%d source=%q role=%q scope=%q expires=%t err=%v",
				primaryOrg, source, role, scope, expires, err)
		}

		var sourceStillPresent bool
		err = db.QueryRow(t.Context(), `SELECT EXISTS (SELECT 1 FROM user_orgs WHERE user_id = $1 AND org_id = $2)`,
			fixture.user, fixture.orgA).Scan(&sourceStillPresent)
		if err != nil || sourceStillPresent {
			t.Fatalf("old source appointment present = %t, want false: %v", sourceStillPresent, err)
		}

		before := transferBusinessSnapshot(t, db)
		if fresh := cli.lookup(t, input.OperationID); fresh != applied {
			t.Fatalf("fresh process retrieved %+v, want %+v", fresh, applied)
		}
		if replay := cli.submit(t, input, 0); replay != applied {
			t.Fatalf("applied replay returned %+v, want %+v", replay, applied)
		}
		conflict := input
		conflict.SourceRevision++
		output, stderr := cli.run(t, conflict.encode(t), 2, "transfer-primary")
		if len(output) != 0 || stderr != "mock_transfer_content_conflict" {
			t.Fatalf("conflicting transfer: stdout=%q stderr=%q", output, stderr)
		}
		if after := transferBusinessSnapshot(t, db); after != before {
			t.Fatal("receipt query, applied replay, or conflicting replay mutated business state")
		}
		assertTransferReceiptStored(t, db, applied)
	})
}

type transferCLI struct {
	binary      string
	databaseURL string
}

func (cli transferCLI) run(t *testing.T, input string, wantExit int, args ...string) ([]byte, string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, cli.binary, args...)
	cmd.Env = append(os.Environ(), "D06_MOCK_DATABASE_URL="+cli.databaseURL)
	if input != "" {
		cmd.Stdin = strings.NewReader(input)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	exitCode := 0
	if err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			t.Fatalf("run mock transfer CLI: %v", err)
		}
		exitCode = exitErr.ExitCode()
	}
	if exitCode != wantExit {
		t.Fatalf("%s exit=%d, want %d, stdout=%q stderr=%q", args[0], exitCode, wantExit, output, stderr.String())
	}
	return output, strings.TrimSpace(stderr.String())
}

const transferScope = "synthetic-single-transfer"

type transferInput struct {
	ProviderRef              string `json:"provider_ref"`
	ScopeRef                 string `json:"scope_ref"`
	OperationID              string `json:"operation_id"`
	EmployeeExternalID       string `json:"employee_external_id"`
	IdentityGeneration       int64  `json:"identity_generation"`
	FromDepartmentExternalID string `json:"from_department_external_id"`
	ToDepartmentExternalID   string `json:"to_department_external_id"`
	SourceRevision           int64  `json:"source_revision"`
}

type transferReceipt struct {
	RunID          string `json:"run_id"`
	ScopeRef       string `json:"scope_ref"`
	OperationID    string `json:"operation_id"`
	Status         string `json:"status"`
	SourceRevision int64  `json:"source_revision"`
	Problem        string `json:"problem"`
}

func (input transferInput) encode(t *testing.T) string {
	t.Helper()
	data, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func (cli transferCLI) submit(t *testing.T, input transferInput, wantExit int) transferReceipt {
	t.Helper()
	output, stderr := cli.run(t, input.encode(t), wantExit, "transfer-primary")
	return decodeTransferReceipt(t, output, stderr)
}

func (cli transferCLI) lookup(t *testing.T, operationID string) transferReceipt {
	t.Helper()
	output, stderr := cli.run(t, "", 0, "transfer-receipt", transferScope, operationID)
	return decodeTransferReceipt(t, output, stderr)
}

func decodeTransferReceipt(t *testing.T, output []byte, stderr string) transferReceipt {
	t.Helper()
	var receipt transferReceipt
	if err := json.Unmarshal(output, &receipt); err != nil {
		t.Fatalf("decode transfer receipt: %v (output=%q stderr=%q)", err, output, stderr)
	}
	return receipt
}

func assertTransferReceipt(t *testing.T, got, want transferReceipt) {
	t.Helper()
	if got.RunID == "" {
		t.Fatal("missing transfer run ID")
	}
	want.RunID = got.RunID
	if got != want {
		t.Fatalf("transfer receipt = %+v, want %+v", got, want)
	}
}

func assertTransferReceiptStored(t *testing.T, db *pgxpool.Pool, want transferReceipt) {
	t.Helper()
	var runID string
	var receipt []byte
	err := db.QueryRow(t.Context(), `SELECT run_id::text, receipt FROM directory_mock_transfer_runs
		WHERE scope_ref = $1 AND operation_id = $2`, want.ScopeRef, want.OperationID).Scan(&runID, &receipt)
	if err != nil {
		t.Fatal(err)
	}
	if got := decodeTransferReceipt(t, receipt, ""); got != want || runID != want.RunID {
		t.Fatalf("stored transfer receipt = %+v and run ID %q, want %+v", got, runID, want)
	}
}

type transferFixture struct {
	user      int64
	orgA      int64
	orgB      int64
	localOrg  int64
	localRole int64
}

func seedTransferFixture(t *testing.T, db *pgxpool.Pool) transferFixture {
	t.Helper()
	ctx := t.Context()
	var f transferFixture
	run := func(query string, args ...any) {
		t.Helper()
		if _, err := db.Exec(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	id := func(query string, args ...any) int64 {
		t.Helper()
		var value int64
		if err := db.QueryRow(ctx, query, args...).Scan(&value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	f.user = id(`INSERT INTO users (username, password, source, external_id)
		VALUES ('synthetic_transfer_user', 'unusable', 'mock-directory', 'employee-transfer') RETURNING id`)
	insertDepartment := `INSERT INTO organizations (code, name, path, source, external_id)
		VALUES ($1, $2, $1::varchar::ltree, 'mock-directory', $3) RETURNING id`
	f.orgA = id(insertDepartment, "department_a", "Synthetic Department A", "dept-a")
	f.orgB = id(insertDepartment, "department_b", "Synthetic Department B", "dept-b")
	f.localOrg = id(`INSERT INTO organizations (code, name, path, source, is_virtual)
		VALUES ('local_appointment', 'Synthetic Local Appointment', 'local_appointment', 'local', true) RETURNING id`)
	f.localRole = id(`INSERT INTO roles (code, name, priority)
		VALUES ('synthetic_direct_role', 'Synthetic Direct Role', 20) RETURNING id`)
	run(`INSERT INTO user_roles (user_id, role_id) VALUES ($1, $2)`, f.user, f.localRole)
	run(`INSERT INTO user_orgs (user_id, org_id, is_primary, source) VALUES ($1, $2, true, 'mock-directory')`,
		f.user, f.orgA)
	run(`INSERT INTO user_orgs (user_id, org_id, is_primary, source, org_member_role, ticket_scope, expires_at)
		VALUES ($1, $2, false, 'local', 'admin', 'group', '2099-01-01T00:00:00Z')`, f.user, f.localOrg)
	run(`INSERT INTO directory_mock_scopes (scope_ref, success_baseline, success_cursor)
		VALUES ($1, 'synthetic-dept-batch', 'synthetic-dept-cursor')`, transferScope)
	run(`INSERT INTO directory_mock_departments (scope_ref, external_id, display_name, source_revision)
		VALUES ($1, 'dept-a', 'Synthetic Department A', 2), ($1, 'dept-b', 'Synthetic Department B', 3),
			($1, 'dept-unresolved', 'Synthetic Department Without Binding', 1)`, transferScope)
	run(`INSERT INTO directory_mock_department_bindings (scope_ref, external_id, org_id)
		VALUES ($1, 'dept-a', $2), ($1, 'dept-b', $3)`, transferScope, f.orgA, f.orgB)
	run(`INSERT INTO directory_mock_employee_bindings (scope_ref, external_id, identity_generation, user_id, source_revision)
		VALUES ($1, 'employee-transfer', 1, $2, 11)`, transferScope, f.user)
	return f
}

func transferBusinessSnapshot(t *testing.T, db *pgxpool.Pool) string {
	t.Helper()
	var state string
	err := db.QueryRow(t.Context(), `SELECT jsonb_build_object(
		'users', (SELECT jsonb_agg(u ORDER BY u.id) FROM users u),
		'organizations', (SELECT jsonb_agg(o ORDER BY o.id) FROM organizations o),
		'memberships', (SELECT jsonb_agg(m ORDER BY m.user_id, m.org_id) FROM user_orgs m),
		'roles', (SELECT jsonb_agg(r ORDER BY r.id) FROM roles r),
		'user_roles', (SELECT jsonb_agg(r ORDER BY r.user_id, r.role_id) FROM user_roles r),
		'org_roles', (SELECT jsonb_agg(r ORDER BY r.org_id, r.role_id) FROM org_roles r),
		'scope', (SELECT to_jsonb(s) FROM directory_mock_scopes s WHERE scope_ref = $1),
		'departments', (SELECT jsonb_agg(d ORDER BY d.external_id) FROM directory_mock_departments d WHERE scope_ref = $1),
		'dept_bindings', (SELECT jsonb_agg(d ORDER BY d.external_id) FROM directory_mock_department_bindings d WHERE scope_ref = $1),
		'employee_bindings', (SELECT jsonb_agg(e ORDER BY e.external_id, e.identity_generation)
			FROM directory_mock_employee_bindings e WHERE scope_ref = $1),
		'batch_runs', (SELECT jsonb_agg(r ORDER BY r.batch_id) FROM directory_mock_runs r WHERE scope_ref = $1)
	)::text`, transferScope).Scan(&state)
	if err != nil {
		t.Fatal(err)
	}
	return state
}

func transferLocalSnapshot(t *testing.T, db *pgxpool.Pool, f transferFixture) string {
	t.Helper()
	var state string
	err := db.QueryRow(t.Context(), `SELECT jsonb_build_object(
		'appointment', (SELECT to_jsonb(m) FROM user_orgs m WHERE user_id = $1 AND org_id = $2),
		'appointment_org', (SELECT to_jsonb(o) FROM organizations o WHERE id = $2),
		'direct_grant', (SELECT to_jsonb(g) FROM user_roles g WHERE user_id = $1 AND role_id = $3),
		'assigned_role', (SELECT to_jsonb(r) FROM roles r WHERE id = $3)
	)::text`, f.user, f.localOrg, f.localRole).Scan(&state)
	if err != nil {
		t.Fatal(err)
	}
	return state
}

func assertSourceRevision(t *testing.T, db *pgxpool.Pool, want int64) {
	t.Helper()
	var revision int64
	err := db.QueryRow(t.Context(), `SELECT source_revision FROM directory_mock_employee_bindings
		WHERE scope_ref = $1 AND external_id = 'employee-transfer' AND identity_generation = 1`,
		transferScope).Scan(&revision)
	if err != nil || revision != want {
		t.Fatalf("employee source revision = %d, want %d: %v", revision, want, err)
	}
}

func assertBranchAccess(t *testing.T, orgs *repository.OrgRepo, org, user int64, want bool) {
	t.Helper()
	got, err := orgs.IsInOrgBranch(t.Context(), org, user)
	if err != nil || got != want {
		t.Fatalf("current branch membership of user %d in org %d = %t, want %t: %v", user, org, got, want, err)
	}
}

func assertLocalEffectiveRole(t *testing.T, db *pgxpool.Pool, user int64) {
	t.Helper()
	roles, err := repository.NewRoleRepo(db).GetEffectiveRoleCodes(t.Context(), user)
	if err != nil || !slices.Equal(roles, []string{"synthetic_direct_role"}) {
		t.Fatalf("effective role codes = %v, want [synthetic_direct_role]: %v", roles, err)
	}
}
