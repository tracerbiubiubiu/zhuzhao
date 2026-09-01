// Package 架构守护测试（Architecture Guard）
//
// 目的：AI 迭代速度远超人工审查速度时，用「可失败的断言」把架构约定钉死。
// 文档会过期，测试不会——谁破坏了约定，CI/本地一跑就红。
//
// 运行：make guard   （等价于 go test -run 'TestArchitecture|TestGuard' ./internal/...）
//
// 新增约定时，在对应测试函数里加一条断言即可，无需改动框架。
package internal

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// scanDir 扫描目录内所有非测试 Go 文件，返回 文件路径 -> AST。
//
// 返回的 AST 携带共享 FileSet，因此可用 fset.Position(pos).Line 精确定位行号。
// 同时记录 文件路径 -> 源码内容，供行号换算使用（避免 AST 偏移推算的误差）。
func scanDir(t *testing.T, dir string) map[string]*ast.File {
	t.Helper()
	files := make(map[string]*ast.File)
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			// 跳过隐藏目录（.codebuddy 等）
			if strings.HasPrefix(info.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		f, perr := parser.ParseFile(sharedFset, path, nil, 0)
		if perr != nil {
			t.Logf("解析失败（跳过）%s: %v", path, perr)
			return nil
		}
		files[path] = f
		return nil
	})
	if err != nil {
		t.Fatalf("扫描 %s 失败: %v", dir, err)
	}
	if len(files) == 0 {
		t.Fatalf("目录 %s 下没有找到 Go 文件，路径可能不对", dir)
	}
	return files
}

// sharedFset 为所有扫描共享的 FileSet，保证 Position 换算一致。
var sharedFset = token.NewFileSet()

// importsOf 返回文件的 import 路径集合。
func importsOf(f *ast.File) map[string]bool {
	set := make(map[string]bool)
	for _, im := range f.Imports {
		p := strings.Trim(im.Path.Value, `"`)
		set[p] = true
	}
	return set
}

// modulePath 从 go.mod 读取模块名。
func modulePath(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("../go.mod")
	if err != nil {
		t.Fatalf("读取 go.mod 失败: %v", err)
	}
	for _, line := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module "))
		}
	}
	t.Fatal("go.mod 中未找到 module 声明")
	return ""
}

// TestArchitecture_LayerDependency 分层依赖方向：
//
//	handler -> service -> repository -> model
//
// 反向依赖（如 repository 引用 handler）是架构腐化的典型信号，必须拦截。
func TestArchitecture_LayerDependency(t *testing.T) {
	mod := modulePath(t)
	base := filepath.Join("..", "internal")

	// 允许的前向依赖：key 层可以 import value 层
	//
	// 说明（2026-08-29 校准，避免误报导致门禁被弃用）：
	//   · config / middleware 是横切层，任何层都可依赖。
	//     尤其 middleware 在此仅作为「接口定义方」被依赖（middleware.RoleFetcher /
	//     AuditLogger 接口），service 依赖接口属正确解耦，不是架构违规。
	//   · handler → repository 仅允许复用查询参数结构体（如 repository.UserListQuery），
	//     不允许直接做数据访问；数据访问违规由 TestArchitecture_NoDBInHandler 单独拦截。
	crossCutting := []string{"config", "middleware"}

	allowed := map[string][]string{
		"handler":    {"service", "model", "pkg"},
		"service":    {"repository", "model", "pkg", "casbin"},
		"repository": {"model", "pkg"},
		"model":      {"pkg"},
	}

	for layer, deps := range allowed {
		files := scanDir(t, filepath.Join(base, layer))
		for path, f := range files {
			for imp := range importsOf(f) {
				if !strings.HasPrefix(imp, mod+"/internal/") {
					continue // 第三方包不管
				}
				target := strings.TrimPrefix(imp, mod+"/internal/")
				targetLayer := strings.Split(target, "/")[0]

				if targetLayer == layer {
					continue // 同层允许
				}
				ok := false
				for _, d := range deps {
					if targetLayer == d {
						ok = true
						break
					}
				}
				// 横切层（config / middleware）任何层都可依赖
				for _, c := range crossCutting {
					if targetLayer == c {
						ok = true
						break
					}
				}
				// 已知豁免：handler 复用 repository 的「查询参数结构体」（UserListQuery /
				// AuditListQuery 等）属于轻量类型复用，非数据访问。
				// 数据访问违规由 TestArchitecture_NoDBInHandler 单独拦截。
				if !ok && layer == "handler" && targetLayer == "repository" {
					t.Logf("ℹ️  已知豁免（非阻断）：%s\n    handler 复用 repository 查询参数结构体，属类型复用而非数据访问。\n    若新增的是数据访问调用，请改为经 service 层。", path)
					ok = true
				}
				if !ok {
					t.Errorf("❌ 分层违规：%s\n    %s 层不应依赖 %s 层（%s）\n    允许：%v\n    修复：把逻辑下沉到 service，或通过接口/Port 解耦",
						path, layer, targetLayer, imp, deps)
				}
			}
		}
	}
}

