# 10 — P5-S10 工单协作与提醒能力（设计稿）

> **段属性**：基线段（phase5 收口对象）。**建议实施序**：S6 交付后（抄送可见性/通知、超时逾期通知依赖 S6）；phase6 硬前置四件之一。
> **量级**：≈ 1.5–2 周（抄送 2–3 天 + 标签 2 天 + 角标 1–2 天 + 节点超时 3–4 天 + 导出含 BK-21 2–3 天）+ cron 可观测面 +0.5 天 + **`status_changed_at` 列 +0.5 天（拍板点 5，2026-10-16 外审补遗）**。**迁移**：有（`ticket_cc`/tag 两表族 + `status_changed_at` ALTER；编号启动时对账）。
> **文档定位**：段级设计稿（2026-10-15 建档，phase5 分支）。由来 = 2026-10-09 所有者完整使用场景 vs 规划/代码差距核查（**五件全为零实现，代码级核实**）。

## 1. 定位与由来

所有者完整使用场景描述暴露五件协作/提醒缺口（抄送/标签/角标/节点超时/导出），2026-10-09 差距核查逐件代码级核实零实现后立项。phase6 依赖四件：风险工单按等级自动选抄送人（抄送 cc 规则）/风险事件打标签（标签）/新风险事件角标（聚合计数）/节点逾期按级别通知（节点超时）——**S10 缺席则四能力降级**（手动抄送/固定筛选/无角标/仅风险单级 deadline）。

## 2. 现状基线（代码级锚点，2026-10-15 核实）

| 项 | 现状 | 锚点 |
|----|------|------|
| L2 可见集 | 属主（created_by/assigned_to）∨ org 锚点子树 ∪ 委托轴（BK-13 三支 SQL）——**无 watcher 通道** | `internal/service/ticket/resource.go` `GetFilter`/`delegatedVisibilitySQL`（≈:140-176） |
| 抄送/标签/角标/超时/导出 | **五件零实现**（ticket_cc/tag 表、聚合计数端点、timeout_minutes、审计/工单导出端点均不存在，2026-10-15 grep 核实；zhuzhao 仓内零 excelize/CSV 导出） | — |
| keyword 搜索 | title ILIKE（转义）+ 纯数字 ID 精确匹配 | `internal/repository/ticket_repo.go` ListTickets 条件构造（≈:145-154） |
| cron 设施 | P4 收归的进程内 CronScheduler（ticker + pg advisory lock + ReentrantJob 契约）已就位，现仅 audit_archive 一任务；**无清单读端点/上次成功时间/失败告警**。**调度分工口径（2026-10-20 规划研究回灌，D18，依据 deliverables/software-company/secops-platform-vision-2026-10/08-phase5-adjustments.md）**：CronScheduler 只承接平台维护型扫描（超期 / 到期 / 对账 / GC），业务 / 拉取 / 脚本类周期工作一律 taskrunner——本段 timeout_scan 属维护型，骑 CronScheduler 口径成立；phase6 三个扫描任务同理 | `internal/service/cron_scheduler.go`；`internal/app/app.go:52-61,104` |
| SLA 死存储 | `tickets.sla_due_at` / `ticket_types.default_sla_hours` **从不写入**（只读兼容） | `internal/model/ticket.go:21,46` |
| 通知依赖 | 抄送可见性通知/超时通知的通道与模板 = S6 交付物 | [06-s6](./06-s6-message-notification.md) |

## 3. 五件设计要点

### 3.1 工单抄送（CC）

- `ticket_cc` 表 + 请求/事件扩展；**抄送人来源三档**：①手动选人；②流程节点配置（S2 节点 meta 增 cc 规则——依赖 S2 适配层扩展）；③按级别规则（phase6 风险工单按等级自动选抄送人——依赖本件的规则挂点）。
- 抄送者可见性与通知走 S6（业界对标 Jira watcher）——**抄送落库同事务 emit `ticket.cc_added`**（DomainEvent，06 号 A.1 枚举：S6 message_rules 订阅抄送通知；无此事件=通知须硬编码进 cc_service 违 06 号 §5.2 禁令；随 B10-1 落，2026-10-16 评审批）。
- **抄送 × L2 可见性拍板点（表设计前必拍）**：现 L2 可见集无 watcher 通道（§2）——phase6 头号场景"风险工单按等级抄送安全负责人"，安全负责人通常**不在业务 org 子树内**，按现 L2 **连工单都打不开 = 抄送失效**。二选一：
  - **① "抄送即可见"L2 特例通道（推荐）**：cc 用户并入 GetFilter/Authorize 可见集（Jira watcher 同语义）——注意与既有三轴的合取关系（特例=新增 OR 支，fail-closed 哨兵与 BK-21 泛化同批对表）；
  - ② 选人限定在可见范围内——**砍掉 phase6 场景，不可取**。
