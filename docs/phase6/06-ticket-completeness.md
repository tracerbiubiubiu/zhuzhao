# 06 — P6-R0b 工单完善批（设计稿）

> **文档定位**：段级设计稿（2026-10-27 建档）——**工单三方对标批**（ficus-oa / eflow+etask / zhuzhao 代码级 14 维度盘点）产出。**状态 = 已批准（2026-10-27 所有者）——批范围 B 档**：件 1–10（件 2 并入 [00 号](00-flow-engine-v2.md) B0-2 后本批九件；件 11 归 [04 号](04-r4-metrics-reports.md) 拍板 3、件 12 留 S3b 对表）；与 00 号**交错**执行（拍板 4）。候选件 13–15 与剩余拍板点见 §7。总纲与拍板状态 = [README](README.md)。
> **立项由来**（2026-10-27 所有者「按建议先改文档」）：三方对比判定 zhuzhao 工单在**权限/表单/通知/并发工程**四项领先两参考系，差距集中在 12 件低成本完善件（单件 0.5–2 天）——本批把工单从「基座可用」补到「比较完善」；结构性差距（会签/或签/认领等**任务行语义族**）归 S3b 触发驱动不入本批（两参考系同样缺加签/委托/假期代理，非门槛项）。
> **量级**：B 档九件 ≈ **9–11 天**单人工（三波 §6；件 2 已移 B0-2）。

## 1. 定位与一句话

十二件小批、零新引擎：**1 件 R3 硬前置**（转换时字段采集——L3 处理人无字段编辑权缺口的解法）+ **4 件体验件**（运行态只读图/四视角/催办/字段类型）+ **4 件工程件**（events payload/类型包导入导出/转办/撤回）+ **3 件集成件**（webhook adapter/固定统计/组织级分派）；全部复用既有设施（transition_meta/exprcalc/S6 链/BK-21 三件套/000062 cc 表），与 00 号互为邻接（件 9 依赖 B0-4 条件词表）。

## 2. 现状基线（代码级锚点，2026-10-27 盘点）

- **L3 处理人无字段编辑权**：`canOperate` 的 update=创建人(+委托)、transition=创建人∨处理人（`internal/service/ticket/resource.go` canOperate:227）——处理人填不了「整改措施」。
- **详情页无流程图**：`zhuzhao-ui/src/views/ticket/detail/index.vue`（750 行）仅状态 tag+转换按钮，无 FlowGraph 引用；已走路径数据现成（ticket_events status_changed 序列）。
- **列表无「抄送我的」维度**：TicketListQuery 仅 AssigneeID/CreatedBy；cc 读特例支已存在（`ccVisibilitySQL`）。
- **手动催办已登记未排期**：[phase5/10 号](../phase5/10-s10-ticket-collaboration.md) 2026-10-26 触发登记（emit→S6 零新通知代码、60s 节流窗径既定）。
- **无关注/watcher**：ticket_cc.role 预留 cc/participant 值无消费（000062）。
- **字段类型 11 种无 user/dept**（`type_admin.go` validFieldTypes「只增不改」）；前端 UserPicker 组件已有。
- **ticket_events 无 payload 列**：键级 diff 只进 DomainEvent 载荷不落库（`service.go` Create 约 :638 注）——字段改动不可追溯。
- **无类型定义包导入导出**（画布 JSON 源码模式可复制单类型，非包级迁移；ficus 有 1302 行导入导出先例）。
- **处理人不能转办**：L3 assign=scope 主管(+委托)——ficus DelegateTask/eflow Transfer 均为处理人自办动作。
- **物理删断审计链**：Delete 事务+deleted 事件，FK SET NULL 随库存活但 ticket_id 失联。
- **零聚合统计端点**（[04 号](04-r4-metrics-reports.md) §2 勘查实证同源）。
- **组织级分派仅日志**：`dispatch_service.go` :62「phase6 资产路由场景再扩」。

## 3. 范围条目（P1 十二件）

### 3.1 件 1 转换时字段采集（R3 硬前置，~2–3 天）

- transition_meta **边级** `fields_on_transition: [{key, required}]` + 边属性 `comment_required: bool`（驳回类边可要求必填说明）；transition 端点 body 扩展 `fields`，随转换**同事务**写入 custom_data（走 validateFieldValue 校验子集）；写面白名单=仅 fields_on_transition 键——不打开处理人全量 update 权，L3 缺口就地闭合。
- 权限挂 transition 本身（创建人∨处理人既定），零新权限码；配置面=B0-5 属性抽屉承接（[00 号](00-flow-engine-v2.md) §3.1）。
- 蓝本：eflow 审批节点表单合并回工单 / ficus 节点表单（`oa_process_form` by elementId）。**先于 R3 复核态联调**（[03 号](03-r3-ticket-linkage.md) §3.2 已注前置第五件）。

