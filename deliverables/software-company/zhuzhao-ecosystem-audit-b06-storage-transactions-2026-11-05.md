# 生态审计 B06：存储并发与事务冲突（2026-11-05）

> **最终后端联合复验（2026-11-06）**：本增量F15/F20在上述完整联合范围代码复验关闭；F01–F04既有证据不外推，F20补齐资料并发写的独立链。 固定源dc3d540完整门禁及test承接已过，下文早先“待验/未部署”为原时点；未合main。

## 范围与基线

主仓原业务基线为 `5c03581d5999139a3e8e5ead5972c026868aa181`。本报告仅收录 S5 存储与 BK11 冲突支路的已证实发现，不代表主仓全部模块、135 份设计文档或 102 项挂账已经审完。设计输入是 phase5/05、S6-S5-debt-ledger 的 B-11/G-3、13 号 BK11 复盘与 phase6/05；未来 dbtx、outbox 等规划不作为当前缺陷重复计数。

修复分列为 `codex/fix-storage-audit` 的 `d07f560c19e311d8c5255a8f7eb5867cc55f488a`、`codex/fix-ticket-conflict-pool` 的 `475b7ac896c94f9c7ef29d27236feff1eb1269e9`。两项均已完成固定提交复验；均未合入 main，常驻 app 尚未升级。

静态/动态双轨：独立 git archive、真实 PostgreSQL 15、确定性锁交错；首轮对象删除端口桩后，又用真实 RustFS 1.0.1 与实际 MinioClient 补验。所有账号/对象均为自有临时资源，不使用 E000001。过程材料及原始日志仅本地 `.codex/audits/ECO-20261104/evidence/ECO-20261104-b06/`，不纳入 Git/Docker。

## 分批发现

### B06-F01 / P0：头像绑定与 enforce GC 交错可误删成功绑定的实物

