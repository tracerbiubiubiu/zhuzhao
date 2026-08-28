-- 000012: 2b-org 组织增强（M2b-org，Step 5）
-- SSOT: docs/proposal/hr-directory-sync.md §2（来源列）/ docs/phase2/03-org-enhance.md /
--       docs/modules/ticket.md §2b（ticket_scope）/ docs/design/rbac-inheritance-and-cascade.md §4（BFS）
-- 原编号 000011（组织增强）按不跳号规则顺延至本号——ticket_visibility 已前移至 000011（Step 4）。

-- === 组织来源扩展（hr-directory-sync §2；HR Sync Job 为 2b-ext，schema 同批落地） ===
ALTER TABLE organizations ADD COLUMN IF NOT EXISTS source VARCHAR(20) NOT NULL DEFAULT 'local';
  -- hr | local | system（种子根 system，管理端 local，HR Job hr）
ALTER TABLE organizations ADD COLUMN IF NOT EXISTS external_id VARCHAR(100);
  -- HR 部门 ID；source=hr 时必填（对账键，2b-ext 启用）
ALTER TABLE organizations ADD COLUMN IF NOT EXISTS synced_at TIMESTAMPTZ;
CREATE UNIQUE INDEX IF NOT EXISTS uq_org_source_external
    ON organizations (source, external_id)
    WHERE deleted_at IS NULL AND external_id IS NOT NULL;

-- === 用户来源扩展（同批；employee_no/domain_account 已在 Phase 1） ===
ALTER TABLE users ADD COLUMN IF NOT EXISTS source VARCHAR(20) NOT NULL DEFAULT 'local';
ALTER TABLE users ADD COLUMN IF NOT EXISTS external_id VARCHAR(100);
ALTER TABLE users ADD COLUMN IF NOT EXISTS synced_at TIMESTAMPTZ;

-- === user_orgs：工单 scope / 来源 / 临时成员（03-org-enhance 预期功能） ===
ALTER TABLE user_orgs ADD COLUMN IF NOT EXISTS ticket_scope VARCHAR(20) NOT NULL DEFAULT 'assigned';
  -- assigned | group | all（09-ticket §5.2 Effective scope，多 org 取 max）
ALTER TABLE user_orgs ADD COLUMN IF NOT EXISTS source VARCHAR(20) NOT NULL DEFAULT 'local';
  -- hr | local（HR Job 只写 hr 主部门，见 D10）
ALTER TABLE user_orgs ADD COLUMN IF NOT EXISTS expires_at TIMESTAMPTZ;
  -- 虚拟组临时成员；读取侧过滤（resolver / 角色展开），不做删除 Job

-- === org_roles：BFS 源 2（modules/organization §2 / auth-design） ===
CREATE TABLE IF NOT EXISTS org_roles (
    org_id     BIGINT NOT NULL REFERENCES organizations(id),
    role_id    BIGINT NOT NULL REFERENCES roles(id),
    created_at TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY (org_id, role_id)
);
-- 语义：仅用户「直接所属」组织节点生效，不沿部门 ltree 继承（rbac-inheritance §4 速查用例）

-- === roles.parent_id：BFS 源 3 角色继承链（rbac-inheritance §4） ===
ALTER TABLE roles ADD COLUMN IF NOT EXISTS parent_id BIGINT REFERENCES roles(id);
  -- 展开方向：child → parent（沿链向上取并集）；管理端维护入口随 2c 角色管理补齐
