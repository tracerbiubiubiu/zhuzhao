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
| 50008–50010 | Step 8 | [04-org-delegation §5](./04-org-delegation.md) |

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
| **M2b-core** 工单可见性 | 4 | R9–R12 + D11（策略 B） | ticket 集成测试扩展 | 迁移 000011；策略 B 透明读 + `ticket_visibility` 默认；RK-11 收窄；BK-1 备注；P2-D1 回归 |
| **M2b-org** 组织增强 | 5 | [03 测试表](./03-org-enhance.md) + [hr-directory-sync §7](../proposal/hr-directory-sync.md) 用例 | `make test-integration` | 迁移 000012；虚拟组/临时成员/BFS |
| **M2b-ext** 附件 | 6 | S1–S6 | `go test` + compose MinIO 冒烟 | 迁移 000013；预签名 + confirm |
| **M2b-ext** 认证增强 | 7 | A1–A6 | `go test ./internal/service/...` | 设备列表/踢出（单轨道）+ 密码策略；可与 Step 6 并行 |
| **M2b-ext** HR 同步 | 7b | HR 对账（fake client） | `make test-integration` | HR Job；**延后，不阻塞 2c** |
| **M2b 验收**（非编号 Step，收口动作） | 4–7 | 2b-core + 2b-org 全量 + 2a 回归 | `bash scripts/acceptance-phase2b.sh` | 工单可见性 + 虚拟组全景 |
| **M2c-1** 委托 API | 8 | D1–D6 | `make test-integration` | 迁移 000013（已随 Step 8 落地）；组内防提权（50008–50010） |
| **M2c-2** Authorize 升级 | 9 | D7–D9 | 同上（扩展） | org admin/owner + ancestor owner |
| **M2c-3** 2c 全量 | 10 | D1–D12 + 2a/2b-core/2b-org 回归（D10 HR 隔离待 2b-ext 落地后补） | `bash scripts/acceptance-phase2c.sh` | 全量收口 |

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
  - **硬删例外**：`tickets` / `ticket_comments` 表无 `deleted_at` 列，走物理 DELETE + ON DELETE CASCADE（`ticket_comments` 用户内容随单销毁）；`ticket_events` **已去 CASCADE**（迁移 000014，2026-08-28 HC2：事件行随库存活，ticket_id 悬空 = 审计语义，删单不再摧毁业务时间线）；`ticket_templates` / `ticket_relations` 走软删 + 部分唯一索引
  - `ticket_events` 2a 建表仅审计用，L1 机制 Phase 3 启动时迁移 000021 补列
