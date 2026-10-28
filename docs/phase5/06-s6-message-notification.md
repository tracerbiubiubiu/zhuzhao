# 06 — P5-S6 消息通知：统一消息平台 + 模板 + 工单触发（设计稿）

> **段属性**：基线段（phase5 收口对象）。**建议实施序**：与 S2 错峰衔接（S1 后；单人串行——S6 一期先于 S2）；**S10 依赖本段**（抄送通知/超时通知），phase6 硬前置 = 本段**一期**口径。
> **量级**：一期约 2–2.5 周（通道/模板/解析/触发/记录各 2–3 天 + 事件面基建 3–5 天）+ **二期站内消息中心 1.5–2 周**（2026-10-13 翻案挂入，S6 总账 3–4 周）。**迁移**：有（一期 `message_templates`/`message_log` 两表族 + `message_rules` 订阅规则轻表（附录 A.5）+ `tickets.human_key` ALTER（拍板点 2 随批时）——**≥3 段迁移（2026-10-16 外审计数对齐）**；二期 notifications 表族——编号启动时对账，不写死）。
> **文档定位**：段级设计稿（2026-10-15 建档，phase5 分支）。对接参考 = [phase3/10 §3 通知服务设计](../phase3/10-ticket-business.md)（邮件矩阵/事件订阅）；立项与演进 = dd §27（2026-10-08 四轮）+ 站内消息中心翻案条（2026-10-13）。
> **事件面公约注（2026-10-28 P1-7 拍板）**：进程内事件面全生态公约层 = [standards §14](../standards.md)——本段为第一消费者，机制细节锚仍在本稿（事件面基建/Drain 语义）；新域消费者按公约三件随批。

## 1.5 通知能力三入口（2026-10-23 对话设计批——架构澄清）

NotificationService = **能力层**（模板渲染 + 收件人解析 + 通道发送，写一次）；触发形态 = **入口层**（多路复用，不重复）：

| 入口 | 触发时机 | 收件人来源 | 内容来源 | 实施时机 |
|------|---------|-----------|---------|---------|
| **事件触发**（B6-5） | 工单状态变更 | 事件上下文解析 | 规则绑定模板 + 事件参数 | **S6 一期 ✅** |
| **定时触发** | cron 扫描超时/到期 | 配置人群 | 数据聚合（在途单 digest） | B10-4 自然出现（不预建） |
| **人工触发** | 管理页/工单页按钮 | 管理员选择 | 自定义或选模板 | 有场景时加薄端点（半天） |

**编排器前置件（2026-10-23 批 3 对表 F-3-1 落档）**：上表「能力层」当前实体化为 `TicketNotifyHook.OnDomainEvent` 内嵌编排链（查模板→渲染→解析收件人→偏好过滤→节流→发送→log，≈115 行，`service/ticket/notify_hook.go`）——**定时/人工入口开工前须先抽出 `MessageDispatcher`**（单方法 `Dispatch(ctx, req)`，hook 变「DomainEvent → DispatchRequest」薄适配器；业界同型=多入口单管道：Laravel Notifications / Knock workflow）。登记 = **B10-4 开工首件（+0.5–1 天，11 号 B10-4 行已同步）**——按「第二个消费者出现时抽」纪律，不单开批（现在抽无第二消费者验证抽象边界）。命名消歧：P4-2 `NotificationService` = 渠道配置管理（List/Create/Update/Delete/NotifyDeadLetter，无 Send/渲染/解析）**不改名**（调用面+swagger 已铺）；新层 = `MessageDispatcher`，上表与 03 号 §3.3 原「NotificationService」能力层指代一律读作此层。

定时和人工触发**不预建**——NotificationService 接口设计留好"可被任意调用方调"即可（B6-2 通道层设计时注意此约束）。

## 1. 定位与由来

**定位 = P4-2 通知通道的能力延伸，非 §23 翻案**：P4-2 已交付渠道配置化 webhook + 死信告警 + taskrunner 终败通知 hook（E-⑥ 形态落地）；§23 暂缓清单中"通知"一项随 P4-2 事实上解封。本段补齐三件：**统一消息平台渠道 + 消息模板 + 工单事件触发**。**SLA/审批/分派/报表维持暂缓不碰**。站内通知原"不取"（2026-10-08/09 两次拍板）——2026-10-13 所有者铃铛需求**翻案**：原口径否决的是"触达通道"，铃铛实际要"站内聚合工作面"，与钉钉/邮件互补非替代——**挂 S6 二期**（见 §6）。

## 2. 现状基线（代码级锚点，2026-10-15 核实）

| 项 | 现状 | 锚点 |
|----|------|------|
| 渠道配置 | P4-2 本体：`notification_configs` 表 + 管理 API 四端点；**首版仅 webhook 渠道**（`Channel` 字段就位、单渠道类型） | 迁移 `000033`；`internal/router/router.go:335-341`；`internal/model/notification.go:11` |
| 发送链 | webhook client 分发尽力而为（单渠道失败不阻断）+ 死信告警全链实测（黑洞回调→死信→webhook 送达）+ taskrunner 终败通知 hook（`TASKRUNNER_NOTIFY_*`） | `internal/service/notification_service.go:13-27`；phase4/02 P4-2 行 |
| 配置页 | **未做**（菜单 visible=false 过渡态，000033 预登记占位无视图） | router.go:335 注释；zhuzhao-ui `viewNames.test.ts:15` |
| 缺口 | **消息模板、统一消息平台渠道类型、工单事件触发挂接、发送记录** 四件零实现 | README §1 现状表 |
| 事件面 | `ticket_events` **无 event_type/processed 列**（000010:70 注释承诺的迁移编号已被 A2 挪用）——选"事件消费面"形态须先补迁移 | 迁移 000010:70；phase4/02 P4-2 行触发源注意 |

## 3. 通道层（环境红利：全渠道单接口）

- 企业统一消息平台 = 邮件/钉钉等所有渠道**共用一个调用接口**——通道抽象只需在既有 `notification_configs` 上**新增一种渠道类型**（type=uip：endpoint/凭据配置），**不做任何渠道 SDK 直连**（直连属环境变化再拍）。
- 发送器抽象为 **`ChannelSender` 端口**（Send）——统一消息平台 = 首个 adapter；换平台/加渠道 = 新 adapter，**业务代码禁止硬编码平台 HTTP 调用**（README §6 清单第 3 行）。
- **两级渠道词表（2026-10-23 批 3 对表 F-3-2 落档）**：`channel` = **adapter 层**（S6 侧传输适配，白名单 `uip`/`webhook`——新渠道=新 adapter，supportedChannels 校验不变）；`delivery` = **投递偏好层**（email/dingtalk/… 等平台内部路由目标——UIP 契约面，非 S6 传输语义）。`channelsender.Message` 增可选 `delivery []string` **透传字段**：一期不校验、UIP 契约确认前允许留空；escalation（10 号 §3.4）/on_complete（03 号 §3.3）设计稿中的 `channels` 字段均按此词表改标 `delivery`——防实施者把投递偏好当 adapter 渠道撞白名单 400（两条路都不通的原死结消解）。
- **直连差异清单（2026-10-16 业界核验回标——「直连属环境变化再拍」补齐"再拍时拍什么"，防重新调研）**：企微/钉钉/飞书服务端推送同为 HTTP+JSON（群机器人 webhook / 应用消息 API 两形态，官方 SDK 仅为薄包装），与 UIP adapter 同构、端口签名不动；直连时三点真差异全落 adapter 内部——①鉴权两族（静态 webhook+可选 HMAC-SHA256 签名 vs app secret→access_token 生命周期管理：缓存+过期刷新）；②收件人 ID 体系渠道私有（企微 userid/飞书 open_id/钉钉手机号——需 user→渠道 ID 映射，UIP 核心价值即收敛此映射）；③每渠道分钟级限频（机器人约 20–100 条/分钟，钉钉超限罚停 10 分钟）须 adapter 自带令牌桶+退避；正文格式差异见 §8/A.4。
- **无契约先行开发策略（2026-10-09 所有者确认）**：平台为 HTTP 发消息形式、正式契约文档不便外传——**端口先行**：模板/渲染/触发/message_log 全链先建，adapter 按"HTTP+JSON"常见形态先写骨架（endpoint/鉴权头 config 化，字段映射层独立成函数），真实契约到手只改映射。
- **桩两态口径（2026-10-09 审计建议采纳，防虚标纪律）**：开发态桩 = 返回成功 + 落日志（供触发→模板→渲染→记录全链 E2E）；生产态 = 未配置 **fail-fast** 报错；两态由 config 开关区分。**桩不得标注为"已实现统一消息平台对接"**。

## 4. 模板层