- **⚠ 表设计对表项（2026-10-15 核对发现）**：[phase3/10 §10.1](../phase3/10-ticket-business.md) 类型级可见性策略蓝本已设计 **`ticket_participants(ticket_id, user_id, role∈{participant,cc})`** 独立 m2m 表（翻案重启点 = 「工单恢复批」，**未立项未排期**）。本段落 `ticket_cc` 前须与其对表：若可见性批先落，cc role 已有承载、`ticket_cc` 勿重复建；若本段先行，表设计**须预留 role 语义**（单表 cc/participant 可演化 vs 分表）或显式拍分表——**两批谁先启动谁对表，防同语义双表**。

### 3.2 用户自定义标签

通用 tag 表（entity_type + entity_id + tag 文本，用户级）挂工单/风险事件——**补充而非替代**既有固定筛选维度；筛选项前端透出。**可见性语义（⚠ 2026-10-16 外审提出歧义——拍板点 6，B10-2 前拍）**：「用户级」两解 = 每人私有 vs 用户创建共享——安全协作场景下私有标签削弱价值。**建议默认共享**（标签内容非敏感、可见性随工单 L2、表带 created_by 留痕；跨用户"标签治理/改删权限"出现再升级），B10-2 前确认。

**姊妹挂账：@提及（触发驱动，2026-10-16 全计划业界对标补——Jira/GitHub/Slack 协作标配，本段未设计）**：评论/描述中 `@用户名` 解析 → 被提及者**通知**（走 S6 模板）+ **可见性**（提及是否纳入可见集 = 轻量版抄送，随拍板点 1「抄送即可见」口径对齐）；量级 1–2 天（解析+通知挂载+解析限流防滥用）。触发 = 协作场景真实出现。

### 3.3 待办角标（轻量提醒，≠站内消息中心）

- 右上角角标 = 待办工单数（assignee=me）+ 新风险事件数（phase6）；实现 = **聚合计数端点**（一次查多计数，替代现状首页每状态一次 total 的权宜）+ 前端定时轮询（60s 级）。
- **端点按 sections 可扩展结构设计**（2026-10-09 补遗）——phase6 增风险事件计数**零改协议**；S6 二期未读数聚合并入同一端点 sections（[06-s6 §6](./06-s6-message-notification.md)）。
- 站内消息流不入本段（翻案后挂 S6 二期——本段角标 = 铃铛一期形态）。

### 3.4 节点级超时 + 逾期分级通知

- **`ticket_types.transition_meta` JSONB 新列承载 `timeout_minutes`（state 级键——2026-10-19 结构修正：原「transitions meta 零 DDL」承载不存在，代码实证 transitions=邻接表 `Record<string,string[]>` 无边属性位）+ B10-4 一枚 ALTER 建列**（S3a 后续扩展 "from>to" 转换级键复用同列——一列两用，键形以 ">" 区分）+ 扫描任务**骑既有 CronScheduler**（`cron_scheduler.go`，advisory lock 单实例已就位）+ S6 模板通知（按级别/角色路由收件人）。**⚠ 时间源前置（2026-10-16 外审代码级核实——拍板点 5，B10-4 前拍）**：`tickets` **无 `status_changed_at` 列**（实核 ticket_repo 列清单）——"节点停留时长"无数据源；`updated_at` 不可靠（评论/编辑都 bump）、`sla_due_at` 是截止时间非进入时间且从不写入。**推荐 (a) 新增 `status_changed_at` 列**（`AssertTransition` 成功同事务写入——扫描 O(1) 读列）；备选 (b) ticket_events 取最近状态事件（依赖事件完整性 + JOIN 成本高，不取）。量级 +0.5 天（ALTER + 写入点）。

- **escalation 催办设计（2026-10-23 对话设计批——超时催办场景核心）**：`transition_meta` 的 state 级 `timeout_minutes` 之外，增补 `escalation` 嵌套段（零新迁移——JSONB 列 B10-4 已建）：`{"notify":["assignee","supervisor"],"channels":["email","dingtalk"],"reminder_interval_minutes":240,"max_reminders":3}`。cron 扫描：每小时 → overdue 且 status 未变 → 距上次催办 > interval？→ 发送催办（supervisor 经 B6-4 收件人解析）→ 更新 last_reminder_at；达 max_reminders 停止；工单离开该状态 = 天然停止。量级 ≈ 1 天（cron 骨架已有，增 escalation 解析 + last_reminder_at 列[tickets 加一列或 transition_meta 内嵌均可]）。
- **轻量版，刻意不做完整 SLA 引擎**（§23 暂缓域不碰）：无 ticket_sla 表/无违约升级链/无暂停恢复——"服务承诺管理"诉求出现再升级。**溯源澄清（2026-10-19 所有者质询边界来源，考古修正）**：§29 原文仅方向性定论「刻意轻量不做完整 SLA 引擎」一句（+死存储核实），**并未逐条列出此四件**——四件=规划写作时从"完整 SLA 组件清单做减法"的**设计推演**（2026-10-09 S10 立项三件+2026-10-16 业界对表补业务日历一件），非所有者逐条拍板；其真实支撑=§23 暂缓纪律（SLA 整域暂缓的真实决策）+**隐性场景假设**（内部工单无对外承诺诉求）——**边界锚点应为触发信号清单（§5 硬五条）而非"定论引用"：假设失效或信号出现即重开，边界是当前态非永久钉死**。
- **`sla_due_at`/`default_sla_hours` 死存储字段去留随本件拍**（启动批）：删除（迁移清理）vs 保留注释标记 deprecated。
- **cron 可观测面随行（2026-10-09 补遗，+0.5 天）**：本件落**首个业务扫描任务**，同批补 cron 设施只读状态端点（任务名/上次成功/耗时/结果行数）+ 失败接 S6 告警——**超期扫描静默失败 = 提醒永不发出，合规场景不可接受静默失败**（audit_archive cron 默认关且未开启是先例）；phase6 三个扫描任务（deadline 超期/接受单到期重审/终态对账）将继承此观测面。

