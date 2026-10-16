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

- **① 计算字段**：按已填字段公式自动算派生字段。落点 = 字段 schema 扩展（meta 增 `formula`，**承载于新列 `field_meta` JSONB**——附录 A.2 方案 b：`field_options` 为数组形态（select 选项），混载元数据须改对象形态、破坏渲染器/optionValues/存量行消费方，**2026-10-16 外审对齐后采纳新列方案，正文与附录以此为准**）+ **服务端求值为 SSOT**（提交时后端算入 `custom_data`，前端仅预览展示——防篡改，与现有哲学一致）。
- **② 提交时条件取数回写**：按字段值判断后从数据源取数自动写入字段。落点 = 字段联动规则（数据层自动化，非流程层）；取数走 `DataFetcher` 端口（按 source 类型注册 fetcher——[README §6 对接预留清单](./README.md)）。
- **③ 编辑时预填 auto_fill（2026-10-16 所有者场景登记——phase6 实际场景驱动；触发驱动挂账不入本段基线，消费者 = phase6 R3）**：与 ① 的语义分层——formula = **强制计算**（服务端 SSOT，人工改无效）；auto_fill = **建议预填**（编辑时实时带出、UI 标注"自动带出可修改"、提交信任用户值）。场景（phase6 R3）：风险事件主机 IP 带入工单系统 IP 字段 + 按 IP 查 **CMDB** 带出责任人——CMDB 即取数白名单（拍板点 2）的首个企业外部源实例（cmdb fetcher，零新拍板）。落点 = `field_meta` 新列扩 `auto_fill` 元数据（触发字段 key + fetcher 类型 + 字段映射，与 formula 同列承载——附录 A.2 方案 b，一枚 ALTER）；**执行时机双态预留**：元数据结构含 timing 标记（提交时/编辑时），**本段实现只做提交时取数（B1-4），编辑时预填留触发驱动**——phase6 R3 启动时消费、届时展开详细设计（交互链：字段 onChange → 轻量取数端点 → 预填+可覆盖）。业界同型：ServiceNow Variable auto-population / Salesforce cross-object lookup 带出 / 低代码「数据联动」；Jira 裸功能无此能力（靠 ScriptRunner 插件补）——真实需求 + 现成范式。
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
| 1 | 设计器选型 | ✅ **已拍（2026-10-15 所有者）：自研**（sortablejs+属性面板）；form-create designer 半天 spike 降为**对称验证**（判据详录 §3.1）——所有者另有疑问保留追问；form-create runtime 作渲染引擎备选在案（§3.1③，触发驱动） |
| 2 | 取数数据源白名单 | 内部资源 / activelist / 外部 HTTP——外部源回 [dd §26.6](../design/design-decisions.md) 出站决策树 |
| 3 | 取数身份 | 系统身份 vs 发起人代理——**L2 可见性边界**，宁窄勿宽 |
| 4 | 字典守卫形态 | 挂接批补删除守卫（推荐）vs 禁用不删使用纪律 |

> **状态注（2026-10-16）**：本表拍板点已随批量预确认拍定（2026-10-16 所有者「按建议」——✅34/🔶1，状态 SSOT = [11 号 §4](./11-implementation-plan.md)；各段启动时异议可翻，翻案成本=改一行）。

## 5. 边界（不做）

- **表单运行时交互层联动不做**（条件显隐/必填动态切换——12 号 §3.1 阶段 1 既定口径，eflow 亦仅 hidden/readonly）；自动化限于提交时计算/取数回写（本段）与流转时动作（S3）。
- 新字段类型不入本段（§3.5）；移动端/响应式不做（12 号 §6）。
- 引擎与存储零改动——**S1 只换配置界面，不构成 §23 自研推进的额外动作**（README §3 不动拍板）。

## 6. 影响面

- **迁移**：一枚 ALTER（`field_meta` JSONB 新列——附录 A.2 方案 b，2026-10-16 外审对齐后采纳为正文口径；原「承载 field_options 零迁移」口径废止——数组形态混载破坏消费方）。
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

## 附录 A：详细技术设计（实施级，2026-10-16 补充——样板深度对齐 05 号附录 A）

> ⚠ 标注 = 实施级设计建议（启动批确认制，不新增拍板点）。

### A.1 模块架构与代码落点

