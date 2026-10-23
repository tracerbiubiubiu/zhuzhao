# 03 — P5-S3 流程运行语义配置化（S3a/S3b）（设计稿）

> **段属性**：**S3a 已升格排期段（2026-10-19 所有者拍「升」，原条件段）——W13–15 实施、纳入 phase5 收官**（升格论据=事件驱动统一模型+phase6 前核心引擎目标，见附录演进路线 gap1）；S3b 审批流**等首个真实审批场景**再启动（无消费者不建引擎——easy-workflow 教训），**不计入 phase5 收口**。
> **量级**：S3a ≈ 2 周（转换级语义 + 分派 + 动作库框架；**assignment_rules 一表**——transition_meta 列复用 S10 B10-4 已建[2026-10-19 结构修正，零新 DDL]，原"零新表或轻表"措辞修正）；S3b 待场景（workflow_* 表族，DDL 蓝本现成）。
> **文档定位**：段级设计稿（2026-10-15 建档，phase5 分支）。**实施蓝本 = [phase3/10-ticket-business.md](../phase3/10-ticket-business.md)**（§23 翻案后恢复实施蓝本地位，2026-10-09）；翻案决策 = [design-decisions §29](../design/design-decisions.md)。

## 1. 定位与由来

工单动作语义现状**代码硬编码**：Close/Assign/Update 各端点方法内按三层鉴权 + 属主/委托判定，状态变更经 `AssertTransition`（`internal/service/ticket/service.go:330/:390/:432`，2026-10-15 核实在位）。目标形态 = **转换级配置**：谁能触发哪个转换（角色/属主/处理人）、转换前置条件、自动分派规则、审批节点（S3b）。

§23（2026-09-02 工单自研暂缓）已于 2026-10-09 翻案（dd §29：内部平台二开对接成本高、自研更适配、工单流程中心是平台重点）——**S3 开工本身即翻案动作，翻案已完成，本段是其排期落位**；未翻案前的"零投入"状态终止。SLA 引擎**不建**（S10 轻量版为准——§29 原文为方向性定论「不做完整引擎」一句；细分边界=设计推演非逐条拍板，触发信号在案 [10 号 §5](./10-s10-ticket-collaboration.md)，2026-10-19 溯源修正）。

## 2. 现状基线（代码级锚点，2026-10-15 核实）

| 项 | 现状 | 锚点 |
|----|------|------|
| 动作语义 | Close/Assign/Update 硬编码于端点方法，按三层鉴权+属主/委托判定 | `service/ticket/service.go:330/:390/:432` |
| 状态机 | 通用引擎，`validateTypeGraph` 端点校验；无转换级权限/条件语义 | `state_machine.go` + `type_admin.go` |
| 分派 | 手动 Assign；自动分派零实现（assignment_rules 蓝本未落） | phase3/10 §5 |
| 审批 | 零实现（风险接受单 = 确认制）——S3b 的消费者缺口 | dd §29 |
| 鉴权基建 | 2c Authorize（org admin/owner + ancestor）已交付 = 分派规则/审批候选的属主依赖项 | phase3/10 §0 前置表 |

## 3. S3a：转换级语义 + 自动分派 + 动作库框架（≈2 周）

> **时点决策与后插代价评估（2026-10-16，所有者问询"phase6 前要不要升级引擎"后定——维持条件段，方案 A+C）**：phase6 MVP **不需要** S3a（硬前置不含；核心链路逐环有落点——派生工单=S1 预填/复核流程=S2 状态图配置/终态联动关险单=S6 共享事件面/超期催办=S10）。**后插代价已评估为小且可预算**：S3a 本质是**叠加式语义层**而非推翻式重构——① 数据零迁移（transitions JSONB meta 扩展，配置空=行为不变，存量单零感知）；② 引擎改造=`AssertTransition` 叠加校验（端点级判定保留、更严叠加），非重写；③ 周边全部在位或已留缝（S1 求值器公共件 / S6 B6-1 事件面对表 / S2 画布适配层分离）。**唯一真实代价 = 绕行债**：phase6 期间撞墙需求**不绕行、记账等 S3a**（人工流程顶滞后期 ≈2 周），**禁**另造状态/另开端点/service 层硬编码特殊判断——绕行实现才是 S3a 到来时真正要清理的不可控成本。~~显式触发信号 = phase6 R3 出现首个转换级权限真实需求~~（**2026-10-19 所有者拍「升」：触发信号不再等待——S3a 升格 W13–15 排期**，论据=事件驱动统一模型[转派动作需同步事务内通道]+phase6 R3 复核态权限为明牌需求，见附录「事件驱动统一模型」与「phase6 前核心引擎目标态」两节；次序靠后项[分派/网关条件]随排期一并实施）；S3a 启动时与 B6-1 事件面按 **DomainEvent/DomainHook** 形态对表（[11 号 §2.4](./11-implementation-plan.md) 已钉——2026-10-16 拍板点 S6#4：通用命名，原 TicketHooks 称谓废止）。

### 3.1 转换级语义（Conditions/Validators）

- **谁能触发哪个转换**的配置化：转换声明触发者范围（角色/属主/处理人）+ 前置条件，落 transitions JSONB meta 扩展。
- **架构约束（不可绕）**：权限判定必须落**策略层**——类型级策略数据 + fail-closed，经既有三层鉴权/Registry 形态接入；**禁止业务层手写绕过**（AGENTS 约定；§25 平台策略库"事实在 DB/语义在代码"分界随翻案批重审）。
- 转换条件表达式 = **与 S1 共用白名单 Go 侧求值器**（expr 预置；禁 SQL 下放——easy-workflow 反面教材）。

### 3.2 自动分派规则

蓝本 = phase3/10 §5 `assignment_rules` 表 + 规则匹配引擎（`OnAfterCreate` Hook 按优先级遍历，首个命中执行分派，走 `TicketService.Assign` 不绕鉴权）。**五项决议既定（2026-08-31 拍板）**：① keyword=LIKE 包含不分词；② 同优先级按 id 升序 tie-break；③ target_user 必须属于 target_org（写侧校验）；④ 无命中兜底保持 open + 写 `dispatch_missed` 事件；⑤ **分派失败不回滚创建**（Hook 捕获 + 事件 + 异步补偿）。迁移编号启动时按 11 号 §4 对账（A2 规则）。

