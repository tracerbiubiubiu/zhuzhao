# 08 · phase5 需要添加 / 修改的计划项（基于段稿逐项核验）

> 性质：发散研究稿（见目录 README），本文为对 **phase5 SSOT 的改动建议**——逐项核验了 `docs/phase5/01–11` 段稿后给出；采纳后按文档同步纪律写入 `docs/phase5/`（含 11 号实施计划的批次表与 §5.9 映射）。核验日期 2026-10-20，phase5 处于 W1（S5 B5-1 开工）。
>
> 三类标记：**A 登记类**（改文档不改代码，零成本）· **B 随行件**（≤1 天的设计约束或小改，需所有者点头进 11 号批次表）· **C 已满足**（无需改，含对前轮判断的修正）。

## 1. 结论速览

> **状态（2026-10-20 晚）**：所有者"其余都按照建议"→ P1–P5 全部批准；A 类与 B 类（P1/P2/P3/P5）**已写入 phase5 SSOT**（执行记录 §8），P4 按拍板放 phase7。下表"判断 / 建议动作 / 窗口"保留为核验时点记录；日历节点见 §7。

| 段 | 核验现状（段稿锚点） | 判断 | 建议动作 | 成本 | 窗口 |
|---|---|---|---|---|---|
| **S5 对象存储** | purpose 枚举已有（05:219）；**无通用 owner**（owner_kind/owner_id 零命中）；confirm 端点工单专属（05:251）；GC 三条全锚工单（05:224/237/242/314）；单桶域前缀 avatars/ tickets/（05:29）；**B5-1（000045）已于 10-19 收口** | **B 需修改** | file_objects 经**新 ALTER 迁移**追加 `owner_kind/owner_id` 可空列（000045 不改，A2 对账口径 000048——**实占 000047**，见 §8 实落更正）；前缀命名空间预留 `scripts/ artifacts/ ingest/ exports/`；confirm 内部按 purpose 绑定器接口化；GC 第③条改"绑定器汇报引用集" | +0.5–1 天 | **S5 收口前，随 B5-4/B5-5**（建表窗口已过，改 ALTER） |
| **S6 事件面** | DomainEvent 含 BizType/Action/BizID/ActorID/OccurredAt/`Payload json.RawMessage`（06:118-125）；异步派发（06:129）；收件人域无关（06:41）；outbox 明确排除但升级路径登记（11:323，1–2 天、DomainHook 零改） | **C 已满足 + A 登记** | 机制无需改；撤回前轮"D7 留门紧迫"判断。顺手采纳事件分类学：`Action = biz_type.verb` 固定词表、`updated` 事件载荷带 `changes[{field,from,to}]`、每 Action 预留 payload schema 登记位（D20 / 03-A9） | 0 | S6 一期（W5–7.5） |
| **S1 表单/取数** | DataFetcher 端口同步 3s（01:160-167），registry 代码注册 fail-closed（01:164），cmdb fetcher 预留（01:48）；字段类型封闭七枚举、新类型三处线性扩 1–2 天（01:31） | **A 登记 + C** | 01:48/160 的 "cmdb(预留)" 改为 "asset-platform（资产只查询，D4）"；asset-ref 控件按既有三处扩路径届时做 | 0 | 任意 |
| **S2 流程图** | 画布/mapper 分离已立（02:32/103-110），mapper 签名绑 transitions（02:118-119），壳层与工单 Pinia store 耦合（02:109），不合并红线（02:34） | **B 增强（可选）** | B2 批加验收项：壳层数据经 props/emit，不直连工单 store | +0.5 天 | W7.5 前 |
| **S3a 动作库** | `post_actions` 字符串数组无参数（03:113）；`TransitionAction.Execute(ctx, ev, *model.Ticket)`（03:146-155）；"提交任务"仅一句理想态（03:40） | **B 需显式添加** | B3-3：`post_actions` 元素允许 `{name, params}` 对象形态；接口加可选 `ParamsSchema() json.RawMessage`（nil 合法）；Execute 签名暂不动 | +0.5 天 | W13 前 |
| **S4 任务面** | `ActionMeta{Name, Description, ParamsSchema json.RawMessage}` 自由 JSON（04:121）；按 schema 渲染=远期、近期 JSON 模式（04:36-38）；明注单执行端"防误扩展"（04:127）；任务定义无 kind | **A 登记 + B 极小** | ParamsSchema **钉为 JSON Schema 2020-12 子集**（与 S1 渲染器、AI 工具同语言）；`GET /jobs/actions` 响应加 `kind:"handler"`、`owner_service` 两字段；"防误扩展"注释改为"多执行端=phase7 触发，见能力目录" | ~0 | W11 前 |
| **S9 缓存代理** | 机制通用（键归一化/TTL per type/singleflight/stale，09:19/68/83/115-117）；实现焊死情报语义：表 intel_cache、端口 ThreatIntelProvider、API /intel/query、权限 intel:query（09:67/93/122） | **B 需修改边界** | B9-1：缓存机制层独立包（如 `internal/pkg/apicache`：key namespace + TTL per type + singleflight + stale），情报为首个消费者；表名/端口名可保留 | +0.5–1 天 | W11 前 |
| **S10 cron 可观测** | 只覆盖进程内 CronScheduler（10:18/50/214）；timeout_scan 骑 CronScheduler（10:47/210） | **C 已满足** | 在 D18 分工口径下无冲突；撤回前轮"phase6 README 口径冲突"判断 | 0 | — |
| **README §6 端口表** | 四列格式 `端口｜段｜形态｜内网时对接方`，8 行（README:201-210） | **A 登记** | 加 5 行（§2.1 给出可直接粘贴的行） | 0 | 任意 |
| **03 演进总表** | 03:228 已登记"通用数据录入接口（统一信封+按源 schema 注册+AK/SK+raw 留档+DomainEvent 出站）"归 phase6，前置 S5/B6-1/S4 | **A 登记** | 该行补"拉取侧：http-pull / db-pull / file-pull + 水位表"（所有者推/拉口径） | 0 | 任意 |
| **S7 / S4 §3.6 备忘** | S7"已拍后置"（07:3）；任务域备忘"不构成触发"（04:56） | **A 登记** | S7 加注"复活形态=智能面（研究稿 07 号 §4.6 phase11 / D17），场景条件已满足、供给条件未满足、范围后置讨论"；§3.6 加注"已进入 phase7/8 规划视野（研究稿），本 phase 口径不变" | 0 | 任意 |

