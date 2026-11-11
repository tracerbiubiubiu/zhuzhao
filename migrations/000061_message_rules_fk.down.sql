-- 000061 down: 删 FK（无数据语义）
ALTER TABLE message_rules DROP CONSTRAINT IF EXISTS fk_message_rules_template_code;
