# Phase 2 实现计划：业务可用

> **核心目标**：让第一个业务模块（工单）带着资源级权限跑起来。安全底线（登录限流、会话吊销）已在 Phase 1 完成，Phase 2 不再堆平台能力。
>
> 创建日期：2026-08-12  
> 修订：2026-08-14 — Phase 2 全套实现计划（01–04、09–10）已编写。

---

## 0. 子阶段总览

| 子阶段 | 目标 | 典型交付 | 验收焦点 | 关键路径 |
|--------|------|----------|----------|---------|
| **2a** | 资源级鉴权 + 工单 MVP | Registry 实现、TicketResource、工单 CRUD+状态机 | **assigned** 范围：仅本人创建/被分派的工单 | ✅ 必做 |
| **2b-core** | 工单可见性本体 | 策略 B（`entity_transparent_read`）+ `ticket_scope`(all/group/assigned) + `ticket_visibility` 字段 + GetFilter `<@` | **group/all** 可见性（部门内默认全可见） | ✅ 关键路径 |
| **2b-org** | 组织增强 | 虚拟组 CRUD + 成员 + `org_roles` + BFS 三源角色 | 跨部门加人、`expires_at` 临时成员 | 并行（依赖 2a） |
| **2b-ext** | 外延增强（可延后/按需） | storage 附件、auth-enhance、HR 目录同步、`project_isolated` 强隔离 | 附件上传、密码策略、组织数据同步 | 延后（不阻塞主线） |
| **2c** | 组织内委托 | owner、`org_member_role`、组内防提权、资源 Authorize | 负责人/组 admin 管成员与绑定 org 的资源（D1–D11） | 依赖 2b-core + 2b-org |

**为什么拆（2026-08-26 调整，宽松优先、基础为先）**：

1. **原 2b 过重**：把"工单可见性（核心）"与"虚拟组/HR同步/附件/密码策略（增强）"塞进同一阶段，关键路径被拖长。
2. **2b-core 是关键路径最短交付**：2a（工单 CRUD）→ 2b-core（工单能按组织可见）即可形成"能跑的工单模块"，符合 Phase 2「打基础」目标；其余增强并行或延后。
3. **2b-org（虚拟组/scope/角色）** 与 2b-core 可同时开工，但 2c 委托需两者都就绪。
4. **2b-ext（HR 同步 / storage / auth-enhance / project_isolated）** 按宽松优先原则**延后**：HR 同步 Phase 2 可用种子/手工维护组织数据，不阻塞主线；`project_isolated` 强隔离极少见，标 future；附件/密码策略与工单鉴权正交，独立可延后。
5. **2c 不拆 2d**：owner/admin 委托强耦合（D7–D9 同时需 `org_member_role` 与 Authorize），单节点交付（见 §1.3）。

**建议顺序（关键路径）**：Phase 1 验收 → **2a** → **2b-core** → **2c**；**2b-org** 与 2b-core 并行，**2b-ext** 按需延后。

---

## 1. Phase 2 边界

### 1.1 Phase 2a — 做什么

| 类别 | 模块 | 核心能力 | 文档 |
|------|------|---------|------|
| 资源级鉴权 | [authz-resource](./02-authz-resource.md) | ResourceRegistry 实现、属主判断、**assigned** 列表过滤 | **已编写** |
| 工单模块 | [ticket](./09-ticket.md) | 类型配置、状态机、TicketResource 注册；**无附件** | **已编写** |

**2a 刻意不做**：虚拟组、group/all scope、BFS 三源角色、对象存储、多设备 UI。

**2a 验收**（可独立演示）：

```
1. 有 ticket:list 权限的用户 A 只能看到 created_by=A 或 assigned_to=A 的工单
2. 对可见工单可 create / update / close（路由级 + 资源级）
3. 不可见工单详情返回 404（非 403）
4. ResourceRegistry.Authorize / GetFilter 有单元测试 + 工单集成测试
```

### 1.2 Phase 2b — 拆为 core / org / ext 三轨（2026-08-26 调整）

> 关键路径：**2b-core**（工单可见性本体）；**2b-org**（组织增强）与 core 并行；**2b-ext**（外延）延后/按需，不阻塞主线。

#### 1.2.1 2b-core — 工单可见性本体（关键路径）

| 类别 | 模块 | 核心能力 | 文档 |
|------|------|---------|------|
| 工单可见性 | [ticket](./09-ticket.md) §2b | 列表过滤升级为 group/all（策略 B `entity_transparent_read`）；`ticket_scope`(all/group/assigned)；`organizations.ticket_visibility` 字段；GetFilter `<@` | **已编写** |

**2b-core 验收**：

```
1. 主管（scope=group）可见本组织及子组织工单（策略 B 默认全可见）
2. assigned / all / group 三档 scope 生效
3. 同一实体子树内虚拟组兄弟节点「可读不可改」
```

