# Phase 5 挂账统一台账（2026-10-23 终版——全仓口径）

> 本文件为 phase5 **全部段**未清项的唯一权威台账（外审 F-2-10 修复）。
> 整理规则：按「消化时机」分五组（A 即时可清 / B 已锁批次 / C 触发驱动 /
> D 外部依赖 / E 待拍板）——组内按段排。已修项不列；N/A 项列并标因。
> 代码实态逐项 grep 交叉核验（滚动基线——当前 zhuzhao `788f027` / ui `7839aea2`，批次记录见文末尾注；2026-10-27 消歧：头尾两套基线并存系收尾期滞后，本行为准）。
>
> **消化容器拍板（2026-10-26 所有者）**：**不立 phase5 二期**——B 组开项默认消化容器 = **phase6 启动批随行**（走查/合 main 批可顺手的仍随最近批带走；B-1/2/3/16 的「B6-7 二期」锚已随 B6-7 交付失效，启动批范围圈定时统一重锚）。

## A. 即时可清（零设计，随最近一批带走）

| # | 段 | 项 | 出处 | 量级 |
|---|---|---|---|---|
| A-1 | S6 | F-1-9b Update 期 placeholders 校验旁路（Create 有 Update 无） | 外审 F-1-9 半截 | ✅ **已修**（本批） |
| A-2 | S1 | 92303 errcode 预留注记 | 01 号 A.6 原登记 | ✅ **已修**（本批） |
| A-3 | S6 | F-D2 rules version 乐观锁 | 000060 ✅ 已入库 **33b42a7**（v3 审计 R-1 勘误——原标 b101cc6 系错标：000060/92506/CAS 实际全在 33b42a7，b101cc6 仅含 F-1-9b/92303/台账） | ✅ **已修** |
| A-4 | — | 外审批 3 对表文档批（06/10/11/05 四稿） | b75917d/baccad6/d14678e | ✅ **已修** |
| A-5 | S6 | 事件总线优雅退出排空——`HookRegistry` goroutine fire-and-forget 无 WaitGroup，**计划内重启也丢在途通知/inbox 行**，与 11 号 §5.10「仅崩溃丢」口径不符 | 2026-10-26 事件面走查 | ✅ **已修**（`Drain(ctx)` + app.Shutdown 步 3 预算——初版 5s，G-30 终修升 8s[发送 5s+落库不挤占]；四单测含 panic 订阅者不悬挂锚 + 变异验证去 Add/Done 两测红） |
| A-6 | S3a/ui | **transition_meta 丢配置 footgun（P1，2026-10-26 核出）**：ui 类型弹窗 `loadMetaFromRow` 把转换级键（"from>to"——condition/validator/post_actions）当 state 行加载，保存走「引用已删状态」分支**静默丢弃**——API 配好的转换级 meta 被一次 UI 保存清掉；修复 = load/serialize 双侧保留含 `>` 键形原样透传 | 03 号 2026-10-26 登记 | +0.5 天（建议随 phase5 走查/合 main 批先行，不等 phase6） ✅ **已修**（ui d45f3506 load/serialize 双侧保留 `>` 键形——2026-10-26 所有者落；**销账登记笔=zhuzhao `24920b3`**[2026-10-27 消歧注：phase6/05 §1.3 所引 24920b3 即本表销账笔非修复笔，两哈希各有所指非两说]） |

## B. 已锁定消化批次（挂而不悬——锚点已写进 11 号/段稿）

