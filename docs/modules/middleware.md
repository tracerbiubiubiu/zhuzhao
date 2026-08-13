# 中间件模块设计

> 模块代码：`internal/middleware/`
>
> 旧系统参考：`doc/module-assessment-2026-08/middleware.md`

---

## 1. 模块定位

**核心底座基础设施**。Gin 中间件集合，提供请求处理管线：CORS、RequestID、安全头、JWT 认证、Casbin 鉴权、审计日志、限流、Panic 恢复。

---

## 2. 中间件链

### 2.1 全局中间件（所有路由）

| 顺序 | 中间件 | 实现方式 | 职责 |
|------|--------|---------|------|
| 1 | Recovery | 自写（`recovery.go`） | panic 恢复 + 记录错误日志到 slog |
| 2 | RequestID | `gin-contrib/requestid` | 生成/传递 request_id + 注入 context |
| 3 | AccessLogger | `gin-contrib/slog` | 请求日志（method/path/status/cost），按状态码/路径分级 |
| 4 | CORS | `gin-contrib/cors` | 跨域处理，仅允许 GET/POST/OPTIONS |
| 5 | SecurityHeaders | 自写（`security.go`） | 安全响应头（5 个 header） |
| 6 | BodyLimit | 自写（`body_limit.go`） | 请求体大小限制（1MB） |

### 2.2 分组中间件

| 路由组 | 中间件 | 说明 |
|--------|--------|------|
| `/api/v1/*` | JWT + Casbin + Audit | 需认证+鉴权+审计 |
| `/api/v1/auth/*` | — | 登录/刷新/登出，仅认证不鉴权 |
| `/health/*` | — | 健康检查，无中间件 |
| `/swagger/*` | — | 文档，无中间件 |

### 2.3 中间件执行顺序

```
请求 → Recovery → RequestID(gin-contrib) → AccessLogger(gin-contrib/slog)
     → CORS(gin-contrib) → SecurityHeaders → BodyLimit
     │
     ├── /health/* → 直接返回
     ├── /swagger/* → 直接返回
     ├── /api/v1/auth/login → JWT 不检查（公开路由）→ Handler
     └── /api/v1/* → JWT → Casbin → Audit → Handler
```

---

## 3. 各中间件设计

### 3.1 JWT 中间件

```go
func JWT(jwtManager *jwt.Manager, rdb *redis.Client) gin.HandlerFunc {
    return func(c *gin.Context) {
        // 1. 提取 Bearer token
        token := extractToken(c)
        if token == "" {
            response.AbortWithStatus(c, 401, "missing token")
            return
        }

        // 2. 验证签名 + 过期
        claims, err := jwtManager.Verify(token)
        if err != nil {
            response.AbortWithStatus(c, 401, "invalid token")
            return
        }

        // 3. Redis 黑名单检查
        if banned, _ := rdb.Exists(c, "blacklist:at:"+claims.JTI).Result(); banned > 0 {
            response.AbortWithStatus(c, 401, "token revoked")
            return
        }

        // 4. 用户禁用检查
        if disabled, _ := rdb.Exists(c, "user:disabled:"+claims.UserID).Result(); disabled > 0 {
            response.AbortWithStatus(c, 403, "user disabled")
            return
        }

        // 5. 注入 context
        c.Set("user_id", claims.UserID)
        c.Set("jti", claims.JTI)
        c.Next()
    }
}
```

### 3.2 Casbin 中间件

详见 [authz.md](./authz.md) §2。

### 3.3 RequestID（gin-contrib/requestid）

```go
// 使用 gin-contrib/requestid，无需自写
router.Use(requestid.New())

// 获取 request_id
rid := requestid.Get(c)

// 在 gin-contrib/slog 中关联 request_id
router.Use(slog.SetLogger(
    slog.WithLogger(func(c *gin.Context, l *slog.Logger) *slog.Logger {
        return l.With("request_id", requestid.Get(c))
    }),
))
```

### 3.4 安全头（借鉴旧系统 5 个 header）

```go
func SecurityHeaders() gin.HandlerFunc {
    return func(c *gin.Context) {
        c.Header("X-Frame-Options", "DENY")
        c.Header("X-Content-Type-Options", "nosniff")
        c.Header("Strict-Transport-Security", "max-age=31536000")
        c.Header("Cache-Control", "no-store")
        c.Header("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
        c.Next()
    }
}
```

### 3.5 限流

```go
// 基于 Redis 的滑动窗口限流
func RateLimit(rdb *redis.Client, limit int, window time.Duration) gin.HandlerFunc {
    return func(c *gin.Context) {
        key := fmt.Sprintf("ratelimit:%s:%s", c.ClientIP(), c.Request.URL.Path)

        // Lua 脚本：INCR + EXPIRE 原子操作
        count, err := rdb.Incr(c, key).Result()
        if err != nil {
            // Redis 故障：放行（可用性优先）或拒绝（安全优先）
            c.Next()
            return
        }
        if count == 1 {
            rdb.Expire(c, key, window)
        }
        if count > int64(limit) {
            response.AbortWithStatus(c, 429, "rate limit exceeded")
            return
        }
        c.Next()
    }
}
```

### 3.6 CORS（gin-contrib/cors）

```go
func CORS() gin.HandlerFunc {
    return cors.New(cors.Config{
        AllowOrigins:     []string{"https://example.com", "http://localhost:*"},
        AllowMethods:     []string{"GET", "POST", "OPTIONS"},  // 仅 GET/POST，不用 PUT/DELETE/PATCH
        AllowHeaders:     []string{"Origin", "Content-Type", "Authorization", "X-Request-Id"},
        AllowCredentials: true,
        MaxAge:           12 * time.Hour,
    })
}
```

---

## 4. 旧系统借鉴

| 设计 | 决策 | 理由 |
|------|------|------|
| 5 个安全头 | ✅ 直接采用 | 安全最佳实践 |
| RequestID 生成 + 四处传播 | ✅ 直接采用 | 链路追踪 |
| PasswordValidator | ✅ 直接采用（放 internal/pkg） | 密码安全 |
| CORS 白名单 | ✅ 采用 gin-contrib/cors | 仅允许 GET/POST/OPTIONS，不用 PUT/DELETE |
| AKSK 中间件 | ⏳ Phase 3 | 外部系统对接，非首期 |
| AccessLog 中间件 | ✅ 直接采用 | 详见 audit.md |
| Recovery panic 恢复 | ✅ 直接采用 | 标准做法 |
| BodyLimit | ✅ 新增 | 防止大请求体 |

---

## 5. 分阶段实施

### Phase 1

- Recovery + RequestID(gin-contrib) + AccessLogger(gin-contrib/slog)
- CORS(gin-contrib) + SecurityHeaders + BodyLimit
- JWT 中间件（验证 + 黑名单 + must_change_password 拦截）
- Casbin 中间件（路由级 RBAC，g 表消除）
- Audit 中间件（同步写入）

### Phase 2

- 用户级限流
- AKSK 中间件（外部系统对接）

### Phase 3

- 熔断中间件
- 请求超时中间件
- Prometheus metrics 中间件
