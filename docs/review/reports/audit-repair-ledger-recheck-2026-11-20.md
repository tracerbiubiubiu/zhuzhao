# 存量修复计划全文复核（2026-11-20）

> 用途：复核 11 月 17 日计划的发现、主线实交与直接同类入口。原文固定于 `5087ef5a184ccb5d6543897f2ad47aecb24b0c26`。本轮为源码、合同与证据审计，合入范围为报告及状态文档。

## 基线与审计计划

冻结五仓主线：Zhuzhao `872a24073628a6226569f136b48c440e0f570367`，UI `8051448ef43a4636ec07e2fc13997c1c9a3e1252`，Taskrunner `1a2d4ab15a0a3ab44493f2250df45cb3f07b8acc`，ActiveList `d2fd4553bfa28c9ee7cb99855e89d026b7bf34bf`，Utils `661f465bd75a9c64c19aa6f14abfdd60f22fed15`。Go 模块中主仓与 ActiveList 消费 Utils 的 `v0.4.4-0.20261106080630-cd759a040245`，Taskrunner 消费 `v0.4.2`。Utils 主线 SHA 用于审计参照。[11 月 19 日合入和验证](../../planning/audit-repair-plan-2026-11-19.md#合入与验证证据)

1. 通读[原计划](../../planning/audit-repair-plan-2026-11-17.md) §1–6。把原编号、工作包、修复切片、启用条件和收据串起来，保留原证据的时点。
2. 并行核查任务执行与对账、IAM/F0 与工单、配置与通知、前端与 ActiveList。沿有效合同追踪实际入口、事务或异步边界、失败处理与对应测试。
3. 对任务取消/重试/启动配置、失锁收尾和跨事务撤权独立盲审。主会话核结论、反例与基线，在开工前仍需裁定的行为留给原合同窗口。
4. 将已验证窄修、未交路径与待验证假设回填既有文档。验候选差异及适用门禁后合入，按分支祖先、独有改动和占用情况清理。

原计划 §1 的依赖与启用顺序、§3 的扣重复估、§4 的交接与真实准入仍由原计划及其权威入口维护。§5 合同、历史产品候选及 pass8 的运行数据各自保持原基线。下文按 §2、§6 的问题及直接同类消费者对账。

## 权限、失锁与工单

- **F0、PLAT-R01/R02**。主仓单实例限制和失锁收尾已合入。 `internal/app/app.go` 的 `App.Run/Shutdown`、`internal/pkg/singleton` 的 `Guard.OwnerContext`、`internal/pkg/audit/policyeval.go` 的 `flushOnce` 将正常排空与失锁取消分开，Redis 批次确认绑定当前认领。已有独占 PG/Redis 验证正常排空与接管。PG 已提交但 Redis 尚未确认的持久行仍可重复处理，内存暂存和尽力分派事件仍可能丢失。Taskrunner 保留停止拉取后排空已接纳工作的独立合同。CF-01/08/09/10/49 的跨实例传播、共享归档、claim 与设置收敛继续沿 HA，目标环境接管和资格探测根因沿运维窗口核实。[主仓 PR #23](https://github.com/tracerbiubiubiu/zhuzhao/pull/23) · [失锁收据](../11-project-control.md#2026-11-19-f0失锁收尾)
- **同类待修：F0 容器停止预算**。`deployments/docker-compose.yaml` 的主仓 `app` 未配置 `stop_grace_period`。默认 Compose 停止宽限为 10 秒，短于 `App.Shutdown` 的 30 秒 HTTP 排空，应用之后还有日志和事件收尾。Taskrunner 的 `deploy/compose.yaml` 同样未配宽限，`App.Run` 依次收尾 HTTP 与已接纳的 worker 任务。按缺省停止时，慢请求和未完成的收尾可能被提前终止。原直接 SIGTERM 测试覆盖进程内排空。E09/G05/F0 应绑定明确的编排预算，并用实际隔离的容器停止路径验收。配置与源码确认，未观察生产发生情况。[Docker Compose 停止宽限](https://docs.docker.com/reference/compose-file/services/#stop_grace_period)
- **F1、IAM-A/B、CF-02/04/05/06**。组织赋角、保留直接角色和任职属性、父级停用、组织管理职责分开、敏感目标账号保护等当前窄修已合入。`internal/service/iam_guard.go` 的共同守卫与 `internal/service/user_service.go` 的 `ensureSensitiveCanManage/ensureTargetProtection` 是现行写侧入口。IAM-R02 保留平台角色与局部委托语义，IAM-R01 已验证拒绝旧策略缓存及受控重启恢复。字面上的「本轮已确认待实现」标题保留历史锚点，交付以合入报告为准。自动策略恢复、完整 IAM-C/C05、会话原子回执及全资源 revision 仍开放。[三仓合入](audit-baseline-main-merge-2026-11-17.md) · [pass8 复验](audit-pass8-followup-2026-11-17.md)
- **F3、F5 已交切片**。普通办理者经 `internal/service/ticket/people_service.go` 的 `PeopleService.Search` 走专用选人入口。AUTH-04 双端关联读取已合入，`Service.ListRelations` 校验请求端，`TicketRepo.ListRelations` 对对端应用读过滤，由 `TestRelationsUsePeerReadScopeInBothDirections` 覆盖。工单更新和公开评论的提交快照由 `TicketRepo.UpdateTx`、`Service.Update/CreateComment` 和 `TestCommitSnapshot_*` 核验。自动首派走 `DispatchService.OnAfterCreate` 到 `Service.assignAuthorized` 的初派分支，在票行锁内检查仍无处理人。可将这些结果登记为当前窄出口。[主仓 PR #26](https://github.com/tracerbiubiubiu/zhuzhao/pull/26)
- **F5、C04/C05 待交**。票行锁、同事务事件和提交快照保护工单事实，当前授权还涉及身份、直接及组织来源、资源策略和业务写入的共同顺序。逐入口复验建单、更新、评论与备注、分派、状态、关联、抄送、附件和后台动作的实际接线。撤权提交先于业务提交的保证要在共同屏障、同事务当前资格及所有相关写方上成立。收件人依赖当前资格的通知沿 D15/C04 补检查，现行分派器按提交事件解析收件人及偏好。[C04 工作包](../../planning/planning-backlog.md) · [11 月 19 日独立验收边界](../../planning/audit-repair-plan-2026-11-19.md#独立验收边界)
- **同类待修：C05 自助改密的凭据版本**。例如用户发起改密请求并通过旧密码检查，管理员随后先完成密码重置。原改密请求此时应被拒，不能覆盖管理员设置的新密码。`AuthService.UpdatePassword` 在取得 IAM 写入守卫前核旧凭据，`UserRepo.UpdatePassword` 不校验该证明仍对应当前凭据版本。两类凭据写虽串行提交，先取得的旧证明仍可能覆盖后已生效的管理重置。现有会话纪元及写后重读回归覆盖令牌，当前凭据证明仍需在同一事务重验。归 C05，验凭据版本变化后零密码写入、零新会话及合法改密成功。源码确认，未做安全场景实跑。[C05 共同合同](../../phase6/foundation-iam-module-detail.md#4-c05账号atrtpat-与停用恢复)
- **TICKET-R01、C02**。历史已删除键的可选值清理、合法替换、只改标题及 required/归属拒绝已有专项。当前普通附件的引用守卫、锁序、事务解绑和逻辑删除已合入，附件用途拒绝先核归属。历史数据清理、旧头像、回填、GC 与全量 C02 继续按各自出口处理。字典删除版本只能确认字典目标，字段引用预检仍先于删除 SQL，字段引用写方和类型子项变化尚无共同保护。沿文档已有兼容边界跟踪。[附件收据](../11-project-control.md) · [字典删除合同](../../api/dictionary.md#并发和兼容边界)

## 任务执行与对账

- **F2a 资格、归因与同号输入**。AUTH-02/06、SIB-R01、固定后台资格、同号异参及可信归因的基线和 pass8 出口已合入。主仓任务维护与 Taskrunner 调度的资格、来源和参数仍以最终执行时的有效合同为准。P2-7 的附加幂等字段及存量快照语义按合同扩展跟踪。[基线验收](audit-baseline-repair-2026-11-17.md) · [pass8 复验](audit-pass8-followup-2026-11-17.md)
- **F2a 干预与补偿**。Taskrunner 的 `TaskService.RetryTask` 按旧执行快照条件重置，保留新终态；人工重试继续使用当前队列预算。 `Service.enqueuePersisted` 和 `ReclaimLoop.requeueUnqueued` 避免撤销合法待重试项，坏快照扫描按固定上界主键窗口推进。语法坏行仍会轮转重查，对可解析但与账本语义不符的存量快照，补隔离与修复验证。 `TaskService.CancelTask` 先提交 DB 取消与清理指示，Redis 删除仍失败时由 `ReclaimLoop.probeCanceled` 重探并记录异常。Redis 预检不可用且已确认入队时仍返回错误，读到终态或执行代次已变时保留冲突响应。相关收据及本轮目标测试见下。[Taskrunner PR #10](https://github.com/tracerbiubiubiu/taskrunner/pull/10)
- **同类待修：取消清理的迟到投递**。`compensateCanceledDelivery` 删除失败时仅留日志。若取消清理已出域后才观察到投递，`Store.ListCanceledUnqueued` 不会再选中该取消行。使用固定主线的临时 SQLite 和队列替身，8 次均得到清理扫描返回 0 行。终态守卫仍拒绝该任务发起业务回调。归 F2a 既有取消对账，补验证迟到后的清理重入或等效收敛，保留取消终态和合法任务的队列副本。
- **F2a 时隙与完成边界**。 `Loop.FireDue` 尝试停用非法存量定义，发生次用原 due 时隙生成稳定 ID，由事务核对当前定义与发生次。匹配同一发生次才能推进。冲突快照保留原时隙，可能逐轮重遇；旧数据处置、描述编辑造成错失的恢复政策和完整 C03/F2b 均开放。 `worker.Deps.finish` 在最后一次持久化失败时把错误交回队列，最后一轮失败时仍可能出现数据库 running、队列归档。外部效果缺完整业务回执与未知对账。按各动作核重复效果和人工处置后再采用自动恢复。SIB-R02 的主仓凭证冲突误报沿准确对账出口复核。已完成任务在资格变化后的重复回执归 C03/F2b，验两仓结果收敛并保留当前资格控制。[调度恢复合同](../../design/schedule-recovery-policy.md) · [任务验收边界](../../planning/audit-repair-plan-2026-11-19.md#独立验收边界)

## 配置、通知与启用

- **P2-5 枚举窄修**。主仓 `internal/config/validate.go` 的 `Config.Validate` 拒绝未知 `server.mode`，保留空值归入 debug 的既定缺省及应急 CLI 的独立装配。Taskrunner `internal/config/config.go` 的 `Load` 拒绝未知 `db.driver`，服务和入队 CLI 都先加载并校验，SQLite 缺省保留。正式 Compose 显式指定 release/PG。将两仓的无效枚举入口标为已交，部署缺省与其它配置消费者仍各自验收。
- **P2-1 图与通知写入校验**。 `internal/service/ticket/transition.go` 的 `ValidateTransitionMeta` 校验键域、角色及表达式，动作名称和参数需与运行注册表核对。 `internal/service/ticket/type_admin.go` 的 `UpdateTicketType` 对本次请求提交的 meta 做校验，图单独改动、启用时保留 meta 须按实际有效配置复验。通知规则的业务域和动作组合，以及 `internal/service/ticket/dispatcher.go` 的 `MessageDispatcher.Dispatch` 对偏好读取失败、发送记录持久化失败的处理，随 D15/MH02 及实际启用旅程验收。
- **P2-2–4 专用消费与配置**。保留 (intel.qps) 的上游限流，`intel.query_rate_per_min` 在配置和缺省处可见，当前 Go 运行路径无该字段消费者。按现行通用限流路由实际覆盖判断影响。逐项核对超时扫描、验证码/情报 TTL、存储负值、归档常规参数与判定日志配置。固定归档已校验专属参数，常规运行、CLI 及旧配置保留各自治理范围。情报真实源切换时仍需验证旧桩缓存及标识来源。
- **P2-6、F5 可靠通知**。存储用量挂常规 cron，`StorageUsageSampler.Sample` 用本次轮询上下文调用异步 `HookRegistry.Dispatch` 后更新档位，`CronScheduler.tickOnce` 结束取消该轮上下文。需核真实计划轮询下的送达、失败可见及水位推进。超时催办、到期提醒和外部发送按各自承诺核验。若告警是某旅程唯一出口，须在实际启用前取得失败和处置证据。
- **同类待修：分用途用量指标保留旧值**。 `StorageUsageSampler.Sample` 只更新当前 SQL 分组返回的用途和总量，`internal/middleware/metrics.go` 的 `SetGauge` 按标签覆盖单项。某用途最后一个对象从事实表移除后，其标签值在同一进程内仍保留，可能误导按用途的排查。总量和档位用本轮总值计算。 `TestC6_StorageUsageAlert_E2E` 复采只断言无错。静态确认，挂现有存储观测后继，验非零→最后一条移除→标签归零或消失，以及其他用途和总量一致。
- **F4、F6 与真实门槛**。release 存储启动、文件及代表恢复和准确证据门已在原隔离栈验过。真实对象存储和恢复策略、异地/PITR 与跨介质一致性仍归 Q12/E09/G05；CF-18 在 F6 只完成本批取证/独审，远端强制与全量门禁自动化沿 G02/G03。Q09 已进入所选正式出口的条件，实际无人值守目标及批准仍待核定。B0 合成、人工风险试用、本地正式和真实 UAA 各走原准入。[首版准入](../../phase6/first-usable-release-plan.md) · [治理与当前健康](../11-project-control.md#6-当前健康状态遗留问题追踪)

## 前端与 ActiveList

- **已交的浏览器范围**。MyOrg 的 `createRosterSession`、状态图的 `snapshotTypeGraph/submitType`、字典的 `fetchItems/submitItem` 和角色菜单的 `createMenuEditor` 已隔离原错序场景。覆盖见 UI 的 `rosterSession.test.ts`、`p5-s2-flow-save-races.spec.ts`、`dictPage.test.ts`、`s4-role-menu-races.spec.ts`。11 月 19 日再交字典管理页启停保留备注、删除的目标身份与版本保护。[角色菜单 PR #10](https://github.com/tracerbiubiubiu/zhuzhao-ui/pull/10) · [三页面 PR #11](https://github.com/tracerbiubiubiu/zhuzhao-ui/pull/11) · [保真 PR #12](https://github.com/tracerbiubiubiu/zhuzhao-ui/pull/12)
- **同类待修：工单配置的稳定目标**。图、字段集和模板仍可只按编码与版本写入。主仓 `TicketRepo.UpdateTicketType/ReplaceTypeFields/UpdateTicketTemplate` 未核稳定 ID，删除按编码及各自现有约束处理。同码新建对象的版本可能与原快照相同，旧确认或编辑的服务端目标仍有错配风险。现有并发预检和 CAS 测试覆盖同一对象的版本冲突。同码重建按源码确认，仍待真实服务回归。类型、模板和字段配置归工单模块与 U04。验新对象持久值保持、同一对象版本冲突、未选对象零写，以及重新确认后的正常操作。
- **同类待修：字典关联及更新保真**。新增字典项经 `DictHandler.CreateItem` 只提交父类型编码，父对象身份未随写入核对。类型和条目的 `DictHandler.UpdateType/UpdateItem` 用普通字符串解析备注，省略与显式清空均落为空值。UI 已交的显式带备注覆盖日常启停，11 月 19 日计划中的 API 省略语义仍开放。与已有的字段引用和子项集合版本边界一并归字典模块，逐交付面验兼容性。
- **同类待修：角色菜单跨客户端集合**。`AssignMenusRequest` 不带菜单集合版本，`RoleRepo.AssignMenus` 锁定角色后全量替换。页面同角色重开隔离已交，后端现有并发测试允许两次替换均成功，只要求结果是其中一整套。两份客户端快照的最后一次保存仍可能覆盖已经提交的管理员修改。与 MyOrg 负责人集合的相近边界一起归 IAM-C/C04、U06。验较早集合保存冲突、最新集合不变和其他角色零写。
- **P2-11/P2-12**。AL 类型历史抽屉 `openHistory` 仍需按类型和请求世代保护，保存回包也需绑定编辑会话。通知管理已接模板、状态和业务 ID 的发送筛选，`fetchTemplates/fetchRules/fetchLogs` 的持续错误状态、`fetchLogs` 乱序、业务 ID 提示和真实旅程仍开放。归 AL 管理与 U06。筛选已接以 `TestMessageLog_Filter` 为证，页面空表不能证明服务成功。
- **P3-1/P3-2 和工单候选**。情报已有本机历史及结果时间，历史选择触发重查、结果标题与实际请求绑定，以及键盘访问仍归 U05。建单组织查询失败可落成未入组，字典首次失败缓存可影响后续恢复，设计器字典预览还需防迟到覆盖。工单关联候选沿现行可读列表，部分候选无写资格。归 U03/U04，真实浏览器验失败与成功空集、按当前上下文重读、候选和服务端最终目标。
- **其余 U02–U06**。任务 `openDetail` 的目标世代和查无/拒绝/故障区分待补。Panic 已接页码和总数，次页实测证据及快速翻页、失败保留状态不足。首页和消息中心待办与角标有自定义非终态范围差异，待办快翻与多来源归并沿 U02/U06、D10/R3。首页 FE-FULL-02 和全站 FE-FULL-10 去重跟踪失败、无权和成功空态，包含 MyOrg、活动清单及消息页的直接消费者。
- **ActiveList 普通写入已交，特定新用例仍待验收**。主仓菜单与 UI `w5-al-pages.spec.ts` 已证明既有活动类型由受权身份在正常页面创建、编辑、删除、恢复及读回。 `DataService.Update/mergeData` 保留未修改字段、数组边界和可选清空语义，旧版本拒绝并保留草稿。行操作的版本保护、坏全量导入回滚、合法尾空白及体积上限已有 ActiveList PR #6 的证据。原文「界面可达待证明」收窄到普通业务角色完整矩阵、受监控类型与所选 TD03/TD04 写入出口。[ActiveList PR #6](https://github.com/tracerbiubiubiu/activelist/pull/6)

## 验证与交接

- 主仓新跑 `TestConfig_Validate_Extended` 和 `TestReleaseComposeStorageAndSecretBoundaries`。Taskrunner 新跑 2 项配置、13 项服务层与 3 项 worker 定向测试，均通过。任务测试使用临时 SQLite、队列替身或本地 HTTP 测试服务器。
- 现有取消测试覆盖先落账再删、状态竞争、前置 Redis 不可用、清理错误重探。另用临时覆盖测试独立验证迟到投递的清理反例，`TestAuditLateCanceledDeliveryStaysInCleanupSweep` 连续 8 次都在「应保留清理行」断言失败，测试及临时数据库已移除。
- 原记录中的 PostgreSQL/Redis、浏览器与恢复收据按对应候选引用。本轮验证未覆盖真实业务、部署和完整运行门禁。已按合并提交或固定基线核对四仓与 Utils 的历史 CI 成功记录。报告及状态文档走规划参考档。

分工与复核使用已配置助手。质量与交付审计者（DeepSeek）、防御性安全与权限审查者（Codex）、发布与运维评审者（ZCode）、产品体验设计者（Codex）负责四路主审。另由 Codex 验收者盲核任务及启动，DeepSeek 架构设计师和验收者分别复核 F0/C04、C05 与存储指标，Codex 独立集成复核者再查错写边界与状态文档。

这次审计开始时，五仓各自只剩 `main` 本地分支和远端主线分支。清理范围为本次审计候选的本地与远端分支及其专属工作树、临时测试文件。合入后核实祖先关系、独有提交、工作树状态和使用方，确认材料交接后再清理。原 F0 快照是已有的 detached 工作树，继续保留作为历史入口。

后继优先处理实际启用的 C04/C05 写入、F2a 终态/取消对账和工单配置的稳定目标。配置与通知按实际启用路径分批验，前端与 ActiveList 按原接收工作包补体验及浏览器场景。估算和启用由原总账及批准入口维护。
