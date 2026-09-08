// E-④ 代理端点 handler 层单测（16 号 §3 测试缺口补齐，2026-09-07）：
// 绑定 400（action/job_id/task_id 缺失、params 64KB 上限）、上游不可达 502
// （client 未配置 / 连接拒绝——细节仅服务端日志，防拓扑泄漏）、信封错误映射
// （errcode 码 → httpStatusByCode；未登记码 → 500 默认）、8 个纯透传端点的
// 出站契约（method/path/query/body 逐项断言，仿 taskrunner 上游）。
// Submit/Trigger 正常路径（落 job_submissions 凭证，触达 repo）属集成范围：
// service 层已盖 taskrunner_service_integration_test.go。
// 注意：工单 handler 测试 = BK-19，已随 §23 工单封版后置，不在此列。
package handler

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao-utils/errcode"

	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/taskrunner"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service"
)

// newTaskrunnerProxyRouter 镜像 internal/router 对 biz 组的任务代理注册
// （/api/v1 前缀 + JWT username 注入桩）。upstream 为 nil 时服务持 nil client
// （等价 config taskrunner.base_url 未配置）。
func newTaskrunnerProxyRouter(t *testing.T, upstream *httptest.Server) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	var client *taskrunner.Client
	if upstream != nil {
		client = taskrunner.New(taskrunner.Config{BaseURL: upstream.URL, AK: "zhuzhao", SK: []byte("sk-test")})
	}
	h := NewTaskrunnerHandler(service.NewTaskrunnerService(client, nil), "http://self:33333")
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("username", "10001"); c.Next() }) // 模拟 JWT 注入
	biz := r.Group("/api/v1")
	{
		biz.POST("/tasks", h.Submit)
		biz.GET("/tasks/:id", h.GetTask)
		biz.POST("/tasks/cancel", h.Cancel)
		biz.POST("/tasks/retry", h.Retry)
		biz.GET("/runs", h.ListRuns)
		biz.GET("/dead-letters", h.DeadLetters)
		biz.GET("/jobs", h.ListJobs)
		biz.POST("/jobs", h.CreateJob)
		biz.POST("/jobs/update", h.UpdateJob)
		biz.POST("/jobs/trigger", h.Trigger)
	}
	return r
}

func postJSON(t *testing.T, r *gin.Engine, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	return w
}

// 绑定/校验 400：全部在触达 client 前返回，nil client 即可测。
func TestTaskrunnerHandler_BadRequest(t *testing.T) {
	r := newTaskrunnerProxyRouter(t, nil)
	cases := []struct {
		name    string
		path    string
		body    string
		wantMsg string
	}{
		{"Submit 空 body", "/api/v1/tasks", "", "action 必填"},
		{"Submit 缺 action", "/api/v1/tasks", `{"params":{}}`, "action 必填"},
		{"Submit params 超 64KB 上限", "/api/v1/tasks",
			`{"action":"audit_archive","params":"` + strings.Repeat("x", 64<<10) + `"}`, "params 超过上限"},
		{"Trigger 缺 job_id", "/api/v1/jobs/trigger", `{}`, "job_id 必填"},
		{"Cancel 缺 task_id", "/api/v1/tasks/cancel", `{}`, "task_id 必填"},
		{"Retry 缺 task_id", "/api/v1/tasks/retry", `{}`, "task_id 必填"},
		{"UpdateJob 缺 job_id", "/api/v1/jobs/update", `{"enabled":false}`, "job_id 必填"},
		{"UpdateJob 非法 JSON", "/api/v1/jobs/update", `{bad`, "job_id 必填"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := postJSON(t, r, tc.path, tc.body)
			require.Equal(t, http.StatusBadRequest, w.Code)
			require.Contains(t, w.Body.String(), tc.wantMsg)
		})
	}
}

// 上游不可达 → 502「任务服务不可达」：非 errcode 错误统一透出，细节不进响应体。
func TestTaskrunnerHandler_UpstreamUnavailable(t *testing.T) {
	t.Run("client 未配置（base_url 空 → ensure 拦截）", func(t *testing.T) {
		r := newTaskrunnerProxyRouter(t, nil)
		w := postJSON(t, r, "/api/v1/tasks", `{"action":"audit_archive"}`)
		require.Equal(t, http.StatusBadGateway, w.Code)
		require.Contains(t, w.Body.String(), "任务服务不可达")
	})
	t.Run("上游连接拒绝", func(t *testing.T) {
		dead := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
		dead.Close() // 立即关闭 → 连接拒绝
		r := newTaskrunnerProxyRouter(t, dead)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/tasks/t-1", nil))
		require.Equal(t, http.StatusBadGateway, w.Code)
		require.Contains(t, w.Body.String(), "任务服务不可达")
	})
}

