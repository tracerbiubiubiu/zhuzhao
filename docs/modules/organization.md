# 组织模块设计

> 模块代码：`internal/service/org_service.go` + `internal/repository/org_repo.go`
>
> 旧系统参考：`doc/module-assessment-2026-08/organization.md`
>
> 主键以 [phase1/06-organization.md](../phase1/06-organization.md) 为准（`BIGINT` id + `VARCHAR code`），下文 UUID schema 过时。

---

## 1. 模块定位

**核心底座配套模块**。组织管理是 RBAC 的组织维度，支持组织层级（树）、成员管理、组织-角色映射。组织是资源级鉴权的重要维度。

与其他模块的关系：
- 为 `role` 提供组织角色（BFS 三源之一）
- 为 `authz` 提供组织关系判断（ltree 查询）
- 自注册 `OrgResource` 到 ResourceRegistry

---

## 2. 数据模型

### 2.1 实体组织（ltree 树形）

```sql
CREATE EXTENSION IF NOT EXISTS ltree;

CREATE TABLE organizations (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    code        VARCHAR(50) UNIQUE NOT NULL,
    name        VARCHAR(100) NOT NULL,
    parent_id   UUID REFERENCES organizations(id),
    path        LTREE NOT NULL,           -- 如 root.tech.be
    org_type    SMALLINT NOT NULL,        -- 1=公司 2=部门 3=小组
    status      SMALLINT DEFAULT 1,
    is_system   BOOLEAN DEFAULT FALSE,
    created_by  VARCHAR(50),
    created_at  TIMESTAMPTZ DEFAULT NOW(),
    updated_at  TIMESTAMPTZ DEFAULT NOW(),
    deleted_at  TIMESTAMPTZ
);

CREATE INDEX idx_org_path ON organizations USING GIST(path);
CREATE INDEX idx_org_parent ON organizations(parent_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_org_code ON organizations(code) WHERE deleted_at IS NULL;
```

### 2.2 虚拟组（独立表）

```sql
CREATE TABLE virtual_groups (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    code        VARCHAR(50) UNIQUE NOT NULL,
    name        VARCHAR(100) NOT NULL,
    description TEXT,
    status      SMALLINT DEFAULT 1,
    created_by  VARCHAR(50),
    created_at  TIMESTAMPTZ DEFAULT NOW(),
    updated_at  TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE virtual_group_members (
    group_id    UUID REFERENCES virtual_groups(id),
    user_id     UUID REFERENCES users(id),
    role_id     UUID REFERENCES roles(id),
    PRIMARY KEY (group_id, user_id, role_id)
);
```

### 2.3 组织角色绑定

```sql
CREATE TABLE org_roles (
    org_id      UUID REFERENCES organizations(id),
    role_id     UUID REFERENCES roles(id),
    PRIMARY KEY (org_id, role_id)
);
```

### 2.4 组织管理员

```sql
-- 组织管理员直接在 organizations 表中
ALTER TABLE organizations ADD COLUMN owner_ids UUID[] DEFAULT '{}';
```

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
    GetTree(ctx context.Context) ([]*OrgNode, error)
    MoveNode(ctx context.Context, code string, newParentCode string) error

    // 成员管理
    AddMember(ctx context.Context, orgCode string, userID string, roleID string) error
    RemoveMember(ctx context.Context, orgCode string, userID string) error
    GetMembers(ctx context.Context, orgCode string) ([]*model.User, error)
    GetUserOrgs(ctx context.Context, userID string) ([]*model.Organization, error)

    // 组织角色
    AssignRoles(ctx context.Context, orgCode string, roleIDs []string) error
    GetRoles(ctx context.Context, orgCode string) ([]*model.Role, error)

    // 层级查询
    GetDescendants(ctx context.Context, code string) ([]*model.Organization, error)
    GetAncestors(ctx context.Context, code string) ([]*model.Organization, error)
    IsInSubtree(ctx context.Context, userOrgCode string, targetOrgCode string) (bool, error)

    // 虚拟组
    CreateVirtualGroup(ctx context.Context, req CreateVirtualGroupRequest) (*model.VirtualGroup, error)
    AddVirtualGroupMember(ctx context.Context, groupCode string, userID string, roleID string) error
}
```

---

## 4. 核心流程

### 4.1 创建组织（ltree path 维护）

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

```
POST /api/v1/orgs/:code/move {newParentCode}

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

### 4.3 删除组织（级联，借鉴旧系统跨集合事务）

```
DELETE /api/v1/orgs/:code

1. 系统组织保护
   → org.is_system == true？返回 403

2. 获取子树
   → SELECT * FROM organizations WHERE path <@ (SELECT path FROM organizations WHERE code = ?)

3. 事务开始
   → BEGIN

4. 删除子树的所有成员关系
   → DELETE FROM user_orgs WHERE org_id IN (SELECT id FROM organizations WHERE path <@ ?)

5. 删除子树的组织角色绑定
   → DELETE FROM org_roles WHERE org_id IN (...)

6. 软删除子树所有组织
   → UPDATE organizations SET deleted_at = NOW() WHERE path <@ ?

7. 事务提交

8. 事务外副作用
   → 失效权限缓存（所有受影响用户）
   → Casbin 策略清理（组织角色相关）
```

### 4.4 组织关系查询（ltree）

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

-- 判断用户是否是目标组织祖先链中某组织的管理员
SELECT EXISTS(
  SELECT 1 FROM user_orgs uo
  JOIN organizations o ON uo.org_id = o.id
  WHERE uo.user_id = $1
  AND $2::ltree @> o.path  -- 目标组织是用户组织的后代
  AND uo.role_id IN (SELECT id FROM roles WHERE code = 'org_admin')
);
```

---

## 5. 旧系统借鉴

| 设计 | 决策 | 理由 |
|------|------|------|
| 物理组织 + 虚拟组分离 | ✅ 直接采用 | 语义不同 |
| 组织层级关系 | ✅ 改用 ltree | 旧系统用 organization_relation_mapping（多对多，支持多父级）+ BFS 遍历，新框架用 ltree 更高效 |
| 组织角色自动继承 | ✅ 直接采用 | 成员加入组织自动获得组织角色 |
| 组织管理员 owner_ids | ✅ 直接采用 | 用于资源级鉴权的 org_admin 判断 |
| 环检测 | ✅ 直接采用 | 移动节点前检查 |
| 乐观锁（updated_at） | ✅ 直接采用 | 并发修改保护 |
| 多父级支持 | ❌ 不支持 | ltree 是树形，大多数企业组织是树形。如需多父级，Phase 3 改用闭包表 |
| hierarchy 通用工具 | ❌ 不需要 | ltree 原生支持层级查询 |
| sequencer 生成 code | ❌ 改用人工/规则 `code` + `BIGINT` id | `code` 须匹配 ltree `[A-Za-z0-9_]`，不用 UUID |

---

## 6. 分阶段实施

### Phase 1

- 组织 CRUD（含 ltree path 维护）
- 组织树查询 + 移动节点（事务内更新子树 path）
- 系统组织保护
- 基础成员管理（AddMember/RemoveMember/GetMembers）
- `code` 校验：仅 `[A-Za-z0-9_]`（ltree 标签）

### Phase 2

- 组织角色绑定
- 组织管理员（owner_ids）
- 虚拟组 CRUD + 成员管理 + 临时成员有效期
- 组织关系查询（IsInSubtree 等，供 authz 调用）

### Phase 3

- 多父级支持（闭包表，如有需求）
- 组织变更事件广播（Redis Pub/Sub）
