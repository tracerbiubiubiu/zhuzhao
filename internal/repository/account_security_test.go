package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
)

func TestAccountSecurityLookup_NilDatabaseReturnsDependencyError(t *testing.T) {
	lookup := NewAccountSecurityLookup(nil)
	_, err := lookup.AccountSecuritySnapshot(context.Background(), 7)
	require.Error(t, err)
}

type accountSecurityDB struct {
	row   pgx.Row
	query string
	args  []any
}

func (d *accountSecurityDB) QueryRow(_ context.Context, query string, args ...any) pgx.Row {
	d.query, d.args = query, args
	return d.row
}

type accountSecurityRow struct {
	status           int
	deleted          bool
	epoch, projected int64
	err              error
}

func (r accountSecurityRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	if len(dest) != 4 {
		return fmt.Errorf("expected four account security facts, got %d", len(dest))
	}
	*dest[0].(*int) = r.status
	*dest[1].(*bool) = r.deleted
	*dest[2].(*int64) = r.epoch
	*dest[3].(*int64) = r.projected
	return nil
}

func TestAccountSecurityLookup_ReadsCurrentAccountAndProjectionEpoch(t *testing.T) {
	db := &accountSecurityDB{row: accountSecurityRow{status: 1, epoch: 7, projected: 6}}
	snap, err := (&AccountSecurityLookup{db: db}).AccountSecuritySnapshot(context.Background(), 42)
	require.NoError(t, err)
	require.Equal(t, AccountSecuritySnapshot{Status: 1, SessionEpoch: 7, ProjectionEpoch: 6}, snap)
	require.Contains(t, strings.Join(strings.Fields(db.query), " "), "session_epoch, session_projection_epoch")
	require.Equal(t, []any{int64(42)}, db.args)
}

func TestAccountSecurityLookup_MissingAccountIsTerminalDeleted(t *testing.T) {
	db := &accountSecurityDB{row: accountSecurityRow{err: pgx.ErrNoRows}}
	snap, err := (&AccountSecurityLookup{db: db}).AccountSecuritySnapshot(context.Background(), 42)
	require.NoError(t, err)
	require.True(t, snap.Deleted)
}

func TestAccountSecurityLookup_DatabaseFailureRemainsDependencyError(t *testing.T) {
	cause := errors.New("synthetic database unavailable")
	db := &accountSecurityDB{row: accountSecurityRow{err: cause}}
	_, err := (&AccountSecurityLookup{db: db}).AccountSecuritySnapshot(context.Background(), 42)
	require.ErrorIs(t, err, cause)
}
