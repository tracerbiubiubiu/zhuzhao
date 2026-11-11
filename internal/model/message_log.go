// MessageLog 发送记录 + MessageRule 订阅规则（S6 B6-6——06 号 §5.4/A.5）。
package model

import "time"

type MessageLog struct {
	ID           int64     `json:"id,string"`
	TemplateCode string    `json:"template_code"`
	BizType      string    `json:"biz_type"`
	BizID        string    `json:"biz_id"`
	Channel      string    `json:"channel"`
	Recipients   string    `json:"recipients"` // 逗号分隔
	Subject      string    `json:"subject"`
	Body         string    `json:"body"`
	Status       string    `json:"status"` // sent / failed
	Error        string    `json:"error,omitempty"`
	ActorID      *int64    `json:"actor_id,string,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

type MessageLogListResponse struct {
	List     []*MessageLog `json:"list"`
	Total    int64         `json:"total"`
	Page     int           `json:"page"`
	PageSize int           `json:"page_size"`
}

// MessageRule 订阅规则（06 号 A.5——event_action→template_code 路由）。
type MessageRule struct {
	ID              int64     `json:"id,string"`
	BizType         string    `json:"biz_type"`
	EventAction     string    `json:"event_action"` // 如 ticket.assigned
	TemplateCode    string    `json:"template_code"`
	RecipientType   string    `json:"recipient_type"` // assignee / creator / fixed
	FixedRecipients string    `json:"fixed_recipients,omitempty"`
	Enabled         bool      `json:"enabled"`
	Version         int64     `json:"version"` // 乐观锁（000060——upsert 携陈旧 version → 92506/409）
	CreatedBy       *int64    `json:"created_by,string,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}