| # | 段 | 项 | 消化点 | 锚 | 量级 |
|---|---|---|---|---|---|
| B-1 | S6 | D20 changes[] 真值 diff（旧值比对+custom_data 键级粒度） | B6-7 二期 | 06 号 A.1 | +0.5 天 |
| B-2 | S6 | variables 端点补动态字段源+时间类——**并账 H-3**（2026-10-27 升格为通知能力增强批核心件=G-2 加重证据） | 通知能力增强批 | 06 号 A.6 | +0.5 天 |
| B-3 | S6 | `'*'` 免打扰跨域生效 | B6-7 二期 | 外审 F-1-12 | +0.5h |
| B-4 | S6 | acceptance 通知深面（注入/节流/fail-fast/偏好四类） | B6-7 前补课 | 06 号 §10 | +0.5 天 |
| B-5 | S6 | opt-out service 直连 db 收敛到 repo | 架构批 | 外审 F-1-12 | +0.5h |
| ~~B-6~~ | S10 | ~~MessageDispatcher 编排器抽出~~ **✅ 已落（B10-4 62c03c2——dispatcher.go 抽出+hook 薄壳+ops 双域订阅）** | — | 06 号 §1.5 | 销账 |
| ~~B-7~~ | S10 | ~~escalation 催办 + ticket_reminders 侧表 + supervisor 第四分支~~ **✅ 已落（B10-4 62c03c2——实施偏差：supervisor 不做 recipient_type 第四枚举，目标集由扫描器 emit 时解析进 cc_users 载荷，规则面零扩）** | — | 10 号 §3.4 | 销账 |
| B-8 | S3a | `run.succeeded` emit + `result.*` 命名空间 **+ 任务结果→自动转换（2026-10-27 三方二次对比补强①，所有者批准）**：事件订阅形态 TransitionAction（03 号 P2-4 双形态②）消费 `run.succeeded`/`run.failed` 执行配置转换（成功→X、失败→Y），或 B0-4 词表加 `last_run.status`——否则「工单触发任务」闭环只到事件不到状态（eflow PassProcessTaskJob 对照）；详 [phase6/06 号 §3.16](../phase6/06-ticket-completeness.md) | phase6 启动批（2026-10-26 重锚——B3-3 交付偏差③未含，原「B3-3 随批」锚失效） | 03 号 §3.3 | 随批（+0.5–1 天） |
| B-9 | S5 | 内嵌图引用集解析（回收最后一段——RK-P5-16） | B6-7 二期或 phase6 | RK-P5-16 | +1 天 |
| B-10 | S6 | message_log GIN 索引 + fetcher 时机 | 触发（工单量级/首源） | S1 审计 | 随批 |
| B-11 | S5 | BindAvatar 非事务（F-A3——"无害"论证因 GC 第三段已成立） | S6 二期或随手 | 五批审计 F-A3 | +0.5h |
| B-12 | S1 | ~~P2-8 游离 godoc（ticket_handler 4 处）~~ **✅ 已修（bf4a388 删 FieldImpact 前重复块——现存 ticket_handler.go:432 为合法附着非游离——v3 审计 L-2 补标）** | 随手批 | 全量核查 | 5min |
| B-13 | S1 | P1-21 `'deleted'` 字面量在 repo + Subscribe 无 name | 随手批 | 外审 F-2-7 | ~0.5h |
| B-14 | S6 | ~~渠道命名两套冲突~~ **✅ 已修（33b42a7 批 3 对表——06 号 §3:39 两级渠道词表成文：channel=adapter 层 uip/webhook + delivery=偏好层——v3 审计 R-2 补标）** | 文档批 | 06 号 §3 | 5min |
| B-15 | S3a | ~~on_complete 直调 send() vs 通知收敛走 S6 规则面——二选一成文~~ **✅ 已拍死（33b42a7 批 3 对表——03 号 §3:48 事件优先成文，原直调表述作废——v3 审计 R-2 补标）** | S3a 开工前 | 03 号 §3.3 | 拍板 |
| B-16 | S6 | A.5③ 守卫拦截/告警（被引用字段改 key/删时规则失效——与 S1 影响分析同件复用） | B6-7 二期 | 06 号 A.5 | +0.5 天 |
| B-17 | S6 | ~~11 号计划面回填~~ **✅ 已修（33b42a7 批 3 对表——11 号 §4/§2.4/§3.3/§5.9 七关键词全回填（escalation/supervisor/ticket_reminders/MessageDispatcher/run.succeeded/manager_id/on_complete）——v3 审计 R-2 补标）** | 文档批 | 外审 F-3-6 | 15min |
| B-18 | S10 | last_reminder_at 落列决策 + recipients_spec JSONB vs 两列口径 | B10-4 开工前 | 外审 F-3-7/8 | 拍板 |
| B-19 | — | B13 权限覆盖矩阵审计（全端点×三层） | phase5 收官前 | ✅ **[14 号](../review/14-b13-permission-matrix-2026-10-26.md)**（2026-10-26 point-in-time：123 绑定+44 豁免全量对账（首版 45 系注释示例误计，验证批勘误）——通过，无新增待拍板） | 销账 |
| B-20 | S3a | spawn_task `from: ticket.field` 工单字段→入参映射（`actions_builtin.go:161` 注释已承诺、实现缺——现仅静态 params） | phase6 启动批 | 03 号 2026-10-26 登记 | +0.5–1 天 |
| B-21 | S2/S3a | 转换级 meta（condition/validator/post_actions）前端配置面（=边属性面板，ecmdb edgeProperty 同款交互）——**范围随 S2 走查结论门控**：基础档=弹窗/页内配置面（+1–2 天）；走查判画布局促则扩容为**设计器整合批**（02 号 2026-10-26 登记四件：类型列表→独立设计器路由页[ecmdb 三栏布局：NodePanel/画布/PropertySetting] + 终态可配化[state 级 meta 加 terminal 布尔——修前端写死 TERMINAL_STATES 名单缺口] + 判断边标签/可选菱形装饰 + 本配置面）≈+3–4 天 | phase6 启动批 | 03 号 2026-10-26 登记 / 02 号 2026-10-26 登记 | +1–2 天（整合批 3–4） |

## C. 触发驱动（有明确触发信号，信号不到不动）

