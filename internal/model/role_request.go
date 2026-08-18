package model

// AssignMenusRequest 角色分配菜单
type AssignMenusRequest struct {
	RoleID  int64   `json:"role_id,string" binding:"required"`
	MenuIDs []int64 `json:"menu_ids" binding:"required"`
}

// RoleIDRequest 带 role_id 的请求体
type RoleIDRequest struct {
	RoleID int64 `json:"role_id,string" binding:"required"`
}
