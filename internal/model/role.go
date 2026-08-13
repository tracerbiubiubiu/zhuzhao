package model

import (
	"time"
)

// Role 角色模型
type Role struct {
	ID          int64     `json:"id,string" db:"id"`
	Code        string    `json:"code" db:"code"` // 角色编码: "admin", "user_manager"
	Name        string    `json:"name" db:"name"`
	Description string    `json:"description" db:"description"`
	Status      int       `json:"status" db:"status"` // 1=启用 0=禁用
	SortOrder   int       `json:"sort_order" db:"sort_order"`
	IsSystem    bool      `json:"is_system" db:"is_system"`
	TenantID    int64     `json:"tenant_id,string" db:"tenant_id"`
	Version     int       `json:"version" db:"version"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time `json:"updated_at" db:"updated_at"`
}
