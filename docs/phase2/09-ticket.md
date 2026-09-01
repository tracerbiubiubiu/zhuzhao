# 09 - 工单模块（ticket，Phase 2a + 2b）

> **Step 2（2a MVP）** + **Step 4（2b-core scope 升级）** + **Step 9（2c Authorize，见 [04-org-delegation](./04-org-delegation.md)）**。  
> 模块完整设计见 [modules/ticket.md](../modules/ticket.md)；**本文档为 phase2 实现 SSOT**。

---

## 0. 子阶段边界

| 子阶段 | Step | 交付 |
|--------|------|------|
| **2a** | 2 | 表结构、类型种子、CRUD、状态机、评论、TicketResource **assigned** |
| **2b** | 4 | `ticket_scope` group/all、ltree 列表过滤、`ticket_visibility` 字段（见 [00 §3 Step 4](./00-implementation-plan.md)） |
| **2c** | 9 | 组 admin/owner + ancestor owner Authorize（[04 §4](./04-org-delegation.md#4-authorize-升级step-9)） |

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

### 工单模板（2a 前移，迁移 000015）

> 2026-08-25 从 Phase 3 前移到 2a（纯 DB，无事件依赖）。`default_sla_minutes` 仅存储，Phase 3 SLA 启用时取用。

```sql
-- 迁移 000015：工单模板（2a 前移，纯 DB）
CREATE TABLE IF NOT EXISTS ticket_templates (
    id                  BIGSERIAL PRIMARY KEY,
    code                VARCHAR(50) NOT NULL,
    name                VARCHAR(200) NOT NULL,
    type_code           VARCHAR(50) NOT NULL REFERENCES ticket_types(code),
    default_priority    SMALLINT DEFAULT 3,
    default_fields      JSONB DEFAULT '{}',       -- 预填字段（title/description/custom_data 片段）
    default_sla_minutes INT,                      -- 仅存储，Phase 3 SLA 启用时取用
    org_id              BIGINT NOT NULL REFERENCES organizations(id),
    org_path            ltree NOT NULL,
    created_by          BIGINT NOT NULL,
    created_at          TIMESTAMPTZ DEFAULT NOW(),
    updated_at          TIMESTAMPTZ DEFAULT NOW(),
    deleted_at          TIMESTAMPTZ
);
CREATE UNIQUE INDEX IF NOT EXISTS uq_ticket_templates_code ON ticket_templates(code) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_ticket_templates_org_path ON ticket_templates USING GIST (org_path);
```

### 工单关联（2a 前移，迁移 000016）

> 2026-08-25 从 Phase 3 前移到 2a（纯 DB）。建立关联时对 `target_ticket_id` 走 L2/L3 鉴权，防止越权关联他人工单。**实现取严（2a 回标）**：对 `source` 与 `target` 双端均要求 `update` 级鉴权（service.go CreateRelation）——关联是双向可见关系，单端可写即可挂接对方工单；注意 2b 把 update 收窄为仅创建人后（RK-11），建关联权限随之收紧，属预期联动。

```sql
-- 迁移 000016：工单关联（2a 前移，纯 DB）
CREATE TABLE IF NOT EXISTS ticket_relations (
    id                BIGSERIAL PRIMARY KEY,
    source_ticket_id  BIGINT NOT NULL REFERENCES tickets(id),
    target_ticket_id  BIGINT NOT NULL REFERENCES tickets(id),
    relation_type     VARCHAR(30) NOT NULL DEFAULT 'related',  -- related/blocks/duplicates/split
    created_by        BIGINT NOT NULL,
    created_at        TIMESTAMPTZ DEFAULT NOW(),
    deleted_at        TIMESTAMPTZ,
    CHECK (source_ticket_id <> target_ticket_id)
);
CREATE UNIQUE INDEX IF NOT EXISTS uq_ticket_relations_pair ON ticket_relations(source_ticket_id, target_ticket_id, relation_type) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_ticket_relations_target ON ticket_relations(target_ticket_id) WHERE deleted_at IS NULL;
```

**创建工单时**：从 `organizations` 读 `path` 写入 `org_path`（与 org_id 同事务）。`POST /tickets` 支持可选 `template_code`：命中则用 `ticket_templates.default_fields` 预填、`default_priority` 覆盖。

**priority 刻度（2026-08-31 收敛，IW1 后随 BK-18）**：`1紧急 / 2高 / 3中 / 4低`（`0` = 未传，归一为 3）。Create/Update 与模板 `default_priority` 三路径统一强制校验，越界 → 400；沿用 SMALLINT 列型，扩刻度不改表。

**组织 move 级联**（[P2-D1](./00-implementation-plan.md) 已拍板方案 A）：`POST /orgs/move` 更新组织子树 ltree path 时，同一事务内级联改写存量工单 `tickets.org_path`——`UPDATE tickets SET org_path = new_path || subpath(org_path, nlevel(old_path)) WHERE org_path <@ old_path`。不处理则 2b scope=group 静默漏单（旧工单从主管列表消失）。落地：2a Step 2 建表同批扩展 `OrgService.Move` + Step 4（2b-core）回归测试（move 后 scope 过滤仍正确）。

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

### 1.1 `000010_menu` 种子明细（迁移脚本，对齐 [data-init §4](../proposal/data-init.md) 写法）

> **原则（见 phase1/07-menu.md）**：操作类 POST 路径**不含 `:id`**（id 放 body），如 `/api/v1/tickets/update`；角色绑定**页面菜单**即获得该页全部 `menu_apis`。`menu_apis` 必须覆盖 API 表（§3）全部路由，否则 Casbin 漏鉴权、T7「无 ticket:list → 403」挂。
> **前端未写**：`menus` 树仅占位（一个「工单管理」目录 + 一个页面菜单），前端接入时再细化结构；但 `menu_apis` + 角色绑定**必须完整种**（后端 Casbin L1 拦截依赖，与前端无关）。

```sql
-- ⚠️ 2026-08-27 修正（3 项原始伪代码 bug 曾被误拷入迁移文件）：
--   ① menus.parent_id 是 INT FK 列；原文「parent_code」为伪代码列名（不存在，FK 漏建
--     会导致 org_move 级联和前端 tree 渲染断链）；
--   ② menus.menu_type 是 SMALLINT（1=目录 2=页面 3=按钮）；原文「'catalog'/'page'」
--     是伪代码语义值，直接执行会报列类型错；
--   ③ roles.code 列存的是 'admin'/'superadmin'（无 role:: 前缀）；role:: 前缀仅用于
--     casbin_rule.ptype=g 行（不同表不同列）。
--   补漏：新增 9 个 level-3 按钮菜单（含 9 个 permission 码）；role_menus 通配 admin/
--   superadmin 全部 11 个菜单（不绑定按钮前端权限配置漏出；绑定后 AssignMenus 生成
--   细粒度 Casbin 行对非通配角色完整生效）。
-- 最终实现文件：菜单 INSERT 并入 migrations/000010_ticket.up.sql（无独立 _menu 文件）

-- ① level-1 目录 + level-2 页面：parent_id 后补（INSERT 时 NULL，防止自引用 FK 顺序依赖）
INSERT INTO menus (code, name, parent_id, menu_type, path, component, icon, permission, sort_order, visible, is_system)
VALUES
  ('ticket_manage', '工单管理', NULL, 1, '/tickets',            '',                    'ticket',       NULL,            2, 1, true),
  ('ticket_list',   '工单列表', NULL, 2, '/tickets',            'ticket/list/index',   'ticket-list',  'ticket:list',   1, 1, true)
ON CONFLICT DO NOTHING;
UPDATE menus SET parent_id = (SELECT m2.id FROM menus m2 WHERE m2.code = 'ticket_manage')
WHERE code = 'ticket_list' AND parent_id IS NULL;

-- ② level-3 9 个按钮菜单（menu_type=3）；permission 码对应 §3 API 表权限列
-- 注：实际迁移文件通过 INSERT … VALUES + JOIN menus parent 实现，code 为 ticket_*_btn（非 p.code||'_*_btn'）
INSERT INTO menus (code, name, parent_id, menu_type, path, component, icon, permission, sort_order, visible, is_system)
SELECT 'ticket_list_btn',   '列表', p.id, 3, '', '', '', 'ticket:list',   10, 1, true FROM menus p WHERE p.code='ticket_list' UNION ALL
SELECT 'ticket_create_btn', '创建', p.id, 3, '', '', '', 'ticket:create', 11, 1, true FROM menus p WHERE p.code='ticket_list' UNION ALL
SELECT 'ticket_read_btn',   '详情', p.id, 3, '', '', '', 'ticket:read',   12, 1, true FROM menus p WHERE p.code='ticket_list' UNION ALL
SELECT 'ticket_update_btn', '编辑', p.id, 3, '', '', '', 'ticket:update', 13, 1, true FROM menus p WHERE p.code='ticket_list' UNION ALL
SELECT 'ticket_close_btn',  '关闭', p.id, 3, '', '', '', 'ticket:close',  14, 1, true FROM menus p WHERE p.code='ticket_list' UNION ALL
SELECT 'ticket_assign_btn', '分派', p.id, 3, '', '', '', 'ticket:assign', 15, 1, true FROM menus p WHERE p.code='ticket_list' UNION ALL
SELECT 'ticket_delete_btn', '删除', p.id, 3, '', '', '', 'ticket:delete', 16, 1, true FROM menus p WHERE p.code='ticket_list' UNION ALL
SELECT 'ticket_comment_btn','评论', p.id, 3, '', '', '', 'ticket:comment',17, 1, true FROM menus p WHERE p.code='ticket_list' UNION ALL
SELECT 'ticket_note_btn',   '备注', p.id, 3, '', '', '', 'ticket:note',   18, 1, true FROM menus p WHERE p.code='ticket_list';

-- ③ menu_apis 页面级绑定（仅绑定到页面菜单，按钮菜单不参与 L1 API 鉴权；按钮 permission 码
--    只用于前端按钮显隐 + 未来 AssignMenus 细粒度角色）。覆盖 §3 16 条路由。
INSERT INTO menu_apis (menu_id, api_path, api_method)
SELECT m.id, v.api_path, v.api_method FROM menus m, (VALUES
  ('ticket_list', '/api/v1/tickets',               'GET'),
  ('ticket_list', '/api/v1/tickets',               'POST'),
  ('ticket_list', '/api/v1/tickets/:id',           'GET'),
  ('ticket_list', '/api/v1/tickets/update',        'POST'),
  ('ticket_list', '/api/v1/tickets/close',         'POST'),
  ('ticket_list', '/api/v1/tickets/assign',        'POST'),
  ('ticket_list', '/api/v1/tickets/delete',        'POST'),
  ('ticket_list', '/api/v1/tickets/:id/comments',  'GET'),
  ('ticket_list', '/api/v1/tickets/comments',      'POST'),
  ('ticket_list', '/api/v1/tickets/notes',         'POST'),
  ('ticket_list', '/api/v1/tickets/:id/relations', 'GET'),
  ('ticket_list', '/api/v1/tickets/relations',     'POST'),
  ('ticket_list', '/api/v1/ticket-types',                 'GET'),
  ('ticket_list', '/api/v1/ticket-types/:code/fields',    'GET'),
  ('ticket_list', '/api/v1/ticket-templates',             'GET'),
  ('ticket_list', '/api/v1/ticket-templates/:code',       'GET')
) AS v(menu_code, api_path, api_method)
WHERE m.code = v.menu_code
ON CONFLICT DO NOTHING;

-- ④ role_menus 通配绑定 admin / superadmin：获得 11 个菜单（目录 + 页面 + 9 按钮）全部权限。
INSERT INTO role_menus (role_id, menu_id)
SELECT r.id, m.id FROM roles r, menus m
WHERE r.code IN ('superadmin', 'admin')
  AND m.code IN (
    'ticket_manage','ticket_list',
    'ticket_list_btn','ticket_create_btn','ticket_read_btn',
    'ticket_update_btn','ticket_close_btn','ticket_assign_btn',
    'ticket_delete_btn','ticket_comment_btn','ticket_note_btn'
  )
ON CONFLICT DO NOTHING;
```

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
| GET | `/api/v1/ticket-templates` | `ticket:list` | ✅ | |
| GET | `/api/v1/ticket-templates/:code` | `ticket:list` | ✅ | |
| GET | `/api/v1/tickets/:id/relations` | `ticket:read` | ✅ | |
| POST | `/api/v1/tickets/relations` | `ticket:update` | ✅ | 关联视为工单修改操作，复用 `update` 权限码（建立关联走 L2/L3 鉴权） |
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
- **Get 不可见**：404 + `ErrTicketNotFound`（**90001**——决策收口，便于客户端区分模块；**已于 2a Step 2 写入 `errcode.go`（L98），勿改号**）
- **Update**：创建人或处理人；否则 403 + 70001

### 5.2 Phase 2b — scope 升级 + 部门内「读/写分离」（策略 B，默认）

> **部门内默认（策略 B）**：同一**实体部门**子树下，虚拟组 **兄弟节点之间可读不可改**——读扩大到实体锚点，写仍绑工单所属虚拟组。  
> **强隔离（策略 A，future）**：实体设 `ticket_visibility=project_isolated`，读仍仅本 virtual group（旧行为）。**标 future（2026-08-26，宽松优先）**：极少见，2b-core 只交付默认 `entity_transparent_read`，强隔离留待真实需求出现再加字段+GetFilter 分支，不进当前范围。见 §5.2.1。

**读 / 写分离（不要混为一轴）**：

| 轴 | 规则（策略 B 默认） |
|----|---------------------|
| **L2 读**（列表/详情/comment） | 实体子树内 **透明可读** + 始终可见 `created_by/assigned_to = 我` 的工单 |
| **L3 写**（update/close/assign/delete） | 默认 **仅创建人** 可 update；**处理人** 可 close；**工单所属虚拟组的 admin/owner**（2c）可管本组；跨虚拟组 **只读不改** |

**Effective scope**（`ticket_scope`，仍作用于 **主管/全量** 等扩展路径）：对用户在某 org 取 `user_orgs.ticket_scope`；多 org **取 max**（all > group > assigned）。与 **实体透明读** OR 合并，不互相替代。

#### 5.2.1 实体 `ticket_visibility`

> **2026-08-26 调整（宽松优先）**：`project_isolated` 标 **future**，2b-core 仅落地默认 `entity_transparent_read`（单字段 + 默认约束即可，CHECK 暂不含 `project_isolated`，避免 GetFilter 提前分支）。强隔离字段与逻辑待真实需求再加。

```sql
-- Phase 2b-core 迁移（organizations 表，仅默认策略）——实际编号 000011，执行于 Step 4；
-- 原 000011（组织增强其余 DDL）按不跳号规则顺延为 000012
ALTER TABLE organizations ADD COLUMN ticket_visibility VARCHAR(30) NOT NULL DEFAULT 'entity_transparent_read';
-- CHECK (ticket_visibility IN ('entity_transparent_read'))   -- future 扩展时再加 project_isolated
-- 仅 org_type IN (1,2,3) 实体有效；虚拟组继承最近实体祖先的配置
```

| 值 | 含义 |
|----|------|
| **`entity_transparent_read`**（**默认，2b-core 交付**） | 子树内任一 `user_orgs`（含虚拟组）成员 → L2 可读该实体子树下 **全部** 工单（含兄弟虚拟组） |
| **`project_isolated`**（**future**） | L2 回退为仅 `ticket_scope` + 用户 **直接** 所属 org 的 ltree 路径（兄弟虚拟组互不可见）；待需求出现再加 |

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
| note | 创建人/处理人（**BK-1 读写一致**：内部备注可见集合 = 创建人/处理人/admin，透明读旁观者不可见亦不可写；Step 4 已落地 `TestB2_InternalNoteVisibility`；2c 扩 org admin/owner） |
| **update** | **`created_by == 我`**（**不含**仅因透明读看到兄弟组工单的处理人；**RK-11 收窄已于 Step 4 落地并回归**） |
| **close** | **`assigned_to == 我` OR `created_by == 我`** |
| assign | `ticket_scope∈{group,all}` 且工单在其 scope 子树内（主管）；2a 仍仅 admin |
| delete | 仅 admin bypass |

**canOperate — 写（2c 增量）**：在 2b 基础上，对 **`ticket.org_id` 所属 org** 增加 `org_member_role ∈ {admin,owner}` 或 `owner_user_ids` / ancestor owner（见 [04-org-delegation §4](./04-org-delegation.md#4-authorize-升级step-9)）。**仍不能**凭 vg_a admin 身份改 vg_b 工单。

**兄弟虚拟组示例**（`tech` 下 `vg_a` / `vg_b`，策略 B）：

| 用户 | 读 vg_b 工单 | 改 vg_b 工单 |
|------|-------------|-------------|
| 仅 vg_a 的 member | ✅ | ❌（非创建人/非 vg_b admin） |
| vg_a admin | ✅ | ❌（仅 vg_a 内可改） |
| vg_b 工单创建人（人在 vg_a） | ✅ | ✅ update（创建人） |
| tech 实体 scope=group 主管 | ✅ | 按 2b 主管 / 2c owner 规则 |

**assign / close「主管」**：2b 中 `ticket_scope IN (group, all)` 且工单 org 在其 **scope 子树**内（与实体透明读独立）。

### 5.3 Phase 2c

CheckOwner 扩展：见 [04-org-delegation §4](./04-org-delegation.md#4-authorize-升级step-9)。**GetFilter 不变**（L2 已在 2b 策略 B 定稿；D11 验收读/写分离）。

### 5.4 全生命周期 × 三层鉴权对照表

> **SSOT**：产品/实现对齐用。L1 = Casbin 路由；L2 = GetFilter / canRead；L3 = Authorize / canOperate。  
> `admin` / `superadmin` 在 `Authorize` 入口 bypass L2/L3；L1 仍走 Casbin matcher。

> **转部门/组织调整场景（2026-08-31 澄清，防重复误报）**：属主（创建人/处理人）经 **Get 与 List 双路径豁免 L2**——`Authorize` 的属主短路 + `GetFilter` 的 `created_by/assigned_to` OR 分支（resource.go），转部门后**历史工单始终可见**，对齐 Freshdesk/Jira 主流行为。转部门后失去可见性的仅为非属主旁观者与被改派的原处理人（组织边界正常语义，业界一致）。**勿再将「转部门后旧工单 404」作为缺陷提出**；若产品需要「历史参与过的工单」入口或 watcher 订阅，属 Phase 3 通知服务（Step 7b）的功能候选，非可见性修复。

> **无组织归属用户语义（2026-09-01 澄清，ginfast 对照核验时确认）**：用户无任何组织成员关系时锚点集为空，`GetFilter` 锚点支恒假，**退化为「属主 ∪ 委托」语义（非全拒）**——仍可见自己创建/被分派/受委托的工单。这是与 ginfast `created_by` 兜底同构的最小可见 fail-safe，**勿将「无组织用户看不到任何工单」作为预期**，也勿误记为「无锚点 = 不可见」。

**统一链路**：JWT → L1 → L2 → L3 → 状态机等业务规则。

**失败语义**：

| 场景 | HTTP | 错误码 |
|------|------|--------|
| 无路由权限 | 403 | 70001 |
| 工单不可见 | 404 | 90001（`ErrTicketNotFound`，已于 2a Step 2 写入 errcode.go） |
| 可见但无权操作 | 403 | 70001 |
| 非法状态转换 | 400 | 90002 |
| 工单已关闭再操作 | 409 | 90004 |

#### 核心 CRUD + 协作

| 动作 | API | L1 | L2 | L3 | 2a | 2b 增量 | 2c 增量 |
|------|-----|----|----|-----|-----|---------|---------|
| 列表 | `GET /tickets` | `ticket:list` | GetFilter | — | `created_by OR assigned_to` | **策略 B**：实体子树透明读 OR 本人（`project_isolated` 强隔离标 future，见 §5.2.1） | GetFilter 不变 |
| 详情 | `GET /tickets/:id` | `ticket:read` | canRead | read = canRead | 仅本人创建/被分派 | **兄弟虚拟组可读不可改**（策略 B） | 同 2b |
| 创建 | `POST /tickets` | `ticket:create` | — | create 恒 true | 校验 org 存在，写 org_path | 同 2a | 同 2a |
| 更新 | `POST /tickets/update` | `ticket:update` | canRead | **创建人**（2b）；+ vg admin/owner（2c） | 创建人或处理人 | **2b：仅创建人**（透明读≠可改） | + 工单 org 的 admin/owner；+ ancestor owner |
| 关闭 | `POST /tickets/close` | `ticket:close` | canRead | 处理人或创建人；+ vg admin（2c） | 仅处理人 | + scope 主管 | + org admin/owner |
| 分派 | `POST /tickets/assign` | `ticket:assign` | canRead | 2a 仅 admin | admin bypass | scope=group/all + 子树内 | + org admin/owner / ancestor owner + 子树 |
| 删除 | `POST /tickets/delete` | `ticket:delete` | — | admin | admin only | admin only | + org admin/owner；+ ancestor owner |
| 回复 | `POST /tickets/comments` | `ticket:comment` | canRead | comment = canRead | 可见即可评 | 同 2a | 同 2a |
| 内部备注 | `POST /tickets/notes` | `ticket:note` | canRead | **2a：创建人或处理人(assigned_to)，与 assigned scope 对齐**；2b 起随 scope 透明读扩大；2c 含 org admin/owner（对齐 modules「处理团队成员」= 进入该容器者） | 见 modules/ticket §2.3 | 待实现对齐 modules | 待实现对齐 modules |
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
| **2b** | **策略 B 默认**：实体子树透明读 + 本人（**`project_isolated` 标 future，不进当前范围**） | **update 仅创建人**；close 处理人/创建人；主管 assign/close（scope） |
| **2c** | 不变（D11） | 工单 **org_id** 的 admin/owner、ancestor owner 可 update/close/assign/delete |

#### 待产品确认（边界）

1. **不可见一律 404**（list 查不到、get 单条）— 已定稿。  
2. **2a assign 仅 admin** — 创建人分派需单独决策。  
3. **delete 不要求先可见** — org admin/ancestor owner 可删委托范围内工单。  
4. **`ticket:note` L3（2026-08-26 澄清）** — 2a 口径与 `assigned` scope 对齐：创建人或处理人(assigned_to) 可见/可写；2b 起随策略 B 透明读扩大到同实体子树成员；2c 含 org admin/owner（对齐 modules「处理团队成员」= 进入该容器者）。实现前在 `02-authz-resource` 按此口径补 canOperate 规则。
5. **多 org scope 合并取 max** — 与实体透明读 **OR** 合并（读取并集）。  
6. **策略 B 为部门内默认** — `ticket_visibility=entity_transparent_read`；**`project_isolated` 强隔离标 future（见 §5.2.1），2b-core 只交付默认 `entity_transparent_read`，不阻塞主线**。
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

> 不可见工单对外统一 **90001**（决策收口：不再与 10004 二选一，10004 保留给通用资源不存在场景；**已于 2a Step 2 写入 `errcode.go`，勿改号**）。

---

## 8. 测试用例

### 2a（Step 3 验收）

| # | 用例 | 预期 | 2a 状态 & 落点 |
|---|------|------|----------------|
| T1 | 创建工单 | 200；org_path 正确 | ✅ PASS; HTTP=`scripts/acceptance-phase2a.sh` §T1；种子=`migrations/000010_ticket.up.sql` §ticket_events.created |
| T2 | A 列表 | 仅 A 相关工单 | ✅ PASS; HTTP=`scripts/acceptance-phase2a.sh` §T2；服务真表=`internal/service/ticket/authz_resource_integration_test.go` TestTicket_R3_AssignedScopeList |
| T3 | A 读 B 工单 | 404 | ✅ PASS; HTTP=`scripts/acceptance-phase2a.sh` §T3；服务真表=TestTicket_R4_InvisibleReturns404 (errcode=90001) |
| T4 | A 更新自己的 open 工单 | 200 | ✅ PASS; HTTP=`scripts/acceptance-phase2a.sh` §T4；服务真表=TestTicket_R5_UpdateOwn |
| T5 | assign 给 B | admin 或 2b 主管路径 | ✅ PASS (2a 仅 admin 口径); HTTP=`scripts/acceptance-phase2a.sh` §T5 (admin bypass + ticket_events.assigned); 2b主管 scope 延期 Step 8 |
| T6 | 非法 transition（默认种子口径 assigned→closed） | 400 + 90002 | ✅ PASS; HTTP=`scripts/acceptance-phase2a.sh` §T6 (assigned→closed NOT in incident/request JSON); 服务真表=TestTicket_T6_InvalidTransitionReturns90002；**T6 表中原 open→closed 在种子里是允许的，改用 assigned→closed 命中非法转换** |
| T7 | 无 ticket:list | 403 + 70001 | ✅ PASS; HTTP=`scripts/acceptance-phase2a.sh` §T7 (viewer 角色 Casbin L1 GET/POST tickets 都 403)；service 层对齐 R8 |

**测试落点约定**（B4）：状态机转换单测 → `internal/service/ticket/state_machine_test.go`（9 用例已 PASS）；T1–T7 真表集成测试 → `internal/service/ticket/authz_resource_integration_test.go`；HTTP/路由级/权限码 → `scripts/acceptance-phase2a.sh` §B + 头段 Section A 跑 Phase 1 27 例回归（P2-D5）。

### 2b（Step 4 2b-core + Step 8 验收）

| # | 用例 | 预期 |
|---|------|------|
| T8 | scope=group 用户列表 | 含子 org 工单。✅ 已覆盖（Step 5）：`TestB2Org_ScopeSupervisorAndAll`（Get/List）+ phase2b 脚本主管分派 |
| T9 | vg_a member、策略 B | **可读** vg_b 工单；**不可 update** vg_b（403） |
| T10 | expires_at 过期成员 | 失去该 org 透明读与 scope 路径。✅ 已覆盖（Step 5）：`TestB2Org_ExpiredMemberLosesAnchor` + BFS 过期隔离 |
| T11 | 附件 confirm | 见 storage S1–S2 |
| T12 | 实体 `project_isolated`（**future，移出 2b-core 验收**） | vg_a member **不可读** vg_b（404） |
| T13 | vg_a admin 改 vg_a 内他人工单 | 2c：**200**；2b：403（非创建人） |
| T14 | vg_a member 改自己创建的 vg_a 工单 | 200。✅ 创建人半边已验（`TestB2_WriteSeparation` R12 段）|

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
