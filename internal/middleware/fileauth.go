package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// FileSessionCookieName 文件通道旁路凭证（A.11 拍板 #8——img 原生可用）。
const FileSessionCookieName = "file_session"

// SetFileSessionCookie 签发文件 cookie（login/refresh 成功响应调用）。
// 形态 B（B5-2-2 实施批确认制，偏离 A.11 原 HMAC uid+exp 方案并落档）：
// cookie 值=同款 access JWT——登出/密码修改/会话吊销经既有黑名单链
// **立即失效**（优于原案无状态形态的「保留至 exp」边界），且零新原语。
// 属性：Path 限定 /api/v1/files + HttpOnly + SameSite=Lax（A.11 规范）。
func SetFileSessionCookie(c *gin.Context, accessToken string, maxAgeSeconds int) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     FileSessionCookieName,
		Value:    accessToken,
		Path:     "/api/v1/files",
		MaxAge:   maxAgeSeconds,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

// ClearFileSessionCookie 过期清除（logout / 密码修改成功响应调用）。
func ClearFileSessionCookie(c *gin.Context) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name: FileSessionCookieName, Value: "", Path: "/api/v1/files",
		MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteLaxMode,
	})
}

// FileAuth files 路由认证（A.11）：Authorization header 优先——并存时
// cookie 静默忽略（hasMixedAuth 的 Bearer/AK 互斥语义不受影响）；无 header
// 时把 file_session cookie 桥接为 Bearer 走既有 JWT 链（过期/黑名单/禁用
// 全联动）。注入式接收 JWT 处理器（可测——桥接次序单测无需 Redis）。
func FileAuth(jwtHandler gin.HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.GetHeader("Authorization") == "" {
			if v, err := c.Cookie(FileSessionCookieName); err == nil && v != "" {
				c.Request.Header.Set("Authorization", "Bearer "+v)
			}
		}
		jwtHandler(c)
	}
}
