package service

import (
	"log/slog"

	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/tracerbiubiubiu/zhuzhao/internal/middleware"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao-utils/validate"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

// OrgService 组织架构管理服务
type OrgService struct {
	orgRepo    *repository.OrgRepo
	userRepo   *repository.UserRepo
	delegation *OrgDelegationService
	roles      middleware.RoleFetcher
}

// NewOrgService 创建组织服务。
// 2c：注入委托服务（组内级别校验）与角色获取器（全局 admin 判定）。
func NewOrgService(orgRepo *repository.OrgRepo, userRepo *repository.UserRepo,
	delegation *OrgDelegationService, roles middleware.RoleFetcher) *OrgService {
	return &OrgService{orgRepo: orgRepo, userRepo: userRepo, delegation: delegation, roles: roles}
}

// isGlobalOrgAdmin 全局组织管理判定（04 §3.1 L3 全局侧）：
// admin/superadmin 角色码快路径 + org:* 权限码精确判定（BFS 有效角色 → role_menus）。
// fail-closed：任一查询失败视为无全局权。
func (s *OrgService) isGlobalOrgAdmin(ctx context.Context, userID int64) bool {
	roleCodes, err := s.roles.GetRoleCodesByUserID(ctx, userID)
	if err != nil {
		return false
	}
	for _, r := range roleCodes {
		if r == "admin" || r == "superadmin" || r == "role::admin" || r == "role::superadmin" {
			return true
		}
	}
	ok, err := s.delegation.HasOrgManagePermission(ctx, userID)
	if err != nil {
		return false
	}
	return ok
}

// GetTree 返回完整组织树（树形结构，按 sort_order、id 排序）
func (s *OrgService) GetTree(ctx context.Context) ([]*model.Organization, error) {
	orgs, err := s.orgRepo.GetTree(ctx)
	if err != nil {
		return nil, err
	}
	return buildOrgTree(orgs), nil
}

// buildOrgTree 平铺列表 → 树（parent_id 归并；父节点不存在时按根节点处理，避免数据异常导致丢节点）。
// 采用递归自底向上组装：节点值拷贝发生在其子树组装完成之后，与 map 遍历顺序无关
// （若在遍历中直接向父节点 Children 追加值拷贝，父节点先于孙节点处理时会嵌入过期拷贝，丢失孙子节点）。
func buildOrgTree(orgs []*model.Organization) []*model.Organization {
	if len(orgs) == 0 {
		return []*model.Organization{}
	}
	byID := make(map[int64]*model.Organization, len(orgs))
	for _, o := range orgs {
		byID[o.ID] = o
	}
	childrenOf := make(map[int64][]*model.Organization, len(orgs))
	var roots []*model.Organization
	for _, o := range orgs {
		if o.ParentID == nil {
			roots = append(roots, o)
			continue
		}
		if _, ok := byID[*o.ParentID]; ok {
			childrenOf[*o.ParentID] = append(childrenOf[*o.ParentID], o)
		} else {
			roots = append(roots, o)
		}
	}
	sortOrgs(roots)
	out := make([]*model.Organization, 0, len(roots))
	for _, r := range roots {
		out = append(out, emitOrgTree(r, childrenOf))
	}
	return out
}

// emitOrgTree 递归组装节点及其完整子树（含排序）
func emitOrgTree(o *model.Organization, childrenOf map[int64][]*model.Organization) *model.Organization {
	kids := append([]*model.Organization(nil), childrenOf[o.ID]...)
	sortOrgs(kids)
	node := new(model.Organization)
	*node = *o
	node.Children = make([]model.Organization, 0, len(kids))
	for _, k := range kids {
		node.Children = append(node.Children, *emitOrgTree(k, childrenOf))
	}
	return node
}

func sortOrgs(orgs []*model.Organization) {
	sort.Slice(orgs, func(i, j int) bool {
		if orgs[i].SortOrder != orgs[j].SortOrder {
			return orgs[i].SortOrder < orgs[j].SortOrder
		}
		return orgs[i].ID < orgs[j].ID
	})
}

func (s *OrgService) GetUserOrgs(ctx context.Context, userID int64) ([]*model.UserOrg, error) {
	if _, err := s.userRepo.FindByID(ctx, userID); err != nil {
		return nil, err
	}
	return s.orgRepo.GetUserOrgs(ctx, userID)
}

func (s *OrgService) AddMember(ctx context.Context, req *model.OrgMemberRequest, actorUserID int64) error {
	if _, err := s.orgRepo.FindByID(ctx, req.OrgID); err != nil {
		return err
	}
	if _, err := s.userRepo.FindByID(ctx, req.UserID); err != nil {
		return err
	}
	// 2c（04 §3.4）：组内级别校验——admin/owner 可加 member；仅 owner 可指定 admin
	role := req.OrgMemberRole
	if role == "" {
		role = "member"
	}
	if err := s.delegation.ensureCanManageMember(ctx, actorUserID, req.OrgID, req.UserID,
		s.isGlobalOrgAdmin(ctx, actorUserID)); err != nil {
		return err
	}
	if role == "admin" {
		// 指定 admin 需 owner 档（admin 调用 → 50008，04 §3.4）
		p, err := s.delegation.EffectiveOrgPriority(ctx, actorUserID, req.OrgID)
		if err != nil {
			return err
		}
		if !s.isGlobalOrgAdmin(ctx, actorUserID) && p > OrgRoleOwnerPriority {
			return errcode.ErrCannotAssignHigherOrgMemberRole
		}
	}
	return s.orgRepo.AddMemberWithRole(ctx, req.OrgID, req.UserID, req.IsPrimary, role, req.TicketScope)
}

// ListOrgRoles 组织已绑定的角色（IW3/BK-12）：org admin/owner 或全局管理员可读。
// org 预检（对齐 AddMember）：不存在/软删 → ErrOrgNotFound，而非空列表冒充 200
func (s *OrgService) ListOrgRoles(ctx context.Context, orgID, actorUserID int64) ([]*model.Role, error) {
	if _, err := s.orgRepo.FindByID(ctx, orgID); err != nil {
		return nil, err
	}
	if !s.isGlobalOrgAdmin(ctx, actorUserID) {
		ok, err := s.delegation.IsOrgAdminOrOwner(ctx, actorUserID, orgID)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, errcode.ErrNoPermission
		}
	}
	return s.orgRepo.ListOrgRoles(ctx, orgID)
}

