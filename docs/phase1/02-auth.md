# 02 - 认证模块（auth）

> **Step 3**，依赖 Step 2（user repo）。  
> `login` / `refresh` 为**公开路由**（M2）；`logout` / `password/update` 的 **AuthService 在本 Step**，Handler 挂在 JWT 组（**Step 4** 后可测）。

---

## 预期功能

> 认证接口不走菜单权限码。管理员重置密码见 [04-user](./04-user.md)（`user:reset_password`）。

| 功能 | 场景 | API | 权限码 |
|------|------|-----|--------|
| 登录 | 用户输入账号密码，系统验证后签发双 Token | `POST /api/v1/auth/login` | `—`（公开） |
| Token 刷新 | AT 过期，前端用 RT 换新 AT + 新 RT（轮换） | `POST /api/v1/auth/refresh` | `—`（持 RT） |
| 登出 | 用户主动登出，AT 加入黑名单，RT 删除 | `POST /api/v1/auth/logout` | `—` |
| 修改密码 | 用户修改自己的密码，旧密码验证后更新 | `POST /api/v1/auth/password/update` | `—` |
| 管理员重置密码 | superadmin 重置任意用户密码，用户首次登录强制改密 | `POST /api/v1/users/password/reset` | `user:reset_password` |
| 首次登录改密 | 被重置密码的用户登录后强制修改密码 | 登录时检测 `must_change_password` 标记 | `—` |
| 登录限流 | 同一工号短时间失败次数超限则 429 | AuthService 内 Redis **Lua**（LoginLocker） | — |
| 会话吊销 | 禁用/删除用户后已签发 AT 立即不可用 | Redis `user:disabled:{userId}` | — |

### Phase 1 不做

| 功能 | 原因 | 阶段 |
|------|------|------|
| 多设备管理（UI） | 允许用户多设备登录，但 Phase 1 不提供设备管理界面 | Phase 2 |
| 设备踢出 | 需要 Redis 设备列表管理 | Phase 2 |
| 登录锁定（Lua） | Phase 1 采用 Lua 原子脚本（见下方 §登录限流） | ✅ |
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
  │   ├── 不存在 → 401 + 20004（RT 已失效、登出或被吊销）
  │   └── 存在 → 继续
  ├── 【防御纵深】会话是否仍有效（见下方说明）
  │   ├── EXISTS user:disabled:{userId} → 401 + 20004，不签发新 Token
  │   └── 或查 DB users.status != 启用 → 401 + 20004（与 RT 失效同一对外语义）
  ├── GetDel 原子删除旧 RT（旧 RT 立即失效）
  ├── 签发新 AT + 新 RT
  └── 存新 RT 到 Redis：refresh:{userId}:{deviceId}
```

> Phase 1 **RT 刷新**用 `GetDel`（Redis 6.2+ 原子「读并删」），保证轮换原子性，**不必**为 Refresh 写 Lua。  
> **登录限流**仍用 **Lua LoginLocker**（`INCR` + 首次 `EXPIRE` 原子），见 §登录限流；与 Refresh 是不同场景。

> **与禁用/删除用户的关系**：吊销时 **DEL 全部 `refresh:{userId}:*`** 是主路径，Refresh 第一步即 401；`user:disabled` / DB 状态检查是兜底，防止删 RT 部分失败时仍换出新 AT。

> **RT Reuse Detection（业界对照）**：Auth0 / OWASP ASVS 推荐的完整方案在 RT 被重用时不仅拒绝该 token，还应**吊销整个 token family**（同一次登录产生的所有 RT 链）。Phase 1 用 `GetDel` 原子读删，旧 RT 再用会因 key 不存在而返回 401 + 20004，但**不追踪 family**（同一设备的 RT key 只有一个槽位，旧 RT 自然不可用，但不跨设备链）。这是**可接受的 Phase 1 安全级别**：单设备 RT 不可重用 + `user:disabled` 兜底；Phase 2+ 若需更强 reuse detection，可在 RT value 中嵌入 `family_id`，GetDel 命中后检查已用列表，发现重用则 DEL `refresh:{userId}:*` 全家桶。详见 [architecture §12.1 RT 并发刷新](../design/architecture.md#121-并发问题与应对)。

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
- AT 短期有效（**30min**），影响窗口有限
- **结论**：可接受的故障，不值得为 Phase 1 引入 DB + Redis 双写复杂度

#### Phase 2 演进方向

如果后续对 RT 可靠性要求更高（多实例、高可用），可平滑迁移到 DB + Redis 混合：
- DB（PostgreSQL）作为持久存储，存 RT 的 hash（SHA-256）
- Redis 作为热缓存，加速查询
- 轮换时先写 DB 再更新 Redis（write-through）
- 参考文档：[modules/auth.md](../modules/auth.md) §3.2

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

采用 **Redis Lua** 原子完成 `INCR` + 首次 `EXPIRE`，避免应用层两次往返之间的竞态（`INCR` 成功但未 `EXPIRE` 导致 key 永不过期）。逻辑借鉴旧系统 LoginLocker。

**参数**：同一 `employee_no`，**15 分钟**窗口内失败 **> 5 次** → **429 + 20006**；登录成功 → `DEL lock:login:{employee_no}`。

**Key**：`lock:login:{employee_no}`

**Lua 脚本（示意，实现时放 `internal/pkg/redis/scripts/login_lock.lua`，由 `scripts.go` go:embed）**：

```lua
-- KEYS[1] = lock:login:{employee_no}
-- ARGV[1] = window_sec (900)
-- ARGV[2] = max_fail   (5)
local n = redis.call('INCR', KEYS[1])
if n == 1 then
  redis.call('EXPIRE', KEYS[1], tonumber(ARGV[1]))
