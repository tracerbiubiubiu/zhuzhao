-- 000042: 000041 的绑定面修正——「工单管理」**子页面**（工单列表/类型配置）的绑定角色
-- 同样授予 GET /api/v1/orgs。背景：000041 的 casbin 发放仅扫 ticket_manage 目录本体的
-- 绑定角色，漏掉只绑子页面的角色（实测 viewer 只绑「类型配置」→ 放开后仍 403，2026-10-12）。
-- 组织管理写操作仍不放开（同 000041 口径）。
--
-- 授权必须有可重建的来源（2026-10-12 复核批 C 缺陷，首版只插 casbin 已证不耐久）：
-- AssignMenus 按「绑定菜单的 menu_apis」全删重建该角色 casbin 规则（role_repo.AssignMenus），
-- 启动期亦无 role_menus→casbin 重建——仅插 casbin 的补授在下一次 UI 保存菜单时即被抹掉。
-- 故 up 先补 menu_apis 行（授权有来源、可被 AssignMenus 重建、down 可精确逆），
-- 再对存量绑定直接补插 casbin（存量不会自动重放）。
INSERT INTO menu_apis (menu_id, api_path, api_method)
SELECT m.id, '/api/v1/orgs', 'GET'
FROM menus m
WHERE m.code IN ('ticket_type_manage', 'ticket_list')
  AND m.deleted_at IS NULL
  AND NOT EXISTS (
    SELECT 1 FROM menu_apis ma
    WHERE ma.menu_id = m.id AND ma.api_path = '/api/v1/orgs' AND ma.api_method = 'GET'
  );

INSERT INTO casbin_rule (ptype, v0, v1, v2)
SELECT DISTINCT 'p', 'role::' || r.code, '/api/v1/orgs', 'GET'
FROM roles r
JOIN role_menus rm ON rm.role_id = r.id
JOIN menus m ON m.id = rm.menu_id
WHERE m.parent_id = (SELECT id FROM menus WHERE code = 'ticket_manage' AND menu_type = 1)
  AND m.deleted_at IS NULL
  AND NOT EXISTS (
    SELECT 1 FROM casbin_rule c
    WHERE c.ptype = 'p' AND c.v0 = 'role::' || r.code
      AND c.v1 = '/api/v1/orgs' AND c.v2 = 'GET'
  );
