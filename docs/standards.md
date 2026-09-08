# 平台工程公约（zhuzhao 生态）

> **适用范围**：zhuzhao / taskrunner / activelist / 后续所有内部项目。各项目仓库的工程文档引用本文件，**不必在各自文档中重复约定**。
> **地位**：跨项目工程公约的 **SSOT**。§25（design-decisions）为权限架构决策 SSOT；[16 号 §9](./phase3/16-external-integration.md) 为服务实例化落地表（允许差异/对齐清单），与本文件冲突时**以本文件为准**并回改本表。
> **修改纪律**：改公约 = 所有者拍板 + 本文件更新 + 受影响仓库文档同步（各仓变更记录留痕）。
> 建档：2026-09-04，由 16 号 §9 基线升格扩编而来。

---

## 1. 项目定位与分工

| 项目 | 形态 | 职责 | 权限 |
|---|---|---|---|
| **zhuzhao** | 单体（IAM 内核 + 统一网关 + 通用能力底座） | 认证/鉴权/用户/角色/组织/菜单/审计/事件发布；对外统一入口（网关） | 三层鉴权全量（L1/L2/L3）；权限管理面 |
| **taskrunner** | 独立仓库 + 独立部署 + 独立 Redis + 独立 PG | 事件/任务总线：调度、重试、死信、记录、看板（通用调度，**不做业务判定**） | 无用户权限模型；服务间 AK/SK |
| **activelist** | 独立仓库 + 独立部署 + 独立 PG | 动态数据模型薄层（类型注册/Schema 演进/数据 CRUD/导入导出） | 用户侧零权限（网关挡）；服务间 AK/SK |

- 微服务拆分不做（无多团队/M2M 需求）；**抄模式不抄架构**。
- 工单模块已封版（对接公司内部平台；重启条件见 design-decisions §23）。

## 2. 服务间通信与鉴权

| 项 | 约定 |
|---|---|
| 协议 | **HTTP + JSON**；gRPC 不引入 |
| 服务间鉴权 | **AK/SK HMAC 签名**（utils `aksk`：按调用方发 SK、env 注入、常量时间比较、时间窗 ±5min 防重放）+ **专用 network 双防线** |
| 身份断言 | 明文 `X-Operator` 头 + **纳入签名覆盖**（不可伪造）；缺失兜底 `"system"`。AT 透传验签（方案 A）降为触发条件驱动 |
| 密钥管理 | 内部服务 = env 静态 SK；外部 M2M 平台凭据（api_keys 表/签发吊销）🚦 随调用方出现（09-platform，算法复用 `aksk`） |

## 3. API 设计约定

1. **方法仅 GET / POST**——PUT / DELETE / PATCH 不引入；
2. **POST URL 不携带业务信息**——资源标识/动作参数全部在请求体（GET 的 path/query 参数不受限）；
3. 响应结构统一 utils `errcode` + `response`；业务失败直接映射 HTTP 状态码（4xx 不可重试 / 5xx 可重试），**响应体不带状态字段**；
4. 错误处理走 errcode 映射，**禁止 raw 500 泄漏内部细节**；
5. 存量豁免：zhuzhao 工单管理面 5 处 PUT/DELETE（ticket-types/templates，封版不改）；新端点一律按本约定；
6. **列表端点必须分页**：page/page_size 服务端钳制 + 确定性排序键（通常 id DESC）——禁止无分页全量返回；
7. **int64 ID 一律字符串序列化**（`json:",string"`）——规避 JS Number 精度丢失；
8. **时间统一 RFC3339**（传输）+ TIMESTAMPTZ（存储）——容器 TZ 统一（§4）；
9. **新增错误码必须登记 api/errcode.md**，编号沿用既有分段，不私造新段。

## 4. 工程结构与代码组织（所有服务同规格）

