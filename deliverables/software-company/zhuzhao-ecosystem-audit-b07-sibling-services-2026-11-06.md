# 生态审计 B07：兄弟服务与公共库（2026-11-06，阶段报告）

## 范围与基线

固定原业务基线：activelist `af0b350da650517df2c9d15cca4892cdcd51be88`、taskrunner `1dcc83185f24b8dc2bed7bba6786272a5fdfb19f`。两消费者实际 pin utils v0.4.2（tag `21155a98e87a944b20600833f940698d2759c2e5`）；库增量兼核 v0.4.3 `9296ae170b71b06c623c54b6010a107406903346`，不将库HEAD当消费者版本。修复均在独立codex分支，禁止合main；正式发布版本与分支push分别记账。

本报告收录F01–F11十一项已证实发现。F08/F09互引README两项既有债，未重复计数；F10仅库修复完成，消费者pin尚未升级。静态已深读生产模块：AL类型/schema/字段校验/动态DDL/CRUD/传输/handler/中间件/装配；TR提交/取消/重试/cron/reclaim/worker/callback/通知/store/handler/配置与装配；utils所有公开生产包。逐文档阅读、部署恢复动态与所有测试源码尚未全部完成，不能据本阶段报告宣告全面审计通过。原始证据仅本地 `.codex/audits/ECO-20261104/evidence/ECO-20261104-b07/`。

## B07-F01 / P1：activelist事务窗口嵌套取池

基线AL `internal/service/type_service.go` 的Register/Deprecate，`internal/meta/meta.go` 的InsertType重复补查，`internal/service/data_service.go` 的Update→mapDataValidationError字段史判断：已持tx却再次走pool。单连接下分别超时，双连接对照409/422/正常幂等返回。五分支各八轮稳定红，修复 `156ff034c1988509b517f246eb519ef81f829dc5` 同tx补查/直接返回tx现态；候选与固定git archive独立lint、全仓race单元/真实PG集成均0。**代码层复验关闭，运行部署另计**。原快照可点击锚点见末节；符号为主、行号为辅。

## B07-F02 / P1：activelist承诺1GiB导入被前置16MiB验签截断

原AL `internal/app/providers.go` 的provideEngine为整个/api/v1挂16MiB内存验签，import handler直到后面才装1GiB限体。实际装配+正确HMAC+真实PG：15MiB+2合法数组200，17MiB+2为413。非配置失效推测，已动态复现。

修复AL `82290bc2982019781db7b12f0a79ab285752b311`将import独立挂utils `GinStreamingMiddleware`，按配置磁盘私有文件有界流式验签，通过后rewind才进入导入事务；普通CRUD保持16MiB。正式依赖 `v0.4.4-0.20261106064944-9ed38f791175`，远端分支提交 `9ed38f791175a2324028a1cf386ef4318f3e5aa9`，没有replace、没有新tag/合main。utils库固定git archive lint/race全仓/build0，64MiB生成流分配<2MiB，三类移除门禁/cleanup/限体目标各8红。

AL实际provideEngine+真实PG：17MiB导入200、普通CRUD同体413；超配置/坏签/取消/读失败/磁盘失败/解析失败保原数据并清理spool，race专项八轮绿；只恢复16MiB cap变异八红。AL完整race PG集成326.97s0；测试setupPG复用已取消cleanup ctx的卫生修复同笔，补正后service复跑277.84秒通过。**固定82290bc归档独立lint/race单元/build/racePG集成209.79秒均0，archive_equivalent=true，F02代码层复验关闭**。临时盘容量/读取超时/SIGKILL残留策略及磁盘503分档已成文。

已核边界：主仓 `docs/phase3/16-external-integration.md` 的2026-09-10拍板明确“网关1MB保持、大导入走直连运维通道，浏览器大导入需求出现再触发”。当前router.BodyLimit(1<<20)+gateway使用aksk.Transport整包缓冲符合该已接受边界；不擅自把网关扩1GiB。网关真实超1MB拒绝已由主会话证实出现502，另记F06并独立修复；本批AL直连验证不声称网关大文件通过。

## B07-F03 / P0：全量导入重置version使旧客户端ABA覆盖新数据

原AL `repository.InsertImportBatch` 重灌version=1，`DataService.Update`仅id+version CAS。实际旧v1→导入同ID新值v1→旧客户端写入200，旧v2对照409；发生静默数据覆盖，正式按项目“数据损坏”口径由过程P1升P0，推翻设计“跨导入必然409”的断言。修复为全库持久document revision：所有写入口分配新revision，空导入/ID重用不重置，schema定义版本维持自身语义。

