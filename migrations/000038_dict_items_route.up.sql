-- 审计修复（2026-09-30 P2）：字典消费端点受众拍板=全员（业务表单选项场景）——
-- 路由挪 SelfService 组（/api/v1/user/dicts/:code/items，免 Casbin）。
-- 原绑定行随之清理（catalog 对账：exempt 路由+残留绑定=dead binding 拒启）。
DELETE FROM menu_apis WHERE api_path = '/api/v1/dicts/:code/items' AND api_method = 'GET';
