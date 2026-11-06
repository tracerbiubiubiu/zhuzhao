# 生态全面审计 B13：环境恢复、启动编排与真实Chrome（2026-11-06）

## 范围与基线

所有者要求先整理开发/测试环境并使用已启用的Chrome插件实查。root独占共享环境与Chrome；并行代理只读配置/源码及独立工作树修复，不启动实验栈。本批不合main，不重置已有业务数据，不将端口在线等同全项目验收通过。

源码基线逐仓为：主仓 `8a3a12fa7b6b58cafb09f661f30080e317551a7a`（phase5）、UI `b20594cc744a61e2bcb41535b0ea668a711aa4df`（phase5）、taskrunner `1dcc83185f24b8dc2bed7bba6786272a5fdfb19f`（main）、activelist `af0b350da650517df2c9d15cca4892cdcd51be88`（main）、utils `9296ae170b71b06c623c54b6010a107406903346`（main/v0.4.3）。这些是原工作树基线，消费者实际版本由各自go.mod决定，不以utils HEAD替代。原UI他方.gitignore在途不纳入、不改写。

恢复前，Chrome用户页指向localhost:4000，但Vite4000、主API33333、TR18080、AL18081均无监听；开发主PG/Redis及AL PG已停止约45小时。test48081/API33334仍在线。未查明这些进程先前为何停止，不把本轮观察当作代码崩溃的根因证明。

## 恢复结果与运行来源

- **开发**：恢复既有 `zhuzhao-dev-postgres`、`zhuzhao-dev-redis`、`activelist-dev-postgres`，容器ID及实际卷分别与恢复前一致；主PG/AL PG备份及离线SQLite备份已保存，迁移输出 `no change`。原卷为 `zhuzhao-dev_dev_pg_data`、`zhuzhao-dev_dev_redis_data`、`deploy_pgdata`。没有down -v、重建空库或清除旧记录。
- **宿主应用**：从上述原工作树分别构建主API、AL、TR，记录二进制SHA、PID、cwd及完整源码hash；33333/18081/18080 ready均200。Vite4000使用UI仓开发mode=base、strictPort，开发徽标明确。配置差异仅私有env，冻结yaml未改；新开发JWT密钥令旧会话需重新登录，未改任何已有账号口令。
- **对象存储**：开发既有 `zhuzhao-rustfs`9000/9001、mirror、backup保留；test使用自身 `zt-rustfs`59000及独立数据卷。没有以新开发storage profile覆盖原存储数据。
- **测试**：主app固定 `7389ed79bdf409e466542992d9f1befa0c1a8529`、AL `19a4c1d5cd84cc892b82ad52554db320bfad8f7f`、TR `5e3f5f3720a54fad9f5cec4a08481364693b8f7d` 保留。UI从干净 `61b35bf75596c79bf42c7c7d26544ac584716ba2` archive重建，以 `VITE_APP_ENV=测试环境` 构建，镜像 `zhuzhao-ui-audit:61b35bf75596`、ID `sha256:7e56188a423402338b3dacd7c515e0607ea69db1ec35fe7d59d668e1c9e403d0`。283个源码blob及259个静态产物逐一匹配，Nginx模板原样保留，版本label与来源一致，旧镜像名/手工复制产物的混淆已消除。仅重建UI容器，测试数据库和其余应用不重启。
- **其他资源**：外部 `cmpfull-20261106-backend-*`、归属未证实的Created容器及可选RedisInsight保留；只删除本审计已释放、ID与owner双核的F15一次性PG。没有全局prune或按名称前缀批清。

恢复使用受控构建与启动，没有运行现有 `scripts/dev-stack.sh` 或用 `make dev` 杀占用端口。下面发现不会因本轮成功恢复而自动消失。

## 分项发现与修复挂点

### ECO-20261104-B13-F01 / P1：跨仓Compose project、服务与PG卷碰撞

