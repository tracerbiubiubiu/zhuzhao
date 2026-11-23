//go:build integration

package storage_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	commentref "github.com/tracerbiubiubiu/zhuzhao/internal/model/comment"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	pkgstorage "github.com/tracerbiubiubiu/zhuzhao/internal/pkg/storage"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service/riskremediation"
	storageservice "github.com/tracerbiubiubiu/zhuzhao/internal/service/storage"
)

// boundTicketFile 走守卫已接线的服务在非 HTTP ctx 下建绑（直连契约——装置步骤）。
func boundTicketFile(t *testing.T, svc *storageservice.Service, actor, tkID int64) string {
	t.Helper()
	key := presignTicketAttachment(t, svc, actor, tkID)
	_, err := svc.Confirm(context.Background(), &model.ConfirmFileInput{ObjectKey: key, TicketID: &tkID}, actor)
	require.NoError(t, err)
	return key
}

func remediationSubmissionFixture(t *testing.T, actor, ticketID int64, evidenceKey, evidenceKind string) {
	t.Helper()
	remediationEvidenceFixture(t, actor, ticketID, false,
		[]riskremediation.EvidenceRef{{Kind: evidenceKind, Ref: evidenceKey, Version: "e"}}, nil)
}

// remediationEvidenceFixture adds one immutable proposal and optionally a review.
func remediationEvidenceFixture(t *testing.T, actor, ticketID int64, closed bool, refs, reviewRefs []riskremediation.EvidenceRef) {
	t.Helper()
	ctx := context.Background()
	submissionJSON, err := json.Marshal(refs)
	require.NoError(t, err)
	var org int64
	require.NoError(t, testPool.QueryRow(ctx,
		`SELECT id FROM organizations WHERE code='root' LIMIT 1`).Scan(&org))
	suffix := uniqueSuffix()
	var family, risk, group, link, submission int64
	require.NoError(t, testPool.QueryRow(ctx,
		`INSERT INTO risk_families(fingerprint_scope,canonical_key_hash) VALUES('it-storage',$1) RETURNING id`,
		"it-del-"+suffix).Scan(&family))
	require.NoError(t, testPool.QueryRow(ctx,
		`INSERT INTO risk_items(human_key,family_id,status,org_id,owner_user_id,treatment,title,created_by)
		 VALUES($1,$2,'remediating',$3,$4,'mitigate',$1,$4) RETURNING id`,
		"IT-DEL-"+suffix, family, org, actor).Scan(&risk))
	require.NoError(t, testPool.QueryRow(ctx,
		`INSERT INTO risk_dispatch_groups(finding_key,owner_user_id,management_org_id,target_org_id,type_code,definition_version,approved_scope,group_key,ticket_id,ticket_ref_snapshot,operator_user_id)
		 VALUES($1,$2,$3,$3,'incident',1,'it-retention',repeat('a',64),$4,'IT-DEL',$2) RETURNING id`,
		"IT-DEL-"+suffix, actor, org, ticketID).Scan(&group))
	require.NoError(t, testPool.QueryRow(ctx,
		`INSERT INTO risk_ticket_links(risk_id,group_id,owner_at_link) VALUES($1,$2,$3) RETURNING id`,
		risk, group, actor).Scan(&link))
	require.NoError(t, testPool.QueryRow(ctx,
		`INSERT INTO risk_remediation_submissions(link_id,risk_id,proposal_version,risk_revision,owner_at_submission,actor_id,resolution,measures,evidence_refs)
		 VALUES($1,$2,1,1,$3,$3,'fixed','it retention measures',$4::jsonb) RETURNING id`,
		link, risk, actor, submissionJSON).Scan(&submission))
	t.Cleanup(func() {
		_, _ = testPool.Exec(ctx, `DELETE FROM risk_remediation_reviews WHERE submission_id=$1`, submission)
		_, _ = testPool.Exec(ctx, `DELETE FROM risk_remediation_submissions WHERE id=$1`, submission)
		_, _ = testPool.Exec(ctx, `DELETE FROM risk_ticket_links WHERE id=$1`, link)
		_, _ = testPool.Exec(ctx, `DELETE FROM risk_dispatch_groups WHERE id=$1`, group)
		_, _ = testPool.Exec(ctx, `DELETE FROM risk_items WHERE id=$1`, risk)
		_, _ = testPool.Exec(ctx, `DELETE FROM risk_families WHERE id=$1`, family)
	})
	if reviewRefs != nil {
		reviewJSON, err := json.Marshal(reviewRefs)
		require.NoError(t, err)
		_, err = testPool.Exec(ctx, `
			INSERT INTO risk_remediation_reviews(submission_id,decision,actor_id,note,evidence_refs,risk_revision,link_revision)
			VALUES($1,'approve',$2,'it retention review',$3::jsonb,1,1)`, submission, actor, reviewJSON)
		require.NoError(t, err)
	}
	if closed {
		_, err = testPool.Exec(ctx, `UPDATE risk_dispatch_groups SET ended_at=now() WHERE id=$1`, group)
		require.NoError(t, err)
		_, err = testPool.Exec(ctx, `UPDATE risk_ticket_links SET ended_at=now(),end_reason='ticket_closed' WHERE id=$1`, link)
		require.NoError(t, err)
		_, err = testPool.Exec(ctx, `UPDATE tickets SET status='closed' WHERE id=$1`, ticketID)
		require.NoError(t, err)
	}
}

