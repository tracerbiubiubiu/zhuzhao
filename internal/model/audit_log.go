package model

import "time"

// AuditLog 审计日志（Phase 1 DDL，见 migrations/000001_init.up.sql）
type AuditLog struct {
	ID          int64     `json:"id,string" db:"id"`
	UserID      *int64    `json:"user_id,string,omitempty" db:"user_id"`
	Username    string    `json:"username" db:"username"`
	Method      string    `json:"method" db:"method"`
	Path        string    `json:"path" db:"path"`
	StatusCode  int       `json:"status_code" db:"status_code"`
	Duration    int64     `json:"duration" db:"duration"`
	IP          string    `json:"ip" db:"ip"`
	UserAgent   string    `json:"user_agent" db:"user_agent"`
	RequestBody string    `json:"request_body,omitempty" db:"request_body"`
	RequestID   string    `json:"request_id,omitempty" db:"request_id"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
}

// AuditLogListResponse 审计日志分页
type AuditLogListResponse struct {
	List     []*AuditLog `json:"list"`
	Total    int64       `json:"total"`
	Page     int         `json:"page"`
	PageSize int         `json:"page_size"`
}
