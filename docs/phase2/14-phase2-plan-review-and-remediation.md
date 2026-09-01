# Phase 2 计划审查与修订总文档

> **合并来源声明**：
> 本文档合并了下列 5 份独立文档，解决多份验证报告的发现重复、描述不一致、SSOT 散落问题：
> 1. `docs/phase2/12-phase1-backlog-and-phase2-review.md`（Phase 1 遗留项 37 项 6 类分类 + Phase 2 六维度二审）
>   ⚠️ **2026-08-31 断链注记**：本文件与下述 12/13 号三份文件**从未入库**（git 历史无删除记录，疑为本地工作稿）。其有效内容已并入本文档（Part 1 分类清单 / Part 3 检查点）与 [review/09-consolidated-findings.md](../review/09-consolidated-findings.md)；引用仅作历史溯源，勿按路径查找。
> 2. `docs/review/05-plan-validation.md`（多轮验证报告，版本 1：编号 P-1~P-13）
> 3. `docs/review/07-project-plan-verification.md`（多轮验证报告，版本 2：编号 P2-1~P2-6 / P3-1~P3-7）
> 4. `docs/phase2/13-project-plan-multi-round-verification.md`（多轮验证报告，版本 3：按用户 5 检查点组织）
> 5. `docs/phase2/13-plan-remediation-actions.md`（修订行动清单步骤 1-6）
>
> **三份验证报告对应说明**：上文第 2/3/4 份即本文 Part 3 所称「三份独立验证报告」：review/05-plan-validation.md（验证 v1）+ review/07-project-plan-verification.md（验证 v2）+ phase2/13-project-plan-multi-round-verification.md（验证 v3）。
> **⚠️ 编号冲突提示**：当前 `docs/phase2/` 目录下存在两个完全独立文档均使用 13 号编号：(i) `13-project-plan-multi-round-verification.md`（验证报告 v3，作为三份验证之一）；(ii) `13-plan-remediation-actions.md`（修订行动清单，作为本文 Part 4 的来源）。二者为完全独立的文档（编号为 Phase 2 规划期间的冲突），本文统一通过全名或简称（13-验证 / 13-行动）区分，旧编号冲突随 5 份源文件一并删除后自然消除。
>
> **文档定位**：Phase 2 开工前唯一的「审查发现 + 遗留项处置 + 行动清单」SSOT 文档。所有发现统一编号为：
> - **A/B/C/D/E/F-X**（Phase 2 PRD 跨文档一致性问题，来自二审）
> - **P2-X / P3-X**（计划本身设计质量/业界对标问题，来自三份验证报告去重合并）
>
> **生成日期**：2026-08-25
> **基准**：Phase 1 已 merge 到 dev，分支 `feature/phase-2` 已从 dev 切出，当前 `make compile vet test` 全绿。

---

## Part 0. 总体结论速览（Executive Summary）

### 0.1 总体评价

| 维度 | 评级 | 核心结论 |
|------|------|---------|
| 整体计划 vs Phase 2 边界与目标 | ✅ 良好 | 16+15+7 三份「不做什么」表完整；Phase 1/2/3 目标各有独立可演示面；仅 4 条 P3 表述/列示级问题 |
| Phase 2a/2b/2c 职责划分 | ✅ 清晰无重叠 | 三段拆分有 4 条充分论证；角色对照表显示零职责重叠；2 条 P2 为跨阶段行为变更未登记 + 路由级权限空白 |
| 各计划衔接关系 | ✅ 顺畅完备 | Phase 1→2a / 2a→2b / 2b→2c 衔接点经文档实证全部在位；4 处缺口集中在「里程碑映射漏 7 条用例」等文档列示级 |
| Phase 1 遗留项落实 | ⚠️ 基本到位（36/37） | 6 类 37 项中 36 项全部正确映射到对应子计划 Step；仅 11 号 §7 设计文档落档未列入 00 §1 拍板 |
| 业界实践符合度 | ✅ 高于平均 | 8 项对齐典范（MVP 分层/里程碑 DoD/编码前拍板/风险登记/验收分段/迁移治理/分支策略/SSOT）；7 条 P3 补全建议 |

**总体判断：计划设计成熟，无 P0/P1 阻塞项，Phase 2a 可进入实施阶段。**

### 0.2 去重后的问题统计

合并三份验证报告 + 二审的 25 项发现，统一去重为两组独立编号：

| 类别 | 项数 | 其中阻塞 2a 的项 | 说明 |
|------|------|-----------------|------|
| A-F 编号（二审，PRD 一致性） | 25（12 P2 / 13 P3） | A4/C1/D2/B1/E1/F1 等 7 项 P2 | 关注「PRD 之间同一主题描述是否一致」 |
| **P 编号（验证，计划质量）** | **17（6 P2 / 11 P3）** | P2-1~P2-4 为开工前须决策/补文档 | 关注「计划本身的边界、职责、衔接、业界对标」 |
| 合计去重后 | 38 条实质问题，其中 13 P2 | 开工前必闭合的最小集=6 条行动清单（步骤 1-6） | — |

### 0.3 Phase 2a 编码启动前必须完成的检查单（开工门禁）

```
□ Step 1（20min）拍板并落档 11-authz-architecture-review 两项真前置（2a Step 2 TicketResource.Authorize 编码前必须收口）
                 → #1-a 鉴权不变量公式（Q1：Allow ⟺ L1 通过 ∧ L2 可见性通过 ∧ canOperate 通过，路径 A L2 在前，含 Q2 默认拒绝语义）→ architecture.md §4.1
                 → #1-b Authorize 错误映射（Q3：确定性拒绝=403 / DB 错误=503 / 未注册资源=500，对齐 Redis fail-close 503 既有模式）→ architecture.md §4.1 同节
                 → 可选 1 行带过：Q5 缓存禁令（L2/L3 资源级判断不缓存，每请求实时查询；Phase 3 perm 缓存仅覆盖 L1 角色列表输入）
                 → P2-D6 挂入 00-implementation-plan.md §1 编码前拍板（5 分钟登记）
                 → 依据：11 号 L7 自述仅 §3 不变量阻塞实现分歧；Q1 公式是 Authorize 语义前置（合取/析取写错=越权），Q3 错误分支编码前需定死；其余 5 项按消费时点拆分（非开工门禁阻塞项，详见 §1.2 拆分表 + §4 步骤 1）：
                    · #2 L2/L3 顺序统一文档修正 + 修 ReBAC 旧称 → 2b Step 5 前（group/all ltree 落地时，顺序差异此时才语义显性化）
                    · #3 ReBAC 业务侧触发表 → Phase 3 启动前评估
                    · #4 SoD 延后决策一句话 + P2-D7 挂入 → Phase 3 启动前登记
                    · #5 scope 枚举映射表 → 2b Step 5 前（group/all 枚举生效时，双轨不一致此时才出 bug）
                    · #6 DB 错误注入 → 拒绝 测试用例 → 2a 测试编写时补入 02-authz-resource.md 测试节
□ Step 2（15min）决策 000010 工单表组是否软删（二审 A4）
□ Step 3（10min）决策 2a→2b update 规则是否保留处理人 update 权（二审 C1 / P2-1）
                 → 在 00 §7 加 RK-11 风险登记
□ Step 4（30min）修复 00 计划任务清单 + T8-T14 里程碑映射
                 → A1/A2/D2/D3/F1/E1 共 6 条 P2
□ Step 5（30min）补 04 §3 路由级权限策略 + 03 §1 前置条件章节（B1/D1 + P2-2/P3-7）
□ Step 6（20min）补 00 §5 回滚策略节（P2-5）
□（可选但强烈建议）补 T-新-1~3 三块 service 测试盲区 + F-缺口-1 菜单深度约束
```

**合计：纯文档修订 + 设计决策 ≈ 2 小时 5 分钟（20+15+10+30+30+20=125min），零代码改动**。完成即可进入 2a Step 0（G-1/G-2 接线）。

---

## Part 1. Phase 1 遗留项分类清单（6 类 37 项）

### 1.0 重要勘误

> **G-1 状态勘误**：04 验证报告 §5.1 称 G-1（Registry 注入边）「✅ 就绪」，但代码实证显示 `wire_gen.go` 仅有 `resource.NewRegistry` 在 `pkgSet` 声明（L98），**`router.Deps` 无 `Registry` 字段、`wire_gen.go` 无实例化代码**——即 provider 声明 ≠ 注入生效。02-authz-resource §1 Step 0 的判断正确，G-1 仍为 2a Step 0 任务。

### 1.1 类别 1：Phase 2a 启动前必须完成（阻塞 2a，Step 0 首动作）

| 编号 | 名称 | 来源 | 严重度 | 描述 | 处置建议 | 阶段归属 |
|------|------|------|--------|------|----------|----------|
| **G-1** | Registry 注入边接线 | `02-authz-resource.md` §1 Step 0；`README.md` §1.6 | P2（阻塞 2a） | `wire.go` 已声明 `resource.NewRegistry` provider，但 `router.Deps` 无 `Registry` 字段、`wire_gen.go` 无实例化代码——无消费者导致 Wire 不生成注入。 | ①`router.Deps` 加 `Registry resource.Registry` 字段；②`router.New()` 加启动期资源清单日志；③`router_test.go` 补 `Registry: resource.NewRegistry()`；④`make wire` 重生成。约 5 行改动。 | 2a Step 0 |
| **G-2** | authz_service.go stub 删除 | `02-authz-resource.md` §1 Step 0；`04 报告` §2.2 | P2（阻塞 2a） | `authz_service.go` 的 `CheckResourcePermission` 恒返回 `not implemented`，已被 ResourceRegistry 模式取代。全仓无调用方，fail-closed 方向正确。 | ①删 `internal/service/authz_service.go`；②`wire.go` provider set 移除 `NewAuthzService`；③`make wire`；④核对 `architecture.md` 三处旧描述（已于 2026-08-19 修正，Step 0 仅核对）。 | 2a Step 0 |

### 1.2 类别 2：Phase 2a 启动前建议完成（不阻塞但强烈建议）

