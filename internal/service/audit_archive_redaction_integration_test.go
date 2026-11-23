//go:build integration

package service_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/config"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service"
)

func TestAuditArchive_RiskPayloadRedaction(t *testing.T) {
	ctx := context.Background()
	old := time.Now().UTC().Add(-200 * 24 * time.Hour).Truncate(time.Microsecond)
	var oldIDs []int64
	for _, path := range []string{"/api/v1/risk-items", "/api/v1/tickets"} {
		var id int64
		require.NoError(t, testPool.QueryRow(ctx, `INSERT INTO audit_logs
			(username,employee_no,real_name,method,path,status_code,duration,ip,user_agent,request_id,request_body,response_summary,created_at)
			VALUES ('synthetic_actor','S000007','Synthetic Actor','POST',$1,403,123,'192.0.2.1','synthetic-agent','req-synthetic','synthetic request','synthetic response',$2) RETURNING id`, path, old).Scan(&id))
		oldIDs = append(oldIDs, id)
	}
	var freshID, policyID int64
	t.Cleanup(func() {
		_, err := testPool.Exec(ctx, `DELETE FROM audit_logs WHERE id=ANY($1)`, append(oldIDs, freshID))
		require.NoError(t, err)
		_, err = testPool.Exec(ctx, `DELETE FROM policy_evaluation_logs WHERE id=$1`, policyID)
		require.NoError(t, err)
	})
	require.NoError(t, testPool.QueryRow(ctx, `INSERT INTO audit_logs (username,method,path,status_code,duration,request_body,response_summary)
		VALUES ('synthetic_actor','POST','/api/v1/risk-items',200,1,'fresh request','fresh response') RETURNING id`).Scan(&freshID))
	require.NoError(t, testPool.QueryRow(ctx, `INSERT INTO policy_evaluation_logs
		(actor_id,resource_type,resource_id,action,result,reason,trace_id,created_at)
		VALUES (7,'risk','synthetic-risk','read',false,'synthetic reason','req-synthetic',$1) RETURNING id`, old).Scan(&policyID))

	dir := t.TempDir()
	job, err := service.NewFixedAuditArchiveJob(repository.NewAuditLogRepo(testPool), config.ArchiveConfig{
		CronEnabled: true, CronTick: "30s", RetentionDays: 180, BatchRows: 2, OutDir: dir, FileRetentionDays: 395,
	}, nil)
	require.NoError(t, err)
	require.NoError(t, job.Handle(ctx, nil))
	lines := readTableArchive(t, dir, "audit_logs")
	require.Len(t, lines, 2)
	for i, line := range lines {
		var got map[string]json.RawMessage
		require.NoError(t, json.Unmarshal([]byte(line), &got))
		var id int64
		require.NoError(t, json.Unmarshal(got["id"], &id))
		require.Equal(t, oldIDs[i], id)
		body, summary := `"synthetic request"`, `"synthetic response"`
		if i == 0 {
			body, summary = `""`, `""`
		}
		require.JSONEq(t, body, string(got["request_body"]))
		require.JSONEq(t, summary, string(got["response_summary"]))
		for name, want := range map[string]string{
			"username": `"synthetic_actor"`, "employee_no": `"S000007"`, "real_name": `"Synthetic Actor"`,
			"method": `"POST"`, "status_code": `403`, "duration": `123`, "ip": `"192.0.2.1"`,
			"user_agent": `"synthetic-agent"`, "request_id": `"req-synthetic"`,
		} {
			require.JSONEq(t, want, string(got[name]), name)
		}
		var created time.Time
		require.NoError(t, json.Unmarshal(got["created_at"], &created))
		require.True(t, old.Equal(created))
	}
	require.EqualValues(t, 0, countRows(t, `SELECT count(*) FROM audit_logs WHERE id=ANY($1)`, oldIDs))
	var freshBody, freshSummary string
	require.NoError(t, testPool.QueryRow(ctx, `SELECT request_body,response_summary FROM audit_logs WHERE id=$1`, freshID).Scan(&freshBody, &freshSummary))
	require.Equal(t, "fresh request", freshBody)
	require.Equal(t, "fresh response", freshSummary)
	policy := readTableArchive(t, dir, "policy_evaluation_logs")
	require.Len(t, policy, 1)
	var gotPolicy struct {
		ID      int64  `json:"id"`
		Reason  string `json:"reason"`
		TraceID string `json:"trace_id"`
	}
	require.NoError(t, json.Unmarshal([]byte(policy[0]), &gotPolicy))
	require.Equal(t, policyID, gotPolicy.ID)
	require.Equal(t, "synthetic reason", gotPolicy.Reason)
	require.Equal(t, "req-synthetic", gotPolicy.TraceID)
	require.EqualValues(t, 0, countRows(t, `SELECT count(*) FROM policy_evaluation_logs WHERE id=$1`, policyID))
	require.NoError(t, job.Handle(ctx, nil))
	require.Len(t, readTableArchive(t, dir, "audit_logs"), 2)
	require.Len(t, readTableArchive(t, dir, "policy_evaluation_logs"), 1)
}
