package model

import "github.com/tracerbiubiubiu/zhuzhao/internal/pkg/jsonutil"

// AssignMenusRequest 角色分配菜单
type AssignMenusRequest struct {
	RoleID  int64               `json:"role_id,string" binding:"required"`
	MenuIDs jsonutil.Int64Slice `json:"menu_ids" binding:"required"`
}

// RoleIDRequest 带 role_id 的请求体
type RoleIDRequest struct {
	RoleID int64 `json:"role_id,string" binding:"required"`
}

// CreateRoleRequest 创建角色
type CreateRoleRequest struct {
	Code        string `json:"code" binding:"required"`
	Name        string `json:"name" binding:"required"`
	Description string `json:"description"`
	Priority    int    `json:"priority" binding:"required"`
	SortOrder   int    `json:"sort_order"`
	Status      int    `json:"status" binding:"omitempty,oneof=0 1"`
}

// UpdateRoleRequest 更新角色
type UpdateRoleRequest struct {
	ID          int64  `json:"id,string" binding:"required"`
	Version     int    `json:"version" binding:"required"`
	Name        string `json:"name" binding:"required"`
	Description string `json:"description"`
	Priority    int    `json:"priority" binding:"required"`
	SortOrder   int    `json:"sort_order"`
	Status      int    `json:"status" binding:"oneof=0 1"`
}
