package service

import (
	"context"
	"fmt"

	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

// UserService 用户管理服务
type UserService struct {
	userRepo *repository.UserRepo
}

func NewUserService(userRepo *repository.UserRepo) *UserService {
	return &UserService{userRepo: userRepo}
}

func (s *UserService) GetByID(ctx context.Context, id string) (interface{}, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *UserService) List(ctx context.Context, page, pageSize int) (interface{}, int64, error) {
	return nil, 0, fmt.Errorf("not implemented")
}

func (s *UserService) Create(ctx context.Context, req interface{}) error {
	return fmt.Errorf("not implemented")
}

func (s *UserService) Update(ctx context.Context, id string, req interface{}) error {
	return fmt.Errorf("not implemented")
}

func (s *UserService) Delete(ctx context.Context, id string) error {
	return fmt.Errorf("not implemented")
}
