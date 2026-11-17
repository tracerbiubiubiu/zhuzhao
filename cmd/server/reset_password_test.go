package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// The subprocess calls the actual main command dispatch, without a database.
// A short synthetic password must fail before connection or account mutation.
func TestResetPasswordMaintenanceCLIProcess(t *testing.T) {
	mode := os.Getenv("ZZ_MAINTENANCE_CLI_HELPER")
	if mode == "" {
		return
	}
	os.Args = []string{"zhuzhao"}
	if mode == "reset" {
		os.Args = append(os.Args, "reset-password", "--username", "synthetic-break-glass-probe", "--password-stdin", "--yes")
	}
	main()
}

func TestResetPasswordMaintenanceRepair_CommandAndServerBoundaries(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(dir, "configs"), 0700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "configs/config.yaml"), []byte(`server:
  mode: debug
  port: 60820
database:
  host: 127.0.0.1
  port: 1
  dbname: synthetic_unreachable
redis:
  host: 127.0.0.1
  port: 1
jwt:
  secret: synthetic-cli-maintenance-boundary-secret-123456
  access_ttl: 15m
  refresh_ttl: 24h
`), 0600))
	for _, raw := range []string{"admin", "42,", "42,42"} {
		for _, mode := range []string{"reset", "server"} {
			t.Run(mode+"/"+raw, func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestResetPasswordMaintenanceCLIProcess$")
				cmd.Dir = dir
				for _, env := range os.Environ() {
					if !strings.HasPrefix(env, "TASK_MAINTAINER_USER_IDS=") && !strings.HasPrefix(env, "ZZ_MAINTENANCE_CLI_HELPER=") {
						cmd.Env = append(cmd.Env, env)
					}
				}
				cmd.Env = append(cmd.Env, "TASK_MAINTAINER_USER_IDS="+raw, "ZZ_MAINTENANCE_CLI_HELPER="+mode)
				cmd.Stdin = strings.NewReader("short\n")
				out, err := cmd.CombinedOutput()
				require.Error(t, err)
				require.NoError(t, ctx.Err(), string(out))
				var exit *exec.ExitError
				require.ErrorAs(t, err, &exit)
				require.Equal(t, 1, exit.ExitCode(), string(out))
				if mode == "reset" {
					require.Contains(t, string(out), "reset-password: 新密码长度须 ≥8", "the actual emergency command must pass unrelated maintenance config and reach its own password validation")
					require.NotContains(t, string(out), "加载配置失败")
				} else {
					require.Contains(t, string(out), "invalid config: TASK_MAINTAINER_USER_IDS", "the ordinary server must reject the same invalid startup grant")
				}
			})
		}
	}
}
