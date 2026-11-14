# 统一日志、业务审计与追踪的有界演进

日期：2026-11-13。用途：承接所有者新增的统一日志/审计要求，供五仓及私有扩展的后续设计、计价和验收使用。状态：**源码定向审计和有限设计；业务历史与专门审计职责分开授权已确认，高风险写的审计失败政策待答，新增运行机制尚未实现**。本页不修改现有日志、鉴权、保留期或运行配置；工作项、窗口和预算须由集成者进入唯一总账后生效。

2026-11-14按[基础先行边界](foundation-release-boundaries.md)收窄首批出口：OBS01处理适用敏感面、身份和关联；OBS02基础纵切先本地可信事实、受权分页查询和现有页面。下文的异步大导出/快照artifact作为增强，在P7出口按真实用途选定后独立交付，不挡基础版；OBS03外送、跨仓追踪和多worker能力按实际链路/拓扑启用。原22–33日是所列完整有界范围候选，不是B0/B1必需价，所选基础净价在B1开工前完成，增强在对应包开工前拆估，旧价不重复收取。

## 1. 目标和首期范围

平台需要能回答“谁、以什么身份、在哪个范围、对什么对象、请求了什么动作、产生什么效果、失败在哪里、证据在哪里”。诊断日志、可归责的业务审计和可靠领域事实分别保留职责，共享关联及脱敏合同。统一不等于写同一张表、收集所有原文或新建SIEM/SOC。

默认仍可独立运行：已有`slog` JSON、主仓PG审计、业务数据库和持久卷提供本地出口；没有OTel Collector、企业日志平台、UAA或模型时，核心业务和本地审计不依赖它们启动。如内网已有日志/监控/安全平台，复用其跨系统检索、长期索引和告警研判能力，实际供给/协议另核；本平台提供受控输出、必要的本地查询及受权跳转，不复制其产品。

首期有界代表集：

- 三Go服务的访问/错误日志和公共logger；UI保留服务端审计读取、请求失败关联及本地错误处理，不能把浏览器console当业务审计。
- 两个新增强审计试点：用户角色授予/撤销、角色菜单/API权限变更。复用现有事务和权限判断，不顺带重写全部IAM。
- 工单流转、TR执行、AL一次受权行修改作为关联对照；其领域写入、可靠意图和效果回执归原工作包，本页不另计一次。
- UAA、私有adapter、Agent/MCP首期交合成合同及关联反例；真实提供方接线在其原窗口逐个验，不把一个公共合同包办所有供应商。

全量存量审计迁移、任意历史日志检索、全站前端埋点、全量trace瀑布图、日志分析DSL、专业威胁检测不属于本次有限范围。新增或触及的能力逐步服从共同合同，旧入口按清单迁移。

## 2. 已有实现及本次发现

这是对主要生成入口、存储、关联、读取和清理路径的**定向源码审计**，不是对每条日志、真实采集环境或全部多实例故障的运行证明。以下符号均在当前工作树核读，未跑新产品测试。

