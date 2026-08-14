# 06 - 组织模块（organization）

> Step 9，依赖 Step 5（authz）。Phase 1 实现基础组织树 CRUD。

---

## 预期功能

| 功能 | 场景 | API |
|------|------|-----|
| 组织树 | 查看完整组织树（树形结构） | `GET /api/v1/orgs` |
| 创建组织 | 在指定父组织下创建子组织 | `POST /api/v1/orgs` |
| 组织详情 | 查看单个组织信息 | `GET /api/v1/orgs/:id` |
| 更新组织 | 修改组织名称、描述 | `POST /api/v1/orgs/update` |
| 删除组织 | 删除组织（需检查是否有子组织和成员） | `POST /api/v1/orgs/delete` |
| 移动组织 | 将组织移动到新的父组织下 | `POST /api/v1/orgs/move` |
| 用户-组织关联 | 查看用户所属组织 | `GET /api/v1/users/:id/orgs` |
| 组织成员列表 | 查看某组织下成员 | `GET /api/v1/orgs/:id/members` |
| 添加组织成员 | 将用户加入组织 | `POST /api/v1/orgs/members` |
| 移除组织成员 | 将用户从组织移出 | `POST /api/v1/orgs/members/delete` |

> Phase 1 `user_orgs` 仅 `(user_id, org_id, is_primary)`，**无** `role_id`。  
> **双 HTTP 入口、单写逻辑**：组织侧 `POST /orgs/members*` 与用户侧 `POST /users/orgs`、创建用户 `org_ids` 均委托 **同一 `OrgService`**，见下文 §成员关系写路径。

### 成员关系写路径（SSOT）

```
OrgService
  ├── AddMember(orgID, userID, isPrimary)      ← POST /orgs/members
  ├── RemoveMember(orgID, userID)              ← POST /orgs/members/delete
  └── SetUserOrgs(userID, orgIDs, primaryOrgID) ← POST /users/orgs、POST /users（org_ids）
```

| 入口 | 适用 UI | 语义 |
|------|---------|------|
| `POST /orgs/members` | 组织树 · 成员页 | 添加**一名**成员（已存在则幂等） |
| `POST /orgs/members/delete` | 组织树 · 成员页 | 移除**一名**成员 |
| `POST /users/orgs` | 用户详情 · 所属组织 | **全量覆盖**该用户的组织列表 |
| `POST /users` + `org_ids` | 新建用户表单 | 创建成功后同事务 `SetUserOrgs` |

用户模块 `GET /users/:id/orgs` 为只读查询；**禁止**在 UserRepo 内复制 `is_primary` 等规则。

### 成员 API 请求体

```json
// POST /api/v1/orgs/members — 添加一名成员
{ "org_id": "2", "user_id": "5", "is_primary": false }

// POST /api/v1/orgs/members/delete
{ "org_id": "2", "user_id": "5" }

// POST /api/v1/users/orgs — 全量设置用户所属组织（见 04-user.md）
{ "user_id": "5", "org_ids": ["2", "3"], "primary_org_id": "2" }
```

**`is_primary` 规则**（Phase 1）：

- 可选，默认 `false`
- 同一用户最多一条 `is_primary = true`；设新 primary 时，事务内清除该用户其它记录的 primary 标记
- 用户可不属于任何组织，也可属于多个组织但只有一个 primary

### Phase 1 不做

| 功能 | 原因 | 阶段 |
|------|------|------|
| 虚拟组（org_type=4） | Phase 1 只做实体组织（org_type 1-3） | Phase 2b |
| 组织级权限（scope） | 资源级鉴权 Phase 2 | Phase 2 |
| 组织角色 | Phase 2 | Phase 2 |
| ltree 路径查询 | 无业务资源需要过滤 | Phase 2 |

---

## 核心设计思路

### Organization 结构体

