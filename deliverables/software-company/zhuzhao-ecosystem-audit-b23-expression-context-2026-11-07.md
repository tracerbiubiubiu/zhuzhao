# B23：表达式字典查询复用当前事务与请求上下文

## 范围与基线

Phase5小范围稳定性批，发现基线为主仓 `996c167f4567e39362226937b25368b55b7f44bd`，正式落入前基线 `f5a8a892579fd44f0bc5c0e633d541757d621c06`。检查 `ticket.Service.Update` 的 formula / visible_when 及同类转换 validator 求值链。UI、AL、TR、utils不需要此实现修改；不合main、不启动Phase6。通用dbtx传播器试点与请求deadline另记，不借本批宣称已实现。

## B23-F01 / P1：lookup_dict在已有事务中再次向连接池取连接

根因链为 `Service.Update → LockCustomDataTx → validateCustomData / applyFormulas → exprcalc → RegisterExprDictLookup`。外层已传入tx，字典闭包却保留装配期pool并执行 `pool.QueryRow(context.Background(), ...)`，既丢当前事务，又切断请求取消。池被事务占满后，第二次获取无法完成；客户端超时不释放仍在等待的事务。

原问题固定锚点：[exprfuncs.go 的 `RegisterExprDictLookup`](https://github.com/tracerbiubiubiu/zhuzhao/blob/996c167f4567e39362226937b25368b55b7f44bd/internal/service/ticket/exprfuncs.go#L15)、[service.go 的 `Service.Update / applyFormulas`](https://github.com/tracerbiubiubiu/zhuzhao/blob/996c167f4567e39362226937b25368b55b7f44bd/internal/service/ticket/service.go#L546)、[type_admin.go 的 `validateCustomData`](https://github.com/tracerbiubiubiu/zhuzhao/blob/996c167f4567e39362226937b25368b55b7f44bd/internal/service/ticket/type_admin.go#L794)。此前修复 `ListTicketTypeFieldsTx` 只覆盖字段定义读取，没有覆盖表达式里的字典查询；不能由旧BK11销账推出所有表达式均安全。

### 动态与变异证据

- 专属真实PG、合法字典/字段配置均保存成功，Service.Create成功。真实TCP HTTP调用现有TicketHandler.Update，MaxConns=1时原公式8个独立进程均触发700ms客户端超时，取消后handler未返回、池占用1、PG为idle in transaction，goroutine同时有lookup闭包/Pool.Acquire。该小型探针注入既有actor身份，定位service/handler资源问题，不作为完整鉴权证书。
- 同配置MaxConns=2和MaxConns=1纯字符串函数对照均HTTP200并释放连接，排除坏配置或PG不可达。
- 恢复原实现与修复实现串行交替8轮，每轮formula和visible_when各一例：恢复后16/16阻塞，修复16/16 HTTP200；修复的查询backend PID等于锁行PID，事务内额外pool acquire=0。另有原公式8轮独立复现，不把准备期SQL 42P08/JSONB 22P05失败计作业务复现。
- 请求上下文最小候选再核8轮两路径16次HTTP200，每次只落1个updated事件、池占用归0。两种请求取消对照均handler返回、占用归0、数据不变且updated事件0；客户端取消不伪记HTTP成功。
- 常规集成回归通过保留备用连接与有界watchdog避免退化测试永久挂死，同时断言额外获取为0和锁/查询同backend；64次相同已编译程序跨两个独立DB/未提交事务求值，互不串连接或数据。单元回归验证64并发调用的上下文隔离、原字段map不变、私有槽覆盖保护、旧纯函数兼容与原有`$env`拒绝边界。

## 修复与影响面

`EvalEnv.Context / ApplyFormulasContext`为每次求值传递context；编译Program只缓存函数定义，不保存请求、tx或ctx。运行时复制字段后写私有槽，借expr既有WithContext注入typed函数参数；原ApplyFormulas纯函数入口保留。ticket在formula/visible_when绑定当前RowQuerier，在事务前的转换validator绑定service池。lookup通过该读通道并使用求值ctx；直接表达式调用的兼容后备池同样遵守ctx。

上述HTTP探针后，独立复核补充了读取收尾错误处理：使用现有pgx.CollectOneRow，在返回label前完成读取并检查最终错误。该最终代码已纳完整Go门禁；新增真实字典行+仅收尾报错的Rows包装回归，避免“已Scan出一列”被误判成功。没有新依赖、迁移、端点、权限码、配置项或DSL语法；纯计算仍只有既有限长等约束，context传递不等于对expr VM纯计算的抢占式超时。对外查询未命中/不可读保持既有参数错误语义。

同类扫描覆盖生产EvalFormula/ApplyFormulas调用、lookup注册、Create/Update/visible_when/转换validator。现存显式q传递不因本批改为全仓隐式事务传播。通用dbtx试点和请求deadline仍为独立项，不关闭其历史债。

## 验证与承接状态

- 明确选择可信原生Go1.26.10，完整原样 `make test-unit / test-cover / test-integration` 均exit0（10.63s / 5.55s / 566.02s），353项Go源码/生成物及go.mod/sum起止SHA相同。该组合同时包含B19存储ID候选，源码与证据分别归属，提交仍分批。
- `make lint` exit0（3.641s）；Swagger三生成物仅为新鲜度比较显式暂存，结束即撤暂存，无新漂移。`make swag wire` 首次off代理因私有业务缓存缺Wire运行库而失败，改用已有可信本地file代理后exit0（3.842s）；不改go.mod/sum、不绕过生成器。
- 最终生产ELF SHA256 `82179b0bc48a4337e78fabdb2448110c4ede038e4b634e6b486c4fcd9dfd95f0`，Go1.26.10；隔离组合镜像 `sha256:1c6db8f4b15a11e2f7f8ab989b09fc7daac48979b381e0f079ec2b62a1d35155`，基镜像层与环境保持。它同时含B19和B23，当前明确标为uncommitted candidate，不冒称某一提交的已部署产物。
- 原样 `make migrate-up` 与四档 `make acceptance` 在新组合隔离栈均exit0：253/0/0，81对迁移往返计数/明细零漂移，前后schema均81 clean（40.192s）。实际三Go容器ELF逐字匹配清单，主仓353源匹配821构建快照。UI完整门禁另出现B20撤销/重做用例首次及重试失败，不将该前端批次记绿；它不替代或否定本批独立Go与API出口。准确修复hash、CI与常驻环境承接尚未完成，整体收口仍未完成。

原始探针、失败/正控/变异、源码SHA、独立复核及完整门禁日志仅在ignored `.codex/audits/ECO-20261104/phase5-closeout-20261107/{dbtx-pilot,expression-gates,expression-runtime}/`。正式新回归锚点：`internal/pkg/exprcalc/context_test.go` 的 `TestContextFunction_*`，`internal/service/ticket/expr_context_integration_test.go` 的 `TestExprLookup_*`；提交后补完整hash链接。常驻服务、数据与main均未由本批更新。

## 固定提交与最终组合补证

本批主仓完整提交 `82bda27e5a46c22d3d57daab4387dc1aa5dbae93`。最终运行源码354项逐字绑定主仓 `f87ab1aefcbf1843bd8714c5accf245033f99d8b`，原样253/81及29文件230单测、41/41 E2E均通过；[组合补证与保留限制](./zhuzhao-ecosystem-audit-b20-ui-feedback-2026-11-07.md#验证证据与已纠正的初版误判)保留初版失败、wrapper缺checker与未执行附加control，不扩大本批原覆盖。后续主4db/UI152准确CI与dev/test承接已核，当前映射及Chrome代表走查见[B27最终收口](./zhuzhao-ecosystem-audit-b27-phase5-closeout-2026-11-07.md)；main/生产未变。上文保留本批原轮次与未执行路径，不用新时点倒补旧证据。
