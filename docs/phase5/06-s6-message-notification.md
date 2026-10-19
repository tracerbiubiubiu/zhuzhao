# 06 — P5-S6 消息通知：统一消息平台 + 模板 + 工单触发（设计稿）

> **段属性**：基线段（phase5 收口对象）。**建议实施序**：与 S2 错峰衔接（S1 后；单人串行——S6 一期先于 S2）；**S10 依赖本段**（抄送通知/超时通知），phase6 硬前置 = 本段**一期**口径。
> **量级**：一期约 2–2.5 周（通道/模板/解析/触发/记录各 2–3 天 + 事件面基建 3–5 天）+ **二期站内消息中心 1.5–2 周**（2026-10-13 翻案挂入，S6 总账 3–4 周）。**迁移**：有（一期 `message_templates`/`message_log` 两表族 + `message_rules` 订阅规则轻表（附录 A.5）+ `tickets.human_key` ALTER（拍板点 2 随批时）——**≥3 段迁移（2026-10-16 外审计数对齐）**；二期 notifications 表族——编号启动时对账，不写死）。
> **文档定位**：段级设计稿（2026-10-15 建档，phase5 分支）。对接参考 = [phase3/10 §3 通知服务设计](../phase3/10-ticket-business.md)（邮件矩阵/事件订阅）；立项与演进 = dd §27（2026-10-08 四轮）+ 站内消息中心翻案条（2026-10-13）。

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
```

- **派发时机 = DB 事务提交后**：ticket service 状态变更路径在 `tx.Commit()` 成功后调 `Dispatch`（事务内只收集事件，提交失败零派发）——异步投递语义（§5.2）的实现锚点；
- **事件枚举补 cc_added（2026-10-16 评审批）**：抄送「收通知」依赖事件可订阅——S10 抄送落库时 emit `ticket.cc_added`（payload 含 cc 用户/role），S6 message_rules 即可订阅抄送通知；**无此事件=抄送通知须硬编码进 cc_service（违 §5.2 禁令）**——枚举随 B10-1 同批钉入；
- **域无关兑现**：DomainEvent 无工单专属字段渗透到订阅者契约（Payload 透传），phase6 R3 关单钩子/任务链按同名接口挂载，**第二订阅者零改 ticket service**（§10 phase6 前向验收）；**命名已拍（拍板点 4，2026-10-16）**：通用 `DomainEvent`/`DomainHook`——原 TicketEvent 系措辞废止（结构不变，本节已按新名改写）；
- **节流/去重落订阅者侧**（S6 消费者自责）：同 ticket_id+Action 窗口 60s 内去重（内存 sliding window = **单实例假设**；**多副本部署时 Redis 化**，登记触发 = 副本数 >1，对齐 [phase3/02](../phase3/02-multi-instance.md) 多实例纪律）。

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
- **两态桩**（§3 口径落地）：`notification.uip.stub=true` → 开发态桩（返回成功 + slog 落 msg 摘要，全链 E2E 可跑）；`false` 且 endpoint 未配置 → `ErrNotificationNotConfigured` fail-fast（**虚标防线负向必测** §10）；
- 发送重试：failed 进既有死信队列（复用 P4-2 链），dead 告警——不新建重试体系。

### A.4 模板渲染器（B6-3）

```go
func Render(tpl string, vars map[string]string, channel string) (string, error)
```

- 单遍正则替换 `{{ticket.assignee_name}}` **点路径占位符**（2026-10-16 升级：原平铺 {{key}} → 点路径——Mustache/Handlebars logic-less 思想，**if/loop/函数执行不进模板** = 业界对用户可编辑模板的安全红线；vars = 调用方从 context 树扁平化的点键 map，实现仍单遍替换；**不引库**——Handlebars-Go raymond 停更/osteele-liquid 过重（2026-10 核实），点路径解析自写 ≤0.5 天）；**禁二次展开**——字段值含 `{{` 不再解析，注入负向 §10；**值转义按渠道进行（三族）**：HTML 渠道 `html.EscapeString`、markdown 渠道转义值中链接/加粗元字符（`[]()*_` 等——防 `[点我](http://evil)` 被渲染成链接的内容注入/钓鱼面：收件人看到的是"系统发的"）、纯文本渠道原样；
- **变量字典 = 服务端 SSOT（2026-10-16 新增）**：按 `(biz_type, event)` 供给可用变量树（键+人读标签，端点=A.6）——工单域=内置字段（human_key/标题/状态/负责人/创建人/链接）+ S1 `field_meta` 动态字段（**与 S1 字段元数据同源**，字段改 key 失效告警同件 §4；**字典 path=派生 slug**——`ticket.fields.f_<字段ID>`：field_key 无字符集约束[代码实证 `validateFieldInputs` 不限 key 字符集，中文/含点键打断点路径]，模板只许引用字典 path，picker 选择即插入为主 UX，vars 扁平化按 field_meta 映射 slug 键；S1 登记新字段 key ASCII slug 软约束不回改存量——2026-10-16 评审批 P1-1）+ 时间类；校验两道=规则保存时**模板引用 ⊆ 字典**拦截 + 渲染时 vars 缺**内置**引用键报错拒发（防静默空串；动态字段缺失分档处置见下方粒度错配注）；`variables` 列保留=编辑器回填的引用清单（展示/审计）；picker 数据源=A.8.1 变量选择器/A.8.2 字段引用下拉；phase6 risk 域照此自供字典（域无关兑现）；**与 S1 expr 求值器分界**=通知占位符零执行 vs 计算字段受控执行，两件勿混；**条件段落**（"有截止日期才显示某段"——Liquid `{% if %}` 形态）登记触发不预建；**⚠ 粒度错配注（2026-10-16 外审发现，方案 a 采纳）**：模板**跨类型共享**（A.8.2 选择器只按渠道过滤）× 动态字段**类型私有**（field_meta per ticket_type）——模板引用类型 A 私有字段 → 规则绑类型 B → 渲染缺键按"拒发"会**断掉类型 B 全部通知**。处置：**渲染缺键分两类**——内置键缺失 = fail（模板 bug，保持拒发）；**动态字段缺失 = skip 该占位符 + message_log 警告**（类型错配可运维发现）——零 schema 变更，B6-3 实施落；**第四象限补齐（键存在但该单无值——存量单无新字段值/内置空值如未指派）= 空串渲染 + message_log 标 warned 不拒发**（数据常态≠配置错误，拒发会系统性丢通知；正文空串≠收件人跳过[A.8.2]，两处分别断言）——四象限至此完整：内置缺失 fail / 动态缺失 skip+警告 / 键在无值 空串+warned / 正常渲染（2026-10-16 评审批 P1-2 后半）；
- 时间类变量：`{{time.now}}`/`{{time.due_in}}` 由调用方注入渲染前 vars（格式 config 化，§4）——**点路径命名空间统一**（2026-10-16 外审对齐：原平铺 {{now}}/{{due_in}} 与点路径约定不一致）；
- 种子模板随迁移 INSERT（`is_system=true`，仅编号+标题+链接三变量——**人读编号拍板点 2 联动**：不随批则种子模板链接-only）。

### A.5 订阅规则与触发链路（B6-5）

- **订阅配置表（⚠ 建议新增轻表 `message_rules`，启动批设计）**：`(id, type_code, event, template_code, recipients_spec JSONB, enabled, version 乐观锁——对齐 notification_configs CAS 全仓惯例)`——收件人 spec 第一档 = `[{"source":"field","key":"assignee"},{"source":"fixed","users":[…]}]`（§5.1）；**域无关**：type_code 换 biz_type 即 phase6 复用（rules 表天然域无关）；
- **引用完整性双向守卫（三层，2026-10-16 评审批采纳）**：①**模板保存**——引用 ⊆ 所绑 rules 的 (biz_type,event) 字典并集，越界拦截/告警（原只有删除守卫——改正文静默破坏既有规则要到下次触发才以 failed 暴露；与 S1 影响分析同件）；②**规则保存**——模板动态字段引用 ⊆ 该 type_code 的 field_meta ∪ 内置 ∪ 时间类，**软告警不拦截**（field_meta 类型私有，(biz_type,event) 粒度不足以表达类型维度——粗校验①兜底+细告警②提示）；③**渲染时四象限**（A.4，运行时兜底）；守卫拦截/告警与模板·规则管理操作审计留痕随批（对齐 settings PUT 变更审计先例——11 号 B6-3 行同源）；
- **触发链**：HookRegistry.Dispatch → TicketNotifyHook（查 message_rules 命中）→ 渲染（A.4）→ 收件人解析 → ChannelSender.Send（异步 goroutine）→ message_log 落行；发送失败不回滚工单事务（事务早已提交，结构上不可能回滚——emit-after-commit 的免费收益）；**emit 单点锚=AssertTransition（2026-10-16 评审批）**：Dispatch 挂接锚定状态机咽喉 AssertTransition 单点——手工/API/批量/S3 动作多入口路径经同一咽喉，锚一处全路径覆盖，多入口各自挂=漏发；
- 人读编号（拍板点 2 随批）：`tickets` 加 `human_key VARCHAR(30) UNIQUE`，Create 时 `TK-yyyymmdd-NNNN`（按日序列，UNIQUE 冲突重试 ≤3）；存量单 NULL——模板变量解析时 NULL 回退展示 id（`#123`）。

### A.6 API 面（通知中心页面族，自 S4 迁入）

| 方法/路径 | 权限 | 用途 |
|-----------|------|------|
| POST `/api/v1/notifications/templates`·`/templates/update`·`/templates/delete` + GET 列表（**2026-10-16 外审修正：原 PUT 违 §3-1 改 house style**——code 入 body） | 通知配置管理权限（对齐 notification_configs 既有码） | 模板 CRUD + 删除守卫（被 rules 引用 → 拦截，与 S1 影响分析同件） |
| POST `/api/v1/notifications/templates/preview` | 同上 | 模板干跑渲染（body+vars 样例 → 渲染结果，零落库——A.8.1 测试数据预览；复用 A.4 Render 零新渲染逻辑，2026-10-16 业界核验批新增设计行） |
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
