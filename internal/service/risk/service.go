package risk

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/casbin/casbin/v3"
	casbinmodel "github.com/casbin/casbin/v3/model"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tracerbiubiubiu/zhuzhao/internal/middleware"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/humankey"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

// Service 风险域写主（Phase 6 B2 第 6 项 D08/D09）。
// 分层：handler → Service → repository；L1 走路由 CasbinAuth（menu_apis 绑定），
// L2/L3 在本层判定（详设 §7）；写命令统一走 IAM 命令守卫并在事务内重验
// 当前凭据与当前 L1（C04/F017——与工单域同一排序屏障）。
type Service struct {
	db        *pgxpool.Pool
	humanKeys *humankey.Generator
	logger    *slog.Logger
	tickets   TicketVisibility
}

func NewService(db *pgxpool.Pool, logger *slog.Logger) *Service {
	return NewServiceWithTicketViews(db, logger, nil)
}

func NewServiceWithTicketViews(db *pgxpool.Pool, logger *slog.Logger, tickets TicketVisibility) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{db: db, humanKeys: humankey.New(db, "RISK"), logger: logger, tickets: tickets}
}

// 本批开放的路由（与 router/menu_apis 绑定一致；写命令 purpose 校验用）。
const (
	riskCreatePath = "/api/v1/risk-items/create"
	riskReportPath = "/api/v1/risk-items/report"
	riskAssessPath = "/api/v1/risk-items/assess"
	riskAssignPath = "/api/v1/risk-items/assign"
)

// withHTTPCommand 统一写命令守卫：IAM 命令守卫（共享排序屏障）内执行；
// HTTP 命令先核 purpose 形态，再在事务内重验当前凭据与当前 L1 授权。
// 直接服务调用者（评审过的内部编排/测试夹具）保留既有 L2/L3 合同，
// 不凭空获得 HTTP L1 身份。
func (s *Service) withHTTPCommand(ctx context.Context, path string, actor int64,
	fn func(context.Context, pgx.Tx) error) error {
	purpose, http := middleware.IAMRequestFromContext(ctx)
	guard := func(run func(context.Context, pgx.Tx) error) error {
		return repository.WithIAMCommandGuard(ctx, s.db, run)
	}
	if !http {
		return guard(fn)
	}
	if purpose.Path != path || purpose.Method != "POST" || purpose.SelfService || purpose.UserID != actor {
		return errcode.ErrNoPermission
	}
	return guard(func(ctx context.Context, tx pgx.Tx) error {
		if err := s.freshCredential(ctx, actor, purpose); err != nil {
			return err
		}
		if _, err := s.freshRoles(ctx, actor, purpose.Model, path); err != nil {
			return err
		}
		return fn(ctx, tx)
	})
}

// freshCredential 在守卫事务内重验当前凭据（会话代际 / PAT 现势）。
// 请求体永不提供这些事实——沿 C05 提供的账号安全快照判定。
func (s *Service) freshCredential(ctx context.Context, actor int64, purpose middleware.IAMRequestPurpose) error {
	if purpose.SessionEpoch != nil && purpose.PATID == 0 {
		var current int64
		err := repository.IAMDB(ctx, s.db).QueryRow(ctx,
			`SELECT session_epoch FROM users WHERE id=$1 AND status=1 AND deleted_at IS NULL`, actor).Scan(&current)
		if errors.Is(err, pgx.ErrNoRows) {
			return errcode.ErrNoPermission
		}
		if err != nil {
			return fmt.Errorf("current risk session: %w", err)
		}
		if current < 1 || current != *purpose.SessionEpoch {
			return errcode.ErrNoPermission
		}
		return nil
	}
	if purpose.PATID > 0 && purpose.SessionEpoch == nil {
		active, err := repository.NewPatRepo(s.db).IsActiveForUser(ctx, purpose.PATID, actor)
		if err != nil {
			return fmt.Errorf("current risk PAT: %w", err)
		}
		if active {
			return nil
		}
	}
	return errcode.ErrNoPermission
}

