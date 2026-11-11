//go:build integration

package service_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/webhook"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service"
)

// P0-4 回归（2026-10-19 外审发现）：notification_configs.code 唯一冲突此前
// repo 层未映射 sentinel，service 判定恒假 → 重复创建落 500（ErrInternal）。
// 钉死全链映射：repo 23505 → ErrNotificationDupCode → 92002（handler 侧 409）。
func TestNotification_Create_DupCode_ReturnsConflict(t *testing.T) {
	ctx := context.Background()
	repo := repository.NewNotificationRepo(testPool)
	svc := service.NewNotificationService(repo, nil, slog.Default())

	code := "dup-" + uniqueSuffix()
	defer func() {
		_ = repo.DeleteByCode(ctx, code)
	}()

	uid := int64(1)
	in := &model.NotificationConfig{
		Code:       code,
		Name:       "重复 code 回归",
		Channel:    "webhook",
		WebhookURL: "https://example.com/hook",
		Enabled:    true,
		CreatedBy:  &uid,
	}
	require.NoError(t, svc.Create(ctx, in))

	err := svc.Create(ctx, in)
	assert.ErrorIs(t, err, errcode.ErrNotificationCodeExists, "重复 code 应返回专属冲突码而非 500")
	assert.NotErrorIs(t, err, errcode.ErrInternal)
	assert.Equal(t, errcode.ErrNotificationCodeExists.Code, 92002)
}

// NotifyDeadLetter 死信告警链 zhuzhao 侧（审计缺口批——本体零覆盖）：
// webhook 收包断言（envelope.event/data 键）+ 尽力而为语义（一个渠道失败不拒收）。
// SSRF 注记：webhook.Client 校验拦环回地址（出站黑名单），故用非 IP 假域名
// + 自定义 DialContext（NewWithClient 测试缝）直达 httptest 监听——校验面全真。
func TestNotification_DeadLetter_Webhook(t *testing.T) {
	ctx := context.Background()
	repo := repository.NewNotificationRepo(testPool)

	var mu sync.Mutex
	var bodies []string
	// SSRF 环回黑名单（6f7238e IP 规范化）拦 127.0.0.1——httptest 默认绑环回，
	// 改绑本机非环回地址（CI=host.docker.internal 不适用本测试；走本机网卡 IP）
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		bodies = append(bodies, string(b))
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	ln, lerr := net.Listen("tcp", "0.0.0.0:0")
	if lerr != nil {
		t.Fatalf("listen: %v", lerr)
	}
	srv.Listener = ln
	srv.Start()
	defer srv.Close()

	// 定向拨号：deadletter.test → httptest 监听；其余假域名 → 拒连端口（坏渠道快失败）
	hc := &http.Client{Transport: &http.Transport{DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, _, _ := net.SplitHostPort(addr)
		if host == "deadletter.test" {
			return net.Dial(network, srv.Listener.Addr().String())
		}
		return net.Dial(network, "127.0.0.1:1")
	}}}
	svc := service.NewNotificationService(repo, webhook.NewWithClient(hc), slog.Default())

	codeOK := "dl-ok-" + uniqueSuffix()
	codeBad := "dl-bad-" + uniqueSuffix()
	require.NoError(t, repo.Create(ctx, &model.NotificationConfig{
		Code: codeOK, Name: "死信测试OK", Channel: "webhook", WebhookURL: "http://deadletter.test/hook", Enabled: true,
	}))
	require.NoError(t, repo.Create(ctx, &model.NotificationConfig{
		Code: codeBad, Name: "死信测试BAD", Channel: "webhook", WebhookURL: "http://deadletter-bad.test/hook", Enabled: true,
	}))
	t.Cleanup(func() {
		_ = repo.DeleteByCode(ctx, codeOK)
		_ = repo.DeleteByCode(ctx, codeBad)
	})

	err := svc.NotifyDeadLetter(ctx, &model.DeadLetterNotifyPayload{
		TaskID: "dl-e2e-1", Action: "audit_archive", Error: "max retries exceeded", Attempts: 3,
	})
	require.NoError(t, err, "尽力而为：单渠道失败不拒收（受理即 2xx）")

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, bodies, 1, "健康 webhook 恰收一包（坏渠道跳过不重试）")
	var env map[string]any
	require.NoError(t, json.Unmarshal([]byte(bodies[0]), &env))
	assert.Equal(t, "dead_letter", env["event"])
	data, ok := env["data"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "dl-e2e-1", data["task_id"])
	assert.Equal(t, float64(3), data["attempts"], "成对恒出键不凭空消失")
}
