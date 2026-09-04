# Phase 3+ · 平台增强（platform）

> **定位**：Phase 3+（3b）按需项——**权限/菜单缓存跨实例失效**、**AK/SK 平台凭据**、关联**事件 L2 升级**（Outbox + Asynq 多消费者）。
> **Wave 归属**：3b（Step 10）｜ **🚦 全部触发条件驱动，是否纳入由所有者决定**。
> **状态**：**已编写（2026-09-02，规划/设计就绪）**。
> **标记约定**：`🚦` = 触发条件驱动；`⚠️` = 待拍板。

---

## 1. 能力总览

| 能力 | 触发条件 | 归属 | 文档 |
|---|---|---|---|
| **权限/菜单缓存跨实例失效** | 多实例 + 热点（QPS 瓶颈） | 🚦 | design-decisions §1 / 本文 §2 |
| **AK/SK 平台凭据** | 有 M2M（机器对机器）调用方 | 🚦 | 本文 §3 |

> **分层注记（2026-09-03 拍板）**：**内部服务间**通信已拍板统一 **AK/SK HMAC 签名**（utils `aksk` 包，静态 env 密钥、按调用方发 SK——zhuzhao 16 号 §9 基线）；本节 AK/SK 指**外部 M2M 调用方**的平台凭据（api_keys 表/哈希存储/签发吊销管理面），仍 🚦 随外部调用方出现——届时**签名算法层直接复用 utils `aksk`**，只新建密钥管理面，不重复造轮子。
| **事件 L2 升级**（Outbox + Asynq worker 多消费者） | 多消费者 / 异步邮件需求（L1 单消费者瓶颈） | 🚦 | ADR-001/002 + 本文 §4 |

> 微服务拆分**不做**（无多团队/M2M 需求）——AK/SK 是 M2M 的最小前置，微服务是更大动作，二者区分对待。

---

## 2. 权限/菜单缓存跨实例失效（🚦）

### 2.1 触发

- 多实例部署（W1 后）且鉴权链路成为热点；QPS/延迟超阈值。

### 2.2 设计（design-decisions §1 既有方案）

- `perm:user:{userId}` Redis 缓存：Casbin 中间件优先读缓存，miss 查 DB（BFS 角色展开结果）。
- **失效**：DB 事务提交 → Casbin 内存更新 → **Redis Pub/Sub 广播失效**（跨实例）→ 各实例清本地 + 缓存 key。
- 兜底：缓存 TTL 自然过期；Pub/Sub 失败可接受短暂不一致（design-decisions §1.5）。
- 菜单缓存同理（`menu:user:*`）。
- ⚠️ 与 W1 Casbin Watcher 的关系：Watcher 解决「策略」跨实例同步；本缓存解决「角色展开结果」热点。若缓存启用，Watcher 仍是策略真相源。

### 2.3 验收

| # | 用例 | 通过标准 |
|---|---|---|
| PL1 | 缓存命中 | 二跳请求角色展开走缓存，DB 查询下降 |
| PL2 | 跨实例失效 | A 实例改角色 → B 实例权限变更 ≤ 秒级（Pub/Sub）或 ≤ TTL 生效 |
| PL3 | miss 兜底 | 缓存 key 失效 → 查 DB 重建，功能正常 |

---

## 3. AK/SK 平台凭据（🚦）

### 3.1 触发

- 出现机器到机器（M2M）调用方（~~如 activelist 对 zhuzhao 的调用~~ **2026-09-03 收敛后 activelist 零认证经网关被调，不再主动调 zhuzhao**；场景 = 外部系统对接、非内网调用），需要非人类凭据。

### 3.2 设计（草案）

- `api_keys` 表：`(key_id, secret_hash, owner, scope, expires_at, enabled)`；KeyId 明文 + Secret 哈希存储（不落明文）。
- 鉴权：`X-Api-Key: <key_id>.<secret>` → 校验哈希 + 有效期 + scope 限制。
- 管理面：管理员签发/吊销/轮换（权限码 `api_key:manage`，⚠️ seed 待定）。
- ⚠️ 与 activelist 的身份断言区分（ADR-003 + design-decisions §25.2）：activelist 经网关访问走**方案 A（AT 原样透传 + 属主共享公钥验签）**——~~X-Operator 明文透传~~ 为被取代的方案 B 类（明文断言头，有伪造面）；AK/SK 用于非内网或需要独立凭据的场景。

### 3.3 验收

| # | 用例 | 通过标准 |
|---|---|---|
| PL4 | 签发/调用 | 签发后携带正确凭据调用成功 |
| PL5 | 吊销 | 吊销后立即 401；secret 不明文落库 |

---

## 4. 事件 L2 升级（🚦，关联 3b Step 9）

### 4.1 触发

- 多消费者 / 异步邮件需求（L1 单消费者瓶颈）出现时。

### 4.2 设计（ADR-001/002）

- L1 → L2：`ticket_events` 信号消费从「单消费者轮询」换为 **PostgreSQL Outbox + Asynq worker 多消费者**。
- **业务逻辑不变，只换调度器**（通知内容、SLA 规则不动）。
- 依赖：Asynq 已引入（~~W2~~ **M-E**，2026-09-02 §23）；Outbox 表 + worker 注册。

### 4.3 验收

| # | 用例 | 通过标准 |
|---|---|---|
| PL6 | 多消费者 | 通知 / SLA / 满意度各自独立消费，互不阻塞 |
| PL7 | 回归 | SLA/通知/审批流用例全量回归（业务不变） |

---

## 5. 涉及文件（规划）

```
internal/service/rbac_service.go    # 角色展开缓存（🚦）
internal/middleware/casbin.go       # 缓存读优先（🚦）
internal/pkg/pubsub/                # Redis Pub/Sub 失效广播（🚦）
internal/service/apikey/            # AK/SK 签发/校验（🚦）
migrations/                         # api_keys（🚦 触发时）
internal/task/outbox_worker.go      # L2 多消费者（🚦）
```

---

## 6. 待决策点（⚠️）

| # | 事项 | 建议 | 状态 |
|---|---|---|---|
| D1 | 三项是否纳入 | 触发条件驱动，均不排期 | 🚦 由你决定 |
| D2 | 缓存失效通道 | Redis Pub/Sub（已有 Redis） | 建议沿用 |
| D3 | AK/SK 哈希 | SHA-256（加盐）或 HMAC | 待拍板 |
| D4 | L2 Outbox 粒度 | 单表 vs 分区 | 待拍板 |

---

## 7. 变更记录

| 日期 | 说明 |
|---|---|
| 2026-09-02 | 已编写：权限/菜单缓存失效 + AK/SK + 事件 L2（全部 🚦 触发条件驱动）+ 验收 + 待决策点 |
