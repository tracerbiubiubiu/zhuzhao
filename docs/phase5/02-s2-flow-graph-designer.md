# 02 — P5-S2 流程状态图可视化设计器（设计稿）

> **段属性**：基线段（phase5 收口对象）。**建议实施序**：S6 错峰衔接（S5→S1 之后，S6 一期收口后进本段——单人串行）；后端零改动。
> **状态**：🔵 **B2-1→B2-4 全批交付（2026-10-24，ui 70e51e77+5414ee79）——待所有者走查翻 ✅**（走查入口见 [11 号 §0 S2 行](./11-implementation-plan.md)；真机走查记录见本稿变更记录 2026-10-24 行）。
> **量级**：1–2 周。**迁移**：零。
> **文档定位**：段级设计稿（2026-10-15 建档，phase5 分支）。范围/边界总纲 = [README §2 P5-S2](./README.md)；选型预置与分层要求 = dd §27（2026-10-08 所有者确认）。

## 1. 定位与由来

状态图本身已"数据可配"（每类型一套 states/transitions JSONB，迁移 000010 起），缺可视化编辑界面——现状 = ui type 页 JSON 源码模式 + 常用模板按钮（phase4 深水区降级兜底形态，[phase3/12 §3.2](../phase3/12-frontend.md)）。本段补"画布"，**只换配置界面，引擎与存储零改动**（§23 口径，不构成自研推进的额外动作）。

## 2. 现状基线（代码级锚点，2026-10-15 核实）

| 项 | 现状 | 锚点 |
|----|------|------|
| 状态机引擎 | 通用引擎，从 `ticket_types.transitions` JSONB 构建，校验 from→to | `internal/service/ticket/state_machine.go` |
| 流程配置数据 | 每类型一套 states/transitions JSONB；**默认六状态图**为列 DEFAULT + 常量双落在 | 迁移 `000010:11-12`；`type_admin.go:19-20`（`defaultStates`/`defaultTransitions`） |
| 图校验 | `validateTypeGraph` 仅查转换端点在 states 内（端点合法性，无 lint 级检查） | `type_admin.go:39` 调用处 |
| 管理端 API | 类型 CRUD（states/transitions 可写）+ 乐观锁，管理端点组在位 | `internal/router/router.go:363-377` |
| 前端 | ticket/type 页 states/transitions = JSON 源码模式 + 模板按钮 | zhuzhao-ui `views/ticket/type/index.vue` |

## 3. 目标形态与范围

### 3.1 画布选型（启动首件）

- **采用现成拖拽画布组件，不自研画布**。✅ **已拍（2026-10-15 所有者）：LogicFlow 优先**——启动批**各 0.5 天同一 PoC spike**（拖节点/连线/序列化回 transitions/回显）降为**对称验证**（vue-flow 同日对照），按映射成本 + 交互手感 + 包体终定实现；LogicFlow 依据 = 编辑器开箱能力（网格/对齐/锚点/JSON 序列化更贴状态图模型；判据 = transitions 映射代码量/自定义节点成本/中文交互/bundle）。
- AntV X6 备选（能力过剩）；**bpmn.js 排除**（BPMN 2.0 标准模型与 transitions JSONB 不匹配且过重）。
- 数据面：**状态=节点、转换=边，与 transitions JSONB 天然同构**，映射器为浅层翻译。

### 3.2 画布化与分层（2026-10-08 所有者问询后定）

- states/transitions 画布化：节点=状态、连线=合法转换，产物序列化回现有 transitions JSONB，**后端零改动**；中文化与交互打磨计入量级。
- **分层要求**：画布层（LogicFlow/vue-flow 封装+交互）与 schema 适配层（transitions 映射器）**分离**——任务域将来若立流程设计器可复用画布层、只换适配器。
- **界注（2026-10-19 外审 P1-2）**：本段画布范围=states/transitions 邻接表编辑+后端零改动——**transition_meta 独立列（S3a：condition/validator/post_actions）的编辑不在本段**（独立列经独立端点，画布序列化只回 transitions JSONB 结构上不可达）；S3a B3-1 交付类型管理页 meta 配置面后，画布二期可扩展（触发驱动）。
- **引擎与数据模型不合并**：工单流程 = 人触发的实体状态图，任务流程 = 完成事件驱动的执行编排，**不同构**——任务侧编排维持 [phase3/16](../phase3/16-external-integration.md) 红线（不做通用流程引擎；轻编排 = zhuzhao 回调串联、重编排 = 外部 DAG [phase3/15](../phase3/15-script-platform-dagu-vs-inhouse.md) GPL 法务前置），**不得为复用而把两者捏成"通用流程模块"**。

