-- 000079: notify 动作默认路由种子（S3a B3-3 欠账收口——2026-10-26 通知面收口批）
-- 翻案注记：S3a 交付时曾把「notify 动作无种子规则、默认静默」登记为接受偏差（11 号 2026-10-24 S3a 行）。
-- 2026-10-26 所有者拍板补默认路由：管理员显式在转换上配了 notify 动作而全程静默是坑不是克制——
-- 补种子模板+规则后开箱即用；要静默删规则即可（规则=开关）。
-- 同批代码改动：NotifyAction 的 template 参数透传（payload.template_code 覆盖规则默认模板）。
INSERT INTO message_templates (code, name, channel, biz_type, subject, body, created_by) VALUES
    ('ticket_action_notify', '工单动作提醒', 'uip', 'ticket',
     '[{{ticket.human_key}}] {{ticket.title}}',
     '工单流程提醒：{{ticket.title}}（{{ticket.human_key}}）\n链接：{{ticket.link}}',
     NULL)
ON CONFLICT (code) DO NOTHING;

-- recipient_type=cc：消费动作 params.recipients → 事件 cc_users
INSERT INTO message_rules (biz_type, event_action, template_code, recipient_type) VALUES
    ('ticket', 'ticket.action_notify', 'ticket_action_notify', 'cc')
ON CONFLICT (biz_type, event_action) DO NOTHING;
