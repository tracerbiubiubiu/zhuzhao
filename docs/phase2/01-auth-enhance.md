# 01 - 认证增强（auth-enhance，Phase 2b）

> **Step 7**，可与 Step 6（storage）并行；依赖 Phase 1 认证链路（双 Token、Redis RT——**设备元数据未落地，见 §1 前置条件**）。  
> 模块背景见 [modules/auth.md](../modules/auth.md)、[phase1/02-auth.md](../phase1/02-auth.md)。

---

## 0. 边界

### 与 Phase 1 会话吊销机制的衔接（B3）

Phase 1 已定型**双轨吊销机制**（SSOT：[phase1/02-auth.md §会话吊销](../phase1/02-auth.md)）：

| 轨道 | 键 | 语义 |
|------|-----|------|
| 拒绝标记 | `user:disabled`（TTL = AT 剩余有效期） | 拦截存量 AT（JWT 中间件 403+30003） |
| 会话清除 | SCAN 删全部 `refresh:{uid}:*` | 使全部设备无法刷新（Refresh 401+20004） |

已接入场景：**禁用 / 删除 / 重置密码 / 修改密码**（后两者吊销后 `clearUserDisabled` 再为当前设备签发新 Token 对）。本模块设备管理在此基础上扩展，**不另建第三套语义**：

- **踢设备 = 单轨道**：仅 `DEL refresh:{userId}:{deviceId}` + `SREM`，**不得触碰 `user:disabled` 键**（那会把用户全部设备的存量 AT 全拦掉，语义从"踢一台"变成"全局封禁"）；
- **管理员全平台封禁**：继续走 Phase 1 `revokeUserSessions`（见 §2.4），不经过设备管理 API。

### 做什么

| 能力 | API | 说明 |
|------|-----|------|
| 设备列表 | `GET /api/v1/auth/devices` | 读 Redis `devices:{userId}` + 各 RT 元数据 |
| 踢出设备 | `POST /api/v1/auth/devices/delete` | 删指定 `refresh:{userId}:{deviceId}` + SREM |
| 密码复杂度 | 改密 / 创建用户 / 管理员重置 | 可配置策略，默认 8 位 + 四类字符 |

### 不做

| 不做 | 阶段 |
|------|------|
| 登录限流 / 会话吊销 | Phase 1 已完成 |
| RS256 / JWKS | Phase 3（暂缓） |
| 异地登录 / CAPTCHA | Phase 3 |
| 密码过期天数 | 按需 |

---

## 1. 前置条件

- [ ] Phase 1 登录/刷新/登出可用
- [ ] `POST /api/v1/auth/password/update` 已实现
- [ ] **（D2-49①，本 Step 首任务负责落地，非 Phase 1 已有）** `SADD devices:{userId}` 集合初始化 + RT value 结构升级——Phase 1 实际只写 `SET refresh:{userId}:{deviceId} = hashToken(rt)`（SHA-256 hex，[auth_service.go issueTokenPair](../../internal/service/auth_service.go)），**无 devices 集合、无设备元数据**（review 03 §10.1 D2-49 实证）。升级内容见 §2.1

> **为何不在 Phase 1 顺带改**：改 RT value 结构牵动 Refresh 的 hash 比较逻辑与 B1-B4 守护测试，违背「Phase 1 收口不再扩面」原则；设备会话本就是本 Step 引入的能力，前置改造属本职范围。

- [ ] **（D2-23，条件触发）收紧 CORS**：Phase 1 `AllowAllOrigins` 全放开（[cors.go](../../internal/middleware/cors.go)，Bearer 认证下低危可接受）。**本模块或后续任何 Step 引入 cookie 会话（凭据型）前必须先收紧**为 Origin 白名单 + `AllowCredentials` 显式约束——`AllowAllOrigins + credentials` 组合会被浏览器拒绝，且全放开凭据构成 CSRF 直接暴露面。本 Step 设备管理仍走 Bearer（不触发）；登记于此防 cookie 方案落地时漏检（[review 03 §10.2](../review/03-second-deep-review-findings.md)）

---

## 2. 设备管理

### 2.1 Redis 结构（**本 Step 首任务落地**，Phase 1 现状为裸 hash——D2-49①）

```
refresh:{userId}:{deviceId}  → JSON { "hash": "<sha256(rt)>", "meta": { jti, device_name, ip, user_agent, created_at, last_refresh_at } }
devices:{userId}             → SET of deviceId
```

**首任务**（工单 A1 之前）：

1. `issueTokenPair` 改为写结构化 value（hash 与元数据并存——Refresh 校验改读 `meta` 同级的 `hash` 字段，兼容逻辑同 Step 重构）；
2. 登录/登出/吊销链路补 `SADD`/`SREM devices:{userId}`（Logout 单设备、`revokeUserSessions` 全量——顺带消解其全 keyspace SCAN）；
3. 既有 Refresh/Logout 守护测试同步改造。

登录 / 刷新时更新 `last_refresh_at`；`device_name` 可由前端传 `device_info.name`，缺省用 User-Agent 截断。

### 2.2 `GET /api/v1/auth/devices`

