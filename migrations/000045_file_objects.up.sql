-- 000045: file_objects 对象元数据表（phase5 S5 B5-1，05 号 A.3 全文级）
--
-- 对象本体存 RustFS（S3 兼容），本表只存元数据。相对 10 号蓝本的三处实施级修正：
-- ① 显式 purpose 列（avatar/ticket_attachment/ticket_richtext 及建单前 *_create 态）
--   ——purpose↔key 前缀一致性校验的查询锚点；
-- ② presign 即写 pending（蓝本原设计 confirm 才写库——孤儿对象 DB 无记录、GC 无从
--   发现），confirm 翻 confirmed；GC 扫 status=pending AND created_at < 阈值；
-- ③ etag 列（2026-10-19 拍板 1b）：confirm 时 StatObject 回填快照——代理下载
--   S3 If-Match 比对（≠快照→404+告警，防 presign TTL 内重放替换已 confirm 对象）
--   + 客户端 If-None-Match 304 数据源（一列两用）。
-- 关联表 ticket_attachments 随 B5-3 批（000046）；richtext 内嵌图不建关联表
--（引用存在于 MD 源文本，生命周期与 GC 对账扫描见 05 号 A.3 注）。
CREATE TABLE file_objects (
    id           BIGSERIAL PRIMARY KEY,
    bucket       VARCHAR(100) NOT NULL,
    object_key   VARCHAR(500) NOT NULL,          -- {domain}/{yyyy}/{mm}/{uuid}.{ext}，服务端生成
    purpose      VARCHAR(30)  NOT NULL,          -- avatar / ticket_attachment / ticket_richtext / *_create（建单前悬挂态）
    status       VARCHAR(20)  NOT NULL DEFAULT 'pending',
                                                          -- pending(已预签未确认) / confirmed / deleted
    file_name    VARCHAR(255) NOT NULL,          -- 原始文件名（仅回显；下载头 RFC 5987 编码防注入）
    mime_type    VARCHAR(100) NOT NULL,          -- presign 钉入签名的类型（防谎报锚点）
    size_bytes   BIGINT,                         -- confirm HEAD 实测回填（声明值预检存 presign 阶段）
    etag         VARCHAR(128),                   -- confirm 时 StatObject 回填快照（修正③，拍板 1b）
    created_by   BIGINT NOT NULL REFERENCES users(id),
    created_at   TIMESTAMPTZ DEFAULT NOW(),
    confirmed_at TIMESTAMPTZ,
    UNIQUE (bucket, object_key)
);
-- 孤儿 GC 扫描锚点：只摸 pending 行（部分索引）
CREATE INDEX idx_file_objects_gc ON file_objects (status, created_at) WHERE status = 'pending';
-- 建单前绑定落绑校验的查询面（created_by+status 复合，A.5 建单前链）
CREATE INDEX idx_file_objects_creator ON file_objects (created_by, status);