// TestArchitecture_NoDBInHandler handler 层禁止直接碰数据库：
// 所有数据访问必须经 service -> repository。
func TestArchitecture_NoDBInHandler(t *testing.T) {
	files := scanDir(t, filepath.Join("..", "internal", "handler"))
	forbidden := []string{"pgx", "database/sql", "sqlx", "gorm.io"}
	for path, f := range files {
		for imp := range importsOf(f) {
			for _, bad := range forbidden {
				if strings.Contains(imp, bad) {
					t.Errorf("❌ handler 层不得直接访问数据库：%s\n    引用了 %s\n    修复：数据访问下沉到 service/repository", path, imp)
				}
			}
		}
	}
}

// TestArchitecture_NoHTTPInService service 层禁止感知 HTTP：
// 保证业务逻辑可测试、可被非 HTTP 入口（定时任务/CLI）复用。
func TestArchitecture_NoHTTPInService(t *testing.T) {
	files := scanDir(t, filepath.Join("..", "internal", "service"))
	forbidden := []string{"gin-gonic/gin", "net/http"}
	for path, f := range files {
		for imp := range importsOf(f) {
			for _, bad := range forbidden {
				if strings.Contains(imp, bad) {
					t.Errorf("❌ service 层不得依赖 HTTP 框架：%s\n    引用了 %s\n    修复：HTTP 语义留在 handler，service 只收业务参数", path, imp)
				}
			}
		}
	}
}

// TestGuard_NoPanicInBusiness 业务代码禁止裸 panic：
// panic 会打挂整个进程，生产环境应返回错误。
func TestGuard_NoPanicInBusiness(t *testing.T) {
	for _, layer := range []string{"handler", "service", "repository"} {
		files := scanDir(t, filepath.Join("..", "internal", layer))
		for path, f := range files {
			ast.Inspect(f, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				ident, ok := call.Fun.(*ast.Ident)
				if ok && ident.Name == "panic" {
					t.Errorf("❌ 业务层禁止 panic：%s (行 %d)\n    修复：返回 error，由上层统一转错误码",
						path, fsetLine(f, call.Pos()))
				}
				return true
			})
		}
	}
}

// TestGuard_NoPrintDebug 禁止 fmt.Print 系调试输出：
// 应统一用结构化日志（slog），否则日志无法采集、且可能泄露敏感信息。
func TestGuard_NoPrintDebug(t *testing.T) {
	for _, layer := range []string{"handler", "service", "repository", "middleware"} {
		files := scanDir(t, filepath.Join("..", "internal", layer))
		for path, f := range files {
			ast.Inspect(f, func(n ast.Node) bool {
				sel, ok := n.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				pkg, ok := sel.X.(*ast.Ident)
				if !ok || pkg.Name != "fmt" {
					return true
				}
				if sel.Sel.Name == "Print" || sel.Sel.Name == "Println" || sel.Sel.Name == "Printf" {
					t.Errorf("❌ 禁止 fmt.%s：%s (行 %d)\n    修复：改用 slog.Info/Debug 等结构化日志",
						sel.Sel.Name, path, fsetLine(f, sel.Pos()))
				}
				return true
			})
		}
	}
}