| # | 段 | 项 | 触发信号 | 出处 |
|---|---|---|---|---|
| C-1 | S1 | phase6 R3 前置批（DataFetcher 超时/限流/审计，~3–4 天） | 接首个外部源前**必须**（否则 P1 升 P0） | 01 号 §3.3③⁺ |
| C-2 | S1 | auto_fill 实现批（prefill 端点/params 泛化/external_http/前端交互） | phase6 R3 首源就位 | 01 号 §3.3③⁺ |
| C-3 | — | BK-21 GetFilter 护栏泛化 | 首个新资源接 L2 / 导出功能 | 工程约束 |
| C-4 | — | BK-19 handler 层测试 | 工单封版翻案时 | phase2 遗留 |
| C-5 | S6 | RT-1 观测面（20015 审计/WARN）。**2026-10-27 拍板（独立于 A-2 四条——本条系降噪前提 gated 型非量级水位型）**：**观测先行**——20015 命中即落审计行+WARN（记录≠告警：多标签/迟到提交合法重放不构成风暴，降噪前提只 gate 告警面不 gate 记录面；三周后盗用追查「一查便知」依赖此行）；降噪（宽限窗口/family_id）维持原触发不动 | 先有降噪前提（宽限窗口/family_id）——**记录面已解除该前提** | 06 号 §5.2；落地=2026-10-27 观测批 |
| C-6 | S5 | RK-P5-15 richtext 图数上限 + 触发信号观测面。**✅ 已拍 a 案（2026-10-27 所有者——A-2 四条之一）**：落地形态=①`file_objects` 按 purpose 的 **SQL 采样 gauge**（`SUM(size_bytes) GROUP BY purpose`——000045 建表即有 size_bytes/purpose 列，不碰存储层不做对象枚举）②阈值检查骑既有 CronScheduler（同 intel_cleanup 形态）③越限 WARN + emit `ops.storage_usage_high`（80% warn/90% crit，容量基线 config 未配=只暴露绝对值不判阈）④事件走 000064 惯例种子（fixed+空收件人=管理员配置后生效，同 ops.cron_failed）。注：RustFS 原生 `/minio/v2/metrics` 兼容性**未实测**——实测前不登记为零开发依赖（运维侧登记行见 runbook §3）；图数上限本体（RK-P5-15）维持触发=水位信号亮起后启动 | 磁盘水位/贴图密集——**观测面已建（2026-10-27 观测批——本批落码：SQL 采样 gauge+storage_usage_sample cron+000080 种子），信号可判**；上限收紧=水位亮起后 | 05 号 §3；落地=2026-10-27 观测批 |
| C-7 | S5 | RK-P5-14 附件独立权限码 | 工单域权限细化诉求 | 05 号 A.7 |
| C-8 | — | BK-12 残留（org 删除守卫不查 org_roles） | 出现孤儿绑定行实际危害时 | AGENTS 遗留 |
| C-9 | S1 | spec JSONB 收件人模型 / form-create runtime 渲染引擎 | ≥2 来源组合 / 渲染联动变重时 | 06 号 A.5 注 |
| C-10 | S1 | manager_id 显式汇报线 | M-HR 启动 | HR 同步待设计 |
| C-11 | S5 | aws-sdk-go 备选。**✅ 已拍 b 案（2026-10-27）改判人判断型**：信号必然以「附件链路报错/升级验收失败」形式显形，观测者=执行存储栈升级的人；配套升级纪律已落 runbook §3（动存储栈必跑附件全链验收）——要的是纪律不是仪表（兼容度 gauge 造不出也不省钱） | rclone/RustFS 兼容劣化（人判断型） | RK-P5-13 |
| C-12 | S6 | ~~stub 独立 config 段（`notification.uip.*`）~~ **✅ 已落（微批 A——Notification.UIP.Endpoint 四链+BindEnv+生产态 fail-fast；契约接入面归 D-1——v3 审计 L-2 补标）** | 接 UIP 真实契约前 | 外审 F-1-14 |
| C-13 | S6 | markdown 渠道转义（`channelEscapes` 加第三族）——**并账 H-2**（2026-10-27 所有者需求明确提前：dingtalk 独立渠道，UIP 契约门控仅约束 UIP 面） | 随通知能力增强批 | 外审 F-2-9 |
| C-14 | S5 | cookie Secure release 强制 | 生产 HTTPS 网关前置 | 外审 F-4-7 |
| C-15 | S6 | 死信/emitEvent ctx 超时+request_id | 接入链路追踪时 | 外审 F-2-8 |
| C-16 | S6 | 节流入口廉价预判。**✅ 已拍 a 案（2026-10-27）**：批量导入场景投递全成功零死信——死信告警抓不住「成功的骚扰」，受害者举报非可靠信号。落地=dispatcher 发送计数进 /metrics（ok/dead 双标）+ 小时级量级 cron 超阈值 WARN（**日志面兜底不走 S6 通知闭环**——通知系统自身故障时其告警通道同时失效，WARN 才自守；阈值 config 可配 0=关） | 通知量级异常增大——**计数器已建（2026-10-27 观测批——本批落码：dispatcher ok/dead 计数+notify_volume_check cron）**；廉价预判本体仍触发型 | 外审 F-2-8 |
| C-17 | S5 | mirror 侧车失败聚合可见面（F-C1）。**2026-10-27 重锚**：触发信号 B10-4 已过点（62c03c2 已交付）——从触发型转待办，**重锚 phase6 Week 0 基建**（运维观测面族随批；2026-10-27 审计注：原「触发已过未销」实为「触发已过未重锚」——销账=事项作废，本事项仍待做，措辞以本行为准） | ~~B10-4~~（已过点→重锚 Week 0） | F-C1 |
| C-18 | S5 | ListDeleted 无索引（000049 partial idx 已建—— 态查询用）/ Mark 失败 500 残留行。**✅ 已拍 b 案（2026-10-27）改判人判断型**：低频渐进有体感，观察者=收「附件页慢」报障的维护者+段收口走查人；巡检 SQL 即观测面=`SELECT status, count(*) FROM file_objects GROUP BY status`（异常增长即信号）——已随本行入档，勿另立埋点 | P3 量级到了（人判断型） | — |
| C-19 | S6 | ~~settings Upsert version 乐观锁~~ **✅ 已落码（000050+settings_service.go:232 CAS+expected_version 双测试——早于本台账终版即已交付，登记滞后——v3 审计 L-2 补标）**。（2026-10-27 编号消歧注：外审 F-D2 原发现覆盖 rules/settings 两面——rules 面=A-3[000060]，settings 面=本条[000050]；原括注「F-D2——000060 在途收尾」迁移号系串写，以本行为准） | 下个迁移顺位 |
| C-20 | S6 | **outbox 持久化事件**：`DomainEvent` 加 EventID + 发布点事务内落 `domain_event_outbox`（12 处收敛于 emitEvent 透传 tx）+ 骑既有 CronScheduler 轮询投递复用现总线与全部订阅者 + 订阅者幂等化（inbox `unique(event_id,user_id)` / 通知 message_log 按 event_id 查重）——语义 at-most-once→at-least-once、崩溃不丢、可重放；零新依赖 ≈1–2 天（03 号 outbox 备选 / 11 号 §5.10）。**注**：MQ/asynq 本身不解决「commit 后崩溃」窄缝（DB 与队列双写），outbox 为必经之路；跨进程消费者出现时仅换轮询器投递目的地，业务代码不动 | 出现「丢一条即事故」的消费者（phase6 R3 若不走对账）或跨进程消费者（taskrunner/未来 SOC 平台订阅工单事件） |
| C-21 | S4 | 任务结果留存/展示（taskrunner result 存储+端点——`job_runs` 现无 result 列，error 为唯一结果载荷）+ 结果可配置落 activelist / 任务数据源可配为 activelist 两设想 | phase7/8 脚本平台立项批（或所有者明示单独立项） | 04 号 §3.5（2026-10-26 登记） |

