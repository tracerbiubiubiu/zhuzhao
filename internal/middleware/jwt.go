package middleware

import (
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

		// TODO: 检查 Redis 黑名单

		// 注入上下文
		c.Set("userID", claims.UserID)
		c.Set("username", claims.Username)
		c.Set("role", claims.Role)
		c.Set("orgID", claims.OrgID)
		c.Set("deviceID", claims.DeviceID)

		c.Next()
	}
}
