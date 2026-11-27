package ticket

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tracerbiubiubiu/zhuzhao/internal/middleware"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	commentref "github.com/tracerbiubiubiu/zhuzhao/internal/model/comment"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/datafetcher"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/domainevent"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/humankey"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/resource"
	pkgstorage "github.com/tracerbiubiubiu/zhuzhao/internal/pkg/storage"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

// Service 工单服务（Phase 2a MVP）
type Service struct {
	b3                *repository.B3Store
	b3DispatchEnabled bool
	db                *pgxpool.Pool
	ticketRepo        *repository.TicketRepo
	orgRepo           *repository.OrgRepo
	registry          resource.Registry
	roleFetcher       middleware.RoleFetcher
	delegation        OrgDelegationChecker
	storageRepo       *repository.StorageRepo // B5-3：建单前附件落绑（域间经数据层共享——禁 service 互 import）
	storageCfg        *pkgstorage.ConfigStore // F-D3：原子快照（附件数上限——05 号 §3 个数上限建单路径执行点）
	fetcherReg        *datafetcher.Registry   // B1-4：取数联动（nil=无注册源，取数跳过）
	eventReg          *domainevent.HookRegistry
	keyGen            *humankey.Generator      // B6-1：领域事件（nil=不派发——首版装配态）
	ccRepo            *repository.TicketCCRepo // S10 B10-1：抄送（自建自用——wire 零改）
	tagRepo           *repository.TagRepo      // S10 B10-2：标签（自建自用——wire 零改）
	dispatcher        *DispatchService         // S3a B3-2：自动分派（OnAfterCreate——app 装配注入）
	actionReg         *ActionRegistry          // S3a B3-3：动作库（post_actions 消费——app 装配注入）
}

// NewTicketService 创建工单服务并自注册 TicketResource 到 Registry。
// Wire 语义保证：Registry 单例，TicketService 构造（自注册）先于 router.New。
// SetEventRegistry B6-1：注入领域事件注册表（app 装配层调用——wire_gen 零改；
// nil=不派发，首版装配态安全默认）。
// SetDispatcher S3a B3-2：注入自动分派器（app 装配层调用——nil=不分派）。
func (s *Service) SetDispatcher(d *DispatchService) *Service {
	s.dispatcher = d
	return s
}

// SetActionRegistry S3a B3-3：注入动作库（post_actions 消费——nil=不执行）。
func (s *Service) SetActionRegistry(r *ActionRegistry) *Service {
	s.actionReg = r
	return s
}

func (s *Service) SetEventRegistry(reg *domainevent.HookRegistry) *Service {
	s.eventReg = reg
	return s
}

// emitTicketEvent 在提交后派发，并从本次业务读取的工单快照携带创建者。
// 创建者取自快照，排队期间删单也能区分原创建者与操作者。
func (s *Service) emitTicketEvent(ticket *model.Ticket, ev domainevent.DomainEvent) {
	if s.eventReg != nil {
		s.eventReg.Dispatch(context.Background(), withTicketIdentity(withTicketSnapshot(ticket, ev), ticket.CreatedBy))
	}
}

// withTicketIdentity 区分工单创建者与本次操作者；复制载荷防止修改其他订阅者使用的map。
// 扫描事件没有操作者（ActorID=0），保持该语义；缺创建者时不猜测回退到actor。
func withTicketIdentity(ev domainevent.DomainEvent, creatorID int64) domainevent.DomainEvent {
	payload := make(domainevent.Payload, len(ev.Payload)+2)
	for key, value := range ev.Payload {
		payload[key] = value
	}
	if creatorID > 0 {
		payload["creator"] = strconv.FormatInt(creatorID, 10)
	}
	if ev.ActorID > 0 {
		payload["actor"] = strconv.FormatInt(ev.ActorID, 10)
	}
	ev.Payload = payload
	return ev
}

// EmitActionNotify post_action notify 动作的事件出口（NotifyAction 专用——
// 走与 created/assigned/closed 同一事件编排链；at-most-once 异步）。
// 载荷键 cc_users=cc 档收件人锚点复用（resolveRecipients cc 分支）。
func (s *Service) EmitActionNotify(ctx context.Context, ticket *model.Ticket, actorID int64, recipients, templateCode string) {
	_ = ctx // 事件派发为异步 fire-and-forget（emitTicketEvent 内部 Dispatch 自管生命周期）
	payload := domainevent.Payload{
		"title": ticket.Title, "human_key": ticket.HumanKey,
		"cc_users": recipients,
	}
	// S3a B3-3：动作显式指定的模板码（2026-10-26 通知面收口批生效——空=走规则/种子默认）
	if templateCode != "" {
		payload["template_code"] = templateCode
	}
	s.emitTicketEvent(ticket, domainevent.DomainEvent{
		BizType: domainevent.BizTicket, Action: domainevent.TicketActionNotify,
		BizID: strconv.FormatInt(ticket.ID, 10), ActorID: actorID,
		Payload: payload,
	})
}

func NewTicketService(
	db *pgxpool.Pool,
	ticketRepo *repository.TicketRepo,
	orgRepo *repository.OrgRepo,
	registry resource.Registry,
	roleFetcher middleware.RoleFetcher,
	delegation OrgDelegationChecker,
	storageRepo *repository.StorageRepo,
	storageCfg *pkgstorage.ConfigStore,
	fetcherReg *datafetcher.Registry,
) *Service {
	ccRepo := repository.NewTicketCCRepo(db)
	tagRepo := repository.NewTagRepo(db)
	s := &Service{
		db:          db,
		ticketRepo:  ticketRepo,
		orgRepo:     orgRepo,
		registry:    registry,
		storageCfg:  storageCfg,
		fetcherReg:  fetcherReg,
		keyGen:      humankey.New(db, "TK"),
		storageRepo: storageRepo,
		roleFetcher: roleFetcher,
		delegation:  delegation,
		ccRepo:      ccRepo,
		tagRepo:     tagRepo,
	}
	// 自注册 TicketResource（§2.5 Wire 自注册）；2b 策略 B 透明读 + 2c 组织委托
	// + S10 B10-1 抄送特例支（读可见集）
	registry.Register(NewResource(s.ticketRepo, NewPgxScopeResolver(db), delegation))
	return s
}