### 3.5 工单/审计导出（P4-4 未兑现部分）

- P4-4 实际落的是 **al 域导出**（activelist `GET /data/:typeName/export`，经网关代理）——**审计/工单查询结果导出（excelize 固定列）在 zhuzhao 仓内零端点**（2026-10-15 核实），本件补齐。
- **端点形态对表注（2026-10-15 公约审计补，standards §3）**：导出属查询语义，形态随批（GET 复用筛选参数为预置方向）；**流式/大结果集属 §3-6「游标仅用于机器全量迭代链路（同步/导出）」的许可场景**——流式/keyset 不进管理台分页信封；与审计导出挂账项（[README §5](./README.md)，`GET /audit/logs/export` 蓝本=activelist 流式导出）统一对表，勿两处漂移。
- **BK-21 护栏泛化同批强制**（导出 = 触发条件；与 phase6 risk_items 接 L2 谁先启动谁触发，另一次自然兑现）——导出必须走 GetFilter 同管线，**导出不越查询可见性**。
- 关联挂账：审计日志按需导出（筛选即导出形态，2026-10-15 用户提出）登记于 [README §5](./README.md)——**触发驱动不占基线**，若与本件撞期可合并实施（蓝本同 = activelist 流式导出）。

## 4. 启动拍板点

| # | 事项 | 建议 |
|---|------|------|
| 1 | 抄送 × L2 可见性 | **"抄送即可见"特例通道**（③选人限可见范围不可取）；与 BK-21 泛化同批对表 |
| 2 | ticket_cc vs ticket_participants 表形态 | 启动时按 §3.1 对表项拍（谁先启动谁对表） |
| 3 | sla_due_at/default_sla_hours 去留 | 随节点超时件拍（倾向注释标记 deprecated，删除收益低） |
| 4 | 导出格式 | excelize 固定列（P4-4 al 域同款）；CSV 流式形态仅审计导出挂账项按需 |
| 5 | **节点超时时间源（2026-10-16 外审补遗）** | **新增 `status_changed_at` 列**（`UpdateStatusTx` 咽喉内同写——2026-10-19 两轮修正：AssertTransition=tx 前纯校验不可作锚；中间版锚 B3-1 端点有排期倒挂[晚于 B10-4 交付]，终版锚 repo 层唯一状态写咽喉[存量 Close/Assign 与 B3-1 端点全经它]——推荐 a 案）；ticket_events 反查不取（完整性依赖+JOIN 成本）；B10-4 前确认，量级 +0.5 天 |
| 6 | **tags 可见性语义（2026-10-16 外审补遗）** | **默认共享**（created_by 留痕；可见性随工单 L2）——私有标签诉求出现再加 owner 限定；B10-2 前确认 |

> **状态注（2026-10-16）**：本表拍板点已随批量预确认拍定（2026-10-16 所有者「按建议」——✅34/🔶1，状态 SSOT = [11 号 §4](./11-implementation-plan.md)；各段启动时异议可翻，翻案成本=改一行）。

## 5. 边界（不做）

