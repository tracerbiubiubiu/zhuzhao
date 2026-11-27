//go:build integration

package directorymock_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tracerbiubiubiu/zhuzhao/internal/testutil"
)

type batchReceipt struct {
	RunID          string         `json:"run_id"`
	ScopeRef       string         `json:"scope_ref"`
	BatchID        string         `json:"batch_id"`
	Status         string         `json:"status"`
	AppliedPages   int            `json:"applied_pages"`
	PageWatermark  string         `json:"page_watermark"`
	BaselineBefore string         `json:"baseline_before"`
	BaselineAfter  string         `json:"baseline_after"`
	CursorAfter    string         `json:"cursor_after"`
	Records        []recordResult `json:"records"`
	Problem        string         `json:"problem"`
}

type recordResult struct {
	ExternalID     string `json:"external_id"`
	SourceRevision int    `json:"source_revision"`
	Outcome        string `json:"outcome"`
}

func TestMockDirectoryBatchCLI(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "directory-sync-mock")
	buildCtx, cancelBuild := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancelBuild()
	build := exec.CommandContext(buildCtx, "go", "build", "-o", binary, "../../../cmd/directory-sync-mock")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build real mock directory CLI: %v\n%s", err, output)
	}

	db, terminate, err := testutil.SetupPostgresShared()
	if err != nil {
		t.Fatalf("start isolated PostgreSQL: %v", err)
	}
	t.Cleanup(terminate)
	cli := directoryCLI{binary: binary, databaseURL: db.Config().ConnString()}

	t.Run("two linked pages persist before advancing the success watermark", func(t *testing.T) {
		input := directoryBatch("scope-full", "full-1", "", "full-cursor", `[
			{"cursor":"","next_cursor":"page-2","done":false,"records":[
				{"external_id":"dept-a","display_name":"Synthetic Alpha","source_revision":1}]},
			{"cursor":"page-2","next_cursor":"","done":true,"records":[
				{"external_id":"dept-b","display_name":"Synthetic Beta","source_revision":3}]}
		]`)
		receipt, _ := cli.receipt(t, input, 0)
		assertReceipt(t, receipt, batchReceipt{
			ScopeRef: "scope-full", BatchID: "full-1", Status: "succeeded", AppliedPages: 2,
			BaselineAfter: "full-1", CursorAfter: "full-cursor",
			Records: []recordResult{
				{ExternalID: "dept-a", SourceRevision: 1, Outcome: "created"},
				{ExternalID: "dept-b", SourceRevision: 3, Outcome: "created"},
			},
		})
		assertStoredReceipt(t, db, receipt)
		assertScopeWatermark(t, db, "scope-full", "full-1", "full-cursor")
		assertDepartment(t, db, "scope-full", "dept-a", "Synthetic Alpha", 1)
		assertDepartment(t, db, "scope-full", "dept-b", "Synthetic Beta", 3)
	})

	t.Run("failed second page recovers in a new batch without rewriting committed departments", func(t *testing.T) {
		seed := directoryBatch("scope-partial", "seed-1", "", "seed-cursor", `[
			{"cursor":"","next_cursor":"","done":true,"records":[
				{"external_id":"dept-a","display_name":"Synthetic Alpha","source_revision":1},
				{"external_id":"dept-absent","display_name":"Synthetic Absent","source_revision":4}]}
		]`)
		seedReceipt, seedStdout := cli.receipt(t, seed, 0)
		absentUpdatedAt := assertDepartment(t, db, "scope-partial", "dept-absent", "Synthetic Absent", 4)
		input := directoryBatch("scope-partial", "partial-1", "seed-1", "must-not-advance", `[
			{"cursor":"","next_cursor":"page-2","done":false,"records":[
				{"external_id":"dept-a","display_name":"Synthetic Alpha Revised","source_revision":2},
				{"external_id":"dept-new","display_name":"Synthetic New","source_revision":1}]},
			{"cursor":"page-2","next_cursor":"","done":false,"error":"fetch_failed","records":[]}
		]`)

		receipt, stdout := cli.receipt(t, input, 3)
		assertReceipt(t, receipt, batchReceipt{
			ScopeRef: "scope-partial", BatchID: "partial-1", Status: "partial", AppliedPages: 1,
			PageWatermark: "page-2", BaselineBefore: "seed-1", BaselineAfter: "seed-1",
			CursorAfter: "seed-cursor", Problem: "fetch_failed",
			Records: []recordResult{
				{ExternalID: "dept-a", SourceRevision: 2, Outcome: "updated"},
				{ExternalID: "dept-new", SourceRevision: 1, Outcome: "created"},
			},
		})
		assertStoredReceipt(t, db, receipt)
		assertScopeWatermark(t, db, "scope-partial", "seed-1", "seed-cursor")
		updatedAt := assertDepartment(t, db, "scope-partial", "dept-a", "Synthetic Alpha Revised", 2)
		newUpdatedAt := assertDepartment(t, db, "scope-partial", "dept-new", "Synthetic New", 1)
		assertNoDepartment(t, db, "scope-partial", "dept-recovered")
		if got := assertDepartment(t, db, "scope-partial", "dept-absent", "Synthetic Absent", 4); !got.Equal(absentUpdatedAt) {
			t.Errorf("absent department timestamp changed from %v to %v", absentUpdatedAt, got)
		}

		recovery := directoryBatch("scope-partial", "recovery-1", "seed-1", "recovery-cursor", `[
			{"cursor":"","next_cursor":"page-2","done":false,"records":[
				{"external_id":"dept-a","display_name":"Synthetic Alpha Revised","source_revision":2},
				{"external_id":"dept-new","display_name":"Synthetic New","source_revision":1}]},
			{"cursor":"page-2","next_cursor":"","done":true,"records":[
				{"external_id":"dept-recovered","display_name":"Synthetic Recovered","source_revision":7}]}
		]`)
		recovered, recoveryStdout := cli.receipt(t, recovery, 0)
		assertReceipt(t, recovered, batchReceipt{
			ScopeRef: "scope-partial", BatchID: "recovery-1", Status: "succeeded", AppliedPages: 2,
			BaselineBefore: "seed-1", BaselineAfter: "recovery-1", CursorAfter: "recovery-cursor",
			Records: []recordResult{
				{ExternalID: "dept-a", SourceRevision: 2, Outcome: "unchanged"},
				{ExternalID: "dept-new", SourceRevision: 1, Outcome: "unchanged"},
				{ExternalID: "dept-recovered", SourceRevision: 7, Outcome: "created"},
			},
		})
		if recovered.RunID == receipt.RunID {
			t.Fatal("completed recovery must have a separate run ID from the partial batch")
		}
		assertStoredReceipt(t, db, recovered)
		assertStoredReceipt(t, db, receipt)
		assertScopeWatermark(t, db, "scope-partial", "recovery-1", "recovery-cursor")
		assertDepartment(t, db, "scope-partial", "dept-recovered", "Synthetic Recovered", 7)
		if got := assertDepartment(t, db, "scope-partial", "dept-a", "Synthetic Alpha Revised", 2); !got.Equal(updatedAt) {
			t.Errorf("recovery rewrote the previously updated department: %v became %v", updatedAt, got)
		}
		if got := assertDepartment(t, db, "scope-partial", "dept-new", "Synthetic New", 1); !got.Equal(newUpdatedAt) {
			t.Errorf("recovery rewrote the previously created department: %v became %v", newUpdatedAt, got)
		}
		if got := assertDepartment(t, db, "scope-partial", "dept-absent", "Synthetic Absent", 4); !got.Equal(absentUpdatedAt) {
			t.Errorf("recovery rewrote the absent department: %v became %v", absentUpdatedAt, got)
		}

		beforeReplay := snapshotDirectory(t, db)
		replayedRecovery, replayRecoveryStdout := cli.receipt(t, recovery, 0)
		assertReceipt(t, replayedRecovery, recovered)
		if !bytes.Equal(recoveryStdout, replayRecoveryStdout) {
			t.Error("recovery replay stdout differs from the original receipt")
		}
		replayed, replayStdout := cli.receipt(t, input, 3)
		assertReceipt(t, replayed, receipt)
		if !bytes.Equal(stdout, replayStdout) {
			t.Error("partial replay stdout differs from the original receipt")
		}
		replayedSeed, replaySeedStdout := cli.receipt(t, "\n  "+seed+"  \n", 0)
		assertReceipt(t, replayedSeed, seedReceipt)
		if !bytes.Equal(seedStdout, replaySeedStdout) {
			t.Error("successful replay stdout differs from the original receipt")
		}
		assertDirectoryUnchanged(t, db, beforeReplay)
	})

	t.Run("same batch ID with changed content is rejected before any write", func(t *testing.T) {
		input := directoryBatch("scope-conflict", "same-1", "", "original-cursor", `[
			{"cursor":"","next_cursor":"","done":true,"records":[
				{"external_id":"dept-a","display_name":"Synthetic Alpha","source_revision":1}]}
		]`)
		original, _ := cli.receipt(t, input, 0)
		before := snapshotDirectory(t, db)

		changed := strings.Replace(input, "Synthetic Alpha", "Conflicting Synthetic Name", 1)
		stdout, stderr := cli.run(t, changed, 2)
		if len(bytes.TrimSpace(stdout)) != 0 || strings.TrimSpace(stderr) != "batch_content_conflict" {
			t.Fatalf("expected only the safe batch_content_conflict diagnostic, got stdout %q and stderr %q", stdout, stderr)
		}
		assertDirectoryUnchanged(t, db, before)
		assertStoredReceipt(t, db, original)
	})

	t.Run("a revision conflict rolls back only its page", func(t *testing.T) {
		seed := directoryBatch("scope-rollback", "seed-1", "", "seed-cursor", `[
			{"cursor":"","next_cursor":"","done":true,"records":[
				{"external_id":"dept-conflict","display_name":"Synthetic Original","source_revision":5}]}
		]`)
		cli.receipt(t, seed, 0)
		originalTimestamp := assertDepartment(t, db, "scope-rollback", "dept-conflict", "Synthetic Original", 5)
		input := directoryBatch("scope-rollback", "rollback-1", "seed-1", "must-not-advance", `[
			{"cursor":"","next_cursor":"page-2","done":false,"records":[
				{"external_id":"dept-retained","display_name":"Synthetic Retained","source_revision":1}]},
			{"cursor":"page-2","next_cursor":"","done":true,"records":[
				{"external_id":"dept-rolled-back","display_name":"Synthetic Rolled Back","source_revision":1},
				{"external_id":"dept-conflict","display_name":"Synthetic Conflict","source_revision":5}]}
		]`)
		receipt, _ := cli.receipt(t, input, 3)
		assertReceipt(t, receipt, batchReceipt{
			ScopeRef: "scope-rollback", BatchID: "rollback-1", Status: "partial", AppliedPages: 1,
			PageWatermark: "page-2", BaselineBefore: "seed-1", BaselineAfter: "seed-1",
			CursorAfter: "seed-cursor", Problem: "source_revision_conflict",
			Records: []recordResult{{ExternalID: "dept-retained", SourceRevision: 1, Outcome: "created"}},
		})
		assertStoredReceipt(t, db, receipt)
		assertScopeWatermark(t, db, "scope-rollback", "seed-1", "seed-cursor")
		assertDepartment(t, db, "scope-rollback", "dept-retained", "Synthetic Retained", 1)
		assertNoDepartment(t, db, "scope-rollback", "dept-rolled-back")
		if got := assertDepartment(t, db, "scope-rollback", "dept-conflict", "Synthetic Original", 5); !got.Equal(originalTimestamp) {
			t.Errorf("conflicting department timestamp changed from %v to %v", originalTimestamp, got)
		}
	})

	t.Run("only an explicit final page advances the success watermark", func(t *testing.T) {
		for _, tc := range []struct {
			name    string
			pages   string
			status  string
			problem string
			applied int
		}{
			{
				name: "missing-final-page", status: "partial", problem: "page_missing", applied: 1,
				pages: `[{"cursor":"","next_cursor":"page-2","records":[]}]`,
			},
			{
				name: "page-after-terminal", status: "failed", problem: "invalid_page",
				pages: `[{"cursor":"","next_cursor":"","done":true,"records":[]},
					{"cursor":"","next_cursor":"","done":true,"records":[]}]`,
			},
			{
				name: "cursor-cycle", status: "partial", problem: "invalid_page", applied: 1,
				pages: `[{"cursor":"","next_cursor":"page-2","done":false,"records":[]},
					{"cursor":"page-2","next_cursor":"page-2","done":false,"records":[
						{"external_id":"dept-invalid","display_name":"Synthetic Invalid","source_revision":1}]}]`,
			},
		} {
			t.Run(tc.name, func(t *testing.T) {
				input := directoryBatch(tc.name, "malformed-1", "", "must-not-advance", tc.pages)
				receipt, _ := cli.receipt(t, input, 3)
				if receipt.Status != tc.status || receipt.Problem != tc.problem || receipt.AppliedPages != tc.applied {
					t.Fatalf("unexpected malformed batch receipt: %+v", receipt)
				}
				if receipt.BaselineAfter != "" || receipt.CursorAfter != "" || len(receipt.Records) != 0 {
					t.Fatalf("malformed batch advanced the watermark or reported unapplied records: %+v", receipt)
				}
				assertStoredReceipt(t, db, receipt)
				assertScopeWatermark(t, db, tc.name, "", "")
				assertNoDepartment(t, db, tc.name, "dept-invalid")
			})
		}
	})

	t.Run("simultaneous submissions share one receipt and competing scopes stay independent", func(t *testing.T) {
		otherScope := directoryBatch("scope-independent", "race-1", "", "other-cursor", `[
			{"cursor":"","next_cursor":"","done":true,"records":[
				{"external_id":"dept-a","display_name":"Synthetic Independent","source_revision":1}]}
		]`)
		cli.receipt(t, otherScope, 0)
		input := directoryBatch("scope-concurrent", "race-1", "", "race-cursor", `[
			{"cursor":"","next_cursor":"","done":true,"records":[
				{"external_id":"dept-a","display_name":"Synthetic Race","source_revision":8}]}
		]`)
		stdout := cli.runPair(t, db, input)
		if !bytes.Equal(stdout[0], stdout[1]) {
			t.Fatalf("concurrent submissions returned different receipts: %q and %q", stdout[0], stdout[1])
		}
		receipt := decodeReceipt(t, stdout[0])
		assertReceipt(t, receipt, batchReceipt{
			ScopeRef: "scope-concurrent", BatchID: "race-1", Status: "succeeded", AppliedPages: 1,
			BaselineAfter: "race-1", CursorAfter: "race-cursor",
			Records: []recordResult{{ExternalID: "dept-a", SourceRevision: 8, Outcome: "created"}},
		})
		var runs, departments int
		if err := db.QueryRow(t.Context(), `SELECT
			(SELECT count(*) FROM directory_mock_runs WHERE scope_ref = 'scope-concurrent'),
			(SELECT count(*) FROM directory_mock_departments WHERE scope_ref = 'scope-concurrent')`).Scan(&runs, &departments); err != nil {
			t.Fatal(err)
		}
		if runs != 1 || departments != 1 {
			t.Fatalf("concurrent scope has %d run rows and %d department rows, want one each", runs, departments)
		}
		assertStoredReceipt(t, db, receipt)
		assertScopeWatermark(t, db, "scope-concurrent", "race-1", "race-cursor")
		assertDepartment(t, db, "scope-concurrent", "dept-a", "Synthetic Race", 8)
		// In this mock, an external department ID is scoped to its synthetic directory projection.
		assertDepartment(t, db, "scope-independent", "dept-a", "Synthetic Independent", 1)
		assertScopeWatermark(t, db, "scope-independent", "race-1", "other-cursor")
	})

	t.Run("database write failure leaves no applied pages or receipt", func(t *testing.T) {
		_, err := db.Exec(t.Context(), `ALTER TABLE directory_mock_departments
			ADD CONSTRAINT d06_test_unavailable CHECK (external_id <> 'dept-db-failure')`)
		if err != nil {
			t.Fatal(err)
		}
		dropFailure := func() {
			if _, err := db.Exec(context.Background(), `ALTER TABLE directory_mock_departments DROP CONSTRAINT IF EXISTS d06_test_unavailable`); err != nil {
				t.Error(err)
			}
		}
		t.Cleanup(dropFailure)
		input := directoryBatch("scope-database-error", "recover-1", "", "checkpoint", `[
			{"cursor":"","next_cursor":"page-2","records":[
				{"external_id":"dept-first","display_name":"Synthetic First","source_revision":1}]},
			{"cursor":"page-2","done":true,"records":[
				{"external_id":"dept-db-failure","display_name":"Synthetic Database Failure","source_revision":1}]}
		]`)
		before := snapshotDirectory(t, db)
		stdout, stderr := cli.run(t, input, 1)
		if len(stdout) != 0 || strings.TrimSpace(stderr) != "mock_directory_apply_unconfirmed_retry_same_batch" {
			t.Fatalf("unexpected database failure: stdout=%q stderr=%q", stdout, stderr)
		}
		assertDirectoryUnchanged(t, db, before)
		dropFailure()
		receipt, _ := cli.receipt(t, input, 0)
		assertStoredReceipt(t, db, receipt)
		assertDepartment(t, db, "scope-database-error", "dept-first", "Synthetic First", 1)
	})

	t.Run("closed stdout still leaves a replayable committed receipt", func(t *testing.T) {
		input := directoryBatch("scope-broken-pipe", "pipe-1", "", "checkpoint", `[
			{"cursor":"","done":true,"records":[
				{"external_id":"dept-pipe","display_name":"Synthetic Pipe","source_revision":1}]}
		]`)
		reader, writer, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = writer.Close() }()
		if err := reader.Close(); err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, cli.binary)
		cmd.Env = append(os.Environ(), "D06_MOCK_DATABASE_URL="+cli.databaseURL)
		cmd.Stdin, cmd.Stdout = strings.NewReader(input), writer
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		err = cmd.Run()
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 ||
			strings.TrimSpace(stderr.String()) != "mock_directory_receipt_output_failed_retry_same_batch" {
			t.Fatalf("closed stdout: exit=%v stderr=%q", err, stderr.String())
		}

		before := snapshotDirectory(t, db)
		receipt, _ := cli.receipt(t, input, 0)
		assertStoredReceipt(t, db, receipt)
		assertDepartment(t, db, "scope-broken-pipe", "dept-pipe", "Synthetic Pipe", 1)
		assertDirectoryUnchanged(t, db, before)
	})

}

