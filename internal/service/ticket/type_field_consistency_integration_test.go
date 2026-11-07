//go:build integration

package ticket

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

type fieldReadPauseKey struct{}
type firstFieldWritePause struct {
	code, phase      string
	arrived, release chan struct{}
	once             sync.Once
}

func (p *firstFieldWritePause) wait(ctx context.Context) {
	p.once.Do(func() { close(p.arrived) })
	select {
	case <-p.release:
	case <-ctx.Done():
	}
}
func (p *firstFieldWritePause) TraceQueryStart(ctx context.Context, _ *pgx.Conn, d pgx.TraceQueryStartData) context.Context {
	if len(d.Args) == 0 || d.Args[0] != p.code {
		return ctx
	}
	if p.phase == "before_write" && strings.Contains(d.SQL, "UPDATE ticket_types SET version = version + 1 WHERE code = $1") {
		p.wait(ctx)
	}
	if p.phase == "after_fields_read" && strings.Contains(d.SQL, "FROM ticket_type_fields WHERE type_code = $1 ORDER BY") {
		return context.WithValue(ctx, fieldReadPauseKey{}, true)
	}
	return ctx
}
func (p *firstFieldWritePause) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, _ pgx.TraceQueryEndData) {
	if ctx.Value(fieldReadPauseKey{}) == true {
		p.wait(ctx)
	}
}

func cleanupTypeConfiguration(t *testing.T, code string) {
	t.Helper()
	t.Cleanup(func() {
		ctx := context.Background()
		for _, q := range []string{`DELETE FROM tickets WHERE type_code=$1`, `DELETE FROM ticket_type_fields WHERE type_code=$1`, `DELETE FROM ticket_types WHERE code=$1`} {
			_, err := testPool.Exec(ctx, q, code)
			require.NoError(t, err)
		}
	})
}

func TestTypeFieldsFirstDefinitionCannotBypassHardLock(t *testing.T) {
	for _, phase := range []string{"before_write", "after_fields_read"} {
		for _, versionMode := range []string{"nil", "snapshot", "future"} {
			t.Run(fmt.Sprintf("%s/version_%s", phase, versionMode), func(t *testing.T) {
				svc, actor, orgID := setupTicketAdmin(t)
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				code := "first_" + uniqueSuffix()
				_, err := svc.CreateTicketType(ctx, &model.CreateTicketTypeRequest{Code: code, Name: code})
				require.NoError(t, err)
				cleanupTypeConfiguration(t, code)
				before, err := svc.ticketRepo.GetTicketType(ctx, code)
				require.NoError(t, err)
				old, err := svc.ticketRepo.ListTicketTypeFieldsAll(ctx, code)
				require.NoError(t, err)
				require.Empty(t, old)
				pause := &firstFieldWritePause{code: code, phase: phase, arrived: make(chan struct{}), release: make(chan struct{})}
				cfg := testPool.Config()
				cfg.MaxConns = 2
				cfg.ConnConfig.Tracer = pause
				blockedPool, err := pgxpool.NewWithConfig(ctx, cfg)
				require.NoError(t, err)
				defer blockedPool.Close()
				var releaseOnce sync.Once
				release := func() { releaseOnce.Do(func() { close(pause.release) }) }
				defer release()
				blocked := *svc
				blocked.db = blockedPool
				blocked.ticketRepo = repository.NewTicketRepo(blockedPool)
				req := &model.ReplaceTypeFieldsRequest{Fields: []model.TicketTypeFieldInput{{FieldKey: "payload", FieldLabel: "Payload", FieldType: "number"}}}
				if versionMode != "nil" {
					v := int(before.Version)
					if versionMode == "future" {
						v++
					}
					req.Version = &v
				}
				done := make(chan error, 1)
				go func() { _, e := blocked.ReplaceTicketTypeFields(ctx, code, req); done <- e }()
				var lateErr error
				completedEarly := false
				select {
				case <-pause.arrived:
				case lateErr = <-done:
					require.Equal(t, "future", versionMode, "fresh/nil writers must reach the forced interleaving")
					require.ErrorIs(t, lateErr, errcode.ErrConcurrentModification)
					completedEarly = true
				case <-ctx.Done():
					t.Fatal("old empty-field precheck did not reach requested barrier")
				}
				_, err = svc.ReplaceTicketTypeFields(ctx, code, &model.ReplaceTypeFieldsRequest{Fields: []model.TicketTypeFieldInput{{FieldKey: "payload", FieldLabel: "Payload", FieldType: "input"}}})
				require.NoError(t, err)
				ticket, err := svc.Create(ctx, &model.CreateTicketRequest{TypeCode: code, Title: code, Priority: 2, OrgID: orgID(), CustomData: json.RawMessage(`{"payload":"string-value"}`)}, actor)
				require.NoError(t, err, "actual ticket must commit under the winning string field")
				_, sequentialErr := svc.ReplaceTicketTypeFields(ctx, code, &model.ReplaceTypeFieldsRequest{Fields: req.Fields})
				requireErrCode(t, sequentialErr, errcode.ErrInvalidParams.Code)
				release()
				if !completedEarly {
					lateErr = <-done
				}
				assert.ErrorIs(t, lateErr, errcode.ErrConcurrentModification, "stale empty precheck must not overwrite the committed schema")
				fields, err := svc.ticketRepo.ListTicketTypeFieldsAll(ctx, code)
				require.NoError(t, err)
				require.Len(t, fields, 1)
				assert.Equal(t, "input", fields[0].FieldType)
				after, err := svc.ticketRepo.GetTicketType(ctx, code)
				require.NoError(t, err)
				assert.Equal(t, before.Version+1, after.Version)
				_, err = svc.Update(ctx, &model.UpdateTicketRequest{ID: ticket.ID, CustomData: json.RawMessage(`{"payload":"same-string"}`)}, actor)
				assert.NoError(t, err, "existing string ticket must remain editable")
				if versionMode == "nil" {
					assert.Nil(t, req.Version)
				}
			})
		}
	}
}

