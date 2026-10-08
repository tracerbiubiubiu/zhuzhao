# phase5/ — Phase 5 规划：前端可视化配置线（规划草案）

> **文档定位**：Phase 5 唯一规划文档（草案）。**状态 = 未排期、未启动**：批次划分/编号分配/量级细化待 Phase 4 收口后启动拍板（届时在本目录另立 02 号式实施计划）。本文件先落**目标形态、范围边界、前置触发**——与 `phase3/*` 同款纪律：设计就绪稿，非交付承诺。
>
> **立项由来**（2026-10-08 所有者指示，决策登记 = [design-decisions §27](../design/design-decisions.md)）：工单域配置面现状是"数据可配、界面兜底"——类型/字段/模板管理页已交付，但 states/transitions 走 JSON 源码模式、字段走表格全量替换（phase4/02 深水区降级决策的兜底形态）。所有者期望：**前端表单与运行流程都能在前端自定义配置**。同日追加指示：**taskrunner 任务面（cron/params/action 输入全手填）同步纳入本 phase 人性化演进（S4）**；**头像改图片上传（现 URL 手填）、工单字段支持富文本与图片/文档上传——MinIO 对象存储纳入本 phase 作公共底座（S5，IW2/10 号存储设计收编实施）**。

## 0. 一句话

把工单域与任务域的"配置面"从手填/JSON 源码兜底升级为前端可视化，并补齐富媒体底座，五段递进：**表单可视化设计（S1）→ 流程状态图可视化设计（S2）→ 流程运行语义配置化（S3，🚦）→ 任务面（taskrunner）配置人性化（S4）→ 对象存储底座与富媒体（S5，MinIO）**。S1/S2/S4 前端为主、存储契约零破坏；S5 为公共底座段（IW2 收编）；S3 后端为主，以工单自研翻案为硬前置。

## 1. 现状基线（代码级锚点）

| 层 | 现状 | 锚点 |
|----|------|------|
| 状态机引擎 | 通用引擎，从 `ticket_types.transitions` JSONB 构建，校验 from→to | `internal/service/ticket/state_machine.go` |
| 流程配置数据 | 每类型一套 states/transitions JSONB（默认六状态图）；字段 schema/模板同表族 | 迁移 000010；默认图常量在 `type_admin.go` |
| 管理端 API | 类型 CRUD（states/transitions 可写，`validateTypeGraph` 校验端点合法）+ 字段全量替换 + 模板 CRUD，乐观锁 | `type_admin.go` + `router.go:352-363` |
| 动作语义 | **代码硬编码**：Close/Assign/Update 各端点方法内按三层鉴权 + 属主/委托判定，状态变更经 `AssertTransition` | `service.go:330/:390/:432` |
| 前端 | ticket/type 页已交付：states/transitions=JSON 源码模式、字段=表格全量替换；工单发起已用 form-create 渲染器 | zhuzhao-ui `src/views/ticket/type/index.vue` |
| 任务面 | task_center 四 Tab 已交付（运行记录/死信/任务定义/提交任务），但 **action_id/cron_spec/params 全手填**；动作合法性回调时才校验（未注册→404，执行期才失败）；动作清单 = zhuzhao `internal/jobs` 注册表，**无读端点**；cron = robfig/cron 五段表达式，分钟级 tick 扫 `next_run` | zhuzhao-ui `views/task/list/index.vue`；zhuzhao `jobs.Registry` + `jobs_callback.go`；taskrunner `cron.go`、`repository/jobs.go` |
| 存储与富媒体 | **全仓无对象存储/附件能力**；个人头像 = URL 字符串手填（`users.avatar`，`max=500`）；工单字段类型七种，无富文本/文件类型 | `internal/model/user.go`（avatar 列）+ `user_request.go`（UpdateProfileRequest）；`type_admin.go` `validFieldTypes` |

**结论**：状态图本身已"数据可配"，缺可视化编辑界面（S2）与语义层可配（S3）；表单渲染器已在位，缺设计器（S1）；任务面四 Tab 已交付但全手填，缺"人性化输入面"（S4）；存储底座缺席，头像/富文本/附件三类富媒体诉求共用同一缺口（S5）。

## 2. 范围：五段递进

### P5-S1 动态表单可视化设计器（纯前端）

- 选型即定：form-create designer（`@form-create/designer` 拖拽 → rules/options JSON）——[12-frontend「阶段 2（远期可选）」](../phase3/12-frontend.md)转正；工单发起页渲染器已在位，本段只补**配置侧**。
- **后端存储不变**：产物仍是 `ticket_type_fields` 字段 schema；字段类型仍限七种（input/textarea/number/date/select/multi_select/tips）。
- 新字段类型 = 另立后端批：渲染器 + `validateFieldInputs`/G2 schema 校验同步扩，遵守"只增不改"兼容纪律，不混入 S1。
- 现 ticket/type 页表格模式保留为**专家模式/兜底**（phase4 深水区降级决策的形态直接复用）。

### P5-S2 流程状态图可视化设计器（纯前端为主）

