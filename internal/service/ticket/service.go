package ticket

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tracerbiubiubiu/zhuzhao/internal/middleware"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/resource"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

// Service 工单服务（Phase 2a MVP）
type Service struct {
	db          *pgxpool.Pool
	ticketRepo  *repository.TicketRepo
	orgRepo     *repository.OrgRepo
	registry    resource.Registry
	roleFetcher middleware.RoleFetcher
}

// NewTicketService 创建工单服务并自注册 TicketResource 到 Registry。
// Wire 语义保证：Registry 单例，TicketService 构造（自注册）先于 router.New。
func NewTicketService(
	db *pgxpool.Pool,
	ticketRepo *repository.TicketRepo,
	orgRepo *repository.OrgRepo,
	registry resource.Registry,
	roleFetcher middleware.RoleFetcher,
	delegation OrgDelegationChecker,
) *Service {
	s := &Service{
		db:          db,
		ticketRepo:  ticketRepo,
		orgRepo:     orgRepo,
		registry:    registry,
		roleFetcher: roleFetcher,
	}
	// 自注册 TicketResource（§2.5 Wire 自注册）；2b 策略 B 透明读 + 2c 组织委托
	registry.Register(NewResource(s.ticketRepo, NewPgxScopeResolver(db), delegation))
	return s
}

// getRoles 获取用户角色码列表
func (s *Service) getRoles(ctx context.Context, userID int64) ([]string, error) {
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
	// 1. 校验 type_code（停用类型对客户端视同不存在，统一 90003）
	ttype, err := s.ticketRepo.GetTicketType(ctx, req.TypeCode)
	if err != nil {
		return nil, err
	}
	if !ttype.IsActive {
		return nil, errcode.ErrTicketTypeNotFound
	}

	// 2. 校验 org_id 存在 + 读 org.path
	org, err := s.orgRepo.FindByID(ctx, req.OrgID)
	if err != nil {
		return nil, err
	}

	// 3. 模板预填（可选，09-ticket §2：default_fields 预填空缺字段、default_priority 作缺省）。
	// 取舍：请求显式值 > 模板默认 > 类型缺省 3；模板只填请求未提供的字段，不覆盖显式输入。
	// title 不预填——请求 binding:"required" 保证非空，模板 title 永远不会被用到。
	priority := req.Priority
	if priority == 0 {
		priority = defaultTicketPriority
	}
	description := req.Description
	customData := req.CustomData
	if req.TemplateCode != "" {
		tmpl, err := s.ticketRepo.GetTicketTemplate(ctx, req.TemplateCode)
		switch {
		case errors.Is(err, errcode.ErrNotFound):
			// 模板不存在：可选参数，静默跳过（「命中则预填」语义）
		case err != nil:
			return nil, fmt.Errorf("get ticket template: %w", err) // DB 错误不得吞掉
		default:
			if tmpl.TypeCode != req.TypeCode {
				return nil, errcode.ErrInvalidParams // 模板属于其它工单类型
			}
			var defaults struct {
				Description *string          `json:"description,omitempty"`
				CustomData  *json.RawMessage `json:"custom_data,omitempty"`
			}
			if len(tmpl.DefaultFields) > 0 {
				if err := json.Unmarshal(tmpl.DefaultFields, &defaults); err != nil {
					return nil, fmt.Errorf("parse template default_fields: %w", err)
				}
			}
			if description == "" && defaults.Description != nil {
				description = *defaults.Description
			}
			if len(customData) == 0 && defaults.CustomData != nil {
				customData = *defaults.CustomData
			}
			if req.Priority == 0 && tmpl.DefaultPriority > 0 {
				priority = tmpl.DefaultPriority
			}
		}
	}

	// 4. 事务内创建工单 + 写事件
	ticket := &model.Ticket{
		TypeCode:    req.TypeCode,
		Title:       req.Title,
		Description: description,
		Priority:    priority,
		Status:      "open",
		CreatedBy:   actorUserID,
		AssignedTo:  req.AssignedTo,
		OrgID:       req.OrgID,
		OrgPath:     org.Path,
		CustomData:  customData,
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	if err := s.ticketRepo.CreateTx(ctx, tx, ticket); err != nil {
		return nil, err
	}
	if err := s.ticketRepo.CreateEventTx(ctx, tx, &model.TicketEvent{
		TicketID: ticket.ID,
		UserID:   actorUserID,
		Action:   "created",
	}); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit tx: %w", err)
	}
	return ticket, nil
}

// Get 获取工单详情（含 L2 可见性校验）
func (s *Service) Get(ctx context.Context, id int64, actorUserID int64) (*model.Ticket, error) {
	if err := s.authorizeCheck(ctx, actorUserID, "read", strconv.FormatInt(id, 10)); err != nil {
		return nil, err
	}
	return s.ticketRepo.GetByID(ctx, id)
}

