//go:build integration

package manageracceptance_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tracerbiubiubiu/zhuzhao/internal/testutil"
)

const managerScope = "synthetic-mgr-scope"

func TestMockDirectoryManagerCLI(t *testing.T) {
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
	cli := managerCLI{binary: binary, databaseURL: db.Config().ConnString()}

	t.Run("missing_receipt", func(t *testing.T) {
		output, stderr := cli.run(t, "", 2, "manager-receipt", managerScope, "synthetic-absent")
		if len(output) != 0 || stderr != "mock_manager_receipt_not_found" {
			t.Fatalf("lookup missing manager receipt: stdout=%q stderr=%q", output, stderr)
		}
	})

	execSQL(t, db, `INSERT INTO directory_mock_scopes (scope_ref, success_baseline, success_cursor)
		VALUES ($1, 'synthetic-baseline', 'synthetic-cursor'), ('synthetic-other-scope', 'synthetic-other', 'synthetic-other-cursor')`, managerScope)
	bindEmployee(t, db, managerScope, "synthetic-employee", "mock-directory", 2, 1)
	bindEmployee(t, db, managerScope, "synthetic-manager", "mock-directory", 4, 1)

	request := managerRequest{
		ProviderRef: "mock-directory", ScopeRef: managerScope, OperationID: "manager-pending",
		EmployeeExternalID: "synthetic-employee", IdentityGeneration: 2,
		ManagerExternalID: "synthetic-manager", ManagerIdentityGeneration: 4,
	}
	t.Run("late_registration_preserves_original_receipt", func(t *testing.T) {
		request := request
		request.ManagerExternalID = "synthetic-late-manager"
		bindEmployee(t, db, "synthetic-other-scope", request.ManagerExternalID, "mock-directory", 4, 1)
		before := businessSnapshot(t, db)
		pending := cli.submit(t, request, 3, "unresolved", "manager_not_registered")
		assertSameReceipt(t, cli.lookup(t, request.OperationID), pending)
		assertBusinessUnchanged(t, db, before)

		bindEmployee(t, db, managerScope, request.ManagerExternalID, "mock-directory", 4, 1)
		before = businessSnapshot(t, db)
		retry := request
		retry.OperationID = "manager-resolved"
		resolved := cli.submit(t, retry, 0, "resolved", "")
		if resolved.RunID == pending.RunID {
			t.Fatalf("new operation reused run ID %q", pending.RunID)
		}
		assertSameReceipt(t, cli.lookup(t, retry.OperationID), resolved)
		assertSameReceipt(t, cli.submit(t, request, 3, "unresolved", "manager_not_registered"), pending)
		assertSameReceipt(t, cli.lookup(t, request.OperationID), pending)

		changed := request
		changed.ManagerIdentityGeneration++
		output, stderr := cli.run(t, changed.encode(t), 2, "resolve-manager")
		if len(output) != 0 || stderr != "mock_manager_content_conflict" {
			t.Fatalf("manager content conflict: stdout=%q stderr=%q", output, stderr)
		}
		assertSameReceipt(t, cli.lookup(t, request.OperationID), pending)
		assertBusinessUnchanged(t, db, before)
	})

	t.Run("invalid_and_self_links", func(t *testing.T) {
		bindEmployee(t, db, managerScope, "synthetic-inactive-manager", "mock-directory", 4, 0)
		bindEmployee(t, db, managerScope, "synthetic-local-manager", "local", 4, 1)
		bindEmployee(t, db, managerScope, "synthetic-deleted-manager", "mock-directory", 4, 1)
		bindEmployee(t, db, managerScope, "synthetic-other-tenant-manager", "mock-directory", 4, 1)
		bindEmployee(t, db, managerScope, "synthetic-system-manager", "mock-directory", 4, 1)
		execSQL(t, db, `UPDATE users SET deleted_at = now() WHERE external_id = 'synthetic-deleted-manager'`)
		execSQL(t, db, `UPDATE users SET tenant_id = 2 WHERE external_id = 'synthetic-other-tenant-manager'`)
		execSQL(t, db, `UPDATE users SET is_system = true WHERE external_id = 'synthetic-system-manager'`)
		cases := []struct {
			name       string
			externalID string
			generation int64
			problem    string
		}{
			{"inactive", "synthetic-inactive-manager", 4, "manager_link_invalid"},
			{"local", "synthetic-local-manager", 4, "manager_link_invalid"},
			{"deleted", "synthetic-deleted-manager", 4, "manager_link_invalid"},
			{"other_tenant", "synthetic-other-tenant-manager", 4, "manager_link_invalid"},
			{"system", "synthetic-system-manager", 4, "manager_link_invalid"},
			{"wrong_generation", "synthetic-manager", 5, "manager_link_invalid"},
			{"self", request.EmployeeExternalID, request.IdentityGeneration, "manager_self_reference"},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				input := request
				input.OperationID = "manager-" + strings.ReplaceAll(tc.name, "_", "-")
				input.ManagerExternalID = tc.externalID
				input.ManagerIdentityGeneration = tc.generation
				before := businessSnapshot(t, db)
				rejected := cli.submit(t, input, 3, "rejected", tc.problem)
				assertSameReceipt(t, cli.lookup(t, input.OperationID), rejected)
				assertBusinessUnchanged(t, db, before)
			})
		}
	})

	t.Run("concurrent_same_operation_creates_one_receipt", func(t *testing.T) {
		input := request
		input.OperationID = "manager-concurrent"
		payload := input.encode(t)
		before := businessSnapshot(t, db)

		var outputs [2][]byte
		if !t.Run("processes", func(t *testing.T) {
			for i, name := range []string{"first", "second"} {
				t.Run(name, func(t *testing.T) {
					t.Parallel()
					output, stderr := cli.run(t, payload, 0, "resolve-manager")
					if receipt := decodeReceipt(t, output, stderr); receipt.Status != "resolved" {
						t.Fatalf("concurrent receipt status = %q", receipt.Status)
					}
					outputs[i] = output
				})
			}
		}) {
			return
		}
		fresh, stderr := cli.run(t, "", 0, "manager-receipt", managerScope, input.OperationID)
		if !bytes.Equal(outputs[0], outputs[1]) || !bytes.Equal(outputs[0], fresh) || stderr != "" {
			t.Fatalf("concurrent replays or fresh lookup diverged: first=%q second=%q receipt=%q stderr=%q",
				outputs[0], outputs[1], fresh, stderr)
		}
		var rows int
		err := db.QueryRow(t.Context(), `SELECT count(*) FROM directory_mock_transfer_runs
			WHERE scope_ref = $1 AND operation_id = 'manager/' || $2`, managerScope, input.OperationID).Scan(&rows)
		if err != nil || rows != 1 {
			t.Fatalf("concurrent operation receipt count: rows=%d err=%v", rows, err)
		}
		assertBusinessUnchanged(t, db, before)
	})

	t.Run("same_public_operation_as_transfer", func(t *testing.T) {
		const operationID = "manager-and-transfer"
		const transferRunID = "fc05120b-946d-49de-b477-949fbb6e78b0"
		// Preserve the canonical JSON order previously used by PrimaryTransfer.
		const legacyTransfer = `{"provider_ref":"mock-directory","scope_ref":"synthetic-mgr-scope","operation_id":"manager-and-transfer","employee_external_id":"synthetic-employee","identity_generation":2,"from_department_external_id":"synthetic-from","to_department_external_id":"synthetic-to","source_revision":11}`
		hash := sha256.Sum256([]byte(legacyTransfer))
		execSQL(t, db, `INSERT INTO directory_mock_transfer_runs (scope_ref, operation_id, run_id, input_hash, receipt)
			VALUES ($1, $2, $3::text::uuid, $4, jsonb_build_object(
				'run_id', $3::text, 'scope_ref', $1::text, 'operation_id', $2::text, 'status', 'held',
				'source_revision', 11, 'problem', 'target_department_unresolved'))`,
			managerScope, operationID, transferRunID, hex.EncodeToString(hash[:]))
		beforeTransfer, stderr := cli.run(t, "", 0, "transfer-receipt", managerScope, operationID)
		if len(beforeTransfer) == 0 || stderr != "" {
			t.Fatalf("lookup existing transfer receipt: stdout=%q stderr=%q", beforeTransfer, stderr)
		}
		input := request
		input.OperationID = operationID
		resolved := cli.submit(t, input, 0, "resolved", "")
		assertSameReceipt(t, cli.lookup(t, operationID), resolved)
		afterTransfer, stderr := cli.run(t, "", 0, "transfer-receipt", managerScope, operationID)
		if !bytes.Equal(beforeTransfer, afterTransfer) || stderr != "" {
			t.Fatalf("transfer receipt changed: before=%q after=%q stderr=%q", beforeTransfer, afterTransfer, stderr)
		}
		replayedTransfer, stderr := cli.run(t, legacyTransfer, 3, "transfer-primary")
		if !bytes.Equal(beforeTransfer, replayedTransfer) || stderr != "" {
			t.Fatalf("legacy transfer replay changed: before=%q replay=%q stderr=%q", beforeTransfer, replayedTransfer, stderr)
		}
		var rows int
		err := db.QueryRow(t.Context(), `SELECT count(*) FROM directory_mock_transfer_runs
			WHERE scope_ref = $1 AND operation_id IN ($2, 'manager/' || $2)`, managerScope, operationID).Scan(&rows)
		if err != nil || rows != 2 {
			t.Fatalf("isolated manager and transfer receipts: rows=%d err=%v", rows, err)
		}
	})
}

