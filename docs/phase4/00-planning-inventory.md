# 00 · Phase 4 规划素材总盘点（规划输入，非计划）

> **文档定位**：Phase 4 首个文档。汇总截至 2026-09-18 全库散落的待办/建议/触发类登记，作为 Phase 4 定计划时的**唯一盘点入口**。计划本身（主轴拍板、批次排期、编号分配）待所有者拍板后在本目录另立文档。
>
> **盘点口径**：登记点 85 处 → 去重后独立事项约 55–60 项 → 其中真正开放的活跃项约 30 项，其余为「翻案/触发条件」挂起项。代码内 `internal/`+`cmd/` 零 TODO/FIXME（phase3/00 §4 既有结论仍成立），待办全部在文档。
>
> **同日补录（2026-09-18 能力对标业界检查）**：另发现三项**登记链断裂真空项**（§8.1，不在正文任何清单/雷达表内，建议进启动批）+ 两项校准（MFA/SCIM 非盲点，§8.2）。
>
> **2026-09-21 补录**：菜单词表只读化批（菜单 CRUD 业界对标核验产出，方案已定 B·只读）——登记 §2.4 主轴④，管理面重建触发登记 §3 信号组 C；前端工程架构五拍板（仓格局/底座模板/范例页制/views 对齐/部署解耦）——登记 §2.1 主轴①，初版设计文档 [01-frontend-design](./01-frontend-design.md) 已立。
>
> **状态基准**：2026-09-18。迁移号现状 = 000001–000029 已占用，**下一编号 000030**（11 号 §4 口径）。编号 namespace 已占用：W/IW/BK/AB/F/C/U/RT/CC/HC/MC/EC/OP/TC/P0/并发-Px——Phase 4 新立项**须启用新 namespace 防撞号**。
>
> **Phase 3 收口状态**：Phase 3 Wave W0–W4 全部收口（11 号 §8 A 档清零、B 档随行项完成），收官报告 2026-09-15 已交付。Phase 4 起点为 Phase 3 完整收口后的干净基线，无 Phase 3 未完成 Wave 需要带入。Phase 3 的 B 档随行项中仍开放的部分（附件/HR 同步/auth-enhance 独立窗口）已归入本盘点 §2 对应主轴。

---

## 1. 登记来源地图（在哪找待办）

| 来源 | 内容 | 权威性 |
|------|------|--------|
| `docs/review/11-project-control.md` §6/§8 | 健康状态表 + A/B/独立窗口三档 + 随手项 | **遗留问题权威清单** |
| `docs/phase2/00-implementation-plan.md` §9 | 代码级 backlog（BK 系列，与 11 号互为镜像） | 代码债权威 |
| `docs/phase3/00-startup-checklist.md` | 决策清单 3 项、§74 随手项、IW2 剩余 | 启动检查口径 |
| `docs/phase3/13-implementation-plan.md` / `14-planning-overview.md` | M-SSO/M-Mig/M-HR/M1 触发条件、M5 触发项表、U 不确定项、迁移号规划 | 排期 SSOT |
| `docs/phase3/16-external-integration.md` | taskrunner 后置项、密钥管理、E-⑤/⑥、activelist 契约整改 | 外部集成 SSOT |
| `docs/phase3/01/02/03/06/07/09/11/12/15` | 各子能力文档内嵌🚦触发项（多实例/HA/审计/安全/平台/分离/前端/脚本平台） | 子能力设计源 |
| `docs/phase2/01-auth-enhance.md` / `10-storage.md` / `11-authz-architecture-review.md` | IW2 两件、ReBAC 触发表、PG RLS 预案、scope 枚举映射表 | Phase 2 遗留 |
| `docs/proposal/hr-directory-sync.md` §3.2/§4.3 | HR 三拍板（离职在途工单/部门撤销级联/跨部门权限分配） | 仅此处登记 |
| `docs/VISION.md` §17 | 供应商协作场景（唯一登记在 VISION 的边界外场景） | 边界外立项入口 |
| `docs/ops/runbook.md` §4 | 四条「登记不修等触发」观察项 | 运营侧唯一定点 |
| `deliverables/software-company/phase3-closure-report-2026-09-15.md` §五 | 收官口径遗留（含 **taskrunner PG 备份口径随 M4——仅此一处登记**） | 收官权威 |
| `deliverables/software-company/zhuzhao-code-review-2026-09-11.md` | P2-2/P2-14 等代码级建议（**无 BK 编号，最易丢失**） | 待核实采纳状态 |
| 2026-09-18 gin-vue-admin 借鉴核验 | 产品完整线新增批（§2.1 主轴①） | 本轮新产 |
| 2026-09-21 菜单 CRUD 业界对标核验 | 菜单词表只读化批（§2.4 主轴④）+ 管理面重建触发（§3 信号组 C） | 本轮新产 |
| 2026-09-21 前端工程架构咨询（仓格局/模板/部署形态） | 前端工程架构五拍板（§2.1 主轴①） | 本轮新产 |

