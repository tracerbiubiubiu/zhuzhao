package middleware

// BK-17：request context 角色缓存单元测试

import (
	"context"
	"testing"
)

func TestRolesFromContext(t *testing.T) {
	ctx := context.WithValue(context.Background(), rolesCtxKey{}, &cachedRoles{userID: 7, roles: []string{"operator"}})
	uid, roles, ok := RolesFromContext(ctx)
	if !ok || uid != 7 || len(roles) != 1 || roles[0] != "operator" {
		t.Fatalf("缓存命中失败: ok=%v uid=%d roles=%v", ok, uid, roles)
	}
	if _, _, ok := RolesFromContext(context.Background()); ok {
		t.Fatal("未经过 CasbinAuth 的 ctx 不应命中缓存")
	}
}
