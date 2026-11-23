//go:build integration

package storage_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
	storageservice "github.com/tracerbiubiubiu/zhuzhao/internal/service/storage"
)

func TestBoundTicketEvidence_AlreadyBoundVersion(t *testing.T) {
	ctx := context.Background()
	u := insertUser(t)
	ticket := insertTicket(t, u)
	_, repo := newService(t, &stubClient{})
	validator := storageservice.NewBoundTicketEvidenceValidator(testPool, itBucket)
	key := "tickets/bound-evidence-" + uniqueSuffix()
	f := &model.FileObject{Bucket: itBucket, ObjectKey: key, Purpose: "ticket_attachment", Status: "pending", FileName: "synthetic.txt", MIMEType: "text/plain", CreatedBy: u}
	require.NoError(t, repo.InsertPending(ctx, f))
	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(), `DELETE FROM ticket_attachments WHERE file_object_id=$1`, f.ID)
		_, _ = testPool.Exec(context.Background(), `DELETE FROM file_objects WHERE id=$1`, f.ID)
	})
	require.NoError(t, repo.ConfirmAndBind(ctx, f.ID, ticket, u, 10, "version-A", 20))
	for _, version := range []string{"version-A", "version-B"} {
		t.Run(version, func(t *testing.T) {
			err := repository.WithIAMCommandGuard(ctx, testPool, func(ctx context.Context, tx pgx.Tx) error {
				var id int64
				require.NoError(t, tx.QueryRow(ctx, `SELECT id FROM tickets WHERE id=$1 FOR UPDATE`, ticket).Scan(&id))
				return validator.ValidateBoundTicketEvidenceTx(ctx, tx, ticket, []storageservice.BoundTicketEvidenceRef{{ObjectKey: key, Version: version}})
			})
			if version == "version-A" {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, errcode.ErrFileNotFound, "different evidence ETag must reject")
			}
		})
	}
}

func TestBoundTicketEvidence_BindingAndMetadata(t *testing.T) {
	for _, name := range []string{"valid", "legacy-owner", "wrong-ticket", "dangling", "deleted", "pending", "wrong-purpose", "wrong-owner", "wrong-bucket", "foreign-link", "missing-etag", "missing-confirmation", "duplicate-version-conflict"} {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			u := insertUser(t)
			ticket := insertTicket(t, u)
			other := insertTicket(t, u)
			_, repo := newService(t, &stubClient{})
			key := "tickets/bound-evidence-" + uniqueSuffix()
			f := &model.FileObject{Bucket: itBucket, ObjectKey: key, Purpose: "ticket_attachment", Status: "pending", FileName: "synthetic.txt", MIMEType: "text/plain", CreatedBy: u}
			require.NoError(t, repo.InsertPending(ctx, f))
			t.Cleanup(func() {
				_, _ = testPool.Exec(context.Background(), `DELETE FROM ticket_attachments WHERE file_object_id=$1`, f.ID)
				_, _ = testPool.Exec(context.Background(), `DELETE FROM file_objects WHERE id=$1`, f.ID)
			})
			if name == "dangling" {
				require.NoError(t, repo.Confirm(ctx, f.ID, 10, "version-A"))
			} else {
				require.NoError(t, repo.ConfirmAndBind(ctx, f.ID, ticket, u, 10, "version-A", 20))
			}
			bucket := itBucket
			target := ticket
			refs := []storageservice.BoundTicketEvidenceRef{{ObjectKey: key, Version: "version-A"}}
			var mutation string
			switch name {
			case "legacy-owner":
				mutation = `UPDATE file_objects SET owner_kind=NULL,owner_id=NULL WHERE id=$1`
			case "wrong-ticket":
				target = other
			case "deleted":
				mutation = `UPDATE file_objects SET status='deleted' WHERE id=$1`
			case "pending":
				mutation = `UPDATE file_objects SET status='pending' WHERE id=$1`
			case "wrong-purpose":
				mutation = `UPDATE file_objects SET purpose='attachment_create' WHERE id=$1`
			case "wrong-owner":
				mutation = `UPDATE file_objects SET owner_kind='user' WHERE id=$1`
			case "wrong-bucket":
				bucket = "different-synthetic-bucket"
			case "foreign-link":
				_, err := testPool.Exec(ctx, `INSERT INTO ticket_attachments(ticket_id,file_object_id,uploaded_by)VALUES($1,$2,$3)`, other, f.ID, u)
				require.NoError(t, err)
			case "missing-etag":
				mutation = `UPDATE file_objects SET etag=NULL WHERE id=$1`
			case "missing-confirmation":
				mutation = `UPDATE file_objects SET confirmed_at=NULL WHERE id=$1`
			case "duplicate-version-conflict":
				refs = append(refs, storageservice.BoundTicketEvidenceRef{ObjectKey: key, Version: "version-B"})
			}
			if mutation != "" {
				_, err := testPool.Exec(ctx, mutation, f.ID)
				require.NoError(t, err)
			}
			validator := storageservice.NewBoundTicketEvidenceValidator(testPool, bucket)
			err := repository.WithIAMCommandGuard(ctx, testPool, func(ctx context.Context, tx pgx.Tx) error {
				var id int64
				require.NoError(t, tx.QueryRow(ctx, `SELECT id FROM tickets WHERE id=$1 FOR UPDATE`, target).Scan(&id))
				return validator.ValidateBoundTicketEvidenceTx(ctx, tx, target, refs)
			})
			if name == "valid" || name == "legacy-owner" {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, errcode.ErrFileNotFound)
			}
			// Validation must never repair ownership or create a missing binding.
			var links int
			require.NoError(t, testPool.QueryRow(ctx, `SELECT count(*) FROM ticket_attachments WHERE file_object_id=$1`, f.ID).Scan(&links))
			expectedLinks := 1
			if name == "dangling" {
				expectedLinks = 0
			} else if name == "foreign-link" {
				expectedLinks = 2
			}
			require.Equal(t, expectedLinks, links)
		})
	}
}

