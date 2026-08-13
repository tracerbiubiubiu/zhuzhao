package model

import (
	"time"
)

// Menu 菜单模型
type Menu struct {
	ID         int64      `json:"id,string" db:"id"`
	ParentID   *int64     `json:"parent_id,string" db:"parent_id"`
	Code       string     `json:"code" db:"code"`           // 业务编码
	Name       string     `json:"name" db:"name"`
	MenuType   int        `json:"menu_type" db:"menu_type"` // 1=目录 2=菜单 3=按钮
	Path       string     `json:"path" db:"path"`           // 前端路由
	Component  string     `json:"component" db:"component"` // 前端组件
	Icon       string     `json:"icon" db:"icon"`
	Permission string     `json:"permission" db:"permission"` // 按钮权限码
	SortOrder  int        `json:"sort_order" db:"sort_order"`
	Visible    bool       `json:"visible" db:"visible"`
	IsSystem   bool       `json:"is_system" db:"is_system"`
	Version    int        `json:"version" db:"version"`
	DeletedAt  *time.Time `json:"deleted_at,omitempty" db:"deleted_at"`
	CreatedAt  time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at" db:"updated_at"`
	Children   []Menu     `json:"children,omitempty" db:"-"` // 前端树形结构，不入库
}

// MenuAPI 菜单-API 绑定（用于 Casbin 策略生成）
type MenuAPI struct {
	MenuID    int64  `json:"menu_id,string" db:"menu_id"`
	APIPath   string `json:"api_path" db:"api_path"`
	APIMethod string `json:"api_method" db:"api_method"`
}

// RoleMenu 角色-菜单关联
type RoleMenu struct {
	RoleID int64 `json:"role_id,string" db:"role_id"`
	MenuID int64 `json:"menu_id,string" db:"menu_id"`
}
