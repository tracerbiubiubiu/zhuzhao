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
	// ticketCmd C04 撤权提交顺序边界：票态附件写入的 IAM 共同共享守卫+
	// 锁票重认证由 ticket 域实现（域间零 import——装配层结构化注入，A.7
	// handler 编排先例）。nil = 既有直连契约（非 HTTP 调用方/既有测试）。
	ticketCmd          TicketAttachmentCommand
	riskEvidenceRoutes func(context.Context, int64, []int64) (map[int64]bool, error)
}

// TicketAttachmentCommand ticket 域守卫边界（实现方=ticket.Service）。fn 在
// 守卫事务内执行——repo 经 repository.IAMDB 取同一连接（savepoint），不取
// 第二个池连接。
type TicketAttachmentCommand interface {
	AttachmentCommand(ctx context.Context, action string, actorUserID, ticketID int64, fn func(context.Context) error) error
}

// TicketAttachmentUploadCommand is an optional, purpose-specific evidence port.
// Richtext and deletion always retain TicketAttachmentCommand's update contract.
type TicketAttachmentUploadCommand interface {
	AttachmentUploadCommand(context.Context, string, int64, int64, *model.FileObject, func(context.Context) error) error
}

// HasTicketAttachmentCommand lets the handler defer upload authorization to the
// service, where the stored purpose is known and preflight can precede object I/O.
func (s *Service) HasTicketAttachmentCommand() bool { return s.ticketCmd != nil }

func (s *Service) runUploadCommand(ctx context.Context, action string, actor, ticketID int64, f *model.FileObject, write func(context.Context) error) error {
	if f.Purpose == "ticket_attachment" {
		if cmd, ok := s.ticketCmd.(TicketAttachmentUploadCommand); ok {
			return cmd.AttachmentUploadCommand(ctx, action, actor, ticketID, f, write)
		}
	}
	return s.runAttachmentCommand(ctx, action, actor, ticketID, write)
}

// UseTicketAttachmentCommand 装配层注入守卫实现（handler 构造时机——一次性，
// 非请求路径）。
func (s *Service) UseTicketAttachmentCommand(cmd TicketAttachmentCommand) {
	s.ticketCmd = cmd
}

