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

预设模式（每天 HH:mm / 每周几 / 每 N 小时 / 每月几号 + 时间段选择）+ 自定义表达式专家模式；产物仍是 `cron_spec` 字符串——taskrunner 侧解析与 tick **零改动**；`next_run` 回显作保存后预览。**选型预置：自写轻量组件**（预设 = el-select + 时间选择组装五段表达式），不引第三方 vue cron 组件（质量参差；五段子集面小，自写可控）。**可点原型随行（2026-10-19 双钻复盘立约）**：B4-2 开工首日半天——假数据把「预设/专家双模式切换 + 四类预设生成 → next_run 预览」交互流亲手点一遍（所有者即第一个用户），再定稿面板布局——UX 手感面验证前移至设计定稿前（与 S1 属性面板原型同批纪律，双钻 Develop 侧用户验证型发散）。

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
| 死信并入运行记录状态筛选 | **✅ 对账结论（2026-10-24）：一一对应不成立——维持三 Tab**。死信=asynq archived 队列残留（Redis 侧，retry 即出列+保留上限），runs=job_runs 表（PG 审计事实）——两存储行数语义天然不对齐，A.4 判据走"不收敛"臂 | ✅ 关闭（2026-10-24） |
| operator 动作只读端点权限 | §3.2 拍板点 | S4 启动拍 |
| params schema 动态表单 | §3.3 远期半，勿重复立项 | 已登记 |
| **taskrunner patch 通道 callback_url 死字段清理（2026-10-17 核出）** | W0b SSRF 收口后三路径（create 拒收用户值/cron 触发读侧覆盖/手动 trigger 读侧覆盖——`cron.go:85`、`task_service.go:588`，出站一律配置值 `TASKRUNNER_CALLBACK_TARGET_URL`）下 jobs 表 callback_url 列已成死存储，但 `patchJobReq.CallbackURL` 仍在 API 面透传（更新死列，**无安全影响**）；清理 = 删字段+测试，一行级 | 挂账（触发 = 下次动 taskrunner 随批，或 B4-1 联动顺手——同一张任务定义契约） |
| **任务结果留存/展示 + 结果·数据源 × 活动列表（所有者 2026-10-26 问询登记）** | 现状核实：taskrunner `job_runs` **无 result/output 列**——运行详情仅状态/耗时/attempts/params 快照/error（error=唯一结果载荷且仅失败有），cron/手动触发跑完均无输出可看；所有者两新设想 = ①任务结果**可配置是否落 activelist** ②任务**数据源可配置为 activelist**（两仓未实现、全仓此前零登记；与 phase6 README §3 理想态③「子任务数据面=activelist」同族）；结果展示交互样例已有 = [研究稿 05 号](../../deliverables/software-company/secops-platform-vision-2026-10/05-script-node-contract.md) §7 IP 富化/§8 证书巡检（运行记录 = 元信息+input/output+日志+工件四段）。前置 = taskrunner 契约扩展（result 存储+读端点+网关透传+详情面渲染） | 挂账（触发 = phase7/8 脚本平台立项批一并，或所有者明示单独立项） |

### 3.6 任务域需求备忘（保留争议，不排期）

