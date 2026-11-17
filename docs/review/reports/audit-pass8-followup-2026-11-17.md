# 存量修复pass8复审补修与验证

> 用途：承接所有者“ok，按建议来修复”的本批回归、维护准入和证据/文档收口；范围与原[前移计划](../../planning/audit-repair-plan-2026-11-17.md)一致，完整IAM-C/C05、C03/F2b、HA及未取用能力保持后继。
> 当前状态：本批实现、完整适用门禁和真实运行专项已通过，源码及运行收据经交叉复核；功能修复已按普通PR合入main、分支/PR CI已通过；两个准确功能main CI均为success，未部署。修前主仓main为`6c3e33883805e359160dea0d8b8b36a7e4b1b552`、UI为`7c52da0a0881c81ff415e674eff8120eaaea6bec`，TR/AL/utils沿各自已合main准确来源；实现短分支为`codex/pass8-review-repair-20261117`。最终远端承接与清理收据另在本报告交付节登记。
> 本地材料ID：`maintenance/audit-pass8-followup-20261117/`。旧审计pass8和四批原封存保留各自时点，不重写修前结论、原失败或旧门禁数量。

## 1. 本批范围与复审纠偏

本批先修明确的新增回归，再落实已确认的固定系统cron合同、当前恢复/配置引导及真实正向证据。实现、准确候选验证、提交/main与生产启用分别登记；写了修复和测试通过均不代替真实环境准入。

| 复审项 | 本批处理 | 必须保持的边界 |
| --- | --- | --- |
| PLAT-R01 | 正常SIGTERM停接纳和后台生产者，已接纳HTTP独立context按30秒预算排空；失锁在排空期间仍立即取消；晚完成请求的判定日志消费者随后收尾 | 不把失锁当普通排空继续执行，不宣称网络分区零重叠或HA |
| PLAT-R02 | Load保留连接读取职责；非法维护名单由服务端Validate拒启，不阻断应急CLI消费无关配置 | 非法/重复名单清空且不能部分授予，服务启动不容错放行；不操作真实种子号 |
| TASK-R01 | 保留固定系统cron，由受审启用配置创建私有固定后台能力；公共归档Handler必须持有私有回调准入 | 固定后台不是人工maintainer或TR job伪装，不从外部actor授维护权，不新增在线批准平台 |
| SIB-R01 | 原有组合成功收据保留；新增维护提示和真实浏览器正向链，原无资格403反例保留 | 空名单是安全默认；浏览器在线marker删除与JSONL文件导出是不同证据 |
| IAM-R01 | 保持策略投影失败关闭；补当前可执行SOP及受控恢复专项 | 不把DB已提交误作回滚，不加自动开闸；完整C05/后台恢复产品仍后继 |
| IAM-R02 | 补组织派生平台角色与本地owner/admin的授权矩阵，原权限模型不变 | org_roles授平台角色；跨组织功能权不等同本地委托。不能仅凭跨组织可用重开AUTH-07 |
| TICKET-R01 | 补删附件后显式清可选引用/替换合法对象及required拒绝的恢复用例和取用说明 | 已删、未确认、他人新对象、错purpose及归属仍拒绝；不自动改写历史custom_data |
| SIB-R02 | 具名后继：冲突行已存在仍可能误报ledger missing | 仅诊断误报不等于执行结果丢失；依赖准确自动对账前优先处理，未关闭 |