---

## 2. 五条候选主轴（Phase 4 主动线待拍板）

> 外部集成线（③）是被动等信号的；真正要拍的是主动线选谁。建议组合：**①为主动线 + ④穿插批 + ②③按真实信号插入**，⑤维持挂起（2026-09-18 建议；~~待拍板~~ **已拍：主动线=前端，2026-09-21 终确——见 [02 号 §1](./02-implementation-plan.md)**）。

### 2.1 主轴① 产品完整线（2026-09-18 gin-vue-admin 借鉴核验产出）

背景：gin-vue-admin v2.9.2 双向代码级核验（四子代理，以代码为准），抛开既有时机约束、以「产品完整性+运营杠杆」两轴重审后的可借鉴清单。

| 项 | 内容 | 前置/边界 | 量级 |
|----|------|-----------|------|
| 附件/文件上传 | = IW2 storage = phase2/10。工单系统无附件属功能残缺 | 启动先拍 phase2/10 三决策点（MinIO 选型/删附件仅删关联+GC/权限码复用 ticket:update 与否）；**占迁移号，与主链竞争按 A2 规则让位重排**；errcode 91000–91999 段待规划（代码中无预留声明） | 独立窗口 |
| 通知通道 | webhook 优先于 email（~~待拍板~~ **已拍（2026-09-21）：webhook 优先，配置能力=P4-2 本体含持久化+管理 API，配置页触发驱动后补**；企业场景主流+实现成本低）+ 抽象渠道接口配置化。场景 = 工单状态流转/待办 + taskrunner 死信告警（死信告警不依赖工单解封） | 新立项 | 2–4 天 |
| 前端 zhuzhao-ui | = 12-frontend（BK-18 类型/字段/模板管理页 + 动态表单渲染器，**后端 IW3 已就绪、规格已写好，仅前端未做**）。裸 API 的 IAM 是运营硬伤；gva 页面清单（superAdmin 七页 + systemTools 四页）可作需求蓝本；form-create（@form-create/designer）为字段设计器现成选型；gva 量级标定 120 .vue ≈ 季度级（1人专注假设；前端验收口径待定，非 make acceptance 覆盖范围） | 唯一登记点：closure report「前端 zhuzhao-ui 等既有触发驱动登记」 | 季度级 |
| **前端工程架构五拍板（2026-09-21，前端批设计输入）** | **① 仓格局=单仓单 SPA**：zhuzhao-ui 为全部页面唯一家（含 al/task 域——000022/000024 种子 component 值即此约定），模块=仓内目录域（`src/api/<域>/`、`src/views/<域>/`，共享壳层 `src/common/`）；**不分仓、不微前端**（壳层多份复制 / Go 门禁被 Node 工具链污染 / 一个控制台被 API 来源撕开，三重否决）；activelist 独立 UI 诉求出现再触发驱动立仓。**② 底座=裁剪模板**：以维护中的 vue3 admin 模板（vue-pure-admin/soybean-admin 量级）起底省重件（layout/构建链/表格封装），但**壳层四件必须按 zhuzhao 契约自写替换**（路由守卫/请求封装/权限指令/菜单渲染 = `/user/menus` addRoute + `/user/permissions` 三件套），模板自带 mock/假权限路由全弃；gva 生成器/热更新/引导初始化/库存代码勿抄在册。**③ 页面级=范例页制**：不做代码生成器——ProTable 列表封装（对齐后端分页形态）+ form-create 动态表单（12-frontend §3.1 已定，勿重复造）+ 2–3 范例页（列表/表单/树管理）作活文档约定载体，保「所有会话写出的页面长一个样」。**④ 命名对齐**：07-menu.md 契约 `src/views/{component}.vue` vs 12-frontend §4 `src/pages/` 冲突——**取 views**（25+ 菜单种子 component 值均按此约定，动态路由字符串直映射），前端启动批改 12-frontend §4。**⑤ 部署解耦**：源码组织与交付打包分离——nginx 静态容器（CI 构建 dist + 反代 API，衔接三栈 compose，**推荐**）vs go:embed 单二进制（前端构建步骤进 Go CI，不取）；源码只住 zhuzhao-ui，产物交付方式是另一层自由 | ① 为仓格局既定约束（立即生效）；②③④⑤ 随前端启动批实施，12-frontend §4 对齐随批落档；**初版设计文档=[01-frontend-design](./01-frontend-design.md)**（2026-09-21 同日立：选型基线补齐/壳层四件实现规格/状态分界/范例页/门禁口径/里程碑切分）；底座模板定案（四候选代码级评估：首选 vue-element-plus-admin v3/备胎 pure-admin-thin/Geeker ProTable 作参考，见 01 §9.1） | 拍板零成本；工程量并入上行前端条目（季度级） |
| MCP server（读侧） | 把 zhuzhao 管理/查询 API 暴露给 AI agent（AI 代运营：查审计/查工单/看死信/核对权限）。zhuzhao 全量 swag 注解+统一信封+request_id 比 gva 更适合；鉴权模式移植 gva（MCP 层透传凭据、判定回落主服务）——映射为一个 service 身份走 AK/SK。读侧先行，写侧后置 | 新立项；维护态系统压运营成本的杠杆 | 3–5 天 |
| 个人 API token（PAT） | 用户侧非交互凭据（脚本/CI 调用）。现状缺口：用户想脚本化只能硬编码密码。**与 09 号「AK/SK 外部 M2M api_keys 管理面」合并规划**（同一凭据面）；GitHub PAT 蓝本已在 design-decisions §26.5 挂名 09 §3.2 | **勿抄 gva 同 signing key 方案**；走独立 secret + sha256 落库 + scope 限定 + 可吊销；KeyGetter/NonceStore 接口未预置，启用时在 utils aksk 增设 | 2–3 天 |
| 字典/系统参数 | 业务枚举运行时化（工单自定义字段选项无处可挂是具体场景）；gva 形态（层级/启停/type 拉取）合适 | **边界：只做业务枚举/运维参数，不碰权限策略面**（平台策略=逻辑在代码，勿提议配置管理面） | 2–3 天 |
| 用户面导出 | 审计/工单查询结果 CSV/Excel（excelize 固定列即可，勿抄 gva 动态 SQL 导出模板） | **高危场景必须接 L2**（= BK-21 触发条件之一，同批实施） | 1–2 天 |
| 登录验证码 | 与既有登录锁定互补（锁定=事后果断，验证码=事中阻力） | 必须 Redis store（gva DefaultMemStore 多实例坑）；随上线前置 | 1 天 |
| P2 小件 | 审计响应体记录（截断+脱敏，现仅记请求体）/ panic 落库聚合+查询（gva sys_errors 朴素部分，AI 方案是噱头不抄）/ 运行时只读对账端点（syncApi 可取部分）/ 服务器指标采集（gopsutil + /system/info 端点，CPU/内存/磁盘；现仅有 /health/live、/health/ready 探活） | 各 ≤半天 | 合计 1.5–2.5 天 |