| 编号 | 名称 | 来源 | 严重度 | 描述 | 处置建议 | 阶段归属 |
|------|------|------|--------|------|----------|----------|
| **T-新-1** | audit_service 全无断言测试 | `04 报告` §4.2 | P2（高） | `audit_service.go` 的 `LogLogin` 11 处调用零验证；D2-27 软删审计查询零回归测试。2a 工单审计将复用此模块。 | 补 `audit_service_test.go`：LogLogin 各分支（成功/失败/503）+ 软删用户审计查询。 | 2a 前补 |
| **T-新-2** | user_service 写路径测试 | `04 报告` §4.2 | P2（高） | `user_service.go` 仅 3 方法受测，`Create`/`Delete`/`SetRoles`/`ResetPassword` 全无直测。2a 工单创建人/处理人依赖用户模块。 | 补写路径单测/集成测试：Create 跨 repo 事务、SetRolesTx 行数校验、ResetPassword 守护。 | 2a 前补 |
| **T-新-3** | auth_service 关键分支测试 | `04 报告` §4.2 | P2（高） | `auth_service.go` 关键分支未测：锁定 429、status=0、disabled、RT 篡改、dummy bcrypt 时延。2a 工单鉴权链路依赖认证状态。 | 补集成测试：锁定窗口/RT 篡改/disabled 用户 RT 场景 + `-race` 并发刷新。 | 2a 前补 |
| **设计文档落档-真前置（2 项）** | 11 号 §2.2 Q1 公式 + §3 Q3 错误映射（可选带过 Q5）| `11-authz-architecture-review.md` §2.2 / §3 | P2（**开工门禁阻塞项，见 §0.3 Step 1**，原列类别 2 属归类矛盾） | TicketResource.Authorize 语义不能写错：三层合取结构（Allow ⟺ L1 ∧ L2 ∧ canOperate）、DB 错误 fail-closed 映射（503/500/403）。11 号文档已给出建议值，拍板确认 ≈20 分钟。 | 采纳 11 号 §2.2/§3 建议值写入 architecture.md §4.1 同一节；P2-D6 同步挂入 00 §1。Q5 缓存禁令 1 行可一并带过。 | **2a Step 2 编码前（§0.3 Step 1）** |
| **设计文档落档-2b 前（2 项）** | 11 号 §7 #2 L2/L3 顺序统一 + 修 ReBAC 旧称 / #5 scope 枚举映射表 | `11-authz-architecture-review.md` §2 全量 / §6 | P3（建议，不阻塞 2a） | 2b 引入 group/all scope ltree 时 L2/L3 顺序才语义显性化（2a 仅 assigned 时 L2 与 L3 属主等价，顺序不影响结果）；scope 枚举双轨不一致此时才会出实际 bug。 | #2 写入 design-decisions.md §3.4；#5 写入 architecture.md §4.3。 | 2b Step 5（group/all 落地）前 |
| **设计文档落档-Phase 3 / 测试时（3 项）** | 11 号 §7 #3 ReBAC 触发表 / #4 SoD 延后决策 + P2-D7 / #6 DB 错误注入测试用例 | `11-authz-architecture-review.md` §4 / §5 / §3 Q3 配套 | P3（不阻塞 2a） | ReBAC 触发表、SoD 延后声明均为 Phase 3 重型工作流触发信号；DB 错误注入测试可在 2a 编码完成后、测试编写时补（Q3 错误映射决策本身仍需 Step 1 拍板，决策与写测试是两个时点）。 | #3 写入 design-decisions.md §12.5；#4 写入 design-decisions.md §14 + P2-D7 挂 00 §1；#6 补 02-authz-resource.md §测试用例节。 | Phase 3 启动前 / 2a 测试编写时 |

### 1.3 类别 3：Phase 2b 启动前必须完成（阻塞 2b，不阻塞 2a）

| 编号 | 名称 | 来源 | 严重度 | 描述 | 处置建议 | 阶段归属 |
|------|------|------|--------|------|----------|----------|
| **D2-49②** | devices 集合初始化 + RT value 结构升级 | `01-auth-enhance.md` §1 + §2.1；`03 报告` §10.1 | P2（2b Step 7 首任务） | Phase 1 实际只写 `SET refresh:{uid}:{devId} = hashToken(rt)`（SHA-256 hex），**无 devices 集合、无设备元数据**。2b 设备列表/踢出 API 的两个前置全缺。 | ①`issueTokenPair` 改写结构化 value（hash + meta JSON 并存）；②登录/登出/吊销补 `SADD`/`SREM devices:{uid}`；③Refresh 比较逻辑改读 `meta.hash`；④既有守护测试同步改造。 | 2b Step 7 首任务 |
| **D2-48** | 迁移编号顺延（已修，需确认） | `03 报告` §10.1；`README.md` §2.4 | P1（规划级，已修复） | Phase 2 迁移编号原写 000008 与 Phase 1 冲突。已修复：Phase 1 用至 000009，Phase 2 自 000010 起。 | 已修，2b 启动前确认 `README.md` §2.4 映射表与 `migrations/` 目录实际文件一致即可。 | 已修复，2b 前复核 |

### 1.4 类别 4：Phase 1 上线前决策点（若 Phase 1 独立对外）

| 编号 | 名称 | 来源 | 严重度 | 描述 | 处置建议 | 阶段归属 |
|------|------|------|--------|------|----------|----------|
| **D2-37** | 组织写路径目标校验 | `03 报告` §3；`04 报告` §5.2 | P2（上线前决策点） | `AddMember`/`RemoveMember`/`Move`/`Update`/`Delete` 五写路径仅做存在性检查，无 `canManageTarget` 类目标校验。**终局保护在 2c Step 9 防提权矩阵，但窗口可能数月，不可无声推到 2c。** | 二选一：① 显式记录「组织为共享资源」设计决策；② 补最小护栏（org 写操作加持权者范围约束）。若 Phase 1 不独立对外可跳过。 | 上线前决策（终局 2c Step 9） |

### 1.5 类别 5：Phase 2 过程中顺带处理

| 编号 | 名称 | 来源 | 严重度 | 描述 | 处置建议 | 阶段归属 |
|------|------|------|--------|------|----------|----------|
| **F-缺口-1** | 菜单层级深度上限未实现 | `04 报告` §2.2 | P3 | `menu_service.go` `validateMenuParent` 仅做类型父子校验，无深度累加。 | `validateMenuParent` 累加祖先深度，超阈值返回 400；或文档显式声明「菜单深度不受限」闭合契约。 | Phase 2 顺带闭合 |
| **V-07** | OrgType==4 死代码清理 | `04 报告` §2.2 | P3 | `org_service.go:194` 的 `if req.OrgType == 4` 检查因 binding `oneof=1 2 3` 永远不可达。 | Phase 2b 引入虚拟组时同步改 binding + service。 | 2b（虚拟组引入时） |
| **V-04** | 脱敏 denylist 扩展 | `04 报告` §3.3 | P3 | `maskSensitive` 的 `sensitiveKeys` 不含 `access_token/refresh_token/authorization/api_key/cookie`。 | Phase 2 新接口引入时扩展列表。 | Phase 2（新接口引入时） |
| **V-01** | ExcludeSuperadminUsers SQL 未过滤 r.status=1 | `04 报告` §3.3 | P3 | `user_repo.go:595-600` 分支无 `r.status = 1`，与同文件其他四处不一致。 | SQL 加 `AND r.status = 1`，一行改动。 | Phase 2 顺带 |
| **V-02** | GetMembers 不排除 superadmin 用户 | `04 报告` §3.3 | P3 | `OrgService.GetMembers` 无 `actorUserID` 参数，非超管 actor 可见 superadmin 用户信息。 | GetMembers 加 `actorUserID` 参数，非超管走 `ExcludeSuperadminUsers` 同型过滤。 | Phase 2 顺带 |
| **V-05** | 登录失败路径补审计 | `04 报告` §3.3 | P3 | `LoginLockClear` 与 `clearUserDisabled` 失败返回 503 但未调 `LogLogin`。 | 两处失败前补 `LogLogin(..., 503)`。 | Phase 2 顺带 |
| **V-06** | bcrypt 密码无 max 长度 | `04 报告` §3.3 | P3 | 三处密码字段仅 `min=8` 无 `max`，bcrypt 对 >72 字节静默截断。 | 三处 binding 加 `max=72`。与 D2-42 同批。 | 2b Step 7 |
| **V-08** | OrgRepo.Move Scan 回输入参数 | `04 报告` §3.3 | P3 | `org_repo.go:329` `.Scan(newParentID, &parentPath)` 把 SELECT id 结果写回输入参数。 | 改 `var scannedID int64; Scan(&scannedID, &parentPath)`。 | Phase 2 顺带 |
| **T-缺口-1~4** | 并发/菜单深度/Casbin 重载/审计边界测试 | `04 报告` §4.2 | P3 | 并发关键路径、菜单深度、reloadPolicy 失败、审计 binary 占位均无针对性测试。 | Phase 2 集成测试扩展时补。 | Phase 2 顺带 |
| **T-新-4** | crypto.go 零测试 | `04 报告` §4.2 | 中 | `CheckDummyPassword`（抗侧信道核心）零测试。 | 补 `crypto_test.go`。 | Phase 2 顺带 |
| **T-新-5** | menu_repo / audit_log_repo 零单测 | `04 报告` §4.2 | 中 | 两个 repo 无单测。 | Phase 2 repo 层扩展时补测。 | Phase 2 顺带 |
| **D2-42** | 密码复杂度策略 | `01-auth-enhance.md` §3.4 | P3（已排期 2b） | Phase 1 仅 `min=8`，无字符类别要求。 | `ValidatePasswordPolicy` + 20013，2b Step 7 落地。 | 2b Step 7 |
| **D2-34** | 写路径 N+1 / 批量 INSERT | `03 报告` §10.2 | P3（已排期 2b） | Create/SetUserOrgs 逐条 FindByID。 | 2b Step 4 HR Sync 时改批量 INSERT。 | 2b Step 4 |
| **D2-23** | CORS AllowAllOrigins 收紧 | `01-auth-enhance.md` §1 | P3（条件触发） | Bearer 认证下低危。引入 cookie 会话前必须收紧。 | 引入 cookie 前改 Origin 白名单 + `AllowCredentials`。 | 引入 cookie 前 |
| **D2-11** | audit_logs 索引/分区/清理 | `03 报告` §10.2 | P3（已排期） | audit_logs 仅时间过滤无索引。 | Phase 2 日志清理随做。 | Phase 2 日志清理 |
| **D2-36（SCAN）** | revokeUserSessions 全 keyspace SCAN | `03 报告` §10.2 | P3（依赖 D2-49） | 全 keyspace SCAN。D2-49② devices 集合落地后自然消解。 | 2b Step 7 devices 集合语义落地后消解。 | 2b Step 7 |
| **R-2** | Refresh 并发刷新与吊销边界 | `04 报告` §3.2 | P3（设计取舍） | `Set` 与 `DEL` 非同一原子操作。文档已声明可接受。 | 无需行动，文档标注为已知权衡。 | 设计取舍 |
| **R-3** | 审计 hasBinaryBody 丢失 username | `04 报告` §3.2 | P3（无功能损失） | D2-19 将 form-encoded body 替换为 `<binary>`，但 LogLogin 独立记录。 | 无需行动。 | 无需行动 |
| **G-3** | Makefile 无 lint 目标 | `README.md` §1.6 | 低 | Makefile 缺 lint 目标。 | 择机加 `make lint` 目标。 | 择机处理 |
| **G-4** | 文档索引滞后 | `README.md` §1.6 | 低 | 文档索引未更新。 | 择机更新文档索引。 | 择机处理 |

### 1.6 类别 6：可延后至 Phase 3 或择机处理

| 编号 | 名称 | 来源 | 严重度 | 描述 | 处置建议 | 阶段归属 |
|------|------|------|--------|------|----------|----------|
| **V-03** | AccessLogger 缺 userID/username 字段 | `04 报告` §3.3 | P3（Phase 3 阻碍） | `middleware/logger.go` access log 不含 userID/username。Phase 3 排障时无法关联用户。 | Phase 3 可观测性前补 `slog.Int64("user_id", ...)`。 | Phase 3 可观测性 |
| **R-1 / D2-38** | Casbin 无 AutoLoadPolicy（多副本陈旧） | `04 报告` §3.2；`03 报告` §10.2 | P3（Phase 3） | 单实例无影响；多副本下策略陈旧。回收方向 LoadPolicy 失败 = fail-open（安全风险）。 | Phase 3 multi-instance 引入 `StartAutoLoadPolicy(30s)` 或 Watcher。 | Phase 3 |
| **D2-36（缓存）** | 菜单/组织树缓存、权限缓存 | `03 报告` §10.2 | P3（Phase 3） | 高频读路径零缓存。Phase 2 数据规模不需要。 | Phase 3 platform 落地。 | Phase 3 |
| **D2-40** | RT 轮换 Lua 原子化 | `03 报告` §10.2 | P3（暂无里程碑） | `GetDel→校验→Set` 崩溃窗口内用户被迫重登。**非安全问题、低优先级**。 | 暂无里程碑绑定。若 D2-49② RT value 重构落地则顺带评估。 | 暂无里程碑 |
| **D2-01（限流）** | IP 级登录限流 | `03 报告` §10.2 | P3（Phase 3） | Phase 1 已修 noeviction + 输入长度上限（治本），限流属治标。 | Phase 3 security-enhance 落地。 | Phase 3 |