func TestStorageDelete_RemediationEvidenceRetention(t *testing.T) {
	const deletePath = "/api/v1/tickets/attachments/delete"

	t.Run("持久整改提交引用——拒绝卸绑与标删", func(t *testing.T) {
		actor, tkID, roleCode := attachmentCommandFixture(t)
		stub := &stubClient{stat: &pkgstorage.ObjectInfo{Size: 10, ETag: "e", ContentType: "image/png"}}
		b := newGuardBundle(t, testPool, stub)
		key := boundTicketFile(t, b.storage, actor, tkID)
		remediationSubmissionFixture(t, actor, tkID, key, "file")

		err := b.storage.DeleteTicketAttachmentAs(mintPurpose(t, actor, roleCode, deletePath, true), actor, tkID, key)
		require.Error(t, err, "持久整改提交引用的证据必须拒绝卸绑/标删")
		assert.Equal(t, errcode.ErrConflict.Code, codeOf(t, err))
		assert.Equal(t, 1, attachmentCount(t, tkID), "证据附件关联行保留")
		assert.Equal(t, model.FileStatusConfirmed, objectStatus(t, key), "证据对象不标 deleted")
		assert.Empty(t, stub.removedKeys(), "物理对象删除不得执行")
	})

	t.Run("同票未引用附件保持旧行为——可删", func(t *testing.T) {
		actor, tkID, roleCode := attachmentCommandFixture(t)
		stub := &stubClient{stat: &pkgstorage.ObjectInfo{Size: 10, ETag: "e", ContentType: "image/png"}}
		b := newGuardBundle(t, testPool, stub)
		referenced := boundTicketFile(t, b.storage, actor, tkID)
		remediationSubmissionFixture(t, actor, tkID, referenced, "file")
		key := boundTicketFile(t, b.storage, actor, tkID)

		require.NoError(t, b.storage.DeleteTicketAttachmentAs(mintPurpose(t, actor, roleCode, deletePath, true), actor, tkID, key),
			"提交引用之外的普通附件不受牵连")
		assert.Equal(t, 1, attachmentCount(t, tkID), "仅未引用附件关联行卸绑")
		assert.Equal(t, model.FileStatusDeleted, objectStatus(t, key))
		assert.Equal(t, []string{key}, stub.removedKeys())
	})

	t.Run("跨票删除保持旧行为——404 同形拒绝", func(t *testing.T) {
		actor, tkA, roleCode := attachmentCommandFixture(t)
		tkB := insertTicket(t, actor)
		stub := &stubClient{stat: &pkgstorage.ObjectInfo{Size: 10, ETag: "e", ContentType: "image/png"}}
		b := newGuardBundle(t, testPool, stub)
		key := boundTicketFile(t, b.storage, actor, tkB)
		remediationSubmissionFixture(t, actor, tkB, key, "file")

		err := b.storage.DeleteTicketAttachmentAs(mintPurpose(t, actor, roleCode, deletePath, true), actor, tkA, key)
		require.Error(t, err, "跨票删除维持旧行为")
		assert.Equal(t, errcode.ErrFileNotFound.Code, codeOf(t, err))
		assert.Equal(t, 1, attachmentCount(t, tkB), "他票关联行不受影响")
		assert.Equal(t, model.FileStatusConfirmed, objectStatus(t, key))
		assert.Empty(t, stub.removedKeys())
	})

	t.Run("reference 类目同键不会误保留附件", func(t *testing.T) {
		actor, tkID, roleCode := attachmentCommandFixture(t)
		stub := &stubClient{stat: &pkgstorage.ObjectInfo{Size: 10, ETag: "e", ContentType: "image/png"}}
		b := newGuardBundle(t, testPool, stub)
		key := boundTicketFile(t, b.storage, actor, tkID)
		remediationSubmissionFixture(t, actor, tkID, key, "reference")

		require.NoError(t, b.storage.DeleteTicketAttachmentAs(mintPurpose(t, actor, roleCode, deletePath, true), actor, tkID, key),
			"非 file 类目不引用对象字节")
		assert.Zero(t, attachmentCount(t, tkID))
		assert.Equal(t, model.FileStatusDeleted, objectStatus(t, key))
	})

	t.Run("已关票历史复测引用保护附件", func(t *testing.T) {
		actor, tkID, roleCode := attachmentCommandFixture(t)
		stub := &stubClient{stat: &pkgstorage.ObjectInfo{Size: 10, ETag: "e", ContentType: "image/png"}}
		b := newGuardBundle(t, testPool, stub)
		first := boundTicketFile(t, b.storage, actor, tkID)
		review := boundTicketFile(t, b.storage, actor, tkID)
		remediationEvidenceFixture(t, actor, tkID, true,
			[]riskremediation.EvidenceRef{{Kind: "file", Ref: first, Version: "e"}},
			[]riskremediation.EvidenceRef{{Kind: "file", Ref: review, Version: "e"}})

		for _, key := range []string{first, review} {
			err := b.storage.DeleteTicketAttachmentAs(mintPurpose(t, actor, roleCode, deletePath, true), actor, tkID, key)
			assert.Equal(t, errcode.ErrConflict.Code, codeOf(t, err))
			assert.Equal(t, model.FileStatusConfirmed, objectStatus(t, key))
		}
		assert.Equal(t, 2, attachmentCount(t, tkID))
		assert.Empty(t, stub.removedKeys())
	})
}

