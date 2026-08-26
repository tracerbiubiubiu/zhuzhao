# 00 - Phase 2 实施总计划（Execution Plan）

> **定位**：六份 PRD（[01](./01-auth-enhance.md) / [02](./02-authz-resource.md) / [03](./03-org-enhance.md) / [04](./04-org-delegation.md) / [09](./09-ticket.md) / [10](./10-storage.md)）回答「做什么」；本文回答「**怎么执行**」——编码前拍板、里程碑门禁、任务分解、批次与分支、流程规范、节奏与风险。  
> 组织方式对齐 [phase1/README §2–§4](../phase1/README.md) 与 [phase1/00-pre-coding-decisions.md](../phase1/00-pre-coding-decisions.md)。  
> 创建日期：2026-08-19。输入：[phase1/12 号验收报告](../phase1/12-phase1-acceptance-report.md)（27/27 通过、修复已合入、G-1/G-2 待 2a Step 0 收尾）。

---

## 1. 编码前拍板（进入 Step 1 前确认）

### P2-D1：组织 move 与 `tickets.org_path` 一致性 ⚠️ PRD 未覆盖 → ✅ 已拍板

**背景**：Phase 1 `POST /orgs/move` 更新组织子树 ltree path；[09-ticket §2a](./09-ticket.md) 规定创建工单时从 organizations 读 path 冗余到 `tickets.org_path`（GIST 过滤用）。**全部文档未定义组织移动后存量工单 org_path 的更新**——不处理则 2b scope=group 静默漏单（旧工单从主管列表消失）。

| 方案 | 说明 | 取舍 |
|------|------|------|
| **A. move 事务内级联改写（推荐）** | org move 同一事务中 `UPDATE tickets SET org_path = new_path || subpath(org_path, nlevel(old_path)) WHERE org_path <@ old_path` | 数据恒一致；move 大子树锁行增多（内部系统量级可接受）；与 Phase 1 Move 的子树 FOR UPDATE 模式天然衔接 |
| B. 查询时 JOIN 实时取 path | 不冗余 | GIST 索引失效、SQL 复杂化，与两份 PRD 冲突 |
| C. 接受 stale | 不处理 | scope=group 静默漏单——不可接受 |

**✅ 已采纳方案 A（2026-08-19 用户拍板）**：move 事务内级联改写。落地动作在 **Step 2**（建表同批扩展 OrgService.Move）+ **Step 5** 回归测试（move 后 scope 过滤仍正确）。

### P2-D2：2a 即建 org_id/org_path 列

按 PRD 执行：DDL 在 2a 一次到位（NOT NULL），创建工单校验 org 存在并读 path；过滤 2b 才启用。避免 2b 二次迁移。无需额外拍板，写明防歧义。

### P2-D3：HR 目录 API 开发期策略

2b HR Sync 依赖公司人员/部门 API，开发期无真实环境。**HRDirectoryClient 接口化 + fake client + fixture JSON 做契约测试**（对齐 Phase 1 stub RoleFetcher 的测试模式）；真实对接在部署期另排（contract 不变）。

### P2-D4：错误码分段写入时机

| 码段 | 写入时机 | PRD |
|------|---------|-----|
| 90001 `ErrTicketNotFound`（+状态机 90002 等） | Step 2 | [09-ticket §7](./09-ticket.md) |
| 91001–91004 | Step 6 | [10-storage §6](./10-storage.md) |
| 20013（密码策略） | Step 7 | [01-auth-enhance](./01-auth-enhance.md) |
| 50008–50010 | Step 9 | [04-org-delegation §5](./04-org-delegation.md) |

均同步写入 `errcode.go` + [api/errcode.md](../api/errcode.md)，勿改号。

### P2-D5：验收脚本分段

对齐 `scripts/acceptance-phase1.sh` 模式，新增 `acceptance-phase2a.sh` / `2b` / `2c` 三段，各自独立可运行；2b/2c 脚本头部先跑上一段用例做回归（防框架级回归，Phase 1 的 27 用例作为常驻回归段并入 2a 脚本）。