- states/transitions 画布化：节点=状态、连线=合法转换，产物序列化回现有 transitions JSONB，**后端零改动**。
- 画布选型（vue-flow / AntV X6 等）启动批调研后拍板；中文化与交互打磨计入量级。
- 可选校验增强批（纯增量、向后兼容）：`validateTypeGraph` 现只查端点在 states 内，可加 lint 级检查——终态可达性/孤立状态/无出边死锁告警（是否阻断保存待启动拍板）。

### P5-S3 流程运行语义配置化（后端为主，🚦 双前置）

- 目标形态：转换级配置——**谁能触发哪个转换**（角色/属主/处理人）、转换前置条件、**自动分派规则**（[phase3/10](../phase3/10-ticket-business.md) `assignment_rules` 蓝本）、（远期）**审批节点**（phase3/10 BranchedStateEngine + `workflow_*` 蓝本）。
- 硬前置：① [design-decisions §23](../design/design-decisions.md) 工单自研翻案条件成立（phase3/10 设计可整体复用为蓝本）；② S1/S2 收口。
- **架构约束**（不可绕）：
  - 权限判定必须落策略层——类型级策略数据 + fail-closed，经既有三层鉴权/Registry 形态接入；**禁止业务层手写绕过**（AGENTS 约定；§25 平台策略库"事实在 DB/语义在代码"的分界随翻案批重审）。
  - 迁移编号按启动时 11 号 §4 迁移地图对账占用（A2 规则：撞号让位重排）。

### P5-S4 任务面（taskrunner）配置人性化（前端为主 + 动作元数据小批）

- 现状痛点（task_center 四 Tab 已交付、输入全手填）与对策：
  - **cron 手填五段表达式 → cron 构造器**：预设模式（每天 HH:mm / 每周几 / 每 N 小时 / 每月几号 + 时间段选择）+ 自定义表达式专家模式；产物仍是 `cron_spec` 字符串——taskrunner 侧 robfig/cron 解析与分钟级 tick **零改动**；jobs 列表已回显 `next_run` 可作保存后预览。
  - **action_id 手填 → 动作下拉**：zhuzhao `jobs.Registry` 暴露**元数据读端点**（id/名称/描述/params schema），提交任务表单换搜索下拉；顺带**创建时 fail-fast 预校验**（现状未注册动作到回调时才 404，失败后置到执行期）。
  - **params 手填 JSON → 两步走**：近期保留 JSON 模式 + 动作说明旁挂（现"勿填口令/密钥/PII"提示与 64KB 预检维持）；远期动作注册时声明 params schema，前端按 schema 渲染动态表单——**与 S1 form-create 渲染器同构复用**（未声明 schema 的动作自动降级 JSON 模式）。
  - 可选后端随行（触发驱动，不预做）：[16 号](../phase3/16-external-integration.md) 后置项「一次性延迟任务 ProcessAt」若启用，提交表单增"指定时间执行一次"模式（与 cron 构造器同面板）。
- **协议衔接（勿另起炉灶）**：动作元数据面 = taskrunner `docs/taskrunner.md`「能力目录」两级路由方案（2026-09-15 定稿、实现后置）的**单服务前段**——目录解决"提交方不手填回调地址"，S4 解决"人不手填 action_id"，共用同一份动作注册元数据；跨服务目录（taskrunner 查目录、端点自注册、`owner_service` 多调用方）维持其既有触发（多服务接入时启用），S4 不依赖、不提前。

### P5-S5 对象存储底座与富媒体（MinIO，后端底座 + 前端上传面）

- **底座（IW2 收编实施）**：[phase2/10-storage.md](../phase2/10-storage.md) 设计现成——S3 兼容客户端（开发 MinIO / 生产云 OSS 仅配置切换）、**预签名上传/下载**（前端直传，后端不扛文件流）、`file_objects` 元数据表、对象 key 规范 `{domain}/{yyyy}/{mm}/{uuid}.{ext}`、10MB 上限 + MIME 前缀白名单。本 phase 为其**实施载体**：compose 增 MinIO 服务（或 config 指向已有 S3）、storage 模块 + 预签名 client、迁移按 IW2 预留号启动时对账、**errcode 91000–91999 段规划随批落**（00 号既有登记）。
- **头像上传**（自服务面）：profile 头像由 URL 手填（`users.avatar` max=500 字符串）改预签名直传 + 回写对象引用；上传组件 + 即时预览。**下载形态启动批小拍板**：预签名 GET（有时效，img src 非稳定）/ 公共读桶 / 后端代理下载端点——头像对登录态可见且需稳定展示，三选一随批定。
- **工单附件（文档上传）**：10 号 `ticket_attachments` 蓝本实施——预签名上传 + 关联 API + 详情页展示/下载；权限码口径（复用 `ticket:update` vs 独立 `ticket:attach`）按 10 号 §1 既有拍板点随批定；删除语义按 10 号（删关联 + GC）。
- **富文本字段类型（richtext，第八种）**：字段类型枚举扩 `richtext`（`validateFieldInputs`/G2 schema 校验同步扩，守"只增不改"）；**正文 HTML 服务端消毒为 SSOT**（allowlist sanitizer——工单内容多人可见，防存储型 XSS；前端渲染不重复信任）；编辑器选型启动批拍（Tiptap / wangEditor 量级）；**编辑器内图片插入走同一预签名通道**（不引入 base64 内联，防库膨胀）；与 S1 设计器联动（富文本进可选字段类型调色板）。
- 顺序约束：附件/富文本均依赖底座先行，底座是本段第一个批次。