// isGlobalOrgAdmin 全局组织管理员判定（commit-review §3.4 口径统一）：
// 角色码 admin/superadmin 短路，回退 delegation.HasOrgManagePermission
// （org:member功能码）——其它org按钮不构成Create归属校验豁免。
func (s *Service) isGlobalOrgAdmin(ctx context.Context, userID int64) bool {
	roles, err := s.getRoles(ctx, userID)
	if err != nil {
		return false
	}
	for _, r := range roles {
		if r == "admin" || r == "superadmin" {
			return true
		}
	}
	ok, err := s.delegation.HasOrgManagePermission(ctx, userID)
	return err == nil && ok
}

// getRoles 获取用户角色码列表
func (s *Service) getRoles(ctx context.Context, userID int64) ([]string, error) {
	if current, ok := ctx.Value(ticketCommandRolesKey{}).(ticketCommandRoles); ok && current.actor == userID {
		return current.roles, nil
	}
	return s.roleFetcher.GetRoleCodesByUserID(ctx, userID)
}

// authorize 资源级鉴权封装
func (s *Service) authorize(ctx context.Context, userID int64, action, resourceID string) (bool, error) {
	roles, err := s.getRoles(ctx, userID)
	if err != nil {
		return false, fmt.Errorf("fetch roles: %w", err)
	}
	return s.registry.Authorize(ctx, "ticket", resource.AuthorizeRequest{
		UserID:     userID,
		Roles:      roles,
		Action:     action,
		ResourceID: resourceID,
	})
}

// authorizeCheck 统一处理 authorize 返回值，区分 404/403/500：
//   - err == errDenied → 可见但无权限 → ErrNoPermission(403)
//   - err != nil（其他） → DB 错误 → 原样上抛（handler 转 500）
//   - err == nil && !ok → 不存在/不可见 → ErrTicketNotFound(404)
//   - err == nil && ok → nil（放行）
func (s *Service) authorizeCheck(ctx context.Context, userID int64, action, resourceID string) error {
	ok, err := s.authorize(ctx, userID, action, resourceID)
	if err != nil {
		if errors.Is(err, errDenied) {
			return errcode.ErrNoPermission // 可见但无权限 → 403
		}
		return err // DB 错误 → 原样上抛 → 500
	}
	if !ok {
		return errcode.ErrTicketNotFound // 不存在/不可见 → 404
	}
	return nil
}

// --- CRUD ---

// defaultTicketPriority 类型级缺省优先级（与 000010 tickets.priority DEFAULT 3 对齐）
const defaultTicketPriority = 3

// Create 创建工单
func (s *Service) Create(ctx context.Context, req *model.CreateTicketRequest, actorUserID int64) (*model.Ticket, error) {
	if req == nil {
		return nil, errcode.ErrInvalidParams
	}
	// W0b（二十四批 P0-5）：assigned_to 在创建请求中拒收——分派须走 Assign 端点
	// 过状态机（open→assigned 事件+指派人校验）；此处直传会落库 open 态带处理人。
	if req.AssignedTo != nil {
		return nil, errcode.New(errcode.ErrInvalidParams.Code, "assigned_to 不允许在创建时指定，请使用分派接口")
	}
	// Reject outside the requested org before any fetch provider is called.
	// The same check is repeated inside the admitting transaction.
	if err := s.authorizeCreateOrg(ctx, actorUserID, req.OrgID); err != nil {
		return nil, err
	}
	p, err := s.prepareTicketCreate(ctx, req, actorUserID)
	if err != nil {
		return nil, err
	}
	var ticket *model.Ticket
	err = s.withHTTPCommand(ctx, "create", actorUserID, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		ticket, err = s.lockPreparedCreateTx(ctx, tx, p, true)
		if err != nil {
			return err
		}
		return s.writePreparedCreateTx(ctx, tx, p, ticket, nil)
	})
	if err != nil {
		return nil, err
	}
	// B6-1：工单创建事件（事务提交后派发——06 号 §5.2a 异步投递）
	s.emitTicketEvent(ticket, domainevent.DomainEvent{
		BizType: domainevent.BizTicket, Action: domainevent.TicketCreated,
		BizID: strconv.FormatInt(ticket.ID, 10), ActorID: actorUserID,
		Payload: domainevent.Payload{
			"type_code": ticket.TypeCode, "title": ticket.Title, "status": ticket.Status,
			"human_key": ticket.HumanKey,
			// actor仅作操作者；creator锚由emitTicketEvent从工单快照统一注入。
			"actor": strconv.FormatInt(actorUserID, 10),
		},
		Timestamp: time.Now(),
	})
	// S3a B3-2：OnAfterCreate 自动分派（决议⑤——失败不回滚创建；在事件 emit 后
	// 调用=事务已提交，分派走 Assign 全链含事件）
	if s.dispatcher != nil {
		s.dispatcher.OnAfterCreate(ctx, ticket)
	}
	return ticket, nil
}

// authorizeCreateOrg uses the same branch/permission decision before preparation
// and inside the command guard, where IAM reads use the owning transaction.
func (s *Service) authorizeCreateOrg(ctx context.Context, actor, orgID int64) error {
	member, err := s.orgRepo.IsInOrgBranch(ctx, orgID, actor)
	if err != nil {
		return err
	}
	if !member && !s.isGlobalOrgAdmin(ctx, actor) {
		return errcode.ErrNoPermission
	}
	return nil
}

// Get 获取工单详情（含 L2 可见性校验）
func (s *Service) Get(ctx context.Context, id int64, actorUserID int64) (*model.Ticket, error) {
	// Preserve the resource decision audit; fetch only a row that remains visible.
	if err := s.authorizeCheck(ctx, actorUserID, "read", strconv.FormatInt(id, 10)); err != nil {
		return nil, err
	}
	filter, err := s.resolveListFilter(ctx, actorUserID)
	if err != nil {
		return nil, err
	}
	ticket, err := s.ticketRepo.GetVisibleByID(ctx, id, filter)
	if err != nil {
		return nil, err
	}
	if s.b3 != nil {
		ticket.TaskRuns, err = s.b3.TicketRuns(ctx, id)
		if err != nil {
			return nil, err
		}
	}
	return s.describeTicketDefinition(ctx, ticket)
}

