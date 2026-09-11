-- 000027: 工单类型/字段/模板三表乐观锁列（P1-3 并发缺陷修复）
-- SSOT: 并发审查 P1-3——两管理员并发编辑同一类型/模板时后写静默覆盖先写（lost update）。
-- 三表各加 version 列（NOT NULL DEFAULT 1），供
--   UpdateTicketType / UpdateTicketTemplate / ReplaceTypeFields 做 CAS 乐观锁：
--     无 version 谓词 = 保持旧的 patch 语义；携 version = 命中才写，0 行 → 409。
-- 说明：ticket_type_fields.version 亦随本迁移加上，但 ReplaceTypeFields 的整体
-- 替换语义以「父类型 ticket_types.version」作为唯一权威 CAS 谓词——字段集是类型
-- 配置的一部分，字段行本身无自然单行版本语义（每次整体清空重插），故其 version 列
-- 仅保持三表结构一致、暂不作并发谓词（详见 ticket_repo.go ReplaceTypeFields 注释）。
ALTER TABLE ticket_types        ADD COLUMN IF NOT EXISTS version INT NOT NULL DEFAULT 1;
ALTER TABLE ticket_type_fields  ADD COLUMN IF NOT EXISTS version INT NOT NULL DEFAULT 1;
ALTER TABLE ticket_templates    ADD COLUMN IF NOT EXISTS version INT NOT NULL DEFAULT 1;
