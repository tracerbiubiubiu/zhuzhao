# 02 - 认证模块（auth）

> Step 3，依赖 Step 2（user repo）。Phase 1 的核心模块。

---

## 预期功能

| 功能 | 场景 | API |
|------|------|-----|
| 登录 | 用户输入账号密码，系统验证后签发双 Token | `POST /api/v1/auth/login` |
| Token 刷新 | AT 过期，前端用 RT 换新 AT + 新 RT（轮换） | `POST /api/v1/auth/refresh` |
| 登出 | 用户主动登出，AT 加入黑名单，RT 删除 | `POST /api/v1/auth/logout` |
| 修改密码 | 用户修改自己的密码，旧密码验证后更新 | `POST /api/v1/auth/password/update` |
| 管理员重置密码 | superadmin 重置任意用户密码，用户首次登录强制改密 | `POST /api/v1/users/:id/password/reset` |
| 首次登录改密 | 被重置密码的用户登录后强制修改密码 | 登录时检测 `must_change_password` 标记 |
| 登录限流 | 同一用户名短时间失败次数超限则 429 | 中间件/AuthService 内 Redis INCR |
| 会话吊销 | 禁用/删除用户后已签发 AT 立即不可用 | Redis `user:disabled:{userId}` |

### Phase 1 不做

| 功能 | 原因 | 阶段 |
|------|------|------|
| 多设备管理（UI） | 允许用户多设备登录，但 Phase 1 不提供设备管理界面 | Phase 2 |
| 设备踢出 | 需要 Redis 设备列表管理 | Phase 2 |
| 登录锁定（Lua） | Phase 1 用 INCR+EXPIRE 限流，不引入 Lua | Phase 2 |
| 邮件/短信密码重置 | 需要邮件/短信通道，Phase 1 用管理员重置替代 | Phase 2 |
| 验证码 | 需要图形验证码生成 | Phase 2 |
| AK/SK | 无服务间调用方 | 有 M2M 需求时 |

---

## 核心设计思路

### 双 Token 机制

```
登录成功
  │
  ├── 签发 Access Token (AT)
  │   ├── 算法：HS256
  │   ├── TTL：30min
  │   ├── Claims：userID, username, jti (唯一 ID)
  │   └── 不存 Redis（纯无状态）
  │
  └── 签发 Refresh Token (RT)
      ├── 算法：HS256
      ├── TTL：7d
      ├── Claims：userID, jti, deviceID
      └── 存 Redis：key = "refresh:{userId}:{deviceId}", value = tokenHash
```

### 多设备登录策略

Phase 1 **允许用户多设备同时登录**，但不提供设备管理 UI。

```
设备 A 登录 → refresh:user123:deviceA = RT_A
设备 B 登录 → refresh:user123:deviceB = RT_B   ← 互不干扰
设备 A 刷新 → 只删 RT_A，不影响 RT_B
```

**deviceId 的生成**：前端生成 UUID 存 localStorage，每次登录带上。Phase 1 不校验 deviceId 的合法性，仅作为 Redis Key 的隔离维度。

**登出策略**：Phase 1 登出只删当前设备的 RT。Phase 2 提供设备列表查询和踢出功能。

### RT 轮换流程

```
前端用 RT 请求 /auth/refresh
  │
  ├── 解析 RT，提取 userID + deviceId
  ├── 查 Redis：refresh:{userId}:{deviceId} 是否存在
  │   ├── 不存在 → 401（RT 已失效或被登出）
  │   └── 存在 → 继续
  ├── GetDel 原子删除旧 RT（旧 RT 立即失效）
  ├── 签发新 AT + 新 RT
  └── 存新 RT 到 Redis：refresh:{userId}:{deviceId}
```

> Phase 1 用 `GetDel`（Redis 6.2+ 原子命令）替代 Lua 脚本，简化实现。Phase 2 如需更复杂的原子操作再引入 Lua。

### RT 存储位置：Redis vs 数据库

#### 业界做法

