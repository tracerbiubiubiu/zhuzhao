# 组织模块设计

> 模块代码（目标路径，Phase 2 迁入）：`internal/service/org/` + `internal/repository/org/` + `internal/handler/org/`
> **Phase 1 现状（D2-43）**：扁平结构——`internal/service/org_service.go` / `internal/repository/org_repo.go` / `internal/handler/org_handler.go`
>
> 旧系统参考：`doc/module-assessment-2026-08/organization.md`
>
> 主键以 [phase1/06-organization.md](../phase1/06-organization.md) 为准（`BIGINT` id + `VARCHAR code`）。

---

## 1. 模块定位

**核心底座配套模块**。组织管理是 RBAC 的组织维度，支持组织层级（树）、成员管理、组织-角色映射。组织是资源级鉴权的重要维度。

与其他模块的关系：
- 为 `role` 提供组织角色（BFS 三源之一）
- 为 `authz` 提供组织关系判断（ltree 查询）
- Phase 2 起自注册 `OrgResource` 到 ResourceRegistry

### 1.1 Phase 1 设计决策：组织为共享资源（D2-37①）

> 决策日期：2026-08-24（[review 03 号报告 D2-37](../review/03-second-deep-review-findings.md) 上线前决策点裁决，选方案①文档化）

Phase 1 组织写路径（AddMember / RemoveMember / Move / Update / Delete）**仅做存在性校验 + 路由级 RBAC（L1）防护，不做资源级属主/组织关系校验（L3）**——与用户侧 `SetUserOrgs` 的 `ensureCanManage` 不对称，属有意取舍而非疏漏：

| 项 | 说明 |
|----|------|
| 暴露面 | 任何持有 org 写路由权限的主体可操作**全部**组织。种子默认态仅 superadmin/admin 持有；但菜单管理页正常使用即可给自定义角色勾选 org 写按钮——第一个自定义角色形成即暴露面形成 |
| 为什么可接受 | Phase 1 组织无权限语义（org_roles 未引入），组织写操作不构成提权通道，属数据完整性缺口而非提权（review 03 D2-37 复核结论） |
| 终局保护 | Phase 2c Step 9 防提权矩阵：AddMember/RemoveMember 扩展目标校验 + org_roles 语义 + 组内级别校验（[phase2/04-org-delegation](../phase2/04-org-delegation.md)） |
| 配套修正 | operator/viewer 种子角色描述修正（原描述暗示的权限绑定并不存在，见迁移 000002/000009） |

**Phase 1 部署约束**：在 2c Step 9 落地前，org 写权限（`org:create/update/delete/move/member` 菜单）只应授予等同管理员的受信角色，不应下放给业务自定义角色。

---

## 2. 数据模型

### 2.1 实体组织（ltree 树形，Phase 1）

> 完整 DDL 见 [phase1/06-organization.md](../phase1/06-organization.md)。

```sql
CREATE EXTENSION IF NOT EXISTS ltree;

CREATE TABLE organizations (
    id          BIGSERIAL PRIMARY KEY,
    code        VARCHAR(50) NOT NULL,
    name        VARCHAR(100) NOT NULL,
    description TEXT,
    parent_id   BIGINT REFERENCES organizations(id),
    path        LTREE NOT NULL,
    org_type    SMALLINT NOT NULL,
    status      SMALLINT NOT NULL DEFAULT 1,
    is_system   BOOLEAN DEFAULT FALSE,
    sort_order  INT DEFAULT 0,
    tenant_id   BIGINT NOT NULL DEFAULT 1,
    version     INT DEFAULT 1,
    deleted_at  TIMESTAMPTZ,
    created_at  TIMESTAMPTZ DEFAULT NOW(),
    updated_at  TIMESTAMPTZ DEFAULT NOW()
);

CREATE UNIQUE INDEX idx_org_code ON organizations(code) WHERE deleted_at IS NULL;
CREATE INDEX idx_org_path ON organizations USING GIST(path);
CREATE INDEX idx_org_parent ON organizations(parent_id) WHERE deleted_at IS NULL;
```