### 3.3 动作库框架（Post-function 形态）

- **流转时自动动作**（2026-10-08 所有者场景）："按字段判断分支后取数回写"若发生在流转时（进入某状态触发），落点 = **转换后动作**——实现形态 = 节点/转换 meta + 显式钩子（**已拍命名 `DomainHook`**——拍板点 S6#4，2026-10-16，原 TicketHooks 称谓废止；phase3/10 §4.2 既定，编译期检查）；**引擎不内置功能节点类型**（节点类型膨胀 = easy-workflow 反面；功能外挂与 Jira post-functions 同构）。
- **on_complete 任务完成通知（2026-10-23 对话设计批——计划任务跑完通知场景）**：任务（cron/script/connector）完成后可配置通知——设计形态 = `on_complete.notify` 段（任务配置内嵌，非独立表）：
  ```json
  { "on_complete": { "notify": {
      "template": "daily_scan_report",
      "params_mapping": { "vuln_count": "result.total", "high_count": "result.high" },
      "recipients": ["security_team"], "channels": ["email", "dingtalk"]
  } } }
  ```
  **params_mapping 语义**：模板占位符 ← 任务返回值路径（`result.xxx`）或固定值（`literal`）——GitHub Actions outputs/inputs 同构。任务跑完 → 返回值 map → params_mapping 解析 → 模板渲染 → NotificationService.send()。实现 = S3a 动作参数化框架的一个消费场景（Action 参数 `from` 来源可以是 `event`/`result`/`literal`）；NotificationService 接口设计需确保可被 Action 框架调用（S6 B6-2 设计时注意——非代码变更）。
- **动作库类型（2026-10-09 登记；2026-10-19 双通道分工修正）**：取数回写 / 抄送（S10）/ **转派**（升级链——扫描器事务内直调，见事件驱动统一模型裁定）/ **提交任务**（走既有 Submit 通道，理想态登记）。**通知收敛走 S6 规则面**——message_rules 直接订阅 DomainEvent，post_action 再配 notify = 同一流转两处可配通知（重复通知/配置漂移无仲裁；业界对照：Jira notification scheme 全局订阅 vs Automation 动作有明确分层）——**notify 动作降为无规则兜底**并注明 S6 节流查重兜底。🚦 前置不变（随场景逐个启用，框架先行不预做动作本体）。
- 提交时机的同款诉求归 S1 字段联动，勿混入流程层（执行时机三分界）。

## 4. S3b：审批流引擎（等首个真实审批场景）

- **蓝本全套现成**（phase3/10 §4）：`BranchedStateEngine`（状态机内核）+ `ApprovalTaskLayer`（人工任务层 claim/submit）两层分离；`workflow_definitions/instances/node_states/records/tasks` 五表 DDL（§4.3）；`CanApproveNode` 契约骨架与调用顺序铁律（L1→解析 Req→L2→L3，§4.5）；`WhatCanIDo` 任务可操作查询；驳回/回退语义（target=prev/origin + 自由驳回 BFS + 会签 batch_code，§4.5(2)）。
- **7-0 设计期五决议（2026-08-31 拍板，随批生效）**：①审批人策略 = `Assignee{rule,values}`（**弃 min_level**——users 无 level 列）；②模板-流程绑定类型 1:1 默认 + 模板 nullable 覆盖；③发起人撤回 = WITHDRAWING 栅栏模式；④**版本/发布快照**（编辑版/发布版分离，Deploy 锁快照，运行时按实例版本取——原地改 definition 禁止，防在途漂移）；⑤signal 双写分离。
- **easy-workflow 借鉴核验闭环（2026-10-08 代码级复核）**：§4.2 十二项借鉴点全部证实、四条避坑点（MySQL CTE/表达式拼 SQL/反射事件/引擎内嵌身份）全部被其真实缺陷证实——**借设计不引库**（MIT 但零测试/裸 map 竞争/改定义即在途漂移），无新增借鉴项。
- **当前风险接受**：审批需求第一档 = superadmin/owner 确认制（phase6 风险接受单同口径）；S3b 落地后可升级。**触发时点 = 首个真实审批场景出现**（残留开放项，dd §29）。
- **S3b 实施换算注（2026-10-15 公约对表）**：蓝本成文于 2026-08-25（早于 standards §3 API 公约建纲 2026-09-07），蓝本内端点形态（如 §4.7 `PUT /workflows/:code`）**实施时按公约新端点口径换算**（POST + 标识/动作入 body，§3-1/§3-2）——照抄即违公约。

## 5. 启动拍板点

| # | 事项 | 说明 |
|---|------|------|
| 1 | S3b 启动时点 | 等首个真实审批场景（既定，非本批新拍） |
| 2 | §25"事实在 DB/语义在代码"分界重审 | 随翻案批——转换级权限落策略库的具体形态 |
| 3 | S3a 动作库首批启用哪几个动作 | 框架先行；动作本体随场景（通知/抄送依赖 S6/S10 交付） |

> **状态注（2026-10-16）**：本表拍板点已随批量预确认拍定（2026-10-16 所有者「按建议」——✅34/🔶1，状态 SSOT = [11 号 §4](./11-implementation-plan.md)；各段启动时异议可翻，翻案成本=改一行）。

## 6. 边界（不做）

- **不做通用流程引擎/BPMN 全家桶**（Camunda/Activiti 级）——16 号红线同构适用工单侧；只做工单域状态图 + 语义配置。
- **不做运行时热插拔自定义脚本动作**（脚本注入面）：本条管**工单转换后动作**——动作须代码注册 handler、不得让用户上传脚本挂上工单流程；任务执行域的"用户上传脚本"另有 15 号 🚦 触发驱动项，**两域勿混**（§3 红线行已加域限定注记）。
- SLA 计时引擎不建（S10 轻量版为准，dd §29）。
- 迁移编号按启动时 11 号 §4 对账占用（A2：撞号让位重排）。

## 7. 影响面