## D. 外部依赖 / 远期

| # | 项 | 依赖 |
|---|---|---|
| D-1 | UIP 契约确认 → `delivery` 字段定形 + `channelEscapes` 重映射（含 C-13 重审） | 企业统一消息平台 |
| D-2 | M-UAA（内网 MQ + 契约四问） | 内网启动 |
| ~~D-3~~ | ~~S6 二期 inbox（notifications 表族+消息中心页）~~ **✅ 已交付（2026-10-24 B6-7——ab63d6f+000072 notifications 表族+inbox 双域订阅+读面三端点+消息中心双 tab 页；G-23 四程终态 000076/78。2026-10-27 销账：本行依赖项早已全满足而条目未销，属收官期登记滞后（全景审计 D 组复核）** | ~~S6 一期 ✅ + S3a ✅~~ |
| D-4 | S8 数据大屏 / S3b 首个审批场景 | 触发条件各自 |
| D-5 | S5 A.6 前端组件路径回填实施形态 | 05 号文档批 |

## E. 待拍板（下次开工前需所有者一句话）

| # | 项 | 拍板时点 | 建议 |
|---|---|---|---|
| ~~E-1~~ | ~~supervisor 的 org 管理角色码~~ | ✅ **已拍（2026-10-24 所有者——方案改写）**：上级来源 = 人工指定（Create/Assign 填 `escalation_supervisor_id`）> HR `manager_id`（M-HR 后）；角色码推导**作废**（dd §31 通则：「人工指定为基线，HR 直属上级为进阶」） | 10 号 §3.4/A.5 修订 |
| ~~E-2~~ | ~~B10-4 时间源~~ | ✅ **已拍（2026-10-24 所有者确认）**：(a) `status_changed_at` 列（`UpdateStatusTx` 咽喉同写）+ 存量回填后**首跑观察模式**（只记日志不发通知，防 updated_at 近似值误报） | 10 号 A.3 |
| ~~E-3~~ | B2-4 lint 是否阻断保存 | ✅ **已拍（2026-10-24 所有者确认——2026-10-16 批量预确认项开工前再确认）** | 告警不阻断起步——与 11 号 §4/02 号 A.5 既有成议一致 |

## F. N/A 已销账

| 项 | 因 |
|---|---|
| rules version 乐观锁 | ✅ 已落码（000060 + UpsertRule CAS + 92506/409 + 四子测） |
| binder 接口化 | YAGNI 销账（外审 F-4-1：purpose 分支内联三处显式=实施终态） |
| exprcalc 超时注释+Now 死码 | ✅ 已清（bf4a388） |
| JWT_SECRET<32 release 拒启 | ✅ 已落码（validate.go:139-147 完整实现——旧登记「未提交」过时） |

## G. 收官审计新增（2026-10-25——分段对表×场景业界×前端全量审计产出）

> 出处：收官全量审计（六路分段取证+前端检查+真机走查）；编号 G-序号（承接 v5 审计 G 系发现位）。

