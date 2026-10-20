# 09 — P5-S9 威胁情报缓存代理（设计稿——概要登记，细节启动批定）

> **段属性**：基线段（phase5 收口对象）；**建议实施序**：靠后（无内部消费方依赖，仅 phase6 R1 前置）。
> **量级**：≈ 1 周（缓存表 + 查询代理 + TTL 管理）。**迁移**：有（`intel_cache` 表族；编号启动时对账）。
> **文档定位**：段级设计稿（2026-10-15 建档，phase5 分支）。立项 = dd §27（2026-10-08 第六轮）；phase5 定位校准（"前端可视化配置线"→"前端可视化配置与通用能力底座线"）即由本段归属判定引出。

## 1. 定位

对**已有威胁情报 web 应用做能力补全**（**应用本体不动**）——解决其每日/每小时查询配额限制。技术形态 = **外部限流 API 的缓存代理**（通用件），威胁情报是**首个消费者**；**消费方 = phase6 风险模块 R1**（风险事件情报源之一）。

随行件（2026-10-20 规划研究回灌，依据 deliverables/software-company/secops-platform-vision-2026-10/08-phase5-adjustments.md）：缓存机制层（键归一化 / TTL per type / singleflight / stale 降级）独立为通用包（如 `internal/pkg/apicache`），情报为首个消费者；表名 / 端口名 / API / 权限码保持情报语义不变。第二消费者 = 资产平台查询门面的读穿缓存（phase6/7）。

**归属判定**：所有者口径"phase5 = 通用能力，phase6 = 业务能力"——本件是通用缓存代理，威胁情报仅为首个消费者，故归 phase5。

## 2. 查询流程

查询 → 先查本地缓存库（命中直接返回）→ 未命中调上游接口 → 结果入库带 TTL。

## 3. 缓存设计要点

- **缓存键** = 情报类型（IP/域名/hash 等）+ 查询值。
- **TTL 双档 7 天 / 30 天**（按指标衰减特性配置——config 化）。
- **配额耗尽降级策略（启动拍板点，README 选型表转推荐）**：返回过期缓存 + **陈旧标记**（风险分析场景，旧情报好过报错——stale-while-revalidate 思路）vs 明确报错。
- 可选第二档 = 高危指标后台批量刷新（触发驱动，不预做）。
- **表预置（建议值）**：`intel_cache`(id, intel_type, query_value, result JSONB, fetched_at, expires_at, hit_count) UNIQUE(intel_type, query_value)。

## 4. 依赖与端口

- **上游 API 契约**（查询接口/认证/限流数值）**由企业方提供**；调用凭据入配置管理（config 化禁硬编码——README §6 总纪律③）。
- 查询走 **`ThreatIntelProvider` 端口**（Query(type,value)）——接口 + 通用 HTTP 实现 + config 挂点，§24 SSOProvider 同款预留模式：**契约到位填配置即用**；本件整体即 ACL（防腐层）典型：外部模型不得渗入域模型（README §6 总纪律①）。

## 5. 启动拍板点

| # | 事项 | 建议 |
|---|------|------|
| 1 | 配额耗尽降级 | 返回过期缓存 + 陈旧标记 |
| 2 | 陈旧标记的透出形态 | 响应体元字段（stale: true + fetched_at）——消费者（phase6 R1）自行决定采信度 |

> **状态注（2026-10-16）**：本表拍板点已随批量预确认拍定（2026-10-16 所有者「按建议」——✅34/🔶1，状态 SSOT = [11 号 §4](./11-implementation-plan.md)；各段启动时异议可翻，翻案成本=改一行）。

## 6. 边界（不做）

不做上游应用本体改造；不做多情报源聚合/关联分析（phase6 R1 域；**⚠ 所有者 2026-10-19 留口**：后续拟对接多源[微信公众号等订阅/推送形态]——端口天然多实现、intel_type 可扩，聚合语义=查询层合并属 phase6 R1，触发驱动不排期）；第二档批量刷新不预做；契约未到前**端口 + 桩**先行（fail-fast 不虚标——与 S6 两态桩同口径）。**采集域归属**：多源内容采集立项时**新建 phase6 文档**（通用数据录入接口形态——03 号演进触发总表行），本文件（查询代理，W11–13 phase5 交付物）**不迁移**——pull 缓存代理与 push 采集入库两形态语义相邻但工程身份不同。

## 7. 验收口径

- 后端随四档 acceptance（缓存桩/模拟上游）。
- 场景标尺：首查回源入库 → 二查命中零上游调用（hit_count 递增）→ TTL 过期后回源 → 模拟配额耗尽（上游 429）→ 返回陈旧缓存带标记（拍板点 1 成立时）→ 上游契约字段映射独立可换。

## 8. 文档衔接

