# 生态审计 B07：兄弟服务与公共库（2026-11-06，阶段报告）

## 范围与基线

固定原业务基线：activelist `af0b350da650517df2c9d15cca4892cdcd51be88`、taskrunner `1dcc83185f24b8dc2bed7bba6786272a5fdfb19f`。两消费者实际 pin utils v0.4.2（tag `21155a98e87a944b20600833f940698d2759c2e5`）；库增量兼核 v0.4.3 `9296ae170b71b06c623c54b6010a107406903346`，不将库HEAD当消费者版本。修复均在独立codex分支，禁止合main；正式发布版本与分支push分别记账。

本报告收录目前五项已证实发现。静态已深读生产模块：AL类型/schema/字段校验/动态DDL/CRUD/传输/handler/中间件/装配；TR提交/取消/重试/cron/reclaim/worker/callback/通知/store/handler/配置与装配；utils所有公开生产包。逐文档阅读、部署恢复动态与所有测试源码尚未全部完成，不能据本阶段报告宣告全面审计通过。原始证据仅本地 `.codex/audits/ECO-20261104/evidence/ECO-20261104-b07/`。

## B07-F01 / P1：activelist事务窗口嵌套取池

基线AL `internal/service/type_service.go` 的Register/Deprecate，`internal/meta/meta.go` 的InsertType重复补查，`internal/service/data_service.go` 的Update→mapDataValidationError字段史判断：已持tx却再次走pool。单连接下分别超时，双连接对照409/422/正常幂等返回。五分支各八轮稳定红，修复 `156ff034c1988509b517f246eb519ef81f829dc5` 同tx补查/直接返回tx现态；候选与固定git archive独立lint、全仓race单元/真实PG集成均0。**代码层复验关闭，运行部署另计**。原快照锚点见末节，符号为主、行号为辅。

## B07-F02 / P1：activelist承诺1GiB导入被前置16MiB验签截断

原AL `internal/app/providers.go` 的provideEngine为整个/api/v1挂16MiB内存验签，import handler直到后面才装1GiB限体。实际装配+正确HMAC+真实PG：15MiB+2合法数组200，17MiB+2为413。非配置失效推测，已动态复现。

修复AL `82290bc2982019781db7b12f0a79ab285752b311`将import独立挂utils `GinStreamingMiddleware`，按配置磁盘私有文件有界流式验签，通过后rewind才进入导入事务；普通CRUD保持16MiB。正式依赖 `v0.4.4-0.20261106064944-9ed38f791175`，远端分支提交 `9ed38f791175a2324028a1cf386ef4318f3e5aa9`，没有replace、没有新tag/合main。utils库固定git archive lint/race全仓/build0，64MiB生成流分配<2MiB，三类移除门禁/cleanup/限体目标各8红。

AL实际provideEngine+真实PG：17MiB导入200、普通CRUD同体413；超配置/坏签/取消/读失败/磁盘失败/解析失败保原数据并清理spool，race专项八轮绿；只恢复16MiB cap变异八红。AL完整race PG集成326.97s0；测试setupPG复用已取消cleanup ctx的卫生修复同笔，补正后service race全量复跑277.84秒/exit0；固定归档独立门禁进行中。**固定82290bc归档独立复验待完成，F02暂不关单**。临时盘容量/读取超时/SIGKILL残留策略及磁盘503分档已成文。

已核边界：主仓 `docs/phase3/16-external-integration.md` 的2026-09-10拍板明确“网关1MB保持、大导入走直连运维通道，浏览器大导入需求出现再触发”。当前router.BodyLimit(1<<20)+gateway使用aksk.Transport整包缓冲符合该已接受边界；不擅自把网关扩1GiB。网关真实超1MB拒绝证据待补，不能把上述AL直连验证声称全网关大文件链通过。

## B07-F03 / P0：全量导入重置version使旧客户端ABA覆盖新数据

原AL `repository.InsertImportBatch` 重灌version=1，`DataService.Update`仅id+version CAS。实际旧v1→导入同ID新值v1→旧客户端写入200，旧v2对照409，推翻设计“跨导入必然409”的断言。过程初核为P1；主审按本轮“P0=数据损坏”口径升为P0，依据是已证实新导入业务数据被旧客户端静默覆盖，不只是假设风险。修复为全库持久document revision：所有写入口分配新revision，空导入/ID重用不重置，schema定义版本维持自身语义。

root已提交 `afc3700eb80c3d2a5b9c4c5e948f32f1e2c3dafb`，候选及固定git archive独立lint/race单元/racePG集成(133.56s)/build0且归档等价。旧置1变异8红；二次验证又发现只seed MAX(current)遗漏已清空旧表的客户端历史，新增原地升级txid_current屏障与回归，移除屏障变异8红。**代码层复验关闭，部署另计**。升级须停全部旧writer；保证限于正常原地升级与连续数据库世代，手改历史版本/逻辑恢复丢事务历史/down-up重建序列不保证隔离所有旧缓存，要求客户端重读。不能宣称跨数据库恢复的外置世代已实现。

## B07-F04 / P1：taskrunner未创建队列时空死信列表500

原TR `TaskService.ListDeadLetters` 将asynq ErrQueueNotFound原样上抛；全新队列/清库时发生，UI两真实角色FE4也实际失败。此为README 2026-11-02已登记P2债，本轮按功能影响P1复核，不重复债项。修复 `a1309cc0fd28a2c488a5ef7fdeb47dc68002d8bd` 将未建队列归为空数组，其他Redis故障仍500；签名HTTP专项八绿/移除修复八红，固定archive lint/race全仓含PG均0。**代码层复验关闭**；root近期已替换测试TR为该固定版本供UI重跑，正式运行证明由root环境记录承接。