### 3.2 件 2 运行态只读图（**已并入 00 号 B0-2**，2026-10-27 拍板 4）

- FlowGraphDesigner 只读模式渲染类型流程图 + **当前状态高亮 + 已走边点亮**（从 ticket_events status_changed 序列推已走路径）；后端 `GET /tickets/:id/trail`（0.5 天，事件序列+actor）。
- 蓝本：ecmdb-web `flow.vue` + GetTraversedEdges（其代理节点回溯复杂度本仓 FSM 无需——线性序列即路径）；ficus 反例=仅 el-steps 线性步骤条看不到分支走向。
- R3 场景：风险 owner 跟踪整改单「走到哪」。**拍板 4：已并入 00 号 B0-2**（随设计器向导页同一前端批交付，后端 trail 端点 0.5 天随主仓）——本件销账，设计留此供 B0-2 取用。

### 3.3 件 3 我的工单四视角 + cc_me（~1 天）

- 列表页四 tab：待办（assignee=me 非终态）/已办/我发起/抄送我（`cc_me=true` EXISTS ticket_cc，BK-21 同源不旁路 L2）；Home 工作台卡片对齐（badges todo 计数同源）。
- 蓝本：ficus 十列表页/ecmdb 四 tab；zhuzhao 现列表仅 assignee/creator 两维。

### 3.4 件 4 手动催办（~1 天，登记升格）

- `POST /tickets/urge`（body: ticket_id——standards §3 POST URL 不携业务信息）→ emit `ticket.urged`（事件词表 13→14）→ S6 规则种子（收件人=assignee）；**60s 同单同人节流复用** notifyThrottle 键径；授权=创建人∨抄送人∨admin（处理人不催自己）。
- [phase5/10 号](../phase5/10-s10-ticket-collaboration.md) 登记的「手动=即时单发、自动=水位多轮」互补形态兑现；R3 场景风险 owner 催整改单为必然操作。

### 3.5 件 5 user/dept 字段类型（~1–2 天）

- validFieldTypes 增 `user`（int64 用户 id——G2 校验存在且 active；显示走 userRef「人名(工号)」；导出列同）+ `dept`（org id 校验；可后置）。渲染器复用 UserPicker。
- 地基意义：R3 责任人预填落点（[03 号](03-r3-ticket-linkage.md) auto_fill）；指派轴「表单字段取人」档的消费对象（[05 号](05-launch-batch.md) §2 依赖 4）。**「只增不改」惯例内**（存量类型零感知）。**G2 校验须查目标用户存在且 active**（补强④——现 Assign 不查 active，README R1 已登记合规硬伤）。

### 3.6 件 6 ticket_events payload（~1 天）

- 迁移加 `payload JSONB`；updated 事件写 `changes: [{key, from, to}]`（richtext/file 类型只记 key 不记值——体积与敏感双收敛）；详情页时间线合流（评论+事件+变更按时间归并——现以评论为主）。
- 时机注记：当年「不为此加迁移」的取舍（service.go 注）在 phase6 合规场景下翻案——「谁把字段改了」须可追溯，与 risk_events 审计线同哲学。

### 3.7 件 7 类型定义包导入导出（~2 天）

- 导出 = 快照同形 JSON 包（type + fields + transition_meta + templates + assignment_rules）；导入 = dry-run 报冲突（code 撞/字段引用缺失/权限码缺失）+ 冲突策略拍板（跳过/覆盖/CAS）；权限 ticket:type:manage。
- 蓝本：ficus 导入导出 1302 行 / eflow 文件系统 bootstrap 模板；用途 = dev 配好风险整改单类型搬 prod（与 B0-3 deploy 快照同形，可合批）。

### 3.8 件 8 处理人转办（~1 天）

- `POST /tickets/transfer`（body: ticket_id + to_user_id + comment?）——L3 判定=**当前处理人**(+)，目标须同 scope 可见；复用 Assign 写事务（assigned_to 更新+事件）；`ticket.transferred` 入事件词表（与 urged 同批 13→14→15 或合并一次迁移）。
- 蓝本：ficus DelegateTask（防重复委托）/ eflow Transfer。**L3 判定与目标校验同步补 target user active 校验**（补强④，随件 5 同批）。

