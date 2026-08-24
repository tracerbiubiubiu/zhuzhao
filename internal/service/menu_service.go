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

	// B4-4：admin/superadmin 有 Casbin matcher bypass（真实权限是全部 API），
	// 权限码按全量菜单展开——修复前仅按 role_menus 勾选下发，超管角色被清空
	// 菜单时 permissions=[] 与真实权限背离（对照 GetRolePermissions 的 *,* 特判）
	roleCodes, err := s.roleRepo.ListRoleCodesByUserIDs(ctx, roleIDs)
	if err != nil {
		return nil, err
	}
	for _, code := range roleCodes {
		if code == "admin" || code == "superadmin" {
			return s.allPermissions(ctx)
		}
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

// allPermissions 全量权限码（admin/superadmin 通配展开）
func (s *MenuService) allPermissions(ctx context.Context) ([]string, error) {
	menus, err := s.menuRepo.ListAll(ctx)
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
			if _, ok := seen[code]; !ok {
				seen[code] = struct{}{}
				perms = append(perms, code)
			}
		case menuTypeDirectory, menuTypePage:
			if m.Path == "" {
				continue
			}
			code := "route:" + m.Path
			if _, ok := seen[code]; !ok {
				seen[code] = struct{}{}
				perms = append(perms, code)
			}
		}
	}
	sort.Strings(perms)
	return perms, nil
}

// GetTree 管理端完整菜单树（含按钮节点：角色分配菜单时需勾选按钮，与 phase1/07 §预期功能一致）。
// 用户侧菜单树不含按钮，见 GetUserMenus。
func (s *MenuService) GetTree(ctx context.Context) ([]model.Menu, error) {
	menus, err := s.menuRepo.ListAll(ctx)
	if err != nil {
		return nil, err
	}
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
	// B4-4：类型必要字段——页面(2)必有 path（动态路由注册）、按钮(3)必有
	// permission（权限码下发）；缺失将出现「树里有节点、权限码里无路由」的矛盾数据
	if err := validateMenuRequiredFields(req.MenuType, req.Path, req.Permission); err != nil {
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
	// B4-4：类型必要字段（同 Create）
	if err := validateMenuRequiredFields(menu.MenuType, req.Path, req.Permission); err != nil {
		return nil, err
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

// validateMenuRequiredFields 菜单类型必要字段校验（B4-4）：
// 页面(2)必有 path、按钮(3)必有 permission——防止「树里有节点、权限码里无路由」
func validateMenuRequiredFields(menuType int, path, permission string) error {
	switch menuType {
	case menuTypePage:
		if path == "" {
			return errcode.ErrInvalidParams
		}
	case menuTypeButton:
		if permission == "" {
			return errcode.ErrInvalidParams
		}
	}
	return nil
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
		// B2-4：用户菜单树只含 visible=true 节点（07-menu.md/modules-menu.md 承诺）。
		// 在 includeMenuAncestors 补链之后执行：父不可见而子可见时，子提升为根。
		// 管理端 GetTree 不走此过滤（含隐藏节点，供角色分配与运维）
		if !m.Visible {
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

// buildMenuTree 平铺列表 → 树。采用递归自底向上组装：节点值拷贝发生在其子树组装完成之后，
// 与 map 遍历顺序无关（若在遍历中直接向父节点 Children 追加值拷贝，父节点先于孙节点处理时
// 会嵌入过期拷贝，丢失孙子节点——管理端菜单树含 3 层「目录→页面→按钮」时曾随机触发）。
func buildMenuTree(menus []*model.Menu) []model.Menu {
	if len(menus) == 0 {
		return []model.Menu{}
	}
	byID := make(map[int64]*model.Menu, len(menus))
	for _, m := range menus {
		byID[m.ID] = m
	}
	childrenOf := make(map[int64][]*model.Menu, len(menus))
	var roots []*model.Menu
	for _, m := range menus {
		if m.ParentID == nil {
			roots = append(roots, m)
			continue
		}
		if _, ok := byID[*m.ParentID]; ok {
			childrenOf[*m.ParentID] = append(childrenOf[*m.ParentID], m)
		} else {
			roots = append(roots, m)
		}
	}
	sortMenus(roots)
	out := make([]model.Menu, 0, len(roots))
	for _, r := range roots {
		out = append(out, *emitMenuTree(r, childrenOf))
	}
	return out
}

// emitMenuTree 递归组装节点及其完整子树（含排序）
func emitMenuTree(m *model.Menu, childrenOf map[int64][]*model.Menu) *model.Menu {
	kids := append([]*model.Menu(nil), childrenOf[m.ID]...)
	sortMenus(kids)
	node := new(model.Menu)
	*node = *m
	node.Children = make([]model.Menu, 0, len(kids))
	for _, k := range kids {
		node.Children = append(node.Children, *emitMenuTree(k, childrenOf))
	}
	return node
}

func sortMenus(menus []*model.Menu) {
	sort.Slice(menus, func(i, j int) bool {
		if menus[i].SortOrder != menus[j].SortOrder {
			return menus[i].SortOrder < menus[j].SortOrder
		}
		return menus[i].ID < menus[j].ID
	})
}