// 信封错误映射：code≠0 → errcode.Error → httpStatusByCode；未登记码落 500 默认。
func TestTaskrunnerHandler_EnvelopeErrorMapping(t *testing.T) {
	newUpstream := func(t *testing.T, code int, message string) *httptest.Server {
		t.Helper()
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"code": code, "message": message})
		}))
	}

	t.Run("已登记码 10001 → 400", func(t *testing.T) {
		up := newUpstream(t, errcode.ErrInvalidParams.Code, "cron_spec 非法")
		defer up.Close()
		w := postJSON(t, newTaskrunnerProxyRouter(t, up), "/api/v1/jobs", `{"action_id":"a"}`)
		require.Equal(t, http.StatusBadRequest, w.Code)
		require.Contains(t, w.Body.String(), "cron_spec 非法")
	})
	t.Run("未登记码 79999 → 500 默认", func(t *testing.T) {
		up := newUpstream(t, 79999, "上游内部态")
		defer up.Close()
		w := postJSON(t, newTaskrunnerProxyRouter(t, up), "/api/v1/jobs", `{"action_id":"a"}`)
		require.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

// 纯透传端点出站契约：method/path/query/body 逐项对仿上游断言 + data 原样透传。
func TestTaskrunnerHandler_ProxyPassthrough(t *testing.T) {
	var gotMethod, gotPath, gotRawQuery, gotBody, gotOperator string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		gotMethod, gotPath, gotRawQuery = req.Method, req.URL.Path, req.URL.RawQuery
		b, _ := io.ReadAll(req.Body)
		gotBody = string(b)
		gotOperator = req.Header.Get("X-Operator")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"message":"success","data":{"echo":"ok"}}`))
	}))
	defer up.Close()
	r := newTaskrunnerProxyRouter(t, up)

	cases := []struct {
		name             string
		method           string
		path             string
		body             string
		wantMethod       string
		wantPath         string
		wantQuery        string // 空 = 不断言
		wantBodyContains []string
		wantOperator     string // 空 = 不断言
	}{
		{"GetTask 路径参数", http.MethodGet, "/api/v1/tasks/t-1", "",
			http.MethodGet, "/v1/tasks/t-1", "", nil, ""},
		{"ListRuns query 透传", http.MethodGet, "/api/v1/runs?status=failed&request_id=rq-1", "",
			http.MethodGet, "/v1/runs", "request_id=rq-1&status=failed", nil, ""},
		{"ListJobs dept 过滤透传（C11）", http.MethodGet, "/api/v1/jobs?dept=dept-a", "",
			http.MethodGet, "/v1/jobs", "dept=dept-a", nil, ""},
		{"DeadLetters 分页透传", http.MethodGet, "/api/v1/dead-letters?page=1", "",
			http.MethodGet, "/v1/dead-letters", "page=1", nil, ""},
		{"CreateJob raw body 透传 + actor 出站", http.MethodPost, "/api/v1/jobs", `{"action_id":"arch","cron_spec":"@daily"}`,
			http.MethodPost, "/v1/jobs", "", []string{`"action_id":"arch"`}, "10001"},
		{"UpdateJob 标识在 body（C10）", http.MethodPost, "/api/v1/jobs/update", `{"job_id":"j-9","enabled":false}`,
			http.MethodPost, "/v1/jobs/update", "", []string{`"job_id":"j-9"`}, "10001"},
		{"Cancel task_id（C10）", http.MethodPost, "/api/v1/tasks/cancel", `{"task_id":"t-7"}`,
			http.MethodPost, "/v1/tasks/cancel", "", []string{`"task_id":"t-7"`}, ""},
		{"Retry task_id（C10）", http.MethodPost, "/api/v1/tasks/retry", `{"task_id":"t-8"}`,
			http.MethodPost, "/v1/tasks/retry", "", []string{`"task_id":"t-8"`}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var w *httptest.ResponseRecorder
			if tc.method == http.MethodGet {
				w = httptest.NewRecorder()
				r.ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, nil))
			} else {
				w = postJSON(t, r, tc.path, tc.body)
			}
			require.Equal(t, http.StatusOK, w.Code)
			require.Equal(t, tc.wantMethod, gotMethod)
			require.Equal(t, tc.wantPath, gotPath)
			if tc.wantQuery != "" {
				require.Equal(t, tc.wantQuery, gotRawQuery)
			}
			for _, sub := range tc.wantBodyContains {
				require.Contains(t, gotBody, sub)
			}
			if tc.wantOperator != "" {
				require.Equal(t, tc.wantOperator, gotOperator, "actor 应以 X-Operator 出站（审计归因）")
			}
			require.Contains(t, w.Body.String(), `"echo":"ok"`) // 上游 data 原样透传
		})
	}
}