### 1.7 汇总统计

| 类别 | 项数 | 关键项 |
|------|------|--------|
| 1. 阻塞 2a（Step 0） | 2 | G-1 Registry 接线、G-2 stub 删除 |
| 2. 建议 2a 前完成 | 4 | T-新-1~3 测试盲区、§7 设计文档落档 |
| 3. 阻塞 2b | 2 | D2-49② devices/RT 升级、D2-48 编号确认 |
| 4. 上线前决策 | 1 | D2-37 组织写路径 |
| 5. Phase 2 顺带 | 22 | F-缺口-1、V-01~08、T-缺口-1~4、T-新-4~5、D2-42/34/23/11/36、R-2/R-3、G-3/G-4 |
| 6. 可延后至 Phase 3 | 6 | V-03、R-1/D2-38、D2-36(缓存)、D2-40、D2-01 |
| **合计** | **37** | — |

---

## Part 2. Phase 2 方案六维度二审报告（25 项发现）

### 2.0 审查范围与基准

- **审查对象**：`docs/phase2/` 全部 9 份文档 + `docs/proposal/hr-directory-sync.md` + `docs/modules/ticket.md`
- **代码实证**：`internal/pkg/errcode/errcode.go`、`internal/router/router.go`、`migrations/`
- **排除项**：首轮已发现的 3 个问题（11 号 §7 落档未执行、09 §5.4 ticket:note L3 规则缺失、11 号 §6 scope 枚举双轨）不再重复
- **结论**：无 P1 阻塞问题，Phase 2 文档总体可进入实现阶段，但建议优先修复下列 P2 项后再启动 2a 编码

### 2.1 维度 A：跨文档数据模型一致性（7 项）

| # | 编号 | 严重度 | 描述 | 建议 |
|---|------|--------|------|------|
| A1 | 二审 A1 | P2 | `09-ticket.md §2a` 迁移 000010 建表 5 张（`ticket_types` / `ticket_type_fields` / `tickets` / `ticket_comments` / `ticket_events`），但 `00 §3 Step 2` 与 `README §2.4` 只列 3 张，漏 `ticket_type_fields`（前端动态表单）+ `ticket_events`（状态变更审计） | 00 §3 Step 2 与 README §2.4 改为 5 张表 |
| A2 | 二审 A2 | P2 | `README §2.4` 的 000013 行只写 `file_objects` 一张表，00 §3 Step 6 写 `file_objects` / `ticket_attachments` 两张 | README §2.4 000013 改为「附件：file_objects / ticket_attachments」 |
| A3 | 二审 A3 | P2 | `modules/ticket.md §2.4` `ticket_scope` DDL 缺 `NOT NULL` + CHECK；09-ticket 有 NOT NULL | modules/ticket.md 改为 `VARCHAR(20) NOT NULL DEFAULT 'assigned'` + CHECK 注释 |
| A4 | 二审 A4 | **P2 需设计决策** | 00 §3 Step 2 要求遵循「软删部分唯一索引三规范」，但 09 §2a + modules/ticket §3.2 的工单三表 DDL 均无 `deleted_at` 列与部分唯一索引；`ticket_types.code` 是硬 UNIQUE，软删后无法复用 code | 二选一：A. 软删（补 deleted_at + 部分唯一索引）；B. 硬删（00 §3 Step 2 显式标注「工单三表例外」）。建议方案 A，与全局模式一致 |
| A5 | 二审 A5 | P2 | 迁移 000011 列散落 03-org-enhance（ticket_visibility）+ 09-ticket（ticket_scope）+ hr-directory-sync（source/external_id/synced_at + user_orgs.source/expires_at）三份文档，无单一 SSOT | 在 03-org-enhance.md 增加完整的「迁移 000011 列清单」表，标注来源 PRD |
| A6 | 二审 A6 | P3 文档勘误 | 03-org-enhance §Schema 增量与 00 §3 Step 4 误将 `user_orgs.is_primary` 列为 2b 增量；`migrations/000001_init.up.sql` L118 已存在 | 删除列举或显式标注「Phase 1 已存在」 |
| A7 | 二审 A7 | P3 文档勘误 | 03 §涉及文件迁移占位名 `0000xx_hr_source.up.sql` 误导——000011 实际涵盖多块，非仅 HR | 改为 `000011_org_enhance.up.sql` |

### 2.2 维度 B：API 签名跨文档一致性（6 项）

| # | 编号 | 严重度 | 描述 | 建议 |
|---|------|--------|------|------|
| B1 | 二审 B1 | **P2 阻塞 2c** | 04 §3 多个新路由列 `org:update 或 effective owner` 等资源级判断，非 Casbin 码。Phase 1 `biz` 路由组在 CasbinAuth 之后必须命中 Casbin p 表策略——effective owner 如何过中间件是空白 | 04 §3 加「路由级权限实现策略」节（建议复用 `org:update` 或新增 bypass）；00 §3 Step 9 补「router.go 注册新路由 + 中间件顺序」 |
| B2 | 二审 B2 | P2 | 删虚拟组（04 §3.6）未检查 org 下 tickets 数量；tickets.org_id 外键无 ON DELETE 约束，软删 org 后产生孤儿引用 | 04 §3.6 与 09 §6 加「org 删除前置：COUNT tickets > 0 → 409」 |
| B3 | 二审 B3 | P3 | 10-storage §4 路由级权限对 presign 写「业务域校验」模糊；但 §9 已定稿「ticket:attach | 复用 ticket:update」 | 10-storage §4 改为「`ticket:update`（purpose=ticket_attachment）」 |
| B4 | 二审 B4 | P3 | 10-storage §4.1 校验写「用户对 ticket 有 update 权限」，但 §4.3 purpose=avatar 不涉及 ticket | 改为「purpose=ticket_attachment 校验 ticket update 权；purpose=avatar 仅校验已认证」 |
| B5 | 二审 B5 | P3 | 09 §5.4 附件小节列 4 行（upload/confirm/list/delete），10-storage §4 有 5 行（多一个 presign/download） | 09 §5.4 补「预签名下载 | POST /storage/presign/download | 隐含 ticket:read」 |
| B6 | 二审 B6 | P3 | modules/ticket §5.5 列了 `GET /ticket-types/:code`（类型详情），但 09 §3 未列该 API；实现按 09 为 SSOT，前端按 modules 取数 404 | 二选一对齐：09 补该行，或 modules 删除并加注释 |

### 2.3 维度 C：鉴权矩阵跨阶段演进一致性（3 项）

| # | 编号 | 严重度 | 描述 | 建议 |
|---|------|--------|------|------|
| C1 | 二审 C1 | **P2 需设计决策 + P2-1 同根因** | 2a update：创建人 OR 处理人；2b update：仅创建人（策略 B「兄弟组透明读 ≠ 可改」的副作用）。00 §7 风险登记无此行为变更。2a 宣传后 2b 默默收窄。 | 在 00 §7 加 **RK-11**；或分场景保留处理人 update（assigned/in_progress 状态下处理人可改） |
| C2 | 二审 C2 | P3 | 02 §2.3 canOperate 2c 增量写「对 ticket.org_id 增加 org admin/owner」——只加 update？04 §4.2 明确是 update/close/delete/assign 四项 | 改为「对 **update/close/assign/delete** 的 ticket.org_id 增加 org admin/owner + ancestor owner」 |
| C3 | 二审 C3 | P3 | modules/ticket §2.3 权限矩阵只列「2b：创建人；2c：+ org admin/owner」，缺 2a 规则。读者误判 2a 即仅创建人可改 | modules/ticket §2.3 update/close/assign 行补「2a：创建人或处理人」前缀 |

### 2.4 维度 D：前置条件完备性（4 项）

| # | 编号 | 严重度 | 描述 | 建议 |
|---|------|--------|------|------|
| D1 | 二审 D1 | **P2 + P3-7 同根因** | 01/02/04/09/10 五份 PRD 都有显式 §1 前置条件清单，唯独 03-org-enhance.md 全文没有——直接从「预期功能」跳到「核心设计」 | 03 加 §1 前置条件：(1) Phase 1 organizations + user_orgs + ltree 可用；(2) Phase 2a 工单 org_id/org_path 列已落地；(3) hr-sync §2 DDL 同迁移；(4) BFS RoleFetcher 扩展同批 |
| D2 | 二审 D2 | **P2** | 09 §2a 明确要建 menu 目录 + ticket:* 权限码种子（§9 涉及文件列 `0000xx_ticket_menu.up.sql`），但 00 §3 Step 2 任务清单无此项。Casbin 码缺失会直接让 R8/T7 「无 ticket:list → 403」验收挂 | 00 §3 Step 2 加「迁移 000010_menu（或并入 000010）：插入 ticket:list/create/read/update/close/assign/delete/comment/note 的 menu + menu_apis + 角色绑定种子」 |
| D3 | 二审 D3 | **P2（与 A1 同根因）** | 00 §3 Step 2 任务清单漏列 ticket_type_fields + ticket_events 两张表 | 同 A1 修复——改为 5 张表 |
| D4 | 二审 D4 / P3-9 同根因 | P3 | 09 §1 2b 前置条件只列 Step 5（scope 升级）的依赖。但 09 §8 T11（附件 confirm）是 Step 8 用例，依赖 Step 6 storage | 09 §1 拆为「2b Step 5 前置」与「2b Step 8 前置」，Step 8 补 Step 6 storage 可用 |

### 2.5 维度 E：测试用例映射（3 项）

| # | 编号 | 严重度 | 描述 | 建议 |
|---|------|--------|------|------|
| E1 | 二审 E1 / P2-3 同根因 | **P2** | 09 §8 2b 列了 T8-T14 七条用例，00 §2 里程碑门禁表 + §8 SSOT 映射表均未收录。特别是 T13（vg_a admin 改他人工单）有「2b:403 / 2c:200」双阶段预期，两端都漏 | 00 §2 M2b-2 行补「T8-T10、T12、T14」；M2b-5 行补「T11、T13（2b 预期 403）」；M2c-3 行补「T13（2c 预期 200）」 |
| E2 | 二审 E2 | P3 | 03-org-enhance §测试用例把 04 §7 的 D1-D12 整段复制进来，但两份已有轻微漂移（D1 双轨、D6 空组、D11 D7 交叉引用） | 03 D1-D12 表替换为一行「验收 SSOT 见 [04-org-delegation §7](./04-org-delegation.md)，本文不再复制」 |
| E3 | 二审 E3 | P3 | 00 §2 M2b-1 引用「03 测试表」，未说明是 2b 节还是 2c D1-D12 节（后者标了非 2b） | 00 §2 M2b-1 改为「03 §测试用例 **2b 节** + hr-directory-sync §7」 |

### 2.6 维度 F：错误码（2 项）

