# 09 · 建议 vs 业界主流：对标自审

> 性质：发散研究稿（见目录 README）。所有者 2026-10-20 晚问"你的建议是否符合业界实践、与主流有无差距"。本文逐项自审，评级诚实标注，差距处给触发点与缓解。
>
> 评级：✅ 与主流一致 · 🟢 主流变体（有意简化，规模适配：单人 / compose / 内网）· 🟡 低于主流（有风险，需触发点复评）· 🔴 偏离主流（战略选择，非技术共识）

## 1. 逐项对标

| # | 建议 | 业界主流 | 评级 | 差距 / 缓解 |
|---|---|---|---|---|
| 1 | 统一 Action 抽象（JSON Schema I/O 注册表，五来源共用） | XSOAR integration/command YAML、Splunk SOAR app action spec、StackStorm action metadata、MCP tool——"schema 即契约"是收敛点 | ✅ | 主流还有内容包 / 市场与依赖管理（F22 后置）；建议补 **OpenAPI 导出**，SOAR 等外部集成方按 REST 习惯消费 |
| 2a | 脚本契约核心（manifest + 类型化 I/O + 日志/数据通道分离 + 退出码 + 超时取消） | XSOAR automations、Cortex analyzers、StackStorm、Rundeck、Lambda、GitHub Actions、Tekton 的公共交集 | ✅ | — |
| 2b | 输出通道 = stdout 整段单 JSON（SDK 把 print 重定向到 stderr 兜底） | GitHub Actions `$GITHUB_OUTPUT`、Tekton `/tekton/results/`、Argo 用**结果文件**，正是为避开 stdout 污染；Terraform external、多数 CLI 用 stdout JSON | 🟢 | 建议**加可选 `ZH_OUTPUT_FILE`**：存在则以文件为结果、stdout 全归日志；两通道并存，脚本作者自选（+0.2 天） |
| 2c | 沙箱第一档 = 容器池 + 子进程 + rlimit/seccomp/只读 rootfs/默认禁网 | 不可信代码主流 = **每运行一容器**（XSOAR/Cortex）或 microVM（Lambda/Firecracker） | 🟡 | 与 D2 威胁模型（仅管理员 + 审批 = 防失误非防恶意）匹配；**触发点**：上传者范围扩大 / 脚本来自部门外 / 处理敏感数据 → 升第二档（每运行一容器）。契约对档位透明，升档不改脚本 |
| 2d | secrets 经环境变量注入 | 主流做法，但偏弱（/proc、子进程继承、崩溃转储可泄露）；K8s/Vault 侧倾向文件挂载或运行时拉取 | 🟢 | 预留 `secrets.mode: env|file`，第二档随容器隔离一起上 |
| 3 | 编排：自研轻 DAG（Stage 2）+ WorkflowEngine Port 逃生门 | SOAR 厂商（XSOAR/Shuffle/Tines）自研 playbook 引擎是常态；**企业内部平台**主流倾向成熟引擎（Temporal / Camunda / Argo Workflows）换取持久执行、定时器、信号、在途版本管理、可视化 | 🟡 | 自研引擎的历史教训 = 补偿 / 在途工作流版本升级 / 长定时器 / 可观测会持续累积复杂度。缓解：节点类型钉死六种不扩、Port 抽象、**phase9 立项前用 phase8 迁移时拿到的 SOAR playbook 真实复杂度数据复评一次**——若多数 playbook 需要长等待 / 人工多步 / 复杂补偿，Temporal 权重上调 |
| 4 | 事件面：DomainEvent + PG outbox 表 + 轮询派发（不引 broker） | 事务性 outbox 是标准模式；大规模用 CDC（Debezium）或 broker；中小规模"表 + 轮询"（Watermill SQL、Rails/Laravel outbox）普遍 | ✅ / 🟢 | 建议事件信封字段**对齐 CloudEvents**（id / type / source / subject / time / data），零成本，便于 SOAR / 智能体 / 出站 webhook 消费 |
| 5 | 调度分工双轨（进程内 CronScheduler 维护型 / taskrunner 业务型） | 主流 = 单调度面或至少**单一可见面**（K8s CronJob、Airflow、Rundeck） | 🟢 | 风险 = 两处计划任务割裂、漂移。缓解：phase7 M4 补"计划任务总览"只读视图（并列两源），standards 写死分工边界（D18） |
| 6a | 统一接入层：推 / 拉、原始落盘、归一化、指纹去重、路由 / 抑制 | SIEM / SOAR 接入层标准形态；OCSF 归一化；dedup key | ✅ | — |
| 6b | db-pull（直连对方数据库拉取） | **业界不推荐**：schema 漂移、跨系统耦合、权限面扩大；主流 = API / 导出文件 / CDC | 🔴（所有者历史特例） | 仅作兜底：只读账号 + 参数化 SQL 模板审批入库 + 水位 + 上限 + 审计 + 网络白名单（D12 全套）；每个 db-pull 源登记"退出条件"（对方提供 API 即切换） |
| 7 | 源 / 平台数据一致性：字段归属 + `source_observed_at` 时间判优 + 每源稳定键 | DefectDojo reimport、ServiceNow VR redetection、Tenable/Qualys 集成的通用逻辑 | ✅ | — |
| 8 | 资产：外部平台 + 查询门面 + 实体存 asset_ref/快照 | CMDB 作真相源 + 安全平台做富化 / 缓存 / 解析时快照 | ✅ | — |
| 9 | 非人身份：service account + actor/principal/on_behalf_of 三元组 + 审计 | OAuth token exchange（RFC 8693）、SPIFFE 服务身份、SOAR 运行身份 | ✅ 意图 / 🟢 实现 | 第一档 on_behalf_of 放请求体（HMAC 覆盖 body，完整性无虞）；**差距 = 缺"谁可代谁"的委托策略**（哪些 service account 可对哪些用户 / 范围断言代理）——随 A6 落一张 delegation_policy 表，否则任何可信调用方都能代任何人 |
| 10 | 告警聚合实体复用 phase6 风险单（D5） | 主流把 **SOC 案例（incident / case）** 与 **风险 / 发现（risk / finding）** 分开建模（TheHive case vs MISP/DefectDojo finding；XSOAR incident vs 漏管平台） | 🟡 | 部门规模下的有意简化，省一套实体与 UI；代价 = 分诊 / 遏制类生命周期挤进风险单。缓解：`source_kind` 判别列 + 风险单状态机允许 alert 型子集；**触发点**：SOC 案例分诊工作量显著上升（如日均案例 > 数十）或需要 war-room 协作时拆独立 `incidents` 表 |
| 11 | SOAR：过渡期用公司 SOAR，**终态编排全部迁回平台** | 主流企业**保留** SOAR 做编排，自建平台做台账 / 工单 / 能力提供方；整体替换 SOAR 的案例少（成本 / 供应商锁定 / 数据驻留驱动） | 🔴（战略选择） | 迁回 = 重建 SOAR 的大面（设计器、连接器库、运行可视化、指标）；缓解：只迁高频 playbook、长尾留 SOAR 或弃用、S2 画布复用、Action 注册表承接连接器；**phase10 前设检查点**：playbook 数量 / 复杂度 / SOAR 续费成本三指标过阈值再决定"全迁"还是"长期双栈" |
| 12 | 不引 Kafka（asynq + PG outbox + Redis） | 内部安全平台在此规模的常态；Kafka 用于流式 / 多消费组 / 日志级流量 | ✅ | 触发条件已登记（D21） |
| 13 | AI 后置，只留 Action schema + on-behalf-of 两个预留 | MCP 正成为智能体工具面事实标准；安全 Copilot 普遍先做 NL 查询 + 摘要再做代操作 | ✅ | — |
| 14 | 处置联动：关闭结论映射 / M:N / 复核门 / 重报语义 | ServiceNow / Jira resolution code 映射、DefectDojo finding 状态机 | ✅ | — |
| 15 | 接口公约：仅 GET/POST、POST URL 不带业务信息（既有 house rule） | REST 主流用 PUT/DELETE/路径参数 | 🟢 | 外部集成方（SOAR）可能预期 REST；靠 OpenAPI 导出 + 文档化映射化解，不改公约 |
| 16 | 可观测：泛写"指标 / 队列水位 / 失败率告警" | 主流 = Prometheus 指标 + OpenTelemetry 追踪 + 结构化日志 | 🟢 | 建议 M4 明确采用 Prometheus 暴露格式 + OTel 语义约定；运行记录带 trace_id 贯穿 taskrunner → sandbox-runner → 回调 |

