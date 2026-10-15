# 04 — P5-S4 管理面配置人性化：taskrunner 任务面（设计稿）

> **段属性**：基线段（phase5 收口对象）。**建议实施序**：S10 之后（依赖序宽松，靠后）；**先行批已落地**（2026-10-13），启动时在其上增强、不推倒。
> **量级**：1–1.5 周（动作元数据读端点 + 预校验 ~1 天随行；通知配置页已迁 S6）。**迁移**：零。
> **文档定位**：段级设计稿（2026-10-15 建档，phase5 分支）。协议蓝本 = taskrunner 仓 `docs/taskrunner.md`「能力目录」两级路由方案（2026-09-15 定稿、实现后置）；立项 = dd §27。

## 1. 定位与由来

task_center 已交付（P4-W5 + 2026-10-13 交互批），但配置输入仍**全手填**：action_id 手填（动作合法性回调时才校验——未注册动作到回调时才 404，失败后置到执行期）、cron 手填五段表达式、params 手填 JSON。所有者 2026-10-08 指示纳入 phase5 人性化演进。（通知渠道配置页原随本段，2026-10-08 第四轮整段迁 S6 通知中心页面族，本段回归任务面。）

## 2. 现状基线（代码级锚点，2026-10-15 核实）

| 项 | 现状 | 锚点 |
|----|------|------|
| 任务中心页 | **三 Tab**（运行记录/任务定义/死信+红点徽标）；提交 = 运行记录页首主按钮 + 对话框（admin=任务定义下拉自动补全 label=description、带出定义级默认参数、无参定义隐藏参数区；**operator 暂保持 action 输入**）；「只看我提交的」= UI 筛选非权限裁剪 | zhuzhao-ui `views/task/list/index.vue:3-14,38` |
| 队列水位 | **前端近似水位条已落**：页首排队/执行/失败/死信计数，30s 轮询——数据源 = runs 端点按 status 各拉 total（零后端改动；**语义 = job_runs 表态，非队列瞬时深度**，页头注释自注） | 同上 `:5-50`（`fetchWaterline`） |
| 动作清单 | zhuzhao `jobs.Registry` 注册表，**无元数据读端点**（`GET /jobs/actions` 未建，2026-10-15 grep 核实） | `internal/pkg/jobs/registry.go` |
| 任务代理 | `/jobs*` 代理组在位（List/Create/Update/Trigger），权限码 task:submit/read/manage（B13 矩阵：任务定义 Tab 数据面 admin 专属） | `internal/router/router.go:301-316` |
| cron 解析 | taskrunner 侧 robfig/cron 五段表达式，分钟级 tick 扫 `next_run`；jobs 列表回显 `next_run` 可作保存后预览 | taskrunner `cron.go`、`repository/jobs.go` |
| params 约束 | 64KB 预检维持；「勿填口令/密钥/PII」提示与快照列（000023）在位；脱敏后置（触发 = 日志出内网/接 ES） | taskrunner.md §脱敏 |

## 3. 目标形态与范围

### 3.1 cron 手填 → cron 构造器

预设模式（每天 HH:mm / 每周几 / 每 N 小时 / 每月几号 + 时间段选择）+ 自定义表达式专家模式；产物仍是 `cron_spec` 字符串——taskrunner 侧解析与 tick **零改动**；`next_run` 回显作保存后预览。**选型预置：自写轻量组件**（预设 = el-select + 时间选择组装五段表达式），不引第三方 vue cron 组件（质量参差；五段子集面小，自写可控）。

### 3.2 action_id 手填 → 动作下拉 + fail-fast 预校验

- zhuzhao `jobs.Registry` 暴露**元数据读端点**：预置 `GET /jobs/actions`（现网关代理组 `/jobs` 下扩展）+ Registry 注册体扩展 name/description/paramsSchema；提交任务表单换搜索下拉。
- 顺带**创建时 fail-fast 预校验**：未注册动作在提交时即拒（现状 404 后置到回调执行期）。
- **权限归属拍板点**：`/jobs` 数据面 admin 专属（B13），operator 提交选单需按 task:submit 口径的**动作只读端点**——权限码归属启动拍（预置方案挂 `task:read`，若 operator 需读则按提交口径放行，宁窄勿宽起步）。

### 3.3 params 手填 JSON → 两步走

- **近期**：保留 JSON 模式 + 动作说明旁挂（脱敏提示与 64KB 预检维持）。
- **远期**：动作注册时声明 params schema，前端按 schema 渲染动态表单——**与 S1 动态表单渲染器同构复用**（未声明 schema 的动作自动降级 JSON 模式）。
- 可选后端随行（触发驱动，不预做）：[phase3/16](../phase3/16-external-integration.md) 后置项「一次性延迟任务 ProcessAt」若启用，提交表单增"指定时间执行一次"模式（与 cron 构造器同面板）。

### 3.4 协议衔接（勿另起炉灶）

动作元数据面 = taskrunner.md「能力目录」两级路由方案的**单服务前段**——目录解决"提交方不手填回调地址"，S4 解决"人不手填 action_id"，共用同一份动作注册元数据；**跨服务目录**（taskrunner 查目录、端点自注册、`owner_service` 多调用方）维持其既有触发（第二执行端接入/zhuzhao 薄化/手填 URL 成负担三者其一），**S4 不依赖、不提前**。命名口径衔接：能力目录落地时用 `action_code`（taskrunner.md 2026-09-08 口径），S4 元数据端点命名随批对齐。

