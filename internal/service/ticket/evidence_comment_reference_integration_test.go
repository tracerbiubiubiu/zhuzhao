//go:build integration

package ticket

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"

	commentref "github.com/tracerbiubiubiu/zhuzhao/internal/model/comment"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

// commentEvidenceEnv 建最小可核引用环境：一名作者、目标票与干扰票。
// 返回作者 ID 与两张票 ID（cleanup 级联删评论，用户行显式删）。
func commentEvidenceEnv(t *testing.T) (authorID, ticketID, otherTicketID int64) {
	t.Helper()
	ctx := context.Background()
	require.NoError(t, testPool.QueryRow(ctx,
		`INSERT INTO users (employee_no, username, password, status) VALUES ($1, $1, 'hash', 1) RETURNING id`,
		"ECREF"+uniqueSuffix()).Scan(&authorID))
	var orgID int64
	require.NoError(t, testPool.QueryRow(ctx, `SELECT id FROM organizations WHERE code='root' LIMIT 1`).Scan(&orgID))
	insert := func(title string) int64 {
		var id int64
		require.NoError(t, testPool.QueryRow(ctx, `
			INSERT INTO tickets (type_code, title, description, priority, status, created_by, org_id, org_path, definition_version)
			VALUES ('incident', $1, 'synthetic', 2, 'in_progress', $2, $3, 'root', 1) RETURNING id`, title, authorID, orgID).Scan(&id))
		return id
	}
	ticketID, otherTicketID = insert("evidence-ref-target"), insert("evidence-ref-other")
	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(), `DELETE FROM tickets WHERE id IN ($1,$2)`, ticketID, otherTicketID)
		_, _ = testPool.Exec(context.Background(), `DELETE FROM users WHERE id=$1`, authorID)
	})
	return authorID, ticketID, otherTicketID
}

// insertCommentRow 写一条评论并返回 ID 与 created_at（RETURNING 原值）。
func insertCommentRow(t *testing.T, ticketID, authorID int64, content string, internal bool) (int64, time.Time) {
	t.Helper()
	var id int64
	var createdAt time.Time
	require.NoError(t, testPool.QueryRow(context.Background(), `
		INSERT INTO ticket_comments (ticket_id, user_id, content, is_internal)
		VALUES ($1, $2, $3, $4) RETURNING id, created_at`,
		ticketID, authorID, content, internal).Scan(&id, &createdAt))
	return id, createdAt
}

// validateCommentRefs 在标准守卫事务里跑一次校验（锁票行后调工单域 owner 方法，
// 与 storage 校验器测试同款外壳）。
func validateCommentRefs(t *testing.T, svc *Service, ticketID int64, refs []commentref.EvidenceRef) error {
	t.Helper()
	return repository.WithIAMCommandGuard(context.Background(), testPool, func(ctx context.Context, tx pgx.Tx) error {
		var id int64
		require.NoError(t, tx.QueryRow(ctx, `SELECT id FROM tickets WHERE id=$1 FOR UPDATE`, ticketID).Scan(&id))
		return svc.ValidateTicketCommentEvidenceTx(ctx, tx, ticketID, refs)
	})
}

