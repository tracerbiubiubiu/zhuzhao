# 核心交互架构复核：调用、可靠交接、审计和内网扩展

> **2026-11-15政策后继，不改本报告时点事实**：[第一包/F017](../../phase6/iam-first-batch-design.md)已确认父停用整子树组织派生资格暂停及SetRoles/AssignMenus本地审计失败回滚。当前产品缺陷仍开放；原净估作历史候选，IAM子估待扣重，不表示已开发或已验收。

日期：2026-11-15。范围：main主仓`aa745173`、TR`b0906b83`、AL`15e93682`、UI`69fbad99`、utils`de3cd0a6`的现状；设计基线为`db483759`。本轮只审计/修设计，**未实现共同调用runtime、可靠事件源、私有插件宿主或新消息渠道**。

结论：方向保留，先把已有C03/R01/C04及首版选定互通链做实。**统一的是业务执行路径和合同；不是所有函数走HTTP、所有操作进队列，或用一个万能callback处理全部消息。首版复用现有TR＋Redis/Asynq，不增加第二消息中间件。** 大部分可靠性要求已写进计划，本轮补实际反例、四类设计接缝和权威文档冲突，不能说“之前没规划”，也不能说“规划了便已可用”。

## 1. 从入口到效果的唯一责任链

页面/API、工单动作、手动/定时任务、源事件、私有应用和后继MCP，按各自可信身份进入同一登记的领域应用用例。同主体/目标/输入应有相同业务校验；入口不同可有不同当前权限，不共享管理员身份。

| 位置 | 应由谁负责 | 当前状态 / 首版出口 |
| --- | --- | --- |
| 认证及参数adapter | 协议解析、可信Context、精确路由/schema | 已有JWT/HMAC/handler，但无全量共同门面；CE01/02随触及能力迁移 |
| 同步应用用例 | 当前L1/L2/L3、领域规则、事务/锁/CAS、稳定错误 | 现有service保留；C04守卫/DBTX试点，网络不持业务锁 |
| 源提交与可靠交接 | 数据属主同事务保存事实/intent，逐消费者delivery | 当前Hook可丢，R01尚未交；选定不可丢消费者启用前补 |
| 调度/执行 | TR唯一业务调度/投递/技术claim；领域写主复验当前业务资格及效果fence | 已有队列/cron/回调；C03仍有下节风险，不能直接承载新自动效果保证 |
| 效果和结果 | 写主保存receipt；票节点/页面只投影原结果 | 当前完整回贴尚缺；D12/CE02/所选AL效果同批交 |
| 消息与审计 | 消息逐recipient资格及送达证据；审计记录真实已提交事实 | 工单业务UIP仍桩，死信Webhook有尽力HTTP；均不等于可靠送达/receipt，MH/OBS与旧inbox待接线 |
| 内网私有能力 | 私有SDK/字段/秘密/存储独立，公共版本化HTTP/Port | X01/X02候选，尚非可配置外置插件产品；先验所选合同/旧消费者 |

