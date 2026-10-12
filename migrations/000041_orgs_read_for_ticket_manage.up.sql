-- 000041: GET /api/v1/orgs（全组织树读）放开给绑定「工单管理」菜单的角色（operator/viewer 等）
--
-- 背景：2026-10-12 多账号前端实测——operator/viewer 的发起工单组织字段（S8「组织双源」）
-- 与类型配置页组织下拉依赖组织树，但 GET /orgs 此前仅绑 system_org（组织管理，admin 专属）
-- → 非 admin 一律 403（前端已静默降级为空 + toast 噪音）。组织树名称属低敏感读，且为
-- 发起工单的业务必需；组织管理页的写操作（POST /orgs 系列、成员、move）不动，仍 admin 专属。
-- Casbin 策略持久化于 casbin_rule（角色绑菜单 UI 操作时生成）——迁移须直接补规则：
-- 为绑定「工单管理」菜单的角色各补一条 GET /orgs（role::<code>，与 ListMenuAPIsByMenuIDs
-- 生成的形态一致）；down 精确逆操作。
INSERT INTO casbin_rule (ptype, v0, v1, v2)
-- DISTINCT 承重（2026-10-12 对齐 000042）：menus.code 仅部分唯一索引，存在软删
-- 同名菜单时 JOIN 会产出同角色重复行，NOT EXISTS 不滤重，第二条触发唯一索引冲突
SELECT DISTINCT 'p', 'role::' || r.code, '/api/v1/orgs', 'GET'
FROM roles r
JOIN role_menus rm ON rm.role_id = r.id
JOIN menus m ON m.id = rm.menu_id
WHERE m.code = 'ticket_manage' AND m.menu_type = 1
  AND m.deleted_at IS NULL
  AND NOT EXISTS (
    SELECT 1 FROM casbin_rule c
    WHERE c.ptype = 'p' AND c.v0 = 'role::' || r.code
      AND c.v1 = '/api/v1/orgs' AND c.v2 = 'GET'
  );
INSERT INTO menu_apis (menu_id, api_path, api_method)
SELECT m.id, '/api/v1/orgs', 'GET'
FROM menus m
WHERE m.code = 'ticket_manage' AND m.menu_type = 1
  AND NOT EXISTS (
    SELECT 1 FROM menu_apis ma
    WHERE ma.menu_id = m.id AND ma.api_path = '/api/v1/orgs' AND ma.api_method = 'GET'
  );