### 2.2 主轴② 上线运维线（全部有明确触发器）

| 项 | 内容 | 触发 |
|----|------|------|
| B7 CORS 收紧 | AllowAllOrigins → 白名单 | 上线时（所有者 2026-09-15 拍板暂不动；源头=phase2/01 D2-23：引入 cookie 会话前必须收紧） |
| 登录验证码 | 见 2.1 | 上线/公网 |
| B9 Casbin Watcher | redis-watcher + StartAutoLoadPolicy（移植 eiam `ioc/casbin.go`）；顺带清 `enforcer.go:35` StopAutoLoadPolicy 死代码（11-authz §9.3 债①） | 多实例部署 |
| M1 可运维基座 | 01-observability + 02-multi-instance + 03-audit-l2 三文档已就绪待实施 + 多实例验收环境（U6） | 部署形态升级为多实例 |
| L1 权限缓存 + Pub/Sub 失效 | `perm:user:{userId}` 缓存 + 跨实例失效 | 多实例+QPS 热点（09 号；守 Q5 禁令只盖 L1） |
| 06-ha 三件 | PG Cluster/Redis Sentinel/Nginx | 99.9%+ SLO |
| 决策清单三项 | K8s vs Compose（建议 Compose）/ Redis HA 方案（建议 Sentinel）/ 部署级分离时机 | 随真实部署拍板（phase3/README §4） |
| 档位 2 | 部署级分离（工单独立扩缩；档位 3 真微服务=推迟不做） | Phase 3 末可选验证（11 号） |
| 配置热重载 | gva 有 reloadSystem（重读配置+重连 DB）；zhuzhao 改配置需重启进程（reload 仅 Casbin 策略内存刷新）。上线后限流阈值/CORS 白名单等调参场景受益 | 频繁调参场景（也可明确标不做） |

### 2.3 主轴③ 外部集成线（Phase 3 已定调的被动主链）

