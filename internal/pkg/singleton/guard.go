// Package singleton enforces the supported single-process deployment on one
// PostgreSQL database. It is a fail-stop deployment guard, not HA fencing for
// transactions already in flight during a network partition.
package singleton

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"
)

const (
	// Fixed across app configuration/host/port; changing either key bypasses the
	// deployment constraint. PostgreSQL advisory locks are scoped to one database.
	lockNamespace int32 = 1515866191
	lockInstance  int32 = 1
	ProbeInterval       = 500 * time.Millisecond
	ProbeTimeout        = time.Second
)

var ErrOccupied = errors.New("singleton: another zhuzhao instance owns this database")
var ErrLost = errors.New("singleton: database lock ownership lost; instance must stop")

type Guard struct {
	conn      *pgx.Conn
	mu        sync.Mutex
	admission sync.Mutex
	requests  sync.WaitGroup
	active    atomic.Bool
	lost      chan struct{}
	ownerCtx  context.Context
	loseOwner context.CancelFunc
	cancel    context.CancelFunc
	done      chan struct{}
	closeOnce sync.Once
}

// Acquire uses a dedicated session, never a pool lease. The caller must close
// the guard after all normal shutdown work has finished.
func Acquire(ctx context.Context, dsn string) (*Guard, error) {
	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		return nil, errors.New("singleton: invalid database configuration")
	}
	cfg.RuntimeParams["application_name"] = "zhuzhao:singleton"
	conn, err := pgx.ConnectConfig(ctx, cfg)
	if err != nil {
		return nil, errors.New("singleton: connect failed")
	}
	var held bool
	if err := conn.QueryRow(ctx, "SELECT pg_try_advisory_lock($1,$2)", lockNamespace, lockInstance).Scan(&held); err != nil || !held {
		closeCtx, cancel := context.WithTimeout(context.Background(), ProbeTimeout)
		defer cancel()
		_ = conn.Close(closeCtx)
		if err != nil {
			return nil, errors.New("singleton: acquire failed")
		}
		return nil, ErrOccupied
	}
	monitorCtx, cancel := context.WithCancel(context.Background())
	ownerCtx, loseOwner := context.WithCancel(context.Background())
	g := &Guard{conn: conn, lost: make(chan struct{}), cancel: cancel, done: make(chan struct{}), ownerCtx: ownerCtx, loseOwner: loseOwner}
	g.active.Store(true)
	go g.monitor(monitorCtx)
	return g, nil
}

func (g *Guard) monitor(ctx context.Context) {
	defer close(g.done)
	tick := time.NewTicker(ProbeInterval)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			checkCtx, cancel := context.WithTimeout(ctx, ProbeTimeout)
			g.mu.Lock()
			var held bool
			err := g.conn.QueryRow(checkCtx, `SELECT EXISTS(SELECT 1 FROM pg_locks WHERE pid=pg_backend_pid() AND locktype='advisory' AND classid=$1::oid AND objid=$2::oid AND objsubid=2 AND granted)`, lockNamespace, lockInstance).Scan(&held)
			g.mu.Unlock()
			cancel()
			if err != nil || !held {
				// Intentional Close cancels the monitor and is not an ownership incident.
				if ctx.Err() != nil {
					return
				}
				g.stopAdmission()
				g.loseOwner()
				close(g.lost)
				return
			}
		}
	}
}

func (g *Guard) Lost() <-chan struct{} { return g.lost }

// OwnerContext remains valid through graceful draining and ends when loss is
// detected or the owner deliberately releases the session.
func (g *Guard) OwnerContext() context.Context { return g.ownerCtx }

// Handler blocks readiness and every API before dispatch once loss is detected.
// BaseContext cancellation and listener shutdown remain the App's responsibility.
func (g *Guard) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		g.admission.Lock()
		if !g.active.Load() {
			g.admission.Unlock()
			http.Error(w, "instance unavailable", http.StatusServiceUnavailable)
			return
		}
		g.requests.Add(1)
		g.admission.Unlock()
		defer g.requests.Done()
		next.ServeHTTP(w, r)
	})
}

func (g *Guard) Close() {
	g.closeOnce.Do(func() {
		g.stopAdmission()
		g.cancel()
		<-g.done
		g.loseOwner()
		ctx, cancel := context.WithTimeout(context.Background(), ProbeTimeout)
		defer cancel()
		g.mu.Lock()
		defer g.mu.Unlock()
		_ = g.conn.Close(ctx) // session close releases the lock; never unlock another owner's session
	})
}

func (g *Guard) stopAdmission() { g.admission.Lock(); g.active.Store(false); g.admission.Unlock() }

// DrainRequests permanently closes HTTP admission before waiting; Add cannot
// race Wait. The App cancels request contexts and closes connections first on
// ownership loss. A handler ignoring cancellation remains a bounded failure.
func (g *Guard) DrainRequests(ctx context.Context) error {
	g.stopAdmission()
	done := make(chan struct{})
	go func() { g.requests.Wait(); close(done) }()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