- **表**：`message_templates`（code/名称/渠道/主题/正文/变量 schema）+ CRUD + 管理页（通知中心页面族，自 S4 迁入）。
- **渲染**：**服务端占位符替换（字段值转义）**——不用完整模板引擎/text/template，防模板语法执行（用户可控的工单字段值不得被当模板代码）；选型预置 = `{{ticket.assignee_name}}` **点路径占位符**（logic-less 零执行——2026-10-16 升级，A.4）+ 值按渠道转义（A.4 三族）。**变量字典 = 服务端 SSOT**：按 (biz_type, event) 供给可用变量树（工单域与 S1 field_meta 同源），模板引用 ⊆ 字典——模板作者零手维护变量清单（A.4）。
- **变量来源含时间类**（当前时间/相对时限，格式可配——2026-10-09 场景补充）。
- **模板内容设计准则（2026-10-09 隐藏需求核查）**：通知出平台边界（邮件/IM）——默认只带"**编号 + 标题 + 链接**"，详情留在平台内（点入过 L2），**防渲染详情字段向 L2 不可见收件人泄露内容**；详情入正文须显式声明为例外。预置种子模板只含编号+标题+链接（不含越 L2 才可见字段；更丰富模板由管理员自担）。
- **模板/字段引用完整性同批**：模板删除守卫 + 被引用字段改 key/删除的失效告警——与 S1 字段变更影响分析同件复用。

## 5. 收件人解析 + 触发层 + 发送记录

### 5.1 收件人解析（人员参数化）

模板收件人为变量——**第一档** = 工单字段引用（assignee/creator）+ 固定人员；**第二档（按需）** = 角色+组织解析（工单组织的 admin/owner——L2/L3 域）。哲学与 assignee rule/事件解析角色同构：**解析归业务侧，消息平台只收最终收件人**。**域无关要求（phase6 复核回写）**：收件人声明方式不得焊死工单域——phase6 风险通知收件人 = risk owner（非工单字段），须支持工单域之外的业务实体声明。

### 5.2 触发层

- 链路：工单状态流转/待办 → 选模板 → 字段值填参 → 解析收件人 → 经统一消息平台发送。
- **启动拍板点：挂接机制**——
  - **a) 显式 emit（推荐起步）**：状态变更 service 层调用；**须做成可订阅内部钩子**（事件类型化/多订阅者/recover 隔离/事务提交后派发）——**工单状态变更事件面 = S6 与 phase6 的共享基础设施**（一次变更、多订阅者；phase6 R3 风险关单/进状态设 timeout 基准/任务链三个消费者共享同一面，dd §29 事件面定论）——**禁止把消费方逻辑硬编码进 ticket service 状态变更路径**（风险域焊进工单域 = 破坏领域隔离）。
  - b) ticket_events 消费面（phase3/10 原设计）：事件表**无 processed 列/无消费面**（§2），选 b 需补迁移；L1 单消费者模型另需分布式锁防重。
- **投递语义（2026-10-09 补遗）**：触发挂接须**异步投递**（状态变更事务外；发送失败不阻塞、不回滚工单事务——通知无状态可补发，同步阻塞反拖累主路径）+ **同单短时多变更的节流/去重**（防状态翻转通知风暴）。
- **用户通知偏好（2026-10-16 全计划业界对标补——业界通知系统 v1 必备：GitHub/Jira notification settings）**：per-user **opt-out/免打扰**（按事件类型静默或全局暂停档），发送链**前置过滤**（收件人解析后、`ChannelSender.Send` 前查偏好）；存储 = 轻表或 `user_preferences` KV（随批拍）。缺了 = 通知骚扰且用户无法自关（运维被迫人工改库）。量级 +0.5–1 天（随 B6-4 收件人解析批）。存储形态随批拍——**若起轻表，回灌 11 号 §5.9 B6 行迁移计数**（现「≥3 段」口径）。
- **一致性分级（dd §29）**：通知 = 低一致性消费者（丢失无害，钩子 at-most-once 可接受）；**高一致性消费者（phase6 风险关单）失败由对账扫描补偿**（phase6 R3 设计项，非本段范围——但事件面设计须留其挂点）。

### 5.3 人读编号（前置随批件）

工单当前唯一对外标识 = 字符串化自增 id（`internal/model/ticket.go:10`）——通知正文"哪张单"无可引用、跨团队核对困难。建议本段随批落 **`TK-yyyymmdd-NNNN`** 形态编号（新列 + Create 时生成，存量单维持 id）；**若不随批，模板先只引用链接不引用编号**。phase6 风险单同款需求（`RISK-2026-NNNN`，表设计前必拍）——两域生成器可共用一件。

### 5.4 发送记录

`message_log` 表（模板/收件人/状态/错误/重试）+ 管理页（列表按 standards §3-6 PageData 形态——offset+total+钳制）；失败复用既有死信告警链——**接缝（2026-10-19 外审补）**：`ChannelSender.Send` 终败 = **进程内直调** notification service 死信告警方法（与 `/internal/notify/dead-letter` HTTP handler 共用内部函数——同进程不自绕 HTTP）；`DeadLetterNotifyPayload` 扩展可选字段 biz_type/biz_id/template_code（指针型——taskrunner 既有投递零破坏），排障可归因到模板与业务对象。

**删除守卫口径注（2026-10-23 外审 F-1-8/F3-1-4）**：模板删除守卫 = 被规则引用即拦（**含禁用规则**——修复前只数 enabled=true，禁用规则引用被忽略→删后规则再启用即静默不通知）；守卫与删除合并为单语句 NOT EXISTS 原子化（无并发窗口）。

**发送超时层次注（2026-10-23 外审 F3-1-5）**：桩期 5s = goroutine 兜底（——防 UIP 端点挂起致 goroutine 永久阻塞）；契约期 = UIP 契约超时为准，5s 降为下限保护。

## 6. 二期：站内消息中心（2026-10-13 翻案，一期交付后实施）

- **定性**：消息中心 = **事件系统的消费者**——S6 channel 抽象新增「站内 inbox」通道 + notifications 表族（recipient/payload 深链/created_at）+ per-user 已读状态（行级 read_at 第一档/全部已读批量/未读数聚合端点）+ 保留策略（N 天封顶，复用 message_log 经验）。
- **硬依赖 S6 一期**：触发/模板/收件人解析全是 S6 资产，inbox 只加通道 + 读面——绕开则成第二套触发/去重/保留的假消息系统。
- **待办与消息语义分离（业界地基）**：待办 = 状态派生（已办自动消失，无已读概念）；消息 = 事件快照（读过即完）——铃铛 UI 第一档双 tab（通知/待办）合并入口，数据模型分开。
- **实时通道分期**：角标轮询（S10 既有 60s）→ 站内消息 SSE（二期，单向推送内部够用；WebSocket 过度）→ 浏览器原生 Notification 弹窗（可选增强件，登记不承诺）。
- 未读数聚合并入 S10 角标端点 sections 扩展；**phase6 硬前置维持 S6 一期口径不变**（风险台账不依赖消息中心）。

## 7. 启动拍板点

| # | 事项 | 建议 |
|---|------|------|
| 1 | 触发挂接机制 | **a) 显式 emit = 可订阅钩子**（推荐起步；b 需补 processed 迁移 + L1 消费面） |
| 2 | 人读编号 TK- 随批与否 | **推荐随批**（+0.5 天；不随批则模板只引用链接）。**生成器抽象（2026-10-16 外审补遗）**：随批时落**跨域公共件** `internal/pkg/humankey`（前缀注册表 + 按日序列 + UNIQUE 冲突重试 ≤3）——phase6 `RISK-2026-NNNN` 零改复用（zhuzhao 单仓，不进 utils——A.10 同纪律），勿等 phase6 返工 |
| 3 | 事件面基建批次 | 事件面 3–5 天随本段首个批次落（S10 超时扫描/phase6 共享） |
| 4 | **事件面命名（2026-10-16 外审提出）** | ✅ **已拍（2026-10-16）：通用 `DomainEvent`/`DomainHook`**（原 `StatusChangeHook`/`TicketEvent`/`TicketHook` 措辞废止——以 Ticket 命名承载 risk 事件与域无关主张错位；订阅按 `biz_type` 路由；包 `internal/pkg/domainevent`；HookRegistry 不变；消费方侧名字保持业务化如 TicketNotifyHook）。A.1 已按新名改写；S6/S3a/phase6 R3 共享枢纽命名一次钉死 |

> **状态注（2026-10-16）**：本表拍板点已随批量预确认拍定（2026-10-16 所有者「按建议」——✅34/🔶1，状态 SSOT = [11 号 §4](./11-implementation-plan.md)；各段启动时异议可翻，翻案成本=改一行）。

## 8. 边界（不做）

