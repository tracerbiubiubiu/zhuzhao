package model

// TokenPair 双 Token 响应
type TokenPair struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"` // accessToken 有效期（秒）
}

// DeviceInfo 设备信息
type DeviceInfo struct {
	DeviceID      string `json:"device_id"`
	DeviceName    string `json:"device_name"`
	IP            string `json:"ip"`
	LastRefreshAt string `json:"last_refresh_at"`
	CreatedAt     string `json:"created_at"`
}

// LoginRequest 登录请求
type LoginRequest struct {
	Username  string `json:"username" binding:"required"`
	Password  string `json:"password" binding:"required"`
	DeviceID  string `json:"device_id" binding:"required"`
	DeviceName string `json:"device_name"`
}

// RefreshRequest 刷新 Token 请求
type RefreshRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}
