// Package service B5-6 配置化设置服务（05 号 §3——三层：config 出厂默认 → KV 运行时覆盖 → 管理面 UI）。
// 语义进代码：catalog（可配键+编辑层级+硬底线校验）不进 DB；单实例 in-memory apply（无多副本部署前提）。
package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	storcfg "github.com/tracerbiubiubiu/zhuzhao/internal/pkg/storage"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

// settingKey catalog 条目（代码目录——不进 DB，§25.3 语义进代码）。
type settingKey struct {
	Key         string
	MinTier     string // superadmin | admin（存储限制类默认 superadmin-only）
	Description string
	Validate    func(json.RawMessage) error                   // 硬底线校验（越界 400 非静默修正）
	Apply       func(json.RawMessage, *storcfg.StorageConfig) // in-memory 生效（单实例）
}

// settingsCatalog 已注册可配键全集（05 号 §3 拍板范围）。
var settingsCatalog = []settingKey{
	{
		Key: "storage.max_upload_bytes", MinTier: "superadmin",
		Description: "单文件上传上限（字节；硬天花板 100MB 不可配）",
		Validate: func(v json.RawMessage) error {
			var n int64
			if err := json.Unmarshal(v, &n); err != nil || n < 1 || n > storcfg.MaxUploadBytesCeil {
				return fmt.Errorf("须为 1–%d 整数（当前天花板 100MB）", storcfg.MaxUploadBytesCeil)
			}
			return nil
		},
		Apply: func(v json.RawMessage, c *storcfg.StorageConfig) {
			var n int64
			if json.Unmarshal(v, &n) == nil {
				c.MaxUploadBytes = n
			}
		},
	},
	{
		Key: "storage.max_attachments_per_ticket", MinTier: "superadmin",
		Description: "单工单附件数上限（硬天花板 100）",
		Validate: func(v json.RawMessage) error {
			var n int
			if err := json.Unmarshal(v, &n); err != nil || n < 1 || n > 100 {
				return fmt.Errorf("须为 1–100 整数")
			}
			return nil
		},
		Apply: func(v json.RawMessage, c *storcfg.StorageConfig) {
			var n int
			if json.Unmarshal(v, &n) == nil {
				c.MaxAttachmentsPerTicket = n
			}
		},
	},
	{
		Key: "storage.max_presign_batch", MinTier: "superadmin",
		Description: "单次预签名批数上限（硬天花板 100）",
		Validate: func(v json.RawMessage) error {
			var n int
			if err := json.Unmarshal(v, &n); err != nil || n < 1 || n > 100 {
				return fmt.Errorf("须为 1–100 整数")
			}
			return nil
		},
		Apply: func(v json.RawMessage, c *storcfg.StorageConfig) {
			var n int
			if json.Unmarshal(v, &n) == nil {
				c.MaxPresignBatch = n
			}
		},
	},
	{
		Key: "storage.allowed_mime_types", MinTier: "superadmin",
		Description: "允许上传的 MIME 类型白名单（黑名单不可配=硬底线）",
		Validate: func(v json.RawMessage) error {
			var arr []string
			if err := json.Unmarshal(v, &arr); err != nil || len(arr) == 0 {
				return fmt.Errorf("须为非空字符串数组")
			}
			blocked := map[string]bool{"text/html": true, "image/svg+xml": true}
			for _, m := range arr {
				if blocked[m] {
					return fmt.Errorf("%q 在黑名单内，不可加入白名单（硬底线）", m)
				}
				if !strings.Contains(m, "/") {
					return fmt.Errorf("%q 非合法 MIME 类型", m)
				}
			}
			return nil
		},
		Apply: func(v json.RawMessage, c *storcfg.StorageConfig) {
			var arr []string
			if json.Unmarshal(v, &arr) == nil && len(arr) > 0 {
				c.AllowedMIMETypes = arr
			}
		},
	},
}

// SettingsService 设置管理服务（B5-6——GET 列表 + POST 更新，tier 分层+硬底线钳制）。
type SettingsService struct {
	repo   *repository.SettingsRepo
	cfg    *storcfg.StorageConfig // 指针——mutate 实现运行时覆盖（单实例）
	logger *slog.Logger
}

// NewSettingsService 创建时读全量 KV 覆盖并 apply（boot 路径——三层 ② 优先于 ①）。
func NewSettingsService(repo *repository.SettingsRepo, cfg *storcfg.StorageConfig, logger *slog.Logger) *SettingsService {
	if logger == nil {
		logger = slog.Default()
	}
	s := &SettingsService{repo: repo, cfg: cfg, logger: logger}
	if repo != nil {
		s.applyAllOverrides(context.Background())
	}
	return s
}

