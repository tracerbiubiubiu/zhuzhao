package model

// TokenPair 双 Token 响应
type TokenPair struct {
	AccessToken       string `json:"access_token"`
	RefreshToken      string `json:"refresh_token"`
	ExpiresIn         int    `json:"expires_in"`           // accessToken 有效期（秒）
	MustChangePassword bool  `json:"must_change_password"` // 首次登录改密标记
}

// DeviceInfo 设备信息（Phase 2 设备管理预留——Phase 1 未使用；
// 随设备会话列表功能落地时启用）
type DeviceInfo struct {
	DeviceID      string `json:"device_id"`
	DeviceName    string `json:"device_name"`
	IP            string `json:"ip"`
	LastRefreshAt string `json:"last_refresh_at"`
	CreatedAt     string `json:"created_at"`
}

// LoginRequest 登录请求（账密登录用工号，不用 username）
type LoginRequest struct {
	EmployeeNo string `json:"employee_no" binding:"required"`
	Password   string `json:"password" binding:"required"`
	DeviceID   string `json:"device_id"` // 前端生成 UUID 存 localStorage，每次登录带上
	DeviceName string `json:"device_name"`
}

// RefreshRequest 刷新 Token 请求
type RefreshRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

// LogoutRequest 登出请求
type LogoutRequest struct {
	DeviceID string `json:"device_id"` // 与登录时一致；空则使用 default
}
