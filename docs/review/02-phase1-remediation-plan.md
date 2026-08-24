# Phase 1 检查发现修复计划

> **来源**：[01-phase1-systematic-review-findings.md](./01-phase1-systematic-review-findings.md)（60 项发现，两轮复核零误报）
> **范围决策**：P1/P2/P3 **全部 60 项在 Phase 1 收尾前修完**（负责人已确认）
> **制定日期**：2026-08-21
> **实施方式**：四批次串行推进，每批次独立提交、独立验证、可独立回滚

---

## 0. 决策点（已拍板，2026-08-21）

| # | 决策 | 结论 |
|---|------|------|
| D1 | 用户 Update 字段覆盖语义（R1-USER-01） | **已拍板：patch 语义**——只改传进来的字段，未传字段不动（请求字段指针化，空串显式清空）。前端未接入，改契约零成本 |
| D2 | 中间件顺序冲突（R2-INFRA-01） | **已拍板：改文档保留实现**（JWT→Audit→Casbin）——被拒请求进审计是安全价值，挂载结构不动，修订三份文档 + 补顺序守护测试 |

---

## 1. 批次总览

| 批次 | 主题 | 项数 | 验收门槛 |
|------|------|------|----------|
| **B1** | 安全收尾（P1×2 + 上线前必改×2） | 4 | 集成测试 + 验收脚本新断言全绿 |
| **B2** | 契约对齐（P2 剩余 9 项 + 关联 P3） | 11 | 文档-代码逐项复核一致 |
| **B3** | 并发与一致性加固（P2×5） | 5 | 集成测试含并发场景 |
| **B4** | P3 全量打磨（40 项，按模块分组） | 40 | go vet + 全量测试 + 抽查 |

> 合并实施说明：部分 P3 与 P2 同文件同类型（如错误码 30010 同时解决 R2-RM-01 与 R1-USER-03），在对应批次一并完成，B4 只做剩余项。

---

## 2. B1：安全收尾（第一批，最先做）

### B1-1｜R1-AUTHZ-01（P1）角色禁用全链路生效

**改动**：
- `internal/repository/user_repo.go`：`GetRoleCodes`（L394）、`GetRoles`（L419）SQL 加 `AND r.status = 1`；L333/L345/L445/L457/L536/L543 共 6 处 roles JOIN **逐处审计用途**——凡结果用于鉴权/菜单/优先级判断的加 status 过滤（列表筛选用途的除外，实施时逐个判断并在 PR 里列出审计结论）
- `internal/repository/role_repo.go`：`ListRoleIDsByUserID`（L235）加 `AND r.status = 1`
- `internal/service/rbac_service.go`：无需改（UpdateRole 置 0 的入口已存在）

**语义**：角色禁用后**下次请求起**生效（与「角色变更下次请求生效」既有语义一致）；superadmin/admin 角色本身不可禁用（`is_system` 已挡 UpdateRole）。

**文档**：05-role.md 新增「角色禁用语义」小节（status=0 → L1 鉴权/用户菜单/优先级档位均不再计入，下次请求生效）。

**验证**：repository 集成测试（禁用后 GetRoleCodes 为空）+ 验收脚本断言：禁用某角色 → 其成员访问该角色 API → 403。

### B1-2｜R2-RM-01（P1）+ R1-USER-03（P3）角色写操作目标校验 + 错误码通用化

**改动**：
- `internal/pkg/errcode/errcode.go`：新增 `ErrCannotManageHigher`（30010，「不能操作同级或更高级权限对象」，403）
- `internal/service/rbac_service.go`：
  - 新增 `ensureCanManageRole(ctx, actorUserID, target)`：`GetRoles(actor)` → `canManageTarget(actorRoles, []*model.Role{target})` 失败返回 30010
  - `DeleteRole`/`UpdateRole`：Find 后调用上述 helper（is_system 检查已有）
  - `AssignMenus`：目标 `IsSystem && !isSuperadmin(actor)` → `ErrRoleIsSystem`；再调 `ensureCanManageRole`（替换现有仅 superadmin 的特判——superadmin 经 canManageTarget 直通，语义等价且更完整）
- `internal/service/user_service.go`：`ensureCanManage` 返回值改为 30010；仅 `ResetPassword` 场景保留 30005 文案（新增 `ensureCanResetPassword` 或参数区分）

**文档**：errcode.md 补 30010；05-role.md「角色 priority」节补目标校验规则；04-user.md 错误码表同步。

**验证**：集成测试——低权角色（priority=25）对更强角色（priority=15）delete/update/assign → 403+30010；admin 对 admin 角色分配菜单 → 403。

