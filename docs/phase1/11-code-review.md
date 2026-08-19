# Phase 1 代码审查记录（2026-08-19）

> **审查范围**：`feature/step-1-infra` 分支本地 3 个提交（Casbin 自服务标签重构 / 用户软删除级联清理 / APP_SERVER_MODE 修复）为核心的定向审查。
>
> **姊妹文档**：全分支深度审查见 [11-code-review-findings.md](./11-code-review-findings.md)（10 项发现 F-1~F-10，经本文档独立复核**全部属实**，其中 F-1 令牌类型混淆为最高优先级安全问题，本文档未覆盖）。两文档问题编号独立，F-5 与本文 §1.1 为同一问题。
>
> **审查结论**：`go build` / `go vet` / 现有单测全部通过；三个本地提交核心逻辑正确。本文档发现 1 个 P0、3 个 P1、5 个 P2（初版 6 个 P2 中的 §3.3 经复核为误报，移入 §4.2）。
>
> **修复状态（2026-08-19）**：§1.1 / §2.1 / §2.2 / §2.3 已全部修复（各节附说明）；姊妹文档 F-1~F-10 同日全部修复。§3.x 保持记录在案。
>
> **勘误（2026-08-19 复核）**：初版 §1.2（环境变量白名单陷阱）与 §3.3（登录无审计）经实测/复核为**误报**，已修正并记录于 §4，避免后人重复怀疑。

## 0. 已确认正确的部分

| 项 | 结论 |
|----|------|
| SelfService 标签方案 | `SelfService()` 与 `CasbinAuth` 同 group 顺序注册（`router.go:83`），中间件执行顺序正确；`CasbinAuth` 在 enforce 前检查标签 |
| SoftDelete 级联 | 事务内先删 `user_roles`/`user_orgs` 再软删，`RowsAffected()==0` 回滚并返回 404，原子性正确 |
| APP_SERVER_MODE 修复 | `SetEnvKeyReplacer` 使 `APP_SERVER_MODE` 可覆盖配置，实测生效；`gin.SetMode` 提前到 `InitializeApp` 前正确 |
| 环境变量覆盖机制（实测） | yaml 已存在键均可用 `APP_<SECTION>_<KEY>` 覆盖（见 §4.1），初版"白名单陷阱"结论有误 |
| superadmin 保护链 | 不能删自己（`ErrForbidden`）、不能删系统用户（`ErrUserIsSystem`）、最后一人拒绝（`ErrCannotRemoveLastSuperadmin`） |
| 登录审计 | `AuthService.Login` 各分支显式调用 `auditService.LogLogin`（成功/失败/锁定/参数错误均覆盖），有意不走中间件（见 §4.2） |
| Dockerfile 路径 | `WORKDIR /app` + `COPY configs/` 与 `config.Load("configs/config.yaml")` 硬编码相对路径匹配 |
| 审计脱敏 | `maskSensitive` 覆盖 password/old_password/new_password/secret/token |

---

## 1. P0（建议 Step 11 验收前必须修复）

### 1.1 审计日志使用 request context，客户端断开即丢失

```go
64:internal/middleware/audit.go
		// 同步写入 DB，失败只记应用日志，不影响业务
		if err := auditLogger.Insert(c.Request.Context(), entry); err != nil {
```

`c.Next()` 返回后若客户端已断开，request context 已取消，`Insert` 必然失败——**恰恰是异常场景（攻击者发完删除请求立刻断连）下审计最不该丢**。优雅关闭期间同理。

**修复方向**：改用 `context.WithoutCancel(c.Request.Context())` + 独立超时（如 3s）：

```go
ctx, cancel := context.WithTimeout(context.WithoutCancel(c.Request.Context()), 3*time.Second)
defer cancel()
if err := auditLogger.Insert(ctx, entry); err != nil { ... }
```

> 注：`context.WithoutCancel` 需 Go 1.21+，本项目 go.mod 满足。
> 本条与姊妹文档 **F-5 为同一问题**，修复后请在两文档同步更新状态。