### 3.3 可选校验增强批（纯增量、向后兼容）

- `validateTypeGraph` 现只查端点在 states 内；可加 **lint 级检查**：终态可达性 / 孤立状态 / 无出边死锁告警——**是否阻断保存待启动拍板**。
- **改图影响在途单告警（必做）**：保存图时扫描在途工单状态分布，存在将脱离新图的状态（改后 `AssertTransition` 恒拒 = 工单僵死）→ 告警确认。沿革：2026-10-08 对标 Jira 在途迁移取舍（迁移向导过重不取）；2026-10-09 第四线建议升级为必做，**采纳**——成本半天、防"改图后在途单僵死"实险。

### 3.4 工单高级搜索（触发驱动挂账，不入本段基线）

keyword 现为 title ILIKE + 纯数字 ID 精确匹配（`ticket_repo.go` ListTickets 条件构造，2026-10-15 核实在位），custom_data 自定义字段筛选缺失是台账类使用的硬伤——**诉求出现时启用**（PG JSONB 谓词，量级与检索形态启动批评估）。登记位 = README §2 S2 段；不在本段量级内。**姊妹挂账**：全局搜索（跨实体快找，2026-10-16 用户提出）登记于 [README §5](./README.md)——本条=工单域内深筛、彼条=跨域快找，互补不混；彼条实现载体建议 S10 导航层，不依赖本条。

## 4. 启动拍板点

| # | 事项 | 建议 |
|---|------|------|
| 1 | 画布选型 | ✅ **已拍（2026-10-15 所有者）：LogicFlow 优先**——spike（vue-flow 同日对照，各 0.5 天）降为对称验证 |
| 2 | lint 增强是否阻断保存 | 告警不阻断起步（告警确认后可保存），阻断收益出现再收紧 |

> **状态注（2026-10-16）**：本表拍板点已随批量预确认拍定（2026-10-16 所有者「按建议」——✅34/🔶1，状态 SSOT = [11 号 §4](./11-implementation-plan.md)；各段启动时异议可翻，翻案成本=改一行）。

## 5. 边界（不做）

- 不自研画布；不做 BPMN 模型适配；不做通用流程引擎/BPMN 全家桶（README §3——16 号红线同构适用工单侧）。
- 任务域流程设计器不随本段建（分层已预留复用缝，触发驱动）。
- 后端零改动 = 不新增迁移/端点（改图在途单告警为既有保存链路内的校验增强，非新资源面）。

## 6. 影响面

- **迁移/权限/配置**：零（纯前端 + 既有类型管理权限）。
- **前端**：`views/ticket/type` 增画布模式（与 JSON 源码模式并存——源码模式保留为专家兜底，与 S1 专家模式同哲学）。
- **公共件**：transitions 映射器（画布层/适配层分离，任务域可复用画布层）。

## 7. 实施序与批次建议

S6 错峰衔接（互不依赖，S6 一期收口后进本段）。批次：①选型 spike（首日）→ ②画布 + 映射器往返（序列化/回显）→ ③改图在途单告警 → ④lint 增强（可选）+ 中文化交互打磨。依赖：无硬前置（S5/S1 非阻塞，仅排期错峰）。

## 8. 验收口径

- zhuzhao-ui CI + E2E + 场景走查。
- 场景标尺：画布拖建六状态图 → 保存 → 重建页面回显一致（序列化往返幂等）→ 在途单覆盖改图告警触发 → 存量类型 JSON 模式与画布模式互切无损。
- 回归：默认六状态图类型零感知；`AssertTransition` 运行时行为零变化（后端零改动的断言）。

## 9. 文档衔接