end
if n > tonumber(ARGV[2]) then
  return 1   -- 已超限，调用方返回 429
end
return 0     -- 未超限，继续校验密码；若最终失败无需再 INCR（本脚本已在失败路径调用）
```

**调用时机（推荐）**：

```
1. 登录入口（校验密码前）
   → GET lock:login:{employee_no}；若 count > 5 → 429 + 20006（已锁定，不泄露原因）

2. 密码错误
   → EVAL LoginLocker 脚本（INCR + 首次 EXPIRE 原子）
   → 脚本返回已超限 → 429 + 20006

3. 密码正确
   → DEL lock:login:{employee_no}
```

**Redis 故障**：`EVAL` / `GET` 失败 → **503 + 10008**（fail-close，不放行登录）。

> RT 刷新仍用 `GetDel`（不必 Lua）；登录限流与 RT 轮换是不同场景。

### 会话吊销（禁用/删除用户）

JWT 无状态，仅靠 AT 黑名单无法覆盖「该用户所有设备上的未过期 AT」。Phase 1 用 **AT 拒绝标记 + 删除全部 RT** 双轨吊销：

| 路径 | 机制 | 客户端表现 |
|------|------|------------|
| **AT** | `SET user:disabled:{userId}`，JWT 中间件 `EXISTS` | 带旧 AT 访问鉴权路由 → **403 + 30003** |
| **RT** | `DEL refresh:{userId}:*`（该用户全部设备） | `POST /auth/refresh` → **401 + 20004**，**不得**签发新 AT/RT |
| **Refresh 兜底** | Refresh 内再查 `user:disabled` 或 DB `status` | 同上 401 + 20004（与 RT 已删同一对外语义，防 DEL 部分失败） |

```
禁用或删除用户成功后（事务提交后，Redis 副作用）：
  SET user:disabled:{userId} 1  EX AT_TTL（30min，覆盖已签发 AT）
  DEL refresh:{userId}:*          （该用户全部 RT，禁止再用 RT 换新 AT）

JWT 中间件（鉴权路由）在黑名单检查之后：
  EXISTS user:disabled:{userId} → 403 + ErrUserDisabled（30003）

AuthService.Refresh（/auth/refresh，无 JWT）在 GetDel 之前：
  EXISTS user:disabled:{userId} 或 users.status=禁用 → 401 + ErrRefreshTokenInvalid（20004）
  （不返回 30003，避免在 refresh 路径暴露「账号已禁用」细节）
