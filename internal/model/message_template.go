// MessageTemplate 消息模板（S6 B6-3——06 号 §4）。
package model

import "time"

type MessageTemplate struct {
	ID             int64     `json:"id,string"`
	Code           string    `json:"code"`
	Name           string    `json:"name"`
	Channel        string    `json:"channel"`                   // uip（首版）
	BizType        string    `json:"biz_type"`                  // ticket（首版）
	Subject        string    `json:"subject"`                   // 主题模板（{{}} 占位符）
	Body           string    `json:"body"`                      // 正文模板
	VariableSchema []byte    `json:"variable_schema,omitempty"` // 可用变量声明（JSONB）
	Enabled        bool      `json:"enabled"`
	IsSystem       bool      `json:"is_system"` // 种子模板只读（B6-3）
	Version        int64     `json:"version"`
	CreatedBy      *int64    `json:"created_by,string,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type MessageTemplateListResponse struct {
	List     []*MessageTemplate `json:"list"`
	Total    int64              `json:"total"`
	Page     int                `json:"page"`
	PageSize int                `json:"page_size"`
}
