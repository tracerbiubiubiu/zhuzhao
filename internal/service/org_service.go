package service

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/validate"
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

// SetUserOrgsTx 在外部事务内全量覆盖用户组织（供 UserService.Create 等事务编排调用）
func (s *OrgService) SetUserOrgsTx(ctx context.Context, tx pgx.Tx, req *model.SetUserOrgsRequest) error {
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
	return s.orgRepo.SetUserOrgsTx(ctx, tx, req.UserID, []int64(req.OrgIDs), req.PrimaryOrgID)
}

func (s *OrgService) GetByID(ctx context.Context, id int64) (*model.Organization, error) {
	return s.orgRepo.FindByID(ctx, id)
}

func (s *OrgService) GetMembers(ctx context.Context, orgID int64) (*model.OrgMemberListResponse, error) {
	if _, err := s.orgRepo.FindByID(ctx, orgID); err != nil {
		return nil, err
	}
	users, err := s.userRepo.ListByOrgID(ctx, orgID)
	if err != nil {
		return nil, err
	}
	return &model.OrgMemberListResponse{
		List:  users,
		Total: int64(len(users)),
	}, nil
}

func (s *OrgService) Create(ctx context.Context, req *model.CreateOrgRequest, actorUserID int64) (*model.Organization, error) {
	if !validate.LtreeLabel(req.Code) {
		return nil, errcode.ErrInvalidParams
	}
	if req.OrgType == 4 {
		return nil, errcode.ErrInvalidParams
	}

	var parentID *int64
	path := req.Code
	if req.ParentID != nil {
		parent, err := s.orgRepo.FindByID(ctx, *req.ParentID)
		if err != nil {
			return nil, err
		}
		parentID = &parent.ID
		path = parent.Path + "." + req.Code
	}

	org := &model.Organization{
		Code:        req.Code,
		Name:        req.Name,
		Description: req.Description,
		ParentID:    parentID,
		Path:        path,
		OrgType:     req.OrgType,
		Status:      1,
		SortOrder:   req.SortOrder,
		CreatedBy:   &actorUserID,
	}
	if err := s.orgRepo.Create(ctx, org); err != nil {
		return nil, err
	}
	return org, nil
}

func (s *OrgService) Update(ctx context.Context, req *model.UpdateOrgRequest) (*model.Organization, error) {
	org, err := s.orgRepo.FindByID(ctx, req.ID)
	if err != nil {
		return nil, err
	}
	if org.IsSystem {
		return nil, errcode.ErrOrgIsSystem
	}
	org.Name = req.Name
	org.Description = req.Description
	org.Status = req.Status
	org.SortOrder = req.SortOrder
	org.Version = req.Version
	if err := s.orgRepo.Update(ctx, org); err != nil {
		return nil, err
	}
	return org, nil
}

func (s *OrgService) Delete(ctx context.Context, id int64) error {
	org, err := s.orgRepo.FindByID(ctx, id)
	if err != nil {
		return err
	}
	if org.IsSystem {
		return errcode.ErrOrgIsSystem
	}
	n, err := s.orgRepo.CountChildren(ctx, id)
	if err != nil {
		return err
	}
	if n > 0 {
		return errcode.ErrOrgHasChildren
	}
	n, err = s.orgRepo.CountMembers(ctx, id)
	if err != nil {
		return err
	}
	if n > 0 {
		return errcode.ErrOrgHasMembers
	}
	return s.orgRepo.Delete(ctx, id)
}

func (s *OrgService) Move(ctx context.Context, req *model.MoveOrgRequest) error {
	org, err := s.orgRepo.FindByID(ctx, req.ID)
	if err != nil {
		return err
	}
	if org.IsSystem {
		return errcode.ErrOrgIsSystem
	}

	var newParentID *int64
	newRootPath := org.Code
	if req.NewParentID != nil {
		if *req.NewParentID == req.ID {
			return errcode.ErrOrgCannotMoveToChild
		}
		parent, err := s.orgRepo.FindByID(ctx, *req.NewParentID)
		if err != nil {
			return err
		}
		ok, err := s.orgRepo.IsDescendant(ctx, req.ID, parent.ID)
		if err != nil {
			return err
		}
		if ok {
			return errcode.ErrOrgCannotMoveToChild
		}
		newParentID = &parent.ID
		newRootPath = parent.Path + "." + org.Code
	}

	return s.orgRepo.Move(ctx, req.ID, newParentID, newRootPath, org.Path)
}