业界普遍采用两种方案，各有取舍：

| 方案 | 代表产品 | 优点 | 缺点 |
|------|---------|------|------|
| **仅 Redis** | 多数中小型 SaaS | 低延迟、TTL 自动过期、实现简单 | Redis 重启数据丢失（除非 AOF） |
| **仅 DB** | 传统企业系统 | 持久化、可审计、ACID 事务 | 每次刷新查 DB、需定期清理过期记录 |
| **DB + Redis 混合** | 大型平台（Auth0 等） | 两全其美：DB 持久 + Redis 加速 | 架构复杂、双写一致性 |

参考 WorkOS、Authgear 等身份平台的实践：
- RT 本质是服务端可控的会话凭证，不同于无状态 JWT
- 存储 RT 不破坏 JWT 无状态性，因为 AT 仍然是无状态验证
- RT 的核心需求是**可撤销**和**可轮换**，这必然需要服务端存储

#### Phase 1 决策：仅用 Redis

**理由**：

1. **TTL 原生支持**：RT 有效期 7 天，Redis `SET ... EX` 自动过期，无需定时清理任务
2. **轮换原子性**：`GetDel` 一条命令完成"删除旧 RT + 返回值"，天然防并发
3. **登录已有 Redis 依赖**：AT 黑名单、权限缓存都需要 Redis，不额外引入依赖
4. **Phase 1 流量低**：单实例部署，Redis 不可用的概率极低
5. **持久化兜底**：配置 Redis AOF（`appendfsync everysec`），最多丢 1 秒数据

**Redis 重启的影响**：
- 所有 RT 失效，用户刷新 AT 时返回 401，前端跳转登录页
- AT 仍然有效（无状态 JWT），在 AT 过期前用户无感知
- AT 短期有效（15-30 分钟），影响窗口有限
- **结论**：可接受的故障，不值得为 Phase 1 引入 DB + Redis 双写复杂度

#### Phase 2 演进方向

如果后续对 RT 可靠性要求更高（多实例、高可用），可平滑迁移到 DB + Redis 混合：
- DB（PostgreSQL）作为持久存储，存 RT 的 hash（SHA-256）
- Redis 作为热缓存，加速查询
- 轮换时先写 DB 再更新 Redis（write-through）
- 参考文档：[modules/auth.md](../modules/auth.md) §2.2

### 登出黑名单

```
POST /auth/logout (携带 AT)
  │
  ├── 解析 AT，提取 jti + 过期时间
  ├── 将 jti 加入 Redis 黑名单：key = "blacklist:at:{jti}", TTL = AT 剩余有效期
  ├── DEL 该用户当前设备的 RT：refresh:{userId}:{deviceId}
  └── 返回成功
```

### 登录限流（Phase 1 必须）

不引入 Lua。同一 `username` 连续失败：

```
key = lock:login:{username}
INCR key
首次 INCR 时 EXPIRE 15min
count > 5 → 返回 429（文案不区分用户是否存在）
登录成功 → DEL key
```

阈值：15 分钟内 5 次失败。Phase 2 再升级为 Lua 原子脚本 + 账号锁定。

### 会话吊销（禁用/删除用户）

JWT 无状态，仅靠 AT 黑名单无法覆盖「该用户所有设备上的未过期 AT」。Phase 1 增加用户级拒绝标记：

```
禁用或删除用户成功后：
  SET user:disabled:{userId} 1  EX AT_TTL（30min 足够覆盖已签发 AT）
  DEL refresh:{userId}:*        （该用户全部 RT）

JWT 中间件在黑名单检查之后：
  EXISTS user:disabled:{userId} → 401
```

登录成功时若该 key 仍在（管理员刚解禁），`DEL user:disabled:{userId}`。

### Redis 故障策略：fail-close

鉴权链路上 Redis 不可用时：

| 操作 | 行为 |
|------|------|
| 登录（写 RT） | 503，不签发 Token |
| 刷新（GetDel RT） | 503 |
| JWT 黑名单 / user:disabled 查询失败 | 503，不放行 |
| 登出（写黑名单） | 503 |

