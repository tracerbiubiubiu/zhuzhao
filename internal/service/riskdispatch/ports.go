package riskdispatch

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

// Reader 供 plan/apply 只读取数（*pgxpool.Pool 与 pgx.Tx 均满足）。
type Reader = repository.RiskDispatchReader

// DB 预览持久化用普通事务源（risk_dispatch_plans 只属本包）。
type DB interface {
	Reader
	Begin(ctx context.Context) (pgx.Tx, error)
}

// DispatchTransactions executes on the owning IAM transaction and context.
type DispatchTransactions interface {
	WithDispatchTransaction(context.Context, int64, func(context.Context, pgx.Tx) error) error
}

// DispatchScope 派单授权轴：单组沿 risk:manage+主锚/owner，多组沿主锚管理（详设 §5.1/§7）。
type DispatchScope = model.RiskDispatchScope

const (
	ScopeSingle = model.RiskDispatchSingle
	ScopeBulk   = model.RiskDispatchBulk
)

// RiskAuthorizer 风险域三层授权：对具名风险集合 × 目标核当前主体派单资格。
// 预检传池（读现势），apply 组内在守卫事务内复验（pgx.Tx 满足 Reader）；
// 拒绝返回 errcode.ErrForbidden。
type RiskAuthorizer interface {
	AuthorizeDispatch(ctx context.Context, db Reader, actorID int64,
		scope DispatchScope, riskIDs []int64, target Target) error
}

// RiskExpected 预览请求携带的乐观版本（详设 §2 Mutation 语义）。
type RiskExpected = model.RiskDispatchExpected

// RiskFact is a visible risk projection from its owning service. DispatchBasis
// is an opaque, stable version of the current verified basis. Empty means unavailable.
type RiskFact = model.RiskDispatchFact

// RiskFacts 风险域事实端口：预览按可见性投影，apply 在锁内取现势。
type RiskFacts interface {
	// CurrentDispatchFacts 仅返回当前主体可见风险的现势事实；不可见/不存在
	// 不返回也不报存在性（详设 §2.1「仅对已可见风险给逐项原因」）。
	CurrentDispatchFacts(ctx context.Context, db Reader, actorID int64,
		expected []RiskExpected) ([]RiskFact, error)
	// LockDispatchFacts apply 用：事务内按 ID 升序 FOR UPDATE 锁定并返回现势事实。
	LockDispatchFacts(ctx context.Context, tx pgx.Tx, ids []int64) ([]RiskFact, error)
}

// DispatchCommitItem 单风险的派单落账输入（ExpectedRevision 为 CAS 基线）。
type DispatchCommitItem = model.RiskDispatchCommitItem

// RiskDispatchCommit 风险域写端口：同一守卫事务内翻转状态并追加领域历史。
// 任一失败必须返回错误，令所在组整体回滚（组原子边界，详设 §5.1）。
type RiskDispatchCommit interface {
	CommitDispatched(ctx context.Context, tx pgx.Tx, items []DispatchCommitItem) error
}

// RiskDispatchLinkWriter 冻结工单桥 link 回调形态：只插/校验风险关联，
// 失败必须返回错误令建票事务回滚；回调内不得开启事务或产生其他副作用。
type RiskDispatchLinkWriter func(ctx context.Context, tx pgx.Tx, ticketID int64) error

// TicketPreparation / LockedTicketCreate 为不透明句柄（any 别名）：由组合根的
// 薄适配器包装工单桥真实类型（RiskDispatchCreatePreparation / LockedRiskDispatchCreate），
// 本包不解引用、不持久化；失败时适配器必须返回 nil 接口值。
type TicketPreparation = any

type LockedTicketCreate = any

// TicketOutcome 受限建票结果：票号 + 人读引用（桥侧分配 human_key）。
type TicketOutcome struct {
	ID  int64
	Ref string
}

// TicketCreator 受限共享创建端口（ticket-bridge-handoff.md 冻结 API 形状一一对应）。
// 实现：Prepare 事务外、每票一次；Lock 段锁 type/template→org 并复钉已发布定义；
// Create 段 created_by=当前已核 owner、created 历史 actor=真实操作者，link 回调先于文件绑定。
type TicketCreator interface {
	PrepareRiskDispatchCreate(ctx context.Context, req *model.CreateTicketRequest,
		actorUserID int64) (TicketPreparation, error)
	LockRiskDispatchCreateTx(ctx context.Context, tx pgx.Tx,
		prepared TicketPreparation) (LockedTicketCreate, error)
	CreateRiskDispatchTicketTx(ctx context.Context, tx pgx.Tx, locked LockedTicketCreate,
		ownerUserID int64, link RiskDispatchLinkWriter) (TicketOutcome, error)
	UpdateRiskDispatchPriorityTx(ctx context.Context, tx pgx.Tx, ticketID, actorUserID int64, priority int) error
}

// TicketViewAuthorizer 工单读权投影：决定原票引用是否对当前主体可见。
// 不可见返回 false，不抛错、不泄露存在性。
type TicketViewAuthorizer interface {
	CanViewTicket(ctx context.Context, actorUserID, ticketID int64) bool
}

// OwnerFact 责任人主体事实：在职状态 + 派单当时主部门。
type OwnerFact struct {
	UserID       int64
	Employed     bool
	PrimaryOrgID int64
}

// OwnerDirectory 主体事实端口：事务内锁读责任人当前状态与主部门。
type OwnerDirectory interface {
	LockOwnerFact(ctx context.Context, tx pgx.Tx, userID int64) (OwnerFact, error)
}

// DefinitionRef 已发布定义的规范引用（版本 + 64 位小写 hex 摘要）。
type DefinitionRef struct {
	Version int64
	Hash    string
}

// DefinitionResolver 计划期解析目标类型当前已发布定义（工单定义域实现）。
type DefinitionResolver interface {
	CurrentPublished(ctx context.Context, db Reader, typeCode string) (DefinitionRef, error)
}
