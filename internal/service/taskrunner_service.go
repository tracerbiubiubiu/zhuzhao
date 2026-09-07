package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"

	"github.com/google/uuid"

	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/reqid"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/taskrunner"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

// TaskrunnerService 任务管理服务（E-④，16 号 §3）：三层校验后的网关代理层。
// 出站一律经 pkg/taskrunner client（AK/SK 签名 + request_id/actor 透传）；
// 提交/触发同步落 job_submissions 提交凭证（E5，薄——执行细节不回传，request_id 跨查）。
type TaskrunnerService struct {
	client *taskrunner.Client // 可为 nil（base_url 未配置）——经 ensure 拦截
	subs   *repository.JobSubmissionRepo
}

// ensure 出站前置检查：taskrunner 未配置（base_url 空 → wire 注入 nil client）时
// 返回普通 error——handler 映射 502（上游不可达），而非 nil 指针 panic。
func (s *TaskrunnerService) ensure() error {
	if s.client == nil {
		return fmt.Errorf("taskrunner 服务未配置（config taskrunner.base_url）")
	}
	return nil
}

func NewTaskrunnerService(client *taskrunner.Client, subs *repository.JobSubmissionRepo) *TaskrunnerService {
	return &TaskrunnerService{client: client, subs: subs}
}

// TaskSubmitInput 用户侧提交任务（经网关；action_id 对应已注册预置动作）。
type TaskSubmitInput struct {
	Action      string          `json:"action" binding:"required"`
	Dept        string          `json:"dept"` // 一次性任务归属标签（E-⑤ 组装用户可见标签；C11 快照列）
	CallbackURL string          `json:"callback_url"`
	Params      json.RawMessage `json:"params"`
	TaskID      string          `json:"task_id"` // 可选：调用方幂等键
	TimeoutSecs int             `json:"timeout_secs"`
}

// Submit 提交一次性任务：request_id 取入站 ctx（唯一关联键）；task_id 缺省生成。
// CallbackURL 缺省按 action 拼本服务内网端点（部署同网可达）。
func (s *TaskrunnerService) Submit(ctx context.Context, in *TaskSubmitInput, actor, sourceIP, selfBaseURL string) (*taskrunner.SubmitResponse, error) {
	if err := s.ensure(); err != nil {
		return nil, err
	}
	taskID := in.TaskID
	if taskID == "" {
		taskID = uuid.NewString()
	}
	callback := in.CallbackURL
	if callback == "" {
		if selfBaseURL == "" {
			return nil, fmt.Errorf("callback_url 未提供且服务自身地址未配置")
		}
		// C10 后回调端点统一 /internal/jobs/callback（action_id 在 body）——
		// 路由只注册了这一个路径，拼接旧格式会导致回调 404 → 任务被判 non-retryable
		callback = selfBaseURL + "/internal/jobs/callback"
	}
	resp, err := s.client.Submit(ctx, taskrunner.SubmitRequest{
		TaskID:      taskID,
		RequestID:   reqid.From(ctx),
		Action:      in.Action,
		Dept:        in.Dept,
		CallbackURL: callback,
		Params:      in.Params,
		SubmittedBy: actor,
		SourceIP:    sourceIP,
		TimeoutSecs: in.TimeoutSecs,
	})
	if err != nil {
		return nil, err
	}
	// E5 提交凭证（薄）：{action, task_id, request_id}——request_id 由 repo 从 ctx 取
	if _, err := s.subs.RecordSubmit(ctx, in.Action, resp.TaskID, actor, sourceIP, string(in.Params)); err != nil {
		// 受理已成立，凭证记账失败不回滚用户侧结果（对账兜底：task_id 在 taskrunner 侧）
		// ——与回调侧 MarkSucceeded 失败同款取舍
		_ = err
	}
	return resp, nil
}

// Trigger 手动执行一次任务定义（前端「立即执行」）。
func (s *TaskrunnerService) Trigger(ctx context.Context, jobID, actor, sourceIP string) (*taskrunner.SubmitResponse, error) {
	if err := s.ensure(); err != nil {
		return nil, err
	}
	resp, err := s.client.TriggerJob(ctx, jobID, actor, sourceIP)
	if err != nil {
		return nil, err
	}
	// 提交凭证：trigger 的真实 action_id 在 taskrunner job 定义内（zhuzhao 不感知），
	// 凭证记 action="trigger:<job_id>"，跨查锚点为 task_id + request_id
	_, _ = s.subs.RecordSubmit(ctx, "trigger:"+jobID, resp.TaskID, actor, sourceIP, "{}")
	return resp, nil
}

// ---- 透传查询/管理（无本地状态；权限码在路由层） ----

func (s *TaskrunnerService) GetTask(ctx context.Context, taskID string) (json.RawMessage, error) {
	if err := s.ensure(); err != nil {
		return nil, err
	}
	return s.client.GetTask(ctx, taskID)
}

func (s *TaskrunnerService) ListRuns(ctx context.Context, query url.Values) (json.RawMessage, error) {
	if err := s.ensure(); err != nil {
		return nil, err
	}
	return s.client.ListRuns(ctx, query)
}

func (s *TaskrunnerService) ListJobs(ctx context.Context, query url.Values) (json.RawMessage, error) {
	if err := s.ensure(); err != nil {
		return nil, err
	}
	return s.client.ListJobs(ctx, query)
}

func (s *TaskrunnerService) CreateJob(ctx context.Context, body json.RawMessage, actor string) (json.RawMessage, error) {
	if err := s.ensure(); err != nil {
		return nil, err
	}
	return s.client.CreateJob(ctx, body, actor)
}

func (s *TaskrunnerService) UpdateJob(ctx context.Context, jobID string, body json.RawMessage, actor string) (json.RawMessage, error) {
	if err := s.ensure(); err != nil {
		return nil, err
	}
	return s.client.UpdateJob(ctx, jobID, body, actor)
}

func (s *TaskrunnerService) CancelTask(ctx context.Context, taskID, actor string) (json.RawMessage, error) {
	if err := s.ensure(); err != nil {
		return nil, err
	}
	return s.client.CancelTask(ctx, taskID, actor)
}

func (s *TaskrunnerService) RetryTask(ctx context.Context, taskID, actor string) (json.RawMessage, error) {
	if err := s.ensure(); err != nil {
		return nil, err
	}
	return s.client.RetryTask(ctx, taskID, actor)
}

func (s *TaskrunnerService) ListDeadLetters(ctx context.Context, query url.Values) (json.RawMessage, error) {
	if err := s.ensure(); err != nil {
		return nil, err
	}
	return s.client.ListDeadLetters(ctx, query)
}