// List 工单列表（L2 行级过滤；admin bypass L2）
func (s *Service) List(ctx context.Context, q model.TicketListQuery, actorUserID int64) (*model.TicketListResponse, error) {
	filter, err := s.resolveListFilter(ctx, actorUserID)
	if err != nil {
		return nil, err
	}
	tickets, total, err := s.ticketRepo.List(ctx, filter, q)
	if err != nil {
		return nil, err
	}
	return &model.TicketListResponse{
		List:     tickets,
		Total:    total,
		Page:     q.Page,
		PageSize: q.PageSize,
	}, nil
}

// resolveListFilter List/Export 共用的行级过滤解析（BK-21 同源 service 面）：
// admin/superadmin 显式 Unscoped，其余走 registry.GetFilter（cc 特例支/L2 三轴同源）。
func (s *Service) resolveListFilter(ctx context.Context, actorUserID int64) (resource.Filter, error) {
	roles, err := s.getRoles(ctx, actorUserID)
	if err != nil {
		return resource.Filter{}, fmt.Errorf("fetch roles: %w", err)
	}
	if HasRole(roles, "admin") || HasRole(roles, "superadmin") {
		// admin bypass L2：显式豁免（IW4——零值 Filter{} 已被 repo 哨兵拒绝，防漏接静默全量）
		return resource.Filter{Unscoped: true}, nil
	}
	return s.registry.GetFilter(ctx, "ticket", actorUserID, "read")
}

// MaxExportRows 导出行数钳制（A.6：≈5 万——超限 400 提示收窄筛选；与审计导出挂账口径对齐）
const MaxExportRows = 50000

// ErrExportTooMany 导出超限（service 层语义码——handler 转 400）
var ErrExportTooMany = errcode.New(errcode.ErrInvalidParams.Code, "导出行数超过上限 50000，请收窄筛选条件")

// ExportStream 导出取行（S10 B10-5）：与 List 同 resolveListFilter + 同
// buildTicketWhere（BK-21 代码级同源）。COUNT只做预检，实际流仍逐行校验5万上限；
// 第50001行报错而非截断成功。成功返回值=实际生成行数，供导出审计使用。
func (s *Service) ExportStream(ctx context.Context, q model.TicketListQuery, actorUserID int64, rowFn func(*model.Ticket) error) (int64, error) {
	filter, err := s.resolveListFilter(ctx, actorUserID)
	if err != nil {
		return 0, err
	}
	total, err := s.ticketRepo.CountByFilter(ctx, filter, q)
	if err != nil {
		return 0, err
	}
	if total > MaxExportRows {
		return total, ErrExportTooMany
	}
	var exported int64
	err = s.ticketRepo.StreamRowsByFilter(ctx, filter, q, func(ticket *model.Ticket) error {
		if exported >= MaxExportRows {
			return ErrExportTooMany
		}
		if err := rowFn(ticket); err != nil {
			return err
		}
		exported++
		return nil
	})
	return exported, err
}

// FieldImpact B1-5 每字段在途使用计数（全量替换前的变更代价提示——软告警
// 不阻断）。返回 = field_key → 在途工单数。
func (s *Service) FieldImpact(ctx context.Context, typeCode string) (map[string]int, error) {
	return s.ticketRepo.FieldImpact(ctx, typeCode)
}

// applyFetches B1-4 取数联动（A.4）：从 field_meta 组装 FetchField 投影 →
// datafetcher.ApplyFetches 回写。失败不阻塞（A.4 失败策略）——missed 记日志。
func (s *Service) applyFetches(typeCode string, customData json.RawMessage, actorUserID int64, fields []*model.TicketTypeField) json.RawMessage {
	if s.fetcherReg == nil || len(fields) == 0 {
		return customData
	}
	var schema []datafetcher.FetchField
	for _, f := range fields {
		if len(f.FieldMeta) == 0 {
			continue
		}
		var meta struct {
			Fetch struct {
				Source    datafetcher.FetchSource `json:"source"`
				TriggerBy string                  `json:"trigger_by"`
				Mapping   map[string]string       `json:"mapping"`
			} `json:"fetch"`
		}
		if err := json.Unmarshal(f.FieldMeta, &meta); err != nil || meta.Fetch.Source.Type == "" {
			continue
		}
		ff := datafetcher.FetchField{
			Key: f.FieldKey, Source: meta.Fetch.Source,
			TriggerBy: meta.Fetch.TriggerBy, Mapping: meta.Fetch.Mapping,
		}
		if ff.TriggerBy == "" {
			ff.TriggerBy = f.FieldKey
		}
		schema = append(schema, ff)
	}
	if len(schema) == 0 {
		return customData
	}
	var data map[string]any
	if err := json.Unmarshal(customData, &data); err != nil || data == nil {
		return customData
	}
	out, missed, err := datafetcher.ApplyFetches(s.fetcherReg, schema, data, datafetcher.Actor{UserID: actorUserID})
	if err != nil {
		return customData // 取数面错误不阻塞提交
	}
	if len(missed) > 0 {
		slog.Warn("ticket fetch linkage missed", "type_code", typeCode, "missed", missed)
	}
	b, err := json.Marshal(out)
	if err != nil {
		return customData
	}
	return json.RawMessage(b)
}

// Update 更新工单（patch 语义）
func (s *Service) Update(ctx context.Context, req *model.UpdateTicketRequest, actorUserID int64) (*model.Ticket, error) {
	var ticket *model.Ticket
	err := s.withHTTPCommand(ctx, "update", actorUserID, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		ticket, err = s.updateCommand(ctx, tx, req, actorUserID)
		return err
	})
	if err != nil {
		return nil, err
	}
	// B6-1/D20：工单更新事件（载荷带 changes[] 字段级新值 diff——旧值比对/custom_data 键级待 B-1，勿以「键级」表述引用本载荷）
	updatedPayload := domainevent.Payload{
		"title": ticket.Title, "type_code": ticket.TypeCode,
		"human_key": ticket.HumanKey, "status": ticket.Status,
		"changes": buildChanges(req, ticket), // 旧值/键级diff继续归B-1，不在通知接线批改语义。
	}
	if ticket.AssignedTo != nil {
		updatedPayload["to_assignee"] = strconv.FormatInt(*ticket.AssignedTo, 10)
	}
	s.emitTicketEvent(ticket, domainevent.DomainEvent{
		BizType: domainevent.BizTicket, Action: domainevent.TicketUpdated,
		BizID: strconv.FormatInt(ticket.ID, 10), ActorID: actorUserID,
		Payload: updatedPayload, Timestamp: time.Now(),
	})
	return ticket, nil
}