// freshRoles 在守卫事务内重验当前 L1（DB 现势角色与 p 行，不用内存策略）。
func (s *Service) freshRoles(ctx context.Context, actor int64, modelText, path string) ([]string, error) {
	users := repository.NewUserRepo(s.db)
	account, err := users.FindByID(ctx, actor)
	if errors.Is(err, errcode.ErrUserNotFound) {
		return nil, errcode.ErrNoPermission
	}
	if err != nil {
		return nil, err
	}
	if account.Status != 1 {
		return nil, errcode.ErrNoPermission
	}
	roles, policies, err := users.CurrentIAMAuthorization(ctx, actor)
	if err != nil {
		return nil, fmt.Errorf("fresh risk authorization: %w", err)
	}
	if len(roles) == 0 {
		return nil, errcode.ErrNoRoles
	}
	m, err := casbinmodel.NewModelFromString(modelText)
	if err != nil {
		return nil, errcode.ErrNoPermission
	}
	enforcer, err := casbin.NewEnforcer(m)
	if err != nil {
		return nil, errcode.ErrNoPermission
	}
	if _, err := enforcer.AddPolicies(policies); err != nil {
		return nil, errcode.ErrNoPermission
	}
	for _, role := range roles {
		granted, enforceErr := enforcer.Enforce("role::"+role, path, "POST")
		if enforceErr != nil {
			return nil, errcode.ErrNoPermission
		}
		if granted {
			return roles, nil
		}
	}
	return nil, errcode.ErrNoPermission
}

// replayAuthorized 同号重放的当前对象复核（外审阻断②）：重放发生在守卫事务内，
// 原收据回链的风险按现行 L2/L3 重判——不可见与不存在同 404，可见而缺当前 L3
// （create/assign=主锚管理专属；assess=管理 ∨ owner）才 403；复核通过才返回原收据，
// 失权者不得从重放读到业务元数据。
func replayAuthorized(ctx context.Context, tx pgx.Tx, receipt *repository.RiskOperationRow, actor int64) error {
	if receipt.ResultRiskID == nil {
		return nil // 未回链对象即无业务元数据可泄露
	}
	_, err := repository.RiskItemAuthorizedTx(ctx, tx, *receipt.ResultRiskID, actor, receipt.Command != "assess")
	return err
}

// ── 视图装配 ────────────────────────────────────────────────────────────

func riskRefFromRow(r *repository.RiskItemRow) model.RiskRef {
	return model.RiskRef{ID: r.ID, HumanKey: r.HumanKey, Revision: r.Revision}
}

// dispatchBlockers 派单准备阻断判据（第 7 项派单预览的拒绝输入）：
// 责任未确认 owner_unconfirmed；未评估 not_assessed。
func dispatchBlockers(r *repository.RiskItemRow) []string {
	var blockers []string
	if r.OwnerUserID == nil {
		blockers = append(blockers, "owner_unconfirmed")
	}
	if r.Level == nil {
		blockers = append(blockers, "not_assessed")
	}
	if blockers == nil {
		blockers = []string{}
	}
	return blockers
}

func severityFromJSON(raw []byte) *model.RiskSourceSeverity {
	if len(raw) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil
	}
	var sev model.RiskSourceSeverity
	if err := json.Unmarshal(raw, &sev); err != nil || sev.Scheme == "" && sev.Value == "" {
		return nil
	}
	return &sev
}

func viewFromRow(r *repository.RiskItemRow, severityJSON []byte) model.RiskView {
	view := model.RiskView{
		Risk:             riskRefFromRow(r),
		Title:            r.Title,
		Status:           r.Status,
		ClosureKind:      r.ClosureKind,
		Level:            r.Level,
		OwnerUserID:      r.OwnerUserID,
		OrgID:            r.OrgID,
		AffectedOrgID:    r.AffectedOrgID,
		Deadline:         r.Deadline,
		InitDeadline:     r.InitDeadline,
		Treatment:        r.Treatment,
		SourceSeverity:   severityFromJSON(severityJSON),
		DispatchBlockers: dispatchBlockers(r),
	}
	if r.AssetProvider != nil && r.AssetExternalID != nil {
		view.AssetRef = &model.RiskAssetRef{Provider: *r.AssetProvider, ExternalID: *r.AssetExternalID}
	}
	if r.FindingKey != nil {
		view.FindingKey = *r.FindingKey
	}
	return view
}

func receiptFromRow(r *repository.RiskOperationRow, replayed bool) model.RiskReceipt {
	rc := model.RiskReceipt{
		OperationID: r.OperationID,
		Command:     r.Command,
		Status:      r.ResultStatus,
		CreatedAt:   r.CreatedAt,
		Replayed:    replayed,
	}
	if r.ResultRiskID != nil {
		rc.RiskID = *r.ResultRiskID
	}
	if r.ResultRevision != nil {
		rc.Revision = *r.ResultRevision
	}
	return rc
}

func replayResult(r *repository.RiskOperationRow) *model.RiskMutationResult {
	res := &model.RiskMutationResult{Receipt: receiptFromRow(r, true)}
	if r.ResultRiskID != nil {
		res.Risk = model.RiskRef{ID: *r.ResultRiskID}
		if r.ResultRevision != nil {
			res.Risk.Revision = *r.ResultRevision
		}
	}
	res.Status = r.ResultStatus
	res.Level = nil
	return res
}

func jsonBytes(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		return []byte("{}")
	}
	return b
}

func newIngestID() string { return uuid.NewString() }