现行[组织合同](../../modules/organization.md)与[委托职责](../../phase2/04-org-delegation.md#311-路由级权限实现策略p2-2b12c-开工前置已决策)保留平台角色语义；原漏洞是任意org按钮粗门，不是所有组织来源功能权必须局部化。正式API仍支持custom_data，不能凭缺UI将TICKET-R01称为未启用；optional清键、required替换、title-only和错误整笔回滚分别验证。

## 2. 实现及运维合同

### 正常排空与失锁

`internal/app/app.go`的`App.Run/Shutdown`分开信号/后台context与HTTP请求context。信号停止新接纳及cron，先排空既有请求；超时或单实例资格丢失时取消请求并强关连接。失锁监视贯穿Shutdown，不能因已进入正常排空就忽略随后失锁。判定日志随HTTP生产者排空后停止，pump完成后再做flusher有界收尾；失败残留与日志fail-open边界如实保留，不扩大为分布式claim或零丢失承诺。

真实专项使用独立PG、真实`App.Run`子进程和被行锁阻塞的在途事务。正常信号释放锁后须HTTP成功且数据库恰好提交一次；失锁分支仍拒接/取消，30秒超时另验。测试helper核对正常/occupied/lost返回，不以子进程退出0替代目标错误证据。

### 维护配置和三个可信来源

`internal/config/config.go`的`Load`记住非法维护名单的私有启动错误，返回连接配置；`Config.Validate`拒绝无效输入。真正`main`分发到reset-password的子进程用合成配置、专属用户名及短口令在连接数据库前退出，证明不会先被无关维护env挡住；正常服务相同非法配置仍拒启。

`service.NewFixedAuditArchiveJob`仅接受显式开启且合法的固定归档定义，创建绑定固定服务主体与配置的私有wrapper；`App.NewApp`只给进程内auditCron使用，公共registry仍登记需TaskActionPolicy私有准入的`AuditArchiveJob`。人工默认维护、临时覆盖和固定TR job原有批准/当前主体检查保持；无准入的裸Handler默认值或system文本均拒绝。启用定义的tick、retention、batch、目录和文件保留期先校验，不引入无出处的batch额外上限。

固定TR定义摘要由离线`cmd/maintenance-definition-digest`复用唯一`MaintenanceDefinitionDigest`算法；读受审JSONstdin，仅输出摘要，不读服务配置/联网/改DB。操作步骤归[runbook §6.4](../../ops/runbook.md#64-审计维护开通与核查)，固定调度二选一归[cron切换](../../ops/cron-migration.md)。

### UI和恢复证据

任务页面只对实际维护action给独立授权提示；403保留服务端原因，仅该动作及准确错误组合附核查说明，不把所有失败解释成缺maintainer。新增正向spec须显式同owner端点、专属维护者、owned PG及只读TR SQLite，缺少前置直接失败，不skip；不向seed或所有admin自动grant。浏览器发起后须核心/上游持久succeeded、准入主体相符、旧marker在线移除/近期保留，并回读任务成功详情。文件实际导出另由JSONL专项证明。

策略恢复步骤归[runbook §6.3](../../ops/runbook.md#63-当前casbin策略投影失败的恢复)。完整LoadPolicy成功或受控重启才能恢复缓存消费者，ready成功不能单独证明开闸；验证保留权限账号成功及被撤权账号拒绝。原组件级“失败适配器+新Enforcer”证据不冒称真实进程恢复，本批运行专项单独登记。

## 3. 红绿、首失败与候选验证

当前已取得的专项包含：SIGTERM在途事务503对200的有效红；late审计消费者提前停止的红；CLI被非法维护env阻断的红；无准入bare Handler实际进入归档效果的红；无效固定定义被放行的红；UI提示目标行为红。删除关键修复的变异及修正后源码、命令、退出码由各专项收据定位，不把环境/编译/夹具错误算有效行为红。

根新增组织矩阵首轮遇到SQL参数varchar/text推断冲突，属于合成夹具错误，显式参数类型后再跑；该首失败保留。初始UI HTTPS fetch超时在任何产品修改前终止，后续按实际远端快照承接；原始非UTF8内联准备脚本在执行前失败，改用文件脚本。上述不记为产品红例。

主仓冻结`r1`通过`quality-plan/check`的说明文档及planning针对检查；ordinary继续提示不是全门禁通过。随后七项`lint/swag/test-unit/test-integration/test-cover/migrate-up/acceptance`均为0：261个顶层默认单测、635个含integration顶层测试（含单测，不能相加），均零失败/跳过；四档验收253项、零失败/跳过；82对一次全链迁移往返零漂移。默认单测语句覆盖率20.9%，较已合baseline的20.4%增加0.5个百分点。原合同253/81/20.2%、原产品253/82/20.5%、CI夹具后继253/82/20.4%及旧UI322/45/TR226收据保留各自时点。

UI首轮全量45/46，新正向spec按输入框placeholder查找Element Plus选择器而超时，未发任务请求；原log、截图和trace保留。只把这一行改成真实“任务定义”combobox定位，双端成功、准入主体、旧删近留及回读详情断言全部保持。另冻`u2`，逐文件证明其余UI和core/TR/AL源不变，构建资产逐项等于真实服务镜像，再重跑UI六项门禁：37文件334单测、46浏览器用例全通过，零跳过/重试/flaky。当前新正向为真实浏览器→核心→TR→回调，并回读PG与SQLite双端`succeeded`。原无资格403反例保留，旧失败不重写为通过。

本批真实运行专项均为0：

- 策略恢复：延迟到IAM外层提交的隔离故障使撤权已提交、两次`LoadPolicy`报42P01，HTTP500/70004；reader/admin缓存入口503、自服200。恢复原表同OID并删故障trigger后仍503；仅重启同owned容器/同镜像，撤权账号403、合法admin200，配置/源码及15个共享容器生命周期不变。这证明当前受控手动恢复，未交自动reconciler。
- 附件API：实际预签/PUT/confirm、绑定及删除后，title-only成功；包含旧引用的合并更新400且工单/稳定事件/附件/文件元数据不变。optional显式null清键，required拒绝清空，合法新文件可替换，重加已删key仍拒绝。首轮探针选取不存在的`ticket_events.event_type`列属于夹具失败；依据真实表改投影后另跑，原失败保留，不算产品红。
- 进程内固定cron：同冻结镜像、空人工维护名单，实际旧marker移除且出现在JSONL、近期整行不变；非法tick启动拒绝、无HTTP，负marker整行不变。结束恢复base配置，未与TR批准来源混跑。
- 人工/TR：五个无资格入口403；默认180天、真实Submit/Trigger双端持久成功及可信principal成立；临时5天批准准确生效，未批/到期拒绝；先停用审阅的固定TR定义真实调度成功，改摘要或批准到期后最终回调403。结束停用该定义、恢复base批准配置。
- 恢复：实际S3原key导出/独立恢复位置/移除404/还原后下载SHA一致；JSONL独立marker恢复整行一致；PG导出快照并实际恢复到独立临时库，40张public表数据计数/明细SHA和所投影列结构SHA一致、迁移82且clean，临时库已删除。仅同机逻辑恢复，不证明异地、PITR、约束/索引/函数逐项哈希或跨介质原子性。

复跑前owned app曾报告singleton资格丢失并退出1，严格归属检查据此拒绝运行；保存原日志后按同ID/同镜像恢复，后续全量及专项通过。当前守卫日志未区分探测错误与锁缺失，根因仍未确认，不能称新产品红或已解释的故障；真实启用前应补运行诊断并核探测预算。本批保留fail-closed，不以增加重试或放宽失锁保护隐藏该事实。

源审分别覆盖App/audit、配置/维护和CLI、UI及组织/附件回归；发现的pump/flusher收尾竞争、JSONL措辞及持久效果断言缺口已修并复核。准确四组件源manifest、重编二进制/镜像/静态资产、原始结果/命令/日志SHA、组合映射及独审结论统一归本批材料ID；测试定位器与本地探针/证明防护修正均保留原版及新版本。

## 4. 状态、后继及真实启用

F023现势承接已合main的原产品baseline，本批pass8 followup单列实现/验证/远端状态；原合同validation整体保留并明确历史，迁移仍82对、下一000083，本批无迁移。CF-18映射到F6本批证据与独审；远端保护/三节机器强制/全量CI自动化继续原治理窗口，不因当前CI绿关闭整项。原findings是修前审计快照，当前修复/部分交付/限制缓解/开放状态由本报告与[控制台§6](../11-project-control.md#6-当前健康状态遗留问题追踪)承接。

完整IAM-C/C05、C03/F2b、HA、真实UAA/企业消息、完整SLA及未取用P2/P3保持原包；已交子项从后续净剩余逐项扣重，本批不重报价、不改97–151或DAG。后继触发与当前依赖见[计划§2](../../planning/audit-repair-plan-2026-11-17.md#2-发现既有归属与可评审切片)。真实环境人员/数据/保留与恢复责任仍按Q12绑定；原开发服务及活跃UI工作树未切换。

本地执行材料归`maintenance/audit-pass8-followup-20261117/`，最终封存以该材料的`sealed-index.json`及逐项SHA/权限复核为准。旧四批封存不改，不清未知归属资源或原数据卷；本批工作树/分支及运行资源清理另核收据。原始审计findings保留修前时点，当前边界由本报告与控制台承接，不整体关闭70条。

## 5. 功能main承接与收尾证据

主仓[PR #15](https://github.com/tracerbiubiubiu/zhuzhao/pull/15)的准确head为`e280cf363d6346c79eaed02cca8025948a25d9c0`，merge为`21c1e2806f39e15e2c4f39a54f27958597813acf`；UI[PR #9](https://github.com/tracerbiubiubiu/zhuzhao-ui/pull/9)的准确head为`7cf165da42b971b1a4e0b2935062e3d160bc0402`，merge为`af457b8ceea380cf7123ca8d384485f42cd75b72`。均按正常merge锁定已验head，父提交和合并树逐项核对，tree等于已验候选。

主仓[分支CI](https://github.com/tracerbiubiubiu/zhuzhao/actions/runs/44110794277)、[PR CI](https://github.com/tracerbiubiubiu/zhuzhao/actions/runs/44110922755)与UI[分支CI](https://github.com/tracerbiubiubiu/zhuzhao-ui/actions/runs/44110840802)、[PR CI](https://github.com/tracerbiubiubiu/zhuzhao-ui/actions/runs/44110922331)均为success。准确功能merge的主仓[main CI](https://github.com/tracerbiubiubiu/zhuzhao/actions/runs/44114420530)和UI[main CI](https://github.com/tracerbiubiubiu/zhuzhao-ui/actions/runs/44114486495)均为success，准确head与结果随本批收据保留。最后的文档收尾只变更状态及planning派生来源哈希，按实际reference/document档检查，不充作又跑一次产品门禁。

本批自建8容器、7卷、3网络及16个临时镜像标签已逐个回收，owner资源及任务宿主进程零残留；受保护15容器及其卷/网络身份不变，各运行专项所比较的共享进程生命周期不变。原开发进程不重启，活跃canonical UI保留原检出；最新UI main由本批干净detached工作树供最终源码白名单封存，封存后再回收该树。最终五仓Git状态、main包含关系、零遗漏有效内容、精确分支/工作树回收及四旧seal复核由同一maintenance目录下的`audit-pass8-followup-20261117-operation-receipt.json`及其SHA承接，明确绑定sealed-index SHA；不把封存前工作树仍存在的快照称为最后清理状态。封存仅复制本批材料和准确白名单源码，不写旧档案或上传原始材料；执行副本在副本逐项核验后退役，封存payload保持不可变。