| # | 段 | 项 | 级 | 量级 | 挂点 |
|---|---|---|---|---|---|
| G-1 | S1 | fetch 装配链生产雷：fetcherReg wire 第 9 参恒 nil（R1 同型第五例）——auto_fill 静默不生效，formula 引用 fetch 字段提交期 400 卡用户；错误 UX 三档未建。**2026-10-27 销账批更新（半清）**：装配雷已排——providers.go:274 已注 `datafetcher.NewRegistry()`（空 Registry：ApplyFetches 跳过非 nil panic，「恒 nil」不再成立，装配处注释在案）；能力缺口（fetcher 实现/超时/限流/审计/UX 三档）归 C-1/C-2 不变 | P1 | 随 phase6 R3 首源批 | C-1/C-2 并账（registry 注入+适配器+超时/限流/审计一次配齐） |
| G-2 | S6 | ~~variables 变量字典端点缺席~~ **✅ 销账（2026-10-27 销账批）**：router.go:511 selfService 端点 + handler Variables 实现均已落（B6-3 增强批交付，审计实证）；残余「白名单硬编码与种子模板不同源/动态字段源+时间类」归 **B-2** 唯一继续（本条原系 B-2 加重证据） | P1 | 销账 | →B-2 |
| G-3 | S5 | avatar confirmed 后用户永不 bind → 对象永无 GC 覆盖（第三段仅扫 ticket purpose）——行+对象永久泄漏。**2026-10-27 销账批复核：成立**（gc.go:83 第三段反连接回收显式限定「工单域 purpose」，avatar 豁免即本缺口；行号已漂移以判据为准）。**2026-10-27 拍板：随观测批同批修**——业界 abandoned-upload reaper 标准形态，三细节入档：①超期窗口**复用第三段既有口径**（OrphanGCDays×2，不另立 TTL 机制）②**观察模式灰度先行**（config 开关默认 observe=只记不删——S10 timeoutObserve 同款纪律，确认无误删后关）③回收留痕进 /metrics（zhuzhao_gc_reclaimed_total 按 purpose）。防误删双保：users.avatar 引用反连接 + TestGCJob_ConfirmedOrphans 守卫同步改写（avatar 豁免断言→bound 不删/unbound+超期 observe 不删/enforce 删 三态） | P2 | +0.5 天（随观测批） | ✅ 随 2026-10-27 观测批落码（第四段 reaper+observe 默认开+三态守卫用例——TestGCJob_ConfirmedOrphans 内 G-3 三态） |
| G-4 | S3a | sla_due_at 仅 Transition 写刻——经 Close 到终态的单无 deadline 刻（Close/Assign 收敛批未做） | P2 | +0.5 天 | Close/Assign 收敛批随行 |
| G-5 | S3a | RunPostActions 失败可观测：ops.action_failed 事件 emit + 收件人锚点（ActorID/payload.actor——规则 creator/fixed 档可路由）+ 词表登记（UIP 面） | P2 | ✅ 2026-10-26 收口批（emit 原已在——补锚点+词表+ui 下拉镜像） | inbox 面因 ops 域无 cc/assignee 锚点归 **G-9** 批（勿加死词行） |
| G-6 | S3a | reassign 声明 at_most_once 但语义=同步强一致通道——失败静默丢转派，一致性档位需复核 | P2 | 拍板 | S3a 二期拍板 |
| G-7 | S6 | inbox 不查 opt-out——站内通知用户无法自关（与外发偏好脱节） | P2 | +0.5 天 | 通知中心权限批（业界=GitHub per-channel） |
| G-8 | S6 | ticket.action_notify 未登记 inbox 词表——站内静默 | P3 | ✅ 2026-10-26 收口批（补词「流程提醒：{{title}}」——payload.cc_users=动作 recipients，锚点实） | 销账 |
| G-9 | S6 | ops.cron_failed 词表行是死行（payload 无 to_assignee/cc_users——collectRecipients 恒空） | P3 | 15min | 随批（改锚点或特判 job 收件人） |
| G-10 | S6 | message_log 实落表与 A.2 漂移（biz_id VARCHAR/无 dead 态/retry_count）+死信归因字段折损 | P3 | 登记不修 | 二期表形态批 |
| G-11 | S9 | ~~CleanupExpired 未挂 cron~~ **✅ 销账（2026-10-27 销账批）**：app.go:195 `intel_cleanup` 已挂——`DELETE FROM intel_cache WHERE expires_at < NOW() - interval '7 days'`，骑 CronScheduler + SetOnFailure 告警（审计实证+独立复核双确认） | P2 | 销账 | — |
| G-12 | ui | 铃铛 invalidateQueries 死代码（角标最长 60s 不更新）/画布 undo 后 lint 态 stale/intel 页裸 fetch 绕 api 层/附件列表失败静默 | P2/P3 | +0.5 天 | 前端随手批 |
| G-13 | 文档 | 10 号变更记录缺 SLA 0.5 交付行；errcode.md 双份已补登 21010/92600-92602（✅ 随本批修） | P3 | ✅/5min | 文档批 |
| G-14 | S1 | FieldImpact radio 选项收窄漏检+dict: 引用收窄天然失效 | P3 | 15min | B1-6 前小修 |
| G-15 | ui/S5 | storage API 六端点漏 `/api/v1` 前缀（b72b854d 建档即如此）——浏览器内附件列表拿回 index.html 渲成 3418 幽灵行、五个 POST 404：头像上传/附件上传删除/富文本渲染整面静默失效 | **P0** | ✅ ui e88173bd | 销账（+`apiPaths.test` 静态守卫，红→绿恰报六处） |
| G-16 | S9/S6-2 | intel_query/message_center 建菜单行未绑 role_menus（000031 惯例漏项）——全员不可达，铃铛 /messages 404 | **P0** | ✅ 000074 | 销账 |
| G-17 | S3a | 分派规则页（ui 45a72256）无菜单行——孤岛页；viewNames 守卫单向不报 | **P0** | ✅ 000073+000074 | 销账（ui 反向守卫同批） |
| G-18 | ui | E2E 三 spec 失配：S5 私有 login 等 submit 型按钮+history 路径（B5-2/3 自建档从未跑绿）/S10 等已被 UserPicker 替换的 ID 输入框/S12 字段配置默认设计模式——三者均被误判「超时」 | P1 | ✅ ui e88173bd | 销账 |
| G-19 | ui | UserPicker `/^\d+$/` 分流工号查询，E 前缀工号永走 username 分支——工号搜索不可用 | P2 | ✅ ui e88173bd | 销账 |
| G-20 | ui | viewNames 派生器两盲区（INSERT…SELECT 页面行只认别名 p 且一律记按钮 / UPDATE visible 不重放）+「零动作」守卫从未登记 0 形同虚设 → system_setting 无 defineOptions、system_notification 名错，两页 keep-alive 恒不命中 | P2 | ✅ 本批 | 销账 |
| G-21 | ui | 菜单 icon 映射漏 intel/message（icons.test 种子清单手工维护未同步）——侧栏图标静默空白 | P3 | ✅ 本批 | 销账 |
| G-22 | ui/CI | viewNames 同级主仓固定拉 main——特性分支种子页在 CI 不进期望名单（反向守卫会误报孤岛） | P3 | ✅ 本批（同名分支优先，push 后核 CI 转绿） | 销账待核 |
| G-23 | S6-2 | 消息中心是否默认全员可见：铃铛对所有登录用户渲染，而 /messages 由菜单驱动仅绑定角色可达（000074 后=admin+superadmin），operator/viewer 点铃铛 404 | 拍板 | ✅ 按建议实施（三程 2026-10-25）：a 变体=inbox 三端点移 selfService（自数据）+ ui /messages 常量路由 + 000076 菜单行惰性化；入口=铃铛，侧栏项消失 + **000078** 删幽灵行/inbox:read 退役（四程） | 销账 |
| G-24 | ui | SEED_ICON_NAMES / MENU_ICON_MAP / viewNames STATIC_ROUTES 三清单手工维护——派生化（从迁移种子/routes.ts 解析） | P3 | 登记 | 触发=再漏一次 |
| G-25 | ui | phase5 新能力 E2E spec 为零——五件待补：CC/标签、S2 画布、S6-2 消息中心+铃铛、S3a 转换+规则页 | P1 | ✅ 本批二程（26 spec / 33 用例） | 销账 |
| G-26 | S3a/全库 | **L1 越权**：000070 把 /assignment-rules 三写端点挂 ticket_type_manage 页面行（000073 照抄）——违反 W1 B 案「页面=读/写挂按钮」；viewer/operator 预设只绑页面即持建/停/删规则权（POST 实测 400 过 L1） | **P1** | ✅ 000075（本域）+ **对账已执行**：全库扫页面行挂 POST 得 system_dict×6/system_notification×8（按钮行 dict_write_btn/notification_write_btn 建行即空绑，两页仅绑 admin/superadmin=潜伏）→ ✅ **000077** 迁按钮行 + intent-preserving；message_center×2 随 G-23；ticket_list transition×1=000068 设计内维持 ；**结构性守卫落地（四程）**：AuditPageWriteBindings+pageWriteAllow→启动 fail-fast / TestCatalogPageRows_NoWriteBindings（CI）/ acceptance D2 B案 三道 | 销账 |
| G-27 | ui/S3a | 规则页 orgOptions IIFE 快照（query 在途即永久空）——冷加载 UI 建不了规则 | P1 | ✅ 本批（computed） | 销账 |
| G-28 | 全库 | 迁移回滚与 B 案守卫耦合：000075/76/77 down 回挂写端点到页面行，而启动 fail-fast 拒启——只回滚 DB 不回滚代码之路被堵（外审 #7） | 登记 | ✅ AGENTS 迁移规范 + runbook §6 操作序 | 文档已落 |
| G-29 | S6-2 | inbox 收件人路由接 message_rules（管理员可关某类站内信）+ 全局 actor 排除（外审 #8 设计半；本批仅 updated/commented 自收去噪）。**✅ 已拍（2026-10-27 所有者，拆两半）**：①message_rules 半部=**做**——与 G-7（用户级 opt-out）并账立项随 phase6 启动批随行（两层一起做才不留「另一层关不掉」半残，~1–1.5 天；登记行待补进 phase6/05 §1.3——该文件并行会话在途，落账随其收口）②actor 半部=**维持窄版白名单**+演进约定「新动作默认通知，实锤噪音再加 selfNoiseActions」——全局排除需破坏 TestB67 既定回执语义，收益配不上，销账 | ~~拍板~~ | ①→phase6 启动批随行（与 G-7 并账）②销账 |
| G-30 | S6 | `HookRegistry.Drain`：`wg.Add` 与 `Wait` 并发——计数 0 时 Add 未 happens-before Wait（sync 文档明令禁止）可漏排空；app.go 注释「UIP 3s」≠实际 5s，5s 预算 < 发送 5s+落库，drain 超时成常态（外审 #10） | P2 | ~~所有者在途 registry.go/app.go 未提交~~ **✅ 已修（2026-10-26，653a3b5 入库后理由失效即修）**：closed 闸（Dispatch 持 RLock 检查+Add，Drain 持 Lock 置位后 Wait——happens-before 成立即拒收残发+Warn 留痕）/ 预算 5s→8s / 注释 3s→5s 勘误；测试=closed 后拒收不迟到送达（变异验证去闸即红）+ 并发锤击 Dispatch×Drain（-race 无 panic、Drain 完成后零迟到） | ✅ 单测+变异+lint 0 + 四档 acceptance 249/0 + 迁移往返 79 对零漂移（当前树构建） |
| G-31 | S10 | 标签摘除 UX：B「挂」已存在标签是 ON CONFLICT 无操作，再摘=摘 A 挂的→需 update 权 403（外审 #18 判设计内，拍板点 6 口径）；可改进=挂接口回告「已存在」 | P3 | 登记 | 触发=用户反馈 |
| G-32 | S3a | **系统身份写操作的委托放大族（2026-10-26 并账扩容）**：①`SystemAsCreator` 以工单创建人身份执行 cc/reassign 写动作（外审 #19）——配置面受 ticket:type:manage 控制；②`assignAsSystem` 跳过 Assign 的 L2/L3（自动分派规则消费，unexport 防包外直调+权限旁路注释——同包唯一调用方 DispatchService）。族口径：**配置自动分派/动作=获得对应写操作的委托放大**，评审新系统身份通道须按此口径并回本条登记 | 登记 | ✅ 03 号变更记录注 + assignAsSystem unexport/注释（本批） | 文档+代码已落 |
| G-33 | S10 | 催办先发后记水位：bumpReminder 失败→下轮重发（at-most-once 退化 at-least-once）取向=宁重发勿漏催（外审 #20） | 登记 | ✅ 代码注 + 10 号变更记录 | 文档已落 |

