// Package storage 对象存储业务服务（phase5 S5 B5-1 底座——presign 限制矩阵执行
// 与 file_objects 状态机）。域边界：本包不 import 其他 service 域（架构守护
// TestArchitecture_ServiceDomainIsolation）；ticket_attachment/ticket_richtext
// purpose 的 L2 管线由 handler 层编排（随 B5-3 接线）。
package storage

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
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
	cfg    *pkgstorage.StorageConfig
	bucket string
	logger *slog.Logger
}

func NewService(repo *repository.StorageRepo, client pkgstorage.Client, cfg *pkgstorage.StorageConfig, logger *slog.Logger) *Service {
	return &Service{repo: repo, client: client, cfg: cfg, bucket: cfg.Bucket, logger: logger}
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

	if _, ok := purposePrefixes[purpose]; !ok {
		return nil, errcode.New(errcode.ErrInvalidParams.Code,
			fmt.Sprintf("purpose 非法（B5-1 支持 %s；ticket 态随 B5-3）", "avatar/attachment_create/richtext_create"))
	}
	if !MIMEAllowed(mime, s.cfg.AllowedMIMETypes, s.cfg.BlockedMIMETypes) {
		return nil, errcode.ErrFileTypeNotAllowed
	}
	if in.SizeBytes <= 0 || in.SizeBytes > s.cfg.MaxUploadBytes {
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

	ttl, err := s.cfg.TTL()
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
	if st.Size > s.cfg.MaxUploadBytes {
		return nil, errcode.ErrFileTooLarge
	}
	if st.ContentType != "" && !strings.EqualFold(st.ContentType, f.MIMEType) {
		return nil, errcode.ErrFileTypeNotAllowed
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
