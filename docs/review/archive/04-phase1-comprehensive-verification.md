# Phase 1 综合验证评估报告（合并版）

> **报告日期**：2026-08-24
> **验证范围**：`docs/`（目标/架构/设计意图）+ `phase1/`（已完成功能）+ `review/`（评审结论 00-03）+ `phase2/` + `phase3/`（下一阶段规划）+ **当前代码实现**（以实现为准）
> **验证方法**：六轮递进——① 文档全景通读 → ② 评审基线确认 → ③ 功能完整性逐模块比对 → ④ Bug 检测（主会话复核 + 并行专项 agent 增量扫描）→ ⑤ 测试覆盖深度评估 → ⑥ Phase 2/3 可行性分析。合并后追加第七轮：对 Agent 报告 Bug #2/#3/#4/#13/#14 逐项读码实证裁决。
> **编译/测试**：`go build ./...` ✅ | `go vet ./...` ✅ | `go test -count=1 ./internal/...` 10 包全绿 ✅（覆盖率数据经实测复核）
> **独立性**：本报告合并原 04/05/06 三份文档，以当前代码为唯一事实来源逐项重验。原三份文档已删除。

---

## 0. 验证结论速览

| 维度 | 结论 | 增量发现 |
|------|------|----------|
| **功能完整性** | 设计意图核心功能已实现；1 处文档承诺未实现（菜单深度上限 F-缺口-1）、G-2 stub 为计划内占位 | 新增 1 处死代码发现（V-07） |
| **Bug 检测** | 历史三轮 106 项发现的核心修复全部实证在位、无回归；无新的功能性 Bug | 新增 8 项 P3 一致性隐患（V-01~V-08），均为非阻塞 |
| **测试覆盖** | 核心鉴权/并发/会话路径已受测；audit/user_service/handler 写路径存在盲区 | 新增 5 项测试缺口（T-新-1~5），其中 3 项高优先 |
| **Phase 2 可行性** | **足以支撑 2a 启动**；G-1 Registry 已注入、Casbin DB adapter 支撑工单过滤 | 无新增阻断项 |

**总体评价**：Phase 1 实现质量高，安全与并发防护达生产级标准（noeviction + TTL、事务化 TOCTOU 修复、原子 RT 轮换、DB-backed Casbin、revoke 重试补偿）。三轮 106 项发现的核心修复全部在位且无回归。增量发现集中于**一致性隐患、测试盲区、演进阻碍**三类，无新的功能性 Bug。

---

## 1. 编译与测试（实测复核）

| 检查项 | 结果 | 备注 |
|--------|------|------|
| `go build ./...` | ✅ 通过 | — |
| `go vet ./...` | ✅ 通过 | — |
| `go test -count=1 ./internal/...` | ✅ 10 包全绿 | — |
| 迁移文件 | ✅ 000001-000009 完整，up/down 对称 | — |

### 1.1 覆盖率数据（实测复核，与原 06 一致）

| 包 | 覆盖率 | 评估 |
|----|--------|------|
| internal/config | 42.9% | 充分（validate 全覆盖） |
| internal/handler | 8.6% | 不足（仅 errors_test） |
| internal/middleware | 56.6% | 良好（jwt/casbin/audit 覆盖） |
| internal/pkg/jwt | 82.9% | 充分 |
| internal/pkg/jsonutil | 76.2% | 充分 |
| internal/pkg/redis | 50.0% | 部分（Lua 脚本有测试） |
| internal/pkg/validate | 100.0% | 完全覆盖 |
| internal/repository | 2.9% | 不足（仅 user_repo_test + pgerr_test 纯单测；4 个 *_integration_test 依赖 Docker 未纳入） |
| internal/router | 89.5% | 充分 |
| internal/service | 15.7% | 不足（集成测试覆盖部分，需 Docker） |

> **覆盖率说明**：repository 2.9% 偏低是因为 4 个 `*_integration_test.go` 依赖 Docker testcontainers，普通 `go test` 自动跳过。实际测试文件共 6 个（user_repo_test.go、pgerr_test.go、user_repo/org_repo/role_repo/main_integration_test.go），但仅前两个为纯单测。覆盖率数据准确，非旧数据。

---

## 2. 功能完整性验证（逐模块实证）

### 2.1 已实现功能（代码证明确认）