### B1-3｜R2-INFRA-02（P2）DSN 密码 URL 编码

**改动**：`internal/config/config.go` `DSN()` 改用 `net/url` 构造：

```go
u := &url.URL{
    Scheme:   "postgres",
    User:     url.UserPassword(c.User, c.Password),
    Host:     fmt.Sprintf("%s:%d", c.Host, c.Port),
    Path:     c.DBName,
    RawQuery: "sslmode=" + url.QueryEscape(c.SSLMode),
}
return u.String()
```

**验证**：config 单测——密码含 `@ : / ? # %` 时 DSN 可被 `pgxpool.ParseConfig` 成功解析且字段正确。

### B1-4｜R1-AUTH-12（P3，升级必改）TrustedProxies

**改动**：
- `internal/config/config.go` `ServerConfig` 加 `TrustedProxies []string \`mapstructure:"trusted_proxies"\``
- `configs/config.yaml` 加示例（注释说明：Nginx 前置时填内网网段）
- `internal/router/router.go`：`NewRouter` 经 Deps 接收并调用 `r.SetTrustedProxies(cfg)`；**空值传 nil**（Gin 语义 = 不信任任何代理，ClientIP=RemoteAddr，安全默认）

**文档**：01-infra.md 配置表补 trusted_proxies；02-auth.md「IP 来源」节标注已落地。

**验证**：单测/手测——伪造 `X-Forwarded-For` 时审计 IP 不被污染。

---

## 3. B2：契约对齐（第二批）

### B2-1｜R1-AUTH-01 JWT 过期 20002

- `internal/middleware/jwt.go` L41-46：`errors.Is(err, jwt.ErrTokenExpired)` → `UnauthorizedError(c, errcode.ErrTokenExpired)`，否则 `ErrTokenInvalid`
- 删除 `internal/service/auth_service.go` L302-305 `ParseTokenExpired`（死代码，中间件直接用 errors.Is）
- 验收脚本断言：过期 AT → 401 + code 20002

### B2-2｜R1-AUTH-02 改密新旧相同校验

- `internal/service/auth_service.go` `UpdatePassword`：旧密码校验通过后加 `oldPassword == newPassword → ErrInvalidParams`（400）
- 测试：02-auth.md:771 用例已有，补集成测试/验收断言

### B2-3｜R1-USER-01 + R1-USER-11 Update patch 语义（D1 已拍板：patch 语义）

- `internal/model/user_request.go`：`UpdateUserRequest` 的 `EmployeeNo/DomainAccount/UserDomain/RealName/Email/Phone/Avatar` 改 `*string`；**`Username` 字段直接移除**（文档「Phase 2 再定改名流程」，R1-USER-11 一并解决）；`UpdateProfileRequest` 同型处理
- `internal/service/user_service.go` `Update`/`UpdateProfile`：nil 跳过赋值（保持 FindByID 的原值），非 nil 空串 = 显式清空；SQL **零改动**（合并进 user 对象后仍全量写）
- 文档：04-user.md 示例加一句「未传字段保持不变，传空串显式清空」
- 验证：集成测试——部分字段请求后未传字段原值不变

### B2-4｜R2-RM-02 用户菜单 visible 过滤

- `internal/service/menu_service.go` `filterMenusForTree` 加 `if !m.Visible { continue }`（在补链之后执行：父不可见子可见时子提升为根，加注释说明该取舍）
- 验收断言：visible=false 菜单不出现在 `GET /user/menus`

### B2-5｜R2-RM-03 AssignMenus 菜单存在性校验

- `internal/service/rbac_service.go` `AssignMenus`：调 repo 前用 `menuRepo.ListByIDs(menuIDs)` 预检，ID 集合不全 → `ErrMenuNotFound`（404）
- `internal/repository/role_repo.go`：INSERT 改 `INSERT INTO role_menus (role_id, menu_id) SELECT $1, id FROM menus WHERE id = $2 AND deleted_at IS NULL ON CONFLICT DO NOTHING`（消灭预检后软删的 TOCTOU 残余窗口）
- 验证：集成测试——不存在 ID → 404；软删菜单 → 404

### B2-6｜R2-RM-05 影子超管读路径

- `internal/service/rbac_service.go`：`GetRole`/`GetRoleMenuIDs`/`GetRolePermissions` 签名加 `actorUserID int64`；目标 `code == "superadmin"` 且 actor 非 superadmin（service 内 `GetRoles` 判断，不依赖中间件 context）→ `ErrRoleNotFound`（404 防推断）
- `internal/handler/role_handler.go`：三处调用传 `c.GetInt64("userID")`
- 文档：05-role.md 影子超管节补「详情/菜单/策略读接口同样 404」
- 验证：admin token 访问 superadmin 角色（ID=1）三接口 → 404

