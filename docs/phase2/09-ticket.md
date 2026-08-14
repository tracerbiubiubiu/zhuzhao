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
- **Get 不可见**：404 + `ErrNotFound`（10004 或工单专用 90001）
- **Update**：创建人或处理人；否则 403 + 70001

### 5.2 Phase 2b — scope 升级

**Effective scope**：对用户在某 org 取 `user_orgs.ticket_scope`；多 org **取最大可见范围**（all > group > assigned）。

**GetFilter**：

```go
switch effectiveScope {
case ScopeAll:
    return Filter{Where: "1=1", Args: nil}, nil
case ScopeGroup:
    paths, _ := scopeResolver.VisibleOrgPaths(ctx, userID)
    return Filter{
        Where: "org_path <@ ANY($1::ltree[])",
        Args:  []interface{}{paths},
    }, nil
case ScopeAssigned:
    return Filter{Where: "(created_by = $1 OR assigned_to = $1)", Args: []interface{}{userID}}, nil
}
```

**VisibleOrgPaths**：用户所属组织的 `path` 列表（2b 含虚拟组；临时成员 `expires_at > now()`）。

**单条 read 可见性**：对 ticket.org_path，存在用户某可见 org 的 path 使 `ticket.org_path <@ userOrg.path`（group），或 assigned/all 规则。

**assign / close「主管」**：2b 中 `ticket_scope IN (group, all)` 且工单 org 在其可见子树内。

### 5.3 Phase 2c

CheckOwner 扩展：见 [04-org-delegation §4](./04-org-delegation.md#4-authorize-升级step-10)。**GetFilter 不变**（D11）。

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

> 不可见工单对外统一 **90001 / 10004**（实现二选一，文档推荐 **90001** 便于客户端区分模块）。  
> 写入 `errcode.go` 时勿改号。

---

## 8. 测试用例

### 2a（Step 3 验收）

| # | 用例 | 预期 |
|---|------|------|
| T1 | 创建工单 | 201/200；org_path 正确 |
| T2 | A 列表 | 仅 A 相关工单 |
| T3 | A 读 B 工单 | 404 |
| T4 | A 更新自己的 open 工单 | 200 |
| T5 | assign 给 B | admin 或 2b 主管路径 |
| T6 | 非法 transition open→closed（若类型不允许） | 400 + 90002 |
| T7 | 无 ticket:list | 403 |

### 2b（Step 5 + 8 验收）

| # | 用例 | 预期 |
|---|------|------|
| T8 | scope=group 用户列表 | 含子 org 工单 |
| T9 | scope=assigned 仅虚拟组成员 | 仅本人工单 |
| T10 | expires_at 过期成员 | 失去 group 可见性 |
| T11 | 附件 confirm | 见 storage S1–S2 |

### 2c

见 [04-org-delegation §7](./04-org-delegation.md#7-测试用例验收-ssot) D7–D11。

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
| [02-authz-resource.md](./02-authz-resource.md) | Registry、assigned |
| [03-org-enhance.md](./03-org-enhance.md) | ticket_scope、虚拟组 |
| [10-storage.md](./10-storage.md) | 附件 |
| [04-org-delegation.md](./04-org-delegation.md) | 2c Authorize |
