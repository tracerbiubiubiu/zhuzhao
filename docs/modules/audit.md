# 审计日志模块设计

> 模块代码：`internal/service/audit_service.go` + `internal/middleware/audit.go`
>
> 旧系统参考：`doc/module-assessment-2026-08/accesslog.md`

---

## 1. 模块定位

**横切基础设施模块**。记录用户操作行为和 HTTP 请求日志，支持审计追溯。

与其他模块的关系：
- 作为中间件挂载到需审计的路由
- 不依赖业务 Service（纯写入）

---

## 2. 数据模型

### 2.1 访问日志（HTTP 请求记录）

```sql
CREATE TABLE access_logs (
    id          BIGSERIAL PRIMARY KEY,
    trace_id    VARCHAR(50) NOT NULL,       -- 请求链路 ID
    user_id     VARCHAR(50),                -- 操作用户
    method      VARCHAR(10) NOT NULL,       -- HTTP 方法
    path        VARCHAR(500) NOT NULL,      -- 请求路径
    status_code INT NOT NULL,               -- 响应状态码
    cost_ms     INT NOT NULL,               -- 耗时（毫秒）
    request_body TEXT,                      -- 请求体（截断 + 脱敏，4KB）
    ip          VARCHAR(50),                -- 客户端 IP
    user_agent  VARCHAR(200),               -- User-Agent
    action      VARCHAR(100),               -- 操作名称（如 list_users）
    created_at  TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_access_logs_user ON access_logs(user_id, created_at DESC);
CREATE INDEX idx_access_logs_created ON access_logs(created_at DESC);
```

### 2.2 操作审计日志（业务操作记录）

```sql
CREATE TABLE audit_logs (
    id          BIGSERIAL PRIMARY KEY,
    trace_id    VARCHAR(50) NOT NULL,
    user_id     VARCHAR(50) NOT NULL,       -- 操作人
    action      VARCHAR(100) NOT NULL,      -- 操作类型（如 delete_user）
    resource    VARCHAR(50),                -- 资源类型（如 user）
    resource_id VARCHAR(50),                -- 资源 ID
    detail      JSONB,                      -- 操作详情
    ip          VARCHAR(50),
    created_at  TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_audit_logs_user ON audit_logs(user_id, created_at DESC);
CREATE INDEX idx_audit_logs_action ON audit_logs(action, created_at DESC);
CREATE INDEX idx_audit_logs_resource ON audit_logs(resource, resource_id);
```

---

## 3. 接口定义

```go
type AuditService interface {
    // 记录访问日志（中间件自动调用）
    LogAccess(ctx context.Context, log AccessLog) error

    // 记录操作审计（Service 层手动调用）
    LogAction(ctx context.Context, log AuditLog) error

    // 查询（管理接口）
    QueryAccessLogs(ctx context.Context, query AccessLogQuery) ([]*AccessLog, int64, error)
    QueryAuditLogs(ctx context.Context, query AuditLogQuery) ([]*AuditLog, int64, error)
}
```

---

## 4. 核心流程

### 4.1 异步写入流程

```
请求 → Audit 中间件
  │  记录：trace_id, user_id, method, path, status_code, cost, request_body(脱敏)
  │  → channel（缓冲 1024）
  │
  → 立即返回（不阻塞请求）
  │
  → 异步 worker goroutine
     → 批量写入 PostgreSQL（每 100 条或每 5 秒）
     → channel 满时降级：同步写入 + 告警
```

### 4.2 action 推导（借鉴旧系统 actionRegistry）

旧系统通过 `accesslog.RegisterAction(method, fullPath, action)` 注册路由→动作映射。

**新框架方案**：用 route 注解或 map 注册。

```go
// 启动时注册
var actionRegistry = map[string]string{} // "POST:/api/users/:id/delete" → "delete_user"

func RegisterAction(method, path, action string) {
    actionRegistry[method+":"+path] = action
}

// 中间件中推导
func deriveAction(method, path string) string {
    // 1. 精确匹配
    if action, ok := actionRegistry[method+":"+path]; ok {
        return action
    }
    // 2. 模糊匹配（从 route pattern 推导）
    // ...
    return method + ":" + path
}
```

### 4.3 敏感字段脱敏（借鉴旧系统）

```yaml
# configs/config.yaml
audit:
  sensitive_fields:
    - password
    - token
    - secret
    - private_key
    - access_key
    - secret_key
  max_body_length: 4096
```

```go
func sanitizeBody(body []byte, maxLen int, sensitiveFields []string) string {
    if len(body) > maxLen {
        body = body[:maxLen]
    }

    var data map[string]interface{}
    if err := json.Unmarshal(body, &data); err != nil {
        return string(body)
    }

    for _, field := range sensitiveFields {
        if _, ok := data[field]; ok {
            data[field] = "***"
        }
    }

    result, _ := json.Marshal(data)
    return string(result)
}
```

---

## 5. 日志清理

PostgreSQL 不支持 TTL 索引，使用定时任务：

```sql
-- cron: 每天 03:00 执行
DELETE FROM access_logs WHERE created_at < NOW() - INTERVAL '90 days';
DELETE FROM audit_logs WHERE created_at < NOW() - INTERVAL '180 days';

-- 或使用分区表（大数据量时）
-- CREATE TABLE access_logs_2026_08 PARTITION OF access_logs
--   FOR VALUES FROM ('2026-08-01') TO ('2026-09-01');
-- DROP TABLE access_logs_2026_01;  -- 删除旧分区
```

---

## 6. 旧系统借鉴

| 设计 | 决策 | 理由 |
|------|------|------|
| 异步写入 + channel | ✅ 直接采用 | 不阻塞请求 |
| channel 满降级 | ✅ 直接采用 | 同步写入 + 告警 |
| actionRegistry 推导 | ✅ 直接采用 | 比 method:path 更可读 |
| 敏感字段脱敏 | ✅ 直接采用 | 安全要求 |
| 请求体截断（4KB） | ✅ 直接采用 | 防止大 body 撑爆日志 |
| trace_id 传播 | ✅ 直接采用 | 链路追踪 |
| MongoDB TTL 自动清理 | ❌ 改为 cron DELETE | 数据库不同 |
| MongoWriteSyncer | ❌ 不需要 | slog + PostgreSQL 直接写 |
| 每服务独立日志集合 | ❌ 统一表 | 简化管理 |

---

## 7. 分阶段实施

### Phase 1

- AccessLog 中间件（同步写入）
- 登录成功/失败单独写审计（公开路由不走 AuditLog 中间件）
- 基本字段记录（method/path/status/cost/user_id）
- trace_id / request_id 注入

### Phase 2

- 无必须项（工单模块可复用同步审计）

### Phase 3

- 异步写入 + channel / Redis List L2
- actionRegistry 推导、敏感字段脱敏
- 审计日志查询接口、清理 cron
- 分区表、导出
