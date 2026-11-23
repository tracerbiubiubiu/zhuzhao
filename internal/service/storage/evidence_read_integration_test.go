//go:build integration

// B2 P1 整改证据文件统一读取判定（隔离 testcontainers PG——不接共享 DB）。
// 两个读面（代理下载 + 证据元数据列表）同判：证据文件要求当前风险可见
// （复用 riskVisiblePredicate）+ 既有工单读取面；覆盖活跃与结束后
// （ticket_closed）历史 submission/review 引用；一般附件兼容性正例；
// 失败封闭；评论类公开材料不受限。
package storage_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	pkgstorage "github.com/tracerbiubiubiu/zhuzhao/internal/pkg/storage"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
	storageservice "github.com/tracerbiubiubiu/zhuzhao/internal/service/storage"
)

type evidenceFixture struct {
	actor, outsider, owner         int64
	ticket, risk, link, group, org int64
	submission                     int64
	mainFileID, reviewFileID       int64
	mainKey, reviewKey             string
	mainETag, reviewETag           string
}

// newEvidenceFixture 建最小整改证据事实链：root org 下 owner=owner 的风险 →
// 派单组挂 ticket → link → submission（主文件证据 + 评论类引用）〔→ review
// （复测文件证据）〕。actor 加入 root org（组织可见）；outsider 无任何可见锚。
func newEvidenceFixture(t *testing.T, withReview, ended bool) *evidenceFixture {
	t.Helper()
	ctx := context.Background()
	f := &evidenceFixture{mainETag: "etag-main", reviewETag: "etag-review"}
	f.mainKey = "tickets/evidence-main-" + uniqueSuffix()
	f.reviewKey = "tickets/evidence-review-" + uniqueSuffix()

	f.actor = insertUser(t)
	f.outsider = insertUser(t)
	f.owner = insertUser(t)
	f.ticket = insertTicket(t, f.owner)
	require.NoError(t, testPool.QueryRow(ctx,
		`SELECT id FROM organizations WHERE code='root' LIMIT 1`).Scan(&f.org))
	_, err := testPool.Exec(ctx, `INSERT INTO user_orgs(user_id,org_id) VALUES($1,$2)`, f.actor, f.org)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = testPool.Exec(ctx, `DELETE FROM user_orgs WHERE user_id=$1 AND org_id=$2`, f.actor, f.org)
	})

	var family int64
	require.NoError(t, testPool.QueryRow(ctx,
		`INSERT INTO risk_families(fingerprint_scope,canonical_key_hash) VALUES('synthetic',$1) RETURNING id`,
		"fam-"+uniqueSuffix()).Scan(&family))
	t.Cleanup(func() { _, _ = testPool.Exec(ctx, `DELETE FROM risk_families WHERE id=$1`, family) })
	require.NoError(t, testPool.QueryRow(ctx,
		`INSERT INTO risk_items(human_key,family_id,status,org_id,owner_user_id,title,created_by)
		 VALUES($1,$2,'remediating',$3,$4,$1,$4) RETURNING id`,
		"rk-"+uniqueSuffix(), family, f.org, f.owner).Scan(&f.risk))
	t.Cleanup(func() { _, _ = testPool.Exec(ctx, `DELETE FROM risk_items WHERE id=$1`, f.risk) })

	require.NoError(t, testPool.QueryRow(ctx,
		`INSERT INTO risk_dispatch_groups(finding_key,owner_user_id,management_org_id,target_org_id,type_code,
		   definition_version,approved_scope,group_key,ticket_id,ticket_ref_snapshot,operator_user_id)
		 VALUES($1,$2,$3,$3,'incident',(SELECT definition_version FROM tickets WHERE id=$4),'synthetic',
		   md5(random()::text)||md5(random()::text),$4,$5,$2) RETURNING id`,
		"find-"+uniqueSuffix(), f.owner, f.org, f.ticket, "TK-"+uniqueSuffix()).Scan(&f.group))
	t.Cleanup(func() { _, _ = testPool.Exec(ctx, `DELETE FROM risk_dispatch_groups WHERE id=$1`, f.group) })
	require.NoError(t, testPool.QueryRow(ctx,
		`INSERT INTO risk_ticket_links(risk_id,group_id,owner_at_link) VALUES($1,$2,$3) RETURNING id`,
		f.risk, f.group, f.owner).Scan(&f.link))
	t.Cleanup(func() { _, _ = testPool.Exec(ctx, `DELETE FROM risk_ticket_links WHERE id=$1`, f.link) })

	require.NoError(t, testPool.QueryRow(ctx,
		`INSERT INTO risk_remediation_submissions(link_id,risk_id,proposal_version,risk_revision,
		   owner_at_submission,actor_id,resolution,measures,evidence_refs)
		 VALUES($1,$2,1,1,$3,$3,'fixed','synthetic measures',$4::jsonb) RETURNING id`,
		f.link, f.risk, f.owner,
		`[{"kind":"file","ref":"`+f.mainKey+`","version":"`+f.mainETag+`"},
		  {"kind":"reference","ref":"ticket-comment:1","version":"`+f.mainETag+`"}]`).Scan(&f.submission))
	t.Cleanup(func() {
		_, _ = testPool.Exec(ctx, `DELETE FROM risk_remediation_reviews WHERE submission_id=$1`, f.submission)
		_, _ = testPool.Exec(ctx, `DELETE FROM risk_remediation_submissions WHERE id=$1`, f.submission)
	})
	if withReview {
		_, err := testPool.Exec(ctx,
			`INSERT INTO risk_remediation_reviews(submission_id,decision,actor_id,note,evidence_refs,risk_revision,link_revision)
			 VALUES($1,'approve',$2,'synthetic review',$3::jsonb,1,1)`,
			f.submission, f.owner,
			`[{"kind":"file","ref":"`+f.reviewKey+`","version":"`+f.reviewETag+`"}]`)
		require.NoError(t, err)
	}
	if ended {
		// 内核 endClosedDispatchScope 口径：组结束 + 链接 ticket_closed（revision+1）。
		_, err := testPool.Exec(ctx,
			`UPDATE risk_dispatch_groups SET ended_at=now() WHERE id=$1`, f.group)
		require.NoError(t, err)
		_, err = testPool.Exec(ctx,
			`UPDATE risk_ticket_links SET ended_at=now(), end_reason='ticket_closed', revision=revision+1 WHERE id=$1`,
			f.link)
		require.NoError(t, err)
	}

	_, repo := newService(t, &stubClient{getBody: []byte("evidence-bytes")})
	f.mainFileID = f.insertConfirmedFile(t, repo, f.mainKey, f.mainETag)
	if withReview {
		f.reviewFileID = f.insertConfirmedFile(t, repo, f.reviewKey, f.reviewETag)
	}
	return f
}

