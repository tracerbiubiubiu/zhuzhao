// Package taskrunner zhuzhao 侧的 taskrunner API client（E-④，16 号 §3）。
//
// 职责：网关代理的唯一出站通道——所有请求以 zhuzhao 自身 SK 做 AK/SK HMAC 签名
// （基线 §9），request_id 从入站请求 ctx 取出透传（body + X-Request-ID 头，
// 03 §3.4 全链路关联），写接口透传 actor 工号 + source_ip（taskrunner 存档仅审计归因）。
// 响应统一解包 taskrunner 的 utils errcode/response 信封：code≠0 → errcode.Error。
package taskrunner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/tracerbiubiubiu/zhuzhao-utils/aksk"
	"github.com/tracerbiubiubiu/zhuzhao-utils/errcode"

	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/reqid"
)

// Config client 参数（零值：AK 默认 zhuzhao、超时 10s；BaseURL 必填）。
type Config struct {
	BaseURL string
	AK      string
	SK      []byte
	Timeout time.Duration
}

// Client taskrunner API client。并发安全（http.Client 可复用）。
type Client struct {
	base string
	ak   string
	sk   []byte
	http *http.Client
}

func New(cfg Config) *Client {
	if cfg.AK == "" {
		cfg.AK = "zhuzhao"
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 10 * time.Second
	}
	return &Client{
		base: cfg.BaseURL,
		ak:   cfg.AK,
		sk:   cfg.SK,
		http: &http.Client{Timeout: cfg.Timeout},
	}
}

// envelope taskrunner 统一响应（utils response）。
type envelope struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

// SubmitRequest POST /v1/tasks（受理语义：accepted ≠ 执行成功）。
type SubmitRequest struct {
	TaskID      string // 调用方生成（幂等键）；空则 client 生成
	RequestID   string // 空 = reqid.From(ctx)
	Action      string // action_id（必填）
	Dept        string // 一次性任务归属标签（zhuzhao E-⑤ 携带，taskrunner 落快照列）
	CallbackURL string // zhuzhao 内网端点（必填，如 http://zhuzhao:33333/internal/jobs/callback）
	Params      json.RawMessage
	SubmittedBy string // actor 工号（审计归因）
	SourceIP    string
	TimeoutSecs int
}

// SubmitResponse 受理结果。
type SubmitResponse struct {
	TaskID   string `json:"task_id"`
	Accepted bool   `json:"accepted"`
}

func (c *Client) Submit(ctx context.Context, req SubmitRequest) (*SubmitResponse, error) {
	if req.Action == "" || req.CallbackURL == "" {
		return nil, errcode.ErrInvalidParams
	}
	requestID := req.RequestID
	if requestID == "" {
		requestID = reqid.From(ctx)
	}
	body := map[string]interface{}{
		"action":       req.Action,
		"callback_url": req.CallbackURL,
		"request_id":   requestID,
		"submitted_by": req.SubmittedBy,
		"source_ip":    req.SourceIP,
	}
	if req.TaskID != "" {
		body["task_id"] = req.TaskID
	}
	if req.Params != nil {
		body["params"] = req.Params
	}
	if req.TimeoutSecs > 0 {
		body["timeout_secs"] = req.TimeoutSecs
	}
	if req.Dept != "" {
		body["dept"] = req.Dept
	}
	var out SubmitResponse
	return &out, c.do(ctx, http.MethodPost, "/v1/tasks", nil, body, req.SubmittedBy, &out)
}

// GetTask GET /v1/tasks/{id}（执行结果唯一出口；live_state 含 in-flight 实时态）。
func (c *Client) GetTask(ctx context.Context, taskID string) (json.RawMessage, error) {
	var out json.RawMessage
	return out, c.do(ctx, http.MethodGet, "/v1/tasks/"+url.PathEscape(taskID), nil, nil, "", &out)
}

// ListRuns GET /v1/runs（request_id/action/status/from/to 查询透传）。
func (c *Client) ListRuns(ctx context.Context, query url.Values) (json.RawMessage, error) {
	var out json.RawMessage
	return out, c.do(ctx, http.MethodGet, "/v1/runs", query, nil, "", &out)
}

