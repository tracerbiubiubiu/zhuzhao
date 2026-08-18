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
