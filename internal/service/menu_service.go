package service

import (
	"context"
	"sort"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/validate"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

const (
	menuTypeDirectory = 1
	menuTypePage      = 2
	menuTypeButton    = 3
)

// MenuService 菜单服务
type MenuService struct {
	menuRepo *repository.MenuRepo
	userRepo *repository.UserRepo
	roleRepo *repository.RoleRepo
}

func NewMenuService(menuRepo *repository.MenuRepo, userRepo *repository.UserRepo, roleRepo *repository.RoleRepo) *MenuService {
	return &MenuService{menuRepo: menuRepo, userRepo: userRepo, roleRepo: roleRepo}
}

// GetUserMenus 获取用户菜单树（不含按钮）
func (s *MenuService) GetUserMenus(ctx context.Context, userID int64) ([]model.Menu, error) {
	roleIDs, err := s.roleRepo.ListRoleIDsByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if len(roleIDs) == 0 {
		return []model.Menu{}, nil
	}

	menus, err := s.menuRepo.ListByRoleIDs(ctx, roleIDs)
	if err != nil {
		return nil, err
	}
	allMenus, err := s.menuRepo.ListAll(ctx)
	if err != nil {
		return nil, err
	}
	menus = includeMenuAncestors(menus, allMenus)
	menus = filterMenusForTree(menus)
	return buildMenuTree(menus), nil
}

// GetUserPermissions 获取用户按钮/路由权限码
func (s *MenuService) GetUserPermissions(ctx context.Context, userID int64) ([]string, error) {
	roleIDs, err := s.roleRepo.ListRoleIDsByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if len(roleIDs) == 0 {
		return []string{}, nil
	}

	menus, err := s.menuRepo.ListByRoleIDs(ctx, roleIDs)
	if err != nil {
		return nil, err
	}

	seen := make(map[string]struct{})
	var perms []string
	for _, m := range menus {
		switch m.MenuType {
		case menuTypeButton:
			if m.Permission == "" {
				continue
			}
			code := "button:" + m.Permission
			if _, ok := seen[code]; ok {
				continue
			}
			seen[code] = struct{}{}
			perms = append(perms, code)
		case menuTypeDirectory, menuTypePage:
			if m.Path == "" {
				continue
			}
			code := "route:" + m.Path
			if _, ok := seen[code]; ok {
				continue
			}
			seen[code] = struct{}{}
			perms = append(perms, code)
		}
	}
	sort.Strings(perms)
	return perms, nil
}

func (s *MenuService) GetTree(ctx context.Context) ([]model.Menu, error) {
	menus, err := s.menuRepo.ListAll(ctx)
	if err != nil {
		return nil, err
	}
	menus = filterMenusForTree(menus)
	return buildMenuTree(menus), nil
}

func (s *MenuService) GetByID(ctx context.Context, id int64) (*model.Menu, error) {
	return s.menuRepo.FindByID(ctx, id)
}

func (s *MenuService) Create(ctx context.Context, req *model.CreateMenuRequest) (*model.Menu, error) {
	if !validate.Identifier(req.Code) {
		return nil, errcode.ErrInvalidParams
	}
	if err := s.validateMenuParent(ctx, req.MenuType, req.ParentID); err != nil {
		return nil, err
	}
	visible := true
	if req.Visible != nil {
		visible = *req.Visible
	}
	menu := &model.Menu{
		ParentID:   req.ParentID,
		Code:       req.Code,
		Name:       req.Name,
		MenuType:   req.MenuType,
		Path:       req.Path,
		Component:  req.Component,
		Icon:       req.Icon,
		Permission: req.Permission,
		SortOrder:  req.SortOrder,
		Visible:    visible,
	}
	if err := s.menuRepo.Create(ctx, menu); err != nil {
		return nil, err
	}
	return menu, nil
}

func (s *MenuService) Update(ctx context.Context, req *model.UpdateMenuRequest) (*model.Menu, error) {
	menu, err := s.menuRepo.FindByID(ctx, req.ID)
	if err != nil {
		return nil, err
	}
	if menu.IsSystem {
		return nil, errcode.ErrMenuIsSystem
	}
	menu.Name = req.Name
	menu.Path = req.Path
	menu.Component = req.Component
	menu.Icon = req.Icon
	menu.Permission = req.Permission
	menu.SortOrder = req.SortOrder
	if req.Visible != nil {
		menu.Visible = *req.Visible
	}
	menu.Version = req.Version
	if err := s.menuRepo.Update(ctx, menu); err != nil {
		return nil, err
	}
	return menu, nil
}

func (s *MenuService) Delete(ctx context.Context, id int64) error {
	menu, err := s.menuRepo.FindByID(ctx, id)
	if err != nil {
		return err
	}
	if menu.IsSystem {
		return errcode.ErrMenuIsSystem
	}
	n, err := s.menuRepo.CountChildren(ctx, id)
	if err != nil {
		return err
	}
	if n > 0 {
		return errcode.ErrMenuHasChildren
	}
	return s.menuRepo.Delete(ctx, id)
}

func (s *MenuService) validateMenuParent(ctx context.Context, menuType int, parentID *int64) error {
	switch menuType {
	case menuTypeDirectory:
		if parentID == nil {
			return nil
		}
		parent, err := s.menuRepo.FindByID(ctx, *parentID)
		if err != nil {
			return err
		}
		if parent.MenuType != menuTypeDirectory {
			return errcode.ErrInvalidParams
		}
	case menuTypePage:
		if parentID == nil {
			return errcode.ErrInvalidParams
		}
		parent, err := s.menuRepo.FindByID(ctx, *parentID)
		if err != nil {
			return err
		}
		if parent.MenuType != menuTypeDirectory {
			return errcode.ErrInvalidParams
		}
	case menuTypeButton:
		if parentID == nil {
			return errcode.ErrInvalidParams
		}
		parent, err := s.menuRepo.FindByID(ctx, *parentID)
		if err != nil {
			return err
		}
		if parent.MenuType != menuTypePage {
			return errcode.ErrInvalidParams
		}
	default:
		return errcode.ErrInvalidParams
	}
	return nil
}

func filterMenusForTree(menus []*model.Menu) []*model.Menu {
	out := make([]*model.Menu, 0, len(menus))
	for _, m := range menus {
		if m.MenuType == menuTypeButton {
			continue
		}
		out = append(out, m)
	}
	return out
}

func includeMenuAncestors(assigned []*model.Menu, all []*model.Menu) []*model.Menu {
	byID := make(map[int64]*model.Menu, len(all))
	for _, m := range all {
		byID[m.ID] = m
	}
	result := make(map[int64]*model.Menu)
	for _, m := range assigned {
		cur := m
		for cur != nil {
			result[cur.ID] = cur
			if cur.ParentID == nil {
				break
			}
			cur = byID[*cur.ParentID]
		}
	}
	out := make([]*model.Menu, 0, len(result))
	for _, m := range result {
		out = append(out, m)
	}
	return out
}

func buildMenuTree(menus []*model.Menu) []model.Menu {
	if len(menus) == 0 {
		return []model.Menu{}
	}
	byID := make(map[int64]*model.Menu, len(menus))
	for _, m := range menus {
		copy := *m
		copy.Children = nil
		byID[m.ID] = &copy
	}
	var roots []*model.Menu
	for _, m := range byID {
		if m.ParentID == nil {
			roots = append(roots, m)
			continue
		}
		if parent, ok := byID[*m.ParentID]; ok {
			parent.Children = append(parent.Children, *m)
		} else {
			roots = append(roots, m)
		}
	}
	sortMenus(roots)
	out := make([]model.Menu, 0, len(roots))
	for _, r := range roots {
		sortMenuChildren(r)
		out = append(out, *r)
	}
	return out
}

func sortMenus(menus []*model.Menu) {
	sort.Slice(menus, func(i, j int) bool {
		if menus[i].SortOrder != menus[j].SortOrder {
			return menus[i].SortOrder < menus[j].SortOrder
		}
		return menus[i].ID < menus[j].ID
	})
}

func sortMenuChildren(m *model.Menu) {
	if len(m.Children) == 0 {
		return
	}
	sort.Slice(m.Children, func(i, j int) bool {
		if m.Children[i].SortOrder != m.Children[j].SortOrder {
			return m.Children[i].SortOrder < m.Children[j].SortOrder
		}
		return m.Children[i].ID < m.Children[j].ID
	})
	for i := range m.Children {
		sortMenuChildren(&m.Children[i])
	}
}