- **不做渠道 SDK 直连**（邮件/钉钉均经统一消息平台）；**不做站内通知一期以外的实时通道承诺**（分期口径 §6）；**模板不做富排版引擎**——占位符替换为唯一模板语法，不做可视化编辑器/完整模板语言（业界该层=独立产品线体量：Novu 渠道编辑器/SendGrid 拖拽设计器/MJML 生态）；**正文格式按渠道 adapter 归属**（纯文本/轻 HTML/markdown 子集——业界无统一 markdown：钉钉子集无表格/企微 markdown 与 markdown_v2 双方言并存/飞书富文本为私有 JSON 结构，2026-10-16 业界核验；渲染转义纪律见 A.4）。
- SLA/审批/分派/报表维持暂缓（§23 域不碰）；站内 inbox 不入一期（§6 二期）。

## 9. 影响面

- **迁移**：一期两表族 + `message_rules` 轻表（附录 A.5）+ `tickets.human_key` ALTER（拍板点 2 随批时）——**≥3 段迁移（2026-10-16 外审计数对齐）**；+（emit 落地所需的事件面挂点，若走钩子形态零迁移）；二期 notifications 表族。
- **权限**：通知中心管理页沿用配置管理权限；`message_log` 管理面权限随批（对齐 notification_configs 既有口径）。
- **端口**：`ChannelSender`（README §6 第 3 行）。
- **消费者**：S10（抄送通知/超时逾期通知）、phase6 R3（风险事件通知 + 关单钩子挂点）、任务链（S3 动作库"通知"动作）。

## 10. 验收口径

- 后端随四档 acceptance；管理页走 ui CI + E2E + 场景走查。
- **API 文档面口径（v3 审计 Y-5 登记）**：自服务端点（`GET/POST /me/notification-optouts`——登录态用户自查自设偏好）**不进 Swagger**——Swagger 面向管理页/前端契约消费，自服务端点无页面消费者且豁免 menu_apis（SelfService 组直挂），补注解属伪需求；后续若通知中心独立页面消费再补。
- 场景标尺：状态流转 → 模板渲染（占位符替换 + 转义——**注入负向必测**：字段值含 `{{`/HTML 不被执行、markdown 渠道不渲染为链接）→ 收件人解析 → 桩发送 → message_log 落行；异步投递（发送失败不回滚工单事务）；同单连发节流生效；未配置渠道生产态 fail-fast 报"未配置"（**虚标防线负向**）。
- phase6 前向验收：事件面多订阅者挂载演示（第二订阅者零改 ticket service）。

## 11. 文档衔接

| 文档 | 关系 |
|------|------|
| [phase3/10 §3](../phase3/10-ticket-business.md) | 对接参考（邮件矩阵/事件订阅/L1 消费——选 b 时的蓝本）；「主管」定义/邮件矩阵随批取用 |
| [phase4/02 P4-2 行](../phase4/02-implementation-plan.md) | 本体现状与触发源注意（processed 列缺口） |
| [phase6/README §3](../phase6/README.md) | 对 phase5 三要求之触发域无关/共享事件面（本段已回写） |
| [design-decisions §27/§29 + 10-13 翻案条](../design/design-decisions.md) | 立项/事件面定论/站内翻案登记 |
| [10-s10](./10-s10-ticket-collaboration.md) | 角标端点 sections（二期未读数并入）；超时通知消费方 |

---

## 附录 A：详细技术设计（实施级，2026-10-16 补充——样板深度对齐 05 号附录 A）

> ⚠ 标注 = 实施级设计建议（启动批确认制，不新增拍板点）。事件面接口 = S3a 对表钉死形态（[03 号 §3 头注](./03-s3-flow-semantics.md)、11 号 §2.4）——**命名已拍（2026-10-16 拍板点 4）：通用 `DomainEvent`/`DomainHook`**（原 TicketEvent 系措辞废止；消费方侧名字保持业务化）。

### A.1 事件面基建（B6-1，✅ 命名已拍 DomainEvent/DomainHook）

```go
// internal/pkg/domainevent/hooks.go（编译期注册，app.go 装配——跨域公共件，ticket service 为首个发布方）
type DomainEvent struct {
    BizType    string          // ticket / risk / …（订阅按 biz_type 路由——域无关路由键）
    Action     string          // ticket.created / ticket.status_changed / ticket.assigned / ticket.cc_added / …
    BizID      int64; RefCode string   // biz_id=业务主键；RefCode=业务编码（工单 type_code 等）
    From, To   string          // status_changed 专用
    ActorID    int64; OccurredAt time.Time
    Payload    json.RawMessage
}
type DomainHook interface { OnDomainEvent(ctx context.Context, ev DomainEvent) }
type HookRegistry struct { subs map[string]DomainHook }        // 有序（注册序）
func (r *HookRegistry) Register(name string, h DomainHook)     // 重复名 panic（装配期暴露）
func (r *HookRegistry) Dispatch(ev DomainEvent)                // 每订阅者独立 goroutine + recover；at-most-once
func (r *HookRegistry) Drain(ctx context.Context) error         // 优雅退出排空在途订阅者（2026-10-26——app.Shutdown 5s 预算；丢失窗口收窄为仅崩溃）
```

- **派发时机 = DB 事务提交后**：ticket service 状态变更路径在 `tx.Commit()` 成功后调 `Dispatch`（事务内只收集事件，提交失败零派发）——异步投递语义（§5.2）的实现锚点；
- **事件枚举补 cc_added（2026-10-16 评审批）**：抄送「收通知」依赖事件可订阅——S10 抄送落库时 emit `ticket.cc_added`（payload 含 cc 用户/role），S6 message_rules 即可订阅抄送通知；**无此事件=抄送通知须硬编码进 cc_service（违 §5.2 禁令）**——枚举随 B10-1 同批钉入；
- **域无关兑现**：DomainEvent 无工单专属字段渗透到订阅者契约（Payload 透传），phase6 R3 关单钩子/任务链按同名接口挂载，**第二订阅者零改 ticket service**（§10 phase6 前向验收）；**命名已拍（拍板点 4，2026-10-16）**：通用 `DomainEvent`/`DomainHook`——原 TicketEvent 系措辞废止（结构不变，本节已按新名改写）；
- **节流/去重落订阅者侧**（S6 消费者自责）：同 ticket_id+Action 窗口 60s 内去重（内存 sliding window = **单实例假设**；**多副本部署时 Redis 化**，登记触发 = 副本数 >1，对齐 [phase3/02](../phase3/02-multi-instance.md) 多实例纪律）。

**事件分类学约定（2026-10-20 规划研究回灌，依据 deliverables/software-company/secops-platform-vision-2026-10/08-phase5-adjustments.md，D20）**：`Action = <biz_type>.<verb>`，verb 取固定词表——实体类 created / updated / status_changed / assigned / commented / closed / reopened / deleted；执行类（run）started / succeeded / failed / canceled / dead / timed_out；系统类 ops.*。`updated` 事件 Payload 必带 `changes: [{field, from, to}]`。每个 Action 预留 payload JSON Schema 登记位（第一档可为空）。零代码约定，防 phase7 运行类事件接入时重命名。

### A.2 表族 DDL（迁移全文级，编号启动对账）

```sql
CREATE TABLE message_templates (
    id         BIGSERIAL PRIMARY KEY,
    code       VARCHAR(50) NOT NULL,             -- 业务引用键（规则/动作库引用）
    name       VARCHAR(100) NOT NULL,
    channel    VARCHAR(20) NOT NULL DEFAULT 'uip',
    subject    VARCHAR(200) NOT NULL,
    body       TEXT NOT NULL,                    -- {{ticket.assignee_name}} 点路径占位符（logic-less，A.4）
    variables  JSONB NOT NULL DEFAULT '[]',      -- 编辑器从变量字典回填的引用清单（展示/审计；渲染校验=引用⊆字典，A.4）
    is_system  BOOLEAN DEFAULT FALSE,            -- 种子模板（编号+标题+链接），防越 L2 泄露准则（§4）
    version    BIGINT NOT NULL DEFAULT 1,        -- 乐观锁（管理面并发编辑——对齐 notification_configs 000033 全仓惯例；CAS 更新携 version 命中才写，0 行=409）
    created_by BIGINT NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE (code)
);

CREATE TABLE message_log (
    id           BIGSERIAL PRIMARY KEY,
    template_code VARCHAR(50),
    channel      VARCHAR(20) NOT NULL,
    recipient    VARCHAR(200) NOT NULL,          -- 平台外标识（邮箱/工号/IM id）
    biz_type     VARCHAR(30) NOT NULL DEFAULT 'ticket',
    biz_id       BIGINT,                         -- ⚠ 域无关：biz_type+biz_id 而非 ticket_id（phase6 risk 通知零改复用）
    subject      VARCHAR(200),
    status       VARCHAR(20) NOT NULL,           -- sent / failed / dead（dead 接既有死信告警链）
    error        TEXT,
    retry_count  INT DEFAULT 0,
    created_at   TIMESTAMPTZ DEFAULT NOW(),
    sent_at      TIMESTAMPTZ
);
CREATE INDEX idx_message_log_biz ON message_log (biz_type, biz_id, created_at);
CREATE INDEX idx_message_log_status ON message_log (status, created_at);

-- 二期（B6-7）：
CREATE TABLE notifications (
    id           BIGSERIAL PRIMARY KEY,
    recipient_id BIGINT NOT NULL REFERENCES users(id),
    biz_type     VARCHAR(30) NOT NULL,
    biz_id       BIGINT,
    payload      JSONB NOT NULL,                 -- 深链（页面路由+参数）+ 摘要
    read_at      TIMESTAMPTZ,                    -- 行级已读（第一档）
    created_at   TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX idx_notifications_unread ON notifications (recipient_id, read_at);
```