- **迁移**：S3a = assignment_rules 一表（`transition_meta` 列**复用 S10 B10-4 已建**——2026-10-19 结构修正，零新 DDL，附录 A.1 列交付时点注）；S3b 有（workflow_* 五表族，DDL 蓝本 §4.3）。
- **权限**：转换级语义落策略层（fail-closed）；动作库执行走既有权限码体系，不新增旁路。
- **端口/公共件**：求值器（与 S1 共用）；动作 handler 注册表（代码注册，编译期检查）。
- **依赖**：S1/S2 收口（**meta 配置面=本段 B3-1 类型管理页扩展自带**——S2 画布范围不含 meta 编辑，02 号 §3 界注；S1 求值器共用）+ S6（通知动作消费者）——**排期硬约束**。

## 8. 验收口径

- 后端件随四档 acceptance；配置面走 ui CI + E2E + 场景走查。
- 场景标尺（S3a）：配置"仅属主可触发 close"→ 非属主转换被拒（fail-closed 负向必测）；建分派规则 → 建单自动分派到目标组 → 无命中单保持 open 且有 `dispatch_missed` 事件；分派失败不阻塞建单（⑤）。S3b 启动时取 phase3/10 §8 TB7/TB8/TB16（审批启动/流转/撤回）为验收底稿。

## 9. 文档衔接

| 文档 | 关系 |
|------|------|
| [phase3/10-ticket-business.md](../phase3/10-ticket-business.md) | **实施蓝本 SSOT**（§4 审批/§5 分派/§4.2 能力映射/§4.10 7-0 决议） |
| [design-decisions §23/§29](../design/design-decisions.md) | 暂缓 → 翻案全链登记 |
| [01-s1](./01-s1-form-designer.md) / [02-s2](./02-s2-flow-graph-designer.md) | 求值器共用；meta 配置面载体 |
| [06-s6](./06-s6-message-notification.md) / [10-s10](./10-s10-ticket-collaboration.md) | 动作库消费者（通知/抄送） |
| [phase6/README §3](../phase6/README.md) | phase6 硬前置链；风险接受审批第一档确认制 |

---

## 附录 A：S3a 详细技术设计（实施级，2026-10-19 补——升格排期后按排期段标准补齐，05 号样板深度）

### A.1 转换级语义：Jira transition 四段对表 + meta 结构

**Jira workflow transition 四段解剖（20 年业界共识）——S3a 三能力精确对齐**：

| Jira 术语 | 语义 | zhuzhao 对应（批次） |
|-----------|------|---------------------|
| **condition** | 谁能看到/执行转换（权限门——不满足则按钮不渲染） | B3-1 转换级权限（roles/属主/处理人） |
| **validator** | 执行前校验（不满足拒绝+报错） | B3-1 前置条件（expr 表达式） |
| **post-function** | 执行后自动化动作 | B3-3 动作库 |

- **meta 结构（对齐 01 号 A.2 方案 b 先例——独立列防结构混载）**：

```sql
-- ⚠ 列交付时点（2026-10-19 结构修正）：本列由 S10 B10-4（W9–11）**先行创建**（列名
-- transition_meta——B10-4 用 state 级键存 timeout_minutes），本段 B3-1 扩展 "from>to"
-- 转换级键复用同列（一列两用、">" 区分键形）——勿重复建列，ALTER 随 B10-4 编号
ALTER TABLE ticket_types ADD COLUMN IF NOT EXISTS transition_meta JSONB DEFAULT '{}';
-- 转换级键 = "from>to"（定位）；值 = condition/validator/post_actions 三段
-- {"in_progress>pending_verify": {"roles": ["assignee","org_admin"],
--   "condition": "priority == 'P1'", "post_actions": ["notify","cc_team","reassign"]}}
-- 缺省 = 既有行为（零破坏）；存储与 transitions 邻接表分列——01 号 A.2 同款决策
```

`post_actions` 元素允许字符串或 `{ "name": "...", "params": {...} }` 对象形态（B3-3 随行件——2026-10-20 规划研究回灌，依据 deliverables/software-company/secops-platform-vision-2026-10/08-phase5-adjustments.md）。

- **可用转换查询**：`GET /tickets/:id/transitions/available`（Jira condition 语义=按当前用户过滤——前端按钮渲染依据；服务端 AssertTransition 仍为 fail-closed 兜底，前端过滤仅 UX）；
- **求值器**：expr 与 S1 共用（编译缓存/沙箱/函数白名单——01 号 A.3 同款），字段上下文=custom_data+内置字段。**condition 类型扩展触发登记（2026-10-19 外审）**：首批=roles/属主/处理人三类（phase6 R3 复核态=属主类，够用）；字段值条件（Jira field-valued condition）由 validator=expr 覆盖数据侧，condition 面的字段值形态出现诉求再扩（触发登记勿预建）。**字段引用约定（2026-10-19 外审补 P2-5）**：内置字段=标识符直引（priority == 'P1'）；**动态字段=map 索引形式 custom_data["字段名"]**（expr 支持字符串索引——中文/含点 key 不作标识符，规避解析失败与内建冲突）；与 S6 变量字典 slug 约定的统一对表随 B3-1（两处引用同一字段语义须一致）。

### A.2 自动分派：assignment_rules DDL（五决议既定 §3.2）

```sql
CREATE TABLE assignment_rules (
    id          BIGSERIAL PRIMARY KEY,
    type_code   VARCHAR(50) NOT NULL,
    priority    INT NOT NULL DEFAULT 100,
    keyword     VARCHAR(100),                     -- LIKE 包含匹配（决议①不分词）
    target_org  BIGINT NOT NULL REFERENCES organizations(id),
    target_user BIGINT REFERENCES users(id),      -- 决议③必须属于 target_org（写侧校验）
    enabled     BOOLEAN NOT NULL DEFAULT TRUE,
    created_by  BIGINT NOT NULL,
    created_at  TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX idx_assignment_rules_type ON assignment_rules (type_code, priority);
-- OnAfterCreate Hook：按 priority 遍历→首个命中执行分派（走 Assign 不绕鉴权）；
-- 无命中保持 open+dispatch_missed 事件（决议④）；失败不回滚创建（决议⑤）
-- 管理面（2026-10-19 外审 P3-9 补）：规则 CRUD 管理端点+页面归 B3-2（类型管理页族扩展；
--   权限沿用类型管理权限——零新码），随 B3-2 验收「规则配置→命中→自动分派」全链
-- 扩展触发登记（2026-10-19 外审）：决议①=keyword LIKE 仅包含匹配——「按字段值分派」
--   （custom_data 条件选组）出现诉求再扩（触发登记勿预建，Jira component 路由同源形态）
```