| 文档 | 关系 |
|------|------|
| [phase6/README §8](../phase6/README.md) | 消费方（R1 情报源之一；S9 交付后直接消费） |
| [README §6](./README.md) | `ThreatIntelProvider` 端口预留行 + ACL 总纪律 |
| [design-decisions §27](../design/design-decisions.md) | 立项与归属判定登记 |

---

## 附录 A：详细技术设计（实施级，2026-10-16 补充——样板深度对齐 05 号附录 A）

> ⚠ 标注 = 实施级设计建议（启动批确认制）。拍板点 1/2 已批量预确认（11 号 §4）。

### A.1 表 DDL（迁移全文级，编号启动对账）

```sql
CREATE TABLE intel_cache (
    id          BIGSERIAL PRIMARY KEY,
    intel_type  VARCHAR(30) NOT NULL,        -- ip / domain / hash / url（config 枚举白名单）
    query_value VARCHAR(200) NOT NULL,       -- 归一化后查询值（ip 原样/domain 小写/hash 小写——归一化规则随类型注册）
    result      JSONB NOT NULL,              -- 上游响应原样（ACL：外部模型透传，不渗入域模型）
    fetched_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at  TIMESTAMPTZ NOT NULL,        -- fetched_at + TTL(intel_type)
    hit_count   INT NOT NULL DEFAULT 0,
    UNIQUE (intel_type, query_value)
);
CREATE INDEX idx_intel_cache_expiry ON intel_cache (expires_at);
-- 2026-10-19 P0-3 修正：原 partial index 谓词 `WHERE expires_at < NOW()` 为非法 DDL
-- （PG 要求索引谓词 IMMUTABLE，NOW() 是 STABLE 不满足——B9-1 迁移即失败；
--  二轮精度修正：原记 VOLATILE 不准，结论不变）；
-- 过期判定移入查询层 WHERE expires_at < NOW()（部分索引收益本就有限——过期行物理删除/清理任务可后续挂账）。
-- 懒过期：查询命中已过期行 → 按降级策略处理（拍板点 1）+ 回源刷新；周期物理清理骑 CronScheduler（低频，防表膨胀）
```

- TTL 双档 config：`intel.ttl: {ip: 168h, domain: 168h, hash: 720h}`（7/30 天按类型——§3 既定，KV 可调随 S5 配置化面复用可选）。

### A.2 端口与两态桩（§4 落地）

```go
// internal/pkg/threatintel/（端口+adapter，ACL 防腐层）
type Query struct { Type, Value string }
type IntelResult struct {
    Data   json.RawMessage // 上游响应透传
    FetchedAt time.Time
    Stale  bool             // 陈旧标记（拍板点 2：响应元字段形态）
}
type ThreatIntelProvider interface { Query(ctx context.Context, q Query) (IntelResult, error) }
```

- **HTTP adapter**（`http.go`）：endpoint/鉴权头/字段映射全部 config（`intel.upstream.*`），**映射层独立函数** `mapUpstreamResponse()`——真实契约到手只改此函数（§24 SSOProvider 同款）；
- **两态桩**：`intel.upstream.stub=true` → 开发态桩（返回构造样例 + 落日志，全链可测）；`false` 且未配置 endpoint → fail-fast `ErrIntelNotConfigured`（errcode 随批登记；与 S6 两态桩同口径防虚标）。

### A.3 查询代理链路

```
Query(type, value) →
  ① 归一化（按类型注册的归一化函数——domain 小写等） →
  ② 查 intel_cache：UNIQUE 命中 →
       未过期：hit_count++ → 返回（stale=false）
       已过期：拍板点 1 预确认 = 返回过期缓存（stale=true + fetched_at）**同时异步回源刷新**（goroutine，失败仅落日志——不阻塞消费者）→
  ③ 未命中 → 调上游（HTTP adapter）→
       成功：INSERT ... ON CONFLICT (intel_type, query_value) DO UPDATE → 返回（stale=false）
       上游 429/配额耗尽：有过期缓存 → 返回过期带标记；无 → 503 ErrIntelUpstreamQuota（errcode 随批）
       上游其他错误：502 透传语义（errcode 归一，不泄内部细节——standards §3-4）
```

- **单飞**（singleflight）：同 key 并发未命中只放一个回源（`golang.org/x/sync/singleflight`——防缓存击穿放大上游配额消耗，本件存在的意义即保护上游配额）；
- **上游 404（查无记录）= 正常结果入缓存（2026-10-16 评审批深审项 D）**：clean 答案同样 INSERT 缓存（TTL 内免打上游——保护配额即本件存在意义）；错误（5xx/网络）≠查无，前者不缓存走失败链；「404 vs 200+空结果」形态随真实契约对表（mapUpstreamResponse 同点改）；
- 上游限流自留量：**`golang.org/x/time/rate.Limiter`**（标准扩展件，勿自写——config QPS 上限，缺省 1 QPS 保守起步），超桶请求直接走降级路径。