```
zhuzhao-ui/
  src/components/FormDesigner/          # 「先目录后包」落点（§3.1），三层分离
    index.vue                           #   壳：三栏（组件库/画布/属性面板）+ 工具栏（预览/保存）
    FieldCanvas.vue                     #   画布交互层：sortablejs 拖拽排序 + 选中态（零业务）
    FieldPalette.vue                    #   组件库：七类型卡片（richtext 随 S5 进调色板）
    FieldPropertyPanel.vue              #   属性面板：按 field_type 分组渲染配置项（el-form 动态 schema）
    schema.ts                           #   FieldDef TS 类型 + 序列化 + 前端校验（与后端 validateFieldInputs 镜像——规则变更两处同步，单测对齐）
  src/views/ticket/type/index.vue       # 画布模式 tab + 表格专家模式 + JSON 模式三态共存
  src/views/ticket/create/index.vue     # 渲染器扩展：formula 前端预览（提示"服务端计算"）/草稿（05 号 §6）
zhuzhao/
  internal/service/ticket/formula_eval.go    # 求值器封装（expr-lang/expr，编译结果 LRU 缓存）
  internal/service/ticket/field_linkage.go   # 提交时取数回写（B1-4）
  internal/pkg/datafetcher/                  # DataFetcher 端口 + source registry（README §6 第 6 行）
```

### A.2 元数据承载（✅ 方案 b 已采纳 2026-10-16——新列 `field_meta`，正文 §3.3①/§6 已对齐；原 ⚠ 候选待拍态废止）

`field_options` 现存**数组形态**（select 选项 `["a","b"]` 或 `[{label,value}]`，`optionValues` 按数组解析）——formula/auto_fill 若并入只能改对象形态 `{options:[...], formula:"…"}`，**破坏既有消费方**（渲染器选项渲染/服务端 optionValues/存量行）。两案：

| 案 | 形态 | 影响 |
|----|------|------|
| a | field_options 改对象 `{options, formula, auto_fill}` | 存量迁移（数组→对象）+ 消费方双形态兼容过渡——违约"只增不改"精神 |
| **b（推荐）** | **新列 `field_meta` JSONB**（`{formula, auto_fill, linkage}`），`field_options` 保持数组语义纯净 | 一枚 ALTER（01 号 §6 预留口子原文：「启动批若拍加专用列则改判为一枚 ALTER」）；optionValues/渲染器零改 |

`field_meta` 结构：`{"formula": "unit_price * quantity", "auto_fill": {"trigger_field": "host_ip", "fetcher": "cmdb", "mapping": {...}, "timing": "submit"} }`（auto_fill 双态预留见 §3.3③，timing=submit 本段唯一实现值）。

### A.3 求值器契约（formula_eval.go，S1/S3 公共件）

```go
type EvalEnv struct { Fields map[string]any; Actor EvalActor; Now time.Time }
func EvalFormula(expr string, env EvalEnv) (any, error)
```

- **expr-lang/expr 默认沙箱**：禁 import/任意方法调用；字段值类型白名单（string/int/float/bool/time）；
- **内置函数白名单**（启动批定首集，建议）：`sum/avg/round/min/max/len` + 日期 `now/dateDiff/formatDate`——注册为 expr 自定义函数，禁用户自定义函数；
- **编译缓存**：按字段存编译后 program（formula 随 schema 变更失效），提交链零重复编译；
- **提交链落点**：`Submit` 校验后、写 `custom_data` 前——**formula 字段一律重算覆盖**（前端预览值不可信，篡改负向必测 §8）；formula 引用字段缺失/类型不符 → 400 带字段名（fail-fast 不静默跳过）。

### A.4 DataFetcher 端口契约（B1-4）

```go
type FetchSource struct { Type string; Config json.RawMessage }   // internal_db / activelist / external_http / cmdb(预留)
type DataFetcher interface { Fetch(ctx, src FetchSource, key string) (map[string]any, error) }
```

- registry 按 Type 注册（`internal/pkg/datafetcher/registry.go`，app.go 装配）——白名单外的 Type 拒启/拒执（fail-closed，拍板点 2 落点）；
- **取数执行时机**：`Submit` 事务内、formula 重算**之前**（取数结果可被 formula 引用）；**失败策略** = 取数失败不阻塞提交、目标字段留空 + `linkage_missed` 事件（与分派失败⑤同哲学）——⚠ 启动批确认；
- **取数身份**（拍板点 3）：默认系统身份（service 账号）；外部 HTTP 走 dd §26.6 出站决策树；
- 超时钳制 3s/次、单次提交取数 ≤3 个字段（防提交被外部源拖死）。

