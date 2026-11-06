//go:build integration

package ticket

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/resource"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

func typeConfigService(pool *pgxpool.Pool) *Service {
	return NewTicketService(pool, repository.NewTicketRepo(pool), repository.NewOrgRepo(pool), resource.NewRegistry(), stubRoleFetcher{}, nil, nil, nil, nil)
}
func assertStoredGraph(t *testing.T, typ *model.TicketType) {
	t.Helper()
	var states []string
	var transitions map[string][]string
	require.NoError(t, json.Unmarshal(typ.States, &states))
	require.NoError(t, json.Unmarshal(typ.Transitions, &transitions))
	set := map[string]bool{}
	for _, s := range states {
		set[s] = true
	}
	for from, tos := range transitions {
		assert.True(t, set[from], "persisted from=%s missing from states", from)
		for _, to := range tos {
			assert.True(t, set[to], "persisted to=%s missing from states", to)
		}
	}
}

func TestTypeGraphPatchUsesPersistedCounterpart(t *testing.T) {
	svc := typeConfigService(testPool)
	ctx := context.Background()
	custom := `["open","assigned","in_progress","pending_verify","closed","rejected","review"]`
	for _, versioned := range []bool{false, true} {
		for i, tc := range []struct {
			name, states, trans, patchStates, patchTrans string
			success                                      bool
		}{
			{"states_cannot_remove_live_endpoint", custom, `{"open":["review"],"review":["closed"]}`, defaultStates, "", false},
			{"transitions_cannot_add_unknown_endpoint", `["open","closed"]`, `{"open":["closed"]}`, "", defaultTransitions, false},
			{"valid_custom_transition_only", custom, defaultTransitions, "", `{"open":["review"],"review":["closed"]}`, true},
			{"both_halves_valid", custom, `{"open":["review"],"review":["closed"]}`, defaultStates, defaultTransitions, true},
			{"valid_default_transition_only", defaultStates, defaultTransitions, "", `{"open":["closed"]}`, true},
		} {
			t.Run(fmt.Sprintf("%s/versioned_%t", tc.name, versioned), func(t *testing.T) {
				code := fmt.Sprintf("g_%s_%d", uniqueSuffix(), i)
				_, err := svc.CreateTicketType(ctx, &model.CreateTicketTypeRequest{Code: code, Name: code, States: []byte(tc.states), Transitions: []byte(tc.trans)})
				require.NoError(t, err)
				before, err := svc.ticketRepo.GetTicketType(ctx, code)
				require.NoError(t, err)
				t.Cleanup(func() {
					_, err := testPool.Exec(context.Background(), `DELETE FROM ticket_types WHERE code=$1`, code)
					require.NoError(t, err)
				})
				req := &model.UpdateTicketTypeRequest{}
				if tc.patchStates != "" {
					req.States = []byte(tc.patchStates)
				}
				if tc.patchTrans != "" {
					req.Transitions = []byte(tc.patchTrans)
				}
				if versioned {
					v := int(before.Version)
					req.Version = &v
				}
				_, err = svc.UpdateTicketType(ctx, code, req)
				stored, readErr := svc.ticketRepo.GetTicketType(ctx, code)
				require.NoError(t, readErr)
				assertStoredGraph(t, stored)
				if tc.success {
					require.NoError(t, err)
					assert.Equal(t, before.Version+1, stored.Version)
					wantStates, wantTrans := tc.states, tc.trans
					if tc.patchStates != "" {
						wantStates = tc.patchStates
					}
					if tc.patchTrans != "" {
						wantTrans = tc.patchTrans
					}
					assert.JSONEq(t, wantStates, string(stored.States))
					assert.JSONEq(t, wantTrans, string(stored.Transitions))
				} else {
					require.Error(t, err)
					assert.Equal(t, before.Version, stored.Version)
					assert.JSONEq(t, tc.states, string(stored.States))
					assert.JSONEq(t, tc.trans, string(stored.Transitions))
				}
				if !versioned {
					assert.Nil(t, req.Version, "service must not require or mutate the optional client version")
				}
			})
		}
	}
}

type graphWriteBarrier struct {
	code    string
	arrived atomic.Int64
	release chan struct{}
	once    sync.Once
}

func (b *graphWriteBarrier) TraceQueryStart(ctx context.Context, _ *pgx.Conn, d pgx.TraceQueryStartData) context.Context {
	if strings.Contains(d.SQL, "UPDATE ticket_types SET") && len(d.Args) > 0 && d.Args[0] == b.code {
		if b.arrived.Add(1) == 2 {
			b.once.Do(func() { close(b.release) })
		}
		select {
		case <-b.release:
		case <-ctx.Done():
		}
	}
	return ctx
}
func (*graphWriteBarrier) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

func TestTypeGraphConcurrentPartialWrites(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	svc := typeConfigService(testPool)
	code := "gc_" + uniqueSuffix()
	_, err := svc.CreateTicketType(ctx, &model.CreateTicketTypeRequest{Code: code, Name: code, States: []byte(`["open","assigned","in_progress","pending_verify","closed","rejected","review"]`), Transitions: []byte(`{"open":["closed"]}`)})
	require.NoError(t, err)
	t.Cleanup(func() {
		_, err := testPool.Exec(context.Background(), `DELETE FROM ticket_types WHERE code=$1`, code)
		require.NoError(t, err)
	})
	barrier := &graphWriteBarrier{code: code, release: make(chan struct{})}
	cfg := testPool.Config()
	cfg.MaxConns = 2
	cfg.ConnConfig.Tracer = barrier
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	require.NoError(t, err)
	defer pool.Close()
	concurrent := typeConfigService(pool)
	requests := []*model.UpdateTicketTypeRequest{{States: []byte(defaultStates)}, {Transitions: []byte(`{"open":["review"],"review":["closed"]}`)}}
	errs := make([]error, 2)
	var wg sync.WaitGroup
	for i := range requests {
		wg.Add(1)
		go func(i int) { defer wg.Done(); _, errs[i] = concurrent.UpdateTicketType(ctx, code, requests[i]) }(i)
	}
	wg.Wait()
	require.EqualValues(t, 2, barrier.arrived.Load(), "both validated partial writes must reach actual UPDATE before either executes")
	success, conflict := 0, 0
	for _, err := range errs {
		if err == nil {
			success++
		} else if errors.Is(err, errcode.ErrConcurrentModification) {
			conflict++
		} else {
			t.Errorf("unexpected write error: %v", err)
		}
	}
	assert.Equal(t, 1, success)
	assert.Equal(t, 1, conflict)
	stored, err := svc.ticketRepo.GetTicketType(ctx, code)
	require.NoError(t, err)
	assertStoredGraph(t, stored)
	assert.EqualValues(t, 2, stored.Version)
	for _, req := range requests {
		assert.Nil(t, req.Version)
	}
}