| 设计意图（phase1 文档） | 实现位置 | 验证结论 |
|---|---|---|
| 认证：登录/刷新/登出/改密 + 双 Token | `auth_service.go` | ✅ Refresh 用 GetDel 原子轮换（L175）；登出/改密写 AT 黑名单 + 清 disabled；UpdatePassword 新旧相同校验（L238） |
| 登录安全：dummy bcrypt 拉平时延 | `auth_service.go:87,105` | ✅ 工号不存在 + 禁用分支均调 `CheckDummyPassword`（B4-1 在位） |
| device_id 白名单 | `auth_service.go:61,169,196` + `validDeviceID` | ✅ 字符 `[a-zA-Z0-9_-]` + 长度 ≤64（D2-22 在位） |
| JWT 中间件：过期/无效分流、disabled 兜底、黑名单、mcp | `jwt.go` | ✅ 过期→20002、无效→20003；`isUserDisabled` 兜底；mcp 经 `WithValue` 入 ctx |
| 会话吊销：重试补偿 | `session_revoke.go:44-63` | ✅ `revokeUserSessionsWithRetry` 3 次退避 + reconcile 标记（D2-05 在位） |
| 鉴权：Casbin PG adapter | `casbin/enforcer.go` | ✅ DB-backed；G-1 内存模式已修 |
| RBAC：角色禁用全链路生效 | `user_repo.go:451,478,499,511` | ✅ GetRoleCodes/GetRoles/IsSuperadminUser/CountActiveSuperadminUsers 均带 `r.status = 1`（B1-1 在位） |
| 角色：目标校验 + 优先级防提权 | `rbac_service.go` + `priority.go` | ✅ `ensureCanManageRole` + `canManageTarget`（B1-2 在位） |
| 用户：superadmin 最后一人保护（事务内） | `user_service.go` | ✅ advisory guard + 事务内重读（L228-285） |
| 用户：patch 语义（指针化） | `user_request.go` + `user_service.go` | ✅ B2-3 在位 |
| 组织：Move 事务化 + advisory lock + 环检测 | `org_repo.go` | ✅ `pg_advisory_xact_lock` + `FOR UPDATE` + ltree `<@`（B3-2 在位） |
| 组织：AddMember 幂等不降级 + primary 唯一索引 | `org_repo.go` + 迁移 000008 | ✅ B3-1/B3-3 在位 |
| 组织：SetUserOrgs 去重 | `org_repo.go` | ✅ B3-4 在位 |
| 菜单：visible 过滤 + 类型必要字段 + 树递归组装 | `menu_service.go` | ✅ B2-4/B4-4 在位；buildMenuTree 递归自底向上（防孙子丢失） |
| 菜单：includeMenuAncestors 环检测 | `menu_service.go:334` | ✅ D2-25 在位 |
| 审计：递归脱敏 + binary 占位 + 截断 | `audit.go` | ✅ D2-19 在位 |
| 审计：分页 page 上限 + ILIKE 转义 | `user_repo.go:616,606` | ✅ D2-13/D2-21 在位 |
| 错误码：表驱动映射 + 全码表测试 | `errors.go` + `errors_test.go` | ✅ D2-06/07 在位 |
| SetRolesTx：INSERT...SELECT + 行数校验 | `user_repo.go:437` | ✅ D2-26 在位 |
| Redis 安全：noeviction + 全键 TTL | `deployments/redis/redis.conf:15` | ✅ D2-01 在位 |
| 部署配置：APP_DATABASE_* 变量 | `docker-compose.yaml` | ✅ D2-02 在位 |
| pgx 优化：QueryExecModeCacheDescribe + Ping 独立超时 | `postgres.go:30,50` | ✅ D2-39/D2-47 在位 |
| 迁移：幂等 + 加固索引 | `migrations/000009` | ✅ D2-10/35/41 在位 |

### 2.2 功能缺失 / 偏差

#### ⚠️ F-缺口-1：菜单层级深度上限未实现（设计文档明确要求）

- **设计意图**：phase1 文档「预期功能」明确要求菜单 depth 上限约束（与组织树防超深同理）。
- **实现现状**：`menu_service.go:266-305` `validateMenuParent` 仅做类型父子校验（目录→页面→按钮），`Create`/`Update` 全程无深度累加检查。对比 `org_service` 有 `maxOrgPathDepth=20`。
- **风险等级**：P3。菜单用 `parent_id` 邻接表（非 ltree），超深不触发 DB 500，但违背设计契约，前端递归渲染可能栈溢出。
- **建议**：`validateMenuParent` 中累加祖先深度，超阈值（5~10）返回 400；或文档显式声明「菜单深度不受限」闭合契约。
- **阻塞**：不阻塞 Phase 2。