| 文档 | 关系 |
|------|------|
| [README §2 P5-S2](./README.md) | 总纲（选型预置表/触发驱动挂账登记） |
| [design-decisions §27](../design/design-decisions.md) | 选型具体化与分层决策登记 |
| [phase3/12 §3.2/§6](../phase3/12-frontend.md) | 现状形态出处；§6 画布远期项随本段转正 |
| [phase3/15 / 16](../phase3/16-external-integration.md) | 任务域编排红线（引擎不合并的边界依据） |
| [01-s1](./01-s1-form-designer.md) | 专家模式共存哲学同源；字段变更影响分析与改图告警同型件 |

---

## 附录 A：详细技术设计（实施级，2026-10-16 补充——样板深度对齐 05 号附录 A）

> ⚠ 标注 = 实施级设计建议（启动批确认制，不新增拍板点）。

### A.1 关键事实：transitions 存储形态 = 邻接表（非边数组）

```json
{"open":["assigned","closed"],"assigned":["in_progress","open"],"in_progress":["pending_verify","rejected","closed"],"pending_verify":["closed","in_progress"],"closed":["open"],"rejected":["open"]}
```

（`type_admin.go:20` defaultTransitions 实证）——映射器核心 = **邻接表 ↔ 画布 graphData 双向转换**：schema→画布 = states 数组逐个建节点 + 邻接表展开为边（from→to 各建一条）；画布→schema = 边列表按 source 折叠回 `map[from][]to` + states 收集。**双向均为浅层纯函数**（选 LogicFlow 的核心理由在此兑现）。

### A.2 模块架构与代码落点

```
zhuzhao-ui/
  src/components/FlowGraphDesigner/     # 画布层/适配层分离（§3.2 纪律）
    index.vue                           #   壳：LogicFlow 容器 + 工具栏（加状态/连线/删除/保存）
    mapper.ts                           #   唯一适配层：邻接表 ↔ LogicFlow graphData 纯函数（单测承重）
    nodes/StateNode.ts                  #   自定义节点（状态名 + 终态/起始态语义样式）
    lint.ts                             #   前端图 lint：可达性 BFS/孤立节点/无出边死锁（B2-4）
  src/views/ticket/type/index.vue       # 画布模式 tab 与 JSON 源码模式共存（同一 Pinia store 单一数据源）
```

- 随行件（2026-10-20 规划研究回灌，依据 deliverables/software-company/secops-platform-vision-2026-10/08-phase5-adjustments.md）：壳层 `FlowGraphDesigner/index.vue` 数据经 props/emit，不直连工单 Pinia store——为 phase10 playbook 设计器复用画布层预留；不越本稿"不合并通用流程模块"红线。

- **后端零改动**兑现：产物序列化回既有 `PUT /ticket-types/:code`（states/transitions + version CAS），无新端点/迁移。
- **节点坐标不入库（⚠ 设计决策）**：transitions JSONB 无 x/y，布局数据是**本地视图偏好**——存 localStorage（按 type_code），重开无缓存时 dagre 自动布局；不为此加 type 级 layout 列（守住"后端零改动"承诺，多端打开各自动布局可接受）。

### A.3 映射器契约（mapper.ts，往返幂等承重）

```ts
toGraph(states: string[], transitions: Record<string,string[]>): GraphData
fromGraph(g: GraphData): { states: string[]; transitions: Record<string,string[]> }
```

- 边去重（邻接表展开天然去重；画布重复连线前端禁 + mapper 兜底 Set）；孤立边（端点不在 states）→ 保存前拦截 + 高亮定位；
- **校验双保险**：前端 `validateTypeGraph` 镜像预检（端点合法性，规则与后端单测对齐）→ 后端既有校验兜底；
- **往返幂等断言**：默认六状态图 toGraph→fromGraph 深比较为零 diff（E2E 承重用例 §8）。

### A.4 改图在途单告警（B2-3，必做件实现口径）

- 链路：保存前 → `GET /tickets?type_code=X&status=S` 逐状态拉 total（既有能力，**零后端改动**）→ 新图 states 集合不含的在途状态 → 命中则弹确认框（列出受影响状态与单量，"仍要保存/取消"）；
- 兜底安全网：告警被绕过（API 直调）时 `AssertTransition` 照旧拒绝——**不产生僵死脏数据，告警纯体验层**；
- ⚠ 可选增强（启动批拍）：后端聚合端点 `GET /ticket-types/:code/inflight-summary` 一次返回分布（前端 6 次请求并 1 次），量级 +0.5 天，非必做。

