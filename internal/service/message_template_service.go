// MessageTemplateService 消息模板管理（S6 B6-3）。
package service

import (
	"context"
	"errors"

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

func (s *MessageTemplateService) Create(ctx context.Context, t *model.MessageTemplate) error {
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
	return s.repo.Update(ctx, t)
}

func (s *MessageTemplateService) Delete(ctx context.Context, code string) error {
	return s.repo.Delete(ctx, code)
}
