package service

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/jobs"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

// JobsCallbackService /internal/jobs 回调编排（E-②，16 号 §3）。
// handler 只做 HTTP 绑定/映射；幂等栅栏、动作分发、错误分类在本科。
type JobsCallbackService struct {
	registry *jobs.Registry
	repo     *repository.JobSubmissionRepo
	logger   *slog.Logger
	policy   CallbackActionPolicy
	activeMu sync.Mutex
	active   map[string]struct{}
}

// CallbackActionPolicy is configured once at startup. It obtains authority
// from durable admission or reviewed fixed-job configuration, never body.actor.
type CallbackActionPolicy interface {
	AuthorizeCallback(context.Context, string, string, json.RawMessage) (context.Context, error)
}

func (s *JobsCallbackService) SetActionPolicy(policy CallbackActionPolicy) { s.policy = policy }

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
	CallbackInProgress                           // 未证明终态成功 → 503，保留重试
	CallbackInvalidInput                         // 格式/存储宽度非法 → 400，零认领
	CallbackForbidden                            // 无当前动作资格/可信准入 → 403，零认领
)

// Execute 回调编排：未知动作拦截 → 幂等栅栏 → 分发执行 → 终态记账。
// ctx 应携带 request_id（jobs_handler 已把 body 值覆盖进 ctx 供落档）。
func (s *JobsCallbackService) Execute(ctx context.Context, in CallbackInput) (CallbackOutcome, string) {
	if strings.TrimSpace(in.TaskID) == "" || utf8.RuneCountInString(in.TaskID) > 64 ||
		utf8.RuneCountInString(in.Action) > 50 || utf8.RuneCountInString(in.RequestID) > 64 ||
		utf8.RuneCountInString(in.Actor) > 50 || utf8.RuneCountInString(in.SourceIP) > 50 || len(in.Params) > 64<<10 {
		return CallbackInvalidInput, "回调标识或参数超过约定范围"
	}
	if _, err := jobs.EqualParams(in.Params, in.Params); err != nil {
		return CallbackInvalidInput, "回调参数须为无重复键的合法 JSON"
	}
	if _, ok := s.registry.Get(in.Action); !ok {
		return CallbackUnknownAction, "未注册的动作: " + in.Action
	}
	// Reject an immutable snapshot mismatch before advancing an existing claim.
	// The post-claim check remains necessary when concurrent first deliveries race.
	if existing, err := s.repo.GetByTaskID(ctx, in.TaskID); err == nil {
		if existing.Action != in.Action || !paramsSemanticEqual(existing.Params, in.Params) {
			return CallbackNonRetryable, "task_id 凭证与回调内容不一致"
		}
	} else if !errors.Is(err, repository.ErrJobSubmissionNotFound) {
		return CallbackRetryable, "回调凭证读取失败"
	}
	if s.policy != nil {
		authorizedCtx, err := s.policy.AuthorizeCallback(ctx, in.TaskID, in.Action, in.Params)
		if err != nil {
			switch {
			case errors.Is(err, errcode.ErrNoPermission), errors.Is(err, errcode.ErrForbidden):
				return CallbackForbidden, "无当前动作资格或可信准入"
			case errors.Is(err, errcode.ErrInvalidParams), errors.Is(err, jobs.ErrAbort):
				return CallbackInvalidInput, "动作参数不符合受控配置"
			default:
				return CallbackRetryable, "当前动作资格尚不可确认"
			}
		}
		if authorizedCtx == nil {
			return CallbackRetryable, "当前动作资格尚不可确认"
		}
		ctx = authorizedCtx
	} else if in.Action == "audit_archive" {
		return CallbackForbidden, "维护动作缺少可信准入策略"
	}
	// The enforced single-instance profile has one callback service. Keep a live
	// handler exclusive even when its DB claim crosses the stale-recovery age.
	// This is process lifetime protection; durable claim/fence checks still own
	// restart recovery, and it is not a multi-replica execution lease.
	if !s.beginActive(in.TaskID) {
		return CallbackInProgress, "任务仍在执行，尚未确认完成"
	}
	defer s.endActive(in.TaskID)

	row, claimed, err := s.repo.ClaimCallbackRow(ctx, in.TaskID, in.Action, in.Actor, in.SourceIP, string(in.Params))
	if err != nil {
		return CallbackRetryable, "回调受理失败"
	}
	// W0b（二十批⑥）快照一致性：凭证行保留首记 action/params（Claim 冲突分支
	// 不采纳新值；查回路径返回既有行），此处与回调 body 比对——不匹配即 SK 泄露/
	// taskrunner 被控下挪用已存在 task_id 执行另一动作，409 拒（不可重试：重试同样
	// 不匹配）。比对先于幂等/在途拦截，防「借 succeeded 语义伪装受理」。
	// W0b-复审（commit-review §3.2）：params 比对改语义化 JSON 比较——字节精确
	// 比对会把等价形态（null/{}、键序空白差异、taskrunner 经 map 重序列化）
	// 误判 409 于合法回调；安全判别力在 action + params 语义值。
	if row.Action != in.Action || !paramsSemanticEqual(row.Params, in.Params) {
		s.logger.Error("callback snapshot mismatch rejected",
			"task_id", in.TaskID, "row_action", row.Action, "body_action", in.Action)
		return CallbackNonRetryable, "task_id 凭证与回调内容不一致"
	}
	if !claimed {
		if row.Status == repository.JobStatusSucceeded {
			return CallbackIdempotent, row.Status
		}
		return CallbackInProgress, "任务仍在执行，尚未确认完成"
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
			s.logger.Warn("jobs: stale writer mark failed skipped (fence miss)",
				"task_id", in.TaskID, "action", in.Action)
			return s.confirmCurrentOutcome(ctx, in)
		}
		if errors.Is(err, jobs.ErrAbort) {
			return CallbackNonRetryable, "动作执行失败（不可重试）: " + err.Error()
		}
		return CallbackRetryable, "动作执行失败（可重试）"
	}
	updated, err := s.repo.MarkSucceeded(ctx, in.TaskID, claimTs)
	if err != nil {
		s.logger.Warn("jobs: mark succeeded failed after execution",
			"task_id", in.TaskID, "action", in.Action, "err", err)
		return s.confirmCurrentOutcome(ctx, in)
	} else if !updated {
		s.logger.Warn("jobs: stale writer mark succeeded skipped (fence miss)",
			"task_id", in.TaskID, "action", in.Action)
		return s.confirmCurrentOutcome(ctx, in)
	}
	return CallbackExecuted, "succeeded"
}

