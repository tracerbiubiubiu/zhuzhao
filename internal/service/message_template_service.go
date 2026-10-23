// MessageTemplateService 消息模板管理（S6 B6-3）。
package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/tracerbiubiubiu/zhuzhao-utils/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

type MessageTemplateService struct {
	repo *repository.MessageTemplateRepo
}

func NewMessageTemplateService(repo *repository.MessageTemplateRepo) *MessageTemplateService {
	return &MessageTemplateService{repo: repo}
}

func (s *MessageTemplateService) List(ctx context.Context, page, pageSize int, codeLike string) ([]*model.MessageTemplate, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	return s.repo.List(ctx, page, pageSize, codeLike)
}

// supportedChannels 渠道白名单（06 号 §3——新渠道=新 adapter 后加此表）。
var supportedChannels = map[string]bool{"uip": true, "webhook": true}

func validateChannel(channel string) error {
	if !supportedChannels[channel] {
		return errcode.New(errcode.ErrInvalidParams.Code, fmt.Sprintf("channel %q 不在支持列表——新渠道须先落 adapter", channel))
	}
	return nil
}

func (s *MessageTemplateService) Create(ctx context.Context, t *model.MessageTemplate) error {
	if err := validateChannel(t.Channel); err != nil {
		return err
	}
	_, err := s.repo.GetByCode(ctx, t.Code)
	if err == nil {
		return repository.ErrTemplateDupCode
	}
	if !errors.Is(err, repository.ErrTemplateNotFound) {
		return err
	}
	// TODO: 真实写入（INSERT）
	return s.repo.Create(ctx, t)
}

func (s *MessageTemplateService) Update(ctx context.Context, t *model.MessageTemplate) error {
	if err := validateChannel(t.Channel); err != nil {
		return err
	}
	return s.repo.Update(ctx, t)
}

func (s *MessageTemplateService) Delete(ctx context.Context, code string) error {
	return s.repo.Delete(ctx, code)
}
