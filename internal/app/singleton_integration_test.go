//go:build integration

package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/tracerbiubiubiu/zhuzhao/internal/config"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/audit"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/singleton"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service"
)

var singletonTestPool *pgxpool.Pool
var singletonTestDSN string

func TestMain(m *testing.M) {
	if os.Getenv("ZZ_F0_HELPER") == "1" {
		os.Exit(m.Run())
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	c, err := postgres.Run(ctx, "postgres:18.6-trixie@sha256:e3d24f6906d0760bf4da0dae872415391ededefad23fd471c47dc53b0ddf3c42", postgres.WithDatabase("f0_only"), postgres.WithUsername("f0_only"), postgres.WithPassword("synthetic-f0-password"), testcontainers.WithWaitStrategy(wait.ForLog("database system is ready to accept connections").WithOccurrence(2), wait.ForListeningPort("5432/tcp")))
	if err != nil {
		fmt.Fprintln(os.Stderr, "F0 exclusive PostgreSQL:", err)
		os.Exit(1)
	}
	singletonTestDSN, err = c.ConnectionString(ctx, "sslmode=disable")
	fmt.Fprintln(os.Stdout, "F0 exclusive PostgreSQL container:", c.GetContainerID())
	if err == nil {
		singletonTestPool, err = pgxpool.New(ctx, singletonTestDSN)
	}
	if err == nil {
		_, err = singletonTestPool.Exec(ctx, "CREATE TABLE f0_effects (id TEXT PRIMARY KEY, n INT NOT NULL DEFAULT 0)")
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "F0 fixture:", err)
		_ = c.Terminate(context.Background())
		os.Exit(1)
	}
	code := m.Run()
	singletonTestPool.Close()
	_ = c.Terminate(context.Background())
	os.Exit(code)
}

type singletonTestStore struct{ pool *pgxpool.Pool }

func (s singletonTestStore) InsertPolicyEvals(ctx context.Context, rows []audit.PolicyEvalEntry) error {
	for _, row := range rows {
		if _, err := s.pool.Exec(ctx, "INSERT INTO f0_effects(id,n) VALUES($1,1) ON CONFLICT(id) DO NOTHING", row.ResourceID); err != nil {
			return err
		}
	}
	return nil
}

// cronFunc keeps the test outside business service/repository paths.
type singletonCronFunc func(context.Context) error

func (f singletonCronFunc) Handle(ctx context.Context, _ json.RawMessage) error { return f(ctx) }