func directoryBatch(scope, batch, baseline, cursor, pages string) string {
	raw, err := json.Marshal(map[string]any{
		"provider_ref": "mock-directory", "domain": "departments", "mode": "full",
		"scope_ref": scope, "batch_id": batch, "expected_baseline": baseline,
		"success_cursor": cursor, "pages": json.RawMessage(pages),
	})
	if err != nil {
		panic(err)
	}
	return string(raw)
}

type directoryCLI struct {
	binary      string
	databaseURL string
}

func (cli directoryCLI) run(t *testing.T, input string, wantExit int) ([]byte, string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	cmd := cli.command(ctx, input)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	stdout, err := cmd.Output()
	exitCode := 0
	if err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			t.Fatalf("start mock directory CLI: %v", err)
		}
		exitCode = exitErr.ExitCode()
	}
	if exitCode != wantExit {
		t.Fatalf("mock directory CLI exit = %d, want %d, stdout=%q, stderr=%q", exitCode, wantExit, stdout, stderr.String())
	}
	return stdout, stderr.String()
}

func (cli directoryCLI) command(ctx context.Context, input string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, cli.binary)
	cmd.Env = append(os.Environ(), "D06_MOCK_DATABASE_URL="+cli.databaseURL)
	cmd.Stdin = strings.NewReader(input)
	return cmd
}