原则：管理后台宁可暂时不可用，也不能在 Redis 宕机时让已吊销 Token 继续访问。错误日志记录原因，响应体不返回 Redis 内部错误。

### 登录审计

`/auth/login` 不走鉴权组上的 AuditLog 中间件，由 AuthService 显式写入：

- 成功：username、user_id、ip、user_agent
- 失败：username（原文）、ip、原因码（密码错误/限流），**不记密码**

### 登录成功副作用

登录成功后更新 `users.last_login_at`、`users.last_login_ip`（失败不更新）。

### JWT Claims 设计（Phase 1 极简）

```go
type AccessClaims struct {
    UserID             int64  `json:"uid,string"`
    Username           string `json:"username"`
    JTI                string `json:"jti"`
    MustChangePassword bool   `json:"mcp,omitempty"`  // 首次登录改密标记
    jwt.RegisteredClaims
}
```

Phase 1 不在 JWT 中存角色/权限信息（保持无状态，权限走 Casbin 内存 enforcer）。

> `must_change_password` 放 JWT Claims 是合理的——它是一个一次性的安全约束，不是需要实时更新的权限信息。用户改密后重新签发 AT，该标记自动消失。中间件检查该标记，拦截非改密接口。

### Redis Key 设计

> 命名对齐 [modules/auth.md](../modules/auth.md) §2.1。

| Key | 用途 | TTL |
|-----|------|-----|
| `refresh:{userId}:{deviceId}` | RT 存储（每设备独立） | 7d |
| `blacklist:at:{jti}` | AT 黑名单 | AT 剩余有效期 |
| `user:disabled:{userId}` | 用户级拒绝（禁用/删除） | 30min（覆盖 AT TTL） |
| `lock:login:{username}` | 登录失败计数 | 15min |
| `devices:{userId}` | 设备列表（SET，Phase 2） | 持久（跟随用户） |
| `perm:user:{userId}` | 权限缓存（工单跑通后按需） | 30min |

### 管理员重置密码 + 首次登录强制改密

#### 设计思路

业界最佳实践（PCI DSS 4.0 §8.3.5、NIST SP 800-63B）要求：管理员重置密码后，必须强制用户首次登录时修改密码，确保只有用户本人知道最终密码。Phase 1 不引入邮件/短信通道，采用管理员重置方案。

#### 数据模型

```sql
-- users 表增加字段
ALTER TABLE users ADD COLUMN must_change_password BOOLEAN NOT NULL DEFAULT false;
```

#### 权限分级

| 操作 | 允许的角色 | 说明 |
|------|-----------|------|
| 重置普通用户密码 | `superadmin`, `admin` | admin 只能重置普通用户 |
| 重置 admin 密码 | `superadmin` 仅此一个 | admin 不能重置同级或更高级 |
| 用户修改自己的密码 | 登录用户本人 | 旧密码验证通过即可 |

> **角色分级**：superadmin > admin > operator > viewer。superadmin 是系统内置角色（`is_system = true`），无法被删除或禁用，确保始终有一个"最高权限"入口。Casbin matcher 中 superadmin 和 admin 都直接 bypass（`r.sub == "role::superadmin" || r.sub == "role::admin"`），区别在业务逻辑层的权限分级校验。

#### 重置流程

```
1. superadmin 调用 POST /api/v1/users/:id/password/reset
2. 后端校验：当前用户角色 ≥ 目标用户角色（防止 admin 重置 superadmin）
3. 生成临时密码（cryptographically secure random）
4. bcrypt 加密后写入 users 表，同时设置 must_change_password = true
5. 返回临时密码给 superadmin（仅此一次，不入库明文）
6. 审计日志记录：谁、重置了谁、何时
```

#### 首次登录改密流程