### 3.9 件 9 发起人撤回（~0.5–1 天，纯配置+谓词）

- 默认图种子加 `cancelled` 终态 + 边 `open→cancelled`（condition roles=[creator] + validator `assigned_to == null`）；**依赖 B0-4 条件词表含系统字段**（validator 求值上下文注入系统字段——本身 ~0.5 天随 B0-4）。
- phase3 起挂账的「发起人撤回 Revoke 业务设计缺失」（[review/11](../review/11-project-control.md) §8 B1）就此销账；撤回三条件蓝本（dd §29）的 FSM 简化档=仅「未受理可撤」。

### 3.10 件 10 webhook ChannelSender adapter（~1 天）

- 实现 `channel=webhook` 的 ChannelSender（复用 P4-2 webhook.Client + HMAC 签名走 standards §4）；payload=DomainEvent 载荷+模板渲染标题/正文 JSON 信封；目标 URL/secret 载体（message_rules 扩展列 vs channel_config JSONB）随启动批细化。**payload 支持群机器人 incoming-webhook 模板形态（飞书/钉钉/企微 JSON 变体，补强②）**——UIP 接线前把超期/指派提醒推进 IM 群的零成本通道（群到达，非个人 DM）；UIP 到位后退为 adapter 之一。
- 意义：「工单事件→外部系统」零新引擎（message_rules 配置即得）；R1 状态口径若拍「处置结论回写镜像侧」，此即出站通道；ficus syncVulnerabilityTask（办结→Kafka 回写漏洞库）同构先例。

### 3.11 件 11 工单侧第一档固定统计（~1–2 天；**B 档不含**——归 04 号拍板 3，第四组确认）

- `GET /tickets/stats`：group_by 白名单枚举（status/priority/type_code）+ 指标（计数、超期在办数/率）——**时长排行类留二档 DSL**（无 closed_at 列硬算成本高）；L2 同源=buildTicketWhere 三件套直接复用（[ticket_export.go](../../internal/handler/ticket_export.go) 先例）。
- 随 R1 聚合端点同批同模式（[04 号](04-r4-metrics-reports.md) §3.1 拍板点 3 同一决策，两处一并拍）；蓝本=ficus dashboard（三方最强统计面）。

### 3.12 件 12 组织级分派落点（**B 档不含**——留 S3b 对表再拍；建议案 A 维持）

- 现状 target_user=nil 仅日志（`dispatch_service.go` :62）。**案 A（建议）=派 target_org 部门负责人**（依赖 Week 0 HR mock 部门负责人轴，零新表）；案 B=候选池+认领（tickets 加 assigned_org 列+`POST /tickets/claim`+「待认领」视图，ficus ClaimNodeTask 蓝本）——B 与 S3b 任务行模型同族，建议留 S3b 一并。

### 3.13 候选件 13 邮件 SMTP ChannelSender adapter（~1 天，◆拍板点 5）

- 实现 `channel=email` 的 ChannelSender（SMTP 主机/凭据走 config+环境变量，禁硬编码——standards 安全基线）；收件人复用 S6 四档解析（users.email）；渲染走 msgrender email 象限（HTML 值级转义既有）。
- 意义：UIP 无时间表时**个人到达的唯一通道**；三方本地代码里都没有真实邮件实现（eflow 占位委托 enotify、ficus 走 Java 消息中心）——这是「完善」门槛项而非对标差距。

### 3.14 候选件 14 分派规则条件表达式化（~1 天，◆拍板点 5）

- assignment_rules 增 `condition` expr 列（与 keyword 并存：keyword 兼容存量；condition 非空时按 expr 判定）——复用 exprcalc 沙箱 + B0-4 条件词表（priority/type_code/custom_data.*/创建人 org）；保存期编译校验、运行期 fail-closed（求值错=不命中+ops 告警事件）。
- 意义：Week 0 指派轴解决「派给谁」，本件解决「**什么条件下派**」——两参考系的分派都是多维规则（eflow 8 种 / ficus 条件候选），现行「标题 LIKE」一档是分派面唯一明显短板。

### 3.15 候选件 15 在途单版本迁移工具（~1 天；可先 P2，◆拍板点 5）

- B0-3 钉版本后长周期在途单永远跑旧定义——管理端动作「迁移到最新版本」：状态映射表（旧 state→新 state，缺省同名直映）+ 逐单校验（目标状态存在、transition_meta 键域）+ 事件留痕（ticket_events `version_migrated` + payload 新旧版本号）；批量按类型/版本筛选。
- 意义：两参考系都没有（eflow 旧单跑快照到死、ficus 直接覆盖在途），是本仓能超过参考系的点；无真实诉求前维持 P2。