// List 工单列表（L2 行级过滤；admin bypass L2）
func (s *Service) List(ctx context.Context, q model.TicketListQuery, actorUserID int64) (*model.TicketListResponse, error) {
	var filter resource.Filter
	roles, err := s.getRoles(ctx, actorUserID)
	if err != nil {
		return nil, fmt.Errorf("fetch roles: %w", err)
	}
	if HasRole(roles, "admin") || HasRole(roles, "superadmin") {
		// admin bypass L2：空 Filter = 无过滤条件
		filter = resource.Filter{}
	} else {
		filter, err = s.registry.GetFilter(ctx, "ticket", actorUserID, "read")
		if err != nil {
			return nil, err
		}
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

// Update 更新工单（patch 语义）
func (s *Service) Update(ctx context.Context, req *model.UpdateTicketRequest, actorUserID int64) (*model.Ticket, error) {
	if err := s.authorizeCheck(ctx, actorUserID, "update", strconv.FormatInt(req.ID, 10)); err != nil {
		return nil, err
	}

	ticket, err := s.ticketRepo.GetByID(ctx, req.ID)
	if err != nil {
		return nil, err
	}
	if ticket.Status == "closed" {
		return nil, errcode.ErrTicketAlreadyClosed
	}
	// patch 语义
	if req.Title != nil {
		ticket.Title = *req.Title
	}
	if req.Description != nil {
		ticket.Description = *req.Description
	}
	if req.Priority != nil {
		ticket.Priority = *req.Priority
	}
	// BK-3：条件更新（WHERE status<>'closed'）+ 同事务事件留痕——
	// 消除「读后写」TOCTOU（并发 close 后命中 0 行 → 90004），补齐 patch 审计断档
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := s.ticketRepo.UpdateTx(ctx, tx, ticket); err != nil {
		return nil, err
	}
	if err := s.ticketRepo.CreateEventTx(ctx, tx, &model.TicketEvent{
		TicketID: ticket.ID,
		UserID:   actorUserID,
		Action:   "updated",
	}); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit tx: %w", err)
	}
	return ticket, nil
}

// Close 关闭工单（状态机校验 + 事件）
func (s *Service) Close(ctx context.Context, req *model.CloseTicketRequest, actorUserID int64) error {
	if err := s.authorizeCheck(ctx, actorUserID, "close", strconv.FormatInt(req.ID, 10)); err != nil {
		return err
	}

	ticket, err := s.ticketRepo.GetByID(ctx, req.ID)
	if err != nil {
		return err
	}
	if ticket.Status == "closed" {
		return errcode.ErrTicketAlreadyClosed
	}

	// 状态机校验
	ttype, err := s.ticketRepo.GetTicketType(ctx, ticket.TypeCode)
	if err != nil {
		return err
	}
	sm, err := FromTicketType(ttype)
	if err != nil {
		return fmt.Errorf("build state machine: %w", err)
	}
	if err := sm.AssertTransition(ticket.Status, "closed"); err != nil {
		return err
	}

	// 更新状态 + 写事件 + 可选关闭说明
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	if err := s.ticketRepo.UpdateStatusTx(ctx, tx, req.ID, ticket.Status, "closed"); err != nil {
		return err
	}
	if err := s.ticketRepo.CreateEventTx(ctx, tx, &model.TicketEvent{
		TicketID:  req.ID,
		UserID:    actorUserID,
		Action:    "status_changed",
		FromValue: ticket.Status,
		ToValue:   "closed",
	}); err != nil {
		return err
	}
	// 可选关闭说明 → 写评论
	if req.Comment != "" {
		if err := s.ticketRepo.CreateCommentTx(ctx, tx, &model.TicketComment{
			TicketID:   req.ID,
			UserID:     actorUserID,
			Content:    req.Comment,
			IsInternal: false,
		}); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// Assign 分派工单（2a：仅 admin bypass；2b 扩展主管 scope）
func (s *Service) Assign(ctx context.Context, req *model.AssignTicketRequest, actorUserID int64) error {
	if err := s.authorizeCheck(ctx, actorUserID, "assign", strconv.FormatInt(req.ID, 10)); err != nil {
		return err
	}

	ticket, err := s.ticketRepo.GetByID(ctx, req.ID)
	if err != nil {
		return err
	}
	// closed 工单不可分派（与 Update 的 90004 对齐；消除 read-then-assign 窗口）
	if ticket.Status == "closed" {
		return errcode.ErrTicketAlreadyClosed
	}
	fromAssignee := ""
	if ticket.AssignedTo != nil {
		fromAssignee = strconv.FormatInt(*ticket.AssignedTo, 10)
	}
	toAssignee := ""
	if req.AssignedTo != nil {
		toAssignee = strconv.FormatInt(*req.AssignedTo, 10)
	}

	// 分派 → assigned 状态（若当前为 open）；取消分派 → open（若当前为 assigned）
	newStatus := ticket.Status
	if ticket.Status == "open" && req.AssignedTo != nil {
		newStatus = "assigned"
	}
	if ticket.Status == "assigned" && req.AssignedTo == nil {
		newStatus = "open"
	}

	// BK-2：状态转换走状态机校验——transitions 是配置即代码（ticket_types.transitions），
	// 手工推算会在类型配置变更后静默写出非法状态（Close 已同款校验）
	if newStatus != ticket.Status {
		ttype, err := s.ticketRepo.GetTicketType(ctx, ticket.TypeCode)
		if err != nil {
			return err
		}
		sm, err := FromTicketType(ttype)
		if err != nil {
			return fmt.Errorf("build state machine: %w", err)
		}
		if err := sm.AssertTransition(ticket.Status, newStatus); err != nil {
			return err
		}
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	if err := s.ticketRepo.UpdateAssignedToTx(ctx, tx, req.ID, req.AssignedTo); err != nil {
		return err
	}
	if newStatus != ticket.Status {
		if err := s.ticketRepo.UpdateStatusTx(ctx, tx, req.ID, ticket.Status, newStatus); err != nil {
			return err
		}
	}
	if err := s.ticketRepo.CreateEventTx(ctx, tx, &model.TicketEvent{
		TicketID:  req.ID,
		UserID:    actorUserID,
		Action:    "assigned",
		FromValue: fromAssignee,
		ToValue:   toAssignee,
	}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// Delete 删除工单（2a：仅 admin）
func (s *Service) Delete(ctx context.Context, id int64, actorUserID int64) error {
	if err := s.authorizeCheck(ctx, actorUserID, "delete", strconv.FormatInt(id, 10)); err != nil {
		return err
	}
	return s.ticketRepo.Delete(ctx, id, actorUserID)
}

// --- 评论 / 备注 ---

// CreateComment 创建公开回复
func (s *Service) CreateComment(ctx context.Context, req *model.CreateCommentRequest, actorUserID int64) (*model.TicketComment, error) {
	if err := s.authorizeCheck(ctx, actorUserID, "comment", strconv.FormatInt(req.TicketID, 10)); err != nil {
		return nil, err
	}
	comment := &model.TicketComment{
		TicketID:   req.TicketID,
		UserID:     actorUserID,
		Content:    req.Content,
		IsInternal: false,
	}
	if err := s.ticketRepo.CreateComment(ctx, comment); err != nil {
		return nil, err
	}
	return comment, nil
}

// CreateNote 创建内部备注（2a：创建人或处理人可写）
func (s *Service) CreateNote(ctx context.Context, req *model.CreateNoteRequest, actorUserID int64) (*model.TicketComment, error) {
	if err := s.authorizeCheck(ctx, actorUserID, "note", strconv.FormatInt(req.TicketID, 10)); err != nil {
		return nil, err
	}
	comment := &model.TicketComment{
		TicketID:   req.TicketID,
		UserID:     actorUserID,
		Content:    req.Content,
		IsInternal: true,
	}
	if err := s.ticketRepo.CreateComment(ctx, comment); err != nil {
		return nil, err
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
	// 自关联校验（DB 有 CHECK 约束，提前拦截返回 400 而非 409）
	if req.SourceTicketID == req.TargetTicketID {
		return nil, errcode.ErrInvalidParams
	}
	// 对 source 和 target 都做 update 鉴权（建立关联视为修改操作，需 update 权限）
	for _, idStr := range []string{strconv.FormatInt(req.SourceTicketID, 10), strconv.FormatInt(req.TargetTicketID, 10)} {
		if err := s.authorizeCheck(ctx, actorUserID, "update", idStr); err != nil {
			return nil, err
		}
	}
	rel := &model.TicketRelation{
		SourceTicketID: req.SourceTicketID,
		TargetTicketID: req.TargetTicketID,
		RelationType:   req.RelationType,
		CreatedBy:      actorUserID,
	}
	if err := s.ticketRepo.CreateRelation(ctx, rel); err != nil {
		return nil, err
	}
	return rel, nil
}

// ListRelations 查询工单关联列表
func (s *Service) ListRelations(ctx context.Context, ticketID, actorUserID int64) ([]*model.TicketRelation, error) {
	if err := s.authorizeCheck(ctx, actorUserID, "read", strconv.FormatInt(ticketID, 10)); err != nil {
		return nil, err
	}
	return s.ticketRepo.ListRelations(ctx, ticketID)
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