| # | 编号 | 严重度 | 描述 | 建议 |
|---|------|--------|------|------|
| F1 | 二审 F1 | **P2** | `00-implementation-plan.md` P2-D4 错误码时机表漏列 20012（`ErrDeviceNotFound`，已在 `internal/pkg/errcode/errcode.go` L71 定义）。2b Step 7 多设备踢出 API 必需 | P2-D4 表「20013」改为「20012（设备不存在）+ 20013（密码策略违规）」 |
| F2 | 二审 F2 | P3 表述勘误 | P2-D4 把 90001-90004 笼统描述为「状态机 90002 等」，不利于实现者核对 | 改为「90001-90004（ErrTicketNotFound / ErrTicketInvalidTransition / ErrTicketTypeNotFound / ErrTicketAlreadyClosed）」 |

### 2.7 二审汇总

| 严重度 | 项数 | 明细编号 |
|--------|------|---------|
| P2（需在开工前修复或决策） | 12 | A1/A2/A3/A4/A5/B1/B2/C1/D1/D2/D3/E1/F1 |
| P3（不阻塞，择机修复或顺带） | 13 | A6/A7/B3/B4/B5/B6/C2/C3/D4/E2/E3/F2 + 剩余非 P2 |
| P1（阻塞） | 0 | — |

### 2.8 最值得优先处理的 5 条 P2

| 顺序 | 编号 | 原因 | 预估工时 |
|------|------|------|---------|
| 1 | **A4（工单表软删决策）** | 阻塞 2a Step 2 迁移编写（DDL 未定） | 15 min |
| 2 | **C1（update 规则收窄决策）** | 不阻塞 2a 但影响对外宣传口径，避免 2a→2b 对业务方的默默收窄体验；且是风险登记表未登记的行为变更（见 P2-1） | 10 min |
| 3 | **A1+D2+D3+A2+F1（00 计划清单补全）** | 同一根因：00 §3 Step 2 与 README §2.4 漏列。实现者按 00 行动，结果缺 ticket_type_fields + ticket_events + menu 种子 + 错误码，直接导致验收失败 | 30 min |
| 4 | **B1 + D1（PRD 补章节）** | B1 阻塞 2c（路由级权限空白）；D1 是 03 缺前置条件章节 | 30 min |
| 5 | **E1（T8-T14 映射）** | 七条用例无人验收，T13 双阶段预期两端都漏 | 含于步骤 4 中 |

---

## Part 3. 项目计划多轮验证报告（5 检查点）

> 本部分合并三份独立验证报告（review/05 + review/07 + 13-project-plan-multi-round），统一按用户要求的 5 检查点组织，发现统一编号为 P2-1~P2-6 和 P3-1~P3-11。与 Part 2 的 A-F 编号区分：**A-F 聚焦 PRD 跨文档一致性，P-X 聚焦计划本身的设计质量**。

### 3.0 文档全景 + 验证方法

**文档体系分层（业界三级：架构→模块→阶段）**：
```
docs/
├── roadmap.md                          ← 跨阶段总览（142 行能力边界）
├── design/ (architecture / design-decisions 等)  ← 为什么这样设计
├── modules/ (ticket / auth / authz 等)           ← 模块完整终态设计
├── phase1/ phase2/ phase3/                       ← 每阶段切片实施计划
└── review/                                       ← 阶段性验证报告
```

**验证维度（用户要求的 5 检查点）**：
1. 整体计划与 Phase 2 边界是否清晰、目标是否明确
2. Phase 2 内部 a/b/c 子计划职责划分是否清楚、无重叠/遗漏
3. 各计划衔接关系是否顺畅（前置条件/交付物/验收标准）
4. Phase 1 37 项遗留项是否完整纳入 Phase 2 并落实到具体子计划
5. 是否符合业界实践（阶段划分/里程碑/风险管理/依赖管理等）

**验证方法**：三份报告合计 7 轮递进交叉验证（全景→边界→职责→衔接→遗留→业界→终审），发现重复率 85%+，说明结论稳健。

---

### 3.1 检查点 1：整体计划与 Phase 2 边界与目标

#### 正面评价 ✅
| 检查项 | 结论 | 证据 |
|--------|------|------|
| Phase 1 做什么/不做什么 | ✅ 明确 | phase1/README §1.1 10 模块 + §1.2 18 行排除表（**业界最佳实践——明确排除比隐式假设更有效**） |
| Phase 2 做什么/不做什么 | ✅ 明确 | phase2/README §0 三子阶段表 + §1.5 7 行排除表 |
| Phase 3 做什么/不做什么 | ✅ 明确 | phase3/README §1.1-§1.3 + 分档 3a-min / 3a-full 验收 |
| 跨阶段能力归属 | ✅ 无重叠 | 14 项抽样（Token/Casbin/工单/scope/虚拟组/HR/存储/委托/可观测性/Multi-instance/JWKS/gRPC/AKSK）全部归属到唯一阶段，无歧义 |
| 部署形态边界 | ✅ 一致 | Phase 1/2 均为单实例 Docker Compose；Phase 3 切多实例 |
| 每阶段目标可独立演示 | ✅ 明确 | P1：27 用例；P2a：4 条；P2b：5 条；P2c：5 条；P3a：2 档分档 |

#### 反面发现（3 条 P3 + 1 条 P2 归到检查点 4）
| 编号 | 严重度 | 描述 | 建议 |
|------|--------|------|------|
| **P3-1** | P3 | roadmap.md Phase 2 第一行括号写「资源级鉴权（ltree）」，但 ltree `<@` 过滤实际在 2b Step 5，2a 仅 assigned。表述漂移让读者误解 2a 已含 ltree | roadmap.md Phase 2 括号改为「资源级鉴权：assigned（2a）+ ltree group/all（2b）」 |
| **P3-2** | P3 | 缺「Phase 1 交付能力 → Phase 2 各子阶段消费时点」交叉矩阵（如 ltree 路径被 2a 冗余 / 2b 过滤 / 2c 祖先链三个时点消费） | 在 phase2/README §1.6 补一张 Phase 1 能力 × 2a/2b/2c 消费矩阵 |
| **P3-3** | P3 | ~~phase2/README §0 标题写「Phase 2b：组织增强 + 工单升级 + **auth-enhance**」，但 §1.2 2b 子阶段明细**完全没列 01-auth-enhance**~~ **【误判，已核实】**：README §1.2 2b 子阶段表 L55 **已列** auth-enhance（「认证增强 \| 01-auth-enhance \| 多设备/密码复杂度 \| 已编写」），00 §6/§8 把 01 当 Step 7（M2b-4）亦一致。读者不会漏 auth-enhance 属 2b。 | ~~README §1.2 2b 小节补一行~~ **无需修改** |
| **P3-4** | P3 | 文档编号体系（00-implementation-plan / 01-12 PRD）与 Step 序号（0-11）两套编号混用，新人易混淆「Step 6 = 文档 06 还是步骤 6」 | 引用时显式区分「文档 03 / Step 5」（建议在 00 §1 开头加一条编号说明） |

---

### 3.2 检查点 2：Phase 2 内部 a/b/c 职责划分

#### 正面评价 ✅
| 检查项 | 结论 | 证据 |
|--------|------|------|
| 2a 定位（Registry + assigned） | ✅ 最小可验证单元 | 故意不做 scope/group/BFS/附件，只验证 Registry + ScopeResolver + TicketResource 三层架构能跑通 assigned 范围 |
| 2b 定位（组织增强 + 存储 + 体验） | ✅ 独立大块集中 | 虚拟组 + scope/HR（03）、对象存储附件（10）、多设备/密码策略（01）—— 三块无耦合，00 §4 标 Step 6 与 Step 7 可并行（批次 γ） |
| 2c 定位（组内委托） | ✅ 依赖 2b 但逻辑独立 | 不拆 2d 的 4 条论证充分（避免半交付；委托层验收面与 2b 不同）；Authorize 只加新条件不改 L2 层（分层隔离） |
| 职责重叠检查 | ✅ 零重叠 | assigned ↔ group/all ↔ owner/ancestor：三层过滤完全独立，同一能力只在唯一子阶段实现（工单 2a/2b/2c 用「子阶段边界表」精确写明增量） |
| 「为什么拆」论证 | ✅ 充分 | README §0 四条拆分理由（难 review / 渐进式验证 / 解耦 auth-enhance / 验收驱动拆分）逐条成立 |

#### 反面发现（2 条 P2 + 2 条 P3）
| 编号 | 严重度 | 描述 | 建议 |
|------|--------|------|------|
| **P2-1** | P2 | 2a→2b update 规则从「创建人 OR 处理人」收紧为「仅创建人」（C1），00 §7 风险登记表无此行为变更。2a 宣传后 2b 默默 403 | 00 §7 加 **RK-11**「2a→2b update 规则收窄：处理人失去 update 权限」+ 触发信号=2b 验收时用例行为变化 |
| **P2-2** | P2（阻塞 2c） | 04 §3 新路由列 `effective owner` / `虚拟组 owner 可删` 等资源级判断，未说明如何通过 Phase 1 `biz` 组的 CasbinAuth 中间件（路由级必须有 Casbin 码） | 04 §3 补「路由级权限实现策略」——建议复用 `org:update` Casbin 码 + 虚拟组 owner 行写入 Casbin p 表种子 |
| **P3-5** | P3 | 2b 体量偏大（5 Step / ~14 人日），含 HR Sync（最大不确定块 RK-3 概率=高）+ 虚拟组/scope + 存储 + 认证增强 | **已落地（2026-08-26 P2-D7）**：2b 拆为 core/org/ext 三轨，HR Sync 降 2b-ext 延后，关键路径最短化（2a→2b-core→2c） |
| **P3-6** | P3 | 000010 工单表组 DDL 缺 deleted_at，与项目「软删部分唯一索引三规范」的全局模式冲突（二审 A4）。虽属 DDL 一致性，但也是职责层的「遗漏检查：工单表是否遵循全局删除语义」 | 决策软删或标注例外（Part 2 A4） |

---

### 3.3 检查点 3：各计划衔接关系

#### 正面评价 ✅
| 衔接对 | 结论 | 关键衔接点证据 |
|--------|------|--------------|
| Phase 1 → 2a | ✅ 顺畅 | G-1/G-2 明确为 Step 0 首动作（见 Part 1 类别 1）；6 项 Phase 1 前置 checkbox 列于 README §1.6 |
| 2a → 2b | ✅ 顺畅 | 2b §1 前置明确：2a 全部交付物（工单+Registry+Move 级联）。P2-D1 决策提前拍板（组织 move 级联改写 tickets.org_path，**放在 2a Step 2 就做**，避免 2b 才做影响 scope 验证数据） |
| 2b → 2c | ✅ 顺畅 | 04 §1 前置绑定 2b-core + 2b-org 交付物（虚拟组/scope/BFS/工单可见性；HR Sync 属 2b-ext 延后非前置）；2c Authorize 只加新条件不改 L2（最小化变动原则） |
| 2c → Phase 3 | ✅ 顺畅 | phase3/README §1.4 明确「2c 不阻塞 Phase 3，但建议上线前完成」——关键路径正确（生产加固优先于组内委托治理） |
| 关键路径依赖图 | ✅ 可追踪 | 00 §4 α(2a)→β(2b-core 关键路径)→δ(2c)；2b-org 与 core 并行、2b-ext 延后，批次拓扑清晰 |

