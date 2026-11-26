//go:build integration

package disableacceptance

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"github.com/tracerbiubiubiu/zhuzhao-utils/jwt"

	"github.com/tracerbiubiubiu/zhuzhao/internal/middleware"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service/directorymock"

	"github.com/tracerbiubiubiu/zhuzhao/internal/testutil"
)

func TestMockDirectoryDisableCLI(t *testing.T) {
	cli := filepath.Join(t.TempDir(), "directory-sync-mock")
	build := exec.CommandContext(t.Context(), "go", "build", "-o", cli, "../../../../cmd/directory-sync-mock")
	output, err := build.CombinedOutput()
	require.NoError(t, err, "build isolation CLI: %s", output)

	db, shutdown, err := testutil.SetupPostgresShared()
	require.NoError(t, err)
	t.Cleanup(shutdown)
	run := func(input string, args ...string) ([]byte, int) {
		t.Helper()
		cmd := exec.CommandContext(t.Context(), cli, args...)
		cmd.Env = append(os.Environ(), "D06_MOCK_DATABASE_URL="+db.Config().ConnString())
		cmd.Stdin = strings.NewReader(input)
		output, err := cmd.CombinedOutput()
		if err == nil {
			return output, 0
		}
		var exit *exec.ExitError
		require.ErrorAs(t, err, &exit)
		return output, exit.ExitCode()
	}

	t.Run("missing_receipt", func(t *testing.T) {
		output, code := run("", "employee-disable-receipt", "synthetic-scope", "absent")
		require.Equal(t, 2, code)
		require.Equal(t, "mock_disable_receipt_not_found", strings.TrimSpace(string(output)))
	})
	ctx := t.Context()
	const scope = "synthetic-disable"
	_, err = db.Exec(ctx, `INSERT INTO directory_mock_scopes(scope_ref) VALUES ($1)`, scope)
	require.NoError(t, err)
	bind := func(external, source string, system bool) int64 {
		t.Helper()
		var id int64
		require.NoError(t, db.QueryRow(ctx, `INSERT INTO users(username,password,source,is_system)
			VALUES ($1,'synthetic',$2,$3) RETURNING id`, external, source, system).Scan(&id))
		_, err := db.Exec(ctx, `INSERT INTO directory_mock_employee_bindings
			(scope_ref,external_id,identity_generation,user_id,source_revision) VALUES ($1,$2,2,$3,4)`, scope, external, id)
		require.NoError(t, err)
		return id
	}
	target := bind("employee-1", "mock-directory", false)
	local := bind("local-1", "local", false)
	seed := bind("system-1", "mock-directory", true)
	protected := bind("protected-1", "mock-directory", false)
	_, err = db.Exec(ctx, `INSERT INTO roles(code,name,priority) VALUES ('admin','synthetic admin',10)
		ON CONFLICT (code) WHERE deleted_at IS NULL DO NOTHING`)
	require.NoError(t, err)
	_, err = db.Exec(ctx, `INSERT INTO user_roles(user_id,role_id)
		SELECT $1,id FROM roles WHERE code='admin' AND deleted_at IS NULL`, protected)
	require.NoError(t, err)

	const partialBatch = `{"provider_ref":"mock-directory","domain":"departments","mode":"full",
	 "scope_ref":"synthetic-disable","batch_id":"partial-1","expected_baseline":"","success_cursor":"snapshot-1",
	 "pages":[{"cursor":"","next_cursor":"page-2","records":[{"external_id":"new-dept","display_name":"Synthetic department","source_revision":1}]},
	 {"cursor":"page-2","error":"fetch_failed"}]}`
	output, code := run(partialBatch)
	require.Equal(t, 3, code, "%s", output)
	require.Contains(t, string(output), `"status":"partial"`)
	assertAccountState(t, db, target, 1, 1, 4, false)

	event := directorymock.EmployeeDisable{ProviderRef: "mock-directory", ScopeRef: scope, OperationID: "terminate-1",
		EmployeeExternalID: "employee-1", IdentityGeneration: 2, SourceRevision: 5, Lifecycle: "terminated"}
	submit := func(event directorymock.EmployeeDisable, wantCode int) directorymock.DisableReceipt {
		t.Helper()
		raw, err := json.Marshal(event)
		require.NoError(t, err)
		output, code := run(string(raw), "disable-employee")
		require.Equal(t, wantCode, code, "%s", output)
		var receipt directorymock.DisableReceipt
		require.NoError(t, json.Unmarshal(output, &receipt), "%s", output)
		return receipt
	}
	t.Run("untrusted_or_stale_identity_preserves_account", func(t *testing.T) {
		for _, tt := range []struct {
			operation, employee, problem string
			generation, revision         int64
			userID                       int64
		}{
			{"wrong-generation", "employee-1", "employee_unresolved", 1, 5, target},
			{"late-event", "employee-1", "source_revision_not_advanced", 2, 3, target},
			{"local-binding", "local-1", "employee_unresolved", 2, 5, local},
			{"system-binding", "system-1", "employee_unresolved", 2, 5, seed},
			{"admin-binding", "protected-1", "protected_account", 2, 5, protected},
		} {
			ev := event
			ev.OperationID, ev.EmployeeExternalID, ev.IdentityGeneration, ev.SourceRevision =
				tt.operation, tt.employee, tt.generation, tt.revision
			result := submit(ev, 3)
			require.Equal(t, "held", result.Status)
			require.Equal(t, tt.problem, result.Problem)
			assertAccountState(t, db, tt.userID, 1, 1, 4, false)
		}
		ev := event
		ev.Lifecycle = "active"
		raw, err := json.Marshal(ev)
		require.NoError(t, err)
		resp, code := run(string(raw), "disable-employee")
		require.Equal(t, 2, code, "%s", resp)
		assertAccountState(t, db, target, 1, 1, 4, false)
	})

	t.Run("directory_receipt_failure_rolls_back_c05_and_lifecycle", func(t *testing.T) {
		_, err := db.Exec(ctx, `CREATE FUNCTION fail_mock_disable_receipt() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN
			IF NEW.operation_id='disable/rollback' THEN
				RAISE EXCEPTION 'synthetic directory receipt error';
			END IF;
			RETURN NEW;
		END $$;
		CREATE TRIGGER fail_mock_disable_receipt BEFORE INSERT ON directory_mock_transfer_runs
			FOR EACH ROW EXECUTE FUNCTION fail_mock_disable_receipt()`)
		require.NoError(t, err)
		t.Cleanup(func() {
			_, err := db.Exec(ctx, `DROP TRIGGER fail_mock_disable_receipt ON directory_mock_transfer_runs;
				DROP FUNCTION fail_mock_disable_receipt()`)
			require.NoError(t, err)
		})
		before := countDisableOperations(t, db, target)
		rejected := event
		rejected.OperationID = "rollback"
		payload, err := json.Marshal(rejected)
		require.NoError(t, err)
		output, code := run(string(payload), "disable-employee")
		require.Equal(t, 1, code)
		require.Equal(t, "mock_disable_unconfirmed_query_receipt", strings.TrimSpace(string(output)))
		assertAccountState(t, db, target, 1, 1, 4, false)
		require.Equal(t, before, countDisableOperations(t, db, target))
	})
	t.Run("disable_withdraws_old_access_and_recovers_original_intent", func(t *testing.T) {
		mr := miniredis.RunT(t)
		rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
		t.Cleanup(func() { _ = rdb.Close() })
		userRepo := repository.NewUserRepo(db)
		mgr := jwt.NewManager(jwt.Config{Secret: "step10-fixture-session-secret", AccessTTL: time.Minute})
		deps := service.NewSessionWorkerDeps(db, userRepo, rdb, mgr.AccessTTL())
		service.RunProjectionSweepOnce(ctx, deps)
		oldToken, _, err := mgr.GenerateAccessTokenWithSessionEpoch(target, "employee-1", false, 0, 1)
		require.NoError(t, err)
		bearerCode := func() int {
			recorder := httptest.NewRecorder()
			test, _ := gin.CreateTestContext(recorder)
			test.Request = httptest.NewRequest(http.MethodGet, "/api/v1/user/profile", nil)
			test.Request.Header.Set("Authorization", "Bearer "+oldToken)
			middleware.JWT(mgr, rdb, nil, middleware.WithAccountSecurity(accountSnapshotSource{db}))(test)
			return recorder.Code
		}
		require.Equal(t, http.StatusOK, bearerCode())

		lookup := func() directorymock.DisableReceiptView {
			t.Helper()
			result, code := run("", "employee-disable-receipt", scope, event.OperationID)
			require.Equal(t, 0, code, "%s", result)
			var view directorymock.DisableReceiptView
			require.NoError(t, json.Unmarshal(result, &view))
			return view
		}
		first := submit(event, 0)
		require.Equal(t, "disabled", first.Status)
		require.EqualValues(t, 2, first.SessionEpoch)
		assertAccountState(t, db, target, 0, 2, 5, true)
		require.Equal(t, first, lookup().DisableReceipt)
		require.Equal(t, "pending", lookup().RevocationStatus)

		mr.SetError("synthetic Redis projection outage")
		service.RunProjectionSweepOnce(ctx, deps)
		require.Equal(t, http.StatusForbidden, bearerCode())
		require.Equal(t, "pending", lookup().RevocationStatus)
		mr.SetError("")
		freshWorker := service.NewSessionWorkerDeps(db, repository.NewUserRepo(db), rdb, mgr.AccessTTL())
		service.RunProjectionSweepOnce(ctx, freshWorker)
		require.Equal(t, "completed", lookup().RevocationStatus)
		require.Equal(t, first, submit(event, 0))
		require.Equal(t, http.StatusForbidden, bearerCode())

		changed := event
		changed.SourceRevision++
		raw, err := json.Marshal(changed)
		require.NoError(t, err)
		output, code := run(string(raw), "disable-employee")
		require.Equal(t, 2, code)
		require.Equal(t, "mock_disable_content_conflict", strings.TrimSpace(string(output)))

		err = repoEnable(ctx, db, userRepo, target)
		require.ErrorIs(t, err, errcode.ErrConflict)
		assertAccountState(t, db, target, 0, 2, 5, true)
	})
}