// updateCommand persists the patch and durable event on the admitted connection.
func (s *Service) updateCommand(ctx context.Context, tx pgx.Tx, req *model.UpdateTicketRequest, actorUserID int64) (*model.Ticket, error) {
	if err := s.authorizeCheck(ctx, actorUserID, "update", strconv.FormatInt(req.ID, 10)); err != nil {
		return nil, err
	}

	ticket, err := s.ticketRepo.GetByIDForUpdateTx(ctx, tx, req.ID)
	if err != nil {
		return nil, err
	}
	_, fields, ref, err := s.resolveTicketDefinition(ctx, ticket)
	if err != nil {
		return nil, err
	}
	if ticket.Status == StatusClosed || ticket.Status == StatusCancelled {
		return nil, errcode.ErrTicketAlreadyClosed
	}
	// patch 语义：仅校验显式传入的 priority（W4 十六批 Med——写侧改字段级 COALESCE，
	// nil 字段不覆盖；此处读出的 ticket 仅作授权/状态预检及类型识别）
	if req.Priority != nil && (*req.Priority < 1 || *req.Priority > 4) {
		return nil, errcode.New(errcode.ErrInvalidParams.Code, "priority 须为 1–4（1紧急 2高 3中 4低）")
	}
	if req.Priority != nil && *req.Priority != ticket.Priority {
		binding, err := s.ticketRepo.ReadActiveRiskTicketBindingTx(ctx, tx, req.ID)
		if err != nil {
			return nil, err
		}
		if len(binding) > 0 {
			return nil, errcode.ErrNoPermission
		}
	}
	// B5-4 custom_data 增量 patch（05 号 §6/A.3 拍板项）：传入键覆盖、null 删键；合并后
	// 整体 G2 重校验（含未改动字段——防基线外字段绕进、required 被 null 清空）。
	// 工单行锁内读取基线和绑定版本，避免并发保存丢失已写字段。
	var customData []byte
	var fieldRefs []repository.StorageFieldReference
	if len(req.CustomData) > 0 {
		base := ticket.CustomData
		merged, err := mergeCustomData(base, req.CustomData)
		if err != nil {
			return nil, errcode.New(errcode.ErrInvalidParams.Code, "custom_data 须为 JSON 对象")
		}
		merged, err = s.validateCustomData(ctx, tx, ticket.TypeCode, merged, base, fields)
		if err != nil {
			return nil, err
		}
		fieldRefs, err = fieldReferences(fields, merged)
		if err != nil {
			return nil, err
		}
		customData = merged
	}
	ticket, err = s.ticketRepo.UpdateTx(ctx, tx, req.ID, req.Title, req.Description, req.Priority, customData)
	if err != nil {
		return nil, err
	}
	// B5-4 编辑增量绑定（05 号 A.3 a 案）：编辑已建单工单时新贴图/附件（悬挂 confirmed 对象）
	// 随保存同事务落绑——与建单同款语义。UpdateTx 的 UPDATE 已持工单行锁且先行，
	// 计数校验无 TOCTOU 窗口（与 ConfirmAndBind 的 FOR UPDATE tickets 同序）。
	if err := s.bindUpdateReferencesTx(ctx, tx, ticket.ID, actorUserID, req.Attachments, fieldRefs); err != nil {
		return nil, err
	}
	if err := s.ticketRepo.CreateEventTx(ctx, tx, &model.TicketEvent{
		TicketID: ticket.ID,
		UserID:   actorUserID,
		Action:   EventUpdated,
	}); err != nil {
		return nil, err
	}
	// Reuse the transaction connection for the snapshot's display references.
	s.ticketRepo.EnrichSnapshotUserNamesTx(ctx, tx, ticket)
	ticket.DefinitionRef = ref
	ticket.DefinitionResolution = "exact"
	return ticket, nil
}

// mergeCustomData custom_data 增量 patch 合并（B5-4）：patch 须为 JSON 对象；其键覆盖基线，
// 值为 JSON null 的键删除（清空字段）；基线为空按空对象起算。返回合并后的紧凑 JSON。
func mergeCustomData(base, patch json.RawMessage) (json.RawMessage, error) {
	merged := map[string]json.RawMessage{}
	if len(base) > 0 {
		if err := json.Unmarshal(base, &merged); err != nil {
			return nil, err
		}
		if merged == nil {
			merged = map[string]json.RawMessage{}
		}
	}
	var p map[string]json.RawMessage
	if err := json.Unmarshal(patch, &p); err != nil || p == nil {
		return nil, fmt.Errorf("custom_data patch must be a JSON object")
	}
	for k, v := range p {
		if string(v) == "null" {
			delete(merged, k)
			continue
		}
		merged[k] = v
	}
	return json.Marshal(merged)
}

