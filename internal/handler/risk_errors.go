package handler

import (
	"errors"

	"github.com/gin-gonic/gin"

	"github.com/tracerbiubiubiu/zhuzhao-utils/response"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service/risk"
)

// 风险域错误码 → HTTP 映射（详设 risk-module-detail §2.2 错误面；
// 码定义收口 repository/risk_errors.go，service/risk 别名——共享码段仍走
// writeServiceError 既有表，不在此重复登记）。
var riskCodeHTTPStatus = map[int]int{
	risk.ErrRiskNotFound.Code:          404, // 不可见与不存在同形（详设 §7）
	risk.ErrRiskInvalidState.Code:      400,
	risk.ErrRiskOperationConflict.Code: 409, // 同号异参
	risk.ErrRiskSourceUnknown.Code:     403, // 来源未登记/停用
	risk.ErrRiskSourceScope.Code:       403, // 超出来源授权范围
	risk.ErrRiskBatchTooLarge.Code:     413,
	risk.ErrRiskOwnerInvalid.Code:      400,
	risk.ErrRiskOwnerUnconfirmed.Code:  400, // 责任未确认无法准备派单
}

// writeRiskError 风险域错误出口：本批码段按上表映射；其余（70001 无权限、
// 400 参数、409 并发修订等共享码）委托既有 writeServiceError。
func writeRiskError(c *gin.Context, err error) {
	var biz *errcode.Error
	if errors.As(err, &biz) {
		if status, ok := riskCodeHTTPStatus[biz.Code]; ok {
			response.Error(c, status, biz)
			return
		}
	}
	writeServiceError(c, err)
}
