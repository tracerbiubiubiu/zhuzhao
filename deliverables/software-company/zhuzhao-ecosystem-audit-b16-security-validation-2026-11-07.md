# 生态全面审计 B16：Phase5安全验证（2026-11-07）

> **B16安全补证与B17修复复验收口**：F01/P2配置层级、F02/P1受审Go运行构建及F03/P2依赖版本升级已完成本批代码和测试栈验证；保留测试栈承接三新Go1.26.10产物及正确配置模板。四源码/三真实ELF复扫仅剩未消费的OpenPGP模块提示，不称绝对零漏洞；主仓/AL stripped扫描精度限制保留。完整CI、253/81、真实E8同Host直传与版本守卫变异均有效。原生dev、main及生产未承接本批运行产物，Phase6未开始，无main PR或正式发布。以下首轮/候选段保留当时状态，当前以“最终复验与承接”节为准。

## 范围与基线

五仓全部纳入：zhuzhao、zhuzhao-ui、taskrunner、activelist、zhuzhao-utils。开工时五个已验收审计头分别为cc8fd4ae4e2a4b91f77886d56f4ef6c38f1a81ae、51d5e9a2524c17e60536240913e341f05cddc5f5、21352322766c26ae9a35a45e3798f6155d6d3acd、50314a7dd0e77b9c27a2ef3f4cd1594cceb77b49、481c7ec3dc53c77cb23ac033985ea78593b0ca9c；与旧运行固定源的非文档/CI/忽略规则内容等值证据沿用并重核，不将头hash变化误说成运行产物已更新。

本批将主仓、UI实际Phase5与审计候选正常合流：主仓`09d224dca1fdeaf9e2e70e8bf7dd5aaedd3ee31b`，UI`01c5deae23e677853316f1ee2dd692c708246651`，两结果树分别逐path/object/mode等于原候选，保留双亲历史。**后续主仓/UI修改基于实际phase5**；三个Go兄弟没有常驻phase5分支，仍在Phase5用途的`codex/phase5-audit`修改，不触及其main。本操作不是准备main合入。

四Go源码扫描首个CI控制提交为`191d6e5efb3dd92d74f9943d803fd3f97f605f23`；兄弟源钉上述三个完整SHA，目标linux/arm64、CGO0、无test tag/replace、mod readonly。真实运行ELF分别仍为主仓/AL Go1.26.6、TR Go1.26.10，逐SHA核对通过；utils没有独立运行ELF，需回到主/AL当前pseudo-version和TR实际v0.4.2的消费图。

## B16-F01 / P2：Secure配置行位于错误命名空间