```go
type Organization struct {
    ID          int64      `json:"id,string" db:"id"`
    Code        string     `json:"code" db:"code"`           // 业务编码，ltree path 用
    Name        string     `json:"name" db:"name"`
    Description string     `json:"description" db:"description"`
    ParentID    *int64     `json:"parent_id,string" db:"parent_id"`
    Path        string     `json:"path" db:"path"`            // ltree: root.tech.fe
    OrgType     int        `json:"org_type" db:"org_type"`    // 1=公司 2=部门 3=小组
    Status      int        `json:"status" db:"status"`        // 1=启用 0=禁用
    IsSystem    bool       `json:"is_system" db:"is_system"`
    SortOrder   int        `json:"sort_order" db:"sort_order"`
    CreatedBy   *int64     `json:"created_by,string" db:"created_by"`
    TenantID    int64      `json:"tenant_id,string" db:"tenant_id"`
    Version     int        `json:"version" db:"version"`
    DeletedAt   *time.Time `json:"deleted_at,omitempty" db:"deleted_at"`
    CreatedAt   time.Time  `json:"created_at" db:"created_at"`
    UpdatedAt   time.Time  `json:"updated_at" db:"updated_at"`
}
```

### ltree 组织树

> 详见 [modules/organization.md](../modules/organization.md) §2。组织用 `code` 作为业务标识，ltree path 用 code 拼接（如 `root.tech.fe`）。

```sql
CREATE EXTENSION IF NOT EXISTS ltree;

CREATE TABLE organizations (
    id          BIGSERIAL PRIMARY KEY,
    code        VARCHAR(50) NOT NULL,            -- 业务编码，如 "tech_dept"
    name        VARCHAR(100) NOT NULL,
    description TEXT,
    parent_id   BIGINT REFERENCES organizations(id),
    path        LTREE NOT NULL,                 -- 如 'root.tech_dept.fe_team'（用 code 拼接）
    org_type    SMALLINT NOT NULL,              -- 1=公司 2=部门 3=小组 4=虚拟组
    status      SMALLINT NOT NULL DEFAULT 1,    -- 1=启用 0=禁用
    is_system   BOOLEAN DEFAULT FALSE,
    sort_order  INT DEFAULT 0,
    created_by  BIGINT,
    tenant_id   BIGINT NOT NULL DEFAULT 1,
    version     INT DEFAULT 1,
    deleted_at  TIMESTAMPTZ,
    created_at  TIMESTAMPTZ DEFAULT NOW(),
    updated_at  TIMESTAMPTZ DEFAULT NOW()
);

CREATE UNIQUE INDEX idx_org_code ON organizations(code) WHERE deleted_at IS NULL;
```

> **种子幂等**：`000002_seed` 使用 `ON CONFLICT (code) WHERE deleted_at IS NULL DO NOTHING`（PG 15+，见 [data-init.md](../proposal/data-init.md)）。

```sql
CREATE INDEX idx_org_path   ON organizations USING GIST(path);
CREATE INDEX idx_org_parent ON organizations(parent_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_org_deleted ON organizations(deleted_at) WHERE deleted_at IS NOT NULL;
```

> **path 用 code 而非 ID**：code 是业务可读的编码，path = `root.tech_dept.fe_team` 比 `1.5.12` 更可读。ltree 标签只允许 `[A-Za-z0-9_]`，**创建/更新 code 时必须校验**，拒绝 `-` 和 `.`。

### 路径维护

> 详见 [modules/organization.md](../modules/organization.md) §4.1。创建组织时，路径 = 父组织路径 + '.' + 自身 code：

```go
func (s *OrgService) Create(ctx context.Context, req CreateOrgRequest) (*model.Organization, error) {
    var parentID *int64
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
        parentID = &parent.ID
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

移动组织（更新子树 path），详见 [modules/organization.md](../modules/organization.md) §4.2：

Phase 1 用 DB 事务 + 行锁，Phase 3 加分布式锁。

```sql
-- 移动组织到新父节点下（事务内）
-- 1. 行锁锁定当前组织及其所有子节点
SELECT id FROM organizations WHERE path <@ (SELECT path FROM organizations WHERE code = $1) FOR UPDATE;

-- 2. 环检测：目标父节点不能是当前节点的子树
SELECT NOT (SELECT path FROM organizations WHERE code = $2) <@ (SELECT path FROM organizations WHERE code = $1);

