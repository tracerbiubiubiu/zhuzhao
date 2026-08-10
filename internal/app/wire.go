//go:build wireinject
// +build wireinject

package app

import (
	"github.com/casbin/casbin/v2"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"log/slog"

	"github.com/tracerbiubiubiu/zhuzhao/internal/config"
	"github.com/tracerbiubiubiu/zhuzhao/internal/handler"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/jwt"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/logger"
	pgredis "github.com/tracerbiubiubiu/zhuzhao/internal/pkg/redis"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
	"github.com/tracerbiubiubiu/zhuzhao/internal/router"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service"

	"github.com/google/wire"
)

// Provider 集合

var pkgSet = wire.NewSet(
	logger.New,
	jwt.NewManager,
	pgredis.New,
)

var repoSet = wire.NewSet(
	repository.NewUserRepo,
	repository.NewRoleRepo,
	repository.NewOrgRepo,
	repository.NewMenuRepo,
	repository.NewAuditLogRepo,
)

var serviceSet = wire.NewSet(
	service.NewAuthService,
	service.NewUserService,
	service.NewRBACService,
	service.NewAuthzService,
	service.NewOrgService,
	service.NewMenuService,
	service.NewAuditService,
)

var handlerSet = wire.NewSet(
	handler.NewAuthHandler,
	handler.NewUserHandler,
	handler.NewRoleHandler,
	handler.NewOrgHandler,
	handler.NewMenuHandler,
)

// InitializeApp Wire 注入入口
func InitializeApp(cfg *config.Config) (*App, func(), error) {
	wire.Build(
		pkgSet,
		repoSet,
		serviceSet,
		handlerSet,
		router.New,
		NewApp,
		// 基础设施 Provider
		wire.FieldsOf(new(*config.Config),
			"JWT", "Redis", "Log",
		),
		wire.FieldsOf(new(config.JWTConfig),
			"Secret", "AccessTTL",
		),
		wire.FieldsOf(new(config.LogConfig),
			"Level", "Dir", "MaxSize", "MaxBackups", "MaxAge",
		),
		// PG 连接池（待实现）
		// Casbin enforcer（待实现）
	)
	return nil, nil, nil
}