| 范围 | 当前实物与证据 | 准确边界/缺口 |
| --- | --- | --- |
| 公共运行日志 | `zhuzhao-utils/logger/logger.go`的`New`：slog JSON、文件及stdout、lumberjack轮转，支持level/大小/份数/天数 | 无统一敏感字段投影、service/instance默认属性或业务审计存储；`io.MultiWriter`先写文件，写失败可能使后续stdout也未写，slog调用方通常未处理写入错误。不能当作可靠审计 |
| 主仓访问日志 | `internal/middleware/logger.go`的`RequestID/AccessLogger`：校验`req-`格式、request context载体、method/path/query/status/operator/auth/caller/request_id等 | RawQuery只截断未脱敏；request_id不是业务幂等身份，也不是已经生成的OTel trace。入口允许的外部关联号不代表可信业务身份 |
| 主仓HTTP审计 | `internal/middleware/audit.go`的`AuditLog/maskSensitive/responseCaptureWriter`；`AuditService.Insert/LogLogin/LogRefreshReplay`；`AuditLogRepo.Create` | 请求结束并Flush后以独立3秒context写PG，失败记Error，不能回滚已完成业务。递归key脱敏、2KiB审计体和512B响应摘要已有；不能声称全部敏感数据已覆盖。读取整个请求体后才截断入库，非有限内存读取；反代前缀跳过body，跳过不等于该上游业务已被完整审计 |
| 主仓审计查询 | `AuditHandler.ListLogs`、`AuditService.List`、`AuditLogRepo.List/buildAuditListWhere`；`router.New`的`/audit/logs` | 已有分页/日期/路径/用户/方法筛选和身份快照，路由归`audit:read`页面族。当前查询不是按每条业务对象实时L2/L3裁剪；不能自然交给某类型业务管理员当作其全部工单查看入口。没有因本页新增受权导出API |
| 现有运维/发送记录 | `PanicRepo.Upsert/List`按fingerprint聚合panic；`middleware.MetricsMiddleware/RenderMetrics`及`router.New`的根级`/metrics`；`MessageLogRepo.Insert/List`、`MessageAdminService.ListLogs` | panic含message/stack，发送记录含recipients/subject/body/error，需分别按用途投影，不能把其原文自动复制到统一日志。指标及panic聚合已有，不重复建设；根级metrics当前不在业务鉴权组，应由显式网络/采集范围保护，不能当公开业务接口。发送记录归D11应发/实发事实，不自动升级为事务审计/outbox |
| 判定日志 | `internal/pkg/resource/registry.go`的判定hook，`internal/pkg/audit/policyeval.go`的`Write/pump/flushOnce`，`AuditLogRepo.InsertPolicyEvals` | 现为channel→Redis→PG，队列满/Redis失败允许丢弃；PG失败留processing重试，插入成功后裁剪失败可重复。既有fail-open不等于业务强审计承诺。共享processing的LRANGE→INSERT→LTRIM未见跨实例claim/fence：并发批次存在重复及错误裁剪风险，属静态发现，尚未故障实测 |
| 工单事实 | `TicketRepo.CreateEventTx`、`ticket.Service`的Create/Update等事务、`transition_service.go`流转；`TicketRepo.Delete`的删除事件与删除事务 | 必要事件可与业务同事务写`ticket_events`，有request_id。事件字段及键级changes不等于统一前后值/字段可见性审计；日志中间件不覆盖未来非HTTP调用，须随共同用例迁移 |
| 进程内事件 | `internal/pkg/domainevent/event.go`的`DomainEvent`，`HookRegistry.Dispatch/Drain` | 每订阅者goroutine、recover和停机drain已有，进程崩溃会丢，非outbox。事件载荷不是审计正本/可靠投递或命令授权；不能靠采集Hook输出补齐事务事实 |
| 主仓归档 | `AuditArchiveJob.Handle/archiveTable/cleanExpiredArchiveFiles`、`AuditLogRepo.ArchiveFetchBatch/ArchiveDeleteBatch`；`app.NewApp`装配归档cron | 默认热库180日、文件395日；先JSONL写入及Sync，再按同批ID删除，失败可重导；文件本地、当前创建mode0644。进程cron有既有锁且与TR触发应二选一；这不是跨主机耐久归档、远端验收回执或防篡改证据 |
| AL | `activelist/internal/middleware/middleware.go`的`RequestID/AccessLog/snapshotParams`，`internal/app/providers.go`的`provideLogger` | 与主仓同request_id/caller/operator跨查，但入站ID只判空；RawQuery及body前4KiB原样进入运行日志，未见按schema sensitive/secret分类裁剪。AK/SK是服务身份，不是用户级审计权；没有独立结构化审计查询/导出或通用源outbox |
| TR | `taskrunner/internal/middleware/middleware.go`的`RequestID/AccessLog`、`internal/repository/store.go`的`Run/InsertPending/MarkRunning/Finish/ListRuns`、`callback.Client.Do` | job_runs有task/request/job/action/attempt/error/params等，DB-first/reclaim有实物，但不是全部尝试/业务效果的统一审计。RawQuery未脱敏、ID只判空；operator在无验签上下文时可回退请求头。回调错误会把对端片段写入error并供读面回显，截断不等于脱敏 |
| UI | `zhuzhao-ui/src/main.ts`的`errorHandler`；`src/views/audit/log/index.vue`；任务列表及情报查询页 | 当前console兜底、审计分页/报文查看与复制、panic/reconcile和任务运行查询已有。没有因此形成可信浏览器审计或全链路OTel。情报localStorage历史是本机记录，不能作服务端取证正本 |