func (f *evidenceFixture) insertConfirmedFile(t *testing.T, repo *repository.StorageRepo, key, etag string) int64 {
	t.Helper()
	fo := &model.FileObject{Bucket: itBucket, ObjectKey: key, Purpose: "ticket_attachment",
		Status: model.FileStatusPending, FileName: "证据-" + uniqueSuffix() + ".png",
		MIMEType: "image/png", CreatedBy: f.owner}
	require.NoError(t, repo.InsertPending(context.Background(), fo))
	require.NoError(t, repo.ConfirmAndBind(context.Background(), fo.ID, f.ticket, f.owner, 12, etag, 20))
	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(), `DELETE FROM ticket_attachments WHERE file_object_id=$1`, fo.ID)
		_, _ = testPool.Exec(context.Background(), `DELETE FROM file_objects WHERE id=$1`, fo.ID)
	})
	return fo.ID
}

func evidenceTestService(t *testing.T) *storageservice.Service {
	t.Helper()
	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(), `DELETE FROM file_objects WHERE bucket=$1`, itBucket)
	})
	cfg := &pkgstorage.StorageConfig{Enabled: true, Endpoint: "s", Bucket: itBucket,
		AccessKey: "ak", SecretKey: "sk", PresignUploadTTL: "15m"}
	cfg.ApplyDefaults()
	svc := storageservice.NewService(repository.NewStorageRepo(testPool),
		&stubClient{getBody: []byte("evidence-bytes")}, pkgstorage.NewConfigStore(cfg), testLogger())
	svc.UseRiskEvidenceRouteReader(func(_ context.Context, _ int64, ids []int64) (map[int64]bool, error) {
		grants := make(map[int64]bool, len(ids))
		for _, id := range ids {
			grants[id] = true
		}
		return grants, nil
	})
	return svc
}

// ticketFaceAllow 模拟既有工单读取面（L2 read）已通过——本批专测新增风险面；
// ticketFaceDeny 反之，验证双资格的合取语义。
func ticketFaceAllow(int64) error { return nil }
func ticketFaceDeny(int64) error  { return errcode.ErrNoPermission }