#### 反面发现（1 条 P2 + 5 条 P3）
| 编号 | 严重度 | 描述 | 建议 |
|------|--------|------|------|
| **P2-3** | P2 | **T8-T14 七条用例未映射到任何里程碑**（E1）。09 §8 2b 列了 7 条用例，00 §2 里程碑门禁表 + §8 SSOT 映射表均无记录。特别是 **T13**（vg_a admin 改 vg_a 内他人工单）有「2b:403 / 2c:200」双阶段预期，两端都漏——等于无人验收 | 00 §2 M2b-2 补「T8-T10、T12、T14」；M2b-5 补「T11、T13（2b 预期 403）」；M2c-3 补「T13（2c 预期 200）」 |
| **P3-7** | P3（与 D1 同根因） | 03-org-enhance.md 全文无 §1 前置条件章节。隐式依赖 Phase 1 组织树、2a 工单、hr-sync DDL、BFS RoleFetcher，但均未列 | 补 03 §1 前置条件清单 4 项（见 Part 2 D1） |
| **P3-8** | P3 | **Phase 1 验收门禁 vs 实测遗留状态存在张力**（review/05 P-7）。00 §3.1 把「Phase 1 验收（27 用例 / -race 全绿）」当 2a 硬前置，但前几轮代码验证发现 menu 模块无深度上限（F-缺口-1）+ audit/auth/user service 约 1000 行核心写路径代码无测试（T-新-1~3）。若严格按「Phase 1 验收通过」则现在不满足，停留在理想态 | 在 00 §3.1 或 Part 1 类别 2 中**显式声明**「进 2a 前最小闭合集 = G-1/G-2（阻塞）+ T-新-1~3（高优先测试守护）+ F-缺口-1（契约）」，把 V-*/R-1 列为 Phase 2 排期项（不阻塞），让前置条件与实际状态对齐 |
| **P3-9** | P3（与 D4 同根因） | 09 §1 2b 前置条件未区分 Step 5 vs Step 8，T11（附件 confirm）依赖 Step 6 storage | 09 §1 拆为两节，Step 8 补 Step 6 storage 前置（见 Part 2 D4） |
| **P3-10** | P3 | Phase 1 的 27 条验收用例仅「人工跑回归」，**未固化为强制门禁**。RK-10 风险登记表提及每里程碑必跑回归，但未声明必须 -race 全绿 + 27 用例不降。Phase 2 改动破坏 Phase 1 时无自动拦截 | 在 00 §5「工程流程规范」下补一条「本地验收纪律（个人项目无 CI 平台，不强行套 CI 门禁）：开工前及每次改完 2a 相关代码手动跑 `make acceptance`（=acceptance-phase1.sh）+ `make test -race` 全绿；MR 合入前自觉跑」。与 Phase 3「集成测试自动化」方向一致，但前移到 Phase 2 开工即生效 |
| **P3-16** | P3（**原 05 P-5 独立诉求**） | **03-org-enhance.md 内部 6 小节未标注里程碑归属（M2b-1/2）**。03 文档包含「虚拟组 CRUD / scope 枚举 / 组织角色边界 / BFS RoleFetcher 扩展 / HR Sync 人员部门同步 / Reparent 组织树重挂」6 块内容，00 §8 SSOT 映射表仅把 03 作为整体映射到 Step 4（M2b-1）+ Step 5（M2b-2），但 03 文档内部没有像 09 §0 那样在每个小节标题旁标注「本块属于 M2b-1 还是 M2b-2」。读者无法从 03 文档内部一眼看出各块的子阶段交付节奏。（注：review/05 P-5 括号中误写「二审 D2 建议补」为笔误——真实的二审 D2 是 00 §3 Step 2 漏列 menu 种子迁移，两者并非同一问题；本项是 review/05 P-5 新增的独立发现，在二审 A-F 中无对应编号） | 在 03-org-enhance.md 各小节标题旁括号中标注归属，例如：「§2 虚拟组 CRUD（M2b-1 / Step 4）」「§3 scope 枚举过滤（M2b-2 / Step 5）」「§4 组织角色边界（M2b-1 / Step 4）」「§5 BFS RoleFetcher（M2b-2 / Step 5）」「§6 HR Sync 同步（M2b-1 / Step 4）」「§7 Reparent 重挂（M2b-2 / Step 5）」，或在 03 §0 末尾补一张 09 风格的「子阶段边界映射表」，三列对应 2a/2b/2c。 |

---

### 3.4 检查点 4：Phase 1 遗留项落实

> 验证方法：对照 Part 1 的 6 类 37 项，交叉 grep Phase 2 全部文档。关键词（G-1/G-2/D2-49/D2-48/T-新/V-*/等）在 Phase 2 文档命中 57 次，分布于 02-authz-resource / 01-auth-enhance / README / 00-implementation-plan / 本文档 5 份核心计划。除 review/05、review/07、phase2/13-验证三份验证报告的交叉核对外，本章同时纳入了 12-phase1-backlog-and-phase2-review.md Part 1 类别 2 中来自「11-authz-architecture-review §7 七项设计文档落档」的遗留项分类（见 P2-4 说明），该部分为 12 号文档已分类但 review/05 验证报告未独立覆盖的内容。

#### 落实状态交叉表

| 类别 | 项数 | 落实方式 | 落实状态 |
|------|------|---------|---------|
| **1. 阻塞 2a（2 项）** G-1/G-2 | 2 | 00 §3 Step 0 明确列为 2a 首动作，附文件级清单 | ✅ 已落实到文件级 |
| **2. 建议 2a 前（4 项）** | T-新-1~3 测试盲区 | 00 §5.2 测试先行章要求 `Registry/ScopeResolver/状态机/密码策略` 单元 + `test-integration` 集成。但**未点名 T-新-1/2/3 的 audit/user/auth 三个模块的 Phase 1 写路径盲区** | ⚠️ 半落实：体系到位，具体盲区未显式列入，可能被遗忘（见 P3-11） |
| | **11 号 §7 设计文档落档（7 项）** | 00 §1 编码前拍板章（P2-D1~D5）是类似模式，但 11 号 §7 的 7 项落档**未显式并入编码前拍板** | ❌ **唯一真正遗漏**：见 **P2-4** |
| **3. 阻塞 2b（2 项）** D2-49②/D2-48 | 2 | D2-49② 明确为 Step 7 首任务且写入 00 §3 Step 7 首行；D2-48 已修仅需复核 | ✅ 已落实 |
| **4. 上线前决策（1 项）** D2-37 | 1 | 归属合理——Phase 2 内部不涉及 Phase 1 是否独立对外 | ✅ 归属合理 |
| **5. Phase 2 顺带（22 项）** | 22 | 00 §3 各 Step 内显式关联的：D2-42→Step 7、D2-34→Step 4（HR Sync 批量）、V-06→Step 7、D2-36 SCAN→Step 7 消解；其余 17 项为「顺带」级别（P3 代码质量项，不阻塞里程碑，由 PR 检查单把控） | ✅ 落实合理 |
| **6. 延后 Phase 3（6 项）** V-03/R-1/D2-38/缓存/D2-40/限流 | 6 | 全部在 phase3/README 对应模块有正确映射：V-03→Phase 3、R-1→Phase 3、缓存→Phase 3、D2-40→暂无里程碑、限流→Phase 3 | ✅ 全部落实 |

#### 反面发现（1 条 P2 + 1 条 P3）
| 编号 | 严重度 | 描述 | 建议 |
|------|--------|------|------|
| **P2-4** | **P2 阻塞 2a 编码质量** | `11-authz-architecture-review §7` 七项设计文档落档（鉴权不变量块 / L2/L3 顺序 / ReBAC 触发表 / SoD 延后 / scope 枚举映射表 / DB 错误拒绝测试用例 / P2-D6+D7 写入拍板）**未列入 00 §1 编码前拍板**。文档自述「若不变量不先收口，2a 编码时将转化为实现分歧」——L2/L3 顺序（属主短路在前还是在后）直接影响 TicketResource.Authorize 的实现逻辑。这是 37 项中唯一真正的落实缺口。（**⚠️ 级别判定来源提示**：该问题原文级别来自 12 号文档类别 2「11-authz-architecture-review §7 七项设计文档落档」，原始标注为「建议 2a 前完成（P2 设计收口，非阻塞）」。本文将其从「建议级」提升为「阻塞 2a 编码质量」级，依据是 11 号文档 L1-L7 的自述（若不变量不先收口，2a 编码时将转化为实现分歧，且 §2.1 明确指出 architecture.md 与 design-decisions.md 对 L2/L3 顺序存在内部矛盾）。该级别提升超出 review/05 验证报告的覆盖范围，请结合 11-authz-architecture-review.md 原文自行确认阻塞性判定是否符合预期。） | 在 00 §1 补 **P2-D6**（L2/L3 顺序 + 不变量块）+ **P2-D7**（SoD 延后决策），作为编码前拍板的第 6、7 项。 |
| **P3-11** | P3 | T-新-1~3 三块 service 测试盲区（audit/user/auth 约 1000 行关键写路径分支无守护）+ F-缺口-1 菜单深度，未在 00 §3 任何 Step 显式列入验收前置。2b 要动 auth_service（RT 升级），无守护测试直接改风险高（回退 bug 会漏） | 在 M2a-0（Step 0 验收）增加一条「T-新-1~3 补测 + F-缺口-1 闭合」作为开工 check，或在 00 §5.2 测试先行章点名这三项作为「Phase 2 扩展前必补的守护基线」 |

---

### 3.5 检查点 5：业界实践对标

#### 8 项对齐业界典范（✅ 优秀/良好）

| 业界实践项 | 本项目对照 | 对标结论 |
|----------|-----------|---------|
| **MVP 先行 + 增量交付** | Phase 1（框架 MVP）→ 2a（工单 MVP assigned 验证 Registry）→ 2b（范围扩展 group/all）→ 2c（组内委托治理）→ 3a（生产加固）→ 3b（按需演进）。每段有独立可演示面。 | ✅ **典范**：对齐 Lean Startup「Build-Measure-Learn」循环。2a assigned 先验证三层鉴权架构再扩展的策略是正确的「架构先最小化验证」实践。 |
| **里程碑门禁 + DoD** | Phase 1 M1-M7 到 Phase 2 M2a-0~M2c-3 共 18+ 里程碑。每个里程碑**只列新增可测用例**（Definition of Done 原则：不重复上一里程碑已验证内容）。 | ✅ **典范**：DoD 定义到位。业界许多项目里程碑只写「完成 XX 模块」，没有可验证用例清单，本项目写得非常好。 |
| **编码前拍板（Design Decisions Upfront）** | 00 §1 P2-D1~P2-D5 提前决策 org move 级联方案、org_path DDL 一次到位、HR fake client + fixture 契约测试、错误码时机表、验收脚本分段回归。P2-D6/D7 建议补齐（见 P2-4）。 | ✅ 良好：对齐「重大架构/设计决策前置，避免编码中返工」原则。 |
| **风险登记表（Risk Register）** | 00 §7 RK-1~RK-10 10 项风险，含概率/影响/缓解/触发信号 4 列，与风险绑定到具体 Step。 | ✅ 良好：符合 NIST SP 800-30 风险登记格式（风险事件、发生概率、影响程度、缓解措施、触发条件）。**建议补 RK-11（update 收窄）**。 |
| **验收脚本分段回归** | P2-D5 决策：三段独立 acceptance-phase2a/2b/2c.sh，上一段用例作为回归段，对齐 Phase 1 模式。 | ✅ 良好：对齐 Continuous Delivery「Build → Deploy → Acceptance → Regression」四步流水线。 |
| **迁移治理三规范 + 编号预分配** | README §2.4 预分配 000010-000014；00 §5.3 迁移检查单 6 条：① 编号不与历史冲突 ② up/down 幂等 ③ down 冲突行让位 ④ 软删部分唯一索引三规范 ⑤ GIN/GIST 索引 CONCURRENTLY ⑥ ACCESS EXCLUSIVE 时长评估。 | ✅ **典范**：对齐 Django/Rails 大型项目迁移治理规范（预分配编号避免多分支并行冲突、up/down 可逆演练、软删语义全局一致）。 |
| **分支策略（短 feature + 合入即删）** | 00 §5.1 对齐 Phase 1 模式：短分支从 dev 切出，Step 0 与 Step 1 因互为验证合分支，其余 Step 独立分支。 | ✅ 标准：对齐 GitHub Flow。**建议**：显式加「分支生命周期 ≤ 1 周（≤ 2 人日量级）」上限。 |
| **SSOT 纪律 + Docs-as-Code** | 每份 PRD 声明「本文档为实现 SSOT」，00 §5.4 规定「实现与 PRD 偏差必须同 PR 修文档」。 | ✅ **典范**：对齐「文档即代码」工程实践与「单一事实来源」信息架构原则。 |

