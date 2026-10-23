// MessageTemplateService 消息模板管理（S6 B6-3）。
package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/msgrender"
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

// validatePlaceholders F-1-9 修复：模板引用的变量路径须 ⊆ 内置键集
// （管理员写了不存在/不支持的变量→保存期 400——比运行时跳过更早反馈）。
func validatePlaceholders(subject, body string) error {
	for _, ph := range msgrender.ExtractPlaceholders(subject + " " + body) {
		if !builtinVarSet[ph] {
			return errcode.New(errcode.ErrInvalidParams.Code,
				fmt.Sprintf("模板引用了不支持的变量 %s——可用变量见 variables 端点", ph))
		}
	}
	return nil
}

// builtinVarSet 内置变量白名单（与 variables 端点返回一致——消息字段动态源待二期补）。
var builtinVarSet = map[string]bool{
	"ticket.title": true, "ticket.type_code": true, "ticket.priority": true,
	"ticket.status": true, "ticket.human_key": true, "ticket.link": true,
}

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
	if err := validatePlaceholders(t.Subject, t.Body); err != nil {
		return err
	}
	_, err := s.repo.GetByCode(ctx, t.Code)
	if err == nil {
		return errcode.ErrMsgTplDupCode
	}
	if !errors.Is(err, errcode.ErrMsgTplNotFound) {
		return err
	}
	return s.repo.Create(ctx, t)
}

func (s *MessageTemplateService) Update(ctx context.Context, t *model.MessageTemplate) error {
	// F-1-7 修复：Update 不校验 channel（不可变——SET 无 channel 列）
	return s.repo.Update(ctx, t)
}

func (s *MessageTemplateService) Delete(ctx context.Context, code string) error {
	return s.repo.Delete(ctx, code)
}
