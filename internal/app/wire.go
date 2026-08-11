//go:build wireinject
// +build wireinject

package app

import (
	"github.com/google/wire"

	"github.com/tracerbiubiubiu/zhuzhao/internal/casbin"
	"github.com/tracerbiubiubiu/zhuzhao/internal/config"
	"github.com/tracerbiubiubiu/zhuzhao/internal/handler"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/jwt"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/logger"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/postgres"
	pgredis "github.com/tracerbiubiubiu/zhuzhao/internal/pkg/redis"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
	"github.com/tracerbiubiubiu/zhuzhao/internal/router"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service"
)

// Provider 集合

var pkgSet = wire.NewSet(
	logger.New,
	jwt.NewManager,
	postgres.New,
	pgredis.New,
	casbin.New,
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
		// 从 *config.Config 中提取各子配置结构体，供基础设施 Provider 使用
		wire.FieldsOf(new(*config.Config),
			"Database", "Redis", "JWT", "Casbin", "Log",
		),
	)
	return nil, nil, nil
}