func TestBoundTicketEvidence_RereadsAfterObjectLockWait(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	u := insertUser(t)
	ticket := insertTicket(t, u)
	_, repo := newService(t, &stubClient{})
	key := "tickets/bound-evidence-" + uniqueSuffix()
	f := &model.FileObject{Bucket: itBucket, ObjectKey: key, Purpose: "ticket_attachment", Status: "pending", FileName: "synthetic.txt", MIMEType: "text/plain", CreatedBy: u}
	require.NoError(t, repo.InsertPending(ctx, f))
	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(), `DELETE FROM ticket_attachments WHERE file_object_id=$1`, f.ID)
		_, _ = testPool.Exec(context.Background(), `DELETE FROM file_objects WHERE id=$1`, f.ID)
	})
	require.NoError(t, repo.ConfirmAndBind(ctx, f.ID, ticket, u, 10, "version-A", 20))
	writer, err := testPool.Begin(ctx)
	require.NoError(t, err)
	defer func() { _ = writer.Rollback(context.Background()) }()
	_, err = writer.Exec(ctx, `UPDATE file_objects SET etag='version-B' WHERE id=$1`, f.ID)
	require.NoError(t, err)
	validator := storageservice.NewBoundTicketEvidenceValidator(testPool, itBucket)
	result := make(chan error, 1)
	go func() {
		result <- repository.WithIAMCommandGuard(ctx, testPool, func(ctx context.Context, tx pgx.Tx) error {
			var id int64
			if err := tx.QueryRow(ctx, `SELECT id FROM tickets WHERE id=$1 FOR UPDATE`, ticket).Scan(&id); err != nil {
				return err
			}
			return validator.ValidateBoundTicketEvidenceTx(ctx, tx, ticket, []storageservice.BoundTicketEvidenceRef{{ObjectKey: key, Version: "version-A"}})
		})
	}()
	require.Eventually(t, func() bool {
		var waiting bool
		err := testPool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE datname=current_database() AND wait_event_type='Lock' AND query LIKE 'SELECT id,object_key FROM file_objects%')`).Scan(&waiting)
		return err == nil && waiting
	}, 5*time.Second, 10*time.Millisecond, "validator must actually wait on the object's row lock")
	require.NoError(t, writer.Commit(ctx))
	select {
	case err := <-result:
		require.ErrorIs(t, err, errcode.ErrFileNotFound)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
}

func TestBoundTicketEvidence_ExactIAMTransaction(t *testing.T) {
	ctx := context.Background()
	validator := storageservice.NewBoundTicketEvidenceValidator(testPool, itBucket)
	tx, err := testPool.Begin(ctx)
	require.NoError(t, err)
	require.ErrorIs(t, validator.ValidateBoundTicketEvidenceTx(ctx, tx, 1, nil), errcode.ErrNoPermission)
	require.NoError(t, tx.Rollback(ctx))
	otherPool, err := pgxpool.New(ctx, testPool.Config().ConnString())
	require.NoError(t, err)
	defer otherPool.Close()
	wrongPool := storageservice.NewBoundTicketEvidenceValidator(otherPool, itBucket)
	require.NoError(t, repository.WithIAMCommandGuard(ctx, testPool, func(ctx context.Context, tx pgx.Tx) error {
		require.NoError(t, validator.ValidateBoundTicketEvidenceTx(ctx, tx, 1, nil))
		require.ErrorIs(t, wrongPool.ValidateBoundTicketEvidenceTx(ctx, tx, 1, nil), errcode.ErrNoPermission)
		child, err := tx.Begin(ctx)
		require.NoError(t, err)
		require.ErrorIs(t, validator.ValidateBoundTicketEvidenceTx(ctx, child, 1, nil), errcode.ErrNoPermission)
		require.NoError(t, child.Rollback(ctx))
		return nil
	}))
}

// The risk kernel enters a real IAM-bound savepoint. A physical connection is
// not an authorization capability: both mismatched parent/child pairs reject,
// while the exact child can consume an already bound attachment.
func TestBoundTicketEvidence_IAMSavepointOwnsEvidenceAndRollsBackFailure(t *testing.T) {
	ctx := context.Background()
	u := insertUser(t)
	ticket := insertTicket(t, u)
	repo := repository.NewStorageRepo(testPool)
	key := "tickets/bound-savepoint-" + uniqueSuffix()
	f := &model.FileObject{Bucket: itBucket, ObjectKey: key, Purpose: "ticket_attachment", Status: "pending", FileName: "synthetic.txt", MIMEType: "text/plain", CreatedBy: u}
	require.NoError(t, repo.InsertPending(ctx, f))
	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(), `DELETE FROM ticket_attachments WHERE file_object_id=$1`, f.ID)
		_, _ = testPool.Exec(context.Background(), `DELETE FROM file_objects WHERE id=$1`, f.ID)
	})
	require.NoError(t, repo.ConfirmAndBind(ctx, f.ID, ticket, u, 10, "version-A", 20))
	validator := storageservice.NewBoundTicketEvidenceValidator(testPool, itBucket)
	refs := []storageservice.BoundTicketEvidenceRef{{ObjectKey: key, Version: "version-A"}}

	require.NoError(t, repository.WithIAMCommandGuard(ctx, testPool, func(parentCtx context.Context, parent pgx.Tx) error {
		var id int64
		require.NoError(t, parent.QueryRow(parentCtx, `SELECT id FROM tickets WHERE id=$1 FOR UPDATE`, ticket).Scan(&id))
		require.NoError(t, repository.WithIAMSavepoint(parentCtx, testPool, parent, func(childCtx context.Context, child pgx.Tx) error {
			require.NotSame(t, parent, child)
			require.Same(t, parent.Conn(), child.Conn(), "the savepoint belongs to the same C04 transaction")
			require.Same(t, child, repository.IAMDB(childCtx, testPool))
			require.ErrorIs(t, validator.ValidateBoundTicketEvidenceTx(parentCtx, child, ticket, refs), errcode.ErrNoPermission)
			require.ErrorIs(t, validator.ValidateBoundTicketEvidenceTx(childCtx, parent, ticket, refs), errcode.ErrNoPermission, "the outer tx cannot borrow a child's authorization")
			return validator.ValidateBoundTicketEvidenceTx(childCtx, child, ticket, refs)
		}))

		// A post-write validation failure must roll the child back even when its
		// caller deliberately handles the error and commits the outer command.
		err := repository.WithIAMSavepoint(parentCtx, testPool, parent, func(childCtx context.Context, child pgx.Tx) error {
			_, err := child.Exec(childCtx, `UPDATE file_objects SET etag='version-B' WHERE id=$1`, f.ID)
			require.NoError(t, err)
			return validator.ValidateBoundTicketEvidenceTx(childCtx, child, ticket, refs)
		})
		require.ErrorIs(t, err, errcode.ErrFileNotFound)
		return validator.ValidateBoundTicketEvidenceTx(parentCtx, parent, ticket, refs)
	}))
	var version string
	var links int
	require.NoError(t, testPool.QueryRow(ctx, `SELECT etag FROM file_objects WHERE id=$1`, f.ID).Scan(&version))
	require.Equal(t, "version-A", version, "failed savepoint metadata did not leak through the committed outer transaction")
	require.NoError(t, testPool.QueryRow(ctx, `SELECT count(*) FROM ticket_attachments WHERE file_object_id=$1 AND ticket_id=$2`, f.ID, ticket).Scan(&links))
	require.Equal(t, 1, links, "validation acquires no new binding")
}