---

## 2. 里程碑门禁（对齐 phase1 M1–M7 模式）

> 原则：每个里程碑只列**该里程碑新增可测**用例；用例号沿用各 PRD 既有编号（R/T/D/S/A），**不重编号**（PRD 交叉引用已成型）。未到里程碑勿误判失败。

| 里程碑 | 完成 Step | 新增可验证 | 验收命令 | 说明 |
|--------|-----------|-----------|---------|------|
| **M2a-0** 接线收尾 | 0 | 无业务用例；装配断言：`grep NewRegistry wire_gen.go` 命中 2 处、启动日志无输出（空表） | Phase 1 全量测试回归 | G-1 Registry 接线 + G-2 删 authz_service.go stub（[02 §1 Step 0](./02-authz-resource.md)） |
| **M2a-1** 资源级鉴权可用 | 1 | R1–R2（Registry 单测）+ TicketResource **契约测试**（fake repo） | `go test ./internal/pkg/resource/... ./internal/service/ticket/...` | R3–R8 需工单真表，落 M2a-2 验证 |
| **M2a-2** 工单 MVP | 2 | T1–T7 + R3–R8（真表集成）+ [README §1.1](./README.md) 4 条验收 | `bash scripts/acceptance-phase2a.sh` | 迁移 000010/000015/000016；assigned 过滤；404 语义；P2-D1 级联（若采纳）；模板预填 + 关联鉴权 |
| **M2a-3** 2a 全量 | 3 | Phase 1 27 用例回归 + T/R 全量 | 同上（含回归段） | 对抗路径：不可见 404、无权限 403、状态机 90002 |
| **M2b-1** 组织增强 | 4 | [03 测试表](./03-org-enhance.md) + [hr-directory-sync §7](../proposal/hr-directory-sync.md) 用例 | `make test-integration` | 迁移 000011；虚拟组/临时成员/BFS/HR Job（fake client） |
| **M2b-2** scope 升级 | 5 | R9–R12 + **D11/D12 首次验收** | ticket 集成测试扩展 | 策略 B 透明读 + project_isolated；P2-D1 回归 |
| **M2b-3** 附件 | 6 | S1–S6 | `go test` + compose MinIO 冒烟 | 迁移 000013；预签名 + confirm |
| **M2b-4** 认证增强 | 7 | A1–A6 | `go test ./internal/service/...` | 设备列表/踢出（单轨道）+ 密码策略；可与 M2b-3 并行 |
| **M2b-5** 2b 全量 | 8 | [README §1.2](./README.md) 5 条验收 + 2a 回归 | `bash scripts/acceptance-phase2b.sh` | 策略 B 全景 + HR 同步链路 |
| **M2c-1** 委托 API | 9 | D1–D6 | `make test-integration` | 迁移 000014；组内防提权（50008–50010） |
| **M2c-2** Authorize 升级 | 10 | D7–D9 | 同上（扩展） | org admin/owner + ancestor owner |
| **M2c-3** 2c 全量 | 11 | D1–D12 + D10 HR 隔离回归 + 2a/2b 回归 | `bash scripts/acceptance-phase2c.sh` | 全量收口 |

---

## 3. 任务分解（Step → 文件级）

> 设计细节（DDL/SQL/接口签名）以各 PRD 为 SSOT，此处只列执行清单与 PRD 节点。

### Step 0（M2a-0）— Phase 1 遗留收尾

- [ ] `router.Deps` 加 `Registry resource.Registry` 字段 + `router.New()` 启动清单日志（[02 §1 Step 0](./02-authz-resource.md) 四项）
- [ ] 删 `internal/service/authz_service.go` + `wire.go` provider 调整 + `make wire`
- [ ] `router_test.go` 手工构造补 `Registry: resource.NewRegistry()`
- [ ] 验证：grep 双 `NewRegistry`；`go build` + Phase 1 单测/集成全量回归