```sql
-- Phase 1 用户-组织（无组织内 role_id）
CREATE TABLE user_orgs (
    user_id     BIGINT NOT NULL REFERENCES users(id),
    org_id      BIGINT NOT NULL REFERENCES organizations(id),
    is_primary  BOOLEAN DEFAULT FALSE,
    joined_at   TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY (user_id, org_id)
);
```

### 2.2 虚拟组与组织角色（Phase 2b）

Phase 1 **不建**独立虚拟组表；`org_type=4` 虚拟组与 `org_roles`、`org_permissions` 在 Phase 2b 引入。挂载与 HR 同步规则见 [hr-directory-sync.md](../proposal/hr-directory-sync.md) §2–4。设计草案（ID 均为 `BIGINT`）：

```sql
-- Phase 2b：虚拟组统一在 organizations（org_type=4），见 hr-directory-sync.md
CREATE TABLE org_roles (
    org_id      BIGINT NOT NULL REFERENCES organizations(id),
    role_id     BIGINT NOT NULL REFERENCES roles(id),
    PRIMARY KEY (org_id, role_id)
);
```

### 2.3 组织负责人与组内分级（Phase 2c）

组织负责人（`owner_user_ids`）与组内分级（`user_orgs.org_member_role`）在 **Phase 2c** 交付，依赖 2b 虚拟组 + scope；Phase 1 不做。实现 SSOT：[phase2/04-org-delegation.md](../phase2/04-org-delegation.md)。

### 2.4 HR 目录同步与虚拟组挂载（Phase 2b）

> **完整方案**：[proposal/hr-directory-sync.md](../proposal/hr-directory-sync.md)。Phase 2b 实施见 [phase2/03-org-enhance.md](../phase2/03-org-enhance.md)。

虚拟组 **挂在实体组织下**（`parent_id` → hr 实体节点），因此：

| HR 变化 | 对虚拟组的影响 | 策略 |
|---------|----------------|------|
| 实体部门 **移动** | 虚拟组 `path` **随子树级联更新** | 预期行为（与 Move 相同 SQL） |
| 实体部门 **撤销** | 不可硬删父节点 | **Reparent** 虚拟组到最近 HR 祖先 |
| HR **全量对账** | 不得删除 `source=local` 节点 | HR Job 只 reconcile `source=hr` |

**字段扩展（Phase 2b 迁移）**：

```sql
-- organizations / users（Phase 2b 新增）
source       VARCHAR(20) NOT NULL DEFAULT 'local',  -- hr | local | system
external_id  VARCHAR(100),
synced_at    TIMESTAMPTZ

-- user_orgs（Phase 1 已有 is_primary；Phase 2b 新增以下字段）
source       VARCHAR(20) NOT NULL DEFAULT 'local',  -- hr 主部门 vs local 虚拟组
expires_at   TIMESTAMPTZ
```

**创建虚拟组约束**：

- `org_type = 4`，`source = 'local'`
- 父节点须为 `source IN ('hr','system')` 且 `org_type IN (1,2,3)`
- 建议 `code` 使用 `vg_` 前缀（与 HR 部门 code 区分）

**权限**：虚拟组成员与 scope 绑在虚拟组自身的 `user_orgs`（`source=local`），**不**继承父实体部门的 HR 角色；父部门撤销 **不**自动清空虚拟组成员。

---

## 3. 接口定义