锚点（原基线）：[`service/storage/service.go` 的 `BindAvatar`](https://github.com/tracerbiubiubiu/zhuzhao/blob/5c03581d5999139a3e8e5ead5972c026868aa181/internal/service/storage/service.go#L297)、[`storage_repo.go` 的 `MarkAvatarOrphanDeleted`](https://github.com/tracerbiubiubiu/zhuzhao/blob/5c03581d5999139a3e8e5ead5972c026868aa181/internal/repository/storage_repo.go#L452)。`SetObjectOwner`、用户头像回写和弃旧分别提交；持用户行锁暂停真实 BindAvatar 时，GC 可把已确认、超期、尚未回写引用的头像标 deleted 并物理删除。释放行锁后 BindAvatar 仍成功。

初核 PG/端口桩串行 5/5 复现；补验真实 RustFS 得到 `bind_error=<nil> avatar_matches=true status=deleted physical_stat_error=storage: object not found`。依项目“数据损坏”口径由过程稿的 P1 调整为 P0。本项须显式 `avatar_gc_observe=false`，默认观察模式不物理清理，不能把它说成默认即可触发。

与既有 B-11 的非事务绑定同源；新增 reaper 使过去“仅可能泄漏、无害”的论证失效，更新原债而非另造重复债。换头像的旧键退役/重绑另有同根 ABA 窗口，已并入同一修复与回归。

### B06-F02 / P1：满额普通附件误拒绝富文本图片确认

锚点：[`storage_repo.go` 的 `ConfirmAndBind`](https://github.com/tracerbiubiubiu/zhuzhao/blob/5c03581d5999139a3e8e5ead5972c026868aa181/internal/repository/storage_repo.go#L205)。设计规定 richtext 不占普通附件额度，但该路径无条件执行 `n+1 > maxCount`。独立 max=1 实验：普通附件确认成功，随后 ticket_richtext 确认被附件数上限拒绝，5/5 复现；默认 max=20 满额时同构。

修复在对象锁内读取 purpose，仅普通附件计硬上限，保留下一普通附件拒绝的对照，不通过提高配置上限掩盖问题。

### B06-F03 / P1：工单状态冲突定性再次取池，满池事务相互等待

锚点：[`ticket_repo.go` 的 `resolveClosedOrMissing`、`UpdateStatusTx`、`UpdateAssignedToTx`](https://github.com/tracerbiubiubiu/zhuzhao/blob/5c03581d5999139a3e8e5ead5972c026868aa181/internal/repository/ticket_repo.go#L378)。两个事务内零行写入口调用的定性 helper 使用 `r.db.QueryRow`，丢失事务连接；N 个事务占满 N 个池连接后都申请第二连接，直到 deadline。生产风险取决于 **MaxConns，N≥池容量**，与核数无直接关系。

原探针串行 5/5 为 `pool_acquired=4 pool_idle=0`，应返回 90004 而实际超时。新增回归在四事务全部准入后才启动查询，保持事务不释放：两个入口各 8/8 稳定红；未提交的自身 closed 状态还会被池读成旧 open，错误返回并发修改。missing/冲突/正常写语义设正向对照。

本项就是 13 号 §8 已登记的 2026-11-01 P2-1，本轮定性为已动态证实的 P1，不重复开债。局部修复复用现有 `rowExec`；dbtx 传播器/请求 deadline/其他域推广依旧是独立规划，不能一并销账。

### B06-F04 / P0：默认工单 confirmed GC 等待落绑后仍用旧引用快照

锚点：[`storage_repo.go` 的 `MarkDeleted`](https://github.com/tracerbiubiubiu/zhuzhao/blob/5c03581d5999139a3e8e5ead5972c026868aa181/internal/repository/storage_repo.go#L468)、`GCJob.Handle` 第三段。BindCreateAttachments 事务已锁对象并插关联、尚未提交时，GC 的单条 UPDATE 先取得“无引用”语句快照，再等对象锁；绑定提交后仍用旧反连接快照退役对象并删实物。此段默认执行，不受 avatar observe 保护。

首轮 PG/端口桩连续 3/3：`binding_committed=true status=deleted ticket_refs=1 removed=[key]`。真实 RustFS 补验同样得到 `binding_committed=true status=deleted refs=1 physical_stat_error=storage: object not found`，确认数据丢失，按项目口径由过程稿 P1 升为 P0。它不是 F01 的同一路径：对象域、默认开关和落绑入口不同，分别保留发现与回归。

## 修复建议、挂点与状态

`d07f560` 将头像用户/对象归属、新引用和旧键退役放同一事务；deleted 为终态，禁止重绑退役键。avatar GC 与 ticket confirmed GC 均先取对象行锁，再用后一条 SQL 的新快照判断引用；GC 赢则绑定拒绝，绑定赢则 GC 让位。普通附件和 richtext 额度按 purpose 区分。无新增路由、权限码、配置项、依赖或迁移。

**F01/F02/F04 在 d07f560 代码层复验关闭**；B-11/G-3 台账补分支证据与主线待承接状态。固定提交独立验证使用原消费 utils v0.4.2，不把本批单独修复误当认证问题也已修；与认证/过期成员的联合构建另做。

`475b7ac` 的 F03 修复已提交，候选专项八轮、lint/单元及移除复用后三目标各 8/8 红成立；固定归档的 lint/单元/专项八轮已过，原样 acceptance 253/0/0、81 对往返已过，但首轮补充 API 探针因本地遗漏 hash_fixture.go 未执行，wrapper exit 1。该失败属证据运行器准备错误，保留原日志，不冒充全绿；补齐辅助文件、保持归档源码不变后，完整集成 **502.17 秒 / exit 0**、覆盖率、原样四档 **253/0/0** 与 **81 对往返**全部通过；真实撤销13步和成员生命周期15步也通过，**F03 在 475b7ac 代码层复验关闭**。运行容器差集仅移除并行测试清理器，常驻实例无新增或重启。该最终成功轮记录于 `pool-fix/postcommit-attempt2/`，没有覆盖首轮失败。

## 验证证据

- storage 固定提交原样 lint/单元通过；首次完整集成被会话中断，无退出码，不记通过；确认无遗留进程后重跑原样完整集成 **454.98 秒 / exit 0**，覆盖率门禁通过。源码逐文件 SHA 与原归档相同，工作树无改动。
- 独立 PG/Redis/RustFS + 两个冻结兄弟应用镜像运行原样 `make acceptance`：**253 PASS / 0 FAIL / 0 SKIP**，**81 对迁移往返零漂移**。自身资源清理为 0，其他运行容器 ID/启动时间前后相同。
- 五项确定性交错/额度回归 `-race -count=8` 全绿；恢复旧绑定/额度三目标合计 **24/24 红**，分别恢复 avatar、ticket 单条 GC 快照各 **8/8 红**。失败断言与目标不变量对应，不是编译或初始化故障。
- 真实 RustFS 探针仅加入两个独立归档：原基线两项均红，d07f560 两项均绿。修复后工单为 `confirmed/refs=1/Stat成功`；GC 赢头像时返回“文件不存在”，用户头像没有悬挂引用。不是浏览器 E2E，也不是完整 HTTP 上传链，本实验通过预签 PUT 放置实物、SQL 设置超期夹具、调用真实 service/GC/PG/S3 路径。
- 候选 storage 最初验收曾有 MSG8 返回 502；未保留足以归因的 taskrunner 退出现场，不能断言已证实其 panic。之后未改源码，增加兄弟服务 readyz 等待与进程监控的独立轮通过，taskrunner 同 PID、无 OOM。固定提交轮亦通过，原失败保留。

本地证据：`storage-fix/fixed-commit/{source-manifest,fixed-verification-result,resumed-results,full-acceptance-result,interrupted-attempt}.json`、`storage-fix/physical/{baseline.log,fixed.log,results.json}`、`storage-fix/mutation*-results.json`；F03 为 `pool-fix/`。完整原始过程报告仍在本地，正式报告只保留必要事实、锚点、结论与边界。

所有新增代码未合 main。分支代码关单不等于当前系统已部署修复；整体审计、未读设计、全角色路由/视觉体验和其他兄弟仓能力继续核验。


## B06-F15 / P1：编辑校验的字典与MIME分支在事务内再取池连接

本增量固定基线 `7389ed79bdf409e466542992d9f1befa0c1a8529`。锚点：[type_admin.go 的 expandDictOptions / validateCustomData](https://github.com/tracerbiubiubiu/zhuzhao/blob/7389ed79bdf409e466542992d9f1befa0c1a8529/internal/service/ticket/type_admin.go#L749)、[Service.Update 的 Begin / LockCustomDataTx / validateCustomData](https://github.com/tracerbiubiubiu/zhuzhao/blob/7389ed79bdf409e466542992d9f1befa0c1a8529/internal/service/ticket/service.go#L687)。schema/formula虽已传tx，字典展开仍用s.db.Query，file白名单仍用池QueryRow；一条调用链局部透传不能覆盖整个事务窗口。合法编辑在池容量1或多事务满池时超时，回滚后数据不变、事件0；本轮没有数据覆盖/权限旁路证据，按功能缺陷P1，与BK11同族。

原Service.Update/真实PG串行八轮，dict/file×单连接/满池四目标共32红，空闲双连接及不走对应分支四控制共32绿；所有连接预热，满池必须确认两个真实LockCustomDataTx屏障准入后才放行，取消取池计数>0才计目标失败。独立修复 **`4b0b788a1d7105a17439543117fbf19460f88f8d`**，分支codex/fix-ticket-validation-tx：私有expandDictOptions接RowQuerier，字典和MIME用同一个q；MIME用q.Query+CollectRows关闭结果集。Create事务前仍传池、Update持锁窗口传tx；schema/字典/MIME/formula同类链已核，无API、依赖、权限、配置或迁移变更。

固定同探针64场全绿；只恢复字典池读或MIME池读，各自八轮稳定命中对应两目标失败，其余六场景通过。永久回归TestValidationReadsReuseTransaction及四类既有dict/file/并发patch回归-race八轮通过；永久回归对两种独立变异也各八红。lint2.007秒、完整单元8.428秒通过。独占PG装配shim仅在忽略归档中用于定向测试，生产与提交三源码逐字一致，未修改原迁移/测试业务断言；原生全仓TestMain集成与acceptance仍由联合门禁承接，不能把专项绿写成全验收完成。

证据 `evidence/ECO-20261104-b06/field-validation-pool/` 的original/fixed/mutation/tracked-*-resume-results.json、final-source-equivalence.json及root-final-cleanup.json；前序夹具42703、错误cwd迁移空集和误选helper变异均单独保留、不计产品红绿。root已将必要差异承接为联合 `b4435022d00ee789b9a62f74036824bd7d3791b7`，当前候选含此修复但尚未部署。两代自有PG均按精确ID/owner确认闲置后清除，容器与匿名卷均不存在；共享dev/test/其他审计资源保留。来源与联合均未合main。


## B06-F20 / P1：资料旧快照覆盖成功换绑后的头像引用

本增量固定基线 `ba3ecb6dc643e529813198af97c8943245e7c500`，已含B14安全字段白名单；与F01的GC锁协议是不同写入链，不撤回F01原限定范围的复验结论。锚点：[StorageRepo.BindAvatar](/Users/bujibuji/.codex/tmp/zhuzhao-audit/ECO-20261104-fix/integration-next/internal/repository/storage_avatar.go:15)及[UserRepo.Update](/Users/bujibuji/.codex/tmp/zhuzhao-audit/ECO-20261104-fix/integration-next/internal/repository/user_repo.go:260)，修复源码hash如下。原BindAvatar换键不推进users.version，通用资料Update仍回写旧Avatar：资料真实读取后暂停，真实换绑完成并退役/物理删除旧键，再放行资料SQL，资料写成功但users.avatar恢复成deleted旧键。

个人/管理员两个实际资料入口各八轮复现，七类顺序、同键、URL手填/拒绝等控制共56场正常。独立真实RustFS经预签PUT、Stat、BindAvatar与Remove路径，两入口原各八红：旧键Stat notfound、新键仍存在，资料写nil却回指旧键。按功能失效P1；没有声称新对象本体已丢失，也不扩大成新的匿名攻击或P0。

修复 **`1550bbf9cf05eda89454021484069ce3e277cc78`**，codex/fix-avatar-profile-consistency：仅实际换键时在既有用户行锁事务内推进version及updated_at，使旧个人/管理资料写返回既有409/10006；同键重绑不推进代际，URL手填、对象校验及弃旧/GC锁序不变。无API签名、权限、配置、依赖或迁移变化。已承接联合 **`6ad4cda60b321ee9420df20415e5d5c422b8a8f3`**，来源和联合均未合main或部署。

永久原生service TestMain回归 `TestUserAvatarProfileCAS` 九场×八轮race共72场绿；真实RustFS两入口各八绿、陈旧资料409且引用保持新键。只移除version、错误地让同键也增代、只去updated_at，三类分别两入口×八轮稳定红；同键绑定后、资料放行前独立核version/时间，避免后续资料写掩盖幂等性缺陷。lint/unit/cover、既有头像与三项GC race回归通过，同行复核无阻断。原生全仓integration/acceptance仍待最终联合轮，未提前关单。

证据 `avatar-profile-consistency/{repair-result.json,report-draft.md}` 含六文件/十六日志SHA、原/固定/变异及物理夹具边界；21只自有容器（含RustFS）均inspect不存在，owner残留0。静读覆盖此前固定b306的156生产Go文件与14最新差集有独立索引；不把它自动外推到之后的新组合。收件人快照时刻仅P2边界候选，尚无明确提交瞬时合同及动态泄露证明，未混成另一已确认P1。


## 最终固定联合与运行承接

固定联合 **`dc3d540a48e67bf56244efdc0f71cf32cbcb5e80`** 的719份跟踪文件逐一gitblob相等、门禁后源码干净：lint3.718秒、unit10.746秒、原生全量integration/race（原Makefile -p1）558.571秒、cover11.547秒通过；默认build标签总覆盖20.4%，不冒报相对增幅。原样四档124+68+26+35=253 PASS/0 FAIL/0 SKIP，81对fresh往返零漂移，R01真实API13步/B09成员15步全过；独占验收栈已清，原运行容器集合/ID/启动时间完全相同。

本增量F15/F20在上述完整联合范围代码复验关闭；F01–F04既有证据不外推，F20补齐资料并发写的独立链。 test现已承接同源app dc3/AL60e/TR eb3/UI c9，三后端二进制SHA、四imageID/源码标签与UI HTTP index核对相同；仅四应用更新，九个基础/开发容器保持，三PG备份目录已验证但未重复恢复演练。未合main或发布生产。前端最终41和Chrome后记另看B08/B13，不将后端门禁扩大成全站认证。

完整证据为本地忽略 `evidence/ECO-20261104-joint/dc3d540/{manifest.json,core-results.json,resume-installed-wire/core-results.json,acceptance/result.json,image-result.json,test-deployment/result.json}`。原make wire失败属B15原有工具sum缺口，CI同款固定v0.7.0 CLI生成及零漂移通过；原生ba3的ticket reaper初始化失败和原始运行器准备失败均保留，不以续跑绿覆盖原失败。
