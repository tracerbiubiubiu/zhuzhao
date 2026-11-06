# 生态全面审计 B14：资料写入覆盖账号安全状态（2026-11-06）

> **最终后端联合复验（2026-11-06）**：B14-F01/P0在上述完整联合范围代码复验关闭，并已test承接；原禁用/强改覆写、两个资料入口/返回状态、旧AT及正常安全写均保留专门回归。 固定源dc3d540完整门禁及test承接已过，下文早先“待验/未部署”为原时点；未合main。

## 结论、范围与基线

**ECO-20261104-B14-F01 / P0，代码复验关闭、已test承接：在途个人资料更新能覆盖管理员刚写入的禁用状态或强制改密标记，使后续新登录重新通过。** 采用本项目“安全/门禁失效”分级；不是匿名访问、密码回滚或R01旧令牌再次复活。两场景同根记一项。

固定受审实现 **`b4435022d00ee789b9a62f74036824bd7d3791b7`**，包含R01纪元修复。UserService/UserRepo相关路径与原主仓一致，Auth/JWT采用该完整固定源码，不能混用主工作树的修复前认证。随后联合候选b306eed的相关路径未变。实验使用独立PG全81迁移和Redis，全部账号为合成E8段；未写共享dev/test、E000001或真实业务数据。

root从只读数据层复核发现候选后占用B14批，要求实际服务交错与认证结果验证；原始静态推断不单独计作P0证据。修复现已在独立codex分支提交并承接到联合候选；尚未完成组合全门禁，未合main或部署。

## 根因与锚点

原固定hash如上；可点击代码：[UserService.UpdateProfile](/Users/bujibuji/code/src/github.com/tracerbiubiubiu/zhuzhao/internal/service/user_service.go:375)、[UserRepo.Update](/Users/bujibuji/code/src/github.com/tracerbiubiubiu/zhuzhao/internal/repository/user_repo.go:259)、[UserRepo.UpdateStatus / UpdateStatusTx / UpdatePassword](/Users/bujibuji/code/src/github.com/tracerbiubiubiu/zhuzhao/internal/repository/user_repo.go:304)。代码符号为主、行号辅助，认证固定源另见本地raw source-manifest。

个人资料先FindByID读取整份User，再调用通用Update。通用SQL不只写请求中的姓名/联系方式，还把旧User.status与must_change_password一并回写，WHERE仅以User.version做CAS。管理员禁用、重置密码使用的安全专用写没有推进该version；因此它们发生在资料读取和最终UPDATE之间时，旧资料写仍成功，把安全字段恢复成旧值。

其他调用方包括管理面资料更新；修复必须对共用写入口做同类扫描，不能只修一个handler或要求个人资料客户端携带原本没有的安全字段。密码hash不在这次通用回写中，实验没有把重置后的密码改回旧口令。

## 真实复现与对照

pgx QueryTracer只在生产UserRepo.Update的SQL即将发送前设置屏障，保留真实FindByID、UpdateProfile与SQL条件，不用仓储桩替代判断。流程为：

1. 合成目标用户初始status=1、must_change_password=false、version=1，正常AuthService.Login取得旧AT。
2. 启动真实UpdateProfile；完成旧读后、实际UPDATE前暂停。
3. 真实管理员UserService.UpdateStatus或ResetPassword成功提交，核DB安全字段已变，version仍1；管理员使用合成superadmin角色账号。
4. 放行旧资料UPDATE，真实DB version变2，资料写成功；再用实际AuthService.Login及真实JWT中间件的受保护HTTP `/probe` 验证后果。

**禁用场景8/8**：管理员后status=0，旧资料写后status=1；原口令重新登录成功，新AT的受保护HTTP为200。**重置场景8/8**：管理员后must_change_password=true，旧资料写后false；使用重置后的新口令登录返回false，新AT直接200而非强改限制403。两个问题都不是仅看数据库字段后推测可登录。

六个顺序/版本控制均正常：每场景分别验证“管理员先完成再读取资料”“资料先完成再管理员”“先有另一资料写推进version，再管理员与旧资料写交错”。前两者安全状态保留；第三者旧资料写返回10006 CAS冲突。六个控制各执行一次，不冒充八轮控制全覆盖。

**所有旧AT仍拒绝，22/22**：16个交错场景为401，禁用控制为403，重置控制为401；R01纪元守卫仍有效。新问题是DB安全状态被另一写入路径覆盖后，后续新登录获得新纪元令牌。HTTP探针仅验证JWT/强改闸门，不把它扩大成全业务L1/L2/L3遍历。

## 修复建议与承接边界

建议将资料写限定为资料字段，彻底移除通用Update对status和强改标记的旧值回写；如需返回User，应回填当前数据库安全字段。保留现有资料version CAS、管理员状态/密码专用入口及全端吊销，不以“只让安全写增version”代替字段所有权约束。

独立修复分支codex/fix-user-security-state-consistency基于b306eed；修复验证须覆盖个人/管理员两资料调用方、禁用及强改保持、正常资料和状态/密码更新、旧AT拒绝与后续新登录，逐项移除修复后稳定失败。代码提交及固定门禁结果到位后另补本报告，当前不标代码层关单。

## 证据、环境与未覆盖项

证据仅本地忽略 `.codex/audits/ECO-20261104/data-coverage/b14/`：完整固定source-manifest、81迁移记录、main.go实际服务/PG/Redis探针、probe.raw.log的22条结构记录、run-result.json及cleanup.json。root已核探针和逐条结果；原两类各八次复现、六个控制、旧AT22次拒绝均有记录，不以描述替代日志。探针Go运行正常退出不表示产品通过，其明确输出16/16违规；原样证据为被测结果断言。

