// Package riskreminder Phase6 B2 第8项——工单首次接单超时的站内提醒存储与执行内核。
//
// 职责边界（验收卡二主路径第 2 步 + risk-module-detail §14-3/§6 D15）：
//   - 分派事务内落「分派事实」（代次/指派人/分派时已发布的提醒参数/接单期限）——RecordAssignmentTx；
//   - 首次实际接单事务内保存原时间事实并抑止待发——RecordAcceptedTx；
//   - 显式 asOf 遍历生成持久待发意图（七元组投递键持久去重）——GenerateDue；
//   - 消费在现势收件人资格 + 仍待接单/当前代次复核后，站内入箱与成功事实同事务
//     （一次性效果；失败按原键恢复并明确留痕；资格撤回/接单/改派抑止留原因）——DeliverDue。
//
// 不做：外送通道、全站通知重建、提醒策略计算（时限/规则匹配由分派方发布）、
// 路由/权限码/Wire 装配（主会话统一接入）。站内入箱只经
// repository.NotificationInboxRepo.InsertTx 复用现有 inbox 语义。
//
// 锁序：IAM 共享守卫（与真实 IAM 撤权独占同键，Begin 前取得）→票→意图。
// 分派/接单事务沿既有 IAM→风险 scope→票→提醒行顺序。
// checker 只读，不得在票/意图锁后反向取得 IAM、风险或组锁。
package riskreminder

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

// D15 投递键固定分量（本内核只发一种语义：首接单超时站内提醒）。
// source_event_id = 分派事实 ID（assignment.id 的字符串形态），是可变的第 7 元前 3 键之一。
const (
	Source      = "ticket"
	Channel     = "inbox"
	SemanticKey = "accept_timeout"
)

// intent_status 意图状态机：pending →（delivered | suppressed）；终态不可逆。
const (
	StatusPending    = "pending"
	StatusDelivered  = "delivered"
	StatusSuppressed = "suppressed"
)

// suppress_reason 内核词表（资格撤回的原因串由 EligibilityChecker 提供，内核原样保存）。
const (
	ReasonAccepted   = "accepted"   // 首次实际接单抑止待发
	ReasonReassigned = "reassigned" // 新代次分派抑止旧代次待发
	ReasonSuperseded = "superseded" // 投递时复核发现已有更新代次
)

// 默认批大小（主会话骑 CronScheduler 时可显式传 limit 覆盖）。
const (
	DefaultScanLimit    = 200
	DefaultDeliverLimit = 100
)

// EligibilityChecker 在真实 IAM 共享守卫与票锁内读取现势收件资格。
// 扫描和投递各调用一次；只读并沿传入 tx，不得提交、另取连接或取得前序锁。
// false 留抑止原因；配置/读取错误返回 error，保留原键可恢复。
type EligibilityChecker func(ctx context.Context, tx pgx.Tx, ticketID, assigneeID int64) (eligible bool, reason string, err error)

// AssignmentInput 分派事实输入（RecordAssignmentTx）。
// Generation=分派代次：该票每次重新分派递增的计次，由票侧维护并传入；
// AcceptDueAt/RuleID/RuleVersion 取分派当时已发布的提醒参数，内核只存不解释。
type AssignmentInput struct {
	TicketID     int64
	Generation   int64
	AssigneeID   int64
	DispatchedBy int64
	DispatchedAt time.Time // 零值→NOW()；可控时钟用例显式传
	AcceptDueAt  time.Time // 接单期限（分派时发布，绝对时间）
	RuleID       string
	RuleVersion  int64
}

// Assignment 分派事实行。
type Assignment struct {
	ID           int64
	TicketID     int64
	Generation   int64
	AssigneeID   int64
	DispatchedBy int64
	DispatchedAt time.Time
	AcceptDueAt  time.Time
	RuleID       string
	RuleVersion  int64
	AcceptedAt   *time.Time // 首次实际接单原时间事实（只写一次，不覆盖）
	AcceptedBy   *int64
}

// Intent 待发/已发/抑止意图行（持久去重与一次性效果的载体）。
type Intent struct {
	ID              int64
	AssignmentID    int64
	TicketID        int64
	Generation      int64
	RecipientID     int64
	DueAt           time.Time
	Status          string
	Source          string
	SourceEventID   string
	RuleID          string
	RuleVersion     int64
	Channel         string
	SemanticKey     string
	NotificationID  *int64
	DeliveredAt     *time.Time
	SuppressedAt    *time.Time
	SuppressReason  *string
	AttemptCount    int
	LastAttemptedAt *time.Time
	LastError       *string
	CreatedAt       time.Time
}

// DeliveryReport DeliverDue 一轮的结果（读取失败须经此明确可见）。
type DeliveryReport struct {
	Delivered  []*Intent         // 入箱+成功事实同事务已提交（含提交确认不明但读回已终态者——报真实结果）
	Suppressed []*Intent         // 资格撤回/已接单/改派抑止（带原因）
	Failed     []DeliveryFailure // 失败留痕：状态仍 pending（或读回确认仍 pending），可按原键恢复
}

// DeliveryFailure 单条投递失败（attempt_count/last_error 已留痕）。
// Err 错误链保留根因——上下文取消经 errors.Is(err, context.Canceled /
// context.DeadlineExceeded) 可辨；已终态的意图不会进 Failed（读回后按真实
// 结果归入 Delivered/Suppressed）。
type DeliveryFailure struct {
	IntentID int64
	Err      error
	// Indeterminate 提交确认不明且读回不可达——真实送达状态未知：消费方
	// 不得据此宣称已送达，也不得宣称必未送达（留痕照写，恢复轮再仲裁）。
	Indeterminate bool
}

// Service 首接单超时提醒内核。
type Service struct {
	db      *pgxpool.Pool
	inbox   *repository.NotificationInboxRepo
	checker EligibilityChecker
	logger  *slog.Logger
	// runDeliveryTx owns the IAM barrier AND the business commit. Tests inject
	// ambiguous commit outcomes only at this complete transaction boundary.
	runDeliveryTx func(context.Context, func(context.Context, pgx.Tx) error) error
}

// NewService 创建。checker 允许 nil（装配先后解耦）：nil 时投递按失败留痕
// （fail-safe——不抑止、不入箱），checker 接线后按原键恢复。
func NewService(db *pgxpool.Pool, inbox *repository.NotificationInboxRepo, checker EligibilityChecker, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{db: db, inbox: inbox, checker: checker, logger: logger, runDeliveryTx: func(ctx context.Context, fn func(context.Context, pgx.Tx) error) error {
		return repository.WithIAMCommandGuard(ctx, db, fn)
	}}
}