```
1. 用户用临时密码登录
2. 登录成功后检查 must_change_password 标记
3. 若为 true：
   a. 签发 AT（但标记 password_pending = true）
   b. 前端检测到该标记，跳转到强制改密页面
   c. 用户修改密码（需满足密码策略）
   d. 后端：更新密码 + 清除 must_change_password 标记
   e. 前端跳转到主页
4. 若标记期间用户尝试访问其他接口：返回 403 + 错误码 PASSWORD_CHANGE_REQUIRED
```

#### 安全要点

- 临时密码使用 `crypto/rand` 生成，最少 16 位，包含大小写+数字+符号
- 重置后不自动失效已有 session（Phase 2 可扩展：重置后踢出所有设备）
- superadmin 重置密码操作全部记入审计日志
- 临时密码不落库明文，只存 bcrypt hash

### 服务间认证（M2M）：Phase 1 不做

无第二个系统调用本 API，Phase 1 **不建 `api_credentials` 表、不写 AK/SK 中间件**。签名方案（HMAC 覆盖 method+path+body+timestamp+nonce）见下文，等出现真实调用方再实现。

以下设计保留为后续实现参考，不进入 Phase 1 迁移和代码。

#### `qingtao/aksk` 项目评估

对 `/Users/bujibuji/code/src/github.com/qingtao/aksk`（`github.com/qingtao/aksk/v2`）进行了详细分析：

**优点**：
- MIT 许可，可导入的纯库（无 main 包），不引入额外服务
- 支持多种签名算法：HMAC-SHA256/384/512、RSA、ECDSA、Ed25519
- 支持时间戳防重放（±60s 可配置）+ 可选 nonce 防重放
- 公钥缓存带 `singleflight` 去重，10 分钟 TTL
- `KeyGetter` 函数类型让存储层完全解耦

**关键安全缺陷**：
- ❌ **签名不覆盖请求体、HTTP 方法、路径、查询参数**——只签 `ak,timestamp,nonce`
- 这意味着攻击者截获有效签名后，可以在时间窗口内**伪造不同的请求**（改方法、改路径、改 body）
- 本质上只是"认证"（证明持有 SK），不是"请求完整性保护"
- 与 AWS Signature V4 相比差距很大（AWS 签名覆盖 method + path + query + headers + body hash）

**其他限制**：
- nonce 存储只有内存版（`freecache`），多实例部署需自实现 Redis 版
- 签名元素用逗号拼接，AK 不能包含逗号
- 无内置 AK/SK 生成、轮换、管理工具

#### 决策：Phase 1 自研签名逻辑，不引入 `qingtao/aksk`

**理由**：
1. 请求完整性是 M2M 认证的核心需求——不覆盖 body 和 path 的签名不可接受
2. 自研签名逻辑不复杂——HMAC-SHA256 + `method\npath\ntimestamp\nnonce\nsha256(body)` 约 50 行代码
3. `qingtao/aksk` 的 `KeyGetter` 和 nonce 设计思路可借鉴，但不直接引用
4. Phase 1 **不建表、不写中间件**。签名方案仅作后续参考。

#### 为什么选 AK/SK 而不是 OAuth2 Client Credentials

| 维度 | AK/SK 签名 | OAuth2 Client Credentials |
|------|-----------|--------------------------|
| 依赖 | 无额外服务（DB 存 AK/SK 即可） | 需要 Authorization Server |
| 复杂度 | 中（签名算法 + 时间戳防重放） | 高（Token 签发 + 刷新 + 撤回） |
| 无状态 | 是（签名自验证，无需查 DB） | 否（需查 Token 有效性或依赖 JWT） |
| 撤回 | 禁用 AK 即时生效 | 需维护 Token 黑名单 |
| 业界验证 | AWS、阿里云、腾讯云、华为云 | Auth0、Keycloak |
| Phase 1 适配 | 可预留表 + 中间件骨架 | 过重 |

**结论**：有真实 M2M 调用方时再实现 AK/SK（表 + 中间件）。Phase 1 不做；拆服务也不必然在 Phase 2 启用。

#### 认证流程

