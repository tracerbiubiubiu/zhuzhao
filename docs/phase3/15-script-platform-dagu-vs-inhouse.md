# 15 · 脚本任务平台选型调研：Dagu（现成）vs 自研（Asynq）

> 状态：调研稿（2026-09-03）｜关联：M-E 事件与任务平台（[13-implementation-plan](./13-implementation-plan.md)）、ADR-002（Asynq 执行器）、[14-planning-overview](./14-planning-overview.md)
> 目标：M-E「自定义脚本任务」（上传 shell/python 脚本，定时/手动运行，管理/历史/日志）选「现成平台集成」还是「自研」做决策支撑。
> ⚠️ 待拍板点已标注，由项目所有者决定。

> **🔻 收敛声明（2026-09-03，SSOT = [13](./13-implementation-plan.md) M-E 行 + taskrunner 仓库 `docs/taskrunner.md`）**：M-E 已从「任务平台」收敛为**事件/任务总线**（taskrunner）——**脚本任务（上传 python/shell）暂不需要，降级 🚦 按需再启**；核心 = Asynq 异步触发**预置动作**（回调 zhuzhao 内网端点，代码注册 handler，非用户上传脚本）。本文档保留为**按需再启时的选型参考**（届时重读 §2–§6）。本次收敛后，仅「脚本任务」这一个子能力是否用现成平台需要决策；事件/任务总线本体 = Asynq（ADR-002），不引入任何平台。

---

## 1. 需求回顾（M-E 脚本任务）

- 允许上传自定义 shell/python 脚本，定时（cron）与手动触发执行；
- 执行记录、历史、日志可查；失败重试、超时控制；
- 权限收死（仅全局管理员可配，2026-09-03 讨论中确认）；
- 执行"任意代码" → 安全（隔离/资源限制/审计）不是可选项；
- 事件/任务栈背景：zhuzhao 已有 L1 事件表（事件源）+ Asynq（执行器，ADR-001/002）；本次评估的是**脚本任务**这一新增能力是否引入现成平台。

## 2. Dagu 深度画像（候选 A：现成平台集成）

### 2.1 是什么
Dagu（`dagu-org/dagu`）——Go 实现的本地优先工作流引擎，单二进制、文件存储、无外部数据库，<128MB 内存。

### 2.2 能力
- **调度**：cron 表达式（含 `CRON_TZ` 时区）、重叠策略、catch-up 窗口、队列、重试、超时、人工任务；
- **执行器 19+**：shell、python、Docker、SSH、Kubernetes Jobs、SQL、HTTP 请求、`dag.run`（子 DAG）等；
- **Web UI**：看板（Cockpit/Kanban）、DAG 运行详情、实时状态、逐步骤日志、运行历史、失败重试、在线编辑 workflow YAML；
- **通知 + webhook**：运行事件路由到通知渠道；外部系统可通过 per-DAG webhook 触发；
- **REST API（OpenAPI）** `/api/v1`、`/api/v2`：
  - 触发：`POST /dags/{fileName}/start`（异步）、`POST /dags/{fileName}/start-sync`（同步）；
  - 定义管理：DAG Definition Endpoints（创建/修改/删除 workflow）；
  - 运行管理：stop / retry / status；
  - 查询：`GET /dags/{dagName}/history`（执行历史）、`GET /dags/{dagName}/log`（日志）；
  - 另有 gRPC（coordinator）、内置 MCP server。
- **分布式**：distributed workers（gRPC）、shared-nothing worker、队列并发控制；
- **部署**：Docker / Helm / Homebrew / npm / 裸二进制 / 系统服务（systemd / LaunchAgent / Windows Service）；
- **AI 原生**（可选面）：内置 LLM agent 创建/编辑/调试 workflow、Slack/Telegram 运维——zhuzhao 不依赖。

### 2.3 关键约束（影响决策）
- **许可 = GPL-3.0**（源码已加 GPL 头）。独立部署 + API 调用（进程隔离）通常不触发传染（非 AGPL，无网络服务强开条款）；**若内嵌/链接进 zhuzhao 代码库则必须整体 GPL**。⚠️ 需法务/所有者确认 zhuzhao 闭源商用边界。
- **状态存本地文件**（`~/.config/dagu` + `~/.local/share/dagu`：logs/data/suspend），无 DB → zhuzhao 侧"谁上传/谁触发"的业务审计要自己记（Dagu 只给运行日志/历史）。
- **无内置沙箱**：workflow 以宿主解释器跑子进程，与自研同等风险，需在桥接层补资源限制；Dagu 提供 step 级 `timeout`，但 CPU/内存/进程数 rlimit、进程组 kill 需自建或借助其 Docker executor 隔离。
- 维护活跃（2026 年持续发版，2300+ commits，249 tags）。

### 2.4 集成桥接设计（Dagu 方案核心工作量）
```
zhuzhao 上传接口（multipart，校验大小/类型/解释器白名单）
   → 脚本落盘到 Dagu 可访问目录（/opt/dagu/scripts/随机文件名）
   → 调 Dagu API 创建/更新 DAG（YAML：schedule=cron、run=bash|python3 <path>、timeout/retry）
   → 手动触发 = POST /dags/{name}/start；定时 = DAG 自带 schedule
   → 查结果 = GET /dags/{name}/history + /log
zhuzhao 侧 job_runs 表记录：上传者/触发者/脚本 hash/结果摘要（审计在 zhuzhao，Dagu 只执行）
```
桥接层预估 1–2 人日（不含平台部署与安全收口）。

