# Phase 3 · 审计日志 L2（audit-l2）

> **定位**：审计管道从 Phase 1 的「中间件同步写 audit_logs」升级为 **L2（Redis List 缓冲，进程崩溃不丢）**，并补齐两块治理缺口：**B11① L2/L3 策略评估日志**（补 L2 拒绝无留痕盲区）、**B11② 审计归档**（大表治理，先建表必先有归档）。
> **Wave 归属**：~~W1（Step 3）~~ **B11① 可与 M-E 合并实施（价值独立于多实例）；B11② 实施随 M-E 事件/任务总线（taskrunner）首个预置动作**（~~Asynq 任务平台~~ 2026-09-03 M-E 收敛为事件/任务总线；zhuzhao 侧配套落位见 [16 号 §3 E-③](./16-external-integration.md)，对齐 taskrunner M3）。
> **状态**：**已编写（2026-09-02，替换原占位）**。写入管道等决策点保留为 ⚠️ 待拍板。
> **权威出处**：检查单 B11 / 11-project-control §8 B9·B11 / go-wind-admin 调研（2026-09-01）。
> **标记约定**：`🚦` = 触发条件驱动，由所有者决定；`⚠️` = 待拍板/不确定性。

---

## 1. 现状与问题

| 现状 | 问题 |
|---|---|
| Phase 1：操作日志中间件**同步写** `audit_logs`（含脱敏 + action 注册） | 请求路径同步写 DB，高并发下增加延迟；进程崩溃在写库前不丢（事务内），但无缓冲 |
| L3 路由拒绝：有 slog Warn + 审计行带 403/404 | ✅ 有留痕 |
| **L2 scope 拒绝：完全静默** | ❌ **盲区**：`resource.Authorize` / `scope_resolver.resolve` 拒绝无任何日志，无法审计「谁在资源级被拒」 |
| `audit_logs` 持续增长 | ❌ 无归档/清理机制，长期膨胀；判定日志是**天然大表**，先建无归档 = 重蹈覆辙 |

---

## 2. 审计管道 L2（Redis List 缓冲）

### 2.1 目标

审计写入改为：**中间件/Service → 内存 channel → Redis List → 异步 goroutine 落库**，进程崩溃不丢日志（Redis 持久化 + 未落库前 List 内保留）。

### 2.2 数据流

```
业务请求
  ├─ 审计中间件  ──写入──▶  L2 writer（channel 缓冲）
  ├─ 判定日志埋点 ──写入──▶   │
  │                          ▼
  │                  Redis List（audit:logs / audit:policy_eval）
  │                          ▼
  │              后台 goroutine 批量落库（audit_logs / policy_evaluation_logs）
  ▼
响应返回（不阻塞）
```

- 写入动作**不阻塞业务**；Redis 不可用时按 ⚠️ 失败容忍策略处理（见 §7）。
- 落库失败重试 + 保留原始记录，防止静默丢审计。

### 2.3 配置段（草案）

```yaml
audit:
  pipeline: l2            # l1（同步写 DB，现状）| l2（Redis List 缓冲，Phase 3）
  buffer_size: 1024       # 内存 channel 容量
  redis_list: audit:logs  # Redis List key
  batch_size: 200         # 批量落库条数
  flush_interval: 1s      # 落库周期
  fail_policy: open       # ⚠️ open（吞错不阻断业务）| close（阻断/降级）—— 待拍板
```

---

## 3. B11① L2/L3 策略评估日志（判定日志）

### 3.1 目的

补 **L2 scope 拒绝无留痕盲区**：每次资源级鉴权判定（允许/拒绝）落一行，可审计、可排障、可复盘越权尝试。

### 3.2 判定日志表（DDL 草案，迁移编号启动时按 A2 核对）

```sql
-- 迁移编号：⚠️ 启动时核对（当前占用至 000019；判定日志表为 Phase 3 新增）
CREATE TABLE policy_evaluation_logs (
    id               BIGSERIAL PRIMARY KEY,
    actor_id         BIGINT NOT NULL,             -- 操作人
    actor_role_codes TEXT[],                       -- 角色码（L1 展开结果）
    resource_type    VARCHAR(50) NOT NULL,         -- ticket / org / ...
    resource_id      VARCHAR(100) NOT NULL,        -- 资源标识（如 ticket:123）
    action           VARCHAR(50) NOT NULL,         -- approve / update / close / ...
    scope_axis       VARCHAR(20),                  -- L1 | L2 | L3（判定层）
    scope_detail     JSONB,                        -- 解析轴细节（锚点/scope 快照，可选）
    result           BOOLEAN NOT NULL,             -- 允许 / 拒绝
    reason           VARCHAR(200),                 -- 拒绝原因（如 scope mismatch / 非属主）
    trace_id         VARCHAR(64),                  -- 请求 trace 串联
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_pel_created ON policy_evaluation_logs(created_at);
-- ⚠️ 大表：必须先落 B11② 归档（保留期默认 180 天）再启用，否则无限膨胀
```