func TestTicketDelete_RemediationEvidenceRetention(t *testing.T) {
	const deletePath = "/api/v1/tickets/delete"
	for _, tc := range []struct {
		name, kind string
		inReview   bool
	}{
		{name: "历史提交引用文件", kind: "file"},
		{name: "历史复测引用文件", kind: "file", inReview: true},
		{name: "历史提交引用公开评论", kind: "reference"},
		{name: "历史复测引用公开评论", kind: "reference", inReview: true},
		{name: "已关风险票历史只有自由引文", kind: "freeform"},
		{name: "普通旧票"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			actor, tkID, roleCode := attachmentCommandFixture(t)
			_, err := testPool.Exec(ctx, `INSERT INTO casbin_rule(ptype,v0,v1,v2) VALUES('p',$1,$2,'POST')`, "role::"+roleCode, deletePath)
			require.NoError(t, err)
			var orgID int64
			err = testPool.QueryRow(ctx, `UPDATE organizations SET owner_user_ids=array_append(owner_user_ids,$1)
				WHERE id=(SELECT org_id FROM tickets WHERE id=$2) RETURNING id`, actor, tkID).Scan(&orgID)
			require.NoError(t, err)
			t.Cleanup(func() {
				_, _ = testPool.Exec(ctx, `UPDATE organizations SET owner_user_ids=array_remove(owner_user_ids,$1) WHERE id=$2`, actor, orgID)
				_, _ = testPool.Exec(ctx, `DELETE FROM casbin_rule WHERE ptype='p' AND v0=$1 AND v1=$2 AND v2='POST'`, "role::"+roleCode, deletePath)
			})
			stub := &stubClient{stat: &pkgstorage.ObjectInfo{Size: 10, ETag: "e", ContentType: "image/png"}}
			b := newGuardBundle(t, testPool, stub)
			key := boundTicketFile(t, b.storage, actor, tkID)
			public := &model.TicketComment{TicketID: tkID, UserID: actor, Content: "synthetic public evidence"}
			require.NoError(t, repository.NewTicketRepo(testPool).CreateComment(ctx, public))
			if tc.kind != "" {
				ref := riskremediation.EvidenceRef{Kind: "file", Ref: key, Version: "e"}
				if tc.kind == "reference" {
					ref = riskremediation.EvidenceRef{Kind: "reference", Ref: fmt.Sprintf("%s%d", commentref.EvidenceRefPrefix, public.ID),
						Version: commentref.EvidenceVersionToken(public.CreatedAt, public.Content)}
				} else if tc.kind == "freeform" {
					ref = riskremediation.EvidenceRef{Kind: "reference", Ref: key, Version: "e"}
				}
				refs := []riskremediation.EvidenceRef{ref}
				var review []riskremediation.EvidenceRef
				if tc.inReview {
					refs = []riskremediation.EvidenceRef{{Kind: "reference", Ref: "synthetic historical freeform reference", Version: "v1"}}
					review = []riskremediation.EvidenceRef{ref}
				}
				remediationEvidenceFixture(t, actor, tkID, true, refs, review)
			} else {
				_, err = testPool.Exec(ctx, `UPDATE tickets SET status='closed' WHERE id=$1`, tkID)
				require.NoError(t, err)
			}

			err = b.ticket.Delete(mintPurpose(t, actor, roleCode, deletePath, false), tkID, actor)
			var tickets, comments, deletes int
			require.NoError(t, testPool.QueryRow(ctx, `SELECT
				(SELECT count(*) FROM tickets WHERE id=$1),
				(SELECT count(*) FROM ticket_comments WHERE ticket_id=$1),
				(SELECT count(*) FROM ticket_events WHERE user_id=$2 AND action='deleted')`, tkID, actor).Scan(&tickets, &comments, &deletes))
			if tc.kind == "" || tc.kind == "freeform" {
				require.NoError(t, err)
				assert.Zero(t, tickets)
				assert.Zero(t, comments)
				assert.Zero(t, attachmentCount(t, tkID))
				assert.Equal(t, 1, deletes)
				return
			}
			assert.ErrorIs(t, err, errcode.ErrConflict)
			assert.Equal(t, 1, tickets)
			assert.Equal(t, 1, comments)
			assert.Equal(t, 1, attachmentCount(t, tkID))
			assert.Equal(t, model.FileStatusConfirmed, objectStatus(t, key))
			assert.Zero(t, deletes)
			assert.Empty(t, stub.removedKeys())
		})
	}
}