补充校准：

1. 主仓脱敏key已含password、old/new_password、secret、token及`*_token`，并可遍历嵌套数组；不能把已修项重复当作全缺。其覆盖不能推定包含任意凭据别名、连接串、签名URL、自由文本、字段值及错误片段。
2. 主仓/AL的go.mod已间接出现OTel相关依赖；本次未见本项目生产入口建立通用span/traceparent/exporter接线。间接包存在不是已接入分布式追踪的证据，也不能宣称完全没有OTel依赖。
3. `reqid.From`、网关签名转发、主仓taskrunner client、TR callback已经保留部分关联，不能全部重做。`DomainEvent`自身没有稳定event_id/operation_id等完整公共元数据；关联号有覆盖空隙，不代表操作已统一。
4. TR当前运行文件logger默认90日轮转，而较新平台合同明确job_runs/执行日志永久保留。不能仅保留job_runs摘要便宣称永久执行日志已经满足；需要明确哪些内容属于必须永久保留的执行证据并可靠保存。既有执行历史承诺不在本页撤销。

## 3. 三类记录分工和失败政策

| 类别 | 权威内容 | 写入/失败政策候选 | 读取与清理 |
| --- | --- | --- | --- |
| operational log | 请求、错误、容量、依赖和进程诊断；可采样的性能信息 | stdout/文件；采集失败不把已提交业务改成失败。缓冲、丢弃、写盘失败有计数/告警和有限stderr兜底，不递归记录日志失败 | 运维范围；按诊断类别与容量保留，不能删除承担永久执行证据职责的内容 |
| business/security audit | 可归责的角色授权、配置、业务变更、批准及结果；记录请求、拒绝与已提交事实时用不同event/outcome | 选定高风险本库写的**成功事实与业务同事务**；本地审计插入失败则该事务失败。未提交拒绝/鉴权诊断另记，外部collector不可用不阻塞已落本地的事实。不得事后日志补写伪造原子性 | 专门audit资格/范围与字段投影；明确导出/访问留痕、受控保留与耐久归档 |
| domain fact / outbox | 属主已提交事实、待执行意图及投递/回执，驱动后继用例 | 原领域事务/outbox/幂等消费者；采样不可丢必要事实。Hook继续标best_effort，不能冒称durable | 原领域授权与恢复规则；outbox临时副本可按ACK回收，不等于删除审计或执行历史 |

这是对**新增强审计试点**的推荐，未把所有审计写失败改为全平台停机，也未更改已确认的判定日志fail-open。是否将更多业务操作列为强制审计，须给具体操作及可用性影响再确认。普通GET/健康探针不因一条诊断日志失败而拒绝业务；鉴权本身仍按原策略fail-closed，不能因“审计fail-open”变成放权。

首期强审计试点使用`UserService.SetRoles`（已有`UserRepo.SetRolesTx`及最后superadmin保护）和`RBACService.AssignMenus`（已有`RoleRepo.AssignMenus`事务及随后策略重载）。应让服务拥有业务+审计事务，保留原保护与失败语义；记录“DB策略已提交、运行时重载待恢复”须与“授权已全实例生效”分开，不从HTTP 200推断后者。

工单同事务事件、D02流转里程碑、R01可靠意图、CE02效果凭证继续由原包实现。审计适配消费真实提交结果和原event/operation身份，不再执行一次业务，也不创建第二个业务重试主。

## 4. 最小共同字段与可信来源

