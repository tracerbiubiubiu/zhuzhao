package service

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"

	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/jobs"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

// JobsCallbackService /internal/jobs 回调编排（E-②，16 号 §3）。
// handler 只做 HTTP 绑定/映射；幂等栅栏、动作分发、错误分类在本科。
type JobsCallbackService struct {
	registry *jobs.Registry
	repo     *repository.JobSubmissionRepo
	logger   *slog.Logger
}

func NewJobsCallbackService(registry *jobs.Registry, repo *repository.JobSubmissionRepo, logger *slog.Logger) *JobsCallbackService {
	if logger == nil {
		logger = slog.Default()
	}
	return &JobsCallbackService{registry: registry, repo: repo, logger: logger}
}

// CallbackInput 回调请求（taskrunner callback client 契约字段）。
type CallbackInput struct {
	TaskID    string
	RequestID string
	Action    string // 来自回调 body.action_id（C10：标识在 body）
	Params    json.RawMessage
	Actor     string
	SourceIP  string
}

// CallbackOutcome 回调处置结果（handler 据此映射 HTTP）。
type CallbackOutcome int

const (
	CallbackExecuted      CallbackOutcome = iota // 执行完全成功 → 2xx
	CallbackIdempotent                           // 已 succeeded 的重复回调 → 2xx（幂等受理）
	CallbackUnknownAction                        // 未注册动作 → 404（P6 快速失败）
	CallbackNonRetryable                         // ErrAbort → 409（P7 不可重试）
	CallbackRetryable                            // 其他错误 → 500（P7 可重试）
)

// Execute 回调编排：未知动作拦截 → 幂等栅栏 → 分发执行 → 终态记账。
// ctx 应携带 request_id（jobs_handler 已把 body 值覆盖进 ctx 供落档）。
func (s *JobsCallbackService) Execute(ctx context.Context, in CallbackInput) (CallbackOutcome, string) {
	if _, ok := s.registry.Get(in.Action); !ok {
		return CallbackUnknownAction, "未注册的动作: " + in.Action
	}

	row, alreadyDone, err := s.repo.EnsureCallbackRow(ctx, in.TaskID, in.Action, in.Actor, in.SourceIP, string(in.Params))
	if err != nil {
		return CallbackRetryable, "回调受理失败"
	}
	if alreadyDone {
		// 幂等拦截：该 task_id 已执行完全成功，重复回调直接受理（不重复执行副作用）
		return CallbackIdempotent, row.Status
	}

	handler, _ := s.registry.Get(in.Action)
	if err := handler.Handle(ctx, in.Params); err != nil {
		_ = s.repo.MarkFailed(ctx, in.TaskID, err.Error())
		if errors.Is(err, jobs.ErrAbort) {
			return CallbackNonRetryable, "动作执行失败（不可重试）: " + err.Error()
		}
		return CallbackRetryable, "动作执行失败（可重试）"
	}
	if err := s.repo.MarkSucceeded(ctx, in.TaskID); err != nil {
		// 执行已成功、记账 UPDATE 失败（极低概率）：业务事实已发生，必须 2xx；
		// 若 taskrunner 因超时等重试会再次进入 Handle——Handler 可重入契约兜底
		s.logger.Warn("jobs: mark succeeded failed after execution",
			"task_id", in.TaskID, "action", in.Action, "err", err)
	}
	return CallbackExecuted, "succeeded"
}