#### NIST/OWASP 鉴权模型对照（补充）
`11-authz-architecture-review.md` 已做充分对照（NIST RBAC / XACML / Zanzibar ReBAC / 若依 data_scope / 混合模型），且对照结论准确。本项目「L1 路由级 Casbin RBAC → L2 资源级 GetFilter/canRead → L3 属主 canOperate + 短路」三层模型与 OWASP PEP-1（路径级拦截）/ PEP-2（资源级拦截）分层模型一致。

#### 反面发现（2 条 P2 + 4 条 P3，剩余 3 条 P3 已在前面 P3-4~P3-11 使用）
| 编号 | 严重度 | 描述 | 建议 |
|------|--------|------|------|
| **P2-5** | P2 | **缺少回滚策略和 Runbook**（07 P2-5 + 05 P-11）。00 §4 写了「每批次合入 dev + 打 tag」，但未写「验收失败如何回滚」。2b HR Sync 是最大不确定块（RK-3 概率=高），数据损坏时回滚步骤不清晰 | 在 00 §4 或 §5 补「回滚策略」节：代码回滚（git revert tag + 重新部署）；迁移回滚（先 down 迁移 + 冲突软删行加 `#del#` 后缀让位 + up→down→up 演练）；Redis RT value 结构升级回滚（旧版本不兼容需降级或接受用户重登）；HR Sync 回滚（通过 hr_sync_runs 对账表重跑）。**Step 6 已有完整模板，直接套用即可**。 |
| **P2-6** | P2 | **缺少容量规划与性能基线**（07 P2-6 + 05 P-13）。00 §6 节奏估算有人日但无性能基线。02 §2.6 提到 Casbin 全量 LoadPolicy <100ms，但无实测数据。Phase 2 引入 tickets + ticket_events 两张高流量表，无基线无法评估 Phase 3 容量规划缺口 | 2a 验收时用 pgbench 或 wrk 补一次基准性能测试（工单列表 P99、组织 ltree 查询 P99、Casbin LoadPolicy 耗时），写入 00 §6 附录。非阻塞，可 2a 完成后顺带做。 |
| **P3-12** | P3 | 缺全局 DoD（Definition of Done）统一声明。各里程碑有验收用例，但无统一口径（如「新代码覆盖率 ≥ X%」「迁移 up→down→up 演练通过」「`acceptance-phase1.sh` + `go test -race` 全绿」）。 | 在 00 §3 或 §5 加全局 DoD 模板，让 M2a/M2b/M2c 各里程碑验收使用统一基线。（**来源：05 P-10 立场修正**；原 05 报告 P-10 判定为 DoD「整体缺失」，本文经复核后判定 DoD 精神实质到位——各里程碑均有「只列新增可测用例」的清单作为 DoD 落地；此处仅建议补全局统一声明模板而非判定为完全空白。） |
| **P3-13** | P3 | HR Sync 真实部署期对接未排期。00 §1 P2-D3 已决策「fake client + fixture JSON 做契约测试」，但「真实 API 对接」的阶段归属不明确（部署期 / Phase 3 ops / 独立 spike？）。05 P-12 + 07 P3-5 都提到。 | 在 00 §3 Step 4 末尾加一行「部署期：独立 spike 对接真实 HR 人员/部门 API（与 2b 开发并行，不阻塞里程碑）」。 |
| **P3-14** | P3 | Phase 2 期间 HR Sync 失败告警缺失（07 P3-6）。Phase 3 才做可观测性，Phase 2 只有 slog + request_id，HR Sync 是定时任务，静默失败数天不被发现的风险真实存在 | 2b HR Sync 补简单失败告警：slog.Error 记录 + 可选 webhook（或返回非 0 让 cron 告警）。失败告警写入 `hr_sync_runs` 对账表（已有）。 |
| **P3-15** | P3 | 00 §6 节奏估算（28 人日 + 30% 缓冲 ≈ 36 人日）未显式纳入文档维护成本（07 P3-7）。本项目文档体系庞大（59 份 .md），每次实现偏差都需要同步文档，文档维护是不可忽视的成本 | 00 §6 估算补 10-15% 文档维护成本，或写进 30% 缓冲的说明中。 |

---

### 3.6 统一问题清单总表（按 P 编号，共 17 项）

#### P2 类（6 项，开工前必须闭合的最小集）

| 编号 | 一句话描述 | 阻塞性 | 对应 Part 2 二审编号 | 对应 Part 0 行动清单步骤 |
|------|-----------|--------|---------------------|------------------------|
| P2-1 | 2a→2b update 规则收窄未登风险登记 | 不阻塞 2a，但影响对外口径 | 二审 C1 | **步骤 3** |
| P2-2 | 2c 新路由 Casbin 路由级权限机制空白 | 阻塞 2c 实现 | 二审 B1 | **步骤 5** |
| P2-3 | T8-T14 七条用例未映射里程碑 | 不阻塞 2a，影响验收完整性 | 二审 E1 | **步骤 4** |
| P2-4 | 11 号 §7 设计文档落档粒度扩大化（原把 7 项全量列开工门禁，实仅 Q1 公式+Q2 默认拒绝+Q3 错误映射 2 项为 Authorize 真前置） | **仅 2 项阻塞 2a 编码质量**（Step 2 TicketResource.Authorize 编码前）；其余 5 项按消费时点拆分（#2/#5→2b，#3/#4→Phase 3，#6→2a 测试编写时） | 11 号 L7 自述仅 §3 不变量阻塞实现分歧；14 号原稿归类矛盾（类别 2「不阻塞」vs 开工门禁「阻塞」）已在 §1.2 消除 | **步骤 1**（20min，原 1h 拆分） |
| P2-5 | 缺少回滚策略和 Runbook | 不阻塞 2a，影响运维就绪 | —（业界对标） | **步骤 6** |
| P2-6 | 缺少容量规划与性能基线 | 不阻塞 2a，可 2a 后补 | —（业界对标） | P3 汇总表，择机处理 |

#### P3 类（12 项，不阻塞，择机或顺带处理）

| 编号 | 一句话描述 | 来源位置 | 建议处理时点 |
|------|-----------|---------|------------|
| P3-1 | roadmap Phase 2 行「ltree 划入 2a」表述漂移 | 检查点 1 | 2a 前修订（1 分钟） |
| P3-2 | 缺 Phase 1 能力 × Phase 2 消费时点矩阵 | 检查点 1（07 P3-1） | 2a 前补（15 分钟，或在步骤 1 同批） |
| P3-3 | README §1.2 2b 明细漏列 01-auth-enhance | 检查点 1（05 P-1） | 2a 前修订（1 分钟） |
| P3-4 | 文档编号 vs Step 编号混用说明缺失 | 检查点 1（05 P-2） | 00 计划补一行编号说明（1 分钟） |
| P3-5 | 2b 体量偏大（5 Step / 14 人日） | 检查点 2（07 P3-4） | 2b 启动前再评估拆分 |
| P3-6 | 工单表是否遵循软删三规范（全局模式一致性） | 检查点 2 + 二审 A4 | **步骤 2 决策时一并明确** |
| P3-7 | 03-org-enhance 缺 §1 前置条件章节 | 检查点 3 + 二审 D1 | **步骤 5 补** |
| P3-8 | Phase 1 验收门禁 vs 实测遗留张力（理想 vs 现实） | 检查点 3（05 P-7） | 00 §3.1 或类别 2 补声明（10 分钟） |
| P3-9 | 09 §1 2b 前置未区分 Step 5 vs Step 8 | 检查点 3 + 二审 D4 | **步骤 5 附带修** |
| P3-10 | Phase 1 27 用例未固化为强制门禁 | 检查点 3（05 P-8） | 00 §5.5 补本地验收纪律（10 分钟） |
| P3-11 | T-新-1~3 测试盲区 + F-缺口-1 未硬绑 2a 开工门禁 | 检查点 4 | M2a-0 或 00 §5.2 绑定（可选但强烈建议） |
| P3-12 | 缺全局 DoD（Definition of Done）声明 | 检查点 5（05 P-10 立场修正） | 00 计划补（15 分钟） |
| P3-13 | HR Sync 真实部署期对接未排期 | 检查点 5（05 P-12 + 07 P3-5） | 00 §3 Step 4 末尾加一行（1 分钟） |
| P3-14 | Phase 2 HR Sync 失败告警缺失 | 检查点 5（07 P3-6） | 2b Step 4 HR Sync 实现时顺带 |
| P3-15 | 节奏估算未含文档维护成本 | 检查点 5（07 P3-7） | 00 §6 估算补说明（1 分钟） |
| P3-16 | 03-org-enhance 文档内部 6 小节缺 M2b-1/2 里程碑归属标注（原 05 P-5） | 检查点 3（05 P-5 独立诉求） | **步骤 5 附加项表补**（20 分钟） |

---

## Part 4. 修订行动清单（步骤 1-6，总工时 ≈ 3 小时）

> **来源**：本总报告 Part 2 二审 12 条 P2 发现 + Part 3 验证报告 P2-1~P2-5 共 6 条 P2（P2-6 不阻塞）+ review/07 §7.3 的建议行动。性质：纯文档修订 + 设计决策，零代码改动。

### 执行总览

```
步骤 1（20min）11 号 §7 设计文档落档 ──────────────┐
步骤 2（15min） 工单表软删决策 ───────────────────┤
步骤 3（10min） update 规则收窄决策 ──────────────┤── 合计 ~2h5min
步骤 4（30min） 00 计划任务清单补全 ──────────────┤
步骤 5（30min） 04/03 PRD 补章节 ────────────────┤
步骤 6（20min） 补回滚策略节 ────────────────────┘
                                                    ↓
步骤 7（2a Step 0） G-1/G-2 接线 + 编码启动
```

---

### 步骤 1：拍板并落档 11-authz-architecture-review 两项真前置（P2-4，原 7 项粒度拆分）

