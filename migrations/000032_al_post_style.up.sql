-- P4-W5 前置批：activelist 三端点 zhuzhao 风格整改同步（02 号穿插池 2026-09-22 拍板：
-- 整改不豁免——改 W5 动工前，前端 al_types 页尚未开发零返工）。
-- 上游 activelist 路由已改静态动词段（标识入 body）：
--   POST /al/api/v1/admin/types/:typeName/deprecate → POST /al/api/v1/admin/types/deprecate
--   POST /al/api/v1/admin/types/:typeName/schema    → POST /al/api/v1/admin/types/schema
--   POST /al/api/v1/data/:typeName/:id/restore      → POST /al/api/v1/data/restore
-- UPDATE 一律按 (api_path, api_method) 复合条件定位（同 path 跨 method 迁移红线）；
-- 三条新 path 均无 GET 孪生，无跨页提权面。/al 前缀路由对账属 activelist 侧职责
-- （catalog 跨仓豁免 v1），本迁移是 zhuzhao 侧唯一同步件。
UPDATE menu_apis SET api_path = '/al/api/v1/admin/types/deprecate'
WHERE api_path = '/al/api/v1/admin/types/:typeName/deprecate' AND api_method = 'POST';

UPDATE menu_apis SET api_path = '/al/api/v1/admin/types/schema'
WHERE api_path = '/al/api/v1/admin/types/:typeName/schema' AND api_method = 'POST';

UPDATE menu_apis SET api_path = '/al/api/v1/data/restore'
WHERE api_path = '/al/api/v1/data/:typeName/:id/restore' AND api_method = 'POST';