**状态**：✅ 已修复（2026-08-19）— `audit.go` 按上述方向改为 `WithoutCancel` + 3s 独立超时；新增 `audit_test.go` 验证请求 context 取消后审计仍写入。姊妹文档 F-5 状态已同步。

---

## 2. P1（建议 Phase 1 收尾处理）

### 2.1 Casbin enforce 错误与 403 拒绝均无日志

```go
64:75:internal/middleware/casbin.go
		for _, role := range roles {
			subject := fmt.Sprintf("role::%s", role)
			ok, err := enforcer.Enforce(subject, path, method)
			if err != nil {
				enforceErr = err
				continue
```

- enforce 报错（adapter 故障、模型错误）被静默吞掉，最终以 503 返回，**无任何日志**；
- 被拒请求（403 + 70001）也不留痕。

中间件未注入 logger。安全系统里「谁在何时被拒绝了什么」是排查与审计刚需（参照 08-audit.md 登录审计的定位）。

**修复方向**：`CasbinAuth` 增加可选 `*slog.Logger` 参数（或走 `gin.DefaultErrorWriter`/slog 全局），enforce 错误记 Error，最终 deny 记 Warn（含 userID/path/method/roles）。

**状态**：✅ 已修复（2026-08-19）— `CasbinAuth` 增加 logger 参数（`router.go` 注入），enforce 错误记 Error（subject/path/method/error），最终 deny 记 Warn（userID/username/path/method/roles）。

### 2.2 最后 superadmin 保护存在 TOCTOU 竞态

`UserService.Delete`（`user_service.go:203`）、`UpdateStatus`（`:225`）、`SetRoles`（`:279`）中 `CountActiveSuperadminUsers` 的检查与后续写操作**不在同一事务**。两个并发请求可同时通过 `n<=1` 检查（各读到 n=2），联手删光/禁光所有 superadmin。

Phase 1 单管理员场景概率低，但属于保护性逻辑的正确性缺陷。

**修复方向**：
1. 将「检查 + 删除」放入同一事务，并在事务内 `SELECT ... FOR UPDATE` 或用 `pg_advisory_xact_lock(hashtext('last_superadmin'))` 串行化；或
2. `CountActiveSuperadminUsers` 改为在写事务内执行且条件包含目标用户当前状态。

**状态**：✅ 已修复（2026-08-19）— 采用方向 1：`user_repo.go` 新增 `RunInTx` / `AcquireSuperadminGuard`（`pg_advisory_xact_lock`）/ `CountActiveSuperadminUsersTx` / `SoftDeleteTx` / `UpdateStatusTx` / `SetRolesTx`；`UserService.Delete` / `UpdateStatus` / `SetRoles` 中超管保护检查与写操作移入同一事务并加 advisory lock。`user_repo` 集成测试覆盖最后一名超管不可删/禁与锁串行化。
> 复验补充：初版修复在 `UpdateStatus` 超管守护分支引入回归——事务成功后直接 return，跳过 `revokeUserSessions`，导致被禁用超管的存量 AT 仍可用最长 30 分钟（JWT 中间件不查 DB status）。已补吊销调用（与 `Delete` 及非超管路径对齐），[02-auth.md](./02-auth.md) §会话吊销同步标注"含守护分支"。

### 2.3 测试覆盖与「模拟链路」测试盲区

审查时现状：`repository` / `service` / `handler` / `router` 全部 `[no test files]`；已有测试仅覆盖 config / middleware / jsonutil（middleware 当时仅 casbin / cors 等少数文件）。

具体盲区：

| 盲区 | 风险 |
|------|------|
| 自服务标签测试是**手动模拟**的（`casbin_test.go:74` `c.Set("self_service", true)`） | 若有人调换 `router.go:83` 中两个中间件注册顺序，测试依然全绿，线上所有自服务接口 403 |
| 测试用字符串字面量 `"self_service"` 而非导出常量 | key 改名不会编译报错，测试与实现可能悄然脱钩 |
| SoftDelete 级联无任何测试 | 与 README §3.1「测试先行」原则相悖；级联回滚路径（删关联成功但软删 0 行）未验证 |