| 项 | 内容 | 触发/量级 |
|----|------|-----------|
| M-SSO | 设计已定稿（design-decisions §24）；SLO 单点登出联动=迁移时拍板；纯网关模型下降级为「非网关路径独立 UI」可选项 | 进内网拿到公司 OAuth2.0 接入信息即开工；2–3 人日 |
| M-Mig | 内网网络/凭据对接 + **正式命名 + module path 迁移**（与换 Git 平台合并一次）+ **工单对接形态/Phase 2 资产处置拍板**（保留兜底/适配层/下线）+ 07/08 生产项重估（CORS/限流/审计保留期）+ 内部工单平台权限表达力评估（对照三轴模型，§23.3） | 迁移窗口 |
| M-HR | 预留接口版 HRFetcher ~~已在位~~（⚠ 2026-09-22 勘误：**未落码**，全仓无此类型，13 号「在位」/收官报告「✅ 完成」均虚标——真实对接时一并开工）；**启动拍三项**（离职在途工单处置策略/部门撤销×tickets.org_path 级联完整性须复用 OrgRepo.Move/跨部门权限分配规则），届时列硬验收项 | 真实对接；3–5 人日 |
| taskrunner PG 备份口径 | job_runs 保留期一并拍 | 随 M4（**仅 closure-report §五 登记，最冷门**） |
| 16 号后置项 | E-⑤ 部门可见性策略表（触发=跨部门隔离管控或任务参数敏感化；当前 params 不携带敏感信息是唯一实质泄露面）/ E-⑥ 终败通知端点（启用终败通知时）/ taskrunner 三则（优先级队列加权防饥饿/一次性延迟任务 ProcessAt/编排——**红线：不做通用流程引擎，首选外部 DAG**）/ nonce 单次校验（非幂等敏感写场景）/ skipbody 关闭（联调稳定后）/ per-前缀 body 上限（浏览器大文件导入需求，~30 行含测试）/ 幂等并发窗口+敏感字段脱敏钩子（登记观察）/ 密钥管理（KeyGetter/NonceStore 未预置）/ 方案 A AT 验签降触发（可达面扩大/行级判定/合规） | 各带🚦，详见 16 号 |
| taskrunner 错误码段 | 101000–101999 预留未启用 | 触发驱动 |

### 2.4 主轴④ 技术债清偿线（低成本，随时可穿插）

