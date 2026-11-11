-- 000076: 消息中心自服务化（G-23 实施——E2E 补齐批三程）
-- 问题：铃铛对所有登录用户渲染，而 /messages 路由由菜单驱动、仅绑定角色可达（000074 后=admin/superadmin），
--       operator/viewer 点铃铛即 404；inbox 三端点又是严格自数据（服务端限 recipient=self、非本人 404），
--       与 /badges（selfService）同性质却挂 biz 组 L1，read/read-all 两 POST 还挂在页面行（G-26 违例）。
-- 处置：①后端 inbox/read/read-all 移 selfService 组（router + catalogExempt 同批）；②前端 /messages 加常量路由
--       （ROUTE_EXEMPT_PATHS 同款 /profile 口径），入口=铃铛（000072 原设计）；③本迁移把菜单行惰性化。
-- 菜单行**不删只 visible=false**：用户菜单树过滤不可见节点 → 不生成动态路由/侧栏项（避免与常量路由同 path
-- 双注册）；不删行是 000048 惯例（删行致自增 ID 漂移触往返门禁）。管理端 GetTree 仍含该行供运维。
UPDATE menus SET visible = false WHERE code = 'message_center' AND deleted_at IS NULL;

DELETE FROM menu_apis
WHERE menu_id = (SELECT id FROM menus WHERE code = 'message_center')
  AND api_path IN ('/api/v1/notifications/inbox', '/api/v1/notifications/read', '/api/v1/notifications/read-all');

DELETE FROM role_menus
WHERE menu_id = (SELECT id FROM menus WHERE code = 'message_center');

-- 非管理角色残留 casbin（移 selfService 后无 L1 意义；admin/superadmin 走 matcher 通配）
DELETE FROM casbin_rule
WHERE ptype = 'p' AND v0 NOT IN ('role::admin', 'role::superadmin')
  AND (v1, v2) IN (
    ('/api/v1/notifications/inbox', 'GET'),
    ('/api/v1/notifications/read', 'POST'),
    ('/api/v1/notifications/read-all', 'POST'));