root已提交 `afc3700eb80c3d2a5b9c4c5e948f32f1e2c3dafb`，候选及固定git archive独立lint/race单元/racePG集成(133.56s)/build0且归档等价。旧置1变异8红；二次验证又发现只seed MAX(current)遗漏已清空旧表的客户端历史，新增原地升级txid_current屏障与回归，移除屏障变异8红。**代码层复验关闭，部署另计**。升级须停全部旧writer；保证限于正常原地升级与连续数据库世代，手改历史版本/逻辑恢复丢事务历史/down-up重建序列不保证隔离所有旧缓存，要求客户端重读。不能宣称跨数据库恢复的外置世代已实现。

## B07-F04 / P1：taskrunner未创建队列时空死信列表500

原TR `TaskService.ListDeadLetters` 将asynq ErrQueueNotFound原样上抛；全新队列/清库时发生，UI两真实角色FE4也实际失败。此为README 2026-11-02已登记P2债，本轮按功能影响P1复核，不重复债项。修复 `a1309cc0fd28a2c488a5ef7fdeb47dc68002d8bd` 将未建队列归为空数组，其他Redis故障仍500；签名HTTP专项八绿/移除修复八红，固定archive lint/race全仓含PG均0。**代码层复验关闭**；root近期已替换测试TR为该固定版本供UI重跑，正式运行证明由root环境记录承接。

## B07-F05 / P1：taskrunner停机重投同attempt时旧delivery可写新执行终态

原TR `internal/repository/store.go` 的MarkRunning/Finish只用status+attempt，`internal/worker/worker.go` 从asynq.GetRetryCount取attempt。实际依赖asynq v0.26.0 `processor.requeue`→RDB.Requeue在停机预算到期时不递增Retried（lease recoverer Retry是另一路径，不能混同）。旧delivery和新delivery均attempt=1，C6“过期写者拒绝”声明不成立。

真实证据：先对原快照SQLite repo探针8红；再在a1309cc源码只加探针，用真实PG/Redis/两个真实Asynq Server：server1回调端口阻塞，shutdown timeout真实重排，server2同attempt=1开始新回调并阻塞；释放旧handler后，DB错误变succeeded而新handler仍在执行。8/8稳定红（16.37s）。回调端口故意忽略取消以暴露在途goroutine，未声称真实HTTP远端恰好复现同序；真实broker/PG/worker路径均未替换。证据 `taskrunner-fence-real-asynq-baseline.{log,json}`，独立容器eco-b07-fence-pg/redis@50072/50073，仅此批自用。

**修复 ed1a40e0dcf1ae6ae9c17e19720259132dc3bc88 已固定归档复验，代码层关单，固定时点尚未部署（最新运行态见末节）**：独立 `codex/fix-taskrunner-delivery-fence` 每delivery UUID写入私有execution_id，Finish三种终态均须匹配；较旧attempt不得覆盖新claim，手动retry可重启计数但不复用身份，过期终态不发死信通知。PG/SQLite启动增列兼容、所有调用口、CI Redis实际跑真实回归配套。PG/SQLite新旧库迁移和身份专项八轮绿；真实停机重投三终态各八轮绿；三类移除修复变异稳定红；固定归档lint、全仓race含PG/Redis、专项八轮、build、19项含cron E2E全过且归档等价。原E2E因W0b后夹具仍按回调路径分流而16绿3红，保留失败；改用签名params选择合成故障，原断言不变。独立自有PG/Redis及恢复夹具库已清理，记录taskrunner-fence-environment-cleanup.json。

## 后续覆盖与限制

- 两项既有panic债已在F08/F09单列关闭，不混入F05；B06历史一次502没有证据归因为panic。
- 双服务部署/备份/恢复脚本已继续阅读全文，真实逻辑恢复演练已完成（见文末）；不能引用历史PITR实测为本轮重跑。AL PITR物理basebackup边界已成文，TR队列不备份/Redis丢失与reclaim三域的精确说明需再对账。
- 剩余历史长文与逐文档/模块覆盖索引继续补齐，未来phase6能力目录/SDK、phase7–9尚未开发不计当前缺陷。
- 本地原始证据保留所有夹具失败、观察超时与重跑；固定提交/运行部署/最终联合验收状态分列。主仓控制台与正式报告由root统一回填提交。

2026-11-06 补验：AL afc3700与TR ed1a40e的真实PG15逻辑备份/恢复已过，使用实际backup.sh do_backup/rotate函数（省略无限cron外壳）。AL活动/软删数据及revision序列恢复后新值5高于原2/4；TR三状态/attempt与execution_id列恢复后可新执行。证据backup-restore/results.json；不冒充本轮PITR。TR恢复口径漂移已在独立docs提交df7ef36824f47eaa4832b20d214fed5a342b9b50修正，仅1文件。