### B2-7｜R2-INFRA-01 中间件顺序（D2 已拍板：改文档保留实现）

- 修订 `docs/phase1/09-middleware.md`：顺序表（JWT 7 / Audit 8 / Casbin 9）与伪代码改为实际链，注明「Audit 前置于 Casbin：被拒请求同样落审计」
- 修订 `docs/modules/middleware.md` §2.3 同步
- 修订 `docs/phase1/08-audit.md`：「未认证请求 user_id=NULL」用例改为「未认证请求被 JWT Abort 短路不产生审计记录；认证失败由 LogLogin 显式记录」
- `internal/router/router_test.go`：补中间件顺序守护测试（断言 Audit 先于 Casbin 执行）

### B2-8｜R1-USER-02 文档用例修正

- `docs/phase1/04-user.md` L650：`{"id":"1"}` → `{"user_id":"1"}`

### B2-9｜R1-USER-09 列表契约对齐

- 修文档（实现不动）：04-user.md 列表字段说明改为「与详情同构，返回完整 User 结构」

---

## 4. B3：并发与一致性加固（第三批）

### B3-1｜R2-ORG-01 AddMember 幂等不降级

- `internal/repository/org_repo.go` L87-90：upsert 改

```sql
ON CONFLICT (user_id, org_id) DO UPDATE
SET is_primary = EXCLUDED.is_primary
WHERE EXCLUDED.is_primary   -- 仅 primary=true 时提升，false 不回写
```

- 验证：集成测试——重复添加未传 is_primary，原 primary 保持

### B3-2｜R2-ORG-02 + R2-ORG-07 Move 事务化重构

- `internal/repository/org_repo.go` `Move` 重构为**单事务完整流程**：
  1. `SELECT pg_advisory_xact_lock(hashtext('org:move'))`（全局串行化 move，项目已有 AcquireSuperadminGuard 同款先例；move 低频管理操作，粗粒度可接受）
  2. 事务内重读 `org`（被移动节点）path；`FOR UPDATE` 锁旧子树（`WHERE path <@ old AND deleted_at IS NULL`，顺带修 R2-ORG-07 谓词过滤）
  3. 有新父：事务内读父行 + 环检测（`child.path <@ newParent.path` SQL 版，ancestor 侧补 deleted_at 过滤）
  4. 现有 UPDATE（谓词补 `AND deleted_at IS NULL`）+ **RowsAffected 校验**：0 行 → `ErrConcurrentModification`
- `internal/service/org_service.go` `Move`：瘦身为参数校验 + 委托（环检测/父 path 读取逻辑移入 repo 事务）
- 文档：06-organization.md Move 流程图更新（advisory lock + 事务内检测 + 行数校验）
- 验证：集成测试——移到自身子孙 → 400；并发交叉移动（两 goroutine）→ 一成一败、树不变量保持

### B3-3｜R2-ORG-03 primary 唯一索引兜底

- 新迁移 `migrations/000007_user_orgs_single_primary.up.sql`：
  1. 存量修复：对每 user 多条 primary 的，保留 id 最小一条，其余 `SET is_primary = false`
  2. `CREATE UNIQUE INDEX idx_user_orgs_single_primary ON user_orgs(user_id) WHERE is_primary`
- `internal/repository/pgerr.go`：`mapUniqueViolation` 补该约束名 → 409（新增 `ErrDuplicatePrimaryOrg` 50008 或并入 errcode.md 50xxx 分段后选号）
- down 迁移：DROP INDEX
- 验证：集成测试——并发 AddMember 同 user 双 primary → 第二个 409

### B3-4｜R2-ORG-04 + R1-USER-06 SetUserOrgs 去重

- `internal/repository/org_repo.go` `SetUserOrgsTx` 开头去重（map 保序去重，一处覆盖 `POST /users/orgs`、`POST /users`、组织侧三个入口）
- 验证：集成测试——`org_ids` 含重复 → 200，且 user_orgs 无重复行

### B3-5｜R2-RM-04 LoadPolicy 失败处理

- `internal/service/rbac_service.go` `DeleteRole`/`AssignMenus`：LoadPolicy 失败 → `slog.Error("casbin reload failed", "role", code)` + 重试 1 次（100ms）→ 仍失败返回新 errcode（`ErrPolicyReloadFailed`，按 errcode.md 分段规则选号，语义「DB 已生效、内存策略刷新失败，请重试或稍后自动恢复」，HTTP 500）
- 文档：05-role.md 链路图补失败语义；errcode.md 补码
- 验证：单测 mock enforcer 失败路径（enforcer 为接口可测性较好；若为具体类型则日志断言 + 代码走查）