### A.5 lint 增强（B2-4，可选批）

- 规则：① 终态可达性（每个非终态存在到终态路径，BFS）；② 孤立状态（无入边且非起始态）；③ 无出边死锁（非终态零出边）；
- **告警不阻断**（拍板点 2 建议）：画布保存时角标列出 + 点击定位节点；阻断收益出现再收紧（配置开关预留）；
- 中文化：LogicFlow 文案包 + 自定义节点文本——量级计入 B2-4。

### A.6 验收补充（在 §8 基础上）

- 往返幂等：默认图 roundtrip 零 diff；自定义图（加状态+跨层连线）保存重开一致；
- 告警：在途单覆盖改图 → 确认框弹 出（受影响状态/单量正确）→ 取消不保存/仍保存后旧状态单转换被拒（AssertTransition 断言）；
- lint：构造孤立节点/死锁图 → 角标告警定位（阻断开关关闭时可保存）；
- 多端布局：清 localStorage 重开 → 自动布局渲染不丢图数据（坐标非数据断言）。

---

## 变更记录

| 日期 | 变更 |
|---|---|
| 2026-10-15 | 建档（phase5 分支十段设计稿批）——现状锚点逐段代码核实；拍板点带建议值；未新增决策（全部收编自 README 总纲与设计源） |
| 2026-10-15 | W0 前置拍板批（所有者逐项确认）：✅拍板点 1 画布选型 = **LogicFlow 优先**（vue-flow 同日对照 spike 降为对称验证）；拍板点 2（lint 是否阻断）维持待启动拍 |
| 2026-10-16 | **详细技术设计批（附录 A，实施级——05 号样板深度）**：A.1 **关键事实落档——transitions 真实形态 = 邻接表**（defaultTransitions 实证，映射器=邻接表↔graphData 浅层纯函数，LogicFlow 选型理由在此兑现）/A.2 模块架构（mapper.ts 唯一适配层+**节点坐标不入库**——localStorage 本地视图偏好，守后端零改动承诺）/A.3 映射器契约（往返幂等+校验双保险）/A.4 在途告警实现口径（逐状态 total 零后端改动+AssertTransition 兜底安全网+可选聚合端点）/A.5 lint 三规则实现/A.6 验收补充 |
| 2026-10-19 | **第二意见体检回声批（并行外审报告——naming 1 处）**：§3 界注 `transitions_meta`→**`transition_meta`**（对齐 10 号 A.3 DDL 权威单数——全仓名统一随本批） |
| 2026-10-24 | **S2 实施批（B2-1→B2-4 全批交付，zhuzhao-ui 70e51e77+5414ee79 CI 面待跑；段状态 🔵 待所有者走查）**：①**B2-1 spike 对称验证落定**——LogicFlow 2.2.5 维持（vue-flow 1.48.2 对照：包体优势 ~1/3 且 Vue 原生，但连线 Handle/标签编辑/校验回调须自写，状态图编辑所需锚点连线/双击改名/网格对齐/键盘删除/JSON 序列化 LogicFlow 开箱即有；映射器两者同为浅层——判据 A.3 契约两者皆 ~40 行纯函数）；实装代价 = type 路由 chunk 75.7→447 KB（gz 24.5→119 KB，懒加载隔离 admin 页）②**实施级偏差四件（均登记不改设计）**：a. 自动布局 **dagre→自写 BFS 分层**（A.2 原文 dagre——≤十余节点分层 BFS 三十行足够，免引 100KB+ 依赖；坐标为本地视图偏好不入库，质量只需"可读"）；b. `fromGraph` **每状态出键**（零出边=[]，与后端默认图"六态皆键"同构——A.3 未指定取自文档化）；c. **stuckStates 判据扩展②**（A.4 原文只列删态；非终态零出边=同一僵死实险的自然延伸——进入后 AssertTransition 恒拒）；d. 画布状态名限 `^[A-Za-z][A-Za-z0-9_]*$`（JSON 源码模式不受限——防空格/中文进 status 码）；连接规则加禁自环 ③**真机走查（dev 栈 E800001 走查号，临时类型 s2_walk_demo 已清理）**：画布渲染六态默认图（角色配色 open 绿/closed·rejected 灰）→ 加状态+双击改名 escalated → 锚点拖连线 in_progress→escalated → 重复连线拦截（「该转换已存在」toast）→ 图检查角标实时 2→1（孤立清/死锁在——规则逐条验证）→ 画布↔JSON 双向同步无损 → **带 1 条 lint 告警保存成功（告警不阻断实证——E-3 拍板）** → 重开回显一致 → **B2-3 在途告警实证**（造 1 张 open 态单→清空 open 出边保存→弹「open·待处理（1 单）…确认仍要保存？」→取消→后端数据零变化）④验收证据：ui lint/typecheck/build 绿 + 单测 158（新增 mapper 14+lint 7——**默认六状态图往返幂等零 diff 承重用例**）；E2E 未本地跑（环境漂移在册，CI 面覆盖——s12/p4-dict 静态核对无破坏面：新建框未动、编辑框 nth(2) 仍为描述域） |
| 2026-10-26 | **设计器页面化与节点能力对照登记（所有者问询「弹大页面编辑 / 参考 ficus 节点类型 / 模板与类型区别」——登记级不排期）**：①**编辑形态**：现状=类型页 980px 弹窗内画布（本段交付形态，待走查手感未验）；独立设计器路由页（业界同构：ficus ProcessManage→ProcessDesign 独立页、钉钉/飞书审批设计器全屏、Jira workflow 独立页）**此前未规划**——若走查后采纳，建议并入 B-21（转换级配置面，phase6 启动批随行）一次做成「类型列表 → 设计器页」（字段/流程分区同页或分页，弹窗只留新建基本信息；画布区内部滚动适配 ui 视口铁律；新页面须配种子或常量路由过 viewNames 守卫）②**节点类型对照**（ficus GRAPH.md 六类 start/activity/exclusive/parallel/inclusive/end）：「判断」语义 zhuzhao 已由**多出边+转换级 condition/validator**（B3-1 引擎侧已交付，缺配置面=B-21）覆盖——**不引入 BPMN 网关节点**（§3.2 不同构拍板维持，bpmn.js 排除既定）；parallel/inclusive/会签=S3b 审批引擎触发域不入画布；ficus 前置/后置任务≈post_actions（转换级）、deadline≈B10-4 节点超时催办（state 级）、抄送≈cc 动作——已有对应物；③**真实差距两件登记（ficus/钉钉有、zhuzhao 无）**：节点级指派（图内每节点配审批人 vs zhuzhao 全局 assignment_rules+transition roles——phase6 前置件「S3a 指派轴扩充」属同族增强但落点仍在规则面）与节点级表单权限（每节点哪些字段可编辑 vs 类型级 schema 全程同权）——触发驱动评估（phase6 R3 工单联动若需「按节点控编辑面」再启）；④**「开始/结束/判断」节点可加性结论（2026-10-26 同问补查 ecmdb[Duke1616/ecmdb-web]——同为 LogicFlow 栈，其 NodePanel 分类面板[control 开始结束/business user·automation·chat/gateway 条件·并行·则并·包容]+PropertySetting 右侧属性面板+每节点类型独立属性组件可直接参考移植）**：分三档——**A 档纯画布装饰零后端**：初始态徽标（推导自 open 常量）、终态样式升级、多出边条件摘要边标签；**顺手修隐性缺口：终态判定现为前端写死 `TERMINAL_STATES=['closed','rejected']` 名单**（FlowGraphDesigner/types.ts——自定义状态无法标终态，lint 死锁判定同基于此名单），建议随 B-21 改 state 级 meta 可配（transition_meta state 级键加 terminal 布尔，validateTypeGraph 放行，零语义变化）；判断的菱形网关可选做**虚拟节点装饰**（序列化时折叠回多出边，mapper 往返幂等补测）；**B 档=B-21 边属性面板**（点连线配 condition/validator/post_actions——Jira transition 条件同构，ecmdb edgeProperty 同款交互）；**C 档真网关节点类型/并行包容不做**（§3.2 不同构拍板维持，状态机无并行语义，会签/并行归 S3b 审批引擎触发域）；ecmdb 的 automation/chat 显式节点不引入——post_actions 挂转换已交付（模型差异非能力缺失） |