#### 1.2.2 2b-org — 组织增强（与 core 并行）

| 类别 | 模块 | 核心能力 | 文档 |
|------|------|---------|------|
| 组织增强 | [org-enhance](./03-org-enhance.md) | 虚拟组、组织角色、scope（all/group/assigned）、临时成员有效期、BFS 三源角色 | 已编写 |

**2b-org 验收**：

```
1. 虚拟组（org_type=4）CRUD + 挂载实体下
2. 跨部门加人、expires_at 临时成员自动失效
3. org_roles + BFS 三源角色展开
```

#### 1.2.3 2b-ext — 外延增强（可延后 / 按需）

| 类别 | 模块 | 核心能力 | 文档 | 状态 |
|------|------|---------|------|------|
| 文件存储 | [storage](./10-storage.md) | S3 兼容、预签名 URL、工单附件 | **已编写** | 延后（与鉴权正交） |
| 认证增强 | [auth-enhance](./01-auth-enhance.md) | 多设备列表/踢出、密码复杂度 | **已编写** | 延后 |
| HR 目录同步 | [org-enhance](./03-org-enhance.md) | 每日拉取公司人员/部门 API | 已编写 | **延后**：Phase 2 组织数据可用种子/手工维护，不阻塞主线 |
| 强隔离 `project_isolated` | [ticket](./09-ticket.md) §5.2.1 | 实体设 `ticket_isolated` 兄弟虚拟组互不可见 | 已编写 | **future**：极少见，2b-core 只交付默认 `entity_transparent_read` |

### 1.3 Phase 2c — 组织内委托（新增节点，**不**并入 2b）