### A.3 动作库框架（Post-function 形态）

```go
type TransitionAction interface {
    Name() string                                    // 注册键（编译期——反射反面）
    Execute(ctx context.Context, ev domainevent.DomainEvent, t *model.Ticket) error
}
// internal/service/ticket/actions/：notify.go(S6) / cc.go(S10) / reassign.go（首批三动作）
// 注册：app.go 装配 actionRegistry.Register(name, NewXxxAction(...))——重复名 panic
// 失败语义：不回滚转换（Hook 捕获 + Dispatch DomainEvent{BizType:"ops", Action:"ops.action_failed"}
//   [payload=动作名/工单/错误——消费方=ops 告警规则(S6 规则面配置)] + 异步补偿队列——分派决议⑤同款）
//   补偿事件命名定案（2026-10-19 外审 P2-6：原三缺[名/消费方/内容]补齐）
```

可选方法 `ParamsSchema() json.RawMessage`：返回动作参数的 JSON Schema（nil 合法）；`Execute` 签名不变（B3-3 随行件，2026-10-20 回灌）。

### A.4 easy-workflow 对照开发法（所有者提议"参考修改补强"的操作化——不 fork 不 import）

**核验结论回顾（2026-10-08 代码级复核）**：四条避坑点全是骨架级——fork 后要改的恰是承重墙：

| 避坑点 | zhuzhao 要求 | fork 改造量 |
|--------|-------------|------------|
| MySQL 绑定 | PG+pgx | 存储层全重写 |
| 表达式拼 SQL+黑名单 | expr 白名单（防注入红线） | 求值层全重写 |
| 反射事件 | 编译期注册 | 事件层全重写 |
| 引擎内嵌身份 | 身份归三层鉴权 | 身份层剥离 |

**量化**：fork 改造 ≈2–3 周（四层重写+继承零测试/裸 map 竞争）> 对照自研 ≈2 周（设计已细化到本附录级）。**操作化方法**：easy-workflow 仓库作为参考实现放在手边——实现每个模块前先读其对应模块，**取结构、避缺陷**（RuoYi/easy-workflow 两次核验同款方法论；它的真实价值=免费错题本）。生态事实：Go 侧无 Flowable 级引擎（Temporal 为 durable execution 不同范式）——自研语义层是 Go 生态正常选择。

### A.5 验收补充（在 §8 基础上）

- Jira 四段：condition 不满足 → available 端点不含该转换 + AssertTransition 拒（双重）；validator 表达式 false → 400 带字段名；post-action 失败 → 转换成功+补偿事件（不回滚断言）；
- meta 往返：**B3-1 配置面**（类型管理页扩展——S2 画布范围不含 meta 编辑，见下方界注）配置 meta → 保存 → 回显一致；无 meta 存量类型零行为变化（零破坏断言）；
- 分派：规则命中自动分派（决议①–⑤逐条）；keyword 注入负向（LIKE 通配符转义）；
- 求值器沙箱负向（与 01 号 A.6 同款）。
- 界注（2026-10-19 外审 P1-2）：**meta 编辑面归本段 B3-1**（类型管理页扩展+配套端点）——S2 画布范围=states/transitions 邻接表+后端零改动（02 号 §3 结构性不覆盖 transition_meta 独立列）；**timeout_minutes 承载=transition_meta 新列（B10-4 建列、state 级键——2026-10-19 结构修正，transitions JSONB 邻接表无边属性位）**，编辑面=B10-4 管理页/JSON 专家模式。

### A.6 引擎独立性评估（2026-10-16/19 所有者两次问询——"工单引擎是否拆独立仓库"）

**结论：现在不拆**——项目第三次面对同一判据，答案一致（FormDesigner「先目录后包」/utils「跨仓消费方才拆」/本次同构）：

**判据（跨仓消费方 ≥2 才拆独立 go module）**：工单引擎（状态机+转换语义+事件面+动作库+未来审批/SLA）当前唯一消费方 = zhuzhao 自己——taskrunner/activelist 无对应场景，utils 零新增（A.10 同款审计）。

**引擎无法干净剥离的耦合面（拆=连带重写）**：
1. AssertTransition 与三层鉴权/策略库（casbin+Registry）深度耦合——转换级权限落策略库（B3-1）加深此耦合；
2. DomainEvent 订阅者（S6 notification/phase6 risk）全在主仓；
3. assignment_rules 引用 organizations/users 表；
4. 动作库首批动作（notify/cc/reassign）全是主仓业务语义。

**演化期纪律**：引擎连续三波演进（S3a/S3b/SLA）——过早抽库=每次演进跨仓同步（版本/pin/发版），单人维护负收益（01 号 FormDesigner 同款三笔负收益论证）。**保持模块化单体（modular monolith）内部领域隔离**即项目既定架构（「微服务不拆，抄模式不抄架构」；IAM 三步走第一步=画边界非拆）。

**拆分触发信号（出现任一即重评，形态届时定）**：①第二消费方出现（独立工单门户/其他产品线复用引擎）；②部署独立性需求（工单流量需独立扩缩容）；③团队拆分（多人分域维护）。**phase5 能做的准备**：附录 A.1 落点已把引擎件聚类（cc/tag/badge/timeout_scanner/export+引擎本体），保持聚类纪律+域间只经 DomainEvent/service 接口通信（不经表直查）即=「画边界」完成。

