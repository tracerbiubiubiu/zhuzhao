-- 000061: message_rules.template_code 补 FK → message_templates(code)
-- （v3 审计 Y-3——修复前无 FK：READ COMMITTED 下删除模板与并发新建规则可产生悬挂引用，
--   删除守卫的 NOT EXISTS 语句级原子 ≠ 事务级无窗口；FK 收口后 DELETE 自然拦悬挂插入）
-- 前置：需先清存量悬挂行（如有）再 ADD CONSTRAINT——此处防御性 DELETE 兜底（正常零行）。
DELETE FROM message_rules mr
WHERE NOT EXISTS (SELECT 1 FROM message_templates mt WHERE mt.code = mr.template_code);

ALTER TABLE message_rules
    ADD CONSTRAINT fk_message_rules_template_code
    FOREIGN KEY (template_code) REFERENCES message_templates(code);
