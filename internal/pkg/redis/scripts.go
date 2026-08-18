package redis

import (
	"context"
	_ "embed"
	"fmt"

	goredis "github.com/redis/go-redis/v9"
)

//go:embed scripts/login_lock.lua
var loginLockScript string

const (
	loginLockKeyPrefix = "lock:login:"
	loginLockWindowSec = 900
	loginLockMaxFail   = 5
)

// Scripts Redis Lua 脚本封装（LoginLocker 等）
type Scripts struct {
	client *goredis.Client
}

// NewScripts 创建脚本执行器
func NewScripts(client *goredis.Client) *Scripts {
	return &Scripts{client: client}
}

func (s *Scripts) loginLockKey(employeeNo string) string {
	return loginLockKeyPrefix + employeeNo
}

// LoginLockIsBlocked 校验密码前检查是否已锁定（count > max_fail）
func (s *Scripts) LoginLockIsBlocked(ctx context.Context, employeeNo string) (bool, error) {
	n, err := s.client.Get(ctx, s.loginLockKey(employeeNo)).Int()
	if err == goredis.Nil {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("login lock get: %w", err)
	}
	return n > loginLockMaxFail, nil
}

// LoginLockIncr 密码错误时原子 INCR + 首次 EXPIRE；返回 true 表示已超限
func (s *Scripts) LoginLockIncr(ctx context.Context, employeeNo string) (blocked bool, err error) {
	res, err := s.client.Eval(
		ctx,
		loginLockScript,
		[]string{s.loginLockKey(employeeNo)},
		loginLockWindowSec,
		loginLockMaxFail,
	).Int()
	if err != nil {
		return false, fmt.Errorf("login lock eval: %w", err)
	}
	return res == 1, nil
}

// LoginLockClear 登录成功时清除计数
func (s *Scripts) LoginLockClear(ctx context.Context, employeeNo string) error {
	if err := s.client.Del(ctx, s.loginLockKey(employeeNo)).Err(); err != nil {
		return fmt.Errorf("login lock clear: %w", err)
	}
	return nil
}