**耦合面定性补充（2026-10-19 追问"是否需重构主仓"——三条本质一条已决，不建议为拆库重构）**：① AssertTransition×策略库 = **本质耦合**（「权限判定在数据处」架构定律——Jira workflow condition 同样长在平台权限体系；B3-1 落策略库加深耦合是正确方向）；② DomainEvent 订阅者全在主仓 = 拆库后正确形态非障碍（内核发事件/业务消费——零重构）；③ assignment_rules 引用 orgs/users 表 = 领域数据引用（单体正常形态，拆时改 ID 引用+解析——现在改 = YAGNI）；④ 动作库 = 框架/实现已在 B3-3 分离 ✅。**画边界补强（2026-10-19 时机优化拍定——两件拆开排）**：① **架构依赖规则工具化**——depguard/go-arch-lint 类把「engine 不 import 业务外依赖/域间禁表直查经接口」变成 CI 强制（modular monolith 标准实践——Spring Modulith 的 Go 等价物，成本半天），**前移至 W1 开工首件**（规则从第一行 B5 代码起生效——五段新代码全在边界内产生，拖至 W13=前 12 周游离规则外一次性清算反痛）；② **engine 子包化**——`internal/service/ticket/engine/` 收拢状态机推进+转换校验+扫描器（纯包内整理不改行为），**维持随 B3-1**（S3a 本要动这些文件，顺手零成本）。**边界从纪律变编译器管，自 W1 起。****反面清单（不做）**：不为拆库重写存储抽象（YAGNI）/不做引擎通用化（16 号红线）/S3a·S3b·SLA 三波演进期不做包大重组（跨波重组=同步灾难）。

**① 落地状态（2026-10-19 W1 开工首件 ✅）**：以**自研架构守护测试扩展**落地（internal/architecture_test.go 增 TestArchitecture_PkgNoBusinessDependency[pkg 横切层禁反向依赖业务层——admincli break-glass CLI 文件级豁免] + TestArchitecture_ServiceDomainIsolation[service 根包↔子包互禁直接 import——域间只经 DomainEvent/Port 解耦]）——未引入 golangci-lint/depguard 工具链：既有 guard 框架已在 CI test job 生效（ci.yml `go test ./internal/...` 天然包含 internal 根包守护测试），扩展断言零新增依赖达成同款 CI 强制；验证=正向 5/5 PASS + 金丝雀负向验证两断言均正确拦截 + make lint 过。「engine 不 import 业务外依赖」专属禁单维持随 B3-1 建目录补强（本条已覆盖 engine→其他域主违规面）；② engine 子包化不变。

---

## 附录：工单引擎演进路线（跨段视图，2026-10-16 所有者提议立项——以业界引擎为参照系的统一演进计划）

> **定位**：S2/S3/S6/S10/SLA 各段能力统一到「工单引擎」主线——**现状处于哪个阶段、每级演进的触发事件、量级预估**，一张图收口。触发事件为立项开关（照单触发勿再推演）；各能力的详细设计归各自段稿（本文只做演进视角汇总）。业界参照系 = Jira（workflow→Automation→JSM）/ ServiceNow（Flow→SLA 引擎）——两家演进史与本阶梯同构。

### 引擎能力矩阵（六能力域 × 现状/已排期/触发驱动）

| 能力域 | 现状（已交付） | 下一态（✅ 已排期） | 再下一态（⏸ 触发驱动） | 远期（🚫 不规划） |
|--------|---------------|---------------------|------------------------|-------------------|
| **流程定义** | JSON 源码模式+表格兜底（000010） | **画布可视化**（S2：LogicFlow+mapper+lint+改图告警） | — | BPMN 全家桶（红线不做） |
| **流转语义** | 端点级权限硬编码（Close/Assign 各端点属主判定） | **转换级权限/前置条件/自动分派/动作库**（S3a——2026-10-19 升格 W13–15） | — | 通用流程引擎（红线不做） |
| **审批** | 确认制（风险接受单=人工确认） | — | **多级审批流**（S3b：BranchedStateEngine+会签/驳回/版本快照） | — |
| **事件/提醒** | 无（ticket_events 无消费面） | **DomainEvent 事件面**（S6 B6-1）+ **状态老化 aging**（S10：timeout_minutes+扫描+分级通知） | — | — |
| **协作** | 无（五件零实现） | **cc/标签/角标/导出**（S10） | — | — |
| **承诺计量** | `sla_due_at` 死字段（从不写入） | **SLA deadline 模型 A**（层 0.5——2026-10-19 gap2 确认 W15 随窗） | **完整 SLA**（层 1：pause/日历/升级链/报表——四层路径详录 [10 号 §5](./10-s10-ticket-collaboration.md)） | 平台级 ITSM（永不） |

### 演进触发事件总表（立项开关——照单触发）

| 演进 | 触发事件 | 量级 | 设计出处 |
|------|---------|------|---------|
| S2 画布 | ✅ 已排期（W7.5–9，无触发门槛） | 1–2 周 | [02 号](./02-s2-flow-graph-designer.md) |
| S6 事件面 | ✅ 已排期（W5–7.5 首批 B6-1） | 3–5 天 | [06 号附录 A.1](./06-s6-message-notification.md) |
| S10 协作五件 | ✅ 已排期（W9–11，S6 后） | 1.5–2 周 | [10 号](./10-s10-ticket-collaboration.md) |
| **S3a 语义引擎** | ✅ **已升格排期（2026-10-19 所有者拍「升」）——W13–15**（触发信号不再等待：事件驱动统一模型+phase6 R3 明牌需求） | ~2 周 | 本文件 §3 |
| **S3b 审批流** | **首个真实审批场景** | 2–3 周（workflow_* 蓝本现成） | 本文件 §4 |
| **SLA 层 0.5（deadline）** | ✅ **已排期（2026-10-19 gap2 确认）——W15 随 S3a 同窗顺带**（原触发「固定时限承诺出现」被排期决策取代——deadline 计量 pre-phase6 铺垫） | 几小时–1 天（phase6 R2 deadline 蓝本现成；sla_due_at 复活） | [10 号 §5](./10-s10-ticket-collaboration.md) 四层路径 |
| **SLA 层 1（完整）** | **硬触发五条任一**（对外承诺/停表语义[分水岭]/业务日历/达成率 KPI/自动升级链）；软三条两条以上评估 | 1–2 周自研（无现成可嵌入引擎——JSM policy 模型借鉴） | [10 号 §5](./10-s10-ticket-collaboration.md) |
| **升级链 escalation** | 超时自动转派诉求（人工处理跟不上量） | ~1 周（**与 S3a 动作库天然结合**：ops.cron_failed/超时 DomainEvent → 动作库「转派」动作——不独立建链） | 本文件 §3.3 + [10 号附录 A.5](./10-s10-ticket-collaboration.md) |
| **表单条件显隐**（交互层联动） | 业务方提出「选了 A 才填 B」类联动诉求（业界表单设计器标配——唯一显式不做的能力边界） | ≈2–3 天（复用 S1/S3 expr 公共件——前端渲染层显隐+必填动态切换+服务端兜底断言；零迁移零结构改动） | [01 号 §5](./01-s1-form-designer.md) |
| **多源内容采集**（微信公众号等订阅/推送源） | ✅ **业务意图已声明（2026-10-19 所有者）**——桥接方案拍板即立项 | MVP ≈1–1.5 周（轮询桥接+HTML 消毒+入库列表+通知复用；图片转存+全文检索 +1 周） | **归属 phase6 新段**（09 号不迁移——查询代理 pull ≠ 推送采集 push 两形态）；形态=**通用数据录入接口**：统一信封（source/type/payload）+按源 schema 注册校验（防「通用=无 schema」dump 陷阱）+服务间 AK/SK HMAC 鉴权[standards §4]+raw 报文留档（对账/重放）+DomainEvent 出站；前置=桥接方案+phase5 S5/B6-1/S4 底座（W13 前全齐）；拉取侧 http-pull / db-pull / file-pull + `ingest_sources` 水位表（推 / 拉皆备——所有者 2026-10-20；2026-10-20 回灌） | [09 号 §6 留口注](./09-s9-threat-intel-cache-proxy.md) |

