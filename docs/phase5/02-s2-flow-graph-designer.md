# 02 — P5-S2 流程状态图可视化设计器（设计稿）

> **段属性**：基线段（phase5 收口对象）。**建议实施序**：S6 错峰衔接（S5→S1 之后，S6 一期收口后进本段——单人串行）；后端零改动。
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
