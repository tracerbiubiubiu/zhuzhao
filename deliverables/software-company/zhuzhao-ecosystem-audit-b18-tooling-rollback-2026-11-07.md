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

B15-F02、B15-F03及B18-F01已通过下述准确提交CI，按本批范围收口。F03只覆盖“操作预检与安全回滚流程”，不承诺所有down自动成功，也不移除FK的数据保护。存储ID合同、UI当前小残留、L5、dbtx预防试点、归档部署前置及原生dev承接继续按各自批次核验；未来规划或外部触发项不随本报告虚销账。


## 提交与准确CI

代码修复`ef90363f0f5fb70cac215c4a2979b7e97f565a0d`，十二文件显式路径提交已回核，无业务源码或依赖文件。文档承接`996c167f4567e39362226937b25368b55b7f44bd`的[准确CI](https://github.com/tracerbiubiubiu/zhuzhao/actions/runs/42350455719)三个job全部通过；两提交之间仅审计文档变化，工具/测试源码字节一致。没有冒称ef自身另跑过一条CI。

## UI节奏联合补证（B15-F04，待UI代码提交绑定）

主仓732项、UI285项固定快照及三运行ELF逐SHA核验，真实编译器均Go1.26.10。attempt7通过原样253/0/0、81对往返及UI六件门禁41/41，默认retries=1但实际0retry/0flaky/0skip；W5额外8轮16例、retries0，功能HTTP429为0。完整门禁三条profile401/403分别对应PAT吊销/S1强制改密预期负测，不当成系统故障。

之前失败轮保留：漏传ZHUZHAO_REPO导致隔离布局单测失败；S8c浏览器预检被宿主代理转发到nghttpx返回400，并未到RustFS；冷PG临时Unix server被socket健康探针误判ready。S8c由真实OPTIONS/NetLog及同栈对照确认，仅本轮Chromium增加目标域名代理例外后完成真实PUT/confirm/建单/详情图片，不改系统代理、CORS规则、浏览器安全策略或原断言。后续fresh完整门禁才算联合通过。attempt2默认outputDir覆盖导致原失败trace/截图丢失的限制如实保留，后续逐阶段独立目录及SHA；共享ignored依赖元数据被并行pnpm刷新，源码/项目锁文件未漂移，不声称共享缓存零写。

W5变异采用限定8轮：恢复01c5原spec、普通负载下16/16通过且无429，**未复现默认用例稳定红**；相同有界真实耗桶前序负载下，修复16/16通过、功能429为0，去等待侧9绿7红、6/8轮有失败，出现21次浏览器429（14次profile/permissions）。耗桶阶段预期429单列（正侧509、负侧440），没有加码请求上限或修改限额/成功响应/功能断言。证明预算等待在已耗桶条件下有效，但负侧仍有概率性，不把它包装成每轮必红。完整原/修/变异数据和限制随UI提交交付，其他B19/B20候选不借本快照冒称已验。

七轮环境验证及额外变异栈的自建容器、卷、网络均已精确回收；清理动作与起跑前资源交集为空，16个既有dev/test容器元数据保持。cmpfull等其他会话资源的并发增删逐ID另记，未清理或据此宣称整机无变化。过程中错误口述Go1.26.7已由实际拷出的三ELF纠正为Go1.26.10，未据错误口述运行其他镜像；root另独立重核三SHA/编译器。

## B22：既有单测的时序假设修复

B19本地门禁先后复现两处旧测试失败，分别是`internal/middleware/ratelimit_test.go`的`TestRateLimit_RouteBucketIsolation`跨Redis秒边界合法补令牌后第三次200，以及`internal/pkg/audit/policyeval_test.go`的`TestPolicyEvalWriteFlushToStore`两记录合法分两批刷盘却断言首批必须两条。生产限流Lua、规则与审计写入逻辑均未变。

前者仅在两处miniredis夹具固定UTC时间；后者跨全部成功批次按原序检查总数和字段，不再假设调度批界。跨真实秒边界原8红/修8绿/去冻结8红，限流全套race×8为48PASS；受控首条刷完再写第二条时原8红/修8绿/移除修复8红，普通修复侧另8绿。未删除或放宽原业务字段/顺序断言，也不以提高生产限额或超时换绿。

首两次完整单位门禁失败原样保存；同一组候选修正后，原生Go1.26.10原make test-unit/test-cover/test-integration全部exit0（15.19/28.38/737.81秒），Go源码在各次运行期间固定。lint首次私有缓存缺Swag运行库且proxy=off失败，保留日志；改用已有已核模块file代理后原make lint通过38.99秒，无新增依赖或生成漂移。原样253/81业务门禁在未变化的生产源码上已由上节验证；B22仅改测试，准确提交及CI继续单独绑定。

**提交绑定**：B22为`596f625e8c678b26bce5bd96a8058cff91c17f5e`，锚点为[`ratelimit_test.go`的`TestRateLimit_RouteBucketIsolation`](https://github.com/tracerbiubiubiu/zhuzhao/blob/596f625e8c678b26bce5bd96a8058cff91c17f5e/internal/middleware/ratelimit_test.go#L121)与[`policyeval_test.go`的`TestPolicyEvalWriteFlushToStore`](https://github.com/tracerbiubiubiu/zhuzhao/blob/596f625e8c678b26bce5bd96a8058cff91c17f5e/internal/pkg/audit/policyeval_test.go#L65)。W5预算等待为UI `b5cbf1596be2d2df98038b6534c38a6242515f25`，锚点为[`w5-al-pages.spec.ts`的`beforeEach`及夹具后预算等待](https://github.com/tracerbiubiubiu/zhuzhao-ui/blob/b5cbf1596be2d2df98038b6534c38a6242515f25/e2e/w5-al-pages.spec.ts#L17)。均为显式路径提交且提交后文件集已核，准确推送/CI另验，不能借用996c167的CI冒称本次新提交已验绿。新发现的公式字典嵌套池连接问题另按P1优先修复，不随本报告关闭。


## 后续准确提交与环境承接

B22、W5与公式字典P1后续均已独立闭环，准确主4db/UI152 CI及dev/test最新产物通过核验，见[B27最终收口](./zhuzhao-ecosystem-audit-b27-phase5-closeout-2026-11-07.md)。原公式问题修复为82bda27，通用dbtx未实现；W5变异仍是上文披露的概率性证据，不升级成每轮必红。此处更新最终承接状态，不覆盖旧轮次失败或假定预检等于DB回滚许可。