## 3. 明确不做（边界）

- **不做通用流程引擎/BPMN 全家桶**（Camunda/Activiti 级流程平台）——16 号 taskrunner 侧"不做通用流程引擎"红线同构适用工单侧；只做工单域状态图 + 语义配置。
- 不做跨系统编排（taskrunner 职责）；不做运行时热插拔自定义脚本动作（脚本注入面）。
- 任务面同构红线：**不做通用编排/DAG**（16 号"不做通用流程引擎"红线直接适用）；能力目录的跨服务自注册不随 S4 提前实施；params 脱敏维持后置（触发 = 日志出内网/接 ES，taskrunner.md §脱敏）。
- 富媒体边界：维持 10 号"不做"清单——病毒扫描/内容审核、>100MB 分片上传、CDN 独立域名均按需另立；云 OSS 生产替换 = 配置切换非代码分支。
- 不动 §23 既有拍板：S1/S2 只换配置界面、引擎与存储零改动，**不构成自研推进**；S3 开工本身即翻案动作，未翻案前零投入。

## 4. 量级草案（初估，启动批细化）

| 段 | 量级 | 迁移 | 主要风险 |
|----|------|------|---------|
| S1 | 1–2 周 | 零 | 设计器复杂度（phase4/02 已实证一次降级——故 JSON 模式常驻兜底） |
| S2 | 1–2 周 | 零 | 画布选型/JSON 序列化往返/交互打磨 |
| S3 | 3–4 周 | 有（策略/规则/审批表族） | 语义面大：与三层鉴权融合、存量类型兼容；审批流依赖的 2c Authorize 已就绪 |
| S4 | 1–1.5 周（动作元数据读端点 + 预校验 ~1 天随行） | 零 | 动作元数据维护纪律（注册不声明 schema/名称则表单与下拉降级现状形态） |
| S5 | 1.5–2 周（底座 1–2 天 → 头像 0.5–1 天 → 附件 2–3 天 → 富文本 3–5 天） | 有（IW2 预留号：`file_objects`/`ticket_attachments`） | 存储型 XSS（服务端消毒 SSOT）；头像下载形态/附件权限码两个既有拍板点；编辑器选型 |

## 5. 排期与触发

- **未排期**。硬前置 = **Phase 4 收口**（zhuzhao-ui 同仓共用门禁链；当前 W0–W2 已收口，下一步 W3 system 域）。
- S3 追加触发 = §23 翻案条件成立。
- S5 = 11 号 §8「独立窗口」IW2（附件/存储）的**实施载体收编**——IW2 行已补指向注记（2026-10-08），无额外触发；迁移用 IW2 预留号 000034 起（启动时按 A2 规则对账）。
- 启动时按 [VISION §4 修改纪律](../VISION.md) 级联：design-decisions（翻案批）→ roadmap 现状 → 本目录另立实施计划（编号 namespace 启用 **P5-***，防撞号——既有 namespace 已全占用）。
- 验收口径：前端件（S1/S2 设计器、S4 构造器与下拉、S5 上传面）走 zhuzhao-ui CI + E2E 链（phase4 已建）+ 场景走查（phase4/03 模式）；后端件（S3 全部、S4 元数据端点、S5 存储底座/附件 API/富文本校验）随四档 acceptance。

## 6. 文档衔接

| 文档 | 关系 |
|------|------|
| [phase3/12-frontend.md](../phase3/12-frontend.md) | S1 规格源（§3.1 DynamicFormField 七字段类型 / 阶段 2 设计器）；S1 启动时"远期可选"转正 |
| [phase3/10-ticket-business.md](../phase3/10-ticket-business.md) | S3 设计蓝本（assignment_rules / BranchedStateEngine / workflow_*；§23 起转对接参考） |
| [phase4/02-implementation-plan.md](../phase4/02-implementation-plan.md) | 深水区降级决策出处（设计器批后置 = 本 phase 由来） |
| [phase2/10-storage.md](../phase2/10-storage.md) | S5 底座 + 附件设计 SSOT（预签名上下行/`file_objects`/`ticket_attachments`/key 规范/MIME 白名单/配置样例）；头像共用 storage 模块（10 号 §0 即列） |
| taskrunner 仓 `docs/taskrunner.md` | S4 协议蓝本（「能力目录」两级路由定稿方案 / 动作注册表 / 内置例外与能力服务后置项——均实现后置，S4 不提前触发） |
| [phase3/16-external-integration.md](../phase3/16-external-integration.md) | S4 可选随行项出处（一次性延迟 ProcessAt / E-⑤ 部门可见性 / E-⑥ 终败通知——均触发驱动，维持 16 号后置不收编） |
| [design-decisions §27](../design/design-decisions.md) | 立项决策登记 |