## B07-F06 / P1：网关本地请求体超限被错报成上游502

主仓BodyLimit仍为已拍板1MiB；Transport读取MaxBytesReader出错进入ReverseProxy统一ErrorHandler，误报502/10008。真实测试app d38和专属E800950/admin：同一不存在的AL类型，100字节404/100002，1048577字节502/10008，与上游实际不可用混淆。账号自清、未改E000001，无token入证据。

独立修复 `02a6e2cdaf00df89f63c95f573675f2fa1896f9e` 在 `internal/gateway/gateway.go` 的 `buildMount/ErrorHandler` 用errors.As识别MaxBytesError返回413/10001，真实上游故障仍502，不扩1MiB边界。原探针八红、修复gateway整包race八绿、只移除映射变异八红；已随固定12401aa完整主仓门禁通过，代码层关闭。常驻f69到AL19真实JWT/Casbin/HMAC链重验小请求404、大请求413；专属号logout并删除完成。证据gateway-body-limit/{live-baseline.json,live-fixed.json,live-fixed-cleanup.json,mutation-result.json}。

## B07-F07 / P2：activelist服务端数据库诊断被原样回显

原AL `internal/handler/response.go` 的 `Fail` 对所有HTTP错误统一composeMsg，而repository六处把数据库错误拼入500 Msg。真实HMAC+PG导出：登记类型后由审计夹具管理性删除动态表，导出500包含SQLSTATE 42P01与内部表名，八轮稳定复现；不声称API用户可DROP表或已泄漏凭证。统一错误治理缺口按P2。

修复 `3d01e8a39b6cec550aa7fd86168251e39465b038`（独立codex/fix-activelist-error-envelope，base82290bc）：Fail对5xx保HTTP/业务code/request ID，对外使用utils规范500/503消息且不拼Detail，原apperr加入c.Errors由现有AccessLog保留诊断；4xx修正提示不变。真实装配+HMAC+PG八轮外无SQLSTATE/表名且内部日志保42P01+request ID；移除统一守卫八红（v2证据断言先检查私有细节，第一次仅校验器匹配断言文字不符，原8红日志保留）。固定git archive独立lint/race全单测/build/race全PG集成138.46s全部0且归档等价，**代码层关单（最新运行态见末节），未push**。证据activelist-error-fixed-results.json、activelist-error-v2-results.json、activelist-raw-error-baseline.json。

## B07-F08 / P2、B07-F09 / P2：两项既有panic债（已复验）

README 2026-11-02两个P2原债已去重。原1dcc831固定归档+故障注入：cron/reclaim/notifier各8轮真实子进程exit2；真实Asynq+SQLite/PG回调末次/耗尽panic归档但DB仍running，三个reclaim域0/0/0，共32红。注入仅证明异常隔离/回写缺口，不声称正常网络输入可直接触发panic；B06历史502无额外证据仍不归因为panic。修复产品提交 `3b268dc350e2f7fb14941963cca902f946798ee7`，测试观测校准后最终固定HEAD `5e3f5f3720a54fad9f5cec4a08481364693b8f7d`，codex/fix-taskrunner-panic-boundaries：后台单tick捕获Error+堆栈后下一tick继续，notifier单通知捕获后下一通知继续；回调panic转普通可重试错误，由现有failed/dead/retry与execution_id栅栏统一处理，过期不写不通知。四种移除捕获分别八轮红（callback双存储末次/耗尽32红），候选八轮绿；最终固定归档lint/全PGRedis race(11.9s)/专项八轮(19.63s)/build/19项含cron E2E(73.0s)全0、archive_equivalent=true。**F08/F09代码层关单（最新运行态见末节），未push**；root接运行部署。第一次3b固定归档专项假红为先读DB running再读broker archived的非原子观测；后笔只改archive后重读DB，原日志保留，改后基线仍32红。自有PG/Redis按ID+owner已清，taskrunner-panic-environment-cleanup.json。回调外任意程序错误、进程致命错误、DB持续写失败仍不扩大恢复承诺。

