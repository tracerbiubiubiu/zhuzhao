package service

import (
	"context"
	"errors"
	"log/slog"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

// DictService P4-3 字典（业务枚举运行时化——边界：不碰权限策略面）。
type DictService struct {
	repo   *repository.DictRepo
	logger *slog.Logger
}

func NewDictService(repo *repository.DictRepo, logger *slog.Logger) *DictService {
	return &DictService{repo: repo, logger: logger}
}

func (s *DictService) ListTypes(ctx context.Context, page, pageSize int, codeLike string) (*model.DictTypeListResponse, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	list, total, err := s.repo.ListTypes(ctx, page, pageSize, codeLike)
	if err != nil {
		s.logger.Error("dict list types", "err", err)
		return nil, errcode.ErrInternal
	}
	return &model.DictTypeListResponse{List: list, Total: total, Page: page, PageSize: pageSize}, nil
}

func (s *DictService) CreateType(ctx context.Context, t *model.DictType) error {
	if err := s.repo.CreateType(ctx, t); err != nil {
		if errors.Is(err, repository.ErrDictTypeDupCode) {
			return errcode.New(errcode.ErrConflict.Code, "字典类型 code 已存在")
		}
		s.logger.Error("dict create type", "err", err)
		return errcode.ErrInternal
	}
	return nil
}

func (s *DictService) UpdateType(ctx context.Context, t *model.DictType) error {
	if err := s.repo.UpdateType(ctx, t); err != nil {
		if errors.Is(err, repository.ErrDictTypeNotFound) {
			return errcode.ErrDictTypeNotFound
		}
		s.logger.Error("dict update type", "err", err)
		return errcode.ErrInternal
	}
	return nil
}

func (s *DictService) DeleteType(ctx context.Context, code string) error {
	if err := s.repo.DeleteTypeByCode(ctx, code); err != nil {
		if errors.Is(err, repository.ErrDictTypeNotFound) {
			return errcode.ErrDictTypeNotFound
		}
		s.logger.Error("dict delete type", "err", err)
		return errcode.ErrInternal
	}
	return nil
}

func (s *DictService) ListItems(ctx context.Context, page, pageSize int, typeCode string) (*model.DictItemListResponse, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	list, total, err := s.repo.ListItems(ctx, page, pageSize, typeCode)
	if err != nil {
		s.logger.Error("dict list items", "err", err)
		return nil, errcode.ErrInternal
	}
	return &model.DictItemListResponse{List: list, Total: total, Page: page, PageSize: pageSize}, nil
}

// EnabledItemsByCode 消费面（业务表单选项等）：按 type 拉取启用项（type 停用返回空）
func (s *DictService) EnabledItemsByCode(ctx context.Context, code string) ([]*model.DictItem, error) {
	items, err := s.repo.ListEnabledItemsByCode(ctx, code)
	if err != nil {
		s.logger.Error("dict enabled items", "err", err)
		return nil, errcode.ErrInternal
	}
	return items, nil
}

func (s *DictService) CreateItem(ctx context.Context, it *model.DictItem) error {
	if err := s.repo.CreateItem(ctx, it); err != nil {
		switch {
		case errors.Is(err, repository.ErrDictItemDupCode):
			return errcode.New(errcode.ErrConflict.Code, "字典项 code 已存在")
		case errors.Is(err, repository.ErrDictTypeNotFound):
			return errcode.ErrDictTypeNotFound
		}
		s.logger.Error("dict create item", "err", err)
		return errcode.ErrInternal
	}
	return nil
}

func (s *DictService) UpdateItem(ctx context.Context, it *model.DictItem) error {
	if err := s.repo.UpdateItem(ctx, it); err != nil {
		if errors.Is(err, repository.ErrDictItemNotFound) {
			return errcode.ErrDictItemNotFound
		}
		s.logger.Error("dict update item", "err", err)
		return errcode.ErrInternal
	}
	return nil
}

func (s *DictService) DeleteItem(ctx context.Context, id int64) error {
	if err := s.repo.DeleteItemByID(ctx, id); err != nil {
		if errors.Is(err, repository.ErrDictItemNotFound) {
			return errcode.New(92302, "字典项不存在")
		}
		s.logger.Error("dict delete item", "err", err)
		return errcode.ErrInternal
	}
	return nil
}