明确执行请求、事实收件ACK、结果投影、渠道送达及授权查询五类“回调”，见[共同合同§2.4/2.5](../../design/capability-invocation-contract.md#24-回调统一约束不统一成万能入口)。复用认证、限额、关联和错误纪律，各自仍有独立schema/写主/状态机。当前`/internal/jobs/callback`是**请求核心执行action**，不是远端业务已完成的通知；不因名称callback就拿它直接回贴结果。

## 2. 实现已确认、原计划仍待修的风险

### CORE-01 · C03并未因已有队列而完成

定向探针确认六组现状，均在C03既有目标中，无须新增一个队列项目：

1. TR `internal/service/submit.go`的`Service.Submit`：同task_id、不同action/params仍返回accepted=false且err=nil，不能证明输入一致。
2. TR `internal/service/cron.go`的`Loop.FireDue`：两个loop在同due job提交点对齐，可各生成随机task_id；推进next_run不是原子occurrence。
3. TR `internal/worker/worker.go`的`Deps.handleCallback`：MarkRunning的DB失败仍调用下游，探针effect次数1且worker返回nil。
4. 同worker：effect发生后Finish失败仍返回nil，重开专属SQLite仍running；worker成功返回不等于记账已收敛。
5. TR `internal/callback/client.go`的`Client.Do`：202/running、200非JSON均成功，429成为不可重试；严格模式必须按协商完成合同解析。
6. core `internal/service/jobs_callback.go`的`paramsSemanticEqual`：两个相邻超安全整数被float解码成相等；所选profile须拒绝超安全整数数值，确需大整数时用合同声明的字符串类型，不能将保真解析误读成扩大本批数值范围。

六个top-level探针各8轮确认，使用临时SQLite、httptest、Go overlay和纯JSON函数，未连共享数据库；**PASS表示复现当前行为，不是目标安全回归通过**。主仓当前running重复/MarkSucceeded失败仍200的源码路径也已被[C03](../../design/cross-repo-execution-contract.md)明确登记，本轮不冒称重新动态证明所有core故障。

最小修复仍是C03原切片：提交先有稳定快照、同号异参拒绝；claim失败零效果；执行与记账/unknown分开；只有持久receipt才完成；cron稳定时隙与有界补偿。新业务先选具备事务、幂等或可核结果的动作；下游无查询/去重时不能靠自动重试变可靠。AUTH-02维护动作资格与AUTH-07赋角问题继续按[权限专项](authorization-design-implementation-review-2026-11-15.md)开放，不在此重复算新发现。

### CORE-02 · Hook、通知与审计不是同一种可靠性

`internal/pkg/domainevent/registry.go`的HookRegistry是进程内goroutine/recover/drain，commit后进程退出仍可漏消费。`ticket_events`有事务事实，不代表已实现可靠扫描/outbox。现有消费者用法可保留，但不可丢后继须转R01，不能Hook和durable目录双跑同一业务。

`internal/service/ticket/inbox_service.go`的`InboxService.Inbox`及`internal/repository/notification_inbox_repo.go`的`Inbox`只按recipient过滤，count/list可返回旧标题快照；内存PG协议夹具驱动真实service/repo确认，**未跑真实撤权链，也不把个人历史收件本身笼统报为新IDOR**。新受限业务启用前按已有目标接当前对象/字段投影，含标题/total/角标；保留原收件事实，不抹掉历史以伪装未发送。

`internal/service/notification_service.go`的`NotifyDeadLetter`对独立url属性mask，却原样记录err；`internal/pkg/webhook/client.go`的`Client.Post`返`url.Error`。无网络RoundTripper的真实服务探针显示合成query token从error重新进入日志。登记OBS01具名修复；**未证明真实公司凭据曾泄漏**。`UIPStubSender`仍未真实发消息，不能把现有日志sent或stub nil视为公司接口已打通。

## 3. 本轮补清或纠正的设计

1. **通知身份冲突**：消息唯一键原缺来源命名空间和rule_id，领域合同也仅写event_id。改为可信`EventRef=(source,event_id)`，delivery键另含rule_id/version、recipient/channel/semantic_key。两来源同id、两规则同version不互吞，同事实同规则重投不重发；这是未实施候选的修正，不是产品数据库已迁移。
2. **跨库撤权承诺**：X01逐请求回查是正确选择，但不能把30秒ref寿命说成授权缓存，也不能保证核心撤权与私有库原子。明确准入前/准入后/分页之间的撤权、单次有界读deadline及下一重验点停止；不持核心锁等HTTP，不为此引入分布式锁。
3. **权威公约误读**：standards§2旧“±5min防重放/X-Operator不可伪造”改为真实边界：无nonce、query未签，时间窗内仍可重放；签名仅防服务声明被途中改写，不证明用户资格。wire/算法未改，业务重放靠稳定operation/receipt，代人身份走受限Context/ref。
4. **整包波次与首版出口**：共同合同的P7链B/C不能遮掉首版本地已前移的AL写→持久事实→只读TR后继。旧adapter×新core的兼容门则明确所选公共合同（该profile已启用的读/写/receipt、权限/错误及适用幂等）与消息接口，不削掉原所选写回归，也不为回归另开新写能力；不能免费塞完整X01-B导航宿主/X02通用平台。S1/MH01/RELEASE-Q1/Q2分别对齐合同、参考fixture、旧消费者回归和外置配置/回退，实施前扣重补子估；本轮不改68–104日数字，也不认定全部新细节零成本。

其它关键设计已对照现有合同检查边界：数据/事务写主、参数/协议/定义版本、操作/事件/投递身份、错误/重试/取消未知、当前权限、字段/附件引用、订阅因果与防循环、退避/流控/积压、秘密/目的地、配置启停、数据迁移/备份及在途恢复；没有逐项新跑所有动态故障。未发现需要重做成微服务/通用DAG/新授权引擎的证据；准确schema和数值仍随真实启用profile冻结，不承诺穷尽未来场景。

## 4. 首版最小闭环和后继

先C04/C05当前权限与安全小修、C03结构化完成/R01持久交接，再选定同用例的HTTP/内部入口；风险→工单、TR/AL真实效果/源后继、消息及薄UI随批交。每个读写主各管自己的DB/迁移/receipt；源事实保存失败使本批不可丢源操作失败，外部通知投递失败不回滚已提交业务。

普通本地CRUD不强制入队；进程内纯工具不公开API。长任务、外部副作用和不可丢独立后继异步化；同库本需全有全无的组合仍同事务。审计拆本地受控事实写入与可选外送adapter：获准强审计不能挂可丢Hook，外送重试不重执行业务，不能开放任填actor/outcome的日志写后门。两IAM强审计失败政策仍按原待决项，不代拍。

首期一个实际消息HTTP profile＋所选公开合同的固定旧参考消费者即有价值。私有配置可经受控路径或只读部署挂载放checkout外；当前main固定相对config入口不等于已经支持任意扩展注册。真实厂商字段/SDK留adapter，不导入core internal、不直库；真实UAA后继接管来源，本地IAM保留。设备/AI、完整插件安装治理、复杂订阅/多维预算/长回放、多副本/微服务按原窗口，不堵本地基本版。

只有实际多消费者回放、实测积压/吞吐或企业事件平台接入需求出现，才评审broker适配；不是一出现异步就装Kafka。[Asynq](https://github.com/hibiken/asynq)承担至少一次任务执行，[事务outbox](https://docs.aws.amazon.com/prescriptive-guidance/latest/cloud-design-patterns/transactional-outbox.html)解决源双写断点，消费者仍需幂等；[CloudEvents](https://github.com/cloudevents/spec/blob/v1.0.2/cloudevents/spec.md)支持source/id事实身份，非新增基础设施决定。[RFC9110](https://www.rfc-editor.org/rfc/rfc9110#section-9.2.2)区分方法幂等与失败重试，不能从HTTP成功码推导业务完成；[OTel日志模型](https://opentelemetry.io/docs/specs/otel/logs/data-model/)也不让trace替代业务receipt。

## 5. 验证与交付边界

原始证据在canonical main ignored `.codex/audits/core-architecture-20261115/invocation-probe/`与`event-probe/`，有源码冻结值、overlay、日志、重跑命令和边界；SQLite/HTTP/内存PG夹具均已关闭，无共享数据写入。设计worktree `.codex/audits/core-architecture-20261115/`保存独占栈完整基线和文档验证，不能将当前行为确认probe混成修复后的绿测试。

本轮没有新增业务API、消息broker、数据库迁移、权限码或私有代码；核心接线仍planned。规划状态/控制台回填开放项，原总价/排程不从“审计完成”推断已实现。真实消息/UAA/提供方契约和阈值在对应启用前核验；无新增必须现在拍板的中间件/插件大选型。

独占新栈完整基线通过lint（含Swagger）、race单元/集成、cover、migrate-up和acceptance：253通过/0失败/0跳过，81对迁移往返零漂移，默认标签覆盖率20.2%。这些运行产品源码与main相同，故证明未修产品的普通回归正常，**不关闭上面的可靠性/泄密反例**；后续先将现状确认改成目标行为测试取得有效红，再实施修绿。本轮没有重跑UI全量E2E/远端CI或真实UAA/公司渠道。

三分工只读核验、两个新上下文交叉复审已完成，修正TR与领域资格归属、工单UIP桩与死信HTTP的区分、整数范围和私有兼容所选写门。临时9容器/项目卷/网络已按owner精确回收，16个原共享容器ID/镜像/状态/启动时间/重启数/挂载/端口指纹不变；canonical五仓main未改，设计只作本地提交，未推送/合main或部署。

规划闭合19项/排程31项测试与DAG检查通过，12份改动文档及机器状态共323处本地链接/锚点有效，git diff检查通过。76父包/17子里程碑/13切片与原价格源不变；原权限开放项不被新报告覆盖成关闭。额外兼容子估及真实启用参数仍须在开工/启用窗口核验。