#### ⚠️ G-2：AuthzService stub 仍为占位（计划内，非 Bug）

- `authz_service.go` `CheckResourcePermission` 恒返回 `not implemented`，注释声明 Phase 2a Step 0 删除并接线 ResourceRegistry。全仓无调用方，fail-closed 方向正确。**Phase 2a 第一动作**。

#### 🆕 V-07：OrgService.Create 的 OrgType==4 检查为死代码

- **位置**：`org_service.go:194`（`if req.OrgType == 4`）+ `model/org_request.go:17`（`binding:"required,oneof=1 2 3"`）
- **问题**：binding 已在 handler 层拒绝 org_type=4 及任何非 1/2/3 值，service 层检查永远不可达。
- **影响**：Phase 2b 引入虚拟组（org_type=4）时需同步改 binding + service，两处不同步即 bug。
- **建议**：删 service 冗余检查，或提取 `validOrgTypes()` 常量集供 binding 与 service 共享。
- **演进影响**：是（Phase 2b 虚拟组织引入时的同步点）。

---

## 3. Bug 检测

### 3.1 历史修复复核（验证已闭环）

对 01（60 项）+ 03（46 项）的核心修复项逐项读码实证，**全部在位、无回归**：

| 修复类别 | 代表项 | 实证位置 |
|---|---|---|
| Redis 驱逐策略 | D2-01 noeviction + max=50/64 | `redis.conf:15` + `token.go` |
| compose 变量命名 | D2-02 APP_DATABASE_* | `docker-compose.yaml` |
| Status 零值穿透 | D2-03 role/org 指针化 | `role_request.go` + `org_request.go` |
| 登录审计断连 | D2-04 LogLogin WithoutCancel | `audit_service.go:52` |
| 会话吊销部分写 | D2-05 revokeUserSessionsWithRetry | `session_revoke.go:44-63` |
| 错误码断链 | D2-06/07 表驱动映射 | `errors.go:17-67` |
| 审计体膨胀 | D2-08 截断 maxAuditBody | `audit.go` |
| 弱密钥防线 | D2-09 无条件拒绝 | `validate.go:75` |
| 缺索引 | D2-10/35/41 五索引 | `migrations/000009` |
| 分页上限 | D2-13 normalizePage 10000 | `user_repo.go:616` |
| AssignMenus 去重 | D2-14 seen map | `rbac_service.go:214-223` |
| AddMember 唯一约束 | D2-15 mapUniqueViolation | `org_repo.go:99` |
| SetUserOrgsTx 软删 | D2-16 WHERE deleted_at IS NULL | `org_repo.go:156` |
| 菜单/组织 patch | D2-17 指针化 | `menu_service.go` + `org_service.go` |
| pageSize 回显 | D2-18 clamp 对齐 | `user_service.go` |
| 递归脱敏 | D2-19 maskSensitiveMap+EqualFold | `audit.go:106-129` |
| 禁用分支 dummy bcrypt | D2-20 CheckDummyPassword | `auth_service.go:105` |
| ILIKE 转义 | D2-21 escapeLike | `user_repo.go:606-611` |
| device_id 白名单 | D2-22 validDeviceID | `auth_service.go:344-354` |
| RequestID 校验 | D2-24 isValidRequestID | `logger.go:28-38` |
| 环检测 | D2-25 includeMenuAncestors visited | `menu_service.go:334` |
| SetRolesTx 行数 | D2-26 RowsAffected | `user_repo.go:437` |
| 含软删审计查询 | D2-27 FindByEmployeeNoIncludeDeleted | `audit_service.go:66` |
| 函数重命名 | D2-46 ListRoleCodesByRoleIDs | `role_repo.go:262` |
| pgx 预编译缓存 | D2-39 QueryExecModeCacheDescribe | `postgres.go:30` |
| Ping 独立超时 | D2-47 10s 独立 context | `postgres.go:50` |
| 迁移编号顺延 | D2-48 | 03 文档 §10.1 已更新：Phase 1 用至 000009，phase2 ticket=000010、org-enhance=000011 ✅ |
| 角色禁用 status=1 | B1-1 | user_repo.go:451/478/499/511 |
| 角色目标校验 | B1-2 canManageTarget | rbac_service.go + priority.go |
| dummy bcrypt | B4-1 CheckDummyPassword | auth_service.go:87,105 |
| patch 语义 | B2-3 指针字段 | user/role/org_request.go |
| visible 过滤 | B2-4 | menu_service.go:316 |
| Move 事务化 | B3-2 advisory lock + FOR UPDATE | org_repo.go |
| primary 唯一索引 | B3-3 迁移 000008 | migrations/ |
| SetUserOrgs 去重 | B3-4 | org_repo.go |
| LoadPolicy 重试 | B3-5 revokeUserSessionsWithRetry | session_revoke.go:44 |