### Step 1（M2a-1）— authz-resource

- [ ] `registry_test.go` 补 Authorize/GetFilter 单测（R1/R2）
- [ ] `internal/service/authz/scope_resolver.go`：assigned 语义（ReadAnchorPaths 空 + `created_by OR assigned_to` 谓词）
- [ ] `internal/service/ticket/resource.go`：TicketResource 骨架 + 契约测试（fake repo，不依赖真表）
- [ ] wire：NewTicketService 接收 Registry 自注册（依赖 Step 0 接线；拓扑序保证注册先于 router.New）

### Step 2（M2a-2）— ticket MVP

- [ ] 迁移 **000010**：`ticket_types` / `tickets` / `ticket_comments`（含 org_path GIST；幂等 + down + 软删部分唯一索引三规范）
- [ ] 迁移 **000015**：`ticket_templates`（模板表，2a 前移，DDL 见 [09 §2](./09-ticket.md#工单模板2a-前移迁移-000015)）
- [ ] 迁移 **000016**：`ticket_relations`（关联表，2a 前移，DDL 见 [09 §2](./09-ticket.md#工单关联2a-前移迁移-000016)）
- [ ] 90001/90002 写入 `errcode.go` + `errcode.md`（P2-D4）
- [ ] TicketService/Handler/Router：CRUD + 状态机（transitions JSONB 校验）+ ticket_events；创建时同事务读 org.path 写 org_path
- [ ] 工单模板：列表/详情 API + `POST /tickets` 支持可选 `template_code` 预填字段（`default_sla_minutes` 仅存储，Phase 3 启用）
- [ ] 工单关联：建立/查询关联 API；建立关联时对 target_id 走 L2/L3 鉴权（防止越权关联他人工单）
- [ ] **P2-D1（已采纳 A）**：`OrgService.Move` 扩展级联改写 `tickets.org_path` + 集成测试
- [ ] `scripts/acceptance-phase2a.sh`（含 Phase 1 27 用例回归段）
- [ ] R3–R8 真表集成测试落位（[02 §3 R 表](./02-authz-resource.md)）

### Step 3（M2a-3）— 2a 集成验收

- [ ] 全量脚本通过；PRD 用例表标注状态；12 号报告模式出 2a 验收记录（可选）

### Step 4（M2b-1）— org-enhance

- [ ] 迁移 **000011**：`source`/`external_id`/`synced_at`、`user_orgs.ticket_scope`/`is_primary`/`source`/`expires_at`、`ticket_visibility`（[hr-directory-sync §2](../proposal/hr-directory-sync.md) DDL）
- [ ] 虚拟组 CRUD（org_type=4、code 前缀 `vg_`）+ Reparent（HR 撤销部门上挂最近实体祖先）
- [ ] 临时成员：`expires_at` 读取时过滤（或惰性清理 Job，随 PRD）
- [ ] BFS 三源 RoleFetcher 扩展（直接 + 组织角色 + 继承）
- [ ] `HRDirectoryClient` 接口 + `HRSyncService`（fake client 契约测试，P2-D3）+ `hr_sync_runs` 对账表 + 分布式锁 Cron
- [ ] 实体 move 子树 path 级联**含虚拟组**（Phase 1 Move 扩展）

### Step 5（M2b-2）— ticket scope 升级

- [ ] `ReadAnchorPaths`（挂载实体 anchor 透明读）+ GetFilter 升级 `org_path <@ ANY($2::ltree[])`（[09 §5.2](./09-ticket.md)）
- [ ] `project_isolated` 回退分支（仅直接 org path + scope）
- [ ] R9–R12 + D11/D12 集成测试；**P2-D1 回归**（move 后 scope 过滤仍正确）

### Step 6（M2b-3）— storage

- [ ] compose 加 MinIO；`config.storage` 段（[10 §2](./10-storage.md)）
- [ ] 迁移 **000013**：`file_objects` / `ticket_attachments`
- [ ] `internal/pkg/storage/s3_client.go` + 预签名 upload/download + confirm（HEAD 校验）+ 附件列表/删除 API
- [ ] 91001–91004 错误码；S1–S6 测试

### Step 7（M2b-4）— auth-enhance（可与 Step 6 并行）

- [ ] **首任务（D2-49②）**：devices 集合初始化（SADD/SREM 接入登录/登出/吊销链路）+ RT value 结构升级（hash 与设备元数据并存，Refresh 比较逻辑与守护测试同 Step 改造——[01 §2.1](./01-auth-enhance.md)）
- [ ] 设备列表/踢出 API（沿用 `devices:{uid}` 集合；**单轨道**：仅删单设备 RT，不触碰 `user:disabled`，[01 §0 B3](./01-auth-enhance.md)）
- [ ] `ValidatePasswordPolicy` + 20013（策略归一：binding 保留 required，长度/复杂度统一走策略校验，[01 §3.4](./01-auth-enhance.md)）
- [ ] 迁移 **000012** 视需要（纯 config 则无迁移，编号顺延规则见 [README §2.4](./README.md)）
- [ ] A1–A6 测试（miniredis）

### Step 8（M2b-5）— 2b 集成验收

- [ ] `acceptance-phase2b.sh`（头段跑 2a 回归）；2b 验收 5 条全过

### Step 9（M2c-1）— org-delegation

- [ ] 迁移 **000014**：`organizations.owner_user_ids` / `user_orgs.org_member_role`（[04 §2.1](./04-org-delegation.md)）
- [ ] `OrgDelegationService`：EffectiveOrgPriority / IsOrgAdminOrOwner / IsAncestorOwner
- [ ] SetOwners / SetMemberRole / AddMember / RemoveMember / 虚拟组删除扩展（防提权矩阵 [04 §3](./04-org-delegation.md)）
- [ ] 50008–50010 错误码；D1–D6 集成测试

### Step 10（M2c-2）— ticket Authorize 升级

- [ ] `canOperate` 扩展（org admin/owner + ancestor owner，[04 §4](./04-org-delegation.md)）；D7–D9 集成测试

### Step 11（M2c-3）— 2c 集成验收

- [ ] `acceptance-phase2c.sh`：D1–D12 全量 + D10 HR 隔离 + 2a/2b 回归

---

## 4. 批次与并行（单人 + AI 协作模式）

> Phase 1 即单人开发模式，Phase 2 沿用。并行仅指「接口契约先定、实现交替推进」。

```
批次 α（串行关键路径）        批次 β（组织主线）           批次 γ（独立能力）        批次 δ（委托收口）
Step 0 → 1 → 2 → 3     →    Step 4 → 5            →    Step 6 ∥ Step 7     →   Step 8 → 9 → 10 → 11
   (2a)                      (2b 核心复杂度)            (无相互依赖)              (2b 验收 + 2c)
```

- **批次 β 说明**：策略 B（透明读/isolated/anchor 计算）是 Phase 2 最大语义复杂度，独占注意力不与 storage/auth-enhance 交叉。
- **批次 γ 说明**：Step 6 与 7 零依赖；单人时建议先 6 后 7（附件影响 2b 验收演示面更大），但两步的 API 契约可在批次 β 期间先定。
- **每批次收口**：合入 dev + 打 tag（如 `phase2a`）+ 全量回归，再进下一批次。

---

## 5. 工程流程规范

### 5.1 分支策略（对齐 [phase1/README §2](../phase1/README.md)）

短 feature 分支从 `dev` 切出，PR base = `dev`，合入后删除：

| 分支 | 覆盖 |
|------|------|
| `feature/step-2a-authz-resource` | Step 0 + 1 |
| `feature/step-2a-ticket` | Step 2 + 3 |
| `feature/step-2b-org-enhance` | Step 4 + 5 |
| `feature/step-2b-storage` / `feature/step-2b-auth-enhance` | Step 6 / 7 |
| `feature/step-2c-org-delegation` | Step 9 + 10 + 11 |

> Step 0 与 1 同分支原因：接线（Deps 字段）与第一个消费者（TicketResource 自注册）互为验证，拆开则接线无回归面。

### 5.2 测试先行（对齐 [phase1/README §3](../phase1/README.md) + B4 测试落点）

| 层 | 范围 | 运行 |
|----|------|------|
| 单元 | Registry / ScopeResolver / 状态机 / 密码策略 / EffectiveOrgPriority | `go test -race ./internal/...` |
| 集成 | ticket/org/storage repo + service（testcontainers PG + MinIO） | `make test-integration` |
| 路由行为 | router_test.go 扩展 ticket/storage 路由与中间件顺序 | 同单测 |
| 验收脚本 | 分段脚本（P2-D5） | `bash scripts/acceptance-phase2x.sh` |

### 5.3 迁移 PR 检查单（每迁移必过）

1. 编号符合 [README §2.4](./README.md) 规划（无迁移则更新该表标注顺延）；
2. 幂等（IF NOT EXISTS / WHERE NOT EXISTS）；
3. down 可逆且先让位（000006 模式）；
4. 唯一索引带 `WHERE deleted_at IS NULL`（F-6 教训）;
5. `testutil/testdb_integration.go` 迁移列表同步 + 新表级联测试；
6. 大子树 DDL（000011 含回填）在真实 PG 演练 up→down→up。

### 5.4 提交与文档纪律

- 提交按问题域分组中文提交（Phase 1 六提交模式：`feat(authz)` / `feat(ticket)` / `fix(...)` / `docs(phase2)`），每提交可独立构建；
- **PRD 是 SSOT**：实现与 PRD 偏差必须同 PR 修文档；验收用例状态在 PRD 测试表回标；
- Phase 1 文档原则不动：发现 Phase 1 缺陷 → 修代码 + 记录到 [phase1/11-code-review.md](../phase1/11-code-review.md) 模式的新条目，不静默改。

---

## 6. 节奏估算（单人全职 + AI 结对假设）

> 规划参考值，非承诺；含 30% 缓冲后的名义工期见末行。

| 里程碑 | 估算（人日） | 主要工作量 |
|--------|-------------|-----------|
| M2a-0 | 0.5 | ~5 行接线 + 删 stub + 回归 |
| M2a-1 | 2 | Registry 已有接口；ScopeResolver + 契约测试 |
| M2a-2 | 4–5 | 3 表迁移 + 状态机 + ~11 条 API + 验收脚本 + P2-D1 |
| M2a-3 | 1 | 脚本调通 + 全量回归 |
| **2a 小计** | **~8** | |
| M2b-1 | 5–6 | HR Sync（fake client + 对账）是最大不确定块 |
| M2b-2 | 3 | 策略 B 语义 + 回归 |
| M2b-3 | 3 | MinIO + 预签名 + confirm |
| M2b-4 | 2 | 设备 API + 密码策略 |
| M2b-5 | 1 | 验收脚本 |
| **2b 小计** | **~14** | |
| M2c-1 | 3 | 委托 API + 防提权 |
| M2c-2 | 2 | Authorize 扩展 |
| M2c-3 | 1 | 全量验收 |
| **2c 小计** | **~6** | |
| **合计** | **~28 人日** | 含 30% 缓冲 ≈ **36 人日 / 7–8 周**；关键路径（α→β→δ）≈ 6 周 |

---

## 7. 风险登记表

| # | 风险 | 阶段 | 概率 | 影响 | 缓解 | 触发信号 |
|---|------|------|------|------|------|---------|
| RK-1 | 组织 move 后 `tickets.org_path` 陈旧 → scope=group **静默漏单** | 2a/2b | 低（P2-D1 已拍板方案 A） | 高 | move 事务内级联改写 + Step 5 回归测试 | move 后旧工单从主管列表消失 |
| RK-2 | 策略 B 语义复杂度（anchor 透明读 vs isolated 回退 vs scope 并集） | 2b | 中 | 高 | R9–R12 先行集成测试；D11/D12 在 2b/2c 双验收 | scope 过滤行数与预期不符 |
| RK-3 | HR 同步半成功 / 外部 API 不稳定 | 2b | 高 | 中 | fake client 契约测试 + `hr_sync_runs` 对账 + 幂等 upsert；真实对接部署期另排 | 每日同步失败 |
| RK-4 | move 大子树事务膨胀（组织 path + 工单 org_path 双级联） | 2b | 低 | 中 | 内部量级评估；超阈值分批改写（先 org 后异步补工单，标记一致性 Job） | move 请求超时 |
| RK-5 | 预签名安全（object_key 伪造 / confirm 绕过 / 越权下载） | 2b | 中 | 高 | UUID key 服务端生成 + confirm HEAD 校验 + 下载预签名走工单可见性检查 | 安全自查 |
| RK-6 | 000011 迁移含数据回填，down 不可逆 | 2b | 中 | 中 | §5.3 检查单第 6 条：真实 PG 演练 up→down→up | down 演练失败 |
| RK-7 | BFS 三源角色查询每请求放大 | 2b | 中 | 中 | 单 SQL 取三源；声明量级边界（对齐 [02 §2.6](./02-authz-resource.md) 模式）；缓存后移 Phase 3 | 角色查询耗时上升 |
| RK-8 | 设备管理与 Phase 1 会话键冲突（误伤其他设备） | 2b | 低 | 中 | 单轨道约束（不触碰 `user:disabled`）写进 A 用例 | 踢单设备致其他设备 403 |
| RK-9 | Casbin LoadPolicy 规模增长 | 2a+ | 低 | 低 | 边界已声明（[02 §2.6](./02-authz-resource.md)）；Watcher 后移 Phase 3 | LoadPolicy 耗时上升 |
| RK-10 | Phase 1 回归破坏（新路由/中间件改动） | 全程 | 低 | 高 | 每里程碑含 27 用例回归段（P2-D5）；批次收口打 tag | 回归段红 |

---

## 8. SSOT 映射总表

| Step | 里程碑 | PRD（设计 SSOT） | 验收用例 | 迁移 |
|------|--------|------------------|---------|------|
| 0 | M2a-0 | [02 §1 Step 0](./02-authz-resource.md) | 装配断言 | — |
| 1 | M2a-1 | [02 全文](./02-authz-resource.md) | R1–R2 + 契约 | — |
| 2 | M2a-2 | [09 §2a/§4/§5.1](./09-ticket.md) | T1–T7、R3–R8 | 000010/000015/000016 |
| 3 | M2a-3 | README §1.1 | 全量 + 回归 | — |
| 4 | M2b-1 | [03](./03-org-enhance.md) + [hr-directory-sync](../proposal/hr-directory-sync.md) | 两表用例 | 000011 |
| 5 | M2b-2 | [09 §5.2](./09-ticket.md) | R9–R12、D11/D12 | — |
| 6 | M2b-3 | [10](./10-storage.md) | S1–S6 | 000013 |
| 7 | M2b-4 | [01](./01-auth-enhance.md) | A1–A6 | 000012（视需要） |
| 8 | M2b-5 | README §1.2 | 全量 + 回归 | — |
| 9 | M2c-1 | [04 §2–§3](./04-org-delegation.md) | D1–D6 | 000014 |
| 10 | M2c-2 | [04 §4](./04-org-delegation.md) | D7–D9 | — |
| 11 | M2c-3 | [04 §7](./04-org-delegation.md) | D1–D12 | — |
