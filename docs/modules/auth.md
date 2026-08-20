# 认证模块设计

> 模块代码（目标路径，见 [architecture §3.5](../design/architecture.md#35-领域模块目录约定单仓可拆分)）：`internal/service/auth/`、`internal/handler/auth/`、`internal/pkg/jwt/`
>
> 旧系统参考：`doc/module-assessment-2026-08/authenticator.md`

---

## 1. 模块定位

**核心底座模块**。认证入口，负责用户登录、JWT 签发/验证/刷新/吊销、暴力破解防护。

与其他模块的关系：
- 依赖 `user` 模块验证密码
- 依赖 `middleware` 提供 JWT 中间件
- 签发的 AT 被 `authz` 模块用于鉴权

---

## 2. 认证安全原则

> **凭证互斥、非法请求处理、日志/审计**：SSOT 为 [phase1/02-auth.md §凭证优先级与混用规则](../phase1/02-auth.md#凭证优先级与混用规则) 与 [§非法认证请求的处理](../phase1/02-auth.md#非法认证请求的处理实现必读)。架构摘要见 [architecture.md §5.4](../design/architecture.md#非法与混用凭证authn-拒绝原则)。

| 原则 | Phase 1 |
|------|---------|
| 密码仅用于 `/auth/login`（`employee_no` + password） | ✅ |
| 业务 API 仅 Bearer JWT | ✅ |
| JWT 与 AK/SK 互斥（混用 400/20008） | 预留检测；M2M 未启用 |
| AuthN 失败 Abort，不进 Casbin | ✅ 中间件实现 |
| 鉴权链 Redis 故障 503 | ✅ fail-close |
| AK 验签失败 / 重放写 Warn + 可选 audit | M2M 上线后 |

---

## 3. 数据模型

### 3.1 Redis 存储结构

```
# RT 存储（每设备独立）
refresh:{userId}:{deviceId} → {rtValue, deviceInfo, createdAt}
TTL: 7d

# AT 黑名单
blacklist:at:{jti} → 1
TTL: AT 剩余有效期

# 用户禁用/删除后即时吊销（JWT 中间件检查）
user:disabled:{userId} → 1
TTL: AT TTL（30min，覆盖已签发 AT）

# 设备列表
devices:{userId} → SET {deviceId1, deviceId2, ...}

# 登录限流（Phase 1：Lua LoginLocker，15min/5 次）
lock:login:{employee_no} → failCount（TTL 15min）
TTL: 15min

# 权限缓存（Phase 3 / 按需，Phase 1 不使用）
# perm:user:{userId} → {roles, org_id, permissions}
# TTL: 30min
```

### 3.2 JWT Payload

```json
{
  "uid": "1",
  "username": "admin",
  "jti": "a1b2c3d4e5f6",
  "mcp": false,
  "exp": 1234567890,
  "iat": 1234560890
}
```

Claims 字段以 [phase1/02-auth.md](../phase1/02-auth.md) 为准：`uid`（int64）、`username`、`jti`、`mcp`（must_change_password）。AT TTL **30 分钟**，不是 2 小时。

---

## 4. 接口定义

```go
type AuthService interface {
    // 登录
    Login(ctx context.Context, req LoginRequest) (*TokenPair, error)

    // 刷新 Token（RT 轮换）
    Refresh(ctx context.Context, rt string) (*TokenPair, error)

    // 登出
    Logout(ctx context.Context, at string) error

    // 踢出设备
    KickDevice(ctx context.Context, userID int64, deviceID string) error

    // 查询设备列表
    ListDevices(ctx context.Context, userID int64) ([]DeviceInfo, error)
}

type LoginRequest struct {
    EmployeeNo string
    Password   string
    DeviceID   string  // 可选，不传则生成
    DeviceInfo string // UA / IP
}

type TokenPair struct {
    AccessToken  string
    RefreshToken string
    ExpiresIn    int64 // AT 过期秒数
}
```

---

## 5. 核心流程

### 5.1 登录流程

```
POST /api/v1/auth/login {employee_no, password}

1. 登录限流检查
   → Redis: lock:login:{employee_no}
   → 已锁定？返回 429

2. 查询用户
   → userRepo.FindByEmployeeNo(employee_no)
   → 不存在或无工号？返回 401（与密码错误相同响应，防枚举）

3. 验证密码
   → bcrypt.Compare(password, user.PasswordHash)
   → 失败？Eval LoginLocker Lua（INCR+EXPIRE 原子）；达阈值 → 429
   → count > 5？返回 429

4. 检查用户状态
   → user.Status != 1？返回 401（与密码错误相同文案，防枚举；见 phase1/02-auth）

5. 签发 Token
   → AT: JWT(uid + username + jti + mcp), TTL=30min，HS256
   → RT: 随机串, TTL=7d

6. 存储
   → Redis SET refresh:{userId}:{deviceId} = rt, TTL=7d
   → Redis SADD devices:{userId} = deviceId

7. 清除登录限流
   → Redis DEL lock:login:{employee_no}

8. 返回 {accessToken, refreshToken, expiresIn}
```

### 5.2 RT 轮换流程

```
POST /api/v1/auth/refresh {refreshToken}

1. 验证 RT
   → 查 Redis: refresh:{userId}:{deviceId}
   → 不存在/不匹配？返回 401 + 20004

2. 会话仍有效（禁用/删除兜底，见 §5.4）
   → EXISTS user:disabled:{userId} 或 users.status=禁用
   → 401 + 20004，不签发新 Token（与 RT 失效同一对外语义）

3. 原子替换（`GETDEL`，Redis 6.2+）
   → 删旧 RT；返回空则已被刷新 → 401 + 20004

4. 签发新 Token 对
   → 新 AT + 新 RT

5. 返回 {accessToken, refreshToken, expiresIn}
```

**并发刷新防护**：Phase 1 用 `GETDEL`（不必 Lua）。两个并发请求用同一 RT 刷新，只有第一个成功，第二个返回 401。

### 5.3 登出流程

```
POST /api/v1/auth/logout
Authorization: Bearer {accessToken}

1. 解析 AT，提取 jti
2. Redis SET blacklist:at:{jti} = 1, TTL = AT 剩余有效期
3. Redis DEL refresh:{userId}:{deviceId}
4. Redis SREM devices:{userId} = deviceId
5. 返回 200
```

### 5.4 DeleteUser 级联吊销

旧系统已知问题：DeleteUser 时 Redis `RemoveByUid` 是 no-op，AT 靠 TTL 自然失效。

**新框架方案**：

```
DeleteUser / 禁用用户 时：
  1. 查 Redis: devices:{userId} → 获取所有 deviceId（若有）
  2. 遍历 deviceId，DEL refresh:{userId}:{deviceId}；或 SCAN + DEL refresh:{userId}:*
  3. DEL devices:{userId}（Phase 2）
  4. SET user:disabled:{userId} = 1, TTL = AT TTL（30min）
  5. AT：JWT 中间件检查 user:disabled → 403 + 30003
  6. RT：Refresh 内 RT 不存在 → 401 + 20004；兜底再查 user:disabled/DB status → 401 + 20004
```

---

## 6. 登录安全（借鉴旧系统 LoginLocker）

### 6.1 限流策略

| 配置项 | 值 | 说明 |
|--------|---|------|
| 最大失败次数 | 5 | 连续 5 次密码错误 |
| 锁定时长 | 15min | 锁定期间拒绝登录 |
| 计数窗口 | 15min | 失败计数 TTL |
| Redis 故障 | fail-close | 返回 503 |

### 6.2 Phase 1 实现：Lua LoginLocker

`INCR` + 首次 `EXPIRE` 在 **一条 Lua 脚本**中原子完成；达 5 次返回 429。脚本见 [phase1/02-auth.md §登录限流](../phase1/02-auth.md#登录限流phase-1-必须)。Redis 故障 **fail-close**（503）。

```go
// 失败路径：Eval(loginLockScript, []string{key}, windowSec, maxFail)
// 成功路径：Del(lock:login:{employee_no})
```

---

## 7. 旧系统借鉴

| 设计 | 决策 | 理由 |
|------|------|------|
| LoginLocker Lua 原子脚本 | ✅ Phase 1 采用 | INCR+EXPIRE 原子，消除竞态 |
| fail-close 策略 | ✅ 直接采用 | 鉴权链路 Redis 故障返回 503 |
| 防用户枚举 | ✅ 直接采用 | 不泄露用户是否存在 |
| RSA 4096 签名 | ❌ Phase 1 HS256；RS256+JWKS 后移 Phase 3 | 单体对称最简，拆服务再用非对称 |
| RT 存 MongoDB | ❌ 改用 Redis | 减少依赖，Redis + AOF 持久化足够 |
| AT 5min | ❌ 改用 30min | Phase 1 无权限缓存，30min 平衡安全与体验 |
| DeleteUser RemoveByUid no-op | ❌ Phase 1 修复 | `user:disabled:{userId}` + 删全部 RT |
| first_login 改密 | ✅ Phase 1 | `must_change_password` + JWT `mcp` |

---

## 8. 分阶段实施

### Phase 1

- 账号密码登录 + 双 Token 签发（AT 30min HS256）
- RT 存 Redis + `GETDEL` 轮换
- 登出 + AT 黑名单
- JWT 中间件（验证 + 黑名单 + `user:disabled`，Redis 故障 503）
- 登录限流（Lua LoginLocker，15min/5 次）
- 登录成功/失败写审计
- `must_change_password` 强制改密
- 禁用/删除用户后吊销会话

### Phase 2

- 多设备管理 UI（设备列表 + 踢出）— 见 [phase2/01-auth-enhance.md](../phase2/01-auth-enhance.md)
- 密码复杂度策略 — 同上

### Phase 3

- 异地登录检测
- CAPTCHA 验证码
- RS256 + JWKS（拆服务时）
