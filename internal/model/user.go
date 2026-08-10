package model

import (
	"time"

	"github.com/google/uuid"
)

// User 用户模型
type User struct {
	ID            uuid.UUID  `json:"id" db:"id"`
	Username      string     `json:"username" db:"username"`
	Password      string     `json:"-" db:"password"`
	RealName      string     `json:"real_name" db:"real_name"`
	Email         string     `json:"email" db:"email"`
	Phone         string     `json:"phone" db:"phone"`
	Avatar        string     `json:"avatar" db:"avatar"`
	Status        int        `json:"status" db:"status"` // 1=启用 0=禁用
	LastLoginAt   *time.Time `json:"last_login_at" db:"last_login_at"`
	LastLoginIP   string     `json:"last_login_ip" db:"last_login_ip"`
	OAuthProvider string     `json:"oauth_provider,omitempty" db:"oauth_provider"`
	OAuthID       string     `json:"oauth_id,omitempty" db:"oauth_id"`
	TenantID      uuid.UUID  `json:"tenant_id" db:"tenant_id"`
	Version       int        `json:"version" db:"version"`
	DeletedAt     *time.Time `json:"deleted_at,omitempty" db:"deleted_at"`
	CreatedAt     time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at" db:"updated_at"`
}