### A.3 ChannelSender 端口与两态桩（B6-2）

```go
// internal/pkg/msgchannel/（端口）+ uip/（adapter）
//   定位注（2026-10-16 模块划分审计）：单消费方（notification service）放 pkg/ =
//   横切能力端口而非领域件（webhook/aksk 既有先例同款）——notification service
//   若升格 internal/service/notification/ 域目录，端口留 pkg 不随迁（adapter 语义稳定）
type OutboundMessage struct { Channel, Recipient, Subject, Body string; Meta map[string]string }
type ChannelSender interface { Send(ctx context.Context, msg OutboundMessage) SendResult }
```

- **按 channel 路由的双 adapter 并存**：`webhook` → 既有 P4-2 client（死信告警链不动）；`uip` → 新 adapter（HTTP+JSON 骨架：endpoint/鉴权头 config `notification.uip.*`，**字段映射独立函数** `mapToUIP()`——真实契约到手只改此函数）；
- **两态桩**（§3 口径落地）：~~`notification.uip.stub=true`~~ **StubMode 由 `server.mode != "release"` 推导，无独立 stub 配置键**（26b238e 实现形态——v3 审计 Y-14 勘误：原描述的 stub 键先在漂移）→ 开发态桩（返回成功 + slog 落 msg 摘要，全链 E2E 可跑）；release 态且 `notification.uip.endpoint` 未配置 → `ErrNotificationNotConfigured`（92004）fail-fast（**虚标防线负向必测** §10；607a2a9 起 port.go 真发射 92004 码对象）；
- 发送重试：failed 进既有死信队列（复用 P4-2 链），dead 告警——不新建重试体系。

### A.4 模板渲染器（B6-3）

```go
func Render(tpl string, vars map[string]string, channel string) (string, error)
```

- 单遍正则替换 `{{ticket.assignee_name}}` **点路径占位符**（2026-10-16 升级：原平铺 {{key}} → 点路径——Mustache/Handlebars logic-less 思想，**if/loop/函数执行不进模板** = 业界对用户可编辑模板的安全红线；vars = 调用方从 context 树扁平化的点键 map，实现仍单遍替换；**不引库**——Handlebars-Go raymond 停更/osteele-liquid 过重（2026-10 核实），点路径解析自写 ≤0.5 天）；**禁二次展开**——字段值含 `{{` 不再解析，注入负向 §10；**值转义按渠道进行（三族）**：HTML 渠道 `html.EscapeString`、markdown 渠道转义值中链接/加粗元字符（`[]()*_` 等——防 `[点我](http://evil)` 被渲染成链接的内容注入/钓鱼面：收件人看到的是"系统发的"）、纯文本渠道原样；
- **变量字典 = 服务端 SSOT（2026-10-16 新增）**：按 `(biz_type, event)` 供给可用变量树（键+人读标签，端点=A.6）——工单域=内置字段（human_key/标题/状态/负责人/创建人/链接）+ S1 `field_meta` 动态字段（**与 S1 字段元数据同源**，字段改 key 失效告警同件 §4；**字典 path=派生 slug**——`ticket.fields.f_<字段ID>`：field_key 无字符集约束[代码实证 `validateFieldInputs` 不限 key 字符集，中文/含点键打断点路径]，模板只许引用字典 path，picker 选择即插入为主 UX，vars 扁平化按 field_meta 映射 slug 键；S1 登记新字段 key ASCII slug 软约束不回改存量——2026-10-16 评审批 P1-1）+ 时间类；校验两道=规则保存时**模板引用 ⊆ 字典**拦截 + 渲染时 vars 缺**内置**引用键报错拒发（防静默空串；动态字段缺失分档处置见下方粒度错配注）；`variables` 列保留=编辑器回填的引用清单（展示/审计）；picker 数据源=A.8.1 变量选择器/A.8.2 字段引用下拉；phase6 risk 域照此自供字典（域无关兑现）；**与 S1 expr 求值器分界**=通知占位符零执行 vs 计算字段受控执行，两件勿混；**条件段落**（"有截止日期才显示某段"——Liquid `{% if %}` 形态）登记触发不预建；**⚠ 粒度错配注（2026-10-16 外审发现，方案 a 采纳）**：模板**跨类型共享**（A.8.2 选择器只按渠道过滤）× 动态字段**类型私有**（field_meta per ticket_type）——模板引用类型 A 私有字段 → 规则绑类型 B → 渲染缺键按"拒发"会**断掉类型 B 全部通知**。处置：**渲染缺键分两类**——内置键缺失 = fail（模板 bug，保持拒发）；**动态字段缺失 = skip 该占位符 + message_log 警告**（类型错配可运维发现）——零 schema 变更，B6-3 实施落；**第四象限补齐（键存在但该单无值——存量单无新字段值/内置空值如未指派）= 空串渲染 + message_log 标 warned 不拒发**（数据常态≠配置错误，拒发会系统性丢通知；正文空串≠收件人跳过[A.8.2]，两处分别断言）——四象限至此完整：内置缺失 fail / 动态缺失 skip+警告 / 键在无值 空串+warned / 正常渲染（2026-10-16 评审批 P1-2 后半）；
- 时间类变量：`{{time.now}}`/`{{time.due_in}}` 由调用方注入渲染前 vars（格式 config 化，§4）——**点路径命名空间统一**（2026-10-16 外审对齐：原平铺 {{now}}/{{due_in}} 与点路径约定不一致）；
- ~~**⚠ 实施差距注（2026-10-23 外审实例批钉账）**~~ **✅ 已实施（c309ca7 B6-3 增强批）**：RenderQuadrant 四象限三分档落地（内置键缺失=fail 拒发[hook 检查 subjR/bodyR.Err] / 动态键缺失=skip+warn / 键在无值=空串+warn）——6 子测覆盖。
- 种子模板随迁移 INSERT（`is_system=true`，仅编号+标题+链接三变量——**人读编号拍板点 2 联动**：不随批则种子模板链接-only）。
- ~~**实施注（2026-10-23）**：种子模板当前无 is_system 只读标识列~~ **✅ 已实施（000058 迁移+c309ca7）**：is_system 列+种子三行标 true+Delete 守卫（Update 拦截面审计批补）；渲染转义 = **渠道→转义族映射**（`channelEscapes`：uip/webhook=纯文本不转义[UIP 契约到手按实际正文格式重映射此表]、email_html 及未知渠道=HTML 转义 fail-safe）。**H-2 前瞻（2026-10-27 已拍）**：dingtalk 第三族（markdown 不转义）+独立 adapter 已立项——公司钉钉为封装 API，`ChannelSender` 端口即通用渠道接口，钉钉/飞书/企微各自实现；UIP 契约门控不阻 dingtalk（台账 H 组）。+ **值级转义**（仅转义替换进正文的变量值，模板自身文本原样）。

### A.5 订阅规则与触发链路（B6-5）

