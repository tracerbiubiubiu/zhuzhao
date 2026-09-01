package model

import "github.com/tracerbiubiubiu/zhuzhao/internal/pkg/jsonutil"

// OrgMemberRequest 组织成员操作
type OrgMemberRequest struct {
	OrgID     int64 `json:"org_id,string" binding:"required"`
	UserID    int64 `json:"user_id,string" binding:"required"`
	IsPrimary bool  `json:"is_primary"`
	// 2c（04 §3.4）：可选组内级别；仅 owner 可指定 admin（service 层校验）
	OrgMemberRole string `json:"org_member_role" binding:"omitempty,oneof=member admin"`
	// IW1/BK-14（09 §5.2）：可选数据范围；缺省 assigned；scope=all 仅全局管理员可授（service 层校验）
	TicketScope string `json:"ticket_scope" binding:"omitempty,oneof=assigned group all"`
}

// BindOrgRoleRequest 组织绑定/解绑角色（IW3/BK-12）
type BindOrgRoleRequest struct {
	OrgID  int64 `json:"org_id,string" binding:"required"`
	RoleID int64 `json:"role_id,string" binding:"required"`
}

// SetMemberScopeRequest 变更成员数据范围（IW1/BK-14）
type SetMemberScopeRequest struct {
	OrgID       int64  `json:"org_id,string" binding:"required"`
	UserID      int64  `json:"user_id,string" binding:"required"`
	TicketScope string `json:"ticket_scope" binding:"required,oneof=assigned group all"`
}

// SetOrgOwnersRequest 设置组织负责人（2c，04 §3.2）
type SetOrgOwnersRequest struct {
	OrgID        int64               `json:"org_id,string" binding:"required"`
	OwnerUserIDs jsonutil.Int64Slice `json:"owner_user_ids" binding:"required"` // 元素为字符串 ID（项目惯例）；空数组=清空（仅全局）
}

// SetOrgMemberRoleRequest 任命/变更组内角色（2c，04 §3.3）
type SetOrgMemberRoleRequest struct {
	OrgID         int64  `json:"org_id,string" binding:"required"`
	UserID        int64  `json:"user_id,string" binding:"required"`
	OrgMemberRole string `json:"org_member_role" binding:"required,oneof=member admin"`
	// owner 仅经 SetOwners；请求 owner → 400（service 层校验）
}

// CreateOrgRequest 创建组织
type CreateOrgRequest struct {
	// B4-5：max 对齐 DB varchar（code(50)/name(100)）——原超长触发 22001 → 500
	Code        string `json:"code" binding:"required,max=50"`
	Name        string `json:"name" binding:"required,max=100"`
	Description string `json:"description"`
	ParentID    *int64 `json:"parent_id,string"`
	OrgType     int    `json:"org_type" binding:"required,oneof=1 2 3 4"` // 4=虚拟组（2b-org，03-org-enhance §2）
	SortOrder   int    `json:"sort_order"`
}

// UpdateOrgRequest 更新组织。
// D2-03/D2-17：Status/Description/SortOrder 指针化 patch 语义——
// 未传（nil）保持现值，原零值穿透静默清空/禁用
type UpdateOrgRequest struct {
	ID          int64   `json:"id,string" binding:"required"`
	Version     int     `json:"version" binding:"required"`
	Name        string  `json:"name" binding:"required,max=100"`
	Description *string `json:"description"`
	Status      *int    `json:"status" binding:"omitempty,oneof=0 1"`
	SortOrder   *int    `json:"sort_order"`
	// BK-13（09 §5.2.1）：仅实体（org_type 1–3）可配置；虚拟组继承最近实体祖先，传入即 400
	TicketVisibility *string `json:"ticket_visibility" binding:"omitempty,oneof=entity_transparent_read project_isolated"`
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

// OrgMemberListResponse 组织成员列表（B4-5：分页）
type OrgMemberListResponse struct {
	List     []*User `json:"list"`
	Total    int64   `json:"total"`
	Page     int     `json:"page"`
	PageSize int     `json:"page_size"`
}