F08源码锚点（固定5e3f5f3；原1dcc同符号）：[cron.Loop.Run](/Users/bujibuji/code/src/github.com/tracerbiubiubiu/taskrunner-audit-panic/internal/service/cron.go:67)、[ReclaimLoop.Run](/Users/bujibuji/code/src/github.com/tracerbiubiubiu/taskrunner-audit-panic/internal/service/reclaim.go:62)、[单tick runBackgroundTick](/Users/bujibuji/code/src/github.com/tracerbiubiubiu/taskrunner-audit-panic/internal/service/background.go:10)、[Notifier.NotifyDeadLetter](/Users/bujibuji/code/src/github.com/tracerbiubiubiu/taskrunner-audit-panic/internal/notify/notifier.go:60)。F09：[Deps.handleCallback回调入口](/Users/bujibuji/code/src/github.com/tracerbiubiubiu/taskrunner-audit-panic/internal/worker/worker.go:115)、[Deps.callCallback恢复边界](/Users/bujibuji/code/src/github.com/tracerbiubiubiu/taskrunner-audit-panic/internal/worker/worker.go:162)。证据：原归档taskrunner-panic-baseline.json/log；四变异taskrunner-panic-mutations.json及对应.log；观测校准候选/基线taskrunner-panic-v2-results.json；最终taskrunner-panic-fixed-v2-results.json。每项与README2026-11-02原始债互引，禁止重复计数。

## B07-F10 / P2：utils双Transport读取失败不Close原Body

9ed38f791175a2324028a1cf386ef4318f3e5aa9固定归档中，internal与compat自动签名Transport在ReadAll失败时直接return，违反Go RoundTripper错误路径亦须Close的资源契约；八轮每实现8次Close=0。生态已发现生产引用仅主仓gateway.buildMount的aksk.Transport（/al），compat无当前生产引用；HTTP入站body可能最终由net/http关闭，未证明当前线上持续FD泄漏，不夸大P0/P1。修复 `cd759a0402455731f805ba507bf8401447e00e29`（codex/fix-aksk-transport-close-audit）读后先Close，再返回原readErr；正向克隆签名、发送错误、close错误优先级不变。两协议五种组合8绿；各自仅恢复错误分支旧Close顺序，分别8轮16红。固定归档lint/race全仓19.89s/build均0，archive等价，**代码关单，未push/无消费者升级/无正式tag**。证据utils-boundary-probe.json、utils-transport-mutations.json、utils-transport-fixed-results.json。

P3补充输入口径：Int64Slice数组[null,"7"]八轮被接受成[0,7]。下游六字段静态核对：User.Create/SetRoles查RoleRepo.FindByID；User.Create/Org.SetUserOrgs查OrgRepo.FindByID；SetUserOrgsTx还以lockAliveOrgForShareTx+SELECT存活兜底；Org.SetOwners查UserRepo.FindByID；RBAC.AssignMenus比对active菜单结果数。未证明0成功授权或数据损坏；应400而可能404的错误层口径偏差只记账，保留顶层null既有约定，本批不扩修。

## B07-F11 / P1：taskrunner生产compose仍探8080，镜像已监听18080

原baseline1dcc831至5e3的Dockerfile ENV为:18080、冻结yaml也:18080，而deploy/compose.yaml未覆盖监听，healthcheck/共享网说明与部署README仍8080，env-only代码default亦遗落8080。实际原production compose自用project装配（仅固定image/资源归属/外网名覆盖，保留原healthcheck 10秒×5次）：原探针连续拒绝后unhealthy，同容器18080/healthz为200，8080拒绝。是生产部署门禁P1，不推断主仓当前test/dev故障。

修复 `eb3c0c07615c9646da9a9ada673de16843c75564`，独立codex/fix-taskrunner-compose-port（base5e3）：compose/部署说明/env-onlydefault对齐既定18080，冻结yaml不动、显式env优先级不变、无宿主新增ports。固定归档lint/race全仓(7.39s)/build均0且等价；按原Dockerfile构建固定镜像sha256:4e5ee12dd3b1e010d0d44b0abaf2a71473bc60e091f96e6c066282538508f455，以固定原production compose装配healthy。实际容器对候选目标8绿/仅恢复旧8080目标8红，config默认/yaml/env8绿/移除default修正8红。**代码与独立production拓扑关单，主仓test运行态仍由root记录，不要求本笔替换5e3**；本笔未重复PG专属/E2E，5e3完整PGRedis/19断言证据单列。证据taskrunner-compose-port-production-baseline.json、taskrunner-port-fixed-results.json、taskrunner-compose-port-fixed-results.json、taskrunner-port-mutation-default.json；自有演练project/volume/network已清（taskrunner-compose-port-cleanup.json）。

## 已核源码锚点与最新边界