主仓基线锚点：[scripts/dev-stack.sh 的 COMPOSE_PROJECT_NAME / up / down](https://github.com/tracerbiubiubiu/zhuzhao/blob/8a3a12fa7b6b58cafb09f661f30080e317551a7a/scripts/dev-stack.sh#L14)。AL基线：[deploy/compose.dev.yaml 的 postgres.volumes](https://github.com/tracerbiubiubiu/activelist/blob/af0b350da650517df2c9d15cca4892cdcd51be88/deploy/compose.dev.yaml#L15)；TR基线：[deploy/compose.yaml 的 postgres.volumes](https://github.com/tracerbiubiubiu/taskrunner/blob/1dcc83185f24b8dc2bed7bba6786272a5fdfb19f/deploy/compose.yaml#L33)。

脚本全局export `COMPOSE_PROJECT_NAME=zhuzhao-dev` 传给三个仓。只读Compose config实证AL开发库与TR部署库均解析为project=zhuzhao-dev、service=postgres、卷=zhuzhao-dev_pgdata，与“三栈隔离”相反，存在同项目服务接管/串卷风险。占位审计env与 `/dev/null` env-file解析，不读实际secret；四份config均exit0。未执行碰撞up/down，未证明本轮已发生数据损坏，不升级为P0。

建议各仓明确独立项目且兼容已有卷身份；AL现场历史项目为deploy，不能直接改项目名创建新空库。先校验配置和归属，再执行有限目标操作；有冲突直接失败，不自动接管。

### ECO-20261104-B13-F02 / P1：开发依赖失败后回落生产拓扑

基线锚点：[scripts/dev-stack.sh 的 up 主依赖启动](https://github.com/tracerbiubiubiu/zhuzhao/blob/8a3a12fa7b6b58cafb09f661f30080e317551a7a/scripts/dev-stack.sh#L22)。

`make docker-dev-up` 任意失败后改起生产Compose的PG/Redis。实际config解析，开发卷是zhuzhao-dev_dev_pg_data，fallback则为zhuzhao-dev_pg_data；fallback还没有宿主5432/6379映射。它既换数据来源又不提供宿主依赖，不是等价重试。建议保留原错误、非零退出，禁止自动跨拓扑回退。未执行此失败回退，不声称旧数据已被覆盖。

### ECO-20261104-B13-F03 / P1：taskrunner失败被吞后仍报告成功

基线锚点：[scripts/dev-stack.sh 的 up taskrunner / 成功输出](https://github.com/tracerbiubiubiu/zhuzhao/blob/8a3a12fa7b6b58cafb09f661f30080e317551a7a/scripts/dev-stack.sh#L27)。

TR Compose失败被 `|| echo` 转为成功路径，最后仍输出编排完成；down同样吞错。脚本还混合开发依赖与无宿主端口的TR部署栈，不能据此保证宿主18080可达。建议预检先于写操作、逐步传递退出码并验证依赖健康，明确“依赖/部署辅助”和宿主应用启动的边界。不要为修文案而暴露生产内网端口。

### ECO-20261104-B13-F04 / P1：make dev无归属检查杀端口进程

基线锚点：[Makefile 的 dev recipe](https://github.com/tracerbiubiubiu/zhuzhao/blob/8a3a12fa7b6b58cafb09f661f30080e317551a7a/Makefile#L25)。

原recipe对目标端口 `lsof | xargs kill -9`，不检查PID归属，可能杀其他会话应用；原APP_PORT可覆盖并不能消除该行为。建议端口占用即失败并提示所有者，不自动kill。静态直接证据明确，本轮没有杀真实占用方，也没有为了测试故意中断环境；修复验证使用命令桩。

### ECO-20261104-B13-F05 / P1：生产UI构建上下文指向不存在目录

基线锚点：[deployments/docker-compose.yaml 的 ui.build.context](https://github.com/tracerbiubiubiu/zhuzhao/blob/8a3a12fa7b6b58cafb09f661f30080e317551a7a/deployments/docker-compose.yaml#L272)。

`../zhuzhao-ui` 按Compose文件目录解析为主仓内的zhuzhao/zhuzhao-ui，标准同级checkout下不存在；真正兄弟仓在主仓的上一级，test Compose使用 `../../zhuzhao-ui` 正确。实际config解析路径及存在性均已核。建议修相对路径并守卫标准同级目录的context可达性，不需动生产实例。未执行生产build，不虚报已发生生产构建失败。

### ECO-20261104-B13-F06 / P2：启动文档口径残留

基线主仓hash同上。锚点：[deployments/README 的任务上游说明](https://github.com/tracerbiubiubiu/zhuzhao/blob/8a3a12fa7b6b58cafb09f661f30080e317551a7a/deployments/README.md#L70)、[docs/ops/port-allocation 的宿主/容器分界](https://github.com/tracerbiubiubiu/zhuzhao/blob/8a3a12fa7b6b58cafb09f661f30080e317551a7a/docs/ops/port-allocation.md#L67)、[deployments/docker-compose.test.yaml 的探针示例](https://github.com/tracerbiubiubiu/zhuzhao/blob/8a3a12fa7b6b58cafb09f661f30080e317551a7a/deployments/docker-compose.test.yaml#L7)。

包括旧TR8080、开发PG/Redis“不映射”旧描述、错误的48081/api/health/ready探针，以及test33334和历史第二实例预约冲突。主dev/AL-dev是仅依赖形态，TR生产零宿主端口，不能把不同入口的up拼接后称全链已启动。主生产migrate有profile、当前test migrate无profile：历史“profile永不起”的事故不能直接套用当前test文件。建议仅校准当前入口，历史演练保留时点，不再复制新一套runbook。

## 真实Chrome验证与前端边界

使用用户已启用的Chrome扩展，控制指定localhost:4000用户页与本任务创建的127.0.0.1:48081测试页；没有采集无关个人标签页内容。开发使用runbook已登记E800001；测试库新建E800006专属admin并登记，未改E000001或任何已有账号口令。开发与测试使用不同host，浏览器Cookie作用域分开。

实际结果：开发正常登录、首页统计与最近工单、AL既有类型及3条记录、任务定义读取成功。测试正常登录，首页待处理卡跳到带status=open且回显正确的列表，实际14条；建单空表单显示类型/标题/组织三处必填反馈，取消未建单；任务读取8条历史记录，AL零类型显示选择提示而没有无目标“写入首条”。未提交任务、改角色或修改既有业务记录。

在真实Chrome1280×800列表、1024×768表单检查中，document.scrollWidth等于clientWidth；窄列表仍有内部横向表格区域，不等同所有列无需滚动。亮/暗主题、表单布局和主操作已看截图；临时视口及主题已恢复。四张证据为dev-home.jpg、test-tickets-1280.jpg、test-create-dark-1024.jpg、test-home-restored.jpg。

**开发原UI b205仍出现8条本次观察窗口内的ResizeObserver.unobserve异常，均对应已登记B08-F08；修复分支未合main，不能把恢复服务写成修复代码已承接。测试修复版本轮Chrome控制台error为0，但B08-F14暗色主按钮仍为白字rgb(255,255,255) / 背景rgb(129,140,248)，保持挂账。** 部分主题/键盘工具调用超时，后置DOM另核实际状态；不把工具失败当产品缺陷或隐去尝试。

本轮是代表旅程与环境复核，不覆盖所有角色、路由、移动端、缩放、屏幕阅读器或完整可访问性规则。f104×7389前次完整41/41、主仓253/0/0+81迁移往返仍属各固定来源的原门禁；新61测试徽标镜像没有重跑完整41，不能继承整套通过结论。

## 验证证据、材料与待办

过程材料只在主仓忽略目录 `.codex/audits/ECO-20261104/`，不纳Git/Docker。环境目录 `environment-recovery-20261106/` 保存恢复前后容器/卷、PID/二进制/来源、PG与SQLite备份、私有env、迁移/构建/启动日志、readiness、UI部署及Chrome result.json/四张截图；`evidence/ECO-20261104-environment/` 保存逐文件blob/SHA、Compose碰撞和标准上下文解析；`evidence/ECO-20261104-b08/p2/image-61b35bf/` 保存干净源码/构建/镜像259文件摘要。凭据/备份仅本机私有权限，报告不复制实际secret。

常规六项HTTP探针200，另核开发原卷/容器身份、test运行镜像和源码/产物链。SQLite原库只读打开备份曾失败，改用确认停写且无WAL的immutable只读快照后quick_check=ok并完成backup；Vite首次无mode导致标题配置缺失，已改base并重启本人进程，两次尝试均保留，不算产品新发现。

### 启动修复提交与固定复验（尚未完成实栈门禁）

独立分支 `codex/fix-dev-stack-isolation` 提交 **`760a58363b0a6b75c4f7e8233f7ebb3cc1137c7b`**（base8a3），9文件显式路径提交且工作树干净。F01–F05分别由 [scripts/dev-stack.sh 的 compose/owner检查/up/down](/Users/bujibuji/.codex/tmp/zhuzhao-audit/ECO-20261104-fix/dev-stack-isolation/scripts/dev-stack.sh:35)、[Makefile 的 dev/docker-dev-*](/Users/bujibuji/.codex/tmp/zhuzhao-audit/ECO-20261104-fix/dev-stack-isolation/Makefile:25)、[生产Compose ui.build.context](/Users/bujibuji/.codex/tmp/zhuzhao-audit/ECO-20261104-fix/dev-stack-isolation/deployments/docker-compose.yaml:272)修复；F06相关当前README/端口/探针说明在候选同步。默认项目明确为zhuzhao-dev/deploy/taskrunner以保AL旧卷；所有配置与归属检查先于写操作，失败传递且等待健康，没有生产fallback或自动kill。TR部署栈仍只在内网，宿主应用另起，不扩安全边界。

新 `scripts/test-dev-stack.py` 是外部命令全桩的黑盒回归，已接入CI lint步骤和make test-dev-stack。原8a归档17用例出现21个失败断言（含子用例），不是21次真实环境故障；最终19用例通过。五类逐项撤回修复的目标变异（全局project/生产fallback/TR吞错/port-kill/UI context）各串行8轮稳定红。4份只读Compose config确认独立卷和端口配置。bash -n、py_compile、diff-check、commit-lint均通过。

root另从固定760a583重新git archive，679个跟踪文件逐一与Git内容匹配、零差异；再独立执行shell语法检查及全部19项黑盒，**8.946秒、exit0**。证据 `evidence/ECO-20261104-environment/dev-stack/root-fixed-760-results.json` 及同目录日志，不以未提交工作树结果替代。

**仍未验证**：未运行候选真实up/down/reset、生产UI构建、Go全门禁、make acceptance或远端CI。独立工作树本身不是标准同级四仓布局，候选UI context在那里不存在；路径修复由标准同级fixture验证，不冒充镜像构建成功。TR原main生产探针的8080残留仍须承接B07-F11修复，不能把主仓候选提交当成兄弟main也已更新。候选未部署、未推送或合main，F01–F05保留“修复验收中”，不标完整关单。

现场恢复与Chrome已完成，文档记录首批提交bf8a267；控制台/runbook明确旧工作树入口限制和当前端口预约。剩余全面审计继续原计划，不因环境恢复或桩测试通过而提前收口。