---

## 5. B4：P3 全量打磨（第四批，按模块分组）

### 5.1 认证（10 项）

| ID | 修改 |
|----|------|
| R1-AUTH-03 | `auth_service.go` 登录用户不存在分支加 dummy bcrypt（包级固定 hash 常量 `CheckPassword(dummyHash, req.Password)`）拉平时延 |
| R1-AUTH-04 | `authz_service.go` stub 加注释「Phase 2a 预留（02-authz-resource.md Step 0 删除），勿调用」 |
| R1-AUTH-05 | `model/token.go` `DeviceInfo` 加注释「Phase 2 设备管理预留」 |
| R1-AUTH-06 | 修 `modules/auth.md` §5.1：顺序改为「锁定检查→用户查询→状态检查→密码验证」，注明禁用/不存在也计数（实现更严格，以实现为准） |
| R1-AUTH-07 | `middleware/jwt.go`：无 Bearer 且存在 `X-AK-*` 头 → 401 + message「暂不支持该认证方式」（code 维持 10002，20009 待 Phase 3 M2M 落地） |
| R1-AUTH-08 | `user_repo.go` `FindByEmployeeNo` SQL 补 `AND employee_no <> ''` |
| R1-AUTH-09 | `auth_service.go` 登录成功后 `UpdateLastLogin`/`issueTokenPair` 失败分支补 `LogLogin(..., 500)` |
| R1-AUTH-10 | `auth_handler.go` `Logout`：body 解析失败 → 400（不再忽略错误） |
| R1-AUTH-11 | `middleware/jwt.go` L74 硬编码路径改 `c.FullPath() == "/api/v1/auth/password/update"` |
| R1-AUTH-13 | `auth_handler.go` `writeAuthError` 补 `case errcode.ErrUserNotFound.Code: NotFoundError` |
| R1-AUTH-14 | `middleware/jwt.go` 黑名单 + disabled 两次 `Exists` 改 `rdb.Pipeline()` 一次往返 |

### 5.2 授权（2 项）

| ID | 修改 |
|----|------|
| R1-AUTHZ-02 | 修 `03-authz.md` 伪代码与 `modules/authz.md` §2.2.1：回写 SelfService 路由组标签方案（注明路径白名单方案已废弃） |
| R1-AUTHZ-03 | 删除 `middleware/casbin.go` `CasbinPassThrough`（全仓无引用，挂上即绕过 RBAC） |

### 5.3 用户（8 项）

| ID | 修改 |
|----|------|
| R1-USER-04 | `UpdateStatus` 加 `userID == actorUserID → ErrForbidden`（不能禁用自己）；04-user.md 补承诺 |
| R1-USER-05 | `UpdateStatus` 加 `user.IsSystem → ErrUserIsSystem`（与 Delete 保护对齐）；04-user.md 补承诺 |
| R1-USER-07 | `pgerr.go` 补 23503 → `ErrInvalidParams`（400，「关联对象不存在或已被删除」） |
| R1-USER-08 | `SetRoles` 第一循环记录 `role.Code == "superadmin"`，删除第二循环 |
| R1-USER-10 | `user_repo.go` `SetRolesTx` INSERT 改 `INSERT ... SELECT ... WHERE deleted_at IS NULL` + 计数比对（与 B2-5 同型防御） |

### 5.4 角色菜单（4 项）

| ID | 修改 |
|----|------|
| R2-RM-06 | `menu_repo.go` `Delete` 事务化：软删 + 同事务 `DELETE FROM role_menus WHERE menu_id`（级联策略重建仍按文档留在 Phase 2） |
| R2-RM-07 | `CreateRoleRequest.Status` 改 `*int`（nil → 1），`rbac_service.go` 相应调整（显式传 0 可创建禁用角色） |
| R2-RM-08 | `role_handler.go` AssignMenus 绑定错误改 `errcodeInvalidParams(c)` |
| R2-RM-09 | `GetUserPermissions`：含 admin/superadmin 角色时基于 `ListAll` 展开全部权限码（与 Casbin matcher bypass 对齐） |
| R2-RM-10 | `menu_service.go` Create/Update：menu_type=2 强制 path 非空；type=3 强制 permission 非空 |

### 5.5 组织（4 项）