### 3.2 现存风险（非 Bug，需关注）

#### R-1：Casbin 策略更新依赖全量 LoadPolicy，无 AutoLoad

- `enforcer.go:34-36` cleanup 调 `StopAutoLoadPolicy`，但启动未 `StartAutoLoadPolicy`。策略生效完全依赖 `rbac_service.reloadPolicy`（L258-269）在事务提交后显式 `LoadPolicy()`。
- **评估**：管理端低频操作下可接受。但 `reloadPolicy` 失败仅日志 + 重试 1 次，不回滚 DB 事务。
- **方向区分（本轮追加）**：
  - **授权方向**（AssignMenus 增菜单）：LoadPolicy 失败 → 内存策略 stale → 新权限未生效 → **under-privileged（安全，fail-closed）**
  - **回收方向**（DeleteRole/撤菜单）：LoadPolicy 失败 → 旧规则残留内存 → 被撤销的 API **继续放行 → over-privileged（fail-open，安全风险）**
- **结论**：回收方向的风险更高，建议在文档标注此为已知权衡。retry 1 次 + 低频管理操作 + 下次 LoadPolicy 收敛仍使整体可接受。
- **排期**：D2-38 已排期 Phase 3a multi-instance（Watcher/StartAutoLoadPolicy）。

#### R-2：Refresh 并发刷新与吊销的边界

- `Refresh` 路径 `GetDel` 保证单设备 RT 不可重用（原子），但 `issueTokenPair` 的 `Set` 与 `revokeUserSessions` 的 `DEL` 非同一原子操作。并发「刷新 A」与「改密吊销」理论上存在 RT 在 DEL 后、Set 前被签发的小窗口。
- **评估**：文档已声明「单设备 RT 不可重用」为可接受级别；且改密后旧 AT 仍被 disabled 键兜底拦截。风险极低，符合设计取舍。

#### R-3：审计中间件在 `hasBinaryBody` 判定下丢失登录 username

- D2-19 将 form-encoded body 整体替换为 `<binary len=N>`，但 `LogLogin` 独立记录 employee_no/ip/statusCode，登录审计可观测性完整。无功能损失。

### 3.3 增量发现（03 未覆盖，8 项，均为 P3）

#### [V-01] P3：ExcludeSuperadminUsers SQL 未过滤 r.status=1（一致性隐患，当前不可达）

- **位置**：`user_repo.go:595-600`
- **证据**：`buildUserListWhere` 的 `ExcludeSuperadminUsers` 分支为 `WHERE r.code = 'superadmin' AND r.deleted_at IS NULL`，**无 `r.status = 1`**。对比同文件 GetRoleCodes:451 / GetRoles:478 / IsSuperadminUser:499 / CountActiveSuperadminUsers:511 均带 `r.status = 1`（B1-1）。
- **可达性分析**：superadmin 是 `is_system` 角色，`UpdateRole` 对 is_system 返回 `ErrRoleIsSystem` 拒绝修改 status，因此 superadmin 角色当前无法被禁用——该不一致不可达。
- **隐患**：若未来放开 is_system 角色的 status 修改，会形成矛盾态。
- **建议**：SQL 加 `AND r.status = 1`，与 B1-1 全局语义对齐（一行改动）。

#### [V-02] P3：GetMembers 不排除 superadmin 用户（信息披露不对称）

- **位置**：`user_repo.go:60-85`（ListByOrgID）+ `org_service.go:156-184`（GetMembers 无 actor 校验）
- **问题**：`UserService.List` 对非超管 actor 设 `ExcludeSuperadminUsers=true` 隐藏超管；但 `OrgService.GetMembers` 无 `actorUserID` 参数，password 虽 `json:"-"`，但 username/employee_no 等敏感字段返回。
- **建议**：GetMembers 加 actorUserID 参数，非超管走 ExcludeSuperadminUsers 同型过滤。

#### [V-03] P3：AccessLogger 缺 userID/username 字段（Phase 3 可观测性阻碍）

