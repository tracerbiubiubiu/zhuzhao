-- B6-3 增强批：种子模板只读标识（A.8.1 语义——is_system=true 管理员不可改/删）
ALTER TABLE message_templates ADD COLUMN IF NOT EXISTS is_system BOOLEAN NOT NULL DEFAULT false;
UPDATE message_templates SET is_system = true WHERE code IN ('ticket_created_notify', 'ticket_assigned_notify', 'ticket_closed_notify');
