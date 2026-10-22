// ChannelSender 端口与 UIP 两态桩单测（B6-2）。
package channelsender

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUIPStubSender_DevMode(t *testing.T) {
	s := NewUIPStubSender(UIPStubConfig{StubMode: true}, nil)
	require.NoError(t, s.Send(context.Background(), Message{
		Recipients: []string{"E000001"}, Subject: "测试", Body: "内容",
		BizType: "ticket", BizID: "123", TemplateCode: "tpl_test",
	}))
	assert.Equal(t, "uip", s.Channel())
}

func TestUIPStubSender_ProdUnconfigured(t *testing.T) {
	s := NewUIPStubSender(UIPStubConfig{StubMode: false, Endpoint: ""}, nil)
	err := s.Send(context.Background(), Message{Recipients: []string{"E1"}})
	require.Error(t, err, "生产态未配置须 fail-fast")
	assert.Contains(t, err.Error(), "未配置")
}

func TestUIPStubSender_ProdWithEndpoint(t *testing.T) {
	s := NewUIPStubSender(UIPStubConfig{StubMode: false, Endpoint: "http://uip.corp"}, nil)
	err := s.Send(context.Background(), Message{Recipients: []string{"E1"}})
	require.Error(t, err, "真实发送未实现（契约到手后补）")
	assert.Contains(t, err.Error(), "未实现")
	assert.True(t, strings.Contains(err.Error(), "uip.corp"), "错误应带 endpoint 上下文")
}
