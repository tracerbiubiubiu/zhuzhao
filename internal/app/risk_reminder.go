package app

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tracerbiubiubiu/zhuzhao/internal/config"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service/riskreminder"
)

func buildRiskReminder(cfg *config.Config, pool *pgxpool.Pool, logger *slog.Logger, registry *service.CronRegistry) (*riskreminder.Service, *service.CronScheduler, error) {
	if !cfg.RiskRemediationEnabled {
		return nil, nil, nil
	}
	inbox := repository.NewNotificationInboxRepo(pool)
	if cfg.RiskReminder.ScanInterval == 0 {
		return riskreminder.NewService(pool, inbox, nil, logger), nil, nil
	}
	model, err := os.ReadFile(cfg.Casbin.Model)
	if err != nil {
		return nil, nil, fmt.Errorf("load risk reminder authorization model: %w", err)
	}
	recorder, job, err := riskreminder.NewRuntime(pool, inbox, string(model), riskreminder.NewBoundPolicyResolver(pool), nil, logger)
	if err != nil {
		return nil, nil, err
	}
	cron := service.NewCronScheduler(pool, riskreminder.JobName, job, cfg.RiskReminder.ScanInterval, logger)
	registry.Register(riskreminder.JobName, cron)
	return recorder, cron, nil
}
