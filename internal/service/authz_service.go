package service

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

// AuthzService 资源级鉴权服务（ReBAC + 属主判断）
type AuthzService struct {
	db    *pgxpool.Pool
	rdb   *redis.Client
}

func NewAuthzService(db *pgxpool.Pool, rdb *redis.Client) *AuthzService {
	return &AuthzService{db: db, rdb: rdb}
}

// CheckResourcePermission 检查资源级权限（属主 → ReBAC）
func (s *AuthzService) CheckResourcePermission(ctx context.Context, userID, roleKey, resourceType, resourceID, action string) (bool, error) {
	return false, fmt.Errorf("not implemented")
}
