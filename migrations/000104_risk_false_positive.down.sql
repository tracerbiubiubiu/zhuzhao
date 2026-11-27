LOCK TABLE risk_suppressions IN ACCESS EXCLUSIVE MODE;
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM risk_suppressions) THEN
        RAISE EXCEPTION 'cannot remove persisted risk suppression history';
    END IF;
END
$$;

DELETE FROM casbin_rule WHERE ptype='p' AND (v1,v2) IN (
    ('/api/v1/risk-suppressions','GET'),
    ('/api/v1/risk-items/false-positive','POST'),
    ('/api/v1/risk-suppressions/remove','POST'));
DELETE FROM menu_apis WHERE (api_path,api_method) IN (
    ('/api/v1/risk-suppressions','GET'),
    ('/api/v1/risk-items/false-positive','POST'),
    ('/api/v1/risk-suppressions/remove','POST'));
DELETE FROM role_menus WHERE menu_id IN (
    SELECT id FROM menus WHERE code IN ('risk_false_positive_btn','risk_suppressions_remove_btn'));
DELETE FROM menus WHERE code IN ('risk_false_positive_btn','risk_suppressions_remove_btn');
DROP TABLE risk_suppressions;