### A.5 设计器交互规格

- **三栏**：左组件库（七类型卡片拖入）／中画布（已选字段列表，sortablejs 拖拽排序，点击选中）／右属性面板（配置项按类型：通用[label/field_key/required/sort_order] + 类型特有[select 选项挂字典键、number 步进、regex]）；
- **field_key 生成**：默认拼音/语义建议 + 唯一性前端预检（后端 validateFieldInputs 兜底）；
- **保存**：全量 PUT fields（既有 ReplaceTypeFields 契约不变）+ version CAS；**影响分析随行件**在保存前调（被 formula/模板引用字段变更 → 告警确认）；
- **预览**：右侧抽屉内嵌**真渲染器**（create 页同组件树）渲染未保存 schema——所见即所得（§3.1 论证）；
- **三模式共存**：画布（默认）/表格（专家）/JSON（源码）tab 切换，同一份 schema 三向同步（单一数据源 = Pinia store）。

### A.6 验收补充（在 §8 基础上）

- schema 往返幂等：设计器 → 保存 → 重开回显 → 再保存，diff 为空；
- formula 链路：A→B 计算字段，改 A 提交 → 服务端重算 B（**前端篡改 B 被覆盖**负向）；formula 引用字段删除 → 保存被影响分析拦截告警；
- 取数联动：mock fetcher 命中回填 / 失败留空 + linkage_missed 事件 / 白名单外 Type 拒执；
- 求值器沙箱负向：expr 含 `import`/函数外调用 → 编译期拒绝。

---

## 变更记录

| 日期 | 变更 |
|---|---|
| 2026-10-15 | 建档（phase5 分支十段设计稿批）——现状锚点逐段代码核实；拍板点带建议值；未新增决策（全部收编自 README 总纲与设计源） |
| 2026-10-15 | 选型论证深化批（业界对标讨论落档）：§3.1 补 view model ≠ domain model 正论（业界四类玩家收敛对照：平台级字段/布局分离·nocobase·JSON Forms 双层）/Vue3 生态情报（formily unmaintained·designable React 为主·活跃项唯 form-create）/四路线代价账（**form-create runtime 作渲染引擎登记正式备选**——单向投影 ≈2–3 天，触发驱动）/「先目录后包」落点纪律（components/FormDesigner 三层分离）/预览一致性优势 + 交互打磨前置为批次①验收项；§4 拍板点 1 补 spike 判据与备选指引——推荐结论不变（自研），spike 照跑 |
| 2026-10-15 | W0 前置拍板批（所有者逐项确认）：✅拍板点 1 = **自研**（form-create designer spike 降为对称验证）——所有者另有疑问保留追问权，论证深化见 §3.1 |
| 2026-10-16 | auto_fill 编辑时预填登记批（所有者 phase6 实际场景提出）：§3.3 增 ③——formula（强制计算）之外的第二种联动语义（建议预填·人工可改），CMDB = 取数白名单首个企业外部源实例；field_options 双态元数据预留（本段实现只做提交时，编辑时预填触发驱动、phase6 R3 启动消费）——登记挂账不入基线，详录 §3.3③ |
| 2026-10-16 | **详细技术设计批（附录 A，实施级——05 号样板深度）**：A.1 模块架构（FormDesigner 三层目录+create 页扩展+后端三落点）/A.2 **元数据承载结构决策**（⚠ field_options 现为数组形态、formula 并入即破坏消费方——推荐新列 field_meta JSONB（§6 影响面预留口子兑现），启动批拍）/A.3 求值器契约（expr 沙箱+函数白名单+编译缓存+提交链重算覆盖）/A.4 DataFetcher 契约（registry fail-closed+取数失败不阻塞留空+linkage_missed 事件+3s/3 字段钳制）/A.5 设计器交互规格（三栏/三模式共存单一数据源/预览=真渲染器）/A.6 验收补充（往返幂等/篡改覆盖/沙箱负向） |
| 2026-10-16 | **外审对齐批（矛盾消解）**：§3.3①③/§6 正文对齐附录 A.2 **方案 b（`field_meta` 新列）**并采纳为正文口径——原「承载 field_options 零迁移」口径废止（数组形态混载破坏消费方）；迁移零→一枚 ALTER；README 同步 |