**修复方向**：
1. 导出 context key（如 `ContextKeySelfService`）供测试引用；
2. 增加一条 `router` 层集成测试：构造真实路由树 + stub RoleFetcher，断言 `GET /api/v1/user/menus` 以 viewer 身份 200（验证中间件顺序本身，而非模拟标签）；
3. 为 `SoftDelete` 补 testcontainers PG 集成测试（README §3.2 本就规划了 Repository 层集成测试，目前未落地）。

**状态**：✅ 已修复（2026-08-19）—
1. `middleware/casbin.go` 导出 `SelfServiceContextKey`，测试引用常量；
2. 新增 `router/router_test.go`：真实路由树 + stub RoleFetcher，覆盖 viewer 自服务放行 / viewer 业务路由 403 / 零角色拒绝 / admin 放行 / 公开路由不挂 JWT（中间件顺序本身被验证）；
3. 新增 `repository/user_repo_integration_test.go`（testcontainers PG）：软删级联清理 `user_roles`/`user_orgs`、软删 0 行整体回滚、F-6 软删复用断言、超管守护事务与 advisory lock 串行化。

---

## 3. P2（记录在案，择机处理）

### 3.1 `viper.BindEnv` 错误被忽略 / 全局 viper 不可重入

`config.go:147-150` 三处 `BindEnv` 返回的 error 被丢弃；viper 全局单例状态使 `Load` 不可重入（当前单次调用无碍，测试中多次 Load 可能互相污染）。

### 3.2 `userSelectColumns` 无表前缀，JOIN 写法脆弱

```go
16:29:internal/repository/user_repo.go
const userSelectColumns = `
	id, username,
	COALESCE(employee_no, '') AS employee_no,
	...