### 3.5 登记项（启动拍板/挂账）

| 项 | 内容 | 状态 |
|----|------|------|
| **队列水位精确化（挂账收窄，2026-10-15 核对更新）** | 前端近似水位条**已落**（runs total 聚合，§2）；挂账项收窄为**精确队列瞬时深度**——taskrunner asynq Inspector 队列统计端点 + zhuzhao 代理透传 + 水位条数据源切换；触发 = 任务量上来出现积压排查需求；量级 ≈ 半天 | 挂账 |
| 死信并入运行记录状态筛选 | 前置 = 确认死信行与 job_runs 一一对应（taskrunner 聚合查询），收敛后 Tab 减为两个 | S4 启动拍 |
| operator 动作只读端点权限 | §3.2 拍板点 | S4 启动拍 |
| params schema 动态表单 | §3.3 远期半，勿重复立项 | 已登记 |

### 3.6 任务域需求备忘（保留争议，不排期）

所有者 2026-10-09 设想 = 任务支持**上传自定义脚本运行** + 定时策略 + **stdout 回显** + 工单流程中编排。**争议保留点**：上传脚本 = 平台代行任意代码（安全/沙箱/许可均未议），本备忘**不构成 15 号 🚦 触发信号**、不进基线/量级/拍板清单。既有资产备查（启动时直接复用勿重新调研）：[phase3/15](../phase3/15-script-platform-dagu-vs-inhouse.md) Dagu(GPL) vs 自研完整选型（§7 五待拍板 = 许可/沙箱/依赖策略/PoC/安全）、16 号「用户上传脚本 🚦 按需再启」；增量点仅二 = stdout 回显（15 号未单列）与流程中编排（落点 = S3 动作库「提交任务」既有登记，非 DAG）。**是否启动全部留所有者。**

## 4. 启动拍板点

| # | 事项 | 建议 |
|---|------|------|
| 1 | 动作只读端点权限码归属 | 预置挂 `task:read`；operator 侧按提交口径放行最小读面 |
| 2 | 死信并入运行记录 | 先做 taskrunner 聚合对应确认，成立则收敛两 Tab |
| 3 | ProcessAt 是否随批 | 维持触发驱动不预做（16 号后置既定） |

## 5. 边界（不做）

- **不做通用编排/DAG**（16 号"不做通用流程引擎"红线直接适用任务面）；能力目录跨服务自注册不随 S4 提前实施。
- params 脱敏维持后置（触发 = 日志出内网/接 ES，taskrunner.md §脱敏）。
- 上传脚本不随本段（§3.6 备忘性质）。
- taskrunner 侧存储/调度零改动（构造器产物仍是 `cron_spec` 字符串）。

## 6. 影响面

- **迁移**：零。
- **权限**：候选新增动作只读面（拍板点 1；复用 task:* 码族，零新码为预置）。
- **跨仓**：zhuzhao `jobs.Registry` 注册体扩展 + 元数据端点；taskrunner 侧**零改动**（水位精确化项除外——Inspector 端点为 taskrunner 侧小改，挂账触发时另批）。
- **前端**：`views/task/list` 构造器/下拉/schema 表单三件，均在先行批形态上增强。

## 7. 实施序与批次建议

靠后实施（无被依赖面；S1 渲染器同构复用在其后更顺）。批次：①动作元数据端点 + fail-fast 预校验 + 提交下拉全量化（含 operator 口径）→ ②cron 构造器 → ③params schema 动态表单（远期半，随 S1 渲染器成熟度）→ 挂账项随触发插入。

## 8. 验收口径

- 前端件走 zhuzhao-ui CI + E2E + 场景走查；元数据端点随四档 acceptance。
- 场景标尺：admin 提交任务全程下拉/构造器无手填 → 未注册动作提交即 400（fail-fast 负向）→ 构造器产物 `next_run` 预览与 taskrunner 解析一致 → 无 schema 动作降级 JSON 模式。
- 回归：operator 现有提交路径零破坏；死信/运行记录数据面零变化。

## 9. 文档衔接

| 文档 | 关系 |
|------|------|
| taskrunner 仓 `docs/taskrunner.md` | 协议蓝本（能力目录两级路由定稿/`action_code` 命名口径/脱敏后置） |
| [phase3/16](../phase3/16-external-integration.md) | ProcessAt/E-⑤ 部门标签/E-⑥ 终败通知——触发驱动不收编 |
| [phase3/15](../phase3/15-script-platform-dagu-vs-inhouse.md) | 脚本平台选型备查（§3.6 备忘，勿重新调研） |
| [01-s1](./01-s1-form-designer.md) | params schema 动态表单同构复用其渲染器 |
| [README §2 P5-S4](./README.md) | 总纲（水位徽标挂账原文——本文 §3.5 已收窄更新） |

---

## 变更记录

| 日期 | 变更 |
|---|---|
| 2026-10-15 | 建档（phase5 分支十段设计稿批）——现状锚点逐段代码核实（含水立挂账收窄：前端近似水位条已落）；未新增决策 |
