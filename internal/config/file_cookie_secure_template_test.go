package config_test

import (
	"bytes"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"

	"github.com/tracerbiubiubiu/zhuzhao/internal/config"
	"github.com/tracerbiubiubiu/zhuzhao/internal/middleware"
)

// TestLoad_FileCookieSecureTemplates 防止实际模板把字段误挂 audit：默认 false
// 会掩盖错位，必须只改模板布尔值就能经 Load 到达真实 StorageConfig 和 cookie。
func TestLoad_FileCookieSecureTemplates(t *testing.T) {
	// Load 使用全局 Viper，禁止并行；空 env 不参与覆盖，隔离宿主进程配置。
	for _, key := range []string{
		"STORAGE_FILE_COOKIE_SECURE", "APP_STORAGE_FILE_COOKIE_SECURE",
		"STORAGE_ENABLED", "APP_STORAGE_ENABLED",
	} {
		t.Setenv(key, "")
	}
	t.Cleanup(viper.Reset)

	for _, template := range []struct {
		name           string
		path           string
		storageEnabled bool
	}{
		{"baseline", filepath.Join("..", "..", "configs", "config.yaml"), true},
		{"test_stack", filepath.Join("..", "..", "deployments", "config.test.yaml"), false},
	} {
		t.Run(template.name, func(t *testing.T) {
			data, err := os.ReadFile(template.path)
			if err != nil {
				t.Fatal(err)
			}
			const setting = "  file_cookie_secure: false"
			if bytes.Count(data, []byte(setting)) != 1 {
				t.Fatal("template must declare file_cookie_secure: false exactly once")
			}
			enabled := bytes.Replace(data, []byte(setting), []byte("  file_cookie_secure: true"), 1)
			enabledPath := filepath.Join(t.TempDir(), "config.yaml")
			if err := os.WriteFile(enabledPath, enabled, 0o600); err != nil {
				t.Fatal(err)
			}

			for _, test := range []struct {
				name string
				path string
				env  string
				want bool
			}{
				{"default_false", template.path, "", false},
				{"yaml_true", enabledPath, "", true},
				{"env_true", template.path, "true", true},
				{"env_false_overrides_yaml_true", enabledPath, "false", false},
			} {
				t.Run(test.name, func(t *testing.T) {
					viper.Reset()
					t.Cleanup(viper.Reset)
					t.Setenv("STORAGE_FILE_COOKIE_SECURE", test.env)
					cfg, err := config.Load(test.path)
					if err != nil {
						t.Fatalf("Load: %v", err)
					}
					if got := cfg.Storage.FileCookieSecure; got != test.want {
						t.Fatalf("Storage.FileCookieSecure = %t, want %t", got, test.want)
					}
					if cfg.Storage.Enabled != template.storageEnabled {
						t.Fatal("cookie configuration must preserve the template's storage enablement")
					}
					response := httptest.NewRecorder()
					ctx, _ := gin.CreateTestContext(response)
					middleware.SetFileSessionCookie(ctx, "test-access-token", 60, cfg.Storage.FileCookieSecure)
					result := response.Result()
					defer result.Body.Close()
					cookies := result.Cookies()
					if len(cookies) != 1 || cookies[0].Name != middleware.FileSessionCookieName {
						t.Fatal("expected exactly one file-session cookie")
					}
					if cookies[0].Secure != test.want {
						t.Fatalf("file-session cookie Secure = %t, want %t", cookies[0].Secure, test.want)
					}
				})
			}
		})
	}
}