- **优先级**：P2（仅 2 项真阻塞 2a 编码质量；其余 5 项按消费时点拆分，非开工门禁阻塞）
- **来源**：`11-authz-architecture-review.md §2.2 + §3`；§0.3 粒度修正说明；§1.2 分类矛盾消除表（已同步：真前置 2 项独立标为「开工门禁阻塞项」，不再与类别 2「不阻塞」冲突）。（⚠️ **粒度修正注记**：原稿把 §7 七项全量升级为阻塞，扩大了 11 号 L7「仅 §3 不变量阻塞实现分歧」的原始语义；本次按 7 项消费时点独立拆分，真前置仅 Q1 公式+Q2 默认拒绝+Q3 错误映射，其余 5 项延后。）
- **问题**：11 号 §7 明确声明「当前全部未执行，等确认后操作」。七项中，**直接影响 2a Step 2 TicketResource.Authorize 代码语义的只有 Q1（三层合取/析取公式写错=越权）和 Q3（Authorize 错误分支的 HTTP/错误码映射需编码前定死）**——这两项拍板约 20 分钟，11 号文档已给出建议值，属于「确认」而非「研发」。其余 5 项消费时点分别在 2b / 2a 测试 / Phase 3，不阻塞 2a 编码启动。
- **Step 1 立即执行内容（真前置，2a Step 2 编码前必闭合 ≈20min）**：

| # | 内容 | 落点 | 为什么真前置 |
|---|------|------|-------------|
| 1-a | 鉴权不变量公式：`Allow ⟺ L1 通过 ∧ L2 可见性通过 ∧ canOperate 通过`（路径 A，L2 在前，对齐 Freshdesk/Jira），含 Q2「默认拒绝 + 合取语义 + 拒绝点只有两个（L1 拒绝 / L2 未命中）」 | `architecture.md §4.1` | 合取结构写错（如把 ∧ 写成 ∨）= 越权；L2 可见性是数据访问边界门，先于属主 |
| 1-b | Authorize fail-closed 错误映射：① 确定性拒绝（L2 未命中）→ 403 + 70001；② DB 错误/无法判定 → **503** + Error 日志（对齐 Redis fail-close 既有模式）；③ 未注册资源码 → **500** + Error 日志（编程错误，不伪装 403） | `architecture.md §4.1` 同节 | `Registry.Authorize` 错误分支要写死返回码，不能靠实现者猜测；与 02 §5.4 全生命周期错误码对齐 |

- **可选 1 行带过（与 1-a 同节，边际成本≈0）**：
  Q5 缓存禁令声明「L2/L3 资源级判断**不缓存**，每请求实时查询；Phase 3 perm 缓存仅覆盖 L1 角色列表输入；任何资源级缓存提案必须连同失效级联方案评审（org move→子树全员、成员变更→单人、scope变更→该角色全员）」
- **同步登记（≈5min）**：P2-D6「L2/L3 顺序 + 鉴权不变量」追加到 `00-implementation-plan.md §1` 编码前拍板清单。

---

- **§7 其余 5 项 — 按消费时点拆分（非本步骤阻塞项，到点再执行）**：

| 原 # | 内容 | 消费时点 | 执行位置 | 为什么不阻塞 2a |
|------|------|---------|---------|----------------|
| 2 | L2/L3 顺序统一为「L1 → L2 可见性 → L3 属主」（路径 A，见 11 号 §2 已拍板）+ 顺带修 §12.7 「ReBAC」旧称 | **2b Step 5 前**（group/all scope ltree 落地时） | `design-decisions.md §3.4` | 2a 仅 assigned scope，L2 条件与 L3 属主定义等价，顺序差异**语义不显性化**；2b 引入 ltree 后顺序才影响实际行为 |
| 3 | ReBAC 业务侧触发表（替换/扩充现有四条技术信号） | **Phase 3 启动前评估** | `design-decisions.md §12.5` | ReBAC 是重型跨资源关系图引擎替换信号，2a/2b/2c 全 PRD 无任何 ReBAC 实现要求 |
| 4 | SoD 延后决策一句话（延后 + 届时优先动态 SoD）+ P2-D7 挂入 | **Phase 3 启动前登记** | `design-decisions.md §14` + `00-implementation-plan.md §1` | 审批流 Phase 3 才来（届时才是动态 SoD「不能审批自己发起的申请」而非静态互斥）；2a/2b/2c 全无 SoD 需求 |
| 5 | scope 枚举映射表（architecture §4.3 泛化模型 vs Phase 2 落地模型双轨统一） | **2b Step 5 前**（group/all 枚举生效时） | `architecture.md §4.3` | 2a 只有 assigned 枚举值，双文档枚举**一致**；2b 引入 group/all 落地时枚举不一致才会出实际 bug |
| 6 | DB 错误注入 → 拒绝 测试用例 | **2a 测试编写时**（编码完成后，与 R3–R8 集成测试同批） | `phase2/02-authz-resource.md` 测试用例节 | 测试品可延后；**注意**：Q3 错误映射「决策」仍需本步骤拍板（决策 ≠ 写测试，两者时点独立） |

- **操作**：执行「立即执行内容」的 1-a/1-b/可选 Q5/P2-D6 登记，共 3 个小落档动作；其余 5 项按消费时点表设置提醒，到点再处理。
- **预估**：20 分钟（纯文档拍板+落档，零代码）

---

### 步骤 2：决策 000010 工单表是否软删（A4 / P3-6）

- **优先级**：P2（阻塞 2a Step 2 迁移编写）
- **来源**：Part 2 二审 A4
- **问题**：00 §3 Step 2 写迁移 000010 须遵循「软删部分唯一索引三规范」，但 09-ticket.md §2a 与 modules/ticket.md §3.2 的 `CREATE TABLE tickets / ticket_types / ticket_comments` 均无 `deleted_at` 列，也无部分唯一索引。ticket_types.code 的 UNIQUE 是硬唯一，软删后无法复用 code。
- **决策选项**：

| 方案 | 说明 | 取舍 |
|------|------|------|
| A. 软删 | 补 `deleted_at` 列 + 部分唯一索引（`WHERE deleted_at IS NULL`） | 与 Phase 1 全局模式一致；工单通常需保留历史轨迹 |
| B. 硬删 | 在 00 §3 Step 2 标注「工单三表例外」 | 工单删除即物理删除；需明确 tickets → ticket_comments → ticket_events 的级联策略（软删 org 时 tickets.org_id 的孤儿引用已在 B2 处理） |

- **建议**：方案 A（软删），与项目全局模式一致，工单作为业务数据应保留历史。
- **落点**：`09-ticket.md §2a` DDL + `modules/ticket.md §3.2` + `00-implementation-plan.md §3 Step 2`
- **预估**：15 分钟（决策 + DDL 补列）

---

### 步骤 3：决策 2a→2b update 规则收窄（C1 / P2-1）

- **优先级**：P2（不阻塞 2a 但影响 2a 对外宣传口径）
- **来源**：Part 2 二审 C1 + Part 3 验证报告 P2-1
- **问题**：
  - 2a update：`创建人或处理人`
  - 2b update：`仅创建人`（策略 B「兄弟组透明读 ≠ 可改」的副作用）
  - 处理人改自己被分派工单的场景与兄弟组透明读无关，但规则一并收紧了。00 §7 RK-1~RK-10 没有提到此行为变更。
- **决策选项**：

| 方案 | 说明 | 取舍 |
|------|------|------|
| A. 保留收窄 | 2b update 仅创建人；处理人只能 close | 策略 B 简洁但收窄了处理人能力 |
| B. 分场景保留处理人 update | 2b update = 创建人 OR（处理人 AND 工单状态 = assigned/in_progress） | 更符合实际工单场景，但规则复杂度上升；需在 canOperate 矩阵里追加条件 |

- **建议**：方案 A（保留收窄），在 00 §7 加 **RK-11** 记录此行为变更；在 09 §5.2 显式标注「2a→2b update 权限收窄」。
- **落点**：`00-implementation-plan.md §7` 加 RK-11；`09-ticket.md §5.2` 标注
- **预估**：10 分钟

---

### 步骤 4：修复 00 计划任务清单 + T8-T14 里程碑映射（A1/A2/D2/D3/F1/E1 / P2-3）

- **优先级**：P2（不阻塞但影响实现完整性）
- **来源**：Part 2 二审 A1/D2/D3/A2/F1/E1 + Part 3 验证报告 P2-3
- **问题汇总与修复**：

| 二审编号 / P 编号 | 问题 | 修复动作 |
|-------------------|------|----------|
| A1 / D3（P2-3 相关） | 00 §3 Step 2 只列 3 张表，实际 09 §2a 共建 5 张（漏 ticket_type_fields + ticket_events） | 00 §3 Step 2 改为「迁移 000010：ticket_types / ticket_type_fields / tickets / ticket_comments / ticket_events」5 张表 |
| D2 | 00 §3 Step 2 漏列 menu_apis/Casbin 种子迁移 | 00 §3 Step 2 加一条「迁移 000010_menu（或并入 000010）：插入 ticket:list/create/read/update/close/assign/delete/comment/note 的 menu + menu_apis 行 + 角色绑定种子」 |
| A2 | README §2.4 的 000013 只写 file_objects，00 §3 列两张 | README §2.4 改为「附件：file_objects / ticket_attachments」 |
| F1 | P2-D4 错误码时机表漏列 20012（ErrDeviceNotFound） | P2-D4 表「20013」改为「20012（设备不存在）+ 20013（密码策略违规）」 |
| F2（附带） | P2-D4 把 90001-90004 笼统描述为「状态机 90002 等」 | 改为「90001-90004（ErrTicketNotFound / ErrTicketInvalidTransition / ErrTicketTypeNotFound / ErrTicketAlreadyClosed）」 |
| E1 / **P2-3** | T8-T14 七条用例未映射到任何里程碑 | 00 §2 M2b-2 行补「T8-T10、T12、T14」；M2b-5 行补「T11、T13（2b 预期 403）」；M2c-3 行补「T13（2c 预期 200）」。00 §8 SSOT 映射表同步。 |

- **落点**：`00-implementation-plan.md §2 + §3`；`README.md §2.4`
- **预估**：30 分钟

---

### 步骤 5：补 04 §3 路由级权限策略 + 03 §1 前置条件（B1/D1 / P2-2/P3-7）

- **优先级**：P2（阻塞 2c 实现 / 影响 03 完整性）
- **来源**：Part 2 二审 B1/D1 + Part 3 验证报告 P2-2/P3-7
- **核心问题与修复**：

| 二审编号 / P 编号 | 问题 | 修复动作 |
|-------------------|------|----------|
| B1 / **P2-2** | 04 §3 多个新路由的 Casbin 路由级权限机制未交代——effective owner 如何过 CasbinAuth 中间件是空白 | 在 04 §3 补「路由级权限实现策略」节（建议复用 `org:update` Casbin 码 + 虚拟组 owner 行写入 Casbin p 表种子）；00 §3 Step 9 补「router.go 注册新路由 + 中间件顺序」 |
| D1 / **P3-7** | 03-org-enhance.md 全文无「前置条件」章节 | 在 03 加 §1 前置条件：(1) Phase 1 organizations + user_orgs + ltree 可用；(2) Phase 2a 工单带 org_id/org_path 列已落地；(3) hr-directory-sync §2 DDL 与本文档同迁移；(4) BFS RoleFetcher 扩展与本 Step 同批 |

- **附加项（P3，建议一并处理，减少后续返工）**：

