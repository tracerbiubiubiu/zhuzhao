-- 000012 down: 移除 2b-org 组织增强（逆序：roles.parent_id → org_roles → user_orgs 列 → users/org 来源列）
ALTER TABLE roles DROP COLUMN IF EXISTS parent_id;
DROP TABLE IF EXISTS org_roles;
ALTER TABLE user_orgs DROP COLUMN IF EXISTS expires_at;
ALTER TABLE user_orgs DROP COLUMN IF EXISTS source;
ALTER TABLE user_orgs DROP COLUMN IF EXISTS ticket_scope;
ALTER TABLE users DROP COLUMN IF EXISTS synced_at;
ALTER TABLE users DROP COLUMN IF EXISTS external_id;
ALTER TABLE users DROP COLUMN IF EXISTS source;
DROP INDEX IF EXISTS uq_org_source_external;
ALTER TABLE organizations DROP COLUMN IF EXISTS synced_at;
ALTER TABLE organizations DROP COLUMN IF EXISTS external_id;
ALTER TABLE organizations DROP COLUMN IF EXISTS source;