| ID | 修改 |
|----|------|
| R2-ORG-05 | `org_repo.go` `Delete` 事务化：同事务内 count children/members + 软删（消灭 check-then-act 窗口） |
| R2-ORG-06 | `org_request.go`：Code 加 `max=50`、Name `max=100`（对齐 DB varchar） |
| R2-ORG-08 | `GetMembers` 加分页（复用 normalizePage 风格，handler 接 page/page_size）；`modules/organization.md` §4.3 签名同步 |
| R2-ORG-09 | 修 `modules/organization.md` §3：`GetUserOrgs` 返回类型改 `[]*model.UserOrg` |
| R2-ORG-10 | 新增 `ErrOrgIsSystemProtected`（「系统内置组织受保护」），Update 场景与 Delete 的 `ErrOrgIsSystem` 区分 |

### 5.6 基础设施（8 项）

| ID | 修改 |
|----|------|
| R2-INFRA-03 | `wire.go` Registry/AuthzService 两 provider 加注释「Phase 2 接线预留，当前无消费者」 |
| R2-INFRA-04 | `app.go` Shutdown 的 TODO 注释清理：删 TODO 2；TODO 3-5 改说明性注释（资源关闭由 main.go defer cleanup 逆序执行） |
| R2-INFRA-05 | `audit_handler.go`：`start > end` → 400「开始日期不能晚于结束日期」 |
| R2-INFRA-06 | `audit_service.go` `normalizeAuditPage` 加 page 上限（≤10000，超限 400）；注明 Phase 2 统一各模块分页上限 |
| R2-INFRA-07 | `main.go` 重构：业务逻辑移入 `run()`（内部 defer cleanup），main 仅调 run + os.Exit——错误路径不再跳过清理 |
| R2-INFRA-08 | `middleware/logger.go` `AccessLogger` 跳过 `/health/live`、`/health/ready` |
| R2-INFRA-09 | 修 `09-middleware.md`/`modules/middleware.md`：RequestID/AccessLogger 选型说明改自写实现，request_id 格式改「req- + 32 hex」 |
| R2-INFRA-10 | `config/validate.go` 扩展：AccessTTL>0 且 ≤ RefreshTTL、Port ∈ (0,65535]、Database.Host/DBName、Redis.Host 非空 |
| R2-INFRA-11 | 修 `modules/audit.md` Phase 1 清单：request_id/trace_id 移至 Phase 3a 条目 |

---

## 6. 测试与验收策略

| 层 | 内容 |
|----|------|
| 单元测试 | config DSN 编码、pgerr 新映射、filterMenusForTree visible、normalizeAuditPage 上限、validate 扩展规则 |
| 集成测试（testdb） | 角色禁用生效链、目标校验 403 矩阵、AddMember 幂等、SetUserOrgs 去重、primary 唯一索引 409、AssignMenus 404、Move 并发交叉 |
| 验收脚本 | `acceptance-phase1.sh` 新增断言：①禁用角色→成员 403；②过期 AT→20002；③admin 读 superadmin 角色→404；④visible=false 不下发；⑤改密新旧相同→400 |
| 静态检查 | `go vet ./...`、`go test ./... -race -count=1` |
| 文档核对 | 每批次完成后按上文「文档」条目逐项核对，保持文档-代码一致（吸取本轮检查教训） |

## 7. 提交策略

- 每批次 1-3 个 commit，沿用现有消息风格（`fix(...)`/`docs(...)`/`test(...)`）
- B1 拆两笔：安全校验（B1-1/B1-2）+ 基础设施（B1-3/B1-4）
- B3-3 迁移单独一笔（含存量数据修复，便于回滚定位）
- 每笔提交前跑全量测试

## 8. 风险与依赖

| 风险 | 缓解 |
|------|------|
| B1-1 的 6 处 roles JOIN 用途判断失误（误过滤列表筛选场景） | 实施时逐处注释用途，PR 描述列出审计结论 |
| B2-3 patch 化改契约（D1 未拍板） | 拍板前不动工；已给推荐 |
| B3-2 Move 重构工作量最大 | advisory lock 方案已选定，SQL 流程本文已给全 |
| B3-3 迁移动存量数据 | 修复 SQL 先 SELECT 预览再执行；备份 |
| B4 量大但均为低风险小改 | 分模块小批量提交，每模块一笔 |

## 9. 执行顺序建议

```
B1（4 项，安全收尾）→ B2（9 项，拍板 D1/D2 后）→ B3（5 项，并发加固）→ B4（40 项，分 6 个模块小批量）
```

B1 与 B2 无依赖可并行；B3-2 依赖无；B4 各模块间无依赖。全程预计 6-8 个工作日内完成（含测试与文档同步）。
