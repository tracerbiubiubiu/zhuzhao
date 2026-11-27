package app

import (
	"context"
	"encoding/json"

	"github.com/tracerbiubiubiu/zhuzhao/internal/service/risk"
)

type riskIntakeRecovery struct {
	service   *risk.Service
	sourceRef string
}

func (j riskIntakeRecovery) Handle(ctx context.Context, _ json.RawMessage) error {
	const batchesPerRun = 20
	return j.service.ResumeSourceReports(ctx, j.sourceRef, batchesPerRun)
}