func (cli directoryCLI) runPair(t *testing.T, db *pgxpool.Pool, input string) [2][]byte {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	gate, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = gate.Rollback(context.Background()) }()
	// Queue both writers at PostgreSQL before releasing them to exercise overlap.
	if _, err := gate.Exec(ctx, `LOCK TABLE directory_mock_scopes IN SHARE MODE`); err != nil {
		t.Fatal(err)
	}
	var commands [2]*exec.Cmd
	var output, diagnostics [2]bytes.Buffer
	defer func() {
		cancel()
		for _, cmd := range commands {
			if cmd != nil {
				_ = cmd.Wait()
			}
		}
	}()
	for i := range commands {
		cmd := cli.command(ctx, input)
		cmd.Stdout = &output[i]
		cmd.Stderr = &diagnostics[i]
		if err := cmd.Start(); err != nil {
			t.Fatalf("start concurrent CLI %d: %v", i, err)
		}
		commands[i] = cmd
	}
	awaitQueuedWriters(t, ctx, db)
	if err := gate.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	for i, cmd := range commands {
		commands[i] = nil
		if err := cmd.Wait(); err != nil || diagnostics[i].Len() != 0 {
			t.Errorf("concurrent CLI %d: exit %v, stdout=%q, stderr=%q", i, err, output[i].Bytes(), diagnostics[i].Bytes())
		}
	}
	if t.Failed() {
		t.FailNow()
	}
	return [2][]byte{output[0].Bytes(), output[1].Bytes()}
}

