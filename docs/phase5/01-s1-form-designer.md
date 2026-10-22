# 01 — P5-S1 动态表单可视化设计器与字段自动化（设计稿）

> **段属性**：基线段（phase5 收口对象）。**建议实施序**：第二棒（S5 底座先行——设计器调色板的富文本类型依赖 S5 预签名通道）；计算字段/取数联动件可后移、不阻收口。
> **量级**：1.5–2.5 周（设计器 1–2 周 + 字段计算/联动 3–5 天；求值器为 S1/S3 公共件）。**迁移**：一枚 ALTER（`field_meta` JSONB——**000051 已实落** 2026-10-22，§6/附录 A.2 方案 b 口径；原「零」系 A.2 采纳前旧值，2026-10-22 勘误）。
> **文档定位**：段级设计稿（2026-10-15 建档，phase5 分支）。范围/边界/量级总纲 = [README §2 P5-S1](./README.md)；规格源 = [phase3/12-frontend §3.1](../phase3/12-frontend.md)（DynamicFormField 七字段类型 +「阶段 2 设计器」——随本段转正）；立项决策 = [design-decisions §27](../design/design-decisions.md)。

## 1. 定位与由来

P4-W4 深水区降级决策（设计器复杂度超预期即降级）实际交付了兜底形态：类型/字段/模板管理页可用，但 states/transitions 走 JSON 源码模式、字段走表格全量替换。所有者 2026-10-08 指示**表单要在前端自定义配置**——设计器由"批内深水区风险项"升格为独立段。同日场景补充：字段还要**自动化**（计算字段/条件取数回写），按执行时机拆落点——提交时计算/取数归本段，流转时取数回写归 S3 转换后动作（勿混层）。

## 2. 现状基线（代码级锚点，2026-10-15 核实）

