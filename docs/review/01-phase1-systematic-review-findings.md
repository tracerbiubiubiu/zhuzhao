# 系统性代码检查发现报告（Phase 1 全库）

> **报告日期**：2026-08-21
> **检查计划**：[00-phase1-systematic-review-plan.md](./00-phase1-systematic-review-plan.md)
> **执行方式**：R1 核心安全（认证/授权/用户 3 agents）→ R2 业务与基础设施（角色菜单/组织/基础设施审计 3 agents）→ R3 主会话交叉验证 → R4 本报告汇总
> **覆盖范围**：`internal/` 全部约 50 个 Go 源文件 ↔ `docs/` 五组文档体系（phase1/modules/api/design/proposal）+ migrations 种子
> **验证状态**：全部 P1/P2（15 条）经主会话逐条读码复核；P3 抽查通过

---

## 1. 执行摘要

| 严重度 | 数量 | 说明 |
|--------|------|------|
| P0 阻断 | **0** | 无安全可利用漏洞、无数据损坏路径 |
| P1 严重 | **2** | 角色禁用语义未落地；角色写操作缺目标校验 |
| P2 一般 | **13** | 契约违背 6、逻辑/并发 5、边界 2 |
| P3 建议 | **45** | 文档偏差、死代码、防御性缺口、性能优化 |

**总体结论**：

1. 核心安全机制实现质量高：JWT 双 Token（GetDel 原子轮换、typ/alg 防混淆）、登录锁定（Lua 原子）、会话吊销时效、L1 Casbin fail-closed、用户模块防提权/乐观锁/软删、审计同步写入——均与主契约一致，[11-code-review.md](../phase1/11-code-review.md) 12 项历史修复**全部无回归**，近期修复（树组装、菜单按钮节点、响应契约）亦无回归。
2. 两个 P1 均集中在**角色模块**：`status=0` 禁用角色后权限全链路不收回（应急止血手段失效）；DeleteRole/UpdateRole/AssignMenus 缺「目标角色」优先级与系统角色校验（分权场景下低权角色可破坏更强角色）。
3. P2 集中于：文档承诺未兑现（JWT 20002、visible 过滤、中间件顺序、幂等语义）与并发窗口（Move TOCTOU、双 primary）。
4. Phase 2 预留项（authz_service stub、空 Registry、RoleService.GetTree）均有文档计划标注，**不构成占位问题**。

---

## 2. 发现总表

### P1（2 条）

| ID | 类型 | 位置 | 摘要 |
|----|------|------|------|
| R1-AUTHZ-01 | SEC/EDGE | user_repo.go:394-425、role_repo.go:235-239 | 角色禁用（status=0）全链路不生效：Casbin enforce、菜单下发、priority 防提权均不过滤禁用角色 |
| R2-RM-01 | SEC/DOC | rbac_service.go:107-167 | 角色写操作缺「目标角色」校验：DeleteRole/AssignMenus 无优先级检查，UpdateRole 可降权更强角色，AssignMenus 可改 is_system 角色菜单 |

### P2（13 条）

| ID | 类型 | 位置 | 摘要 |
|----|------|------|------|
| R1-AUTH-01 | DOC/ERR | middleware/jwt.go:41-46 | JWT 过期未按文档返回 20002，过期/无效统称 20003；`ParseTokenExpired` 死代码 |
| R1-AUTH-02 | DOC/EDGE | auth_service.go:194-232 | 改密未校验「新密码 ≠ 旧密码」，文档测试用例承诺 400，实际成功并吊销全部会话 |
| R1-USER-01 | LOGIC/DOC | user_service.go:165-188,342-355 | Update/UpdateProfile 全量覆盖语义，按文档示例（部分字段）调用会清空 employee_no（登录键）等字段 |
| R2-RM-02 | DOC/LOGIC | menu_service.go:224-233 | GetUserMenus 未过滤 visible=false，违反 07-menu.md/modules-menu.md 双文档承诺 |
| R2-RM-03 | LOGIC/EDGE | role_repo.go:77-83 | AssignMenus 不校验 menu_id：不存在 ID 触发 FK 500（文档要求 ErrMenuNotFound）；软删菜单可写入产生脏绑定 |
| R2-RM-04 | LOGIC | rbac_service.go:125-129,161-165 | 事务提交后 LoadPolicy 失败：DB 已生效、内存策略陈旧（权限回收不生效窗口），无重试/补偿 |
| R2-RM-05 | DOC/SEC | role_handler.go:48-133 | 影子超管读路径不完整：GET /roles/:id、/:id/menus、/:id/permissions 对 superadmin 未 404，admin 可探测 |
| R2-ORG-01 | LOGIC | org_repo.go:88-91 | AddMember 重复添加不幂等：upsert 覆盖 is_primary，重添加未传 is_primary 时静默降级 primary |
| R2-ORG-02 | LOGIC | org_service.go:248-278 | Move 环检测与新父 path 读取在事务外（TOCTOU）：并发交叉移动可破坏 ltree 树形不变量；过期 path 匹配 0 行仍返回成功 |
| R2-ORG-03 | EDGE | org_repo.go:81-95 + 迁移 115-121 | primary 互斥仅靠应用层，无 `UNIQUE(user_id) WHERE is_primary` 索引兜底，并发可产生双 primary |
| R2-ORG-04 | ERR | org_repo.go:126-139 | SetUserOrgs 重复 org_id 未去重：DELETE 后逐条 INSERT 触发 PK 23505，未映射 → 500 |
| R2-INFRA-01 | DOC | router.go:76-84 | 中间件顺序与 09-middleware.md 契约不符：实现 JWT→Audit→Casbin，文档 JWT(7)→Casbin(8)→Audit(9) |
| R2-INFRA-02 | EDGE | config/config.go:67-70 | DSN() 密码未 URL 编码，密码含 `@ : / ? #` 时 ParseConfig 失败或错连主机 |

