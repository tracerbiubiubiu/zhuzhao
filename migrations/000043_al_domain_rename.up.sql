-- 000043: al 域菜单命名统一为「活动列表」系（2026-10-13 用户拍板）——
-- 顶级 名单管理→活动列表（对齐 activelist 域名直译）；子级对称去「名单」前缀：
-- 活动列表(al_data)→活动数据（避免与顶级重名）、名单类型→活动类型。
-- 前端页头/空态文案/E2E 锚点随批同步（zhuzhao-ui 同名提交）。
UPDATE menus SET name = '活动列表' WHERE code = 'al_manage' AND name = '名单管理';
UPDATE menus SET name = '活动数据' WHERE code = 'al_data' AND name = '活动列表';
UPDATE menus SET name = '活动类型' WHERE code = 'al_types' AND name = '名单类型';
