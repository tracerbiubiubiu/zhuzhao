-- 000013 down: 移除 2c 组织委托
DROP INDEX IF EXISTS idx_user_orgs_org_role;
ALTER TABLE user_orgs DROP COLUMN IF EXISTS org_member_role;
ALTER TABLE organizations DROP COLUMN IF EXISTS owner_user_ids;

-- 移除 2c 新路由 menu_apis
DELETE FROM menu_apis
WHERE (api_path, api_method) IN (('/api/v1/orgs/owners', 'POST'), ('/api/v1/orgs/members/role', 'POST'));
