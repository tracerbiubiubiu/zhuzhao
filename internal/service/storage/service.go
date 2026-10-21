// Package storage 对象存储业务服务（phase5 S5 B5-1 底座——presign 限制矩阵执行
// 与 file_objects 状态机）。域边界：本包不 import 其他 service 域（架构守护
// TestArchitecture_ServiceDomainIsolation）；ticket_attachment/ticket_richtext
// purpose 的 L2 管线由 handler 层编排（随 B5-3 接线）。
package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	pkgstorage "github.com/tracerbiubiubiu/zhuzhao/internal/pkg/storage"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

// purposePrefixes B5-1 支持面（A.7 权限矩阵——三类均为登录态/本人语义；
// ticket_attachment/ticket_richtext 的 ticket:update+L2 管线随 B5-3 扩）。
// 值 = 对象 key 域前缀（purpose↔前缀一致性校验锚点，05 号 §3 key 细节）。
var purposePrefixes = map[string]string{
	"avatar":            "avatars/",
	"attachment_create": "tickets/",
	"richtext_create":   "tickets/",
	"ticket_attachment": "tickets/",
	"ticket_richtext":   "tickets/",
}

// ticketPurposes ticket 态集合（presign/confirm 必带 ticket_id——L2 在
// handler 编排层判定，service 校验形态完整性）。
var ticketPurposes = map[string]bool{
	"ticket_attachment": true,
	"ticket_richtext":   true,
}

// mimeExt 白名单类型的扩展名推导表（ext 从签名钉死的 Content-Type 推导，
// 不取用户文件名——A.2 key 规范）。通配类（officedocument.*）按具体子型入表。
var mimeExt = map[string]string{
	"image/jpeg": "jpg",
	"image/png":  "png",
	"image/webp": "webp",
	"image/gif":  "gif",

	"application/pdf":    "pdf",
	"text/plain":         "txt",
	"application/zip":    "zip",
	"application/msword": "doc",
	"application/vnd.openxmlformats-officedocument.wordprocessingml.document":   "docx",
	"application/vnd.openxmlformats-officedocument.presentationml.presentation": "pptx",
	"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":         "xlsx",
}

const maxFileNameRunes = 255

type Service struct {
	repo   *repository.StorageRepo
	client pkgstorage.Client
	cfg    *pkgstorage.ConfigStore // F-D3（2026-10-21）：原子快照——KV 运行时覆盖与请求侧读并发安全
	bucket string
	logger *slog.Logger
}

func NewService(repo *repository.StorageRepo, client pkgstorage.Client, cfg *pkgstorage.ConfigStore, logger *slog.Logger) *Service {
	return &Service{repo: repo, client: client, cfg: cfg, bucket: cfg.Load().Bucket, logger: logger}
}

// PresignUpload 预签名上行（A.4 #1）：限制矩阵预检 → 服务端生成 key →
// 落 pending → 签发钉死 Content-Type 的 PUT URL。
// 禁用态（storage.enabled=false——装配侧注入 nil client）显式拒绝而非 panic。
func (s *Service) PresignUpload(ctx context.Context, in *model.PresignUploadInput, userID int64) (*model.PresignUploadResponse, error) {
	if s.client == nil {
		return nil, errcode.New(errcode.ErrInternal.Code, "对象存储未启用（storage.enabled=false）")
	}
	purpose := strings.TrimSpace(in.Purpose)
	mime := strings.ToLower(strings.TrimSpace(in.MIMEType))
	cfg := s.cfg.Load() // 快照只读（F-D3）

	if _, ok := purposePrefixes[purpose]; !ok {
		return nil, errcode.New(errcode.ErrInvalidParams.Code, "purpose 非法")
	}
	if ticketPurposes[purpose] && in.TicketID == nil {
		return nil, errcode.New(errcode.ErrInvalidParams.Code, "ticket 态 purpose 必带 ticket_id")
	}
	if !MIMEAllowed(mime, cfg.AllowedMIMETypes, cfg.BlockedMIMETypes) {
		return nil, errcode.ErrFileTypeNotAllowed
	}
	if in.SizeBytes <= 0 || in.SizeBytes > cfg.MaxUploadBytes {
		return nil, errcode.ErrFileTooLarge
	}
	fileName := strings.TrimSpace(in.FileName)
	if fileName == "" || len([]rune(fileName)) > maxFileNameRunes {
		return nil, errcode.New(errcode.ErrInvalidParams.Code, "file_name 非法（1–255 字符）")
	}

	key, err := KeyFor(purpose, mime)
	if err != nil {
		return nil, err
	}
	f := &model.FileObject{
		Bucket: s.bucket, ObjectKey: key, Purpose: purpose,
		Status: model.FileStatusPending, FileName: fileName, MIMEType: mime, CreatedBy: userID,
	}
	if err := s.repo.InsertPending(ctx, f); err != nil {
		s.logger.Error("storage insert pending", "err", err)
		return nil, errcode.ErrInternal
	}

	ttl, err := cfg.TTL()
	if err != nil {
		return nil, errcode.New(errcode.ErrInvalidParams.Code, "presign ttl 非法")
	}
	pp, err := s.client.PresignPut(ctx, key, mime, ttl)
	if err != nil {
		s.logger.Error("storage presign put", "err", err)
		return nil, errcode.ErrInternal
	}
	return &model.PresignUploadResponse{
		UploadURL: pp.URL, Method: "PUT", Headers: pp.Headers,
		ObjectKey: key, ExpiresAt: pp.ExpiresAt,
	}, nil
}

