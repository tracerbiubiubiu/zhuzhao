package model

import "github.com/tracerbiubiubiu/zhuzhao/internal/pkg/jsonutil"

// CreateUserRequest 创建用户
type CreateUserRequest struct {
	Username      string              `json:"username" binding:"required"`
	Password      string              `json:"password" binding:"required,min=8"` // F-9：最小长度（完整复杂度策略 Phase 2）
	EmployeeNo    string              `json:"employee_no"`
	DomainAccount string              `json:"domain_account"`
	UserDomain    string              `json:"user_domain"`
	RealName      string              `json:"real_name"`
	Email         string              `json:"email"`
	Phone         string              `json:"phone"`
	Avatar        string              `json:"avatar"`
	RoleIDs       jsonutil.Int64Slice `json:"role_ids"`
	OrgIDs        jsonutil.Int64Slice `json:"org_ids"`
	PrimaryOrgID  *int64              `json:"primary_org_id,string"`
}

// UpdateUserRequest 更新用户（B2-3 patch 语义：未传字段保持不变，传空串显式清空；
// username 不可改——文档「Phase 2 再定改名流程」）
type UpdateUserRequest struct {
	ID      int64 `json:"id,string" binding:"required"`
	Version int   `json:"version" binding:"required"`
	// 指针字段：nil = 未传（保持原值），非 nil 空串 = 显式清空
	EmployeeNo    *string `json:"employee_no"`
	DomainAccount *string `json:"domain_account"`
	UserDomain    *string `json:"user_domain"`
	RealName      *string `json:"real_name"`
	Email         *string `json:"email"`
	Phone         *string `json:"phone"`
	Avatar        *string `json:"avatar"`
}

// UserIDRequest 带 user_id 的请求
type UserIDRequest struct {
	UserID int64 `json:"user_id,string" binding:"required"`
}

// UpdateUserStatusRequest 启用/禁用
type UpdateUserStatusRequest struct {
	UserID int64 `json:"user_id,string" binding:"required"`
	Status int   `json:"status" binding:"oneof=0 1"`
}

// SetUserRolesRequest 分配角色
type SetUserRolesRequest struct {
	UserID  int64               `json:"user_id,string" binding:"required"`
	RoleIDs jsonutil.Int64Slice `json:"role_ids"`
}

// ResetPasswordRequest 管理员重置密码
type ResetPasswordRequest struct {
	UserID   int64  `json:"user_id,string" binding:"required"`
	Password string `json:"password" binding:"required,min=8"`
}

// SetUserOrgsRequest 全量设置用户组织
type SetUserOrgsRequest struct {
	UserID       int64               `json:"user_id,string" binding:"required"`
	OrgIDs       jsonutil.Int64Slice `json:"org_ids"`
	PrimaryOrgID *int64              `json:"primary_org_id,string"`
}

// UpdateProfileRequest 更新个人资料（patch 语义同 UpdateUserRequest）
type UpdateProfileRequest struct {
	RealName *string `json:"real_name"`
	Email    *string `json:"email"`
	Phone    *string `json:"phone"`
	Avatar   *string `json:"avatar"`
}

// UpdatePasswordRequest 用户修改密码
type UpdatePasswordRequest struct {
	OldPassword string `json:"old_password" binding:"required"`
	NewPassword string `json:"new_password" binding:"required,min=8"`
	DeviceID    string `json:"device_id"` // 与登录时一致；空则使用 default
}

// UserListResponse 用户列表
type UserListResponse struct {
	List     []*User `json:"list"`
	Total    int64   `json:"total"`
	Page     int     `json:"page"`
	PageSize int     `json:"page_size"`
}
