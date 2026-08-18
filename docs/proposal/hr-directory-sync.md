# HR 目录同步与虚拟组挂载

> 公司 HR/主数据提供「人员 + 部门」接口，IAM 定时拉取并写入本地。**虚拟组挂在实体组织下**，会随实体树 **path 级联**变化，但 **成员与权限不得被 HR 对账误伤**。
>
> **阶段**：Schema 与虚拟组同属 **Phase 2b**（[phase2/03-org-enhance.md](../phase2/03-org-enhance.md)）；Phase 1 仍为管理端手工 CRUD + 种子数据。
>
> 创建日期：2026-08-14

---

## 1. 背景与目标

### 1.1 典型场景

| 场景 | 说明 |
|------|------|
| 每日同步 | Cron 调用公司 HR API，upsert 部门与用户 |
| 虚拟项目组 | 管理员在「技术中心」下创建虚拟组，成员可跨多个实体部门 |
| 部门调整 | HR 侧部门合并、换父节点、撤销 |
| 人员变动 | 入职、离职、换主部门 |

### 1.2 设计目标

1. **实体组织（org_type 1–3）** 以 HR 为 source of truth（带 `source=hr` 标记）。
2. **虚拟组（org_type 4）** 本地托管（`source=local`），挂在实体节点下，**不参与 HR 删库对账**。
3. 实体部门 **移动** 时，ltree **整棵子树 path 更新**（含其下虚拟组）——这是预期行为。
4. 实体部门 **撤销** 时，其下虚拟组走 **reparent 策略**，不丢成员、不静默删除。
5. 用户同步 **只动 HR 主部门**；虚拟组成员、`user_roles` 不被覆盖。

### 1.3 非目标（Phase 2b）

- 实时双向同步（仅每日 batch + 手动触发重跑）
- HR 侧管理虚拟组
- 多父级组织（仍用 ltree 单父树；见 [system-comparison.md](../design/system-comparison.md) #9）

---

## 2. 数据模型扩展（Phase 2b 迁移）

在 [phase1/06-organization.md](../phase1/06-organization.md) 的 `organizations` / `users` 上增加：

```sql
-- organizations
ALTER TABLE organizations ADD COLUMN source VARCHAR(20) NOT NULL DEFAULT 'local';
  -- hr | local | system
ALTER TABLE organizations ADD COLUMN external_id VARCHAR(100);
  -- HR 部门 ID；source=hr 时必填，UNIQUE(source, external_id) WHERE deleted_at IS NULL
ALTER TABLE organizations ADD COLUMN synced_at TIMESTAMPTZ;

-- users
ALTER TABLE users ADD COLUMN source VARCHAR(20) NOT NULL DEFAULT 'local';
ALTER TABLE users ADD COLUMN external_id VARCHAR(100);   -- HR 主键（API record id）
ALTER TABLE users ADD COLUMN synced_at TIMESTAMPTZ;
-- employee_no / domain_account / user_domain 已在 Phase 1 users 表；HR Job 按 external_id + employee_no 对账

-- user_orgs：Phase 1 已有 is_primary；Phase 2b 增加 source / expires_at
ALTER TABLE user_orgs ADD COLUMN source VARCHAR(20) NOT NULL DEFAULT 'local';
  -- hr | local
ALTER TABLE user_orgs ADD COLUMN expires_at TIMESTAMPTZ;
  -- 虚拟组临时成员（Phase 2b）
```

### 2.1 节点分类

| 类型 | org_type | source | 谁维护 | 示例 |
|------|----------|--------|--------|------|
| 种子根 | 1 | system | 迁移种子 | `root` |
| 实体部门 | 1–3 | hr | HR Sync Job | `tech`、`root.tech` |
| 虚拟组 | 4 | local | 管理端 IAM | `root.tech.vg_alpha` |
| 手工实体 | 1–3 | local | 管理端（可选） | 临时项目组挂靠节点 |

虚拟组 **创建约束**：

- `parent_id` 必须指向 `source IN ('hr','system')` 且 `org_type IN (1,2,3)` 的节点（或已 disabled 但尚未 reparent 的 hr 节点——见 §4.3）。
- `code` 须匹配 ltree 标签 `[A-Za-z0-9_]`；建议虚拟组 `code` 以 `vg_` 前缀区分 HR 部门编码。

### 2.2 树示例

```text
root (system)
└── tech (hr, path=root.tech)
    ├── fe (hr, path=root.tech.fe)
    └── vg_project_alpha (local, org_type=4, path=root.tech.vg_project_alpha)
```

---

## 3. HR Sync Job（分域 desired-state）

