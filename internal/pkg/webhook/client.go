// Package webhook P4-2 通知渠道出站 HTTP（架构红线：service 层禁 HTTP 框架——
// 出站客户端住 pkg 层，同 internal/pkg/taskrunner/client.go 先例；service 只收业务参数）。
package webhook

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client webhook 投递器（5s 超时——告警链路尽力而为，不挂长尾）
type Client struct {
	http *http.Client
}

func New() *Client {
	return &Client{http: &http.Client{Timeout: 5 * time.Second}}
}

// Post JSON 载荷投递：≥3xx 视为失败（含响应码上下文）；读弃响应体防连接泄漏。
func (c *Client) Post(ctx context.Context, url string, body []byte) error {
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
