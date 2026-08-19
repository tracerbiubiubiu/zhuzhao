package model

// OrgMemberRequest 组织成员操作
type OrgMemberRequest struct {
	OrgID     int64 `json:"org_id,string" binding:"required"`
	UserID    int64 `json:"user_id,string" binding:"required"`
	IsPrimary bool  `json:"is_primary"`
}

// CreateOrgRequest 创建组织
type CreateOrgRequest struct {
	Code        string `json:"code" binding:"required"`
	Name        string `json:"name" binding:"required"`
	Description string `json:"description"`
	ParentID    *int64 `json:"parent_id,string"`
	OrgType     int    `json:"org_type" binding:"required,oneof=1 2 3"`
	SortOrder   int    `json:"sort_order"`
}

// UpdateOrgRequest 更新组织
type UpdateOrgRequest struct {
	ID          int64  `json:"id,string" binding:"required"`
	Version     int    `json:"version" binding:"required"`
	Name        string `json:"name" binding:"required"`
	Description string `json:"description"`
	Status      int    `json:"status" binding:"oneof=0 1"`
	SortOrder   int    `json:"sort_order"`
}

// OrgIDRequest 带 org_id 的请求
type OrgIDRequest struct {
	OrgID int64 `json:"org_id,string" binding:"required"`
}

// MoveOrgRequest 移动组织
type MoveOrgRequest struct {
	ID          int64  `json:"id,string" binding:"required"`
	NewParentID *int64 `json:"new_parent_id,string"`
}

// OrgMemberListResponse 组织成员列表
type OrgMemberListResponse struct {
	List  []*User `json:"list"`
	Total int64   `json:"total"`
}