## 2. A 类：登记（改文档不改代码）

### 2.1 README §6 端口表新增五行（按既有四列格式）

```
| `IngestSource` / `SourceAdapter`（统一接入：推/拉） | phase6 新段（跨段引用；03 演进总表 03:228 行） | 推：上报 API + webhook 入口站；拉：http-pull / db-pull / file-pull + `ingest_sources` 水位表；每源一 normalizer | 漏扫平台（首源）→ SOC / 日志平台告警 |
| `AssetResolver`（资产查询门面） | phase6 R3 / 告警路由（跨段引用） | `Resolve(identifiers) / List(kind, org)`——调外部资产平台，字段归一 + 键映射 + 可选读穿缓存；实体存 asset_ref + asset_snapshot，**不建台账（D4）** | 公司资产平台 |
| `Connector`（声明式出站连接器） | phase7（跨段引用） | HTTP 连接器通用件：auth 类型族 / 限流 / 熔断 / 健康；operation → `actions(kind=connector)` | SOC / SOAR / 资产平台 / 智能体平台 |
| `ScriptExecutor`（沙箱执行端） | phase8（跨段引用） | taskrunner 第二执行端；契约 = manifest + stdin/stdout/stderr/env/exit code（研究稿 05 号） | — （内部） |
| `AgentProvider`（AI 智能体平台） | 终态（D17，跨段引用） | 出：智能体为能力来源（kind=agent 连接器）；入：产出经统一接入层；反向：Action/MCP 暴露 + on-behalf-of | 公司 AI 智能体平台 |
```

### 2.2 其余登记项