type managerCLI struct {
	binary      string
	databaseURL string
}

func (cli managerCLI) run(t *testing.T, input string, wantExit int, args ...string) ([]byte, string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, cli.binary, args...)
	cmd.Env = append(cmd.Environ(), "D06_MOCK_DATABASE_URL="+cli.databaseURL)
	cmd.Stdin = strings.NewReader(input)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	exit := 0
	if err != nil {
		var commandErr *exec.ExitError
		if !errors.As(err, &commandErr) {
			t.Fatalf("run %v: %v", args, err)
		}
		exit = commandErr.ExitCode()
	}
	stderrText := strings.TrimSpace(stderr.String())
	if exit != wantExit {
		t.Fatalf("run %v exit=%d want=%d stdout=%q stderr=%q", args, exit, wantExit, output, stderrText)
	}
	return output, stderrText
}

type managerRequest struct {
	ProviderRef               string `json:"provider_ref"`
	ScopeRef                  string `json:"scope_ref"`
	OperationID               string `json:"operation_id"`
	EmployeeExternalID        string `json:"employee_external_id"`
	IdentityGeneration        int64  `json:"identity_generation"`
	ManagerExternalID         string `json:"manager_external_id"`
	ManagerIdentityGeneration int64  `json:"manager_identity_generation"`
}