activelist原快照：[`TypeService.Register`](https://github.com/tracerbiubiubiu/activelist/blob/af0b350da650517df2c9d15cca4892cdcd51be88/internal/service/type_service.go#L32)、[`InsertType`](https://github.com/tracerbiubiubiu/activelist/blob/af0b350da650517df2c9d15cca4892cdcd51be88/internal/meta/meta.go#L82)、[`DataService.Update`](https://github.com/tracerbiubiubiu/activelist/blob/af0b350da650517df2c9d15cca4892cdcd51be88/internal/service/data_service.go#L185)、[`provideEngine`](https://github.com/tracerbiubiubiu/activelist/blob/af0b350da650517df2c9d15cca4892cdcd51be88/internal/app/providers.go#L60)、[`InsertImportBatch`](https://github.com/tracerbiubiubiu/activelist/blob/af0b350da650517df2c9d15cca4892cdcd51be88/internal/repository/transfer.go#L67)。

taskrunner原快照：[`TaskService.ListDeadLetters`](https://github.com/tracerbiubiubiu/taskrunner/blob/1dcc83185f24b8dc2bed7bba6786272a5fdfb19f/internal/service/task_service.go#L282)、[`Store.MarkRunning`](https://github.com/tracerbiubiubiu/taskrunner/blob/1dcc83185f24b8dc2bed7bba6786272a5fdfb19f/internal/repository/store.go#L443)、[`Store.Finish`](https://github.com/tracerbiubiubiu/taskrunner/blob/1dcc83185f24b8dc2bed7bba6786272a5fdfb19f/internal/repository/store.go#L475)。F05真实worker探针受审快照为a1309cc（只改死信读面，MarkRunning/Finish仍与上述原基线相同），不把其他候选修复混入原证据。

F06：[internal/gateway/gateway.go:86](https://github.com/tracerbiubiubiu/zhuzhao/blob/5c03581d5999139a3e8e5ead5972c026868aa181/internal/gateway/gateway.go#L86)，符号/配置锚 `func buildMount`。

F07：[internal/handler/response.go:25](https://github.com/tracerbiubiubiu/activelist/blob/af0b350da650517df2c9d15cca4892cdcd51be88/internal/handler/response.go#L25)，符号/配置锚 `func Fail`。

F08：[internal/service/cron.go:55](https://github.com/tracerbiubiubiu/taskrunner/blob/1dcc83185f24b8dc2bed7bba6786272a5fdfb19f/internal/service/cron.go#L55)，符号/配置锚 `) Run(`。

F09：[internal/worker/worker.go:68](https://github.com/tracerbiubiubiu/taskrunner/blob/1dcc83185f24b8dc2bed7bba6786272a5fdfb19f/internal/worker/worker.go#L68)，符号/配置锚 `func (d Deps) handleCallback`。

F10：[aksk/gin.go:154](https://github.com/tracerbiubiubiu/zhuzhao-utils/blob/9ed38f791175a2324028a1cf386ef4318f3e5aa9/aksk/gin.go#L154)，符号/配置锚 `func (t *Transport) RoundTrip`。

F11：[deploy/compose.yaml:99](https://github.com/tracerbiubiubiu/taskrunner/blob/1dcc83185f24b8dc2bed7bba6786272a5fdfb19f/deploy/compose.yaml#L99)，符号/配置锚 `8080/healthz`。

## 最新联合源码与测试运行态（2026-11-06）

AL联合 `19a4c1d5cd84cc892b82ad52554db320bfad8f7f`（池/revision/流式导入/通用5xx）固定归档lint/单元/完整racePG集成151.1s及Linux构建通过；TR `5e3f5f3720a54fad9f5cec4a08481364693b8f7d` 含死信/delivery/panic，固定完整PGRedis与19项E2E通过。仅替换zt-activelist/zt-taskrunner，容器内二进制SHA和image ID逐一匹配，readyz均200，AL新序列/TR execution_id列实际可查；app保持f69，UI/RustFS/PG/Redis及dev容器ID/启动时间保持。运行证据ECO-20261104-joint/test-runtime-siblings-result.json，未合main。

utils cd759a0修复仅独立库分支，AL仍pin reader的9ed正式pseudo-version、主仓联合仍pin v0.4.3，F10消费端承接明确未完成。TR eb3c0c0默认/production health修复仅独立生产拓扑验证，常驻测试TR仍5e；当前测试显式监听18080，不把本次生产探针缺陷推断为测试栈故障。自有生产演练项目均按归属清理。

全文覆盖和所有组合故障尚未完成；逻辑恢复已实测，不宣称本轮PITR。新增关键缺陷继续按报告/修复/变异/固定门禁闭环，其余P2/P3统一记账，不能以十一项已核或运行升级替代全面审计结论。