func TestEvidenceDownload_UnifiedJudgement(t *testing.T) {
	type scenario struct {
		name       string
		withReview bool
		ended      bool
		key        func(f *evidenceFixture) string
		actor      func(f *evidenceFixture) int64
		authz      func(int64) error
		etag       func(f *evidenceFixture) string // 非空时作 If-None-Match（304 次序场景）
		wantErr    error
	}
	scenarios := []scenario{
		{
			name: "活跃组证据文件_风险可见工单面放行", withReview: false, ended: false,
			key:   func(f *evidenceFixture) string { return f.mainKey },
			actor: func(f *evidenceFixture) int64 { return f.actor }, authz: ticketFaceAllow,
		},
		{
			name: "结束后历史证据文件仍按当前资格判定_放行", withReview: false, ended: true,
			key:   func(f *evidenceFixture) string { return f.mainKey },
			actor: func(f *evidenceFixture) int64 { return f.actor }, authz: ticketFaceAllow,
		},
		{
			name: "复测阶段review文件引用同判放行", withReview: true, ended: false,
			key:   func(f *evidenceFixture) string { return f.reviewKey },
			actor: func(f *evidenceFixture) int64 { return f.actor }, authz: ticketFaceAllow,
		},
		{
			name: "票面可见但风险不可见_同形404不放行", withReview: false, ended: false,
			key:   func(f *evidenceFixture) string { return f.mainKey },
			actor: func(f *evidenceFixture) int64 { return f.outsider }, authz: ticketFaceAllow,
			wantErr: errcode.ErrFileNotFound,
		},
		{
			name: "结束后历史_风险不可见仍同形404", withReview: false, ended: true,
			key:   func(f *evidenceFixture) string { return f.mainKey },
			actor: func(f *evidenceFixture) int64 { return f.outsider }, authz: ticketFaceAllow,
			wantErr: errcode.ErrFileNotFound,
		},
		{
			name: "风险可见但工单面拒绝_双资格合取不放行", withReview: false, ended: false,
			key:   func(f *evidenceFixture) string { return f.mainKey },
			actor: func(f *evidenceFixture) int64 { return f.actor }, authz: ticketFaceDeny,
			wantErr: errcode.ErrNoPermission,
		},
		{
			name: "零值调用者_失败封闭不放行", withReview: false, ended: false,
			key:   func(f *evidenceFixture) string { return f.mainKey },
			actor: func(f *evidenceFixture) int64 { return 0 }, authz: ticketFaceAllow,
			wantErr: errcode.ErrFileNotFound,
		},
		{
			// 授权先于缓存语义：不可见方持从别处获知的 ETag 换 304 即存在性泄露。
			name: "不可见方持已知ETag_拒绝先于304短路", withReview: false, ended: false,
			key:   func(f *evidenceFixture) string { return f.mainKey },
			actor: func(f *evidenceFixture) int64 { return f.outsider }, authz: ticketFaceAllow,
			etag:    func(f *evidenceFixture) string { return f.mainETag },
			wantErr: errcode.ErrFileNotFound,
		},
	}
	for _, s := range scenarios {
		t.Run(s.name, func(t *testing.T) {
			f := newEvidenceFixture(t, s.withReview, s.ended)
			svc := evidenceTestService(t)
			inm := ""
			if s.etag != nil {
				inm = s.etag(f)
			}
			dl, err := svc.Download(context.Background(), s.key(f), inm, s.actor(f), s.authz)
			if s.wantErr != nil {
				require.ErrorIs(t, err, s.wantErr)
				require.NotErrorIs(t, err, storageservice.ErrNotModified, "拒绝不得降级为 304 哨兵")
				return
			}
			require.NoError(t, err)
			require.NotNil(t, dl)
			require.NoError(t, dl.Body.Close())
		})
	}
}

// TestEvidenceDownload_OldAttachmentPositive 旧附件正例：未被整改证据引用的
// 普通附件行为不变——无风险资格也放行（一般附件兼容性）。
func TestEvidenceDownload_OldAttachmentPositive(t *testing.T) {
	f := newEvidenceFixture(t, false, false)
	svc := evidenceTestService(t)
	plain := "tickets/plain-" + uniqueSuffix()
	_, repo := newService(t, &stubClient{getBody: []byte("plain-bytes")})
	fo := &model.FileObject{Bucket: itBucket, ObjectKey: plain, Purpose: "ticket_attachment",
		Status: model.FileStatusPending, FileName: "普通附件.png", MIMEType: "image/png", CreatedBy: f.owner}
	require.NoError(t, repo.InsertPending(context.Background(), fo))
	require.NoError(t, repo.ConfirmAndBind(context.Background(), fo.ID, f.ticket, f.owner, 8, "etag-plain", 20))

	dl, err := svc.Download(context.Background(), plain, "", f.outsider, ticketFaceAllow)
	require.NoError(t, err, "非证据附件不触发风险资格约束（既有工单面不变）")
	require.NotNil(t, dl)
	require.NoError(t, dl.Body.Close())
}

func TestEvidenceList_CurrentRiskVisibility(t *testing.T) {
	for _, ended := range []bool{false, true} {
		f := newEvidenceFixture(t, true, ended)
		svc := evidenceTestService(t)
		f.insertConfirmedFile(t, repository.NewStorageRepo(testPool), "tickets/ordinary-"+uniqueSuffix(), "plain")
		authorized, err := svc.ListTicketAttachments(context.Background(), f.ticket, f.actor)
		require.NoError(t, err)
		require.Len(t, authorized, 3)
		ordinaryOnly, err := svc.ListTicketAttachments(context.Background(), f.ticket, f.outsider)
		require.NoError(t, err)
		require.Len(t, ordinaryOnly, 1)
		require.NotEqual(t, f.mainKey, ordinaryOnly[0].ObjectKey)
		require.NotEqual(t, f.reviewKey, ordinaryOnly[0].ObjectKey)
		svc.UseRiskEvidenceRouteReader(nil)
		routeDenied, err := svc.ListTicketAttachments(context.Background(), f.ticket, f.actor)
		require.NoError(t, err)
		require.Len(t, routeDenied, 1)
		_, err = svc.Download(context.Background(), f.mainKey, f.mainETag, f.actor, ticketFaceAllow)
		require.ErrorIs(t, err, errcode.ErrFileNotFound)
	}
}
