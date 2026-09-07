//go:build integration

// E-④ 集成验证：任务提交/触发经（模拟）taskrunner 后，job_submissions 提交凭证
// 落档（E5：origin=api，request_id 从 ctx 取——与 taskrunner job_runs 跨查锚点）。
package service_test

import (
	"context"
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
		c.JSON(200, gin.H{"code": 0, "message": "success", "data": gin.H{"task_id": "t-e4-2", "accepted": true}})
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
	}, "10001", "10.0.0.9", "http://self:33333")
	require.NoError(t, err)
	require.Equal(t, "t-e4-1", resp.TaskID)
	require.Contains(t, gotBody, `"action":"audit_archive"`)
	require.Contains(t, gotBody, `"submitted_by":"10001"`)

	var action, requestID, origin, status string
	require.NoError(t, testPool.QueryRow(context.Background(),
		`SELECT action, COALESCE(request_id,''), origin, status FROM job_submissions WHERE task_id='t-e4-1'`).
		Scan(&action, &requestID, &origin, &status))
	require.Equal(t, "audit_archive", action)
	require.Equal(t, "req-e4-voucher", requestID, "提交凭证的 request_id 取入站 ctx（跨查锚点）")
	require.Equal(t, "api", origin)
	require.Equal(t, "submitted", status)

	// 触发：凭证落档（action=trigger:<job_id>）
	resp2, err := svc.Trigger(ctx, "job-42", "10001", "10.0.0.9")
	require.NoError(t, err)
	require.Equal(t, "t-e4-2", resp2.TaskID)
	var tAction string
	require.NoError(t, testPool.QueryRow(context.Background(),
		`SELECT action FROM job_submissions WHERE task_id='t-e4-2'`).Scan(&tAction))
	require.Equal(t, "trigger:job-42", tAction)
}
