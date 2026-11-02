package config

import (
	"os"
	"path/filepath"
	"testing"
)

// TestLoad_GatewayAlTargetEnvOverride 双拓扑免改文件治理（2026-11-02）：GATEWAY_AL_TARGET
// 非空覆盖 /al 上游 target、空回落基线——compose 服务名与宿主回环两形态随环境注入，
// 基线文件不再手工编辑（二次泄漏史：1d4d36c 曾把 127.0.0.1 本地态带入库）。
func TestLoad_GatewayAlTargetEnvOverride(t *testing.T) {
	writeCfg := func(t *testing.T) string {
		t.Helper()
		path := filepath.Join(t.TempDir(), "config.yaml")
		const yaml = `gateway:
  ak: test-ak
  sk: test-sk
  upstreams:
    - prefix: /al
      target: http://activelist:18081
      strip_prefix: true
`
		if err := os.WriteFile(path, []byte(yaml), 0o600); err != nil {
			t.Fatalf("write config: %v", err)
		}
		return path
	}
	findAl := func(cfg *Config) string {
		for _, up := range cfg.Gateway.Upstreams {
			if up.Prefix == "/al" {
				return up.Target
			}
		}
		return ""
	}
	t.Run("env 非空覆盖 /al target", func(t *testing.T) {
		t.Setenv("GATEWAY_AL_TARGET", "http://127.0.0.1:18081")
		cfg, err := Load(writeCfg(t))
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if got := findAl(cfg); got != "http://127.0.0.1:18081" {
			t.Fatalf("/al target = %q, want env 覆盖值 http://127.0.0.1:18081", got)
		}
	})
	t.Run("env 空回落基线", func(t *testing.T) {
		t.Setenv("GATEWAY_AL_TARGET", "")
		cfg, err := Load(writeCfg(t))
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if got := findAl(cfg); got != "http://activelist:18081" {
			t.Fatalf("/al target = %q, want 基线值 http://activelist:18081", got)
		}
	})
}
