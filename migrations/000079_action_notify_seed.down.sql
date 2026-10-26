-- 000079 down: 撤种子模板与规则（若管理员已把规则改绑其他模板，仅撤本种子模板行——规则行
-- 的 template_code 指向已删模板会走 dispatcher not-found 拒发并 Warn，与撤除语义一致）。
DELETE FROM message_rules WHERE biz_type='ticket' AND event_action='ticket.action_notify'
  AND template_code='ticket_action_notify';
DELETE FROM message_templates WHERE code='ticket_action_notify';