## H. 通知能力增强批（2026-10-27 立项——所有者需求明确："多类型工单差异化模板 + 钉钉 markdown + 工单字段变量"；大部分架构已实现，三缺口增量）

> **需求对照备查（2026-10-27 核验）**：①"到流程节点自动按模板发消息"=✅已实现（AssertTransition 单点 emit → rules 路由 → 渲染 → 发送 → message_log → 死信，三轮审计过）；②"邮件 HTML"=✅已实现（渲染器渠道分族：HTML 渠道值级转义+模板 HTML 原样）；③"钉钉 markdown"=⚠C-13 已登记（channelEscapes 加第三族，触发=契约确认——本批提前）；④"变量来源工单字段"=⚠架构已实现（`ticket.fields.f_<ID>` slug + 四象限渲染）但 variables 字典端点缺席=B-2/G-2 已挂账（本批并账）；⑤"每类型单独配模板"=🔴核心缺口（rules UNIQUE(biz_type,event_action) 无 type_code 维度——一事件动作只能绑一模板）。

| # | 项 | 内容 | 量级 | 拍板 |
|---|---|---|---|---|
| H-1 | S6 | **规则粒度加 type_code**：`message_rules` 加 `type_code VARCHAR(64) NULL`（NULL=全类型通配，存量行回填 NULL 语义不变）+ UNIQUE 改 `(biz_type, event_action, COALESCE(type_code,''))`（函数唯一索引）+ 渲染链规则查询两级回退（类型精确 → 通配）+ 规则管理面加类型下拉 + `/jobs/actions` 同款 schema 面 | ~1 天 | **✅ 已拍（2026-10-27 所有者「按建议」）**：粒度加在规则（模板跨类型复用） |
| H-2 | S6 | **通用渠道接口 + 钉钉首发 adapter**（2026-10-27 所有者拍板修订——公司钉钉为**封装接口**，渠道抽象必须泛化）：`ChannelSender` 端口（已存在：`Send(ctx, Message) + Channel()`，UIP 桩为首个实现）即该通用接口——dingtalk 作为第二个实现类（封装钉钉 API 适配器：认证/消息体封装/markdown 透传），**后续飞书/企微/外部钉钉等各自实现同一接口**；配套：`channelEscapes` 加 `dingtalk` 第三族（markdown 不做 HTML 转义——C-13 提前）+ 渠道白名单/管理页枚举扩展 + notification_configs 加 dingtalk 配置段（endpoint/secret/封装 API 参数）+ message_log 渠道记录 | ~1–2 天 | **✅ 已拍（2026-10-27 所有者）**：通用接口 = 既有 `ChannelSender` 端口（不新造）；各渠道各自实现（钉钉封装 API 首发 adapter），禁业务代码硬编码平台 HTTP（06 号 §3 红线既有） |
| H-3 | S6 | **variables 字典端点 + 编辑器变量 picker**（=B-2/G-2 并账升级为本批核心件）：按 `(biz_type, event)` 供变量树（内置六键 + S1 `field_meta` 动态字段 slug + 时间类）+ 模板编辑器变量选择器（点击即插 `ticket.fields.f_<ID>` path）——修复"管理员重存种子模板 400"（G-2） | +0.5 天 | 无（B-2/G-2 既定方案） |
| H-4 | S6 | **测试与文档随行**：四象限渲染回归（dingtalk 渠道下 markdown 原样）、规则两级回退（精确/通配）、钉钉 adapter 负向（签名错/非 2xx）→ message_log failed+死信；06 号 §3 渠道词表加 dingtalk+§4 H-1 粒度注 | ~0.5 天 | — |

