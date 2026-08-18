package casbin

import (
	"fmt"

	"github.com/casbin/casbin/v2"
	"github.com/casbin/casbin/v2/model"
	"github.com/jackc/pgx/v5/pgxpool"
	pgxadapter "github.com/pckhoi/casbin-pgx-adapter/v3"

	"github.com/tracerbiubiubiu/zhuzhao/internal/config"
)

// New 创建 Casbin SyncedEnforcer（PG adapter，启动时 LoadPolicy）
func New(cfg config.CasbinConfig, pool *pgxpool.Pool) (*casbin.SyncedEnforcer, func(), error) {
	m, err := model.NewModelFromFile(cfg.Model)
	if err != nil {
		return nil, nil, fmt.Errorf("load casbin model: %w", err)
	}

	adapter, err := pgxadapter.NewAdapter("",
		pgxadapter.WithConnectionPool(pool),
		pgxadapter.WithSkipTableCreate(),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("create casbin adapter: %w", err)
	}

	enforcer, err := casbin.NewSyncedEnforcer(m, adapter)
	if err != nil {
		return nil, nil, fmt.Errorf("create casbin enforcer: %w", err)
	}
	if err := enforcer.LoadPolicy(); err != nil {
		return nil, nil, fmt.Errorf("load casbin policy: %w", err)
	}

	cleanup := func() {
		enforcer.StopAutoLoadPolicy()
	}
	return enforcer, cleanup, nil
}