// applyAllOverrides 启动路径：读全量 KV → 按 catalog Apply 到 config 指针。
func (s *SettingsService) applyAllOverrides(ctx context.Context) {
	kvs, err := s.repo.ListAll(ctx)
	if err != nil {
		s.logger.Warn("settings: boot apply overrides failed (config defaults active)", "err", err)
		return
	}
	applied := 0
	for _, sk := range settingsCatalog {
		if kv, ok := kvs[sk.Key]; ok && kv != nil {
			sk.Apply(kv.Value, s.cfg)
			applied++
		}
	}
	if applied > 0 {
		s.logger.Info("settings: boot applied KV overrides", "count", applied)
	}
}

// List 管理面列表：返回全部 catalog 键的有效值（KV > config）+ 来源 + 编辑层级。
func (s *SettingsService) List(ctx context.Context) ([]model.SettingView, error) {
	kvs, err := s.repo.ListAll(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]model.SettingView, 0, len(settingsCatalog))
	for _, sk := range settingsCatalog {
		v := model.SettingView{Key: sk.Key, MinEditorTier: sk.MinTier, Description: sk.Description, Source: "config"}
		if kv, ok := kvs[sk.Key]; ok && kv != nil {
			v.Value = kv.Value
			v.Source = "kv"
			v.UpdatedBy = kv.UpdatedBy
			v.UpdatedAt = &kv.UpdatedAt
		} else {
			v.Value = s.defaultValue(sk.Key)
		}
		out = append(out, v)
	}
	return out, nil
}

// Update 更新设置：tier 校验 → 硬底线 Validate → KV upsert → in-memory Apply。
// 审计：POST 经 authed 级通用审计中间件自动留痕（who/when/old→new body）。
func (s *SettingsService) Update(ctx context.Context, req *model.UpdateSettingRequest, actorUserID int64, actorRoles []string) error {
	var sk *settingKey
	for i := range settingsCatalog {
		if settingsCatalog[i].Key == req.Key {
			sk = &settingsCatalog[i]
			break
		}
	}
	if sk == nil {
		return errcode.New(errcode.ErrInvalidParams.Code, "未知设置键 "+req.Key)
	}
	// 编辑层级校验（05 号 §3——存储限制类默认 superadmin-only，admin 无写路径）
	if !tierAllowed(sk.MinTier, actorRoles) {
		return errcode.New(errcode.ErrNoPermission.Code,
			fmt.Sprintf("该设置需 %s 层级（当前角色无权修改）", sk.MinTier))
	}
	// 硬底线校验（越界 400 非静默修正——不自动钳到边界）
	if err := sk.Validate(req.Value); err != nil {
		return errcode.New(errcode.ErrInvalidParams.Code, fmt.Sprintf("值非法：%s", err))
	}
	// KV upsert
	if err := s.repo.Upsert(ctx, req.Key, req.Value, actorUserID); err != nil {
		return err
	}
	// in-memory apply（运行时生效——单实例）
	sk.Apply(req.Value, s.cfg)
	s.logger.Info("setting updated", "key", req.Key, "by", actorUserID)
	return nil
}

// tierAllowed 编辑层级判定（min_editor_tier——静态分层，无运行时锁）。
func tierAllowed(minTier string, roles []string) bool {
	hasAdmin := false
	for _, r := range roles {
		if r == "superadmin" {
			return true
		}
		if r == "admin" {
			hasAdmin = true
		}
	}
	return minTier == "admin" && hasAdmin
}

// defaultValue 从 config 读出厂默认（用于列表展示 Source=config 行）。
func (s *SettingsService) defaultValue(key string) json.RawMessage {
	switch key {
	case "storage.max_upload_bytes":
		b, _ := json.Marshal(s.cfg.MaxUploadBytes)
		return b
	case "storage.max_attachments_per_ticket":
		b, _ := json.Marshal(s.cfg.MaxAttachmentsPerTicket)
		return b
	case "storage.max_presign_batch":
		b, _ := json.Marshal(s.cfg.MaxPresignBatch)
		return b
	case "storage.allowed_mime_types":
		b, _ := json.Marshal(s.cfg.AllowedMIMETypes)
		return b
	}
	return json.RawMessage(`null`)
}