```

> **登录 vs 已签发 AT vs RT**：  
> - **登录时** `status=禁用` → **401 + 20001**（与密码错误同文案，防枚举）。  
> - **已登录后**管理员禁用 → 旧 **AT** → **403 + 30003**；旧 **RT** → **401 + 20004**。  
> - 解禁后登录成功 → `DEL user:disabled:{userId}`。

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

- 成功：employee_no、user_id、ip、user_agent
- 失败：employee_no（原文）、ip、原因码（密码错误/限流），**不记密码**

### 登录成功副作用

登录成功后更新 `users.last_login_at`、`users.last_login_ip`（失败不更新）。

**IP 来源**：Handler 传入 `c.ClientIP()`（Gin；Nginx 反代时依赖 `X-Forwarded-For` 与 `TrustedProxies` 配置，见 [architecture §Nginx](../design/architecture.md)）。值为客户端 perceived IP 字符串，写入 `VARCHAR(50)`。

### 登录与工号解析

`POST /auth/login` 使用 **`employee_no` + 密码**（不用 `username`）。工号有值时**全局唯一**（见 [04-user §唯一性](./04-user.md#身份标识字段工号-登录名-域账号)）。登录时：

```
SELECT id, password, ... FROM users
 WHERE employee_no = $1 AND deleted_at IS NULL
   AND employee_no IS NOT NULL AND employee_no <> ''
```

| 命中行数 | 行为 |
|----------|------|
| 0 | 401 + 20001（与密码错误同文案，防枚举） |
| 1 | 校验 bcrypt；成功则签发 Token |

**无工号账号**（`employee_no` 为空，典型 `source=local` 外包/测试号）：**不能**用工号登录 → 401 + 20001；须管理员补填工号或走 Phase 3 SSO。

管理端找人：`GET /users?employee_no=`（精确 0/1 条）或 `?username=`（模糊）。Phase 3 可扩展 `(user_domain, domain_account)` 联邦登录。

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

Phase 1 不在 JWT 中存角色/权限信息（保持无状态，权限走 Casbin SyncedEnforcer + PG adapter）。

> `must_change_password` 放 JWT Claims 是合理的——它是一个一次性的安全约束，不是需要实时更新的权限信息。用户改密后重新签发 AT，该标记自动消失。中间件检查该标记，拦截非改密接口。

### Redis Key 设计

> 命名对齐 [modules/auth.md](../modules/auth.md) §3.1。

| Key | 用途 | TTL |
|-----|------|-----|
| `refresh:{userId}:{deviceId}` | RT 存储（每设备独立） | 7d |
| `blacklist:at:{jti}` | AT 黑名单 | AT 剩余有效期 |
| `user:disabled:{userId}` | 用户级拒绝（禁用/删除） | 30min（覆盖 AT TTL） |
| `lock:login:{employee_no}` | 登录失败计数 | 15min |
| `devices:{userId}` | 设备列表（SET，Phase 2） | 持久（跟随用户） |
| `perm:user:{userId}` | 权限缓存（工单跑通后按需） | 30min |

### 管理员重置密码 + 首次登录强制改密

#### 设计思路

业界最佳实践（PCI DSS 4.0 §8.3.5、NIST SP 800-63B）要求：管理员重置密码后，必须强制用户首次登录时修改密码，确保只有用户本人知道最终密码。Phase 1 不引入邮件/短信通道，采用管理员重置方案。

#### 数据模型

`must_change_password` 已在 Phase 1 建表时包含（见 [04-user.md](./04-user.md) `users` DDL），无需单独 migration。

#### 权限分级

| 操作 | 允许的角色 | 说明 |
|------|-----------|------|
| 重置普通用户密码 | `superadmin`, `admin` | admin 只能重置普通用户 |
| 重置 admin 密码 | `superadmin` 仅此一个 | admin 不能重置同级或更高级 |
| 用户修改自己的密码 | 登录用户本人 | 旧密码验证通过即可 |

> **角色分级**：`roles.priority` 越小越强（superadmin=1 … viewer=30）。superadmin 是系统内置角色（`is_system = true`），无法被删除或禁用，确保始终有一个"最高权限"入口。Casbin matcher 中 superadmin 和 admin 都直接 bypass（`r.sub == "role::superadmin" || r.sub == "role::admin"`），区别在业务逻辑层的 priority 防提权校验。
>
> **多角色**：业务层用 `roles.priority`（越小越强），双方均取 **EffectivePriority = min(priority)**（见 [04-user §多角色与有效 priority](./04-user.md#多角色与有效-priority)）。`superadmin` vs `admin` 见 [05-role](./05-role.md#superadmin-与-admin-的区别)。

#### 重置流程

```
1. superadmin 调用 POST /api/v1/users/password/reset
2. 后端校验：操作者 EffectivePriority **<** 目标用户（防止 admin 重置 superadmin **或同级 admin**）
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
   a. 签发 AT（JWT `mcp=true`，仅允许访问改密接口）
   b. 前端检测到该标记，跳转到强制改密页面
   c. 用户修改密码（需满足密码策略）
   d. 后端：更新密码 + 清除 must_change_password 标记
   e. 前端跳转到主页
