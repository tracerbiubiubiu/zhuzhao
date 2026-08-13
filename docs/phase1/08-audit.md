# 08 - 审计日志模块（audit）

> Step 10，依赖 Step 4（middleware）。Phase 1 实现操作日志中间件 + 查询。

---

## 预期功能

| 功能 | 场景 | 说明 |
|------|------|------|
| 操作日志中间件 | 自动记录所有 API 请求 | 同步写入 DB，不阻塞响应（见下方分析） |
| 日志查询 | 管理员查看操作日志 | `GET /api/v1/audit/logs` |
| 日志筛选 | 按用户、操作类型、时间范围筛选 | query string 参数 |
| 敏感字段脱敏 | 密码等字段不记录明文 | 请求体中的 password 字段替换为 `***` |

### Phase 1 不做

| 功能 | 原因 | 阶段 |
|------|------|------|
| channel + batch 异步写入 | Phase 1 请求量低，同步够用 | Phase 2 |
| Redis List 队列 | 高吞吐场景 | Phase 3 |
| 日志过期清理 | 分区表或 cron | Phase 2 |

---

## 核心设计思路

### 同步 vs 异步：Phase 1 用同步

审计日志异步写入（channel + goroutine 消费）的利弊：

| 维度 | 异步（channel + goroutine） | 同步（请求内写 DB） |
|------|---------------------------|-------------------|
| 请求延迟 | 低（日志写入不阻塞响应） | 略高（多一次 DB INSERT） |
| 可靠性 | **进程崩溃丢 channel 内日志** | 不丢（DB 事务保证） |
| 实现复杂度 | 高（channel 管理、关闭时刷空、满时降级） | 低 |
| 优雅关闭 | 需等待 channel 消费完 | 不需要 |
| DB 压力 | 可 batch 合并，压力可控 | 每请求一次 INSERT |

**Phase 1 决策：同步写入。** 理由：

1. Phase 1 请求量低（内部办公系统），每请求多一次 INSERT 对延迟影响可忽略（PG 本地 INSERT 约 0.1ms）
2. 审计日志的核心要求是**不丢**——安全审计场景下，丢失操作记录比慢 0.1ms 严重得多
3. 异步引入的复杂度（channel 满降级、关闭时刷空、goroutine 泄漏）在 Phase 1 不值得
4. Phase 2 引入 channel + batch 时，接口不变，只改 `AuditRepo.Insert` 内部实现

**优化措施**：同步写入虽在请求链路内，但放在 `c.Next()` 之后（响应已写完），用户感知延迟不受影响：

```go
func OperationLog(auditRepo AuditRepo) gin.HandlerFunc {
    return func(c *gin.Context) {
        start := time.Now()

        // 读取请求体（用于记录操作参数）
        bodyBytes, _ := io.ReadAll(c.Request.Body)
        c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

        // 执行请求（响应在此期间已写完）
        c.Next()

        // 同步记录（响应已返回给客户端，此处延迟用户无感知）
        log := AuditLog{
            UserID:      c.GetInt64("userID"),
            Username:    c.GetString("username"),
            Method:      c.Request.Method,
            Path:        c.Request.URL.Path,
            StatusCode:  c.Writer.Status(),
            Duration:    time.Since(start).Milliseconds(),
            IP:          c.ClientIP(),
            UserAgent:   c.Request.UserAgent(),
            RequestBody: maskSensitive(bodyBytes),
            CreatedAt:   time.Now(),
        }
        // 同步写入 DB，失败只记应用日志，不影响业务
        if err := auditRepo.Insert(c.Request.Context(), log); err != nil {
            slog.Error("audit log write failed", "err", err, "path", log.Path)
        }
    }
}
```

> 注意：`c.Next()` 之后 Gin 已经把响应写给了客户端，但 HTTP 那一层的连接还未关闭（Gin 在中间件链全部执行完后才关闭）。所以同步写审计日志确实会增加一点点连接占用时间，但用户端已经收到了响应。

### 敏感字段脱敏

```go
func maskSensitive(body []byte) string {
    var m map[string]any
    if json.Unmarshal(body, &m) != nil {
        return string(body)
    }
    sensitiveKeys := []string{"password", "old_password", "new_password", "secret", "token"}
    for _, key := range sensitiveKeys {
        if _, ok := m[key]; ok {
            m[key] = "***"
        }
    }
    result, _ := json.Marshal(m)
    return string(result)
}
```

### 审计日志表

```sql
CREATE TABLE audit_logs (
    id           BIGSERIAL PRIMARY KEY,
    user_id      BIGINT,
    username     VARCHAR(100),
    method       VARCHAR(10) NOT NULL,
    path         VARCHAR(500) NOT NULL,
    status_code  INT NOT NULL,
    duration     BIGINT NOT NULL,          -- 毫秒
    ip           VARCHAR(50),
    user_agent   VARCHAR(500),
    request_body TEXT,                     -- 脱敏后的请求体
    created_at   TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_audit_user_time ON audit_logs(user_id, created_at DESC);
CREATE INDEX idx_audit_path_time ON audit_logs(path, created_at DESC);
```

---

## 测试用例

### 中间件

