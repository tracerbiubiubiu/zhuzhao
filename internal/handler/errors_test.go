package handler

import (
	"encoding/json"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/tracerbiubiubiu/zhuzhao-utils/response"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
)

// allErrCodes errcode.go 全量业务码清单（D2-32）。
// 新增错误码时必须同步：① 本清单 ② httpStatusByCode 或 unmappedAllowlist，
// 否则 count 断言/逐码断言失败——防止再出现 50012/70004 式静默断链。
var allErrCodes = []*errcode.Error{
	// S4 B4-1：动作未注册（fail-fast）
	errcode.ErrActionNotRegistered,
	// S9 B9-1：情报缓存段
	errcode.ErrIntelNotConfigured, errcode.ErrIntelUpstreamQuota, errcode.ErrIntelUpstream,
	// 通用
	errcode.ErrInternal, errcode.ErrInvalidParams, errcode.ErrUnauthorized,
	errcode.ErrForbidden, errcode.ErrNotFound, errcode.ErrConflict,
	errcode.ErrConcurrentModification, errcode.ErrTooManyReqs, errcode.ErrServiceUnavailable,
	// 认证（auth handler 直写响应，不经 writeServiceError）
	errcode.ErrInvalidCredentials, errcode.ErrTokenExpired, errcode.ErrTokenInvalid,
	errcode.ErrRefreshTokenInvalid, errcode.ErrTokenAlreadyRefreshed, errcode.ErrAccountLocked,
	errcode.ErrPasswordChangeRequired, errcode.ErrMultipleAuthMethods,
	errcode.ErrPasswordChanged, errcode.ErrRefreshTokenReplayed,
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
	errcode.ErrCannotAssignHigherOrgMemberRole, errcode.ErrCannotManageOrgMember,
	errcode.ErrNotOrgOwner, errcode.ErrOrgHasOpenTickets,
	// 菜单
	errcode.ErrMenuNotFound,
	// 权限
	errcode.ErrNoPermission, errcode.ErrPolicyExists, errcode.ErrNoRoles, errcode.ErrPolicyReloadFailed,
	// 工单
	errcode.ErrTicketNotFound, errcode.ErrTicketInvalidTransition,
	errcode.ErrTicketTypeNotFound, errcode.ErrTicketAlreadyClosed,
	// 通知（P4-2）/字典（P4-3）/PAT（P4-6）——审计修复 2026-09-30 补登记
	errcode.ErrNotificationConfigNotFound, errcode.ErrNotificationCodeExists, errcode.ErrNotificationChannelNotSupport,
	errcode.ErrNotificationNotConfigured,
	errcode.ErrDictTypeNotFound, errcode.ErrDictItemNotFound, errcode.ErrDictTypeInUse,
	// S6 B6-3 消息模板段（外审 F-1-4 修复）
	errcode.ErrMsgTplNotFound, errcode.ErrMsgTplDupCode, errcode.ErrMsgTplSystem,
	errcode.ErrMsgTplReferenced, errcode.ErrMsgTplVersionConflict, errcode.ErrMsgRuleVersionConflict,
	errcode.ErrPatNotFound,
	// 对象存储（phase5 S5 B5-1）
	errcode.ErrFileTooLarge, errcode.ErrFileTypeNotAllowed, errcode.ErrFileNotFound, errcode.ErrFileAlreadyBound,
}

