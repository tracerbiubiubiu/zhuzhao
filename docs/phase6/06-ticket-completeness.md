# 06 — P6-R0b 工单完善批（设计稿）

> **文档定位**：段级设计稿（2026-10-27 建档）——**工单三方对标批**（ficus-oa / eflow+etask / zhuzhao 代码级 14 维度盘点）产出。**状态 = 草案待立项（未批准）**——候选启动批，与 [00 号](00-flow-engine-v2.md)（P6-R0 引擎 v2）并列；范围与量级随立项拍板（§7）。总纲与拍板状态 = [README](README.md)。
> **立项由来**（2026-10-27 所有者「按建议先改文档」）：三方对比判定 zhuzhao 工单在**权限/表单/通知/并发工程**四项领先两参考系，差距集中在 12 件低成本完善件（单件 0.5–2 天）——本批把工单从「基座可用」补到「比较完善」；结构性差距（会签/或签/认领等**任务行语义族**）归 S3b 触发驱动不入本批（两参考系同样缺加签/委托/假期代理，非门槛项）。
> **量级**：十二件合计 ≈ 12–15 天单人工（三波 §6；范围拍板见 §7）。

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

### 3.2 件 2 运行态只读图（~1–2 天）

- FlowGraphDesigner 只读模式渲染类型流程图 + **当前状态高亮 + 已走边点亮**（从 ticket_events status_changed 序列推已走路径）；后端 `GET /tickets/:id/trail`（0.5 天，事件序列+actor）。
- 蓝本：ecmdb-web `flow.vue` + GetTraversedEdges（其代理节点回溯复杂度本仓 FSM 无需——线性序列即路径）；ficus 反例=仅 el-steps 线性步骤条看不到分支走向。
- R3 场景：风险 owner 跟踪整改单「走到哪」。**可随 00 号 B0-2 顺手做**（拍板点 4）。

### 3.3 件 3 我的工单四视角 + cc_me（~1 天）

- 列表页四 tab：待办（assignee=me 非终态）/已办/我发起/抄送我（`cc_me=true` EXISTS ticket_cc，BK-21 同源不旁路 L2）；Home 工作台卡片对齐（badges todo 计数同源）。
- 蓝本：ficus 十列表页/ecmdb 四 tab；zhuzhao 现列表仅 assignee/creator 两维。

### 3.4 件 4 手动催办（~1 天，登记升格）

- `POST /tickets/urge`（body: ticket_id——standards §3 POST URL 不携业务信息）→ emit `ticket.urged`（事件词表 13→14）→ S6 规则种子（收件人=assignee）；**60s 同单同人节流复用** notifyThrottle 键径；授权=创建人∨抄送人∨admin（处理人不催自己）。
- [phase5/10 号](../phase5/10-s10-ticket-collaboration.md) 登记的「手动=即时单发、自动=水位多轮」互补形态兑现；R3 场景风险 owner 催整改单为必然操作。

### 3.5 件 5 user/dept 字段类型（~1–2 天）

- validFieldTypes 增 `user`（int64 用户 id——G2 校验存在且 active；显示走 userRef「人名(工号)」；导出列同）+ `dept`（org id 校验；可后置）。渲染器复用 UserPicker。
- 地基意义：R3 责任人预填落点（[03 号](03-r3-ticket-linkage.md) auto_fill）；指派轴「表单字段取人」档的消费对象（[05 号](05-launch-batch.md) §2 依赖 4）。**「只增不改」惯例内**（存量类型零感知）。

### 3.6 件 6 ticket_events payload（~1 天）

- 迁移加 `payload JSONB`；updated 事件写 `changes: [{key, from, to}]`（richtext/file 类型只记 key 不记值——体积与敏感双收敛）；详情页时间线合流（评论+事件+变更按时间归并——现以评论为主）。
- 时机注记：当年「不为此加迁移」的取舍（service.go 注）在 phase6 合规场景下翻案——「谁把字段改了」须可追溯，与 risk_events 审计线同哲学。

### 3.7 件 7 类型定义包导入导出（~2 天）

- 导出 = 快照同形 JSON 包（type + fields + transition_meta + templates + assignment_rules）；导入 = dry-run 报冲突（code 撞/字段引用缺失/权限码缺失）+ 冲突策略拍板（跳过/覆盖/CAS）；权限 ticket:type:manage。
- 蓝本：ficus 导入导出 1302 行 / eflow 文件系统 bootstrap 模板；用途 = dev 配好风险整改单类型搬 prod（与 B0-3 deploy 快照同形，可合批）。

