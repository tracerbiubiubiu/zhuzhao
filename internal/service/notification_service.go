package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

// NotificationService P4-2 通知通道：配置管理（管理 API 面）+ 事件分发
// （死信告警等——webhook 优先，渠道接口配置化）。分发尽力而为：单渠道失败
// 记日志不阻塞调用方（告警链路自身不得成为新的故障源）。
type NotificationService struct {
	repo   *repository.NotificationRepo
	logger *slog.Logger
	client *http.Client
}

func NewNotificationService(repo *repository.NotificationRepo, logger *slog.Logger) *NotificationService {
	return &NotificationService{repo: repo, logger: logger, client: &http.Client{Timeout: 5 * time.Second}}
}

// ---- 配置管理 ----

func (s *NotificationService) List(ctx context.Context, page, pageSize int, codeLike string) (*model.NotificationConfigListResponse, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	list, total, err := s.repo.List(ctx, page, pageSize, codeLike)
	if err != nil {
		return nil, errcode.ErrInternal
	}
	return &model.NotificationConfigListResponse{List: list, Total: total, Page: page, PageSize: pageSize}, nil
}

func (s *NotificationService) Create(ctx context.Context, in *model.NotificationConfig) error {
	in.Channel = strings.TrimSpace(in.Channel)
	if in.Channel == "" {
		in.Channel = "webhook"
	}
	if in.Channel != "webhook" {
		return errcode.ErrNotificationChannelNotSupport
	}
	if !strings.HasPrefix(in.WebhookURL, "http") {
		return errcode.New(errcode.ErrInvalidParams.Code, "webhook_url 须为 http(s) 地址")
	}
	if err := s.repo.Create(ctx, in); err != nil {
		if errors.Is(err, repository.ErrNotificationDupCode) {
			return errcode.ErrNotificationCodeExists
		}
		s.logger.Error("notification create", "err", err)
		return errcode.ErrInternal
	}
	return nil
}

func (s *NotificationService) Update(ctx context.Context, in *model.NotificationConfig) error {
	if in.Channel != "" && in.Channel != "webhook" {
		return errcode.ErrNotificationChannelNotSupport
	}
	if in.Channel == "" {
		in.Channel = "webhook"
	}
	if err := s.repo.Update(ctx, in); err != nil {
		if errors.Is(err, repository.ErrNotificationNotFound) {
			return errcode.ErrNotificationConfigNotFound
		}
		s.logger.Error("notification update", "err", err)
		return errcode.ErrInternal
	}
	return nil
}

func (s *NotificationService) Delete(ctx context.Context, code string) error {
	if err := s.repo.DeleteByCode(ctx, code); err != nil {
		if errors.Is(err, repository.ErrNotificationNotFound) {
			return errcode.ErrNotificationConfigNotFound
		}
		s.logger.Error("notification delete", "err", err)
		return errcode.ErrInternal
	}
	return nil
}

// ---- 事件分发 ----

// notifyEnvelope webhook 载荷（渠道无关事件信封——渠道侧按需适配字段）
type notifyEnvelope struct {
	Event     string                         `json:"event"`
	Timestamp string                         `json:"timestamp"`
	Data      *model.DeadLetterNotifyPayload `json:"data"`
}

// NotifyDeadLetter 死信告警分发（taskrunner 经 /internal/notify/dead-letter 投递）：
// 按启用配置逐渠道 POST；无配置=静默跳过（告警通道未配置不构成错误——受理侧零依赖）。
func (s *NotificationService) NotifyDeadLetter(ctx context.Context, p *model.DeadLetterNotifyPayload) error {
	configs, err := s.repo.ListEnabled(ctx)
	if err != nil {
		s.logger.Error("dead-letter notify: list configs", "err", err)
		return errcode.ErrInternal
	}
	env := notifyEnvelope{Event: "dead_letter", Timestamp: time.Now().Format(time.RFC3339), Data: p}
	body, _ := json.Marshal(env)
	for _, c := range configs {
		if c.Channel != "webhook" {
			continue
		}
		if err := s.postWebhook(ctx, c, body); err != nil {
			s.logger.Error("dead-letter notify: webhook failed",
				"code", c.Code, "url", maskURL(c.WebhookURL), "err", err)
		} else {
			s.logger.Info("dead-letter notify: webhook sent", "code", c.Code, "task_id", p.TaskID)
		}
	}
	// 尽力而为语义：受理即 2xx（部分渠道失败不拒收——告警重投属事件源职责，
	// 死信本身已终态无重投语义，失败只记日志）
	return nil
}

func (s *NotificationService) postWebhook(ctx context.Context, c *model.NotificationConfig, body []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.WebhookURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.client.Do(req)
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

// maskURL 日志脱敏：query 参数可能携带 token（钉钉/飞书 webhook 形态）——只留 origin+path
func maskURL(u string) string {
	if i := strings.IndexByte(u, '?'); i >= 0 {
		return u[:i] + "?…"
	}
	return u
}
