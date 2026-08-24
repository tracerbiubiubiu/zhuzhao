package service

import (
	"context"
	"sort"

	"github.com/jackc/pgx/v5"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/validate"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

// OrgService 组织架构管理服务
type OrgService struct {
	orgRepo  *repository.OrgRepo
	userRepo *repository.UserRepo
}

func NewOrgService(orgRepo *repository.OrgRepo, userRepo *repository.UserRepo) *OrgService {
	return &OrgService{orgRepo: orgRepo, userRepo: userRepo}
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

func (s *OrgService) AddMember(ctx context.Context, req *model.OrgMemberRequest) error {
	if _, err := s.orgRepo.FindByID(ctx, req.OrgID); err != nil {
		return err
	}
	if _, err := s.userRepo.FindByID(ctx, req.UserID); err != nil {
		return err
	}
	return s.orgRepo.AddMember(ctx, req.OrgID, req.UserID, req.IsPrimary)
}

func (s *OrgService) RemoveMember(ctx context.Context, req *model.OrgMemberRequest) error {
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

func (s *OrgService) GetMembers(ctx context.Context, orgID int64) (*model.OrgMemberListResponse, error) {
	if _, err := s.orgRepo.FindByID(ctx, orgID); err != nil {
		return nil, err
	}
	users, err := s.userRepo.ListByOrgID(ctx, orgID)
	if err != nil {
		return nil, err
	}
	return &model.OrgMemberListResponse{
		List:  users,
		Total: int64(len(users)),
	}, nil
}

func (s *OrgService) Create(ctx context.Context, req *model.CreateOrgRequest, actorUserID int64) (*model.Organization, error) {
	if !validate.LtreeLabel(req.Code) {
		return nil, errcode.ErrInvalidParams
	}
	if req.OrgType == 4 {
		return nil, errcode.ErrInvalidParams
	}

	var parentID *int64
	path := req.Code
	if req.ParentID != nil {
		parent, err := s.orgRepo.FindByID(ctx, *req.ParentID)
		if err != nil {
			return nil, err
		}
		parentID = &parent.ID
		path = parent.Path + "." + req.Code
	}

	org := &model.Organization{
		Code:        req.Code,
		Name:        req.Name,
		Description: req.Description,
		ParentID:    parentID,
		Path:        path,
		OrgType:     req.OrgType,
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
		return nil, errcode.ErrOrgIsSystem
	}
	org.Name = req.Name
	org.Description = req.Description
	org.Status = req.Status
	org.SortOrder = req.SortOrder
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
	n, err := s.orgRepo.CountChildren(ctx, id)
	if err != nil {
		return err
	}
	if n > 0 {
		return errcode.ErrOrgHasChildren
	}
	n, err = s.orgRepo.CountMembers(ctx, id)
	if err != nil {
		return err
	}
	if n > 0 {
		return errcode.ErrOrgHasMembers
	}
	return s.orgRepo.Delete(ctx, id)
}

// Move 移动组织子树（B3-2：环检测/父读取/锁定已全部移入 repo 单事务，
// 消灭并发交叉移动破坏树不变量与静默失效两个窗口）
func (s *OrgService) Move(ctx context.Context, req *model.MoveOrgRequest) error {
	if req.NewParentID != nil && *req.NewParentID == req.ID {
		return errcode.ErrOrgCannotMoveToChild
	}
	return s.orgRepo.Move(ctx, req.ID, req.NewParentID)
}