### 3.8 件 8 处理人转办（~1 天）

- `POST /tickets/transfer`（body: ticket_id + to_user_id + comment?）——L3 判定=**当前处理人**(+)，目标须同 scope 可见；复用 Assign 写事务（assigned_to 更新+事件）；`ticket.transferred` 入事件词表（与 urged 同批 13→14→15 或合并一次迁移）。
- 蓝本：ficus DelegateTask（防重复委托）/ eflow Transfer。

### 3.9 件 9 发起人撤回（~0.5–1 天，纯配置+谓词）

- 默认图种子加 `cancelled` 终态 + 边 `open→cancelled`（condition roles=[creator] + validator `assigned_to == null`）；**依赖 B0-4 条件词表含系统字段**（validator 求值上下文注入系统字段——本身 ~0.5 天随 B0-4）。
- phase3 起挂账的「发起人撤回 Revoke 业务设计缺失」（[review/11](../review/11-project-control.md) §8 B1）就此销账；撤回三条件蓝本（dd §29）的 FSM 简化档=仅「未受理可撤」。

### 3.10 件 10 webhook ChannelSender adapter（~1 天）

- 实现 `channel=webhook` 的 ChannelSender（复用 P4-2 webhook.Client + HMAC 签名走 standards §4）；payload=DomainEvent 载荷+模板渲染标题/正文 JSON 信封；目标 URL/secret 载体（message_rules 扩展列 vs channel_config JSONB）随启动批细化。
- 意义：「工单事件→外部系统」零新引擎（message_rules 配置即得）；R1 状态口径若拍「处置结论回写镜像侧」，此即出站通道；ficus syncVulnerabilityTask（办结→Kafka 回写漏洞库）同构先例。

### 3.11 件 11 工单侧第一档固定统计（~1–2 天，◆拍板点 2）

- `GET /tickets/stats`：group_by 白名单枚举（status/priority/type_code）+ 指标（计数、超期在办数/率）——**时长排行类留二档 DSL**（无 closed_at 列硬算成本高）；L2 同源=buildTicketWhere 三件套直接复用（[ticket_export.go](../../internal/handler/ticket_export.go) 先例）。
- 随 R1 聚合端点同批同模式（[04 号](04-r4-metrics-reports.md) §3.1 拍板点 3 同一决策，两处一并拍）；蓝本=ficus dashboard（三方最强统计面）。

### 3.12 件 12 组织级分派落点（◆拍板点 3，实现 1–3 天）

- 现状 target_user=nil 仅日志（`dispatch_service.go` :62）。**案 A（建议）=派 target_org 部门负责人**（依赖 Week 0 HR mock 部门负责人轴，零新表）；案 B=候选池+认领（tickets 加 assigned_org 列+`POST /tickets/claim`+「待认领」视图，ficus ClaimNodeTask 蓝本）——B 与 S3b 任务行模型同族，建议留 S3b 一并。

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
| B1 | **引擎配套件** | 件 1 转换字段采集（+comment_required）+ 件 8 转办 + 件 9 撤回 | 主仓为主+前端转换对话框 | 3–4 天 | 件 9 依赖 B0-4；件 1 先于 R3 复核联调 |
| B2 | **体验件** | 件 2 只读图 + 件 3 四视角 + 件 4 催办 + 件 5 user/dept 字段 + 件 6 payload/时间线 +（既有登记）发起页模板选择接入（03 号 §2 缺口①顺带） | 前端为主+主仓端点 | 5–6 天 | 件 5 先于指派轴「表单取人」档 |
| B3 | **集成件** | 件 7 导入导出 + 件 10 webhook adapter + 件 11 统计 + 件 12 组织级分派 | 主仓 | 4–5 天 | 件 11/12 各有拍板点 |

## 7. 启动拍板点

1. **批范围**：A=全 12 件（~2 周）vs **B=核心 9 件**（件 1–8+10，≈10–12 天；建议——件 11 并入 04 号拍、件 12 留 S3b 对表）vs C=仅 R3 硬前置最小集（件 1+2+5，~4–5 天）。
2. 件 11 是否随 R1 聚合端点同批（与 [04 号](04-r4-metrics-reports.md) 拍板点 3 同一决策）。
3. 件 12 组织级分派落点：案 A 部门负责人（建议）vs 案 B 候选池+认领（留 S3b）。
4. 与 00 号的批次关系：串行（先 00 后 06）vs 交错（B0-2/B0-5 前端窗并行 B2 波，前后端分离）——件 2 若随 B0-2 顺手做则本批减一件；立项时一并定。

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
