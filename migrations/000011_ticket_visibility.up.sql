-- 000011: 2b-core 工单可见性（organizations.ticket_visibility，仅默认策略）
-- SSOT: docs/phase2/09-ticket.md §5.2.1
-- 2026-08-26 宽松优先（P2-D6/P2-D7）：project_isolated 标 future，
-- 2b-core 仅落地默认 entity_transparent_read（单字段 + 默认约束，CHECK 暂不含
-- project_isolated，避免 GetFilter 提前分支）；强隔离待真实需求再加。
-- 实际执行于 Step 4（M2b-core）——原规划归并 000011 于 Step 5，按 PRD SSOT 前移；
-- Step 5（2b-org）其余 DDL 顺延为 000012（README §2.4 不跳号规则）。

ALTER TABLE organizations
    ADD COLUMN IF NOT EXISTS ticket_visibility VARCHAR(30) NOT NULL DEFAULT 'entity_transparent_read';

-- 仅实体 org_type IN (1,2,3) 语义有效；虚拟组（2b-org 引入）继承最近实体祖先配置（查询侧实现）
-- future: 需求出现后再加 CHECK (ticket_visibility IN ('entity_transparent_read','project_isolated'))
--         及 GetFilter 隔离分支
