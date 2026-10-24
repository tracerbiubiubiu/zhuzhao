-- S9 B9-1 情报缓存表（09 号附录 A.1 全文级）：
-- 缓存键 = intel_type + 归一化 query_value（domain 小写等——归一化规则按类型注册）；
-- result = 上游响应原样 JSONB（ACL 防腐层：外部模型透传不渗入域模型）；
-- 过期判定在查询层 WHERE expires_at < NOW()（原 partial index 谓词含 NOW()
-- 非法——P0-3 修正，普通索引）；懒过期回源 + 周期物理清理骑 CronScheduler。
CREATE TABLE IF NOT EXISTS intel_cache (
    id          BIGSERIAL PRIMARY KEY,
    intel_type  VARCHAR(30) NOT NULL,        -- ip / domain / hash / url（config 枚举白名单）
    query_value VARCHAR(200) NOT NULL,       -- 归一化后查询值
    result      JSONB NOT NULL,              -- 上游响应原样
    fetched_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at  TIMESTAMPTZ NOT NULL,        -- fetched_at + TTL(intel_type)
    hit_count   INT NOT NULL DEFAULT 0,
    UNIQUE (intel_type, query_value)
);
CREATE INDEX IF NOT EXISTS idx_intel_cache_expiry ON intel_cache (expires_at);

-- S9 A.4：查询端点权限码 intel:query（本段唯一新码）——菜单挂在审计域
-- （威胁情报查询属安全运营读面；menu_type=2 页面行挂 audit_log 同级）。
INSERT INTO menus (code, name, parent_id, menu_type, path, component, icon, permission, sort_order, is_system)
VALUES ('intel_query', '情报查询', NULL, 2, '/intel', 'intel/query/index', 'intel', 'intel:query', 6, true)
ON CONFLICT DO NOTHING;

INSERT INTO menu_apis (menu_id, api_path, api_method)
SELECT m.id, '/api/v1/intel/query', 'GET'
FROM menus m
WHERE m.code = 'intel_query' AND m.deleted_at IS NULL
AND NOT EXISTS (SELECT 1 FROM menu_apis ma WHERE ma.menu_id = m.id AND ma.api_path = '/api/v1/intel/query' AND ma.api_method = 'GET');