```
调用方                              被调用方（本系统）
  │                                    │
  │  1. 构造请求                       │
  │     - method, path, body           │
  │     - timestamp (Unix秒)           │
  │     - nonce (随机串，防重放)        │
  │                                    │
  │  2. 计算签名                       │
  │     string_to_sign = method\n      │
  │                     path\n         │
  │                     timestamp\n    │
  │                     nonce\n        │
  │                     sha256(body)   │
  │     signature = HMAC-SHA256(       │
  │         SK, string_to_sign)        │
  │                                    │
  │  3. 发送请求 ──────────────────────>│
  │     Header:                        │
  │       X-AK-Access-Key: {AK}        │
  │       X-AK-Timestamp: {timestamp}  │
  │       X-AK-Nonce: {nonce}          │
  │       X-AK-Signature: {signature}  │
  │                                    │
  │                    4. 验证签名      │
  │                       - 查 AK → SK │
  │                       - 检查时间偏移 (±5min) │
  │                       - 检查 nonce 未用过（Redis，TTL=10min）│
  │                       - 重算 HMAC 比对 │
  │                                    │
  │  <────────────────── 5. 返回结果    │
```

#### 数据模型

```sql
CREATE TABLE api_credentials (
    id              BIGSERIAL PRIMARY KEY,
    access_key      VARCHAR(64) NOT NULL UNIQUE,   -- AK，如 "ak_xxxxxxxxxxxx"
    secret_key      VARCHAR(128) NOT NULL,          -- SK，bcrypt 存储；验签用原文，需可逆 → AES 加密存储
    name            VARCHAR(100) NOT NULL,          -- 凭证名称（如"工单系统"）
    description     TEXT,
    is_active       BOOLEAN DEFAULT TRUE,
    expires_at      TIMESTAMPTZ,                    -- 可选过期时间
    last_used_at    TIMESTAMPTZ,                    -- 最近使用时间
    created_at      TIMESTAMPTZ DEFAULT NOW(),
    updated_at      TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_api_credentials_ak ON api_credentials(access_key) WHERE is_active = TRUE;
```

> **SK 存储注意**：与密码不同，SK 需要可逆解密用于验签（密码只需单向验证）。Phase 1 用 AES-GCM 加密存储，密钥从配置读取。如嫌复杂，Phase 1 可先明文存 DB（仅开发环境），Phase 2 加密。

#### AK/SK 与 JWT 的关系

```
请求进入 → AuthN 中间件
              │
              ├── 检测 Authorization: Bearer xxx → JWT 路径（人类用户）
              │
              └── 检测 X-AK-Access-Key → AK/SK 路径（服务调用）
                    │
                    ├── 验签通过 → 注入 context:
                    │     auth_type = "m2m"
                    │     credential_id = {AK 对应的记录 ID}
                    │     credential_name = "工单系统"
                    │
                    └── 验签失败 → 401
```

两种认证方式共用 Casbin 鉴权层。AK/SK 对应的 Casbin subject 为 `service:{credential_name}`，可分配独立的策略。

#### Phase 1 做什么

| Phase 1 | 有 M2M 需求时 |
|---------|----------------|
| 不建表、不写中间件 | 建表 + 管理 API + 验签中间件 + nonce 防重放 |


---

## 测试用例

### 登录

| 用例 | 输入 | 预期 |
|------|------|------|
| 登录成功 | 正确账号密码 | 200 + AT + RT |
| 密码错误 | 正确账号 + 错误密码 | 401 + "用户名或密码错误" |
| 用户不存在 | 不存在的账号 | 401 + "用户名或密码错误"（同密码错误，防枚举） |
| 用户已禁用 | status=disabled 的用户 | 401 + "账号已禁用" |
| 连续失败超限 | 同一用户名第 6 次失败 | 429 |
| Redis 不可用 | Redis 宕机时登录 | 503 |
| 请求参数缺失 | 无 password | 400 + 参数校验错误 |

### Token 刷新