func TestStorageGC_DetachedRemediationEvidenceRetention(t *testing.T) {
	ctx := context.Background()
	actor := insertUser(t)
	tkID := insertTicket(t, actor)
	bucket := "it-remediation-gc-" + uniqueSuffix()
	cfg := &pkgstorage.StorageConfig{Enabled: true, Bucket: bucket, OrphanGCDays: 2}
	cfg.ApplyDefaults()
	cfgStore := pkgstorage.NewConfigStore(cfg)
	repo := repository.NewStorageRepo(testPool)
	stub := &stubClient{stat: &pkgstorage.ObjectInfo{Size: 10, ETag: "e", ContentType: "image/png"}}
	svc := storageservice.NewService(repo, stub, cfgStore, testLogger())
	t.Cleanup(func() {
		_, _ = testPool.Exec(ctx, `DELETE FROM file_objects WHERE bucket=$1`, bucket)
	})
	confirmed := boundTicketFile(t, svc, actor, tkID)
	tombstone := boundTicketFile(t, svc, actor, tkID)
	unreferenced := boundTicketFile(t, svc, actor, tkID)
	remediationEvidenceFixture(t, actor, tkID, true,
		[]riskremediation.EvidenceRef{{Kind: "file", Ref: confirmed, Version: "e"}},
		[]riskremediation.EvidenceRef{{Kind: "file", Ref: tombstone, Version: "e"}})
	// Reproduce an old deletion before the retention guards existed.
	_, err := testPool.Exec(ctx, `DELETE FROM tickets WHERE id=$1`, tkID)
	require.NoError(t, err)
	_, err = testPool.Exec(ctx, `UPDATE file_objects SET confirmed_at=now()-interval '30 days'
		WHERE bucket=$1 AND object_key=ANY($2)`, bucket, []string{confirmed, tombstone, unreferenced})
	require.NoError(t, err)
	_, err = testPool.Exec(ctx, `UPDATE file_objects SET status='deleted' WHERE bucket=$1 AND object_key=$2`, bucket, tombstone)
	require.NoError(t, err)

	orphans, err := repo.ListConfirmedOrphans(ctx, bucket, time.Now().AddDate(0, 0, -4), 10)
	require.NoError(t, err)
	if assert.Len(t, orphans, 1) {
		assert.Equal(t, unreferenced, orphans[0].ObjectKey)
	}
	deleted, err := repo.ListDeleted(ctx, bucket, 10)
	require.NoError(t, err)
	assert.Empty(t, deleted, "二扫不应选中仍被历史引用的墓碑")
	var fileID int64
	require.NoError(t, testPool.QueryRow(ctx, `SELECT id FROM file_objects WHERE bucket=$1 AND object_key=$2`, bucket, confirmed).Scan(&fileID))
	marked, err := repo.MarkDeleted(ctx, fileID)
	require.NoError(t, err)
	assert.False(t, marked, "对象锁后二次检查仍须阻止标删")
	job := storageservice.NewGCJob(repo, stub, cfgStore, testLogger(), true)
	require.NoError(t, job.Handle(ctx, nil))
	require.ElementsMatch(t, []string{unreferenced}, stub.removedKeys())
	retained, err := repo.GetByObjectKey(ctx, bucket, confirmed)
	require.NoError(t, err)
	assert.Equal(t, model.FileStatusConfirmed, retained.Status)
	retained, err = repo.GetByObjectKey(ctx, bucket, tombstone)
	require.NoError(t, err)
	assert.Equal(t, model.FileStatusDeleted, retained.Status)
}
