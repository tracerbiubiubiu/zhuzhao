package router

import (
	"github.com/gin-gonic/gin"
	"github.com/tracerbiubiubiu/zhuzhao-utils/aksk"
	"github.com/tracerbiubiubiu/zhuzhao-utils/response"

	"github.com/tracerbiubiubiu/zhuzhao/internal/config"
)

func mountRiskFeed(router *gin.Engine, cfg config.RiskIngestConfig, report func(*gin.Context, string)) {
	const path = "/api/v1/risk-items/report"
	if !cfg.Enabled || report == nil {
		router.POST(path, response.ServiceUnavailable)
		return
	}
	verifier := &aksk.Verifier{Keys: map[string][]byte{cfg.AK: []byte(cfg.SK)}}
	router.POST(path, aksk.GinMiddleware(verifier, response.AKSKFail()), func(ctx *gin.Context) {
		report(ctx, cfg.SourceRef)
	})
}
