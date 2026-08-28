-- 000011 down: 移除 2b-core 工单可见性列
ALTER TABLE organizations DROP COLUMN IF EXISTS ticket_visibility;
