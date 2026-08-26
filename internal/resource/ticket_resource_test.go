package ticket_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/resource"
	rsrc "github.com/tracerbiubiubiu/zhuzhao/internal/resource"
)

// TestTicketResourceSkeleton 验证 TicketResource 骨架符合契约
func TestTicketResourceSkeleton(t *testing.T) {
	res := rsrc.NewResource()

	assert.Equal(t, "ticket", res.Code())
	assert.NotEmpty(t, res.Name())
	assert.Contains(t, res.Actions(), "create")
	assert.Contains(t, res.Actions(), "read")
	assert.Contains(t, res.Actions(), "update")
	assert.Contains(t, res.Actions(), "delete")
	assert.Contains(t, res.Actions(), "close")
	assert.Contains(t, res.Actions(), "assign")
	assert.Contains(t, res.Actions(), "comment")

	// Step 1 骨架应 fail-closed：Authorize 返回 false 且报错
	ok, err := res.Authorize(context.Background(), resource.AuthorizeRequest{})
	assert.Error(t, err)
	assert.False(t, ok)

	// Step 1 骨架可见性为空集
	filter, err := res.GetFilter(context.Background(), 1, "read")
	require.NoError(t, err)
	assert.Equal(t, "1 = 0", filter.Where, "骨架阶段 GetFilter 应返回空过滤条件（1=0）")
}

// TestTicketResource_RegistryIntegration 验证 TicketResource 可正确注册到 Registry 并被路由
func TestTicketResource_RegistryIntegration(t *testing.T) {
	reg := resource.NewRegistry()
	reg.Register(rsrc.NewResource())

	got, ok := reg.Get("ticket")
	require.True(t, ok, "Registry 应能获取到已注册的 ticket 资源")
	assert.Equal(t, "ticket", got.Code())

	// 调用 Registry.Authorize 应能路由到 TicketResource (fail-closed)
	authzRes, err := reg.Authorize(context.Background(), "ticket", resource.AuthorizeRequest{
		UserID: 1, Action: "read", ResourceID: "100",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not implemented", "应路由到 TicketResource 的 fail-closed 实现")
	assert.False(t, authzRes)

	// 调用 Registry.GetFilter 应能路由到 TicketResource
	filter, err := reg.GetFilter(context.Background(), "ticket", 1, "read")
	require.NoError(t, err)
	assert.Equal(t, "1 = 0", filter.Where)
}