func (request managerRequest) encode(t *testing.T) string {
	t.Helper()
	data, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

type managerReceipt struct {
	RunID                     string `json:"run_id"`
	ScopeRef                  string `json:"scope_ref"`
	OperationID               string `json:"operation_id"`
	Status                    string `json:"status"`
	EmployeeExternalID        string `json:"employee_external_id"`
	IdentityGeneration        int64  `json:"identity_generation"`
	ManagerExternalID         string `json:"manager_external_id"`
	ManagerIdentityGeneration int64  `json:"manager_identity_generation"`
	Problem                   string `json:"problem"`
}

func (cli managerCLI) submit(t *testing.T, request managerRequest, exit int, status, problem string) managerReceipt {
	t.Helper()
	output, stderr := cli.run(t, request.encode(t), exit, "resolve-manager")
	receipt := decodeReceipt(t, output, stderr)
	want := managerReceipt{
		RunID: receipt.RunID, ScopeRef: request.ScopeRef, OperationID: request.OperationID, Status: status,
		EmployeeExternalID: request.EmployeeExternalID, IdentityGeneration: request.IdentityGeneration,
		ManagerExternalID: request.ManagerExternalID, ManagerIdentityGeneration: request.ManagerIdentityGeneration,
		Problem: problem,
	}
	assertSameReceipt(t, receipt, want)
	return receipt
}

func (cli managerCLI) lookup(t *testing.T, operationID string) managerReceipt {
	t.Helper()
	output, stderr := cli.run(t, "", 0, "manager-receipt", managerScope, operationID)
	return decodeReceipt(t, output, stderr)
}

func decodeReceipt(t *testing.T, output []byte, stderr string) managerReceipt {
	t.Helper()
	var receipt managerReceipt
	decoder := json.NewDecoder(bytes.NewReader(output))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&receipt); err != nil || receipt.RunID == "" || stderr != "" {
		t.Fatalf("decode manager receipt: stdout=%q stderr=%q err=%v", output, stderr, err)
	}
	return receipt
}