- **位置**：`middleware/logger.go:62-71`
- **问题**：access log 含 method/path/status/latency/ip/request_id，**不含 userID/username**，尽管 jwt.go 已 `c.Set("userID"/"username")`。
- **影响**：Phase 3 排障时 access log 无法直接关联用户。phase3/01-observability.md 要求「可关联用户行为」。
- **建议**：加 `slog.Int64("user_id", ...)` + `slog.String("username", ...)`。
- **演进影响**：是（Phase 3 可观测性直接依赖）。

#### [V-04] P3：maskSensitive 为固定 denylist（Phase 2 新接口需扩展）

- **位置**：`middleware/audit.go:107`
- **问题**：sensitiveKeys 仅 `{password, old_password, new_password, secret, token}`，不含 `access_token/refresh_token/authorization/api_key/cookie`。
- **建议**：扩展列表或改 allowlist。
- **演进影响**：是（Phase 2 新接口脱敏需扩展）。

#### [V-05] P3：LoginLockClear/clearUserDisabled 失败无审计

- **位置**：`auth_service.go:123-128`
- **问题**：`LoginLockClear` 与 `clearUserDisabled` 失败返回 503，但未调 `LogLogin`。
- **建议**：两处失败前补 `LogLogin(..., &user.ID, user.Username, 503)`。

#### [V-06] P3：bcrypt 密码无 max 长度（72 字节静默截断）

- **位置**：`model/user_request.go:8,56,77`（Password/NewPassword/ResetPassword 均 `min=8` 无 max）
- **问题**：bcrypt 对 >72 字节密码静默截断。
- **建议**：三处 binding 加 `max=72`。与 D2-42（Phase 2 复杂度策略）可顺带处理。

#### [V-07] P3：见 §2.2（OrgType==4 死代码，演进同步点）

#### [V-08] P3：OrgRepo.Move 将 SQL id 列 Scan 回输入参数（代码异味）

- **位置**：`org_repo.go:329`
- **问题**：`.Scan(newParentID, &parentPath)` 把 `SELECT id` 结果写回输入参数 `*newParentID`。当前功能等价 no-op；但若未来 SQL 改 `WHERE code = $1` 会静默损坏调用方变量。
- **建议**：改 `var scannedID int64; Scan(&scannedID, &parentPath)` 或 SELECT 只取 path 列。

### 3.4 Agent 报告 Bug 裁决（第七轮实证）

> 本轮对并行专项 Agent 扫描发现的 Bug #2/#3/#4/#13/#14 逐项读码实证。

#### Bug #14 裁决：disabled TTL 应为 refreshTTL？— **❌ 非安全漏洞，Agent 判断有误**

- **Agent 主张**：disabled 键 TTL=accessTTL（30m），RT TTL=refreshTTL（7d）。30m 后 disabled 键过期，残留 RT 可刷新 → 最高优先级安全风险。
- **实证**：
  - `session_revoke.go:14`：disabled 键 TTL = `accessTTL`（30m）✓
  - `auth_service.go:151-166` Refresh 路径：`isUserDisabled`（Redis 检查，L151）→ **`user.Status != 1`（DB 校验，L164）** → 双重防线
  - 即使 disabled 键过期，`FindByID` 从 DB 读取 `user.Status=0` 仍拒绝刷新（L164-166 返回 `ErrRefreshTokenInvalid`）
  - AT TTL（30m）= disabled 键 TTL（30m）→ 同步过期，AT 侧无窗口
- **结论**：Redis disabled 键是**性能优化**（快速路径避免每次请求查 DB），**非唯一防线**。DB `user.Status` 是终极安全边界。TTL 选择 accessTTL 是正确设计——AT 过期时 disabled 键同步失效，之后由 DB 接管防护。**Agent 的「最高优先级安全风险」判断不成立。**

#### Bug #13 裁决：Casbin reloadPolicy 失败无补偿 — **⚠️ 方向区分有效，已并入 R-1**

- **Agent 主张**：reloadPolicy 失败时权限回收场景被撤销的 API 继续放行（fail-open）。
- **实证**：`rbac_service.go:258-269` reloadPolicy 失败仅重试 1 次 + 日志，不回滚 DB 事务。
- **裁决**：Agent 的**方向区分有效**——回收方向（DeleteRole/撤菜单）LoadPolicy 失败 = over-privileged（fail-open，安全风险），授权方向 = under-privileged（安全）。此区分已并入 §3.2 R-1。但 retry + 低频管理操作 + 下次 LoadPolicy 收敛仍使整体可接受。

#### Bug #2/#3/#4 裁决：DeleteRole/Menu TOCTOU + user_roles 未清理 — **存在但影响可忽略**

