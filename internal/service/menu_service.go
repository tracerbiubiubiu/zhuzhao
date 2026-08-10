package service

import (
	"context"
	"fmt"

	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

// MenuService 菜单服务
type MenuService struct {
	menuRepo *repository.MenuRepo
}

func NewMenuService(menuRepo *repository.MenuRepo) *MenuService {
	return &MenuService{menuRepo: menuRepo}
}

// GetUserMenus 获取用户菜单树
func (s *MenuService) GetUserMenus(ctx context.Context, userID string) (interface{}, error) {
	return nil, fmt.Errorf("not implemented")
}

// GetUserPermissions 获取用户按钮权限码列表
func (s *MenuService) GetUserPermissions(ctx context.Context, userID string) ([]string, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *MenuService) GetTree(ctx context.Context) (interface{}, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *MenuService) Create(ctx context.Context, req interface{}) error {
	return fmt.Errorf("not implemented")
}

func (s *MenuService) Update(ctx context.Context, id string, req interface{}) error {
	return fmt.Errorf("not implemented")
}

func (s *MenuService) Delete(ctx context.Context, id string) error {
	return fmt.Errorf("not implemented")
}