// BindOrgRole 绑定角色到组织（IW3/BK-12）：**仅全局管理员**——org_roles 赋出的是
// 全局 Casbin 角色（BFS 源 2 → L1 全局能力），影响面 = 全组织成员，授权面对齐
// BK-14 的 scope=all 决议。系统角色不可绑定（repo 层守卫）。
func (s *OrgService) BindOrgRole(ctx context.Context, req *model.BindOrgRoleRequest, actorUserID int64) error {
	// org 预检（对齐 AddMember）：不存在/软删 → ErrOrgNotFound；否则落到 repo 层
	// FK 23503 → 400/500，且软删 org 可静默绑成脏数据
	if _, err := s.orgRepo.FindByID(ctx, req.OrgID); err != nil {
		return err
	}
	if !s.isGlobalOrgAdmin(ctx, actorUserID) {
		return errcode.ErrNoPermission
	}
	return s.orgRepo.BindOrgRole(ctx, req.OrgID, req.RoleID)
}

// UnbindOrgRole 解绑组织角色（IW3/BK-12）：仅全局管理员
func (s *OrgService) UnbindOrgRole(ctx context.Context, req *model.BindOrgRoleRequest, actorUserID int64) error {
	if !s.isGlobalOrgAdmin(ctx, actorUserID) {
		return errcode.ErrNoPermission
	}
	return s.orgRepo.UnbindOrgRole(ctx, req.OrgID, req.RoleID)
}

// SetMemberScope 变更成员数据范围（IW1/BK-14，09 §5.2）：org admin/owner 或全局管理员；
// scope=all 旁路整个 L2（全局可见），仅全局管理员可授（BK-14 决议）并留审计日志
func (s *OrgService) SetMemberScope(ctx context.Context, req *model.SetMemberScopeRequest, actorUserID int64) error {
	if _, err := s.orgRepo.FindByID(ctx, req.OrgID); err != nil {
		return err
	}
	global := s.isGlobalOrgAdmin(ctx, actorUserID)
	if err := s.delegation.ensureCanManageMember(ctx, actorUserID, req.OrgID, req.UserID, global); err != nil {
		return err
	}
	if req.TicketScope == "all" && !global {
		return errcode.ErrNoPermission
	}
	if err := s.orgRepo.SetMemberScope(ctx, req.OrgID, req.UserID, req.TicketScope); err != nil {
		return err
	}
	slog.Info("member ticket_scope changed", "actor", actorUserID, "org", req.OrgID,
		"target", req.UserID, "scope", req.TicketScope)
	return nil
}

