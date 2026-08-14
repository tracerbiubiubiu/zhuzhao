package model

import "time"

// AuditLog 审计日志
type AuditLog struct {
	ID           int64     `json:"id,string" db:"id"`
	UserID       *int64    `json:"user_id,string,omitempty" db:"user_id"`
	Username     string    `json:"username" db:"username"`
	OrgID        *int64    `json:"org_id,string,omitempty" db:"org_id"`
	OrgPath      string    `json:"org_path" db:"org_path"`
	Method       string    `json:"method" db:"method"`
	Path         string    `json:"path" db:"path"`
	Action       string    `json:"action" db:"action"`
	ResourceType string    `json:"resource_type" db:"resource_type"`
	ResourceID   *int64    `json:"resource_id,string,omitempty" db:"resource_id"`
	RequestBody  []byte    `json:"request_body,omitempty" db:"request_body"`
	ResponseCode int       `json:"response_code" db:"response_code"`
	IP           string    `json:"ip" db:"ip"`
	UserAgent    string    `json:"user_agent" db:"user_agent"`
	LatencyMs    int       `json:"latency_ms" db:"latency_ms"`
	Status       int       `json:"status" db:"status"` // 1=成功 0=失败
	ErrorMsg     string    `json:"error_msg,omitempty" db:"error_msg"`
	TenantID     int64     `json:"tenant_id,string" db:"tenant_id"`
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
}
