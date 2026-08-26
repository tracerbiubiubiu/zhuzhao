package resource

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubResource 实现 Resource 接口的最小 stub，用于测试 Registry
type stubResource struct {
	code     string
	actions  []string
	authzRes bool
	authzErr error
	filter   Filter
	filterErr error
}

func (s *stubResource) Code() string { return s.code }
func (s *stubResource) Name() string { return s.code + "-name" }
func (s *stubResource) Actions() []string { return s.actions }
func (s *stubResource) Authorize(_ context.Context, _ AuthorizeRequest) (bool, error) {
	return s.authzRes, s.authzErr
}
func (s *stubResource) GetFilter(_ context.Context, _ int64, _ string) (Filter, error) {
	return s.filter, s.filterErr
}

func TestNewRegistry_Empty(t *testing.T) {
	r := NewRegistry()
	assert.NotNil(t, r)
	assert.Empty(t, r.List())
}

func TestRegistry_RegisterAndGet(t *testing.T) {
	r := NewRegistry()
	res := &stubResource{code: "ticket", actions: []string{"read", "write"}}

	r.Register(res)

	got, ok := r.Get("ticket")
	require.True(t, ok)
	assert.Equal(t, "ticket", got.Code())
	assert.Equal(t, []string{"read", "write"}, got.Actions())

	_, ok = r.Get("unknown")
	assert.False(t, ok, "未注册的资源应返回 false")
}

func TestRegistry_List(t *testing.T) {
	r := NewRegistry()
	r.Register(&stubResource{code: "ticket"})
	r.Register(&stubResource{code: "user"})

	list := r.List()
	assert.Len(t, list, 2)
	codes := map[string]bool{}
	for _, res := range list {
		codes[res.Code()] = true
	}
	assert.True(t, codes["ticket"])
	assert.True(t, codes["user"])
}

func TestRegistry_Authorize(t *testing.T) {
	t.Run("未注册资源应报错", func(t *testing.T) {
		r := NewRegistry()
		ok, err := r.Authorize(context.Background(), "unknown", AuthorizeRequest{})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "resource unknown not registered")
		assert.False(t, ok)
	})

	t.Run("已注册资源正确返回鉴权结果", func(t *testing.T) {
		r := NewRegistry()
		r.Register(&stubResource{code: "ticket", authzRes: true})

		ok, err := r.Authorize(context.Background(), "ticket", AuthorizeRequest{})
		require.NoError(t, err)
		assert.True(t, ok)
	})
}

func TestRegistry_GetFilter(t *testing.T) {
	t.Run("未注册资源应报错", func(t *testing.T) {
		r := NewRegistry()
		_, err := r.GetFilter(context.Background(), "unknown", 1, "read")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "resource unknown not registered")
	})

	t.Run("已注册资源正确返回 Filter", func(t *testing.T) {
		r := NewRegistry()
		expected := Filter{Where: "created_by = ?", Args: []any{int64(1)}}
		r.Register(&stubResource{code: "ticket", filter: expected})

		got, err := r.GetFilter(context.Background(), "ticket", 1, "read")
		require.NoError(t, err)
		assert.Equal(t, expected, got)
	})
}
