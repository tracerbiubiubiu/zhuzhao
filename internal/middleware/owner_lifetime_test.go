package middleware

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/singleton"
)

type lifetimeSink struct{ errs chan error }

func (s lifetimeSink) Insert(ctx context.Context, _ AuditLogEntry) error {
	s.errs <- ctx.Err()
	return nil
}

func (s lifetimeSink) Upsert(ctx context.Context, _, _, _, _ string) error {
	s.errs <- ctx.Err()
	return nil
}

func (s lifetimeSink) FindActiveByHash(context.Context, string) (int64, string, int64, error) {
	return 1, "operator", 2, nil
}

func (s lifetimeSink) TouchLastUsed(ctx context.Context, _ int64) {
	s.errs <- ctx.Err()
}

func TestDetachedMiddlewareWritesRespectOwnerLifetime(t *testing.T) {
	for _, middleware := range []struct {
		name    string
		install func(lifetimeSink) gin.HandlerFunc
		handle  gin.HandlerFunc
	}{
		{"operation audit", func(s lifetimeSink) gin.HandlerFunc { return AuditLog(s) }, func(c *gin.Context) {
			c.Set("userID", int64(1))
			c.Status(http.StatusNoContent)
		}},
		{"panic aggregation", func(s lifetimeSink) gin.HandlerFunc {
			return Recovery(slog.New(slog.NewTextHandler(io.Discard, nil)), s)
		}, func(*gin.Context) { panic("owner lifecycle test") }},
		{"PAT last used", func(s lifetimeSink) gin.HandlerFunc { return JWT(nil, nil, s) }, func(c *gin.Context) {
			c.Status(http.StatusNoContent)
		}},
	} {
		t.Run(middleware.name, func(t *testing.T) {
			for _, scenario := range []struct {
				name      string
				loseOwner bool
				want      error
			}{
				{"client disconnected", false, nil},
				{"owner lost", true, context.Canceled},
			} {
				t.Run(scenario.name, func(t *testing.T) {
					owner, loseOwner := context.WithCancel(context.Background())
					defer loseOwner()
					request, disconnect := context.WithCancel(singleton.WithOwner(context.Background(), owner))
					disconnect()
					if scenario.loseOwner {
						loseOwner()
					}

					sink := lifetimeSink{errs: make(chan error, 1)}
					router := gin.New()
					router.Use(middleware.install(sink))
					router.POST("/test", middleware.handle)
					req := httptest.NewRequest(http.MethodPost, "/test", nil).WithContext(request)
					req.Header.Set("Authorization", "Bearer zpat_fixture")
					router.ServeHTTP(httptest.NewRecorder(), req)
					select {
					case err := <-sink.errs:
						if err != scenario.want {
							t.Fatalf("write context error = %v, want %v", err, scenario.want)
						}
					case <-time.After(time.Second):
						t.Fatal("write did not reach the sink")
					}
				})
			}
		})
	}
}