- **不滑向完整 SLA 引擎**（边界已钉：无计时表/无升级链/无暂停恢复）。
- **SLA 业界对表 + 升级触发信号（2026-10-16 审计姊妹批——文档化"为什么不建引擎"）**：业界完整 SLA 六组件=目标定义（scope+goal）/计时模型（**事件式 start-stop-pause**，JSM·ServiceNow·Zendesk 同构）/多级阈值预警（75/90/100%）/违约升级链（escalation 自动转派）/达成率报表/业务日历（工作时段计量）；**时钟三模型**：A 截止时间戳（`sla_due_at` 死字段即当年模型 A 残留——暂停/日历不可支持）/B 计时器事件（业界主流）/C **状态老化检测（本件=模型 C）**——C 不是 A/B 的劣化版而是独立成熟形态（GitLab health 衰减/GitHub stale bot 同款：运营提醒语义，非服务承诺计量）。**设计理由五条**：无消费者不建引擎（最高纪律）/复杂度不对称（aging 0.5 天 vs 完整 SLA 2–4 周+SLA 定义本身即管理负担）/§23 暂缓域纪律/aging 为独立形态非劣化/`status_changed_at` 已是模型 B 事件地基（升级不推翻）。**升级触发信号（硬触发任一即立项；软信号两条以上启动评估）**：硬=①对外承诺响应/解决时限（进 SLA/合同）②停表语义（等客户期间不计时的考核场景——**aging→SLA 分水岭**）③业务日历（非 7×24 运营）④达成率 KPI 考核 ⑤超时自动升级链；软=超时通知被普遍忽略（阈值语义需细化为"承诺剩余时间"）/phase6 整改 deadline 升格监管承诺/平台对外服务化。**升级形态预置**：`status_changed_at` 保留 → 加 `sla_timer_events`（ticket_id+event[start/pause/resume]+at）+ goal 定义表（JSM policy 同构）+ `timeout_minutes` 保留为 aging 提醒层与 SLA 计量层**并存**（两档语义不冲突）。**引擎可嵌入性结论（2026-10-16 补——"有没有现成可直接用"）**：独立可嵌入的开源 SLA 引擎**基本不存在**（生态真空：SLA=业务规则×状态机×时钟语义的耦合体，只沉淀在平台内不独立成库）——开源仅两形态均不可嵌入：①整套 ITSM 平台（JSM/ServiceNow/Zammad——SLA 为内置模块，引入=换整个工单系统）②通用工作流定时器原语（Camunda Boundary Timer/Temporal Timer——能表达但需把工单流程搬进其运行时，架构代价远超 SLA 本身）；Go 生态零 SLA 语义库。**立项时=自研计量层 1–2 周**（难点在语义设计——start/stop/pause 必贴自身状态机，引库省不掉；实现为纯 SQL/Go 事件回放+阈值比较，`status_changed_at` 地基已在；设计借 JSM policy 模型，不引代码）。**四层落地路径**：层 0 aging（已有）→ 层 0.5 **deadline 模型 A**（工单 due_at 字段+按 deadline 扫描+到期通知——phase6 R2 deadline 蓝本现成，几小时级，可先顶"固定时限承诺"且让 `sla_due_at` 复活；**✅ 2026-10-19 gap2 确认：W15 随 S3a 同窗顺带排期**）→ 层 1 模型 B 完整 SLA（硬触发命中，1–2 周自研）→ 层 2 平台级 ITSM 永不（自研平台不换）。
- 站内消息流不入本段（S6 二期）；实时推送不做（轮询分期口径，S6 §6）。
- 标签不做权限级可见性（**默认共享 + created_by 留痕**——拍板点 6 口径；跨用户标签治理随场景另议）。
- 迁移两表族编号启动对账（A2）。

## 6. 影响面

- **迁移**：`ticket_cc` + tags 两表族 + `status_changed_at` ALTER + **`ticket_types.transition_meta` JSONB 一枚 ALTER**（timeout_minutes 承载——结构修正后口径；编号对账）。
- **权限**：导出走 GetFilter 同管线（BK-21 泛化同批）；抄送特例通道若立 = L2 可见集新增 OR 支（**策略库 Registry 同批登记**，防漏调）。
- **消费方**：S6（抄送/超时通知）、phase6（四件依赖 + 风险关单对账扫描继承 cron 观测面）、S2（节点 meta cc 规则）。
- **前端**：详情页抄送人区/标签编辑、全局角标（布局层）、工单/审计列表导出按钮。

## 7. 实施序与批次建议

S6 后实施。批次：①抄送（含 L2 特例通道 + 表形态拍板）→ ②标签 → ③角标聚合计数端点 → ④节点超时 + cron 可观测面 → ⑤导出 + BK-21 泛化。②③与①无硬依赖可并行；⑤依赖 BK-21 排期对齐（与 phase6 谁先启动谁触发）。

## 8. 验收口径

- 后端随四档 acceptance；前端走 ui CI + E2E + 场景走查。
- 场景标尺：抄送安全负责人（org 子树外）→ 其列表可见该单 + 收到通知（拍板点 1 成立的正向）；**非抄送人不可见**（特例通道收口的负向必测）；角标端点一次请求返回多 sections；节点超时扫描触发分级通知 + cron 状态端点可见上次成功；导出行数与同筛选列表一致（可见性同源断言）+ 无权限用户导出 403。
- 回归：现 L2 三轴判定零变化（特例支纯增量）；sla_due_at 去留处置后存量单零影响。

## 9. 文档衔接