先采用版本化JSON记录/字段词典及窄接口，不先建通用日志SDK或万能`/log`接口。公共logger只收纯机制，不能下沉业务表/权限到utils。以下为**首期冻结候选**，准确DDL/DTO、枚举和兼容adapter在OBS01出口冻结；当前旧API不因本页自动改wire。

共同必要字段：`schema_version`、`record_id`、`record_kind`、`event_name`、`occurred_at`、`observed_at`、`service.name/version/instance_id/environment`、`outcome`及适用的关联/主体/资源。时间统一RFC3339Nano/UTC语义，保留来源时间质量；未知字段为空，不伪造已知时刻。

- `record_id`是本条记录身份；业务事实稳定`event_id + producer/source`、`operation_id`、`invocation_id`、`request_id`、TR task/execution/occurrence分别复用[共同能力合同](capability-invocation-contract.md)语义。投递重试保留原事实和operation身份，新的网络attempt有自己的request/span/delivery身份。不能用一次请求号去重整项业务，也不把新生成日志ID当原事件ID。
- `trace_id/span_id/trace_flags`仅在实际tracer存在时按OTel/W3C语义记录。现`req-...`保留在request_id；旧policy日志的trace_id若实际装的是request_id，应通过legacy映射说明，不剥前缀伪装为真实trace。采样率不是审计丢弃率。
- `actor`区分真实用户、服务主体、代人委托和实际执行方；引用本地稳定principal ID、当前认证来源/issuer、service ID、必要grant/批准引用。不默认把工号、姓名、邮箱放入所有运行日志。操作者、业务owner、created_by及collector不是同一身份。
- `resource`只放受控类型/ID/版本/组织范围引用，`capability_id/contract_version`来自登记目录，`action/outcome/reason_code`使用允许词表。before/after只含该审计类型允许的字段，含敏感值时保存受权证据引用/摘要而非秘密。
- `causation_id/parent_invocation_id/root_invocation_id`引用原共同运行合同；Agent run/skill/model配置版本、UAA batch/revision、adapter来源与回执引用按对应合同选用，不能把完整prompt、知识正文、token或公司报文塞进关联字段。

| 来源 | 可采信的构造位置 | 不可用来授信 |
| --- | --- | --- |
| 用户HTTP | 认证后服务端principal、已核当前scope；请求中间件生成/校验request_id | 客户端自报user_id、任意X-Operator、任意审计event_name |
| 服务/私有adapter | 验证后的caller、明确服务授权、来源合同和受管映射；operator仅归因 | AK/SK通过即代任意用户，adapter整份权限或未经验证的租户/组织 |
| UAA | 公共port经已核验来源/issuer/batch映射所得，纳管与本地来源分列 | UAA token/真实私有字段入日志，认证成功等于任意业务授权 |
| worker/cron/事件 | 持久记录中受控principal/grant、operation和发生次；执行时重新核授权，source event映射 | 把排队时快照当永久许可，重建随意换ID，用同一trace表达多次效果 |
| Agent/MCP | 实际host/tool用例构造的主体、批准/plan引用、工具/skill版本及效果回执 | 模型自行声明“已获准/已成功”、allowed-tools代替权限、泄露知识全文 |
| UI | 服务端返回的request/operation引用；浏览器诊断带来源不可信标记 | 把console/客户端事件作为权限变更或业务完成正本 |

跨服务trace上下文只用于关联，不能替代认证/授权。只接受符合格式和长度预算的traceparent，外部不可信baggage默认不透传个人信息/权限；源头、传播与本地生成策略在边界测试中明确。`service.instance_id`按实际实例区分，不让多副本共写同一个本地app.log；容器优先stdout由现有采集器收集，单机文件模式显式目录/实例隔离。

## 5. 脱敏、访问与导出

首期先修真实敏感面：三服务query采用允许键/字段策略；AL不再把任意body/import原文放运行日志；回调错误仅保留安全错误类别/对端状态/有限字段摘要。请求/响应、错误、URL、表单字段、附件引用、任务参数、事件和导出分别处理；2KiB/4KiB截断不是脱敏。

