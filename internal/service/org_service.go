package service

import (
	"context"
	"fmt"

	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

// OrgService 组织架构管理服务
type OrgService struct {
	orgRepo *repository.OrgRepo
}

func NewOrgService(orgRepo *repository.OrgRepo) *OrgService {
	return &OrgService{orgRepo: orgRepo}
}

func (s *OrgService) GetTree(ctx context.Context) (interface{}, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *OrgService) Create(ctx context.Context, req interface{}) error {
	return fmt.Errorf("not implemented")
}

func (s *OrgService) Update(ctx context.Context, id string, req interface{}) error {
	return fmt.Errorf("not implemented")
}

func (s *OrgService) Delete(ctx context.Context, id string) error {
	return fmt.Errorf("not implemented")
}

func (s *OrgService) Move(ctx context.Context, id, parentID string) error {
	return fmt.Errorf("not implemented")
}