- **B6-4 收件人来源增补（2026-10-23 对话设计批——升级催办场景依赖；2026-10-23 批 3 对表 F-3-4/F-3-5 解析规则拍死）**：第一档 `field` 引用（assignee/creator）之外增补 **`supervisor`** 来源（`recipient_type` 第四枚举值——下条列式口径）。**解析规则拍死三句**：①组织选取 = `user_orgs.is_primary=true` 的组织，无则 `joined_at` 最早一条，**多组织不广播**（user_orgs 多对多且 is_primary 无唯一约束——000001:115-121，不定义即实施期各写一套）；②收件人 = 该组织内持有**拍定 org 管理角色码**的用户（`user_orgs ⋈ user_roles ⋈ roles` 两跳；角色码 **B10-4 前拍**——现角色体系若无 org_admin/org_owner 则种子）；③**不沿部门 ltree 继承**（000012:36 口径——上级部门管理员收不到下级升级通知，边界显式接受）。~~「org_roles 表既有」~~ 勘误：`org_roles(org_id, role_id)` = 组织↔角色绑定、**不含人**（000012:30-35），取人须两跳非单表直查。实现 = 解析器第四分支（org 查询+缓存），不新增表（成本假设仍成立）。**目标态登记（挂 M-HR 前置）**：业界"上级"从不由组织成员关系推导（ServiceNow `user.manager` / Jira SM 目录 manager 属性 / Workday 显式汇报线——组织管理员是权限角色非汇报关系）；`users.manager_id` 显式汇报线随 HR 同步回灌后，supervisor 解析改 **`manager_id` 优先、org 管理员推导降 fallback**——防 B10-4 把推导误作终局。
**⚠ 2026-10-24 所有者拍板修订（B10-4 开工前——dd §31 通则落实例）**：解析顺序终局 = **工单级人工指定（`tickets.escalation_supervisor_id`——Create/Assign 可填可改）> `users.manager_id`（M-HR 后，人工未指定时）> 缺席（不推导不广播）**。上列三句中的"org 管理角色码推导"档**作废**（台账 E-1 建议值销账——组织管理员是权限角色非汇报关系，10-23 目标态注"防把推导误作终局"本次直接落终局）；通则 =「人工指定为基线，HR 直属上级为进阶（人工优先）」——后续所有"找上级/找责任人"场景同构适用（dd §31）。
- **订阅配置表（⚠ 建议新增轻表 `message_rules`，启动批设计）**：`(id, type_code, event, template_code, recipients_spec JSONB, enabled, version 乐观锁——对齐 notification_configs CAS 全仓惯例)`——收件人 spec 第一档 = `[{"source":"field","key":"assignee"},{"source":"fixed","users":[…]}]`（§5.1）；**实现形态注（2026-10-23 批 3 对表 F-3-8——承认列式）**：实落为 `recipient_type VARCHAR(32)` 枚举（assignee/creator/fixed，supervisor 作第四值）+ `fixed_recipients TEXT`（000054:30-31）——单来源规则枚举列够用（业界惯例：spec JSONB 在「一条规则多来源组合」时才值回票价）；spec 模型**登记触发驱动**（出现 ≥2 来源组合需求时再上），本条与 11 号 B6-4 行以列式为实现口径、spec 保留为演进形态；**域无关**：type_code 换 biz_type 即 phase6 复用（rules 表天然域无关；列名落 DDL 时取 `biz_type` 与 message_log 对齐——工单域非工单事件[如 ops.cron_failed 类]订阅键届时随二期/phase6 需求写死，2026-10-20 评审回声批注）；
- **引用完整性双向守卫（三层，2026-10-16 评审批采纳）**：①**模板保存**——引用 ⊆ 所绑 rules 的 (biz_type,event) 字典并集，越界拦截/告警（原只有删除守卫——改正文静默破坏既有规则要到下次触发才以 failed 暴露；与 S1 影响分析同件）；②**规则保存**——模板动态字段引用 ⊆ 该 type_code 的 field_meta ∪ 内置 ∪ 时间类，**软告警不拦截**（field_meta 类型私有，(biz_type,event) 粒度不足以表达类型维度——粗校验①兜底+细告警②提示）；③**渲染时四象限**（A.4，运行时兜底）；守卫拦截/告警与模板·规则管理操作审计留痕随批（对齐 settings PUT 变更审计先例——11 号 B6-3 行同源）；
- **触发链**：HookRegistry.Dispatch → TicketNotifyHook（查 message_rules 命中）→ 渲染（A.4）→ 收件人解析 → ChannelSender.Send（异步 goroutine）→ message_log 落行；发送失败不回滚工单事务（事务早已提交，结构上不可能回滚——emit-after-commit 的免费收益）；**emit 单点锚=统一转换执行入口 commit-后回调（2026-10-19 P0-2 修正，原 2026-10-16 评审批表述作废）**：原表述「Dispatch 挂接锚定状态机咽喉 AssertTransition 单点」**技术上不成立**——AssertTransition=tx.Begin 前纯校验（service.go:352 vs :357 实证），作锚=事务前派发（回滚也发通知）且 created/cc_added 不经它；修正后锚=B3-1 首件 `POST /tickets/transition` 的 commit-后回调（单一咽喉覆盖全路径的**意图保留**，挂点从事务前移到事务后）；**过渡态显式化（2026-10-19 三轮回声）**：B6-5（W5–7.5）远早于 B3-1（W13–15）——S6 一期收口前 status 类事件 emit=Close/Assign 服务层 commit 后**显式 emit 两处**（拍板点① a 案本意），B3-1 端点收敛后归一、临时代码随收敛删除；**非 transition 事件 emit 点枚举钉死（2026-10-19 二轮回声补）**：transition 类事件统一走执行入口回调；`ticket.created`=Create 服务 commit 后、`ticket.cc_added`=B10-1 抄送落库 commit 后各自显式 emit（两处不经转换咽喉——防实施期「挂哪」开放问题）；**事件常量归属与命名映射（P1-21 采纳，2026-10-19）**：DomainEvent Action 常量单一来源=`internal/pkg/domainevent`（repository 层可引——事件名非服务私有）；`ticket_events.action`（bare 值审计惯例，service 包 Event* 常量随迁上移）与 DomainEvent.Action（点前缀订阅键）映射函数同包收口——顺带消灭 repository 层事件名字面量（ticket_repo:345 `'deleted'` 即此类，根因=常量居 service 包 repository 不可 import）；
- 人读编号（拍板点 2 随批）：`tickets` 加 `human_key VARCHAR(30) UNIQUE`，Create 时 `TK-yyyymmdd-NNNN`（按日序列，UNIQUE 冲突重试 ≤3）；存量单 NULL——模板变量解析时 NULL 回退展示 id（`#123`）。**✅ 已落（2026-10-23 f055c60，迁移 000057 + `internal/pkg/humankey` 前缀注册表生成器——phase6 RISK- 同件复用）**。
- **收件人解析实施注（2026-10-23 拍板 A 落地）**：`resolveRecipientsFromRule` **消费 message_rules 的 recipient_type/fixed_recipients 列**（assignee→payload.to_assignee / creator→payload.actor / fixed→DDL 列表）——修复前旁路 DDL 硬编码 switch（收件人列为装饰性）；**created 删硬编码"不送自通知"特判**（原实现擅自发明产品决策且使 created 种子模板成死场景）——走规则驱动，种子规则不含 created=开箱零通知（06 号 §0 原则），配 creator/fixed 档规则后真实生效；**未指派单关闭**=assignee 档解析空→静默不通知（b65 未指派子测钉此行为）。

### A.6 API 面（通知中心页面族，自 S4 迁入）

| 方法/路径 | 权限 | 用途 |
|-----------|------|------|
| POST `/api/v1/notifications/templates`·`/templates/update`·`/templates/delete` + GET 列表（**2026-10-16 外审修正：原 PUT 违 §3-1 改 house style**——code 入 body） | 通知配置管理权限（对齐 notification_configs 既有码） | 模板 CRUD + 删除守卫（被 rules 引用 → 拦截，与 S1 影响分析同件） |
| POST `/api/v1/notifications/templates/preview` | 同上 | 模板干跑渲染（body+vars 样例 → 渲染结果，零落库——A.8.1 测试数据预览；复用 A.4 Render 零新渲染逻辑，2026-10-16 业界核验批新增设计行）**→ H-6 已立项**（2026-10-27 通知能力增强批，含变量选择器 picker 与前端预览面板；动态字段值注入 H-3③a 为预览出真值的前置） |
| GET `/api/v1/notifications/variables`（query: biz_type+event） | 同上 | **变量字典树**（键+人读标签——A.8.1 变量选择器/A.8.2 字段引用下拉数据源；工单域与 S1 field_meta 同源；phase6 risk 自供同款，2026-10-16 新增设计行）；**查询粒度（评审批 P2-1）**：模板页无 event 上下文→按 biz_type 取各 event 并集（超集）展示；规则保存按精确 (biz_type,event)+类型维度（动态字段 ⊆ 该 type_code field_meta，软告警——A.5 守卫②） |
| GET `/api/v1/notifications/logs` | 同上 | message_log 列表（PageData §3-6） |
| GET/POST `/api/v1/notifications/rules`、启停 | 同上 | 订阅规则管理 |

二期（B6-7）：`GET /api/v1/notifications/inbox`（PageData）+ `POST /read`（单条/全部）+ 未读数并入 S10 角标 sections。

### A.7 验收补充（在 §10 基础上）

