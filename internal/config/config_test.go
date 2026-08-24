package config

import (
	"strings"
	"testing"
)

// TestDatabaseConfig_DSN B1-3：密码含 URI 保留字符时 DSN 仍可被正确解析
func TestDatabaseConfig_DSN(t *testing.T) {
	tests := []struct {
		name          string
		password      string
		wantContained string // DSN 中应包含的转义后密码片段
	}{
		{"plain", "simple123", "simple123"},
		{"at_sign", "p@ss", "p%40ss"},
		{"colon_slash", "p:ss/w?rd#x", "p%3Ass%2Fw%3Frd%23x"},
		{"percent", "100%pass", "100%25pass"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := DatabaseConfig{
				Host: "localhost", Port: 5432,
				User: "zhuzhao", Password: tt.password,
				DBName: "zhuzhao_db", SSLMode: "disable",
			}
			dsn := c.DSN()
			if !strings.Contains(dsn, tt.wantContained) {
				t.Fatalf("DSN() = %q, want contained %q", dsn, tt.wantContained)
			}
			// 密码原文的保留字符不应裸露在 @host 之前（避免解析歧义）
			if tt.password != tt.wantContained && strings.Contains(dsn, tt.password+"@") {
				t.Fatalf("DSN() = %q leaks unescaped password", dsn)
			}
			// 结构骨架：scheme://user:pass@host:port/db?sslmode=...
			for _, part := range []string{"postgres://", "zhuzhao:", "@localhost:5432/zhuzhao_db?sslmode=disable"} {
				if !strings.Contains(dsn, part) {
					t.Fatalf("DSN() = %q, missing %q", dsn, part)
				}
			}
		})
	}
}
