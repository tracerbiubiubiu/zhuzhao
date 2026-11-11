# Phase6前五仓审查与全项目重规划

日期：2026-11-11。性质：固定实现基线上的规划审计、范围重划与正式文档落档。后续功能仍未实施；本轮没有改源码、依赖、迁移、业务配置或远端仓库设置，没有发布生产。原main工作树保持只读，规划修改在五仓各自的`codex/pre-phase6-replanning`本地分支；共66个文档/受控清单路径，未推送、未合main，远端CI没有被冒称验证本轮新文档。

## 1. 结论

平台目标收敛为**数据聚合与安全全生命周期管控**。已有SOC/SIEM、SOAR、CMDB、TI、企业Agent/知识平台提供专业能力，本地只建设责任、处置、工单、受控调用与证据等必要增量。AI提前以受权查询/草案/计划接入，外部Agent与平台特有领域Agent双轨。通用DAG、自建知识/模型基础设施、通用市场及强制SOAR迁回退出默认范围。

主要问题是：工程守卫已有但合入与隔离未强制闭合；跨仓成功/调度/结果合同存在缺口；旧阶段多批叠加、估算和触发登记漂移；前端有规范但未形成全站任务/状态/视觉统一验收。保留可用底座，按依赖补可信合同，优先接已有专业供给。

## 2. 基线与审查范围

| 仓库 | 固定main提交 |
|---|---|
| zhuzhao | `5c0e248deb03c1f8477b0cac052597113f599f03` |
| zhuzhao-ui | `73a18d39f695f093992d3447027a57c442e50686` |
| taskrunner | `340c7a63a7b8ab94353cc792e3fa2bf0b7dba9ba` |
| activelist | `93ecbcb47eff4e9a6e11da2648f2a0605dc2edb0` |
| zhuzhao-utils | `b027089d544b192a6472b6c9c3c3b596b7e66526` |

本轮只读GitHub API返回main与上述准确SHA一致。270份基线Markdown库存见[范围快照](../planning-document-scope.json)；库存/hash/行数不代表每篇全文语义审计。当前产品/阶段/proposal/模块合同重点深读，历史阶段和报告按固定时点参考并复核本轮承接，源码按装配/关键调用与失败边界定向核验。

工程专项覆盖五仓CI/守卫/GitHub设置、TR受理/cron/worker/回调/取消恢复、AL类型/CRUD/导入/并发约束和utils协议；产品专项核对主仓领域/授权/事务/事件、未来阶段及专业供给/AI设计；前端覆盖27个路由视图文件、9组共享组件、4类未来页面。前端逐页现状与计划见[UI清单](../../../../zhuzhao-ui/docs/design/frontend-pages.json)，设计见[全站方案](../../../../zhuzhao-ui/docs/design/frontend-redesign.md)。这些不是27页浏览器视觉验收证明。

历史输入为`project-comparison-audit`和`zhuzhao-phase6-9-planning-review-2026-11-07`。30项能力与81条决策记录逐项进入来源映射，保留“旧报告建议不等于所有者原有决定”的边界；本轮允许改变旧设定，不照抄旧问题为当前缺陷。没有上传内部资料或复制识别性公司数据。

## 3. 约束、取舍与新权威链

所有者明确：大量mock保留；模型只调用已有API、不考虑模型部署，内网只内部模型；平台专业供给均已有，避免大而全和重复造轮子。对平台特有自建Agent的问题，本轮推荐领域Agent与外部Agent双轨，限定最小受控host。

