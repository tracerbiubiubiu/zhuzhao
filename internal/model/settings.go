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
	Version   int64           `json:"version"` // F-D2 乐观锁（000050）
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
	Version       int64           `json:"version,omitempty"` // kv 行当前版本（回传 expected_version 用）
}

// UpdateSettingRequest POST /system/settings/update
type UpdateSettingRequest struct {
	Key   string          `json:"key" binding:"required"`
	Value json.RawMessage `json:"value" binding:"required"`
	// ExpectedVersion F-D2 乐观锁（可选）：传 List 返回的 version——并发他人
	// 已改过即 400 提示刷新；不传=服务端重读重试（兼容存量调用，last-write-wins）。
	ExpectedVersion *int64 `json:"expected_version,omitempty"`
}
