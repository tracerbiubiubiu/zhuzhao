package service

import (
	"context"
	"fmt"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

// OrgService 组织架构管理服务
type OrgService struct {
	orgRepo  *repository.OrgRepo
	userRepo *repository.UserRepo
}

func NewOrgService(orgRepo *repository.OrgRepo, userRepo *repository.UserRepo) *OrgService {
	return &OrgService{orgRepo: orgRepo, userRepo: userRepo}
}

func (s *OrgService) GetTree(ctx context.Context) ([]*model.Organization, error) {
	return s.orgRepo.GetTree(ctx)
}

func (s *OrgService) GetUserOrgs(ctx context.Context, userID int64) ([]*model.UserOrg, error) {
	if _, err := s.userRepo.FindByID(ctx, userID); err != nil {
		return nil, err
	}
	return s.orgRepo.GetUserOrgs(ctx, userID)
}

func (s *OrgService) AddMember(ctx context.Context, req *model.OrgMemberRequest) error {
	if _, err := s.orgRepo.FindByID(ctx, req.OrgID); err != nil {
		return err
	}
	if _, err := s.userRepo.FindByID(ctx, req.UserID); err != nil {
		return err
	}
	return s.orgRepo.AddMember(ctx, req.OrgID, req.UserID, req.IsPrimary)
}

func (s *OrgService) RemoveMember(ctx context.Context, req *model.OrgMemberRequest) error {
	return s.orgRepo.RemoveMember(ctx, req.OrgID, req.UserID)
}

func (s *OrgService) SetUserOrgs(ctx context.Context, req *model.SetUserOrgsRequest) error {
	if _, err := s.userRepo.FindByID(ctx, req.UserID); err != nil {
		return err
	}
	for _, orgID := range req.OrgIDs {
		if _, err := s.orgRepo.FindByID(ctx, orgID); err != nil {
			return err
		}
	}
	if req.PrimaryOrgID != nil {
		found := false
		for _, id := range req.OrgIDs {
			if id == *req.PrimaryOrgID {
				found = true
				break
			}
		}
		if !found {
			return errcode.ErrInvalidParams
		}
	}
	return s.orgRepo.SetUserOrgs(ctx, req.UserID, []int64(req.OrgIDs), req.PrimaryOrgID)
}

func (s *OrgService) Create(ctx context.Context, req interface{}) error {
	return fmt.Errorf("not implemented")
}

func (s *OrgService) Update(ctx context.Context, id int64, req interface{}) error {
	return fmt.Errorf("not implemented")
}

func (s *OrgService) Delete(ctx context.Context, id int64) error {
	return fmt.Errorf("not implemented")
}

func (s *OrgService) Move(ctx context.Context, id, parentID int64) error {
	return fmt.Errorf("not implemented")
}