**合计 ≈ 2.5–4 天；建议容器 = 独立「通知能力增强批」或并入通知中心权限批（G-7+G-29 同域）；phase6 R3 风险单联动 = 多类型差异化通知的第一个真实消费者。**依赖：无硬前置（钉钉机器人协议公开，UIP 契约不影响 dingtalk 独立通道——C-13 的"契约确认"门控仅约束 UIP 面正文格式，本批绕开）。

## L. 文档架构整理（2026-10-27 立项——phase5 合回 main 后执行）

| # | 项 | 内容 | 量级 | 触发 |
|---|---|---|---|---|
| L-1 | 全仓 | **deliverables/ 归位 docs/ 体系**：24 文件 git mv——A 组审计终稿 11 篇（含 json-contract-audit-2026-10-14，系审计终稿非提案——外部报告分组修正）+ B 组收尾 2 篇 → `docs/review/`；C 组研究稿 10 篇（secops-platform-vision-2026-10/）+ 任务卡 → `docs/proposal/`；overview.md **保留**（时间线索引有独立价值） | ~0.5 天 | **phase5 合回 main 后**独立 doc-only 批——并行会话在途期间不搬（26 文件未暂存改动撞车风险） |
| L-2 | 全仓 | **36 处 deliverables/ 引用改写**：22 个 docs/ 文件（phase3/4/5/6 段稿+11 号控制台+roadmap+runbook+standards；11 个 markdown 链接+25 个纯文本提及）随 L-1 同批改新路径；**高权重三件必须核**：11 号控制台批次行 / standards / runbook | 随 L-1 | 同上 |
| L-3 | deliverables/overview.md | 修 5 处 `.workbuddy/` 死链（cb996f2 已删该目录；:24/25/52/53/65 五处）——**L-1 搬移时顺带**（overview 保留不拆，外部报告"拆进 review/README"建议不采纳） | 5min | 随 L-1 |
| L-4 | 全仓 | **overview.md 指向更新**（保留决策修正外部报告 D 组"拆解删除"建议）+ `docs/review/README.md` 与 `docs/proposal/README.md` 各补一行归位说明 | 10min | 随 L-1 |