// ListJobs GET /v1/jobs（dept/action_id/enabled 过滤透传——dept 语义归 zhuzhao E-⑤）。
func (c *Client) ListJobs(ctx context.Context, query url.Values) (json.RawMessage, error) {
	var out json.RawMessage
	return out, c.do(ctx, http.MethodGet, "/v1/jobs", query, nil, "", &out)
}

// CreateJob POST /v1/jobs（action_id + trigger_type + cron_spec + params + dept + enabled...）。
func (c *Client) CreateJob(ctx context.Context, body json.RawMessage, actor string) (json.RawMessage, error) {
	var out json.RawMessage
	return out, c.do(ctx, http.MethodPost, "/v1/jobs", nil, body, actor, &out)
}

// UpdateJob POST /v1/jobs/update（C10：标识在 body；body 由 service 层构造、含 job_id）。
func (c *Client) UpdateJob(ctx context.Context, jobID string, body json.RawMessage, actor string) (json.RawMessage, error) {
	var out json.RawMessage
	return out, c.do(ctx, http.MethodPost, "/v1/jobs/update", nil, body, actor, &out)
}

// TriggerJob POST /v1/jobs/trigger（手动执行一次，前端「立即执行」；body 带 job_id）。
func (c *Client) TriggerJob(ctx context.Context, jobID, actor, sourceIP string) (*SubmitResponse, error) {
	body := map[string]interface{}{"job_id": jobID, "actor": actor, "source_ip": sourceIP, "request_id": reqid.From(ctx)}
	var out SubmitResponse
	return &out, c.do(ctx, http.MethodPost, "/v1/jobs/trigger", nil, body, actor, &out)
}

// CancelTask POST /v1/tasks/cancel（仅未开始；执行中 409；body 带 task_id）。
func (c *Client) CancelTask(ctx context.Context, taskID, actor string) (json.RawMessage, error) {
	body := map[string]interface{}{"task_id": taskID}
	var out json.RawMessage
	return out, c.do(ctx, http.MethodPost, "/v1/tasks/cancel", nil, body, actor, &out)
}

// RetryTask POST /v1/tasks/retry（失败/死信重试；body 带 task_id）。
func (c *Client) RetryTask(ctx context.Context, taskID, actor string) (json.RawMessage, error) {
	body := map[string]interface{}{"task_id": taskID}
	var out json.RawMessage
	return out, c.do(ctx, http.MethodPost, "/v1/tasks/retry", nil, body, actor, &out)
}

// ListDeadLetters GET /v1/dead-letters（运维向）。
func (c *Client) ListDeadLetters(ctx context.Context, query url.Values) (json.RawMessage, error) {
	var out json.RawMessage
	return out, c.do(ctx, http.MethodGet, "/v1/dead-letters", query, nil, "", &out)
}

// do 统一请求：签名（body 序列化后）→ 发送 → 解信封。actor 为空则不携带 operator 属性。
func (c *Client) do(ctx context.Context, method, path string, query url.Values, body interface{}, actor string, out interface{}) error {
	var payload []byte
	if body != nil {
		switch b := body.(type) {
		case json.RawMessage:
			payload = b
		default:
			var err error
			if payload, err = json.Marshal(body); err != nil {
				return fmt.Errorf("taskrunner: marshal: %w", err)
			}
		}
	}

	u := c.base + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, u, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("taskrunner: build request: %w", err)
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	aksk.Sign(req, payload, aksk.SignOptions{
		AK: c.ak, SK: c.sk,
		RequestID: reqid.From(ctx), // 头通道（taskrunner 中间件读；body 通道由各方法自带）
		Operator:  actor,
	})

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("taskrunner: do: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return fmt.Errorf("taskrunner: read resp: %w", err)
	}

	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return fmt.Errorf("taskrunner: bad envelope (http %d): %s", resp.StatusCode, truncate(string(raw), 200))
	}
	if env.Code != 0 {
		return errcode.New(env.Code, env.Message)
	}
	if out != nil {
		if err := json.Unmarshal(env.Data, out); err != nil && len(env.Data) > 0 && string(env.Data) != "null" {
			return fmt.Errorf("taskrunner: decode data: %w", err)
		}
	}
	return nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