- `01-s1-form-designer.md:48/160`：`cmdb(预留)` → `asset-platform（资产平台查询门面，只查询不建台账——D4）`。
- `03-s3-flow-semantics.md:228`：通用数据录入接口行补"拉取侧"与水位表（见 §1 表）。
- `04-s4-task-console-ux.md:121`：`ParamsSchema json.RawMessage` 注明 **JSON Schema draft 2020-12 子集**；`:127` "防误扩展"注释改为指向能力目录触发条件；`:56` §3.6 备忘加"已进入规划视野"注记。
- `07-s7-ai-assistant.md:3`：加复活形态注记（场景条件已被 2026-10-20 愿景输入满足；供给条件未满足；形态见研究稿 07 号智能面 + D17）。
- `06-s6-message-notification.md` A.1（DomainEvent）：登记事件分类学约定——`Action = <biz_type>.<verb>`，verb 取固定词表（实体类 created / updated / status_changed / assigned / commented / closed / reopened / deleted；执行类 started / succeeded / failed / canceled / dead / timed_out；系统类 ops.*）；`updated` 载荷必带 `changes: [{field, from, to}]`；每 Action 预留 payload JSON Schema 登记位。零代码，防止 phase7 运行类事件接入时重命名（D20 / 03-A9）。
- `README.md` §5/§7 不改口径（S7 仍"已拍后置"，不计基线）。

## 3. B 类：随行件（建议进 11 号 §1 总表 / §2 段表 / §5.9 映射）

> **状态（2026-10-20 晚）**：B5 / B2 / B3-3 / B4 / B9-1 五项已写入 11 号 §1 / §2 / §5.9 与对应段稿（见 §8）；"可选"一行（on_success）按 P4 拍板放 phase7。

| 批次 | 随行件 | 为什么现在 | 成本 |
|---|---|---|---|
| **B5-4/B5-5（S5，W2–3）** | ① `file_objects` **新 ALTER 迁移**追加 `owner_kind VARCHAR(30) NULL, owner_id TEXT NULL`（零回填、down=DROP；工单附件填 ticket/头像填 user，便于后续 script_package / run / ingest_event 复用）；② key 前缀命名空间文档化预留 `scripts/ artifacts/ ingest/ exports/`（不建、只占名）；③ confirm 流程内部拆为"通用确认 + 按 purpose 绑定器"接口（对外路由不动）；④ GC 第③条"悬挂对象对账"改为各绑定器汇报引用集的 union（首个绑定器=解析 custom_data，行为不变） | **B5-1（000045）已于 10-19 收口、零迁移窗口已过**——改为一枚 ALTER 迁移，成本不变；GC/confirm 焊死工单域后，脚本包/工件/原始日志/导出四类对象进来要改模型，接口化仍须在 S5 内约定 | +0.5–1 天 |
| B2-x（S2） | 壳层 `FlowGraphDesigner/index.vue` 数据经 props/emit，不直连工单 Pinia store（验收项） | playbook 设计器复用画布层的前提；不越 02:34 不合并红线 | +0.5 天 |
| B3-3（S3a） | `post_actions` 元素允许 `{name, params}`；`TransitionAction` 加可选 `ParamsSchema()` | 动作参数化是 Action 注册表对齐的最小面；接口泛化（去 `*model.Ticket`）不在本阶段做 | +0.5 天 |
| B4-x（S4） | `GET /jobs/actions` 加 `kind`、`owner_service`；ParamsSchema 钉标准 | 让 S4 的"按 schema 渲染"与 S1 渲染器、AI 工具定义三者同语言 | ~0 |
| B9-1（S9） | 缓存机制层独立包，情报为首个消费者 | 资产查询门面读穿缓存直接复用，避免二实例并行 | +0.5–1 天 |
| （可选，需单独拍）随 S4 批 | taskrunner `on_success` 钩子 + `job_runs.result` 摘要列（16 号"最小替代"两件套） | 立刻让"轻编排=回调串联"可用，phase8 不改表；但属范围增加 | 0.5–1 周 |

合计（不含可选项）≈ **2–3.5 天**，分散在五个批次里。

## 4. C 类：已满足 / 对前轮判断的修正

1. **S6 事件面无需改**：结构体可序列化（`Payload json.RawMessage`）、事务提交后异步派发、收件人与 message_log 均 biz_type 域无关、outbox 升级路径已登记（11 号 §5.10：同事务写 outbox + 投递，DomainHook 消费方零改，1–2 天）。前轮把 D7 列为"最近窗口必拍"**过度**——它是 phase7/9 的触发式决策，不是 phase5 的。
2. **cron 归属改为"分工口径"（新增 D18）**：进程内 CronScheduler = 平台自身维护型扫描（超期 / 到期 / 对账 / GC——需同进程 DB 事务，advisory lock 已就位，S10 可观测面覆盖）；taskrunner = 业务任务 / 脚本 / 连接器拉取 / 一切需要重试·死信·审计·UI 可见的周期工作；**CronScheduler 不再承接业务任务**。在此口径下 phase6 README"扫描骑 CronScheduler"与 S10 设计都成立，前轮所称"口径冲突"撤回；03-A4 已同步改写为"分工收敛"。
3. **S1 DataFetcher 与求值器端口已满足**：读穿缓存归资产门面侧，不改 fetcher；3s 超时意味着门面必须命中缓存或快速返回——这是门面的设计约束。
4. **S2 目录级分层已满足**：壳层去耦是增强项（B 类可选），不是缺口。