| 文档 | 关系 |
|------|------|
| [phase3/10 §10](../phase3/10-ticket-business.md) | ticket_participants 蓝本对表项（§3.1 ⚠）；类型级可见性批 = 潜在前置 |
| [06-s6](./06-s6-message-notification.md) | 通知通道/模板/未读数 sections；实施序前置 |
| [02-s2](./02-s2-flow-graph-designer.md) | 画布范围界注互指（meta 编辑不在 S2——transition_meta 编辑归本段 B10-4 管理面/S3a B3-1） |
| [phase6/README §3](../phase6/README.md) | 四件依赖与降级说明；cron 观测面继承方 |
| [review/11 §8 BK-21 行](../review/11-project-control.md) | 护栏泛化触发实锤（导出/抄送特例支同批） |

---

## 附录 A：详细技术设计（实施级，2026-10-16 补充——样板深度对齐 05 号附录 A）

> ⚠ 标注 = 实施级设计建议（启动批确认制，不新增拍板点）。**拍板点 1–6 已全部批量预确认**（2026-10-16，11 号 §4 台账 S10#1–6 ✅）。

### A.1 模块架构与代码落点

```
internal/service/ticket/
  cc_service.go                    # 抄送（B10-1）：加/删/列表 + L2 特例支生效点
  tag_service.go                   # 标签（B10-2）：挂/摘/按标签筛选
  badge_service.go                 # 角标聚合（B10-3）：sections 计数——**计算器注册制**：各域向
                                   #   badge 注册 section 计算函数（ticket 域注册 todo；phase6
                                   #   risk 域注册 risk），handler 只做编排——防 phase6 往
                                   #   ticket 域塞 risk 计数（域边界，对齐 DomainEvent 注册哲学）——定位注（2026-10-16 外审）：badge 实为跨域聚合枢纽却宿主 ticket 域，
                                   #   可接受（import 方向 risk→ticket 消费方引宿主）；phase6 注册时再议升位
  timeout_scanner.go               # 节点超时扫描（B10-4）：骑 CronScheduler，发 DomainEvent
                                   #   ⚠ 落点迁移注（2026-10-19）：随 03 号附录 A.6 engine 子包化，
                                   #   B10-4 实施时收拢至 internal/service/ticket/engine/
internal/repository/
  ticket_cc_repo.go / tag_repo.go  # 两新表族 CRUD
  ticket_repo.go                   # 扩展：status_changed_at 写入点与超时候选查询
internal/handler/
  ticket_handler.go                # cc/tag 端点扩展；export 端点
  badge_handler.go                 # 角标聚合端点
  system_handler.go                # cron 状态只读端点（B10-4 随行）
internal/service/cron_scheduler.go # 扩展：任务状态登记（last_success/duration/rows）——观测面
zhuzhao-ui/
  views/ticket/detail/             # 抄送人区 + 标签编辑区
  components/NavbarBadge.vue       # 全局角标（60s 轮询 sections）
  views/ticket/list + views/audit  # 导出按钮（当前筛选透传）
  src/api/collab.ts                # cc/tag/badge/export API 封装
```

### A.2 L2 特例通道实现（拍板点 1 已批量预确认：新增 OR 支 + fail-closed 哨兵同批）

- **语义分层（关键设计约束）**：cc 支只进**读可见集**，不进操作授权集——
  - `GetFilter`（列表/导出的行级过滤）**新增第四 OR 支**：`EXISTS (SELECT 1 FROM ticket_cc WHERE ticket_id = tickets.id AND user_id = :me)`——特例支进 GetFilter 本体 = **所有列表/导出/关联查询路径自动生效**（抄送可见性无逐调用点接线问题，与 BK-21「显式调用零护栏」焦虑正交：GetFilter 本体增强非新调用点）；
  - `resource.Authorize`（操作授权：update/close/assign）**不加 cc 支**——抄送人能看不能改（Jira watcher 同语义：watch ≠ edit）；detail 读取判定走读可见集同源函数；
  - **策略库 Registry 同批登记**（§6 影响面既定）：可见集变更登记防后续新查询路径漏用 GetFilter。
- **哨兵同批（fail-closed）**：特例支上线同批加**负向断言**入 acceptance——非 cc 且三轴外用户对该单 403/不可见（§8 已列）；BK-21 泛化（导出同管线）与特例支同一提交，防「加了 OR 支忘收口导出」。

### A.3 表族 DDL（迁移全文级，编号启动对账）