// Confirm 确认上行（A.4 #2 底座态）：归属/状态校验 → HEAD 实测（size/类型/etag）
// → 条件更新 pending→confirmed。ticket 关联落绑随 B5-3 扩展。
func (s *Service) Confirm(ctx context.Context, in *model.ConfirmFileInput, userID int64) (*model.FileObject, error) {
	if s.client == nil {
		return nil, errcode.New(errcode.ErrInternal.Code, "对象存储未启用（storage.enabled=false）")
	}
	f, err := s.repo.GetByObjectKey(ctx, s.bucket, in.ObjectKey)
	if err != nil {
		if errors.Is(err, repository.ErrStorageObjectNotFound) {
			return nil, errcode.ErrFileNotFound
		}
		s.logger.Error("storage get by key", "err", err)
		return nil, errcode.ErrInternal
	}
	// 防枚举：非本人对象与不存在同形 404（不透归属差异）
	if f.CreatedBy != userID {
		return nil, errcode.ErrFileNotFound
	}
	if f.Status != model.FileStatusPending {
		return nil, errcode.ErrFileAlreadyBound
	}
	// purpose↔key 前缀一致性（防同 key 跨 purpose 复用）
	if prefix, ok := purposePrefixes[f.Purpose]; !ok || !strings.HasPrefix(f.ObjectKey, prefix) {
		return nil, errcode.ErrFileAlreadyBound
	}

	st, err := s.client.StatObject(ctx, f.ObjectKey)
	if err != nil {
		if errors.Is(err, pkgstorage.ErrObjectNotFound) {
			return nil, errcode.ErrFileNotFound
		}
		s.logger.Error("storage stat object", "err", err)
		return nil, errcode.ErrInternal
	}
	// HEAD 实测双卡点：size 实测超限拒（谎报声明绕不过）；类型与签名钉死值一致
	cfg := s.cfg.Load()
	if st.Size > cfg.MaxUploadBytes {
		return nil, errcode.ErrFileTooLarge
	}
	if st.ContentType != "" && !strings.EqualFold(st.ContentType, f.MIMEType) {
		return nil, errcode.ErrFileTypeNotAllowed
	}

	// ticket 态分流（B5-3）：落绑关联行（repo 内单事务）；建单前态维持悬挂翻 confirmed
	if ticketPurposes[f.Purpose] {
		if in.TicketID == nil {
			return nil, errcode.New(errcode.ErrInvalidParams.Code, "ticket 态 confirm 必带 ticket_id")
		}
		if err := s.repo.ConfirmAndBind(ctx, f.ID, *in.TicketID, userID, st.Size, st.ETag, cfg.MaxAttachmentsPerTicket); err != nil {
			if errors.Is(err, repository.ErrStorageAlreadyBound) {
				return nil, errcode.ErrFileAlreadyBound
			}
			if errors.Is(err, repository.ErrStorageAttachmentLimit) {
				return nil, errcode.New(errcode.ErrInvalidParams.Code,
					fmt.Sprintf("工单附件数超过上限（%d）", cfg.MaxAttachmentsPerTicket))
			}
			s.logger.Error("storage confirm and bind", "err", err)
			return nil, errcode.ErrInternal
		}
		size := st.Size
		etag := st.ETag
		now := time.Now()
		f.Status = model.FileStatusConfirmed
		f.SizeBytes = &size
		f.ETag = &etag
		f.ConfirmedAt = &now
		// 归属已由 ConfirmAndBind 同语句落库（000047）——内存态同步供响应回显
		ownerKind, ownerID := model.FileOwnerTicket, strconv.FormatInt(*in.TicketID, 10)
		f.OwnerKind, f.OwnerID = &ownerKind, &ownerID
		return f, nil
	}

	if err := s.repo.Confirm(ctx, f.ID, st.Size, st.ETag); err != nil {
		if errors.Is(err, repository.ErrStorageAlreadyBound) {
			return nil, errcode.ErrFileAlreadyBound
		}
		s.logger.Error("storage confirm", "err", err)
		return nil, errcode.ErrInternal
	}
	size := st.Size
	etag := st.ETag
	now := time.Now()
	f.Status = model.FileStatusConfirmed
	f.SizeBytes = &size
	f.ETag = &etag
	f.ConfirmedAt = &now
	return f, nil
}

