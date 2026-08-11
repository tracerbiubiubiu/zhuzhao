package casbin

import (
	"fmt"

	"github.com/casbin/casbin/v2"
	"github.com/casbin/casbin/v2/model"

	"github.com/tracerbiubiubiu/zhuzhao/internal/config"
)

// New 创建 Casbin SyncedEnforcer
// 当前阶段使用内存 adapter，后续切换为 PostgreSQL adapter
func New(cfg config.CasbinConfig) (*casbin.SyncedEnforcer, error) {
	m, err := model.NewModelFromFile(cfg.Model)
	if err != nil {
		return nil, fmt.Errorf("failed to load casbin model: %w", err)
	}

	enforcer, err := casbin.NewSyncedEnforcer(m)
	if err != nil {
		return nil, fmt.Errorf("failed to create casbin enforcer: %w", err)
	}

	// TODO: 后续切换为 PostgreSQL adapter
	// adapter, err := pgadapter.NewAdapter(...)
	// enforcer.SetAdapter(adapter)
	// if err := enforcer.LoadPolicy(); err != nil { ... }

	return enforcer, nil
}
