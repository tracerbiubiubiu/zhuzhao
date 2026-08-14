# 09 - 中间件模块（middleware）

> Step 4，依赖 Step 3（auth）。所有 HTTP 请求的通用处理层。

---

## 预期功能

| 中间件 | 功能 | 顺序 | 实现方式 | 说明 |
|--------|------|------|---------|------|
| Recovery | panic 兜底，返回 500 | 1 | 自写（包装 `gin.Recovery()` + slog） | 最外层，确保不崩 |
| RequestID | 生成/传递 trace ID | 2 | `gin-contrib/requestid` | 注入 context + response header，串联 slog |
| AccessLogger | 请求日志（access log） | 3 | `gin-contrib/slog` | method/path/status/cost，支持按状态码/路径分级 |
| CORS | 跨域允许 | 4 | `gin-contrib/cors` | Phase 1 **AllowAllOrigins**（全放开，无白名单）；见下方说明 |
| SecurityHeaders | 安全响应头 | 5 | 自写（5 行代码） | 5 个安全头（借鉴旧系统） |
| BodyLimit | 请求体大小限制 | 6 | 自写（`http.MaxBytesReader`） | 默认 1MB，防止大请求体 |
| JWTAuth | AT 解析 + 黑名单 + 用户禁用检查 | 7 | 自写 | 仅鉴权路由；**AuthN 拒绝原则**见 [02-auth §非法认证请求的处理](./02-auth.md#非法认证请求的处理实现必读) |
| CasbinAuth | 路由级 RBAC 校验 | 8 | 自写 | 仅鉴权路由，g 表消除模型，逐角色 enforce |
| AuditLog | 操作日志记录 | 9 | 自写 | 仅需审计的路由 |

> **登录限流**：不在中间件链，由 `AuthService.Login` 调用 Redis **Lua LoginLocker**（见 [02-auth.md](./02-auth.md) §登录限流）。

### 中间件选型说明

经过对 `gin-contrib`、`gin-gonic/contrib`、`appleboy/gin-jwt`、`gin-contrib/authz`、`eddycjy/go-gin-example` 的详细评估：

- **采用 gin-contrib 的**：RequestID、CORS、AccessLogger（slog）——成熟库，不重复造轮子
- **自写的**：Recovery（需适配 slog）、SecurityHeaders（库过重，5 行代码）、BodyLimit（1 行代码）、JWTAuth（双 token + Redis 黑名单 + must_change_password 拦截，无库满足）、CasbinAuth（g 表消除 + RoleFetcher + 逐角色 enforce，无库满足）、AuditLog（业务逻辑强相关）
- **不采用的库及原因**：
  - `appleboy/gin-jwt`：v3 虽支持双 token，但 RT 是 opaque token（非 JWT）、无 AT 黑名单、Redis 库用 `rueidis` 不是 `go-redis`
  - `gin-contrib/authz`：56 行代码，硬编码 BasicAuth，不支持 g 表消除和 SyncedEnforcer
  - `gin-contrib/secure`：功能过重（SSL redirect 等），5 个 header 直接写更简洁

---

## 核心设计思路

### 中间件链路

```go
router := gin.New()
router.Use(
    middleware.Recovery(logger),
    requestid.New(),                    // gin-contrib/requestid
    slog.SetLogger(                     // gin-contrib/slog
        slog.WithSkipPath([]string{"/health/live", "/health/ready"}),
        slog.WithLogger(func(c *gin.Context, l *slog.Logger) *slog.Logger {
            return l.With("request_id", requestid.Get(c))
        }),
    ),
    middleware.CORS(),                   // gin-contrib/cors DefaultConfig + AllowAllOrigins
    middleware.SecurityHeaders(),
    middleware.BodyLimit(1<<20), // 1MB
)

// 无需鉴权的路由
public := router.Group("/api/v1")
{
    public.POST("/auth/login", authHandler.Login)
    public.POST("/auth/refresh", authHandler.Refresh)
    public.GET("/health/live", healthHandler.Live)
}

// 需要鉴权的路由
authed := router.Group("/api/v1")
authed.Use(
    middleware.JWTAuth(jwtManager, rdb),
    middleware.CasbinAuth(enforcer, roleFetcher),
    middleware.AuditLog(auditRepo),
)
{
    authed.POST("/auth/logout", authHandler.Logout)
    authed.GET("/users", userHandler.List)
    // ...
}
```

### JWT 中间件

> 详见 [modules/middleware.md](../modules/middleware.md) §3.1。

```go
func JWTAuth(jwt *jwt.Manager, rdb *redis.Client) gin.HandlerFunc {
    return func(c *gin.Context) {
        // 1. 提取 AT
        token := extractToken(c)  // 从 Authorization: Bearer xxx 提取
        if token == "" {
            response.Unauthorized(c, errcode.ErrUnauthorized)
            c.Abort()
            return
        }
        
        // 2. 解析 AT
        claims, err := jwt.ParseAccessToken(token)
        if err != nil {
            response.Unauthorized(c, errcode.ErrTokenInvalid)
            c.Abort()
            return
        }
        
        // 3. Redis 故障 → 503（fail-close）
        exists, err := rdb.Exists(c, fmt.Sprintf("blacklist:at:%s", claims.JTI)).Result()
        if err != nil {
            response.ServiceUnavailable(c, errcode.ErrServiceUnavailable)
            c.Abort()
            return
        }
        if exists > 0 {
            response.Unauthorized(c, errcode.ErrTokenInvalid)
            c.Abort()
            return
        }

        // 4. 用户级吊销（禁用/删除）
        disabled, err := rdb.Exists(c, fmt.Sprintf("user:disabled:%d", claims.UserID)).Result()
        if err != nil {
            response.ServiceUnavailable(c, errcode.ErrServiceUnavailable)
            c.Abort()
            return
        }
        if disabled > 0 {
            response.Forbidden(c, errcode.ErrUserDisabled)
            c.Abort()
            return
        }

        // 5. 首次登录改密检查
        if claims.MustChangePassword {
            // 只允许访问改密接口
            if c.Request.URL.Path != "/api/v1/auth/password/update" {
                response.Forbidden(c, errcode.ErrPasswordChangeRequired)
                c.Abort()
                return
            }
        }
        
        // 6. 注入用户信息到 context
        c.Set("userID", claims.UserID)
        c.Set("username", claims.Username)
        c.Set("jti", claims.JTI)
        c.Set("must_change_password", claims.MustChangePassword)
        c.Next()
    }
}
```

### Casbin 中间件（g 表消除）

> 详见 [modules/authz.md](../modules/authz.md) §2.2。Phase 1：无 g 表，`RoleFetcher` 查直接角色后逐 `role::{code}` enforce。

```go
func CasbinAuth(enforcer *casbin.SyncedEnforcer, roleFetcher RoleFetcher) gin.HandlerFunc {
    return func(c *gin.Context) {
        userID := c.GetInt64("userID")
        
        // 1. 获取用户角色（Phase 1 只查直接角色）
        roles, err := roleFetcher.FetchRoleCodes(c.Request.Context(), userID)
        if err != nil {
            response.InternalError(c, errcode.ErrInternal)
            c.Abort()
            return
        }
        if len(roles) == 0 {
            response.Forbidden(c, errcode.ErrNoRoles)
            c.Abort()
            return
        }
        
        // 2. 逐角色 enforce（superadmin/admin 在 matcher 中自动 bypass）
        path := c.Request.URL.Path
        method := c.Request.Method
        allowed := false
        for _, role := range roles {
            if enforcer.Enforce("role::"+role, path, method) {
                allowed = true
                break
            }
        }
        
        if !allowed {
            response.Forbidden(c, errcode.ErrNoPermission)
            c.Abort()
            return
        }
        
        // 3. 存入 context 供 handler 复用
        c.Set("roles", roles)
        c.Next()
    }
}

// RoleFetcher 接口，避免中间件直接依赖 UserRepo
type RoleFetcher interface {
    FetchRoleCodes(ctx context.Context, userID int64) ([]string, error)
}
```

### RequestID（gin-contrib/requestid）

```go
// 使用 gin-contrib/requestid，无需自写
router.Use(requestid.New())

// 获取 request_id：
rid := requestid.Get(c)

// 在 gin-contrib/slog 中关联 request_id：
router.Use(slog.SetLogger(
    slog.WithLogger(func(c *gin.Context, l *slog.Logger) *slog.Logger {
        return l.With("request_id", requestid.Get(c))
    }),
))
```

### 安全头（借鉴旧系统 5 个 header）

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

### CSRF 防护

Phase 1 **不需要 CSRF 中间件**。原因：

- API 仅通过 `Authorization: Bearer {AT}` 鉴权，**不使用 Cookie** 传递凭证
- CSRF 攻击的前提是浏览器自动携带 Cookie；Bearer Token 不会自动发送，攻击者无法伪造
- 前端需在每次请求 header 中显式设置 `Authorization`，这天然防 CSRF

> 若 Phase 3+ 改用 HttpOnly Cookie 传递 Token（如支持 SSR 场景），则需补 CSRF Token 或 SameSite=Strict 策略。

### BodyLimit

```go
func BodyLimit(limit int64) gin.HandlerFunc {
    return func(c *gin.Context) {
        c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, limit)
        c.Next()
    }
}
// 默认 1MB
```

### AuthN 拒绝原则（实现 JWTAuth / 未来 AKSK 时必读）

> **完整 SSOT**：[02-auth.md §非法认证请求的处理](./02-auth.md#非法认证请求的处理实现必读)（分类表、slog 字段、审计、伪代码、测试用例）

实现中间件时须遵守：

1. **受保护路由**：Bearer 与 `X-AK-*` **互斥**；混用 → `response.BadRequest` + **20008**，`c.Abort()`，不解析 JWT。
2. **链式失败即 Abort**：黑名单 / `user:disabled` / mcp 任一失败 → 不进 Casbin。
3. **Redis 鉴权查询失败** → **503/10008**（fail-close）。
4. **业务 body 中的 password 不参与 AuthN**；无 Token → 401。
5. **日志**：`slog.Warn` + `reason` + `request_id`；禁止记录 token/SK/密码。

Phase 1 仅 JWT；检测到 `X-AK-*` 且未启用 M2M → **401/20009**。

---

## 测试用例

### JWT 中间件

| 用例 | 请求 | 预期 |
|------|------|------|
| 无 Token | 不带 Authorization header | 401 |
| Bearer + X-AK 混用（M2M 预留） | 两种头同时存在 | **400 + 20008**，Abort |
| Token 格式错误 | `Bearer 乱字符串` | 401 |
| Token 过期 | 过期的 AT | 401 |
| Token 被黑名单 | 登出后的 AT | 401 |
| 用户已禁用 | user:disabled 存在 | 403 + 30003 |
| Redis 不可用 | Exists 失败 | 503 |
| Token 有效 | 正常 AT | 放行，context 有 userID |

### Casbin 中间件

> `user_manager` 为测试用自建角色（非种子角色）；`viewer` 为种子只读角色。

| 用例 | 角色 | 请求 | 预期 |
|------|------|------|------|
| admin 角色 | admin | 任意 | 放行 |
| 有权限 | user_manager | `GET /users` | 放行 |
| 无权限 | viewer | `POST /users` | 403 |
| 路径通配符 | user_manager | `GET /users/123` | 放行（keyMatch2） |

### Recovery

| 用例 | 请求 | 预期 |
|------|------|------|
| Handler panic | 触发 panic 的路由 | 500 + 统一错误响应，不崩 |

### RequestID

| 用例 | 请求 | 预期 |
|------|------|------|
| 无 RequestID | 不带 header | 生成新 UUID，response header `X-Request-Id` 返回 |
| 有 RequestID | `X-Request-ID: abc-123` | 透传，response header 返回 abc-123 |
| slog 关联 | 任意请求 | 应用日志中包含 `request_id` 字段 |

### CORS

> **Phase 1 决策**：`gin-contrib/cors` 的 `DefaultConfig()` + `AllowAllOrigins = true`（等同 `cors.Default()` 思路，**全 Origin 放开**，不做域名白名单）。另追加 `Authorization`、`X-Request-Id` 到 `AllowHeaders`（JWT 浏览器预检必需）。`AllowAllOrigins` 时 **不能** 开 `AllowCredentials`（库限制）；Bearer Token 走 Header，不受影响。生产上线前再改为 `AllowOrigins` 白名单。

| 用例 | 请求 | 预期 |
|------|------|------|
| 预检请求 | OPTIONS + Origin | 204 + CORS headers |
| 正常请求 | GET + 任意 Origin | 200 + `Access-Control-Allow-Origin: *` |
| 带 Authorization | POST + Origin + Bearer | 预检通过，业务正常 |

---

## 涉及文件

> 横切层保持 `internal/middleware/`；AuthN 拒绝原则见 [02-auth §非法认证](./02-auth.md#非法认证请求的处理实现必读)。

```
internal/middleware/jwt.go             # 含混用检测（20008）+ 黑名单 + mcp
internal/middleware/casbin.go          # Casbin 中间件（g 表消除，逐角色 enforce，RoleFetcher 接口）
internal/middleware/recovery.go        # Recovery 中间件（已有，包装 gin.Recovery + slog）
internal/middleware/security.go        # 安全头中间件（已有，5 个 header）
internal/middleware/body_limit.go      # 请求体限制中间件（已有）
internal/middleware/audit.go           # 操作日志中间件（需创建）
internal/middleware/cors.go             # gin-contrib DefaultConfig + AllowAllOrigins
```
