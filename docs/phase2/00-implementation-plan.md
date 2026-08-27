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

**✅ 已采纳方案 A（2026-08-19 用户拍板）**：move 事务内级联改写。落地动作在 **Step 2**（建表同批扩展 OrgService.Move）+ **Step 4** 回归测试（move 后 scope 过滤仍正确）。

### P2-D2：2a 即建 org_id/org_path 列

按 PRD 执行：DDL 在 2a 一次到位（NOT NULL），创建工单校验 org 存在并读 path；过滤 2b 才启用。避免 2b 二次迁移。无需额外拍板，写明防歧义。

### P2-D3：HR 目录 API 开发期策略

2b HR Sync 依赖公司人员/部门 API，开发期无真实环境。**HRDirectoryClient 接口化 + fake client + fixture JSON 做契约测试**（对齐 Phase 1 stub RoleFetcher 的测试模式）；真实对接在部署期另排（contract 不变）。

### P2-D4：错误码分段写入时机

| 码段 | 写入时机 | PRD |
|------|---------|-----|
| 90001 `ErrTicketNotFound` / 90002 `ErrInvalidStateTransition`（状态机）/ 90003 `ErrTicketTypeNotFound` / 90004 `ErrTicketAlreadyClosed` | Step 2 | [09-ticket §7](./09-ticket.md) |
| 91001–91004 | Step 6 | [10-storage §6](./10-storage.md) |
| 20012（`ErrDeviceNotFound`）+ 20013（密码策略） | Step 7 | [01-auth-enhance](./01-auth-enhance.md) |
| 50008–50010 | Step 9 | [04-org-delegation §5](./04-org-delegation.md) |

均同步写入 `errcode.go` + [api/errcode.md](../api/errcode.md)，勿改号。

### P2-D5：验收脚本分段

对齐 `scripts/acceptance-phase1.sh` 模式，新增 `acceptance-phase2a.sh` / `2b` / `2c` 三段，各自独立可运行；2b/2c 脚本头部先跑上一段用例做回归（防框架级回归，Phase 1 的 27 用例作为常驻回归段并入 2a 脚本）。

### P2-D6：工单可见性设计边界（跨部门 / 分子公司 / 组织刷新）✅ 2026-08-26 用户拍板

> 背景：Phase 2 可见性引入策略 B（`entity_transparent_read`）+ `assigned` scope。以下把"跨部门、分子公司多树、组织刷新"等边界场景拍板为简化原则，防止后续实现横跳。依据：tickets 单 `org_path` 快照（`09-ticket §2a`）+ L2 `assigned`/`canOperate` + L1 实体子树透明读（`02 §2` 策略 B）。业界范式：Jira/ServiceNow「工单跟人（reporter/assignee）走 + 组织路径快照，组织重构不自动联动」。
>
> **总体取向（P2-D6 总则）**：Phase 2 目标是**把工单模块基础能力打牢**，不是完美实现所有边界场景。凡细节场景会**显著增加开发量/复杂度**，一律采用**宽松策略 + 业界通用兜底**（assigned_to 拉人、scope 主管、组织快照），不阻塞主线、不进入 2a/2b 当前范围；真有强需求再作为局部增量（future）。

| # | 原则 | 说明 | 落点 |
|---|------|------|------|
| **V0** | 宽松优先、基础为先 | 细节场景显著增开发量时，用宽松/业界兜底处理，不追求完美；主线是工单 CRUD/状态机/关联/模板等基础能力 | 全局 |
| **V1** | 2a 最简 `assigned` | Phase 2a 仅内置 `assigned`（created_by OR assigned_to 可见）为唯一 scope，先开工 | 2a Step 1–2 |
| **V2** | 2b 统一策略 B | 单实体子树内（含分子公司同集团分支）部门级全可见：`org_path <@ ANY(anchor_paths)` | 2b Step 5 |
| **V3** | 跨部门宽松三机制 | ① 同根透明读（V2）；② 把对方部门的人设为 `assigned_to`（最常用兜底，跨树也有效）；③ 对方主管 `user_orgs.ticket_scope=group/all` 且工单在其 scope 子树 | 2b |
| **V4** | per-ticket 隔离 = future | 工单限某虚拟组/子集可见（如保密工单）未建模，列为 future（不进 2a/2b） | Phase 3+ |
| **V5** | per-ticket 跨多组织 = future | 工单原生对多个组织可见（独立根分子公司跨部门）未建模，列为 future；当前用 V3② 拉处理人兜底 | Phase 3+ |
| **V6** | 类型差异化 = future | 不同工单类型走不同可见性策略（咨询 vs 故障）未建模，2b 统一策略 B | Phase 3+ |
| **V7** | 工单跟人走、组织快照化 | `created_by`/`assigned_to` 是稳定锚（跟人，组织刷新不影响）；`org_path` 是创建时快照，组织刷新不自动改写历史工单；部门透明读基于快照路径 | 全局 |
| **V8** | 组织重构不自动联动 | 合并/拆分/虚拟根调整时，已存在工单 `org_path` 不变；如需 re-parent，由显式迁移脚本处理（非触发器）；与 P2-D1 的 move 级联不同——D1 是**同一子树内 path 重写**，本原则是**跨结构重构不联动** | 全局 |
| **V9** | 模块耦合边界 | 工单**只借** `org_path` 做可见性计算（创建时快照 + 查询期 `<@` 计算），**不订阅组织事件、不反向影响组织**；组织是唯一变动方。工单与组织是弱单向耦合，实现者写 2b 策略 B / P2-D1 时有清晰红线：什么该联动（同子树 path 重写）、什么不该（跨结构重构不联动） | 全局 |

