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
| 2 | RequestID | 自写（`logger.go`） | 生成/传递 request_id + 注入 context；格式 `req-` + 32 hex |
| 3 | AccessLogger | 自写（`logger.go`） | 请求日志（method/path/status/cost），按状态码/路径分级 |
| 4 | CORS | `gin-contrib/cors` | Phase 1 AllowAllOrigins（全放开） |
| 5 | SecurityHeaders | 自写（`security.go`） | 安全响应头（5 个 header） |
| 6 | BodyLimit | 自写（`body_limit.go`） | 请求体大小限制（1MB） |

### 2.2 分组中间件

| 路由组 | 中间件 | 说明 |
|--------|--------|------|
| `/api/v1/*` | JWT + Audit + Casbin | 需认证+审计+鉴权（B2-7：Audit 前置于 Casbin，被拒请求同样落审计） |
| `/api/v1/auth/*` | — | 登录/刷新/登出，仅认证不鉴权 |
| `/health/*` | — | 健康检查，无中间件 |
| `/swagger/*` | — | 文档，无中间件 |

### 2.3 中间件执行顺序

```
请求 → Recovery → RequestID(自写) → AccessLogger(自写)
     → CORS(gin-contrib) → SecurityHeaders → BodyLimit
     │
     ├── /health/* → 直接返回
     ├── /swagger/* → 直接返回
     ├── /api/v1/auth/login → JWT 不检查（公开路由）→ Handler
     └── /api/v1/* → JWT → Audit → Casbin → Handler
                      （B2-7：Audit 前置——Casbin 403 拒绝同样落审计，
                        越权尝试留痕；JWT 401 拒绝不产生审计记录，
                        认证失败由 LogLogin 显式记录）
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
        if disabled, _ := rdb.Exists(c, fmt.Sprintf("user:disabled:%d", claims.UserID)).Result(); disabled > 0 {
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

> **登录限流不在中间件**：Phase 1 由 `AuthService.Login` 内 **Lua LoginLocker** 实现（见 [phase1/02-auth.md](../phase1/02-auth.md) §登录限流、[modules/auth.md](./auth.md) §5）。  
> 下列示例仅作 **Phase 3 API 级通用限流** 参考；Phase 1 不挂载此中间件。

```go
// Phase 3 参考：基于 Redis 的固定窗口限流（应用层 INCR+EXPIRE 有竞态，生产应用 Lua 或 ulule/limiter）
func RateLimit(rdb *redis.Client, limit int, window time.Duration) gin.HandlerFunc {
    return func(c *gin.Context) {
        key := fmt.Sprintf("ratelimit:%s:%s", c.ClientIP(), c.Request.URL.Path)
        // ...
    }
}
```

### 3.6 CORS（gin-contrib/cors）

> Phase 1：**DefaultConfig + AllowAllOrigins**（全 Origin 放开，无白名单）；生产再收紧为 `AllowOrigins`。

```go
func CORS() gin.HandlerFunc {
    config := cors.DefaultConfig()
    config.AllowAllOrigins = true
    config.AllowHeaders = append(config.AllowHeaders, "Authorization", "X-Request-Id")
    return cors.New(config)
}
```

---

## 4. 旧系统借鉴

| 设计 | 决策 | 理由 |
|------|------|------|
| 5 个安全头 | ✅ 直接采用 | 安全最佳实践 |
| RequestID 生成 + 四处传播 | ✅ 直接采用 | 链路追踪 |
| PasswordValidator | ✅ 直接采用（放 internal/pkg） | 密码安全 |
| CORS 全放开 | ✅ Phase 1 Default + AllowAllOrigins | 便于联调；生产改白名单 |
| AKSK 中间件 | ⏳ Phase 3 / 按需 | 外部系统对接，非首期 |
| AccessLog 中间件 | ✅ 直接采用 | 详见 audit.md |
| Recovery panic 恢复 | ✅ 直接采用 | 标准做法 |
| BodyLimit | ✅ 新增 | 防止大请求体 |

---

## 5. 分阶段实施

### Phase 1

- Recovery + RequestID(gin-contrib) + AccessLogger(gin-contrib/slog)
- CORS(gin-contrib) + SecurityHeaders + BodyLimit
- JWT 中间件（验证 + 黑名单 + `user:disabled` + must_change_password；Redis 故障 503）
- **非法 AuthN 处理**：混用凭证 400/20008、链式失败立即 `Abort`、日志/审计规则见 [phase1/02-auth.md §非法认证请求的处理](../phase1/02-auth.md#非法认证请求的处理实现必读)
- Casbin 中间件（路由级 RBAC，g 表消除）
- Audit 中间件（同步写入；登录单独审计）

### Phase 2

- 无新增必做中间件（登录限流在 Phase 1 **AuthService + Lua**，非本模块）

### Phase 3

- API 级通用限流
- AKSK 中间件（有 M2M 调用方时）
- 熔断 / 超时 / Prometheus metrics
