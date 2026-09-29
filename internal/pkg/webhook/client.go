// Package webhook P4-2 通知渠道出站 HTTP（架构红线：service 层禁 HTTP 框架——
// 出站客户端住 pkg 层，同 internal/pkg/taskrunner/client.go 先例；service 只收业务参数）。
package webhook

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	neturl "net/url"
	"time"
)

// Client webhook 投递器（5s 超时——告警链路尽力而为，不挂长尾）
type Client struct {
	http *http.Client
}

func New() *Client {
	return &Client{http: &http.Client{
		Timeout: 5 * time.Second,
		// 禁重定向：3xx 直接返回响应（由 ≥3xx 分支判失败）——防 302 绕过 URL 校验
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}}
}

// blockedHosts SSRF 元地址黑名单（管理面可配 URL 的出站面收敛——内网正常目标不受限）
var blockedHosts = map[string]bool{
	"169.254.169.254":          true, // 云元数据
	"metadata.google.internal": true,
	"127.0.0.1":                true, "localhost": true, "::1": true,
	"0.0.0.0": true,
}

// ValidateURL 出站前校验（Create/Update 配置面与 Post 投递面双保险）
func ValidateURL(u string) error {
	pu, err := neturl.Parse(u)
	if err != nil {
		return fmt.Errorf("URL 非法: %w", err)
	}
	if pu.Scheme != "http" && pu.Scheme != "https" {
		return fmt.Errorf("URL 须为 http(s)")
	}
	host := pu.Hostname()
	if blockedHosts[host] {
		return fmt.Errorf("目标地址在出站黑名单（元地址/环回）")
	}
	return nil
}

// Post JSON 载荷投递：≥3xx 视为失败（含响应码上下文）；读弃响应体防连接泄漏。
// 审计修复（2026-09-30 P2 SSRF）：URL 黑名单（元地址/环回——含 host 解析后复核）+
// 禁跟随重定向（302 跳转绕过校验面）。
func (c *Client) Post(ctx context.Context, url string, body []byte) error {
	if err := ValidateURL(url); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10))
	if resp.StatusCode >= 300 {
		return fmt.Errorf("webhook status %d", resp.StatusCode)
	}
	return nil
}