```sql
-- ① ticket_cc（拍板点 2 预确认：S10 先行 → role 语义预留，防 ticket_participants 双表）
CREATE TABLE ticket_cc (
    id         BIGSERIAL PRIMARY KEY,
    ticket_id  BIGINT NOT NULL REFERENCES tickets(id) ON DELETE CASCADE,
    user_id    BIGINT NOT NULL REFERENCES users(id),
    role       VARCHAR(20) NOT NULL DEFAULT 'cc',   -- 预留：cc / participant（对表项 §3.1）
    created_by BIGINT NOT NULL REFERENCES users(id), -- 谁加的抄送（操作留痕）
    created_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE (ticket_id, user_id, role)
);
CREATE INDEX idx_ticket_cc_user ON ticket_cc (user_id);   -- 「抄送给我的」反查（GetFilter OR 支走此索引）

-- ② tags（拍板点 6 预确认：默认共享 + created_by 留痕）
CREATE TABLE tags (
    id          BIGSERIAL PRIMARY KEY,
    entity_type VARCHAR(30) NOT NULL,    -- ticket（risk 为 phase6 预留值）
    entity_id   BIGINT NOT NULL,
    tag         VARCHAR(50) NOT NULL,
    created_by  BIGINT NOT NULL REFERENCES users(id),
    created_at  TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE (entity_type, entity_id, tag) -- 同实体同标签唯一（幂等挂载）
);
CREATE INDEX idx_tags_entity ON tags (entity_type, entity_id);
CREATE INDEX idx_tags_tag    ON tags (tag);            -- 标签筛选与输入联想

-- ③ ticket_types.transition_meta（2026-10-19 结构修正新增：timeout_minutes 承载——
--    原「走 transitions meta 零 DDL」不成立：transitions=邻接表 Record<string,string[]> 无边属性位）
ALTER TABLE ticket_types ADD COLUMN IF NOT EXISTS transition_meta JSONB DEFAULT '{}';
-- state 级键先入（B10-4）：{"pending_verify": {"timeout_minutes": 240}}
-- S3a B3-1 扩展 "from>to" 转换级键复用同列（condition/validator/post_actions）——一键两形以 ">" 区分

-- ④ status_changed_at（拍板点 5 预确认：UpdateStatusTx 咽喉内同写——2026-10-19 终版锚，修正史见 §3.4 写入点注）
ALTER TABLE tickets ADD COLUMN IF NOT EXISTS status_changed_at TIMESTAMPTZ;
-- 存量一次性回填（随迁移）：UPDATE tickets SET status_changed_at = updated_at WHERE status_changed_at IS NULL;
--   ⚠ 回填值 = 近似（updated_at 被评论/编辑污染）——存量单首轮扫描可能误报超时，
--   缓解：迁移后首跑扫描设观察模式（只记日志不发通知）或回填后重置活跃单为 NOW()（启动批二选一拍）。
CREATE INDEX idx_tickets_timeout ON tickets (status, status_changed_at)
    WHERE status NOT IN ('closed', 'rejected');       -- 部分索引：扫描只摸活跃单
```

- 写入点：**`UpdateStatusTx`（repo 层唯一状态写咽喉）内 `SET status_changed_at = NOW()`**（与状态写同语句同事务，零漂移窗口；CreateTicket 初始化同写）。⚠ 2026-10-19 两轮修正：①原「`AssertTransition` 成功后同事务」表述不准——AssertTransition=tx.Begin 前纯校验（service.go:352 vs :357 实证）；②中间版「锚 B3-1 执行端点事务内」有**排期倒挂**——B10-4（W9–11）先于 B3-1（W13–15），W11–W13 窗口存量 Close/Assign（service.go:363/:432）不经新端点、该列持续 stale=扫描器产生新漂移；③**终版锚 `UpdateStatusTx`**：存量路径与 B3-1 端点（收敛封装后）全部经此咽喉，**B10-4 交付时点即全量正确写入，无需等 B3-1**（03 号 A.3 同步）。

### A.4 角标聚合端点（B10-3）

```
GET /api/v1/badges                      # 登录态；响应 sections 为开放 map（扩展零改协议）
{ "code":0, "data": { "sections": {
    "todo": 12,                          # assignee=me AND status NOT IN 终态（不走 GetFilter 全量——assignee 查询天然可见）
    "unread": —                          # 二期（S6 B6-7 并入，06 号 §6）
} } }
```

- 前端 `NavbarBadge` 60s 轮询 + 页面切换即时刷新；未知 section 前端忽略（向前兼容 phase6 `risk` section）；
- 非数字/缺失 section 容错；单请求多计数（替代现状首页每状态一次 total）。

### A.5 节点超时扫描器（B10-4）