-- 3. 更新当前节点及所有子节点 path（ltree 子树更新）
UPDATE organizations
SET path = text2ltree($3) || subpath(path, nlevel((SELECT path FROM organizations WHERE code = $1)) - 1)
WHERE path <@ (SELECT path FROM organizations WHERE code = $1);
```

### 删除保护

详见 [modules/organization.md](../modules/organization.md) §4.4：

```go
func (s *orgService) Delete(ctx context.Context, code string) error {
    // 1. 系统组织保护
    org, _ := s.repo.GetByCode(ctx, code)
    if org.IsSystem {
        return ErrOrgIsSystem  // "系统内置组织不可删除"
    }

    // 2. 检查子组织
    children, _ := s.repo.CountChildren(ctx, code)
    if children > 0 {
        return ErrOrgHasChildren  // "该组织下有子组织，无法删除"
    }

    // 3. 检查成员
    members, _ := s.repo.CountMembers(ctx, code)
    if members > 0 {
        return ErrOrgHasMembers  // "该组织下有成员，无法删除"
    }

    return s.repo.Delete(ctx, code)  // 软删除
}
```

> **Phase 2b**：若子节点含 `org_type=4` 虚拟组，HR 撤销走 Reparent（见 [hr-directory-sync.md](../proposal/hr-directory-sync.md)）；管理端 Delete 仍默认拒绝有子节点。HR Sync 不得硬删其下仍有虚拟组的 HR 实体。
```

---

## 测试用例

### Repository 层

| 用例 | 验证点 |
|------|--------|
| 创建根组织 | path = '{code}'，如 'tech_dept' |
| 创建子组织 | path = 'parent_code.code'，如 'root.tech_dept' |
| 创建三级组织 | path = 'root.tech_dept.fe_team' |
| 查询组织树 | 返回树形结构，层级正确 |
| 移动组织 | 自身及子组织 path 全部更新 |
| 删除组织 - 系统组织 | 返回 ErrOrgIsSystem |
| 删除组织 - 有子组织 | 返回 ErrOrgHasChildren |
| 删除组织 - 有成员 | 返回 ErrOrgHasMembers |
| 删除组织 - 叶子节点无成员 | 成功（软删除） |
| 添加成员 | org_id + user_id | user_orgs 插入成功 |
| 添加成员 - 重复 | 同一 org_id + user_id | ON CONFLICT 幂等或返回已存在 |
| 移除成员 | org_id + user_id | user_orgs 删除成功 |
| 设 is_primary | is_primary=true | 该用户其它 primary 被清除 |
| SetUserOrgs | user_id + org_ids | user_orgs 全量替换 |
| SetUserOrgs - 清空 | org_ids: [] | 该用户无组织关联 |

### Service 层

| 用例 | 输入 | 预期 |
|------|------|------|
| 创建组织 - 正常 | name + parentID | 返回组织 |
| 创建组织 - 父组织不存在 | 不存在的 parentID | 返回 ErrOrgNotFound |
| 创建组织 - code 含 `-` | code=`tech-dept` | 返回 ErrInvalidParams |
| 移动组织 - 移到自己的子节点下 | orgID = 1, newParentID = 3（3 是 1 的子节点） | 返回 ErrInvalidMove |
| 删除组织 - 有子组织 | 有子节点的 orgID | 返回 ErrOrgHasChildren |
| 添加成员 - 用户不存在 | 无效 user_id | 返回 ErrUserNotFound |
| 添加成员 - 组织不存在 | 无效 org_id | 返回 ErrOrgNotFound |
| SetUserOrgs - primary 不在 org_ids 中 | primary_org_id 无效 | 返回 ErrInvalidParams |
| 移除成员 - 未加入 | 无对应 user_orgs 行 | 404 + 50007 |

---

## 涉及文件

> 目标目录见 [architecture §3.5](../design/architecture.md#35-领域模块目录约定单仓可拆分)。成员 API 见上文 §预期功能。

```
internal/repository/org/                # ltree + user_orgs 成员
internal/service/org/
internal/handler/org/
internal/model/org.go                 # 或 model/organization.go
```