## 5. 对前轮"现在就需要确认"清单的更新

| 前轮编号 | 变化 |
|---|---|
| #1 D7 事件面留门 | **撤回**——S6 已满足，转为 phase7/9 触发式决策 |
| #2 cron 归属 | **改为 D18 分工口径**（维护型留进程内 / 业务·拉取·脚本走 taskrunner），建议采纳；phase6 扫描任务维持原口径 |
| 新增 | **S5 本周随行件**（§3 第一行）——唯一真正"本周"的项 |
| 其余 #3–#11 | 不变 |

## 6. 落档方式

- A 类：直接改 `docs/phase5/01 / 03 / 04 / 07 / README §6`（零代码），可随任一提交批。
- B 类：写入 11 号 §1 总表对应 Wave 行的"批次"列（随行件先例：W1 行"架构依赖规则工具化开工首件"、B5-5 cron 最小件）、§2 段表与 §5.9 映射总表；S5 两列经**新 ALTER 迁移**（000045 已收口不改，口径 000048——**实占 000047**，见 §8 实落更正）随 B5-4/B5-5 落。
- 本文不改 SSOT；以上均待所有者点头。→ **2026-10-20 晚所有者"其余都按照建议"：P1–P5 已批准**，执行记录见 §8。

## 7. 日历时间节点与"新功能 vs 随行件"原则

**所有者问：是完成现有计划后再继续添加功能吗？——是。** 新功能一律在 phase5 收口后按 phase6 → 7 → … 顺序进入，不与 phase5 并行（单人串行）。phase5 内只放两类东西：① **零代码登记**（A 类，随时）；② **随行件**（B 类，≤1 天的预留 / 约束，必须在对应段"建表 / 定接口"的那一周就位，否则后补就是迁移或重构）。它们不是功能，是给后续 phase 留的门。

按 11 号实施计划 Wave 序折算（W0 收口 2026-10-19，W1 起算 2026-10-20，单人、不计节假日，日期为约值）：

| 项 | 挂靠批次 | 必须就位的时点 | 日历（约） | 性质 |
|---|---|---|---|---|
| **P1** S5 随行件四项（owner 两列 / 前缀占名 / confirm 绑定器 / GC 绑定器） | B5-4 首件（**已实落 70a33af**） | **S5 收口前**（owner 两列 ALTER **实占 000047**——§8 实落更正，B5-6 顺位 000048；B5-1 已收口、建表窗口已过） | 已于 2026-10-20 随 B5-4 落 | 随行件 +0.5–1 天 |
| **P5** A 类登记（§6 五行 / 03:228 拉取侧 / 01 asset-platform / S4 schema 钉标注记 / S7 注记 / §3.6 注记 / S6 分类学） | 文档批 | 随本批提交 | **本周** | 零代码 |
| S6 事件分类学约定（D20） | B6-1（W5–7.5） | DomainEvent 定稿前 | **2026-11-23 前** | 零代码约定 |
| **P3** B2 壳层去耦验收项 | B2 批（W7.5–9） | 壳层组件开工前 | **2026-12-10 前** | +0.5 天 |
| **P2** B4：ParamsSchema 钉 JSON Schema + `/jobs/actions` 加 kind / owner_service | B4 批（W11–13） | 端点定稿前 | **2027-01-04 前** | ≈0 |
| **P2** B9-1：缓存机制层独立包 | B9-1（W11–13） | 包边界定稿前 | **2027-01-04 前** | +0.5–1 天 |
| **P2** B3-3：post_actions `{name, params}` + 可选 `ParamsSchema()` | B3-3（W13–15） | 动作库框架定稿前 | **2027-01-18 前** | +0.5 天 |
| **P4** on_success + result 摘要列 | — | 不进 phase5 | phase7 地基批（2027-04） | 已拍放 phase7 |