// unmappedAllowlist 有意不进 httpStatusByCode 的码（writeServiceError 落 default 500+10000）：
// 现役认证码由 auth/JWT 直写，10007/10008 由中间件直写；兼容定义20005/70002无生产出口。
var unmappedAllowlist = map[int]string{
	errcode.ErrInternal.Code:               "内部错误本义 500",
	errcode.ErrTooManyReqs.Code:            "429 由登录锁定路径直写（TooManyRequests）",
	errcode.ErrServiceUnavailable.Code:     "503 由鉴权中间件直写（ServiceUnavailable）",
	errcode.ErrInvalidCredentials.Code:     "auth handler 直写",
	errcode.ErrTokenExpired.Code:           "auth handler 直写",
	errcode.ErrRefreshTokenInvalid.Code:    "auth handler 直写",
	errcode.ErrTokenAlreadyRefreshed.Code:  "仅兼容定义，无当前生产出口，默认降级",
	errcode.ErrAccountLocked.Code:          "auth handler 直写",
	errcode.ErrPasswordChangeRequired.Code: "auth handler 直写",
	errcode.ErrMultipleAuthMethods.Code:    "auth handler 直写",
	errcode.ErrPasswordChanged.Code:        "auth handler 直写",
	errcode.ErrRefreshTokenReplayed.Code:   "auth handler 直写",
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

	errcodeNames(t) // 真实定义↔运行时清单双向核对，不能只锁清单自身的固定长度

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

// TestErrcodeDocMessageSync 定义、登记、默认文案、HTTP四向对账。
// B12：仅遍历文档已有行会漏掉整段丢失；必须从实际定义出发，也拒绝幽灵登记。
func TestErrcodeDocMessageSync(t *testing.T) {
	gin.SetMode(gin.TestMode)
	names := errcodeNames(t)
	rows := readErrcodeRegistry(t)
	for i, ec := range allErrCodes {
		row, ok := rows[ec.Code]
		if !ok {
			t.Errorf("缺少已定义码登记：%d %s", ec.Code, names[i])
			continue
		}
		delete(rows, ec.Code)
		if row.name != names[i] || row.message != ec.Message {
			t.Errorf("码%d登记漂移：name=%q/message=%q，want %q/%q", ec.Code, row.name, row.message, names[i], ec.Message)
		}
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		// auth/中间件直写不走默认service映射，使用其实际渲染函数。
		switch ec.Code {
		case errcode.ErrInvalidCredentials.Code, errcode.ErrRefreshTokenInvalid.Code,
			errcode.ErrAccountLocked.Code, errcode.ErrPasswordChanged.Code, errcode.ErrRefreshTokenReplayed.Code:
			writeAuthError(c, ec)
		case errcode.ErrTokenExpired.Code:
			response.UnauthorizedError(c, ec)
		case errcode.ErrPasswordChangeRequired.Code:
			response.ForbiddenError(c, ec)
		case errcode.ErrMultipleAuthMethods.Code:
			response.Error(c, 400, ec)
		case errcode.ErrTooManyReqs.Code:
			response.TooManyRequests(c, ec)
		case errcode.ErrServiceUnavailable.Code:
			response.ServiceUnavailable(c)
		default:
			writeServiceError(c, ec)
		}
		if row.http != w.Code {
			t.Errorf("码%d登记HTTP=%d，实际所属出口=%d", ec.Code, row.http, w.Code)
		}
	}
	for code, row := range rows {
		t.Errorf("登记无对应当前定义：%d %s（预留/退役码不得冒充现役）", code, row.name)
	}
}

type errcodeRegistryRow struct {
	name, message string
	http          int
}

func readErrcodeRegistry(t *testing.T) map[int]errcodeRegistryRow {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(repoRoot(t), "docs", "api", "errcode.md"))
	if err != nil {
		t.Fatal(err)
	}
	rows := map[int]errcodeRegistryRow{}
	for n, line := range strings.Split(string(b), "\n") {
		if !strings.HasPrefix(line, "|") {
			continue
		}
		cells := strings.Split(strings.Trim(line, "|"), "|")
		if len(cells) < 4 {
			continue
		}
		code, err := strconv.Atoi(strings.TrimSpace(cells[0]))
		if err != nil {
			continue
		}
		httpCode, err := strconv.Atoi(strings.TrimSpace(cells[3]))
		if err != nil {
			t.Fatalf("登记第%d行缺少HTTP：%s", n+1, line)
		}
		if _, exists := rows[code]; exists {
			t.Fatalf("登记码%d重复", code)
		}
		rows[code] = errcodeRegistryRow{
			name: strings.Trim(strings.TrimSpace(cells[1]), "`"), message: strings.TrimSpace(cells[2]), http: httpCode,
		}
	}
	return rows
}

// errcodeNames 把编译后的错误值与清单选择器逐位对应，再与真实导出的Err变量集合比较。
// 新增定义却不更新allErrCodes、只加清单却漏登记，两个方向都必须失败。
func errcodeNames(t *testing.T) []string {
	t.Helper()
	root := repoRoot(t)
	src, err := parser.ParseFile(token.NewFileSet(), filepath.Join(root, "internal/pkg/errcode/errcode.go"), nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	defined := map[string]bool{}
	for _, decl := range src.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.VAR {
			continue
		}
		for _, spec := range gen.Specs {
			for _, name := range spec.(*ast.ValueSpec).Names {
				if strings.HasPrefix(name.Name, "Err") {
					defined[name.Name] = true
				}
			}
		}
	}
	tests, err := parser.ParseFile(token.NewFileSet(), filepath.Join(root, "internal/handler/errors_test.go"), nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	ast.Inspect(tests, func(n ast.Node) bool {
		v, ok := n.(*ast.ValueSpec)
		if !ok || len(v.Names) != 1 || v.Names[0].Name != "allErrCodes" {
			return true
		}
		list, ok := v.Values[0].(*ast.CompositeLit)
		if !ok {
			t.Fatal("allErrCodes必须为显式错误变量清单")
		}
		for _, elt := range list.Elts {
			sel, ok := elt.(*ast.SelectorExpr)
			if !ok {
				t.Fatal("allErrCodes只能引用errcode导出变量")
			}
			pkg, ok := sel.X.(*ast.Ident)
			if !ok || pkg.Name != "errcode" || !defined[sel.Sel.Name] {
				t.Fatalf("清单引用重复或未定义码：%v", sel.Sel.Name)
			}
			delete(defined, sel.Sel.Name)
			names = append(names, sel.Sel.Name)
		}
		return false
	})
	for name := range defined {
		t.Errorf("已定义码未进入运行时清单：%s", name)
	}
	if len(names) != len(allErrCodes) || len(defined) != 0 {
		t.Fatalf("错误码定义/清单不完整：names=%d values=%d missing=%d", len(names), len(allErrCodes), len(defined))
	}
	return names
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
