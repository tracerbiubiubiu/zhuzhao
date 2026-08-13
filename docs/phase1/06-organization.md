# 06 - 组织模块（organization）

> Step 9，依赖 Step 5（authz）。Phase 1 实现基础组织树 CRUD。

---

## 预期功能

| 功能 | 场景 | API |
|------|------|-----|
| 组织树 | 查看完整组织树（树形结构） | `GET /api/v1/orgs` |
| 创建组织 | 在指定父组织下创建子组织 | `POST /api/v1/orgs` |
| 组织详情 | 查看单个组织信息 | `GET /api/v1/orgs/:id` |
| 更新组织 | 修改组织名称、描述 | `POST /api/v1/orgs/:id/update` |
| 删除组织 | 删除组织（需检查是否有子组织和成员） | `POST /api/v1/orgs/:id/delete` |
| 移动组织 | 将组织移动到新的父组织下 | `POST /api/v1/orgs/:id/move` |
| 用户-组织关联 | 查看用户所属组织 | `GET /api/v1/users/:id/orgs` |

### Phase 1 不做

| 功能 | 原因 | 阶段 |
|------|------|------|
| 虚拟组（org_type=4） | Phase 1 只做实体组织（org_type 1-3） | Phase 2 |
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

CREATE INDEX idx_org_path   ON organizations USING GIST(path);
CREATE INDEX idx_org_parent ON organizations(parent_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_org_code   ON organizations(code) WHERE deleted_at IS NULL;
CREATE INDEX idx_org_deleted ON organizations(deleted_at) WHERE deleted_at IS NOT NULL;
```

> **path 用 code 而非 ID**：code 是业务可读的编码，path = `root.tech_dept.fe_team` 比 `1.5.12` 更可读，且 ltree 标签只允许字母/数字/下划线，code 需遵守此约束（不能用 `-`）。

### 路径维护

> 详见 [modules/organization.md](../modules/organization.md) §4.1。创建组织时，路径 = 父组织路径 + '.' + 自身 code：

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

详见 [modules/organization.md](../modules/organization.md) §4.3：

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

### Service 层

| 用例 | 输入 | 预期 |
|------|------|------|
| 创建组织 - 正常 | name + parentID | 返回组织 |
| 创建组织 - 父组织不存在 | 不存在的 parentID | 返回 ErrOrgNotFound |
| 移动组织 - 移到自己的子节点下 | orgID = 1, newParentID = 3（3 是 1 的子节点） | 返回 ErrInvalidMove |
| 删除组织 - 有子组织 | 有子节点的 orgID | 返回 ErrOrgHasChildren |

---

## 涉及文件

```
internal/repository/org_repo.go       # 组织数据访问（ltree 操作）
internal/service/org_service.go       # 组织业务逻辑
internal/handler/org_handler.go       # HTTP Handler
internal/model/org.go                 # 组织模型
```
