// Package channelsender 通道发送端口（S6 B6-2——06 号 §3 通道层）。
//
// 统一消息平台 = 所有渠道（邮件/钉钉/…）共用一个调用接口——通道抽象
// 只需在 notification_configs 上新增渠道类型（type=uip），不做渠道 SDK
// 直连。业务代码禁止硬编码平台 HTTP 调用（06 号 §3 红线）。
package channelsender

import (
	"context"
	"fmt"
	"log/slog"
)

// Message 出站消息（渠道无关——收件人/主题/正文已解析为最终形态）。
type Message struct {
	Recipients   []string // 收件人标识（工号/邮箱——UIP 统一映射）
	Subject      string   // 主题（邮件/IM 标题）
	Body         string   // 正文（纯文本/轻 HTML——按渠道 adapter 归属）
	BizType      string   // 业务域（审计归因）
	BizID        string   // 业务对象 ID（审计归因）
	TemplateCode string   // 模板 code（审计归因）
}

// ChannelSender 通道发送端口——换平台/加渠道 = 新 adapter（06 号 §3）。
type ChannelSender interface {
	// Send 发送消息；返回 error 时由调用方决定重试/死信。
	Send(ctx context.Context, msg Message) error
	// Channel 渠道标识（与 notification_configs.channel 对齐）。
	Channel() string
}

// ── UIP 两态桩（06 号 §3 桩两态口径——防虚标纪律）──
//
// 开发态（stub_mode=true）= 返回成功 + 落日志（供触发→模板→渲染→记录全链 E2E）。
// 生产态（未配置 endpoint）= fail-fast 报"未配置"（虚标防线——桩不得标注为
// "已实现统一消息平台对接"）。

// UIPStubConfig UIP 桩配置（从 notification_configs 或 config 段读取）。
type UIPStubConfig struct {
	StubMode bool   // true=开发态桩（返回成功+落日志）
	Endpoint string // 生产态 UIP 端点（空=fail-fast）
}

// UIPStubSender 统一消息平台两态桩（S6 B6-2 首版——真实契约到手只改映射层）。
type UIPStubSender struct {
	cfg    UIPStubConfig
	logger *slog.Logger
}

// NewUIPStubSender 创建 UIP 桩发送器。
func NewUIPStubSender(cfg UIPStubConfig, logger *slog.Logger) *UIPStubSender {
	if logger == nil {
		logger = slog.Default()
	}
	return &UIPStubSender{cfg: cfg, logger: logger}
}

func (s *UIPStubSender) Channel() string { return "uip" }

func (s *UIPStubSender) Send(ctx context.Context, msg Message) error {
	if s.cfg.StubMode {
		// 开发态：返回成功 + 落日志（全链可跑——但不虚标为真实送达）
		s.logger.Info("UIP stub send (dev mode)",
			"recipients", fmt.Sprint(msg.Recipients),
			"subject", msg.Subject,
			"biz_type", msg.BizType, "biz_id", msg.BizID,
			"template", msg.TemplateCode,
			"body_len", len(msg.Body))
		return nil
	}
	if s.cfg.Endpoint == "" {
		// 生产态未配置：fail-fast（虚标防线——06 号 §3「桩不得标注为已实现」）
		return fmt.Errorf("UIP 渠道未配置 endpoint（生产态 fail-fast——非开发态桩）")
	}
	// TODO B6-2 后续：真实 UIP HTTP 调用（契约到手后补——endpoint/鉴权头/字段映射
	// 层独立成函数，06 号 §3「无契约先行开发」口径）
	return fmt.Errorf("UIP 真实发送未实现（endpoint=%s）——契约到手后补映射层", s.cfg.Endpoint)
}
