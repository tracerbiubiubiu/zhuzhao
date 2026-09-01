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
	// B4-4：指针区分「未传」（nil → 默认启用）与「显式传 0」（创建即禁用）——
	// 原零值合并导致无法创建禁用角色，与 Update 的 oneof=0 1 行为不一致
	Status *int `json:"status" binding:"omitempty,oneof=0 1"`
	// BK-12：继承父（可选）。校验规则（2026-08-31 拍板）：child.priority ≤ parent.priority
	// （子不弱于父）+ 父启用/非系统/无环/非自身
	ParentID *int64 `json:"parent_id,string,omitempty"`
}

// UpdateRoleRequest 更新角色。
// D2-03/D2-17：Status/Description/SortOrder 指针化 patch 语义——
// 未传（nil）保持现值（原零值穿透：改名即静默禁用/清描述/归零排序）
type UpdateRoleRequest struct {
	ID          int64   `json:"id,string" binding:"required"`
	Version     int     `json:"version" binding:"required"`
	Name        string  `json:"name" binding:"required"`
	Description *string `json:"description"`
	Priority    int     `json:"priority" binding:"required"`
	SortOrder   *int    `json:"sort_order"`
	Status      *int    `json:"status" binding:"omitempty,oneof=0 1"`
	// BK-12：ParentID 显式变更（nil=保持现值）；ClearParent=true 清除继承
	ParentID    *int64 `json:"parent_id,string,omitempty"`
	ClearParent bool   `json:"clear_parent"`
}