func awaitQueuedWriters(t *testing.T, ctx context.Context, db *pgxpool.Pool) {
	t.Helper()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		var waiting int
		if err := db.QueryRow(ctx, `SELECT count(*) FROM pg_locks
			WHERE relation = 'directory_mock_scopes'::regclass AND NOT granted`).Scan(&waiting); err != nil {
			t.Fatalf("check concurrent directory writers: %v", err)
		}
		if waiting == 2 {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatalf("expected both CLI writers waiting at the database, observed %d", waiting)
		case <-ticker.C:
		}
	}
}

func (cli directoryCLI) receipt(t *testing.T, input string, wantExit int) (batchReceipt, []byte) {
	t.Helper()
	stdout, stderr := cli.run(t, input, wantExit)
	if stderr != "" {
		t.Fatalf("receipt-producing CLI wrote to stderr: %q", stderr)
	}
	return decodeReceipt(t, stdout), stdout
}

func decodeReceipt(t *testing.T, raw []byte) batchReceipt {
	t.Helper()
	var receipt batchReceipt
	if err := json.Unmarshal(raw, &receipt); err != nil {
		t.Fatalf("decode mock directory receipt: %v", err)
	}
	return receipt
}

func assertReceipt(t *testing.T, got, want batchReceipt) {
	t.Helper()
	if got.RunID == "" {
		t.Fatal("receipt is missing run_id")
	}
	if want.RunID == "" {
		want.RunID = got.RunID
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("receipt = %+v, want %+v", got, want)
	}
}

