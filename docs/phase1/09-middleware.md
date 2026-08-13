# 09 - 中间件模块（middleware）

> Step 4，依赖 Step 3（auth）。所有 HTTP 请求的通用处理层。

---

## 预期功能

| 中间件 | 功能 | 顺序 | 实现方式 | 说明 |
|--------|------|------|---------|------|
| Recovery | panic 兜底，返回 500 | 1 | 自写（包装 `gin.Recovery()` + slog） | 最外层，确保不崩 |
| RequestID | 生成/传递 trace ID | 2 | `gin-contrib/requestid` | 注入 context + response header，串联 slog |
| AccessLogger | 请求日志（access log） | 3 | `gin-contrib/slog` | method/path/status/cost，支持按状态码/路径分级 |
| CORS | 跨域允许 | 4 | `gin-contrib/cors` | 前端开发必需，仅允许 GET/POST/OPTIONS |
| SecurityHeaders | 安全响应头 | 5 | 自写（5 行代码） | 5 个安全头（借鉴旧系统） |
| BodyLimit | 请求体大小限制 | 6 | 自写（`http.MaxBytesReader`） | 默认 1MB，防止大请求体 |
| JWTAuth | AT 解析 + 黑名单 + 用户禁用检查 | 7 | 自写 | 仅鉴权路由，双 token + Redis 黑名单 + must_change_password |
| CasbinAuth | 路由级 RBAC 校验 | 8 | 自写 | 仅鉴权路由，g 表消除模型，逐角色 enforce |
| AuditLog | 操作日志记录 | 9 | 自写 | 仅需审计的路由 |

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
    cors.New(cors.Config{               // gin-contrib/cors
        AllowOrigins:     []string{"http://localhost:*"},
        AllowMethods:     []string{"GET", "POST", "OPTIONS"},
        AllowHeaders:     []string{"Origin", "Content-Type", "Authorization", "X-Request-Id"},
        AllowCredentials: true,
        MaxAge:           12 * time.Hour,
    }),
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
            response.Unauthorized(c, errcode.ErrTokenRequired)
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
        
        // 3. 检查黑名单
        if exists, _ := rdb.Exists(c, fmt.Sprintf("blacklist:at:%s", claims.JTI)).Result(); exists > 0 {
            response.Unauthorized(c, errcode.ErrTokenRevoked)
            c.Abort()
            return
        }
        
        // 4. 首次登录改密检查
        if claims.MustChangePassword {
            // 只允许访问改密接口
            if c.Request.URL.Path != "/api/v1/auth/password/update" {
                response.Forbidden(c, errcode.ErrPasswordChangeRequired)
                c.Abort()
                return
            }
        }
        
        // 5. 注入用户信息到 context
        c.Set("userID", claims.UserID)
        c.Set("username", claims.Username)
        c.Set("jti", claims.JTI)
        c.Set("must_change_password", claims.MustChangePassword)
        c.Next()
    }
}
```

### Casbin 中间件（g 表消除）

> 详见 [modules/authz.md](../modules/authz.md) §2.2。不使用 Casbin g 表，中间件层 BFS 展开角色后逐个 enforce。

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

---

## 测试用例

### JWT 中间件

| 用例 | 请求 | 预期 |
|------|------|------|
| 无 Token | 不带 Authorization header | 401 |
| Token 格式错误 | `Bearer 乱字符串` | 401 |
| Token 过期 | 过期的 AT | 401 |
| Token 被黑名单 | 登出后的 AT | 401 |
| Token 有效 | 正常 AT | 放行，context 有 userID |

### Casbin 中间件

| 用例 | 角色 | 请求 | 预期 |
|------|------|------|------|
| admin 角色 | admin | 任意 | 放行 |
| 有权限 | user_manager | `GET /users` | 放行 |
| 无权限 | user_viewer | `POST /users` | 403 |
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

| 用例 | 请求 | 预期 |
|------|------|------|
| 预检请求 | OPTIONS + Origin | 204 + CORS headers |
| 正常请求 | GET + Origin | 200 + Access-Control-Allow-Origin |
| 不允许的方法 | PUT + Origin | 不返回 CORS headers（仅允许 GET/POST/OPTIONS） |

---

## 涉及文件

```
internal/middleware/jwt.go             # JWT 中间件（已有，需完善黑名单 + must_change_password 拦截）
internal/middleware/casbin.go          # Casbin 中间件（g 表消除，逐角色 enforce，RoleFetcher 接口）
internal/middleware/recovery.go        # Recovery 中间件（已有，包装 gin.Recovery + slog）
internal/middleware/security.go        # 安全头中间件（已有，5 个 header）
internal/middleware/body_limit.go      # 请求体限制中间件（已有）
internal/middleware/audit.go           # 操作日志中间件（需创建）
# 以下使用 gin-contrib 库，无需自写文件：
# - RequestID  → github.com/gin-contrib/requestid
# - CORS       → github.com/gin-contrib/cors
# - AccessLogger → github.com/gin-contrib/slog
```
