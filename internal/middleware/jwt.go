package middleware

import (
	"fmt"
	"net/http"
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
		if hasMixedAuth(c) {
			response.Error(c, http.StatusBadRequest, errcode.ErrMultipleAuthMethods)
			c.Abort()
			return
		}

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

		claims, err := jwtManager.ParseAccessToken(tokenString)
		if err != nil {
			response.UnauthorizedError(c, errcode.ErrTokenInvalid)
			c.Abort()
			return
		}

		blacklistKey := fmt.Sprintf("blacklist:at:%s", claims.JTI)
		exists, err := rdb.Exists(c, blacklistKey).Result()
		if err != nil {
			response.ServiceUnavailable(c)
			c.Abort()
			return
		}
		if exists > 0 {
			response.UnauthorizedError(c, errcode.ErrTokenInvalid)
			c.Abort()
			return
		}

		disabledKey := fmt.Sprintf("user:disabled:%d", claims.UserID)
		disabled, err := rdb.Exists(c, disabledKey).Result()
		if err != nil {
			response.ServiceUnavailable(c)
			c.Abort()
			return
		}
		if disabled > 0 {
			response.ForbiddenError(c, errcode.ErrUserDisabled)
			c.Abort()
			return
		}

		if claims.MustChangePassword && c.Request.URL.Path != "/api/v1/auth/password/update" {
			response.ForbiddenError(c, errcode.ErrPasswordChangeRequired)
			c.Abort()
			return
		}

		c.Set("userID", claims.UserID)
		c.Set("username", claims.Username)
		c.Set("jti", claims.JTI)
		c.Set("must_change_password", claims.MustChangePassword)

		c.Next()
	}
}

func hasMixedAuth(c *gin.Context) bool {
	auth := c.GetHeader("Authorization")
	if auth == "" || !strings.HasPrefix(auth, "Bearer ") {
		return false
	}
	return c.GetHeader("X-AK-Access-Key") != ""
}
