package model

import (
	"time"

	"github.com/google/uuid"
)

// Role 角色模型
type Role struct {
	ID          uuid.UUID `json:"id" db:"id"`
	Code        string    `json:"code" db:"code"` // 角色key: "admin", "editor"
	Name        string    `json:"name" db:"name"`
	Description string    `json:"description" db:"description"`
	Status      int       `json:"status" db:"status"` // 1=启用 0=禁用
	SortOrder   int       `json:"sort_order" db:"sort_order"`
	TenantID    uuid.UUID `json:"tenant_id" db:"tenant_id"`
	Version     int       `json:"version" db:"version"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time `json:"updated_at" db:"updated_at"`
}
