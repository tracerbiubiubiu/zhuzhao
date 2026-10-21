// Package service B5-6 配置化设置服务（05 号 §3——三层：config 出厂默认 → KV 运行时覆盖 → 管理面 UI）。
// 语义进代码：catalog（可配键+编辑层级+硬底线校验）不进 DB；单实例 in-memory apply（无多副本部署前提）。
package service

import (
	"context"
	"encoding/json"
	"errors"
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
	{
		// 05 号 §3 可配四类之一（大小/个数/MIME/孤儿 GC 天数）——S5 收口后评审
		// 补批 2026-10-21：首版 catalog 漏此键（GC 阈值只能 config 重启改）。
		Key: "storage.orphan_gc_days", MinTier: "superadmin",
		Description: "孤儿对象 GC 阈值天数（pending 超此天数未 confirm 即回收）",
		Validate: func(v json.RawMessage) error {
			var n int
			if err := json.Unmarshal(v, &n); err != nil || n < 1 || n > 365 {
				return fmt.Errorf("须为 1–365 整数")
			}
			return nil
		},
		Apply: func(v json.RawMessage, c *storcfg.StorageConfig) {
			var n int
			if json.Unmarshal(v, &n) == nil {
				c.OrphanGCDays = n
			}
		},
	},
}

// SettingsService 设置管理服务（B5-6——GET 列表 + POST 更新，tier 分层+硬底线钳制）。
type SettingsService struct {
	repo   *repository.SettingsRepo
	cfg    *storcfg.ConfigStore // F-D3（2026-10-21）：原子快照——clone→改→Store，与请求侧读并发安全
	logger *slog.Logger
}

// NewSettingsService 创建时读全量 KV 覆盖并 apply（boot 路径——三层 ② 优先于 ①）。
func NewSettingsService(repo *repository.SettingsRepo, cfg *storcfg.ConfigStore, logger *slog.Logger) *SettingsService {
	if logger == nil {
		logger = slog.Default()
	}
	s := &SettingsService{repo: repo, cfg: cfg, logger: logger}
	if repo != nil {
		s.applyAllOverrides(context.Background())
	}
	return s
}

// applyAllOverrides 启动路径：读全量 KV → 逐键 Validate（硬底线在 boot 路径同样
// 设防——DB 被手工写入越界值时跳过保 config 默认，分批审计 F-D1 修复 2026-10-21）
// → clone→Apply→Store 整体替换快照（F-D3——不再原地 mutate 共享指针）。
func (s *SettingsService) applyAllOverrides(ctx context.Context) {
	kvs, err := s.repo.ListAll(ctx)
	if err != nil {
		s.logger.Warn("settings: boot apply overrides failed (config defaults active)", "err", err)
		return
	}
	merged := *s.cfg.Load()
	applied, skipped := 0, 0
	for _, sk := range settingsCatalog {
		if kv, ok := kvs[sk.Key]; ok && kv != nil {
			if err := sk.Validate(kv.Value); err != nil {
				skipped++
				s.logger.Warn("settings: boot skip invalid KV (config default active)",
					"key", sk.Key, "err", err)
				continue
			}
			sk.Apply(kv.Value, &merged)
			applied++
		}
	}
	if applied > 0 {
		s.cfg.Store(&merged)
		s.logger.Info("settings: boot applied KV overrides", "count", applied)
	}
	if skipped > 0 {
		s.logger.Warn("settings: boot skipped invalid KV overrides", "count", skipped)
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
			v.Version = kv.Version // F-D2：乐观锁期望值（前端回传 expected_version 用）
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
// 审计双留痕：①POST 经 authed 级通用审计中间件（who/when/path/新值 body）；
// ②旧值→新值由本处日志补位（A.7「旧值→新值」——中间件 body 只含新值，评审补批 2026-10-21）。
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
	// 旧值快照（upsert 前读——A.7 变更审计的旧值侧；首次更新记 -）
	oldVal := "-"
	if old, err := s.repo.Get(ctx, req.Key); err != nil {
		s.logger.Warn("settings: read old value failed (audit old_value=-)", "key", req.Key, "err", err)
	} else if old != nil {
		oldVal = string(old.Value)
	}
	// KV upsert（F-D2：version 乐观锁——expected_version 非空时严格 CAS）
	if err := s.repo.Upsert(ctx, req.Key, req.Value, actorUserID, req.ExpectedVersion); err != nil {
		if errors.Is(err, repository.ErrSettingsVersionConflict) {
			return errcode.New(errcode.ErrInvalidParams.Code, "设置已被并发修改，请刷新后重试")
		}
		return err
	}
	// in-memory apply（F-D3：clone→改→Store 整体换快照——并发读安全）
	merged := *s.cfg.Load()
	sk.Apply(req.Value, &merged)
	s.cfg.Store(&merged)
	s.logger.Info("setting updated", "key", req.Key, "by", actorUserID,
		"old_value", oldVal, "new_value", string(req.Value))
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
	c := s.cfg.Load()
	switch key {
	case "storage.max_upload_bytes":
		b, _ := json.Marshal(c.MaxUploadBytes)
		return b
	case "storage.max_attachments_per_ticket":
		b, _ := json.Marshal(c.MaxAttachmentsPerTicket)
		return b
	case "storage.max_presign_batch":
		b, _ := json.Marshal(c.MaxPresignBatch)
		return b
	case "storage.allowed_mime_types":
		b, _ := json.Marshal(c.AllowedMIMETypes)
		return b
	case "storage.orphan_gc_days":
		b, _ := json.Marshal(c.OrphanGCDays)
		return b
	}
	return json.RawMessage(`null`)
}