4. 若标记期间用户尝试访问其他接口：返回 403 + **20007**（`ErrPasswordChangeRequired`）
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

> **复核日期**：2026-08-14。对照 **`github.com/qingtao/aksk/v2@v2.1.0`** 源码与 README（非 v1 旧文档）。

对 `github.com/qingtao/aksk/v2` 进行了详细分析：

**优点**：
- MIT 许可，可导入的纯库（无 main 包），不引入额外服务
- 支持多种签名算法：HMAC-SHA256/384/512、RSA、ECDSA、Ed25519
- 支持时间戳防重放（±60s 可配置）+ 可选 nonce 防重放
- 公钥缓存带 `singleflight` 去重，10 分钟 TTL
- `KeyGetter` 函数类型让存储层完全解耦
- 作者 README 对 **MemoryStore 多实例、LRU 淘汰、nonce TTL 与 skew 对齐** 有明确说明（可借鉴）

**关键安全缺陷（v2.1.0 源码确认）**：

待签名字符串 **仅** 由 `access_key`、`timestamp`、`nonce` 三字段 **字符串排序后用英文逗号拼接** 构成，**不包含** HTTP Method、Path、Query、Body：

```go
// v2.1.0 core/core.go — Sign / Verify
sort.Strings(elems)
data := []byte(strings.Join(elems, ","))

// v2.1.0 request/request.go — 客户端签名
signature, err := a.Sign(cfg.Sk, cfg.Ak, ts, nonce)
// Validate 时同样 Verify(ak, sk, signature, ak, ts, nonce)
// 全程未读取 req.Method / URL.Path / Body
```

因此：

- ❌ **无请求完整性（Integrity）**：签名只证明「谁在时间窗内发起了某次认证」，**不绑定**具体 API 操作。
- **攻击示例**：攻击者截获合法 `POST /api/v1/users/delete` 的 `x-auth-*` 头，在 ±60s 内可原样复用到 **`GET /api/v1/users`** 或 **`POST /api/v1/orgs/members/delete`**（改 body），服务端验签仍可通过（若该 AK 对多路由有权限）。
- 与项目目标 **`method + path + sha256(body)`**（见下文认证流程）及 AWS SigV4 / 国内云 API 签名模型 **不一致**。
- 仓库标题写「校验请求内容」，但 **v2 已去掉 body hash**（网上部分摘要混用了 **v1** 的 `x-auth-body-hash` 描述，易误判）。

**其他限制**：
- nonce 默认 **MemoryStore（freecache）**；多实例必须自实现 `NonceStore`（Redis）——与 Phase 3 多副本一致，但 **无官方 Redis 实现**。
- AK 不得含英文逗号；排序+逗号拼接对 key 格式有约束。
- Header 名为 `x-auth-access-key` 等，与本文 **`X-AK-*`** 约定不同；接入需适配或改客户端。
- 中间件为 **标准库 `http.Handler`**，错误处理默认 **401 + err.Error() 明文**，不符合 `{ code, message, data, request_id }` envelope。
- 社区体量小（GitHub ~3 star），**无 AK 轮换/管理**；非对称密钥路径增加复杂度，Phase 1 M2M 用不到。
- v2.1.0 `go.mod` 声明 **Go 1.25+**（可编译，但与「少依赖、自控」目标相比收益有限）。

#### 决策：Phase 1 自研签名逻辑，不引入 `qingtao/aksk`

**理由**：
1. **请求完整性是 M2M 硬需求**——v2 只签 `ak,timestamp,nonce`，不满足。
2. 自研 HMAC 验签 **约 50 行** + 现有 Redis nonce（与 LoginLocker 同栈），可控且与 `errcode`/中间件一致。
3. **借鉴** qingtao 的 `KeyGetter`、`NonceStore` 接口、skew/TTL 对齐思路；**不直接依赖**。
4. 若将来要用第三方库，应优先评估 **canonical request 含 Method/Path/Body** 的实现（如自研或 `ak-auth-go` 类方案），而非 qingtao v2。
5. Phase 1 **不建表、不写中间件**。签名方案仅作后续参考。

