package casbin

import (
	"fmt"
	"os"

	"github.com/casbin/casbin/v3"
	"github.com/casbin/casbin/v3/model"
	"github.com/jackc/pgx/v5/pgxpool"
	pgxadapter "github.com/noho-digital/casbin-pgx-adapter"

	"github.com/tracerbiubiubiu/zhuzhao/internal/config"
)

// LoadModel returns the parsed model and the same original definition for
// authorization checks that reload policy from PostgreSQL at use time.
func LoadModel(path string) (model.Model, string, error) {
	source, err := os.ReadFile(path)
	if err != nil {
		return nil, "", fmt.Errorf("load casbin model: %w", err)
	}
	definition := string(source)
	m, err := model.NewModelFromString(definition)
	if err != nil {
		return nil, "", fmt.Errorf("load casbin model: %w", err)
	}
	return m, definition, nil
}

// New 创建 Casbin SyncedEnforcer（PG adapter，启动时 LoadPolicy）
func New(cfg config.CasbinConfig, pool *pgxpool.Pool) (*casbin.SyncedEnforcer, string, func(), error) {
	m, definition, err := LoadModel(cfg.Model)
	if err != nil {
		return nil, "", nil, err
	}

	adapter, err := pgxadapter.NewAdapterWithPool(pool)
	if err != nil {
		return nil, "", nil, fmt.Errorf("create casbin adapter: %w", err)
	}

	enforcer, err := casbin.NewSyncedEnforcer(m, adapter)
	if err != nil {
		return nil, "", nil, fmt.Errorf("create casbin enforcer: %w", err)
	}
	if err := enforcer.LoadPolicy(); err != nil {
		return nil, "", nil, fmt.Errorf("load casbin policy: %w", err)
	}

	cleanup := func() {
		enforcer.StopAutoLoadPolicy()
	}
	return enforcer, definition, cleanup, nil
}