### 3.16 二次对比 A 类设计补强登记（2026-10-27 所有者批准，零量级）

| # | 补强 | 落点 |
|---|------|------|
| ① | **B-8 范围补「任务结果→自动转换」**：事件订阅形态 TransitionAction（[phase5/03 号](../phase5/03-s3-flow-semantics.md) P2-4 双形态②）消费 `run.succeeded`/`run.failed` 执行配置转换（成功→X、失败→Y），或 B0-4 词表加 `last_run.status`——否则「工单触发任务」闭环只到事件不到状态（eflow PassProcessTaskJob 对照） | 债台账 B-8 行（✅ 已回写） |
| ② | 件 10 payload 支持群机器人 incoming-webhook 形态 | §3.10（✅ 已回写） |
| ③ | 03 号拍板 4 建议「补消费」：模板 `default_sla_minutes` 进 Create 预填链→按级别选模板即得按级别时限 | [03 号 §4](03-r3-ticket-linkage.md)（✅ 已回写） |
| ④ | Assign/转办/user 字段三处补目标用户 active 校验 | §3.5/§3.8（✅ 已回写） |

## 4. P2 触发登记（不入排期；README §7 总账导航至此）

| 项 | 触发条件 |
|----|---------|
| 关注 watcher（复用 ticket_cc.role 自助挂/摘+读特例） | R3 owner 长周期跟踪整改单诉求（cc 需他人操作、watcher 自助——互补） |
| 状态级字段权限矩阵（state×field editable/required/hidden） | 复核态需「某字段仅处理人常驻可编辑」（件 1 转换窗采集不覆盖的常驻编辑） |
| 软删替代物理删 | 合规保留期/审计回放诉求（§2 断链问题翻案窗口） |
| 临期 predue 提醒相位 | R2 deadline 临期提醒镜像到工单侧诉求 |
| 每日待办汇总 digest | 催办轰炸/待办过载反馈（ficus DailyTodoTask 先例） |
| 小件包：评论编辑删除 / 列表动态列 / 批量操作 / 类型克隆 / 工单评价 / 类型级发起范围 | 各自诉求出现（ficus IndexTable/dashboard 蓝本在档） |

## 5. 边界（不做）

**任务行语义族**（会签/或签/认领/加签/委托/多级审批——S3b 触发驱动，[00 号](00-flow-engine-v2.md) §6 同边界）；并行/包容网关/子流程/草稿态/父子工单/合并拆分/轮询负载/工作日历/假期代理/移动端（README §5 维持——三方对照后仍非门槛项）；S3b 依赖的 workflow_* 表族零改动。

## 6. 批次切分（B1 系三波，草案）

| 波 | 批 | 内容 | 两仓分工 | 量级 | 依赖 |
|----|----|------|---------|------|------|
| B1 | **引擎配套件** | 件 1 转换字段采集（+comment_required）+ 件 8 转办（+active 校验）+ 件 9 撤回 | 主仓为主+前端转换对话框 | 3–4 天 | 件 9 依赖 B0-4 词表；件 1 先于 R3 复核联调 |
| B2 | **体验件** | 件 3 四视角 + 件 4 催办 + 件 5 user/dept 字段（+active 校验）+ 件 6 payload/时间线 +（既有登记）发起页模板选择接入（03 号 §2 缺口①顺带）；~~件 2 只读图~~ 已并入 00 号 B0-2 | 前端为主+主仓端点 | 4–5 天 | **与 00 号 B0-2/B0-5 前端窗并行**（拍板 4：交错）；件 5 先于指派轴「表单取人」档 |
| B3 | **集成件** | 件 7 导入导出 + 件 10 webhook adapter（含群机器人形态） | 主仓 | 2–3 天 | 件 11（归 04 号拍板 3）/ 件 12（留 S3b）**不含于 B 档** |
| 候选 | **件 13–15** | 邮件 adapter / 分派条件表达式化 / 在途单版本迁移 | 主仓 | +1 天/件 | ◆拍板点 5 |

## 7. 启动拍板点