OBS01同时覆盖主仓`AuditLog`的`io.ReadAll`有限内存及原body重放语义：在适用入口建立有界读取/流式限制，保留合法请求交给handler的内容；上传、导入和反代按端点合同处理，不把日志摘要预算冒充文件上传额度。用超大body、读取失败和后续handler仍能正常读取的反例验收，不只截断最终入库字符串。

至少测试password/token/secret及大小写、嵌套数组、access_key/secret_key/authorization/cookie/DSN/签名URL、非JSON、自由文本、超长值与CRLF。禁止记录完整Authorization、refresh/access token、签名、私钥、设备密码或模型/知识秘密。对错误对象不作“任意Error字符串天然安全”的假设。业务受管字段先按schema与审计用途投影，不能只靠通用key黑名单。

读取分两个用途，**所有者已于后继F007确认分开授权**；实际审计人员/作用范围及保留政策在部署前绑定，不混一个“管理员”：

1. **对象业务历史**：先验当前对象/类型/组织/字段资格再查、计数和展示，B01/B03继续有效。普通类型管理员或org owner不因此获得所有受限票的审计正文/统计。
2. **平台安全审计**：需要明确独立审计职责、作用范围、可见字段与使用目的；既有系统级管理例外保留，但不默许将旧`audit:read`广泛授权后带出全部业务body。跨业务范围取证的明确权限应由所有者按具体场景确认，不能靠日志绕过业务限制。

兼容旧查询时，默认元数据投影，原始body/stack/敏感详情按额外资格开放；归档文件也采用相同权限，不暴露任意服务器路径。API/列表/搜索/total/下载/Agent使用同一个后端范围谓词，不在前端裁剪。审计查询/导出本身留独立的元数据访问记录，避免递归把上一页审计正文再抄进新审计。

新增查询候选按time window、actor、resource、event、outcome、request/operation/trace精确筛选，保持确定性分页；首期不开放任意SQL/全文payload检索。既有page_size≤100保留；大导出走受控异步作业、固定快照/条件、字段投影、逐页预算和内容校验，生成artifact/manifest后下载仍验当前读权及有效期。初始查询窗口/导出条数/并发等参数在OBS02以代表数据实测冻结，不伪装成已批准SLA。导出写端点按B案绑定按钮，候选权限名不是已存在权限码。

## 6. 审计可靠性、并发和存储预算

本地强审计最低合同：同事务业务成功事实与audit row共享commit；失败不留假成功。事实ID/版本/允许diff/actor由服务构造；当前授权锁序与C04一致。提交响应丢失/commit结果未知按operation/领域receipt查询，不重复执行业务以“补审计”；幂等重放可记录读取尝试，但原成功事实不换号。

外送现有采集/审计平台时，本地durable row或原可靠outbox是来源，ACK只推进该副本水位。外送重试按`producer + event_id`去重，原始发生时间与观察时间分开；不能先推进checkpoint再保存数据，不能通过collector的2xx证明业务效果。I03/R01/CE已有传递主不变，OBS只增加审计投影和受控输出。

判定日志当前shared processing的多实例问题应在正式启用多worker前处理：候选为数据库/Redis上的明确单消费者lease+fence，或原子claim批次+事件去重；以两实例屏障测试证明A/B批次不会被另一实例LTRIM误删。重启、PG成功后Redis裁剪失败允许重复时，记录实际语义和去重策略。毒丸不能静默消失，记录隔离元数据/计数/可审阅处置，不把损坏原文放普通日志。

运行日志、诊断判定、强审计和必要领域事件分预算：高频成功GET可按已批准profile采样，拒绝/高风险变更和强审计不得因同一日志采样器静默丢失。设置记录字节/字段数、buffer、batch、重试/磁盘/导出inflight预算；超限用明确drop计数或held/backpressure，避免审计故障递归打满盘。具体上限由测量冻结，不一次引入新消息中间件或无限队列。

本地归档现状需补manifest、hash、来源范围、行数和可恢复性验证；未取得耐久目标的可核持久回执前不能销热库。多主机不能以“某副本fsync了本地文件”代替共享可恢复归档。首期可以单副本＋持久卷/备份，真实多副本切换前验选定拓扑；采集器、归档以及业务事务各自一个重试/清理主。现有归档日期与fileRetention配置保留历史事实，不自动沿用作所有新类别政策。

