CREATE TABLE risk_suppressions (
    id BIGSERIAL PRIMARY KEY,
    family_id BIGINT NOT NULL REFERENCES risk_families(id),
    risk_id BIGINT NOT NULL UNIQUE REFERENCES risk_items(id),
    valid_until TIMESTAMPTZ NOT NULL,
    reason TEXT NOT NULL CHECK (btrim(reason) <> ''),
    created_by BIGINT NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    ended_at TIMESTAMPTZ,
    ended_by BIGINT REFERENCES users(id),
    ended_reason TEXT,
    revision BIGINT NOT NULL DEFAULT 1,
    CHECK (valid_until > created_at),
    CHECK (
        (ended_at IS NULL AND ended_by IS NULL AND ended_reason IS NULL AND revision = 1)
        OR (ended_at IS NOT NULL AND ended_at >= created_at AND ended_at < valid_until AND ended_by IS NOT NULL
            AND ended_reason IS NOT NULL AND btrim(ended_reason) <> '' AND revision = 2)
    )
);
CREATE INDEX idx_risk_suppressions_family ON risk_suppressions (family_id, id DESC);

INSERT INTO menus (parent_id,code,name,menu_type,permission,sort_order,is_system)
SELECT p.id,b.code,b.name,3,'risk:manage',b.sort_order,true
FROM menus p CROSS JOIN (VALUES ('risk_false_positive_btn','关闭误报',14),
                                ('risk_suppressions_remove_btn','终止误报抑制',15)) b(code,name,sort_order)
WHERE p.code='risk_list';

INSERT INTO role_menus (role_id,menu_id)
SELECT r.id,m.id FROM roles r CROSS JOIN menus m
WHERE r.deleted_at IS NULL AND r.code IN ('admin','superadmin')
  AND m.code IN ('risk_false_positive_btn','risk_suppressions_remove_btn');

INSERT INTO menu_apis (menu_id,api_path,api_method)
SELECT m.id,a.api_path,a.api_method
FROM menus m JOIN (VALUES ('risk_list','/api/v1/risk-suppressions','GET'),
                          ('risk_false_positive_btn','/api/v1/risk-items/false-positive','POST'),
                          ('risk_suppressions_remove_btn','/api/v1/risk-suppressions/remove','POST')) a(code,api_path,api_method)
  ON m.code=a.code;

INSERT INTO casbin_rule (ptype,v0,v1,v2)
SELECT DISTINCT 'p','role::' || r.code,a.api_path,a.api_method
FROM roles r JOIN role_menus rm ON rm.role_id=r.id JOIN menu_apis a ON a.menu_id=rm.menu_id
WHERE r.deleted_at IS NULL
  AND a.api_path IN ('/api/v1/risk-suppressions','/api/v1/risk-items/false-positive','/api/v1/risk-suppressions/remove')
ON CONFLICT DO NOTHING;
