//go:build integration

// E-② 内网回调端点集成验证（真 PG + 复刻 router 的 /internal 组挂法）：
// aksk 验签（未签/篡改 401、合法 200）、P6 未知动作 404、P7 错误映射
// （ErrAbort→409 / 其他→500）、幂等栅栏（succeeded 后重复回调不重复执行、
// failed 后重试可再执行）。
package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao-utils/aksk"

	"github.com/tracerbiubiubiu/zhuzhao/internal/handler"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/jobs"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

const (
	testAK = "taskrunner"
	testSK = "sk-test-taskrunner"
)

// newCallbackRouter 复刻 router.New 的 /internal 组挂法（验签中间件 + 端点）。
func newCallbackRouter(t *testing.T, registry *jobs.Registry) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	repo := repository.NewJobSubmissionRepo(testPool)
	h := handler.NewJobsHandler(registry, repo)
	r := gin.New()
	verifier := &aksk.Verifier{Keys: map[string][]byte{testAK: []byte(testSK)}}
	g := r.Group("/internal", aksk.GinMiddleware(verifier, nil))
	g.POST("/jobs/:action_id", h.Callback)
	return r
}

// signedPost 构造带签名的回调请求（模拟 taskrunner callback client：签名 + X-Request-ID/X-Operator）。
func signedPost(t *testing.T, r *gin.Engine, action, taskID, requestID string, params map[string]any, sk string) *httptest.ResponseRecorder {
	t.Helper()
	body, _ := json.Marshal(map[string]any{
		"task_id": taskID, "request_id": requestID, "params": params, "actor": "10001",
	})
	req := httptest.NewRequest(http.MethodPost, "/internal/jobs/"+action, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if sk != "" {
		aksk.Sign(req, body, aksk.SignOptions{AK: testAK, SK: []byte(sk), RequestID: requestID, Operator: "10001"})
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func cleanupSubmission(t *testing.T, taskID string) {
	t.Helper()
	testPool.Exec(context.Background(), `DELETE FROM job_submissions WHERE task_id=$1`, taskID)
}

func TestJobsCallbackAKSKVerify(t *testing.T) {
	registry := jobs.NewRegistry()
	var n atomic.Int64
	registry.Register("it_echo", jobs.HandlerFunc(func(ctx context.Context, _ json.RawMessage) error {
		n.Add(1)
		return nil
	}))
	r := newCallbackRouter(t, registry)

	t.Run("unsigned 401", func(t *testing.T) {
		w := signedPost(t, r, "it_echo", "t-unsigned", "", nil, "")
		require.Equal(t, 401, w.Code)
		require.Zero(t, n.Load())
	})
	t.Run("wrong secret 401", func(t *testing.T) {
		w := signedPost(t, r, "it_echo", "t-wrongsk", "", nil, "sk-other")
		require.Equal(t, 401, w.Code)
		require.Zero(t, n.Load())
	})
	t.Run("signed 200", func(t *testing.T) {
		t.Cleanup(func() { cleanupSubmission(t, "t-signed") })
		w := signedPost(t, r, "it_echo", "t-signed", "req-signed", map[string]any{"k": 1}, testSK)
		require.Equal(t, 200, w.Code)
		require.EqualValues(t, 1, n.Load())
	})
}

func TestJobsCallbackContract(t *testing.T) {
	suffix := time.Now().UnixNano()

	t.Run("unknown action 404 (P6)", func(t *testing.T) {
		r := newCallbackRouter(t, jobs.NewRegistry())
		w := signedPost(t, r, "ghost_action", fmt.Sprintf("t-%d-ghost", suffix), "", nil, testSK)
		require.Equal(t, 404, w.Code)
	})

	t.Run("ErrAbort 409 non-retryable (P7)", func(t *testing.T) {
		registry := jobs.NewRegistry()
		registry.Register("it_abort", jobs.HandlerFunc(func(context.Context, json.RawMessage) error {
			return fmt.Errorf("参数非法: %w", jobs.ErrAbort)
		}))
		r := newCallbackRouter(t, registry)
		taskID := fmt.Sprintf("t-%d-abort", suffix)
		t.Cleanup(func() { cleanupSubmission(t, taskID) })
		w := signedPost(t, r, "it_abort", taskID, "", nil, testSK)
		require.Equal(t, 409, w.Code)
		// 失败记账：status=failed，允许重试路径
		var status string
		require.NoError(t, testPool.QueryRow(context.Background(),
			`SELECT status FROM job_submissions WHERE task_id=$1`, taskID).Scan(&status))
		require.Equal(t, "failed", status)
	})

	t.Run("generic error 500 retryable (P7)", func(t *testing.T) {
		registry := jobs.NewRegistry()
		registry.Register("it_boom", jobs.HandlerFunc(func(context.Context, json.RawMessage) error {
			return fmt.Errorf("db down")
		}))
		r := newCallbackRouter(t, registry)
		taskID := fmt.Sprintf("t-%d-boom", suffix)
		t.Cleanup(func() { cleanupSubmission(t, taskID) })
		w := signedPost(t, r, "it_boom", taskID, "", nil, testSK)
		require.Equal(t, 500, w.Code)
	})

	t.Run("idempotency: succeeded 后重复回调不重复执行；failed 后重试可执行", func(t *testing.T) {
		registry := jobs.NewRegistry()
		var n atomic.Int64
		registry.Register("it_flaky", jobs.HandlerFunc(func(ctx context.Context, _ json.RawMessage) error {
			if n.Add(1) == 1 {
				return fmt.Errorf("第一次失败（可重试）")
			}
			return nil
		}))
		r := newCallbackRouter(t, registry)
		taskID := fmt.Sprintf("t-%d-idem", suffix)
		t.Cleanup(func() { cleanupSubmission(t, taskID) })

		w1 := signedPost(t, r, "it_flaky", taskID, "req-idem", nil, testSK)
		require.Equal(t, 500, w1.Code) // 第 1 次失败 → 5xx，taskrunner 会重试

		w2 := signedPost(t, r, "it_flaky", taskID, "req-idem", nil, testSK)
		require.Equal(t, 200, w2.Code) // 重试成功 → 2xx

		w3 := signedPost(t, r, "it_flaky", taskID, "req-idem", nil, testSK)
		require.Equal(t, 200, w3.Code) // succeeded 后重复回调 → 幂等受理

		require.EqualValues(t, 2, n.Load(), "第三次回调不得重复执行副作用")

		var status, requestID string
		require.NoError(t, testPool.QueryRow(context.Background(),
			`SELECT status, COALESCE(request_id,'') FROM job_submissions WHERE task_id=$1`, taskID).
			Scan(&status, &requestID))
		require.Equal(t, "succeeded", status)
		require.Equal(t, "req-idem", requestID, "回调 request_id 应落档（taskrunner job_runs 跨查键）")
	})
}