func TestTicketCommentEvidence_ReferenceContract(t *testing.T) {
	authorID, ticketID, otherTicketID := commentEvidenceEnv(t)
	svc := &Service{db: testPool}
	content := "修复完成：升级组件至 2.3.1，复测环境 synthetic-01，变更记录见发布版本。"
	publicID, publicAt := insertCommentRow(t, ticketID, authorID, content, false)
	noteID, _ := insertCommentRow(t, ticketID, authorID, "内部备注：等待窗口期", true)
	otherID, _ := insertCommentRow(t, otherTicketID, authorID, content, false)

	// 复测人视角：既有 GET 工单评论返回的 evidence_version 即可提交的版本规范形
	//（GET 侧授权门在 handler/service 层，此处直证 repo 回填值与核验口径一致；
	// includeInternal=true 以顺带断言备注行不带可核版本）。
	list, err := repository.NewTicketRepo(testPool).ListComments(context.Background(), ticketID, true)
	require.NoError(t, err)
	var apiVersion string
	var noteLeaked bool
	for _, c := range list {
		if c.ID == publicID {
			apiVersion = c.EvidenceVersion
		}
		if c.ID == noteID {
			noteLeaked = c.EvidenceVersion != ""
		}
	}
	require.NotEmpty(t, apiVersion, "公开评论必须回填 evidence_version")
	require.Equal(t, commentref.EvidenceVersionToken(publicAt, content), apiVersion)
	require.False(t, noteLeaked, "内部备注不得携带 evidence_version")

	token := apiVersion
	valid := []commentref.EvidenceRef{{Ref: fmt.Sprintf("ticket-comment:%d", publicID), Version: token}}

	t.Run("valid-public-comment", func(t *testing.T) {
		require.NoError(t, validateCommentRefs(t, svc, ticketID, valid))
	})
	t.Run("duplicate-same-version", func(t *testing.T) {
		require.NoError(t, validateCommentRefs(t, svc, ticketID, []commentref.EvidenceRef{valid[0], valid[0]}))
	})
	t.Run("internal-note", func(t *testing.T) {
		refs := []commentref.EvidenceRef{{Ref: fmt.Sprintf("ticket-comment:%d", noteID), Version: token}}
		require.ErrorIs(t, validateCommentRefs(t, svc, ticketID, refs), errcode.ErrNotFound)
	})
	t.Run("cross-ticket", func(t *testing.T) {
		refs := []commentref.EvidenceRef{{Ref: fmt.Sprintf("ticket-comment:%d", otherID), Version: token}}
		require.ErrorIs(t, validateCommentRefs(t, svc, otherTicketID, refs), errcode.ErrNotFound)
		require.ErrorIs(t, validateCommentRefs(t, svc, ticketID, refs), errcode.ErrNotFound)
	})
	t.Run("fabricated-hash", func(t *testing.T) {
		hash, createdAt, err := commentref.ParseEvidenceVersion(token)
		require.NoError(t, err)
		forged := "000000" + hash[6:] + "@" + createdAt.UTC().Format(time.RFC3339Nano)
		refs := []commentref.EvidenceRef{{Ref: fmt.Sprintf("ticket-comment:%d", publicID), Version: forged}}
		require.ErrorIs(t, validateCommentRefs(t, svc, ticketID, refs), errcode.ErrNotFound)
	})
	t.Run("fabricated-version", func(t *testing.T) {
		hash, createdAt, err := commentref.ParseEvidenceVersion(token)
		require.NoError(t, err)
		refs := []commentref.EvidenceRef{{Ref: fmt.Sprintf("ticket-comment:%d", publicID),
			Version: hash + "@" + createdAt.Add(time.Microsecond).UTC().Format(time.RFC3339Nano)}}
		require.ErrorIs(t, validateCommentRefs(t, svc, ticketID, refs), errcode.ErrNotFound)
	})
	t.Run("missing-comment", func(t *testing.T) {
		refs := []commentref.EvidenceRef{{Ref: "ticket-comment:999999999", Version: token}}
		require.ErrorIs(t, validateCommentRefs(t, svc, ticketID, refs), errcode.ErrNotFound)
	})
	t.Run("mixed-valid-and-internal", func(t *testing.T) {
		refs := append([]commentref.EvidenceRef{}, valid...)
		refs = append(refs, commentref.EvidenceRef{Ref: fmt.Sprintf("ticket-comment:%d", noteID), Version: token})
		require.ErrorIs(t, validateCommentRefs(t, svc, ticketID, refs), errcode.ErrNotFound)
	})
	t.Run("non-canonical-ref", func(t *testing.T) {
		for _, ref := range []string{"ticket-comment:01", "ticket-comment:-1", "ticket-comment:+1", "comment:1", fmt.Sprintf("ticket-comment:%d ", publicID)} {
			refs := []commentref.EvidenceRef{{Ref: ref, Version: token}}
			require.ErrorIs(t, validateCommentRefs(t, svc, ticketID, refs), errcode.ErrInvalidParams, ref)
		}
	})
	t.Run("non-canonical-version", func(t *testing.T) {
		hash, _, err := commentref.ParseEvidenceVersion(token)
		require.NoError(t, err)
		refs := []commentref.EvidenceRef{{Ref: fmt.Sprintf("ticket-comment:%d", publicID),
			Version: hash + "@2026-11-23T16:30:05.123456+08:00"}}
		require.ErrorIs(t, validateCommentRefs(t, svc, ticketID, refs), errcode.ErrInvalidParams)
	})
	t.Run("duplicate-conflicting-version", func(t *testing.T) {
		hash, _, err := commentref.ParseEvidenceVersion(token)
		require.NoError(t, err)
		refs := []commentref.EvidenceRef{valid[0], {Ref: valid[0].Ref, Version: hash + "@2026-11-23T08:30:06Z"}}
		require.ErrorIs(t, validateCommentRefs(t, svc, ticketID, refs), errcode.ErrInvalidParams)
	})
	t.Run("over-limit", func(t *testing.T) {
		refs := make([]commentref.EvidenceRef, 51)
		for i := range refs {
			refs[i] = valid[0]
		}
		require.ErrorIs(t, validateCommentRefs(t, svc, ticketID, refs), errcode.ErrInvalidParams)
	})
	t.Run("guard-rejects-unguarded-context", func(t *testing.T) {
		tx, err := testPool.Begin(context.Background())
		require.NoError(t, err)
		defer tx.Rollback(context.Background())
		err = svc.ValidateTicketCommentEvidenceTx(context.Background(), tx, ticketID, valid)
		require.ErrorIs(t, err, errcode.ErrNoPermission)
	})
}
