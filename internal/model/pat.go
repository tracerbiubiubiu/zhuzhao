package model

import "time"

// PersonalAccessToken P4-6 PAT（GitHub 蓝本：明文 zpat_* 仅创建时返回一次、
// 落库 sha256；可吊销）。scope 首版恒 full（字段留位——细分触发驱动）。
type PersonalAccessToken struct {
	ID         int64      `json:"id,string"`
	UserID     int64      `json:"user_id,string"`
	Name       string     `json:"name"`
	SecretHash string     `json:"-"` // 永不出网
	Scope      string     `json:"scope"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
}