保留类别必须单列：

- **执行历史/执行日志**：按已确认永久保留约定承接，可分热/冷但永久可取与授权语义不变。不能恢复旧M4自动删job_runs；不能将app.log的90日轮转当永久执行日志已满足。永久执行证据需要明确来源、完整性、引用及恢复，普通诊断与临时副本另列。
- **平台审计/鉴权判定**：当前180日热库/395日文件是已有实现，不是本次所有者批准的新统一政策。上线前按实际用途/企业要求确认类别和归档目标，不能默改成永久，也不能把执行永久承诺降为这些期限。
- **outbox/队列/cache/导出副本**：可按已确认ACK、幂等窗口和下载期回收，原审计/执行证据不随副本删除；证据legal hold/保留变更须有明确权限和记录。

现有项目测试栈/卷可重建的F005授权继续有效。这里的永久执行证据约定用于正式运行能力，不把合成测试数据变成永不清理；测试重建授权也不扩大为删除真实运行证据的政策。

## 7. 最小UI和运维交付

OBS02复用现有审计页，新增必要筛选及结构化详情：事件/结果、真实主体与执行方、对象及允许diff、request/operation/trace/源引用、脱敏/不可用说明。业务对象页只给获准历史链接，运维日志入口可跳既有专业平台；未配置外部平台时仍能查本地审计元数据/按runbook查自身日志。不在前端提供任意文件读取或跨全部业务的超级搜索。

同时交：权限矩阵及反例、logging profile和来源版本、stdout/文件/可选collector接线、采集故障及空间耗尽处置、审计写失败/队列丢弃或积压/归档失败指标、导出/下载记录、备份恢复验证和变更回滚。logger失败的stderr兜底应限频且只含安全错误类别，避免自身递归；状态显示区分“采集暂不可用”“本地审计已提交”“业务未提交/unknown”。

不要求先部署某观测后端、Jaeger/Loki/Elastic或Kubernetes才交本地能力；也不以standalone模式为由删除企业外送接缝。端到端关联验证可用合成collector和失败注入，真实内网认证/网络/容量另列提供方窗口。

## 8. 分期、净估和原包扣重

以下是本次**新增净增建议**，包含对应最小UI/运维和验证，不是已批准实施或已进入当前总价；一有效研发席位工程日，不计真实提供方排队。原估算不得宣称免费包含本页新增范围。集成者须独立复核并落总账，窗口不在本页另立第二份排程。

| 子项 | 窗口与有限出口 | WBS净增 | 建议工日 |
| --- | --- | --- | --- |
| OBS01共同合同与敏感日志边界 | P6.0；三Go入口、公共logger及UI关联字段说明；保留原wire、独立运行 | 日志/审计特有分类及既有公共字段映射1；三服务query/body/error脱敏及ID/归因边界2–3；logger失败/预算反例与运维profile1–2 | **4–6** |
| OBS02本地统一审计最小纵切 | P6.1；两IAM写试点、受限本地读/导出和薄UI，工单只消费原事实 | audit row/迁移/投影2–3；两条IAM命令审计语义/事务接入点3–4；查询/导出/当前权限与薄UI3–5；故障/恢复/runbook2–3 | **10–15** |
| OBS03跨仓可靠接入与选定外送 | P7；一条core→TR→callback链、AL一个受权写、合成UAA/Agent/私有adapter引用；一个可选外送profile；多worker判定/选定归档恢复 | 共同log输出与外送边界2–3；已有事实到审计的跨仓投影2–3；判定管道并发/归档manifest与恢复2–3；独立故障/去重/重启验收2–3 | **8–12** |

净增建议合计**22–33工程日**。这是选定有限纵切的可审阅粗估，假设现有PG/Redis、对象/文件持久化和专业采集接入可复用，无需新采购或自建观测服务；真实归档/collector协议缺口、全域IAM/158能力历史迁移、所有厂商、完整永久日志冷存储产品均不能由此包干。若永久执行证据所需外部能力不具备，先用具名本地持久路径保留并列容量缺口，不允许静默删除；额外建设先补范围/子估。

