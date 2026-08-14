# 10 - 文件存储（storage，Phase 2b）

> **Step 6**，依赖 Phase 2a（工单 MVP 可不带附件运行）；与 Step 7（auth-enhance）可并行。  
> 用户头像、工单附件共用同一 storage 模块，见 [phase1/04-user §头像](../phase1/04-user.md#头像与对象存储phase-1--2b)。

---

## 0. 边界

### 做什么

| 能力 | 说明 |
|------|------|
| S3 兼容客户端 | MinIO（开发）/ 云 OSS（生产） |
| 预签名上传 | 前端直传，后端不扛文件流 |
| 预签名下载 | 工单附件、头像只读访问 |
| `file_objects` 元数据 | 记录 bucket/key/owner/关联业务 |
| 工单附件关联 | `ticket_attachments` 表 + API |

### 不做

| 不做 | 阶段 |
|------|------|
| 病毒扫描 / 内容审核 | 按需 |
| 分片上传 >100MB | 按需 |
| CDN 独立域名 | 运维配置，非代码 |

---

## 1. 前置条件

- [ ] Phase 2a 工单 CRUD 可用
- [ ] Docker Compose 增加 MinIO 服务（或 config 指向已有 S3）
- [ ] Casbin 已有 `ticket:update` 等权限（附件上传走工单 update 或独立 `ticket:attach`）

---

## 2. 配置

```yaml
storage:
  provider: s3
  endpoint: http://minio:9000
  region: us-east-1
  bucket: zhuzhao
  access_key: ${MINIO_ACCESS_KEY}
  secret_key: ${MINIO_SECRET_KEY}
  use_ssl: false
  presign_upload_ttl: 15m
  presign_download_ttl: 1h
  max_upload_bytes: 10485760   # 10MB
  allowed_mime_prefixes:
    - image/
    - application/pdf
    - text/
```

**对象 key 规范**：`{domain}/{yyyy}/{mm}/{uuid}.{ext}`  
示例：`tickets/2026/08/550e8400.pdf`、`avatars/user_123.jpg`

---

## 3. 数据模型

```sql
CREATE TABLE file_objects (
    id            BIGSERIAL PRIMARY KEY,
    bucket        VARCHAR(100) NOT NULL,
    object_key    VARCHAR(500) NOT NULL,
    file_name     VARCHAR(255) NOT NULL,
    mime_type     VARCHAR(100),
    size_bytes    BIGINT,
    created_by    BIGINT NOT NULL REFERENCES users(id),
    created_at    TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE (bucket, object_key)
);

CREATE TABLE ticket_attachments (
    id              BIGSERIAL PRIMARY KEY,
    ticket_id       BIGINT NOT NULL REFERENCES tickets(id) ON DELETE CASCADE,
    file_object_id  BIGINT NOT NULL REFERENCES file_objects(id),
    uploaded_by     BIGINT NOT NULL REFERENCES users(id),
    created_at      TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE (ticket_id, file_object_id)
);

CREATE INDEX idx_ticket_attachments_ticket ON ticket_attachments (ticket_id);
```

**上传流程**：预签名 → 前端 PUT 到 MinIO → `POST .../attachments/confirm` 写 `file_objects` + `ticket_attachments`（校验 key 前缀与 ticket 权限）。

---

## 4. API

| 方法 | 路径 | 权限 | 说明 |
|------|------|------|------|
| POST | `/api/v1/storage/presign/upload` | 业务域校验 | 返回 upload URL + fields |
| POST | `/api/v1/storage/presign/download` | 属主/工单可见 | 返回 download URL |
| POST | `/api/v1/tickets/attachments/confirm` | `ticket:update` + 资源级 | 确认上传并关联工单 |
| GET | `/api/v1/tickets/:id/attachments` | `ticket:read` + 可见性 | 附件列表 |
| POST | `/api/v1/tickets/attachments/delete` | `ticket:update` + 属主 | 解除关联（可选删对象） |

### 4.1 预签名上传

```json
// POST /api/v1/storage/presign/upload
{
  "purpose": "ticket_attachment",
  "ticket_id": "1001",
  "file_name": "screenshot.png",
  "mime_type": "image/png",
  "size_bytes": 102400
}
```

**响应**：

```json
{
  "code": 0,
  "data": {
    "upload_url": "http://minio:9000/zhuzhao/...",
    "method": "PUT",
    "headers": { "Content-Type": "image/png" },
    "object_key": "tickets/2026/08/uuid.png",
    "expires_at": "2026-08-14T12:15:00Z"
  }
}
```

**校验**：`size_bytes <= max_upload_bytes`；mime 在白名单；用户对 ticket 有 update 权限。

### 4.2 确认关联

```json
// POST /api/v1/tickets/attachments/confirm
{
  "ticket_id": "1001",
  "object_key": "tickets/2026/08/uuid.png",
  "file_name": "screenshot.png",
  "mime_type": "image/png",
  "size_bytes": 102400
}
```

服务端 HEAD 对象验证存在与 size，再 INSERT 元数据。

### 4.3 头像（复用 storage）

`POST /api/v1/storage/presign/upload` + `purpose: avatar` → 前端 PUT → `POST /api/v1/user/profile/update` 写入 `users.avatar` 为对象 URL 或 key（与前端约定）。

---

## 5. 权限与安全

- 下载 URL **短期有效**（默认 1h），不暴露永久公开 bucket
- `object_key` 必须带随机 UUID，禁止客户端自选路径
- 确认接口校验 `created_by == 当前用户` 且 object 尚未绑定
- 删除工单时 CASCADE 删 `ticket_attachments`；对象存储 **异步 GC**（Phase 2b 可只删 DB，MinIO 对象保留或同步 DeleteObject）

---

## 6. 错误码

| code | 常量 | message | HTTP |
|------|------|---------|------|
| 91001 | `ErrFileTooLarge` | 文件大小超过限制 | 400 |
| 91002 | `ErrFileTypeNotAllowed` | 文件类型不允许 | 400 |
| 91003 | `ErrFileNotFound` | 文件不存在 | 404 |
| 91004 | `ErrFileAlreadyBound` | 文件已关联其它资源 | 409 |

区间 **91000–91999** 存储模块；实现时写入 `errcode.go`。

---

## 7. 测试用例

| # | 用例 | 预期 |
|---|------|------|
| S1 | 预签名上传 PNG | 返回 URL；PUT 成功 |
| S2 | confirm 关联工单 | 附件列表可见 |
| S3 | 无 ticket 权限请求 presign | 403 |
| S4 | 超 10MB | 400 + 91001 |
| S5 | 下载预签名 | 1h 内可 GET |
| S6 | 头像 presign + profile update | users.avatar 更新 |

---

## 8. 涉及文件

```
internal/pkg/storage/s3_client.go
internal/service/storage_service.go
internal/service/ticket/attachment_service.go
internal/handler/storage_handler.go
internal/handler/ticket_handler.go       # attachments 路由
deployments/docker-compose.yaml          # minio 服务
migrations/0000xx_storage.up.sql
```

---

## 9. 待决策点

| 事项 | 建议 | 状态 |
|------|------|------|
| 开发环境 | Docker MinIO | ✅ 建议 |
| 删附件是否删对象 | 2b 仅删关联；GC Job 后移 | ✅ 建议 |
| 独立权限码 ticket:attach | 复用 ticket:update | ✅ 建议 |
