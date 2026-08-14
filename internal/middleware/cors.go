package middleware

import (
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

// CORS 跨域中间件。
// Phase 1：gin-contrib DefaultConfig + AllowAllOrigins（全 Origin 放开，不做白名单）；上线前再收紧。
func CORS() gin.HandlerFunc {
	config := cors.DefaultConfig()
	config.AllowAllOrigins = true
	config.AllowHeaders = append(config.AllowHeaders, "Authorization", "X-Request-Id")
	return cors.New(config)
}