| 项 | 内容 | 量级 |
|----|------|------|
| **code-review 一颗雷（建议无论主轴先修）** | ~~P2-2 优雅关停未 join 判定日志管道~~ ✅ **已修**（policyeval.go WaitGroup + app.go Shutdown join，2026-09-18 代码核实）/ P2-14 `InsertPolicyEvals` batch_size > ~7281 行触发 PG 65535 参数上限（当前 BatchSize=200 常量，风险低；配置侧设上限仍建议）。**无 BK 编号，最易丢** | P2-14 半天 |
| B13 权限覆盖矩阵审计 | 全端点×三层×「设计内豁免 vs 遗漏」逐行对账产出矩阵文档；顺带产出 IAM 平面边界清单（§26.1 输入）+ 全文档 `internal/*` 路径核对（F-1 路径腐烂教训推广） | 0.5–1 天 doc-only，~~待拍板~~ **已拍（2026-09-21）：做——W1 收口后立即执行，W2 启动前必须完成**（02 §6 #3） |
| BK-21 护栏泛化 | fail-closed 哨兵泛化 registry 层（List 查询构造强制经 Filter）或 AST 守护扩展覆盖全部 repo.List 调用点；蓝本=Ruby Pundit `Policy::Scope` | 触发=首个新资源接 L2/导出功能；1–2 天 |
| **菜单词表只读化（2026-09-21 对标拍板）** | 现三写接口（`POST /menus`、`/menus/update`、`/menus/delete`）**零真实消费**：真实菜单全 `is_system=true`（000002/22/24/25 四批）改删即拒（`ErrMenuIsSystem`），07-menu.md「登记 API=热修入口」定位名存实亡；zhuzhao-ui 空壳无页面消费方。**拍板 B（只读）**：删三写接口，菜单行全走种子迁移；GET 树/详情保留（角色分配 UI 数据源），AssignMenus 绑定面不动。**弃 A（窄写面）依据**：① visible 仅显示层隐藏（route: 码照发、Casbin 照旧），弱于现有 AssignMenus 解绑（导航+权限码+Casbin 策略同事务收）——应急隐藏正确姿势=角色解绑，runbook 落档；② rename/icon/sort 低频化妆不值得换环境漂移（热修后 DB 偏离种子，新建环境 migrate-up 不一致）；③ 业界共识「词表进代码/绑定进数据」（Keycloak 无菜单管理、Grafana/Backstage 导航=代码、Casbin model/policy 同构分界），menus 表每列都是前端代码耦合物=词表非绑定。**改动清单**：删 3 路由 + handler/service 各 3 方法 + model 2 结构体（CreateMenuRequest/UpdateMenuRequest）；迁移（~~000030 单号~~ **拆 30/31 两连号——十一批：本清单属 000031 词表重排，BK-18 整改在 000030**；menu_apis 三行 + system_menu_create/update/delete 三按钮行 + admin/superadmin role_menus 绑定行，up/down 成对；phase1 断言 `>=25` 下界不破已核）；ErrMenuHasChildren 随迁清理（ErrMenuNotFound 留——AssignMenus 在用）；repo Create/Update 转 test-fixture-only 注记；swag 重生成；文档四处=07-menu.md（登记 API 定位改写+应急隐藏 runbook）/ design-decisions §26.2（词表随代码+重建触发注记）/ 11 号能力矩阵 / 迁移地图 | **初版估 ~1 天；终版范围已扩（B 案词表拆分+补三新按钮行（ticket_relation/task_operate/ticket_type_write）+验收账号基座+BK-18 方法整改+死码扩容+errcode.md 同步——2026-09-21 十一批定稿，**迁移拆 000030/000031 两连号**）→ 以 [02 号 §2 P4-W1](./02-implementation-plan.md) 为准，4–5 天**；~~迁移 000030 与附件批同争一号~~（**已失效——十二批：W1 占 30/31，IW2 让位 000032**）；批启动取新 namespace（现有全占用） | 
| 随手项打包 | BK-9 测试死代码 / F-31④ relation 越权负向用例 / F-32 audit·user service 分支单测（低优）/ TC-2 ListRelations·字段·模板 service 直测 / TC-3 非叶子节点 Move 并发 / TC-4 UpdateTicketType patch 测试 / 双删 404 回归断言（可选）/ Q5 组织赋角注记（doc-only）/ user_orgs 孤儿行清理（可选，已论证不清理亦可）/ P2-3 BFS CTE 双处一致性 / 角色复制（gva CopyAuthority 蓝本，克隆角色含菜单绑定；~2h） | 各 0.5–2h，合计 1 天内 |
| 03 号两拍板+一触发 | D2 判定日志 fail-open 最终确认（基本只剩「channel 满丢弃」语义）/ D4 判定日志是否默认全开（超量降采样 or 仅记拒绝）/ 主审计切异步（pipeline 开关未实现；启用须补停机 drain 等待） | 拍板零成本；异步=触发驱动 |
| activelist 契约整改② | 「动作进 URL」（POST .../schema、/deprecate、/:id/restore）~~待豁免登记或整改~~——**✅ 已拍板整改（2026-09-22）：W5 前置小批 ~1 天两仓（前端 al_types 页动工前改零返工），见 02 号穿插池行；standards 豁免诉求消失** | ~1 天 |
| 文档治理 | ADR 补 5–8 篇（三层鉴权/ltree 选型/双 Token+黑名单/迁移编号治理/ticket_visibility/虚拟组）/ architecture.md（1,899 行）+ design-decisions.md（1,414 行）拆分 / S-5 docs 中 15 处失效 `internal/*` 旧路径 / S-2 策略库「🟡 库就绪待接线」标注 / S-6 ADR-003 版本号滞后 / phase2/00:361 文档失实更正（tickets 无 version 列实走 status-CAS） | 纯文档约 1 天 |
| cron 调度依赖 | zhuzhao 自身无 cron 引擎（无 robfig/cron）；审计归档 cron 由 taskrunner 侧定义并回调（audit_archive.go 注释）。**风险：taskrunner 不可用时审计归档无自动周期触发**。评估是否引入轻量 cron 或接受外部依赖 | 初版估 0.5 天；**终版=收归实施批 1.5–2 天（含 taskrunner 退役），以 02 号穿插池为准（2026-09-21 拍板）** |

### 2.5 主轴⑤ 工单翻案线（全部挂起，等 §23 翻案条件）

翻案条件 = 公司平台无法承接工单诉求 或 迁移取消（design-decisions §23）。翻案后整套复活：BK-19 handler 测试（~0.5–1 天：httptest 绑定/L1 拒绝/正常路径）/ B1 Step 7 设计期拍板清单（SLA 暂停态/通知矩阵/§2.5 二选一/signal 双写/min_level 悬空→Assignee{rule,values}/分派深度/报表深度/TB 负向）/ B2 权限码 seed（ticket:approve 等）/ B3 in_progress·pending_verify 推进端点 / B4 BranchedStateEngine 本体 / B5 / 10 号重启点（7-0 修订/发布快照表 DDL/§4 节点 meta 更新）/ 12 号 W2·7c 前端页（审批人配置/审批操作/SLA 报表）/ created_org_id（Step 7e 报表按需）/ 审批流引擎选型。**新增设计输入（2026-09-20）**：类型级可见性策略（按类型配可见性：默认=发起人∪参与人∪抄送人，B 类=部门锚定）——S1 设计+四铁律+演进阶梯 T0–T7 判断点已落 [10 号 §10](../phase3/10-ticket-business.md)，触发表联动 [11-authz §5](../phase2/11-authz-architecture-review.md)（补行 #7+中间档位说明），前端配置页规格 [12-frontend §3.6](../phase3/12-frontend.md)；随 T0 翻案批取用。