- 事件面：第二订阅者挂载演示（phase6 前向验收的实现路径——mock hook 验证）；订阅者 panic 不影响主链与其他订阅者（recover 负向）；事务回滚零派发；
- 节流：同单 60s 内连续翻转 → 仅首发通知落 message_log；
- 渲染注入与字典校验负向：字段值含 `{{evt}}`/`<script>`/`[x](http://evil)` → 单遍替换不执行 + HTML 渠道转义断言 + markdown 渠道元字符转义后不渲染为链接（A.4 三族，2026-10-16 扩）；模板引用字典外变量 → 规则保存时拦截 + 渲染缺键**分档断言**（A.4 粒度错配注两档）：**内置键缺失 = 拒发**（模板 bug）/ **动态字段缺失 = skip 占位符 + message_log 警告**（类型错配，不判失败）——"不静默空串"仅指内置档（变量字典批 2026-10-16；分档对齐 2026-10-16 外审尾巴修复）；
- 缺值与守卫负向（2026-10-16 评审批）：①模板引用的动态字段在工单 custom_data 无值 → 空串渲染 + log warned + 状态非 failed（不拒发——与「键缺失 skip」分别断言）；②收件人字段空 → 跳过该收件人（A.8.2 语义），与①独立用例；③模板追加某规则字典外占位符 → 保存拦截（改守卫——删除守卫已有）；④field_key 含中文/点的字段 → 字典供 `ticket.fields.f_<ID>` slug path 渲染正常（P1-1 字符集隔离）；
- 两态桩：stub=true 全链绿 / stub=false 未配置 → 10007 类"未配置"错误码（errcode 随批登记）——**虚标防线**；
- 域无关：mock risk 通知（biz_type=risk）走全链成功，ticket 表零改动。

### A.8 通知中心页面族 UI 详设（实施级，2026-10-16 补——此前页面级细节仅有归属句，四页 + 铃铛对表业界落定）

> **文档归属纪律（2026-10-16 确认）**：功能规格/页面设计 SSOT 集中主仓 `docs/`（先例 phase3/12-frontend、phase4/01-frontend-design——标题即"zhuzhao-ui 工程设计"亦坐主仓）；zhuzhao-ui 仓 docs 只放工程规范（frontend-standard/testing-standard），ui 仓只承载实现产物（组件/E2E spec/测试）。主仓管"页面该长什么样、为什么"，ui 仓管"实现成了什么样"。
> **业界对表（2026-10-16 核验）**：模板编辑=Novu 渠道级编辑器+云短信控制台变量约束；收件人=Jira Automation Send notification picker；发送记录=Novu activity feed；铃铛=GitLab To-dos/GitHub Notifications——均为通行交互，对表即可不重新发明。

**A.8.1 模板管理页（B6-3）**：列表（code/名称/渠道/is_system 只读标识/更新时间）+ 编辑器三件套——①**变量选择器**（吃 A.6 变量字典树：键+人读标签——2026-10-16 由手维护清单升级，选择即插入）；②占位符**点击插入**（选择器选中 → 光标处插点路径——动态字段为 slug path `ticket.fields.f_<ID>`[字典权威，键字符集隔离]，防手输打错被校验拒发）；③**测试数据预览**=新增 `POST /templates/preview` 干跑 A.4 Render（body+vars 样例 → 渲染结果，零落库——业界标配里本设计原唯一缺件，A.6 已同步加行）。种子模板只读并展示准则出处（编号+标题+链接防越 L2，§4）。

**A.8.2 订阅规则页（B6-6）**：列表（type_code/event/模板/收件人摘要/启停）；编辑器=type_code+event 下拉（event=DomainEvent Action 枚举）→ 模板选择器（按渠道过滤）→ **收件人 picker**（对表 Jira Automation）：字段引用下拉（候选=变量字典，§5.1 第一档）+ 固定人员远程搜索选人（复用既有 el-select 用户搜索）；spec 结构同 A.5。**fallback 语义（业界缺口，编辑器须透出）**：字段引用空值 → 第一档默认**跳过该收件人**（落 message_log 不发不报错）；回退语义（assignee 空发 reporter——Jira smart value 默认值形态）=二档按需，第一档 spec 无 fallback 字段。

**A.8.3 发送记录页（B6-6）**：筛选维度对表 Novu activity（渠道/状态 sent·failed·dead/时间范围/模板 code/biz_type）+ PageData（§3-6）+ **单条详情面板**（错误全文/重试次数/渲染后 subject——排障主入口）；dead 行醒标并链死信告警语境；**失败聚合视角**（按 template_code+error 聚合——系统性配置错误[一个模板坏了断一片]一眼定位免逐行翻）；failed 行「重发」动作=**触发驱动登记**（新建 log 行不覆盖原行——首次真实需要或 B6-6 顺手，评审批 P3）。

**A.8.4 渠道配置页（type=uip，B6-2）**：继承 P4-2 notification_configs 既有页面骨架扩展（endpoint/鉴权头表单区）——**不单独详设**；两态桩状态管理面透出（stub=true 徽标——虚标纪律的页面锚）。

**A.8.5 铃铛双 tab（二期 B6-7）**：语义分离已定（§6：待办=状态派生/消息=事件快照）——对表 GitLab To-dos（已办自动消失、无已读概念）/GitHub Notifications（读过即完）；补交互：消息行=摘要+时间+深链（payload 路由已定），单条点击即读 + 全部已读按钮（read_at 行级第一档）；待办行=派生自工单状态；未读数并入 S10 角标 sections（已定不重复做）。

---

## 变更记录