**组织建模决策点（P2-D6 附）**：分子公司须有共同总集团根（单树）才能让策略 B（V2）全覆盖跨部门；若为独立多根（无共同母公司），跨部门用 V3② 拉处理人兜底即可（宽松优先），不必为"原生跨树可见"提前建虚拟总根——真有强需求再建 `virtual-holding` 归一成单树（组织层改动、工单层零改，虚拟根需在 scope 治理）。

**不在当前范围（确认为 future，非缺陷）**：V4 / V5 / V6 三者。若真实需求出现，均为局部中等改动（加字段 + `GetFilter` OR 分支），不伤 L1/L2/L3 三层架构。

### P2-D7：Phase 2b 拆为 core / org / ext 三轨（2026-08-26，宽松优先、基础为先）

> 背景：原 2b 把"工单可见性（核心）"与"虚拟组/HR同步/附件/密码策略（增强）"塞进同一阶段，关键路径被拖长。按用户"Phase 2 主要打基础、不过度设计"取向，将 2b 拆分。

| 子阶段 | 内容 | 关键路径 | 落点 |
|--------|------|----------|------|
| **2b-core** | 工单可见性本体：策略 B（`entity_transparent_read`）+ `ticket_scope`(all/group/assigned) + `ticket_visibility` 字段 + GetFilter `<@` | ✅ 关键路径（2a 直接后继） | Step 4 |
| **2b-org** | 虚拟组 CRUD + 成员 + `org_roles` + BFS 三源角色 | 并行（依赖 2a） | Step 5 |
| **2b-ext** | storage 附件、auth-enhance、HR 目录同步、`project_isolated` 强隔离 | **延后/按需，不阻塞 2c** | Step 6/7/HR |

**决策要点**：
- 关键路径最短化：2a → 2b-core → 2c；2b-org 与 core 并行；2b-ext 延后。
- **HR 目录同步**从 2b 降为 2b-ext 延后：Phase 2 组织数据可用种子/手工维护，不阻塞主线。
- **`project_isolated` 强隔离**从 2b-core 标 future：极少见，2b-core 只交付默认 `entity_transparent_read`（CHECK 暂不含 `project_isolated`，避免 GetFilter 提前分支）。
- **`ticket:note` 2a 口径**对齐 `assigned`：创建人或处理人(assigned_to) 可见/可写，与 2a scope 一致（"处理团队成员"是 2b-org 虚拟组后的扩展）。
- 2c 前置条件改为"2b-core + 2b-org 验收通过"；Step 编号重排为 2c: Step 8–10（原 9–11）。

---

## 2. 里程碑门禁（对齐 phase1 M1–M7 模式）

> 原则：每个里程碑只列**该里程碑新增可测**用例；用例号沿用各 PRD 既有编号（R/T/D/S/A），**不重编号**（PRD 交叉引用已成型）。未到里程碑勿误判失败。