| 二审/P 编号 | 问题 | 修复动作 |
|-------------|------|----------|
| A5 | 000011 列散落三份文档无单一 SSOT | 在 03-org-enhance.md 增加「迁移 000011 完整列清单」表，列出所有 ALTER 语句并标注来源 PRD |
| A6 / P3-4 相关 | 03/00 误将 is_primary 列为 2b 增量（Phase 1 已存在） | 删除列举或显式标注「Phase 1 已存在」 |
| A7 | 03 文件迁移占位名 `0000xx_hr_source` 误导 | 改为 `000011_org_enhance` |
| B5 | 09 §5.4 附件表缺预签名下载行 | 09 §5.4 附件小节补一行「预签名下载 | POST /storage/presign/download | 隐含 ticket:read」 |
| B6 | modules/ticket §5.5 多了 09 §3 未列的 API（GET /ticket-types/:code） | 二选一对齐：09 补该行，或 modules 删除并加注释 |
| C2 / P2-2 相关 | 02 §2.3 canOperate 2c 增量表述含糊（只写 update 实际是 4 种操作） | 改为「对 **update/close/assign/delete** 的 ticket.org_id 增加 org admin/owner、ancestor owner 委托」 |
| C3 | modules/ticket §2.3 权限矩阵只写 2b/2c 不写 2a | update/close/assign 行补「2a：创建人或处理人」前缀 |
| E2 / P3-7 相关 | 03 复制 D1-D12 与 04 漂移 | 03 D1-D12 表替换为一行「验收 SSOT 见 [04-org-delegation §7](./04-org-delegation.md)」 |
| E3 | 00 §2 M2b-1 引用「03 测试表」未区分 2b/2c 节 | 00 §2 M2b-1 改为「03 §测试用例 **2b 节** + hr-directory-sync §7」 |
| D4 / **P3-9** | 09 §1 2b 前置未区分 Step 5 vs Step 8 | 09 §1 拆为「2b Step 5 前置」与「2b Step 8 前置」，Step 8 补 Step 6 storage 可用 |
| P3-16（原 05 P-5） | 03-org-enhance.md 内部 6 小节缺里程碑归属标注（M2b-1/2） | 03 文档内两种二选一方式：① 各小节标题旁括号标注：「虚拟组 CRUD（M2b-1 / Step 4）」「scope 枚举过滤（M2b-2 / Step 5）」「组织角色边界（M2b-1）」「BFS RoleFetcher（M2b-2）」「HR Sync 同步（M2b-1）」「Reparent 重挂（M2b-2）」；或② §0 末尾补一张 09 风格「子阶段边界映射表」（三列：2a/2b/2c × 6 个小节交付）。预计 20 分钟。 |

- **落点**：`04-org-delegation.md §3`；`03-org-enhance.md §1`；其余各 PRD 对应章节
- **预估**：约 50 分钟（核心 P2 修复 15 分钟 + 原 10 项附加 P3 项 15 分钟 + P3-16 原 05 P-5 里程碑归属标注 20 分钟）。核心 P2 修复与前 10 项附加 P3 建议一并做；P3-16（03 里程碑归属）可在 03 §1 前置条件补完后顺手完成。

---

### 步骤 6：补回滚策略节（P2-5）

- **优先级**：P2（不阻塞 2a 但影响运维就绪）
- **来源**：Part 3 验证报告 P2-5（review/07 §5.3 P2-5 + 05 P-11）
- **问题**：00 §4 写「每批次收口：合入 dev + 打 tag + 全量回归」，但未描述「如果验收失败如何回滚」。2b HR Sync 是最大不确定块（RK-3 概率=高），数据损坏时回滚步骤不清晰。
- **补充内容（直接写入 00 §4 或新增 §5.5 即可）**：

```markdown
### 回滚策略

1. 代码回滚
   - 每批次收口打 tag（如 `phase2a-complete` / `m2b1-step4-hrsync`）
   - 回滚：`git revert` 到上一个 tag + 重新部署（合入使用 `--no-ff`，保证 revert 清晰）

2. 迁移回滚
   - down 迁移必须可逆且先让位（00 §5.3 迁移检查单第 3 条：软删冲突行加 `#del#` 后缀让位）
   - 大子树 DDL（如 000011 含数据回填）在真实 PG 实例先演练 `up → down → up`
   - 回滚前先处理会阻塞还原的数据（如部分唯一索引还原前，冲突软删行先 `UPDATE ... SET code = code || '#del#' || id WHERE deleted_at IS NOT NULL`）

3. Redis 状态回滚
   - D2-49② RT value 结构化升级后，旧版本代码不兼容新结构
   - 回滚方式二选一：(a) 同步降级 Redis 中的 RT value（写脚本遍历替换）；(b) 接受受影响用户重新登录（推荐 b：简化回滚流程）
   - devices 集合在回滚后自然失效（旧代码不读该集合，无副作用）

4. HR Sync 回滚
   - `hr_sync_runs` 对账表保留每次运行完整记录（source=hr 记录的写入前后快照）
   - 回滚后重跑 HR Sync（fake client + fixture）即可恢复正确的 source=hr 数据
   - 关键不变量：**source=local 的虚拟组/owner/org_member_role 绝对不受 HR 回滚影响**（HR Sync 只处理 source=hr）
```

- **落点**：`00-implementation-plan.md §4` 或新增 §5.5
- **预估**：20 分钟

---

### P3 项汇总（择机处理，不单独排期）

| 编号 | 问题 | 来源 | 建议处理时点 |
|------|------|------|------------|
| P3-1 | roadmap.md Phase 2 「ltree 划入 2a」表述漂移 | Part 3 §3.1 | 2a 前顺手改（1 分钟） |
| P3-2 | 缺 Phase 1 能力 → Phase 2 消费时点矩阵 | Part 3 §3.1（07 P3-1） | 2a 前补 phase2/README §1.6（15 分钟） |
| P3-3 | README §1.2 2b 明细漏列 01-auth-enhance | Part 3 §3.1（05 P-1） | 2a 前顺手改（1 分钟） |
| P3-4 | 文档编号 vs Step 编号混用说明缺失 | Part 3 §3.1（05 P-2） | 00 §1 开头加编号说明（1 分钟） |
| P3-5 | Phase 2b 体量偏大（5 Step / 14 人日） | Part 3 §3.2（07 P3-4） | 2b 启动前评估拆分可能性 |
| P3-8 | Phase 1 验收门禁 vs 实测遗留张力 | Part 3 §3.3（05 P-7） | 00 §3.1 加最小闭合集声明（10 分钟） |
| P3-10 | Phase 1 27 用例未固化为强制门禁 | Part 3 §3.3（05 P-8） | 00 §5.5 工程流程补本地验收纪律（10 分钟） |
| P3-11 | T-新-1~3 测试盲区 + F-缺口-1 未硬绑开工 | Part 3 §3.4 | 2a Step 0 验收前补（强烈建议） |
| P3-12 | 缺全局 DoD（Definition of Done）声明 | Part 3 §3.5（05 P-10） | 00 §3 或 §5 补（15 分钟） |
| P3-13 | HR Sync 真实部署期对接未排期 | Part 3 §3.5（05 P-12 / 07 P3-5） | 00 §3 Step 4 末尾加一行（1 分钟） |
| P3-14 | Phase 2 HR Sync 失败告警缺失 | Part 3 §3.5（07 P3-6） | 2b Step 4 HR Sync 实现时顺带（slog.Error + 非 0 退出码） |
| P3-15 | 节奏估算未含文档维护成本 | Part 3 §3.5（07 P3-7） | 00 §6 补说明（1 分钟） |
| **P2-6** | 容量规划与性能基线 | Part 3 §3.5（07 P2-6） | 2a 验收时用 pgbench 或 wrk 做一次基准测试 |

---

### 完成标准

| 步骤 | 完成标志 |
|------|----------|
| **1** | `architecture.md §4.1/§4.3` + `design-decisions.md §3.4/§12.5/§14` + `00-implementation-plan.md §1 P2-D6/D7` 均已写入 |
| **2** | `09-ticket.md §2a` DDL 含 `deleted_at` 列 + 部分唯一索引（或在 00 §3 Step 2 明确标注「工单三表例外，走硬删」） |
| **3** | `00-implementation-plan.md §7` 有 **RK-11** 行；`09-ticket.md §5.2` 有「2a→2b update 权限收窄」显式标注 |
| **4** | `00 §2` 里程碑表含 T8/T9/T10/T11/T12/T13/T14 完整映射；`00 §3 Step 2` 列 5 张表 + menu + Casbin 种子；`README §2.4` 与 00 一致；P2-D4 含 20012 + 90001-90004 完整列表 |
| **5** | `04-org-delegation.md §3` 有「路由级权限实现策略」节；`03-org-enhance.md` 有 §1 前置条件；附加 P3 项已按列修或明确推迟 |
| **6** | `00-implementation-plan.md §4` 或 §5.5 有完整的「回滚策略」四小节（代码/迁移/Redis/HR Sync） |

**全部完成后，Phase 2a 即可正式进入 Step 0 编码（G-1/G-2 接线）。**

---

## Part 5. 最终结论与后续建议

### 5.1 综合结论

项目计划成熟度高，体现在：

1. **分层清晰**：Phase 1（框架）→ Phase 2（业务）→ Phase 3（加固），每阶段目标明确、边界清晰
2. **渐进式交付**：2a→2b→2c 拆分有 4 条充分论证，每段可独立验收
3. **风险管理到位**：00 §7 十项风险登记 + 五维度（概率/影响/缓解/触发信号），符合 NIST SP 800-30
4. **文档体系完整**：59 份文档覆盖 design/proposal/modules/phase1-3/review，SSOT 原则贯彻
5. **遗留项管理**：37 项遗留项六类分类，逐项标注阶段归属和处置建议，36/37 落实到位
6. **业界对齐**：MVP 分层、里程碑 DoD、编码前拍板、风险登记、验收分段、迁移治理、分支策略、Docs-as-Code 八项达到业界典范；与 NIST RBAC / OWASP PEP 分层一致

**无 P0/P1 阻塞项，6 项 P2 在开工前闭合（步骤 1-6，合计约 2 小时 5 分钟纯文档）即可进入 2a。**

### 5.2 旧文档归档与删除记录

本文档生效后，下列 5 份旧文档**已统一删除（用户指令 2026-08-25）**，内容已完整合并至 14 号文档，无信息回退风险：

（注：原计划给以下 5 份源文档顶部加「已废弃声明」模板；实际按用户指令 **2026-08-25 已直接删除这 5 份源文档**，模板保留作为历史记录，不再需要手动给旧文件加声明。）

```markdown
> ⚠️ **已废弃声明**：本文档内容已合并至 [14-phase2-plan-review-and-remediation.md](./phase2/14-phase2-plan-review-and-remediation.md)（若位于 review 目录则路径为 `../phase2/14-phase2-plan-review-and-remediation.md`）。
> 自 2026-08-25 起，以 14 号文档为 Phase 2 计划审查类内容的唯一 SSOT，本文档仅保留供历史参考，不再更新。
```

涉及文件列表（5 份，**已删除 2026-08-25**）：
- `docs/phase2/12-phase1-backlog-and-phase2-review.md`（Phase 1 遗留 37 项分类 + Phase 2 六维度二审）
- `docs/phase2/13-plan-remediation-actions.md`（修订行动清单步骤 1-6 原稿）
- `docs/phase2/13-project-plan-multi-round-verification.md`（多轮验证报告版本 3 原稿）
- `docs/review/05-plan-validation.md`（多轮验证报告版本 1 原稿）
- `docs/review/07-project-plan-verification.md`（多轮验证报告版本 2 原稿）

### 5.3 下一步行动（用户可选）

1. ✅ **立即（建议）**：执行 Part 4 步骤 1-6（纯文档修订 + 设计决策，约 3 小时）
2. 执行完步骤 1-6 后，按 Part 0.3 的开工门禁 checklist 逐项勾选
3. 通过 2a Step 0（G-1 接线 + G-2 stub 删除）
4. 如用户需要，可由 Agent 代为执行步骤 1-6（所有改动仅触及文档，零代码）