1. **批范围**：**✅ 已拍 B 档（2026-10-27 所有者）**——件 1–10（件 2 并入 B0-2 后本批九件：1·3·4·5·6·7·8·9·10），≈9–11 天；件 11 并入 04 号拍板 3、件 12 留 S3b 对表。**枚举勘误**：初稿写「件 1–8+10」漏了件 9，与 §6 B1 波含件 9 不一致——按 §6 与「撤回属可感知完善件」口径定为件 1–10，量级不变。（原选项：A 全 12 件 / C 最小集件 1+2+5）
2. 件 11 是否随 R1 聚合端点同批——**归 [04 号](04-r4-metrics-reports.md) 拍板点 3**（第四组确认）。
3. 件 12 组织级分派落点：B 档不含；落点随 S3b 对表再拍（案 A 部门负责人建议维持，案 B 候选池+认领与任务行同族）。
4. 与 00 号的批次关系：**✅ 已拍交错（2026-10-27）**——B0-2/B0-5 前端窗并行本批 B2 波（两仓前后端分离、契约先行）；件 2 并入 B0-2。
5. **候选件 13–15 是否入批**（§3.13–3.15，2026-10-27 新增）：建议——件 13 邮件 adapter 入批（若 UIP 无明确时间表）、件 14 分派条件表达式化入批、件 15 维持 P2。

## 8. 与 00 号/启动批的关系

- **00 号 = 引擎运行面/编辑面重做**（快照/自动路由/设计器 v2）；**本批 = 语义与体验完善件**，除件 1 边级 meta 与件 9 依赖 B0-4 词表外零引擎改动。
- 准入门槛（[05 号](05-launch-batch.md) §4 四条）逐件核对：不依赖 phase6 范围拍板（件 11/12 除外，各有独立拍板点）✚ 不依赖外部契约（件 10 无外部契约）✚ 有独立价值或为 phase6 硬依赖（件 1=硬前置）✚ 单件 ≤4 天——**全中**。
- 结构性差距归因（防蔓延）：三方对比确认「状态 vs 任务行」是唯一结构性差——本批所有件均在 FSM 语义内，禁止以「完善」为名引入任务行/审批语义（那是对 S3b 的绕行，违 [03 号 §3](../phase5/03-s3-flow-semantics.md) 绕行禁令）。

## 9. 文档衔接

| 文档 | 关系 |
|------|------|
| [00 号](00-flow-engine-v2.md) | §2.3 三方对标（本批盘点来源）；件 9 依赖其 B0-4；件 2 可随 B0-2 |
| [03 号](03-r3-ticket-linkage.md) | 件 1=前置第五件（§3.2）；件 5=责任人预填落点；件 10=状态回写出站候选 |
| [04 号](04-r4-metrics-reports.md) | 件 11 与其拍板点 3 同一决策 |
| [05 号](05-launch-batch.md) | §1.1 立项动作行 / §2 依赖 4 |
| [phase5/03 号演进总表](../phase5/03-s3-flow-semantics.md) | 四触发行已入（转换字段采集/只读图/状态级字段权限/软删） |
| [phase5/10 号](../phase5/10-s10-ticket-collaboration.md) | 件 4=手动催办登记的兑现 |

## 10. 验收口径（能力语言，走查式）

处理人在 in_progress→pending_verify 转换对话框填必填「整改措施」并随转换同事务落库、驳回边要求必填说明；创建人撤回未受理单成功、已受理被拒且理由可读；处理人转办同 scope 同事后留痕可查；详情页流程图当前态高亮+已走边点亮；四视角 tab 与 cc_me 筛选计数与 badges/消息中心一致；owner 手动催办后处理人站内收到、60s 内重复催办被节流；详情时间线可见字段级变更 diff；dev 导出类型包在 prod dry-run 报冲突后按策略导入成功；webhook 规则配置后外部端点收到合法 HMAC 签名事件；（若拍件 11）统计端点行集与列表同筛选可见集一致（L2 不旁路）。

## 变更记录

| 日期 | 变更 |
|---|---|
| 2026-10-27 | 建档（工单三方对标批产出：P1 十二件+P2 触发登记六组；§7 四拍板点待立项评审） |
| 2026-10-27 | **第一轮拍板回写**（所有者）：状态翻「已批准 B 档」（件 1–10，枚举勘误含件 9；≈9–11 天）；拍板 4=交错、**件 2 并入 00 号 B0-2**（§3.2 销账）；件 11 归 04 号拍板 3、件 12 留 S3b；**二次对比登记**——§3.13–3.15 候选件 13–15（◆拍板 5）+ §3.16 A 类补强四条（B-8 范围/群机器人形态/拍板 4 补消费/active 校验，落点均已回写）；§6 批次表随之重排 |
