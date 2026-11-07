# 五仓审计 B18：生成工具与模板回滚预检

## 范围与基线

承接B15-F02/F03，仅修Phase5工程入口和回滚操作前置；不改业务依赖、历史迁移、API、权限或数据库数据。主仓基线`2ed8069370540fb7228b9f50e431eb2bc995dce8`，UI `01c5deae23e677853316f1ee2dd692c708246651`；AL `db6a17bb686f09287d8793d1a3f98783bcc2e09d`、TR `1e31e3e95dc442b8384f3582cfc2850bd57d260b`、utils `563785a83ce2aeb12c5f2b65c42defb58a48c331`本批无产品改动。主仓业务源码仍与`603c1b64810bfa5c1fa4688b7ea413747e372d27`相同，三真实运行产物沿B17已核SHA；新工具不冒充新业务构建。

B15-F01存储ID合同、F04浏览器夹具节奏另批处理；UI门禁未通过的结果不记为本报告已完成。当前不合main、不发新tag、不开始Phase6。所有者指定Go1.27与Node24/26 LTS整体升级在Phase5合main后，基于main独立实施；本批保留Go1.26.10安全修复。

## 发现、修复与挂点

### B15-F02 / P2：Wire入口依赖业务go.sum

原Makefile以业务模块执行Wire CLI，Go1.26.10下可稳定复现缺`google/subcommands`及`pmezard/go-difflib`校验项；CI另行安装CLI能过，不证明文档入口可用。修复为[Makefile](../../Makefile)的`tools-install/wire/swag`统一调用[scripts/go-tool.py](../../scripts/go-tool.py)的`go_context/obtain/verify`。

Wire固定v0.7.0、Swag CLI固定现行CI的v1.16.4；CLI与业务Swag库版本分别管理。安装在业务模块外执行，私有模块/构建缓存位于ignored `.codex/tools/`，按实际Go版本/宿主平台/工具版本分目录；保留官方sumdb校验，核CLI构建信息的模块、包、版本、h1、编译器及平台，发布缓存前完成核验。显式BIN覆盖同样校验；损坏缓存不静默执行或无限重装。

SDK低于go.mod要求时先拒绝，不隐式下载SDK。安装默认180秒、工具执行300秒，参数限1–600秒，超时停止本次进程组。`GO`覆盖用于工具入口，其余原Make目标仍由本次PATH选择SDK。CI复用同一Make入口，不再保留第二套go install命令。业务mod/sum及Wire/Swagger生成字节未变。

### B15-F03 / P2：定制模板引用需要回滚前置

[scripts/migration-template-preflight.py](../../scripts/migration-template-preflight.py)的`snapshot_sql/inspect`在一个REPEATABLE READ READ ONLY事务中读取检查点、模板入站FK和规则，按down62/64/71/79/80的真实DELETE谓词顺序模拟，列明待删规则及仍存活引用。79的template_code限定单独建模；匹配种子业务键不等于规则未被定制。

工具要求显式`ROLLBACK_PREFLIGHT_DSN`，无默认目标；URI不接受重定向参数，口令不入argv/输出。已核down文件整份SHA、未知入站FK、dirty、权限、连接或范围不成立均失败关闭。支持target>=61；跨越既有FK/版本列边界不臆测。target<=74输出代码回退耦合标记，操作序见[runbook §6.1](../../docs/runbook.md#61-模板种子回滚前的只读依赖预检b15-f03)。

exit0仅证明模板引用无阻断，不是整体回滚批准；exit1须由所有者决定保留模板或改指规则后复查；exit2为未证实。工具不执行down/force、不删除规则或改历史SQL。预检不锁住后续写入，维护窗口及临近down重跑仍必要；失败后先核业务数据与dirty，禁止盲force。

### B18-F01 / P2：lint递归进入ignored工具缓存

原生SDK可用后，真实`make lint`中的`gofmt -l .`扫描ignored `.codex` SDK、模块和历史证据，因第三方/证据文件格式退出；这不是业务源码格式回归。修复为[scripts/check-gofmt.py](../../scripts/check-gofmt.py)的`main`使用Git枚举tracked及新增、未忽略Go文件，保持原vendor排除；新增未跟踪业务源码仍检查，Git清单失败或无源码不放绿。仅检查、不自动格式化缓存或业务文件。

## 验证证据

原生Go1.26.10 SDK通过官方toolchain模块及官方sumdb取得，ZIP SHA256为`afb16ae24c172f008db87119f34cf903a295bbad3fa526f7443914b0240b33c7`；只放项目ignored缓存，系统Go1.26.6和全局PATH未替换。明确SDK后执行原目标：

- `make test-unit` exit0，60.27秒；539条含子测试PASS记录，无FAIL/SKIP。
- `make test-integration` exit0，727.46秒；原`-v -race -count=1 -p 1 -tags=integration`，1,204条含子测试PASS记录，无FAIL/SKIP。
- `make test-cover` exit0，17.28秒；2042/10059=20.3002%，仅普通tag内部包。B17 Ubuntu为2041/10059，平台/执行路径不同，不宣称覆盖率提升。
- 首轮lint保留ignored缓存误入的失败；修复后`make lint wire` exit0，5.31秒，Swagger/Wire无漂移。
- Wire/Swag从空私有工具目录安装，各真再生两轮，mod/sum、Wire及三份Swagger SHA完全不变。9组工具CLI黑盒、恢复旧Make入口的反例、真实错工具/缺BIN/旧SDK负控通过；损坏缓存8轮连续拒绝。
- 回滚CLI黑盒12组通过。独立PG15留35条实核证据：fresh81到78预检与实际down成功；五类合法定制引用预检均阻断，真实down由FK拒绝且规则/模板数据指纹不变、检查点变dirty；dirty复查拒绝。未知FK、只读账号、权限不足、源漂移及连接参数负控覆盖。每次预检本身前后规则/模板/检查点指纹相同。去掉存活引用守卫后8轮错误放绿，恢复后8轮稳定阻断。
- Go格式边界5组真实Git/gofmt回归通过；“递归进入ignored缓存”和“漏掉新增源码”两个变异各2轮均使回归报红。
- B18自有八容器原样`make migrate-up`及`make acceptance`通过253/0/0、81对往返零漂移。其后UI完整gate首轮因隔离布局漏传既有`ZHUZHAO_REPO`而失败，保留原结果，不把整个联合运行称成功；修正夹具后的UI全链另行记账。首轮自建容器、卷、网络已回收，既有容器无移除/变更，其他会话新增资源另记。

脚本和正式报告入Git；原命令、JSON、快照、归档、SDK与失败日志仅ignored `.codex/audits/ECO-20261104/phase5-closeout-20261107/`，不纳Docker上下文。测试未触共享账号/库、生产或main。修复提交完整hash及准确CI结果提交后回填，不以提交正文代替独立验证。

## 收口边界

B15-F02、B15-F03及B18-F01本地复验满足本批出口，交付关单仍待准确提交CI。F03只覆盖“操作预检与安全回滚流程”，不承诺所有down自动成功，也不移除FK的数据保护。存储ID合同、UI节奏/当前小残留、L5、dbtx预防试点、归档部署前置及原生dev承接继续按各自批次核验；未来规划或外部触发项不随本报告虚销账。


## Commit binding

Code fix: `ef90363f0f5fb70cac215c4a2979b7e97f565a0d`. The twelve-file explicit-path commit was checked after commit; no business source or dependency files were included. CI for this exact revision remains pending.
