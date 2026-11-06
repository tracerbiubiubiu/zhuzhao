package handler

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
)

// 与真实AddCC相同的ShouldBindJSON路径：字符串ID精确且旧数字请求兼容。
func TestTicketCC_StringIDsAndLegacyNumbers(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		want       int64
	}{
		{"string", `{"ticket_id":"1","user_ids":["9007199254740993"]}`, 9007199254740993},
		{"number", `{"ticket_id":"1","user_ids":[9007199254740993]}`, 9007199254740993},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("POST", "/api/v1/tickets/cc", strings.NewReader(tc.body))
			c.Request.Header.Set("Content-Type", "application/json")
			var req model.AddTicketCCRequest
			require.NoError(t, c.ShouldBindJSON(&req))
			require.Equal(t, []int64{tc.want}, []int64(req.UserIDs))
		})
	}
	for _, raw := range []string{`[]`, `null`, `["0"]`, `["-1"]`, `["9223372036854775808"]`, `[1.5]`, `["1.5"]`, `["not-id"]`, `[` + strings.Repeat(`"1",`, 20) + `"1"]`} {
		t.Run(raw, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("POST", "/api/v1/tickets/cc", strings.NewReader(`{"ticket_id":"1","user_ids":`+raw+`}`))
			c.Request.Header.Set("Content-Type", "application/json")
			var req model.AddTicketCCRequest
			require.Error(t, c.ShouldBindJSON(&req), "元素正数和单次20个上限不能随兼容解码丢失")
		})
	}
}