### 事件驱动统一模型（2026-10-19 所有者架构提议采纳——引擎扩展的统一形态）

**提议**：工单状态变化触发 DomainEvent，**通知/升级转派/计划任务统一为事件的消费者或生产者**——业界收敛形态（Jira Automation trigger→condition→action / ServiceNow Flow / Salesforce Platform Events；Jira 自己即从分散 notification scheme+listener 演进到统一 Automation）。**底座已就位**：DomainEvent（BizType 路由）+ message_rules（事件→动作规则表）+ S3a 动作库——提议实质 = DomainEvent 从「通知触发器」升格为「全引擎统一扩展模型」：新能力=新订阅者，内核零改动。

**三分架构（一致性等级显式分层——事件统一≠全部异步）**：

| 消费者/生产者 | 通道 | 一致性等级 |
|--------------|------|-----------|
| 消息通知 | S6 规则面（message_rules） | 异步 at-most-once（可补发不阻塞） |
| **升级/转派**（状态变更写操作） | **S3a 动作库**（DomainHook 转换后同步动作，事务内） | **同步强一致**——挂 at-most-once 钩子会丢+重复消费=重复转派；可靠性升级路径=transactional outbox（1–2 天 DomainHook 零改，§5.10 既评） |
| 计划任务（超时扫描） | 事件**生产者**（扫描器=把时间条件转换为事件的适配器） | 扫描幂等+事件查重 |

**执行端点显式化（P0-1，2026-10-19 并行外审合并报告——B3-1 首件交付）**：`POST /tickets/transition`（body: to + comment 可选）——运行时转换现状仅三条路（Create 硬编码 open/Close→closed/Assign 推算 open↔assigned——router.go:344-352 实证，无 submit 端点），画布可配多状态图但无泛化执行落点；本端点=校验（三层鉴权+AssertTransition 叠加+validator）→ **单事务执行**（状态写+status_changed_at 同事务——写点终版锚 `UpdateStatusTx` 咽喉，10 号 §3.4 二轮修正）→ **commit-后回调**（DomainEvent emit 锚+TransitionAction post-function 触发——P0-2 修正后的正确挂点）；现 Close/Assign 定向端点收敛为其封装（Create=创建非转换，保留独立端点、commit 后 emit `ticket.created`；Update 非 status 端点不涉收敛）（对齐 Jira transition 端点形态）。**L1 权限归属随 B3-1 拍**：menu_apis 声明式注册对账（挂既有工单操作菜单 / 新权限码二选一——转换级 condition 细粒度由本段 L2/L3 兜底，L1 只做粗粒度入口）。

**⚠ 升级链通道裁定（2026-10-19 外审发现自相矛盾，修法拍定）**：升级转派是写操作需强一致，但"超时 DomainEvent → 转派" rides at-most-once 派发=事件丢失即转派静默丢失——**违反自家分层**。修法：**转派由超时扫描器事务内直调**（扫描批事务=强一致通道；扫描幂等/查重已有），DomainEvent 仅作通知面；outbox（1–2 天）登记为扫描器形态不可行时的备选。**同根裁定（外审 P2-4）**：TransitionAction 框架补**两种触发形态**——①转换后形态（post-function，事务=转换事务）②事件订阅形态（超时/升级链等无转换上下文，事务=触发方扫描批事务）——两形态同一接口、事务边界随触发方，B3-3 框架按双形态设计。

**防蔓延边界两条**：①动作实现分层——**代码注册为主（DomainHook 编译期）、配置规则限于通知/收件人**（不做全可配 Automation 引擎：规则冲突/顺序/循环治理成本）；②**循环触发防护**注记（事件→动作→状态变更→事件需链深限制——**落点=B6-1/B10-4/B3-3 三处**[2026-10-19 外审补：动作库是升格后最大新事件面，post_actions 链深限制随 B3-3 框架落]）。

### phase6 前核心引擎目标态（2026-10-19 gap 分析——所有者问"phase6 前能否做到规范完整的核心引擎"）

**判据**：核心能力齐（可配置/事件面/语义引擎/提醒计量），外围等触发。**gap 两项**：

| gap | 现状 | 升格代价 | 理由 |
|-----|------|---------|------|
| **gap 1：S3a 语义引擎** | ✅ **已拍升格（2026-10-19 所有者「升」）——W13–15 排期** | **~2 周** | 论据=事件驱动统一模型（转派动作需同步通道）+phase6 R3 明牌需求 |
| **gap 2：SLA 层 0.5（deadline）** | ✅ **已确认排期（2026-10-19 所有者）——W15 随 S3a 同窗顺带**（不占独立 Wave） | 0.5–1 天（R2 蓝本现成） | phase6 风险整改 deadline 诉求大概率——排期后「核心引擎目标态」时点锁定 ≈W15 |