---

## 3. 触发信号雷达表（信号 → 动作索引）

> Phase 4 计划中每个触发项标注三态：**主动做 / 持续监听 / 明确不做**。

### 信号组 A：部署/上线

| 触发信号 | 触发后做什么 | 编号 |
|----------|--------------|------|
| 正式上线 | B7 CORS 收紧 | B7 |
| 上线/公网 | 登录验证码（Redis store） | 07 号🚦 |
| 多实例部署 | B9 Watcher + enforcer.go:35 死代码清理；M1 基座 + U6 验收环境 | B9/M1 |
| 多实例+QPS 热点 | L1 权限缓存 + Pub/Sub 失效 | 09 号 |
| 99.9%+ SLO | HA 三件 | 06 号 |
| 真实部署拍板 | 决策清单三项 | 00 号 §3 |
| 工单需独立扩缩 | 分离档位 2 | 11 号 |

### 信号组 B：外部组织

| 触发信号 | 触发后做什么 | 编号/量级 |
|----------|--------------|-----------|
| 进内网/拿到 OAuth2.0 接入信息 | M-SSO（SLO 迁移时拍） | 2–3 人日 |
| 迁移窗口 | M-Mig 全套（含工单对接形态拍板） | 13 号 |
| HR 对接启动 | M-HR + 三拍板（离职在途单=业务后果最重） | 3–5 人日 |
| taskrunner M4 | PG 备份口径 + job_runs 保留期 | closure-report |
| 真实供应商接入诉求 | 先改 VISION 立项 → 外部账号类型+供应商组织形态+组级共享可见性 | VISION §17 |

### 信号组 C：需求

| 触发信号 | 触发后做什么 | 编号 |
|----------|--------------|------|
| 首个新资源接 L2 / 导出功能 | BK-21 护栏泛化（导出必须接 L2） | BK-21/B12 |
| 工单附件需求启动 | storage（先拍三决策点；占号按 A2） | IW2/10 号 |
| 强制下线/会话审计诉求 或 M-SSO 多端会话 | 在线用户管理面（原语全现成；基础版 0.5–1 天，按设备精确强退 +1 天） | 11 §8 |
| 浏览器端大文件导入 | per-前缀 body 上限 | 16 号 |
| 外部 M2M 调用方出现 | 密钥管理面（与 PAT 合并规划） | 09/16 号 |
| 第三方系统需接任务回调（外部 callback 诉求） | **决策树（2026-09-22 业界对照+十七批补第 0 档）**：**⓪内部新服务（最先命中）=taskrunner 配置 action→base URL 映射，调用方只传 action——不是 code，注册表只给管理员登记外部 HTTPS 用**；①**拉模式首选倾向**——外部持 PAT（P4-6）轮询 GET /tasks/:id 或事件查询，零新增出站面、复用三层鉴权（Stripe/GitHub webhook+polling 双通道惯例）；②必须实时+消费方可控 → 预注册 code 模式（代号→注册表映射 URL）；③消费方自带任意 URL → egress proxy 沙箱（Stripe Smokescreen 形态：DNS 解析后校验 IP 非链路本地/私有/metadata 段，防 rebinding）；④网络层出站控制（K8s NetworkPolicy/egress gateway，随部署决策清单）——W0 SSRF 修复后 callback 一律服务端定（用户级 URL 已拒），外部需求出现才开此面；**两条设计约束（2026-09-22 十八批补）：不做 push+pull 双执行引擎（拉模式=读 API 消费方式，非第二套执行器——防「zhuzhao 再轮询 taskrunner 做 handler」误解）；将来外部 webhook payload=通知型（task_id/status/run_id），业务细节以 GET 详情为准、webhook 不当 RPC（SaaS 事件模型惯例）** | 02 §2-W0/十六批 |
| 非幂等敏感写场景 | nonce 单次校验 | 16 号 |
| 跨部门隔离管控/任务参数敏感化 | E-⑤ 部门可见性策略表 | 16 号 |
| 启用终败通知 | E-⑥ 端点 | 16 号 |
| 事件多消费者/异步邮件 | 事件 L2 Outbox | 09 号/ADR-001 |
| 合规/安全要求 | 密码过期/异地登录检测 + MFA（standards.md:118「未立项（触发驱动）」在册，2026-09-18 对标检查补入本行） | 07 号🚦 / standards §安全基线 |
| 真实敏感字段诉求 | 工单字段级加密（AES-GCM） | BK-18 随手项 |
| RuoYi data_scope 需求形态 | 自定义部门集（~1–2 天） | 11-authz §1.2 |
| 多租户/租户自定义菜单诉求 | 菜单管理面按「组件池白名单 + manifest 下发」形态重建，**勿恢复自由 CRUD**（07-menu.md 未来钩子；design-decisions §26.2 注记随只读化批落档） | 菜单只读化批（§2.4） |