func countDisableOperations(t *testing.T, db *pgxpool.Pool, target int64) int {
	t.Helper()
	var count int
	require.NoError(t, db.QueryRow(t.Context(), `SELECT count(*) FROM user_session_operations
		WHERE target_user_id=$1 AND namespace='mock-directory'`, target).Scan(&count))
	return count
}

func assertAccountState(t *testing.T, db *pgxpool.Pool, userID int64, wantStatus int, wantEpoch, wantRevision int64, wantTerminated bool) {
	t.Helper()
	var status int
	var epoch, revision int64
	var terminated bool
	require.NoError(t, db.QueryRow(t.Context(), `SELECT u.status,u.session_epoch,b.source_revision,b.source_terminated
		FROM users u JOIN directory_mock_employee_bindings b ON b.user_id=u.id WHERE u.id=$1`, userID).
		Scan(&status, &epoch, &revision, &terminated))
	require.Equal(t, wantStatus, status)
	require.Equal(t, wantEpoch, epoch)
	require.Equal(t, wantRevision, revision)
	require.Equal(t, wantTerminated, terminated)
}

func repoEnable(ctx context.Context, db *pgxpool.Pool, users *repository.UserRepo, target int64) error {
	return repository.WithIAMGuard(ctx, db, func(ctx context.Context) error {
		tx, err := repository.IAMDB(ctx, db).Begin(ctx)
		if err != nil {
			return err
		}
		defer func() { _ = tx.Rollback(ctx) }()
		snap, err := users.LockSecuritySnapshotTx(ctx, tx, target)
		if err != nil {
			return err
		}
		if _, _, err := users.AdvanceEpochSetStatusTx(ctx, tx, target, snap.SessionEpoch, 1); err != nil {
			return err
		}
		return tx.Commit(ctx)
	})
}

type accountSnapshotSource struct{ pool *pgxpool.Pool }

func (s accountSnapshotSource) AccountSnapshot(ctx context.Context, userID int64) (middleware.AccountSnapshot, error) {
	snap, err := repository.NewAccountSecurityLookup(s.pool).AccountSecuritySnapshot(ctx, userID)
	return middleware.AccountSnapshot{Status: snap.Status, Deleted: snap.Deleted,
		SessionEpoch: snap.SessionEpoch, ProjectionEpoch: snap.ProjectionEpoch}, err
}