这由[F006补充估算册](../review/post-baseline-scope-estimates.json)承接，**当前76父包基线DAG不因本页自动增加已批准节点或改价**。F004五组方向已确认，实际配置与新增净估尚未全完成，不能把公共394–656日简单加22–33就称为更新后的完整总价。统一净估、补充册与正式DAG按启用组合整合：**B1所选基础增量在B1开工前**给净价/依赖/出口；大导出、外送或多实例等增强在对应包开工前拆估，记录具名窗口，不强迫基础版开工前细化所有远期场景。P6.0/6.1/P7的OBS出口是拟定实施窗口，未改变当前“不急开发”的授权边界。

逐项扣重卡：

| 新净价 | 本项只计的新增工作 | 明确排除/复用的旧价 |
| --- | --- | --- |
| OBS01 4–6 | 日志/审计类别profile、三服务具体敏感面修正、logger写失败可见性和专用反例 | CE01已有能力/消息/身份词典不重设计收费；G04的一般测试框架/覆盖率与G06一般formatter/bundle不重复；I03的执行span/指标不移入本项 |
| OBS02 10–15 | 独立审计事实投影/持久row；`UserService.SetRoles`的角色授予/撤销和`RBACService.AssignMenus`的角色菜单/API授权这两条命令的安全diff/真实主体/提交与重载状态、具体事务接入点；现有审计页上的有限读权/导出/详情 | C04已有DBTX/WithinTx、授权锁序及事务审计公共机制复用，不另造或重复计框架；现有`SetRolesTx`/`RoleRepo.AssignMenus`业务事务及superadmin保护保留；CE02共同调用记录/幂等/receipt不重建；D02工单事实、R01 outbox不另收一次；U02/U06全站视觉不计入本项，也不另造审计总控台 |
| OBS03 8–12 | 已有事实到统一审计的跨仓映射、一个log外送profile及其脱敏/故障接线、判定队列多消费者安全、选定归档manifest/恢复证据 | I03已价的执行trace/metrics、恢复/attempt状态和相关UI继续由原包交；R01可靠意图及CE02-D/CE03效果事务/源事实不重做；UAA P/R、I02主体/grant实现不重计；G05备份机制、E09完整拓扑/灾备接线与R03永久执行证据供给继续归旧包。本项仅增加其统一审计映射和选定归档验证，不包办这些产品 |

OBS02将强审计row插入现有业务事务，必要的窄接入改造属于该命令新增审计点；若C04已经交同样接入/投影或实现期发现某WBS内容已在旧包完成，必须从OBS实价扣除，不能因为写在本页就二次收费。所有真实提供方等待/额外协议适配单列，不伪装成工程日或由mock代验。

扣重与依赖：

- D02仍拥有工单转换原子字段/事件和F004所述里程碑；R01仍拥有业务intent/outbox/重试/对账；CE02仍拥有共同业务执行入口/效果receipt。OBS不重写这些机制，只规定/接入审计字段及两个新的IAM试点。
- I03已含执行生命周期/恢复及1–3日指标/告警/trace补量；TR span、队列/attempt状态和恢复入口继续在I03计。OBS03的外送profile、跨域审计投影、判定日志多消费者和归档证据是其未覆盖的新范围；不能把“统一日志”再次收一次同链trace开发价。
- I02/UAA P/R已有principal/grant、撤权/投影与专属审计要求；OBS复用其可信结果/receipt，不重估25–39公共实现或真实联调。没有真实UAA不阻断本地审计；真实来源激活才验其适用方向。
- D11发送应发实发、D12结果/日志引用、D13指标、R03专业runner及执行证据、G05机制、E09实际生产备份/拓扑沿原包。本页不包其业务功能、整套灾备或专业日志平台；OBS03只加统一审计映射/选定归档manifest和专属故障证据。
- OBS01无CE前置，可先修日志敏感面；OBS02共享C04事务纪律，但只改两个IAM试点，不等所有能力完成CE02-L。OBS03按真实链启用依R01及适用I02/I03/R06/CE03，原I03只需共同字段合同，不反依OBS03形成环。判定日志多worker保护是启用多实例前的必要条件，不强制独立首发先上HA。

