-- 000080: 存储用量越限告警种子（C-6——2026-10-27 观测批，A-2 拍板 a 案落地件）
-- 事件源=storage_usage_sample cron（容量阈值档位跃迁 emit，80% warn/90% critical；
-- capacity_bytes=0 时零事件——只暴露用量绝对值）。种子形态同 000064 ops.cron_failed
-- 惯例：recipient_type=fixed 且收件人空=暂不发送，管理员在消息管理面配置后生效（规则=开关）。
INSERT INTO message_templates (code, name, channel, biz_type, subject, body, created_by) VALUES
    ('ops_storage_usage_alert', '存储用量越限告警', 'uip', 'ops',
     '[ops] 存储用量 {{event.level}}：{{event.ratio}}',
     '对象存储用量越限（{{event.level}}）：{{event.usage_bytes}} / {{event.capacity_bytes}} 字节（{{event.ratio}}）\n按 purpose 用量见 /metrics zhuzhao_storage_usage_bytes',
     NULL)
ON CONFLICT (code) DO NOTHING;

INSERT INTO message_rules (biz_type, event_action, template_code, recipient_type, fixed_recipients) VALUES
    ('ops', 'ops.storage_usage_high', 'ops_storage_usage_alert', 'fixed', '')
ON CONFLICT (biz_type, event_action) DO NOTHING;
