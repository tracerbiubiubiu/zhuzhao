package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/tracerbiubiubiu/zhuzhao-utils/response"

	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/b3protocol"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service"
)

type B3Handler struct{ flow *service.B3Flow }

func NewB3Handler(flow *service.B3Flow) *B3Handler { return &B3Handler{flow: flow} }

func (h *B3Handler) Push(c *gin.Context) {
	var input b3protocol.SourceDelivery
	if b3protocol.Decode(http.MaxBytesReader(c.Writer, c.Request.Body, 64<<10), &input) != nil {
		response.BadRequest(c, "无效的活动清单变化投递")
		return
	}
	ack, err := h.flow.Push(c.Request.Context(), input)
	if err != nil {
		writeServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, ack)
}

func (h *B3Handler) Status(c *gin.Context) {
	var input struct {
		TaskID      string `json:"task_id"`
		OperationID string `json:"operation_id"`
	}
	if b3protocol.Decode(http.MaxBytesReader(c.Writer, c.Request.Body, 4096), &input) != nil {
		response.BadRequest(c, "无效的任务查询")
		return
	}
	receipt, err := h.flow.Status(c.Request.Context(), input.TaskID, input.OperationID)
	if err != nil {
		writeServiceError(c, err)
		return
	}
	response.OK(c, receipt)
}