## 9. 验收及尚需确认

实施继续standards测试先行：先冻结本包允许/拒绝/边界/权限预期，取得目标行为红例，再实现；并发修复按原要求移除修复后至少8次稳定红，不能用文档/Schema绿或本轮旧产品门禁代替新实现证明。本页未运行这些候选用例。

下列是所列完整有界范围的验收清单，随实际启用组合进入对应出口；基础版先验适用敏感面、当前授权和本地事实，跨仓/导出/多实例并非一齐前置。第2项强审计失败回滚仍是待所有者确认的推荐候选：

1. 三服务/反代/body/error/query中的秘密、嵌套及非JSON不落普通日志；允许的安全诊断仍可定位；超大日志、非法ID、未验签X-Operator和外部baggage不能伪造主体或放大内存。
2. 两IAM操作从HTTP与共同用例访问得到同一允许diff；业务写失败无成功审计，强审计写失败无业务提交；policy reload失败不伪装权限已全实例生效。拒绝尝试与已提交效果分别可解释。
3. 关联从请求经异步重试/重启回调仍能找到同operation/原事实，实际attempt不同；collector断网不丢已提交强审计、不重做效果，恢复后重复投递可去重；trace采样不删审计。
4. 两实例对同processing批次、写库后裁剪失败、毒丸、停机drain、磁盘满/文件权限/PG断连均有确定性反例；只有实际单副本受支持时明确限制，不虚报HA。
5. 普通业务管理员不能经审计列表、total、报文、复制、导出或Agent读到受限票；平台审计员按明确资格工作，撤权后新下载被拒。query与count同范围，导出快照不携带未允许字段。
6. 无collector/UAA/模型仍可独立登录、业务写入和本地审计查询；重建测试环境与迁移/恢复分别验。TR永久执行历史/所需日志证据不因采样、轮转、ACK或归档副本清理而消失，选定永久路径恢复后可取。

当前只待一项已提出的政策选择：用户角色/角色权限两类高风险修改，本地必需审计失败时是否同事务回滚；建议采用，外部collector断网不阻断已落本地证据的成功。分开授予业务历史和专门审计职责已确认，不重问；实际跨模块审计名单/范围、各非执行类别的保留/导出政策和真实采集/归档供给在对应部署窗口绑定。执行永久约定保持。供应方支持协议、字段类型、logger/OTel版本、lease/CAS及压测参数由实施方先核验并给可审结果，不让所有者盲选技术参数。

## 10. 官方依据与适用限界

- [OpenTelemetry Logs Data Model](https://opentelemetry.io/docs/specs/otel/logs/data-model/)定义Timestamp/ObservedTimestamp、Resource、Severity、TraceId/SpanId和Attributes等公共语义。支持映射共同元数据，不证明本项目已有tracing，也不要求将所有审计变成OTLP。
- [非OTLP日志的Trace Context](https://opentelemetry.io/docs/specs/otel/compatibility/logging_trace_context/)说明既有日志可保留结构化trace字段；现有request_id仍是独立关联字段，不能冒充规范trace。
- [OpenTelemetry敏感数据处理](https://opentelemetry.io/docs/security/handling-sensitive-data/)说明应控制收集内容及敏感属性；本项目仍须在数据源/权限层阻止秘密进入，collector处理不替代源端最小化。
- [OWASP Logging Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Logging_Cheat_Sheet.html)区分不同记录用途，强调who/what/when/where、interaction关联、排除秘密、读取权限、失败及容量测试。它不替本项目决定永久历史之外的具体保留天数，也不证明当前中间件是强事务审计。

以上用于约束具体设计，未作合规认证、日志平台选型或真实内网连通声明；当前阶段不新增外部观测服务依赖、业务代码、权限码或数据库迁移。