func assertStoredReceipt(t *testing.T, db *pgxpool.Pool, want batchReceipt) {
	t.Helper()
	var runID, inputHash string
	var raw []byte
	err := db.QueryRow(t.Context(), `
		SELECT run_id, input_hash, receipt FROM directory_mock_runs
		WHERE scope_ref = $1 AND batch_id = $2`, want.ScopeRef, want.BatchID).Scan(&runID, &inputHash, &raw)
	if err != nil {
		t.Fatalf("read persisted receipt: %v", err)
	}
	if runID != want.RunID || inputHash == "" {
		t.Fatalf("stored run ID = %q, want %q, hash present = %t", runID, want.RunID, inputHash != "")
	}
	assertReceipt(t, decodeReceipt(t, raw), want)
}

func assertScopeWatermark(t *testing.T, db *pgxpool.Pool, scope, wantBaseline, wantCursor string) {
	t.Helper()
	var baseline, cursor string
	err := db.QueryRow(t.Context(), `
		SELECT success_baseline, success_cursor FROM directory_mock_scopes WHERE scope_ref = $1`, scope).Scan(&baseline, &cursor)
	if err != nil {
		t.Fatalf("read persisted scope watermark: %v", err)
	}
	if baseline != wantBaseline || cursor != wantCursor {
		t.Fatalf("scope %q baseline/cursor = %q/%q, want %q/%q", scope, baseline, cursor, wantBaseline, wantCursor)
	}
}

