-- 000017: project_isolated 强隔离开关激活（IW1 / BK-13，2026-08-31 拍板）
-- SSOT: docs/phase2/09-ticket.md §5.2.1
-- 000011 预留的 future CHECK 本次放开（000011 注释所预告的迁移）；
-- L2 配套：resolver 锚点门控（scope_resolver.go）与 GetFilter/Authorize
-- 委托轴分支已在 2b 代码内，本迁移仅放开值域。
-- 编号说明（A2 规则：谁先启动谁占用）：000017 由 BK-13 占用，
-- 附件（原拟 000017）与 SLA（原拟 000017–000021）启动时整体顺延重排。

ALTER TABLE organizations
    ADD CONSTRAINT organizations_ticket_visibility_check
    CHECK (ticket_visibility IN ('entity_transparent_read', 'project_isolated'));