- [x] 迁移 **000015**：`ticket_templates`（模板表，2a 前移，DDL 见 [09 §2](./09-ticket.md#工单模板2a-前移迁移-000015)）
- [x] 迁移 **000016**：`ticket_relations`（关联表，2a 前移，DDL 见 [09 §2](./09-ticket.md#工单关联2a-前移迁移-000016)）
- [x] 90001/90002 写入 `errcode.go` + `errcode.md`（P2-D4）
- [x] ~~迁移 **000010_menu**（或并入 000010）~~：已并入 000010（ticket_types 建表后追加菜单 INSERT）
- [x] TicketService/Handler/Router：CRUD + 状态机（transitions JSONB 校验）+ ticket_events；创建时同事务读 org.path 写 org_path
- [x] 工单模板：列表/详情 API + `POST /tickets` 支持可选 `template_code` 预填字段（`default_sla_minutes` 仅存储，Phase 3 启用）
- [x] 工单关联：建立/查询关联 API；建立关联时对 source/target 均走 update 鉴权（严于 PRD target-only，防止越权关联他人工单）
- [x] **P2-D1（已采纳 A）**：`OrgRepo.Move` 事务内级联改写 `tickets.org_path`（含虚拟组）+ 集成测试（`TestB2_MoveCascadeRemapsDescendantTicketPath`，BK-6）
- [x] `scripts/acceptance-phase2a.sh`（含 Phase 1 27 用例回归段；155 断言全绿）
- [x] R3–R8 真表集成测试落位（[02 §3 R 表](./02-authz-resource.md)；`authz_resource_integration_test.go`）

### Step 3（M2a-3）— 2a 集成验收

- [x] 全量脚本通过（155 断言）；PRD 用例表已标注状态

### Step 4（M2b-core）— ticket scope 升级（2b 关键路径）

- [x] `ReadAnchorPaths`（挂载实体 anchor 透明读）+ GetFilter 升级 `org_path <@ ANY($2::ltree[])`（[09 §5.2](./09-ticket.md)）——2026-08-28 落地
- [x] 迁移 **000011**：`organizations.ticket_visibility` 默认 `entity_transparent_read`（**不含 `project_isolated`，标 future**，[09 §5.2.1](./09-ticket.md)；原规划 Step 5，按 PRD SSOT 前移至 Step 4，其余 DDL 顺延 000012）
- [x] BK-1 内部备注过滤（透明读旁观者仅公开回复，读写集合一致）
- [x] RK-11 收窄回归（处理人 update → 403）
- [x] R9–R12 + D11 集成测试（✅ 机制形态：同事透明读/跨子树隔离/读写分离/备注可见性/ticket_visibility 列；R9/R10 虚拟组完整形态待 Step 5）
- [x] **P2-D1 回归**：`TestB2_MoveCascadeRemapsDescendantTicketPath`（move 后后代工单 org_path 按 newRoot||subpath 重映射 + 透明读在新路径继续生效、跨子树仍隔离）——**BK-6 关闭**（2026-08-28）

### Step 5（M2b-org）— org-enhance（与 Step 4 并行）

- [x] 迁移 **000012**：organizations/users `source`/`external_id`/`synced_at`、`user_orgs.ticket_scope`/`source`/`expires_at`、`org_roles` 表、`roles.parent_id`（2026-08-28 落地；`ticket_visibility` 已前移至 Step 4/000011）
- [x] 虚拟组 CRUD 约束（org_type=4、code 前缀 `vg_`、父级必须实体）；Reparent 自动化属 HR Sync（2b-ext），手工移动经既有 Move API 天然支持（级联已含虚拟组）
- [x] 临时成员：`expires_at` 读取侧过滤（resolver 锚点 + BFS 组织角色，无清理 Job）
- [x] BFS 三源 RoleFetcher：`RoleRepo.GetEffectiveRoleCodes`（直接 ∪ org_roles 直接所属节点 ∪ parent_id 继承链），RBACService 已切换
- [x] 实体 move 子树 path 级联**含虚拟组**（`TestB2Org_VgCreateAndMoveCascade`；Phase 1 Move 基于organizations 全行级联，虚拟组天然覆盖并有测试固化）
- [x] scope 激活：group 主管分派（`TestB2Org_ScopeSupervisorAndAll`）、all 全量、R9/R10 完整形态（`TestB2Org_R9R10_VgSiblingReadWrite`）

### Step 6（M2b-ext）— storage（延后/按需）

- [ ] compose 加 MinIO；`config.storage` 段（[10 §2](./10-storage.md)）
- [ ] 迁移**顺延为 000017**：`file_objects` / `ticket_attachments`（000013=2c 委托已执行、000014=审计修复已执行、000015/000016=2a 模板/关联已执行；README §2.4）
- [ ] `internal/pkg/storage/s3_client.go` + 预签名 upload/download + confirm（HEAD 校验）+ 附件列表/删除 API
- [ ] 91001–91004 错误码；S1–S6 测试

### Step 7（M2b-ext）— auth-enhance（可与 Step 6 并行，延后/按需）

- [ ] **首任务（D2-49②）**：devices 集合初始化（SADD/SREM 接入登录/登出/吊销链路）+ RT value 结构升级（hash 与设备元数据并存，Refresh 比较逻辑与守护测试同 Step 改造——[01 §2.1](./01-auth-enhance.md)）
- [ ] 设备列表/踢出 API（沿用 `devices:{uid}` 集合；**单轨道**：仅删单设备 RT，不触碰 `user:disabled`，[01 §0 B3](./01-auth-enhance.md)）
- [ ] `ValidatePasswordPolicy` + 20013（策略归一：binding 保留 required，长度/复杂度统一走策略校验，[01 §3.4](./01-auth-enhance.md)）
- [ ] 迁移**视需要**（纯 config 则无迁移；若需表结构取执行期下一可用编号，见 [README §2.4](./README.md) 注）
- [ ] A1–A6 测试（miniredis）

### Step 7b（M2b-ext）— HR 目录同步（延后，独立 Job）

- [ ] `HRDirectoryClient` 接口 + `HRSyncService`（fake client 契约测试，P2-D3）+ `hr_sync_runs` 对账表 + 分布式锁 Cron（[03-org-enhance](./03-org-enhance.md)）
- [ ] Phase 2 组织数据可种子/手工维护，HR 同步不阻塞主线

### 2b 收口验收（M2b，非编号 Step）— 2b-core + 2b-org 验收

- [ ] `acceptance-phase2b.sh`（头段跑 2a 回归）；2b-core 可见性验收 + 2b-org 虚拟组验收全过

### Step 8（M2c-1）— org-delegation

- [x] 迁移 **000013**：`organizations.owner_user_ids` / `user_orgs.org_member_role` + 委托路由 menu_apis（2026-08-28 落地）
- [x] `OrgDelegationService`：EffectiveOrgPriority / IsOrgAdminOrOwner / IsAncestorOwner / HasOrgManagePermission / ensureCanManageMember 防提权矩阵
- [x] SetOwners（双轨对齐+移出降级）/ SetMemberRole / AddMember·RemoveMember 委托扩展 / DeleteOrgDelegated（owner 派生行不占位语义）
- [x] 50008–50010 错误码；D1–D6 集成测试全绿（org_delegation_integration_test.go）

### Step 9（M2c-2）— ticket Authorize 升级

- [x] `canOperate` 委托扩展（note/update/close/assign/delete + org admin·owner / ancestor owner；跨 vg 不越容器）；D7–D9 + 边界集成测试全绿

### Step 10（M2c-3）— 2c 集成验收

- [x] `acceptance-phase2c.sh`：D1–D9 + D11 HTTP 全量（D10 为列存在静态断言，HR 动态回归待 2b-ext）+ 链式回归；211 断言全绿

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
| RK-6 | 000012 迁移含数据回填，down 不可逆 | 2b | 中 | 中 | §5.3 检查单第 6 条：真实 PG 演练 up→down→up | down 演练失败 |
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
| 4 | M2b-core | [09 §5.2](./09-ticket.md) | R9–R12、D11 | 000011 |
| 5 | M2b-org | [03](./03-org-enhance.md) | 两表用例 | 000012 |
| 6 | M2b-ext | [10](./10-storage.md) | S1–S6 | 000014（延后顺延） |
| 7 | M2b-ext | [01](./01-auth-enhance.md) | A1–A6 | 视需要（执行期取下一可用编号） |
| 7b | M2b-ext | [03-org-enhance HR 同步](./03-org-enhance.md) | HR 对账 | — |
| — | M2b 验收 | README §1.2 | 2b-core + 2b-org 全量 + 回归 | — |
| 8 | M2c-1 | [04 §2–§3](./04-org-delegation.md) | D1–D6 | 000013 |
| 9 | M2c-2 | [04 §4](./04-org-delegation.md) | D7–D9 | — |
| 10 | M2c-3 | [04 §7](./04-org-delegation.md) | D1–D12 | — |

---

## 9. 2a 收口遗留清单（backlog）

> 2026-08-27 Step 0–3 全量审查（P0×1 / P1×6 / P2 批量）后登记：P0 与 P1 已全部修复（迁移合并、authorize 404/403/500 三路语义、swagger 重生成、gofmt 门禁、模板预填、软删例外回标）；以下为**有意延后**的 P2 级事项，按消费时点归位，防止只存在于对话记忆。修复时同 PR 回标本表。

| # | 事项 | 说明 | 消费时点 |
|---|------|------|---------|
| BK-1 | ~~ListComments 不过滤 `is_internal`~~ | **✅ 已落地（Step 4，2026-08-28）**：透明读旁观者仅公开回复；读写集合一致（写者 ⊆ 可见内部备注集合）；测试 `TestB2_InternalNoteVisibility` | 已关闭 |
| BK-2 | ~~Assign 状态转换绕过状态机~~ | **✅ 已关闭（2026-08-28）**：Assign 走 `FromTicketType + AssertTransition`；回归 `TestB2_AssignStateMachineAndUpdateGuard`（自定义类型双向验证） | 已关闭 |
| BK-3 | ~~Update patch 不写 ticket_events；closed 检查读后写（TOCTOU）~~ | **✅ 已关闭（2026-08-28）**：Update 改事务内 `UpdateTx`（`WHERE status<>'closed'` 条件更新）+ `CreateEventTx(action=updated)`；close 后 update → 90004（90004 复活） | 已关闭 |
| BK-4 | ~~`ticket_events.action` 与 status 字面量未常量化~~ | **✅ 已关闭（Step 4，2026-08-28；2026-08-31 审计复核确认，行漏关）**：state_machine.go 常量块（StatusOpen/StatusClosed/EventCreated/EventUpdated/EventStatusChanged/EventAssigned）落地，service.go 字面量清零 | 已关闭 |
| BK-5 | ~~CreateRelation 反向判重缺失~~ | **✅ 已实施（A5，2026-08-31）**：`ExistsRelationBetween` 双向 EXISTS 前置拦截（同 type 下 A→B/B→A 视为同一关联）→ 409；TestBK5_RelationReverseDedup 回归 | 已关闭 |
| BK-6 | ~~P2-D1 级联后代分支无 Go 测试~~ | **✅ 已关闭（Step 4，2026-08-28）**：`TestB2_MoveCascadeRemapsDescendantTicketPath` 覆盖孙代组织 + 工单 subpath 重映射 + move 后透明读仍正确 | 已关闭 |
| BK-7 | ~~handler List 非法 priority 静默忽略；page/page_size 回显未归一~~ | **✅ 已关闭（2026-08-31 代码级复核）**：handler 已实现 priority 非法→400（ticket_handler.go:49-52）、page/page_size 在回显前钳制（:43-47），两半项均落地，登记行过时 | 已关闭 |
| BK-8 | ~~scope_resolver.go 文件名与内容不符~~ | **✅ 已解决（Step 4，2026-08-28）**：真 ScopeResolver（ReadAnchorPaths 策略 B）落地于该文件，文件名恢复名副其实 | 已关闭 |
| BK-9 | setupTicket2a 死代码回退分支 | `if orgID == 0` 不可达（前置 require.NoError 已拦），且其 ON CONFLICT DO NOTHING RETURNING 在冲突时返回空行会 Scan 报错 | 顺手 |
| BK-10 | ~~2a 无状态推进端点~~ | **✅ 已拍板（2026-08-28 用户确认）**：2b/2c 不补「开始处理/待验证」推进端点（无真实诉求）；in_progress/pending_verify 状态与推进 API 归 Phase 3 工单业务深化（10-ticket-business）一并设计。当前行为（assigned 工单先取消分派回 open 再 close，T6 固化）为预期语义 | 已关闭 |
| **BK-11** | org_path 快照竞态（Create×Move 并发 → 旧 path 快照残留，实测复现） | Create 在事务外 FindByID 读 org.path、事务内写入——与 Move 并发时写入过期快照（P2-D1 级联追不上窗口内新建行）。**完整影响面**：① scope 视角（含 vg admin/owner）对该单 Get/List 暂 404/不可见——**本级 admin 也救不了，仅全局 admin bypass 可救**；② ancestor owner（$3 防御）失效；③ 触发域：单实例 gin 并发请求窗口即可命中（非仅多实例）。**无越权无损坏（fail-safe）**。**决策（两步）**：① 立即实施 Create 事务内 `FOR SHARE` 锁 org 行后读 path（~15 行，确定性消除单条竞态；将来切运行时 JOIN 方案时随列消亡，无浪费）；② 「快照 vs 运行时 JOIN」数据结构决策登记 phase3/README 待决策表，Phase 3 启动时拍板 | **① ✅ 已实施（2026-08-31）**：`OrgRepo.FindByIDForShareTx`（事务内 FOR SHARE）+ `ticket.Service.Create` 重构（org 读取移入事务）；回归 `TestBK11_CreateBlocksBehindConcurrentMove`（写锁窗口阻塞验证）+ `TestBK11_CreateVsMoveHammer`（真实 Move×Create 交叉锤击），并经变异验证（去掉 FOR SHARE 测试必失败）。**② ✅ 已拍板（2026-08-31）：保留镜像列**（FOR SHARE 兜底；去列 JOIN 备选弃用；`created_org_id` 留 Step 7e 报表设计按需评估） |
| **BK-12** | `org_roles` / `roles.parent_id` 写侧管理接口缺失（BFS 源 2/3 只有读侧） | 000012 建表后仅 BFS 三源展开消费（role_repo.go）；handler/router 零端点，验收脚本用 psql 直插搭场景（acceptance-phase2b.sh:177）；`parent_id` 管理入口按 000012 注释承诺「随 2c 角色管理补齐」未兑现。**必要性评估（2026-08-31）**：机制符合业界主流实践（Entra ID / Google Groups / Keycloak / AWS Identity Center 组驱动赋角）；当前 fail-inert（无人能写即无行为），价值随成员高频变动兑现。**非纯 CRUD**：org_roles 绑出的是全局 Casbin 角色（进 L1），防提权须按「发全局角色」设计（复用 user_roles assign 优先级规则 + 全局管理员限定） | 触发条件驱动：① 真实「进组赋角色」诉求；② **2b-ext HR 同步启动 = 硬触发器**（届时逐人赋角不可运营，必补）；实施时 org_roles + parent_id 写侧**一次补齐** |
| **BK-13** | 工单可见性默认方向与业务形态不匹配：兄弟虚拟组透明可读（策略 B 默认），「默认只看自己组 + 实体级可配置」（`project_isolated`）未交付 | **触发场景（2026-08-31 用户确认，即文档预告的「真实需求」）**：实体 a（领导 aa）+ 虚拟组 b/c/d（各组多领导）；aa 看全部、bb/cc/dd 默认只看自己组、可配置放宽。**现状对照**：aa 全子树可见/管理（owner 锚点 + ancestor owner D9）✅；bb 管本组（D7）✅；个人级放宽（跨部门成员 + expires_at）✅；唯「默认收紧」相反——透明读下 bb 可读 c/d（D11），暴露面随 vg 规模线性增长（工单含客户/故障信息时为横向泄露面）。**业界**：多团队/多项目服务台主流 = 默认隔离 + 显式打通（ServiceNow Domain / Zendesk org restriction / Freshdesk group / GitHub·GitLab private default）。**机制骨架已埋**：scope_resolver.go:98 锚点门控只认透明读值（配隔离值自动收锚点，核心读路径零改动）、`TestB2_TicketVisibilityColumn` 已验收收紧语义、D12 用例已设计 | **① ✅ 已实施（IW1）**：迁移 000017 CHECK 放开 + org update 透出 ticket_visibility（虚拟组 400 守卫）+ **L2 委托轴**（Authorize/GetFilter 委托 OR 分支，强隔离下 D7–D9 保活）+ D12 测试（TestD12_ProjectIsolated / DelegationAxisStaysBounded）；**② 已随 A3 拍板搁置**（保留镜像列）：① CHECK 约束放开 `project_isolated`（迁移号按实施时点顺延，先启动先占用）；② org update API 透出 `ticket_visibility` 配置（现仅 DBA SQL）；③ D12 集成测试激活（兄弟 404 + 开关切换）；④ 粒度组合语义文档化（实体级开关 × 个人级覆盖）；可顺带「至少一个 owner」校验。默认值不动、不配不收紧、HR 同步不触碰 |
| **BK-14** | 成员 scope（`user_orgs.ticket_scope`）无配置面：读侧在用（resolver 三档 all/group/assigned + max 合并），写侧零端点 | 第三个「读侧在用、写侧缺失」缺口（与 BK-12 org_roles、BK-13 ticket_visibility 同构）：AddMember 仅收 `org_member_role`，全仓无 API 写 `ticket_scope`。默认已是 `assigned`（只看分派给自己的，符合最小可见预期），**缺的是配置入口**。**设计拍板（2026-08-31，关系派）**：scope 挂**成员关系**而非角色——一人可隶属多组织且各组织范围不同（角色全局 × scope 局部语义冲突）；业界对照：Freshdesk 组/GitLab 成员级别（关系派）vs Zendesk/若依 data_scope（角色派），多组织隶属模型选关系派，与现有 resolver 实现一致。**实施**：① AddMember 请求扩展 `ticket_scope`（`oneof=assigned/group/all`，缺省 assigned）；② 成员 scope 变更端点（`POST /orgs/members/scope`），防提权复用 `ensureCanManageMember`，L1 挂 `org:update` 系（2c §3.1.1 策略）；③ **scope=all 赋权边界**：all = AllScope 旁路整个 L2 = 全局可见，**仅全局管理员可授**（org owner/admin 限 assigned/group），写审计日志；④ 与 BK-13 同批实施（11 §8 W1 场景闭环：bb「看自己组全部」依赖 BK-14 配 scope=group）。**不做**：角色级 data_scope（语义冲突）、org 级默认 scope（YAGNI） | **✅ 已实施（IW1）**：AddMember 扩展 `ticket_scope`（缺省 assigned、重复添加不重置）+ `POST /orgs/members/scope`（org admin/owner，scope=all 仅全局管理员 + slog 审计）；测试 TestBK14_SetMemberScope / AddMemberWithScope |
| **BK-15** | 代码卫生：`UpdateAssignedTo` 死包装（`currentStatus` 参数零使用、函数全仓零调用方）+ Delete / UpdateAssignedToTx 新旧注释叠行 | CC2/HC2 修复时的叠注遗留（外部 review 核验发现，2026-08-31）。旧 Delete 注释两处过时：「admin only」——2c 后 org owner/admin、ancestor owner 亦可删；「关联表由 DB CASCADE 处理」仅对 comments/relations 属实，ticket_events 为 SET NULL（000014） | **✅ 已修复（2026-08-31）**：删除死包装；两处注释合并为单条并更正删单权限与 FK 行为描述 |
| **BK-16** | ~~`TicketRepo.Delete` 不校验 RowsAffected~~ | **✅ 复核关闭（2026-08-31，误报）**：校验自 Phase 2a（66e2c39）即存在——`DELETE` 0 行 → `ErrTicketNotFound` → defer 回滚事件插入，重复/并发删除返回 404 且无重复事件。误报根因：并发审计时读码截断于 L215（校验在其后 5 行），未读全函数。可选防回归：双删 404 断言（随手项） | 已关闭（误报） |
| **BK-17** | 角色展开每请求双查：CasbinAuth 中间件（casbin.go:46）与 service 层（ticket service.go:52、org_service.go:37）各跑一次 BFS WITH RECURSIVE；中间件放入 gin ctx 的 roles 无任何 service 消费方 | 2026-08-31 外评核实属实；非正确性问题，纯性能优化（热路径列表每次白付一次 BFS）；受影响面 = 全部工单 API + 组织委托 API | **✅ 已实施（IW1）**：CasbinAuth 将 roles 挂 request context（`middleware.RolesFromContext`），`RBACService.GetRoleCodesByUserID` 按 userID 命中即返回；TestRolesFromContext 单测：方案 A——中间件 fetch 后把 roles 同时挂到 `c.Request` 的 request context，`RBACService.GetRoleCodesByUserID` 先查 ctx（按 userID 匹配）命中即返回；签名零改动、集成测试零改动。**不做**：签名透传（侵入大）、Redis/TTL 缓存（权限撤销延迟生效） |
| **BK-18** | 工单类型/字段/模板**管理闭环**：仅 4 个只读 API，增删改只能写 SQL（用户确认痛点：不可能手动数据库建类型）；Create 的 custom_data 不按 `ticket_type_fields` schema 校验 | 2026-08-31 需求确认 + Duke1616/eflow 范式调研支撑。**数据骨架已就绪**（types.states/transitions 数据驱动状态机 + ticket_type_fields schema 表 + 读 API），缺管理面与校验闭环。**方案（eflow 范式）**：① 类型/字段/模板 CRUD + **发布/停用两态**（有工单的类型禁删只可停用、code 不可改）；② Create 按 schema 校验（required + regex；字段类型 7 种：input/textarea/number/date/select/multi_select/tips）；③ 前端管理页（列表+三步向导+字段右抽屉）+ 动态表单渲染器（消费既有读 API；**前端规格见 phase3/12-frontend §2/§3.1–3.2**，2026-08-31 已编写） | **✅ 已实施（IW3，2026-08-31）**：迁移 000018（validate_regex 列 + 类型配置页菜单/menu_apis，权限码 `ticket:type:manage`）+ 7 个管理端点（类型 CRUD/字段全量替换/模板 CRUD，含防呆：有工单禁删只可停用、code 不可改、select 必填选项、正则可编译）+ **G2 创建时 schema 校验**（required/类型/选项/regex）+ 集成测试 TestBK18×2；前端管理页/动态表单照 12-frontend 施工（另排期）。与 Phase 3 引擎零耦合如前述 |

---

## 10. 变更记录

| 日期 | 说明 |
|------|------|
| 2026-08-19 | 初版：P2-D1~D5 编码前拍板 |
| 2026-08-26 | P2-D6：工单可见性设计边界（V1~V8 + V0 宽松优先总则 + V9 模块耦合边界：2a 最简 assigned / 2b 策略B / 跨部门宽松三机制 / per-ticket 隔离·跨多组织·类型差异为 future / 跟人走+组织快照 / 组织重构不联动 / 工单只借 org_path 不订阅组织事件）+ 组织建模决策点（独立多根用 assigned_to 兜底，不强建虚拟总根）+ RK-12 |
| 2026-08-27 | 2a Step 0–3 收口：全量审查修复（P0 迁移合并 + P1×6：authorize 语义/swagger/gofmt 门禁/模板预填/软删例外/文档回标）；验收脚本首次端到端跑通（2a 脚本 ,string 字段补引号 + operator 绑菜单前置 + move 字段名修正 + None 安全解析；phase1 脚本 D2 断言前移 + B1-2 真同级 peer + #12 对齐 B4-3）；2c Step 编号统一为 8–10（对齐 P2-D7 与 README/04）；§9 登记遗留 BK-1~BK-10；**验收 88+65=153 用例全绿（M2a-3 达成）** |
| 2026-08-26 | P2-D7：Phase 2b 拆 core/org/ext 三轨（关键路径 2a→2b-core→2c；2b-org 并行；2b-ext 延后）；HR 同步降 2b-ext 延后、project_isolated 标 future（2b-core 仅交付 entity_transparent_read）、ticket:note 2a 口径对齐 assigned；Step 重排 2c:8–10；批次/分支策略同步更新 |
| 2026-08-31 | BK-11 ① 实施收口：Create 事务内 FOR SHARE 锁 org 行（FindByIDForShareTx）+ 锁窗口阻塞/Move×Create 锤击两回归 + 变异验证；§9 登记 **BK-12**（org_roles/parent_id 写侧缺口，触发条件驱动，HR 同步启动为硬触发器）；11-project-control §8 重写为「Phase 3 前置 vs 随行」遗留问题分类 |
| 2026-08-31（晚） | BK-12 必要性/业界评估补录（Entra/Google Groups/Keycloak 组驱动对照）；§9 登记 **BK-13**（project_isolated 强隔离激活：用户多虚拟组场景触发，业界默认隔离对照 + 机制骨架盘点，待批准实施）；phase2/README §1.2.3 project_isolated 状态翻转 future→已触发（顺修 `ticket_isolated` 值名笔误）；全量扫 phase1/2/3 文档归拢散落项：SoD 延后决策未落档（11-authz-review §4，P2-D7 编号已被三轨拆分复用）、phase2/12·13 号断链（14 号 5 处引用、文件从未入库）、review §7 item6 错误注入测试未落——三项并入 11 §8 A6 |
| 2026-08-31（夜） | §9 登记 **BK-14**（成员 scope 配置面：关系派拍板、scope=all 仅全局管理员可授、防提权复用 ensureCanManageMember，与 BK-13 同批实施）；外部 review 验证收口：**AGENTS.md 遗留节校准**（TC2/HC2 标已修、TC1 改「脚本已覆盖/Go 层缺」）、11 §6 TC1 口径修正、11 §8 增 **A7**（TC1-Go 测试）+ W1 升级为「BK-13+BK-14 场景闭环」；建 **docs/phase3/00-startup-checklist.md**（Phase 3 启动检查单：A/B/W 三档 + 决策清单 + 七步流程） |
| 2026-08-31（IW1 实施） | **IW1 批次代码落地**：BK-13（迁移 000017 + ticket_visibility 全链 + **L2 委托轴** + D12/委托轴测试）、BK-14（scope 配置面全链 + scope=all 管理员限定 + 审计）、BK-17（角色缓存消重）；验证：新测试 7 个全绿、集成 13 包 `-race` 全绿、acceptance 全链 **211 PASS / 0 FAIL**（000017 已应用）、`make swag`（新增 /orgs/members/scope） |
| 2026-08-31（A 档清零） | A1/A4/A5/A6/A7 全部完成（HC1 事件、BK-5 反向判重、TC1-Go、SoD 落档、review/10 处置、README 修正）；BK-5 关闭；**Phase 2 收官**——开放项仅剩触发条件驱动的 IW2/B6 与 Phase 3（暂缓） |
| 2026-08-31（IW3 实施） | **IW3/BK-18 管理闭环后端落地**：迁移 000018（validate_regex + 类型配置页菜单/menu_apis，`ticket:type:manage`）+ 7 管理端点（类型 CRUD/字段全量替换/模板 CRUD，防呆守卫）+ **G2 创建时 schema 校验**（required/类型/选项/regex，含模板预填合并后校验）+ TestBK18×2；验证：集成 13 包 `-race` 全绿、acceptance 全链 **211 PASS / 0 FAIL**（000017+000018 已应用）、swag/lint 干净。管理闭环后端收官，前端照 12-frontend 施工 |
| 2026-08-31（BK-16 复核关闭） | 外部核验指出 BK-16 疑似已落地 → git 考古（66e2c39，Phase 2a）证实 RowsAffected 校验**一直存在**：BK-16 系并发审计读码截断导致的误报，登记关闭；描述的"重复 200 + 重复事件"不存在（0 行守卫回滚）。教训：读函数必须读到函数尾。双删 404 回归断言列入随手项（可选） |
| 2026-08-31（跨文档同步） | 外评五条核验：11 §8 B9 跟进（6→5 份待编写，02/12 已编写入账 + W2 硬前置收紧）；README ④ 与 B1③ 对齐为同组三项（撤回/快照/Assignee；绑定另拍为 §4.10 决议 2）；12-frontend 定位明确为 BK-18 前端+审批前端总规格（与 BK-18 互引）；README 勾选序 ①②③⑤④ → ①②③④⑤；activelist 重复登记已于上轮修复 |
| 2026-08-31（Phase 3 补版+21 项拍板） | 21 项讨论项全部拍板：Wave 结构/W2 硬前置/12-frontend/activelist W4（README §2.1.0 勾选落定）；**A2 拍板**（000017 谁先启动谁占用）、**A3 拍板**（BK-11② 保留镜像列）；7-0 十二项决议回写 10 号（§2.5/§2.2/§3.2/§4.10/§5/§6/§8 TB12-16）；新文档 **02-multi-instance**（Watcher 移植 eiam + L1 消费 advisory lock + MI1-5 验收）与 **12-frontend**（技术栈拍板 + 四范式 + FE1-4） |
| 2026-08-31（生态调研） | Duke1616 生态调研（eflow/ecmdb-web/eiam/etask）：G1+G2 方案具体化，§9 登记 **BK-18**（类型/字段/模板管理闭环，IW1 后批次）；10 号 §4.8 补实地调研修正（驳回回退硬骨头/workflow_definitions 版本快照/撤回设计缺失/网关条件勿用裸 SQL）；B1 增研究吸收项（撤回 WITHDRAWING 栅栏/快照/审批人策略模型 Assignee{rule,values} 解 min_level）；B9 补参考指针（Casbin Watcher 移植 eiam、Asynq 模式仿 etask） |
| 2026-08-31（性能审计） | 外评核实「角色展开每请求双查」（casbin.go:46 + ticket service.go:52 / org_service.go:37；ctx 中 roles 无 service 消费方）；§9 登记 **BK-17**（方案 A：request context 透传缓存，签名/测试零改动，随 IW1 修复） |
| 2026-08-31（并发审计） | 工单模块并发/多实例审计：并发防护逐项验证在位（乐观锁/守卫/FOR SHARE/竞态双测试）；多实例就绪——无进程内状态、共享层全在 Redis/DB（Lua 限流、advisory lock、DB 时间戳），唯一缺口 Casbin Watcher 维持 Phase 3 B9；§9 登记 **BK-16**（Delete RowsAffected 审计脏点，随 IW1 修复） |
| 2026-08-31（代码级审计） | **以代码为准的第二轮审计**：BK-7 两半项均已在代码落地（priority 400 + page 钳制回显）→ 关闭并同步 11 §8/检查单（删 B8）；testutil 迁移清单缺 000002/000007 系刻意排除纯数据 seed（DDL 16 个全齐，非缺口）；非测试代码 TODO/FIXME 零命中、无请求路径 panic、关键错误码与 api/errcode.md 一致、swagger 含 2c 端点；CC1/CC2/CC3/P0/MC2/MC3/BK-1/BK-2/BK-3/BK-15 修复声明逐一代码复核属实 |
| 2026-08-31（审计） | 完整性审计：BK-1..15 全量复核，**BK-4 行漏关修正**（常量化已落地）；review/10 C1–C4 逐条核验（C1 驳回维持/C2 已修/C4 真开放→11 §8 A1②、C3→B10）；09 合集抽查回注（F-01/02/03/18 已修、F-31④/F-32 入随手项） |
| 2026-08-31（夜二） | §9 登记 **BK-15** 并即时修复：删除 `UpdateAssignedTo` 死包装（currentStatus 参数零使用、全仓零调用方）；合并 Delete / UpdateAssignedToTx 新旧叠注释，更正删单权限（2c 委托可删，非 admin only）与 FK 行为（comments/relations CASCADE、events SET NULL）描述 |
