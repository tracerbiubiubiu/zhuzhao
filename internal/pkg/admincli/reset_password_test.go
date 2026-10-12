package admincli

import (
	"errors"
	"testing"
)

// TestCheckTarget 目标校验判定矩阵（纯函数；DB/Redis 面见 integration 测试）。
func TestCheckTarget(t *testing.T) {
	cases := []struct {
		name       string
		exists     bool
		isSuperadm bool
		want       error
	}{
		{"不存在", false, false, ErrTargetNotFound},
		{"存在但非 superadmin", true, false, ErrTargetNotSuperadmin},
		{"存在且持角色", true, true, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := checkTarget(c.exists, c.isSuperadm)
			if !errors.Is(got, c.want) {
				t.Fatalf("checkTarget(%v,%v) = %v, want %v", c.exists, c.isSuperadm, got, c.want)
			}
		})
	}
}

// TestResetOptionsValidation 入参护栏（连接/凭据缺失在触 DB 前拦截）。
func TestResetOptionsValidation(t *testing.T) {
	if _, err := ResetSuperadminPassword(t.Context(), ResetOptions{Username: "admin", Password: "x", RDB: nil, Pool: nil}); err == nil {
		t.Fatal("Pool/RDB 未初始化应报错")
	}
}
