package handler

import (
	"bytes"
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/tracerbiubiubiu/zhuzhao-utils/aksk"
	"github.com/tracerbiubiubiu/zhuzhao-utils/response"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/channelsender"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service"
)

type MessageDeliveryHandler struct {
	service  *service.MessageDeliveryService
	verifier *aksk.Verifier
}

func NewMessageDeliveryHandler(svc *service.MessageDeliveryService, callbackAK, callbackSK string) *MessageDeliveryHandler {
	h := &MessageDeliveryHandler{service: svc}
	if callbackAK != "" && callbackSK != "" {
		h.verifier = &aksk.Verifier{Keys: map[string][]byte{callbackAK: []byte(callbackSK)}}
	}
	return h
}

func (h *MessageDeliveryHandler) CallbackEnabled() bool { return h.verifier != nil }

type messageDeliveryResponse struct {
	ID        string    `json:"delivery_id"`
	State     string    `json:"state"`
	Reason    string    `json:"reason,omitempty"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Receipt returns a metadata-only receipt for the original delivery.
// @Summary  查询本人的原投递状态
// @Tags     notification
// @Param    delivery_id query string true "原投递 ID"
// @Success  200 {object} response.Response{data=messageDeliveryResponse}
// @Router   /api/v1/notifications/delivery [get]
func (h *MessageDeliveryHandler) Receipt(c *gin.Context) {
	result, err := h.service.Receipt(c.Request.Context(), c.GetInt64("userID"), c.Query("delivery_id"))
	if err != nil {
		writeServiceError(c, err)
		return
	}
	response.OK(c, messageDeliveryResponse{ID: result.ID, State: result.State, Reason: result.Reason, UpdatedAt: result.UpdatedAt})
}

func (h *MessageDeliveryHandler) ProviderReceipt(c *gin.Context) {
	r := c.Request
	if r.URL.EscapedPath() != "/internal/messages/receipts" || r.URL.RawQuery != "" || r.URL.ForceQuery ||
		len(r.Header.Values(aksk.HeaderAuthorization)) != 1 || len(r.Header.Values(aksk.HeaderXOperator)) != 0 ||
		len(r.Header.Values(aksk.HeaderXRequestID)) > 1 {
		c.Status(http.StatusUnauthorized)
		return
	}
	data, err := io.ReadAll(http.MaxBytesReader(c.Writer, r.Body, 32<<10))
	if err != nil {
		c.Status(http.StatusBadRequest)
		return
	}
	if h.verifier == nil || h.verifier.Verify(r, data) != nil {
		c.Status(http.StatusUnauthorized)
		return
	}
	receipt, err := channelsender.DecodeAsyncMessageReceipt(bytes.NewReader(data))
	if err != nil {
		c.Status(http.StatusBadRequest)
		return
	}
	if err := h.service.AcceptProviderReceipt(r.Context(), receipt); err != nil {
		writeServiceError(c, err)
		return
	}
	c.Status(http.StatusOK)
}