func TestTypeFieldsNilVersionNonConflictingWrites(t *testing.T) {
	svc, actor, orgID := setupTicketAdmin(t)
	ctx := context.Background()
	code := "nil_fields_" + uniqueSuffix()
	_, err := svc.CreateTicketType(ctx, &model.CreateTicketTypeRequest{Code: code, Name: code})
	require.NoError(t, err)
	cleanupTypeConfiguration(t, code)
	first := &model.ReplaceTypeFieldsRequest{Fields: []model.TicketTypeFieldInput{{FieldKey: "payload", FieldLabel: "Original", FieldType: "input"}}}
	_, err = svc.ReplaceTicketTypeFields(ctx, code, first)
	require.NoError(t, err)
	assert.Nil(t, first.Version)
	ticket, err := svc.Create(ctx, &model.CreateTicketRequest{TypeCode: code, Title: code, Priority: 2, OrgID: orgID(), CustomData: json.RawMessage(`{"payload":"string-value"}`)}, actor)
	require.NoError(t, err)
	second := &model.ReplaceTypeFieldsRequest{Fields: []model.TicketTypeFieldInput{{FieldKey: "payload", FieldLabel: "Renamed", FieldType: "input", SortOrder: 10}, {FieldKey: "extra", FieldLabel: "Extra", FieldType: "number"}}}
	_, err = svc.ReplaceTicketTypeFields(ctx, code, second)
	require.NoError(t, err)
	assert.Nil(t, second.Version)
	fields, err := svc.ticketRepo.ListTicketTypeFieldsAll(ctx, code)
	require.NoError(t, err)
	require.Len(t, fields, 2)
	_, err = svc.Update(ctx, &model.UpdateTicketRequest{ID: ticket.ID, CustomData: json.RawMessage(`{"payload":"still-valid","extra":42}`)}, actor)
	require.NoError(t, err)
}
