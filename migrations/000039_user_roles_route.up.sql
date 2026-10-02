-- P2-5（复检遗留）：GET /api/v1/users/:id/roles 页面读绑定——用户页「分配角色」
-- 对话框回显反查端点（替代「工号精确+角色过滤」N+1 推导）。
-- B 案口径：读 API 挂页面行（system_user）——写端点 POST /users/roles 已挂
-- system_user_assign_role 按钮（000031）。
INSERT INTO menu_apis (menu_id, api_path, api_method)
SELECT m.id, '/api/v1/users/:id/roles', 'GET'
FROM menus m
WHERE m.code = 'system_user' AND m.deleted_at IS NULL
ON CONFLICT DO NOTHING;