#### 已决策：存量 qingtao/aksk 迁移策略

> **状态**：✅ 已确认（2026-08-14）。**仅自研 Canonical，强制迁移；不做双栈过渡、不长期并存 qingtao 验签。**

**背景**：存量调用方曾用 **`github.com/qingtao/aksk/v2`**（`x-auth-*`，待签名 `ak,timestamp,nonce`）。自研 **Canonical**（`X-AK-*`，含 `method + path + sha256(body)`）**算法不互通**，且安全模型更完整（绑定请求完整性）。

**决策**：

| 项 | 结论 |
|----|------|
| 服务端验签 | **只实现 Canonical**；不引入 `qingtao/aksk` 依赖，不保留 `x-auth-*` 验签分支 |
| 存量客户端 | **必须升级**到新签名 SDK / 调用约定；按 M2M 上线窗口排迁移 |
| 双栈 / `sign_scheme` | **不做** |
| `api_credentials` | 无需 `sign_scheme` 字段（单一 scheme） |

**与 qingtao v2 差异（迁移对照用）**：

| 维度 | qingtao v2（弃用） | 自研 Canonical（唯一方案） |
|------|-------------------|---------------------------|
| Header | `x-auth-*` | `X-AK-*` |
| 待签名 | `sort(ak,timestamp,nonce)` | `method\npath\ntimestamp\nnonce\nsha256(body)` |
| 绑定 path/body | ❌ | ✅ |
| 签名编码 | Base64 | 实现时统一（hex / base64） |

**M2M 上线前**：通知调用方、提供 Canonical 签名示例与 AK 轮换流程；旧 `x-auth-*` 请求一律 **401**（无兼容模式）。

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
请求进入 → AuthN 中间件（按路由类型分支，见下节「混用规则」）
              │
              ├── 公开路由（/auth/login 等）→ 不校验 JWT/AK/SK
              │
              └── 受保护路由
                    │
                    ├── 同时存在 Bearer 与 X-AK-* → 400 + 20008（拒绝混用）
                    │
                    ├── 仅有 Authorization: Bearer → JWT 路径（人类用户）
                    │     auth_type = "user"
                    │     user_id / username / jti …
                    │
                    ├── 仅有 X-AK-Access-Key + 签名头 → AK/SK 路径（服务调用）
                    │     auth_type = "m2m"
                    │     credential_id / credential_name
                    │
                    └── 两者皆无 → 401
