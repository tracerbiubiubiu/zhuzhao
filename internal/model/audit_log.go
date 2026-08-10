package model

import (
	"time"

	"github.com/google/uuid"
)

// AuditLog 审计日志
type AuditLog struct {
	ID           uuid.UUID  `json:"id" db:"id"`
	UserID       *uuid.UUID `json:"user_id" db:"user_id"`
	Username     string     `json:"username" db:"username"`
	OrgID        *uuid.UUID `json:"org_id" db:"org_id"`
	OrgPath      string     `json:"org_path" db:"org_path"`
	Method       string     `json:"method" db:"method"`
	Path         string     `json:"path" db:"path"`
	Action       string     `json:"action" db:"action"`
	ResourceType string     `json:"resource_type" db:"resource_type"`
	ResourceID   *uuid.UUID `json:"resource_id" db:"resource_id"`
	RequestBody  []byte     `json:"request_body,omitempty" db:"request_body"`
	ResponseCode int        `json:"response_code" db:"response_code"`
	IP           string     `json:"ip" db:"ip"`
	UserAgent    string     `json:"user_agent" db:"user_agent"`
	LatencyMs    int        `json:"latency_ms" db:"latency_ms"`
	Status       int        `json:"status" db:"status"` // 1=成功 0=失败
	ErrorMsg     string     `json:"error_msg,omitempty" db:"error_msg"`
	TenantID     *uuid.UUID `json:"tenant_id" db:"tenant_id"`
	CreatedAt    time.Time  `json:"created_at" db:"created_at"`
}
