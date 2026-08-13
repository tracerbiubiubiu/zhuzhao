# 认证模块设计

> 模块代码：`internal/service/auth_service.go` + `internal/pkg/jwt/`
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

## 2. 数据模型

### 2.1 Redis 存储结构

```
# RT 存储（每设备独立）
refresh:{userId}:{deviceId} → {rtValue, deviceInfo, createdAt}
TTL: 7d

# AT 黑名单
blacklist:at:{jti} → 1
TTL: AT 剩余有效期

# 设备列表
devices:{userId} → SET {deviceId1, deviceId2, ...}

# 登录限流
lock:login:{username} → {failCount, lockedUntil}
TTL: 15min（锁定后）

# 权限缓存
perm:user:{userId} → {roles, org_id, permissions}
TTL: 30min
```

### 2.2 JWT Payload

```json
{
  "user_id": "00000000-0000-0000-0000-000000000020",
  "jti": "a1b2c3d4e5f6",
  "exp": 1234567890,
  "iat": 1234560890
}
```

---

## 3. 接口定义

```go
type AuthService interface {
    // 登录
    Login(ctx context.Context, req LoginRequest) (*TokenPair, error)

    // 刷新 Token（RT 轮换）
    Refresh(ctx context.Context, rt string) (*TokenPair, error)

    // 登出
    Logout(ctx context.Context, at string) error

    // 踢出设备
    KickDevice(ctx context.Context, userID, deviceID string) error

    // 查询设备列表
    ListDevices(ctx context.Context, userID string) ([]DeviceInfo, error)
}

type LoginRequest struct {
    Username string
    Password string
    DeviceID string  // 可选，不传则生成
    DeviceInfo string // UA / IP
}

type TokenPair struct {
    AccessToken  string
    RefreshToken string
    ExpiresIn    int64 // AT 过期秒数
}
```

---

## 4. 核心流程

### 4.1 登录流程

```
POST /api/v1/auth/login {username, password}

1. 登录限流检查
   → Redis: lock:login:{username}
   → 已锁定？返回 429

2. 查询用户
   → user.GetByUsername(username)
   → 不存在？返回 401（与密码错误相同响应，防枚举）

3. 验证密码
   → bcrypt.Compare(password, user.PasswordHash)
   → 失败？INCR lock:login:{username}，返回 401
   → 达 5 次？SET locked_until，返回 429

4. 检查用户状态
   → user.Status != 1？返回 403

5. 签发 Token
   → AT: JWT(user_id + jti + exp), TTL=2h
   → RT: 随机 UUID, TTL=7d

6. 存储
   → Redis SET refresh:{userId}:{deviceId} = rt, TTL=7d
   → Redis SADD devices:{userId} = deviceId

7. 清除登录限流
   → Redis DEL lock:login:{username}

8. 返回 {accessToken, refreshToken, expiresIn}
```

### 4.2 RT 轮换流程

```
POST /api/v1/auth/refresh {refreshToken}

1. 验证 RT
   → 查 Redis: refresh:{userId}:{deviceId}
   → 不存在/不匹配？返回 401

2. 原子替换（Lua 脚本）
   → DEL 旧 RT + SET 新 RT（原子操作，防并发刷新）

3. 签发新 Token 对
   → 新 AT + 新 RT

4. 返回 {accessToken, refreshToken, expiresIn}
```

**并发刷新防护**：用 Redis Lua 脚本保证"删旧 RT + 写新 RT"原子完成。两个并发请求用同一 RT 刷新，只有第一个成功，第二个返回 401 `token_already_refreshed`。

### 4.3 登出流程

```
POST /api/v1/auth/logout
Authorization: Bearer {accessToken}

1. 解析 AT，提取 jti
2. Redis SET blacklist:at:{jti} = 1, TTL = AT 剩余有效期
3. Redis DEL refresh:{userId}:{deviceId}
4. Redis SREM devices:{userId} = deviceId
5. 返回 200
```

### 4.4 DeleteUser 级联吊销

旧系统已知问题：DeleteUser 时 Redis `RemoveByUid` 是 no-op，AT 靠 TTL 自然失效。

**新框架方案**：

```
DeleteUser 时：
  1. 查 Redis: devices:{userId} → 获取所有 deviceId
  2. 遍历 deviceId，DEL refresh:{userId}:{deviceId}
  3. DEL devices:{userId}
  4. 设置 user:disabled:{userId} = 1（永久标记）
  5. JWT 中间件检查 user:disabled:{userId}，命中则拒绝
```

---

## 5. 登录安全（借鉴旧系统 LoginLocker）

### 5.1 限流策略

| 配置项 | 值 | 说明 |
|--------|---|------|
| 最大失败次数 | 5 | 连续 5 次密码错误 |
| 锁定时长 | 15min | 锁定期间拒绝登录 |
| 计数窗口 | 15min | 失败计数 TTL |
| Redis 故障 | fail-close | 返回 503 |

### 5.2 Lua 原子脚本

```lua
-- KEYS[1] = lock:login:{username}
-- ARGV[1] = max_fails (5)
-- ARGV[2] = lock_ttl (900)

local fails = redis.call('INCR', KEYS[1])
if fails == 1 then
    redis.call('EXPIRE', KEYS[1], ARGV[2])
end
if fails >= tonumber(ARGV[1]) then
    redis.call('SET', KEYS[1]..':locked', '1', 'EX', ARGV[2])
end
return fails
```

---

## 6. 旧系统借鉴

| 设计 | 决策 | 理由 |
|------|------|------|
| LoginLocker Lua 原子脚本 | ✅ 直接采用 | 经过验证的成熟设计 |
| fail-close 策略 | ✅ 直接采用 | 安全优先 |
| 防用户枚举 | ✅ 直接采用 | 不泄露用户是否存在 |
| RSA 4096 签名 | ❌ Phase 1 用 HS256，Phase 2 切 RS256 | 单体用对称最简，微服务用非对称安全 |
| RT 存 MongoDB | ❌ 改用 Redis | 减少依赖，Redis + AOF 持久化足够 |
| AT 5min | ❌ 改用 2h | 权限走缓存不需要频繁刷新 |
| DeleteUser RemoveByUid no-op | ❌ 修复 | 用 user:disabled:{userId} 标记 |
| first_login 改密 | ⏳ Phase 2 | 借鉴旧系统，非首期必须 |

---

## 7. 分阶段实施

### Phase 1

- 账号密码登录 + 双 Token 签发
- RT 存 Redis + 轮换
- 登出 + AT 黑名单
- JWT 中间件（验证 + 黑名单检查）
- 基础登录限流（INCR + EXPIRE，非 Lua）

### Phase 2

- LoginLocker Lua 原子脚本
- 多设备管理（设备列表 + 踢出）
- first_login 强制改密
- DeleteUser 级联吊销（user:disabled 标记）

### Phase 3

- 异地登录检测
- CAPTCHA 验证码
| RSA 签名（微服务化时） | Phase 2 切 RS256 + JWKS |