```go
type OrgService interface {
    // CRUD
    Create(ctx context.Context, req CreateOrgRequest) (*model.Organization, error)
    GetByCode(ctx context.Context, code string) (*model.Organization, error)
    Update(ctx context.Context, code string, req UpdateOrgRequest) error
    Delete(ctx context.Context, code string) error
    List(ctx context.Context, query OrgListQuery) ([]*model.Organization, int64, error)

    // 树操作
    // GetTree 返回树形结构：model.Organization 递归嵌套 Children（db:"-"，平铺数据由 OrgRepo 提供，
    // OrgService 按 parent_id 归并组装；与 MenuService.GetTree 同构）。Phase 1 实现：
    // internal/service/org_service.go buildOrgTree。
    GetTree(ctx context.Context) ([]*model.Organization, error)
    MoveNode(ctx context.Context, code string, newParentCode string) error

    // 成员管理（Phase 1：无 role_id；写逻辑 SSOT 在此）
    AddMember(ctx context.Context, orgID int64, userID int64, isPrimary bool) error
    RemoveMember(ctx context.Context, orgID int64, userID int64) error
    SetUserOrgs(ctx context.Context, userID int64, orgIDs []int64, primaryOrgID *int64) error
    SetUserOrgsTx(ctx context.Context, tx pgx.Tx, userID int64, orgIDs []int64, primaryOrgID *int64) error
    GetMembers(ctx context.Context, orgID int64, query MemberListQuery) ([]*model.User, int64, error)
    // GetUserOrgs 返回用户组织关系（含 is_primary/joined_at）——B4-5 修订：
    // 实际返回 []*model.UserOrg（is_primary 是该接口核心信息），原设计签名
    // []*model.Organization 有误；handler 包装为 {"orgs": [...]}
    GetUserOrgs(ctx context.Context, userID int64) ([]*model.UserOrg, error)

    // 组织角色（Phase 2b）
    AssignRoles(ctx context.Context, orgCode string, roleIDs []int64) error
    GetRoles(ctx context.Context, orgCode string) ([]*model.Role, error)

    // 层级查询
    GetDescendants(ctx context.Context, code string) ([]*model.Organization, error)
    GetAncestors(ctx context.Context, code string) ([]*model.Organization, error)
    IsInSubtree(ctx context.Context, userOrgCode string, targetOrgCode string) (bool, error)

    // 虚拟组（Phase 2b；统一 organizations 表 org_type=4）
    CreateVirtualGroup(ctx context.Context, req CreateVirtualGroupRequest) (*model.Organization, error)
    AddVirtualGroupMember(ctx context.Context, groupCode string, userID int64, roleID *int64) error
    ReparentLocalVirtualGroup(ctx context.Context, groupCode, newParentCode string) error
}
```

---

## 4. 核心流程

### 4.1 创建组织（ltree path 维护）

> **D2-44④ 层级上限**：父节点 path 段数 + 1 > 20 时前置拒绝（400）——
> 超深子树拼 path 时 ltree 报错为 500，前置校验转业务参数错误。

```go
func (s *OrgService) Create(ctx context.Context, req CreateOrgRequest) (*model.Organization, error) {
    var path ltree.Ltree

    if req.ParentCode == "" {
        // 根组织
        path = ltree.Ltree(req.Code)
    } else {
        // 子组织：parent.path + . + code
        parent, err := s.repo.GetByCode(ctx, req.ParentCode)
        if err != nil {
            return nil, err
        }
        // D2-44④：strings.Count(parent.Path, ".") + 2 > maxOrgPathDepth(20) → 400
        path = ltree.Ltree(parent.Path.String() + "." + req.Code)
    }

    org := &model.Organization{
        Code:     req.Code,
        Name:     req.Name,
        ParentID: parentID,
        Path:     path,
        OrgType:  req.OrgType,
    }
    return s.repo.Create(ctx, org)
}
```

### 4.2 移动节点（更新子树 path）

> **Phase 1 实现状态（B3-2）**：`internal/repository/org_repo.go` Move 已事务化——
> `pg_advisory_xact_lock(hashtext('org:move'))` 全局串行化并发移动（替换下述步骤 3 的
> Redis 分布式锁，Phase 3 多实例时再切换）→ 事务内 `FOR UPDATE` 重读节点与新父行 →
> 事务内环检测（`parentPath <@ oldPath` 前缀判断）→ 锁旧子树（谓词含 deleted_at）→
> 级联 UPDATE 带 **RowsAffected 校验**（0 行 = 节点被并发移动，409 而非静默成功）。
> 并发交叉移动由集成测试 `TestOrgRepo_MoveConcurrentCrossMove` 守护。

```
POST /api/v1/orgs/move {id, newParentId}

1. 获取当前节点和目标父节点
2. 环检测：目标父节点不能是当前节点的子树
   → SELECT path @> target_path（如果 target 是 current 的后代，拒绝）
3. 分布式锁（防并发移动）
   → Redis SETNX lock:org:move:{code}
4. 事务开始
   → BEGIN
5. 计算新 path
   → newPath = targetParent.path + . + current.code
6. 更新当前节点 path
   → UPDATE organizations SET path = newPath WHERE id = current.id
7. 更新所有子节点 path（ltree 子树更新）
   → UPDATE organizations
     SET path = newText || subpath(path, nlevel(oldPath) - 1)
     WHERE path <@ oldPath
8. 事务提交
9. 释放锁
10. 失效组织缓存
```