**结论（2026-10-19 两 gap 均拍定）**：gap1 已拍（S3a 排期 W13–15）+ gap2 已确认（层 0.5 随 W15 同窗顺带，0.5–1 天）= phase6 前达成「规范完整核心引擎」且**时点锁定 ≈W15**：可视化配置/事件面/语义引擎（转换级权限+动作库）/aging/deadline 计量全齐；**S3b 审批维持触发驱动**（无场景不建——easy-workflow 教训，骨架蓝本现成，场景出现 2–3 周可加不构成缺口）。~~决策项=所有者：S3a 是否升格排期（+2 周）~~（已拍「升」）。

### 演进纪律（三条，防两个极端）

1. **防过早**：触发未到不建（无消费者不建引擎——easy-workflow 教训；S3a 绕行禁令、SLA 层 0.5 前不碰层 1）；
2. **防撞墙**：地基已预埋（DomainEvent 事件面=语义引擎与 SLA 的共享底座/`status_changed_at`=SLA 事件时钟/S2 适配层=画布可替换）——撞墙时增量演进不推翻；
3. **防漂移**：每级触发事件照单立项、量级照单预估——本表为演进 SSOT，新触发信号经评审入表。

---

---

## 变更记录

| 日期 | 变更 |
|---|---|
| 2026-10-15 | 建档（phase5 分支十段设计稿批）——现状锚点逐段代码核实；phase3/10 蓝本转录；未新增决策 |
| 2026-10-15 | 公约对表批：§4 补 S3b 实施换算注——蓝本（08-25 成文，早于 §3 公约建纲）内 `PUT /workflows/:code` 等端点形态实施时按 standards §3-1/§3-2 换算，照抄即违公约 |
| 2026-10-16 | 时点决策批（所有者问询"phase6 前要不要升级引擎"）：维持条件段（方案 A+C）——phase6 MVP 不依赖 S3a，后插代价评估=叠加式语义层/数据零迁移/周边在位（详录 §3 头注）；**绕行禁令**（撞墙需求记账等 S3a，禁另造状态/端点/硬编码）+ 显式触发信号（phase6 R3 首个转换级权限需求，预期撞墙点=复核态）登记 |
| 2026-10-16 | **工单引擎演进路线批（所有者提议：aging/SLA/S3 统一引擎视角规划）**：新增「附录：工单引擎演进路线」——六能力域矩阵（流程定义/流转语义/审批/事件提醒/协作/承诺计量 × 现状/已排期/触发驱动）+ **演进触发事件总表**（八级演进逐条：触发事件/量级/设计出处——S3a=phase6 R3 转换级权限需求、S3b=首个审批场景、SLA 层 0.5=固定时限承诺、层 1=硬五条任一、升级链=自动转派诉求[与 S3a 动作库结合不独立建]）+ 演进纪律三条（防过早/防撞墙/防漂移）——业界参照系=Jira workflow→Automation→JSM 与 ServiceNow 同构 |
| 2026-10-19 | **SLA 边界溯源修正批（随 10 号同款）**：§1「SLA 引擎不建（dd §29 定论）」修正——§29 原文仅方向性一句，细分边界=设计推演；触发信号在案 10 号 §5 |
| 2026-10-19 | **事件驱动统一模型+phase6 前核心引擎目标批（所有者架构提议：状态变化触发事件，通知/升级转派/计划任务统一为事件消费者或生产者）**：03 号演进路线新增两节——①**事件驱动统一模型**（业界收敛形态 Jira Automation 同构；三分架构=通知走 S6 规则面 at-most-once/转派走 S3a 动作库同步事务内/扫描器=事件生产者——一致性等级显式分层；防蔓延两条=代码注册为主不做全可配+循环触发防护注记）②**phase6 前核心引擎目标态**（gap 两项：S3a 升格 ~2 周+SLA 层 0.5 0.5–1 天≈15 周达成；S3b 维持触发驱动不构成缺口；**决策项=所有者：S3a 是否升格排期**） |
| 2026-10-19 | **S3a 升格排期批（所有者拍「升」）**：头注段属性（条件段→排期段 W13–15、纳入 phase5 收官；S3b 维持触发驱动）/§1 溯源修正联动/§3 头注时点决策更新（触发信号不再等待——A+C 方案被升格决策取代，绕行禁令 W13 前仍有效）/附录演进路线矩阵与触发总表与 gap 表三处 S3a 状态翻 ✅已排期 |
| 2026-10-19 | **S3a 附录 A 详细技术设计批（升格排期后按排期段标准补齐——05 号样板深度）**：A.1 **Jira transition 四段对表**（condition=转换级权限/validator=前置条件/post-function=动作库——20 年业界解剖照表设计不漏项）+ transitions_meta 独立列（对齐 01 号 A.2 方案 b）+ available 端点（前端过滤 UX+服务端兜底双重）/A.2 assignment_rules DDL（五决议既定）/A.3 动作库框架（TransitionAction 编译期注册+失败不回滚）/A.4 **easy-workflow 对照开发法**（所有者提议"参考修改补强"的操作化——fork 改造≈2–3 周四层重写>对照自研 2 周，参考实现=免费错题本）/A.5 验收补充 |
| 2026-10-19 | **引擎独立性评估批（所有者问"工单引擎是否拆独立仓库"）**：附录 A.6 新增——**现在不拆**（判据=跨仓消费方≥2，项目第三次同判据同答案：FormDesigner/utils/本次同构）；引擎耦合面四条（策略库/DomainEvent 订阅者/assignment_rules 表依赖/动作库业务语义——拆=连带重写）；演化期纪律（三波演进跨仓同步负收益，modular monolith 内部领域隔离=既定架构「画边界非拆」）；**拆分触发信号三条**（第二消费方/部署独立/团队拆分）+phase5 准备（引擎件聚类纪律+域间禁表直查） |
| 2026-10-19 | **S3a 详设外审深修批（外审 P1×3+P2×4+P3×2 全修）**：**P1-1 升级链一致性自相矛盾修法拍定**=转派由超时扫描器事务内直调（扫描批事务=强一致通道）+DomainEvent 仅通知面+outbox 备选登记（事件驱动统一模型节裁定）；**P1-2 meta 配置面归 B3-1**（类型管理页扩展——02 号画布范围结构性不覆盖独立列+A.5 验收主语改 B3-1+A.5 加界注）；**P1-3 通知双通道分工**=流转通知收敛走 S6 规则面（notify 动作降无规则兜底——§3.3 修正）；P2-4 框架补两触发形态（转换后/事件订阅——事务边界随触发方）；P2-5 expr 字段引用约定（内置=标识符直引/动态=map 索引+与 S6 slug 统一随 B3-1）；P2-6 补偿事件定案（ops.action_failed+消费方 ops 告警规则+payload）；P2-7 循环防护落点补 B3-3；P3-9 assignment_rules 管理面归 B3-2；03 号 A.7 界注+变更记录批行 |
| 2026-10-19 | **边界工具化时机前移批（随 11 号同款，所有者拍「按建议修改」）**：A.6 画边界补强两件拆开排——工具化前移 W1 开工首件（CI 自第一行代码生效）/engine 子包化维持随 B3-1（顺手零成本） |
| 2026-10-19 | **timeout 承载修正级联（随 10 号结构修正批）+ 两扩展触发登记（外审 P3）**：A.1 DDL 注列交付时点（B10-4 先建 transition_meta、B3-1 扩展键复用——勿重复建列）+A.5 界注修正（上轮「transitions JSONB 内既有 meta」假设错误已改）+condition 字段值形态/assignment_rules 字段值分派两处扩展触发登记 |
| 2026-10-19 | **transition_meta 口径级联收尾批（外审复验补——头部量级行/§6 影响面行/§7 依赖行三处）**：①量级/迁移行「+ transitions_meta 一枚 ALTER」→「**列复用 S10 B10-4 已建，零新 DDL**」（对齐 A.1 列交付时点注——消双建列歧义）②§7 依赖行「画布/设计器承载 meta 配置面」→「meta 配置面=本段 B3-1 类型管理页扩展自带」（P1-2 同族残留——S2 画布不承载，02 号 §3 界注） |
| 2026-10-19 | **第二意见体检回声批（并行外审报告——本级 3 处）**：①naming：头注量级行/§6 影响面行/A.5 界注 `transitions_meta`→**`transition_meta`**（对齐 10 号 A.3 DDL 权威单数——全仓名统一随本批）②**附录排序归位**（A.5 验收补充移至 A.6 之前——原 A.1→A.2→A.3→A.4→A.6→A.5 逆序，跨节引用[A.5 引 01 号 A.6]不受影响）③A.5 内悬空引用「见 A.7 界注」→「见下方界注」（界注本就在 A.5 内，A.7 节不存在） |
| 2026-10-19 | **条件显隐演进触发登记批（所有者「按方案登记」——随 01 号量级补完）**：演进触发事件总表 +1 行「**表单条件显隐**（交互层联动）」——触发=业务方提出「选了 A 才填 B」类联动诉求 / 量级 ≈2–3 天（复用 S1/S3 expr 公共件，零迁移）/ 设计出处 01 号 §5——演进触发总表至此九级全带量级，照单触发 SSOT 完整 |
| 2026-10-19 | **gap2 排期确认批（所有者确认）+ W0 收口级联**：①gap 表/结论/触发总表/能力矩阵四处——gap2「SLA 层 0.5」✅ 确认排期 **W15 随 S3a 同窗顺带**（0.5–1 天，不占独立 Wave——「核心引擎目标态」时点锁定 ≈W15，决策项销账）②W0 ✅ 收口级联（11 号 §1/§2.1/README §5——详录 11 号） |
| 2026-10-19 | **P0 四项修正批（并行外审合并报告五项实证全证实——本文件 2 处）**：①**A.3 执行端点显式化（P0-1）**：`POST /tickets/transition` B3-1 首件交付——校验→单事务执行（status_changed_at 同写）→commit-后回调（emit 锚+post-function 挂点），Close/Assign 收敛为封装；②P0-2 锚点修正牵连（修正详录 06 号 A.5/10 号 §3.4）——合并报告同时修正本会话此前「emit 锚=AssertTransition」错误口径（深审 C 轮未验证调用位置即提议，认领） |
| 2026-10-19 | **P0 修正批二轮回声（修正批自身精度问题三处）**：A.3 执行端点段——①「submit/close/assign」→「Create 硬编码 open/Close→closed/Assign 推算 open↔assigned」（无 submit 端点，router.go:344-352 实证）②收敛列表去 Update（Update 非 status 端点）+补 Create 保留说明（创建非转换，独立 emit ticket.created）③补 L1 权限归属拍板项（menu_apis 注册对账——挂既有菜单/新码二选一）+status_changed_at 写点指向 UpdateStatusTx 终版锚（10 号二轮修正牵连） |
| 2026-10-19 | **多源内容采集演进登记批（所有者问询「通用录入接口+归属 phase6」——方向确认落档）**：演进触发总表 +1 行「**多源内容采集**（微信公众号等）」——触发=业务意图已声明+桥接方案拍板即立项 / 量级 MVP ≈1–1.5 周 / **归属 phase6 新段（09 号不迁移——pull 缓存代理≠push 采集两形态）** / 形态=**通用数据录入接口**（统一信封+按源 schema 注册校验[防 dump 陷阱]+AK/SK HMAC[standards §4]+raw 留档+DomainEvent 出站）/ 前置=桥接方案+phase5 S5/B6-1/S4 底座（W13 前全齐）——演进总表至此**十一级**全带量级与归属 |
| 2026-10-19 | **W1 开工首件落地批（① 架构依赖规则工具化 ✅——A.6 拍板执行）**：A.6 补落地状态注——以自研 guard 扩展两条断言落地（pkg 禁反向依赖业务层[admincli 豁免] + service 域间横向隔离[根包↔子包互禁 import，DomainEvent/Port 解耦]）；未引 golangci-lint 工具链（CI test job 天然包含、零新增依赖）；正向+金丝雀负向+lint 三验证过；「engine 不 import 业务外依赖」专属禁单维持随 B3-1（详录 11 号同批变更记录） |
