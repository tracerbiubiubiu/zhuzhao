package model

// CreateMenuRequest 创建菜单
type CreateMenuRequest struct {
	ParentID   *int64 `json:"parent_id,string"`
	Code       string `json:"code" binding:"required"`
	Name       string `json:"name" binding:"required"`
	MenuType   int    `json:"menu_type" binding:"required,oneof=1 2 3"`
	Path       string `json:"path"`
	Component  string `json:"component"`
	Icon       string `json:"icon"`
	Permission string `json:"permission"`
	SortOrder  int    `json:"sort_order"`
	Visible    *bool  `json:"visible"`
}

// UpdateMenuRequest 更新菜单
type UpdateMenuRequest struct {
	ID         int64  `json:"id,string" binding:"required"`
	Version    int    `json:"version" binding:"required"`
	Name       string `json:"name" binding:"required"`
	Path       string `json:"path"`
	Component  string `json:"component"`
	Icon       string `json:"icon"`
	Permission string `json:"permission"`
	SortOrder  int    `json:"sort_order"`
	Visible    *bool  `json:"visible"`
}

// MenuIDRequest 带 menu_id 的请求
type MenuIDRequest struct {
	MenuID int64 `json:"menu_id,string" binding:"required"`
}
