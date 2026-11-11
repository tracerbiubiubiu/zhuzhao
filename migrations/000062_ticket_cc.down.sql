-- 000062 down：对称回滚——先撤种子规则（000061 FK→message_templates），再撤种子模板，
-- 最后落表（新表无存量依赖；cascade 清理随 DROP）。
DELETE FROM message_rules WHERE biz_type = 'ticket' AND event_action = 'ticket.cc_added';
DELETE FROM message_templates WHERE code = 'ticket_cc_notify';
DROP TABLE IF EXISTS ticket_cc;