func (s *JobsCallbackService) beginActive(taskID string) bool {
	s.activeMu.Lock()
	defer s.activeMu.Unlock()
	if _, exists := s.active[taskID]; exists {
		return false
	}
	if s.active == nil {
		s.active = make(map[string]struct{})
	}
	s.active[taskID] = struct{}{}
	return true
}

func (s *JobsCallbackService) endActive(taskID string) {
	s.activeMu.Lock()
	defer s.activeMu.Unlock()
	delete(s.active, taskID)
}

// A stale writer or failed ledger update cannot assert another writer finished.
// Only a matching, durable succeeded row permits a success acknowledgement.
func (s *JobsCallbackService) confirmCurrentOutcome(ctx context.Context, in CallbackInput) (CallbackOutcome, string) {
	row, err := s.repo.GetByTaskID(ctx, in.TaskID)
	if err != nil {
		return CallbackRetryable, "任务完成状态尚不可确认"
	}
	if row.Status == repository.JobStatusSucceeded && row.Action == in.Action && paramsSemanticEqual(row.Params, in.Params) {
		return CallbackIdempotent, row.Status
	}
	return CallbackInProgress, "任务完成状态尚不可确认"
}

// paramsSemanticEqual 回调 params 与凭证快照的语义等价比较：
// null/空均视为 {}；数字精确比较（含大整数和指数），键序/空白不敏感。
// 非法 JSON、重复键均拒绝，不因字节相同放行含歧义的输入。
func paramsSemanticEqual(a string, b json.RawMessage) bool {
	equal, err := jobs.EqualParams([]byte(a), b)
	return err == nil && equal
}
