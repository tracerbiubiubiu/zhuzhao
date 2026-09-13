package service

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

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

	row, claimed, err := s.repo.ClaimCallbackRow(ctx, in.TaskID, in.Action, in.Actor, in.SourceIP, string(in.Params))
	if err != nil {
		return CallbackRetryable, "回调受理失败"
	}
	if !claimed {
		// 未取得执行权：他人已执行完全成功（succeeded，终态幂等拦截），或他人正在途
		// 执行（running，10 分钟抢占窗口内）——两种情形本回调都不再执行副作用，直接受理。
		return CallbackIdempotent, row.Status
	}

	handler, _ := s.registry.Get(in.Action)
	claimTs := time.Time{}
	if row.ClaimedAt != nil {
		claimTs = *row.ClaimedAt
	}
	if err := handler.Handle(ctx, in.Params); err != nil {
		updated, merr := s.repo.MarkFailed(ctx, in.TaskID, err.Error(), claimTs)
		if merr != nil {
			return CallbackRetryable, "回调受理失败"
		}
		if !updated {
			// 过期写者：fence 失配（10 分钟陈旧重认领后新执行已在途/已终态）——
			// 本执行的副作用已被新执行接管，静默受理 2xx，不再触发重投
			s.logger.Warn("jobs: stale writer mark failed skipped (fence miss)",
				"task_id", in.TaskID, "action", in.Action)
			return CallbackIdempotent, row.Status
		}
		if errors.Is(err, jobs.ErrAbort) {
			return CallbackNonRetryable, "动作执行失败（不可重试）: " + err.Error()
		}
		return CallbackRetryable, "动作执行失败（可重试）"
	}
	updated, err := s.repo.MarkSucceeded(ctx, in.TaskID, claimTs)
	if err != nil {
		// 执行已成功、记账 UPDATE 失败（极低概率）：业务事实已发生，必须 2xx；
		// 若 taskrunner 因超时等重试会再次进入 Handle——Handler 可重入契约兜底
		s.logger.Warn("jobs: mark succeeded failed after execution",
			"task_id", in.TaskID, "action", in.Action, "err", err)
	} else if !updated {
		// 过期写者（fence 失配）：副作用已被新执行接管，静默受理
		s.logger.Warn("jobs: stale writer mark succeeded skipped (fence miss)",
			"task_id", in.TaskID, "action", in.Action)
	}
	return CallbackExecuted, "succeeded"
}