- **Agent 主张**：DeleteRole 的 CountUsersByRoleID 在事务外 → TOCTOU；DeleteRole/DeleteMenu 不清理 user_roles → 孤儿数据。
- **实证**：
  - `rbac_service.go:157-164`：`CountUsersByRoleID`（事务外）→ `roleRepo.Delete`（事务内 L176-199）。TOCTOU **存在**。
  - `role_repo.go:176-199` Delete：事务内删除 role_menus + casbin_rule + 软删角色，**不删 user_roles**。
  - `user_repo.go:357-372`：GetRoleCodes / CountActiveSuperadminUsers 均 `INNER JOIN roles ... WHERE r.deleted_at IS NULL AND r.status = 1`。
- **裁决**：角色为**软删**（`deleted_at IS NOT NULL`），所有权限查询过滤已删角色 → 孤儿 user_roles 行被过滤，**无安全影响**。TOCTOU 窗口内即使新分配用户，该角色已软删，新用户_roles 行同样被过滤。`MenuRepo.Delete`（L161-180）事务内软删 + 清理 role_menus ✓，casbin 规则清理明确注释「留 Phase 2」。**结论：真实代码模式，非安全缺陷，软删 + 查询过滤设计使其无害。**

### 3.5 并发与数据一致性结论

经逐路径实证（superadmin guard + advisory lock、SetRoles/Create 跨 repo 事务共享、Move 单事务+FOR UPDATE+ltree 环检测、RT GetDel 原子轮换、revokeUserSessionsWithRetry 重试补偿、乐观锁四模块一致），**未发现新并发问题**。核心并发防护机制全部在位且正确。

---

## 4. 测试覆盖评估

### 4.1 覆盖矩阵（源文件 → 测试 → 等级）

| 模块 | 充分 | 部分 | 无 |
|------|------|------|-----|
| config | config/validate | — | — |
| middleware | jwt/casbin | audit(仅mask)/logger(仅ID) | body_limit/cors/recovery/security |
| pkg | jwt/scripts/jsonutil/validate | — | crypto/jti/response |
| repository | user/org/role/pgerr | — | audit_log_repo/menu_repo |
| service | menu/org/priority/session_revoke/rbac | auth_service(关键分支缺) | audit_service/user_service(仅3方法) |
| handler | errors | audit_handler | auth/menu/org/role/user_handler |
| casbin | — | — | enforcer |

### 4.2 测试缺口

#### 04 报告已知缺口

| 项 | 描述 | 优先级 |
|----|------|--------|
| T-缺口-1 | 并发关键路径无针对性回归测试（乐观锁冲突/advisory lock/FOR UPDATE） | P3 |
| T-缺口-2 | 菜单深度限制无测试（因功能未实现，见 F-缺口-1） | P3 |
| T-缺口-3 | Casbin 策略重载失败路径无测试 | P3 |
| T-缺口-4 | 审计 `<binary>` 占位与 16KB 截断边界 | P3 |

#### 增量测试缺口（05 报告发现）

| 项 | 优先级 | 描述 |
|----|--------|------|
| T-新-1 | **高** | audit_service.go 全无断言测试（LogLogin 11 处调用无验证；D2-27 软删审计查询零回归测试） |
| T-新-2 | **高** | user_service.go 仅 3 方法受测（Create/Delete/SetRoles/ResetPassword 全无） |
| T-新-3 | **高** | auth_service.go 关键分支未测（锁定429/status0/disabled/RT篡改/dummy bcrypt时延） |
| T-新-4 | 中 | crypto.go 零测试（CheckDummyPassword 抗侧信道核心） |
| T-新-5 | 中 | menu_repo.go / audit_log_repo.go 零单测 |

### 4.3 测试基建就绪度（Phase 2 地基）

| 项 | 状态 | 风险 |
|---|---|---|
| testutil 迁移列表 | 完备（000001~000009） | 手工维护，新增迁移不同步无报错机制 |
| 共享容器清理 | TRUNCATE CASCADE | 新增表需同步更新 4 个 reset 函数 |
| Redis testcontainers | 无（miniredis 单机） | 无法验证 cluster/真实 EVALSHA |
| 事务回滚包裹 | 无 | Phase 2 新增表迁移成本线性增长 |
| handler httptest 基建 | 无 | gin.TestContext 只能测单 handler |

**结论**：核心鉴权/并发/会话路径已受测，但 audit/user_service/handler 写路径存在大面积无守护盲区（T-新-1~3 合计 ~1000 行核心代码）。建议 Phase 2a 前补齐。

---