// A subprocess runs the real App.Run lifecycle, including signals and HTTP.
func TestSingletonAppProcess(t *testing.T) {
	if os.Getenv("ZZ_F0_HELPER") != "1" {
		return
	}
	pgcfg, err := pgx.ParseConfig(os.Getenv("ZZ_F0_DSN"))
	require.NoError(t, err)
	pool, err := pgxpool.New(context.Background(), os.Getenv("ZZ_F0_DSN"))
	require.NoError(t, err)
	defer pool.Close()
	id := os.Getenv("ZZ_F0_ID")
	_, err = pool.Exec(context.Background(), "INSERT INTO f0_effects(id) VALUES($1)", id)
	require.NoError(t, err)
	port, err := strconv.Atoi(os.Getenv("ZZ_F0_PORT"))
	require.NoError(t, err)
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.GET("/health/ready", func(c *gin.Context) { c.Status(http.StatusOK) })
	r.GET("/effect", func(c *gin.Context) {
		_, err := pool.Exec(c.Request.Context(), "UPDATE f0_effects SET n=n+1 WHERE id=$1", id)
		if err != nil {
			c.Status(503)
			return
		}
		c.Status(200)
	})
	r.GET("/hold", func(c *gin.Context) {
		_, err := pool.Exec(c.Request.Context(), "INSERT INTO f0_effects(id) VALUES($1)", id+"_request_entered")
		if err != nil {
			c.Status(500)
			return
		}
		<-c.Request.Context().Done()
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_, _ = pool.Exec(ctx, "INSERT INTO f0_effects(id) VALUES($1)", id+"_request_cancelled")
	})

	mr := miniredis.RunT(t)
	redis := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	defer redis.Close()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	writer := audit.NewPolicyEvalWriter(audit.PolicyEvalConfig{}, redis, singletonTestStore{pool: pool}, logger)
	registerShutdownTestRoutes(r, pool, id, writer)
	a := &App{cfg: &config.Config{Server: config.ServerConfig{Port: port}, Database: config.DatabaseConfig{Host: pgcfg.Host, Port: int(pgcfg.Port), User: pgcfg.User, Password: pgcfg.Password, DBName: pgcfg.Database, SSLMode: "disable"}}, logger: logger, router: r, policyEval: writer}
	a.auditCron = service.NewCronScheduler(pool, "f0_helper_"+id, singletonCronFunc(func(ctx context.Context) error {
		_, err := pool.Exec(ctx, "UPDATE f0_effects SET n=n+1 WHERE id=$1", id)
		return err
	}), 10*time.Millisecond, logger)
	err = a.Run()
	switch os.Getenv("ZZ_F0_EXPECT") {
	case "occupied":
		require.ErrorIs(t, err, singleton.ErrOccupied)
	case "lost":
		require.ErrorIs(t, err, singleton.ErrLost)
	case "deadline":
		require.ErrorIs(t, err, context.DeadlineExceeded)
	default:
		require.NoError(t, err)
	}
}

type singletonProcess struct {
	cmd  *exec.Cmd
	done chan struct{}
	mu   sync.Mutex
	err  error
	port int
	log  string
}

func startSingletonProcess(t *testing.T, id string) *singletonProcess {
	return startSingletonProcessExpected(t, id, "normal")
}

func startSingletonProcessExpected(t *testing.T, id, expected string) *singletonProcess {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := listener.Addr().(*net.TCPAddr).Port
	require.NoError(t, listener.Close())
	cmd := exec.Command(os.Args[0], "-test.run=^TestSingletonAppProcess$", "-test.timeout=45s")
	cmd.Env = append(os.Environ(), "ZZ_F0_HELPER=1", "ZZ_F0_DSN="+singletonTestDSN, "ZZ_F0_ID="+id, "ZZ_F0_PORT="+strconv.Itoa(port), "ZZ_F0_EXPECT="+expected)
	log, err := os.CreateTemp(t.TempDir(), "child-*.log")
	require.NoError(t, err)
	cmd.Stdout, cmd.Stderr = log, log
	p := &singletonProcess{cmd: cmd, done: make(chan struct{}), port: port, log: log.Name()}
	require.NoError(t, cmd.Start())
	go func() {
		err := cmd.Wait()
		p.mu.Lock()
		p.err = err
		p.mu.Unlock()
		_ = log.Close()
		close(p.done)
	}()
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		select {
		case <-p.done:
		case <-time.After(5 * time.Second):
			t.Error("F0 child did not exit")
		}
	})
	return p
}
func (p *singletonProcess) ready() bool {
	c := &http.Client{Timeout: 100 * time.Millisecond}
	resp, err := c.Get(fmt.Sprintf("http://127.0.0.1:%d/health/ready", p.port))
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}
func (p *singletonProcess) wait(t *testing.T) {
	p.waitWithin(t, 5*time.Second)
}

func (p *singletonProcess) waitWithin(t *testing.T, timeout time.Duration) {
	t.Helper()
	select {
	case <-p.done:
		p.mu.Lock()
		err := p.err
		p.mu.Unlock()
		if err != nil {
			childLog, _ := os.ReadFile(p.log)
			t.Logf("singleton helper log:\n%s", childLog)
		}
		require.NoError(t, err)
	case <-time.After(timeout):
		childLog, _ := os.ReadFile(p.log)
		t.Logf("singleton helper log:\n%s", childLog)
		t.Fatal("App.Run failed to stop after singleton loss or signal")
	}
}
func f0Effects(t *testing.T, id string) int {
	t.Helper()
	var n int
	require.NoError(t, singletonTestPool.QueryRow(context.Background(), "SELECT n FROM f0_effects WHERE id=$1", id).Scan(&n))
	return n
}