| 里程碑 | 完成 Step | 新增可验证 | 验收命令 | 说明 |
|--------|-----------|-----------|---------|------|
| **M2a-0** 接线收尾 | 0 | 无业务用例；装配断言：`grep NewRegistry wire_gen.go` 命中 2 处、启动日志无输出（空表） | Phase 1 全量测试回归 | G-1 Registry 接线 + G-2 删 authz_service.go stub（[02 §1 Step 0](./02-authz-resource.md)） |
| **M2a-1** 资源级鉴权可用 | 1 | R1–R2（Registry 单测）+ TicketResource **契约测试**（fake repo） | `go test ./internal/pkg/resource/... ./internal/service/ticket/...` | R3–R8 需工单真表，落 M2a-2 验证 |
| **M2a-2** 工单 MVP | 2 | T1–T7 + R3–R8（真表集成）+ [README §1.1](./README.md) 4 条验收 | `bash scripts/acceptance-phase2a.sh` | 迁移 000010/000015/000016；assigned 过滤；404 语义；P2-D1 级联（若采纳）；模板预填 + 关联鉴权 |
| **M2a-3** 2a 全量 | 3 | Phase 1 27 用例回归 + T/R 全量 | 同上（含回归段） | 对抗路径：不可见 404、无权限 403、状态机 90002 |
| **M2b-core** 工单可见性 | 4 | R9–R12 + D11（策略 B） | ticket 集成测试扩展 | 策略 B 透明读 + `ticket_visibility` 默认；P2-D1 回归 |
| **M2b-org** 组织增强 | 5 | [03 测试表](./03-org-enhance.md) + [hr-directory-sync §7](../proposal/hr-directory-sync.md) 用例 | `make test-integration` | 迁移 000011；虚拟组/临时成员/BFS |
| **M2b-ext** 附件 | 6 | S1–S6 | `go test` + compose MinIO 冒烟 | 迁移 000013；预签名 + confirm |
| **M2b-ext** 认证增强 | 7 | A1–A6 | `go test ./internal/service/...` | 设备列表/踢出（单轨道）+ 密码策略；可与 Step 6 并行 |
| **M2b-ext** HR 同步 | 7b | HR 对账（fake client） | `make test-integration` | HR Job；**延后，不阻塞 2c** |
| **M2b 验收** | 8 | 2b-core + 2b-org 全量 + 2a 回归 | `bash scripts/acceptance-phase2b.sh` | 工单可见性 + 虚拟组全景 |
| **M2c-1** 委托 API | 9 | D1–D6 | `make test-integration` | 迁移 000014；组内防提权（50008–50010） |
| **M2c-2** Authorize 升级 | 10 | D7–D9 | 同上（扩展） | org admin/owner + ancestor owner |
| **M2c-3** 2c 全量 | 11 | D1–D12 + 2a/2b-core/2b-org 回归（D10 HR 隔离待 2b-ext 落地后补） | `bash scripts/acceptance-phase2c.sh` | 全量收口 |

---

## 3. 任务分解（Step → 文件级）

> 设计细节（DDL/SQL/接口签名）以各 PRD 为 SSOT，此处只列执行清单与 PRD 节点。

### Step 0（M2a-0）— Phase 1 遗留收尾

- [ ] `router.Deps` 加 `Registry resource.Registry` 字段 + `router.New()` 启动清单日志（[02 §1 Step 0](./02-authz-resource.md) 四项）
- [ ] 删 `internal/service/authz_service.go` + `wire.go` provider 调整 + `make wire`
- [ ] `router_test.go` 手工构造补 `Registry: resource.NewRegistry()`
- [ ] 验证：grep 双 `NewRegistry`；`go build` + Phase 1 单测/集成全量回归
- [ ] swag 集成（[modules/menu.md §5](../modules/menu.md) swagger ⏳ Phase 2 遗留）：`make swag` 目标 + handler 加 `@Summary/@Router/@Tags` 注解，生成 `docs/swagger.json`（API 文档自动同步）

### Step 1（M2a-1）— authz-resource

- [ ] `registry_test.go` 补 Authorize/GetFilter 单测（R1/R2）
- [ ] `internal/service/ticket/scope_resolver.go（2a：HasRole 辅助函数；assigned 语义已直接实现在 resource.go 的 canRead/GetFilter 中）`：assigned 语义（ReadAnchorPaths 空 + `created_by OR assigned_to` 谓词）
- [ ] `internal/service/ticket/resource.go`：TicketResource 骨架 + 契约测试（fake repo，不依赖真表）
- [ ] wire：NewTicketService 接收 Registry 自注册（依赖 Step 0 接线；拓扑序保证注册先于 router.New）

### Step 2（M2a-2）— ticket MVP