### A.4 API 契约

```
GET /api/v1/intel/query?type=ip&value=1.2.3.4    # 登录态 + 权限码 intel:query（新码，B9-1 随批对 menu_apis/种子）
响应：{ "code":0, "data": { "type":"ip", "value":"1.2.3.4",
         "result": {...上游原样...},
         "stale": false, "fetched_at": "…", "hit_count": 3 } }
```

- 权限码 `intel:query` 为本段唯一新码（管理/清理端点 superadmin-only 挂既有系统管理族）；phase6 R1 消费 = 进程内调 service 层（模块化单体直依赖，与 R3 惯例一致），**不经 HTTP 自调用**；
- 限流：query 端点挂 rate_limit 独立桶（外部配额保护的镜像——平台侧也限流，双保险）。

### A.5 验收补充（在 §7 基础上）

- singleflight：并发 10 同 key 查询 → 上游仅 1 次调用（计数断言）；
- 降级矩阵：有缓存+429 → stale 返回；无缓存+429 → 503 errcode；stub 态全链绿 / 非 stub 未配置 → fail-fast 错误码；
- 归一化：`EXAMPLE.COM` 与 `example.com` 同缓存行（UNIQUE 命中）；
- 404 入缓存（2026-10-16 评审批深审项 D）：上游 404（查无记录）→ 正常入库返回 stale=false → 二查命中零上游调用；上游 5xx → 不缓存走失败链；
- hit_count 递增与 expires_at 语义（7 天类型 30 天类型各一例）；
- ACL 断言：上游响应未知字段透传不解析（result 原样）——防腐层行为验证。

---

## 变更记录

| 日期 | 变更 |
|---|---|
| 2026-10-15 | 建档（phase5 分支十段设计稿批，概要登记细化）——未新增决策 |
| 2026-10-16 | **详细技术设计批（附录 A，实施级——05 号样板深度）**：A.1 intel_cache DDL 全文（UNIQUE+部分过期索引+**归一化查询值**——domain 小写等按类型注册；TTL 双档 config）/A.2 ThreatIntelProvider 端口+HTTP adapter（映射层独立函数 mapUpstreamResponse——契约到手只改此函数）+两态桩（stub config/fail-fast ErrIntelNotConfigured 防虚标）/A.3 查询代理链路（命中/懒过期**异步回源**/未命中回源/**singleflight 防击穿**——同 key 并发仅一次上游调用，保护配额即本件存在意义+上游令牌桶自留量）/A.4 API 契约（intel:query 唯一新码；phase6 R1 消费=进程内 service 直依赖不经 HTTP 自调用）/A.5 验收补充（singleflight 并发断言/降级矩阵/归一化/ACL 透传负向） |
| 2026-10-16 | **评审批深审项 D（所有者「落」并批）**：A.3 补**上游 404=正常结果入缓存**（clean 答案缓存 TTL 内免打上游——保护配额；错误≠查无前者走失败链；404 vs 200+空结果形态随契约对表） |
| 2026-10-16 | **404 缓存验收断言批（一致性尾巴修复）**：A.5 补「404 入缓存→二查零上游调用；上游 5xx 不缓存走失败链」用例（对齐 A.3 深审项 D） |
| 2026-10-19 | **P0-3 非法 DDL 修正批（并行外审合并报告）**：A.3 索引改普通 `(expires_at)`——原 partial index 谓词 `WHERE expires_at < NOW()` 非法（PG 要求 IMMUTABLE，NOW() 是 STABLE 不满足，B9-1 迁移即失败）；过期判定移入查询层 |
| 2026-10-19 | **P0 修正批二轮回声（纯精度）**：DDL 注与上行「NOW() 是 VOLATILE」→「STABLE」（PG 语义 now()=STABLE，非 IMMUTABLE 即不可入索引谓词——结论不变；上行同批未提交随改，无历史改写） |
| 2026-10-19 | **多源聚合留口登记（所有者）**：§6 边界行补留口注——后续拟对接多源（微信公众号等订阅/推送形态），端口天然多实现/intel_type 可扩，聚合语义（查询层合并）属 phase6 R1，触发驱动不排期 |
| 2026-10-19 | **采集域归属澄清批（所有者问询「文档是否迁移 phase6」）**：§6 补**采集域归属**注——多源采集立项时**新建 phase6 文档**（通用数据录入接口形态——03 号演进总表行），**本文件不迁移**（W11–13 phase5 交付物；pull 缓存代理≠push 采集入库两形态语义相邻但工程身份不同） |
