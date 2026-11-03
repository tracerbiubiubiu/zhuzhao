//go:build integration

// E-④ 集成验证：任务提交/触发经（模拟）taskrunner 后，job_submissions 提交凭证
// 落档（E5：origin=api，request_id 从 ctx 取——与 taskrunner job_runs 跨查锚点）。
package service_test

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/reqid"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/taskrunner"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service"
)

func TestTaskrunnerServiceSubmitRecordsVoucher(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var gotBody string
	fake := gin.New()
	fake.POST("/v1/tasks", func(c *gin.Context) {
		buf := make([]byte, c.Request.ContentLength)
		_, _ = c.Request.Body.Read(buf)
		gotBody = string(buf)
		c.JSON(200, gin.H{"code": 0, "message": "success", "data": gin.H{"task_id": "t-e4-1", "accepted": true}})
	})
	fake.POST("/v1/jobs/trigger", func(c *gin.Context) {
		// 2026-11-03 trigger 回调修复：上游 SubmitOutput 回带定义真实 action/params 快照
		c.JSON(200, gin.H{"code": 0, "message": "success", "data": gin.H{
			"task_id": "t-e4-2", "accepted": true, "action": "audit_archive", "params": gin.H{"retention_days": 90}}})
	})
	srv := httptest.NewServer(fake)
	defer srv.Close()

	client := taskrunner.New(taskrunner.Config{BaseURL: srv.URL, AK: "zhuzhao", SK: []byte("sk")})
	subs := repository.NewJobSubmissionRepo(testPool)
	svc := service.NewTaskrunnerService(client, subs)

	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM job_submissions WHERE task_id IN ('t-e4-1','t-e4-2')`)
	})

	ctx := reqid.With(context.Background(), "req-e4-voucher")

	// 提交：凭证落档（action + task_id + request_id）
	resp, err := svc.Submit(ctx, &service.TaskSubmitInput{
		Action: "audit_archive",
		Params: json.RawMessage(`{"retention_days":90}`),
	}, "10001", "10.0.0.9", "http://self:33333")
	require.NoError(t, err)
	require.Equal(t, "t-e4-1", resp.TaskID)
	require.Contains(t, gotBody, `"action":"audit_archive"`)
	require.Contains(t, gotBody, `"submitted_by":"10001"`)
	// W0b 四口同收（6aa7d52）：taskrunner 拒收出站 callback_url（非空 400）——zhuzhao
	// 出站不再拼接传递（回调目标一律 taskrunner 配置 TASKRUNNER_CALLBACK_TARGET_URL）。
	// 出站带该字段=两仓契约断裂（提交恒 400）——断言方向随 6aa7d52 反转，防回潮
	require.NotContains(t, gotBody, `callback_url`)

	var action, requestID, origin, status string
	require.NoError(t, testPool.QueryRow(context.Background(),
		`SELECT action, COALESCE(request_id,''), origin, status FROM job_submissions WHERE task_id='t-e4-1'`).
		Scan(&action, &requestID, &origin, &status))
	require.Equal(t, "audit_archive", action)
	require.Equal(t, "req-e4-voucher", requestID, "提交凭证的 request_id 取入站 ctx（跨查锚点）")
	require.Equal(t, "api", origin)
	require.Equal(t, "submitted", status)

	// C11/可观测：job_submissions.params 快照（提交入参定格——audit_archive 的调用参数）
	var params string
	require.NoError(t, testPool.QueryRow(context.Background(),
		`SELECT params FROM job_submissions WHERE task_id='t-e4-1'`).Scan(&params))
	require.Contains(t, params, "retention_days")

	// 触发：凭证落档（action/params=上游回带的定义真实值——2026-11-03 trigger 回调修复：
	// 旧合成键 "trigger:<job_id>" 过不了回调快照一致性校验（jobs_callback W0b），
	// 手动触发恒 409 失真；记账同源后回调方可落 succeeded 终态）
	resp2, err := svc.Trigger(ctx, "job-42", "10001", "10.0.0.9")
	require.NoError(t, err)
	require.Equal(t, "t-e4-2", resp2.TaskID)
	var tAction, tParams string
	require.NoError(t, testPool.QueryRow(context.Background(),
		`SELECT action, params FROM job_submissions WHERE task_id='t-e4-2'`).Scan(&tAction, &tParams))
	require.Equal(t, "audit_archive", tAction)
	require.Contains(t, tParams, "retention_days")
}

// 旧上游兼容（2026-11-03 trigger 回调修复）：上游未回带 action/params（零值）时
// 记账回退合成键 "trigger:<job_id>"——行为同修复前，混部窗口不为更坏。
func TestTaskrunnerServiceTriggerVoucherFallbackLegacyUpstream(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fake := gin.New()
	fake.POST("/v1/jobs/trigger", func(c *gin.Context) {
		c.JSON(200, gin.H{"code": 0, "message": "success", "data": gin.H{"task_id": "t-e4-legacy", "accepted": true}})
	})
	srv := httptest.NewServer(fake)
	defer srv.Close()

	client := taskrunner.New(taskrunner.Config{BaseURL: srv.URL, AK: "zhuzhao", SK: []byte("sk")})
	subs := repository.NewJobSubmissionRepo(testPool)
	svc := service.NewTaskrunnerService(client, subs)

	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM job_submissions WHERE task_id='t-e4-legacy'`)
	})

	ctx := reqid.With(context.Background(), "req-e4-legacy")
	_, err := svc.Trigger(ctx, "job-legacy", "10001", "10.0.0.9")
	require.NoError(t, err)
	var tAction string
	require.NoError(t, testPool.QueryRow(context.Background(),
		`SELECT action FROM job_submissions WHERE task_id='t-e4-legacy'`).Scan(&tAction))
	require.Equal(t, "trigger:job-legacy", tAction)
}