phase5 之外第一批"新功能" = phase6（起点单口径：S6+S10 交付即启动 ≈2027-01 初；若等一期全收口则顺延至 2027-02——07 §6 同口径）。

## 8. 执行记录（2026-10-20 晚，SSOT 回灌已落，未提交）

A 类 + B 类（P1–P3、P5）已写入 phase5 权威文档，`git diff --stat`：`docs/phase5/01 02 03 04 05 06 07 09 11 README` + `docs/review/11-project-control.md` 共 11 文件，+53 / −29，纯文档：
- README §6 +5 行端口；01 两处 cmdb → asset-platform；03 演进总表拉取侧 + post_actions 对象形态 + `ParamsSchema()` 可选方法注记；04 §3.6 注记 + `/jobs/actions` 响应 `kind`/`owner_service` + ParamsSchema 钉 JSON Schema 2020-12 + A.2 "防误扩展"改写；05 A.3 新增"随行件迁移"段（ALTER 追加 `owner_kind/owner_id` 两可空列，口径 000048；§3 表族清单同步）+ §3 前缀占名 + A.4 confirm 绑定器注记 + 000046 段 GC 第③条绑定器 union + 迁移计数三枚→四枚 + 变更记录行；06 A.1 事件分类学约定段；07 复活形态注记；02 A.2 壳层去耦随行件；09 §1 缓存层独立包随行件；11 号 §1 五行 / §2 七行 / §5.9 六行映射（S5 两列经新 ALTER 迁移随 B5-4/B5-5）；review/11 头部 2026-10-20 批次行。
- P4 按拍板不进 phase5（phase7 地基批）。
- 抽查：README 表 4 列、review/11 行格式均正确；无代码 / 迁移 / 权限码 / 路由变更。
- **校正（同晚全量复核）**：核实 11 号后发现 **B5-1（迁移 000045）已于 10-19 收口**（740d2e9），代理首版把两列直接写进 000045 的 DDL 文本、并在 11 号 §5.9 写"随 B5 建表迁移落、不另开编号"——与事实不符，已改为：05 号 A.3 恢复 000045 原文 + 新增"随行件迁移"段（`ALTER TABLE file_objects ADD` 两可空列，down=DROP，零回填；编号按 A2 当天对账、口径 **000048**——排在 B5-6 的 000047 之后；随 B5-4/B5-5 落）；05 号 §3 表族注、§10 涉及文件与 B5-6 段的迁移计数三枚→四枚；05 号变更记录加 2026-10-20 回灌行；11 号 §1 W1–3 行、§2 B5-4 行、§5.9 B5-1/B5-4 行同步校正。本文 §1/§3/§7 的 S5 行与窗口随之改为"S5 收口前、随 B5-4/B5-5（~10-27 ~ 11-08）"。
- **终审补齐（同晚）**：① `docs/phase5/10-s10` cron 设施行加 D18 调度分工口径注（timeout_scan 属维护型，口径成立）；② `docs/phase5/README.md` §2 S4 任务域备忘加"已进入 phase7/8 规划视野、本 phase 口径不变"注（与 04 号 §3.6 同注）；③ `docs/phase3/17-uaa-integration.md` §6#7 source 列状态改"✅ 已拍方案 a（随 phase6 迁移批合批）"；④ review/11 批次行补记上述三项与 000048 口径。docs 侧合计 13 文件改动，仍为 doc-only、未提交。
- **计数更正（2026-10-20 双审计终核）**：docs 侧 13 文件终值实核 **+59/−36**（首报 +53/−29 未含终审三补齐追加；11 号 §2 实改 8 行 / §5.9 实改 7 行，与首报"七行/六行"各 ±1）——以本行终值为准。
- **P1 随行件迁移实落（2026-10-20 B5-4 后端首件）**：owner 两列迁移**实占 000047**（非本文预估口径 000048）——B5-6 未启动、golang-migrate 不允许事后插入更低版本，A2 先建先占，B5-6 KV 顺位 000048；写侧三路径同批（ticket 态内联 / avatar `SetObjectOwner` 归属先于回写）；四档 acceptance 237/0。本文 §1/§3/§6/§7 的"000048"口径为预估留痕不改，以 docs/phase5/05 A.3 与 review/11 §4 实落为准。