## 5. Phase 2/3 可行性分析

### 5.1 Phase 2a 启动就绪度

| Phase 2a 依赖的 Phase 1 能力 | 状态 | 证据 |
|---|---|---|
| ResourceRegistry 注入边（G-1） | ✅ 就绪 | `wire_gen.go` Registry 已注入；Phase 1 不注册业务 Resource（计划内） |
| authz_service stub 接线（G-2） | ⚠️ 计划内 | stub 待删，列为 2a Step 0 第一动作 |
| Casbin 策略动态加载 | ✅ 就绪 | DB adapter + 显式 reloadPolicy；工单 assigned_to 过滤是 DB 查询不依赖 Casbin reload |
| 用户/角色/组织/菜单 CRUD | ✅ 就绪 | 完整且带事务/乐观锁 |
| 审计同步写入 | ✅ 就绪 | 工单操作审计可直接复用 |
| 迁移编号无冲突 | ✅ 就绪 | D2-48 已修，Phase 1 用至 000009，Phase 2 自 000010 起（ticket=000010、org-enhance=000011） |

**结论：足以支撑 Phase 2a 启动。**

### 5.2 影响 Phase 2/3 的未决项

| 项 | 类型 | 阻塞 | 处置 |
|---|---|---|---|
| D2-49 设备管理前置 | 已排期 | 不阻塞 2a | PRD 已修，devices 集合+RT value 升级排入 2b Step 7 首任务 ✅ |
| D2-37 组织写路径目标校验 | 上线前决策点 | 不阻塞 2a | 若 Phase 1 独立对外需决策（共享资源文档化 vs 最小护栏），终局在 2c Step 9 |
| F-缺口-1 菜单深度 | 功能契约 | 不阻塞 | 建议闭合（实现或声明） |
| V-03 access log 缺用户关联 | Phase 3 | 不阻塞 2 | Phase 3 可观测性前补 |
| V-04 脱敏 denylist | Phase 2 | 不阻塞 2a | Phase 2 新接口引入时扩展 |
| V-07 OrgType 死代码 | Phase 2b | 不阻塞 2a | 2b 引入虚拟组时同步 |
| T-新-1~3 测试盲区 | 测试基建 | 不阻塞但建议补 | 建议 Phase 2a 前补 audit/user_service/auth_service 写路径测试 |

### 5.3 Casbin 策略加载对工单的支撑评估

- 工单 `assigned` 范围过滤属 DB 查询层（`WHERE created_by = $1 OR assigned_to = $1`），不依赖 Casbin reload，R-1 权衡不影响 2a。
- 路由级权限（ticket:list/create/update）走 Casbin PG adapter + reloadPolicy，与现有角色/菜单机制同构。
- TicketResource 注册到 Registry 后，Authorize 走资源属主判断（Phase 2a Step 1 实现），G-2 stub 接线即激活。

---

## 6. 文档同步验证

| 文档 | 检查项 | 状态 |
|------|--------|------|
| errcode.md vs errcode.go | 44 码逐条一致 | ✅ 同步 |
| errcode.md 预留段 | D2-44 醒目标注 | ✅ 同步 |
| response.md | request_id 格式/流程图 | ✅ 同步 |
| phase2 编号 | 000010-000014 重排 | ✅ 同步（03 文档 §10.1 已确认更新） |
| modules 路径 | 标注"目标路径"非当前结构 | ✅ 可接受 |
| 03 文档 §10.1 D2-48 | 已更新为 org-enhance=000011 | ✅ 同步（原 04 报告称「§10.2 仍写 000009」为过时信息，已纠正） |

---

## 7. 综合结论与优先级

### 7.1 已解决问题（验证闭环）

B1-B4（60 项）+ 03 文档 D2-* 核心项（noeviction、compose 变量、Status 指针化、错误码表驱动、审计脱敏、Move 事务化、revoke 重试、迁移编号、pgx 缓存、函数重命名等）**全部代码落地，编译/vet/单测通过，无新功能性 Bug，无回归**。G-1（Casbin 内存模式）已修。

### 7.2 待办清单（按优先级）