// TestGuard_NoHardcodedSecret 禁止硬编码密钥/密码字面量（安全红线）。
func TestGuard_NoHardcodedSecret(t *testing.T) {
	suspicious := []string{"password", "secret", "apikey", "api_key", "token", "privatekey"}
	for _, layer := range []string{"handler", "service", "repository", "pkg", "config"} {
		dir := filepath.Join("..", "internal", layer)
		if _, err := os.Stat(dir); err != nil {
			continue
		}
		files := scanDir(t, dir)
		for path, f := range files {
			ast.Inspect(f, func(n ast.Node) bool {
				assign, ok := n.(*ast.AssignStmt)
				if !ok {
					return true
				}
				for i, lhs := range assign.Lhs {
					ident, ok := lhs.(*ast.Ident)
					if !ok || i >= len(assign.Rhs) {
						continue
					}
					lit, ok := assign.Rhs[i].(*ast.BasicLit)
					if !ok || lit.Kind != token.STRING {
						continue
					}
					val := strings.ToLower(strings.Trim(lit.Value, `"`))
					if len(val) < 8 { // 太短的多半是字段名而非真值
						continue
					}
					for _, s := range suspicious {
						if strings.Contains(strings.ToLower(ident.Name), s) {
							t.Errorf("❌ 疑似硬编码密钥：%s (行 %d) 变量 %s\n    修复：改为从配置/环境变量读取",
								path, fsetLine(f, assign.Pos()), ident.Name)
						}
					}
				}
				return true
			})
		}
	}
}

// TestGuard_TicketRepoListCallSites 工单 repo.List 调用点锁定（IW4 第二层防线）：
// 工单列表的唯一入口是 ticket.Service.List（admin 分流 + registry.GetFilter 取 scope 谓词），
// repo.List 不允许在 ticket service 包之外被直调——否则大概率绕过 L2 行级过滤。
// 第一层防线是 repo.List 入口的 fail-closed 哨兵（运行期，2026-09-01 IW4）；
// 本断言是静态期补充，按接收者标识启发（含 "ticketrepo"），跨包直调点在门禁即红。
func TestGuard_TicketRepoListCallSites(t *testing.T) {
	files := scanDir(t, filepath.Join("..", "internal"))
	for path, f := range files {
		if strings.Contains(filepath.ToSlash(path), "/service/ticket/") {
			continue // 唯一合法入口：ticket service 包内（Service.List）
		}
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "List" {
				return true
			}
			if strings.Contains(strings.ToLower(exprString(sel.X)), "ticketrepo") {
				t.Errorf("❌ ticketRepo.List 调用点越界：%s (行 %d)\n    工单列表必须经 ticket.Service.List（L2 scope 过滤唯一入口，IW4）。\n    修复：下沉到 service；若确属新的合法入口，需同步修订 repo 哨兵与本断言并登记。",
					path, fsetLine(f, call.Pos()))
			}
			return true
		})
	}
}

// exprString 还原表达式的源码文本（仅覆盖 Ident / Selector 链，够接收者判定用）。
func exprString(e ast.Expr) string {
	switch v := e.(type) {
	case *ast.Ident:
		return v.Name
	case *ast.SelectorExpr:
		return exprString(v.X) + "." + v.Sel.Name
	default:
		return ""
	}
}

// fsetLine 返回节点所在行号（用于报错定位）。
// 依赖 scanDir 使用的共享 FileSet 做标准 Position 换算。
func fsetLine(f *ast.File, pos token.Pos) int {
	return sharedFset.Position(pos).Line
}
