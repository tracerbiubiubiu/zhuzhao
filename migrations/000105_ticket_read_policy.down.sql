LOCK TABLE ticket_types IN ACCESS EXCLUSIVE MODE;
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM ticket_types WHERE read_policy <> 'inherit') THEN
        RAISE EXCEPTION 'cannot remove configured ticket read restrictions';
    END IF;
END
$$;
DELETE FROM casbin_rule WHERE ptype='p' AND v1='/api/v1/tickets/stats' AND v2='GET';
DELETE FROM menu_apis WHERE api_path='/api/v1/tickets/stats' AND api_method='GET';
DELETE FROM role_menus WHERE menu_id IN (SELECT id FROM menus WHERE code='ticket_stats_btn');
DELETE FROM menus WHERE code='ticket_stats_btn';
ALTER TABLE ticket_types DROP COLUMN read_role_codes, DROP COLUMN read_policy;
