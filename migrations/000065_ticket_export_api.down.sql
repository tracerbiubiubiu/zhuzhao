-- 000065 down：来源维度精确回滚（仅本迁移种的行）。
DELETE FROM menu_apis WHERE api_path = '/api/v1/tickets/export' AND api_method = 'GET';