### 信号组 D：故障/运维/体验

| 触发信号 | 触发后做什么 | 编号 |
|----------|--------------|------|
| 多标签用户真实反馈 RT 误伤 | RT-1 宽限窗口/family_id 评估 + 20015 审计观测**同框**（降噪是加观测前提；「先 GET 比对、命中才 DEL」会削弱盗用检测须权衡） | RT-1 衍生 |
| BK-21 泛化后仍现漏调事故 | PG RLS 兜底预案（Supabase 蓝本） | 11-authz §9.3 |
| 正式部署形成 | **恢复流程 runbook 成文**（org 软删恢复谁/怎么/如何验证；taskrunner PG 备份口径冷门登记一并收——二十一批 §5.2） | ops/M1 同窗 |
| fence 失配（2xx 受理后接管失败）频率上升 | 死信自动重驱 | runbook §4① |
| org 恢复 / HR 对账启动 | 顺带清 org_roles 孤儿绑定行 | BK-12 残留 |
| 判定日志量超预期 | D4 拍板 + 主审计切异步（须补 drain） | 03 号 |

### 信号组 E：翻案

| 触发信号 | 触发后做什么 |
|----------|--------------|
| §23 工单翻案条件命中 | 主轴⑤整套复活 |
| activelist 独立项目成型 | G1/G3/G4 蓝图项（E13 已并入批次 B 完成；ADR-003） |
| ReBAC 触发表 6 条任一（跨资源关系链/per-resource 临时共享/策略运行时可配/多维组织/统一 PDP/ListObjects 核心化） | 评估 ReBAC 转向（§9.2：最现实=前两条；OPA 已复核不迁移） | 
| 脚本平台诉求 | 15 号重读 §2–§6；选 Dagu 先过 GPL-3.0 法务确认 |

---

## 4. 一次性拍板项（非触发，Phase 4 启动批候选）

| # | 拍板项 | 内容 | 成本 |
|---|--------|------|------|
| 1 | 03 号 D2 | 判定日志 fail-open 最终确认（基本只剩「channel 满丢弃」语义） | 零成本 |
| 2 | 03 号 D4 | 判定日志是否默认全开（超量降采样 or 仅记拒绝） | 零成本 |
| 3 | B13 | 权限覆盖矩阵审计做不做 | 0.5–1 天 doc-only |
| 4 | scope 枚举映射表 | 泛化 scope 1/2/3 与 ticket_scope all/group/assigned 缺映射表；scope=3 是否保留——11-authz §6 待落档清单 #5，**疑似仍未回标** | 半小时 doc-only |

---

## 5. 挂起观察（登记不修，勿开单）

- **RT-1 衍生观察**：20015 命中不落审计/WARN（安全信号仅响应码可见），须先有降噪前提（宽限窗口/family_id）再同框评估。
- **runbook §4 四条**：① 回调 fence 失配 2xx 受理后接管失败无自动重驱（观察任务失败率）② Logout×并发 Refresh 单设备毫秒级幸存窗口 ③ activelist `statement_timeout`<5s 时锁等待返回 500 而非 409（配置口径须 >5s 或 0）④ activelist 大导入真实上限=ReadTimeout 30s，更大走运维通道。
- **design-decisions §21**：已结工单委托残留=档案连续性，登记不修（翻案条件=删除即权限终止类合规要求）。
- **ReBAC 触发表 6 条**（行序冻结）/ **PG RLS 预案** / **自定义部门集**——见 §3 信号组 C/D/E。
- **在线用户管理面**——见 §3 信号组 C（「暂缓 ≠ 搁置」纪律，触发条件已对齐）。
- **roadmap 预留扩展**：多租户 tenant_id/第三方登录 oauth 字段/Kafka/K8s/RS256+JWKS（拆服务时）/密码过期——各带启用条件。
- **20009 JWT/AK-SK 互斥检测**：未启用占位（modules/auth.md）。
- **taskrunner 错误码 101000–101999 / errcode 91000 storage 段（待规划，代码中无预留声明）/ 20009–20013 预留段**：触发驱动启用。
- **zz_walk 演示残留**：deprecated 类型 + 动态表 3 行（演示库卫生，低优）。
- **U9 日历排期**：待给启动日+人力。

---

## 6. 过时失真行（盘点时勿当待办）

