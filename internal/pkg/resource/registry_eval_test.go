package resource

import (
	"context"
	"errors"
	"testing"

	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/reqid"
)

// stubRes 固定判定结果的测试资源。
type stubRes struct {
	code    string
	allowed bool
	err     error
}

func (s *stubRes) Code() string                                              { return s.code }
func (s *stubRes) Name() string                                              { return s.code }
func (s *stubRes) Actions() []string                                         { return []string{"read"} }
func (s *stubRes) Authorize(context.Context, AuthorizeRequest) (bool, error) { return s.allowed, s.err }
func (s *stubRes) GetFilter(context.Context, int64, string) (Filter, error) {
	return Filter{Where: "1=1"}, nil
}

func TestEvalHookFires(t *testing.T) {
	reg := NewRegistry()
	var got []EvalEntry
	reg.SetEvalHook(func(_ context.Context, e EvalEntry) { got = append(got, e) })

	ctx := reqid.With(context.Background(), "req-hook-1")

	// 允许行
	reg.Register(&stubRes{code: "ok-res", allowed: true})
	if _, err := reg.Authorize(ctx, "ok-res", AuthorizeRequest{
		UserID: 7, Roles: []string{"user"}, Action: "read", ResourceID: "42"}); err != nil {
		t.Fatal(err)
	}
	// 拒绝行（false, nil → reason=denied）
	reg.Register(&stubRes{code: "deny-res"})
	if _, err := reg.Authorize(ctx, "deny-res", AuthorizeRequest{UserID: 8, Action: "read"}); err != nil {
		t.Fatal(err)
	}
	// 错误行（err → reason=error:...）
	reg.Register(&stubRes{code: "err-res", err: errors.New("db down")})
	if _, err := reg.Authorize(ctx, "err-res", AuthorizeRequest{UserID: 9, Action: "read"}); err == nil {
		t.Fatal("want error propagated")
	}

	if len(got) != 3 {
		t.Fatalf("hook fired %d times, want 3", len(got))
	}
	if got[0].Result != true || got[0].TraceID != "req-hook-1" || got[0].ResourceType != "ok-res" {
		t.Fatalf("entry0 = %+v", got[0])
	}
	if got[1].Result != false || got[1].Reason != "denied" {
		t.Fatalf("entry1 = %+v", got[1])
	}
	if got[2].Result != false || got[2].Reason != "error: db down" {
		t.Fatalf("entry2 = %+v", got[2])
	}
}

func TestEvalHookReasonTruncated(t *testing.T) {
	reg := NewRegistry()
	var got EvalEntry
	reg.SetEvalHook(func(_ context.Context, e EvalEntry) { got = e })
	long := errors.New(string(make([]byte, 500)))
	reg.Register(&stubRes{code: "long", err: long})
	_, _ = reg.Authorize(context.Background(), "long", AuthorizeRequest{UserID: 1, Action: "read"})
	if len(got.Reason) != 200 {
		t.Fatalf("reason len = %d, want 200", len(got.Reason))
	}
}

func TestEvalHookAbsentNoPanic(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&stubRes{code: "x", allowed: true})
	ok, err := reg.Authorize(context.Background(), "x", AuthorizeRequest{UserID: 1})
	if err != nil || !ok {
		t.Fatalf("(%v,%v)", ok, err)
	}
}