// ErrNotModified 客户端 If-None-Match 命中库内 etag 快照——代理侧直接 304
// （不触存储、不落审计行——A.4 #5 审计仅 200 全量投递落行）。
var ErrNotModified = errors.New("storage: not modified")

// Download 代理下载投递（body 流 + 元数据；handler 负责响应头与审计收窄）。
type Download struct {
	Body io.ReadCloser
	File *model.FileObject
}

// Download 代理下载（A.4 #5）：定位元数据 → 状态/purpose 面校验 →
// If-None-Match 命中快照则 304 哨兵 → GetObject 条件读（库内 etag 作
// If-Match——对象被重放替换即 ErrObjectReplaced，拍板 1b 防御默认开）。
// B5-2 支持面=avatar（登录态可见，A.7）；ticket 态 L2 管线随 B5-3 扩。
func (s *Service) Download(ctx context.Context, key, ifNoneMatch string, ticketAuthz func(ticketID int64) error) (*Download, error) {
	if s.client == nil {
		return nil, errcode.New(errcode.ErrInternal.Code, "对象存储未启用（storage.enabled=false）")
	}
	f, err := s.repo.GetByObjectKey(ctx, s.bucket, key)
	if err != nil {
		if errors.Is(err, repository.ErrStorageObjectNotFound) {
			return nil, errcode.ErrFileNotFound
		}
		s.logger.Error("storage get by key", "err", err)
		return nil, errcode.ErrInternal
	}
	// 同形 404：pending（未确认）/ deleted / 非支持面——不泄露状态差异
	if f.Status != model.FileStatusConfirmed || (f.Purpose != "avatar" && !ticketPurposes[f.Purpose]) {
		return nil, errcode.ErrFileNotFound
	}
	// ticket 态：须已绑定工单 + L2 回调判定（A.7 下载=工单 L2 可见性）
	if ticketPurposes[f.Purpose] {
		ticketID, err := s.repo.GetBoundTicketID(ctx, s.bucket, key)
		if err != nil {
			if errors.Is(err, repository.ErrStorageObjectNotFound) {
				return nil, errcode.ErrFileNotFound
			}
			// DB 故障不再吞成 404（S5 评审 P2-2 service 层残留——修复验证
			// 审计 M-1：repo 层分流被本消费点抵消，端到端仍静默 404 且无留痕）
			s.logger.Error("storage get bound ticket", "key", key, "err", err)
			return nil, errcode.ErrInternal
		}
		if ticketAuthz != nil {
			if err := ticketAuthz(ticketID); err != nil {
				return nil, err
			}
		}
	}
	if ifNoneMatch != "" && f.ETag != nil && ifNoneMatch == *f.ETag {
		return nil, ErrNotModified
	}
	var matchETag string
	if f.ETag != nil {
		matchETag = *f.ETag
	}
	body, _, err := s.client.GetObject(ctx, key, matchETag)
	if err != nil {
		if errors.Is(err, pkgstorage.ErrObjectReplaced) {
			s.logger.Error("storage object replaced after confirm（重放替换嫌疑）", "key", key)
			return nil, errcode.ErrFileNotFound
		}
		if errors.Is(err, pkgstorage.ErrObjectNotFound) {
			return nil, errcode.ErrFileNotFound
		}
		s.logger.Error("storage get object", "err", err)
		return nil, errcode.ErrInternal
	}
	return &Download{Body: body, File: f}, nil
}

