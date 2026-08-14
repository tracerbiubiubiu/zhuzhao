package model

import (
	"time"
)

// User 用户模型
type User struct {
	ID                 int64      `json:"id,string" db:"id"`
    Username           string     `json:"username" db:"username"`
    EmployeeNo         string     `json:"employee_no" db:"employee_no"`
    DomainAccount      string     `json:"domain_account" db:"domain_account"`
    UserDomain         string     `json:"user_domain" db:"user_domain"`
    Password           string     `json:"-" db:"password"`
	RealName           string     `json:"real_name" db:"real_name"`
	Email              string     `json:"email" db:"email"`
	Phone              string     `json:"phone" db:"phone"`
	Avatar             string     `json:"avatar" db:"avatar"`
	Status             int        `json:"status" db:"status"` // 1=启用 0=禁用
	MustChangePassword bool       `json:"must_change_password" db:"must_change_password"`
	LastLoginAt        *time.Time `json:"last_login_at" db:"last_login_at"`
	LastLoginIP        string     `json:"last_login_ip" db:"last_login_ip"`
	IsSystem           bool       `json:"is_system" db:"is_system"`
	TenantID           int64      `json:"tenant_id,string" db:"tenant_id"`
	Version            int        `json:"version" db:"version"`
	DeletedAt          *time.Time `json:"deleted_at,omitempty" db:"deleted_at"`
	CreatedAt          time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at" db:"updated_at"`
}