func (s *OrgService) RemoveMember(ctx context.Context, req *model.OrgMemberRequest, actorUserID int64) error {
	if _, err := s.orgRepo.FindByID(ctx, req.OrgID); err != nil {
		return err
	}
	// 2c（04 §3.5）：组内防提权——admin 仅可移除 member；owner 任意；全局绕过
	if err := s.delegation.ensureCanManageMember(ctx, actorUserID, req.OrgID, req.UserID,
		s.isGlobalOrgAdmin(ctx, actorUserID)); err != nil {
		return err
	}
	return s.orgRepo.RemoveMember(ctx, req.OrgID, req.UserID)
}

func (s *OrgService) SetUserOrgs(ctx context.Context, req *model.SetUserOrgsRequest) error {
	if _, err := s.userRepo.FindByID(ctx, req.UserID); err != nil {
		return err
	}
	for _, orgID := range req.OrgIDs {
		if _, err := s.orgRepo.FindByID(ctx, orgID); err != nil {
			return err
		}
	}
	if req.PrimaryOrgID != nil {
		found := false
		for _, id := range req.OrgIDs {
			if id == *req.PrimaryOrgID {
				found = true
				break
			}
		}
		if !found {
			return errcode.ErrInvalidParams
		}
	}
	return s.orgRepo.SetUserOrgs(ctx, req.UserID, []int64(req.OrgIDs), req.PrimaryOrgID)
}

// SetUserOrgsTx 在外部事务内全量覆盖用户组织（供 UserService.Create 等事务编排调用）
func (s *OrgService) SetUserOrgsTx(ctx context.Context, tx pgx.Tx, req *model.SetUserOrgsRequest) error {
	if req.PrimaryOrgID != nil {
		found := false
		for _, id := range req.OrgIDs {
			if id == *req.PrimaryOrgID {
				found = true
				break
			}
		}
		if !found {
			return errcode.ErrInvalidParams
		}
	}
	return s.orgRepo.SetUserOrgsTx(ctx, tx, req.UserID, []int64(req.OrgIDs), req.PrimaryOrgID)
}

func (s *OrgService) GetByID(ctx context.Context, id int64) (*model.Organization, error) {
	return s.orgRepo.FindByID(ctx, id)
}