| 项 | 约定 |
|---|---|
| 分层 | handler → service → repository（handler 禁 DB、service 禁 HTTP、repository 禁外层依赖） |
| 目录结构 | 各服务同构骨架：`cmd/<server>/` 入口、`internal/app/`（wire 装配）、`internal/handler|service|repository|model|middleware/`、`internal/pkg/`（服务内公共构件，**升格 zhuzhao-utils 的候选区**）、`configs/ migrations/ scripts/ docs/`；同名目录跨服务职责一致 |
| 依赖注入 | google/wire（装配收敛于 `internal/app`） |
| 配置 | yaml + `${VAR}` 环境变量展开；敏感值（SK/密码）env 注入不入库不入 git |
| 优雅启停 | 信号处理 + 依赖关闭顺序；**防孤儿进程**（重启先杀端口占用） |
| 门禁 | 统一 Makefile：`lint`（vet + gofmt）/ `test` / `build`；zhuzhao 另有 `test-integration`（-race -p 1）与 `acceptance` 四档链 |
| 健康检查 | `/healthz`（存活）+ `/readyz`（检各自硬依赖：Redis/PG） |
| 时区 | 容器固定 `TZ=Asia/Shanghai` |
| **代码注释** | **关键函数必须注释**：导出符号有 doc comment（说什么）；复杂逻辑/安全相关/非直观分支注释写**为什么**（不复述代码）；对外 API 带 swagger 注记。惯例：中文注释 |
| **文件规模** | 单文件 **超过 200 行即评估拆分**（按职责/子域切，不机械按行数硬切）；新增文件尽量低于该阈值。存量豁免：已冻结模块（ticket）与历史大文件不做突击拆分，触碰时顺手拆 |

## 5. 公共包（zhuzhao-utils）

- 清单：crypto / errcode / jsonutil / jwt / logger / postgres / redis / response / validate / aksk（**10 包；aksk 已实现未发版——随 v0.2.0 发版后各仓去 replace**，2026-09-04 审计校准）；
- **抽取边界：只抽无数据依赖的纯工具**——绑定 zhuzhao 库 schema 的构件（如策略谓词）不进 utils（design-decisions §25.3）；
- 版本策略：语义化版本，发版后各仓 pin（去 replace）；临时 replace 仅限联调窗口；
- **新增第三方依赖必须说明理由与替代方案**（go.mod 铁律）；公共能力优先沉淀进 zhuzhao-utils 而非各仓自引。

## 6. 可观测性

| 项 | 约定 |
|---|---|
| request_id | `X-Request-ID` 头进出全程透传；zhuzhao 生成（`req-`+32hex）→ 透传下游 → 回调带回；与业务 body 内 request_id 同键关联（job_runs / trace_id） |
| 访问日志 | 统一中间件出口，每请求一行（method/path/operator/trace_id/参数截断/状态）；网关对 proxy 路由**跳 body** |
| 审计 | **审计正本全在 zhuzhao**：请求级（audit_logs，脱敏+截断）+ 登录显式审计 + 业务操作点显式发布；服务自身零业务语义日志 |
| 判定日志 | L2/L3 判定落 `policy_evaluation_logs`（B11①，E-① 已实施）；归档超期导出 JSONL 后删行（B11②/E-③，保留期 180 天可配置） |
| 日志框架 | utils `logger`（slog + lumberjack，JSON Lines，字段稳定命名供 ES 演进） |

## 7. 权限架构（SSOT：design-decisions §25；三层详设：authz.md）

1. **PDP/PEP 分工**：身份/角色/组织关系**集中一份**（zhuzhao 库）；行级策略**归属主自治**；行级 PEP 必然在数据处——网关只做认证 + 路由级授权 + 身份断言透传，**不做行级判定**；
2. **三层**：L1 路由级（Casbin，policy=角色×path×method）→ L2 数据范围（策略库 builtin 或手写 Resource，Registry 统一接缝）→ L3 动作矩阵；
3. **平台策略库**（触发驱动）：`org-member`/`owner-only`/`role-gated` 三内置 + `Builtin()` 一行注册；**适用前提 = 资源表在 zhuzhao 库**；跨库资源走参数级组装（E-⑤ 模式）；
4. **Q5 禁令**：L2/L3 判定每请求实时查询，**不缓存**；任何缓存提案必须连同失效级联方案评审；
5. **ReBAC/PBAC 不演进**（触发器 = 11-authz §5 清单；命中时经 ResourceAuthorizer 接缝换判定后端，L1 不动）；
6. 角色自定义全链（角色 CRUD / AssignMenus / org_roles 组织赋角 / BFS 三源展开）为平台本职，所有服务共用。
7. **权限码命名与注册**：码 = `resource:verb`（K8s verb 规范同款，禁复合码）；服务 API 的码↔路由绑定一律迁移申报（声明式，与代码同版本，design-decisions §26.2），运行时只开放菜单/角色绑定管理，**不开放 API 绑定编辑**。