首轮PG临时初始化服务器曾使健康探针过早成功，迁移未连接最终数据库；已保留失败日志并清理两只容器。正式轮改用TCP+SELECT1核最终库就绪；两轮四只资源均按精确ID+owner清理、inspect不存在。没有把初始化失败计作产品缺陷。

该证明是实际服务写入和真实登录/JWT HTTP探针，不是完整HTTP资料请求并发压测。未来完整联合integration、acceptance、R01十三步及本批真实回归仍待执行；当前主线/开发原版/测试部署均未因此自动修复。全面审计继续，控制台和会话台账同步P0开放状态。


## 修复提交与独立复验续记

修复提交 **`f917c64242e08fdc59bd29179602148a9e8d00fd`**，分支 `codex/fix-user-security-state-consistency`，基于b306eed。实际挂点为 `UserRepo.Update`：资料SQL移除status和must_change_password写列，RETURNING回填它们的当前持久值；保留version CAS与既有10006映射。两个生产调用方 `UserService.Update` / `UpdateProfile` 共用此修复，管理员专用禁用/重置密码以及正常自主改密仍走原入口。根因修复没有新增全局锁、版本字段或依赖，也没有把密码hash列加入资料写。

永久回归 `TestUserSecurityState_ProfileCannotUndoAdmin` 使用原生service TestMain的PG及独立真实Redis，实际管理员安全写、两个资料入口、AuthService登录与JWT HTTP链。两资料入口×两类安全写×8轮，共**32场通过**；验证安全状态保持、返回对象与DB一致、正常资料更新、专用恢复/改密、新令牌受限或正常可用、旧AT继续拒绝及旧资料CAS冲突。固定fixture在-race下通过；不是只核SQL或mock仓储。

在最终同一fixture上，恢复原SQL、只恢复status回写、只恢复强改标记回写、仅移除安全字段RETURNING，分别八轮稳定失败。原SQL及三个变异断言均命中目标不变量；早期fixture的超时清理边界经同行指出后用sync.Once关闭release channel、有限context及缓冲结果通道修正，最终绿与变异全部重跑，未沿用旧收据。lint、完整单元及定向race通过；全仓integration、cover与acceptance尚待联合冻结轮，**不据此最终关闭P0**。

root已独立核生产SQL、191行永久回归、交接收据与测试边界，并按显式路径承接为联合提交 `ba3ecb6dc643e529813198af97c8943245e7c500`。原生回归未改TestMain/迁移，联合候选尚未替换当前test app7389或dev原版。来源与联合提交均未合main；修复分支未推送。

固定交接 `data-coverage/b14/repair-result.json` 列完整5文件hash、验证与清理；本批原探针四只容器已清，后续53只testcontainers逐一终止，owner标签残留0。证据留在忽略目录，报告及现行设计/控制台纳管。


## 原生TestMain联合专项续记

root固定联合ba3ecb6的709跟踪文件逐一gitblob相等，未使用DSN装配shim或overlay。三包race/count8命令中，service的B14八轮共32场通过（包358.441秒），handler的F19两个永久目标各八轮通过（包7.560秒）；ticket包在创建PG前因Testcontainers reaper状态created失败，未进入业务断言，故整条命令exit1，不记全绿。原日志/result完整保留，不能把包装脚本正常退出当Go测试通过。

保持同一固定源码，仅单独重跑ticket包原生TestMain：F15/F17/F18目标race/count8通过，12.731秒；没有修改测试或放宽启动预算，原reaper已自行清除。两轮原有运行容器无移除/重启，源码都干净。证据 `evidence/ECO-20261104-joint/ba3ecb6-native-targets/{result.json,ticket-retry/result.json}`。该续记补齐原生装配证明，仍非最终组合全量integration/cover/acceptance，也不因一次重跑绿断言已修复测试初始化时序风险。


## 最终固定联合与运行承接

固定联合 **`dc3d540a48e67bf56244efdc0f71cf32cbcb5e80`** 的719份跟踪文件逐一gitblob相等、门禁后源码干净：lint3.718秒、unit10.746秒、原生全量integration/race（原Makefile -p1）558.571秒、cover11.547秒通过；默认build标签总覆盖20.4%，不冒报相对增幅。原样四档124+68+26+35=253 PASS/0 FAIL/0 SKIP，81对fresh往返零漂移，R01真实API13步/B09成员15步全过；独占验收栈已清，原运行容器集合/ID/启动时间完全相同。

B14-F01/P0在上述完整联合范围代码复验关闭，并已test承接；原禁用/强改覆写、两个资料入口/返回状态、旧AT及正常安全写均保留专门回归。 test现已承接同源app dc3/AL60e/TR eb3/UI c9，三后端二进制SHA、四imageID/源码标签与UI HTTP index核对相同；仅四应用更新，九个基础/开发容器保持，三PG备份目录已验证但未重复恢复演练。未合main或发布生产。前端最终41和Chrome后记另看B08/B13，不将后端门禁扩大成全站认证。

完整证据为本地忽略 `evidence/ECO-20261104-joint/dc3d540/{manifest.json,core-results.json,resume-installed-wire/core-results.json,acceptance/result.json,image-result.json,test-deployment/result.json}`。原make wire失败属B15原有工具sum缺口，CI同款固定v0.7.0 CLI生成及零漂移通过；原生ba3的ticket reaper初始化失败和原始运行器准备失败均保留，不以续跑绿覆盖原失败。
