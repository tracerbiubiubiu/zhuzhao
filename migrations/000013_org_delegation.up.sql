-- 000013: 2c 组织委托（M2c-1，Step 8）
-- SSOT: docs/phase2/04-org-delegation.md §2.1

-- 组织负责人（可多人；Phase 2c 用数组，后续可迁 org_owners 关联表）
ALTER TABLE organizations
    ADD COLUMN IF NOT EXISTS owner_user_ids BIGINT[] NOT NULL DEFAULT '{}';

-- 组内级别（与全局 user_roles 分离；member=20 < admin=10 < owner=1，数字越小权限越高）
ALTER TABLE user_orgs
    ADD COLUMN IF NOT EXISTS org_member_role VARCHAR(20) NOT NULL DEFAULT 'member';
-- CHECK 暂不启用：BIGINT[] 双轨对齐由 service 层保证（SetOwners 同步 org_member_role='owner'）

CREATE INDEX IF NOT EXISTS idx_user_orgs_org_role ON user_orgs (org_id, org_member_role)
    WHERE org_member_role IN ('admin', 'owner');

-- 2c 新路由的 menu_apis（04 §3.1.1：L1 复用 system_org 菜单，不新增权限码；
-- effective owner/admin 细粒度校验下沉 service 层 L3）
INSERT INTO menu_apis (menu_id, api_path, api_method)
SELECT m.id, v.api_path, v.api_method
FROM (VALUES
    ('/api/v1/orgs/owners',        'POST'),
    ('/api/v1/orgs/members/role',  'POST')
) AS v(api_path, api_method)
JOIN menus m ON m.code = 'system_org'
ON CONFLICT DO NOTHING;
