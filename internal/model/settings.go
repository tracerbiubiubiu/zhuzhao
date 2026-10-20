package model

import (
	"encoding/json"
	"time"
)

// SystemSetting KV 设置行（000048——存储限制为首个消费者）。
// min_editor_tier 在代码 catalog（不进 DB——语义进代码纪律）。
type SystemSetting struct {
	Key       string          `json:"key"`
	Value     json.RawMessage `json:"value"`
	UpdatedBy int64           `json:"updated_by,string"`
	UpdatedAt time.Time       `json:"updated_at"`
}

// SettingView 管理面列表项（B5-6 A.4 #6——含来源与编辑层级，前端渲染需要）。
type SettingView struct {
	Key           string          `json:"key"`
	Value         json.RawMessage `json:"value"`
	Source        string          `json:"source"`          // config=出厂默认 | kv=运行时覆盖
	MinEditorTier string          `json:"min_editor_tier"` // superadmin | admin
	Description   string          `json:"description"`
	UpdatedBy     int64           `json:"updated_by,string,omitempty"`
	UpdatedAt     *time.Time      `json:"updated_at,omitempty"`
}

// UpdateSettingRequest POST /system/settings/update
type UpdateSettingRequest struct {
	Key   string          `json:"key" binding:"required"`
	Value json.RawMessage `json:"value" binding:"required"`
}
