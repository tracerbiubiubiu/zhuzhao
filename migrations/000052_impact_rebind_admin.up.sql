-- 000052: B1-7 批2 impact 端点归位管理组（01 号 §3.4 拍板）——路由自 SelfService
-- 标记组移挂 ticketMeta（biz/Casbin）后，BK-22 路由↔menu_apis 对账要求绑定。
-- 绑定菜单 = ticket_type_manage（「能改字段即可看影响」——与 fields/replace 同菜单）。
-- 授权可重建（000042 先例）：先补 menu_apis 来源行（AssignMenus 按其全删重建
-- casbin），再对存量绑定直插 casbin（存量不会自动重放）。
INSERT INTO menu_apis (menu_id, api_path, api_method)
SELECT m.id, '/api/v1/ticket-types/:code/fields/impact', 'GET'
FROM menus m
WHERE m.code = 'ticket_type_manage'
  AND m.deleted_at IS NULL
  AND NOT EXISTS (
    SELECT 1 FROM menu_apis ma
    WHERE ma.menu_id = m.id
      AND ma.api_path = '/api/v1/ticket-types/:code/fields/impact'
      AND ma.api_method = 'GET'
  );

INSERT INTO casbin_rule (ptype, v0, v1, v2)
SELECT DISTINCT 'p', 'role::' || r.code, '/api/v1/ticket-types/:code/fields/impact', 'GET'
FROM roles r
JOIN role_menus rm ON rm.role_id = r.id
JOIN menus m ON m.id = rm.menu_id
WHERE m.code = 'ticket_type_manage'
  AND m.deleted_at IS NULL
  AND r.deleted_at IS NULL AND r.status = 1
  AND NOT EXISTS (
    SELECT 1 FROM casbin_rule c
    WHERE c.ptype = 'p' AND c.v0 = 'role::' || r.code
      AND c.v1 = '/api/v1/ticket-types/:code/fields/impact' AND c.v2 = 'GET'
  );