func assertSameReceipt(t *testing.T, got, want managerReceipt) {
	t.Helper()
	if got != want {
		t.Fatalf("manager receipt got %+v, want %+v", got, want)
	}
}

func bindEmployee(t *testing.T, db *pgxpool.Pool, scope, externalID, source string, generation int64, status int) {
	t.Helper()
	execSQL(t, db, `WITH account AS (
		INSERT INTO users (username, password, source, external_id, status)
		VALUES ($1 || '-' || $2, 'unusable', $3, $2, $4) RETURNING id
	)
	INSERT INTO directory_mock_employee_bindings (scope_ref, external_id, identity_generation, user_id, source_revision)
	SELECT $1, $2, $5, id, 11 FROM account`, scope, externalID, source, status, generation)
}

func execSQL(t *testing.T, db *pgxpool.Pool, query string, args ...any) {
	t.Helper()
	if _, err := db.Exec(t.Context(), query, args...); err != nil {
		t.Fatal(err)
	}
}

func assertBusinessUnchanged(t *testing.T, db *pgxpool.Pool, before string) {
	t.Helper()
	if businessSnapshot(t, db) != before {
		t.Fatal("manager lookup or receipt recovery mutated business state")
	}
}

func businessSnapshot(t *testing.T, db *pgxpool.Pool) string {
	t.Helper()
	var snapshot string
	err := db.QueryRow(t.Context(), `SELECT jsonb_build_object(
		'users', (SELECT jsonb_agg(u ORDER BY u.id) FROM users u),
		'organizations', (SELECT jsonb_agg(o ORDER BY o.id) FROM organizations o),
		'memberships', (SELECT jsonb_agg(m ORDER BY m.user_id, m.org_id) FROM user_orgs m),
		'roles', (SELECT jsonb_agg(r ORDER BY r.id) FROM roles r),
		'user_roles', (SELECT jsonb_agg(r ORDER BY r.user_id, r.role_id) FROM user_roles r),
		'org_roles', (SELECT jsonb_agg(r ORDER BY r.org_id, r.role_id) FROM org_roles r),
		'scopes', (SELECT jsonb_agg(s ORDER BY s.scope_ref) FROM directory_mock_scopes s),
		'departments', (SELECT jsonb_agg(d ORDER BY d.scope_ref, d.external_id) FROM directory_mock_departments d),
		'department_bindings', (SELECT jsonb_agg(d ORDER BY d.scope_ref, d.external_id) FROM directory_mock_department_bindings d),
		'employee_bindings', (SELECT jsonb_agg(e ORDER BY e.scope_ref, e.external_id) FROM directory_mock_employee_bindings e),
		'batch_runs', (SELECT jsonb_agg(r ORDER BY r.scope_ref, r.batch_id) FROM directory_mock_runs r)
	)::text`).Scan(&snapshot)
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}