| 位置 | 失真内容 | 实际状态 |
|------|----------|----------|
| `deliverables/.../activelist-stage-a-walkthrough` §遗留 | 「activelist 54cec15 与 zhuzhao 配置/文档均未 push」 | 09-15/09-16 已推送，四仓齐平 |
| `docs/phase2/README.md` §93 | BK-13 仍标「待实施」 | IW1 已实施（2026-08-31，迁移 000017） |
| `docs/modules/middleware.md:169` | AKSK 中间件标「⏳ Phase 3b/按需」 | 内部 aksk 已全面落地（utils v0.4.0 四仓齐平） |
| `docs/review/11-project-control.md` §4 迁移地图 | 附件迁移号标「现 **000026**」 | 000026 已被 `job_submissions_claimed_at` 占用，附件实际下一号 ~~000030~~ **000032（十二批勘误：W1 已占 000030/31 两连号）** |

> 顺手修正建议：上述四行可在 Phase 4 首个文档批一并回标（归 §2.4 文档治理）。

---

## 7. 规划口径建议（定计划时使用）

1. **主轴拍板**：五选一主动线（建议①）+ 穿插线（建议④）+ 信号线（②③被动插入）+ 挂起线（⑤不动）。
2. **三态标注**：雷达表逐项标「主动做/持续监听/明确不做」（**三态落位见 [02 号 §3](./02-implementation-plan.md)——本表信号组 A–E 是信号→动作索引不带三态列，十一批措辞勘误**），杜绝「暂缓≠搁置」歧义（11 §8 在线用户管理面先例）。
3. **编号分配**：Phase 4 新立项启用新 namespace（现有 W/IW/BK/AB/F/C/U/RT 已占用）；迁移从 **000030** 起取号，附件若先启动按 A2 规则（谁先启动谁占用，后者整体重排）。
4. **启动批建议**：§4 四个一次性拍板项 + code-review P2-14 一颗雷（P2-2 已修）+ §8.1 登记链断裂三项定性，合计 ≤2 天，可作为 Phase 4 W0。
5. **门禁纪律不变**：每批次跑全四档 acceptance + 全门禁绿后提交（AGENTS.md 口径）。

---

---

## 8. 附录 · 能力对标检查补录（2026-09-18 同日，正文盘点后新增）

> 来源：所有者问询「当前能力与业界实践差距」的检查产出（分域对标 + 登记链断裂 grep 核验）。分域对标结论（安全内核一流/产品面二流/合规供应链三流）属评估判断不入本档；本节只收录**可执行的登记修正**。

### 8.1 登记链断裂三项（Phase 4 启动批候选）

| # | 项 | 证据（grep 核验） | 建议处置 |
|---|----|------------------|----------|
| 1 | **F-23 审计防篡改断裂** | review/09 F-23（🟠中）「audit/ticket_events 防篡改零设计（无触发器/RLS）」曾列 2b 处置项；核验：29 对迁移零 TRIGGER、phase2/3/11 号零回标——评审登记后从追踪链脱落 | **重定性**：要么实施（hash 链/WORM/触发器，~1–2 天），要么显式关闭并注明信任边界（「防 DBA 级篡改不在威胁模型内」）。现状最差：有结论、无处置、无豁免声明 |
| 2 | **依赖漏洞扫描真空** | govulncheck/dependabot/renovate/snyk 全库零登记零实施（Makefile/standards/CI 均 grep 空） | `govulncheck ./...` 入 Makefile 本地门禁（~半小时；与「无 CI 平台」取舍不冲突，供应链安全标配） |
| 3 | **密码历史/邮箱重置悬空** | architecture.md:1320–1326 + user.md:318 登记「Phase 2+ 可选（最近 5 次 hash 防重用 / 邮箱重置）」，从未进任何 phase 执行清单 | 随密码策略复杂度批（07 号/IW2 auth-enhance）同批定性——排期或移入 §5 挂起观察，终结悬空 |

### 8.2 校准两项（防误判为盲点）

- **MFA 非盲点**：standards.md:118「未立项（触发驱动）」在册；design-decisions §24 已论证 SSO 上线后 `source=hr` 账号禁账密避免绕过公司 MFA/密码策略——该细节本身对标一线 IdP 实践。已补入 §3 信号组 C「合规/安全要求」行。
- **SCIM 非缺口**：全库零登记，但身份生命周期入站方向由 M-HR pull 模型覆盖（内部场景合理替代）；协议层空白仅在外部协作者场景才有意义（已登记 VISION §17 供应商协作）。

---

*盘点方法：2026-09-18 全库扫描（Explore 子代理 × 1 扫 docs/deliverables 登记点 + 权威源 11 号 §6/§8、phase2/00 §9 人工复核）+ gin-vue-admin 借鉴核验（四子代理代码级对比）+ 同日能力对标业界检查（分域对标 + 登记链断裂 grep 核验，产出 §8）。*
