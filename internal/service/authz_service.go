package service

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

// AuthzService 资源级鉴权服务（ReBAC + 属主判断）
type AuthzService struct {
	db  *pgxpool.Pool
	rdb *redis.Client
}

func NewAuthzService(db *pgxpool.Pool, rdb *redis.Client) *AuthzService {
	return &AuthzService{db: db, rdb: rdb}
}

// CheckResourcePermission 检查资源级权限（属主 → ReBAC）
//
// ⚠️ Phase 2a 预留占位（docs/phase2/02-authz-resource.md Step 0 将删除本 stub
// 并接线 ResourceRegistry），当前全仓无调用方——勿在 Phase 1 调用：
// 恒定返回 not implemented（fail-closed 方向，但语义未定义）。
func (s *AuthzService) CheckResourcePermission(ctx context.Context, userID, roleKey, resourceType, resourceID, action string) (bool, error) {
	return false, fmt.Errorf("not implemented")
}