> **参考（go-wind-admin `sys_policy_evaluation_logs`，2026-09-01 调研）**：装饰器引擎包装 `IsAuthorized` 每次判定同步落一行（写失败吞错不阻断鉴权），字段含 result / effect_details / evaluation_context / trace_id。**注意其只盖 API 级鉴权（数据级 scope_sql 为预留空字段）**——zhuzhao 的痛点在**资源级 L2**，必须埋在 `resource.Authorize` / `scope_resolver.resolve`，**不能照抄中间件装饰器模式**。

### 3.3 埋点位置

| 位置 | 覆盖 | 埋点方式 |
|---|---|---|
| `resource.Authorize` | 资源级 L2（scope/虚拟组/BFS 三源/委托） | 判定入口统一埋（含结果+原因） |
| `scope_resolver.resolve` | scope 解析轴 | 解析细节入 scope_detail（可选，量可控时开） |
| 中间件层 | L1 路由拒绝（已有 slog Warn + 审计 403/404） | 维持现状，不重复落 |

- 埋点写入随 §2 管道（L2 writer），与业务同事务解耦、不阻塞。
- ⚠️ **失败容忍**：写失败默认 fail-open 吞错不阻断鉴权（参考 go-wind-admin），与 §7 拍板一致。

### 3.4 全链路 request_id 关联（2026-09-03 核查登记；目标 = 日志全链路可追溯）

**现状矩阵**（✓ 已关联 / ✗ 断链）：

| 环节 | request_id 现状 |
|---|---|
| HTTP 入站（RequestID 中间件：生成/接受入站 `req-`+32hex） | ✓ gin context + 响应头 |
| AccessLogger（slog，每请求一行，含 401/403） | ✓ |
| `audit_logs` 表 | ✗ **无该列**（user+path+时间窗只能近似） |
| Casbin L1 拒绝 slog Warn（`middleware/casbin.go`） | ✗ 打 userID/path/method/roles，无 rid（AccessLogger 另有一行含 rid，靠近似） |
| service 层（`resource.Authorize` / `scope_resolver` / 业务 slog） | ✗ **request_id 未注入 `c.Request.Context()`**（仅 gin ctx），service 拿不到 |
| `ticket_events`（L1 事件表） | ✗ 无该列 |
| `policy_evaluation_logs`（B11①，DDL 草案） | 草案有 `trace_id` 列，来源定为 request_id |
| 出站 → taskrunner（提交/触发） | ✓ 契约有 body `request_id`（taskrunner 落 `job_runs.request_id`）；zhuzhao client（E-④）实施时取 ctx rid 透传 |
| taskrunner 回调 → zhuzhao `/internal` | ⚠️ body 带 request_id，**但 HTTP 头不带 → zhuzhao 入站中间件会生成新 rid，回调链路（access log/审计/handler 日志）与 job_runs 断链** |
| 出站 → activelist（client 封装层） | ✓ 已拍板生成并透传 `X-Request-ID`（ADR-003 审计落点专节）；对齐增强：**优先透传入站 rid、无则生成** |

**E-① 实施清单**（一次迁移 000020 合并 DB 侧三处）：

1. **RequestID 中间件**：rid `context.WithValue` 进 request context（打通 service/判定点/事件写入）；
2. **`audit_logs` 加 `request_id` 列** + **`ticket_events` 加 `request_id` 列**（事务内写事件的 repo 从 ctx 取，G1 后天然可得；L1 事件 → 触发动作的追溯依赖它）；
3. **Casbin L1 拒绝打点补 rid**（gin ctx 现成，`c.GetString("request_id")`）；
4. **`policy_evaluation_logs.trace_id` = request_id**（原 §3.4 内容）；
5. **跨服务两刀**：taskrunner callback client 回调时带 `X-Request-ID` 头（= payload request_id，有则带；zhuzhao 入站中间件「接受入站」逻辑直接复用同一 rid——cron 触发 request_id 为空则不带，zhuzhao 生成新 rid，靠 task_id 关联）；activelist client 封装层 `X-Request-ID` 优先透传入站 rid。

打通后全链路一键贯通：**slog = `audit_logs` = `policy_evaluation_logs` = `ticket_events` = taskrunner `job_runs` = activelist 访问日志/`activelist_audit_log`**，同一个 `req-` 键。

---

## 4. B11② 审计归档（随 M-E taskrunner 首个预置动作，回调 zhuzhao 执行，16 号 E-③）

### 4.1 目标

`audit_logs` + `policy_evaluation_logs` 超期数据**导出 JSONL 后删行**，控制大表膨胀。

### 4.2 机制（参考 go-wind-admin）

