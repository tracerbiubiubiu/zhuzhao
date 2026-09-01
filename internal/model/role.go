package model

import (
	"time"
)

// Role 角色模型
type Role struct {
	ID          int64  `json:"id,string" db:"id"`
	Code        string `json:"code" db:"code"` // 角色编码: "admin", "user_manager"
	Name        string `json:"name" db:"name"`
	Description string `json:"description" db:"description"`
	Status      int    `json:"status" db:"status"` // 1=启用 0=禁用
	Priority    int    `json:"priority" db:"priority"`
	// BK-12：角色继承父（BFS 源 3：绑子得父，沿链向上展开）；单调规则 child.priority ≤ parent.priority
	ParentID  *int64     `json:"parent_id,string,omitempty" db:"parent_id"`
	SortOrder int        `json:"sort_order" db:"sort_order"`
	IsSystem  bool       `json:"is_system" db:"is_system"`
	TenantID  int64      `json:"tenant_id,string" db:"tenant_id"`
	Version   int        `json:"version" db:"version"`
	DeletedAt *time.Time `json:"deleted_at,omitempty" db:"deleted_at"`
	CreatedAt time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt time.Time  `json:"updated_at" db:"updated_at"`
}
