package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// B5-4 richtext 渲染端点（05 号 §6 渲染/消毒 SSOT）：契约形态 + XSS 负向 + 上限。
// 不依赖 ticketService（纯渲染），NewTicketHandler(nil) 即可。
func TestTicketHandler_RenderRichtext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewTicketHandler(nil, nil)
	r := gin.New()
	r.POST("/render", h.RenderRichtext)

	post := func(body string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/render", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)
		return w
	}

	t.Run("XSS 负向：script/事件属性/javascript 链接不得存活，排版保留", func(t *testing.T) {
		w := post(`{"content":"<script>alert(1)</script>\n\n# ok\n\n[x](javascript:alert(2))\n\n![i](/api/v1/files/tickets/a.png)"}`)
		require.Equal(t, http.StatusOK, w.Code)
		var env struct {
			Code int `json:"code"`
			Data struct {
				HTML string `json:"html"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env))
		assert.Equal(t, 0, env.Code)
		assert.NotContains(t, env.Data.HTML, "alert(1)")
		assert.NotContains(t, env.Data.HTML, "javascript:")
		assert.Contains(t, env.Data.HTML, "<h1>")
		assert.Contains(t, env.Data.HTML, `src="/api/v1/files/tickets/a.png"`, "稳定 URL 图片保留")
	})

	t.Run("缺 content 400", func(t *testing.T) {
		w := post(`{}`)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("超 256 KiB 上限 400（与字段校验同限）", func(t *testing.T) {
		big, _ := json.Marshal(map[string]string{"content": strings.Repeat("a", 256<<10+1)})
		w := post(string(big))
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}
