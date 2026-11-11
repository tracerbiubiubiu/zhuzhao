-- 000080 down: 删本批种子（模板+规则按 code 精确删——不影响其他种子行）
DELETE FROM message_rules WHERE biz_type = 'ops' AND event_action = 'ops.storage_usage_high';
DELETE FROM message_templates WHERE code = 'ops_storage_usage_alert';