// Close 关闭工单（状态机校验 + 事件）
func (s *Service) Close(ctx context.Context, req *model.CloseTicketRequest, actorUserID int64) error {
	var ticket *model.Ticket
	err := s.withHTTPCommand(ctx, "close", actorUserID, func(ctx context.Context, owner pgx.Tx) error {
		var err error
		if err := s.authorizeCheck(ctx, actorUserID, "close", strconv.FormatInt(req.ID, 10)); err != nil {
			return err
		}

		ticket, err = s.ticketRepo.GetByID(ctx, req.ID)
		if err != nil {
			return err
		}
		ttype, _, _, err := s.resolveTicketDefinition(ctx, ticket)
		if err != nil {
			return err
		}
		if ticket.Status == StatusClosed || ticket.Status == StatusCancelled {
			return errcode.ErrTicketAlreadyClosed
		}
		sm, err := FromTicketType(ttype)
		if err != nil {
			return fmt.Errorf("build state machine: %w", err)
		}
		if err := sm.AssertTransition(ticket.Status, StatusClosed); err != nil {
			return err
		}
		// R3 审计修复：Close 也走 condition+validator（原旁路——/close 端点绕过
		// transition_meta 语义检查，phase6 依赖 condition 做权限时=安全漏洞）
		if _, err := s.checkTransitionMeta(ctx, ticket, StatusClosed, actorUserID, ttype); err != nil {
			return err
		}

		// 更新状态 + 写事件 + 可选关闭说明
		tx := owner
		ownTx := tx == nil
		if ownTx {
			tx, err = s.db.Begin(ctx)
			if err != nil {
				return fmt.Errorf("begin tx: %w", err)
			}
			defer tx.Rollback(ctx)
		}

		if err := s.ticketRepo.LockTransitionSnapshotTx(ctx, tx, ticket, nil); err != nil {
			return err
		}
		if err := s.rejectRiskLifecycleCommandTx(ctx, tx, "close", ticket.ID); err != nil {
			return err
		}
		if err := s.ticketRepo.UpdateStatusTx(ctx, tx, req.ID, ticket.Status, StatusClosed); err != nil {
			return err
		}
		if err := s.applyStateSLARuleTx(ctx, tx, ttype, ticket, StatusClosed); err != nil {
			return err
		}
		if err := s.ticketRepo.CreateEventTx(ctx, tx, &model.TicketEvent{
			TicketID:  req.ID,
			UserID:    actorUserID,
			Action:    EventStatusChanged,
			FromValue: ticket.Status,
			ToValue:   StatusClosed,
		}); err != nil {
			return err
		}
		// 可选关闭说明 → 写评论
		if err := s.writeLifecycleCommentTx(ctx, tx, req.ID, actorUserID, req.Comment); err != nil {
			return err
		}
		if ownTx {
			if err := tx.Commit(ctx); err != nil {
				return err
			}
		}
		return nil
	}, req.ID)
	if err != nil {
		return err
	}
	// D1 审计修复：关闭=状态离开旧状态——清旧状态催办水位（与 Transition 同源；
	// 修复前 assigned 催满→关闭→重开回 assigned 残留水位致永不催）
	s.clearReminderWatermark(ctx, req.ID, ticket.Status)
	// B6-5：工单关闭事件（事务提交后派发）
	closedPayload := domainevent.Payload{
		"title": ticket.Title, "type_code": ticket.TypeCode,
		"status": StatusClosed,
	}
	// B6-4 收件人锚点：assignee 档通知链读此键——修复前 payload 缺此键
	// 致 closed 通知 100% 静默丢失（外审实例批发现）
	if ticket.AssignedTo != nil {
		closedPayload["to_assignee"] = strconv.FormatInt(*ticket.AssignedTo, 10)
	}
	s.emitTicketEvent(ticket, domainevent.DomainEvent{
		BizType: domainevent.BizTicket, Action: domainevent.TicketClosed,
		BizID: strconv.FormatInt(req.ID, 10), ActorID: actorUserID,
		Payload:   closedPayload,
		Timestamp: time.Now(),
	})
	return nil
}

// Assign 分派工单（2a：仅 admin bypass；2b 扩展主管 scope）
func (s *Service) Assign(ctx context.Context, req *model.AssignTicketRequest, actorUserID int64) error {
	return s.assignWithGuard(ctx, req, actorUserID, false, true)
}

// validateAssignTargetUser 分派目标用户校验（2026-10-29 幽灵分派修复）：
// 存在且启用（软删由 WHERE 排除；status=1 启用）。违规用 30002/30003 码+
// 定制 message（码=语义档位，message 带目标 ID 供排障——同 ErrInvalidParams
// 定制先例）。EscalationSupervisor 亦为用户 ID，同规则。
func (s *Service) validateAssignTargetUser(ctx context.Context, userID int64) error {
	var status int
	err := repository.IAMDB(ctx, s.db).QueryRow(ctx,
		`SELECT status FROM users WHERE id = $1 AND deleted_at IS NULL`, userID).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return errcode.New(errcode.ErrUserNotFound.Code,
			fmt.Sprintf("分派目标用户不存在: %d", userID))
	}
	if err != nil {
		return fmt.Errorf("query assign target user: %w", err)
	}
	if status != 1 {
		return errcode.New(errcode.ErrUserDisabled.Code,
			fmt.Sprintf("分派目标用户已禁用: %d", userID))
	}
	return nil
}

// assignAsSystem 系统身份分派，仅供同包已获权规则或后置动作调用。
// ⚠ 权限旁路：跳过 Assign 的 L2/L3 鉴权——调用方须已有 ticket:type:manage 语义授权
// （鉴权已在规则创建期行使：谁能配置自动分派）。修前用 rule.CreatedBy 走 Assign，
// 规则创建者非该单可操作人（非 admin 的管理员建的规则、跨组织）即 403→仅 Warn+
// dispatch_missed，自动分派在真实角色分布下静默失效。
// 状态机/条件校验/事务/事件全链与 Assign 一致；auditActor 记入事件与 ticket_events 供归因
// （=规则创建者，与 post_actions「系统身份重放」同口径——G-32 委托放大族）。
func (s *Service) assignAsSystem(ctx context.Context, req *model.AssignTicketRequest, auditActor int64) error {
	return s.assignAuthorized(ctx, req, auditActor, false)
}

// assignAuthorized applies the admitted assignment. Initial dispatch is
// restricted to an open, unassigned snapshot fenced by LockTransitionSnapshotTx.
func (s *Service) assignAuthorized(ctx context.Context, req *model.AssignTicketRequest, actorUserID int64, initialOnly bool) error {
	return s.assignWithGuard(ctx, req, actorUserID, initialOnly, false)
}

