DO $$
BEGIN
    LOCK TABLE notification_review_notices, notification_deliveries IN ACCESS EXCLUSIVE MODE;
    IF EXISTS (SELECT 1 FROM notification_review_notices) OR EXISTS (SELECT 1 FROM notification_deliveries) THEN
        RAISE EXCEPTION 'Durable notifications exist; disable mock sending and retain the receipt schema';
    END IF;
END
$$;

DELETE FROM casbin_rule
WHERE ptype='p' AND v1='/api/v1/message-templates/preview' AND v2='POST';
DELETE FROM menu_apis
WHERE api_path='/api/v1/message-templates/preview' AND api_method='POST';
DELETE FROM message_templates t
WHERE t.code='ticket_review_http_mock' AND t.review_http_mock_seed AND t.version=1
  AND NOT EXISTS (SELECT 1 FROM message_rules r WHERE r.template_code=t.code);
ALTER TABLE message_templates DROP COLUMN review_http_mock_seed;
DROP TABLE notification_review_notices;
DROP TABLE notification_provider_events;
DROP TABLE notification_delivery_attempts;
DROP TABLE notification_deliveries;
DROP TABLE notification_http_profiles;
