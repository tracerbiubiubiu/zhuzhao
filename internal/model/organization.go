package model

import (
	"time"

	"github.com/google/uuid"
)

// Organization 组织模型（实体组织 + 虚拟组统一）
type Organization struct {
	ID         uuid.UUID  `json:"id" db:"id"`
	ParentID   *uuid.UUID `json:"parent_id" db:"parent_id"`
	Name       string     `json:"name" db:"name"`
	OrgType    int        `json:"org_type" db:"org_type"` // 1=实体组织 2=虚拟组
	Code       string     `json:"code" db:"code"`
	Path       string     `json:"path" db:"path"` // ltree 路径: root.tech.fe
	LeaderID   *uuid.UUID `json:"leader_id" db:"leader_id"`
	Status     int        `json:"status" db:"status"`
	SortOrder  int        `json:"sort_order" db:"sort_order"`
	TenantID   uuid.UUID  `json:"tenant_id" db:"tenant_id"`
	Version    int        `json:"version" db:"version"`
	DeletedAt  *time.Time `json:"deleted_at,omitempty" db:"deleted_at"`
	CreatedAt  time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at" db:"updated_at"`
}

// UserOrg 用户-组织关系
type UserOrg struct {
	UserID    uuid.UUID `json:"user_id" db:"user_id"`
	OrgID     uuid.UUID `json:"org_id" db:"org_id"`
	RoleID    uuid.UUID `json:"role_id" db:"role_id"`
	IsPrimary bool      `json:"is_primary" db:"is_primary"`
	JoinedAt  time.Time `json:"joined_at" db:"joined_at"`
}
