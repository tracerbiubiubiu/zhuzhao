package ticket

import (
	"context"
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
) *Service {
	scope := NewStubScopeResolver()
	s := &Service{
		db:          db,
		ticketRepo:  ticketRepo,
		orgRepo:     orgRepo,
		registry:    registry,
		roleFetcher: roleFetcher,
	}
	// 自注册 TicketResource（§2.5 Wire 自注册）
	registry.Register(NewResource(s.ticketRepo, scope))
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

// --- CRUD ---

// Create 创建工单
func (s *Service) Create(ctx context.Context, req *model.CreateTicketRequest, actorUserID int64) (*model.Ticket, error) {
	// 1. 校验 type_code
	ttype, err := s.ticketRepo.GetTicketType(ctx, req.TypeCode)
	if err != nil {
		return nil, err
	}
	_ = ttype // Phase 3 可用 ttype 做字段校验

	// 2. 校验 org_id 存在 + 读 org.path
	org, err := s.orgRepo.FindByID(ctx, req.OrgID)
	if err != nil {
		return nil, err
	}

	// 3. 模板预填（可选）
	priority := req.Priority
	if priority == 0 {
		priority = 3
	}
	if req.TemplateCode != "" {
		tmpl, err := s.ticketRepo.GetTicketTemplate(ctx, req.TemplateCode)
		if err == nil && tmpl != nil {
			if tmpl.DefaultPriority > 0 {
				priority = tmpl.DefaultPriority
			}
		}
		// 模板不存在不报错（可选参数）
	}

	// 4. 事务内创建工单 + 写事件
	ticket := &model.Ticket{
		TypeCode:    req.TypeCode,
		Title:       req.Title,
		Description: req.Description,
		Priority:    priority,
		Status:      "open",
		CreatedBy:   actorUserID,
		AssignedTo:  req.AssignedTo,
		OrgID:       req.OrgID,
		OrgPath:     org.Path,
		CustomData:  req.CustomData,
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
	ok, err := s.authorize(ctx, actorUserID, "read", strconv.FormatInt(id, 10))
	if err != nil {
		return nil, errcode.ErrTicketNotFound
	}
	if !ok {
		return nil, errcode.ErrTicketNotFound // 不可见 → 404
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
	idStr := strconv.FormatInt(req.ID, 10)
	ok, err := s.authorize(ctx, actorUserID, "update", idStr)
	if err != nil {
		return nil, errcode.ErrTicketNotFound
	}
	if !ok {
		return nil, errcode.ErrNoPermission
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
	if err := s.ticketRepo.Update(ctx, ticket); err != nil {
		return nil, err
	}
	return ticket, nil
}

// Close 关闭工单（状态机校验 + 事件）
func (s *Service) Close(ctx context.Context, req *model.CloseTicketRequest, actorUserID int64) error {
	idStr := strconv.FormatInt(req.ID, 10)
	ok, err := s.authorize(ctx, actorUserID, "close", idStr)
	if err != nil {
		return errcode.ErrTicketNotFound
	}
	if !ok {
		return errcode.ErrNoPermission
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

	if err := s.ticketRepo.UpdateStatusTx(ctx, tx, req.ID, "closed"); err != nil {
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
	idStr := strconv.FormatInt(req.ID, 10)
	ok, err := s.authorize(ctx, actorUserID, "assign", idStr)
	if err != nil {
		return errcode.ErrTicketNotFound
	}
	if !ok {
		return errcode.ErrNoPermission
	}

	ticket, err := s.ticketRepo.GetByID(ctx, req.ID)
	if err != nil {
		return err
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

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	if err := s.ticketRepo.UpdateAssignedToTx(ctx, tx, req.ID, req.AssignedTo); err != nil {
		return err
	}
	if newStatus != ticket.Status {
		if err := s.ticketRepo.UpdateStatusTx(ctx, tx, req.ID, newStatus); err != nil {
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
	idStr := strconv.FormatInt(id, 10)
	ok, err := s.authorize(ctx, actorUserID, "delete", idStr)
	if err != nil {
		return errcode.ErrTicketNotFound
	}
	if !ok {
		return errcode.ErrNoPermission
	}
	return s.ticketRepo.Delete(ctx, id)
}

// --- 评论 / 备注 ---

// CreateComment 创建公开回复
func (s *Service) CreateComment(ctx context.Context, req *model.CreateCommentRequest, actorUserID int64) (*model.TicketComment, error) {
	idStr := strconv.FormatInt(req.TicketID, 10)
	ok, err := s.authorize(ctx, actorUserID, "comment", idStr)
	if err != nil {
		return nil, errcode.ErrTicketNotFound
	}
	if !ok {
		return nil, errcode.ErrTicketNotFound
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
	idStr := strconv.FormatInt(req.TicketID, 10)
	ok, err := s.authorize(ctx, actorUserID, "note", idStr)
	if err != nil {
		return nil, errcode.ErrTicketNotFound
	}
	if !ok {
		return nil, errcode.ErrTicketNotFound
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
	idStr := strconv.FormatInt(ticketID, 10)
	ok, err := s.authorize(ctx, actorUserID, "read", idStr)
	if err != nil {
		return nil, errcode.ErrTicketNotFound
	}
	if !ok {
		return nil, errcode.ErrTicketNotFound
	}
	return s.ticketRepo.ListComments(ctx, ticketID)
}

// --- 工单关联 ---

// CreateRelation 建立工单关联（对 target 走 L2/L3 鉴权）
func (s *Service) CreateRelation(ctx context.Context, req *model.CreateRelationRequest, actorUserID int64) (*model.TicketRelation, error) {
	// 自关联校验（DB 有 CHECK 约束，提前拦截返回 400 而非 409）
	if req.SourceTicketID == req.TargetTicketID {
		return nil, errcode.ErrInvalidParams
	}
	// 对 source 和 target 都做 update 鉴权（建立关联视为修改操作，需 update 权限）
	sourceStr := strconv.FormatInt(req.SourceTicketID, 10)
	targetStr := strconv.FormatInt(req.TargetTicketID, 10)
	for _, idStr := range []string{sourceStr, targetStr} {
		ok, err := s.authorize(ctx, actorUserID, "update", idStr)
		if err != nil {
			return nil, errcode.ErrTicketNotFound
		}
		if !ok {
			return nil, errcode.ErrTicketNotFound
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
	idStr := strconv.FormatInt(ticketID, 10)
	ok, err := s.authorize(ctx, actorUserID, "read", idStr)
	if err != nil {
		return nil, errcode.ErrTicketNotFound
	}
	if !ok {
		return nil, errcode.ErrTicketNotFound
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
