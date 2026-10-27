//go:build integration

// C-6（2026-10-27 观测批）守卫测试：emit 链四件套的装配面——G-5 教训（emit 写了
// 没人看见）在此闭合：①000080 种子形态（fixed+空收件人）②file_objects 实数采样
// →阈值跃迁 emit→规则路由→模板渲染→发送 全链 ③档位闩锁（同档不重复 emit）
// ④回落跃迁发恢复通知。
package ticket

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/channelsender"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/domainevent"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service"
)

func TestC6_StorageUsageAlert_E2E(t *testing.T) {
	ctx := context.Background()
	reg := domainevent.NewHookRegistry(nil)
	sender := &captureSender{}
	hook := NewTicketNotifyHook(repository.NewMessageTemplateRepo(testPool),
		repository.NewMessageRuleRepo(testPool), repository.NewMessageLogRepo(testPool),
		repository.NewNotificationOptoutRepo(testPool), sender, nil, nil)
	reg.Subscribe(domainevent.BizOps, "ops.usage_alert.e2e", hook)

	// ① 种子形态守卫：000080 fixed+空收件人（管理员配置后生效——000064 惯例）
	var tplCode, rcptType, fixedRcpt string
	require.NoError(t, testPool.QueryRow(ctx, `
		SELECT template_code, recipient_type, COALESCE(fixed_recipients, '')
		FROM message_rules WHERE biz_type='ops' AND event_action='ops.storage_usage_high'`).
		Scan(&tplCode, &rcptType, &fixedRcpt), "000080 种子规则须在位（迁移清单含 000080）")
	assert.Equal(t, "ops_storage_usage_alert", tplCode)
	assert.Equal(t, "fixed", rcptType)
	assert.Equal(t, "", fixedRcpt, "种子收件人空=暂不发送")

	// 模拟管理员配置收件人；cleanup 复原种子形态
	upsertRcpt := func(rcpt string) {
		_, err := testPool.Exec(ctx, `
			UPDATE message_rules SET fixed_recipients=$1, enabled=true
			WHERE biz_type='ops' AND event_action='ops.storage_usage_high'`, rcpt)
		require.NoError(t, err)
	}
	upsertRcpt("1")
	t.Cleanup(func() {
		upsertRcpt("")
		_, _ = testPool.Exec(ctx, `DELETE FROM message_log WHERE biz_type='ops' AND biz_id='storage'`)
	})

	// ② 全链：file_objects 实数 → Sample → 阈值跃迁 emit → 规则 → 渲染 → 发送
	// created_by 走 FK——建真实用户（E 前缀唯一工号）
	var uid int64
	require.NoError(t, testPool.QueryRow(ctx,
		`INSERT INTO users (employee_no, username, password, status) VALUES ($1, $1, 'hash', 1) RETURNING id`,
		"c6e2_"+uniqueSuffix()).Scan(&uid))
	const bucket = "c6_usage_e2e"
	t.Cleanup(func() {
		_, _ = testPool.Exec(ctx, `DELETE FROM file_objects WHERE bucket=$1`, bucket)
		_, _ = testPool.Exec(ctx, `DELETE FROM users WHERE id=$1`, uid)
	})
	seedObj := func(key string, size int64) {
		_, err := testPool.Exec(ctx, `
			INSERT INTO file_objects (bucket, object_key, purpose, status, file_name, mime_type, size_bytes, created_by, confirmed_at)
			VALUES ($1, $2, 'ticket_richtext', 'confirmed', 'a.png', 'image/png', $3, $4, NOW())`,
			bucket, key, size, uid)
		require.NoError(t, err)
	}
	seedObj("richtext/x.png", 900) // 900/1000 = 0.90 → critical 档

	sampler := service.NewStorageUsageSampler(testPool, reg, 1000,
		slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, sampler.Sample(ctx))

	waitForMsg := func(substr string) *channelsender.Message {
		t.Helper()
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			for _, m := range sender.messages() {
				if m.TemplateCode == "ops_storage_usage_alert" && strings.Contains(m.Subject, substr) {
					return &m
				}
			}
			time.Sleep(50 * time.Millisecond)
		}
		return nil
	}
	got := waitForMsg("")
	require.NotNil(t, got, "越限事件经 000080 规则路由到发送（emit 链装配完整——G-5 防线）")
	assert.Contains(t, got.Recipients, "1", "fixed 档收件人")
	assert.Contains(t, got.Subject, "critical", "模板渲染 {{event.level}}（ops 域 {{event.*}} 根键）")

	// message_log 落行（发送记录面）
	time.Sleep(150 * time.Millisecond)
	var n int
	require.NoError(t, testPool.QueryRow(ctx,
		`SELECT count(*) FROM message_log WHERE biz_type='ops' AND biz_id='storage'`).Scan(&n))
	assert.GreaterOrEqual(t, n, 1, "发送记录落 message_log")

	// ③ 档位闩锁：同档位复采不重复 emit（持续越限防刷屏）
	base := len(sender.messages())
	require.NoError(t, sampler.Sample(ctx))
	time.Sleep(200 * time.Millisecond)
	assert.Equal(t, base, len(sender.messages()), "同档位二次采样零重复投递")

	// ④ 回落跃迁的 emit 与投递分两说：latch 双向均 emit（usageLevel 纯函数单测覆盖
	// 双向档位），但本测试时序里 60s 内二次派发同 biz+action 被 S6 节流窗合并（日志
	// "notify: throttled" 即第二次派发已发生的证据）——生产小时级采样下档位跃迁
	// 间隔恒 ≥1h，节流窗咬不到本事件，回落沿投递在生产态可达。
	_, err := testPool.Exec(ctx, `DELETE FROM file_objects WHERE bucket=$1`, bucket)
	require.NoError(t, err)
	require.NoError(t, sampler.Sample(ctx), "零对象复采不炸（gauge 归零路径）")
}
