//go:build wireinject
// +build wireinject

package app

import (
	"github.com/google/wire"

	"github.com/tracerbiubiubiu/zhuzhao/internal/casbin"
	"github.com/tracerbiubiubiu/zhuzhao/internal/config"
	"github.com/tracerbiubiubiu/zhuzhao/internal/handler"
	"github.com/tracerbiubiubiu/zhuzhao/internal/middleware"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/jwt"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/logger"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/postgres"
	pgredis "github.com/tracerbiubiubiu/zhuzhao/internal/pkg/redis"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/resource"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
	"github.com/tracerbiubiubiu/zhuzhao/internal/router"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service"
	ticketsvc "github.com/tracerbiubiubiu/zhuzhao/internal/service/ticket"
)

// Provider 集合

var pkgSet = wire.NewSet(
	logger.New,
	jwt.NewManager,
	postgres.New,
	pgredis.New,
	pgredis.NewScripts,
	// B4-6：Phase 2 接线预留（G-1，见 docs/phase2/02-authz-resource.md Step 0）——
	// 当前注入链路无消费者，Phase 2a 资源级鉴权时接线
	resource.NewRegistry,
	casbin.New,
)

var repoSet = wire.NewSet(
	repository.NewUserRepo,
	repository.NewRoleRepo,
	repository.NewOrgRepo,
	repository.NewMenuRepo,
	repository.NewAuditLogRepo,
	repository.NewTicketRepo,
)

var serviceSet = wire.NewSet(
	service.NewAuthService,
	service.NewUserService,
	service.NewRBACService,
	service.NewOrgDelegationService,
	service.NewOrgService,
	service.NewMenuService,
	service.NewAuditService,
	// Phase 2a：工单服务构造时自注册 TicketResource 到 Registry（§2.5）。
	// Wire 单例保证 registry.Register 先于 router.New，资源级鉴权可路由。
	ticketsvc.NewTicketService,
	wire.Bind(new(middleware.RoleFetcher), new(*service.RBACService)),
	wire.Bind(new(middleware.AuditLogger), new(*service.AuditService)),
	// 2c：工单委托判定接口绑定 OrgDelegationService（ticket 包不依赖 service 具体类型）
	wire.Bind(new(ticketsvc.OrgDelegationChecker), new(*service.OrgDelegationService)),
)

var handlerSet = wire.NewSet(
	handler.NewAuthHandler,
	handler.NewUserHandler,
	handler.NewRoleHandler,
	handler.NewOrgHandler,
	handler.NewMenuHandler,
	handler.NewAuditHandler,
	handler.NewTicketHandler,
)

// InitializeApp Wire 注入入口
func InitializeApp(cfg *config.Config) (*App, func(), error) {
	wire.Build(
		pkgSet,
		repoSet,
		serviceSet,
		handlerSet,
		// B1-4：从配置提取信任代理网段，供 router.Deps.TrustedProxies 消费
		provideTrustedProxies,
		wire.Struct(new(router.Deps), "*"),
		router.New,
		NewApp,
		// 从 *config.Config 中提取各子配置结构体，供基础设施 Provider 使用
		wire.FieldsOf(new(*config.Config),
			"Database", "Redis", "JWT", "Casbin", "Log",
		),
	)
	return nil, nil, nil
}

// provideTrustedProxies 信任代理网段（空 = 不信任任何代理，安全默认）
func provideTrustedProxies(cfg *config.Config) []string {
	return cfg.Server.TrustedProxies
}