```

两种认证方式共用 Casbin 鉴权层。AK/SK 对应的 Casbin subject 为 `service:{credential_name}`，可分配独立的策略。

#### 凭证优先级与混用规则

文档里容易混淆的是三类凭证：**工号密码**、**JWT（Bearer）**、**AK/SK 签名**。它们出现的位置和互斥关系如下。

**1. 工号 + 密码 — 只用于登录，不参与业务 API 鉴权**

| 场景 | 凭证位置 | 说明 |
|------|----------|------|
| `POST /auth/login` | JSON body `{ employee_no, password }` | 唯一入口；换 AT/RT |
| 业务 API（`/users`、`/orgs`…） | **不出现** employee_no/password | 登录后只带 `Authorization: Bearer {AT}` |

因此不存在「业务请求里 employee_no/password 和 AK/SK 谁优先」——业务 API **根本不应**再带密码；若有人把密码塞进 body/header，中间件 **忽略**，仍按 JWT 或 AK/SK 鉴权；若两者都没有则 401。

**2. JWT 与 AK/SK — 受保护路由上互斥，不允许同时带**

| 请求头 | 身份 |
|--------|------|
| `Authorization: Bearer {AT}` | 人类用户（或已登录客户端） |
| `X-AK-Access-Key` + `X-AK-Timestamp` + `X-AK-Nonce` + `X-AK-Signature` | 调用方服务（M2M） |

**决策：两者同时出现 → 400 + `20008 ErrMultipleAuthMethods`（「不能同时使用多种认证方式」）**

理由：

- 避免「JWT 优先还是 AK/SK 优先」的歧义；
- 防止攻击者在合法 M2M 请求上再挂一个伪造 Bearer，或反过来干扰审计身份；
- 客户端必须明确：这是 **用户会话调用** 还是 **服务凭证调用**。

**不以其中一种为准**——混用直接拒绝，而不是静默选一条路径。

**3. 登录接口 `/auth/login` 与 AK/SK**

- 登录是 **公开路由**（仅 LoginLocker 限流），不走 JWT/AK/SK 中间件。
- 即使请求误带了 `Authorization` 或 `X-AK-*`，**忽略**，只处理 body 里的 employee_no/password。
- AK/SK 不能替代登录换 Token；服务若需调用 API，应走 AK/SK 路径，而不是先伪造用户登录。

**4. 服务代用户操作（常见真实需求，Phase 1 / 首版 AK/SK 不做）**

若工单系统等 **用 AK/SK 证明自己是可信服务**，又要 **代表某个用户** 操作，不能在同一请求里混 Bearer + AK/SK。推荐后续（Phase 3b / 有 M2M 时）单独设计，例如：

| 模式 | 做法 |
|------|------|
| **A. 纯服务身份** | 仅 AK/SK；Casbin subject = `service:工单系统`；权限由服务级策略决定 |
| **B. 网关透传** | 网关验 AK/SK 后向内网注入 **内部 JWT**（含 `user_id`）；下游只认内部 JWT |
| **C. 显式委托头** | AK/SK 验签通过 + 额外头 `X-On-Behalf-Of: {user_id}`（须服务有 impersonate 权限，且写审计） |

首版实现 M2M 时建议只做 **模式 A**；B/C 有真实调用方再扩展，并在本文档补请求示例。

**5. 小结**

```
/auth/login          → 只看 body employee_no/password（忽略 JWT/AK/SK 头）
受保护 API           → JWT 或 AK/SK 二选一；混用 → 400/20008
业务 API             → 不带 password
Casbin subject       → user:{roles…} 或 service:{name}，由 auth_type 决定
```

#### 非法认证请求的处理（实现必读）

遇到格式错误、混用凭证、重放、验签失败等 **非法或未授权请求** 时，按下列规则处理。**禁止** fail-open（任一路径验过就放行）、**禁止** 「选一种凭证继续业务」、**禁止** 在响应里返回 Redis/DB/SK 等内部细节。

**1. 中间件检测顺序（受保护路由）**

在解析 JWT 或验 AK/SK **之前** 先做互斥与路由分类，避免对已明显非法的请求做多余密码学运算：

```
1. 路由是否公开？→ 是：跳过 AuthN（login 仍走 LoginLocker）
2. 是否同时存在 Bearer 与 X-AK-Access-Key（或完整 AK 签名四件套）？
   → 是：立即 Abort，400 + 20008，不解析 JWT、不验签