| 用例 | 请求 | 验证点 |
|------|------|--------|
| 记录 GET 请求 | `GET /api/v1/users` | audit_logs 表有记录，method=GET |
| 记录 POST 请求 | `POST /api/v1/users` | request_body 有内容 |
| 密码脱敏 | body 含 `{"password":"123456"}` | request_body 中 password 为 `***` |
| 未认证请求 | 无 AT 的请求 | user_id 为 NULL |
| 响应时间记录 | 任意请求 | duration > 0 |

### 查询

| 用例 | 参数 | 预期 |
|------|------|------|
| 按用户筛选 | `?user_id=1` | 只返回该用户的日志 |
| 按时间范围 | `?start=2026-08-01&end=2026-08-12` | 只返回时间范围内 |
| 按路径筛选 | `?path=/api/v1/users` | 只返回该路径的日志 |
| 分页 | `?page=1&page_size=20` | 返回分页结果 |
| 无权限 | 非 admin 角色请求 | 403 |

---

## 涉及文件

```
internal/middleware/audit.go           # 操作日志中间件
internal/repository/audit_repo.go      # 审计日志数据访问
internal/service/audit_service.go      # 审计日志查询
internal/handler/audit_handler.go      # HTTP Handler
internal/model/audit.go                # 审计日志模型
```

## 待决策点

> 以下决策已在讨论中确认：

- ✅ **同步 vs 异步**：Phase 1 同步写入 DB。理由：审计日志核心要求是不丢，Phase 1 请求量低，同步足够。Phase 2 引入 channel + batch 时接口不变。
- ✅ **日志保留策略**：Phase 1 不做清理，手动管理。Phase 2 用 PG 分区表或 cron 定期清理。

---

## 补充：应用日志（service log）规划

> 审计日志是"谁在什么时间做了什么操作"（记入 DB，面向安全审计），应用日志是"系统运行时发生了什么"（记入文件，面向排障）。两者完全不同。

### 两类日志的区别

| 维度 | 审计日志（audit_logs 表） | 应用日志（app.log 文件） |
|------|------------------------|------------------------|
| 用途 | 安全审计、合规追溯 | 程序排障、性能分析 |
| 存储 | PostgreSQL | 文件（Lumberjack 轮转） |
| 内容 | 谁、什么时间、做了什么操作 | 错误堆栈、调试信息、请求链路 |
| 查询 | API 查询，支持筛选 | grep / 日志平台 |
| 保留 | 长期（合规要求） | 短期（轮转覆盖） |

### 应用日志 Phase 1 方案

**同步写入，不异步。** 理由：

1. **slog 本身已足够快** — slog 的 JSONHandler 写入文件 + stdout，单次日志调用约 1-2 微秒，不构成瓶颈
2. **异步反而有风险** — 异步意味着日志有延迟，程序崩溃时最后几条日志（往往是最关键的错误信息）可能丢失
3. **Lumberjack 处理轮转** — 文件写入由 Lumberjack 管理，按大小自动轮转，不需要异步

### 日志层级使用规范

```go
// Error：影响业务流程的错误，必须排查
slog.Error("user login failed", "username", username, "err", err)

// Warn：异常但可恢复，需关注
slog.Warn("casbin reload failed, using stale policy", "err", err)

// Info：关键业务节点，生产环境默认级别
slog.Info("user logged in", "user_id", userID, "ip", ip)

// Debug：调试信息，生产环境关闭
slog.Debug("casbin enforce", "sub", sub, "obj", obj, "act", act, "result", allowed)
```

### 日志中必须包含的字段

每条应用日志通过 `gin-contrib/requestid` 中间件注入的 request_id 串联，便于追踪：

```go
// gin-contrib/requestid 生成 request_id，gin-contrib/slog 关联到日志
router.Use(requestid.New())
router.Use(slog.SetLogger(
    slog.WithLogger(func(c *gin.Context, l *slog.Logger) *slog.Logger {
        return l.With("request_id", requestid.Get(c))
    }),
    slog.WithSkipPath([]string{"/health/live", "/health/ready"}),
))

// 在 handler/service 中使用 slog（自动带 request_id）
func (s *userService) Create(ctx context.Context, user *User) error {
    slog.InfoContext(ctx, "creating user", "username", user.Username)
    // ...
}
```

> `gin-contrib/slog` 通过 `slog.Get(c)` 获取带 request_id 的 logger，handler 中可直接使用。
> Service 层通过 `context.Context` 传递，使用 `slog.InfoContext(ctx, ...)` 等方法。

### 日志文件规划

| 文件 | 内容 | 轮转策略 |
|------|------|---------|
| `logs/app.log` | 应用日志（Info 及以上） | 100MB/文件，保留 7 个，压缩 |
| `logs/error.log` | 错误日志（Error 及以上） | 100MB/文件，保留 14 个，压缩 |
| stdout | 同时输出到标准输出（Docker 日志收集） | — |

> Phase 1 暂不拆分 error.log，统一写 app.log + stdout。Phase 2 可按需拆分。

### 涉及文件

```
internal/pkg/logger/logger.go          # slog + Lumberjack（已有，需补 context 传递）
# AccessLogger 使用 gin-contrib/slog，无需自写 access_log.go
```