// runAttachmentCommand 票态附件写入路由：已注入守卫时走 ticket 域守卫
// （errcode 面透传 handler），否则直连执行（既有契约）。
func (s *Service) runAttachmentCommand(ctx context.Context, action string, actorUserID, ticketID int64, fn func(context.Context) error) error {
	if s.ticketCmd == nil {
		return fn(ctx)
	}
	return s.ticketCmd.AttachmentCommand(ctx, action, actorUserID, ticketID, fn)
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
	insert := func(ctx context.Context) error { return s.repo.InsertPending(ctx, f) }
	if ticketPurposes[purpose] {
		err = s.runUploadCommand(ctx, "presign", userID, *in.TicketID, f, insert)
	} else {
		err = insert(ctx)
	}
	if err != nil {
		var biz *errcode.Error
		if errors.As(err, &biz) {
			return nil, err
		}
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
	if ticketPurposes[f.Purpose] {
		if in.TicketID == nil {
			return nil, errcode.New(errcode.ErrInvalidParams.Code, "ticket 态 confirm 必带 ticket_id")
		}
		// A current preflight precedes external HEAD; the persistent command below
		// rechecks the same purpose and facts after HEAD, in its own IAM ordering.
		if err := s.runUploadCommand(ctx, "confirm", userID, *in.TicketID, f, func(context.Context) error { return nil }); err != nil {
			return nil, err
		}
	} else if in.TicketID != nil {
		// Preserve the handler's original update check even for a create/avatar
		// object presented with ticket_id. Its stored purpose grants no evidence port.
		if err := s.runAttachmentCommand(ctx, "confirm", userID, *in.TicketID, func(context.Context) error { return nil }); err != nil {
			return nil, err
		}
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
		// C04 撤权提交顺序：HEAD 实测已在 IAM 锁外完成（外部动作不进锁），
		// 落绑在守卫内锁票重认证后同连接提交。
		if err := s.runUploadCommand(ctx, "confirm", userID, *in.TicketID, f, func(ctx context.Context) error {
			return s.repo.ConfirmAndBind(ctx, f.ID, *in.TicketID, userID, st.Size, st.ETag, cfg.MaxAttachmentsPerTicket)
		}); err != nil {
			if errors.Is(err, repository.ErrStorageAlreadyBound) {
				return nil, errcode.ErrFileAlreadyBound
			}
			if errors.Is(err, repository.ErrStorageAttachmentLimit) {
				return nil, errcode.New(errcode.ErrInvalidParams.Code,
					fmt.Sprintf("工单附件数超过上限（%d）", cfg.MaxAttachmentsPerTicket))
			}
			var biz *errcode.Error
			if errors.As(err, &biz) {
				return nil, err // 守卫 errcode 面（403/404/70003）透传 handler
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
func (s *Service) Download(ctx context.Context, key, ifNoneMatch string, callerUserID int64, ticketAuthz func(ticketID int64) error) (*Download, error) {
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
	// 同形 404：pending（未确认）/ deleted / 非支持面——不泄露状态差异。
	// richtext_create 悬挂态对创建人本人放行（2026-11-03 贴图批——建单前编辑期
	// 预览：自己的草稿图自己可见；他人/未登录仍同形 404 防枚举不变；未转正
	// 悬挂对象由 GC 对账扫描收尾，兜底不变）
	if f.Status != model.FileStatusConfirmed ||
		!(f.Purpose == "avatar" || ticketPurposes[f.Purpose] ||
			(f.Purpose == "richtext_create" && f.CreatedBy == callerUserID)) {
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
		hidden, err := s.repo.IsHiddenRiskEvidence(ctx, key, callerUserID)
		if err != nil {
			s.logger.Error("storage evidence authorization", "err", err)
			return nil, errcode.ErrInternal
		}
		if hidden {
			return nil, errcode.ErrFileNotFound
		}
		hiddenRoutes, err := s.hiddenEvidenceRoutes(ctx, callerUserID, []string{key})
		if err != nil {
			s.logger.Error("storage evidence route authorization", "err", err)
			return nil, errcode.ErrInternal
		}
		if hiddenRoutes[key] {
			return nil, errcode.ErrFileNotFound
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
	if s.client == nil || !s.cfg.Load().Enabled {
		return nil, errcode.New(errcode.ErrInternal.Code, "对象存储未启用（storage.enabled=false）")
	}
	objectKey, _ = repository.NormalizeAvatar(objectKey)
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

	// 用户行串行化多次绑定；对象状态、归属、新引用和旧头像退役同事务提交。
	// GC 若先锁定并退役新键，本次绑定返回同形 404，绝不把 deleted 对象挂给用户。
	old, err := s.repo.BindAvatar(ctx, s.bucket, objectKey, userID)
	if errors.Is(err, repository.ErrStorageObjectNotFound) {
		return nil, errcode.ErrFileNotFound
	}
	if err != nil {
		s.logger.Error("storage bind avatar", "key", objectKey, "err", err)
		return nil, errcode.ErrInternal
	}
	ownerKind, ownerID := model.FileOwnerUser, strconv.FormatInt(userID, 10)
	f.OwnerKind, f.OwnerID = &ownerKind, &ownerID
	// 仅处理事务内已退役的旧键。deleted 为终态，后续绑定无法重用该键，
	// 因此提交后的 best-effort 物理删除不会与另一请求的重新绑定产生 ABA。
	if old != "" {
		if err := s.client.RemoveObject(ctx, old); err != nil {
			s.logger.Warn("storage drop old avatar remove deferred to gc", "key", old, "err", err)
		}
	}
	return f, nil
}

// ListTicketAttachments 附件列表视图（B5-3 批3；L2 read 面由 handler 编排）。
func (s *Service) ListTicketAttachments(ctx context.Context, ticketID, actorID int64) ([]*model.TicketAttachmentView, error) {
	if s.repo == nil {
		return nil, errcode.New(errcode.ErrInternal.Code, "对象存储未启用（storage.enabled=false）")
	}
	list, err := s.repo.ListAttachmentsByTicket(ctx, ticketID, actorID)
	if err != nil {
		s.logger.Error("storage list attachments", "err", err)
		return nil, errcode.ErrInternal
	}
	keys := make([]string, 0, len(list))
	for _, attachment := range list {
		keys = append(keys, attachment.ObjectKey)
	}
	hidden, err := s.hiddenEvidenceRoutes(ctx, actorID, keys)
	if err != nil {
		s.logger.Error("storage evidence route authorization", "err", err)
		return nil, errcode.ErrInternal
	}
	visible := list[:0]
	for _, attachment := range list {
		if !hidden[attachment.ObjectKey] {
			visible = append(visible, attachment)
		}
	}
	return visible, nil
}

// DeleteTicketAttachment checks current references and atomically unlinks and
// marks deleted. 既有直连形态（签名兼容既有直连调用方与 ticket 包 C02 不变量
// 测试）：无 actor 不进守卫——HTTP 路径一律走 DeleteTicketAttachmentAs
// （C04 撤权提交顺序守卫）；本形态在已准入 HTTP ctx 下 fail-closed。
func (s *Service) DeleteTicketAttachment(ctx context.Context, ticketID int64, objectKey string) error {
	return s.deleteTicketAttachment(ctx, 0, ticketID, objectKey)
}

// DeleteTicketAttachmentAs C04 守卫形态：handler 供 actor，守卫在 IAM 共同
// 共享锁内锁票并同连接重认证后提交删关联+标删；物理对象删除仍在提交后。
func (s *Service) DeleteTicketAttachmentAs(ctx context.Context, actorUserID, ticketID int64, objectKey string) error {
	return s.deleteTicketAttachment(ctx, actorUserID, ticketID, objectKey)
}

func (s *Service) deleteTicketAttachment(ctx context.Context, actorUserID, ticketID int64, objectKey string) error {
	if s.client == nil {
		return errcode.New(errcode.ErrInternal.Code, "对象存储未启用（storage.enabled=false）")
	}
	err := s.runAttachmentCommand(ctx, "delete", actorUserID, ticketID, func(ctx context.Context) error {
		return s.repo.DeleteTicketAttachment(ctx, s.bucket, ticketID, objectKey)
	})
	switch {
	case errors.Is(err, repository.ErrStorageObjectNotFound):
		return errcode.ErrFileNotFound
	case errors.Is(err, repository.ErrStorageInlineAttachment):
		return errcode.New(errcode.ErrInvalidParams.Code, "内嵌图随工单生命周期管理，不单独删除（从正文移除引用即可，对象由对账扫描回收）")
	case errors.Is(err, repository.ErrStorageAttachmentReferenced):
		return errcode.New(errcode.ErrConflict.Code, "附件仍被引用，请先移除引用")
	case err != nil:
		var biz *errcode.Error
		if errors.As(err, &biz) {
			return err // 守卫 errcode 面（403/404/70003）透传 handler
		}
		s.logger.Error("storage delete attachment transaction", "err", err)
		return errcode.ErrInternal
	}
	// DB commit is complete. Failed physical cleanup retains a deleted tombstone
	// for the existing GC retry; it must never run inside the DB transaction.
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