// BindAvatar 头像回写（A.4 #7——B5-2 契约新增行）：校验对象
// （confirmed+本人+avatar 面，非皆同形 404 防枚举）→ users.avatar 落对象键
// → 旧头像对象 best-effort 删除+标 deleted（A.7 弃旧语义）；同键重复 bind 幂等。
func (s *Service) BindAvatar(ctx context.Context, objectKey string, userID int64) (*model.FileObject, error) {
	if s.client == nil {
		return nil, errcode.New(errcode.ErrInternal.Code, "对象存储未启用（storage.enabled=false）")
	}
	f, err := s.repo.GetByObjectKey(ctx, s.bucket, objectKey)
	if err != nil {
		if errors.Is(err, repository.ErrStorageObjectNotFound) {
			return nil, errcode.ErrFileNotFound
		}
		s.logger.Error("storage get by key", "err", err)
		return nil, errcode.ErrInternal
	}
	if f.CreatedBy != userID || f.Status != model.FileStatusConfirmed || f.Purpose != "avatar" {
		return nil, errcode.ErrFileNotFound
	}

	old, err := s.repo.GetUserAvatar(ctx, userID)
	if err != nil {
		s.logger.Error("storage get user avatar", "err", err)
		return nil, errcode.ErrInternal
	}
	// 归属先于回写（000047 随行件）：对象置 owner=user 后再写 users.avatar——若后者失败，
	// 留下的只是"已归属本人但未挂为头像"的 confirmed 对象（无害）；反序则可能出现挂为头像
	// 却无归属的行，未来按 NULL=未归属的对账/GC 消费者会误判。
	ownerID := strconv.FormatInt(userID, 10)
	if err := s.repo.SetObjectOwner(ctx, s.bucket, objectKey, model.FileOwnerUser, ownerID); err != nil {
		s.logger.Error("storage set avatar owner", "key", objectKey, "err", err)
		return nil, errcode.ErrInternal
	}
	if err := s.repo.UpdateUserAvatar(ctx, userID, objectKey); err != nil {
		s.logger.Error("storage update user avatar", "err", err)
		return nil, errcode.ErrInternal
	}
	ownerKind := model.FileOwnerUser
	f.OwnerKind, f.OwnerID = &ownerKind, &ownerID
	// 弃旧：先回写新键成功再处理旧对象（标 deleted 行态优先→物理删 best-effort，
	// 失败残留由 GC 二扫收——与 DeleteTicketAttachment 同款口径，2026-10-20 对齐）
	if old != "" && old != objectKey {
		if err := s.repo.MarkAvatarDropped(ctx, s.bucket, old); err != nil {
			s.logger.Error("storage mark avatar dropped", "key", old, "err", err)
		}
		if err := s.client.RemoveObject(ctx, old); err != nil {
			s.logger.Warn("storage drop old avatar remove deferred to gc", "key", old, "err", err)
		}
	}
	return f, nil
}

// ListTicketAttachments 附件列表视图（B5-3 批3；L2 read 面由 handler 编排）。
func (s *Service) ListTicketAttachments(ctx context.Context, ticketID int64) ([]*model.TicketAttachmentView, error) {
	if s.repo == nil {
		return nil, errcode.New(errcode.ErrInternal.Code, "对象存储未启用（storage.enabled=false）")
	}
	list, err := s.repo.ListAttachmentsByTicket(ctx, ticketID)
	if err != nil {
		s.logger.Error("storage list attachments", "err", err)
		return nil, errcode.ErrInternal
	}
	return list, nil
}

