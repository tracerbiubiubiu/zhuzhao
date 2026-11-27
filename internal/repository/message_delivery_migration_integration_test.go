//go:build integration

package repository_test

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

func TestReviewNoticeMigrationOwnsOnlyItsSeed(t *testing.T) {
	const code = "ticket_review_http_mock"
	ctx := context.Background()
	up, err := os.ReadFile("../../migrations/000102_review_notification_delivery.up.sql")
	require.NoError(t, err)
	down, err := os.ReadFile("../../migrations/000102_review_notification_delivery.down.sql")
	require.NoError(t, err)
	delivery := repository.NewMessageDeliveryRepo(testPool)

	exec := func(t *testing.T, tx pgx.Tx, sql string, args ...any) {
		t.Helper()
		_, err := tx.Exec(ctx, sql, args...)
		require.NoError(t, err)
	}
	newMigrationTx := func(t *testing.T) pgx.Tx {
		t.Helper()
		tx, err := testPool.Begin(ctx)
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, tx.Rollback(ctx)) })
		exec(t, tx, string(down))
		return tx
	}
	insertUserTemplate := func(t *testing.T, tx pgx.Tx) int64 {
		t.Helper()
		var id int64
		require.NoError(t, tx.QueryRow(ctx, `INSERT INTO message_templates
			(code, name, channel, biz_type, subject, body)
			VALUES ($1, 'Existing template', 'webhook', 'ticket', 'Existing subject', 'Existing body')
			RETURNING id`, code).Scan(&id))
		return id
	}
	assertUserTemplate := func(t *testing.T, tx pgx.Tx, wantID int64) {
		t.Helper()
		var id, version int64
		var subject, body string
		require.NoError(t, tx.QueryRow(ctx, `SELECT id, subject, body, version FROM message_templates
			WHERE code=$1`, code).Scan(&id, &subject, &body, &version))
		require.Equal(t, wantID, id)
		require.Equal(t, int64(1), version)
		require.Equal(t, "Existing subject", subject)
		require.Equal(t, "Existing body", body)
	}

	t.Run("preexisting same-code template survives and is never rendered", func(t *testing.T) {
		tx := newMigrationTx(t)
		id := insertUserTemplate(t, tx)
		exec(t, tx, string(up))
		assertUserTemplate(t, tx, id)
		_, _, _, err := delivery.ReviewTemplate(ctx, tx)
		assert.ErrorIs(t, err, pgx.ErrNoRows)
		exec(t, tx, string(down))
		assertUserTemplate(t, tx, id)
	})

	t.Run("new seed is available and reversible", func(t *testing.T) {
		tx := newMigrationTx(t)
		exec(t, tx, string(up))
		subject, body, version, err := delivery.ReviewTemplate(ctx, tx)
		require.NoError(t, err)
		require.Equal(t, "有工单待复核", subject)
		require.Equal(t, "请登录工作台查看待复核工单。", body)
		require.Equal(t, int64(1), version)
		exec(t, tx, string(down))
		var exists bool
		require.NoError(t, tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM message_templates WHERE code=$1)`, code).Scan(&exists))
		require.False(t, exists)
	})

	t.Run("editing keeps ownership and survives rollback", func(t *testing.T) {
		tx := newMigrationTx(t)
		exec(t, tx, string(up))
		exec(t, tx, `UPDATE message_templates SET body='Edited body', version=version+1 WHERE code=$1`, code)
		_, body, version, err := delivery.ReviewTemplate(ctx, tx)
		require.NoError(t, err)
		require.Equal(t, "Edited body", body)
		require.Equal(t, int64(2), version)
		exec(t, tx, string(down))
		require.NoError(t, tx.QueryRow(ctx, `SELECT body, version FROM message_templates WHERE code=$1`, code).Scan(&body, &version))
		require.Equal(t, "Edited body", body)
		require.Equal(t, int64(2), version)
	})

	t.Run("replacement does not inherit ownership and survives rollback", func(t *testing.T) {
		tx := newMigrationTx(t)
		exec(t, tx, string(up))
		exec(t, tx, `DELETE FROM message_templates WHERE code=$1`, code)
		id := insertUserTemplate(t, tx)
		_, _, _, err := delivery.ReviewTemplate(ctx, tx)
		assert.ErrorIs(t, err, pgx.ErrNoRows)
		exec(t, tx, string(down))
		assertUserTemplate(t, tx, id)
	})
}