- **注册**：CronScheduler 增 `timeout_scan` 任务（周期 config，默认 5m；advisory lock 单实例既有）；
- **扫描逻辑**：逐活跃 ticket_type → 解析 `transition_meta` JSONB（state 级键→`timeout_minutes`；**编辑面=B10-4 管理页扩展或 JSON 专家模式——S2 画布不承载**[结构修正]）→ 候选查询（部分索引 `idx_tickets_timeout`：`status = ? AND status_changed_at < NOW() - interval`）→ 命中则 **Dispatch `DomainEvent{BizType:"ticket", Action:"ticket.timeout"}`**；
- **通知复用 S6 规则面**（关键复用设计）：扫描器只发事件、不发通知——S6 `message_rules` 配置 `ticket.timeout` 事件的模板/收件人（按级别路由 = recipients spec，§5.1 第一档）——超时通知与状态通知同一条渲染→发送→message_log 链，零新通知代码；
- **查重**：S6 消费者侧节流窗口既定（60s）不适用于扫描周期场景——扫描器侧同单同状态只发一次：`message_log` 查重（biz_id + Action + 当前状态存在 sent 记录则跳过）；状态流转后 `status_changed_at` 刷新，自然重置；
- **cron 可观测面随行**：CronScheduler 增任务状态登记（`Status() map[string]{LastSuccess, LastDuration, LastRows}`）+ `GET /api/v1/system/cron-status`（admin 只读）+ 扫描失败 Dispatch `DomainEvent{BizType:"ops", Action:"ops.cron_failed"}` → S6 规则告警（audit_archive 先例的静默失败教训收口）。**BizType=`ops`（运维域，非 ticket——域边界显式化，2026-10-16 审计补：按 Action 命名约定 `{biz_type}.{动作}` 统一，避免隐式落 message_rules 表 DEFAULT 'ticket'）**。

### A.6 导出端点（B10-5）

```
GET /api/v1/tickets/export              # 与 List 同筛选参数（GET 复用，§3.5 对表注）
GET /api/v1/audit/logs/export           # 审计导出挂账项（README §5，撞期合并实施）
```

- **流式**：excelize `StreamWriter`——header 先写、逐行 flush（防大结果集内存膨胀）；行数钳制（⚠ 对齐审计导出挂账口径 ≈5 万，超出 400 提示收窄筛选）；挂既有 rate_limit 桶；
- **BK-21 铁律落地**：导出查询**调用与 List 同一个 filter 构造函数**（代码级同源——cc 特例支/三轴自动一致），验收断言「导出行数 = 同筛选列表 total」；
- 响应：`Content-Disposition: attachment; filename=*.xlsx`（RFC 5987）；审计落行（谁/筛选/行数——对齐审计导出挂账的「导出自审计」业界标配）。

### A.7 验收补充（在 §8 基础上）

- cc 特例四向：org 外 cc 用户列表可见 + detail 可开；**非 cc 且三轴外不可见（403）**；cc 用户 update/close **403**（Authorize 不加支断言）；删 cc 后可见性即时回收；
- 超时链路：造候选 → 扫描 → `ticket.timeout` 事件 → 规则通知；同单同状态二次扫描**不重发**；状态流转后重新计时；
- cron 观测：状态端点可见 timeout_scan 上次成功/耗时；注入失败 → `ops.cron_failed` 事件（BizType=ops）→ 告警规则触发；
- 导出：同筛选行数一致断言；无权限 403；超行数钳制 400；xlsx 打开可读（列头/编码）；
- tags：跨用户共享可见（B 见 A 挂的标签）；重复挂载幂等；按标签筛选与列表联动。

---

## 变更记录