func (s *Service) assignWithGuard(ctx context.Context, req *model.AssignTicketRequest, actorUserID int64, initialOnly, manual bool) error {
	var ticket *model.Ticket
	var newStatus, fromAssignee, toAssignee string
	run := func(ctx context.Context, owner pgx.Tx) error {
		if manual {
			if err := s.authorizeCheck(ctx, actorUserID, "assign", strconv.FormatInt(req.ID, 10)); err != nil {
				return err
			}
		}
		if !manual {
			if err := s.lockCommandTickets(ctx, owner, req.ID); err != nil {
				return err
			}
			if err := s.rejectRiskLifecycleCommandTx(ctx, owner, "assign", req.ID); err != nil {
				return err
			}
		}
		var err error
		ticket, err = s.ticketRepo.GetByID(ctx, req.ID)
		if err != nil {
			return err
		}
		if initialOnly && (ticket.Status != StatusOpen || ticket.AssignedTo != nil) {
			return nil
		}
		ttype, _, _, err := s.resolveTicketDefinition(ctx, ticket)
		if err != nil {
			return err
		}
		// closed 工单不可分派（与 Update 的 90004 对齐；消除 read-then-assign 窗口）
		if ticket.Status == StatusClosed || ticket.Status == StatusCancelled {
			return errcode.ErrTicketAlreadyClosed
		}
		// 幽灵分派校验（2026-10-29）：目标用户/催办上级须存在且启用——原实现直写
		// assigned_to 无校验，幽灵 assignee 可挂单（详情页处理人残缺/通知无法送达，
		// 所有者测试复现）。取消分派（AssignedTo nil）不校验。自动分派同链受控
		// （目标来自成员解析，成员被禁用后规则触发→响亮 400+dispatch_missed，
		// 优于原幽灵挂单静默成功）。
		if req.AssignedTo != nil {
			if err := s.validateAssignTargetUser(ctx, *req.AssignedTo); err != nil {
				return err
			}
		}
		if req.EscalationSupervisor != nil {
			if err := s.validateAssignTargetUser(ctx, *req.EscalationSupervisor); err != nil {
				return err
			}
		}
		fromAssignee = ""
		if ticket.AssignedTo != nil {
			fromAssignee = strconv.FormatInt(*ticket.AssignedTo, 10)
		}
		toAssignee = ""
		if req.AssignedTo != nil {
			toAssignee = strconv.FormatInt(*req.AssignedTo, 10)
		}

		// 分派 → assigned 状态（若当前为 open）；取消分派 → open（若当前为 assigned）
		newStatus = ticket.Status
		if ticket.Status == StatusOpen && req.AssignedTo != nil {
			newStatus = StatusAssigned
		}
		if ticket.Status == StatusAssigned && req.AssignedTo == nil {
			newStatus = StatusOpen
		}

		// BK-2：状态转换按工单绑定的状态机校验（同 Close）。
		if newStatus != ticket.Status {
			sm, err := FromTicketType(ttype)
			if err != nil {
				return fmt.Errorf("build state machine: %w", err)
			}
			if err := sm.AssertTransition(ticket.Status, newStatus); err != nil {
				return err
			}
			// R3 审计修复：Assign 状态转换时也走 condition+validator（同 Close）
			if _, err := s.checkTransitionMeta(ctx, ticket, newStatus, actorUserID, ttype); err != nil {
				return err
			}
		}

		tx := owner
		ownTx := tx == nil
		if ownTx {
			tx, err = s.db.Begin(ctx)
			if err != nil {
				return fmt.Errorf("begin tx: %w", err)
			}
			defer tx.Rollback(ctx)
		}

		if err := s.ticketRepo.LockTransitionSnapshotTx(ctx, tx, ticket, nil); err != nil {
			return err
		}
		if err := s.rejectRiskLifecycleCommandTx(ctx, tx, "assign", ticket.ID); err != nil {
			return err
		}
		if err := s.ticketRepo.UpdateAssignedToTx(ctx, tx, req.ID, req.AssignedTo); err != nil {
			return err
		}
		// B10-4：催办上级人工指定随分派事务更新（nil=保持不动；dd §31 基线档）
		if req.EscalationSupervisor != nil {
			if err := s.ticketRepo.UpdateEscalationSupervisorTx(ctx, tx, req.ID, *req.EscalationSupervisor); err != nil {
				return err
			}
		}
		if newStatus != ticket.Status {
			if err := s.ticketRepo.UpdateStatusTx(ctx, tx, req.ID, ticket.Status, newStatus); err != nil {
				return err
			}
			if err := s.applyStateSLARuleTx(ctx, tx, ttype, ticket, newStatus); err != nil {
				return err
			}
		}
		if err := s.ticketRepo.CreateEventTx(ctx, tx, &model.TicketEvent{
			TicketID:  req.ID,
			UserID:    actorUserID,
			Action:    EventAssigned,
			FromValue: fromAssignee,
			ToValue:   toAssignee,
		}); err != nil {
			return err
		}
		if ownTx {
			if err := tx.Commit(ctx); err != nil {
				return err
			}
		}
		return nil
	}
	var err error
	if manual {
		err = s.withHTTPCommand(ctx, "assign", actorUserID, run, req.ID)
	} else {
		// System dispatch has its own reviewed identity; it does not consume
		// the HTTP route or actor that triggered a post action.
		err = repository.WithIAMGuard(ctx, s.db, func(ctx context.Context) error {
			tx := repository.IAMDB(ctx, s.db).(pgx.Tx)
			return run(ctx, tx)
		})
	}
	if err != nil {
		return err
	}
	if ticket == nil || initialOnly && (ticket.Status != StatusOpen || ticket.AssignedTo != nil) {
		return nil
	}
	// D1 审计修复：状态变更即离开旧状态——清旧状态催办水位（与 Transition/Close
	// 同源三路；修复前 assigned 催满→取消分派回 open→重分派残留水位致永不催）
	if newStatus != ticket.Status {
		s.clearReminderWatermark(ctx, req.ID, ticket.Status)
	}
	// B6-1：工单分派事件（事务提交后派发）
	s.emitTicketEvent(ticket, domainevent.DomainEvent{
		BizType: domainevent.BizTicket, Action: domainevent.TicketAssigned,
		BizID: strconv.FormatInt(req.ID, 10), ActorID: actorUserID,
		Payload: domainevent.Payload{
			"from_assignee": fromAssignee, "to_assignee": toAssignee,
			"title": ticket.Title, "type_code": ticket.TypeCode,
			"priority": ticket.Priority, "status": newStatus,
		},
		Timestamp: time.Now(),
	})
	return nil
}

