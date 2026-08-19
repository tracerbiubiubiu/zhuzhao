package middleware_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/middleware"
)

type stubAuditLogger struct {
	inserted       int
	ctxErrAtInsert error // Insert 执行瞬间的 ctx 状态（写入是否有效）
}

func (s *stubAuditLogger) Insert(ctx context.Context, log middleware.AuditLogEntry) error {
	s.inserted++
	s.ctxErrAtInsert = ctx.Err()
	return nil
}

// F-5 特殊场景：客户端断连（request context 已取消）后，审计写入必须仍以独立 context 执行。
// 修复前：Insert 收到的就是已取消的 request ctx，pgx Exec 必然失败，审计静默丢失。
// 注意：断言的是 Insert 执行瞬间的 ctx 状态（中间件返回后 defer cancel() 属正常清理）。
func TestAuditLog_InsertSurvivesClientDisconnect(t *testing.T) {
	gin.SetMode(gin.TestMode)
	stub := &stubAuditLogger{}

	r := gin.New()
	r.Use(middleware.AuditLog(stub))

	// handler 内模拟"业务执行到一半客户端断开"
	reqCtx, cancel := context.WithCancel(context.Background())
	r.POST("/api/v1/users/delete", func(c *gin.Context) {
		cancel() // 客户端断连 → request context 取消
		c.Status(http.StatusOK)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/users/delete", nil).WithContext(reqCtx)
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, 1, stub.inserted, "审计必须写入，即使客户端已断连")
	require.NoError(t, stub.ctxErrAtInsert,
		"审计写入瞬间的 context 不得已取消（应为 WithoutCancel + 独立超时）")
}

// 常规路径：正常请求下审计照常写入
func TestAuditLog_NormalRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	stub := &stubAuditLogger{}

	r := gin.New()
	r.Use(middleware.AuditLog(stub))
	r.POST("/x", func(c *gin.Context) { c.Status(http.StatusCreated) })

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/x", nil))

	require.Equal(t, 1, stub.inserted)
	require.NoError(t, stub.ctxErrAtInsert)
}