**鉴权**：须有效 AT（JWT 中间件）。

**响应示例**：

```json
{
  "code": 0,
  "data": {
    "devices": [
      {
        "device_id": "550e8400-e29b-41d4-a716-446655440000",
        "device_name": "MacBook Chrome",
        "ip": "10.0.0.1",
        "created_at": "2026-08-14T10:00:00Z",
        "last_refresh_at": "2026-08-14T12:00:00Z",
        "is_current": true
      }
    ]
  }
}
```

`is_current`：与当前请求 JWT 内 `device_id` claim 比对（Phase 1 若 JWT 未含 device_id，则从 RT 刷新链路补 claim 或仅标记「最近 refresh 的设备」）。

### 2.3 `POST /api/v1/auth/devices/delete`

```json
{ "device_id": "550e8400-e29b-41d4-a716-446655440000" }
```

**逻辑**：

1. 校验 `device_id` 属于 `devices:{userId}`
2. `DEL refresh:{userId}:{deviceId}`
3. `SREM devices:{userId} deviceId`
4. 若踢的是当前设备，可选：将当前 AT 加入黑名单（TTL = AT 剩余有效期）

**响应**：200

| 场景 | HTTP | code |
|------|------|------|
| device_id 不存在 | 404 | 20012 |
| 踢他人设备（仅本人列表） | — | 只删本人名下 device |

### 2.4 管理员踢人（可选，2b 不做）

全平台「禁用用户」仍走 Phase 1 `user:disabled` + 删全部 RT（SCAN `refresh:{userId}:*`）。**不提供** admin 查他人设备列表 UI（合规需要时再开）。

---

## 3. 密码复杂度

### 3.1 配置（config.yaml）

```yaml
auth:
  password:
    min_length: 8
    require_upper: true
    require_lower: true
    require_digit: true
    require_special: true
    special_chars: "!@#$%^&*()-_=+[]{}|;:,.<>?"
```

### 3.2 校验入口

| 入口 | 说明 |
|------|------|
| `POST /api/v1/auth/password/update` | 用户改密 |
| `POST /api/v1/users` | 创建用户初始密码 |
| `POST /api/v1/users/password/reset` | 管理员重置 |

**校验函数**：

```go
func ValidatePasswordPolicy(pwd string, cfg config.PasswordPolicy) error {
    if len(pwd) < cfg.MinLength { return errcode.ErrPasswordTooWeak }
    // 四类字符逐一检查 ...
}
```

### 3.3 错误码

| code | 常量 | message | HTTP |
|------|------|---------|------|
| 20012 | `ErrDeviceNotFound` | 设备不存在 | 404 |
| 20013 | `ErrPasswordTooWeak` | 密码不符合复杂度要求 | 400 |

> 写入 `errcode.go` 时勿改号；同步 [api/errcode.md](../api/errcode.md)。

### 3.4 与 Phase 1 关系（基线更新 2026-08-19）

- Phase 1 **已实现** `min=8` 最小长度校验（F-9 修复，三处密码字段 `binding:"required,min=8"`，返回 gin 参数绑定错误）——原文档"Phase 1 不校验复杂度"的表述已过时，当前基线为「仅最小长度，无字符类别要求」；
- 2b 上线后，`ValidatePasswordPolicy` **接管完整策略**（min_length + 四类字符，可配置）；**策略校验归一**：binding 保留 `required`（非空 + 结构完整性），长度与复杂度统一由 `ValidatePasswordPolicy` 校验并返回 **20013**——避免「binding 的 min=8 报参数错误、策略的 min_length>8 报 20013」两套错误码并存；
- **新密码**必须符合策略；旧密码登录后改密时强制满足（与原文一致）。

---

## 4. 测试用例

| # | 用例 | 预期 |
|---|------|------|
| A1 | 两设备登录同一用户 | devices 列表 2 条 |
| A2 | 踢设备 B | B 的 RT refresh 失败 20004；A 仍可用 |
| A3 | 踢当前设备 | 当前 AT 可选黑名单；后续 API 401 |
| A4 | 改密 `abc123` | 400 + 20013 |
| A5 | 改密 `Abcdefg1!` | 200 |
| A6 | 管理员重置弱密码 | 400 + 20013 |

**测试落点约定**（B4）：A1–A3 单测（miniredis）→ `internal/service/auth_service_test.go` 扩展；A4–A6 密码策略单测 → `internal/pkg/crypto` 或 `internal/config` 校验函数所在包；设备管理路由行为 → `internal/router/router_test.go` 扩展。

---

## 5. 涉及文件

```
internal/service/auth_service.go      # ListDevices, KickDevice, ValidatePassword
internal/handler/auth_handler.go      # 新路由
internal/config/config.go             # PasswordPolicy
configs/config.yaml
```

---

## 6. 待决策点

| 事项 | 建议 | 状态 |
|------|------|------|
| JWT 含 device_id | 2b 增加 claim，便于 is_current | ✅ 建议 |
| 踢当前设备是否黑名单 AT | 是 | ✅ 建议 |
| 密码策略可关闭 | config 开关 `enabled: true` | ✅ 建议 |