// DeleteTicketAttachment 删附件（A.4 #4：删关联 → 行翻 deleted（行态优先）→
// 对象物理删 best-effort——失败残留由 GC 二扫收（2026-10-20 删除口径对齐：
// 原「删本体+标 deleted」顺序在物理删失败时对象无扫描覆盖=永久泄漏）。
// L2 update 面由 handler 编排。
func (s *Service) DeleteTicketAttachment(ctx context.Context, ticketID int64, objectKey string) error {
	if s.client == nil {
		return errcode.New(errcode.ErrInternal.Code, "对象存储未启用（storage.enabled=false）")
	}
	// 拍板 P3（2026-10-21）：richtext 内嵌图随工单生命周期——不单独删（A.7）。
	// 误删即 MD 正文死链且对象物理删后无恢复路径；正确姿势=从正文移除引用，
	// 对象由对账扫描家族收（A.3 第三条登记）。
	f, err := s.repo.GetByObjectKey(ctx, s.bucket, objectKey)
	if err != nil {
		if errors.Is(err, repository.ErrStorageObjectNotFound) {
			return errcode.ErrFileNotFound
		}
		s.logger.Error("storage get object", "err", err)
		return errcode.ErrInternal
	}
	if f.Purpose == "ticket_richtext" {
		return errcode.New(errcode.ErrInvalidParams.Code,
			"内嵌图随工单生命周期管理，不单独删除（从正文移除引用即可，对象由对账扫描回收）")
	}
	if err := s.repo.UnlinkAttachment(ctx, s.bucket, ticketID, objectKey); err != nil {
		if errors.Is(err, repository.ErrStorageObjectNotFound) {
			return errcode.ErrFileNotFound
		}
		s.logger.Error("storage unlink attachment", "err", err)
		return errcode.ErrInternal
	}
	// F-A6（2026-10-21 审计）：Mark 失败降级 Warn 不返 500——unlink 已成功
	//（用户视角删除完成），返 500 反致重试撞 404；行残留 confirmed 无关联由
	// 对账扫描家族收（与 confirmed 孤儿同族）
	if err := s.repo.MarkObjectDeleted(ctx, s.bucket, objectKey); err != nil {
		s.logger.Warn("storage mark object deleted deferred to reconciliation", "key", objectKey, "err", err)
	}
	if err := s.client.RemoveObject(ctx, objectKey); err != nil {
		s.logger.Warn("storage remove attachment object deferred to gc", "key", objectKey, "err", err)
	}
	return nil
}

// InlineDisposition 代理下载 Content-Disposition 分型（inline 前缀表判定）。
func (s *Service) InlineDisposition(mime string) bool {
	for _, p := range s.cfg.Load().InlineMIMEPrefixes {
		if strings.HasPrefix(strings.ToLower(mime), p) {
			return true
		}
	}
	return false
}

// MIMEAllowed 白名单显式枚举（支持尾部 `.*` 通配）∧ 不在黑名单。
func MIMEAllowed(mime string, allowed, blocked []string) bool {
	mime = strings.ToLower(strings.TrimSpace(mime))
	if mime == "" {
		return false
	}
	for _, b := range blocked {
		if strings.EqualFold(b, mime) {
			return false
		}
	}
	for _, a := range allowed {
		a = strings.ToLower(a)
		if a == mime || (strings.HasSuffix(a, ".*") && strings.HasPrefix(mime, strings.TrimSuffix(a, "*"))) {
			return true
		}
	}
	return false
}

// KeyFor 服务端生成对象键：{domain}/{yyyy}/{mm}/{uuid}.{ext}（禁止客户端自选路径）。
func KeyFor(purpose, mime string) (string, error) {
	prefix, ok := purposePrefixes[purpose]
	if !ok {
		return "", errcode.New(errcode.ErrInvalidParams.Code, "purpose 非法")
	}
	ext, ok := mimeExt[strings.ToLower(strings.TrimSpace(mime))]
	if !ok {
		return "", errcode.ErrFileTypeNotAllowed
	}
	now := time.Now()
	return fmt.Sprintf("%s%04d/%02d/%s.%s", prefix, now.Year(), int(now.Month()), uuid.NewString(), ext), nil
}
