# 09 - 工单模块（ticket，Phase 2a + 2b）

> **Step 2（2a MVP）** + **Step 5（2b scope 升级）** + **Step 10（2c Authorize，见 [04-org-delegation](./04-org-delegation.md)）**。  
> 模块完整设计见 [modules/ticket.md](../modules/ticket.md)；**本文档为 phase2 实现 SSOT**。

---

## 0. 子阶段边界

| 子阶段 | Step | 交付 |
|--------|------|------|
| **2a** | 2 | 表结构、类型种子、CRUD、状态机、评论、TicketResource **assigned** |
| **2b** | 5 | `ticket_scope` group/all、ltree 列表过滤、附件（见 [10-storage](./10-storage.md)） |
| **2c** | 10 | 组 admin/owner + ancestor owner Authorize（[04 §4](./04-org-delegation.md#4-authorize-升级step-10)） |

**2a 不做**：附件、虚拟组绑定、group/all 过滤、SLA、Outbox。

---

## 1. 前置条件

**2a**：

- [ ] Phase 1 组织树 + 用户可用
- [ ] [02-authz-resource](./02-authz-resource.md) ScopeResolver + Registry 就绪

**2b**：

- [ ] [03-org-enhance](./03-org-enhance.md)：`user_orgs.ticket_scope`、虚拟组
- [ ] BFS 三源角色（RoleFetcher 扩展）可选，与 scope 独立

---

## 2. 数据模型

### Phase 2a 迁移

```sql
-- 见 modules/ticket.md §3；迁移文件 0000xx_ticket.up.sql

CREATE TABLE ticket_types (...);
CREATE TABLE ticket_type_fields (...);
CREATE TABLE tickets (
    id           BIGSERIAL PRIMARY KEY,
    type_code    VARCHAR(50) NOT NULL REFERENCES ticket_types(code),
    title        VARCHAR(200) NOT NULL,
    description  TEXT,
    priority     SMALLINT DEFAULT 3,
    status       VARCHAR(20) DEFAULT 'open',
    created_by   BIGINT NOT NULL,
    assigned_to  BIGINT,
    org_id       BIGINT NOT NULL REFERENCES organizations(id),
    org_path     ltree NOT NULL,
    custom_data  JSONB DEFAULT '{}',
    sla_due_at   TIMESTAMPTZ,
    created_at   TIMESTAMPTZ DEFAULT NOW(),
    updated_at   TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX idx_tickets_org_path ON tickets USING GIST (org_path);
-- ticket_comments, ticket_events 同上 modules/ticket.md
```

**创建工单时**：从 `organizations` 读 `path` 写入 `org_path`（与 org_id 同事务）。

### Phase 2b 增量

```sql
ALTER TABLE user_orgs ADD COLUMN ticket_scope VARCHAR(20) NOT NULL DEFAULT 'assigned';
-- CHECK (ticket_scope IN ('assigned', 'group', 'all'))
```

附件表见 [10-storage.md](./10-storage.md)。

### 种子数据（2a）

| type code | name | 默认 transitions |
|-----------|------|------------------|
| incident | 故障事件 | open → assigned → in_progress → closed |
| request | 服务请求 | 同上 |

菜单 / Casbin：新增「工单管理」目录 + `ticket:list/create/read/update/...` 按钮 → `menu_apis` → 角色绑定（seed 或迁移脚本）。

---

## 3. API 一览

| 方法 | 路径 | 权限码 | 2a | 2b 备注 |
|------|------|--------|-----|---------|
| GET | `/api/v1/tickets` | `ticket:list` | ✅ assigned 过滤 | group/all |
| POST | `/api/v1/tickets` | `ticket:create` | ✅ | |
| GET | `/api/v1/tickets/:id` | `ticket:read` | ✅ 404 语义 | |
| POST | `/api/v1/tickets/update` | `ticket:update` | ✅ 属主 | |
| POST | `/api/v1/tickets/close` | `ticket:close` | ✅ | |
| POST | `/api/v1/tickets/assign` | `ticket:assign` | admin/2b 主管 | scope=group/all |
| POST | `/api/v1/tickets/delete` | `ticket:delete` | admin | 2c + org admin |
| GET | `/api/v1/tickets/:id/comments` | `ticket:read` | ✅ | |
| POST | `/api/v1/tickets/comments` | `ticket:comment` | ✅ | |
| POST | `/api/v1/tickets/notes` | `ticket:note` | ✅ | |
| GET | `/api/v1/ticket-types` | `ticket:list` | ✅ | |
| GET | `/api/v1/ticket-types/:code/fields` | `ticket:list` | ✅ | |
| POST | `/api/v1/tickets/attachments/*` | — | ❌ | 2b storage |

**请求体规范**（POST 更新类）：

```json
// POST /api/v1/tickets/update
{ "id": "1001", "title": "新标题", "description": "...", "priority": 2 }

// POST /api/v1/tickets/assign
{ "id": "1001", "assigned_to": "5" }

// POST /api/v1/tickets/close
{ "id": "1001", "comment": "已解决" }
```

---

## 4. 状态机

默认 transitions（`ticket_types.transitions` JSONB）：

```json
{
  "open": ["assigned", "closed"],
  "assigned": ["in_progress", "open"],
  "in_progress": ["pending_verify", "rejected", "closed"],
  "pending_verify": ["closed", "in_progress"],
  "closed": ["open"],
  "rejected": ["open"]
}
```

**Transition 校验**：Service 内查配置，非法转换 → **400 + 90002**。

**写 ticket_events**：每次状态变更、分派、关闭。

---

## 5. 三层鉴权实现

### 5.1 Phase 2a（assigned）

见 [02-authz-resource](./02-authz-resource.md)：

- **List/GetFilter**：`(created_by = $uid OR assigned_to = $uid)`
- **Get 不可见**：404 + `ErrTicketNotFound`（**90001**——决策收口，便于客户端区分模块；**90001 / `ErrTicketNotFound` 尚未在 `errcode.go` 定义，Phase 2a 工单模块实现时写入**）
- **Update**：创建人或处理人；否则 403 + 70001

### 5.2 Phase 2b — scope 升级 + 部门内「读/写分离」（策略 B，默认）

> **部门内默认（策略 B）**：同一**实体部门**子树下，虚拟组 **兄弟节点之间可读不可改**——读扩大到实体锚点，写仍绑工单所属虚拟组。  
> **强隔离（策略 A）**：实体设 `ticket_visibility=project_isolated`，读仍仅本 virtual group（旧行为）。见 §5.2.1。

**读 / 写分离（不要混为一轴）**：

| 轴 | 规则（策略 B 默认） |
|----|---------------------|
| **L2 读**（列表/详情/comment） | 实体子树内 **透明可读** + 始终可见 `created_by/assigned_to = 我` 的工单 |
| **L3 写**（update/close/assign/delete） | 默认 **仅创建人** 可 update；**处理人** 可 close；**工单所属虚拟组的 admin/owner**（2c）可管本组；跨虚拟组 **只读不改** |

**Effective scope**（`ticket_scope`，仍作用于 **主管/全量** 等扩展路径）：对用户在某 org 取 `user_orgs.ticket_scope`；多 org **取 max**（all > group > assigned）。与 **实体透明读** OR 合并，不互相替代。

#### 5.2.1 实体 `ticket_visibility`

```sql
-- Phase 2b 迁移（organizations 表）
ALTER TABLE organizations ADD COLUMN ticket_visibility VARCHAR(30) NOT NULL DEFAULT 'entity_transparent_read';
-- CHECK (ticket_visibility IN ('entity_transparent_read', 'project_isolated'))
-- 仅 org_type IN (1,2,3) 实体有效；虚拟组继承最近实体祖先的配置
```

| 值 | 含义 |
|----|------|
| **`entity_transparent_read`**（**默认**） | 子树内任一 `user_orgs`（含虚拟组）成员 → L2 可读该实体子树下 **全部** 工单（含兄弟虚拟组） |
| **`project_isolated`** | L2 回退为仅 `ticket_scope` + 用户 **直接** 所属 org 的 ltree 路径（兄弟虚拟组互不可见） |

**EntityAnchorPath**：对用户的每个 `user_orgs.org_id`，取最近 `org_type IN (1,2,3)` 且 `source IN ('hr','system','local')` 的祖先（含自身）的 `path`；虚拟组成员的 anchor 为挂载实体，**不是**虚拟组 path。

**ReadAnchorPaths**（L2 并集）：

```go
func (r *ScopeResolver) ReadAnchorPaths(ctx context.Context, userID int64) ([]string, error) {
    paths := map[string]struct{}{}
    for _, m := range r.userOrgs(ctx, userID) { // expires_at 未过期
        entity := r.nearestEntityOrg(ctx, m.OrgID)
        if entity.TicketVisibility == "entity_transparent_read" {
            paths[entity.Path] = struct{}{}
        }
        // ticket_scope group/all：仍并入 VisibleOrgPaths（主管/全量）
        for _, p := range r.scopePathsForMembership(m) {
            paths[p] = struct{}{}
        }
    }
    return keys(paths), nil
}
```

**GetFilter**（策略 B 默认）：

```go
readPaths, _ := scopeResolver.ReadAnchorPaths(ctx, userID)
return Filter{
    Where: `(created_by = $1 OR assigned_to = $1 OR org_path <@ ANY($2::ltree[]))`,
    Args:  []interface{}{userID, readPaths},
}, nil
```

`ticket_scope=assigned` 且 **无** 实体透明读路径时，退化为 `(created_by OR assigned_to)`；有透明读 anchor 时仍可见兄弟虚拟组工单（**只读**）。

**canRead（2b）**：满足 GetFilter 条件；不可见 → **404 + 90001**。

**canOperate — 写（2b，2c 前）**：

| action | L3（2b） |
|--------|----------|
| read / comment | canRead |
| **update** | **`created_by == 我`**（**不含**仅因透明读看到兄弟组工单的处理人） |
| **close** | **`assigned_to == 我` OR `created_by == 我`** |
| assign | `ticket_scope∈{group,all}` 且工单在其 scope 子树内（主管）；2a 仍仅 admin |
| delete | 仅 admin bypass |

**canOperate — 写（2c 增量）**：在 2b 基础上，对 **`ticket.org_id` 所属 org** 增加 `org_member_role ∈ {admin,owner}` 或 `owner_user_ids` / ancestor owner（见 [04-org-delegation §4](./04-org-delegation.md#4-authorize-升级step-10)）。**仍不能**凭 vg_a admin 身份改 vg_b 工单。

**兄弟虚拟组示例**（`tech` 下 `vg_a` / `vg_b`，策略 B）：

| 用户 | 读 vg_b 工单 | 改 vg_b 工单 |
|------|-------------|-------------|
| 仅 vg_a 的 member | ✅ | ❌（非创建人/非 vg_b admin） |
| vg_a admin | ✅ | ❌（仅 vg_a 内可改） |
| vg_b 工单创建人（人在 vg_a） | ✅ | ✅ update（创建人） |
| tech 实体 scope=group 主管 | ✅ | 按 2b 主管 / 2c owner 规则 |

**assign / close「主管」**：2b 中 `ticket_scope IN (group, all)` 且工单 org 在其 **scope 子树**内（与实体透明读独立）。

### 5.3 Phase 2c

CheckOwner 扩展：见 [04-org-delegation §4](./04-org-delegation.md#4-authorize-升级step-10)。**GetFilter 不变**（L2 已在 2b 策略 B 定稿；D11 验收读/写分离）。

### 5.4 全生命周期 × 三层鉴权对照表

> **SSOT**：产品/实现对齐用。L1 = Casbin 路由；L2 = GetFilter / canRead；L3 = Authorize / canOperate。  
> `admin` / `superadmin` 在 `Authorize` 入口 bypass L2/L3；L1 仍走 Casbin matcher。

**统一链路**：JWT → L1 → L2 → L3 → 状态机等业务规则。

**失败语义**：

| 场景 | HTTP | 错误码 |
|------|------|--------|
| 无路由权限 | 403 | 70001 |
| 工单不可见 | 404 | 90001（待 Phase 2a 在 errcode.go 定义 `ErrTicketNotFound`） |
| 可见但无权操作 | 403 | 70001 |
| 非法状态转换 | 400 | 90002 |
| 工单已关闭再操作 | 409 | 90004 |

#### 核心 CRUD + 协作

| 动作 | API | L1 | L2 | L3 | 2a | 2b 增量 | 2c 增量 |
|------|-----|----|----|-----|-----|---------|---------|
| 列表 | `GET /tickets` | `ticket:list` | GetFilter | — | `created_by OR assigned_to` | **策略 B**：实体子树透明读 OR 本人；`project_isolated` 时按 scope | GetFilter 不变 |
| 详情 | `GET /tickets/:id` | `ticket:read` | canRead | read = canRead | 仅本人创建/被分派 | **兄弟虚拟组可读不可改**（策略 B） | 同 2b |
| 创建 | `POST /tickets` | `ticket:create` | — | create 恒 true | 校验 org 存在，写 org_path | 同 2a | 同 2a |
| 更新 | `POST /tickets/update` | `ticket:update` | canRead | **创建人**（2b）；+ vg admin/owner（2c） | 创建人或处理人 | **2b：仅创建人**（透明读≠可改） | + 工单 org 的 admin/owner；+ ancestor owner |
| 关闭 | `POST /tickets/close` | `ticket:close` | canRead | 处理人或创建人；+ vg admin（2c） | 仅处理人 | + scope 主管 | + org admin/owner |
| 分派 | `POST /tickets/assign` | `ticket:assign` | canRead | 2a 仅 admin | admin bypass | scope=group/all + 子树内 | + org admin/owner |
| 删除 | `POST /tickets/delete` | `ticket:delete` | — | admin | admin only | admin only | + org admin/owner；+ ancestor owner |
| 回复 | `POST /tickets/comments` | `ticket:comment` | canRead | comment = canRead | 可见即可评 | 同 2a | 同 2a |
| 内部备注 | `POST /tickets/notes` | `ticket:note` | canRead | 处理团队成员 | 见 modules/ticket §2.3 | 待实现对齐 modules | 待实现对齐 modules |
| 回复列表 | `GET /tickets/:id/comments` | `ticket:read` | canRead | read = canRead | 同详情 | 同详情 | 同详情 |

#### 元数据（2a）

| 动作 | API | L1 | L2 | L3 |
|------|-----|----|----|-----|
| 工单类型列表 | `GET /ticket-types` | `ticket:list` | — | — |
| 类型字段 | `GET /ticket-types/:code/fields` | `ticket:list` | — | — |

#### 附件（2b，见 [10-storage](./10-storage.md)）

| 动作 | API | L1 | L2 | L3 |
|------|-----|----|----|-----|
| 预签名上传 | `POST /storage/presign/upload` | 隐含 `ticket:update` | canRead | 对 ticket 有 update 权 |
| 确认关联 | `POST /tickets/attachments/confirm` | `ticket:update` | canRead | update 属主；对象 `created_by == 当前用户` |
| 附件列表 | `GET /tickets/:id/attachments` | `ticket:read` | canRead | read |
| 删附件 | `POST /tickets/attachments/delete` | `ticket:update` | canRead | update 属主 |

#### 阶段速查

| 阶段 | 可见性（L2） | L3 写 |
|------|-------------|--------|
| **2a** | 仅 `created_by OR assigned_to` | assign/delete 仅 admin；close 处理人；update 创建人/处理人 |
| **2b** | **策略 B 默认**：实体子树透明读 + 本人；可选 `project_isolated` | **update 仅创建人**；close 处理人/创建人；主管 assign/close（scope） |
| **2c** | 不变（D11） | 工单 **org_id** 的 admin/owner、ancestor owner 可 update/close/assign/delete |

#### 待产品确认（边界）

1. **不可见一律 404**（list 查不到、get 单条）— 已定稿。  
2. **2a assign 仅 admin** — 创建人分派需单独决策。  
3. **delete 不要求先可见** — org admin/ancestor owner 可删委托范围内工单。  
4. **`ticket:note` L3** — modules 要求「处理团队成员」，实现前需在 `02-authz-resource` 补 canOperate 规则。  
5. **多 org scope 合并取 max** — 与实体透明读 **OR** 合并（读取并集）。  
6. **策略 B 为部门内默认** — `ticket_visibility=entity_transparent_read`；敏感项目改 `project_isolated`。✅ 已定稿  
7. **透明读不可改** — 兄弟虚拟组工单仅 read/comment；update 需创建人或 **该工单 org** 的 admin/owner。✅ 已定稿

---

## 6. Service 流程（Create 示例）

```
POST /api/v1/tickets
1. Casbin ticket:create
2. 校验 type_code、org_id 存在
3. 读 org.path → org_path
4. INSERT tickets（created_by=当前用户, status=open）
5. INSERT ticket_events(action=created)
6. Hook OnAfterCreate（默认空）
7. 审计（可选 ticket 模块 tag）
```

---

## 7. 错误码（90000–90999）

| code | 常量 | message | HTTP |
|------|------|---------|------|
| 90001 | `ErrTicketNotFound` | 工单不存在 | 404 |
| 90002 | `ErrTicketInvalidTransition` | 非法状态转换 | 400 |
| 90003 | `ErrTicketTypeNotFound` | 工单类型不存在 | 404 |
| 90004 | `ErrTicketAlreadyClosed` | 工单已关闭 | 409 |

> 不可见工单对外统一 **90001**（决策收口：不再与 10004 二选一，10004 保留给通用资源不存在场景；**90001 / `ErrTicketNotFound` 尚未在 `errcode.go` 定义，Phase 2a 工单模块实现时写入，勿改号**）。

---

## 8. 测试用例

### 2a（Step 3 验收）

| # | 用例 | 预期 |
|---|------|------|
| T1 | 创建工单 | 200；org_path 正确 |
| T2 | A 列表 | 仅 A 相关工单 |
| T3 | A 读 B 工单 | 404 |
| T4 | A 更新自己的 open 工单 | 200 |
| T5 | assign 给 B | admin 或 2b 主管路径 |
| T6 | 非法 transition open→closed（若类型不允许） | 400 + 90002 |
| T7 | 无 ticket:list | 403 |

**测试落点约定**（B4）：状态机转换单测 → `internal/service/ticket/`；T1–T7 集成测试（testcontainers PG，复用 phase1 `testutil` 模式）→ `internal/service/ticket/`；路由/权限码 → `internal/router/router_test.go` 扩展。

### 2b（Step 5 + 8 验收）

| # | 用例 | 预期 |
|---|------|------|
| T8 | scope=group 用户列表 | 含子 org 工单 |
| T9 | vg_a member、策略 B | **可读** vg_b 工单；**不可 update** vg_b（403） |
| T10 | expires_at 过期成员 | 失去该 org 透明读与 scope 路径 |
| T11 | 附件 confirm | 见 storage S1–S2 |
| T12 | 实体 `project_isolated` | vg_a member **不可读** vg_b（404） |
| T13 | vg_a admin 改 vg_a 内他人工单 | 2c：**200**；2b：403（非创建人） |
| T14 | vg_a member 改自己创建的 vg_a 工单 | 200 |

### 2c

见 [04-org-delegation §7](./04-org-delegation.md#7-测试用例验收-ssot) D7–D12。

---

## 9. 涉及文件

```
internal/service/ticket/
  service.go
  resource.go
  state_machine.go
  hooks.go
internal/repository/ticket/
internal/handler/ticket_handler.go
internal/router/router.go              # 注册路由 + menu_apis 对齐
migrations/0000xx_ticket.up.sql
migrations/0000xx_ticket_menu.up.sql   # 可选：菜单与 Casbin
```

---

## 10. 待决策点

| 事项 | 建议 | 状态 |
|------|------|------|
| 不可见 404 code | 90001 | ✅ 建议 |
| 2a assign | 仅 admin | ✅ 见 02-authz-resource |
| 多 org scope 合并 | 取 max(all, group, assigned) | ✅ 建议 |
| Hook / 进程内事件 | 2a DefaultTicketHooks + channel | ✅ 见 modules/ticket |

---

## 11. 文档交叉引用

| 文档 | 关系 |
|------|------|
| [modules/ticket.md](../modules/ticket.md) | 完整模块设计、Hook、Phase 3 |
| [02-authz-resource.md](./02-authz-resource.md) | Registry、assigned；**全生命周期对照表见本文 §5.4** |
| [03-org-enhance.md](./03-org-enhance.md) | ticket_scope、虚拟组 |
| [10-storage.md](./10-storage.md) | 附件 |
| [04-org-delegation.md](./04-org-delegation.md) | 2c Authorize |