| 日期 | 变更 |
|---|---|
| 2026-10-15 | 建档（phase5 分支十段设计稿批）——现状锚点逐段代码核实（五件零实现负向核查）；含 participants 对表项（2026-10-15 核对发现）；未新增决策 |
| 2026-10-15 | 公约对表批：§3.5 导出端点形态对表注——查询语义 GET 为预置方向、流式/keyset 属 §3-6 机器迭代许可场景不进管理台信封，与审计导出挂账统一对表 |
| 2026-10-16 | **第一批外审回声批（两设计缺口落档）**：§3.4 补**时间源前置**（tickets 无 `status_changed_at` 列——代码级核实；推荐新列 AssertTransition 同事务写，拍板点 +5，量级 +0.5 天）；§3.2/§5 补 **tags 可见性语义拍板点 +6**（「用户级」歧义——建议默认共享+created_by 留痕，B10-2 前拍）；头部量级/迁移行同步 |
| 2026-10-16 | **详细技术设计批（附录 A，实施级——05 号样板深度）**：A.1 模块架构（cc/tag/badge/timeout_scanner 四服务落点）/A.2 **L2 特例通道语义分层**（cc 支进 GetFilter 读可见集=全路径自动生效、**不进 Authorize 操作授权**——抄送人能看不能改；Registry 登记+BK-21 同批哨兵）/A.3 三表族 DDL 全文（ticket_cc 带 role 预留防 participants 双表/tags 共享+UNIQUE 幂等/status_changed_at+部分索引+**存量回填近似性警示**——首跑观察模式或重置二选一）/A.4 角标 sections 开放 map 契约/A.5 超时扫描器（**复用 S6 规则面**：扫描器只发 ticket.timeout DomainEvent、S6 message_rules 路由通知零新通知代码；message_log 查重防重发）+ cron 可观测面（Status()+只读端点+cron.failed 事件告警）/A.6 导出（流式 StreamWriter+行数钳制+**与 List 同 filter 函数同源断言**+导出自审计）/A.7 验收补充（cc 四向/超时链路/cron 观测/导出四向/tags 三向负向） |
| 2026-10-16 | **全计划业界对标批（所有者）**：§3.2 补姊妹挂账 **@提及**（触发驱动——Jira/GitHub/Slack 协作标配：解析→通知+可见性[随拍板点 1 口径]，量级 1–2 天） |
| 2026-10-16 | **badge 注册制批（实施线详设+A.1 补注）**：badge_service 计算器注册制（各域注册 section 计算函数，handler 只编排——防 phase6 往 ticket 域塞 risk 计数）+ 定位注（跨域枢纽宿主 ticket 域可接受，phase6 注册时再议升位）——外审评审确认设计方向 ✅ |
| 2026-10-16 | **评审批 cc_added 事件补（深审项 A）**：§3.1 抄送落库同事务 emit `ticket.cc_added`（DomainEvent——S6 message_rules 订阅抄送通知的订阅依据；无此事件=通知须硬编码进 cc_service 违 06 号 §5.2 禁令）；06 号 A.1 枚举行同步 |
| 2026-10-16 | **SLA 业界对表与升级触发批（所有者问"为什么这样设计/后续对齐业界的触发事件"）**：§5 边界补 SLA 业界对表注——六组件/时钟三模型（A 截止时间戳=B 纪元 B 事件式/C 状态老化=本件）/设计理由五条/**升级触发信号清单**（硬五条：对外承诺·停表语义[分水岭]·业务日历·达成率考核·自动升级链；软三条）+升级形态预置（sla_timer_events+goal 表+aging/SLA 两层并存——status_changed_at 即模型 B 地基） |
| 2026-10-19 | **SLA 边界溯源修正批（所有者质询"四件边界从哪里来"——考古发现标注瑕疵）**：dd §29 原文（行 1447 实证）仅方向性定论「刻意轻量不做完整 SLA 引擎」一句，**未列四件细分**；git log -S 考古：三连（无升级链/无暂停恢复）首现于 9e7b856 S10 立项批=规划写作时从完整组件清单减法的**设计推演**、业务日历第四件首现于 d79af12 业界对表批——原「dd §29 定论，边界钉死」表述将推演产物冠以定论引用（标注纪律瑕疵，BK-16 同族）；§3.4 修正为三层溯源（方向定论=§29/四件=设计推演/真实锚点=§23 纪律+隐性场景假设[内部无承诺诉求]+触发信号清单硬五条）——**边界=当前态非永久钉死，假设失效或信号出现即重开** |
| 2026-10-19 | **timeout_minutes 承载结构修正批（外审全面审计 P1——三方矛盾：代码 transitions=纯邻接表无边属性位/02 号画布签名 Record<string,string[]>+后端零改动/S3a transitions_meta 列 W13–15 晚于 B10-4 W9–11）**：修法 (a) 拍定——`ticket_types.transition_meta` JSONB 一枚 ALTER **随 B10-4 建**（state 级键存 timeout_minutes），S3a 扩展 "from>to" 键复用同列（一列两用、">" 区分键形）——§3.4 主句/§6 迁移行/§9 衔接行/A.3 DDL 补第③枚/A.5 扫描逻辑句五处修正；「零 DDL」口径废止；**timeout 编辑面=B10-4 管理页或 JSON 专家模式（S2 画布不承载——与 02 号界注对齐）** |
| 2026-10-19 | **gap2 排期确认级联（随 03 号——所有者确认）**：§5 四层路径层 0.5 处补「✅ W15 随 S3a 同窗顺带排期」（deadline 模型 A——phase6 前核心引擎目标态收口件；transition_meta 列本段 B10-4 交付、S3a 复用扩展） |
| 2026-10-19 | **P0-2 写入点口径修正批（并行外审合并报告）**：拍板点 5/A.3 DDL 注/§3.4 写入点三处——「AssertTransition 同事务写」→「**统一转换执行入口事务内同写**」（AssertTransition=tx 前纯校验 service.go:352 vs :357 实证——写点锚定 B3-1 `POST /tickets/transition` 事务内） |
| 2026-10-19 | **P0 修正批二轮回声（写点终版锚——消排期倒挂）**：拍板点 5/A.3 DDL 注/§3.4 三处「统一转换执行入口事务内同写」→「**`UpdateStatusTx` 咽喉内同写**」——上批中间版锚 B3-1 端点（W13–15）晚于 B10-4 扫描器（W9–11）：W11–W13 窗口存量 Close/Assign（service.go:363/:432）不经新端点、该列持续 stale=扫描器新漂移；终版锚 repo 层唯一状态写咽喉，存量路径与 B3-1 封装全经它，**B10-4 交付时点即全量正确** |