## 8. 数据与迁移

| 项 | 约定 |
|---|---|
| 迁移文件 | **up/down 成对**；编号**全局唯一**（zhuzhao 仓内按 A2 规则：谁先启动谁占用，后者整体重排）；**各独立服务迁移编号自成体系，与 zhuzhao 无关** |
| 写法建议 | 优先幂等（IF NOT EXISTS / ON CONFLICT / DO $$ 判列）；zhuzhao 集成测试容器按 testutil **显式清单**重执行——**新迁移必须同步登记该清单**，漏登 = 测试容器缺列全包炸 |
| 建表基线 | id 自增 / version 乐观锁 / created_at·updated_at / 软删 deleted_at（唯一索引配 `WHERE deleted_at IS NULL` 部分索引）；业务唯一键语义固定后建部分唯一索引 |
| 事务与并发 | 跨表写**必须同事务**（检查+写入同事务，防 check-then-act 窗口，B4-5）；悲观锁按场景选用（FOR SHARE 防写锁窗口读旧快照 / FOR UPDATE 队列竞争 / advisory lock 串行化长操作，BK-11/B3-2 模式）；**全表带 version 乐观锁**（并发写 → ErrConcurrentModification）；失败路径必须整体回滚不留部分写 |
| 跨库边界 | 服务间**不直连对方库**；身份数据需要时走事件同步副本（契约由数据接收方定义） |

## 9. 测试约定

| 项 | 约定 |
|---|---|
| 三档分层 | **单测**（无 build 标签，plain `testing`，不起容器）/ **集成**（`//go:build integration` + testify + `-race`，共享 PG 容器）/ **acceptance**（bash 黑盒 API 验收，四档链式——与 Go 测试互补：前者测权限语义，后者测端到端流程） |
| 容器与迁移 | 集成测试共享 PG 容器（testutil）+ **显式迁移清单**——新迁移必须同步登记该清单，漏登 = 测试容器缺列全包炸（000019 教训） |
| 数据隔离 | fixture **每跑唯一化**（uniqueSuffix）；清理用 `t.Cleanup`（软删释放唯一索引/删行）；跨测试残留数据 = 隔离债，修 bug **必带回归测试** |
| 负向用例 | 权限类功能**强制三断言**：可见 / 不可见→404 / 越权→403；错误注入（DB 错误→拒绝不留部分写） |
| 隔离复验 | `-count=1` 为门禁基线；`-count=2` 作数据隔离的复验手段（隔离债的探测探针） |
| 测试即验收 | 测试 = 可执行的验收标准：写不出测试说明设计未想清（回 §11 设计阶段） |

## 10. 安全基线

| 项 | 约定 |
|---|---|
| 登录 | 失败锁定（Redis Lua，fail-close）；登录事件显式审计（含 method=sso/local） |
| 密码 | 复杂度策略（网关化后=门户责任，待实施 C）；HR 同步账号密码为占位值，SSO 上线后默认禁账密（`auth.local_password_for_hr`） |
| 限流 | API 级限流（网关职责，复用 Redis Lua 基建）；登录限流 fail-close |
| 会话 | AT/RT 双 token；Logout/改密吊销；SSO 后 `source=hr` 禁账密旁路 |
| MFA | 未立项（触发驱动） |

## 11. 开发工作流（先框架 → 先测试 → 分批实现）

**设计原则：对齐业界实践**——实现可以先不完整，**设计思路必须与业界成熟形态一致**。设计评审的必答题是「业界怎么做」：说不清就先调研，有对照再动手（现有对照范例：PDP/PEP = NIST 框架、投递语义 = at-least-once、服务签名 = AWS SigV4 式 HMAC、可见性 = tag-based ABAC、组织树 = ltree；总方针 = **抄模式不抄架构**）。「业界没有先例」多数情况说明问题定义有误。

**标准节奏（2026-09-04 所有者拍板）**：