func assertDepartment(t *testing.T, db *pgxpool.Pool, scope, externalID, wantName string, wantRevision int) time.Time {
	t.Helper()
	var name string
	var revision int
	var updatedAt time.Time
	err := db.QueryRow(t.Context(), `
		SELECT display_name, source_revision, updated_at FROM directory_mock_departments
		WHERE scope_ref = $1 AND external_id = $2`, scope, externalID).Scan(&name, &revision, &updatedAt)
	if err != nil {
		t.Fatalf("read persisted department %q: %v", externalID, err)
	}
	if name != wantName || revision != wantRevision || updatedAt.IsZero() {
		t.Fatalf("department %q name/revision/timestamp = %q/%d/%v, want %q/%d/nonzero",
			externalID, name, revision, updatedAt, wantName, wantRevision)
	}
	return updatedAt
}

func assertNoDepartment(t *testing.T, db *pgxpool.Pool, scope, externalID string) {
	t.Helper()
	var exists bool
	err := db.QueryRow(t.Context(), `SELECT EXISTS (SELECT 1 FROM directory_mock_departments
		WHERE scope_ref = $1 AND external_id = $2)`, scope, externalID).Scan(&exists)
	if err != nil {
		t.Fatal(err)
	}
	if exists {
		t.Fatalf("department %q unexpectedly persisted in scope %q", externalID, scope)
	}
}