3. 仅有 Bearer → JWT 链（解析 → 黑名单 → user:disabled → mcp）
4. 仅有 X-AK-* → AK/SK 链（时间窗 → nonce → 验签 → is_active）
5. 皆无 → 401 + ErrUnauthorized（10002 或统一 401 业务码，与 errcode 表一致）
6. 任一链失败 → Abort，不进入 Casbin、不进入 Handler
```

**2. 分类处理表**

| 场景 | HTTP | code | 是否进 Casbin | 响应 message（示例） | 应用日志 | 审计库 |
|------|------|------|---------------|----------------------|----------|--------|
| Bearer + AK/SK 同时存在 | 400 | 20008 | 否 | 不能同时使用多种认证方式 | **Warn**：`auth_mixed` + request_id + ip + path + ak 前缀 | 否（量大，仅日志） |
| 受保护路由无任何凭证 | 401 | 10002 | 否 | 未登录 | Info | 否 |
| JWT  malformed / 签名错 | 401 | 20003 | 否 | token 已失效 | Info | 否 |
| JWT 过期 | 401 | 20002 | 否 | token 已过期 | Info | 否 |
| AT 在黑名单（已登出） | 401 | 20003 | 否 | token 已失效 | Info | 否 |
| 用户已禁用（AT 路径） | 403 | 30003 | 否 | 用户已禁用 | Info | 可选（Phase 2+） |
| mcp 访问非改密接口 | 403 | 20007 | 否 | 需要修改密码 | Info | 否 |
| AK 不存在 / 已禁用 | 401 | 20009 | 否 | 访问密钥无效 | **Warn**：`aksk_reject` + ak 前缀 | 否 |
| 签名错误 / body 被篡改 | 401 | 20009 | 否 | 访问密钥无效 | **Warn**：`aksk_bad_sig` + ak 前缀 | **建议**记一条安全审计（见下） |
| 时间戳超出 ±5min | 401 | 20010 | 否 | 请求已过期 | Warn：`aksk_skew` | 否 |
| nonce 重放 | 401 | 20011 | 否 | 重复请求 | **Warn**：`aksk_replay` + ak + nonce | **建议**记安全审计 |
| Redis 不可用（鉴权链） | 503 | 10008 | 否 | 服务暂时不可用 | **Error** + 内部 err | 否 |
| 业务 body 带 employee_no/password 但无 Bearer | 401 | 10002 | 否 | 未登录 | **Warn**：`credential_in_body`（可疑，不尝试用密码登录） | 否 |

> AK/SK 相关码 `20009`–`20011` 在 M2M 上线时写入 `errcode.go`；**对外 message 统一模糊**（「访问密钥无效 / 请求已过期 / 重复请求」），不区分「AK 不存在」与「SK 算错」，防探测。

**3. 响应体**

一律走统一 envelope（见 [response.md](../api/response.md)）：

```json
{
  "code": 20008,
  "message": "不能同时使用多种认证方式",
  "data": null,
  "request_id": "..."
}
```

- **`data` 恒为 `null`**，不返回「哪条凭证错了」的详情。
- **HTTP 状态** 按 [errcode.md](../api/errcode.md) 映射；混用凭证用 **400**（客户端错误），而非 401。

**4. 应用日志（slog）— 必须打**

所有非法 AuthN 拒绝至少记录：

```go
slog.Warn("auth rejected",
    "reason", "auth_mixed",           // 见上表 reason 枚举
    "request_id", requestID,
    "method", c.Request.Method,
    "path", c.FullPath(),             // 路由模板，非原始 URL（防日志注入）
    "client_ip", clientIP,
    "user_agent", c.Request.UserAgent(),
    "ak_prefix", maskAK(accessKey),   // 仅前 8 位 + "***"，无 AK 则省略
    // 禁止：password、完整 token、SK、signature 原文
)
```

**5. 审计库（audit_logs）— 选择性写入**

| 写入审计 | 不写入审计 |
|----------|------------|
| AK/SK **验签失败**、**nonce 重放**（疑似攻击） | 普通 JWT 过期、未带 Token |
| 同一 IP **短时间大量** 20008/20009（可选阈值告警） | 单次 20008（可能是集成配置错误） |

审计字段建议：`module=auth`、`action=auth_reject`、`detail={"reason":"aksk_replay","ak_prefix":"ak_abc***"}`；**无 user_id**（未识别身份）。IP、request_id 必填。

**6. 明确禁止**

| 禁止行为 | 正确做法 |
|----------|----------|
| Bearer 和 AK/SK 同时存在时「优先 JWT」或「优先 AK/SK」 | 400 + 20008，Abort |
| 验签失败仍尝试解析 JWT 兜底 | 401，Abort |
| 业务 API 发现 body 里有 password 就当作登录 | 忽略 password 字段；无 Bearer 则 401 |
| 响应里返回 `redis: connection refused` | 503 + 10008，细节只进 slog |
| 非法请求继续 `c.Next()` 进 Casbin | AuthN 失败必须 `c.Abort()` |

**7. Phase 1 与 M2M 上线后**

| 阶段 | 非法请求处理 |
|------|--------------|
| **Phase 1**（无 AK/SK） | 仅 JWT 链 + 混用检测可 **预留**：若检测到 `X-AK-*` 但系统未启用 M2M → **401 + 20009** 或 **501**（推荐 **401 + message「不支持该认证方式」**，避免泄露「将来会支持 AK/SK」） |
| **M2M 上线后** | 完整 AK/SK 链 + 20008/20009/20010/20011；管理端可查询 `auth_reject` 审计 |

**8. 参考实现（AuthN 入口伪代码）**

```go
func AuthN() gin.HandlerFunc {
    return func(c *gin.Context) {
        if isPublicRoute(c) {
            c.Next()
            return
        }
        hasBearer := hasBearerToken(c)
        hasAK := hasAKHeaders(c)

        if hasBearer && hasAK {
            logAuthReject(c, "auth_mixed")
            response.BadRequest(c, errcode.ErrMultipleAuthMethods) // 400, 20008
            c.Abort()
            return
        }
        if hasBearer {
            jwtAuth(c) // 失败则 Abort，成功则 c.Next 仅当链末尾
            return
        }
        if hasAK {
            if !cfg.M2MEnabled {
                logAuthReject(c, "aksk_disabled")
                response.Unauthorized(c, errcode.ErrAKInvalid)
                c.Abort()
                return
            }
            akskAuth(c)
            return
        }
        response.Unauthorized(c, errcode.ErrUnauthorized)
        c.Abort()
    }
}
```

> JWT 与 AK/SK 子链内部同样：**任一检查失败立即 Abort**，不降级、不跳过 Casbin 前的任何一步。

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
| 登录成功副作用 | 正确账号密码 | `last_login_at`、`last_login_ip` 更新（`ClientIP()`） |
| 登录 - 无工号用户 | employee_no 为空 | 401 + 20001（与密码错误同文案） |
| 登录 - 工号不存在 | 错误 employee_no | 401 + 20001 |
| 密码错误 | 正确工号 + 错误密码 | 401 + "工号或密码错误" |
| 用户不存在 | 不存在的工号 | 401 + "工号或密码错误"（同密码错误，防枚举） |
| 用户已禁用 | status=disabled 的用户 | 401 + "工号或密码错误"（登录时与密码错误同文案） |
| 连续失败超限 | 同一工号第 6 次失败 | 429 |
| Redis 不可用 | Redis 宕机时登录 | 503 |
| 请求参数缺失 | 无 password 或无 employee_no | 400 + 参数校验错误 |

### Token 刷新

| 用例 | 输入 | 预期 |
|------|------|------|
| 刷新成功 | 有效 RT | 200 + 新 AT + 新 RT |
| 旧 RT 失效 | 使用已刷新过的旧 RT | 401 + 20004 |
| **用户被禁用后 refresh** | 管理员禁用后，客户端仍用禁用前持有的 RT | **401 + 20004**，无新 AT/RT（RT 已 DEL；兜底查 user:disabled/DB） |
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
| 首次登录后访问其他接口 | AT（`mcp=true`） | 403 + 20007 |
| 首次登录后修改密码 | 临时密码 + 新密码 | 200，清除 `must_change_password` |
| 改密后正常访问 | AT（已改密） | 200 |

### 非法认证（JWT / 混用 / M2M）

| 用例 | 输入 | 预期 |
|------|------|------|
| 受保护路由无 Token | 无 Authorization | 401 + 10002，`c.Abort` 不进 Casbin |
| Bearer + AK/SK 混用 | 两种头同时存在 | **400 + 20008**，不解析 JWT、不验签 |
| 业务 API body 带 password 无 Bearer | POST /users + password 字段 | 401，**不**用 password 尝试登录 |
| Phase 1 请求带 X-AK-* | 未启用 M2M | 401 + 20009（或统一「不支持该认证方式」） |
| AK/SK 签名错误（M2M 启用后） | 篡改 body | 401 + 20009，Warn 日志 + 可选 audit |
| AK/SK nonce 重放（M2M 启用后） | 同一 nonce 两次 | 401 + 20011，Warn 日志 + 建议 audit |
| 鉴权链 Redis 故障 | 黑名单 EXISTS 失败 | 503 + 10008，Error 日志 |

---

## 涉及文件

> 目标目录形态见 [architecture §3.5](../design/architecture.md#35-领域模块目录约定单仓可拆分)；骨架扁平文件迁入 `{domain}/` 后再删旧路径。

```
internal/service/auth/              # Login/Refresh/Logout/UpdatePassword
internal/service/user/              # ResetPassword（管理员重置）
internal/handler/auth/
internal/handler/user/              # 管理员重置密码
internal/pkg/jwt/
internal/repository/user/           # 密码验证依赖
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
- ✅ **Casbin adapter**：直接上 PG adapter（`noho-digital/casbin-pgx-adapter`，Casbin v3）。
- ✅ **组织模块范围**：Phase 1 实现完整 CRUD。
- ✅ **AK/SK**：Phase 1 不做。签名方案保留，有调用方再实现。
- ✅ **登录限流**：Phase 1 用 Redis Lua（INCR+EXPIRE 原子），阈值 15min/5 次。
- ✅ **Redis 故障**：鉴权链路 fail-close（503）。
- ✅ **会话吊销**：禁用/删除用户写 `user:disabled:{userId}`。