func (s *OrgService) GetMembers(ctx context.Context, orgID int64, page, pageSize int) (*model.OrgMemberListResponse, error) {
	if _, err := s.orgRepo.FindByID(ctx, orgID); err != nil {
		return nil, err
	}
	// B4-5：分页（原全量返回，与 modules/organization.md §4.3 承诺不符）
	users, total, err := s.userRepo.ListByOrgID(ctx, orgID, page, pageSize)
	if err != nil {
		return nil, err
	}
	// 回显规范化与 repo normalizePage 对齐（D2-13：含 page 上限，防溢出回绕负 OFFSET）
	if page < 1 {
		page = 1
	}
	if page > 10000 {
		page = 10000
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return &model.OrgMemberListResponse{
		List:     users,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	}, nil
}

// maxOrgPathDepth 组织树层级上限（D2-44④）——超深子树拼 path 时 ltree
// 报错为 500，前置校验转 400（20 层远超「集团→公司→部门→中心→组」实际需求）
const maxOrgPathDepth = 20

func (s *OrgService) Create(ctx context.Context, req *model.CreateOrgRequest, actorUserID int64) (*model.Organization, error) {
	if !validate.LtreeLabel(req.Code) {
		return nil, errcode.ErrInvalidParams
	}
	// 2b-org 虚拟组（03-org-enhance §2 / hr-directory-sync §2.1）：
	// code 须以 vg_ 前缀区分 HR 部门编码，且必须挂载在实体组织（is_virtual=false）下
	if req.IsVirtual && !strings.HasPrefix(req.Code, "vg_") {
		return nil, &errcode.Error{Code: errcode.ErrInvalidParams.Code, Message: "虚拟组 code 须以 vg_ 前缀"}
	}

	var parentID *int64
	path := req.Code
	if req.ParentID != nil {
		parent, err := s.orgRepo.FindByID(ctx, *req.ParentID)
		if err != nil {
			return nil, err
		}
		// D2-44④：层级前置校验（ltree path 以 '.' 分隔，段数即层级）
		if depth := strings.Count(parent.Path, ".") + 2; depth > maxOrgPathDepth {
			return nil, &errcode.Error{
				Code:    errcode.ErrInvalidParams.Code,
				Message: fmt.Sprintf("组织层级超过上限 %d 层", maxOrgPathDepth),
			}
		}
		// 虚拟组父级必须为实体组织（system 根亦为实体）；
		// 虚拟组下不再挂虚拟组（兄弟可读语义以「同实体锚点」为前提）
		if req.IsVirtual && parent.IsVirtual {
			return nil, &errcode.Error{Code: errcode.ErrInvalidParams.Code, Message: "虚拟组必须挂载在实体组织下"}
		}
		parentID = &parent.ID
		path = parent.Path + "." + req.Code
	}
	if req.ParentID == nil && req.IsVirtual {
		return nil, &errcode.Error{Code: errcode.ErrInvalidParams.Code, Message: "虚拟组必须挂载在实体组织下"}
	}

	org := &model.Organization{
		Code:        req.Code,
		Name:        req.Name,
		Description: req.Description,
		ParentID:    parentID,
		Path:        path,
		IsVirtual:   req.IsVirtual,
		Status:      1,
		SortOrder:   req.SortOrder,
		CreatedBy:   &actorUserID,
	}
	if err := s.orgRepo.Create(ctx, org); err != nil {
		return nil, err
	}
	return org, nil
}

func (s *OrgService) Update(ctx context.Context, req *model.UpdateOrgRequest) (*model.Organization, error) {
	org, err := s.orgRepo.FindByID(ctx, req.ID)
	if err != nil {
		return nil, err
	}
	if org.IsSystem {
		// B4-5：与删除场景的 ErrOrgIsSystem（「不可删除」）区分文案
		return nil, errcode.ErrOrgSystemProtected
	}
	org.Name = req.Name
	// BK-13（09 §5.2.1）：ticket_visibility 仅实体可配；虚拟组继承最近实体祖先，传入即 400
	if req.TicketVisibility != nil {
		if org.IsVirtual {
			return nil, errcode.ErrInvalidParams
		}
		org.TicketVisibility = *req.TicketVisibility
	}
	// D2-03/D2-17：nil = 未传 → 保持现值（patch 语义）
	if req.Description != nil {
		org.Description = *req.Description
	}
	if req.Status != nil {
		org.Status = *req.Status
	}
	if req.SortOrder != nil {
		org.SortOrder = *req.SortOrder
	}
	org.Version = req.Version
	if err := s.orgRepo.Update(ctx, org); err != nil {
		return nil, err
	}
	return org, nil
}

func (s *OrgService) Delete(ctx context.Context, id int64) error {
	org, err := s.orgRepo.FindByID(ctx, id)
	if err != nil {
		return err
	}
	if org.IsSystem {
		return errcode.ErrOrgIsSystem
	}
	// B4-5：children/members 检查已移入 repo.Delete 同事务（消灭 check-then-act 窗口）
	return s.orgRepo.Delete(ctx, id)
}

// SetOwners 设置组织负责人（2c，04 §3.2；D1）。
// 全局 org 管理员或该 org effective owner 可调用；非 owner 调用方不可移除现有 owner（防自我降权踢人）。
// 双轨对齐：SetOwnersTx 同步 user_orgs.org_member_role='owner'。
func (s *OrgService) SetOwners(ctx context.Context, req *model.SetOrgOwnersRequest, actorUserID int64) (*model.Organization, error) {
	org, err := s.orgRepo.FindByID(ctx, req.OrgID)
	if err != nil {
		return nil, err
	}
	globalAdmin := s.isGlobalOrgAdmin(ctx, actorUserID)
	if !globalAdmin {
		p, err := s.delegation.EffectiveOrgPriority(ctx, actorUserID, req.OrgID)
		if err != nil {
			return nil, err
		}
		if p > OrgRoleOwnerPriority {
			return nil, errcode.ErrNotOrgOwner // 50010
		}
		// 非 owner 调用方（即 effective owner 自身）不可移除仍在列表中的现有 owner——
		// 仅全局管理员可清空/剔除 owner（04 §3.2.3）
		for _, existing := range org.OwnerUserIDs {
			kept := false
			for _, id := range req.OwnerUserIDs {
				if id == existing {
					kept = true
					break
				}
			}
			if !kept && existing != actorUserID {
				// owner 移除其他 owner 不允许（自保）；移除自己允许（让位）
				return nil, errcode.ErrNotOrgOwner
			}
		}
	}
	// 校验各 user 存在且未软删（04 §3.2.1）
	for _, uid := range req.OwnerUserIDs {
		if _, err := s.userRepo.FindByID(ctx, uid); err != nil {
			return nil, err
		}
	}
	if err := s.orgRepo.RunInTx(ctx, func(tx pgx.Tx) error {
		return s.orgRepo.SetOwnersTx(ctx, tx, req.OrgID, req.OwnerUserIDs)
	}); err != nil {
		return nil, err
	}
	return s.orgRepo.FindByID(ctx, req.OrgID)
}

// SetMemberRole 任命/变更组内角色（2c，04 §3.3；D2/D3）。
// 仅 effective owner（或全局管理员）可调用；不可设 owner（→400）；不可改 owner_user_ids 用户的角色（→50009）。
func (s *OrgService) SetMemberRole(ctx context.Context, req *model.SetOrgMemberRoleRequest, actorUserID int64) error {
	if req.OrgMemberRole == "owner" {
		return &errcode.Error{Code: errcode.ErrInvalidParams.Code, Message: "owner 仅通过 SetOwners 设置"}
	}
	if _, err := s.orgRepo.FindByID(ctx, req.OrgID); err != nil {
		return err
	}
	// 调用方档位：admin → 50010（D3）；member → 70001
	if err := s.delegation.ensureCanManageMember(ctx, actorUserID, req.OrgID, 0,
		s.isGlobalOrgAdmin(ctx, actorUserID)); err != nil {
		return err
	}
	// 目标非 owner_user_ids 派生 owner（50009，04 §3.3.4）；成员存在性检查（50007）
	// 由 repo 层 SetMemberRoleTx 事务内完成（UPDATE 0 行 → ErrNotOrgMember）
	org, err := s.orgRepo.FindByID(ctx, req.OrgID)
	if err != nil {
		return err
	}
	for _, id := range org.OwnerUserIDs {
		if id == req.UserID {
			return errcode.ErrCannotManageOrgMember
		}
	}
	return s.orgRepo.RunInTx(ctx, func(tx pgx.Tx) error {
		return s.orgRepo.SetMemberRoleTx(ctx, tx, req.OrgID, req.UserID, req.OrgMemberRole)
	})
}

// DeleteOrgDelegated 委托删除入口（2c，04 §3.6；D6）：
// 虚拟组（is_virtual=true）且调用方为 effective owner（或全局）→ 允许删除（仍有成员 → 50005，与 Phase 1 一致）。
// 实体组织删除规则不变。
func (s *OrgService) DeleteOrgDelegated(ctx context.Context, id int64, actorUserID int64) error {
	org, err := s.orgRepo.FindByID(ctx, id)
	if err != nil {
		return err
	}
	if org.IsSystem {
		return errcode.ErrOrgIsSystem
	}
	// 全局管理员走既有 Delete；虚拟组 effective owner 亦可
	if !s.isGlobalOrgAdmin(ctx, actorUserID) {
		if !org.IsVirtual {
			return errcode.ErrNoPermission // 实体删除仅全局（Phase 1 语义）
		}
		ok, err := s.delegation.IsOrgAdminOrOwner(ctx, actorUserID, id)
		if err != nil {
			return err
		}
		if !ok {
			return errcode.ErrNoPermission
		}
	}
	// D6 语义（CC3 原子化）：虚拟组的 owner 成员行是 SetOwners 派生数据，
	// 组消亡即失效——同一事务内预清 owner 行 + 守卫 + 软删；仍有非 owner
	// 成员 → 409+50005（不动任何状态）
	return s.orgRepo.DeleteVgWithOwnerCleanup(ctx, id)
}

// Move 移动组织子树（B3-2：环检测/父读取/锁定已全部移入 repo 单事务，
// 消灭并发交叉移动破坏树不变量与静默失效两个窗口）
func (s *OrgService) Move(ctx context.Context, req *model.MoveOrgRequest) error {
	if req.NewParentID != nil && *req.NewParentID == req.ID {
		return errcode.ErrOrgCannotMoveToChild
	}
	return s.orgRepo.Move(ctx, req.ID, req.NewParentID)
}
