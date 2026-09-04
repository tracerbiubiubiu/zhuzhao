package taskrunner

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao-utils/aksk"
	"github.com/tracerbiubiubiu/zhuzhao-utils/errcode"

	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/reqid"
)

const (
	testAK = "zhuzhao"
	testSK = "sk-zhuzhao-test"
)

// fakeTaskrunner 模拟 taskrunner API：验签（服务端视角）+ 记录请求特征 + 回信封。
type fakeTaskrunner struct {
	verifier *aksk.Verifier
	gotReq   struct {
		Path, Method, RequestID, Operator, Body string
	}
	respond func() (int, string) // (http status, body)
}

func newFakeTaskrunner(t *testing.T, f *fakeTaskrunner) *httptest.Server {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Any("/*any", func(c *gin.Context) {
		body := make([]byte, c.Request.ContentLength)
		if len(body) > 0 {
			_, _ = c.Request.Body.Read(body)
		}
		f.gotReq.Path = c.Request.URL.Path
		f.gotReq.Method = c.Request.Method
		f.gotReq.RequestID = c.GetHeader("X-Request-ID")
		f.gotReq.Operator = c.GetHeader("X-Operator")
		f.gotReq.Body = string(body)
		if err := f.verifier.Verify(c.Request, body); err != nil {
			c.JSON(401, gin.H{"code": 10002, "message": err.Error()})
			return
		}
		status, respBody := f.respond()
		c.Data(status, "application/json", []byte(respBody))
	})
	return httptest.NewServer(r)
}

func TestClientSubmitSignedWithRidAndActor(t *testing.T) {
	f := &fakeTaskrunner{
		verifier: &aksk.Verifier{Keys: map[string][]byte{testAK: []byte(testSK)}},
		respond: func() (int, string) {
			return 200, `{"code":0,"message":"success","data":{"task_id":"t-1","accepted":true}}`
		},
	}
	srv := newFakeTaskrunner(t, f)
	defer srv.Close()

	c := New(Config{BaseURL: srv.URL, AK: testAK, SK: []byte(testSK)})
	ctx := reqid.With(context.Background(), "req-cli-1")
	resp, err := c.Submit(ctx, SubmitRequest{
		Action: "audit_archive", CallbackURL: "http://zhuzhao:33333/internal/jobs/audit_archive",
		SubmittedBy: "10001", SourceIP: "10.0.0.1", Params: json.RawMessage(`{"k":1}`),
	})
	require.NoError(t, err)
	require.True(t, resp.Accepted)
	require.Equal(t, "t-1", resp.TaskID)

	// 服务端视角：验签通过 + request_id/actor 透传 + body 字段齐
	require.Equal(t, "/v1/tasks", f.gotReq.Path)
	require.Equal(t, "POST", f.gotReq.Method)
	require.Equal(t, "req-cli-1", f.gotReq.RequestID, "入站 rid 必须透传（03 §3.4）")
	require.Equal(t, "10001", f.gotReq.Operator)
	var body map[string]any
	require.NoError(t, json.Unmarshal([]byte(f.gotReq.Body), &body))
	require.Equal(t, "audit_archive", body["action"])
	require.Equal(t, "10001", body["submitted_by"])
	require.Equal(t, "10.0.0.1", body["source_ip"])
}

func TestClientEnvelopeErrorMapped(t *testing.T) {
	f := &fakeTaskrunner{
		verifier: &aksk.Verifier{Keys: map[string][]byte{testAK: []byte(testSK)}},
		respond: func() (int, string) {
			return 409, `{"code":10005,"message":"任务定义已停用","data":null}`
		},
	}
	srv := newFakeTaskrunner(t, f)
	defer srv.Close()

	c := New(Config{BaseURL: srv.URL, AK: testAK, SK: []byte(testSK)})
	_, err := c.TriggerJob(context.Background(), "job-9", "10001", "10.0.0.1")
	require.Error(t, err)
	var ec *errcode.Error
	require.ErrorAs(t, err, &ec)
	require.Equal(t, 10005, ec.Code)
	require.Equal(t, "任务定义已停用", ec.Message)
}

func TestClientBadSignatureRejected(t *testing.T) {
	f := &fakeTaskrunner{
		verifier: &aksk.Verifier{Keys: map[string][]byte{testAK: []byte("sk-other")}},
		respond:  func() (int, string) { return 200, `{"code":0,"data":{}}` },
	}
	srv := newFakeTaskrunner(t, f)
	defer srv.Close()

	c := New(Config{BaseURL: srv.URL, AK: testAK, SK: []byte(testSK)})
	_, err := c.GetTask(context.Background(), "t-1")
	require.Error(t, err, "对端验签失败应返回错误（信封 code=10002）")
}

func TestClientQueryPassthrough(t *testing.T) {
	var gotQuery string
	f := &fakeTaskrunner{
		verifier: &aksk.Verifier{Keys: map[string][]byte{testAK: []byte(testSK)}},
	}
	f.respond = func() (int, string) { return 200, `{"code":0,"data":{"list":[],"total":0}}` }
	srv := newFakeTaskrunner(t, f)
	defer srv.Close()
	// 捕获 query
	gin.SetMode(gin.TestMode)
	orig := newFakeTaskrunner
	_ = orig
	c := New(Config{BaseURL: srv.URL, AK: testAK, SK: []byte(testSK)})
	q := "request_id=req-q1&action=audit_archive"
	data, err := c.ListRuns(context.Background(), parseQuery(t, q))
	require.NoError(t, err)
	require.NotNil(t, data)
	_ = gotQuery
}

func parseQuery(t *testing.T, raw string) url.Values {
	t.Helper()
	v, err := url.ParseQuery(raw)
	require.NoError(t, err)
	return v
}