| 用例 | 输入 | 预期 |
|------|------|------|
| 刷新成功 | 有效 RT | 200 + 新 AT + 新 RT |
| 旧 RT 失效 | 使用已刷新过的旧 RT | 401 |
| 并发双刷新 | 同一 RT 同时两次 | 仅一次 200 |
| Redis 不可用 | Redis 宕机时刷新 | 503 |
| RT 过期 | 超过 7d 的 RT | 401 |
| RT 格式错误 | 乱字符串 | 401 |

### 登出

| 用例 | 输入 | 预期 |
|------|------|------|
| 登出成功 | 有效 AT | 200 |
| 登出后 AT 失效 | 登出后用同一 AT 请求 | 401 |
| 重复登出 | 已登出的 AT | 401 |
| Redis 不可用 | Redis 宕机时登出 | 503 |

### 修改密码

| 用例 | 输入 | 预期 |
|------|------|------|
| 修改成功 | 正确旧密码 + 新密码 | 200 |
| 旧密码错误 | 错误旧密码 | 401 |
| 新密码与旧密码相同 | 旧密码 = 新密码 | 400 |

### 管理员重置密码 + 首次登录改密

| 用例 | 输入 | 预期 |
|------|------|------|
| superadmin 重置普通用户 | superadmin 调用 reset | 200，返回临时密码 |
| admin 重置普通用户 | admin 调用 reset | 200，返回临时密码 |
| admin 重置 superadmin | admin 调用 reset | 403（越权） |
| admin 重置 admin | admin 调用 reset | 403（越权） |
| 重置后首次登录 | 临时密码 | 200，返回 AT + `must_change_password: true` |
| 首次登录后访问其他接口 | AT（password_pending） | 403 PASSWORD_CHANGE_REQUIRED |
| 首次登录后修改密码 | 临时密码 + 新密码 | 200，清除 `must_change_password` |
| 改密后正常访问 | AT（已改密） | 200 |

---

## 涉及文件

```
internal/service/auth_service.go     # Login/Refresh/Logout/UpdatePassword
internal/service/user_service.go     # ResetPassword（管理员重置）
internal/handler/auth_handler.go     # HTTP Handler
internal/handler/user_handler.go     # 管理员重置密码 Handler
internal/pkg/jwt/jwt.go              # JWT 签发与解析（已有，需完善）
internal/repository/user_repo.go     # 用户查询（密码验证依赖）
```

## 待决策点

> 以下决策已在讨论中确认：

- ✅ **密码复杂度**：Phase 1 仅 bcrypt cost=12，不增加复杂度校验。
- ✅ **RT 存储粒度**：支持多设备（Redis key 设计已预留），但不实现设备管理 UI。
- ✅ **RT 存储位置**：Phase 1 仅用 Redis（AOF 持久化兜底），不引入 DB + Redis 双写。Phase 2 可演进。
- ✅ **密码重置方式**：Phase 1 用管理员重置 + 首次登录强制改密，不引入邮件/短信通道。权限分级：superadmin 可重置所有用户，admin 只能重置普通用户。
- ✅ **多设备登录**：Phase 1 允许多设备同时登录，不做设备踢出，不提供设备管理 UI。
- ✅ **用户 ID 类型**：`BIGINT`/`int64`，JSON 加 `,string` tag（前端精度安全）。不用 UUID。
- ✅ **组织 ID / 编码**：ID 为 `BIGINT`；业务编码 `code` 为 `VARCHAR`（ltree 用 code）。
- ✅ **Casbin adapter**：直接上 PG adapter（`pckhoi/casbin-pgx-adapter/v3`）。
- ✅ **组织模块范围**：Phase 1 实现完整 CRUD。
- ✅ **AK/SK**：Phase 1 不做。签名方案保留，有调用方再实现。
- ✅ **登录限流**：Phase 1 用 INCR+EXPIRE，阈值 15min/5 次。
- ✅ **Redis 故障**：鉴权链路 fail-close（503）。
- ✅ **会话吊销**：禁用/删除用户写 `user:disabled:{userId}`。
