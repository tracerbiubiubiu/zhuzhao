package middleware

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"

	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao-utils/jwt"
	"github.com/tracerbiubiubiu/zhuzhao-utils/response"
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
			// B4-1：仅带 X-AK-* 无 Bearer——M2M 未上线，明确告知（20009 待 Phase 3 落地）
			if hasAKHeaders(c) {
				response.Unauthorized(c, "暂不支持该认证方式")
				c.Abort()
				return
			}
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
			// B2-1：过期与无效区分——过期 → 20002（客户端可静默 refresh），
			// 签名错/typ 混淆/黑名单等 → 20003（需跳登录页）
			if errors.Is(err, jwt.ErrTokenExpired) {
				response.UnauthorizedError(c, errcode.ErrTokenExpired)
			} else {
				response.UnauthorizedError(c, errcode.ErrTokenInvalid)
			}
			c.Abort()
			return
		}

		// B4-1：黑名单 + disabled 两键 pipeline 一次往返（原两次串行 EXISTS）
		blacklistKey := fmt.Sprintf("blacklist:at:%s", claims.JTI)
		disabledKey := fmt.Sprintf("user:disabled:%d", claims.UserID)
		pipe := rdb.Pipeline()
		blacklistCmd := pipe.Exists(c, blacklistKey)
		disabledCmd := pipe.Exists(c, disabledKey)
		if _, err := pipe.Exec(c); err != nil {
			response.ServiceUnavailable(c)
			c.Abort()
			return
		}
		if exists, err := blacklistCmd.Result(); err != nil {
			response.ServiceUnavailable(c)
			c.Abort()
			return
		} else if exists > 0 {
			response.UnauthorizedError(c, errcode.ErrTokenInvalid)
			c.Abort()
			return
		}
		if disabled, err := disabledCmd.Result(); err != nil {
			response.ServiceUnavailable(c)
			c.Abort()
			return
		} else if disabled > 0 {
			response.ForbiddenError(c, errcode.ErrUserDisabled)
			c.Abort()
			return
		}

		// B4-1：FullPath 用路由模板比对（原硬编码 URL 字符串与注册解耦，
		// 前缀调整/trailing slash 时静默失效）
		if claims.MustChangePassword && c.FullPath() != "/api/v1/auth/password/update" {
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
	return hasAKHeaders(c)
}

// hasAKHeaders 是否携带 AK/SK 请求头（M2M 认证，Phase 3 上线）
func hasAKHeaders(c *gin.Context) bool {
	return c.GetHeader("X-AK-Access-Key") != ""
}