```

`ListByOrgID`（`:60-72`）以 `INNER JOIN user_orgs` 使用该常量。当前 `user_orgs` 恰好无同名列（`user_id`/`org_id`/`is_primary`/`joined_at`）不报错，但**将来给 `user_orgs` 加任何与 users 同名的列（如 `created_at`）即触发 ambiguous 错误**。建议统一加 `u.` 前缀。

### 3.4 Casbin enforcer 无策略 reload 路径

`enforcer.go:34-37` cleanup 调用 `StopAutoLoadPolicy`，但从未 `StartAutoLoadPolicy`；除 `AssignMenus`/`DeleteRole` 显式 `LoadPolicy()` 外无其他 reload。Phase 1 策略静态（admin/superadmin 通配种子），影响小；**直接改 `casbin_rule` 表（如 DBA 手工操作）后需重启进程或触发一次 AssignMenus 才生效**，值得在运维文档标注。

### 3.5 SoftDelete 与会话吊销非原子

`user_service.go:214-217`：`SoftDelete` 成功后 `revokeUserSessions` 失败会向客户端返回错误，但用户实际已删。属于最终一致性小瑕疵（Redis 恢复后 disabled key 才生效；期间该用户旧 AT 仍可用）。可接受，但建议至少 log.Error 而非仅向客户端报错，并考虑后续对账任务。

### 3.6 `List` 的 count 与 list 两次查询非同一快照

`user_repo.go:152-180` 先 COUNT 后 SELECT，两次独立查询之间有并发写时 total 与实际行数可能不一致。分页场景普遍接受此权衡，记录在案即可。

---

## 4. 勘误：初版误报记录

> 以下两条为初版错误结论，经实测/复核推翻。保留记录以防后人重复怀疑，也作为审查方法教训。

### 4.1 误报：`APP_*` 环境变量"白名单陷阱"（初版 P0 §1.2）

**初版结论**（错误）：仅 `APP_SERVER_MODE`/`JWT_SECRET`/`DB_PASSWORD`/`REDIS_PASSWORD` 生效，其余 `APP_*` 静默失效。

**实测推翻**：临时测试（`t.Setenv` + `Load`）验证以下变量**全部生效**：
- `APP_DATABASE_HOST` → 覆盖 yaml `database.host` ✓
- `APP_REDIS_PORT` → 覆盖 `redis.port` ✓
- `APP_DATABASE_SSLMODE` → 覆盖 `database.sslmode` ✓
- `APP_SERVER_MODE` → 覆盖 `server.mode` ✓

**准确规则**（viper `AutomaticEnv` + `SetEnvPrefix("APP")` + `SetEnvKeyReplacer` 的实际行为）：

| 键的来源 | 对应环境变量 | 是否生效 |
|----------|--------------|---------|
| yaml 中已存在的键 | `APP_<SECTION>_<KEY>`（如 `APP_DATABASE_HOST`） | ✅ 全部生效 |
| 显式 `BindEnv` 的键 | 绑定名（`JWT_SECRET`/`DB_PASSWORD`/`REDIS_PASSWORD`，**不带 APP_ 前缀**的别名） | ✅ 生效 |
| yaml 中**不存在**的键 | 任何变量 | ❌ 不生效（唯一盲区） |

即：`BindEnv` 的 3 个无前缀名是刻意提供的别名；`BindEnv("server.mode", "APP_SERVER_MODE")` 实为冗余（AutomaticEnv 已覆盖）。commit f0f19c0 中真正起修复作用的是 `SetEnvKeyReplacer`。

**残留的合理注意点**（初版有价值部分，降级保留）：
- 新增配置项若**只写进代码默认值而未写进 yaml**，将无法通过环境变量覆盖（上表第三行盲区）——运维需知晓此规则；
- `BindEnv` 返回的 error 被忽略（见 §3.1）。

### 4.2 误报：`/auth/login` 无登录审计（初版 P2 §3.3）

**初版结论**（错误）：登录成败均无操作审计。

**复核推翻**：`AuthService.Login` 在**所有分支**显式调用 `s.auditService.LogLogin`——参数错误 400（`auth_service.go:57`）、锁定 429（`:66`）、用户不存在 401（`:81`）、禁用 401（`:96`）、密码错误 401（`:109`）、成功 200（`:127`）；`audit_service.go:34` 注释明确说明"公开路由不走 AuditLog 中间件，由 AuthService 显式调用"为有意设计。初版仅检查了路由与中间件层，未查 service 层，结论草率。

**教训**：审查"某能力是否存在"时必须检索实现层（service），不能只看挂载层（router/middleware）。

---

## 5. 修复排期建议

| 优先级 | 项 | 建议时机 | 状态（2026-08-19） |
|--------|-----|---------|------|
| **P0** | §1.1 审计 context（=F-5） | Step 11 验收前（一行级改动，影响审计可信度） | ✅ 已修复 |
| **P0**（全局视角） | 姊妹文档 F-1 令牌类型混淆 | 立即（安全漏洞，登出机制被整体绕过） | ✅ 已修复（F-1~F-10 同日全部修复，见姊妹文档） |
| P1 | §2.1 enforce 日志 | Phase 1 收尾 | ✅ 已修复 |
| P1 | §2.2 TOCTOU | Phase 1 收尾（与 10-concurrency.md 事务模式统一考虑） | ✅ 已修复（advisory lock 方案） |
| P1 | §2.3 测试补齐 | Step 11 验收前补 router 链路测试；其余随模块演进 | ✅ 已修复（router 链路 + SoftDelete 级联集成测试） |
| P2 | §3.1–3.6 | 择机 / Phase 2 | 记录在案 |

## 6. 与既有文档的关系

- §2.2 与 [10-concurrency.md](./10-concurrency.md) 的 DB 事务/锁约定相关，修复时应复用其模式；
- §2.3 与 [README §3 测试策略](./README.md#3-测试策略) 相呼应（审查时落地与规划的差距，现已按 §2.3 状态补齐）；
- §3.4 涉及运维文档（策略 reload 需重启或触发 AssignMenus）；
- 全分支深度问题（含 F-1~F-10）以 [11-code-review-findings.md](./11-code-review-findings.md) 为 SSOT，本文档仅覆盖本地 3 提交定向审查 + 两处勘误。
