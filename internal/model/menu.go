package model

import (
	"time"

	"github.com/google/uuid"
)

// Menu 菜单模型
type Menu struct {
	ID         uuid.UUID  `json:"id" db:"id"`
	ParentID   *uuid.UUID `json:"parent_id" db:"parent_id"`
	Name       string     `json:"name" db:"name"`
	Path       string     `json:"path" db:"path"`
	RouteName  string     `json:"route_name" db:"route_name"`
	Component  string     `json:"component" db:"component"`
	Redirect   string     `json:"redirect" db:"redirect"`
	Icon       string     `json:"icon" db:"icon"`
	SortOrder  int        `json:"sort_order" db:"sort_order"`
	Type       int        `json:"type" db:"type"` // 1=目录 2=菜单 3=按钮
	Permission string     `json:"permission" db:"permission"`
	Hidden     bool       `json:"hidden" db:"hidden"`
	Version    int        `json:"version" db:"version"`
	CreatedAt  time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at" db:"updated_at"`
	Children   []Menu     `json:"children,omitempty" db:"-"` // 前端树形结构，不入库
}

// RoleMenu 角色-菜单关联
type RoleMenu struct {
	RoleID uuid.UUID `json:"role_id" db:"role_id"`
	MenuID uuid.UUID `json:"menu_id" db:"menu_id"`
}