### 4.3 成员管理

> **双 HTTP 入口、单写逻辑**（Keycloak / Azure AD 常见模式）：组织页与用户页都能操作，均调用本节的 `OrgService` 方法，禁止 UserRepo 重复实现规则。

```
OrgService（SSOT）
  AddMember       ← POST /orgs/members
  RemoveMember    ← POST /orgs/members/delete
  SetUserOrgs     ← POST /users/orgs、POST /users（org_ids，经 SetUserOrgsTx）
```

#### AddMember / RemoveMember

> **实现（B3-1，`internal/repository/org_repo.go`）**：幂等不降级——重复添加已存在
> 成员且未传 is_primary（零值 false）时 primary 保持原状；仅显式 primary=true 才提升
> （同事务先清该用户其它 primary）。并发双 primary 由 000008 部分唯一索引兜底
>（`idx_user_orgs_single_primary`，触发时 409 + 50011——D2-15：AddMember 路径
> 已同型映射，原仅 SetUserOrgsTx 覆盖）。

```
POST /api/v1/orgs/members
Body: { org_id, user_id, is_primary? }

1. 校验 org、user 存在且未软删
2. 若 is_primary=true → 同事务 UPDATE user_orgs SET is_primary=false WHERE user_id=?
3. INSERT ... ON CONFLICT (user_id, org_id) DO UPDATE
   SET is_primary = true WHERE EXCLUDED.is_primary
   （false 不回写——幂等不降级；true 提升）
```

```
POST /api/v1/orgs/members/delete
Body: { org_id, user_id }

1. DELETE FROM user_orgs WHERE user_id=? AND org_id=?
2. 无行受影响 → 404 + `ErrNotOrgMember`（50007）
```

#### SetUserOrgs（全量覆盖，与 SetRoles 对称）

```
POST /api/v1/users/orgs
Body: { user_id, org_ids[], primary_org_id? }

1. 校验 user、各 org 存在
2. primary_org_id 须在 org_ids 内（若提供）
3. 事务：DELETE FROM user_orgs WHERE user_id=?
4. org_ids 去重（B3-4：保序去重，重复 ID 不再触发主键冲突 500）
5. INSERT 各 (user_id, org_id, is_primary)
   —— D2-16：INSERT...SELECT 过滤软删组织，命中软删 org_id → 404 + `ErrOrgNotFound`
   （原裸 INSERT 触发 FK 23503 → 500 且静默丢绑定；与 SetRolesTx B4-3 对齐）
```

创建用户时 `org_ids` 由 `UserService.Create` 在同一事务内调用 `SetUserOrgsTx`，见 [phase1/04-user.md](../phase1/04-user.md)。

```
GET /api/v1/orgs/:id/members

→ 分页列表，JOIN users，返回成员基本信息
```

### 4.4 删除组织

