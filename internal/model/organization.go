package model

import (
	"time"

	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/jsonutil"
)

// Organization 组织模型（实体组织 + 虚拟组统一）
type Organization struct {
	ID           int64               `json:"id,string" db:"id"`
	Code         string              `json:"code" db:"code"` // 业务编码，ltree path 用
	Name         string              `json:"name" db:"name"`
	Description  string              `json:"description" db:"description"`
	ParentID     *int64              `json:"parent_id,string" db:"parent_id"`
	Path         string              `json:"path" db:"path"`            // ltree 路径: root.tech.fe
	Children     []Organization      `json:"children,omitempty" db:"-"` // 前端树形结构，不入库
	OrgType      int                 `json:"org_type" db:"org_type"`    // 1=公司 2=部门 3=小组 4=虚拟组
	Status       int                 `json:"status" db:"status"`        // 1=启用 0=禁用
	IsSystem     bool                `json:"is_system" db:"is_system"`
	SortOrder    int                 `json:"sort_order" db:"sort_order"`
	OwnerUserIDs jsonutil.Int64Slice `json:"owner_user_ids" db:"owner_user_ids"` // 2c 负责人（可多人；元素字符串 ID）
	CreatedBy    *int64              `json:"created_by,string" db:"created_by"`
	TenantID     int64               `json:"tenant_id,string" db:"tenant_id"`
	Version      int                 `json:"version" db:"version"`
	DeletedAt    *time.Time          `json:"deleted_at,omitempty" db:"deleted_at"`
	CreatedAt    time.Time           `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time           `json:"updated_at" db:"updated_at"`
}

// UserOrg 用户-组织关系
type UserOrg struct {
	UserID    int64     `json:"user_id,string" db:"user_id"`
	OrgID     int64     `json:"org_id,string" db:"org_id"`
	IsPrimary bool      `json:"is_primary" db:"is_primary"`
	JoinedAt  time.Time `json:"joined_at" db:"joined_at"`
}