当前权威链为[产品定义](../../vision.md)→[决策§33](../../design/design-decisions.md#33-phase5合并后的供给优先重规划2026-11-11)→[路线](../../roadmap.md)→阶段与模块合同→[总账](../planning-backlog.md)→[控制台](../11-project-control.md)。[平台演进](../../design/platform-evolution.md)、[AI设计](../../design/ai-capabilities.md)、[工程治理](../../design/engineering-governance.md)分别回答范围、接口与实施质量；各仓自己的docs维护其合同，不复制多套估算。

旧整包引擎/全部增强挡R1改为真正的不变量先行；R01持久意图前置到P6.1；D15风险通知最小合同先于首条风险通知，D16完整自动路由P6.3承接。旧“升级链并入”不是已实现关闭。首页正文/验收已改权限门控，自动路由明确不跳业务validator。

## 4. 工程发现与去向

以下是固定快照事实与审查结论。远端配置与测试记录属时点证据，不表示改造已经实施。

### ENG-01 并行写入改为独立 worktree，单一集成者合流

事实：主仓 `docs/standards.md` §12-10、`scripts/zz-commit.sh`、`.githooks/pre-commit`。当前预提交只提示；路径工具不知道一份文件内部哪些行属于哪个会话。同文件混写时，显式路径提交仍会带入他人内容。包装器的暂存集校验还是字符串子串匹配，且有空格路径限制；它不是文件归属机制。

当前处置：G01，详细合同见工程治理和该仓平台集成文档；估算只计主总账一次。

### ENG-02 把 CI 绿灯提升为 main 合入的服务器约束

事实：五仓GitHub API返回main protected=false、required checks为空、rulesets为空，脱敏收据保留在本轮本地证据；固定基线 `docs/standards.md` §12-7 允许小批直推 main。工作流存在不等于推送之前被强制执行。

当前处置：G02，详细合同见工程治理和该仓平台集成文档；估算只计主总账一次。

### ENG-03 UI CI 存在不可信分支名直接生成 shell 的风险

事实：UI `.github/workflows/ci.yml` 的 `Resolve sibling zhuzhao ref` 步骤，`BR="${{ github.head_ref || github.ref_name }}"` 将 PR 可控 head_ref 直接插入脚本文本。[GitHub 官方](https://docs.github.com/en/actions/concepts/security/script-injections)明确分支名是不可信输入，并建议通过中间 env 变量传入脚本。

当前处置：G02，详细合同见工程治理和该仓平台集成文档；估算只计主总账一次。

### ENG-04 安全扫描从阶段证据转成持续治理

事实：主仓 `.github/workflows/go-security.yml` 只在 phase5/codex/phase5-audit 特定路径 push 及 dispatch 触发，兄弟仓扫描固定历史 revision。不能把它当作当前五仓 main 的持续安全门禁。GitHub 依赖告警/自动安全更新关闭；已有 secret scanning/push protection 保留。

当前处置：G03，详细合同见工程治理和该仓平台集成文档；估算只计主总账一次。

### ENG-05 用固定组合清单替代跨仓“同名分支或 main”隐式配对

事实：UI CI 的 sibling resolve 取同名主仓分支，缺失回落 main；`scripts/codegen.sh` 从相邻主仓 Swagger 生成类型，但 CI 不再生核零 diff。UI AGENTS 也说明生成类型仅部分 API 接线。主仓生产/test compose 依赖同级 checkout，跨仓 build context 并不固定消费者组合。

当前处置：G03，详细合同见工程治理和该仓平台集成文档；估算只计主总账一次。

### ENG-06 大量 mock 与真实验证分层，先补测试证据口径

事实：UI `vitest.config.ts` 只有 node 环境和纯逻辑测试，未使用 DOM 组件环境；固定main基线的`docs/guides/testing.md`与AGENTS排除中间mock集成层；本轮文档已取消此禁令，实际新增mock工具仍按计划实施。Playwright 默认串行共享 dev、retries=1、reuseExistingServer=true。主仓 `Makefile.test-cover` 仅默认 tag 的 `./internal/...`，不包含 integration tag。历史控制台约 20.3% 是该口径，不能当全系统综合覆盖率，也不能当本轮重新实测结果。

当前处置：G04，详细合同见工程治理和该仓平台集成文档；估算只计主总账一次。

### ENG-07 Fuzz、并发性质与容量测试按风险落地

事实：固定快照主仓353个Go文件/196个测试文件/538个Test前缀函数（含7个TestMain；扣除后531个静态命名测试），其中115文件带integration tag；TR41/20/94，AL42/21/81，utils36/17/71。此计数不含子测试、不是测试通过数。四Go仓均未找到顶层 `Fuzz*` 或 `Benchmark*` 函数；主仓 Makefile 虽有 benchmark 入口，但不能据此声称已有性能基线。结构化明细见 inventory。

当前处置：G04，详细合同见工程治理和该仓平台集成文档；估算只计主总账一次。

### ENG-08 供应链与发布从“本机构建+latest”升级为可追溯组合

事实：五仓 Actions 多用 tag（checkout@v4/setup-go@v5等）；普通CI未显式声明 workflow permissions，但GitHub默认read已核实。主仓/TR/AL Docker基础镜像是可变tag，应用compose用latest；UI `.nvmrc=20.19.0`、Docker `node:20-alpine`，CI/容器不绑定同一准确补丁版本。仓内未见 Dependabot/Renovate配置或镜像发布/来源证明 workflow。现有Go sum、pnpm lock与固定生成工具应继续保留。不能从未检查到生产签名链推定生产一定没有外部机制。

当前处置：G05，详细合同见工程治理和该仓平台集成文档；估算只计主总账一次。

### ENG-09 先修跨仓完成语义，再承接 AI/流程自动化

事实：主仓 `service/jobs_callback.go` 的 `JobsCallbackService.Execute` 对未取得执行权的running行返回CallbackIdempotent；`handler/jobs_handler.go.Callback` 回2xx。TR `callback.Client.Do` 不读业务响应体，对所有2xx按成功；`worker.handleCallback` 再写succeeded。故“另一份仍running”的请求可使执行事实提前成功，execution_id只保护执行库写入身份，不能证明业务完成。

当前处置：C03，详细合同见工程治理和该仓平台集成文档；估算只计主总账一次。

### ENG-10 自然语言周期任务前补“计划发生次”幂等与调度政策

事实：TR `service/cron.go.Loop.FireDue` 查到期jobs、生成随机新task_id、Submit后独立推进next_run；`repository/jobs.go.ListDueCronJobs` 无领取锁/CAS；`UpdateJobNextRun` 按job_id无条件写。代码明确记录推进失败会以下个新task_id重复触发，多副本cron也无生成防重。任务ID终身唯一防不了“同一计划发生次生成多个任务”。

当前处置：C03，详细合同见工程治理和该仓平台集成文档；估算只计主总账一次。

### ENG-11 任务身份与可见性不能继续把标签当授权

事实：TR `handler/server.go.New` 只校验服务AK；任务dept/submitted_by是过滤与归因，不做用户行授权。主仓 `TaskrunnerHandler.ListRuns` 只把submitted_by=me替换为actor，其余查询可透传；`TaskrunnerService` 管理方法也是薄代理。`docs/design/zhuzhao-integration.md` 明确共享作业队列。该现态是既有取舍，不应伪报为遗漏的既有实现。

当前处置：I02，详细合同见工程治理和该仓平台集成文档；估算只计主总账一次。

### ENG-12 恢复边界、结果与取消必须配套，不能只加 result 列

事实：TR `ReclaimLoop` 仅自动修复queued=0+pending；queued=1+pending只告警，running不自动重投。`Deps.handleCallback` 在MarkRunning缺行/DB临时失败时仍回调；Finish失败除身份被拒外主要记日志，外部动作可能已成功但执行库仍旧态。`TaskService.CancelTask` 仅取消未开始任务；RetryTask依赖Redis archived，键已清理无法原ID重试。`notify.Notifier` 尽力异步、不持久；探针不是业务积压告警。`docs/ops/backup.md` 的日级RPO是现有选择，非新自动化目标已满足。

当前处置：I03，详细合同见工程治理和该仓平台集成文档；估算只计主总账一次。

### ENG-13 taskrunner 保留期与敏感输入制定固定实施批

事实：TR `docs/design/taskrunner.md`、`internal/config/config.go.Config.Log`、job_runs.enqueue_payload及params；日志/数据库可能持有完整参数，敏感参数被要求在队列前禁止，但没有通用凭据引用/字段脱敏治理。job_runs清理仍等M4；它又是task_id终身去重事实，不能按日期随手删。

当前处置：R05，详细合同见工程治理和该仓平台集成文档；估算只计主总账一次。

### ENG-14 unique_fields 与破坏性schema演进须从限定语升级到可执行合同

事实：AL `DataService.checkUniqueDup` 为SELECT后写，`repository.FindActiveDupID` 无硬唯一索引；`CreateTableIfNotExists` 只建主键/分页索引；并发同键可双写。导入不做业务键查重。`TypeService.Evolve` 替换当前schema，不扫描重写旧行；数据行不带schema版本，旧定义已读后的并发写窗口是现有取舍。

当前处置：R06，详细合同见工程治理和该仓平台集成文档；估算只计主总账一次。

### ENG-15 让 AL 查询进入统一权限，而非直接给智能体一个内部服务SK

事实：AL `handler.New` 业务组只有服务签名；`DataService.List` 支持q/单字段等值/keyset；服务不做用户/行策略。当前普通CRUD验签16MiB、浏览器网关body1MiB、大导入直连运维；这些边界见 `docs/api/contract.md`/`docs/design/overview.md`。`meta.Field.Sensitive` 是存储标记，不启用日志脱敏。

当前处置：I02，详细合同见工程治理和该仓平台集成文档；估算只计主总账一次。

### ENG-16 全量替换导入是独立高风险动作，不能当普通写工具

事实：AL `DataService.Import` 锁表→删除全部（含软删）→流式重灌→校序列；Export输出完整裸数组、含软删。流中途失败只能截断响应，原文件不是带完整性承诺的导出包；单事务保数据原子但不等于业务“追加导入”。

当前处置：R06，详细合同见工程治理和该仓平台集成文档；估算只计主总账一次。

### ENG-17 补 AL 运维与技术信息暴露的小缺口

事实：AL `handler/server.go.New` 的免鉴权readyz在PG失败时返回`err.Error()`，与TR隐藏细节的做法不一致；`middleware.snapshotParams` 保存query+前4KiB body，`Sensitive`不脱敏；`App.Run` 丢弃Shutdown错误，配置说明承认日志目录没挂卷。没有metrics路由。已有备份/PITR模板不代表具体环境已启用或测过恢复。

当前处置：R06，详细合同见工程治理和该仓平台集成文档；估算只计主总账一次。

### ENG-18 utils 维持工具库，协议与版本要能独立演进

事实：utils根目录是Go公共包，业务表不在库内；`aksk/aksk.go` 明确query不入签、无nonce单次消费、operator/request-id入签。主仓/AL使用`v0.4.4-0...cd759a04`，TR仍`v0.4.2`；这不自动构成漏洞或必须升级，TR主要消费Sign。`docs/ops/releases.md` 当前说明v0.4.3已发、streaming等未有v0.4.4正式tag；旧候选状态与main合入状态需在新正式发布批核对，不能由本文虚构发布。

当前处置：R07，详细合同见工程治理和该仓平台集成文档；估算只计主总账一次。

### ENG-19 统一目录的适用规格与自动检查，不机械同构

事实：主仓/TR/AL已有cmd/configs/internal，TR/AL部署目录为deploy，主仓/UI为deployments；主仓/AL有显式迁移，TR迁移嵌在repository/store.go启动兼容逻辑，utils不需要service/migrations。主仓 `docs/documentation.md` 已规定每仓docs分类、入口、状态、代码符号引用与不造空目录。兄弟仓手工维护 `wire_gen.go`，并非主仓那种自动Wire再生；目录名相似不代表相同工具语义。

当前处置：G06，详细合同见工程治理和该仓平台集成文档；估算只计主总账一次。

### ENG-20 插拔按端口与能力包演进，服务拆分由证据决策

事实：主仓 `internal/pkg/jobs.Registry`、gateway.Registry、AI/DataFetcher等已有端口/注册表；jobs.Registry新增handler仍需发版，gateway.New装配后不可变，不是任意运行时热插拔。`docs/phase3/11-deployment-split.md` 明确APP_MODULES未实现，Wire不等于运行时可选Provider。TR/AL已是独立服务，不经同二进制模块启动路线。

当前处置：E07，详细合同见工程治理和该仓平台集成文档；估算只计主总账一次。

## 5. 前端发现与去向

FE-PRE-01–12的完整源码锚点、静态/待运行区别和方案位于[全站方案](../../../../zhuzhao-ui/docs/design/frontend-redesign.md)。前端专项初审为静态；随后在独立栈运行既有42项E2E作为现有实现回归。未复现用户特定账号的logo问题，也未建立全站视觉基线，不把静态条件链说成用户具体实例已经复现。

- FE-PRE-01：Logo→根→动态home与普通未绑home角色存在404链；现有测试预先补绑，不能覆盖该边界。U01先修。
- FE-PRE-02：首页已有，但固定工单请求/错误变0/无资格门控不足；U02冻结设计，U06交跨域首页。
- FE-PRE-03：创建位置、卡头和技术说明不一致；U02/U06统一层级与业务文案。
- FE-PRE-04：筛选折叠可能隐藏仍生效条件，缺少摘要和语义；U03承接。
- FE-PRE-05：批量标签是新增前后端能力，必须逐项授权/幂等/部分失败，不能只加多选框；U03。
- FE-PRE-06：情报历史是本机成功查询元数据，不含历史结果快照；先校准语义再比较左右主从等方案；U05。
- FE-PRE-07：字体/颜色/间距变量部分具备，页面词法计数不能证明实际低对比；U02样板、U07运行测量。
- FE-PRE-08：错误/空/陈旧/写成功刷新失败未形成统一合同；U06按页推广。
- FE-PRE-09：若干自定义可点击元素和拖拽缺键盘等价；U07固定批次验证。
- FE-PRE-10：复杂配置对话框与未保存策略需要重设；U04。
- FE-PRE-11：字典截断、人员搜索语义/竞态、Profile清空、树和AL导入风险分别排U06，不靠共用组件修改代替逐页验收。
- FE-PRE-12：未来AI页面是已有服务连接、领域Agent配置、受权会话/计划确认/结果；A01–A07逐切片，不造模型部署或通用Agent站。

## 6. 产品与架构发现

### PA-01 AI已有方向，缺完整产品合同

历史D8/D10/D17和S7主要为后置设计位；当前未实现LLMProvider/MCP/RAG/Agent。新的模型/配置/运行/工具/知识/skill合同见AI设计；A01–A07分批交付，研究候选不等于已选依赖。

### PA-02 目标、范围和估算漂移

Phase6已叠加多个批准/候选批，旧愿景和后增总周数分母不同。所有来源合并进一份工作账；专业平台供给优先，不能把差距清单整包批准。

### PA-03 前置需要按业务不变量拆开

发布快照、转换采集、资源护栏与可靠交接保留硬前置；高级画布/向导/报表/Office等分别排窗。本地FSM字段取人和自动规则仍有明确承接，范围收缩不等于遗漏本地关键能力。

### PA-04 可靠意图不能等最后阶段

进程内Hook、落库审计与执行fence不等价于跨仓可靠效果。R01在首个不可丢消费者之前交付，专业workflow/Agent各有唯一权威，重投与重执行分开。

### PA-05 MCP/Agent不能直接旁路HTTP授权

主仓路由L1不会因传入userID而自动执行。所有新入口显式复用操作资格与实时资源/域约束，首批工具少量审核，不直接生成任意HTTP/SQL工具。AL内部SK不能直接交Agent。

## 7. 研究采用与未采用

只使用公开一手文档研究通用模式，查询日期2026-11-11，未上传内部资料。来源与具体适用方式已就近写入AI、工程和前端设计：

- Git worktree用于索引/文件隔离；保留额外运行资源隔离，不声称worktree隔离所有Git公共状态。
- Go fuzz/漏洞治理、GitHub脚本注入/保护、Playwright mock用于持续检查和分层证据；不套全仓80%或为指标堆测试。
- MCP规范/Go SDK用于协议和客户端版本合同；Skills格式只作内容/版本引用，不赋权、不建市场。
- 成熟Agent SDK/有限运行组件、LangGraph暂停恢复、Temporal副作用与确定性模式作为有界比较；未选框架/未跑原型，不自动引入新语言或引擎。
- 企业知识API按来源ACL/引用/撤权合同接入；不重建RAG基础设施。
- Carbon/Fluent/WCAG/WAI用于筛选、表格、信息层级、焦点/缩放合同；不换组件库，不宣称已符合全部可访问性标准。

## 8. 全量挂账与执行排期

当前总账含**61工作包/494来源**。来源含81条历史阶段决策（有重复交叉引用）、30项比较、116行触发索引、26件工单设计、P5各组和本轮用户/审查项；不能当作494个新功能。每项有主包，复合项有关联包，已修仅回归，部分关闭拆出余项。

P6.0→P6.1→P6.2→P6.3→P6.E/P7→P8→P9；P10仅接已裁决的有限补充。T0/投入未定，采用波次、工作日区间、责任角色、依赖和出口，不编造硬日历。早期决策截止与后续实施依赖分列；E05等可选工作量只含裁决/有限adapter，不假称数日建成整套专业产品。

所有条件项必须在指定窗口输出建设/接入/深链/退出决定；真实条件缺失排mock/contract与再次核验，不以“等有需求”永久保留。语义错归和原“闭合”中的余项经过独立复核修正；来源历史不改写成当前通过。

## 9. 验证证据

本轮仅文档/受控清单变化。主仓固定实现新编译，TR/AL亦新构建并核实际容器binary SHA，使用八服务私有栈执行原样门禁。验证汇总为本地`validation/final-summary.json`，正式复核数据如下：

- `make lint`通过，Swagger生成物无漂移。
- `make test-unit`通过：233个顶层测试，失败/跳过均0。
- `make test-integration`通过：531个顶层测试（含默认tag用例，不能与233相加当去重总数），失败/跳过0，race告警0，夹具自有testcontainers。
- `make test-cover`通过：默认tag的`./internal/...`为20.4%，不含integration。本轮没有改测试/实现，不能把它与历史20.3%的差异称作覆盖率改善。
- 私有`make migrate-up`与原样`make acceptance`通过：253通过/0失败/0跳过，81对迁移往返零漂移，schema前后81且非dirty。
- UI六件等价门禁通过：lint、typecheck、31文件244单测、audit:prod无已知漏洞、build、全部42项真实E2E（0失败/0跳过/0flaky）。原spec/globalSetup/helpers不改，ignored配置只调整私有地址、代理、DNS、输出与定位，原worker/retries保留；273项UI运行输入前后相同。
- 五仓718项Go/模块/SQL/配置/脚本输入前后无变化；共享六个容器ID、状态、启动时间、镜像、挂载均不变；本轮两个owner的容器/卷/网络均清净，testcontainers已回收。

首次私有栈准备遗漏旧compose占位环境变量，在PG前置失败；原失败日志与精确清理记录保留，新attempt2补齐合成凭据后通过。该失败属于本轮夹具准备，未冒称产品失败，也未用重跑覆盖原证据。旧验收收据没有被当成本轮通过结果。UI首次隔离配置因缺ESM上下文在进入用例前装载失败，保留原日志；补齐ignored配置上下文后全部42项通过，未改产品或用例。UI两个owner资源、Vite端口均回收，既有state文件复原，共享容器/系统卷仍零变化。UI收据为本地`validation/ui/final-summary.json`。

文档与规划静态检查通过：286份Markdown、2,800个本地链接/片段零缺失；全部JSON可解析，61工作包/494来源/132依赖无缺失、环或后波依赖。116条触发、26件工单、81条历史决策、30项比较及P5 A–H/L各组均有去向；Markdown投影与JSON权威字段一致。三路独立复核发现的前置、错归、半关闭余项和旧正文冲突已逐项回读关闭；最后补核L组/历史随手项并单列真实现状待复核，不靠关键词库存宣称全部缺陷已复现。UI视觉及真实提供方功能仍见下一节边界。

## 10. 限制与后续真正需要验证的事项

- 未改CI/保护/Dependabot设置；当前缺口仍开放，按G02/G03/G05实施。
- 未实现风险、发布定义、AI/MCP/领域Agent或全站改造；主仓门禁只证明当前实现回归，不能证明新能力已交付。
- 未运行真实模型/知识/SOC/CMDB/SOAR/企业Agent契约和生产效果、生产容量或灾备；E01/E09及各能力包另验，mock保留。
- UI设计审查是源码/规范/旅程检查，既有真实栈42项回归另已通过；用户特定账号问题、新设计全站颜色/比例/缩放、视觉基线和完整键盘验收尚未完成；这些在U01/U02/U07及每批发布出口执行。
- 缺失API字段、委托模型、知识ACL、真实模型版本/usage等不编造；外部类别供给已确认，不能因未联通又记为“没有平台”。
- 估算是冻结前范围的工作模型，非人力无限或日历承诺；真实提供方待验只影响对应放行。
