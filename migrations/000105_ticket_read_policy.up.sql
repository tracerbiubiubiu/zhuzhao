-- Keep all stored types on their current scope, including locally configured types.
ALTER TABLE ticket_types
    ADD COLUMN read_policy text NOT NULL DEFAULT 'inherit'
        CHECK (read_policy IN ('inherit','participants_only','role_restricted')),
    ADD COLUMN read_role_codes text[] NOT NULL DEFAULT '{}',
    ADD CHECK ((read_policy='role_restricted') = (cardinality(read_role_codes)>0));
ALTER TABLE ticket_types ALTER COLUMN read_policy SET DEFAULT 'participants_only';

INSERT INTO menus(parent_id,code,name,menu_type,permission,sort_order,is_system)
SELECT id,'ticket_stats_btn','查看工单统计',3,'ticket:stats',18,true
FROM menus WHERE code='ticket_list' AND deleted_at IS NULL;
INSERT INTO menu_apis(menu_id,api_path,api_method)
SELECT id,'/api/v1/tickets/stats','GET' FROM menus WHERE code='ticket_stats_btn' AND deleted_at IS NULL;
INSERT INTO role_menus(role_id,menu_id)
SELECT r.id,m.id FROM roles r CROSS JOIN menus m WHERE r.code IN ('admin','superadmin')
  AND m.code='ticket_stats_btn' AND r.deleted_at IS NULL AND m.deleted_at IS NULL;
INSERT INTO casbin_rule(ptype,v0,v1,v2)
SELECT 'p','role::'||r.code,a.api_path,a.api_method FROM roles r JOIN role_menus rm ON r.id=rm.role_id
JOIN menu_apis a ON a.menu_id=rm.menu_id WHERE a.api_path='/api/v1/tickets/stats' AND a.api_method='GET'
ON CONFLICT DO NOTHING;