| 类别 | 模块 | 核心能力 | 文档 |
|------|------|---------|------|
| 组织委托 | [04-org-delegation](./04-org-delegation.md) | `owner_user_ids`、`user_orgs.org_member_role`、组内防提权 API | **已编写** |
| 资源 Authorize | [04-org-delegation §4](./04-org-delegation.md#4-authorize-升级step-9) + ticket | 负责人/组 admin 对绑定 org 的资源 CRUD | [modules/ticket.md](../modules/ticket.md) |

**为何不加进 2b**：委托层是 **「容器建好之后的组内治理」**，验收面不同（D1–D11），独立节点便于 review 与回滚；且依赖 2b-core + 2b-org 两者就绪。

**2c 验收**（见 [04-org-delegation §7](./04-org-delegation.md#7-测试用例验收-ssot)）：

```
1. owner / org_member_role DDL + 设负责人、任命组内 admin API
2. 组 admin 只能管 member；不能升/admin 互删
3. 负责人/组 admin 对绑定 org 的工单 update/delete 通过；member 403
4. 实体部门 owner 对子树 org 下资源可管（ltree + owner 链）
5. HR 不覆盖 owner 与 org_member_role
```

**前置**：Phase **2b-core + 2b-org** 验收通过（虚拟组、scope、TicketResource group 过滤已存在）。清单见 [04-org-delegation §1](./04-org-delegation.md#1-前置条件2b-必须已验收)。

**分期决策**：2c **不再拆** 2d；Step 8（成员分级）与 Step 9（Authorize）同批交付，理由见 [04-org-delegation §0](./04-org-delegation.md#0-边界与分期决策)。

### 1.4 文档索引

Phase 2 各子阶段 SSOT 见 [§4 文档索引](#4-文档索引)。

### 1.5 不做什么（整个 Phase 2）

| 不做 | 原因 | 阶段 |
|------|------|------|
| 每资源独立 Casbin Enforcer | 代码内联 + ltree 足够 | 策略需可配置时 |
| JWT RS256 / JWKS | 仍是单体 | Phase 3 |
| AK/SK | 无 M2M 调用方 | Phase 3 / 按需 |
| IAM 独立 / gRPC | 同进程 | Phase 3 |
| 缓存平台 | 工单跑通后再说 | Phase 3 |
| 审计异步 / Redis List | Phase 1 同步够用 | Phase 3 |
| API 级通用限流 | 登录限流在 Phase 1 | Phase 3 |

### 1.6 前置条件

**Phase 2a** 开始前，Phase 1 必须已完成：

- [ ] 认证鉴权框架可运行（含对抗路径验收）
- [ ] 所有 Phase 1 测试用例通过
- [ ] DB 迁移可重复执行（幂等）

**Phase 1 验收门禁证据链**（2026-08-19 核验，见 [phase1/12 号报告](../phase1/12-phase1-acceptance-report.md)）：

| 证据 | 运行入口 | 要求 |
|------|---------|------|
| 27 个验收用例（含限流/并发 refresh/Redis 停机/混合凭证等对抗路径） | `bash scripts/acceptance-phase1.sh`（需 Docker Compose 环境：`make docker-dev-up && make migrate-up && make dev`） | 27/27 通过 |
| 单元测试（jwt / middleware / config / jsonutil / priority / resource 等 6 包） | `go test -race -count=1 ./internal/...` | 全绿 |
| 集成测试（repository / service / router 等 7 包，testcontainers PG） | `make test-integration` | 全绿 |
| Phase 1 审查修复（F-1~F-10 + TOCTOU 等，提交 8be8205..d54bda6） | `git log --oneline` | 已合入当前分支 |
| 遗留缺口 G-1（Registry 注入边）/ G-2（authz_service stub 处置） | 见 [02-authz-resource.md §1 Step 0](./02-authz-resource.md) | 作为 2a 第一动作处理，不阻塞切分支 |

> 遗留缺口 G-3（Makefile 无 lint 目标）、G-4（文档索引滞后）不阻塞 2a，择机处理。

**Phase 2b** 开始前，**Phase 2a** 必须已完成：

- [ ] 工单 MVP + assigned 范围验收通过
- [ ] TicketResource 已注册且测试覆盖主路径

**Phase 2b 额外要求**（Phase 1 收尾）：

- [ ] Step 9 组织 CRUD 已实现（见 [phase1/README §2.4](../phase1/README.md#24-step-79-crud-补全计划)）

**Phase 2c** 开始前，**Phase 2b** 必须已完成：

- [ ] 虚拟组 + `ticket_scope` 验收通过
- [ ] TicketResource group/all 过滤可用
- [ ] 详见 [04-org-delegation §1](./04-org-delegation.md#1-前置条件2b-必须已验收)

---

## 2. 实施顺序

### 2.1 Phase 2a

```
Phase 1 完成
   │
   ├── Step 1: authz-resource（Registry + assigned GetFilter + TicketResource）
   │      │
   │      └── Step 2: ticket MVP（表结构、状态机、API、TicketResource）
   │
   └── Step 3: 2a 集成验收（assigned 范围 + 三层鉴权主路径）
```

| Step | 子阶段 | 模块 | 依赖 | 文档 |
|------|--------|------|------|------|
| 1 | 2a | authz-resource | Phase 1 | [02-authz-resource.md](./02-authz-resource.md) |
| 2 | 2a | ticket | Step 1 | [09-ticket.md](./09-ticket.md) |
| 3 | 2a | 集成验收 | Step 1–2 | 本文档 §1.1 |

### 2.2 Phase 2b（拆 core / org / ext）

```
2a 验收通过
   │
   ├── 2b-core（关键路径）
   │      └── Step 4: ticket 升级（group/all 过滤 + ticket_visibility 字段 + 404 语义不变）
   │
   ├── 2b-org（并行）
   │      └── Step 5: org-enhance（虚拟组 / scope / BFS / 临时成员 / org_roles）
   │
   └── 2b-ext（延后/按需，不阻塞 2c）
          ├── Step 6: storage（MinIO + 预签名 + 工单附件）
          ├── Step 7: auth-enhance（可与 Step 6 并行）
          └── HR 目录同步（延后，独立 Job）
```

| Step | 子阶段 | 模块 | 依赖 | 文档 |
|------|--------|------|------|------|
| 4 | 2b-core | ticket（scope 升级） | 2a | [09-ticket.md](./09-ticket.md) |
| 5 | 2b-org | org-enhance | 2a | [03-org-enhance.md](./03-org-enhance.md) |
| 6 | 2b-ext | storage | 2a | [10-storage.md](./10-storage.md) |
| 7 | 2b-ext | auth-enhance | Phase 1 | [01-auth-enhance.md](./01-auth-enhance.md) |

> 2b-core 与 2b-org 可并行；2b-ext（Step 6–7 + HR 同步）**不阻塞 2c**，按需排期。

### 2.3 Phase 2c

```
2b-core + 2b-org 验收通过
   │
   ├── Step 8: org-delegation（owner + org_member_role + 成员分级 API）
   │      │
   │      └── Step 9: ticket Authorize 升级（组 admin/owner 资源操作）
   │
   └── Step 10: 2c 集成验收（D1–D12）
```

| Step | 子阶段 | 模块 | 依赖 | 文档 |
|------|--------|------|------|------|
| 8 | 2c | org-delegation | 2b-core + 2b-org | [04-org-delegation.md](./04-org-delegation.md) |
| 9 | 2c | ticket Authorize | Step 8 | [04-org-delegation.md §4](./04-org-delegation.md#4-authorize-升级step-9) |
| 10 | 2c | 集成验收 | Step 8–9 | [04-org-delegation §7](./04-org-delegation.md#7-测试用例验收-ssot) |

### 2.4 迁移编号规划（自 000010 起）

Phase 1 已用至 **000009**（000008 双 primary 部分唯一索引、000009 Phase 1 加固索引与角色描述修正，均 review 03 号报告修复批次落地——D2-48）。Phase 2 各 PRD 中迁移文件占位 `0000xx` 统一按下表分配（按 Step 顺序预分配；**实际执行顺序编号、不跳号**——若某模块最终无表结构变更，其后编号依次前移并在对应 PRD 标注）：

| 编号 | 子阶段 | 内容 | PRD |
|------|--------|------|-----|
| 000010 | 2a | 工单表组：`ticket_types` / `ticket_type_fields` / `tickets` / `ticket_comments` / `ticket_events`（含 org_path 冗余；`ticket_events` 2a 建表仅审计用，L1 机制 Phase 3 启动时迁移 000021 补列） | [09-ticket.md §2](./09-ticket.md) |
| 000011 | 2b-core | 工单可见性：`organizations.ticket_visibility`（**Step 4 已执行**，[09 §5.2.1](./09-ticket.md)） | [09-ticket.md](./09-ticket.md) |
| 000012 | 2b-org | 组织增强：虚拟组（org_type=4）/ `user_orgs.ticket_scope` / 临时成员 / org `source` 列（原 000011 其余内容按不跳号规则顺延） | [03-org-enhance.md](./03-org-enhance.md) |
| 000013 | 2c | 组织委托：`organizations.owner_user_ids` / `user_orgs.org_member_role`（**2c 先行执行**，不跳号） | [04-org-delegation.md](./04-org-delegation.md) |
| 000014 | 2b-ext | 附件：`file_objects` / `ticket_attachments`（延后顺延） | [10-storage.md](./10-storage.md) |
| 000015 | 2a | 工单模板：`ticket_templates`（2a 前移，纯 DB） | [09-ticket.md §2](./09-ticket.md#工单模板2a-前移迁移-000015) |
| 000016 | 2a | 工单关联：`ticket_relations`（2a 前移，纯 DB） | [09-ticket.md §2](./09-ticket.md#工单关联2a-前移迁移-000016) |

**迁移编写规范**（对齐 Phase 1 经验）：

1. **幂等**：种子数据一律 `INSERT ... WHERE NOT EXISTS`（见 000002 模式）；DDL 用 `CREATE TABLE IF NOT EXISTS` / `CREATE INDEX IF NOT EXISTS`；
2. **down 迁移必须可逆且先让位**：回滚前先处理会阻塞还原的数据（如部分唯一索引还原前，冲突软删行加 `#del#` 后缀让位——见 000006 down 加固经验）；仅回滚无意义的数据修复（如弱口令标记）可为空操作但须注释说明；
3. **唯一索引过滤软删除**：新表唯一索引一律带 `WHERE deleted_at IS NULL` 的部分索引写法（Phase 1 F-6 教训）。

---

## 3. 待决策点

| 事项 | 说明 | 状态 |
|------|------|------|
| 2a 是否含 BFS 三源 | 建议 **不含**，assigned 只需直接角色；BFS 随 org-enhance 进 2b | ✅ 建议已采纳 |
| 工单状态机 | 自研简单状态机 | 建议自研，见 `modules/ticket.md` |
| 虚拟组与实体组织 | 统一建表 + org_type=4；HR 同步见 [hr-directory-sync.md](../proposal/hr-directory-sync.md) | ✅ 已决策 |
| HR Reparent 策略 | 实体部门撤销时虚拟组默认上挂最近 HR 祖先 | ✅ 建议（hr-directory-sync §4.3） |
| 对象存储 | 开发 MinIO，生产按需 | 建议 MinIO |
| 附件 | 预签名 URL 直传 | 建议预签名，**2b 再做** |

---

## 4. 文档索引

| 文档 | 子阶段 | 状态 |
|------|--------|------|
| [00-implementation-plan.md](./00-implementation-plan.md) | 全程 | **执行总计划**：编码前拍板（P2-D1~D7）、里程碑门禁（M2a-0~M2c-3）、任务分解、批次/分支/流程、节奏估算、风险登记、2a 收口遗留清单（§9 backlog） |
| [02-authz-resource.md](./02-authz-resource.md) | 2a | **已编写** |
| [09-ticket.md](./09-ticket.md) | 2a + 2b + 2c 引用 | **已编写** |
| [03-org-enhance.md](./03-org-enhance.md) | 2b | 已编写（HR 同步见 proposal） |
| [04-org-delegation.md](./04-org-delegation.md) | 2c | **已编写** |
| [10-storage.md](./10-storage.md) | 2b | **已编写** |
| [01-auth-enhance.md](./01-auth-enhance.md) | 2b | **已编写** |

> cache / audit-async / RS256 / AK/SK 等已后移至 Phase 3，见 [phase3/README.md](../phase3/README.md)。
