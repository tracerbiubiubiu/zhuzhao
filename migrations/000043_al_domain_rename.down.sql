-- 000043 回滚：恢复「名单管理 / 活动列表 / 名单类型」原命名
UPDATE menus SET name = '名单管理' WHERE code = 'al_manage' AND name = '活动列表';
UPDATE menus SET name = '活动列表' WHERE code = 'al_data' AND name = '活动数据';
UPDATE menus SET name = '名单类型' WHERE code = 'al_types' AND name = '活动类型';