## 2. 真正的差距（需要你知道，并带触发点）

1. **编排自研（#3）**——与企业内部平台主流最大的技术分歧。不是错，是赌：赌安全剧本的复杂度停留在"几十步内、少量长等待"。对冲 = Port + phase9 前复评。
2. **告警聚合实体复用风险单（#10）**——有意合并 SOC 案例与风险登记两套语义；规模小时划算，规模大时要拆。对冲 = 判别列 + 拆分触发点。
3. **沙箱第一档（#2c）**——低于"每运行一容器"主流；由 D2（仅管理员 + 审批）托底。上传者范围一变就升档。
4. **委托策略缺位（#9）**——on_behalf_of 有载体没有策略表，是我前面漏写的一块；随 A6 必须补。
5. **db-pull（#6b）**——业界不推荐，仅因历史特例保留；每个源登记退出条件。

## 3. 低成本补强（建议直接采纳进设计）

> **状态（2026-10-20 晚）**：所有者"加"——九项已全部折入 03 / 04 / 05 / 07 对应段落，各处带"09 号补强"标记；检查点登记为 D23。

| 补强 | 对应 | 成本 |
|---|---|---|
| `ZH_OUTPUT_FILE` 可选结果文件通道（GitHub Actions / Tekton 式） | 05 号 §4 | +0.2 天 |
| 事件信封对齐 CloudEvents 字段（id / type / source / subject / time / data） | 03-A9、06 号 S6 分类学 | 零（命名约定） |
| `secrets.mode: env\|file` 预留 | 05 号 manifest | 零（字段预留） |
| delegation_policy（service account → 可代理的用户 / 组织范围 / 动作范围） | 03-A6 | +0.5 天随 A6 |
| 计划任务统一只读视图（并列 CronScheduler 与 taskrunner jobs） | phase7 M4 | +0.5 天 |
| Prometheus 暴露格式 + OTel 语义约定 + trace_id 贯穿 | phase7 M4 | 设计约定 |
| actions 注册表 → OpenAPI 导出 | 03-A1 / 04-§3.2 | +0.5 天 |
| db-pull 源登记"退出条件" | 04-§4.10 | 零 |
| phase10 前 SOAR 迁回检查点（playbook 数 / 复杂度 / 续费成本阈值） | 07-§4.5 | 零（拍板项） |

## 4. 一句话总评

技术面建议整体在业界航道内，偏离处多为有意的规模适配；两处 🔴 来自所有者的战略与历史约束而非技术共识，已登记对冲。最该盯住的是 #3 与 #11——它们决定 phase9/10 的体量，也是业界最容易"自研出一个半成品 SOAR"的地方。
