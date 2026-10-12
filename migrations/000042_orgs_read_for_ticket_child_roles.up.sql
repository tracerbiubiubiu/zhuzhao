-- 000042: 000041 的绑定面修正——「工单管理」**子菜单**（类型配置/工单列表等）的绑定角色
-- 同样授予 GET /api/v1/orgs。背景：000041 的 casbin 发放仅扫 ticket_manage 目录本体的
-- 绑定角色，漏掉只绑子菜单的角色（实测 viewer 只绑「类型配置」→ 放开后仍 403，2026-10-12）。
-- 组织管理写操作仍不放开（同 000041 口径）。
INSERT INTO casbin_rule (ptype, v0, v1, v2)
SELECT DISTINCT 'p', 'role::' || r.code, '/api/v1/orgs', 'GET'
FROM roles r
JOIN role_menus rm ON rm.role_id = r.id
JOIN menus m ON m.id = rm.menu_id
WHERE m.parent_id = (SELECT id FROM menus WHERE code = 'ticket_manage' AND menu_type = 1)
  AND NOT EXISTS (
    SELECT 1 FROM casbin_rule c
    WHERE c.ptype = 'p' AND c.v0 = 'role::' || r.code
      AND c.v1 = '/api/v1/orgs' AND c.v2 = 'GET'
  );
