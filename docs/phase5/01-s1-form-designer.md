# 01 — P5-S1 动态表单可视化设计器与字段自动化（设计稿）

> **段属性**：基线段（phase5 收口对象）。**建议实施序**：第二棒（S5 底座先行——设计器调色板的富文本类型依赖 S5 预签名通道）；计算字段/取数联动件可后移、不阻收口。
> **量级**：1.5–2.5 周（设计器 1–2 周 + 字段计算/联动 3–5 天；求值器为 S1/S3 公共件）。**迁移**：零。
> **文档定位**：段级设计稿（2026-10-15 建档，phase5 分支）。范围/边界/量级总纲 = [README §2 P5-S1](./README.md)；规格源 = [phase3/12-frontend §3.1](../phase3/12-frontend.md)（DynamicFormField 七字段类型 +「阶段 2 设计器」——随本段转正）；立项决策 = [design-decisions §27](../design/design-decisions.md)。

## 1. 定位与由来

P4-W4 深水区降级决策（设计器复杂度超预期即降级）实际交付了兜底形态：类型/字段/模板管理页可用，但 states/transitions 走 JSON 源码模式、字段走表格全量替换。所有者 2026-10-08 指示**表单要在前端自定义配置**——设计器由"批内深水区风险项"升格为独立段。同日场景补充：字段还要**自动化**（计算字段/条件取数回写），按执行时机拆落点——提交时计算/取数归本段，流转时取数回写归 S3 转换后动作（勿混层）。

## 2. 现状基线（代码级锚点，2026-10-15 核实）

| 项 | 现状 | 锚点 |
|----|------|------|
| 渲染器 | 自研七类型轻量渲染器已交付（W4）；**头注释明示：form-create 库引入决策顺延至本设计器批** | zhuzhao-ui `src/views/ticket/create/index.vue:3-8` |
| 字段类型 | 七种白名单：input/textarea/number/date/select/multi_select/tips | zhuzhao `internal/service/ticket/type_admin.go:24`（`validFieldTypes`） |
| 字段保存 | **全量替换**：事务内先清后插 + 父类型 `version` CAS；**对存量 `custom_data` 零检查** | `internal/repository/ticket_repo.go` `ReplaceTypeFields`（≈:786 起） |
| 字典（P4-3 已交付） | `/dicts` + `/dict-items` 管理四端点 + 用户侧 `EnabledItems` 读端点 | `internal/router/router.go:199`（EnabledItems）、`:320-325`（管理组） |
| 字典删除 | dict_types/dict_items **无守卫硬删**（items 级联随删）——挂接字典键后成引用完整性缺口 | `internal/repository/dict_repo.go` `DeleteTypeByCode`（≈:89 起） |
| 详情回显 | custom_data 仅 create 页提交，**detail/list 页零渲染**（"填了看不见"台账硬伤） | zhuzhao-ui `views/ticket/`（grep 仅 create/index.vue:143 命中） |
| 专家模式 | ticket/type 页表格模式 = phase4 深水区降级决策的兜底形态，保留 | zhuzhao-ui `views/ticket/type/index.vue` |

## 3. 目标形态与范围

### 3.1 可视化设计器（前端为主）

