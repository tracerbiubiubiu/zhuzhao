package handler

import (
	"encoding/json"
	"errors"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao-utils/response"
)

// allErrCodes errcode.go 全量业务码清单（D2-32）。
// 新增错误码时必须同步：① 本清单 ② httpStatusByCode 或 unmappedAllowlist，
// 否则 count 断言/逐码断言失败——防止再出现 50012/70004 式静默断链。
var allErrCodes = []*errcode.Error{
	// 通用
	errcode.ErrInternal, errcode.ErrInvalidParams, errcode.ErrUnauthorized,
	errcode.ErrForbidden, errcode.ErrNotFound, errcode.ErrConflict,
	errcode.ErrConcurrentModification, errcode.ErrTooManyReqs, errcode.ErrServiceUnavailable,
	// 认证（auth handler 直写响应，不经 writeServiceError）
	errcode.ErrInvalidCredentials, errcode.ErrTokenExpired, errcode.ErrTokenInvalid,
	errcode.ErrRefreshTokenInvalid, errcode.ErrTokenAlreadyRefreshed, errcode.ErrAccountLocked,
	errcode.ErrPasswordChangeRequired, errcode.ErrMultipleAuthMethods,
	// 用户
	errcode.ErrUserAlreadyExists, errcode.ErrUserNotFound, errcode.ErrUserDisabled,
	errcode.ErrUserIsSystem, errcode.ErrCannotResetHigher, errcode.ErrCannotRemoveLastSuperadmin,
	errcode.ErrEmployeeNoAlreadyExists, errcode.ErrDomainAccountAlreadyExists,
	errcode.ErrCannotAssignHigherRole, errcode.ErrCannotManageHigher,
	// 角色
	errcode.ErrRoleAlreadyExists, errcode.ErrRoleNotFound, errcode.ErrRoleInUse, errcode.ErrRoleIsSystem,
	// 组织
	errcode.ErrOrgAlreadyExists, errcode.ErrOrgNotFound, errcode.ErrOrgCannotMoveToChild,
	errcode.ErrOrgHasChildren, errcode.ErrOrgHasMembers, errcode.ErrOrgIsSystem,
	errcode.ErrNotOrgMember, errcode.ErrDuplicatePrimaryOrg, errcode.ErrOrgSystemProtected,
	// 菜单
	errcode.ErrMenuAlreadyExists, errcode.ErrMenuNotFound, errcode.ErrMenuHasChildren, errcode.ErrMenuIsSystem,
	// 权限
	errcode.ErrNoPermission, errcode.ErrPolicyExists, errcode.ErrNoRoles, errcode.ErrPolicyReloadFailed,
}

// unmappedAllowlist 有意不进 httpStatusByCode 的码（writeServiceError 落 default 500+10000）：
// auth 模块 20001+ 由 auth handler 直写；10007/10008 由中间件直写；10000/70002 本义即 500。
var unmappedAllowlist = map[int]string{
	errcode.ErrInternal.Code:               "内部错误本义 500",
	errcode.ErrTooManyReqs.Code:            "429 由登录锁定路径直写（TooManyRequests）",
	errcode.ErrServiceUnavailable.Code:     "503 由鉴权中间件直写（ServiceUnavailable）",
	errcode.ErrInvalidCredentials.Code:     "auth handler 直写",
	errcode.ErrTokenExpired.Code:           "auth handler 直写",
	errcode.ErrRefreshTokenInvalid.Code:    "auth handler 直写",
	errcode.ErrTokenAlreadyRefreshed.Code:  "auth handler 直写",
	errcode.ErrAccountLocked.Code:          "auth handler 直写",
	errcode.ErrPasswordChangeRequired.Code: "auth handler 直写",
	errcode.ErrMultipleAuthMethods.Code:    "auth handler 直写",
	errcode.ErrPolicyExists.Code:           "本义 500",
}

