-- 000040 回滚：恢复「名单数据」
UPDATE menus SET name = '名单数据' WHERE code = 'al_data' AND name = '活动列表';