> **Phase 1 行为**（与 [phase1/06-organization.md](../phase1/06-organization.md) 一致）：有子组织或有成员则拒绝删除；仅允许删除无子节点、无成员的非系统组织（软删）。  
> 业界删部门/移成员/Reparent 对照见 [rbac-inheritance-and-cascade.md §2.5–2.6](../design/rbac-inheritance-and-cascade.md#25-场景-4hr-把技术中心整棵子树迁到产品中心下)。

```
POST /api/v1/orgs/delete

1. 系统组织保护 → is_system == true？返回 ErrOrgIsSystem
2. 检查子组织 → CountChildren > 0？返回 ErrOrgHasChildren
3. 检查成员   → CountMembers > 0？返回 ErrOrgHasMembers
4. 软删除该组织节点
```

**Phase 2b 扩展**（见 [hr-directory-sync.md](../proposal/hr-directory-sync.md) §3.1 / §5.2）：

- 子节点为 `org_type=4, source=local` 虚拟组时，**HR 撤销**走 Reparent，管理端删除实体走 §4.4 拒绝或先 reparent 再删（实现时二选一，默认拒绝）。
- HR Sync Job **不得**硬删其下仍有虚拟组的 HR 实体节点。

### 4.5 组织关系查询（ltree）

```sql
-- 查询子树（含自身）
SELECT * FROM organizations WHERE path <@ 'root.tech' AND deleted_at IS NULL;

-- 查询祖先链
SELECT * FROM organizations WHERE path @> 'root.tech.be' AND deleted_at IS NULL;

-- 判断用户是否在目标组织的子树中
SELECT EXISTS(
  SELECT 1 FROM user_orgs uo
  JOIN organizations o ON uo.org_id = o.id
  WHERE uo.user_id = $1
  AND o.path <@ (SELECT path FROM organizations WHERE code = $2)
);

-- Phase 2c：组内分级（user_orgs.org_member_role），非 Phase 1 role_id
-- 见 phase2/04-org-delegation.md
SELECT EXISTS(
  SELECT 1 FROM user_orgs uo
  JOIN organizations o ON uo.org_id = o.id
  WHERE uo.user_id = $1
  AND $2::ltree @> o.path
  AND uo.org_member_role IN ('owner', 'admin')  -- Phase 2c 迁移列
);
```

---

## 5. 旧系统借鉴

| 设计 | 决策 | 理由 |
|------|------|------|
| 物理组织 + 虚拟组语义分离 | ✅ 同表 `org_type` 区分（1–3 实体 / 4 虚拟组） | 语义不同，但统一 ltree 树便于 scope 与 HR 级联 |
| 组织层级关系 | ✅ 改用 ltree | 旧系统用 organization_relation_mapping（多对多，支持多父级）+ BFS 遍历，新框架用 ltree 更高效 |
| 组织赋角色（org_roles） | ✅ Phase 2b | 成员加入组织获得**该组织**绑定的角色；**不**向子组织成员继承父组织角色 |
| 组织树数据范围 | ✅ ltree + scope | 上级看下级**数据**（group/all），与 org_roles 分离 |
| 组织负责人 owner_user_ids | ✅ Phase 2c | 容器级委托；Authorize 见 04-org-delegation |
| 环检测 | ✅ 直接采用 | 移动节点前检查 |
| 乐观锁（updated_at） | ✅ 直接采用 | 并发修改保护 |
| 多父级支持 | ❌ 不支持 | ltree 是树形，大多数企业组织是树形。如需多父级，Phase 3 改用闭包表 |
| hierarchy 通用工具 | ❌ 不需要 | ltree 原生支持层级查询 |
| sequencer 生成 code | ❌ 改用人工/规则 `code` + `BIGINT` id | `code` 须匹配 ltree `[A-Za-z0-9_]`，不用 UUID |

---

## 6. 分阶段实施

### Phase 1

- 组织 CRUD（含 ltree path 维护）— **Phase 1 收尾必完成**（进 Phase 2b 前）
- 组织树查询 + 移动节点（事务内更新子树 path）
- 系统组织保护
- 基础成员管理（AddMember/RemoveMember/GetMembers；API：`POST /orgs/members`、`POST /orgs/members/delete`、`GET /orgs/:id/members`）
- `code` 校验：仅 `[A-Za-z0-9_]`（ltree 标签）

> 排期详见 [phase1/README §2.4](../phase1/README.md#24-step-79-crud-补全计划)。

### Phase 2b

- 组织角色绑定（`org_roles`）
- 虚拟组 CRUD + 成员管理 + 临时成员有效期
- **HR 目录同步** + 实体撤销时虚拟组 Reparent（见 [hr-directory-sync.md](../proposal/hr-directory-sync.md)）
- 组织关系查询（IsInSubtree 等，供 authz 调用）

### Phase 2c

- 组织负责人（`owner_user_ids` / `org_owners`）— 见 [phase2/04-org-delegation.md](../phase2/04-org-delegation.md)
- **组内分级**（`user_orgs.org_member_role`：member/admin/owner）
- 组内资源 Authorize、防越权（D1–D11）

### Phase 3

- 多父级支持（闭包表，如有需求）
- 组织变更事件广播（Redis Pub/Sub）
