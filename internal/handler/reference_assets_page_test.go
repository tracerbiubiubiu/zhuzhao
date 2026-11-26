package handler

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestReferenceAssetsPageServesSameOriginResources(t *testing.T) {
	engine := gin.New()
	engine.GET("/extensions/reference-assets", ReferenceAssetsPage)
	engine.GET("/extensions/reference-assets/app.js", ReferenceAssetsPageScript)
	engine.GET("/extensions/reference-assets/app.css", ReferenceAssetsPageStyles)

	for _, tc := range []struct {
		path, contentType, contains string
	}{
		{"/extensions/reference-assets", "text/html", "参考资产目录"},
		{"/extensions/reference-assets/app.js", "text/javascript", "/api/v1/extensions/reference-assets"},
		{"/extensions/reference-assets/app.css", "text/css", ".results"},
	} {
		t.Run(tc.contentType, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			res := httptest.NewRecorder()
			engine.ServeHTTP(res, req)
			body, err := io.ReadAll(res.Result().Body)
			if err != nil {
				t.Fatal(err)
			}
			if res.Code != http.StatusOK || !strings.HasPrefix(res.Header().Get("Content-Type"), tc.contentType) ||
				!strings.Contains(string(body), tc.contains) {
				t.Fatalf("status %d, content type %q, missing %q", res.Code, res.Header().Get("Content-Type"), tc.contains)
			}
			if res.Header().Get("Cache-Control") != "no-store" || res.Header().Get("X-Content-Type-Options") != "nosniff" {
				t.Fatalf("resource allows caching or content sniffing: %v", res.Header())
			}
			if tc.contentType == "text/html" {
				csp := res.Header().Get("Content-Security-Policy")
				for _, clause := range []string{"default-src 'none'", "script-src 'self'", "style-src 'self'", "connect-src 'self'", "form-action 'none'"} {
					if !strings.Contains(csp, clause) {
						t.Errorf("page CSP lacks %q: %q", clause, csp)
					}
				}
				if strings.Contains(csp, "'unsafe-inline'") || res.Header().Get("Referrer-Policy") != "no-referrer" {
					t.Errorf("page allows inline scripts or referrer: %v", res.Header())
				}
			}
		})
	}
}
