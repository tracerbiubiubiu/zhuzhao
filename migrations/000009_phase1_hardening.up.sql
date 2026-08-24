-- Phase 1 加固迁移（review 03 号报告修复批次）：
-- 1. D2-37①：operator/viewer 角色描述修正（存量库）——种子中两者均无
--    role_menus/casbin 绑定，原描述（“可管理组织成员/角色/子组织”/“只读访问”）
--    暗示的权限并不存在；000002_seed 已同步修正，本条覆盖已初始化的库
-- 2. D2-10/D2-35/D2-41：写路径/过滤组合缺索引（见下方 CREATE INDEX）

UPDATE roles SET description = '系统预留角色：默认未绑定权限，需管理员显式分配后生效'
WHERE code = 'operator' AND description = '可管理组织成员/角色/子组织';

UPDATE roles SET description = '系统预留角色：默认未绑定权限，需管理员显式分配只读菜单后生效'
WHERE code = 'viewer' AND description = '只读访问';

-- D2-10：user_orgs.org_id——GetMembers COUNT+LIST / CountMembers / 删除保护检查
-- 均按 org_id 过滤（org_id 非复合 PK 前导列，原顺序扫描）
CREATE INDEX IF NOT EXISTS idx_user_orgs_org ON user_orgs(org_id);

-- D2-35：role_menus.menu_id / user_roles.role_id——菜单软删清理、按角色反查成员
CREATE INDEX IF NOT EXISTS idx_role_menus_menu ON role_menus(menu_id);
CREATE INDEX IF NOT EXISTS idx_user_roles_role ON user_roles(role_id);

-- D2-41：roles(status, deleted_at)——GetRoleCodes/GetRoles 每请求过滤该组合（B1-1 status=1）
CREATE INDEX IF NOT EXISTS idx_roles_status_deleted ON roles(status, deleted_at);

-- D2-11（Phase 1 部分）：audit_logs.created_at——时间范围过滤/默认排序/COUNT
-- （分区/清理策略仍留 Phase 2，见 review 03 §10.2）
CREATE INDEX IF NOT EXISTS idx_audit_logs_created ON audit_logs(created_at);