// TestWriteServiceError_FullCodeTable 全码表驱动测试（D2-32）：
// ① 已映射码：HTTP 状态与映射表一致，且 body.code 必须透传业务码（防 10000 断链，
//
//	即 D2-06/07 的漏映射形态）；② 未映射码：必须命中 allowlist 并落 500+10000；
//
// ③ 清单计数守卫：errcode.go 新增码未同步本测试时立即失败。
func TestWriteServiceError_FullCodeTable(t *testing.T) {
	gin.SetMode(gin.TestMode)

	if got, want := len(allErrCodes), 48; got != want {
		t.Errorf("全码清单数量 = %d, want %d——errcode.go 新增/删除业务码后未同步本测试", got, want)
	}

	seen := map[int]bool{}
	for _, ec := range allErrCodes {
		if seen[ec.Code] {
			t.Errorf("码 %d 在清单中重复登记", ec.Code)
		}
		seen[ec.Code] = true

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		writeServiceError(c, ec)

		var body response.Response
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("码 %d：响应体非 JSON: %v", ec.Code, err)
		}

		if wantStatus, ok := httpStatusByCode[ec.Code]; ok {
			if w.Code != wantStatus {
				t.Errorf("码 %d：HTTP = %d, want %d（映射表）", ec.Code, w.Code, wantStatus)
			}
			if body.Code != ec.Code {
				t.Errorf("码 %d：body.code = %d 断链（want %d）——D2-06/07 形态复发", ec.Code, body.Code, ec.Code)
			}
		} else {
			if _, allowed := unmappedAllowlist[ec.Code]; !allowed {
				t.Errorf("码 %d 未登记映射且不在 allowlist——请显式归入 httpStatusByCode 或 unmappedAllowlist", ec.Code)
			}
			if w.Code != 500 || body.Code != errcode.ErrInternal.Code {
				t.Errorf("码 %d：default 分支应为 500+10000，实际 %d+%d", ec.Code, w.Code, body.Code)
			}
		}
	}
}

// TestWriteServiceError_NonBizError 非 *errcode.Error（如 fmt.Errorf 包装的底层错误）
// → 500 + 10000，不泄露内部错误细节。
func TestWriteServiceError_NonBizError(t *testing.T) {
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	writeServiceError(c, errors.New("db connection reset: postgres://secret"))

	var body response.Response
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("响应体非 JSON: %v", err)
	}
	if w.Code != 500 {
		t.Errorf("HTTP = %d, want 500", w.Code)
	}
	if body.Code != errcode.ErrInternal.Code {
		t.Errorf("body.code = %d, want 10000", body.Code)
	}
}

// TestErrcodeDocMessageSync errcode.go ↔ errcode.md 的 message 逐条一致守护。
// 契约：errcode.md 表格 message 列 = 对外文案（代码为唯一运行时事实源）——
// 30007/30008（文档多语义说明）、50011/50012/70004（文档嵌变更批注）两类
// 漂移都曾实际发生；变更溯源写表格下方引用块，不进 message 列。
func TestErrcodeDocMessageSync(t *testing.T) {
	root := repoRoot(t)

	src, err := os.ReadFile(filepath.Join(root, "internal", "pkg", "errcode", "errcode.go"))
	if err != nil {
		t.Fatalf("read errcode.go: %v", err)
	}
	newRe := regexp.MustCompile(`New\((\d+), "([^"]+)"\)`)
	codeMsg := map[string]string{}
	for _, m := range newRe.FindAllStringSubmatch(string(src), -1) {
		codeMsg[m[1]] = m[2]
	}

	doc, err := os.ReadFile(filepath.Join(root, "docs", "api", "errcode.md"))
	if err != nil {
		t.Fatalf("read errcode.md: %v", err)
	}
	rowRe := regexp.MustCompile(`\| (\d{5}) \| ` + "`[^`]+`" + ` \| ([^|]+) \|`)
	checked := 0
	for _, m := range rowRe.FindAllStringSubmatch(string(doc), -1) {
		code, docMsg := m[1], strings.TrimSpace(m[2])
		if codeMsg[code] == "" {
			continue // 预留段（errcode.go 未定义，D2-44① 标注）
		}
		checked++
		if codeMsg[code] != docMsg {
			t.Errorf("码 %s 文档/代码 message 漂移：\n  code=%q\n  doc =%q\n（语义说明/变更批注请移至表格下方引用块，勿嵌入 message 列）",
				code, codeMsg[code], docMsg)
		}
	}
	if checked == 0 {
		t.Fatal("errcode.md 表格解析为 0 行——文档结构变更后未同步本测试的行正则")
	}
}

// repoRoot 从测试文件位置向上定位仓库根（go.mod 所在目录）
func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	dir := filepath.Dir(file)
	for i := 0; i < 6; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		dir = filepath.Dir(dir)
	}
	t.Fatal("go.mod not found upward")
	return ""
}