> **外部报告核验结论备查（2026-10-27）**：盘点/分组/不按 phase 拆三点采纳；四处修正——①实为 24 文件非 23；②json-contract-audit 归 review（审计终稿非提案——三仓修复已提交+standards §3-14 立约）；③overview 保留不拆（时间线索引独立价值，仅修死链）；④**成本重估**：24 mv + 22 文件 36 处引用改写（11 号/standards/runbook 三 SSOT 在内）——非"git mv+相对路径修正"轻量级。deliverables/ 现位系刻意决策（11 号批次行"deliverables 另笔"——立项前研究→结论蒸馏 docs/→研究稿留档的工作流载体），非历史混乱；L 组执行即工作流终态归位。

## 挂账质量判据（外审 F-4-5 推广）

> **登记了触发信号 ≠ 触发信号可观测**——C 组「触发驱动」类挂账若代码面零实现
> 观测面（端点/日志/SQL），事实上等于「永不触发」。后续登记触发驱动项须同时
> 回答：触发信号由什么代码产生？谁来读？
> **豁免（v3 审计 L-2 补）**：诉求/决策型触发（如 C-3 护栏泛化、C-4 翻案、
> C-8 危害实证）信号=所有者人判断，不要求代码观测面——判据只约束技术信号型
> 条目（量级水位/契约到达/依赖启动类）。
>
> **观测面现状（2026-10-27 全景审计 A-2 核）**：现有观测面 = `/metrics` 手写
> 三计数器（requests/panics/5xx——P4-8，internal/middleware/metrics.go，零依
> 赖）；**业务水位零埋点**（磁盘用量/通知量级/intel 命中率/池饱和）——量级水
> 位型 C 项（C-6/11/16/18）在 A-2 拍板前按「永不触发」管理；若拍 a 案（补观测
> 面），/metrics 即扩展基座。
>
> **A-2 拍板结果（2026-10-27 所有者）**：四条不同质、分项拍——**C-6/C-16 a 案**
> （观测批同日落地：storage 用量 SQL 采样 gauge+阈值 emit；dispatcher 计数+量级
> WARN）；**C-11/C-18 b 案**改判人判断型（升级纪律→runbook §3；巡检 SQL→C-18 行
> 本体）；C-5（非 A-2 范畴）观测先行独立小批。评审注记两条入档：①RustFS 原生
> metrics 端点兼容性未实测，不构成免开发依据；②phase6 R1 水位系 `ingest_sources`
> 表增量拉取机制（01 号 §1），与本观测面非同一基座——勿以「复用」为由合并论证。

---

*基线：zhuzhao `2dcb496` / zhuzhao-ui `7839aea2`（2026-10-25 收官审计）——A/E/F 组全清+B/C/D 组按上表销账；G 组=收官全量审计新增 14 件（P1×2/P2×7/P3×5）**+ E2E 补齐批（2026-10-25，ui e88173bd 起）新增 13 件 G-15~G-27（P0×3/**P1×4**[G-18/25/26/27——2026-10-27 计数勘误：原记 P1×5 系把 G-23 拍板行误计入]/P2×2/P3×3——除 G-24 登记外全部销账；G-23 三程按建议实施）**；**+ 外审 v4 批（2026-10-26）G-28~G-33 六件（登记已落 3 / 拍板 1 / 待所有者在途 1 / UX 登记 1）**；**+ 提交与规划全景审计销账批（2026-10-27，基线 aa34302 报告对称验证后执行）**：G-2/G-11 销账、G-1 半清更新（装配雷已排）、G-3 复核成立重锚、C-6/11/16/18 标◆待拍（A-2 观测面决策待所有者）、判据段+观测面现状注——审计抽验 3 例滞后（G-2/G-11/G-1）全部落账；下一迁移编号 **000080**（000073–000079 占用——000079 = action_notify_seed 已落盘）；**+ A-2/G-29 拍板落账批（2026-10-27 所有者拍板，本批）**：C-6/C-16 拍 a 案（落地形态入行——观测批同日执行）、C-11/C-18 拍 b 案改判人判断型、C-5 观测先行独立小批、G-29 拆两半（message_rules+G-7 并账→phase6 启动批随行；actor 维持窄版销账）、G-3 随观测批同修（reaper 三细节入行）、C-17 触发过点重锚 Week 0、D-3 销账（B6-7 已交付）、C-19 迁移号串写订正+A-6 双哈希消歧+A-5 预算值订正+头基线滚动化；合 main 缓办（所有者 2026-10-27：先执行批 1–5，合 main 后置）；**+ 观测批落码（2026-10-27，C-5/C-6/C-16/G-3 四件）**：C-5=20015 审计行+WARN（fix(auth) 批）；C-6=file_objects SQL 采样 gauge+阈值 cron+跃迁 emit+000080 种子；C-16=dispatcher ok/dead 计数+小时量级 WARN cron；G-3=GC 第四段 avatar 弃置收割（observe 默认开+users.avatar 反连接双保+三态守卫用例）——下一迁移编号 **000081***
