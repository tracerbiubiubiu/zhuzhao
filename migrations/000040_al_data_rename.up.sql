-- 000040: al_data 菜单改名「名单数据」→「活动列表」（用户拍板 2026-10-08——
-- activelist 域语义对齐：活动列表即 activelist 直译；顶级「名单管理」与「名单类型」暂不动）
UPDATE menus SET name = '活动列表' WHERE code = 'al_data' AND name = '名单数据';