## 3. 自研方案深度画像（候选 B：Asynq + job_configs + os/exec）

沿用 M-E 原设计 + 2026-09-03 安全清单：
- **底座**：Asynq Scheduler/PeriodicTask（cron 定时）+ worker + 重试/超时/死信；复用 Redis，零新增基础设施；
- **配置**：`job_configs` 表（脚本路径/参数/类型/周期/仅全局管理员）；`job_runs` 表（执行记录）；
- **执行器**：handler 内 `os/exec.CommandContext`（bash/python3，解释器白名单从配置读），超时 + 进程组 kill + rlimit；
- **管理端**：自建 CRUD / 启停 / 手动触发 / 历史端点（走 zhuzhao 三层鉴权）；
- **审计**：原生（job_runs + L1 事件），全自控；
- **安全**：受控用户 + 资源限制 + 超时三件套自建，演进可容器化；
- **工作量**：M-E 合计 6–8 人日（含脚本任务 + 管理端）；"现成 Web UI/历史/重试"这些 Dagu 白送的能力都要自己写。

## 4. 逐维度对比

| 维度 | Dagu（集成） | 自研 Asynq |
|---|---|---|
| Web 管理台 / 运行历史 / 逐步骤日志 | ✅ 全内置 | ❌ 自建（M-E 规划内） |
| cron 定时 / 重叠策略 / catch-up | ✅ 丰富 | ✅ 基本（PeriodicTask） |
| 手动触发 / 重试 / 超时 | ✅ start / retry / timeout | ✅ Enqueue / MaxRetry / Timeout |
| 上传脚本 | ⚠️ 需桥接（落盘 + API 建 DAG） | ✅ 原生 |
| 权限/审计与 zhuzhao 一体 | ⚠️ 审计在 zhuzhao 侧自记 | ✅ 原生三层鉴权 + job_runs |
| 安全隔离（沙箱） | ❌ 无内置，需桥接层补（或 Docker executor） | ⚠️ 自建三件套，演进容器 |
| 许可 | ⚠️ **GPL-3.0** | ✅ 无第三方平台 |
| 基础设施 | +1 独立进程（无 DB，文件存储） | 复用 Redis + PG |
| 故障/多实例 | 支持分布式 worker | Asynq 天然多 worker |
| 工作量（新增） | 桥接 1–2 人日 + 部署 + 安全收口 | M-E 6–8 人日（管理端自建） |
| 长期可维护性 | 依赖上游（活跃） | 全自控，无上游风险 |

## 5. 架构契合度判断

- **Dagu 方案**与 zhuzhao「网关 + 独立能力平台」模式一致（activelist 同构）：zhuzhao 作网关（鉴权/审计/编排），Dagu 只提供"调度执行脚本"能力。代价 = 引入一个 GPL 独立服务 + 无内置沙箱的安全收口。
- **自研方案**与 zhuzhao「单体优先 + 零新增基础设施」一致：全内嵌、审计原生、无许可问题，代价 = 管理端/历史/UI 全部自建（M-E 6–8 人日）。

## 6. 决策建议

1. 若「开箱即用、少写代码」优先，且接受 GPL 独立部署 + 自补安全收口 → **选 Dagu**（桥接层小，符合网关模式）；
2. 若「全自控、审计权限一体、无许可顾虑、不引入第二系统」优先 → **选自研 Asynq**（代价是多写管理端）；
3. 折中：**先做 Dagu 1 天 PoC**（验证上传→落盘→API 建 DAG→start/history 全链路 + 资源限制收口），跑通则选 Dagu；暴露集成缝/许可顾虑则回退自研。PoC 成本低，两个方案都先不动正式代码。

## 7. 待拍板点（⚠️）

- [ ] **许可边界**：zhuzhao 是否接受独立部署 GPL 服务（非内嵌）；若闭源商用，需确认合规口径。
- [ ] **沙箱程度**：两方案均需"受控用户 + 资源限制 + 超时 kill"最低三件套；生产/多实例是否直接上容器隔离（Dagu Docker executor / 自研容器化）。
- [ ] **脚本依赖策略**：仅标准库 + 白名单包，还是支持 requirements.txt（两方案同问）。
- [ ] **是否走 PoC**：先 Dagu 1 天验证再拍板，还是一步到位自研。
- [ ] 三个安全拍板点（上一轮讨论的权限边界/沙箱/依赖）与 M-E 脚本任务合并决策。

## 8. 变更记录

| 日期 | 变更 |
|---|---|
| 2026-09-03 | 建档：Dagu vs 自研深度调研（能力/API/许可/安全/桥接/工作量），作为 M-E 脚本任务选型决策支撑 |
| 2026-09-03 | 🔻 收敛：M-E 降级脚本任务为 🚦，收敛为事件/任务总线（Asynq 预置动作）；本文档转为按需再启时的选型参考（顶部收敛声明） |