- [x] 迁移 **000010**：`ticket_types` / `ticket_type_fields` / `tickets` / `ticket_comments` / `ticket_events` + 工单管理菜单（catalog/page/button 三层）+ menu_apis + 角色绑定（D2 已并入 000010，不再单独 _menu 文件）
  - **硬删例外**：`tickets` / `ticket_comments` / `ticket_events` 表无 `deleted_at` 列，走物理 DELETE + ON DELETE CASCADE（P2-D1 设计；审计链由 `ticket_events` 外置保留）；`ticket_templates` / `ticket_relations` 走软删 + 部分唯一索引
  - `ticket_events` 2a 建表仅审计用，L1 机制 Phase 3 启动时迁移 000021 补列
- [x] 迁移 **000015**：`ticket_templates`（模板表，2a 前移，DDL 见 [09 §2](./09-ticket.md#工单模板2a-前移迁移-000015)）
- [x] 迁移 **000016**：`ticket_relations`（关联表，2a 前移，DDL 见 [09 §2](./09-ticket.md#工单关联2a-前移迁移-000016)）
- [x] 90001/90002 写入 `errcode.go` + `errcode.md`（P2-D4）
- [x] ~~迁移 **000010_menu**（或并入 000010）~~：已并入 000010（ticket_types 建表后追加菜单 INSERT）
- [x] TicketService/Handler/Router：CRUD + 状态机（transitions JSONB 校验）+ ticket_events；创建时同事务读 org.path 写 org_path
- [x] 工单模板：列表/详情 API + `POST /tickets` 支持可选 `template_code` 预填字段（`default_sla_minutes` 仅存储，Phase 3 启用）
- [x] 工单关联：建立/查询关联 API；建立关联时对 source/target 均走 update 鉴权（严于 PRD target-only，防止越权关联他人工单）
- [ ] **P2-D1（已采纳 A）**：`OrgService.Move` 扩展级联改写 `tickets.org_path` + 集成测试
- [ ] `scripts/acceptance-phase2a.sh`（含 Phase 1 27 用例回归段）
- [ ] R3–R8 真表集成测试落位（[02 §3 R 表](./02-authz-resource.md)）

### Step 3（M2a-3）— 2a 集成验收

- [ ] 全量脚本通过；PRD 用例表标注状态；12 号报告模式出 2a 验收记录（可选）

### Step 4（M2b-core）— ticket scope 升级（2b 关键路径）

- [ ] `ReadAnchorPaths`（挂载实体 anchor 透明读）+ GetFilter 升级 `org_path <@ ANY($2::ltree[])`（[09 §5.2](./09-ticket.md)）
- [ ] `ticket_visibility` 字段默认 `entity_transparent_read`（**不含 `project_isolated`，标 future**，[09 §5.2.1](./09-ticket.md)）
- [ ] R9–R12 + D11 集成测试；**P2-D1 回归**（move 后 scope 过滤仍正确）

### Step 5（M2b-org）— org-enhance（与 Step 4 并行）

- [ ] 迁移 **000011**：`source`/`external_id`/`synced_at`、`user_orgs.ticket_scope`/`is_primary`/`source`/`expires_at`、`ticket_visibility`（[hr-directory-sync §2](../proposal/hr-directory-sync.md) DDL）
- [ ] 虚拟组 CRUD（org_type=4、code 前缀 `vg_`）+ Reparent（HR 撤销部门上挂最近实体祖先）
- [ ] 临时成员：`expires_at` 读取时过滤（或惰性清理 Job，随 PRD）
- [ ] BFS 三源 RoleFetcher 扩展（直接 + 组织角色 + 继承）
- [ ] 实体 move 子树 path 级联**含虚拟组**（Phase 1 Move 扩展）

### Step 6（M2b-ext）— storage（延后/按需）

- [ ] compose 加 MinIO；`config.storage` 段（[10 §2](./10-storage.md)）
- [ ] 迁移 **000013**：`file_objects` / `ticket_attachments`
- [ ] `internal/pkg/storage/s3_client.go` + 预签名 upload/download + confirm（HEAD 校验）+ 附件列表/删除 API
- [ ] 91001–91004 错误码；S1–S6 测试

### Step 7（M2b-ext）— auth-enhance（可与 Step 6 并行，延后/按需）

- [ ] **首任务（D2-49②）**：devices 集合初始化（SADD/SREM 接入登录/登出/吊销链路）+ RT value 结构升级（hash 与设备元数据并存，Refresh 比较逻辑与守护测试同 Step 改造——[01 §2.1](./01-auth-enhance.md)）
- [ ] 设备列表/踢出 API（沿用 `devices:{uid}` 集合；**单轨道**：仅删单设备 RT，不触碰 `user:disabled`，[01 §0 B3](./01-auth-enhance.md)）
- [ ] `ValidatePasswordPolicy` + 20013（策略归一：binding 保留 required，长度/复杂度统一走策略校验，[01 §3.4](./01-auth-enhance.md)）
- [ ] 迁移 **000012** 视需要（纯 config 则无迁移，编号顺延规则见 [README §2.4](./README.md)）
- [ ] A1–A6 测试（miniredis）

### Step 7b（M2b-ext）— HR 目录同步（延后，独立 Job）

- [ ] `HRDirectoryClient` 接口 + `HRSyncService`（fake client 契约测试，P2-D3）+ `hr_sync_runs` 对账表 + 分布式锁 Cron（[03-org-enhance](./03-org-enhance.md)）
- [ ] Phase 2 组织数据可种子/手工维护，HR 同步不阻塞主线

### Step 8（M2b-core 集成验收）— 2b-core + 2b-org 验收

- [ ] `acceptance-phase2b.sh`（头段跑 2a 回归）；2b-core 可见性验收 + 2b-org 虚拟组验收全过

### Step 9（M2c-1）— org-delegation

- [ ] 迁移 **000014**：`organizations.owner_user_ids` / `user_orgs.org_member_role`（[04 §2.1](./04-org-delegation.md)）
- [ ] `OrgDelegationService`：EffectiveOrgPriority / IsOrgAdminOrOwner / IsAncestorOwner
- [ ] SetOwners / SetMemberRole / AddMember / RemoveMember / 虚拟组删除扩展（防提权矩阵 [04 §3](./04-org-delegation.md)）
- [ ] 50008–50010 错误码；D1–D6 集成测试

### Step 10（M2c-2）— ticket Authorize 升级

- [ ] `canOperate` 扩展（org admin/owner + ancestor owner，[04 §4](./04-org-delegation.md)）；D7–D9 集成测试

### Step 11（M2c-3）— 2c 集成验收

- [ ] `acceptance-phase2c.sh`：D1–D12 全量 + 2a/2b-core/2b-org 回归

---

## 4. 批次与并行（单人 + AI 协作模式）

> Phase 1 即单人开发模式，Phase 2 沿用。并行仅指「接口契约先定、实现交替推进」。

```
批次 α（关键路径）             批次 β（组织增强，与α并行）    批次 γ（外延，延后/按需）      批次 δ（委托收口）
Step 0→1→2→3 → Step 4   →    Step 5                 →    Step 6 ∥ Step 7 ∥ HR →   Step 8 → 9 → 10
   (2a)          (2b-core)       (2b-org)                  (2b-ext)                  (2c)
```

- **批次 α 关键路径**：2a（Step 0–3）→ 2b-core（Step 4，工单可见性本体）后即可形成"能跑的工单模块"，是 Phase 2 最短交付链。
- **批次 β（2b-org）**：虚拟组/scope/角色，与 2b-core 可并行；2c 委托需 α+β 都就绪。
- **批次 γ（2b-ext）**：storage(6)/auth-enhance(7)/HR 同步 **延后，不阻塞 2c**；按宽松优先原则排期。
- **每批次收口**：合入 dev + 打 tag（如 `phase2a`/`phase2b-core`）+ 全量回归，再进下一批次。

---

## 5. 工程流程规范

### 5.1 分支策略（对齐 [phase1/README §2](../phase1/README.md)）

短 feature 分支从 `dev` 切出，PR base = `dev`，合入后删除：

| 分支 | 覆盖 |
|------|------|
| `feature/step-2a-authz-resource` | Step 0 + 1 |
| `feature/step-2a-ticket` | Step 2 + 3 |
| `feature/step-2b-core-ticket-scope` | Step 4（2b-core：工单可见性本体） |
| `feature/step-2b-org-enhance` | Step 5（2b-org：虚拟组/scope/角色） |
| `feature/step-2b-storage` / `feature/step-2b-auth-enhance` / `feature/step-2b-hr-sync` | Step 6 / 7 / HR（2b-ext，延后） |
| `feature/step-2c-org-delegation` | Step 8 + 9 + 10 |

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

### 5.5 本地验收纪律（个人项目无 CI 平台，不强行套 CI 门禁）

- 开工前、以及每次改完 2a 相关代码后，手动跑 `make acceptance`（=scripts/acceptance-phase1.sh，27 用例回归）+ `make test -race` 全绿，再继续；MR 合入前自觉跑。
- 目的：锁定 Phase 1 行为不被 Phase 2 改动悄悄破坏（替代企业级 CI 强制门禁，成本极低、无需任何平台）。
- Phase 1 验收门禁完整证据链见 [README §1.6](./README.md)；Phase 2 各里程碑验收脚本分段（P2-D5）头段即跑上一段回归。

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
| M2b-core | 3 | 策略 B 语义 + 回归 |
| M2b-org | 4–5 | 虚拟组/BFS/迁移（HR 已延后） |
| M2b-ext（storage+auth） | 5 | MinIO + 设备 API + 密码策略 |
| M2b-ext（HR 同步） | 3–4 | **延后，不计入关键路径** |
| M2b 验收 | 1 | 验收脚本 |
| **2b 关键路径小计** | **~13** | 不含延后 HR 同步 |
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
| RK-11 | `update` 权限 2a→2b 收窄：2a 处理人（`assigned_to`）可 update，2b 收为仅创建人可 update（[02 §2.3](./02-authz-resource.md#23-ticketresource) canOperate 2a/2b 对照；[09 §5.1/§5.2](./09-ticket.md)） | 2b | 中 | 中 | 2b 升级时显式回归「处理人 update 应 403」；处理人需改单走 `assign` 重分派或 scope 主管权限，不回退 canOperate | 处理人 2a 能 update、2b 报 403 |
| RK-12 | 组织重构后「同部门透明读」基于快照 `org_path`，新部门同事看不到旧工单（V7/V8 预期行为，但易被误判为 bug） | 2b | 中 | 低 | P2-D6 已声明快照语义；文档/验收明确「跨部门靠 assigned_to（V3②）而非靠部门透明读」；必要时提供 org_path re-parent 迁移脚本 | 用户反馈"部门合并后旧工单同事看不到了" |

---

## 8. SSOT 映射总表

| Step | 里程碑 | PRD（设计 SSOT） | 验收用例 | 迁移 |
|------|--------|------------------|---------|------|
| 0 | M2a-0 | [02 §1 Step 0](./02-authz-resource.md) | 装配断言 | — |
| 1 | M2a-1 | [02 全文](./02-authz-resource.md) | R1–R2 + 契约 | — |
| 2 | M2a-2 | [09 §2a/§4/§5.1](./09-ticket.md) | T1–T7、R3–R8 | 000010/000015/000016 |
| 3 | M2a-3 | README §1.1 | 全量 + 回归 | — |
| 4 | M2b-core | [09 §5.2](./09-ticket.md) | R9–R12、D11 | — |
| 5 | M2b-org | [03](./03-org-enhance.md) | 两表用例 | 000011 |
| 6 | M2b-ext | [10](./10-storage.md) | S1–S6 | 000013 |
| 7 | M2b-ext | [01](./01-auth-enhance.md) | A1–A6 | 000012（视需要） |
| 7b | M2b-ext | [03-org-enhance HR 同步](./03-org-enhance.md) | HR 对账 | — |
| 8 | M2b 验收 | README §1.2 | 2b-core + 2b-org 全量 + 回归 | — |
| 9 | M2c-1 | [04 §2–§3](./04-org-delegation.md) | D1–D6 | 000014 |
| 10 | M2c-2 | [04 §4](./04-org-delegation.md) | D7–D9 | — |
| 11 | M2c-3 | [04 §7](./04-org-delegation.md) | D1–D12 | — |

---

## 9. 变更记录

| 日期 | 说明 |
|------|------|
| 2026-08-19 | 初版：P2-D1~D5 编码前拍板 |
| 2026-08-26 | P2-D6：工单可见性设计边界（V1~V8 + V0 宽松优先总则 + V9 模块耦合边界：2a 最简 assigned / 2b 策略B / 跨部门宽松三机制 / per-ticket 隔离·跨多组织·类型差异为 future / 跟人走+组织快照 / 组织重构不联动 / 工单只借 org_path 不订阅组织事件）+ 组织建模决策点（独立多根用 assigned_to 兜底，不强建虚拟总根）+ RK-12 |
| 2026-08-26 | P2-D7：Phase 2b 拆 core/org/ext 三轨（关键路径 2a→2b-core→2c；2b-org 并行；2b-ext 延后）；HR 同步降 2b-ext 延后、project_isolated 标 future（2b-core 仅交付 entity_transparent_read）、ticket:note 2a 口径对齐 assigned；Step 重排 2c:8–10；批次/分支策略同步更新 |