## B07-F05 / P1：taskrunner停机重投同attempt时旧delivery可写新执行终态

原TR `internal/repository/store.go` 的MarkRunning/Finish只用status+attempt，`internal/worker/worker.go` 从asynq.GetRetryCount取attempt。实际依赖asynq v0.26.0 `processor.requeue`→RDB.Requeue在停机预算到期时不递增Retried（lease recoverer Retry是另一路径，不能混同）。旧delivery和新delivery均attempt=1，C6“过期写者拒绝”声明不成立。

真实证据：先对原快照SQLite repo探针8红；再在a1309cc源码只加探针，用真实PG/Redis/两个真实Asynq Server：server1回调端口阻塞，shutdown timeout真实重排，server2同attempt=1开始新回调并阻塞；释放旧handler后，DB错误变succeeded而新handler仍在执行。8/8稳定红（16.37s）。回调端口故意忽略取消以暴露在途goroutine，未声称真实HTTP远端恰好复现同序；真实broker/PG/worker路径均未替换。证据 `taskrunner-fence-real-asynq-baseline.{log,json}`，独立容器eco-b07-fence-pg/redis@50072/50073，仅此批自用。

**修复中、尚未提交/复验关单**：独立 `codex/fix-taskrunner-delivery-fence` 每delivery UUID写入私有execution_id，Finish三种终态均须匹配；较旧attempt不得覆盖新claim，手动retry可重启计数但不复用身份，过期终态不发死信通知。PG/SQLite启动增列兼容、所有调用口、CI Redis实际跑真实回归配套。当前候选真实重投八轮绿，更多同类/变异/门禁待完成，不能据此关单或宣称已部署。

## 后续覆盖与限制

- taskrunner README两个既有panic类债仍在核验：后台cron/reclaim/notifier无recover，worker panic后DB running；不将它们混入F05，未将B06一次502推断为panic。
- 双服务部署/备份/恢复脚本已继续阅读全文，恢复演练尚未完成；不能引用历史PITR实测为本轮重跑。AL PITR物理basebackup边界已成文，TR队列不备份/Redis丢失与reclaim三域的精确说明需再对账。
- 剩余历史长文与逐文档/模块覆盖索引继续补齐，未来phase6能力目录/SDK、phase7–9尚未开发不计当前缺陷。
- 本地原始证据保留所有夹具失败、观察超时与重跑；固定提交/运行部署/最终联合验收状态分列。主仓控制台与正式报告由root统一回填提交。

## 已核源码锚点与最新边界

activelist原快照：[`TypeService.Register`](https://github.com/tracerbiubiubiu/activelist/blob/af0b350da650517df2c9d15cca4892cdcd51be88/internal/service/type_service.go#L32)、[`InsertType`](https://github.com/tracerbiubiubiu/activelist/blob/af0b350da650517df2c9d15cca4892cdcd51be88/internal/meta/meta.go#L82)、[`DataService.Update`](https://github.com/tracerbiubiubiu/activelist/blob/af0b350da650517df2c9d15cca4892cdcd51be88/internal/service/data_service.go#L185)、[`provideEngine`](https://github.com/tracerbiubiubiu/activelist/blob/af0b350da650517df2c9d15cca4892cdcd51be88/internal/app/providers.go#L60)、[`InsertImportBatch`](https://github.com/tracerbiubiubiu/activelist/blob/af0b350da650517df2c9d15cca4892cdcd51be88/internal/repository/transfer.go#L67)。

taskrunner原快照：[`TaskService.ListDeadLetters`](https://github.com/tracerbiubiubiu/taskrunner/blob/1dcc83185f24b8dc2bed7bba6786272a5fdfb19f/internal/service/task_service.go#L282)、[`Store.MarkRunning`](https://github.com/tracerbiubiubiu/taskrunner/blob/1dcc83185f24b8dc2bed7bba6786272a5fdfb19f/internal/repository/store.go#L443)、[`Store.Finish`](https://github.com/tracerbiubiubiu/taskrunner/blob/1dcc83185f24b8dc2bed7bba6786272a5fdfb19f/internal/repository/store.go#L475)。F05真实worker探针受审快照为a1309cc（只改死信读面，MarkRunning/Finish仍与上述原基线相同），不把其他候选修复混入原证据。

utils流式提交仅推独立分支，远端head已实核`9ed38f791175a2324028a1cf386ef4318f3e5aa9`；无main合并、新tag或所有消费者已升级的声明。AL F03固定归档`revision-fixed-results.json`的lint/race单元/race集成/build均0且archive_equivalent=true。TR a1309cc已在隔离test部署，容器内二进制SHA实核，dev不动；AL池/revision/import尚待最终联合运行版本承接。

F05原样含cron的E2E首轮16过/3败，故障夹具仍试图用callback路径控制404/500，但W0b后产品固定唯一服务端callback目标，故请求均落200。属于测试夹具与已拍板接口失配；原始失败保留，修复须改合成参数控制故障、保留原断言，不恢复客户端任意callback URL。F05仍待全部正常门禁/变异/固定归档复验，不据候选八绿关单。

本批最终收口仍须真实网关1MB负向、部署恢复演练与完整覆盖索引，所有必要实现提交后还要按发现回归及相似场景二次验证。未合main；不以报告落档代表全面审计完成。
