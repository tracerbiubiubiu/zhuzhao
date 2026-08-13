package middleware

import (
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"

	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/jwt"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/response"
)

// JWT JWT 认证中间件
func JWT(jwtManager *jwt.Manager, rdb *redis.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 提取 token
		auth := c.GetHeader("Authorization")
		if auth == "" {
			response.Unauthorized(c, errcode.ErrUnauthorized.Message)
			c.Abort()
			return
		}

		parts := strings.SplitN(auth, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			response.Unauthorized(c, "无效的认证格式")
			c.Abort()
			return
		}

		tokenString := parts[1]

		// 解析 token
		claims, err := jwtManager.ParseAccessToken(tokenString)
		if err != nil {
			response.Unauthorized(c, errcode.ErrTokenInvalid.Message)
			c.Abort()
			return
		}

		// 检查 Redis 黑名单
		blacklistKey := fmt.Sprintf("blacklist:at:%s", claims.JTI)
		if exists, _ := rdb.Exists(c, blacklistKey).Result(); exists > 0 {
			response.Unauthorized(c, errcode.ErrTokenInvalid.Message)
			c.Abort()
			return
		}

		// 首次登录改密检查：只允许访问改密接口
		if claims.MustChangePassword && c.Request.URL.Path != "/api/v1/auth/password/update" {
			response.Forbidden(c, errcode.ErrPasswordChangeRequired.Message)
			c.Abort()
			return
		}

		// 注入上下文
		c.Set("userID", claims.UserID)
		c.Set("username", claims.Username)
		c.Set("jti", claims.JTI)
		c.Set("must_change_password", claims.MustChangePassword)

		c.Next()
	}
}