| 项 | 现状 | 锚点 |
|----|------|------|
| 渲染器 | 自研七类型轻量渲染器已交付（W4）；**头注释明示：form-create 库引入决策顺延至本设计器批** | zhuzhao-ui `src/views/ticket/create/index.vue:3-8` |
| 字段类型 | **十一种**白名单：input/textarea/number/date/select/multi_select/tips/richtext/**radio/rating/file**（richtext B5-4 已落；**radio 第 9 + rating 第 10 + file 第 11 = B1-6 能力面扩展批 2026-10-22 所有者"全加"拍定**——radio 与 select 同模型（单值字符串）不同渲染（el-radio-group）；rating 与 number 同模型（el-slider/el-rate）；**file = 上传字段类型**（custom_data 存 object_key 数组、G2 required→至少 1 个 + 可选 MIME 白名单校验、前端复用 S5 presign/confirm 链内联渲染——业界 Jira/SN/SF/Typeform 均标配，安全场景"上传截图/日志/报告"刚需） | zhuzhao `internal/service/ticket/type_admin.go:24`（`validFieldTypes`——B1-6 时扩 radio/rating/file） |
| 字段保存 | **全量替换**：事务内先清后插 + 父类型 `version` CAS；**对存量 `custom_data` 零检查**——**2026-10-22 拍板锁死守卫（终版=兼容性判据版，待实施；判据 = 是否破坏已落库数据或旧单编辑）**：**硬锁 400** = 改 field_type / 改 field_key / 删除；**软提示**（可继续，借 impact 在途计数提醒）= options 收窄 / required false→true；**自由** = label / 排序 / 新增 / col_span / group / visible_when / formula / fetch / validation / options 只增不减 / required true→false；附带语义：改公式/挂取数**不回溯存量单**，仅影响后续提交与编辑（业界对标：Jira 自定义字段类型不可变/SN 列 key 不可变/SF 删字段依赖门硬拦） | `internal/repository/ticket_repo.go` `ReplaceTypeFields`（≈:786 起） |
| 字典（P4-3 已交付） | `/dicts` + `/dict-items` 管理四端点 + 用户侧 `EnabledItems` 读端点 | `internal/router/router.go:199`（EnabledItems）、`:320-325`（管理组） |
| 字典删除 | dict_types/dict_items **无守卫硬删**（items 级联随删）——挂接字典键后成引用完整性缺口 | `internal/repository/dict_repo.go` `DeleteTypeByCode`（≈:89 起） |
| 详情回显 | ~~custom_data 仅 create 页提交，detail/list 页零渲染~~ **已闭合（2026-10-22 勘误——S1 执行期已落，旧差距行过时）**：detail 页按 schema 渲染 custom_data（customValue 读取 + richtext 图处理均实装）；list 页表格不渲染自定义字段=设计内 | zhuzhao-ui `views/ticket/detail/index.vue:73-92` |
| 专家模式 | ticket/type 页表格模式 = phase4 深水区降级决策的兜底形态，保留 | zhuzhao-ui `views/ticket/type/index.vue` |

## 3. 目标形态与范围

### 3.1 可视化设计器（前端为主）

- **启动首件 = 设计器选型拍板**。**推荐自研**：sortablejs 拖拽排序 + 属性面板，产物即 field schema **零映射**；form-create designer 作**半天 spike 对照**——其 rules/options 组件树 JSON 与规范化 schema 不同构，引入需双向映射器 + 面板裁剪到七类型，高级布局能力用不上，**模型税大于成熟度收益**（phase4/02 深水区教训同源）。
- **选型论证深化（2026-10-15 业界对标讨论落档；2026-10-21 措辞校正——Jira 表单能力来自 ProForma 收购（2019）非自研，业界准确表述=「自建或收编，不引开源表单库」）**：
  - **正论升级——view model ≠ domain model**：业界主流无一把渲染器组件实例树存库——Jira/ServiceNow/Salesforce/Dynamics 均「字段定义（封闭类型枚举）与布局配置分离」；nocobase collection/block 同构分离；[JSON Forms](https://jsonforms.io/docs/uischema) 将其标准化为 JSON Schema（数据契约）+ UI Schema（视图）双层。存储契约三性质：服务端可校验（G2/custom_data 管线依赖）、跨端中立（详情回显/S6 模板引用/S3 条件/S7 AI 填表消费）、纯声明式（rules 的 on/hook/update 为**函数体**，JSONB 不可存且成注入面）。**本仓 schema 已在业界主流侧，差距在配置体验层（= 本段本体），不在模型层**。
  - **生态情报**：Vue3 表单设计器开源面狭窄——formily 2025 起 unmaintained、designable 以 React 为主（Vue3 支持有限）、form-generator 类停更，活跃项唯 form-create——「成熟方案替代」并无更优候选，单点上游集中度反而加分自研。
  - **四路线代价账**：整体引入（designer+rules 入库）≈2–4 周一次性 + 永久映射税 + 服务端开放集校验重做，业界无人走；自研 ≈1–2 周（已入总账）+ 每新类型 1–2 天三处线性扩；SaaS 表单（Jotform/简道云类）内网+IAM 集成不可行；**form-create runtime 作渲染引擎 = 正式备选**（schema→rules **单向投影**、只渲染不保存——无往返漂移，白拿其 validate 数组/control 渲染层能力，≈2–3 天可启用；触发 = 渲染层联动需求变重时重评，本段边界 §5 不变）。
  - **自研落点纪律（先目录后包）**：zhuzhao-ui 内 `src/components/FormDesigner/` 目录域，内部三层分离（拖拽画布交互层/属性面板层/schema 适配层——与 S2「画布层/适配层分离」同纪律）；出现第二仓消费方（如 taskrunner params 动态表单跨仓复用）才评估抽包，现在抽 = 版本发布/依赖对齐/单人维护三笔负收益。
  - **自研独有优势**：设计器预览 = 真渲染器渲染真 schema（所见即所得零偏差——form-create designer 预览的是 rules 效果，与本仓渲染器是两条渲染路径）；**交互打磨（拖拽锚点/占位反馈/面板分组/实时预览）列入批次①验收项而非锦上添花**——业界差距既在体验层，预算前置。
  - **spike 判据更新**（半天，批次①首日）：form-create designer 裁剪到七类型+regex 校验+忽略布局后，实测映射损耗与交互残缺度，对照自研交互补齐成本——若裁剪后映射接近无损则翻案成立，否则维持自研。
  - **✅ spike 完成（2026-10-22 B1-1 首日，双路实测）——判定：翻案不成立，维持自研**：
    - **A 路 form-create designer v3.5.0 对称验证**（/tmp scratch + 浏览器实测）：①**regex pattern 经 JSON 往返丢失**（RegExp → `{}`——判据点名的「regex 校验」实测即损，最重一项）②richtext=内置 HTML 富文本编辑器，与我方 MD 存储+goldmark 渲染 SSOT **语义冲突** ③tips 需映射原生 div（非 input，双向识别需约定）④field 默认 uniqueId 随机串（需自定义面板强制命名）⑤field_meta（formula/validation 元数据面）无对应物（需自定义属性组件）⑥裁剪面：默认 40 组件全量透出、menu 定制 prop 未生效（裁剪入口有学习成本）、**v3.5.0 内置 AI 助理面板**（内网部署需评估关闭与外联面）⑦内部字段污染（_fc_id/_fc_drag_tag/name/display/hidden 需适配层剥除）⑧HTML5 DnD 自动化不可驱动（E2E 须走 setRule 程序化）；可用面=setRule/getRule 程序化往返成功（API 六方法）。**结论：8 类型中 6 个字段级近无损，但 regex 往返丢+richtext 语义断+两元数据面全缺——非「接近无损」**；
    - **B 路 sortablejs 自研可点原型**（[prototypes/s1-form-designer.html](./prototypes/s1-form-designer.html)——单文件 vanilla JS ≈400 行 + sortablejs 44KB 内联，浏览器亲点验证；内联 vendor = Sortable **1.14.0**、MIT 头保留——与 ui 仓实现版 1.15.7 版本差无碍原型用途，2026-10-22 修复批注）：8 类型添加/拖拽排序/选中-属性面板联动/required/选项编辑/**regex 语法即时校验**（错则面板即报）/公式占位（B1-3 预留）/预览切换/**序列化 ticket_type_fields 形态且 regex 字符串无损往返**（与 A 路丢失实锤成对照）/导入回放双向一致——主交互流全通。**结论：自研交互补齐成本可控（原型即批次①壳的雏形），佐证维持自研**；原型交付所有者亲点定属性面板信息架构（双钻 Develop 侧用户验证）。
  - **可点原型扩展（2026-10-19 双钻复盘立约）**：B1-1 首日 spike（选型对称验证）之外，同批追加半天**交互流可点原型**——假数据把「拖入七类型 → 逐类型属性面板填写 → 预览切换 → schema 序列化」主交互流亲手点一遍（不接后端，所有者即第一个用户），再定属性面板信息架构——UX 手感面的答案在「用起来」里，文档论证与外审均查不出「别扭」（双钻 Develop 侧用户验证型发散；与上行「交互打磨列入批次①验收」同源，把验收前移到设计定稿前）。
- **后端存储不变**：产物仍是 `ticket_type_fields` 字段 schema；字段类型仍限七种；`validateFieldInputs`/G2 schema 校验维持。
- 现 ticket/type 页表格模式保留为**专家模式/兜底**，不拆除（phase4/02 降级决策形态直接复用）。

### 3.2 select/multi_select 选项引用字典（P4-3 衔接）+ 人员自动补全

- 设计器选项支持**挂字典键**而非死枚举，选项变更全系统生效（字典端点已核在位，见 §2）。
  - **校验侧展开（2026-10-22 F-B-2 外审 P0 修复）**：`validateCustomData` 对 field_options 含 `dict:<code>` 的 select/multi_select 自动展开为真实字典项值集再校验——修复前 optionValues 只见字面量 `["dict:code"]`，用户提交真实字典项（如 `high`）必被拒（B1-2 核心功能 100% 失效）。
- **人员/部门类字段自动补全**（2026-10-09 场景补充）：指派/抄送/参与人选人走既有用户搜索端点；M-HR 对接后（HRFetcher）组织人员数据入本地表，补全零额外对接——依赖 M-HR 触发，**不阻塞本段**。

### 3.3 字段自动计算/联动（2026-10-08 所有者场景补充）

- **① 计算字段**：按已填字段公式自动算派生字段。落点 = 字段 schema 扩展（meta 增 `formula`，**承载于新列 `field_meta` JSONB**——附录 A.2 方案 b：`field_options` 为数组形态（select 选项），混载元数据须改对象形态、破坏渲染器/optionValues/存量行消费方，**2026-10-16 外审对齐后采纳新列方案，正文与附录以此为准**）+ **服务端求值为 SSOT**（提交时后端算入 `custom_data`，前端仅预览展示——防篡改，与现有哲学一致）。
- **② 提交时条件取数回写**：按字段值判断后从数据源取数自动写入字段。落点 = 字段联动规则（数据层自动化，非流程层）；取数走 `DataFetcher` 端口（按 source 类型注册 fetcher——[README §6 对接预留清单](./README.md)）。
- **③ 编辑时预填 auto_fill（2026-10-16 所有者场景登记——phase6 实际场景驱动；触发驱动挂账不入本段基线，消费者 = phase6 R3）**：与 ① 的语义分层——formula = **强制计算**（服务端 SSOT，人工改无效）；auto_fill = **建议预填**（编辑时实时带出、UI 标注"自动带出可修改"、提交信任用户值）。场景（phase6 R3）：风险事件主机 IP 带入工单系统 IP 字段 + 按 IP 查 **CMDB** 带出责任人——CMDB 即取数白名单（拍板点 2）的首个企业外部源实例（fetcher = asset-platform——资产平台查询门面，只查询不建台账；2026-10-20 规划研究回灌，依据 deliverables/software-company/secops-platform-vision-2026-10/08-phase5-adjustments.md；零新拍板）。落点 = `field_meta` 新列扩 `auto_fill` 元数据（触发字段 key + fetcher 类型 + 字段映射，与 formula 同列承载——附录 A.2 方案 b，一枚 ALTER）；**执行时机双态预留**：元数据结构含 timing 标记（提交时/编辑时），**本段实现只做提交时取数（B1-4），编辑时预填留触发驱动**——phase6 R3 启动时消费、届时展开详细设计（交互链：字段 onChange → 轻量取数端点 → 预填+可覆盖）。业界同型：ServiceNow Variable auto-population / Salesforce cross-object lookup 带出 / 低代码「数据联动」；Jira 裸功能无此能力（靠 ScriptRunner 插件补）——真实需求 + 现成范式。
- **③⁺ auto_fill 全链定稿（2026-10-22 所有者场景走查批——canonical 例：漏洞修复工单「填主机 IP → 自动带出责任人（可人工改）」；本条为设计定稿，实现随 phase6 R3 首源批）**：
  - **语义分层终版**：formula = 强制计算（服务端 SSOT，人工改无效）；fetch = **建议预填——查询结果是该字段的默认值而非终值，人工改有效**。提交链「仅补空不覆盖」已实装（`datafetcher.ApplyFetches`：仅当目标字段不存在时写入——datafetcher.go 注释明示）；提交兜底边界拍板 = **总是补空**（用户留空也查，"希望自动完成"初衷，故意留空罕见不设例外）；
  - **三层分工（"设计器配的是数据不是组件"）**：引擎一份（读配置调度，B1-4 已落）+ **适配器按源一份**（"怎么跟外部系统说话"在代码：URL/鉴权/协议/超时/身份——写一次服务该源所有字段所有类型；`external_http` 通用适配器落地后新系统零代码，仅白名单+URL 模板配置）+ 字段配置纯数据（"查什么、放哪"，零代码）。渲染形态 11 种 field_type 写死（唯一"组件清单"），字段数量/配置无限=schema 表里的行——新字段不产生新组件新代码；
  - **配置结构定稿**：`field_meta.fetch = { source, params, mapping }`——**params 多参绑定**（泛化替换单 `trigger_by`：适配器入参名 → 表单字段 key 或常量，如 `{ip: host_ip, region: "cn-east"}`；**适配器代码声明入参契约**（名称/必填），属性面板据此自动渲染绑定控件——ServiceNow IntegrationHub / n8n action 入参绑定同型）；**mapping 一查多填**（一次查询多结果键 → 多个目标字段，如 `{owner_name→owner, level→asset_level}`）；mapping 目标键不得重复（同目标多源 = Go map 迭代序随机定胜者——datafetcher 实码注记 2026-10-22，保存期校验随 B1-7）；
  - **编辑时交互（主路径，phase6 R3）**：触发字段失焦防抖 500ms → `POST /ticket-types/{code}/fields/prefill`（登录即可 + 按用户限流 + 3s）→ 预填 + 浅蓝「自动带出」标签 + 刷新图标（tooltip："根据 XX 从资产平台自动带出，可修改"）；人工改 → 标签变灰「已手动修改」，提交以用户值为准；改触发值/点刷新 → 重取；详情页字段标签挂**静态徽标「自动带出」**（schema 驱动，不逐值记来源）；
  - **错误 UX 三档**：查询失败（超时/源不可用）= 红字"自动查询失败，请手动填写" + 重试图标（复用表单统一 error 位，不发明新控件）；查无结果 = 灰字"未查询到，请手动填写"；查到但值不对 = 无警告直接改。均记 `linkage_missed` 事件（已有）；
  - **链式取数（fetch→fetch）**：保存期校验三件（params 引用字段须存在 / **不许绑 formula 字段**——时序不可能：链=取数→校验→公式 / 触发引用图**无环**——A 触发 B、B 触发 A 保存时 400）；编辑时级联**限深 ≤3 跳**（中间值落表单后自然接力，人工改后用人工值）；提交兜底**多趟收敛 ≤3 趟**（第 2 趟处理第 1 趟填出的触发值——替换单趟读原始提交值）；配置习惯：**一查多填优先，两跳只用于真跨源接力**（IP→源1→employee_id→源2 HR），更复杂多跳做进专用 fetcher（服务端代码内部接力，配置仍一跳）；
  - **canonical 走查**：设计器——`host_ip`(input,required) + `owner` 挂 fetch{source: asset-platform, params:{ip: host_ip}, mapping:{owner_name→owner, level→asset_level}} + `asset_level`(input，无自身配置，被映射顺带填) → 保存即五行 schema 数据零代码；建单——填 IP `10.1.2.3` → 500ms → owner 预填「张三(E000123)」可改、asset_level 同查落值；提交——未预填且空 → 兜底查补空；详情——通用渲染 + 「自动带出」徽标；
  - **业界同型**：ServiceNow lookup 自动带出 / IntegrationHub 入参绑定 / Salesforce cross-object 带出 / 国内低代码「数据联动」（简道云/明道云）——交互链同构，方向无偏差；对照差距=本条各实现件均未落地（registry 生产装配为空、prefill 端点未建），phase6 R3 首源批一次接通。
- **求值器统一为白名单 Go 侧表达式求值器**（phase3/10 §4.2 既定路线；easy-workflow 表达式拼 SQL + 黑名单防注入为反面教材）；选型预置 = **expr-lang/expr**（MIT/活跃/默认沙箱/自定义函数白名单——README §2 选型表）。字段公式（本段）与网关条件（S3）**共用同一件**。
- **④ 字段级校验规则（2026-10-16 全计划业界对标补——业界表单设计器标配，现仅 required 单档）**：`field_meta.validation`（与 formula/auto_fill 同列承载——附录 A.2 方案 b）：regex 匹配 / 数值 min-max / 文本长度区间；**渲染端 rules 生成 + 服务端提交校验双落**（防绕过——服务端为 SSOT 同 formula 哲学）。量级 +0.5–1 天（随 B1-3 求值器批）。业界同型：JSON Forms validators / Jira field context。

### 3.4 随行件三件（2026-10-09 隐藏需求核查补遗）

| 件 | 内容 | 量级 |
|----|------|------|
| 字段变更在途影响分析 | 字段保存=全量替换、对存量 custom_data 零检查；引入 formula 与 S6 模板变量引用后，**保存前须做影响面告警**（被 formula/模板引用的字段、存量单含该字段数据 → 告警确认，防"引用字段被删/改 key → 公式与模板静默失效"）——与 S2 改图告警同型。**2026-10-22 拍板角色缩小**：字段锁死（§2）后删/改不可达，破坏性告警面消亡；剩余用途=**新增 required 字段与软提示两件（options 收窄 / required 加严）的在途量提醒**；**端点归位管理组（终版=与 fields/replace 同组——"能改字段即可看影响"）**（撤 SelfService 挂法——catalog「ticket_list 消费」注记失实，ui 仓零消费实证 2026-10-22；外审 F-D-1 P0 移挂方向随之修正为归位） | +0.5 天 |
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

- **条件显隐——✅ 翻案进 B1-6（2026-10-22 所有者"全加"拍定）**：`field_meta.visible_when` 表达式（复用 S1/S3 expr 公共件——B1-3 exprcalc 已落为集成件）+ 渲染端 watch 联动 + G2 提交时跳过隐藏字段（required 不校验、值不写入——防绕过）；量级 ≈1–2 天（原 2–3 天登记基于无 expr 底座，现已有减为集成）。~~原"不做"口径留痕~~（2026-10-19 前唯一显式不做边界——触发信号"选了 A 才填 B"由所有者 2026-10-22 主动提出，翻案成立）。
- 新字段类型（radio/rating/file）与布局件（col_span/group）归 **B1-6 能力面扩展批**（2026-10-22 拍定——§2 字段类型行已同步**十一种**[含 file 第 11，本行 2026-10-22 勘误]）；移动端/响应式维持不做（phase3/12 §6）。
- **file×附件体系交互五点（2026-10-22 文档批展开——第三路核查提醒；建议口径，B1-6 首件前随批确认；11 号 B1-6 既定口径 = "ticket_attachments 关联行走既有建单绑定 A.5"）**：①**复用 B5 全链**——上传 = 表单位置 presign/confirm（attachment_create 悬挂态）→ CreateTicket 同事务落绑 ticket_attachments（既有 bindAttachmentsTx 共用件）；②**额度共享**——file 字段对象与附件区**合并计入**单单 ≤20（同一 FOR UPDATE 计数链，防双通道绕限）；③**MIME 双层**——字段级 field_options 白名单（G2 侧）∩ 全局 settings `allowed_mime_types`（presign 侧既有硬底线）；④**GC/删除**——purpose 走 attachment 既有语义（工单删除级联行 + GC 收本体；字段删除属锁死硬锁不可达，无孤儿新增面）；⑤**custom_data 只存 object_key 数组**（展示态引用），权威归属在关联行——与 S5 附件同构不另起体系。
- 引擎与存储零改动——**S1 只换配置界面，不构成 §23 自研推进的额外动作**（README §3 不动拍板）。

## 6. 影响面

- **迁移**：一枚 ALTER（`field_meta` JSONB 新列——附录 A.2 方案 b，2026-10-16 外审对齐后采纳为正文口径；原「承载 field_options 零迁移」口径废止——数组形态混载破坏消费方）。
- **权限**：零新增码（管理面沿用既有类型管理权限；取数联动运行期走既有 L2 管线）。
- **前端**：`views/ticket/type` 设计器面板、`views/ticket/create` 渲染器扩展（formula 预览）、detail 回显。
- **端口**：`DataFetcher`（取数源白名单；README §6 清单第 6 行，随本段登记）。
- **公共件**：白名单表达式求值器（S1/S3 共用，选型 expr-lang/expr 建议值）。

## 7. 实施序与批次建议

依赖 S5 底座（富文本进调色板需预签名通道先行）。批次切分：①设计器壳 + 七类型拖拽配置（含专家模式共存）→ ②字典引用 + 人员补全（含字典守卫 + 新字段 key 建议 ASCII slug 软约束——中文/含点键由 S6 变量字典供派生 slug 兜底，06 号 A.4）→ ③求值器 + formula 计算字段 → ④取数联动（拍板点 2/3 之后）→ ⑤随行三件收尾（影响分析可提前到②后）。**③④可后移不阻收口**；设计器选型 spike 在①首日。

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
- **函数面（2026-10-22 f32464c 修正——原「白名单 sum/avg/…」系文档声称实现空转）**：当前 = `DisableAllBuiltins` 仅算术/比较/逻辑运算符可用；函数白名单重注（AddFunction 首批 sum/avg/round/min/max/len）随 B1-6 函数注册表；
- **编译缓存**：按字段存编译后 program（formula 随 schema 变更失效），提交链零重复编译；
- **提交链落点**：`Submit` 校验后、写 `custom_data` 前——**formula 字段一律重算覆盖**（前端预览值不可信，篡改负向必测 §8）；formula 引用字段缺失/类型不符 → 400 带字段名（fail-fast 不静默跳过）。
- **审计回声注（2026-10-22 全面审计批 B）**：
  - **$env 语义**：expr 的 `$env` 标识符返回完整 env map（= custom_data 全量）——非安全漏洞（formula 本就引用全字段），但公式中无意嵌入 `$env` 会将全量数据 dump 到计算字段值；宜文档注明 + 后续禁用（`expr.DisableBuiltin("env")`）
  - **除零/NaN**：IEEE 754 除零产生 ±Inf——已加 IsInf/IsNaN 校验拒提交（F-B-2 修复 2026-10-22）
  - **级联引用取快照值**：formula A 引用 formula B 时，A 使用 B 的**提交时快照值**（用户填入值），非 B 的公式计算结果——~~单趟评估防级联篡改的正确设计（F-B-4 注记）~~ **定性反了（N1）且已拍终局（2026-10-22 所有者场景走查）：禁止 formula 引用 formula**——保存时 400，表达式=数据字段引用+注册函数调用，快照语义随禁令消亡（详见 N1 终局注）
  - **取数顺序**：Create 链 = fetch → validate → formula（F-B-3 修复 2026-10-22——fetch 写入的值须过 G2 校验）；**Update 链不调 applyFetches**（仅 Create 取数，编辑时预填是 §3.3③ 触发驱动件——F-B-5 注记）
- **独立复核注（2026-10-22 全面审计 A/B 批复核落档——两段结论全证实；探针实证 = /tmp 沙箱同版本 v1.17.8）**：
  - **N1（P2·B1-6 前拍）级联快照定性修正**：上「级联引用取快照值」条的「防级联篡改的正确设计」定性**不成立（方向反了）**——快照语义（单趟 env=提交值）恰**开**篡改通道：formula A 引用 formula B 时，A 取 B 的**客户端提交值**固化进服务端计算结果（提交链不剥离 formula 字段提交值 + required 又豁免），篡改 B 自身会被覆盖（A.6 有测）但**经 A 引用 B 可洗入任意值（零测试覆盖）**；两级联（按 schema 序把已算值写回 env）终止性相同、通道关闭。~~待拍：(a) 改两级联（推荐）或 (b) 维持快照 + 该条改如实表述（实现取舍：单趟保终止，代价 = 级联引用可被提交值污染）+ 补篡改经引用负向测试~~ **已拍终局（2026-10-22 所有者场景走查）：③禁止 formula 引用 formula——超出原 a/b 两案**。理据：复杂/多步组合的归宿=注册函数（B1-6 函数注册表，Go 侧可组合可测身份可控）与 fetcher 接力（§3.3③⁺），配置面保持一跳，链式需求不存在则链式问题整类消除；两级联方案作废。实现三件：保存校验（formula 引用仅限同类型数据字段）+ 提交链剥离 formula 字段提交值（兜底）+ 篡改经引用负向测试
  - **N3（P3·随 B1-6）$env 禁用法修正**：$env 条建议的 `expr.DisableBuiltin("env")` 医不对症——`$env` 是语言级变量非内置函数（DisableAllBuiltins 下探针实证仍返回全量 Fields map）；禁用须源码级拒绝（编译前检 `$env` 子串）或结果侧校验
  - **N2（P3·随 B1-6 函数注册表）函数集现状对齐**：f32464c 后实际**零内置函数放行**（仅算术/比较/逻辑运算符——字段依赖调用运行期 fail-closed「cannot call nil」，纯字面量调用编译期常量折叠、静态受 2048 钳制无放大向量，无害）；本节「建议首集 sum/avg/…+日期三件」由 B1-6 函数注册表（AddFunction 首批 5 函数）实际取代；`exprcalc.go` 头注「仅放行 sum/avg/round/min/max/len」失实随批改
  - **余开小项（P3·随批）**：「超时经 context 取消传递」注释失实（实际仅长度钳制、无 context 接线）；`EvalEnv.Now` 死代码（设而不可达）

### A.4 DataFetcher 端口契约（B1-4）

```go
type FetchSource struct { Type string; Config json.RawMessage }   // internal_db / activelist / external_http / asset-platform（资产平台查询门面，只查询不建台账——2026-10-20 回灌）
type DataFetcher interface { Fetch(ctx, src FetchSource, key string) (map[string]any, error) }
```

- registry 按 Type 注册（`internal/pkg/datafetcher/registry.go`，app.go 装配）——白名单外的 Type 拒启/拒执（fail-closed，拍板点 2 落点）；
- **取数执行时机**：`Submit` 事务内、formula 重算**之前**（取数结果可被 formula 引用）；**失败策略** = 取数失败不阻塞提交、目标字段留空 + `linkage_missed` 事件（与分派失败⑤同哲学）——⚠ 启动批确认；
- **取数身份**（拍板点 3，2026-10-21 修正后拍定——按源三分治）：
  - `internal_db` = **发起人身份**（查询注入发起人 L2 范围，取不到即留空）——理由 = 回写即探测：发起人可控字段值 → Submit 事务内 fetcher → 结果写 custom_data → 发起人可见；若用系统身份则 fetcher 能读到发起人 L2 看不到的行（建单即泄露通道）。此理由 2026-10-21 外审核验发现，原文档（"默认系统身份"）未评估此面；
  - `activelist` = **服务身份**——ADR-003：activelist 零权限检查、X-Operator 仅入 AK/SK 签名覆盖做审计归因（非权限边界）、独立服务+独立库。用户身份在此**不产生行级过滤效果**，只多一个审计标签；
  - `asset-platform` / `external_http` = **服务身份** + 平台侧只回填非敏感白名单字段（外部系统鉴权形态待 X5 契约确认后细化）；
  - ~~默认系统身份~~（原已拍口径保留留痕——与"internal_db 用发起人"不矛盾：按源分治取代全局默认）；
- **取数限流与审计**（2026-10-21 A 类登记入 A.4——核验批补点，非拍板项）：
  - **限流**：per-source fail-fast——命中即按既有失败策略降级（留空 + `linkage_missed`），**不排队等待**（取数在 Submit 事务内，排队会拉长事务占锁）；Redis 故障时不照抄路由中间件 503 fail-close（会让建单直接炸），改 fail-open 跳过限流（写 Warn 日志）；阈值 config 化（`datafetcher.rate_limit_per_source`，维度 = per source × per request，默认并发 ≤3 / 60s 窗口 ≤10 次，启动批可调）；
  - **审计**：每次取数日志含 `operator/source/target/duration/hit`——`operator` 必填（缺了它审计对"谁在探测"零信息；internal_db 用发起人身份时尤其关键——它是唯一能探测 L2 边界的源）；
- 超时钳制 3s/次、单次提交取数 ≤3 个字段（防提交被外部源拖死——最坏 3×3s=9s 在事务内，限流 fail-fast 是唯一防线）。

### A.5 设计器交互规格

- **三栏**：左组件库（七类型卡片拖入）／中画布（已选字段列表，sortablejs 拖拽排序，点击选中）／右属性面板（配置项按类型：通用[label/field_key/required/sort_order] + 类型特有[select 选项挂字典键、number 步进、regex]）；
- **field_key 生成**：默认拼音/语义建议 + 唯一性前端预检（后端 validateFieldInputs 兜底）；
- **保存**：全量 PUT fields（既有 ReplaceTypeFields 契约不变）+ version CAS；**锁死守卫（2026-10-22 拍板终版=兼容性判据版，待实施）**：按 key diff 新旧 schema——**硬锁 400**（改 field_type / 改 field_key / 删除）、**软提示**（options 收窄、required false→true——借 impact 在途计数）、**自由**（label/排序/新增/col_span/group/visible_when/formula/fetch/validation/options 只增不减/required true→false）；保存校验随行三件：formula 引用仅限同类型数据字段（禁 formula→formula）、fetch params 引用字段存在且非 formula 字段、触发引用图无环；**影响分析随行件角色缩小为「新增 required 字段 + 软提示两件的在途计数提醒」**（§3.4 2026-10-22 拍板）；
- **预览**：右侧抽屉内嵌**真渲染器**（create 页同组件树）渲染未保存 schema——所见即所得（§3.1 论证）；
- **三模式共存**：画布（默认）/表格（专家）/JSON（源码）tab 切换，同一份 schema 三向同步（单一数据源 = Pinia store）。

### A.6 验收补充（在 §8 基础上）

- schema 往返幂等：设计器 → 保存 → 重开回显 → 再保存，diff 为空；
- formula 链路：**A→B 链式保存被拒 400（2026-10-22 拍板禁引用，替原 A→B 级联用例）**；单 formula 前端篡改提交值 → 服务端重算覆盖（负向）；formula 引用不存在字段 → 保存 400（锁死后删除不可达，引用存在性校验前置）；
- 取数联动：mock fetcher 命中回填 / 失败留空 + linkage_missed 事件 / 白名单外 Type 拒执；
- 求值器沙箱负向：expr 含 `import`/函数外调用 → 编译期拒绝；
- **语义收口批测试矩阵（2026-10-22 定稿——与上列既有用例互补；实施时逐条先红后绿）**：
  - **禁链式保存校验**：〔负〕formula A 引用 formula B 的 key → 保存 400；〔负〕formula 引用不存在的 key → 保存 400；〔正〕formula 引用数据字段保存成功 + 提交重算覆盖篡改（既有用例保留不动）；N1 篡改经引用通道随禁令结构性闭合——无提交期路径可测，保存期 400 即回归锚点；
  - **锁死守卫（ReplaceTypeFields diff，按 key 对比新旧）**：〔正〕新类型首次全量保存；〔正〕存量类型改 label + 新增字段 + 调排序 → 200；〔负〕存量字段改 field_type → 400；〔负〕改 field_key（=删+增）→ 400；〔负〕从列表移除存量字段（=删除）→ 400；〔负〕硬锁三件为拍板终版断言（2026-10-22 = 兼容性判据版）；〔正·自由面〕改 label / 加 col_span / 改 formula / 补挂 fetch → 200（判据版解锁面——B1-6 布局与存量加取数两场景的回归锚点）；〔软提示〕options 收窄 / required false→true → 保存通过且返回在途计数提示；version CAS 冲突由既有用例覆盖；
  - **impact 端点归位管理组**：〔正〕带类型管理权限角色调用 → 200；〔负〕普通登录用户（无菜单权限）→ 403；〔测试反转〕impact 移出 catalogExempt 后 `TestCatalogExempt_NotBlockedByL1`（router_test.go:122）**全集循环自动脱测**（无 impact 专属子测——2026-10-22 修复批勘误原「子测反转」措辞）+ 新增无权限 403 负向；既有 TestFieldImpact_Analysis 功能断言不动；
  - **$env 源码级拒绝（N3 落地）**：〔负〕公式含 `$env` → 拒——保存 CheckSyntax 与提交求值**双防线各一测**；〔负〕`$env['x']` 取值路径 → 拒；〔正·不误伤〕字段名含 env 字样（如 `environment`）与字符串字面量含 "$env" 字样均不误拒（2026-10-22 拍板=**token 级检测**——解析后拦 `$env` 标识符本身，非子串扫描）；〔重写〕exprcalc 既有"禁环境逃逸标识符"测试名实对齐——现测 `$now.__unix`（未知标识符，过因错误原因），从未覆盖 `$env` 本尊（探针实证仍可取值），重写为真 $env 用例；
  - **随行修正（无新负向）**：`EvalEnv.Now` 死码删除后 exprcalc_test 两处构造引用同步改；~~92303 errcode 预留注记~~ **92303 处置 = 删注**（2026-10-22 修复批勘误——全仓无定义无登记的悬空引用；锁死/禁链式保存拒绝沿用 ErrInvalidParams 带明细 message[validateFieldInputs 既有形态]，独立码按需再定）。
  - **类型漂移与存在性补充组（2026-10-22 修复批补齐——B1-7 行范围与矩阵对齐）**：〔负〕input 型字段 formula 产出数字 → 提交 400（**提交期重算后按 field_type 类型校验、硬 400 不宽容转换**——所有者 2026-10-22「按建议」拍定；保存期预判不可实施：AsAny 无静态结果类型、保存期无字段值）；〔负〕保存 schema 引用不存在字典码 → 400（字典码存在性——运行期空集兜底已随 **P1-1 修复**钉死：expandDictOptions 错误臂返 `[]`、回归测试三臂在档 dict_expand_integration_test）；〔随 B1-7〕A.5 保存校验三件之②③（fetch params 引用字段存在且非 formula / 触发引用图无环）用例随实施补。

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
| 2026-10-16 | **全计划业界对标批（所有者）**：§3.3 补 ④**字段级校验规则**（业界表单设计器标配：regex/min-max/长度，现仅 required 单档；field_meta.validation 双落渲染+服务端 SSOT；随 B1-3，+0.5–1 天） |
| 2026-10-16 | **S6 变量字典 slug 软约束登记批（06 号 A.4 悬空引用补锚）**：§7 批次②补「新字段 key 建议 ASCII slug 软约束」——中文/含点键由 S6 变量字典供派生 slug 兜底（`ticket.fields.f_<ID>`），不回改存量 |
| 2026-10-19 | **第二意见体检回声批（并行外审报告——本级 2 处）**：①§5「表单运行时交互层联动不做」补**显式触发信号**（业务方提出「选了 A 才填 B」类联动诉求即立项——业界表单设计器标配而明确不做的唯一边界，按边界纪律补出口）②§5 两处「12 号」引用限定为 **phase3/12**（消与 11 号 §5.7 计划中 phase5/12-scenarios-and-tests.md 的同名二义——10 号两义旧疾同款） |
| 2026-10-19 | **条件显隐量级登记批（所有者「按方案登记」——业界持平代价账补完）**：§5 边界行补**量级 ≈2–3 天**（复用 S1/S3 expr 公共件——前端渲染层显隐+必填动态切换+服务端兜底断言，零迁移零结构改动）——此前只有触发信号无量级，为六能力域唯一缺口；03 号演进触发总表同步加行（演进 SSOT 完整性） |
| 2026-10-19 | **可点原型扩展批（双钻复盘立约——§3.1）**：B1-1 首日 spike（选型对称验证）外追加半天**交互流可点原型**（假数据点完「拖入→属性面板→预览→序列化」主交互流，所有者亲点后再定属性面板信息架构）——UX 手感面验证前移至设计定稿前；同日 standards §12-10 立外审截断规则（详 standards 变更记录） |
| 2026-10-22 | **B1-1 首日 spike 双路完成（§3.1 判据红线判定）**：A 路对称验证 8 项发现（regex 往返丢失[RegExp→`{}` 实测]/richtext HTML 编辑器与 MD SSOT 语义断/tips 原生 div 映射/uniqueId 默认键/field_meta 面板缺/裁剪 menu 未生效+内置 AI 助理待裁/内部字段污染/DnD 自动化不可驱动）→ 非接近无损；B 路 sortablejs 可点原型主交互流全通（regex 字符串无损往返成对照）——**翻案不成立维持自研，原型落 prototypes/ 供所有者亲点** |
| 2026-10-22 | **全面审计 A/B 批独立复核落档批（两段结论全证实；探针实证 expr v1.17.8）**：P0（committed HEAD 缺 CheckSyntax 不可独立编译）✅ 已修 = 105c62c；F-B-1 内置全集超宣称 ✅ 已修 = f32464c DisableAllBuiltins（字段依赖调用运行期 fail-closed / 字面量常量折叠无害 / now() 逃逸面消除）；A 批（B1-2 守卫/92304 三处登记/引用形态）逐项属实。**新登记 3 项（复核 N 系前缀——与双路 F-B-x 撞车区隔，后者同号异义 ≥4 处）**：N1 级联快照「防篡改」定性反了（P2·B1-6 前拍——A 引用 B 取提交值 = 篡改经引用固化通道，零测试覆盖，详见 A.3 复核注）/ N3 $env 禁用建议 `DisableBuiltin("env")` 医不对症（P3·随 B1-6——语言级变量非内置函数）/ N2 exprcalc 头注与零函数现状矛盾（P3·随 B1-6 函数注册表）；余开：超时注释失实 / `EvalEnv.Now` 死码（P3·随批） |
| 2026-10-22 | **所有者场景走查定稿批（表单联动全链——canonical 例：漏洞修复工单「填主机 IP → 自动带出责任人（可人工改）」）**：①**N1 终局=禁止 formula 引用 formula**（超出两级联/维持两案——复杂组合归注册函数与 fetcher 接力，配置面一跳，链式问题整类消除；两级联方案作废，A.3/A.6 同步）②**字段锁死拍板**（§2/A.5：存量字段仅 label 可改，改类型/改 key/删除 400，新增与排序自由——ServiceNow 稳态型；Jira 类型不可变/SF 删字段依赖门对标）③**§3.3③⁺ auto_fill 全链定稿**（语义分层终版「默认值而非终值」/params 多参绑定泛化替换单 trigger_by+适配器入参契约声明/一查多填/编辑时预填交互与三档错误 UX/链式限深 ≤3+提交多趟收敛 ≤3 趟/兜底总是补空/detail 静态徽标——实现随 phase6 R3 首源批）④**impact 端点角色缩小+归位管理组**（锁死后破坏性告警消亡，剩新增 required 在途提醒；catalog「ticket_list 消费」注记失实勘误——ui 仓零消费实证；外审 F-D-1 P0 移挂方向修正）⑤**§2 detail 渲染差距行勘误**（S1 执行期已闭合）⑥**A.6 语义收口批测试矩阵定稿**（禁链式/锁死守卫/impact 归位/$env 拒绝四组正负向，含 $env 既有测试名实不符重写与 catalog impact 子测反转）⑦**六项拍板终版（所有者"按推荐"）**：锁死范围=**兼容性判据版**（硬锁三件 type/key/删除、软提示两件 options 收窄/required 加严、其余自由——判据"是否破坏已落库数据或旧单编辑"；附带语义改公式/挂取数不回溯存量单；严格字面版因自锁死区[错误公式不可修正]与挡 B1-6 布局/存量加取数而弃）/ impact 归位=与 fields/replace 同组 / auto_fill 兜底=总是补空 / 编辑时带出=维持 phase6 R3 / $env 检测=token 级 / ui 仓 e2e 键数断言修复=并入语义收口批。实现分两波：锁死守卫/禁链式保存校验/impact 归位=近期修复批；prefill 端点/params 泛化/external_http 适配器/前端交互=phase6 R3 |
| 2026-10-22 | **文档勘误与计划同步批（第三路核查遗漏收口，先于语义收口批）**：①头注「迁移：零」→一枚 ALTER 000051 已实落（同文档自相矛盾消解）②§3.5「十种」→十一种（补 file）③§3.5 增 **file×附件体系交互五点**（建议口径 B1-6 首件确认——复用 B5 全链/额度共享 ≤20/MIME 双层取交/GC 删除走 attachment 语义/custom_data 只存 key 数组）④11 号 **B1-7 语义收口批立项**（~1–1.5 天排 B1-6 前——锁死/禁链式+引用存在性/类型漂移/$env token 级/impact 归位/92304 入 allErrCodes/ui 两件）+ B1-6 量级统一 5–6 天 + phase6 R3 前置批登记（③⁺ 四件 ~3–4 天触发驱动）⑤phase6 README R3 同步（③⁺ 待建四件 + field_options→field_meta 承载勘误）⑥review/11 B1-6 行增量补登（file 第 11 类型） |
| 2026-10-22 | **B1-7 三批全落（S1 语义收口告毕——A.6 五组矩阵全部实施）**：批1 $env token 级+92304 入册（9194f6c）/ 批2 后端五件+迁移 000052（f657aa8——锁死守卫兼容性判据版/禁链式 N1 终局/字典码存在性[只查存在不查启用——与运行期展开语义对齐]/fetch 三校验/类型漂移提交期硬 400/impact 归位）/ 批3 ui 两件（ui 1a94cf46——detail 字典 label/e2e 硬断言）；验证=15 子测红→绿+四档 241/0+双仓 CI 绿。余=B1-6 能力面（首件前确认 file×附件五点 §3.5） |
| 2026-10-22 | **P1-1/P2-6 修复批 + 文档一致性收口（全量核查报告[24 笔未推送]采纳执行——所有者「按建议来」）**：①**expandDictOptions 错误臂返 `[]`**（P1-1：DB 故障原返回原文→optionValues 字面量单选项→一切合法 select 值 400 拒，fail-closed 实效与注释 fail-open 意图相反；回归测试三臂先红后绿 dict_expand_integration_test——与三路F-B-8「码不存在→空集放行」= 同函数两互补臂，后者由 B1-7 保存期存在性收口）②validateCustomData 三重 isEmpty 死代码合并（P2-6，行为零变化——全量集成回归绿）③类型漂移方案拍定=**提交期重算后类型校验硬 400**（保存期预判不可实施——A.6 补用例组）④A.6 勘误两处：TestCatalogExempt 措辞（全集循环无专属子测）/92303 悬空引用删注（沿用 ErrInvalidParams）⑤③⁺ mapping 目标键不重复注 + prototypes Sortable 1.14.0 版本差注⑥11 号 B1-7 行三路 F-B-x 前缀化 + A.5 保存校验②③挂 B1-7 载体 + §3 迁移计数统一（六枚/下一编号 000052）+ 05 号两处「四枚」→六枚（详各稿变更记录） |