建议独立 Job：`cmd/sync-hr` 或 Asynq periodic task（Phase 2b），**幂等**，可手动重跑。

> **本节补充**：HR API **分页拉取**、**落库策略**、**定时调度**、**失败与重试** — 见 [§3.4](#34-拉取分页落库与调度) 与 [§3.5](#35-失败场景与重试)。

### 3.1 组织同步 — 三条硬规则

**规则 A — 只 reconcile HR 实体节点**

```text
允许 INSERT / UPDATE / SOFT-DELETE：
  organizations WHERE source = 'hr' AND org_type IN (1,2,3)

禁止 DELETE / UPDATE（结构）：
  source IN ('local', 'system')  OR  org_type = 4
```

**规则 B — 实体移动时，子树 path 级联（含虚拟组）**

与 [phase1/06-organization.md](../phase1/06-organization.md) Move 相同：凡 `path <@ old_entity_path` 的节点（含 `org_type=4`）一并更新 path。  
部门从 `root.tech` 调到 `root.product` 下时，`root.tech.vg_alpha` → `root.product.vg_alpha`。

**规则 C — HR 撤销部门：不得硬删「其下仍有虚拟组」的节点**

```text
HR 标记部门 D 撤销：
  IF EXISTS 子节点 WHERE source='local' AND org_type=4：
    → 不 DELETE D；D.status=禁用 或 D 软删但保留 id
    → 执行 §4.3 Reparent 策略
  ELSE IF 仅有 hr 子部门：
    → 按 HR 树合并/软删（先子后父）
```

### 3.2 用户对账

| 操作 | 范围 |
|------|------|
| Upsert 用户 | `source=hr`；对账键 **`external_id` 优先**，其次 **`employee_no`**；同步 `real_name`、主部门等 |
| 域字段 | HR/AD 若提供则写入 `domain_account`、`user_domain`；**不覆盖**本地 `username`（除非策略配置） |
| 更新主部门 | `user_orgs`：`is_primary=true AND source=hr` 一条，指向 HR 主部门 |
| 离职 | `users.status=禁用` + `user:disabled` + 删全部 RT（同 [phase1/02-auth.md](../phase1/02-auth.md)） |
| **不覆盖** | `user_orgs` 中 `source=local`（虚拟组）、`user_roles`、`is_system` 用户 |

登录时账号禁用仍返回 **401 + 与密码错误相同文案**（防枚举）；已登录会话用 **403 + 30003**。

### 3.3 幂等与对账键

- 组织：`UNIQUE(source, external_id) WHERE deleted_at IS NULL`（hr 域）
- 用户：`external_id` 同上；`employee_no` / `(user_domain, domain_account)` 为**全局唯一**（含软删，不可复用，见 [04-user §软删除](../phase1/04-user.md#软删除)）
- 不用「全表 DELETE 再 INSERT」；用 upsert + 软删缺失项
- 本地 `code` 若与 HR 编码一致，**以 `external_id` 为对账主键**，`code` 可随 HR 变更而更新并重算 path（需子树级联）

### 3.4 拉取、分页、落库与调度

#### 3.4.1 HR API 分页拉取

公司 HR 接口通常分页返回，**`HRDirectoryClient` 负责翻页**，`HRSyncService` 只消费「完整快照迭代器」：

```go
// internal/integration/hr/client.go

type PageOptions struct {
    PageSize int // 默认 200，可配置 hr.page_size
}

// 方式 A（推荐）：Client 内部翻页，对 Sync 暴露迭代器
type HRDirectoryClient interface {
    ListDepartments(ctx context.Context, opts PageOptions) HRDepartmentIterator
    ListUsers(ctx context.Context, opts PageOptions) HRUserIterator
}

type HRDepartmentIterator interface {
    Next(ctx context.Context) (HRDepartment, error) // io.EOF = 本域拉取结束
}

// 方式 B：显式页码（HR API 若是 page/pageNum 风格）
FetchDepartmentsPage(ctx context.Context, page, pageSize int) (items []HRDepartment, total int, err error)
```

**拉取顺序**：先 **部门全量分页** → 内存或临时表建树 → 再 **用户全量分页** reconcile（用户依赖部门 `external_id` 已存在或占位）。

**限流**：Client 内可配置 `hr.qps` / 请求间隔，避免打爆 HR 网关。

#### 3.4.2 落库：写业务表，不是只缓存 HR 响应

HR 数据 **必须持久化到 PostgreSQL 业务表**（IAM 运行时只读本地库，不实时调 HR）：

| 数据 | 落库位置 | 说明 |
|------|----------|------|
| 部门 | `organizations` | `source='hr'`，`external_id`，`synced_at` |
| 人员 | `users` | `source='hr'`，`employee_no`，`external_id`，`synced_at` |
| 主部门 | `user_orgs` | `source='hr'`，`is_primary=true` |
| 虚拟组 / 本地绑定 | 不变 | HR Job **不覆盖** `source=local` |

**不采用**「全表 DELETE 再 INSERT」。对账模型：**upsert + 本次快照未出现的 hr 记录软删/禁用**（§3.1 规则 C）。

**可选：`hr_sync_runs` 运行记录表**（建议 Phase 2b 一并建，便于排障与告警）：

```sql
CREATE TABLE hr_sync_runs (
    id            BIGSERIAL PRIMARY KEY,
    started_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    finished_at   TIMESTAMPTZ,
    status        VARCHAR(20) NOT NULL,  -- running | success | failed | partial
    trigger       VARCHAR(20) NOT NULL,  -- cron | manual
    org_fetched   INT DEFAULT 0,
    org_upserted  INT DEFAULT 0,
    user_fetched  INT DEFAULT 0,
    user_upserted INT DEFAULT 0,
    error_message TEXT,
    error_detail  JSONB                  -- 失败页码、external_id 等
);
CREATE INDEX idx_hr_sync_runs_started ON hr_sync_runs(started_at DESC);
```

> **是否需要 HR 原始 JSON 暂存表？** Phase 2b **不强制**；若 HR 字段复杂可先写 `hr_staging_departments/users`（JSONB），再 reconcile 到业务表。默认 **Client DTO → 直接 upsert 业务表** 即可。

#### 3.4.3 定时任务与并发控制

| 项 | 建议 |
|----|------|
| **调度** | Cron **每日 1 次**（如 `0 2 * * *` 凌晨 2 点，可配置 `hr.cron`） |
| **执行体** | Phase 2b：`cmd/sync-hr` + 系统 Cron / K8s CronJob；Phase 3b 可迁 **Asynq periodic**（与 [event-driven](../phase3/04-event-driven.md) 统一） |
| **手动触发** | 管理端 `POST /api/v1/admin/hr/sync`（superadmin，Phase 2b）或运维 CLI |
| **互斥** | 同一时刻只允许 **一个** Sync Run（Redis 分布式锁 `lock:hr_sync` 或 PG advisory lock）；Cron 触发时若已在跑则 **跳过并 Warn** |
| **超时** | 整 Job `context` 上限（如 2h）；单页 HR 请求 timeout（如 30s） |

```
Cron 02:00
  → 获取 lock:hr_sync
  → INSERT hr_sync_runs status=running
  → SyncAll（先 org 后 user）
  → UPDATE hr_sync_runs status=success|failed
  → 释放锁
```

### 3.5 失败场景与重试

#### 3.5.1 原则

1. **拉取失败 ≠ 清空本地 HR 数据**：对账未完成前，**保留上一轮成功同步的业务数据**（用户仍可登录、组织树仍可用）。
2. **幂等**：修复后可 **整 Job 重跑** 或 **手动触发**，不产生重复行（靠 `external_id` / upsert）。
3. **分阶段提交**：组织对账 **整阶段成功** 后再进用户对账；避免「用户指向尚未写入的部门」。

#### 3.5.2 HR API 拉取失败（网络 / 4xx / 5xx / 单页超时）

| 场景 | 处理 |
|------|------|
| **单页 transient 错误**（超时、502、429） | Client 内 **有限重试**（如 3 次，exponential backoff + jitter，尊重 `Retry-After`） |
| **单页重试仍失败** | **中止本次 Run**，`hr_sync_runs.status=failed`，**不写**本阶段 partial upsert（若尚未开始 reconcile） |
| **已拉取部分页后失败** | **中止 Run**；已拉数据仅存在于内存则丢弃；若用 staging 表则标记 run failed，**不 promote** 到业务表 |
| **HR 全量接口不可用** | 记录 failed + 告警；本地 `source=hr` 数据 **不变** |

#### 3.5.3 落库 / 对账失败（PG 错误、约束冲突、path 更新失败）

| 场景 | 处理 |
|------|------|
| **单条 upsert 失败**（如数据脏） | 记录 `error_detail`（external_id），**跳过该条继续**；Run 结束 `status=partial`，写审计 + 告警 |
| **事务性操作失败**（Move 子树、Reparent） | **回滚该事务**，Run `failed`；已成功的 earlier org 保留（按 run 设计：组织阶段可 **整阶段一个事务** 或 **按顶级部门分事务** — 建议 **按顶级 HR 根子树分事务**，失败子树跳过并 partial） |
| **用户对账阶段失败** | 组织阶段已成功则 **保留**；用户阶段 partial/failed，下次重跑只补 user |

#### 3.5.4 重试策略汇总

| 层级 | 机制 |
|------|------|
| **HTTP 单页** | Client 内立即重试 3 次 |
| **整 Job** | **不自动连续重试**；等 **下一次 Cron** 或 **人工触发**（避免 HR 故障时打满网关） |
| **Phase 3b 可选** | Asynq 任务：`hr:sync:all`，`MaxRetry=2`，间隔 1h（与 periodic 二选一，勿双调度） |

#### 3.5.5 可观测与告警

- 每次 Run 写 `hr_sync_runs` + 应用日志（`slog`：`run_id`、页码、耗时）。
- `status in (failed, partial)` → 审计事件 `hr.sync.failed` + 运维告警（邮件/ webhook，Phase 3a 可观测栈）。
- 管理端可查最近 N 次 Sync 状态（Phase 2b 可选 API）。

---

## 4. 虚拟组挂在实体下的影响与策略

### 4.1 两种「影响」

| 层次 | HR 树变化 | 期望行为 |
|------|-----------|----------|
| **结构挂载（ltree path）** | 父部门 move | 虚拟组 path **跟随**（规则 B） |
| **业务（成员/权限）** | 父部门撤销、人员换部门 | **不**自动清空虚拟组成员；走 reparent / 主部门更新 |

### 4.2 权限计算（与 HR 树解耦）

```text
实体 HR 树（org_type 1–3, source=hr）
  → Phase 2b：org_roles + group/all scope（ltree <@）

虚拟组（org_type 4, source=local）
  → user_orgs（source=local）+ ticket_scope / org_roles 绑在虚拟组 id 上
  → 成员可跨多个 HR 部门；不继承「父部门 HR 角色」
```

同一用户可在 A 部门 `scope=all`、在 B 虚拟组 `scope=assigned`（见 [modules/ticket.md](../modules/ticket.md)）。

### 4.3 实体部门撤销时的 Reparent 策略

可配置，**默认策略 A**：

| 策略 | 行为 | 适用 |
|------|------|------|
| **A. 自动上挂（默认）** | 虚拟组 `parent_id` 改为 D 的 **最近存在 HR 祖先**；重算 path；写审计 | 组织结构调整 |
| **B. 待管理员处理** | D 禁用，虚拟组标 `anchor_stale=true`（扩展字段），管理端告警 | 大合并前 |
| **C. 归档** | 虚拟组软删 + 成员 `expires_at` 到期 | 项目结束 |

默认 A 的伪代码：

```go
func (s *OrgSyncService) onHRDeptRemoved(ctx context.Context, dept *Organization) error {
    locals, _ := s.repo.ListLocalVirtualGroupsUnder(ctx, dept.ID)
    if len(locals) == 0 {
        return s.repo.SoftDeleteHRDept(ctx, dept.ID)
    }
    ancestor, _ := s.repo.NearestHRAncestor(ctx, dept.ParentID)
    for _, vg := range locals {
        if err := s.orgService.ReparentLocalVirtualGroup(ctx, vg.Code, ancestor.Code); err != nil {
            return err
        }
        s.audit.Log("hr.dept.removed.vg_reparented", vg.Code, ancestor.Code)
    }
    return s.repo.DisableHRDept(ctx, dept.ID) // 保留 id，不物理删
}
```

`ReparentLocalVirtualGroup` 复用手工 Move 的子树 path 更新逻辑。

---

## 5. 接口与模块边界

### 5.1 建议接口（Phase 2b）

```go
// 外部 HR 适配：不同公司 API 只实现此接口（内部分页，见 §3.4.1）
type HRDirectoryClient interface {
    ListDepartments(ctx context.Context, opts PageOptions) HRDepartmentIterator
    ListUsers(ctx context.Context, opts PageOptions) HRUserIterator
}

type HRSyncService interface {
    SyncOrganizations(ctx context.Context, runID int64) error
    SyncUsers(ctx context.Context, runID int64) error
    SyncAll(ctx context.Context, trigger string) error // 先 org 后 user；写 hr_sync_runs
}
```

- **OrgService / UserService**：管理端 CRUD + 虚拟组；**不**直接调 HR API。
- **HRSyncService**：唯一写 `source=hr` 数据的入口；依赖 `HRDirectoryClient` + `OrgRepo` / `UserRepo`。
- **Repository**：`OrgRepo` / `UserRepo` 为 interface（见 [phase1/01-infra.md](../phase1/01-infra.md)）；PG 实现 + 测试 Mock。

### 5.2 与 Phase 1 的关系

| 项 | Phase 1 | Phase 2b+ |
|----|---------|-----------|
| 组织/用户来源 | 手工 + 种子 | 增加 HR Job |
| `source` / `external_id` 字段 | 无 | 迁移增加 |
| 虚拟组 | 无 | org_type=4 |
| 删除组织 | 有子节点则拒绝 | 细化：`org_type=4` 子节点触发 reparent 流程 |

### 5.3 与 `POST /users` 手工创建的分工

| 问题 | 答案 |
|------|------|
| 正式员工谁创建？ | **HR Job** Upsert（`source=hr`），不是管理端 Create |
| `POST /users` 干什么？ | 建 **`source=local`** 账号：外包、测试号、HR 无编制、空窗期临时号等 |
| HR 已同步的人怎么登录？ | 管理员 **重置密码**；登录用 HR 同步的 **employee_no**；或 Phase 3 SSO |
| 角色、虚拟组谁管？ | **IAM 管理端**（`user_roles`、`user_orgs(source=local)`）；HR Job **不覆盖** |
| 冲突 | 同工号 `employee_no` 唯一 → 手工 Create 与 HR 撞车 **409**；应走同步或对账，勿双开 |
| 创建时调 HR API？ | **否**；只查本地库。HR 已同步 → 409 引导重置密码；local 账号建议不填工号 |

详见 [phase1/04-user §用户来源与创建场景](../phase1/04-user.md#用户来源与创建场景post-users-对应什么) 与 [§创建时要不要校验 HR](../phase1/04-user.md#创建时要不要校验hr-里是否存在)。

---

## 6. 架构示意

```text
                 ┌──────────── HR API (daily) ────────────┐
                 │  HRDirectoryClient 实现（按公司替换）   │
                 └──────────────────┬─────────────────────┘
                                    ▼
                          HRSyncService（分域对账）
                 ┌──────────────────┴─────────────────────┐
                 │ 仅写 source=hr  org_type 1-3           │
                 │ move → 子树 path 级联（含 org_type=4）   │
                 │ 撤销 → reparent 虚拟组，不硬删           │
                 └──────────────────┬─────────────────────┘
                                    ▼
              organizations (ltree)          users / user_orgs
              ├─ hr 实体                     ├─ hr 用户 + 主部门
              └─ local 虚拟组 (type=4)       └─ local 虚拟组绑定（不覆盖）

              本地 IAM：虚拟组 CRUD、成员、expires_at、user_roles
```

---

## 7. 验收用例（Phase 2b）

| # | 场景 | 预期 |
|---|------|------|
| 1 | HR 新增部门 | `source=hr` 节点出现，path 正确 |
| 2 | HR 移动部门 | 该部门及子树 path 更新；其下虚拟组 path 同步变更 |
| 3 | HR 撤销部门且下有虚拟组 | 虚拟组 reparent 到 HR 祖先；虚拟组成员仍在 |
| 4 | HR 全量对账 | `source=local` 虚拟组未被删 |
| 5 | HR 用户换主部门 | 仅 `user_orgs(is_primary, source=hr)` 更新 |
| 6 | HR 用户离职 | 账号禁用 + 会话吊销；虚拟组 `user_orgs(local)` 保留至 expires |
| 7 | 重复跑 Job | 幂等，不重复插入，不覆盖 `created_at` |
| 8 | HR API 第 2 页超时 | Client 重试 3 次后 Run `failed`，本地 hr 数据不变 |
| 9 | 用户对账 1 条脏数据 | Run `partial`，其余用户成功，告警 |
| 10 | Cron 触发时上一 Run 仍在跑 | 跳过本次，Warn 日志 |
| 11 | 手动重跑 | 新 `hr_sync_runs` 行，幂等 upsert |
| 12 | 父部门 org_roles | 子部门成员不自动获得父部门绑定角色 | 403 或菜单不可见（仅 direct/org 本地 org_roles 生效） |

---

## 8. 相关文档

| 文档 | 关系 |
|------|------|
| [modules/organization.md](../modules/organization.md) | 组织模块、虚拟组、挂载约束 |
| [modules/user.md](../modules/user.md) | 用户、主部门与虚拟组绑定 |
| [phase2/03-org-enhance.md](../phase2/03-org-enhance.md) | Phase 2b 实施范围 |
| [proposal/data-init.md](./data-init.md) | 种子幂等（`source=system`），与 HR 域分离 |
| [phase1/06-organization.md](../phase1/06-organization.md) | ltree Move 子树 SQL |
| [design/system-comparison.md](../design/system-comparison.md) | 实体树 + 虚拟组分离决策 |