基线主仓09d224d（与前固定dc3生产内容相同）：[configs/config.yaml](../../configs/config.yaml)及[deployments/config.test.yaml](../../deployments/config.test.yaml)的`file_cookie_secure`位于`audit`段；实际字段为[storage/config.go的StorageConfig.FileCookieSecure](https://github.com/tracerbiubiubiu/zhuzhao/blob/09d224dca1fdeaf9e2e70e8bf7dd5aaedd3ee31b/internal/pkg/storage/config.go#L31)，经[config.go的Load/BindEnv](https://github.com/tracerbiubiubiu/zhuzhao/blob/09d224dca1fdeaf9e2e70e8bf7dd5aaedd3ee31b/internal/config/config.go#L351)读取，最终由[SetFileSessionCookie](https://github.com/tracerbiubiubiu/zhuzhao/blob/09d224dca1fdeaf9e2e70e8bf7dd5aaedd3ee31b/internal/middleware/fileauth.go#L19)消费；原错位行分别为[基线配置L75](https://github.com/tracerbiubiubiu/zhuzhao/blob/09d224dca1fdeaf9e2e70e8bf7dd5aaedd3ee31b/configs/config.yaml#L75)、[test配置L80](https://github.com/tracerbiubiubiu/zhuzhao/blob/09d224dca1fdeaf9e2e70e8bf7dd5aaedd3ee31b/deployments/config.test.yaml#L80)。Ruby YAML真实解析确认`audit.file_cookie_secure=false`而storage对应键缺失，不仅是注释或rg猜测。

当前dev/test是HTTP且缺省false，本批未证明泄露或绕过；`STORAGE_FILE_COOKIE_SECURE`环境覆盖和Secure setter已有实现。问题是编辑模板该行true不能到达真实Storage字段，按安全配置合同偏差P2，不升P1或推断生产已受影响。

修复仅将两行移入各自既有`storage`段，仍为false；不新增重复storage、不改enabled/端口/密钥/运行配置，不启用Secure或归档。新增[file_cookie_secure_template_test.go](../../internal/config/file_cookie_secure_template_test.go)经真实模板→`config.Load`→StorageConfig→Cookie头验证默认false、只改模板true、env true正控与env false覆盖；包外测试隔离Viper和环境，串行执行。

专项证据：最终测试SHA下，原两模板各8/8精确失败于Secure=false/want=true，其余6正控绿；修复8叶race串行8轮绿；ignored副本逐模板移回旧层各8/8红，其余正控保持绿，恢复后绿。config+middleware完整race单测、vet/gofmt/diff通过，Ruby语义比较确认除键路径外所有值不变。初版测试误判既有storage.enabled导致控制夹具红，已保留并剔除，最终测试全重跑；没有把夹具错误算产品发现。

整仓重新冻结191d6e5+仅三文件修复（725文件SHA清单）：原`make lint`、`test-unit`、完整原生`test-integration -race -p1`（572.033s）、`test-cover`全通过，另有三套脚本守卫和module verify通过。原样`make migrate-up`/`make acceptance`在独立新7容器+原生app上通过253/0/0（39.593s）、81对往返零漂移，隔离E000001密码终态admin12345；源码SHA前后未变。三owned SHA分别c2d90fb3b28f46c133dc65f7ba9cabef4e6a2f2c1eaf349290b3e8d42a4ea4fb、52ff6b3d15d0b1b53e4faaffcd33c970229b3b2dc8dce0bd9b9e03bca0569165、84dbabd4f72c63862a8462533a515675de059258c879e8439fcf1b84a0a655ea。F01代码层关闭，未切常驻dev/test配置。

验收首轮保留97PASS/5FAIL：隔离runner使用Docker动态published=0，验收stop/start Redis后端口重分配，app仍连旧端口；独立同容器stop/start探针稳定确认HostPort变更。第二轮仅将本轮自有端口在随机选定后显式固定，完整新栈/原样断言通过，不把环境首红算产品回归或抹去。两轮均精确清理自有容器/卷/网络；共享dev/test ID/PID/StartedAt无变化，期间消失的Ryuk已由完整集成轮原生TestSessionID736f57ab确认归属，不谎称全部容器无差异。

修复提交为`bcdf133a8e014577dd5d99e580f73e4dab437b5e`，Refs为B16-F01；同类扫描已覆盖两模板、Load/BindEnv、StorageConfig及Cookie setter，配置声明均在storage段，未见同类残留。证据解释修复为`bb3bfc65c7e8ae733f2bc1b3e0517a13a04111e4`，Refs为B16扫描精度边界；同类扫描覆盖source/binary配置、缺module frame、stripped通配符/具名frame及各异常流，23条CLI回归有效。两笔已推fix正文有三节，但漏同类扫描/Refs且多摘要用了项目要求编号之外的普通列表；commit-lint为WARN而非FAIL，不等于人工格式合规。本文补齐永久hash/关联与扫描证据，保留已推历史不改写；后续fix按完整模板提交。

## Go工具与扫描证据状态

本机固定官方工具获取在约5分钟预算后停止：精确依赖ZIP不完整、h1未闭合，未产出可执行文件，未运行source或binary扫描；失败与partial保留，未替换依赖/关闭sumdb/改全局配置。后续改用[go-security.yml](../../.github/workflows/go-security.yml)在隔离runner采集四模块源码，官方v1.4.0工具及实际依赖h1独立校验；另交叉构建Darwin arm64官方工具并保存SHA/构建信息，供本机对三真实ELF扫描。

[parse-govulncheck.py](../../scripts/parse-govulncheck.py)完整解码JSON对象流，区分执行失败、findings待分级及限定配置无发现；source报告的调用链与binary精度分开，binary Go版本按SBOM核。缺module frame仍保留待分级；不能按JSON exit0、仅有OSV元数据或CI采集成功宣称零漏洞。[离线回归](../../scripts/test-parse-govulncheck.py)最初20/20通过，补stripped回退解释后23/23通过，覆盖截断/非法JSON、缺config/SBOM、源变化/错误Go/目标及exit0有发现等假绿路径。该workflow是证据采集，不是自动安全批准/升级或发布授权。

## 本轮Go采集结果与分批处置

[准确191d6e5的Actions运行](https://github.com/tracerbiubiubiu/zhuzhao/actions/runs/42333853841)五jobs采集成功；四源码artifact均核GitHub archive digest后安全解包，用固定191解析器离线重放一致。数据库更新时间`2026-11-05T23:49:57Z`。源码独立GO-ID数：主仓34（17个有symbol级报告）、AL18（9）、utils18（11）、TR9（5）；跨仓合并为34个GO-ID、39个GO-ID+模块组合、6个受影响模块，不能加总为漏洞或可利用项数。

官方Darwin工具ZIP digest与GitHub一致，实际本机`go version -m`及官方v1.4.0/依赖h1再核，工具SHA为`25ad1de33f62bc5557214b2bfdfc77a86f6b5fe7e765ce074ba79a776fd57047`。本机三真实ELF扫描均exit0、前后SHA不变、完整JSON解析通过：主仓33、AL17、TR9个GO-ID。主仓/AL剥离符号，v1.4.0的[binary回退规则](https://github.com/golang/vuln/blob/v1.4.0/internal/vulncheck/binary.go#L107)会把受影响模块的OSV符号全集合成函数frame，含OpenPGP通配符；**两份整份结果仅模块精度**，不能称这些函数存在或可达。TR的具名符号也不证明业务调用图/利用路径。解析器保留原始发现并显式标出全目标精度限制，新增三条真实CLI回归；旧解析结果保留，不覆盖原始JSON。

### B16-F02 / P1：受审运行构建仍含已修复的Go标准库版本

主仓、AL真实ELF为Go1.26.6，包含官方已修复区间；TR已1.26.10。Go[官方发布记录](https://go.dev/doc/devel/release#go1.26.10)及[GO-2026-6809](https://pkg.go.dev/vuln/GO-2026-6809)确认同1.26分支最小统一目标1.26.10；不要求跳1.27。受审源码锚点为[主仓go.mod](https://github.com/tracerbiubiubiu/zhuzhao/blob/191d6e5efb3dd92d74f9943d803fd3f97f605f23/go.mod#L3)及构建入口的`go`版本控制；实际产物SHA/Go信息以本轮manifest为准，不用go.mod最低版本替代真实编译版本。

该P1为已知受影响运行构建的处置优先级，未证明当前HTTP部署存在可利用的HTTP2攻击、证书链绕过或已发生入侵。下一修复批将统一固定安全补丁、重建并重扫实际产物、完整功能门禁后再关闭；改go.mod或CI绿均不能单独关闭部署版本问题。

### B16-F03 / P2：依赖图公告与真实入口需分层收敛

- `golang.org/x/net`目标正式v0.62.0；旧兄弟v0.57.0/主v0.60.0与本轮公告范围相交。主仓[go.mod的x/net](https://github.com/tracerbiubiubiu/zhuzhao/blob/191d6e5efb3dd92d74f9943d803fd3f97f605f23/go.mod#L148)为可复核锚点，HTTP2与html的包/symbol命中分列，不凭module存在断言当前输入可触发。
- 主仓`excelize/v2`目标正式v2.11.1，覆盖所报全部修复；当前[ticket_export.go的导出入口](https://github.com/tracerbiubiubiu/zhuzhao/blob/191d6e5efb3dd92d74f9943d803fd3f97f605f23/internal/handler/ticket_export.go#L60)只用`NewFile`、固定列流式导出，无用户工作簿导入/公式求值。source把embedded模板读取或宽泛OSV符号列入报告，不能称已证任意XLSX/公式DoS。
- `quic-go`正式v0.59.1、主仓`compress`正式v1.20.2是已有修复候选；当前生产没有RunQUIC/HTTP3配置，compress在源码仅module命中，不新报线上崩溃。优先随兼容的小批升级收敛，不增加新协议/新能力。
- `x/crypto`的GO-2026-5932仅已弃OpenPGP模块包，四源码均无package/symbol命中；当前正式最新v0.58.0仍可有module提示，不能误指bcrypt或整个crypto。没有正式v0.59.0；不为凑零报告强塞删除OpenPGP的pseudo-version，保留未导入证据与后续触发条件。上项升级自然带到v0.58.0时仍须照实记module提示。

源码`./...`的调用分析可能包括Gin未启用的导出入口或保守接边；本批没有把扫描器frame逐条升格为已确认业务漏洞。F02/F03先登记、再按最小版本批修与重扫，不在本报告偷换成已修复。

## B17依赖修复候选与联合复验窗口（提交时状态）

本小批仍为Phase5，承接B16-F02/F03。主仓基于bcdf133准备（3b721b4仅补Markdown），三兄弟分别基于50314a7/2135232/481c7ec。四Go模块最低版本及三Docker构建入口统一1.26.10；CI沿go.mod精确选版，旧本地编译器需安装或允许自动下载该SDK。主仓/UI分支归属不变，没有main合并或正式发布。

已提交并推送的三个兄弟候选：AL `db6a17bb686f09287d8793d1a3f98783bcc2e09d`、TR `1e31e3e95dc442b8384f3582cfc2850bd57d260b`、utils `563785a83ce2aeb12c5f2b65c42defb58a48c331`。net0.62.0/quic0.59.1及必要MVS依赖已get/tidy/verify，官方sumdb开启、无replace、业务源码与utils消费者pin不变。三准确SHA的CI已通过：AL [race及完整PG](https://github.com/tracerbiubiubiu/activelist/actions/runs/42337699606)、TR [race及专属PG/Redis](https://github.com/tracerbiubiubiu/taskrunner/actions/runs/42337704491)、utils [全部race](https://github.com/tracerbiubiubiu/zhuzhao-utils/actions/runs/42337711033)，实际Go1.26.10；未用无-v日志虚报每test/skip数。

主仓候选另升Excelize2.11.1/compress1.20.2；MVS共14项，除四直接目标外含既有mscfb/efp/crypto/image/sync/sys/telemetry/term/text/tools的必要版本选择。efp和telemetry的精确pseudo由上游正式库的go.mod带入并经h1验证，不是为OpenPGP提示主动引入crypto pseudo；crypto仍为正式0.58.0。三F01文件逐SHA不变，业务Go源码无改动。正式目标的tag→commit、mod/ZIP SHA与h1、完整前后graph只在ignored原始证据留档。

三个新linux/arm64、CGO0运行ELF均已按原生产buildflags重建：主仓`8709ba31478bee57c1fe58dedbe2c556a32ff54e19d1c7309b6a2a4a8bfe8a6e`、AL `b3ed59569cbc4fd7709a8c62f6dab8dadd120808c0ae37b8af8e0b84a2affc9b`、TR `69d73b46500d2c3426a99f130d272b95b47dcb66e7a140658a5f8ba17fb2fc2d`，全部实际Go1.26.10。AL/TR先行复扫均exit0、JSON完整、SHA未变，仅剩5932；AL仍仅模块精度，TR该项仅module，不能称绝对零漏洞或已验证OpenPGP可用。

本机原生SDK获取先遇官方JSON TLS失败，另一次经官方toolchain模块机制/独立cache/开启sumdb的下载180s超时，无完整可信SDK；Linux工具镜像apk安装也在有界获取后取消，未跑过的race/PG均未伪记通过。改为准确新提交的Ubuntu CI执行原`make lint/test-unit/test-integration/test-cover`，集成维持race/p1、保留Wire守卫；不是删门禁绕过。主仓完整CI、四源码与主ELF复扫、新8容器原样253/81尚待，F02/F03暂不关单。已过的F01旧门禁不充当本批新依赖验证。

## 最终复验与承接

**代码固定点**：主仓`603c1b64810bfa5c1fa4688b7ea413747e372d27`；AL/TR/utils为上一节三个完整SHA。主仓[准确提交的普通CI](https://github.com/tracerbiubiubiu/zhuzhao/actions/runs/42338549931)三个job全过：原`make lint`（vet/gofmt/Swagger）、`make test-unit`、`make test-integration`（原样`-v -race -count=1 -p 1 -tags=integration`，整job12m39s）、`make test-cover`，加Wire再生零差和四套脚本回归；实际Go1.26.10 linux/amd64，集成原始日志无FAIL/SKIP。覆盖率原profile为2041/10059=20.2903%，仅普通tag内部包单测，未计集成覆盖，不以不同平台的旧20.4%宣称提升。

**源码和真实产物复扫**：[准确603的安全证据CI](https://github.com/tracerbiubiubiu/zhuzhao/actions/runs/42338549928)四源码/工具五job全过，artifact SHA与API一致，固定603解析器重放一致，全部tracked文件逐一匹配对应Git对象、前后无漂移，官方工具及依赖h1闭合。四源码各只有GO-2026-5932一条module finding，package/symbol均0；生产配置下的605/393/440/370包依赖清单没有OpenPGP前缀。主仓/AL/TR三新实际ELF也各仅5932、扫描exit0且前后SHA一致；strip和TR无调用图的精度边界不变。原其它GO-ID已不再被本轮配置报告，未将“CI采集成功”或宽泛OSV符号当可利用性结论。

**原样生态验收**：同三ELF在独立新8容器栈通过`make migrate-up`及原`make acceptance`：253PASS/0FAIL/0SKIP（124+68+26+35），81对迁移往返零漂移，schema前后81|f；隔离E000001终态admin12345真实bcrypt验证通过。专属E8账号完成Login→presign→保持签名Host的真实PUT200；JWT/签名值未留档。主源码726文件前后不变，8容器/7卷/1网精确回收，外部容器及网卷前后完全一致。S3同Host DNS夹具仅用于这一隔离拓扑；保留栈原有共同可达endpoint和CORS另经只读核验，不借夹具误报原栈DNS故障。

首轮原make实际252/0/0且81对过，采集器的253硬断言失败；唯一少项为`#MSG-8 runs 观测面`，宿主环境漏传原脚本条件`TASKRUNNER_BASE_URL`，不是MCP或产品回归。第二轮仅补宿主本轮loopback地址、app仍内部服务名，在完整新栈原样重跑253/81；首轮与差集未覆盖。依赖解析初次临时ZIP错误、原SDK/工具获取失败也保留，没有把补充CGO0普通测试冒称race/完整PG。

**最低版本守卫**：四真实模块mod/sum固定后，旧Go1.26.6在GOTOOLCHAIN=local/无网络/无缓存下各8轮被最低1.26.10拒绝；仅降低副本floor到1.26.0各8轮放行，恢复后各8轮再拒绝，共96次。固定Go1.26.10四模块正控通过。这只证明模块版本前置，不冒称已用旧Go实际编译或复现漏洞利用。

**保留测试栈**：仅替换`zhuzhao_test`的app/activelist/taskrunner为同三新ELF；新image只改变已验收runtime的metadata，运行文件字节不变，真实revision分别绑定603/db6/1e31。先留3份PG custom备份并核SHA/只读目录，保留准确旧image/反向override；TR原两个匿名卷、PG/Redis/RustFS/UI、网络/端口与全部Env键值保持。app的只读配置源改指当前phase5的`deployments/config.test.yaml`（SHA52ff6b3d…），语义仅已验收F01键路径修正，其他值全等。三ready、三ELF/Go1.26.10及无token401只读边界均核；没有对共享库跑acceptance或改账号角色。原生dev未切换，main/生产未承接，不将健康探针扩大为整站认证。

配置挂载切换首次因附加metadata断言失败已自动回退到上一健康B17镜像/旧bind；采集器确有错误的Env原始list顺序比较，原/新键均唯一、canonical键值相同。首次具体失败行及已删中间容器ID未完整采集，不能把该谓词猜成已证首因；缺口原样登记。唯一一次改用逐checkpoint/唯一键canonical比较的重做通过，最终只三目标容器变化，其他资源保持，未隐去失败或伪造中间ID。

**处置口径**：F01与F02在上述代码/保留test范围关闭，F03的已发布依赖修复完成；5932保留为未导入OpenPGP的模块级边界。后续出现实际OpenPGP导入或新的正式修复版本时重开对应处置，当前不强塞删除API的pseudo，也不说零漏洞。许可证、所有镜像OS组件、原生dev和生产不在本批安全认证范围。本地SDK获取仍受阻，后续原生dev/本地工具链整理继续按Phase5安排；原B15四P2及历史债没有随本批虚销账。

## 本批提交窗口

主仓提交按实际phase5 first-parent窗口`17da0a3ae51f93f9ee2d05cf55fa7bae6cec5824..603c1b64810bfa5c1fa4688b7ea413747e372d27`逐笔为：

- `09d224dca1fdeaf9e2e70e8bf7dd5aaedd3ee31b`：正常承接既有已审候选，结果树等于cc8；第二亲携带的旧审计历史不重计为本批新修复。
- `191d6e5efb3dd92d74f9943d803fd3f97f605f23`：隔离安全证据采集。
- `f0e000490bf35ce1377ab199b6c7dc3e6906f602`：B16/分支归档正式记录。
- `bb3bfc65c7e8ae733f2bc1b3e0517a13a04111e4`：stripped精度解释与CLI回归。
- `bcdf133a8e014577dd5d99e580f73e4dab437b5e`：F01配置与专项/整仓验收收口。
- `3b721b4484c559f1c9349de9091d2cb2a4de2687`：完整hash/提交关联补录。
- `603c1b64810bfa5c1fa4688b7ea413747e372d27`：B17依赖、编译版本与CI原Make目标。

UI本批只正常合流`01c5deae23e677853316f1ee2dd692c708246651`，树等于51d；AL/TR/utils各一笔B17新修复hash如上。报告收口的后续纯Markdown提交另计，不改变上述被测源码或运行产物来源；不用包含第二亲历史的普通双点log冒称本批仅七笔全部历史。

## 前端与部署安全差集

- UI本轮`pnpm audit --prod --json --registry=https://registry.npmjs.org` exit0，各级已知漏洞报告均0；metadata dependencies233/optional96/total329，锁文件和package.json前后SHA不变。结果来自51d5e9a，Phase5合流树与之完全一致，不扩大为所有前端输入安全认证。
- 登录已有12处显式`LogLogin`，logout位于authed AuditLog链；只读实际库存在login成功/失败及logout记录，request_id非空，旧“零审计”不能转成重复开发任务。部分前置拒绝/设备校验/内部错误早返仅列覆盖边界，未据此报新P1。
- 当前dev/test未启自动归档；TR只有manual/disabled cron，目录无归档文件。test及生产Compose模板缺归档持久卷；必须先核持久化、JSONL备份恢复与保留/删除策略再考虑启用。源码fsync后删DB批次的既有保护不等于已完成部署恢复；PGdump或AL/TR逻辑恢复不能代替主仓JSONL恢复。未启任何删除开关、未证已丢数据，不凭此新报P0/P1。

## 本地分支归档与恢复

56个本地heads逐项复核后保留10个活跃分支：主仓/UI各main+phase5；三个Go兄弟各main+Phase5审计短分支。46个旧head先复制到`refs/archive/phase5-audit/20261107/…`，五个完整bundle的SHA/verify/list-heads均闭合；21个祖先用branch-d，25个人工承接来源未谎称merged，在历史已完整归档后按预期tip CAS移出heads。40旧工作树仅同hash detach，54目录、ignored证据/备份/原型均保留，未删远端分支/改main。恢复可由archive ref或完整bundle重建旧branch；归档不是历史丢失。

## 记录与后续出口

原始命令/JSON/配置解析/只读DB聚合、分支逐ref表、归档bundle与恢复说明仅存主仓ignored `.codex/audits/ECO-20261104/security-20261107/`。正式结论入本报告及项目控制台；分批修复按发现编号/固定hash/原与变异证据记录，完成必要门禁后才关单。四源码/三ELF已完成分层复验，F01/F02与已发布依赖修复按上述代码/test范围收口，仅未导入OpenPGP模块提示继续保留；主/AL二进制明确LIMITED，工具与DB时点、真实构建版本不缺省为零。本批不涉及稳定性下一批或Phase6实现。