func TestAppSingletonRejectsReplicaBeforeHTTPAndBackgroundThenRecovers(t *testing.T) {
	for round := 0; round < 8; round++ {
		t.Run(strconv.Itoa(round), func(t *testing.T) {
			ownerID := fmt.Sprintf("owner_%d", round)
			owner := startSingletonProcessExpected(t, ownerID, "lost")
			require.Eventually(t, owner.ready, 5*time.Second, 20*time.Millisecond)
			require.Eventually(t, func() bool { return f0Effects(t, ownerID) > 0 }, time.Second, 10*time.Millisecond)
			replicaID := fmt.Sprintf("replica_%d", round)
			replica := startSingletonProcessExpected(t, replicaID, "occupied")
			// Acquire allows five seconds; the race-instrumented child also waits
			// at process exit. This budget bounds termination, while every poll
			// still forbids serving HTTP or starting any background effect.
			deadline := time.After(8 * time.Second)
		waitReplica:
			for {
				select {
				case <-replica.done:
					break waitReplica
				case <-deadline:
					childLog, _ := os.ReadFile(replica.log)
					t.Logf("rejected replica helper log:\n%s", childLog)
					t.Fatal("second App.Run was not rejected")
				default:
					if replica.ready() {
						t.Fatal("second App.Run accepted HTTP before singleton ownership")
					}
					time.Sleep(10 * time.Millisecond)
				}
			}
			replica.wait(t)
			require.Equal(t, 0, f0Effects(t, replicaID), "rejected replica must never start its scheduler")

			// An already admitted request must observe cancellation on ownership loss.
			heldDone := make(chan struct{})
			go func() {
				defer close(heldDone)
				resp, err := (&http.Client{Timeout: 5 * time.Second}).Get(fmt.Sprintf("http://127.0.0.1:%d/hold", owner.port))
				if err == nil {
					_ = resp.Body.Close()
				}
			}()
			require.Eventually(t, func() bool {
				var n int
				err := singletonTestPool.QueryRow(context.Background(), "SELECT count(*) FROM f0_effects WHERE id=$1", ownerID+"_request_entered").Scan(&n)
				return err == nil && n == 1
			}, time.Second, 10*time.Millisecond)
			// Only this private fixture's lock session is terminated; no shared DB can match.
			var terminated int
			require.NoError(t, singletonTestPool.QueryRow(context.Background(), "SELECT count(*) FROM (SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname=current_database() AND application_name='zhuzhao:singleton') s").Scan(&terminated))
			require.Equal(t, 1, terminated)
			owner.wait(t)
			select {
			case <-heldDone:
			case <-time.After(time.Second):
				t.Fatal("in-flight request did not stop on lock loss")
			}
			var cancelled int
			require.NoError(t, singletonTestPool.QueryRow(context.Background(), "SELECT count(*) FROM f0_effects WHERE id=$1", ownerID+"_request_cancelled").Scan(&cancelled))
			require.Equal(t, 1, cancelled, "HTTP request context must be cancelled on singleton loss")
			require.False(t, owner.ready(), "lost singleton must stop readiness and HTTP")
			after := f0Effects(t, ownerID)
			time.Sleep(40 * time.Millisecond)
			require.Equal(t, after, f0Effects(t, ownerID), "lost singleton must cancel and join its background scheduler")
			recovered := startSingletonProcess(t, fmt.Sprintf("recovered_%d", round))
			require.Eventually(t, recovered.ready, 5*time.Second, 20*time.Millisecond)
			require.NoError(t, recovered.cmd.Process.Signal(syscall.SIGTERM))
			recovered.wait(t)
		})
	}
}
