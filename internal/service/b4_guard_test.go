package service

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
)

// B4-4 守护：菜单类型必要字段——页面(2)必有 path、按钮(3)必有 permission
func TestValidateMenuRequiredFields(t *testing.T) {
	assert.NoError(t, validateMenuRequiredFields(1, "", ""))    // 目录：无要求
	assert.NoError(t, validateMenuRequiredFields(2, "/sys", "")) // 页面：有 path
	assert.ErrorIs(t, validateMenuRequiredFields(2, "", ""), errcode.ErrInvalidParams) // 页面无 path
	assert.NoError(t, validateMenuRequiredFields(3, "", "user:delete")) // 按钮：有 permission
	assert.ErrorIs(t, validateMenuRequiredFields(3, "", ""), errcode.ErrInvalidParams) // 按钮无 permission
}

// B4-6 守护：审计分页 page 上限（防巨量 OFFSET 扫描）
func TestNormalizeAuditPageCap(t *testing.T) {
	assert.Equal(t, 10000, func() int { p, _ := normalizeAuditPage(2000000000, 20); return p }())
	assert.Equal(t, 1, func() int { p, _ := normalizeAuditPage(0, 20); return p }())
	_, ps := normalizeAuditPage(1, 500)
	assert.Equal(t, 100, ps)
}