1. **决策面清零再动工**：动工前把拍板项/语义规则全部关闭并写进设计文档（参照 M-E 的 P1–P7 流程——决策清零后「剩余均为实施项」）；跨仓库的契约变更**先改契约文档、联调冻结前提出**（C11 教训）；
2. **先搭框架，不急着实现**：接口/类型/路由/装配/迁移骨架先立起来（保持编译通过），行为留空或显式返回「未实现」（如 501/`ErrNotImplemented`）——**禁止静默空实现**（半成品被误当完成是最难排查的坑）；
3. **测试先行**：按设计把测试用例**全量**写完（正/负向/边界/权限拒绝路径），先跑出红——测试就是可执行的验收标准，写不出测试说明设计还没想清；
4. **分批实现**：按设计分小批填肉到绿，**每批结束保持全门禁可过、主干可交付**，不留跨批次的半成品；
5. **小步快跑提交**：一个逻辑批次一个 commit（中文提交信息 + 验证证据），**禁止一次提交大段代码**——提交粒度以「可独立 review」为锚点（参照 b4cfe96：16 文件为量级感知上限）。

**配套纪律（本项目实测教训）**：

- 收口/替换类操作用 grep 全量清点对账，**不凭记忆宣告完成**（setupD9 漏网教训）；机械批量替换（sed）后必须全量 grep 残留并逐行核对（多行 VALUES 曾被吞括号）；
- 基础设施登记与代码**同批**：新迁移 → testutil 迁移清单 + 迁移地图；新端点 → 权限码/menu_apis + SSOT 索引；
- 改 API 契约后**重启常驻进程再验收**（acceptance 打 33333 常驻 server，旧进程 = 过期 API 假绿/假红）；
- 门禁未绿不算完成；门禁失败如实报告，不掩饰不跳过；
- 完成（DoD）= 门禁绿 + 文档回填（SSOT/变更记录）+ 变更评审说明三节。

## 12. 工程与协作纪律

1. **提交**：中文提交、分批提交（一个逻辑批次一个 commit）、**先验收后提交**（全门禁绿才可提交）、push 单独指示；
2. **变更评审说明**：任何 `internal/ migrations/ configs/ cmd/ docs/` 改动附带三节——改动摘要 / 影响面 / 验证证据；未跑门禁必须如实说明；
3. **文档体系规范**：docs 树 = `modules/`（能力文档）/ `phase1-3/`（阶段计划）/ `review/`（评审发现）/ `adr/`（重大架构决策）/ `design/`（设计推演与细节）/ `proposal/`（提案），顺序编号 + README 索引。**分工**：ADR = 重大不可逆架构决策（事件机制/任务执行器/集成形态），design-decisions = 演进中的设计细节拍板（编号顺延不回收），modules = 能力的长期文档。每个 SSOT 文档必须带**变更记录表**；**文档断言必须能与代码对账**（状态以代码为准）；
4. **编号 namespace**：W（Wave）/ IW（独立窗口）/ BK（backlog）/ E/D（16 号配套）/ P（待拍板）/ F/C/AB（各评审文档本地）——引用前必查归属；
5. **触发条件驱动**：暂缓 ≠ 搁置——未命中的能力不提前实现，触发器写清（冻结的工单模块同此纪律）；
6. **跨仓库契约变更**：先改契约文档（双方仓库同步）→ 再改代码；契约冻结（联调开始）后变更成本陡增；
7. **分支与版本**：功能分支开发（如 `feature/phase-2`），合入前全门禁；zhuzhao-utils 语义化版本，发版后各仓 pin。

## 13. SSOT 索引

| 主题 | 权威文档 |
|---|---|
| 权限架构（PDP/PEP、网关、策略库、ReBAC 触发器） | design-decisions §25 |
| 软删组织委托语义 / SoD / Phase 3 重定位 / SSO | design-decisions §21 / §20 / §22–24 |
| 服务实例基线（允许差异、taskrunner 对齐清单 C 系列） | phase3/16 号 §9 |
| taskrunner 设计 / API / job_runs | taskrunner 仓库 docs/taskrunner.md |
| activelist 契约 / 存储模型 | activelist 仓库 docs/ADR-003-integration-contract.md |
| 排期（里程碑/人日/主链） | phase3/13 号 §1 |
| 事件机制 / Asynq | ADR-001 / ADR-002 |
| 工单（封版历史设计与对接参考） | phase2/09、phase3/10 |
| AI/协作者流程协议（zhuzhao 仓） | AGENTS.md |
