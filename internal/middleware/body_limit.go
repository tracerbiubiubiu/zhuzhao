package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// BodyLimit 请求体大小限制中间件
func BodyLimit(limit int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, limit)
		c.Next()
	}
}