所有者 2026-10-09 设想 = 任务支持**上传自定义脚本运行** + 定时策略 + **stdout 回显** + 工单流程中编排。**争议保留点**：上传脚本 = 平台代行任意代码（安全/沙箱/许可均未议），本备忘**不构成 15 号 🚦 触发信号**、不进基线/量级/拍板清单。既有资产备查（启动时直接复用勿重新调研）：[phase3/15](../phase3/15-script-platform-dagu-vs-inhouse.md) Dagu(GPL) vs 自研完整选型（§7 五待拍板 = 许可/沙箱/依赖策略/PoC/安全）、16 号「用户上传脚本 🚦 按需再启」；增量点仅二 = stdout 回显（15 号未单列）与流程中编排（落点 = S3 动作库「提交任务」既有登记，非 DAG）。**是否启动全部留所有者。**（2026-10-20：已进入 phase7/8 规划视野——研究稿 deliverables/software-company/secops-platform-vision-2026-10/；本 phase 口径不变，仍不排期）（2026-10-26 所有者续问四件核实现状：**结果展示**——ui 任务详情弹窗只有状态/耗时/params 快照/错误，无输出区（taskrunner 无 result 列），交互样例=研究稿 05 §7/§8 运行记录；**结果可配置落 activelist**/**任务数据源可配为 activelist**——两仓均未实现亦无登记，本批挂账 §3.5；**计划任务入参**——现状=定义级静态 JSON（新建定义时手写 textarea、创建后 ui 无编辑入口——updateJob 的 params 指针可改但未做编辑面；cron 每次触发原样提交 `cron.go:86`，无变量/取数；动作注册表 params_schema 已随 /jobs/actions 返回但仅作下拉清单，schema 动态表单维持 §3.3 远期半））

## 4. 启动拍板点

| # | 事项 | 建议 |
|---|------|------|
| 1 | 动作只读端点权限码归属 | 预置挂 `task:read`；operator 侧按提交口径放行最小读面 |
| 2 | 死信并入运行记录 | 先做 taskrunner 聚合对应确认，成立则收敛两 Tab |
| 3 | ProcessAt 是否随批 | 维持触发驱动不预做（16 号后置既定） |

> **状态注（2026-10-16）**：本表拍板点已随批量预确认拍定（2026-10-16 所有者「按建议」——✅34/🔶1，状态 SSOT = [11 号 §4](./11-implementation-plan.md)；各段启动时异议可翻，翻案成本=改一行）。

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

## 附录 A：详细技术设计（实施级，2026-10-16 补充——样板深度对齐 05 号附录 A）

> ⚠ 标注 = 实施级设计建议（启动批确认制）。拍板点 1/2/3 已批量预确认（11 号 §4）。

### A.1 动作元数据端点（B4-1）

```
GET /api/v1/jobs/actions                # 挂 task:read（拍板点 1 预确认：task:read + 最小读面）
{ "code":0, "data": { "list": [
  { "action_id": "audit_archive",
    "name": "审计归档",
    "description": "按保留期归档审计日志到 JSONL",
    "kind": "handler",
    "owner_service": "zhuzhao",
    "params_schema": {...} | null       # 未声明 schema = null（前端降级 JSON 模式）
} ], "total": 7 } }
```

kind / owner_service 为多执行端预留字段（2026-10-20 规划研究回灌，依据 deliverables/software-company/secops-platform-vision-2026-10/08-phase5-adjustments.md）。

- **`jobs.Registry` 注册体扩展**：现 `Register(actionID, Handler)` → `Register(actionID, Handler, ActionMeta{Name, Description, ParamsSchema json.RawMessage})`——meta 缺省允许（存量注册零破坏，前端降级）；ParamsSchema 钉为 **JSON Schema draft 2020-12 子集**（与 S1 渲染器、AI 工具定义同语言；2026-10-20 回灌）；
- **最小读面边界（拍板点 1 预确认口径）**：响应**不含**参数默认值快照/历史运行数据/凭据类字段——动作清单是「能提交什么」，不是「跑过什么」；与 `/jobs` 数据面（admin 专属）语义分离；
- **命名衔接**（§3.4）：端点响应字段 `action_id` 维持现网关代理组口径；taskrunner 能力目录落地时对齐其 `action_code` 命名（随批）。

### A.2 fail-fast 预校验（B4-1 随行）

- **落点 = zhuzhao 提交代理 handler**：`POST /jobs`（Create/Trigger 代理）转发前查 `jobs.Registry.Exists(actionID)`——未注册 → 400（errcode 随批登记，如 21010 ErrActionNotRegistered）；
- **校验范围边界**：zhuzhao registry 只含 **zhuzhao 侧回调动作**——taskrunner 侧任务定义（admin 下拉已有）不经此校验（其合法性由 taskrunner Create 校验，既有）；预校验仅覆盖「zhuzhao 回调型动作」提交路径；多执行端（脚本沙箱 / 连接器）= phase7+ 按 taskrunner 能力目录方案触发后扩展，当前仅 zhuzhao 回调型（2026-10-20 回灌）。

### A.3 cron 构造器组件（B4-2，前端）

```
zhuzhao-ui/src/components/CronBuilder/
  index.vue            # 双模式 tab：预设 / 专家（表达式直填+robfig 五段语法提示）
  presets.ts           # 预设模式 → 五段表达式纯函数（单测承重：每种预设的生成/反解析）
```

- 预设四类（**五段表达式，对齐 taskrunner `NewParser(Minute|Hour|Dom|Month|Dow)`——无秒字段，2026-10-16 跨仓实证**）：每天 HH:mm（`M H * * *`）/ 每周几 HH:mm（`M H * * D`，D=dow 0–6）/ 每 N 小时（`0 */N * * *`）/ 每月几号 HH:mm（`M H D * *`）；
- **双向**：专家模式手填表达式 → 反解析回显预设（可识别时）；不可识别保持专家态（不强行归一）；
- 产物 = `cron_spec` 字符串原样提交，taskrunner 解析零改动；保存后 `next_run` 回显作预览（既有）；
- 五段非法值（月 13 日 32）前端预检 + taskrunner 校验兜底（400 回显）。

### A.4 死信收敛前置确认（拍板点 2，B4-3 前置）

- taskrunner 侧聚合查询验证：`dead_letter` 表行 vs `job_runs.status='dead'` 行**一一对应**（SQL 对账脚本一次性跑演示/测试双栈）——成立才收敛 Tab（死信 Tab 并入运行记录 `status=dead` 筛选）；不成立（如重试产生多条 dead_letter 对一 run）则维持三 Tab 并登记差异清单。

### A.5 验收补充（在 §8 基础上）

- 元数据端点：operator 持 task:submit 可读清单（200）/ 无任何 task 码 403；响应不含参数快照与运行数据（最小读面断言）；
- fail-fast：未注册动作提交 400+21010（**替代既有回调期 404**——负向必测）；存量已注册动作提交路径零变化；
- 构造器：四类预设产物的表达式经 taskrunner `ParseSpec` 解析成功且 `next_run` 一致（**六段带秒输入必须被拒**——parser 无秒字段，负向必测）；专家模式非法表达式 400 回显；
- 降级：`params_schema=null` 动作自动 JSON 模式；有 schema 动作渲染动态表单（远期半，B4 期不实现渲染器时此项顺延）。

---

## 变更记录

| 日期 | 变更 |
|---|---|
| 2026-10-15 | 建档（phase5 分支十段设计稿批）——现状锚点逐段代码核实（含水立挂账收窄：前端近似水位条已落）；未新增决策 |
| 2026-10-16 | **详细技术设计批（附录 A，实施级——05 号样板深度）**：A.1 /jobs/actions 端点契约（Registry 注册体扩 ActionMeta{Name/Description/ParamsSchema}缺省允许存量零破坏；**最小读面边界**——不含参数快照/运行数据/凭据字段；action_id 命名衔接 taskrunner action_code）/A.2 fail-fast 预校验落点（zhuzhao 提交代理 handler 查 Registry.Exists，400+21010；**校验范围=仅 zhuzhao 回调型动作**注释防误扩展）/A.3 cron 构造器组件（四预设→五段表达式纯函数+专家反解析+非法值双检）/A.4 死信收敛前置（taskrunner 一一对应对账脚本，不成立维持三 Tab 登记差异）/A.5 验收补充（operator 读面/21010 负向/八预设一致性/降级） |
| 2026-10-17 | **§3.5 挂账 +1（所有者问询「回调地址改名字」后代码核实）**：名字制本体**已落地**（taskrunner W0b SSRF 收口批——create 拒收/cron+trigger 读侧配置覆盖，用户只传 action_id）；本批仅登记 patch 通道死字段清理挂账（触发驱动，B4-1 联动顺手） |
| 2026-10-24 | **S4 全批交付（B4-1 zhuzhao 80adb2c+ui d39fe5eb / B4-2 ui 014fc42c；B4-3 对账结论=B 维持现状）**：①**B4-1**：jobs.Registry 扩 ActionMeta（Register 可变参缺省存量零破坏；旧 Actions()[]string 更名 ActionIDs）+GET /jobs/actions 本地直读端点（000066 绑 task_center 行 task:read 码——operator 提交选单口径；audit_archive 注册带元数据首例）+**提交 fail-fast 21010**（Submit/CreateJob 查 Registry 未注册即 400；Trigger 不拦防误伤多执行端）+operator 提交对话框 action 手填改可搜索下拉（加载失败回退手填）；②**B4-2**：CronBuilder 组件（预设四类/专家双模式/双向反解析/产物 cron_spec 五段 taskrunner 零改动）接入任务定义表单；presets 纯函数 17 测（六段带秒必拒/步进 */N 先于 daily 判定坑在册）；可点原型纪律合并入段走查；③**B4-3 死信对账结论=一一对应不成立**：taskrunner 死信=asynq Inspector archived 队列残留视图（Redis 侧），runs=job_runs 表（PG 审计事实）——两存储行数语义天然不对齐（archived 保留上限+retry 即出列）→ 按 A.4 判据**维持三 Tab 不收敛**（运行记录 status=dead 筛选已覆盖审计面查询诉求）。验证=22 包单测+fail-fast 负向+四档 246/0+ui 四件绿（test 175） |
| 2026-10-19 | **可点原型随行批（双钻复盘立约——§3.1）**：B4-2 开工首日半天可点原型（预设/专家双模式切换 + 四类预设生成 → next_run 预览，假数据亲点后定稿面板布局）——UX 手感面验证前移至设计定稿前；同日 standards §12-10 立外审截断规则（详 standards 变更记录） |
| 2026-10-26 | **§3.5 挂账 +1 + §3.6 备忘补记（所有者问询「结果展示/结果落活动列表/数据源=活动列表/计划任务入参」核出）**：①§3.5 新挂账行——任务结果留存/展示（job_runs 无 result 列，error 为唯一结果载荷）+ 结果/数据源 × activelist 两设想（此前零登记，与 phase6 理想态③同族，phase6 README 已加指回注记）；②§3.6 补记——结果展示现状与样例指向研究稿 05 §7/§8；计划任务入参现状 = 定义级静态 JSON（创建后无编辑入口/cron 原样提交/schema 表单维持远期半）。实现零改动，纯登记 |
