package handler

import (
	"context"

	"github.com/gin-gonic/gin"

	"github.com/tracerbiubiubiu/zhuzhao-utils/response"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service"
)

func iamOperationContext(c *gin.Context) (context.Context, bool) {
	keys, ids := c.Request.Header.Values("Idempotency-Key"), c.Request.Header.Values("X-Operation-ID")
	if len(keys) > 1 || len(ids) > 1 || (len(keys) == 1 && keys[0] == "") || (len(ids) == 1 && ids[0] == "") || (len(keys) == 1 && len(ids) == 1 && keys[0] != ids[0]) {
		response.BadRequest(c, errcodeInvalidParams(c))
		return nil, false
	}
	id := c.GetHeader("X-Operation-ID")
	if len(keys) == 1 {
		id = keys[0]
	}
	ctx, id, err := service.IAMOperationContext(c.Request.Context(), id)
	if err != nil {
		writeServiceError(c, err)
		return nil, false
	}
	c.Header("X-Operation-ID", id)
	return ctx, true
}