### P3（45 条）

| ID | 类型 | 位置 | 摘要 |
|----|------|------|------|
| R1-AUTH-03 | SEC | auth_service.go:70-85 | 登录定时侧信道：工号不存在分支无 dummy bcrypt，可枚举有效工号 |
| R1-AUTH-04 | STUB | authz_service.go:22-24 | CheckResourcePermission 返回 not implemented，wire 注册但零调用（Phase 2 预留，建议加注释标注） |
| R1-AUTH-05 | STUB | model/token.go:11-18 | DeviceInfo 结构体死代码（Phase 2 设备管理预留） |
| R1-AUTH-06 | DOC | auth_service.go:87-98 | 登录流程顺序与 modules/auth.md §5.1 不一致（状态检查先于密码验证、禁用也计数） |
| R1-AUTH-07 | DOC | middleware/jwt.go:25-30 | X-AK-only 请求返回 10002 而非文档约定的 20009「不支持该认证方式」（置信度中） |
| R1-AUTH-08 | DOC | user_repo.go:75-79 | FindByEmployeeNo SQL 缺文档承诺的 `employee_no <> ''` 过滤（当前不可利用） |
| R1-AUTH-09 | EDGE | auth_service.go:119-126 | 登录成功后 UpdateLastLogin/签发失败分支无登录审计记录 |
| R1-AUTH-10 | EDGE | auth_handler.go:66-67 | Logout 忽略 body 解析错误，DeviceID 缺省删除 default 设备 RT |
| R1-AUTH-11 | EDGE | middleware/jwt.go:74 | mcp 白名单硬编码 URL 字符串，与路由定义脆弱耦合 |
| R1-AUTH-12 | SEC | router.go:37-46 | 未 SetTrustedProxies，ClientIP 可被 XFF 伪造（影响审计 IP，不影响认证决策） |
| R1-AUTH-13 | ERR | auth_handler.go:99-117 | writeAuthError 未映射 ErrUserNotFound(30002)，落 default 500（可达性低） |
| R1-AUTH-14 | PERF | middleware/jwt.go:48-67 | 每请求 2 次串行 Redis EXISTS，可 pipeline 合并 |
| R1-AUTHZ-02 | DOC | 03-authz.md:101、modules/authz.md:105 | 自服务白名单实现已改为路由组标签，两份 SSOT 文档仍写路径匹配方案 |
| R1-AUTHZ-03 | STUB | middleware/casbin.go:35-40 | CasbinPassThrough 直通中间件（挂上即绕过全部 RBAC）无引用仍保留 |
| R1-USER-02 | DOC | user_handler.go:92-103 vs 04-user.md:650 | 删除接口 body 字段：文档测试用例写 `{"id"}`，实现要求 `user_id` |
| R1-USER-03 | ERR | user_service.go:371-387 | ensureCanManage 失败统一返回 30005「不能重置密码」，禁用/删除场景文案错位 |
| R1-USER-04 | EDGE | user_service.go:232-268 | 允许禁用自己（文档未承诺，非违约；自禁用后需他人恢复） |
| R1-USER-05 | EDGE | user_service.go:232-268 | 可禁用 is_system 种子用户（文档未承诺，非违约） |
| R1-USER-06 | EDGE/ERR | org_repo.go:126-139 | org_ids 重复触发主键冲突 500（与 R2-ORG-04 同源，R1 视角记录） |
| R1-USER-07 | ERR | pgerr.go:11-29 | 未映射 23503 外键违规，竞态插入返回 500 而非精确错误 |
| R1-USER-08 | PERF | user_service.go:278-301 | SetRoles 每角色查两次 FindByID（2N 次查询） |
| R1-USER-09 | DOC | user_repo.go:152-180 vs 04-user.md:277-279 | 列表响应与详情同构，超出文档「列表较简」契约 |
| R1-USER-10 | EDGE | user_service.go:104-135 | 角色/组织校验在事务外，TOCTOU 可绑定软删「幽灵角色」（无提权后果） |
| R1-USER-11 | DOC/EDGE | user_service.go:173-174 | Update 允许改 username，文档标注「Phase 2 再定改名流程」 |
| R2-RM-06 | EDGE | menu_service.go:165-181 | 菜单删除不清理 role_menus，GetRoleMenuIDs 回显已删菜单（级联已排期 Phase 2） |
| R2-RM-07 | EDGE | rbac_service.go:53-56 | CreateRole 显式 status:0 被强制改 1，无法创建禁用角色；与 Update 行为不一致 |
| R2-RM-08 | ERR | role_handler.go:94-96 | AssignMenus 绑定错误返回固定「参数错误」，未用 errcodeInvalidParams |
| R2-RM-09 | EDGE | menu_service.go:54-96 | GetUserPermissions 对 admin/superadmin 无通配兜底，清空超管菜单后 permissions=[] 而 Casbin 仍放行 |
| R2-RM-10 | EDGE | menu_service.go:112-139 | type=2 页面不强制 path，空 path 页面进树但生成不了 route: 权限码 |
| R2-RM-11 | STUB | authz_service.go:22-24 | 同 R1-AUTH-04（B1 交叉记录，供汇总核对） |
| R2-ORG-05 | EDGE | org_service.go:223-246 | Delete 三重保护检查与软删非同事务，check-then-act 窗口可留残留成员 |
| R2-ORG-06 | EDGE | org_request.go:11-18 | code/name 无长度校验，超长触发 22001 → 500 |
| R2-ORG-07 | EDGE | org_repo.go:235-249 | 软删 code 复用后 path 重叠，Move 谓词无 deleted_at 过滤，连带更新软删行 |
| R2-ORG-08 | DOC | org_service.go:154-166 | GetMembers 无分页，modules 文档承诺「分页列表」 |
| R2-ORG-09 | DOC | org_service.go:88-93 | GetUserOrgs 返回 []*UserOrg，modules 签名承诺 []*Organization |
| R2-ORG-10 | ERR | org_service.go:209-211 | Update 拒绝系统组织复用 ErrOrgIsSystem「不可删除」，文案错位 |
| R2-INFRA-03 | STUB | wire.go:31,47 | Registry/AuthzService 注册进 Provider 集合但注入链路零消费者（G-1 已知，建议加注释） |
| R2-INFRA-04 | STUB | app.go:87-90 | Shutdown 残留 TODO「刷空审计日志队列」与同步审计契约矛盾 |
| R2-INFRA-05 | EDGE | audit_handler.go:38-53 | 审计查询 start>end 未校验，静默返回空列表 |
| R2-INFRA-06 | PERF | audit_service.go:107-118 | 审计分页 page 无上限，巨量 OFFSET 扫描（DoS 放大） |
| R2-INFRA-07 | EDGE | main.go:39-45 | 错误路径 os.Exit(1) 跳过 defer cleanup()，不优雅关闭 |
| R2-INFRA-08 | PERF | middleware/logger.go:32-46 | AccessLogger 未跳过 /health/* 探针路径，日志噪音 |
| R2-INFRA-09 | DOC | middleware/logger.go:13-29 | RequestID/AccessLogger 自写偏离文档选型，request_id 格式与「UUID」描述不符 |
| R2-INFRA-10 | EDGE | config/validate.go:19-25 | 配置校验仅覆盖 jwt.secret，TTL>0/端口范围/必填项缺失 |
| R2-INFRA-11 | DOC | modules/audit.md:244 | modules 文档要求 Phase 1 审计含 request_id/trace_id，与 phase1 SSOT（无此列）矛盾 |

---

## 3. P1 详情

### [R1-AUTHZ-01] P1 / SEC/EDGE：角色禁用后权限不收回，「禁用」操作全链路无效

- **位置**：`internal/repository/user_repo.go:394-399`（GetRoleCodes）、`user_repo.go:419-426`（GetRoles）、`internal/repository/role_repo.go:235-239`（ListRoleIDsByUserID）；触发入口 `internal/service/rbac_service.go:71-93`（UpdateRole）
- **问题描述**：`UpdateRoleRequest.Status` 允许传 0（`binding:"oneof=0 1"`），管理员可禁用角色。但授权链路三处查询均只过滤软删、不过滤 `status`：
  - Casbin 中间件照常取到禁用角色 code 并逐角色 enforce（casbin_rule 策略未因禁用清除）
  - `GetUserMenus/GetUserPermissions` 照常下发菜单与按钮权限码
  - priority 防提权照常把禁用角色计入
  即：**安全应急场景下管理员禁用某角色，该角色全部成员的 L1 路由权限、前端菜单、管理权档位均原样保留**。文档未定义角色禁用的失效语义（05-role.md 仅一行字段注释），也非 Phase 2 计划内预留。
- **证据**（主会话复核确认）：
  ```sql
  -- GetRoleCodes（Casbin 每请求调用）
  SELECT r.code FROM roles r
  INNER JOIN user_roles ur ON ur.role_id = r.id
  WHERE ur.user_id = $1 AND r.deleted_at IS NULL   -- 无 status 过滤
  ```
- **改进建议**（三选一，按成本递增）：① 三处 SQL 加 `AND r.status = 1`（禁用即下次请求失效，与「角色变更下次请求生效」语义一致）；② UpdateRole 检测 status 1→0 时同步清 casbin_rule + LoadPolicy；③ 若定位「status 仅展示」，须在 05-role.md 明确并让 API 拒绝 status=0
- **置信度**：高 ｜ **状态**：已确认（主会话读码复核）

### [R2-RM-01] P1 / SEC/DOC：角色写操作缺「目标角色」优先级与系统角色校验

- **位置**：`internal/service/rbac_service.go:107-131`（DeleteRole）、`71-93`（UpdateRole）、`134-167`（AssignMenus）
- **问题描述**：F-2 修复只覆盖「新 priority 值」校验，未对**目标角色**做强弱检查（文档定义 priority 语义为双向：`actorP < targetP` 严格更强才能操作目标）：
  1. **DeleteRole**：仅查 is_system 与用户绑定。持有 role:delete 的低权自定义角色（priority=25）可删除更强自定义角色（priority=15）
  2. **UpdateRole**：`ensureRolePriorityAllowed` 只校验新 priority 值 ≥ actor 档位。低权角色可把更强角色的 priority 改弱后接管其用户（is_system 有保护，自定义角色无）
  3. **AssignMenus**：仅挡 `target=superadmin`。低权角色可清空/改写 **admin/operator（is_system）** 的 role_menus——如清空 admin 菜单使 admin 用户 GetUserMenus 为空
- **前置条件**：superadmin 给低权自定义角色分配了「角色管理」相关权限（分权管理是 05-role.md 明确设想场景）
- **证据**（主会话复核确认）：AssignMenus 目标校验仅有 superadmin 一段；DeleteRole 无任何优先级检查；`canManageTarget` 已存在于 priority.go:34 但未接入角色模块
- **改进建议**：三处写操作统一接入 `canManageTarget(actorRoles, []*model.Role{target})`；AssignMenus/DeleteRole 对 is_system 目标至少拒绝非 superadmin actor（与 UpdateRole 的 ErrRoleIsSystem 对齐）
- **置信度**：高 ｜ **状态**：已确认（主会话读码复核）

---

## 4. P2 详情

### [R1-AUTH-01] P2 / DOC/ERR：JWT 过期错误码未按文档区分（20002 失效）
- **位置**：`internal/middleware/jwt.go:41-46`、`internal/service/auth_service.go:302-305`
- **问题**：文档三段语义（过期→20002、malformed/黑名单→20003、格式错误）；实现把 ParseAccessToken 所有错误（含 ErrTokenExpired）统一 20003。`ParseTokenExpired` helper 全仓库零调用（写了没接线）。客户端无法据 code 判断「静默 refresh」还是「跳登录页」。
- **建议**：中间件用 `errors.Is(err, jwt.ErrTokenExpired)` 区分映射 20002/20003；接线或删除 `ParseTokenExpired`。
- **状态**：已确认（主会话读码复核）

### [R1-AUTH-02] P2 / DOC/EDGE：改密未校验新旧密码相同
- **位置**：`internal/service/auth_service.go:194-232`
- **问题**：文档测试用例「新密码与旧密码相同 → 400」；实现无比较，相同密码改密成功并吊销全部会话。
- **建议**：校验 `oldPassword == newPassword` 时返回 `ErrInvalidParams`。
- **状态**：已确认（agent 代码+文档双证据）

### [R1-USER-01] P2 / LOGIC/DOC：Update/UpdateProfile 全量覆盖与文档示例冲突
- **位置**：`internal/service/user_service.go:165-188,342-355`
- **问题**：除 username（`!= ""` 才覆盖）外，employee_no/domain_account/real_name/email/phone/avatar 均无条件覆盖（SQL `NULLIF($n,'')` 空串置 NULL）。按文档示例（部分字段 body）调用会把未传字段全部清空——包括 employee_no（登录键）。
- **建议**：三选一：① 改 patch 语义（指针字段/仅覆盖显式传入）；② 保持 PUT 语义但修正文档示例并声明「未传字段将被清空」；③ 至少对 employee_no 做防误清空保护。
- **状态**：已确认（主会话读码复核）

### [R2-RM-02] P2 / DOC/LOGIC：GetUserMenus 未过滤 visible=false
- **位置**：`internal/service/menu_service.go:224-233`
- **问题**：两级文档承诺用户菜单树只返回 `menu_type IN (1,2) AND visible=true`；`filterMenusForTree` 只滤按钮。管理员设 visible=false 期望隐藏的菜单仍下发并注册动态路由。
- **建议**：`filterMenusForTree` 增加 `if !m.Visible { continue }`（仅用户侧树；管理端 GetTree 保留隐藏节点）。
- **状态**：已确认（主会话读码复核）

### [R2-RM-03] P2 / LOGIC/EDGE：AssignMenus 不校验 menu_id 存在性/活跃性
- **位置**：`internal/repository/role_repo.go:77-83`
- **问题**：文档测试用例「分配菜单 - 菜单不存在 → ErrMenuNotFound」；实现直接 INSERT：不存在 ID 触发 FK → 500；软删菜单可成功写入 role_menus（脏绑定，策略静默为空，前端回显幽灵勾选）。
- **建议**：事务内先比对存在性与活跃性，缺失即回滚返回 ErrMenuNotFound。
- **状态**：已确认（主会话读码复核 AssignMenus 调用链）

### [R2-RM-04] P2 / LOGIC：LoadPolicy 失败后 DB 与内存策略不一致窗口
- **位置**：`internal/service/rbac_service.go:125-129,158-165`
- **问题**：事务提交后 LoadPolicy 失败仅包装返回 500：DB 已生效（调用方重试困惑），且**权限回收场景**内存 enforcer 仍持旧策略，被撤销的 API 继续放行，直到下一次成功 LoadPolicy——无重试、无告警、无对账。
- **建议**：失败时记 Error 日志（含 role/subject 便于对账）并返回明确「已提交但策略刷新失败」语义；或重试 + 启动/定时兜底 LoadPolicy。
- **状态**：已确认（主会话读码复核）

### [R2-RM-05] P2 / DOC/SEC：影子超管读路径不完整
- **位置**：`internal/handler/role_handler.go:48-60,106-133`
- **问题**：影子超管要求「列表/下拉/详情均不可见 + 防推断」；实现只做了 List 过滤。`GET /roles/:id`、`/:id/menus`、`/:id/permissions` 对 superadmin 角色（ID 可枚举）照常 200——admin 可确认超管存在并读取其菜单绑定与 `*,*` 通配策略。
- **建议**：三个读接口在 service 层复用 isSuperadminActor 语义：目标为 superadmin 且 actor 非 superadmin 时返回 ErrRoleNotFound（404，防推断）。
- **状态**：已确认（主会话读码复核 Get handler）

### [R2-ORG-01] P2 / LOGIC：AddMember 重复添加不幂等，静默降级 primary
- **位置**：`internal/repository/org_repo.go:87-90`
- **问题**：文档承诺「已存在则幂等 / ON CONFLICT DO NOTHING」；实现 `DO UPDATE SET is_primary = EXCLUDED.is_primary`——重复添加已存在 primary 成员且未传 is_primary（零值 false）时静默清除其 primary，用户从 1 个 primary 变 0 个。
- **建议**：`DO UPDATE SET is_primary = true WHERE EXCLUDED.is_primary`（仅提升不降级），或 service 层先查存在性。
- **状态**：已确认（主会话读码复核）

### [R2-ORG-02] P2 / LOGIC：Move 环检测与父路径读取在事务外（TOCTOU）
- **位置**：`internal/service/org_service.go:248-278`（检测）、`org_repo.go:228-256`（事务）
- **问题**：环检测（IsDescendant）与新父 path 读取在事务开始前；事务内仅锁被移动节点旧子树。并发交叉移动（A 移入 B 子树同时 B 移入 A 子树）锁集合不相交、检测均通过、均提交 → path 互为祖先前缀，破坏 ltree 树形不变量。且检查后本节点被并发移动时，UPDATE 以过期 oldPath 匹配 0 行仍返回成功（静默失效）。
- **建议**：把「读取双方 path + 环检测 + 锁」移入同一事务（先 FOR UPDATE 锁移动子树与目标父行，事务内重读检测）；至少校验受影响行数 ≥1，空更新返回冲突错误。
- **状态**：已确认（主会话读码复核）

### [R2-ORG-03] P2 / EDGE：primary 互斥无数据库约束，并发可产生双 primary
- **位置**：`internal/repository/org_repo.go:80-91`、migrations 000001（user_orgs 无部分唯一索引）
- **问题**：「同一用户最多一条 is_primary」仅由应用层维护（先 UPDATE 清除旧行再 INSERT）；两个并发 AddMember（同用户、不同 org、均 primary）清除语句只触碰旧行、新行 PK 不同互不冲突 → 双 primary。
- **建议**：新增迁移 `CREATE UNIQUE INDEX idx_user_orgs_primary ON user_orgs(user_id) WHERE is_primary` 兜底（并发第二个事务收 23505 而非脏数据），应用层映射为友好错误。
- **状态**：已确认（主会话读码复核 AddMember 事务结构）

### [R2-ORG-04] P2 / ERR：SetUserOrgs 重复 org_id 触发 500
- **位置**：`internal/repository/org_repo.go:126-139`
- **问题**：`SetUserOrgsRequest.OrgIDs` 无去重校验；SetUserOrgsTx 先 DELETE 再逐条裸 INSERT，重复 org_id 触发 PK 23505，该路径未调 mapUniqueViolation → 500。影响 `POST /users/orgs` 与 `POST /users`（含 org_ids）两入口。
- **建议**：service 层去重（重复即 400）或 INSERT 改 `ON CONFLICT DO NOTHING`。
- **状态**：已确认（主会话读码复核）

### [R2-INFRA-01] P2 / DOC：中间件顺序与 09-middleware.md 契约不符
- **位置**：`internal/router/router.go:76-84`；文档 `docs/phase1/09-middleware.md` L26-36（顺序表：JWTAuth 7 / CasbinAuth 8 / AuditLog 9）、L84-86（伪代码 `JWTAuth, CasbinAuth, AuditLog`）
- **问题**：实现为 `authed.Use(JWT, AuditLog)` + 子组 CasbinAuth，实际链 **JWT → Audit → Casbin**。后果：① Casbin 拒绝的 403 请求也写审计（文档设计只记通过鉴权的请求）；② 08-audit.md「未认证请求 user_id=NULL」用例在任何顺序下均不可满足（JWT Abort 后 Audit 不执行），该文档用例本身亦需修订。
- **建议**：与产品确认审计语义。若维持现行为（记录被拒操作更利于安全审计，**实现方更安全**），更新 09-middleware.md/modules/middleware.md 顺序说明并修订 08-audit.md 用例；若遵从文档，把 AuditLog 移到 CasbinAuth 之后。同时补路由顺序守护测试。
- **状态**：已确认（主会话读码+读文档复核；B1 负面确认第 9 条与此冲突，裁决见 §6）

### [R2-INFRA-02] P2 / EDGE：DSN 密码未 URL 编码
- **位置**：`internal/config/config.go:67-70`
- **问题**：`DSN()` Sprintf 直接拼接；密码含 `@ : / ? # %` 保留字符时 pgxpool.ParseConfig 失败或错连主机。生产密码经 DB_PASSWORD 环境变量注入，字符不受控。
- **建议**：改用 `url.UserPassword` 构造 URL，或 ParseConfig 后直接赋值 ConnConfig 字段。
- **状态**：已确认（主会话读码复核）

---

## 5. P3 清单（45 条，紧凑格式）

**认证（12）**：登录定时侧信道可枚举工号（R1-AUTH-03，建议 dummy bcrypt）｜authz stub 建议加「Phase 2 预留勿调用」注释（04）｜DeviceInfo 死代码（05）｜登录流程顺序与 modules/auth.md §5.1 不一致（06，建议修文档）｜X-AK-only 响应未按约定 20009（07，置信度中）｜FindByEmployeeNo SQL 补 `<> ''`（08）｜登录成功后中间失败无审计（09）｜Logout 忽略 body 解析错误（10）｜mcp 白名单硬编码改 `c.FullPath()`（11）｜SetTrustedProxies 缺失，审计 IP 可伪造（12，上线前建议处理）｜writeAuthError 补 30002 映射（13）｜JWT 中间件 2 次 Redis EXISTS 可 pipeline（14）

**授权（2）**：自服务白名单标签方案未回写两份 SSOT 文档（R1-AUTHZ-02）｜删除 CasbinPassThrough 死代码（03，挂上即绕过 RBAC）

**用户（10）**：删除接口文档用例 `{"id"}` 改 `{"user_id"}`（02）｜ensureCanManage 通用化错误码（03）｜禁用自己/is_system 用户视运营补保护（04/05）｜org_ids 重复 500（06，并入 R2-ORG-04）｜pgerr 补 23503 映射（07）｜SetRoles 2N 查询合并（08）｜列表响应超文档契约，择一对齐（09）｜Create 校验事务内复核（10）｜username 更新与文档「Phase 2 再定」对齐（11）

**角色菜单（6）**：菜单删除低成本补 role_menus 清理（06）｜CreateRole Status 改 *int（07）｜AssignMenus 绑定错误统一 errcodeInvalidParams（08）｜GetUserPermissions 超管通配兜底（09）｜type=2 强制 path 非空（10）｜authz stub 交叉记录（11，并入 R1-AUTH-04）

**组织（6）**：Delete 保护改同事务（05）｜code/name 补 max 长度（06）｜Move/IsDescendant 谓词补 deleted_at 过滤（07）｜GetMembers 补分页或修文档（08）｜GetUserOrgs 签名回写文档（09）｜Update 系统组织错误文案（10）

**基础设施（9）**：wire 预留 provider 加注释（03，G-1 已知）｜Shutdown TODO 注释清理（04）｜审计 start>end 校验（05）｜审计分页 page 上限（06）｜main 错误路径显式 cleanup（07）｜AccessLogger 跳过 /health/*（08）｜RequestID 选型/格式回写文档（09）｜配置校验补 TTL/端口/必填（10）｜modules/audit.md request_id 字段矛盾修正（11）

---

## 6. 交叉验证记录

**复核方法**：R3 由主会话（非 agent）对全部 2 条 P1 + 13 条 P2 逐条读码复核（15 条中 14 条亲自读码，R1-AUTH-02 依据 agent 代码+文档双证据采纳）；P3 抽查通过。

**冲突裁决**（1 处）：

| 冲突 | B1 结论 | B3 结论 | 裁决 |
|------|---------|---------|------|
| 中间件顺序是否符合文档 | 负面确认#9「符合承诺」 | R2-INFRA-01「违约」 | **B3 胜出**。主会话读取 09-middleware.md L26-36（顺序表 JWT 7/Casbin 8/Audit 9）与 L84-86（伪代码）实证文档要求 JWT→Casbin→Audit；实现为 JWT→Audit→Casbin。B1 对事实行为描述正确但「符合承诺」结论错误，予以剔除 |

**误报剔除**：无硬性误报。R1-AUTH-07（X-AK 20009）因文档措辞为「推荐」而非验收必需，置信度降为中、状态「待人工复核」。

**Phase 2 预留判定**（D2 维度负面确认）：authz_service stub、空 Registry、wire 未接线（G-1）、RoleService.GetTree、DeviceInfo——均与 phase2/02-authz-resource.md、11-authz-architecture-review.md、modules/role.md 计划标注对应，**不计占位问题**。

### 6.1 第二轮独立复核（2026-08-21，应负责人要求追加）

不依赖 agent 与首轮结论，对关键发现重新取证（grep 全仓 + 逐文件读码 + 文档原文定位）：

| 复核项 | 独立取证证据 | 结论 |
|--------|--------------|------|
| R1-AUTHZ-01（P1） | grep 全 repository 层 14 处 `roles` 查询**零处** `r.status` 过滤；`RoleRepo.Update` L151 `status = $4` 无条件写入；binding `oneof=0 1` 放行 0 | **成立** |
| R2-RM-01（P1） | `canManageTarget` 确存于 priority.go:34；rbac_service.go DeleteRole(L107-131)/UpdateRole(L71-93)/AssignMenus(L134-167) 三处均未调用；AssignMenus 目标校验仅 `role.Code == "superadmin"` 一段（L139） | **成立** |
| R1-AUTH-01 | errcode.go:34 定义 20002；02-auth.md:606、errcode.md:81 双文档承诺；jwt.go:41-46 统一 20003；grep 证实 `ParseTokenExpired` 全仓零调用（仅定义处） | **成立** |
| R1-AUTH-02 | 02-auth.md:771 测试用例「新密码与旧密码相同 → 400」；UpdatePassword（auth_service.go:194-232）主会话亲读，无新旧比较 | **成立**（升级为主会话亲证） |
| R1-USER-01 | 04-user.md:304-312 示例仅传 id/version/real_name/email；user_repo.go:187-193 `employee_no = NULLIF($3,'')` 等全字段覆盖 | **成立** |
| R2-RM-02 | 07-menu.md:303「只返回 menu_type IN (1, 2)，visible=true」；filterMenusForTree（menu_service.go:224-233）仅滤按钮 | **成立** |
| R2-RM-03 | role_repo.go:77-83 事务内直接 INSERT role_menus，无存在性/活跃性校验 | **成立** |
| R2-RM-04 | rbac_service.go:125-129、158-165 LoadPolicy 失败仅 `fmt.Errorf` 包装 | **成立** |
| R2-RM-05 | role_handler.go Get(L48-60)/GetMenus(L106-118)/GetPermissions(L121-133) 均 FindByID 直查；grep 证实 `isSuperadminActor` 仅 List(L24) 一处调用——「列表过滤、详情泄露」不对称成立 | **成立** |
| R2-ORG-01/02/04 | org_repo.go:87-90（DO UPDATE 覆盖 is_primary）、235-238（仅锁旧子树）+ org_service.go:248-278（检测在事务外）、129-136（裸 INSERT） | **成立** |
| R2-ORG-03 | grep 全部 migrations：user_orgs 仅建表（000001:115）与列定义（:118），**无部分唯一索引** | **成立** |
| R2-INFRA-01/02 | router.go:76-84 vs 09-middleware.md:26-36、84-86；config.go:67-70 Sprintf 拼接 | **成立** |
| P3 抽查（R1-AUTH-12/R1-AUTHZ-03/R2-INFRA-07） | grep 证实全仓无 `SetTrustedProxies`；`CasbinPassThrough` 仅定义无调用；main.go:42-45 `os.Exit(1)` 跳过 L39 `defer cleanup()` | **成立** |

**第二轮结论**：2 P1 + 13 P2 全量复核 15/15 成立，P3 抽查 3/3 成立，**零误报**；统计口径（0/2/13/45 = 60）与总表一致。

---

## 7. 与历史发现对照

| 基线 | 结果 |
|------|------|
| [11-code-review.md] 12 项修复（F-1 typ 校验、F-2 优先级、F-3 菜单 API 过滤、F-4 改密吊销、F-5 审计 WithoutCancel、F-7 ctx 传播、F-8 HTTP 超时、F-9 min=8、F-10 种子改密、提权/TOCTOU/fail-close/审计断连等） | **全部在位，无回归**（A1/A2/A3/B1 各自逐项核对） |
| 近期修复：buildOrgTree/buildMenuTree 递归组装（9f97483） | 无回归（单测覆盖 + B1/B2 复核） |
| 近期修复：管理端菜单树按钮节点（9f97483） | 无回归（验收断言 #6c） |
| 近期修复：/user/menus、/user/permissions 响应契约文档（3b2655a） | 无回归（B1 复核一致） |
| 近期修复：审计断连丢失 + HTTP 超时（de470bf） | 无回归（B3 复核 F-5/F-8 在位） |
| 本报告 60 项发现 | **全部为新增**（未与历史重叠） |

---

## 8. 负面确认清单（关键项摘要）

完整清单见各 agent 过程报告（本次会话存档）；以下为安全关键项：

**认证**：RT 轮换 GetDel 原子防并发刷新，重放窗口为零（存储 SHA-256 hash 比对）｜login_lock.lua INCR+EXPIRE 原子，15min/5 次与文档一致，锁定期间不执行密码比对｜会话吊销 user:disabled TTL=AccessTTL 必然覆盖存量 AT，中间件每请求检查｜JWT 强制 HMAC + AT/RT 双向 typ 校验｜bcrypt cost=12｜Redis/DB 故障全链路 fail-closed（503/500，不放行）｜jti UUID v4 + 黑名单 TTL=AT 剩余寿命

**授权**：enforcer 出错 → 503 拒绝（fail-closed）；零角色 → 403+70003（自服务不豁免，有测试守护）｜TOCTOU 无回归：JWT claims 不含角色，每请求实时查 DB｜自服务白名单为路由注册期标签，请求侧无法伪造｜superadmin 放行在 matcher+种子双保险，Go 代码无特判 fail-open｜39 条路由 ↔ 32 条 menu_apis 种子 ↔ 文档权限码三方逐条对应｜策略加载失败 fail-fast（启动退出）

**用户**：全部 7 个写方法均接防提权校验，无遗漏路径｜effectivePriority=min(priority) 与文档一致，同级严格拒绝｜乐观锁 WHERE 带 version，冲突 409+10006｜部分唯一索引支持软删后重建同名｜最后 superadmin 三路径 advisory lock 保护｜非超管对超管 404 隔离

**组织**：Move 级联 UPDATE 前缀替换 SQL 正确｜删除保护（系统/子组织/成员）拒绝式与文档一致｜双 HTTP 入口单写路径属实｜ltree 标签校验拒绝特殊字符

**基础设施**：errcode 全表与文档逐条一致（含预留段）｜response Envelope/BIGINT string/request_id 注入符合 SSOT｜CORS/安全头/BodyLimit 与文档一致｜优雅关停主链路正确（SIGTERM→Shutdown 30s→cleanup 逆序）｜recovery 不泄露堆栈｜审计脱敏与参考实现逐字一致

---

## 9. 修复优先级建议

| 批次 | 内容 | 理由 |
|------|------|------|
| **第一批（Phase 1 收尾前）** | R1-AUTHZ-01、R2-RM-01（两个 P1）+ R2-INFRA-02、R1-AUTH-12（DSN 编码、TrustedProxies 上线前必改） | 安全语义完整性 |
| **第二批** | 契约对齐类 P2：R1-AUTH-01/02、R1-USER-01、R2-RM-02/03/05、R2-INFRA-01 | 前端联调依赖契约 |
| **第三批** | 并发加固类 P2：R2-ORG-01/02/03/04、R2-RM-04 | 触发窗口小但修复成本低 |
| **第四批（可入 Phase 2 backlog）** | 全部 P3 | 打磨项 |