| 日期 | 变更 |
|---|---|
| 2026-10-15 | 建档（phase5 分支十段设计稿批）——现状锚点逐段代码核实（P4-2 本体/事件面缺口）；含二期站内消息中心（2026-10-13 翻案）；未新增决策 |
| 2026-10-15 | 公约对表批：§5.4 message_log 管理页列表补 PageData 形态注（standards §3-6） |
| 2026-10-16 | **详细技术设计批（附录 A，实施级——05 号样板深度）**：A.1 StatusChangeHook 接口全文（TicketEvent 结构/编译期注册/after-commit 派发=异步投递实现锚点/recover 隔离/节流落订阅者侧）——即 S3a 对表钉死形态/A.2 三表族 DDL 全文（**message_log 用 biz_type+biz_id 域无关键**——phase6 risk 零改复用；notifications 二期表随落）/A.3 ChannelSender 端口+双 adapter 并存（webhook 既有链不动）+两态桩实现（stub config 开关+ErrNotificationNotConfigured 虚标防线）/A.4 渲染器契约（单遍替换禁二次展开+变量清单校验拒发）/A.5 订阅规则轻表 message_rules 建议（域无关 recipients spec）+触发链装配+人读编号实现（human_key 列+UNIQUE 冲突重试）/A.6 API 面/A.7 验收补充（第二订阅者/panic 隔离/节流/注入/两态桩/域无关六负向） |
| 2026-10-16 | **第一批外审回声批**：**拍板点 +4（事件面命名）**——A.1 TicketEvent 系以 Ticket 命名与域无关主张错位，通用命名候选 DomainEvent/BizHook（B6-1 前钉死，S3a/phase6 共享枢纽）；§4/§9 迁移计数对齐（两表族→+message_rules+human_key ALTER ≥3 段）；§5.3 生成器抽象登记（internal/pkg/humankey 跨域公共件——phase6 RISK- 零改复用） |
| 2026-10-16 | **第二批外审回声批（公约硬冲突）**：A.6 模板端点 GET/PUT → POST house style（`/templates`·`/update`·`/delete`，code 入 body——§3-1 首批应用例自指冲突修正）；A.1 补**节流单实例假设注**（多副本 Redis 化，触发=副本数>1） |
| 2026-10-16 | **事件面命名拍定批（所有者）**：✅ 拍板点 4 = **DomainEvent/DomainHook**（包 internal/pkg/domainevent；biz_type 路由；HookRegistry 不变；消费方侧名字业务化）——A.1 全节按新名改写（原 StatusChangeHook/TicketEvent/TicketHook 措辞废止）；03 号两处同步 |
| 2026-10-16 | **全计划业界对标批（所有者）**：§5.2 补**用户通知偏好**（业界通知系统 v1 必备：per-user opt-out/免打扰，发送链前置过滤；随 B6-4，+0.5–1 天——缺了=骚扰且无法自关） |
| 2026-10-16 | **业界核验回标批（所有者问询三线：通道模式/页面 UI/富排版边界）**：①§3 补**直连差异清单**（企微/钉钉/飞书同为 HTTP+JSON 推送与 UIP adapter 同构——直连再拍时拍什么：鉴权两族/收件人 ID 映射/渠道限频，均 adapter 内部消化端口不动）②**附录 A.8 新增**（通知中心页面族 UI 详设五小节：模板编辑器三件套[变量 tag/点击插入/preview 干跑端点——A.6 同步加行]/规则页 Jira 式收件人 picker+fallback 语义[第一档=字段空跳过]/日志页筛选维度/渠道页继承 P4-2 不单独详设/铃铛交互细节；文档归属注=设计 SSOT 集中主仓，ui 仓只放工程规范与实现）③§8 边界行校准（不做的是富排版引擎/可视化编辑器/完整模板语言——正文格式按渠道 adapter 归属：纯文本/轻 HTML/markdown 子集，业界无统一 markdown——钉钉无表格/企微双方言/飞书私有 JSON）④A.4 转义纪律扩三族（+markdown 渠道值转义——防字段值 `[x](http://evil)` 渲染成链接的内容注入面）⑤§10/A.7 注入负向补 markdown 渠道断言（11 号「六负向」计数不变——并入既有注入条目） |
| 2026-10-16 | **模板语法与变量字典批（所有者「补，然后看看是否有详细计划」）**：占位符 {{key}} 平铺 → **点路径 logic-less**（{{ticket.assignee_name}}——Mustache/Handlebars 思想零执行语义；禁二次展开/渠道转义纪律不变；不引库[raymond 停更/osteele-liquid 过重 2026-10 核实]，点路径解析自写 ≤0.5 天）+ **变量字典=服务端 SSOT**（按 biz_type+event 供给变量树：工单域=内置字段+S1 field_meta 同源+时间类——校验反转=模板引用⊆字典+渲染缺键拒发；variables 列退为编辑器回填展示；端点=A.6 新行 GET /notifications/variables；picker 数据源=A.8.1/A.8.2；phase6 risk 自供字典=域无关兑现）+ 条件段落（Liquid {% if %} 形态）登记触发不预建 + 与 S1 expr 求值器分界注；§4 渲染句/A.2 variables 列注/A.7 字典校验负向/A.8.1·A.8.2/11 号 B6-3·B6-4 行/README §2·选型表·§3 边界同步（+0.5–1 天一期上缘含，与 B6-4 通知偏好合计 +1–2 天总账对账留实施批） |
| 2026-10-16 | **渲染缺键分档对齐批（外审尾巴修复）**：A.4 校验两道句 + A.7 字典校验负向按粒度错配注两档改写（**内置键缺失=拒发 / 动态字段缺失=skip+警告**——原统一"缺键拒发"表述与已落分档矛盾，验收测试照旧句会写错方向）——四象限之"键存在无值"两态（全景计划 P1-2）另批待采纳 |
| 2026-10-16 | **评审采纳批（P1×3+P2×2+P3×2+深审 7 项并批，所有者「落」）**：①A.4 **slug 字典供给**（`ticket.fields.f_<ID>`——field_key 无字符集约束代码实锤 `validateFieldInputs`，中文/含点键打断点路径；字典权威供 path+picker 即插+vars 按 field_meta 映射；S1 登记 ASCII slug 软约束）+ **四象限齐**（+键存在无值=空串渲染+warned 不拒发——存量单/未指派防系统性丢通知；与 A.8.2 收件人跳过区分断言）②A.5 **引用完整性双向守卫三层**（模板保存字典并集拦截[改守卫补齐——原只删守卫]/规则保存类型精确软告警/渲染四象限兜底）+ **emit 单点锚=AssertTransition**（状态机咽喉一处挂接全路径覆盖，多入口各自挂=漏发）③A.1 枚举补 **ticket.cc_added**（S10 抄送通知订阅依赖——无此事件须硬编码进 cc_service 违 §5.2 禁令；10 号 §3.1 同步，随 B10-1 钉入）④A.6 字典端点查询粒度句（模板页并集超集/规则侧类型精确）⑤A.7 负向 +4（空串渲染/收件人跳过独立/模板改守卫/slug 隔离）⑥A.8.1 slug 即插注/A.8.3 失败聚合视角+重发触发驱动登记——**S6 侧量级 +1–1.5 天一期上缘含（11 号 B6-3 同批，累计 +2–3 天总账对账留实施批）；深审 7 项分落 05 号（ETag/仅 200 审计+演练联合场景）/09 号（404 入缓存）/README（digest 挂账）** |
| 2026-10-16 | **守卫审计留痕补锚批（一致性尾巴修复）**：A.5 双向守卫补「守卫拦截/告警与模板·规则管理操作审计留痕」（对齐 settings PUT 变更审计先例——11 号 B6-3 行的 06 号侧锚点） |
| 2026-10-19 | **第二意见体检回声批（并行外审报告独立复核属实——本级 4 处）**：①A.2 `message_templates` DDL 补 **`version` 乐观锁列**（对齐 notification_configs 000033 全仓管理面惯例——CAS 更新携 version 命中才写 0 行=409；报告 A-1）②A.5 `message_rules` spec 同步补 version ③§5.4 补**死信接缝设计**（Send 终败=进程内直调 notification service 死信告警方法[与 /internal/notify/dead-letter handler 共用内部函数，不自绕 HTTP]+DeadLetterNotifyPayload 扩展可选 biz_type/biz_id/template_code 指针字段 taskrunner 零破坏——报告 A-2 实施卡点）④头注「与 S2 并行」→「错峰衔接（S6 先——单人串行）」（报告 B-6 镜像侧）⑤§5.2 用户通知偏好补「若起轻表回灌 11 号 §5.9 B6 行迁移计数」（报告 B-9 条件回灌） |
| 2026-10-19 | **P0-2 锚点修正批+P1-21（并行外审合并报告）**：①A.5 触发链 emit 锚改「**统一转换执行入口 commit-后回调**」（原「AssertTransition 单点」技术不成立——tx 前纯校验，作锚=回滚也发且 created/cc 不经它；service.go:352 vs :357 实证）②A.5 补**事件常量归属与命名映射**（P1-21：domainevent 包单一来源，ticket_events bare 值↔DomainEvent 前缀键映射同包收口——消灭 repository 层字面量） |
| 2026-10-19 | **P0 修正批二轮回声**：A.5 补非 transition 事件 emit 点枚举——`ticket.created`=Create 服务 commit 后、`ticket.cc_added`=B10-1 抄送落库 commit 后各自显式 emit（两处不经转换咽喉，防实施期「挂哪」开放问题——上批锚修正只覆盖 transition 类事件，created/cc 两类原悬空） |
| 2026-10-19 | **P0 修正批三轮回声（emit 锚过渡态显式化）**：A.5 锚句补过渡态——B6-5（W5–7.5）远早于 B3-1（W13–15），S6 一期收口前 status 类事件 emit=Close/Assign 服务层 commit 后**显式 emit 两处**（拍板点① a 案本意），B3-1 收敛后归一（消「一期验收时无 emit 点可走」的同族排期缝隙——写点倒挂[UpdateStatusTx 终版锚]的 emit 侧镜像，11 号 B6-5 行同步） |
| 2026-10-22 | **S6 一期实施收官批（B6-1→B6-6 全批代码面交付）**：B6-1 DomainEvent/DomainHook 事件面（internal/pkg/domainevent——biz_type 路由/goroutine+recover 隔离/at-most-once/事务提交后派发）→ B6-2 ChannelSender 端口+UIP 两态桩（internal/pkg/channelsender）→ B6-3 message_templates（000053，含种子模板三枚）+msgrender 点路径渲染器 → B6-5 触发挂接（TicketNotifyHook+Close emit 补全）→ B6-6 message_log/message_rules（000054 含种子规则）+管理 API 四端点（BK-22 绑定 000055）→ B6-4 notification_optouts（000056）+发送链偏好过滤；+同单连发节流（notifyThrottle 60s 窗口+并发安全）；四档 241/0+双仓 CI（52d0f23）。**收口后注**：acceptance 通知链路用例缺口与四象限/转义/收件人面欠账经外审全量审计登记（下两行收口） |
| 2026-10-23 | **外审全量审计修复批（P1×7 收口——实例场景驱动）**：①**收件人解析消费 rules**（resolveRecipientsFromRule 读 recipient_type/fixed_recipients 三档——修复前旁路 DDL 硬编码 switch，收件人列为装饰性，管理面配了不生效）②**拍板 A**：created 删"不送自通知"硬编码特判走规则驱动（原实现擅自发明产品决策+种子模板死场景；无 created 种子规则=开箱零通知原则不变）③**closed 链路静默丢失修复**：Close emit payload 补 to_assignee（外审实例实证：assignee 档恒空→关单通知 100% 丢失且 log 零行）+Create payload 补 actor（creator 档锚点）+两端钉测（832de93/ca61e63）④**转义落地**：RenderEscaped 渠道→转义族映射（uip/webhook=纯文本不转义[UIP 契约到手重映射]、email_html 及未知渠道=HTML fail-safe）+**值级转义**（escapeValues 只转变量值——模板自身文本原样；修复前零转义[注入面]+email_html 幻渠道名[转义对真实流量零效果]双问题）⑤**拍板 B 过渡**：message_log 列表 body>200 截断脱敏（越 L2 面；通知中心独立权限码+菜单重绑挂二期或 S10 前）⑥DeadLetterNotifyPayload 扩 biz_type/biz_id/template_code 指针字段+92004 ErrNotificationNotConfigured 登记（92003 stale 修正）⑦human_key 落地（000057+internal/pkg/humankey——§5.3 拍板点②随批件）+死信进程内直调接缝 ⑧**四象限差距钉账**：测试名实对齐（原名"四象限"实测"统一保留原样"与 A.4 设计语义相反）——内置缺失=fail 拒发档未实现，显式挂 **B6-3 增强批**（A.4 实施差距注）。验证=b65 四子测集成 -race 绿（新增 closed 全链回归锚点[自定义图 assigned→closed]+未指派静默两子测）+msgrender 转义/四象限子测+22 包单测+lint 0 |
| 2026-10-23 | **微批 A——审计残余收口（所有者「按这个方案来」）**：①**rules version 乐观锁**（A.5 设计原文欠账——迁移 000060 + UpsertRule CAS：陈旧 version→92506/409 不静默覆盖；四子测含变异锚）②**channel 错配运行时守卫**（遵 F-3-2 保留 uip/webhook 白名单——错配模板显式失败+log failed 替代静默错发；路由层落地后自然 no-op）③**notification.uip.endpoint config 段**（BindEnv NOTIFICATION_UIP_ENDPOINT+字面值+装配接线——生产态 fail-fast 92004 自此可达）④92004 漏映射补登记+92506 登记（errcode.go/errors.go/台账/全码表 73）⑤P3 清：resolveRecipients 死码兼容层删/Subscribe 重名防护（A.1「重复名 panic」落地+调用点具名）/errors.go 注释错位；**拍板登记**：binder 已由所有者销账（05 号 F-4-1 YAGNI）/variables 维持 SelfService 待通知中心权限码一并收紧/发送超时 ctx 挂 UIP 契约接入批。验证=rules CAS 与 b65 错配子测先红后绿+service/repository/ticket 三包集成 -race 绿+全量单测绿+lint 0+000060 往返零漂移 |
| 2026-10-24 | **B6-7 站内消息中心交付（后端 zhuzhao 8df6105 + 前端 ui d7f1254c）——S6 全段（一期+二期）收官**：①迁移 000072=notifications 表族（recipient/biz 深链/title+link **入站即渲染**[读面零渲染逻辑]/行级 read_at 第一档）+未读部分索引+message_center 菜单行（inbox:read 码）+三读面路由绑定；②InboxService（DomainHook **ticket/ops 双域订阅**——「消息中心=事件系统的消费者」定性落地）：事件→站内落库（收件人锚点 to_assignee∪cc_users 去重——S6 一期同锚点；词表 seedTemplates 十动作收敛**防全量事件灌 inbox**；入站渲染缺键保留原样）+读面三端点（inbox 分页倒序/单条已读幂等[非本人 404 同形]/全部已读批量）+**unread 聚合并入 badges sections**（RegisterUnread 注入——badge 计算器注册制零改协议兑现）+CleanupExpired 保留策略（90 天封顶骑 CronScheduler[cronJobFunc 适配器]+cron-status 可见）；③前端：消息中心页双 tab（通知=事件快照读过即完/待办=状态派生 assignee=me——GitLab To-dos vs GitHub Notifications **语义分离落地**[06 号 §6 业界地基]）+通知行摘要/时间/深链 link 直跳+未读红点+仅未读过滤+全部已读。**验证**：TestB67 四子测全绿（事件落库+cc 去重/未登记动作不落库/读面全链[分页倒序·单条已读·非本人 404·unread 过滤·全部已读]/保留策略清理）+集成两包+22 包单测+四档 247/0+往返 72 对零漂移；ui 四件绿（test 177——viewNames 守卫抓 intel_query 种子无落成页面→补情报查询页先红后绿；epImports 抓三缺 import）。**实施偏差两件**：①「通知=双 tab 合并入口」落在消息中心页内（铃铛=未读数徽标入口简化——双 tab 完整交互随页走查）；②SSE 实时通道未随批（分期口径[轮询→SSE→弹窗]既定，触发驱动）
| 2026-10-25 | **message_center 页可达性回补（E2E 补齐批核出）**：000072 建菜单行**未绑 role_menus**（000031 惯例漏项）→ 消息中心全员不可达、铃铛 goInbox 跳 /messages 命中 404 兜底页；**迁移 000074** 绑 admin+superadmin（operator/viewer 等非管理角色按现行模型由角色管理分配本页——是否默认全员可见属产品拍板，台账 G-23）；ui 侧 icon 裸名 `message` 补映射；`system/notification` 页 defineOptions 名 'SystemNotification'→'system_notification_page'（与路由名不符致 keep-alive 恒不命中——viewNames 派生器 UPDATE visible 盲区致该页三段未被检，同批修派生器） |
| 2026-10-25 | **G-23 实施——消息中心自服务化（E2E 补齐批三程）**：①后端 inbox/read/read-all 三端点由 biz 组（L1=inbox:read 菜单绑定）移 **selfService 组**（严格自数据：服务端限 recipient=self、MarkRead 非本人 404——与 /badges 同性质，不应随角色菜单绑定放行）+ catalogExempt 三条；②**迁移 000076** message_center 菜单行 **visible=false 惰性化**（不删行——防自增 ID 漂移触往返门禁；用户菜单树过滤不可见节点 → 不生成动态路由/侧栏项）+ 摘除三端点绑定 + 解绑 role_menus + 非管理角色残留 casbin 清除；③ui `/messages` **常量路由**（Layout 常驻页、ROUTE_EXEMPT_PATHS/NO_RESET_WHITE_LIST 白名单、name=message_center_page 供 keep-alive）。入口=铃铛（本批前对非绑定角色为 404 兜底页），侧栏「消息中心」项随之消失——与 000072「铃铛点入/消息中心页」原设计一致。`inbox:read` 码随菜单行惰性化失去唯一使用点（保留在行上，不另删）。验证=viewer 点铃铛可达 + inbox 200（p5-s6 尾段）；四档 246/0 + 往返 77 对 |
| 2026-10-25 | **四程：000078 删 message_center 菜单行 + inbox:read 退役**（G-23 收尾）：000076 只 visible=false 过于保守——管理端 GetTree 含隐藏节点，「角色管理→分配菜单」树里该行可见可勾、勾了无效；复核往返门禁语义（行在两次全量 up 终态均不存在=零漂移，000073 删叶子行先例）后补删。`inbox:read` 码随行退役（三端点已 selfService，无第二使用点）。ui 侧 viewNames 派生器补 DELETE 形态、icons 撤 `message` 裸名。消息中心自此为纯常量路由页：入口=铃铛，权限=身份作用域（登录即可），与菜单/角色体系完全解耦 |
| 2026-10-26 | **外审 v4 修复批（S6 域两条）**：①**P1 深链失效**（#1）：`notify_hook` 兜底 `link` 写的是 `/ticket/detail/{id}`，ui 路由实为 `/tickets/:id`——该处为全仓**唯一** link 产生点（所有 emit 点不带 link），000053/000062/000064/000071 六枚种子模板的 `{{ticket.link}}` 此前点击全部 404；`inboxLink` 同概念写法正确，一错一对并存两段；msgrender fixture 把错值固化成断言致测试永不发现——两处同修。②**站内信渲染/收件人**（#8）：`renderTitle` 改单次扫描（替换入的值不再被重新扫描——修前按 payload 逐键 ReplaceAll 且 map 随机序，标题含 `{{human_key}}` 字面可被二次展开，msgrender 有同款防护此处平行实现曾缺）；`ticket.updated/commented` 不再给 actor 自己落站内信（处理人编辑自己的单收「工单已更新」=噪音）——assigned/cc_added 保留 actor 可为收件人（TestB67「cc 收件人落库」既定语义）；**未动**：inbox 收件人路由接 message_rules、全局 actor 排除——属产品拍板，台账 G-29 与 G-7 并账。另 TestB67 两事件紧挨 Dispatch 的落库序 flaky 修为先落后发 |
| 2026-10-26 | **通知面收口批：模板/规则管理面编辑化 + notify 动作模板参数生效**：①通知配置页只读→编辑面（模板 CRUD：is_system 置灰、channel=uip 一期固定；规则 CRUD：事件词表下拉/模板下拉/收件人四档/fixed 数字校验，upsert 同键=改绑）——「多模板」对管理员成为真能力（B6 polish 欠项销账）。②notify 动作 params.template 生效：EmitActionNotify 透传 template_code → dispatcher 覆盖规则默认（**规则仍是开关与兜底**）；词表补 ticket.action_notify。③**迁移 000079** 种子模板 ticket_action_notify + 规则（cc 档）——**翻案**「无种子默认静默」登记（拍板：显式配动作而静默是坑；删规则即回静默）。E2E 27 spec/34 用例全绿；四档 249/0 + 往返 79 对。任务完成通知（run 成败接模板）仍属 B-8/taskrunner 契约面挂账未动 |
| 2026-10-28 | 头注 +事件面公约锚（P1-7 拍 b：SSOT=standards §14，本稿=机制细节锚——归属落定不搬家） |