// Delete 删除工单（2a：仅 admin）
func (s *Service) Delete(ctx context.Context, id int64, actorUserID int64) error {
	return s.withHTTPCommand(ctx, "delete", actorUserID, func(ctx context.Context, tx pgx.Tx) error {
		if err := s.authorizeCheck(ctx, actorUserID, "delete", strconv.FormatInt(id, 10)); err != nil {
			return err
		}
		if err := s.requireTicketDefinition(ctx, id); err != nil {
			return err
		}
		if err := s.lockCommandTickets(ctx, tx, id); err != nil {
			return err
		}
		if err := s.rejectRiskLifecycleCommandTx(ctx, tx, "delete", id); err != nil {
			return err
		}
		return s.ticketRepo.DeleteTx(ctx, tx, id, actorUserID)
	}, id)
}

// --- 评论 / 备注 ---

// CreateComment 创建公开回复
func (s *Service) CreateComment(ctx context.Context, req *model.CreateCommentRequest, actorUserID int64) (*model.TicketComment, error) {
	var comment *model.TicketComment
	var ticket *model.Ticket
	err := s.withHTTPCommand(ctx, "comments", actorUserID, func(ctx context.Context, owner pgx.Tx) error {
		var err error
		if err := s.authorizeCheck(ctx, actorUserID, "comment", strconv.FormatInt(req.TicketID, 10)); err != nil {
			return err
		}
		comment = &model.TicketComment{
			TicketID:   req.TicketID,
			UserID:     actorUserID,
			Content:    req.Content,
			IsInternal: false,
		}
		// HC1（A4）：评论与事件同事务落库（事件流供 Phase 3 SLA/通知消费）
		tx := owner
		ownTx := tx == nil
		if ownTx {
			tx, err = s.db.Begin(ctx)
			if err != nil {
				return fmt.Errorf("begin tx: %w", err)
			}
			defer tx.Rollback(ctx)
		}
		// 公开回复与通知事实共用本写事务内的工单版本。先锁票，再写评论及审计，
		// 使并发改票/删票与本次提交有明确顺序；消费者不在提交后回查工单。
		ticket, err = s.ticketRepo.GetByIDForUpdateTx(ctx, tx, req.TicketID)
		if err != nil {
			return err
		}
		if _, _, _, err := s.resolveTicketDefinition(ctx, ticket); err != nil {
			return err
		}
		if err := s.ticketRepo.CreateCommentTx(ctx, tx, comment); err != nil {
			return err
		}
		// 公开回复随建随给可核证据版本（与 GET 评论同一规范形——internal/model/comment）
		comment.EvidenceVersion = commentref.EvidenceVersionToken(comment.CreatedAt, comment.Content)
		if err := s.ticketRepo.CreateEventTx(ctx, tx, &model.TicketEvent{
			TicketID: req.TicketID, UserID: actorUserID, Action: EventComment,
		}); err != nil {
			return err
		}
		if ownTx {
			if err := tx.Commit(ctx); err != nil {
				return fmt.Errorf("commit tx: %w", err)
			}
		}
		return nil
	}, req.TicketID)
	if err != nil {
		return nil, err
	}
	payload := domainevent.Payload{
		"title": ticket.Title, "type_code": ticket.TypeCode,
		"human_key": ticket.HumanKey, "status": ticket.Status,
	}
	if ticket.AssignedTo != nil {
		payload["to_assignee"] = strconv.FormatInt(*ticket.AssignedTo, 10)
	}
	// Public reply only: internal notes have a narrower visibility boundary and
	// do not enter this notification path. Never publish the comment body here.
	s.emitTicketEvent(ticket, domainevent.DomainEvent{
		BizType: domainevent.BizTicket, Action: domainevent.TicketCommented,
		BizID: strconv.FormatInt(ticket.ID, 10), ActorID: actorUserID,
		Payload: payload, Timestamp: time.Now(),
	})
	return comment, nil
}

// CreateNote 创建内部备注（2a：创建人或处理人可写）
func (s *Service) CreateNote(ctx context.Context, req *model.CreateNoteRequest, actorUserID int64) (*model.TicketComment, error) {
	var result *model.TicketComment
	err := s.withHTTPCommand(ctx, "notes", actorUserID, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		result, err = s.createNoteCommand(ctx, tx, req, actorUserID)
		return err
	}, req.TicketID)
	return result, err
}

func (s *Service) createNoteCommand(ctx context.Context, tx pgx.Tx, req *model.CreateNoteRequest, actorUserID int64) (*model.TicketComment, error) {
	if err := s.authorizeCheck(ctx, actorUserID, "note", strconv.FormatInt(req.TicketID, 10)); err != nil {
		return nil, err
	}
	comment := &model.TicketComment{
		TicketID:   req.TicketID,
		UserID:     actorUserID,
		Content:    req.Content,
		IsInternal: true,
	}
	// HC1（A4）：内部备注与事件同事务（读写集合一致：能写 note 者必能见其事件流）
	ownTx := tx == nil
	if ownTx {
		var err error
		tx, err = s.db.Begin(ctx)
		if err != nil {
			return nil, fmt.Errorf("begin tx: %w", err)
		}
		defer tx.Rollback(ctx)
	}
	if err := s.requireTicketDefinition(ctx, req.TicketID); err != nil {
		return nil, err
	}
	if err := s.ticketRepo.CreateCommentTx(ctx, tx, comment); err != nil {
		return nil, err
	}
	if err := s.ticketRepo.CreateEventTx(ctx, tx, &model.TicketEvent{
		TicketID: req.TicketID, UserID: actorUserID, Action: EventNote,
	}); err != nil {
		return nil, err
	}
	if ownTx {
		if err := tx.Commit(ctx); err != nil {
			return nil, fmt.Errorf("commit tx: %w", err)
		}
	}
	return comment, nil
}

// ListComments 查询工单回复列表
func (s *Service) ListComments(ctx context.Context, ticketID, actorUserID int64) ([]*model.TicketComment, error) {
	if err := s.authorizeCheck(ctx, actorUserID, "read", strconv.FormatInt(ticketID, 10)); err != nil {
		return nil, err
	}
	// BK-1（2b Step 4 门禁）：内部备注仅 创建人/处理人/admin 可见，
	// 透明读旁观者只返回公开回复。note 动作的放行集合恰为该集合，直接复用判定：
	// errDenied（可见但非内部可见集合）→ 降级为仅公开；DB 错误上抛（Q3）。
	ok, err := s.authorize(ctx, actorUserID, "note", strconv.FormatInt(ticketID, 10))
	if err != nil {
		if errors.Is(err, errDenied) {
			return s.ticketRepo.ListComments(ctx, ticketID, false)
		}
		return nil, err
	}
	return s.ticketRepo.ListComments(ctx, ticketID, ok)
}