| 项 | 建议 |
|---|---|
| 触发 | taskrunner cron job（如每日 03:30）回调 zhuzhao `audit_archive` handler；归档漏跑可容忍 > 并发（阻塞/去重随 job 策略） |
| 保留期 | 默认 **180 天**（等保 ≥6 个月口径），配置可调 |
| 导出 | JSONL，单批 5000 行，单表失败跳过（不阻塞另一表） |
| 删除 | **导出成功后才按同批 id 删行**（防「删了没导出」）；导出失败 → handler 返 5xx（P7：业务失败映射状态码）→ taskrunner 重试 + task_id 幂等兜住重复执行 |
| 存储 | ✅ **已拍板（2026-09-03）**：本地 JSONL（Docker 卷）+ 纳入宿主卷备份，对象存储后置；卷备份覆盖 180 天口径入 M-Mig 部署清单（见 §7 D3） |

### 4.3 顺序

先建表埋点（随 M1/M-E，B11①）→ Asynq 到位（随 **M-E**）→ 接归档 periodic task（B11②，M-E 首个预置任务）。归档只需 Asynq 落地（M-E），**不依赖多实例 M1**（2026-09-02 §22.1/§23 修订：M2 硬依赖 = Asynq 底座；M2 工单业务随 §23 暂缓，Asynq 基建归 M-E）。

---

## 5. 涉及文件（规划）

```
internal/middleware/audit.go        # 审计中间件接入 L2 writer（pipeline 开关）
internal/service/audit_service.go   # 落库 goroutine + 批量写
internal/service/ticket/resource.go # resource.Authorize 判定日志埋点
internal/service/ticket/scope_resolver.go # resolve 埋点（可选）
internal/pkg/audit/                 # L2 writer（channel + Redis List）
configs/config.yaml                 # audit 段
migrations/                         # policy_evaluation_logs（编号启动时核对）
internal/task/audit_archive.go      # Asynq 归档 periodic task（M-E）
```

---

## 6. 验收用例

| # | 用例 | 通过标准 |
|---|---|---|
| AL1 | L2 拒绝留痕 | scope 拒绝请求 → `policy_evaluation_logs` 落一行（actor/资源/动作/scope 轴/结果/原因/trace_id） |
| AL2 | L2 允许留痕 | 正常授权请求 → 允许行（result=true） |
| AL3 | 写失败不阻断 | 判定日志写入失败（mock Redis down）→ 鉴权仍正常（fail-open） |
| AL4 | 崩溃不丢 | L2 writer 缓冲未落库 → 进程重启后 List 内记录仍落库 |
| AL5 | 归档导出后删 | 超期行导出 JSONL 成功 → 原行删除；导出失败 → 不删（M-E） |
| AL6 | 保留期可配 | 修改保留期配置 → 归档边界随之变化（M-E） |

---

## 7. 待决策点（⚠️）

| # | 事项 | 建议 | 状态 |
|---|---|---|---|
| D1 | 写入管道：同步落库 vs Redis List 缓冲 | ✅ **已拍板（2026-09-03）：异步写**——判定日志每次鉴权都触发，同步写在请求路径上代价不可接受（所有者拍板）。形态 = **内存 channel → Redis List（AOF）→ 批量落库 goroutine**（本文 §2 原设计）；选 Redis 而非纯协程管道的关键论据：**鉴权链对 Redis 本就 fail-close**（黑名单/user:disabled，design-decisions §1.5）——Redis 挂时请求到不了 Authorize，writer 依赖 Redis **零新增可用性风险**，持久化等于免费；崩溃不丢（AL4），多实例直接复用 |
| D2 | 失败容忍：fail-open vs fail-close | **fail-open**（吞错不阻断鉴权/业务，参考 go-wind-admin）——D1 拍板异步后此项基本只剩「channel 满丢弃」一个语义，随 E-① 实现确认 | 待最终确认 |
| D3 | 归档存储位置：对象存储 vs PG 备份 vs 本地 | ✅ **已拍板（2026-09-03）**：本地 JSONL（Docker 卷）+ 纳入宿主卷备份，对象存储后置；注意点：删库重建场景归档不随 PG dump 回来，180 天等保口径靠**卷备份**覆盖——入 M-Mig 部署清单 |
| D4 | 判定日志是否默认全开 | 量可控时开；超量可降采样/仅记录拒绝 | 待拍板 |
| D5 | 保留期默认值 | 180 天（等保口径），可配 | 建议沿用 |

---

## 8. 变更记录

| 日期 | 说明 |
|---|---|
| 2026-09-01 | 建范围占位（B11 登记） |
| 2026-09-02 | 正式编写：管道 L2 + B11① 判定日志 + B11② 归档 + 埋点/DDL 草案 + 验收用例 + 待决策点 |
| 2026-09-03 | 拍板同步：D1 写入管道 ✅ **异步**（channel → Redis List → 批量落库，所有者拍板）+ D3 归档存储 ✅ 本地卷；新增 §3.4 **全链路 request_id 关联矩阵**（request_id 注入 ctx / audit_logs·ticket_events 加列 / Casbin 打点补 rid / taskrunner 回调带 X-Request-ID / activelist client 透传入站 rid，随 000020 迁移合并）；§4 B11② 改 taskrunner 回调形态（导出失败返 5xx + 幂等，P7 定案） |