| 优先级 | 项 | 类型 | 阻塞 |
|---|---|---|---|
| **P2** | G-2 stub 接线 | 计划内占位 | 阻塞 2a 资源级鉴权（2a Step 0） |
| **P2** | T-新-1 audit_service 测试 | 测试 | 不阻塞但建议 2a 前补 |
| **P2** | T-新-2 user_service 写路径测试 | 测试 | 不阻塞但建议 2a 前补 |
| **P2** | T-新-3 auth_service 关键分支测试 | 测试 | 不阻塞但建议 2a 前补 |
| P3 | F-缺口-1 菜单深度上限（实现或声明） | 功能契约 | 不阻塞 |
| P3 | V-01 ExcludeSuperadminUsers status 对齐 | 一致性 | 不阻塞 |
| P3 | V-02 GetMembers 排除超管 | 信息披露 | 不阻塞 |
| P3 | V-03 access log 加用户字段 | Phase 3 | 不阻塞 |
| P3 | V-04 脱敏 denylist 扩展 | Phase 2 | 不阻塞 |
| P3 | V-05 登录失败路径补审计 | 审计完整 | 不阻塞 |
| P3 | V-06 密码 max=72 | 输入校验 | 不阻塞 |
| P3 | V-07 OrgType 死代码清理 | Phase 2b | 不阻塞 |
| P3 | V-08 Move Scan 异味 | 代码质量 | 不阻塞 |
| P3 | T-缺口-1 并发路径回归测试 | 测试 | 不阻塞 |
| P3 | T-缺口-3 Casbin 重载失败测试 | 测试 | 不阻塞 |
| P3 | R-1 Casbin AutoLoadPolicy 兜底（可选） | 健壮性 | 不阻塞 |

### 7.3 总体评价

Phase 1 实现质量高，设计意图兑现度高。核心安全机制（JWT 双 Token + typ/alg 防混淆、bcrypt cost=12、登录锁定 Lua 原子、会话吊销 + disabled 键兜底、noeviction + TTL、优先级防提权、乐观锁、advisory lock、TOCTOU 事务化修复）经多轮独立检查互相印证，达生产级标准。

三轮累计 106 项发现的核心修复全部在位且无回归。第七轮 Agent 报告 Bug 裁决：Bug #14（disabled TTL）非安全漏洞（DB status 为终极防线）；Bug #13（reloadPolicy）方向区分有效但整体可接受；Bug #2/#3/#4（TOCTOU + user_roles）存在但软删 + 查询过滤使其无害。增量发现 13 项（V-01~V-08 + T-新-1~5），**无新的功能性 Bug，无新的 P1/P2**——发现集中于一致性隐患、测试盲区、演进阻碍三类。

**当前实现足以支撑 Phase 2a 启动。** 建议 Phase 2a 启动前：① 执行 G-2 stub 接线（2a Step 0）；② 择机补 T-新-1~3 测试；③ 闭合 F-缺口-1 菜单深度契约。

---

## 8. 已修复项复核记录（抽样实证）

> 本轮对历史修复项以当前 HEAD 重验，以下为关键抽样（全部通过）：

| 修复项 | 实证位置 | 状态 |
|---|---|---|
| B1-1 角色禁用 status=1 过滤 | user_repo.go:451/478/499/511 | ✅ |
| B1-2 角色目标校验 canManageTarget | rbac_service.go + priority.go | ✅ |
| B2-3 patch 语义指针化 | user/role/org_request.go | ✅ |
| B2-4 visible 过滤 | menu_service.go:316 | ✅ |
| B3-2 Move advisory lock + FOR UPDATE | org_repo.go | ✅ |
| B3-3 primary 唯一索引 | migrations/000008 | ✅ |
| B4-1 dummy bcrypt | auth_service.go:87,105 | ✅ |
| D2-01 noeviction + max=50/64 | redis.conf:15 + token.go | ✅ |
| D2-05 revoke 重试补偿 | session_revoke.go:44-63 | ✅ |
| D2-06/07 错误码表驱动 | errors.go + errors_test.go | ✅ |
| D2-13 normalizePage 上限 | user_repo.go:616 | ✅ |
| D2-19 递归脱敏 + binary 占位 | audit.go | ✅ |
| D2-21 ILIKE 转义 | user_repo.go:606 | ✅ |
| D2-26 SetRolesTx 行数校验 | user_repo.go:437 | ✅ |
| D2-39 pgx 预编译缓存 | postgres.go:30 | ✅ |
| D2-46 ListRoleCodes 重命名 | menu_service.go:66 | ✅ |
| D2-48 迁移编号顺延 | 03 文档 §10.1 已更新 | ✅ |

---

*本报告合并原 04/05/06 三份文档，基于当前工作区状态以代码实现为准核验，独立于 01/02/03 评审文档的结论并对其逐项复核。增量发现（V-01~V-08、T-新-1~5）及第七轮 Agent Bug 裁决为本轮独有。*