// --- 工单关联 ---

// CreateRelation 建立工单关联（对 target 走 L2/L3 鉴权）
func (s *Service) CreateRelation(ctx context.Context, req *model.CreateRelationRequest, actorUserID int64) (*model.TicketRelation, error) {
	if req.SourceTicketID == req.TargetTicketID {
		return nil, errcode.ErrInvalidParams
	}
	var result *model.TicketRelation
	err := s.withHTTPCommand(ctx, "relations", actorUserID, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		result, err = s.createRelationCommand(ctx, tx, req, actorUserID)
		return err
	})
	return result, err
}

func (s *Service) createRelationCommand(ctx context.Context, tx pgx.Tx, req *model.CreateRelationRequest, actorUserID int64) (*model.TicketRelation, error) {
	// W0b（二十四批侧信道）：鉴权先行——不可见工单不得经查重 409 泄露存在性，
	// 须先走 authorizeCheck 返回 404+90001（防枚举语义与 Get/Update 同型）。
	// 对 source 和 target 都做 update 鉴权（建立关联视为修改操作，需 update 权限）
	for _, idStr := range []string{strconv.FormatInt(req.SourceTicketID, 10), strconv.FormatInt(req.TargetTicketID, 10)} {
		if err := s.authorizeCheck(ctx, actorUserID, "update", idStr); err != nil {
			return nil, err
		}
	}
	// Authorize in source/target order before the stable two-row lock.
	// A source rejection must not depend on whether the target exists.
	if err := s.lockCommandTickets(ctx, tx, req.SourceTicketID, req.TargetTicketID); err != nil {
		return nil, err
	}
	for _, id := range []int64{req.SourceTicketID, req.TargetTicketID} {
		if err := s.requireTicketDefinition(ctx, id); err != nil {
			return nil, err
		}
	}
	// BK-5（A5）：反向判重——A→B 与 B→A 视为同一关联（DB 唯一索引仅防同向）；
	// 预检后移为友好路径，并发/顺序重复由 uq_ticket_relations_normalized
	//（000028）兜底 → repo 层 23505 映射 ErrConflict（409），语义不变
	relType := req.RelationType
	if relType == "" {
		relType = "related"
	}
	dup, err := s.ticketRepo.ExistsRelationBetweenTx(ctx, tx, req.SourceTicketID, req.TargetTicketID, relType)
	if err != nil {
		return nil, err
	}
	if dup {
		return nil, errcode.ErrConflict
	}
	rel := &model.TicketRelation{
		SourceTicketID: req.SourceTicketID,
		TargetTicketID: req.TargetTicketID,
		RelationType:   relType, // 回显规范化值（2026-10-14 契约审计 B2：空入参原样回显致创建响应 "" 与列表 "related" 漂移）
		CreatedBy:      actorUserID,
	}
	if err := s.ticketRepo.CreateRelationTx(ctx, tx, rel); err != nil {
		return nil, err
	}
	return rel, nil
}

// ListRelations 查询工单关联列表
func (s *Service) ListRelations(ctx context.Context, ticketID, actorUserID int64) ([]*model.TicketRelation, error) {
	if err := s.authorizeCheck(ctx, actorUserID, "read", strconv.FormatInt(ticketID, 10)); err != nil {
		return nil, err
	}
	filter, err := s.resolveListFilter(ctx, actorUserID)
	if err != nil {
		return nil, err
	}
	return s.ticketRepo.ListRelations(ctx, ticketID, filter)
}

// --- 元数据 ---

// ListTicketTypes 工单类型列表
func (s *Service) ListTicketTypes(ctx context.Context) ([]*model.TicketType, error) {
	return s.ticketRepo.ListTicketTypes(ctx)
}

// ListTicketTypeFields 工单类型字段定义
func (s *Service) ListTicketTypeFields(ctx context.Context, typeCode string) ([]*model.TicketTypeField, error) {
	return s.ticketRepo.ListTicketTypeFields(ctx, typeCode)
}

// ListTicketTemplates 工单模板列表
func (s *Service) ListTicketTemplates(ctx context.Context) ([]*model.TicketTemplate, error) {
	return s.ticketRepo.ListTicketTemplates(ctx)
}

// GetTicketTemplate 按 code 查询工单模板
func (s *Service) GetTicketTemplate(ctx context.Context, code string) (*model.TicketTemplate, error) {
	return s.ticketRepo.GetTicketTemplate(ctx, code)
}

// AuthorizeTicketAttachment L2 面（B5-3）：storage 端点对 ticket 态 purpose
// 的工单可见性判定（action=update——附件上传/确认/删除同面）。供 handler
// 编排层结构性注入（TicketAuthorizer 接口），域间零 import。
func (s *Service) AuthorizeTicketAttachment(ctx context.Context, userID, ticketID int64) error {
	return s.authorizeCheck(ctx, userID, "update", strconv.FormatInt(ticketID, 10))
}

// AuthorizeTicketFileRead 附件下载/列表的 L2 读面（B5-3 批3——A.7：下载=工单
// L2 可见性 ticket:read 管线）。与 update 面同款结构性注入。
func (s *Service) AuthorizeTicketFileRead(ctx context.Context, userID, ticketID int64) error {
	return s.authorizeCheck(ctx, userID, "read", strconv.FormatInt(ticketID, 10))
}

// buildChanges records requested fields with their values from the committed
// projection. Custom data changes carry only the field name.
func buildChanges(req *model.UpdateTicketRequest, ticket *model.Ticket) []domainevent.Change {
	var changes []domainevent.Change
	if req.Title != nil {
		changes = append(changes, domainevent.Change{Key: "title", NewVal: ticket.Title})
	}
	if req.Priority != nil {
		changes = append(changes, domainevent.Change{Key: "priority", NewVal: ticket.Priority})
	}
	if req.Description != nil {
		changes = append(changes, domainevent.Change{Key: "description", NewVal: ticket.Description})
	}
	if req.CustomData != nil {
		changes = append(changes, domainevent.Change{Key: "custom_data"})
	}
	return changes
}