- **启动首件 = 设计器选型拍板**。**推荐自研**：sortablejs 拖拽排序 + 属性面板，产物即 field schema **零映射**；form-create designer 作**半天 spike 对照**——其 rules/options 组件树 JSON 与规范化 schema 不同构，引入需双向映射器 + 面板裁剪到七类型，高级布局能力用不上，**模型税大于成熟度收益**（phase4/02 深水区教训同源）。
- **选型论证深化（2026-10-15 业界对标讨论落档）**：
  - **正论升级——view model ≠ domain model**：业界主流无一把渲染器组件实例树存库——Jira/ServiceNow/Salesforce/Dynamics 均「字段定义（封闭类型枚举）与布局配置分离」；nocobase collection/block 同构分离；[JSON Forms](https://jsonforms.io/docs/uischema) 将其标准化为 JSON Schema（数据契约）+ UI Schema（视图）双层。存储契约三性质：服务端可校验（G2/custom_data 管线依赖）、跨端中立（详情回显/S6 模板引用/S3 条件/S7 AI 填表消费）、纯声明式（rules 的 on/hook/update 为**函数体**，JSONB 不可存且成注入面）。**本仓 schema 已在业界主流侧，差距在配置体验层（= 本段本体），不在模型层**。
  - **生态情报**：Vue3 表单设计器开源面狭窄——formily 2025 起 unmaintained、designable 以 React 为主（Vue3 支持有限）、form-generator 类停更，活跃项唯 form-create——「成熟方案替代」并无更优候选，单点上游集中度反而加分自研。
  - **四路线代价账**：整体引入（designer+rules 入库）≈2–4 周一次性 + 永久映射税 + 服务端开放集校验重做，业界无人走；自研 ≈1–2 周（已入总账）+ 每新类型 1–2 天三处线性扩；SaaS 表单（Jotform/简道云类）内网+IAM 集成不可行；**form-create runtime 作渲染引擎 = 正式备选**（schema→rules **单向投影**、只渲染不保存——无往返漂移，白拿其 validate 数组/control 渲染层能力，≈2–3 天可启用；触发 = 渲染层联动需求变重时重评，本段边界 §5 不变）。
  - **自研落点纪律（先目录后包）**：zhuzhao-ui 内 `src/components/FormDesigner/` 目录域，内部三层分离（拖拽画布交互层/属性面板层/schema 适配层——与 S2「画布层/适配层分离」同纪律）；出现第二仓消费方（如 taskrunner params 动态表单跨仓复用）才评估抽包，现在抽 = 版本发布/依赖对齐/单人维护三笔负收益。
  - **自研独有优势**：设计器预览 = 真渲染器渲染真 schema（所见即所得零偏差——form-create designer 预览的是 rules 效果，与本仓渲染器是两条渲染路径）；**交互打磨（拖拽锚点/占位反馈/面板分组/实时预览）列入批次①验收项而非锦上添花**——业界差距既在体验层，预算前置。
  - **spike 判据更新**（半天，批次①首日）：form-create designer 裁剪到七类型+regex 校验+忽略布局后，实测映射损耗与交互残缺度，对照自研交互补齐成本——若裁剪后映射接近无损则翻案成立，否则维持自研。
- **后端存储不变**：产物仍是 `ticket_type_fields` 字段 schema；字段类型仍限七种；`validateFieldInputs`/G2 schema 校验维持。
- 现 ticket/type 页表格模式保留为**专家模式/兜底**，不拆除（phase4/02 降级决策形态直接复用）。

### 3.2 select/multi_select 选项引用字典（P4-3 衔接）+ 人员自动补全

- 设计器选项支持**挂字典键**而非死枚举，选项变更全系统生效（字典端点已核在位，见 §2）。
- **人员/部门类字段自动补全**（2026-10-09 场景补充）：指派/抄送/参与人选人走既有用户搜索端点；M-HR 对接后（HRFetcher）组织人员数据入本地表，补全零额外对接——依赖 M-HR 触发，**不阻塞本段**。

### 3.3 字段自动计算/联动（2026-10-08 所有者场景补充）

- **① 计算字段**：按已填字段公式自动算派生字段。落点 = 字段 schema 扩展（meta 增 `formula`，**承载于既有 `ticket_type_fields.field_options` JSONB 列**——000010:20，全表唯一扩展列）+ **服务端求值为 SSOT**（提交时后端算入 `custom_data`，前端仅预览展示——防篡改，与现有哲学一致）。
- **② 提交时条件取数回写**：按字段值判断后从数据源取数自动写入字段。落点 = 字段联动规则（数据层自动化，非流程层）；取数走 `DataFetcher` 端口（按 source 类型注册 fetcher——[README §6 对接预留清单](./README.md)）。
- **求值器统一为白名单 Go 侧表达式求值器**（phase3/10 §4.2 既定路线；easy-workflow 表达式拼 SQL + 黑名单防注入为反面教材）；选型预置 = **expr-lang/expr**（MIT/活跃/默认沙箱/自定义函数白名单——README §2 选型表）。字段公式（本段）与网关条件（S3）**共用同一件**。

### 3.4 随行件三件（2026-10-09 隐藏需求核查补遗）

| 件 | 内容 | 量级 |
|----|------|------|
| 字段变更在途影响分析 | 字段保存=全量替换、对存量 custom_data 零检查；引入 formula 与 S6 模板变量引用后，**保存前须做影响面告警**（被 formula/模板引用的字段、存量单含该字段数据 → 告警确认，防"引用字段被删/改 key → 公式与模板静默失效"）——与 S2 改图告警同型 | +0.5 天 |
| 详情页 custom_data 回显 | 现状零渲染（§2 已核）；随渲染器批补详情页按 schema 回显 | ≤1 天 |
| 字典引用守卫 | select 挂字典键后，dict 删除须有守卫（存在引用 schema → 拦截或告警）或拍"禁用不删"使用纪律——org 删守卫不查 org_roles 教训同型 | 挂接批随行 |

### 3.5 新字段类型另立后端批

渲染器 + `validateFieldInputs`/G2 schema 校验同步扩，遵守"**只增不改**"兼容纪律，不混入 S1。（richtext 第八类型随 S5 段落，见 [05 号](./05-s5-object-storage-richmedia.md)。）

## 4. 启动拍板点

| # | 事项 | 建议（README 既定口径） |
|---|------|------------------------|
| 1 | 设计器选型 | **自研**（sortablejs+属性面板）；form-create designer 半天 spike 对照后正式拍（判据详录 §3.1）；form-create runtime 作渲染引擎备选在案（§3.1③，触发驱动） |
| 2 | 取数数据源白名单 | 内部资源 / activelist / 外部 HTTP——外部源回 [dd §26.6](../design/design-decisions.md) 出站决策树 |
| 3 | 取数身份 | 系统身份 vs 发起人代理——**L2 可见性边界**，宁窄勿宽 |
| 4 | 字典守卫形态 | 挂接批补删除守卫（推荐）vs 禁用不删使用纪律 |

## 5. 边界（不做）

- **表单运行时交互层联动不做**（条件显隐/必填动态切换——12 号 §3.1 阶段 1 既定口径，eflow 亦仅 hidden/readonly）；自动化限于提交时计算/取数回写（本段）与流转时动作（S3）。
- 新字段类型不入本段（§3.5）；移动端/响应式不做（12 号 §6）。
- 引擎与存储零改动——**S1 只换配置界面，不构成 §23 自研推进的额外动作**（README §3 不动拍板）。

## 6. 影响面

- **迁移**：零（formula 承载于既有 `field_options` JSONB 列，无新列/新表；启动批若拍加专用列则改判为一枚 ALTER）。
- **权限**：零新增码（管理面沿用既有类型管理权限；取数联动运行期走既有 L2 管线）。
- **前端**：`views/ticket/type` 设计器面板、`views/ticket/create` 渲染器扩展（formula 预览）、detail 回显。
- **端口**：`DataFetcher`（取数源白名单；README §6 清单第 6 行，随本段登记）。
- **公共件**：白名单表达式求值器（S1/S3 共用，选型 expr-lang/expr 建议值）。

## 7. 实施序与批次建议

依赖 S5 底座（富文本进调色板需预签名通道先行）。批次切分：①设计器壳 + 七类型拖拽配置（含专家模式共存）→ ②字典引用 + 人员补全（含字典守卫）→ ③求值器 + formula 计算字段 → ④取数联动（拍板点 2/3 之后）→ ⑤随行三件收尾（影响分析可提前到②后）。**③④可后移不阻收口**；设计器选型 spike 在①首日。

## 8. 验收口径

- 前端件走 zhuzhao-ui CI + E2E 链（phase4 已建）+ 场景走查（phase4/03 模式）；求值器后端件随四档 acceptance。
- 场景标尺：设计器建类型全流程无 SQL → 发起页按 schema 渲染 → 计算字段提交时服务端算出（**篡改提交值被服务端覆盖**的负向用例必测）→ 详情页回显 → 改挂字典键选项全量生效。
- 回归：专家模式（表格）仍可完成同套配置；存量类型/存量单零影响（只增不改）。

## 9. 文档衔接

| 文档 | 关系 |
|------|------|
| [phase3/12-frontend §3.1](../phase3/12-frontend.md) | 规格源（DynamicFormField 七类型；「阶段 2 设计器」随本段转正） |
| [README §2 P5-S1 / §6](./README.md) | 总纲与 DataFetcher 端口预留 |
| [design-decisions §27](../design/design-decisions.md) | 立项与 S1 扩展决策登记 |
| [phase3/10 §4.2](../phase3/10-ticket-business.md) | 白名单求值器既定路线（S1/S3 公共件） |
| [05-s5](./05-s5-object-storage-richmedia.md) | 依赖其底座（richtext 调色板）；字典引用守卫与 S6 模板引用告警同件 |
| [03-s3](./03-s3-flow-semantics.md) | 求值器共用；「流转时取数回写」归 S3 不入本段 |

---

## 变更记录

| 日期 | 变更 |
|---|---|
| 2026-10-15 | 建档（phase5 分支十段设计稿批）——现状锚点逐段代码核实；拍板点带建议值；未新增决策（全部收编自 README 总纲与设计源） |
| 2026-10-15 | 选型论证深化批（业界对标讨论落档）：§3.1 补 view model ≠ domain model 正论（业界四类玩家收敛对照：平台级字段/布局分离·nocobase·JSON Forms 双层）/Vue3 生态情报（formily unmaintained·designable React 为主·活跃项唯 form-create）/四路线代价账（**form-create runtime 作渲染引擎登记正式备选**——单向投影 ≈2–3 天，触发驱动）/「先目录后包」落点纪律（components/FormDesigner 三层分离）/预览一致性优势 + 交互打磨前置为批次①验收项；§4 拍板点 1 补 spike 判据与备选指引——推荐结论不变（自研），spike 照跑 |