func snapshotDirectory(t *testing.T, db *pgxpool.Pool) string {
	t.Helper()
	var snapshot string
	// xmin also exposes writes that leave logical row values identical.
	err := db.QueryRow(t.Context(), `
		SELECT jsonb_build_object(
			'scopes', (SELECT jsonb_agg(to_jsonb(s) || jsonb_build_object('xmin', s.xmin::text) ORDER BY s.scope_ref)
				FROM directory_mock_scopes s),
			'departments', (SELECT jsonb_agg(to_jsonb(d) || jsonb_build_object('xmin', d.xmin::text) ORDER BY d.scope_ref, d.external_id)
				FROM directory_mock_departments d),
			'runs', (SELECT jsonb_agg(to_jsonb(r) || jsonb_build_object('xmin', r.xmin::text) ORDER BY r.scope_ref, r.batch_id)
				FROM directory_mock_runs r)
		)::text`).Scan(&snapshot)
	if err != nil {
		t.Fatalf("snapshot persisted directory state: %v", err)
	}
	return snapshot
}

func assertDirectoryUnchanged(t *testing.T, db *pgxpool.Pool, before string) {
	t.Helper()
	if after := snapshotDirectory(t, db); after != before {
		t.Fatalf("directory state changed\nbefore: %s\nafter:  %s", before, after)
	}
}
