package model

import "time"

// NotificationConfig 通知渠道配置（P4-2 本体——webhook 优先，渠道接口配置化）。
// 死信告警等事件触发时按 enabled 配置逐个分发；version 乐观锁防管理面并发覆盖。
type NotificationConfig struct {
	ID         int64     `json:"id,string"`
	Code       string    `json:"code"`
	Name       string    `json:"name"`
	Channel    string    `json:"channel"` // 首版仅 webhook
	WebhookURL string    `json:"webhook_url"`
	Enabled    bool      `json:"enabled"`
	CreatedBy  *int64    `json:"created_by,string,omitempty"`
	Version    int64     `json:"version"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// NotificationConfigListResponse 配置分页（PageData 形态）
type NotificationConfigListResponse struct {
	List     []*NotificationConfig `json:"list"`
	Total    int64                 `json:"total"`
	Page     int                   `json:"page"`
	PageSize int                   `json:"page_size"`
}

// DeadLetterNotifyPayload 死信告警载荷（taskrunner 终败时经 /internal/notify/dead-letter 投递）
// 2026-10-14 契约审计 B3：RequestID/JobID/Attempts 去 omitempty，与投递方
// （taskrunner notify.Payload）成对恒出——Attempts=0/空 id 不得凭空消失
type DeadLetterNotifyPayload struct {
	TaskID    string `json:"task_id"`
	RequestID string `json:"request_id"`
	Action    string `json:"action"`
	JobID     string `json:"job_id"`
	Dept      string `json:"dept,omitempty"`
	Error     string `json:"error,omitempty"`
	Attempts  int    `json:"attempts"`
	FailedAt  string `json:"failed_at,omitempty"`
}
