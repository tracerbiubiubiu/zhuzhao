# 工程与正确性施工设计 — Phase 6 近期工作包的操作合同

日期：2026-11-11。状态：**设计交付，全部新增机制待实施**。对应总账 G01/G02/G03/G04/G06、C01/C02/C04/C05；C03/R01 见[跨仓执行合同](cross-repo-execution-contract.md)。排期、工作量与唯一状态仍以[规划总账](../review/planning-backlog.md)为准，本页只拆任务，不重复计价。

2026-11-14基础先行边界见[基础版交付](foundation-release-boundaries.md)：近期检查当前固定组合、一个入网目标与已启用能力；第二支持代际、完整发布自动化、全工具离线开发闭包不作为B0/B1前置。减少首批功能组合，保留该组合内的数据、安全与测试要求。

本页在主仓 `00494900` 文档基线上读取实现；五仓业务源码沿用本轮 Phase 5 main 固定快照。既有验收收据证明原实现回归，不证明本页目标已启用。新增模块与内网连接器的运行边界分别由[扩展兼容设计](extension-compatibility.md)、[内网适配迁移设计](intranet-adapter-migration.md)拥有；本页负责把它们变成工程门禁。

## 已有底座及本次差距

- 五仓已有 `.githooks/`、`scripts/commit-policy.py` 和显式路径提交协议；`scripts/zz-commit.sh` **仅主仓存在**，其余四仓当前用等价 `git commit -F msg -- <paths>`，不能宣称包装器已同步。主仓 `Makefile` 的 `lint`、`test-unit`、`test-integration`、`test-cover` 与四档 `acceptance` 已存在。主仓 `.github/workflows/ci.yml` 的 `commit-policy/lint/test/test-integration` 已跑 main/PR；不得重新建设一套同名业务测试。主仓 `lint` 已验 Swagger，CI 已验 Wire 新鲜度。
- 主仓 `.github/workflows/go-security.yml` 的 `source-evidence` 仍限定旧 phase5 分支及扫描脚本路径，兄弟 SHA 为历史固定值；并不代表 main 每次改动有漏洞门禁。UI `.github/workflows/ci.yml` 在 shell 内直接插入 `github.head_ref`，且按“同名兄弟分支，否则 main”选 Swagger 源；这不是可复现的五仓候选组合。UI CI 有 lint/typecheck/build/audit/test，真实 E2E 仍在独立交付链。
- 本轮只读 GitHub 设置快照中，五仓 main 未保护、无 required checks/ruleset；secret scanning/push protection 已开启，vulnerability alerts/Dependabot 未开启。这是 2026-11-11 时点事实，实施 G02 前必须再读远端，不能凭文稿宣称已经改变。
- 主仓 `internal/pkg/exprcalc/exprcalc.go` 的 `Identifiers` 按函数**名称**全局排除标识符；`ApplyFormulasContext` 将原输入直接作为公式环境。`internal/service/ticket/service.go` 的 `Create` 已收集文件字段并同事务绑定，`Update` 只把 `req.Attachments` 交给 `bindAttachmentsTx`，未同规处理修改后的 file 字段。
- 已有 `repository.RowQuerier` 与局部 `*Tx` 方法，尚无全仓 DBTX 传播器；utils 的 `postgres.New` 负责连接池而非业务事务。`OrgRepo.Delete` 已锁组织并检查子组织、成员、未结工单，未检查 `org_roles`。
- `internal/middleware/jwt.go` 的 `JWT` 已按黑名单、disabled、密码纪元即时拒绝旧会话；disabled 分支是 403+30003。UI `src/common/request/src/index.ts` 的错误拦截器只在指定 401 码清会话；`src/store/modules/user.ts` 的 `resetState` 已完整清菜单/路由/页签/query cache，单标签已有 session generation 与刷新单飞。F38 是合同未收敛，不是撤权失效。
- 主仓 `internal/gateway/gateway.go` 的 `Registry/New/Mount` 在装配时从配置构建不可变代理；业务仍依赖 app/Wire、router、资源注册和菜单绑定。已有代理不等于新模块无需改核心。目标是一次性接入受控扩展接缝，此后按合同与配置升级，不引入 Go 动态 plugin、任意目标代理或私有 SDK 进入主仓依赖。

## G01：任务、工作树与运行资源独占

**输入与产物。** 每个任务创建本地 ignored 的 `task-manifest.json`，包含 `schema_version=1`、随机 `owner_id`、任务号、操作者、创建/到期时间、各仓绝对 worktree 路径/branch/base_sha、允许写入的路径集合、集成负责人，以及资源账单。资源账单逐项记录 compose project、容器/卷/网络标签、数据库名、Redis 实例、对象存储 bucket、端口、合成账号范围、临时目录、日志路径。凭据只记 secret reference，不记值；任务号不能直接充当 shell/SQL 标识符。

**执行合同。** 一任务一分支一 worktree；跨仓修改每仓独立 worktree，候选组合归集成负责人。创建前检查分支未被占用、目标目录不存在、端口未监听；创建后校验 `git rev-parse --show-toplevel` 与清单一致，再为该 worktree 装 hooks。Git worktree 隔离索引和工作文件，但共享对象库/部分 Git 配置；不承诺隔离所有仓库级操作。fetch 可读，共同 remote/历史维护仍由集成负责人协调。

运行栈不得使用共享 dev/zt 的 PG/Redis/卷/测试账号；单独 Redis 实例优于只换 DB 编号。容器、网络、卷必须有 owner label，绑定 loopback 端口。既有 acceptance/globalSetup 种子账号例外只限该私有栈，其余人工场景用专属账号。启动和停止各保存共享资源指纹（ID、image、start time、mount/labels），停止只按本次清单与匹配标签操作；owner 不符即拒绝。禁止全局 `docker system prune`、按模糊名称删除和“先抢默认端口再说”。

**提交与恢复。** 提交前比较 `git diff --name-only`、staged 集合与允许路径，主仓用现有 `zz-commit.sh`，其余四仓先用等价显式路径形态，随后核 `git show --stat`。G01补包装器精确集合/路径边界，G06将通过反例的版本同步给适用仓；未同步不能调用不存在的脚本。hooks不能代替worktree。集成者按提交SHA cherry-pick，冲突时核实归属，不把另一任务工作区打包提交。任务中断保留清单/日志/首次失败；恢复先核owner与实际进程，不重置他人工作区。清理证据最后写cleanup_complete，漏项必须逐项列为运行资源未收尾。

**实施切片。** G01.1 定清单 JSON Schema 与本地创建/验所属检查；G01.2 为现有 acceptance/UI runner 提供参数化私有 profile（保留原命令入口）；G01.3 精确清理与崩溃接管演练。出口：两任务并发修改同名路径、提交及测试互不卷入；故意写错 owner/占用端口/清理共享卷均被拒绝；kill runner 后仅凭清单可查明并清理本任务资源。

## G02：main 合入纪律与例外

所有者已确认五仓统一 PR 合入 main、允许单维护者在检查通过后自行合入。目标禁止 force push/delete、要求最新目标分支组合检查、解决 review conversation，不强制第二位维护者批准。本轮记录ENG-D01a已定方向，G02实施时显式同步 UI 等仓“低风险直提 main”的旧规则及远端保护，当前尚未执行这些设置变更。

启用顺序：先在普通 PR 实跑下节 jobs，记录准确 check 名称和产出 GitHub App；再设置 main required checks 为唯一聚合 `quality-gate`（指定 Actions App 来源），保护 force push/delete 与管理员绕过；最后用一个故意失败 PR 证明不能合入、一个全绿 PR 证明可合入。required job 始终创建；路径分流在 job 内计算，不在整个必需 workflow 顶层用 paths 过滤。`quality-gate` 使用 always 执行并检查每项结果与清单：缺失/取消/应跑而 skipped 都失败，真正不适用要有机器可判的理由。GitHub 把 skipped job 视作成功，故不能只依赖平台绿灯。[GitHub checks 语义](https://docs.github.com/en/pull-requests/reference/status-checks)、[过滤陷阱](https://docs.github.com/en/pull-requests/how-tos/merge-and-close-pull-requests/troubleshooting-required-status-checks)。

紧急修复仍建 PR；推荐仅仓主可对指定 PR 使用一次性 bypass，收据记录原因、差异 SHA、缺失门禁、风险、操作者、24 小时内补证期限和回退 SHA。安全泄露/服务不可用可走该流程，不允许“改文档”成为永久裸推白名单。超期阻止下一次普通发布而不撤销已恢复的服务；取消规则或扩大 bypass 另有审计。主分支保护配置先导出 JSON，失败则还原；保留已开启的 secrets/push protection。漏洞告警与自动依赖 PR 单独启用，自动合并默认关闭。

## G03：固定组合、具体 jobs 与扩展兼容门禁

### 2026-11-15实际入口与最小接线

下表按当前五仓main源码核对，替代上方11/11快照中的“当时尚无当前扫描/直接插分支”等现状判断；历史设置快照仍仅属其时点。这里只确定实施接线，不表示新工作流/远端保护已启用，也不重复已交G07A/B。

| 仓库与基线 | 已有受控入口 | G01–G04/G06补的接缝 |
|---|---|---|
| core `610c1d76` | `.github/workflows/ci.yml`的commit-policy/lint/test/test-integration/security-current；`helper-tools.yml`的helper-security；Make现有lint/test-unit/test-integration/test-cover/acceptance | 保留命令与job身份；新增固定五仓组合/contract与quality-gate汇总，组合验收准确调用现有四档和迁移roundtrip。当前security-current已扫当前源/实际Linux ELF，不再称main没有当前扫描 |
| UI `875df0bf` | `ci.yml`的commit-policy/lint/test；package.json的lint/typecheck/test/audit:prod/build/test:e2e与gate；`scripts/codegen.sh`、Playwright | CI分支输入已走env，不再复述直接shell插值缺陷；但sibling仍“同名否则main”，须改候选manifest准确SHA。现CI不跑真实E2E；增加组合栈旅程，不删除本地六项门禁 |
| AL `15e93682` | `ci.yml`的commit-policy/test，后者现含race及真实PG integration；Make lint/test/test-integration/build | 接同一候选组合和AL授权/效果/源事实合同，不能拿普通go test替代监控写面/receipt/inbox链 |
| TR `b0906b83` | `ci.yml`的commit-policy/test，含race、真实PG覆盖与make e2e；Make lint/test/build/e2e | 复用回调/cron/e2e，补C03新完成语义、稳定slot和组合contract，不再加第二队列或同名重复测试 |
| utils `de3cd0a6` | `ci.yml`的commit-policy/test及版本矩阵/race；Make lint/vet/test/build | 对支持最低/推荐Go及真实签名/JWT消费者组合验兼容，不造空Swagger/Wire；新claims方法须保旧调用者 |

先保留这些真实job名，把policy/static/unit/security/contract作为**逻辑检查类**映射，新增一个总是创建的quality-gate；不为统一名称重命名所有仓再维护双套required checks。汇总读取各适用结果：必需项success才绿，失败/canceled/缺job不被success吞掉；被严格路径分类证明不适用的项记录原因，混合/规则/协议/测试改动不能走普通文档豁免。required check配置只在新job于准确PR SHA成功/故意失败都实证后启用，仍遵单维护者PR自合原则。

UI当前codegen已通过ZHUZHAO_REPO取主仓Swagger，优先复用这条显式路径；G03让CI按manifest checkout该SHA后设置它，生成diff失败，不用同名分支回落。主仓lint已有Swagger、CI已有Wire新鲜度，不能重复计一套。Go source/binary漏洞与UI生产依赖检查各自保留证据范围；旧`go-security.yml`历史source-evidence不是新组合四仓全覆盖证明。

**私有运行profile的准确边界**：G01候选`task-run`包装入口接受manifest后先preflight，再start/verify/cleanup；当前并无该包装器。运行前验证五仓realpath/base/allow_paths、无他会索引、owner标签和端口，secret只经引用解析到本机0600临时配置。现acceptance可接BASE_URL/PG_CONTAINER/REDIS_CONTAINER/ROUNDTRIP_DSN，roundtrip已要求自定义PG显式DSN，复用其解析和临时数据库，不改成shell拼SQL/URI。缺任一私有配置即拒绝，不回落共享dev。

UI当前Playwright的baseURL可配置，但webServer探测仍固定4000、reuseExistingServer=true；仅设置E2E_BASE_URL不足以证明隔离。G01须把server端口、探测URL、是否复用和Vite API/AL代理从同一manifest显式供给；先核实际VITE_PORT和现有代理配置再装配，缺项拒绝。e2e/global-setup的种子号例外仅限自有栈；不能用自定义UI端口掩盖后台仍连共享33333。包装器证明API/PG/Redis/对象存储均属本owner后才允许运行现有完整命令，停止只按清单与一致指纹回收。

**验收卡ENG-01–08（待实现）**：双worktree/索引互不卷入；错误owner/端口/路径不启动也不清理；kill后按manifest恢复并保首次失败；UI端口改而代理漏改会被预检拒绝；准确SHA不存在/同名回退被拒；quality-gate在失败/canceled/缺必需job时红且确实创建；生成物漂移及新旧adapter语义反例会红；清理本owner资源后共享容器/卷/账号指纹不变。现`test-acceptance-isolation.py`是命令替身黑盒，能证明脚本路径/环境约束，不能替代双私有真实栈演练；实际gate/e2e和恢复证据仍按源码触及范围交。

### 候选 manifest

目标受控位置为主仓 `configs/compatibility/candidate-manifest.json`，本轮**不创建**。唯一 `schema_version` 控制解析；未知主版本拒绝，未知可选字段忽略但保留摘要。字段必须包括：

- `combination_id`、`created_at`、`previous_accepted_id`；五仓 `repository/full_commit_sha/module_version/source_tree_digest`。模块 pseudo-version 须解析回同一 commit，禁止浮动 branch/tag、“同名分支否则 main”和本机 go.work 偷换依赖。
- `toolchains`（Go/Node/pnpm/扫描器/生成器精确版本）、`runtime`（OS/arch/CGO、依赖 image digest、配置模板 digest、migration tree digest、兼容 schema 范围）、`artifacts`（构建输入摘要、binary/OCI digest、UI bundle digest、生成 API schema digest）。密钥/私有 endpoint 不入 manifest。
- `contracts`（public API、callback、intent、feed、extension manifest、SDK 版本和消费者集合）；`capabilities` 与最低/最高支持版本；`expected_subscriptions`（域、唯一订阅名、delivery 类别、feature flag/profile）。core N/N−1 的 N 指发布支持代际，不是第几个任意 commit。
- `extension_receipts` 仅公开合成扩展 ID/合同摘要/回执摘要/签发环境/有效期；私有源码 SHA、真实路由和凭据仅内网清单。公开记录可用不透明制品摘要关联，不能为了“完整矩阵”上传内部源码或配置。
- `evidence`（job/run/source SHA、suite、用例分母/执行/skip/retry、首次失败、mock/contract/real 类别、结果、日志摘要、cleanup）；`waivers`（精确 case/advisory、理由、风险、责任者、到期、补证项），空泛“已验收”字符串不合格。

PR 读取 base 的已接受组合，以本 PR event SHA 替换当前仓并明确登记其他候选提交；生成新 combination_id。CI 校验所有 revision 可获取且不可漂移。跨仓未推提交的本地证据只能是 local receipt，不能伪装远端通过。文档候选采用独立验证档的方向已由ENG-D01b确认，实施与启用仍归G03，详见本页验证档合同；若以后另建runtime收据复用，仍必须证明受控输入/套件/环境/期限适用、记录原producer和目标SHA，不能移花接木。

### 作业集合和资源预算

所有仓push main、pull_request覆盖policy（现有commit-policy、路径归属/清单/文档链接、workflow负例）、static、unit、security、contract逻辑类及quality-gate汇总。具体job名复用上表，先不重命名；若以后确需改名，先验证新job再更新required check，不留互斥双规则。Go主仓保integration，AL/TR保真实PG，UI保Vitest，utils保race。每job设timeout，单机Go并发预算GOMAXPROCS=2、集成-p 1起步，按实测调，不为追求并行耗尽PG/内存。

- `policy`：5 分钟；读取事件 JSON，不能将 branch/title/body 直接拼进 run 脚本。分支先通过 env 传递，`git ls-remote/fetch` 用显式 ref 和 `--`/固定 refspec；恶意引号、换行、`$()` 仅作数据。checkout 禁持久凭据；默认 `contents:read`，fork 不取内部 secret、不运行高权限 `pull_request_target` 的 PR 代码。[GitHub 安全边界](https://docs.github.com/en/actions/reference/security/securely-using-pull_request_target)。
- `static`：Go lint/build/Wire/Swagger（主仓）；UI lint/typecheck/build/codegen 输入 SHA；utils 不生成空 Swagger/Wire。生成后有受控 diff 即失败，保存 diff 不自动提交。每个仓 profile 声明适用项。
- `unit`：已有入口，保留独立单元覆盖率分母；新 mock 交互状态亦在此，执行时禁止意外外网请求。
- `integration`：独立 PG/Redis/对象存储，原业务用例不删；预估主仓 30 分钟上限，基础设施等待、业务失败、清理失败分码，不反复自动重试掩盖 flaky。一次重试必须保留两份结果。
- `security`：沿用官方 scanner 证据解析器，支持 main/PR 及每周运行，跟随 manifest 的当前四 Go 模块；分别保存可达调用/仅模块发现、scanner 错误与未覆盖。可达高危阻断；module-only 必须定期到期复核，不能写作没有漏洞。UI `audit:prod` 保留；依赖锁、镜像/Action immutable digest、SBOM/制品来源随发布固化，G05 负责备份发布运维的其余实施，勿在 G03 重复计价。
- `contract`：合成 consumer/provider fixture、Swagger/codegen、callback/intent/extension/feed 版本矩阵；验证启动所需订阅集合。不能只读文档或数 `SubscriberCount`：给 `HookRegistry` 增加只读“域+订阅名+交付类别”快照、装配后对声明集合 fail-fast；重复注册继续失败，关闭 profile 的订阅显式不适用；缺 `ticket.notify/inbox.notify/ops.notify/inbox.ops` 等启用项必须启动失败。用真实装配测试删一项验证其拦截作用。旧有进程 Hook 标为 best_effort，不因名字存在被认作 durable。
- `full-stack`：每个可交付候选组合必须跑原样主仓 `make acceptance` 四档全链、迁移 up/down 与 schema 漂移核验，以及 UI 六件等价链和全部现有真实 E2E；各 owner 独立，可共享只读镜像缓存，不共享数据库/账号/端口。超出公共 runner 能力时，由维护者调度隔离 runner，回执绑定组合、实际二进制摘要和 suite 原脚本摘要；`quality-gate` 验该回执，不能只手填绿色。此前本轮 253 acceptance/81 迁移、UI 42 E2E 的结果只作原实现基线，未来数目按实际 suite 清单，不硬编码为永久分母。
- `scheduled-risk`：每周完整组合＋漏洞扫描＋长 fuzz，每月恢复/升级回退样本（G05 负责真实恢复能力）。PR 短 fuzz 与变更域性能 smoke 由 G04 接入；定时绿不能替代新候选真实门禁。

PR cheap/contract 必跑；完整门禁可由同 SHA 的组合回执供给。ENG-D01b已确认采用独立文档/原型档；旧回执复用另有更高输入证明成本，不作为首选。采用新档前须明确修订AGENTS等对应条文，不能通过路径规则暗中豁免。真正源配置/迁移/fixture/spec 变化一律新跑。合入后 main 重新跑 cheap/contract 并确认合入树与候选组合相符；不同树重新生成候选。

### 自行打包的版本与内网离线制品

版本选择不要求与公司环境统一，也不再受“main之后才升级”限制；[依赖现代化设计](dependency-modernization.md)拥有具体版本与官方支持周期，本页只定义实施验收。目标单一版本源声明Go/Node/pnpm、PG/Redis/对象存储、基础镜像、Action与生成/扫描器版本；go.mod/packageManager/CI/container配置通过机械一致性检查关联该源，锁定精确patch或digest，不使用latest。每月固定patch评审/升级候选窗口，已确认可达高危另开紧急窗口；major/minor先出兼容实验PR，再按组合发布，不能以“内网已有旧版本”为永久冻结理由。

首个入网开发/试验包只选一个目标平台，带固定运行制品、依赖锁/校验、配置/迁移、mock/合同验证说明及所选adapter开发所需依赖。需要内网重建哪一仓才准备其工具缓存；不先要求全五仓、多架构、全部生成器和SDK的离线闭包。正式发布的SBOM/签名与自动化来源链由G05/P7按实际发布政策扩充；公司强制要求的制品准入在相应真实使用前满足。联网构建与离线运行分开，缺失必要制品/校验失败须显式报错，不静默回外网。私有adapter及真实SDK留内网，公共CI只用合成材料。

版本升级先验实际登记的当前核心/消费者/依赖组合及当前交付目标；选定继续支持的旧代际再加入对应兼容格，不预设全部N/N−1笛卡尔积。真实需要保留的数据仍要验证升级/恢复：PG major不能靠旧镜像挂新目录回退，Redis/对象存储沿官方兼容路径；测试栈可重建不等于真实运行数据可丢。G05复用已做恢复回执并补所选形态，不让单测/健康检查冒充数据库恢复。

### 公共和内网两套执行环境，同一合同

首版最小矩阵为实际五仓固定组合、当前公共合同、一个参考adapter/私有应用及一个入网目标；公共CI与内网CI沿同一合同case IDs验证各自已启用的方向。当前尚无正式部署/历史消费者，不强制构造core、SDK、adapter、feed各两代的全部组合。G05/R07在P7复核实际消费者及需要继续支持的代际；首次承诺第二代支持前补相应兼容格，真实不适用要写明。私有SDK不进核心依赖，厂商变化由对应connector处理。

每格按已启用capability验证适用方向、游标提交与断点续拉、乱序/重复/重放去重、身份映射/撤权、超时/限流/部分失败、撤销/停用和声明支持的回退。只读HR/CMDB源不要求开放回写；有回传的提供方分别验证取数/回写授权与receipt，不以单向通过代替另一方向。当前未正式部署可协调替换旧组合，不能因此省略数据/在途盘点。合成 mock 不是一次性替身：开发、展示、大数据量、错误态持续使用；contract 检测 fixture/schema 漂移；real 证明网络、凭据、权限和真实语义。没有真实 API 契约/沙箱的格标 `real_pending`，可完成设计/公开契约施工，不能批准生产切换。

扩展 manifest 引用根页定义的 schema/注册/导航/路由字段；本页另要求 receipt 含 `schema_version`、发布/兼容范围、提供者来源 digest、授权 scope/主体、出入站目标 allowlist 的摘要、资源预算/timeout、撤销版本和 last-known-good 版本。过期/未知 schema 主版本/已撤销/越权能力/不支持的组合拒绝启用；断路器按扩展隔离，单个扩展坏不得令主平台启动失败，**核心强依赖**才 fail-fast。撤销应关闭新调用，已发命令进入可对账状态；不能把未知外部效果标取消成功。核心升级 expand→合同验证→小流量→收据→切换，回退时保留新写事实与 intent，不做破坏性 schema down；若 N−1 不识别已启用能力则先停该能力和新提交，维持兼容适配层完成在途，不能硬降并丢事实。

## G04：mock、正确性测试与第一批 fuzz

先执行[standards §11](../standards.md#11-开发工作流先框架--先测试--分批实现)的现有测试先行顺序：当前批准范围的预期/全量用例→因目标行为失败→分批实现→完整门禁绿。G04的风险—测试对应表记录业务合同/不变量、case ID、真实调用入口/数据面及红/绿证据；实现与独立验证分别消费同一预期，不能由实现结果反推“正确答案”。编译/环境失败不算目标红例，测试/门禁削弱在[短审阅卡](../phase6/implementation-handoff.md#单维护者短审阅卡)显影。并发至少8次稳定红、UI守卫验牙、准确快照和失败/skip披露继续按原规则。

单元覆盖率、带 integration 标签覆盖率、关键合同 case 覆盖和浏览器旅程分开发布。主仓默认 `test-cover ./internal/...` 的 20.4% 只是原快照默认标签语句口径；不把未进分母的 integration 或 UI 算进去，不靠镜像实现的测试凑 80%。新增变更要求关键失败支路有证据，包覆盖率不退化需解释；项目级阈值先采两轮基线后定。

G04 第一批目标和预算均为建议实施参数：

- `FuzzExprReferences`：exprcalc AST 引用分析、局部变量、函数同名、输入隔离；不访问 DB；PR 20 秒/目标、每周 10 分钟。断言保存与提交使用同一 analyzer、原 map 不变、公式值注入不影响输出、非法引用恒拒绝；随机表达式必须有节点/长度/容器预算，不能拿无限算式拖死 CI。
- `FuzzAKSKCanonical`：utils `aksk/aksk.go` 的 `Sign/Verify` 与 `aksk/gin.go` 的 `Transport`；PR 20 秒、每周 10 分钟。固定已知向量+方法/路径/体/签名头扰动；验证支持的协议差异，不能假设当前 query/nonce 一次性已签/已消费。签名协议升级由兼容合同另管，不在 fuzz 中悄悄改协议。
- `FuzzFieldSchema` 与 `FuzzImportRows`：主仓 `validateFieldInputs/validateCustomData` 的纯校验投影、AL import 解析/大小限制；PR 20 秒、每周各 10 分钟；数据库操作用定向 integration 测原子性，不让 fuzz 每样本开库。
- `FuzzOccurrenceKey`：TR `ParseSpec/NextRun` 及 C03 新稳定时隙键；PR 20 秒、每周 10 分钟。同 job/revision/UTC slot 恒同键，clock 回退/DST/多副本不生成第二个业务 occurrence，边界区间合理。

先做以上五目标，PR 最多串行 100 秒 fuzz，`-parallel=2`；每周总 50 分钟可拆 jobs，不泛开全函数 fuzz。失败 corpus 随修复纳入 `testdata/fuzz/<target>`，原始 crash/seed/Go 版本/复现命令保留；普通 go test 继续跑种子，不做“删坏种子使绿”。Go 官方说明失败输入会成为普通测试的回归种子：[Go fuzz 教程](https://go.dev/doc/tutorial/fuzz)。

并发与恢复用确定性屏障而非随机 sleep：C01 同名/旧公式值；C02 同对象竞争绑定/事务回滚/GC 并发；C03 lease 过期旧写者/response 丢失/DB commit 效果未知/cron 双副本；R01 业务提交后 kill 与消费后 ack 前 kill；C05 撤权+迟到刷新+换用户。mock 对应 case 保留同 ID；刻意删一次授权/完成校验，门禁必须红，防样本只验证 happy path。

性能首批只做本平台接口：有界列表/筛选、并发 ticket patch、公式最坏合法输入、附件引用校验、intent backlog 排空与 callback。用合成 1万/10万记录、固定硬件/并发/依赖版本采 P50/P95/错误率/PG 连接等待/内存/队列龄；先记录基线，回归超过 P95 20% 或稳定内存增长 20% 进入人工解释，初轮不凭未经测量的绝对 SLO 拦截。E08 拥有规模目标，G04 只提供工具和样本，禁止把 AL 变事件湖或 TR 变 SOAR 压测平台。

## G06：仓库规格与规则同步

公共机械规则从主仓发布版本化 bundle，首批仍各仓存副本，不新增规范服务/monorepo。目标 `scripts/policy-bundle.lock.json` 含 upstream repo/full SHA、bundle_version、每文件 SHA256、适用 profile/本仓例外；升级脚本先下载到临时目录验 digest，显示 diff，以显式路径 PR 更新。CI 对版本与哈希漂移报错，不执行未审查远端脚本。长篇 standards 只链接主仓 SSOT；本仓保留特有职责，不复制成五份互斥公约。

新仓 profile 必选 `go-service/go-library/ui/contract/private-connector`：共同必需 README、AGENTS、LICENSE/贡献与安全报告入口、docs/design/guides/ops/review 按实际材料建立、CI、hooks/commit script、依赖锁、合成配置示例/secret reference 说明、支持版本/维护人/生命周期、兼容 manifest 和 contract fixtures。service 增 `cmd/internal/config/ops` 实际入口与健康/关闭/迁移（有持久库才要 migrations）；library 公共包在根层、禁止业务表与服务编排；ui 保留集中请求/生成类型/路由/测试；contract 可只含 schema/fixtures/验证器，不强塞 Go；private connector 独立服务和私有运行说明，不 import core internal、直写 core 库或复制权限判定。没代码/没职责的目录不建。

ADR 记录跨仓授权、可靠交接、扩展版本、provider切换、破坏性迁移等重大选择；保留 supersedes 链。文档链接/符号与合同 digest 检查随 PR；Q5 历史注记与 L 组未决在 G06 固定窗口逐项采纳/替代/退出，入口指回唯一总账。已完成目录搬迁不重演；新增仓先用一张“谁拥有数据/权限/恢复、为何不是模块”的开仓卡，通过具体交付证明独立边界。

### G06语言与机械文件profile

当前四Go仓的Makefile均已有gofmt/vet；UI的`eslint.config.mjs`使用skipFormatting、只匹配src的TS/Vue逻辑，package.json无formatter检查命令。因此“已有lint”不能证明五仓代码排版/样式一致。跨语言一致指相同命名/边界/错误/文档原则与可重复检查，不强迫Go和Vue使用相同缩进。

| profile | 推荐规范与检查 | 边界与实施出口 |
|---|---|---|
| Go（主仓/TR/AL/utils） | 保留gofmt与vet，按各自最低/推荐Go版本执行；新增目录遵现有包命名与依赖方向 | 不为统一名义换另一套formatter；生成物由钉版本生成器核新鲜度，手写与生成分开 |
| UI TS/JS/Vue | 保留ESLint/vue-tsc；G06新增钉版本的独立Prettier格式检查，候选2空格、单引号、无分号；在本批改动文件先启用 | 核ESLint排除项逐项原因，不将种子TSX长期排除自动扩大；不把类型检查当组件/授权测试，不顺手全仓格式重写 |
| UI Less/CSS/Vue样式 | 同一formatter管格式；语义引用U02的tokens/状态/布局规范，新写样式优先token而非复制色号 | 先检查改动文件及少量token硬编码反例；只有重复实际缺陷才增样式lint规则，不先引一套无使用者的样式平台 |
| Shell/Python公共脚本 | 按shebang选sh -n/bash -n；Python AST/编译语法检查使用隔离cache，现有提交策略黑盒继续执行；LF/UTF-8/末尾换行，Python4空格，shell沿现有脚本约定 | 语法通过不证明安全；owner/路径/恶意输入反例归G01/G02，不执行刚下载的policy脚本。格式化工具另经收益验证，不一次新增多套依赖 |
| YAML/JSON/Markdown/Make | YAML/JSON解析+schema/目录来源检查；工作流注入/权限归G02；Markdown本地链接/围栏/符号；Make recipe保留tab | 同步EditorConfig按文件类型；JSON/代码生成清单保留自己的canonical序列化，不用全局formatter破坏hash或制造巨diff；配置解析不等于真实环境生效 |

Prettier仅作为推荐开发依赖，实施前核目标Node兼容和准确版本，不声称当前已装。采用独立格式检查与现有质量lint分工，避免双重格式规则冲突，参见[官方说明](https://prettier.io/docs/integrating-with-linters)。本轮未新增依赖或格式化源码。

bundle记录profile名称/版本、配置hash和例外理由/复核窗口；每仓公布fmt-check/lint/docs-check等实际入口。CI读取真实diff按适用profile检查，新仓必须有一个应通过与一个应拒绝的最小例子；不能靠复制同名配置证明生效。G06/P6.1出口是五仓真实文件各跑适用检查并拒绝故意格式/解析/边界反例；新增profile与规范仍由同一SSOT维护。

## C01：表达式引用、作用域与输入隔离

目标入口建议 `AnalyzeExpression(src, schema, policy) → Analysis{free_fields, calls, policy_version}`，位于主仓 exprcalc；此名字尚未实现。`CheckSyntax/Identifiers/ApplyFormulasContext` 与 ticket 的 formula/visible_when/transition validator 逐步调同一分析器，但各用不同 policy，不能把公式禁链规则误施到转换的系统字段。合法纯函数、`lookup_dict`、`let` 局部变量保留。

分析不能再“收集全部 identifier 后按字符串减 callee 集合”。递归遍历 AST 并保留**节点角色与词法作用域**：CallNode 的直接 callee 仅按函数注册表解析，参数按值引用；变量声明的初始化表达式先在外层作用域解析，其 body 再扩展局部绑定；闭包参数/指针节点按语言语法绑定；成员下标的对象/下标仍访问，禁止 `$env`、私有 context 槽及未声明宿主访问。值位同名 `upper` 与调用位 `upper(x)` 各自解析，只有真正局部绑定才豁免。未知 AST 节点默认拒绝而非自动认作安全；将 pinned expr v1.17.8 的全部支持节点列入测试快照。[Expr visitor](https://expr-lang.org/docs/visitor) 提供 AST 遍历机制，不替项目决定词法权限。

formula policy：自由字段必须存在且是合法普通输入，非formula（包括自身），链式/环式公式仍拒绝；visible_when/validator按原可引用集合分别检查。最终公式环境从schema投影非formula输入，删除请求/历史公式键及显隐prune后隐藏键；各公式读同一只读环境，输出独立累积。**visible_when当前允许引用formula，不能因隔离输入取消它**；准确一次快照算法见下方，不信客户端伪formula决定required跳过。类型、required、非有限数、JSON预算及字典disabled兼容保留，字典用当次tx/context，不缓存值。缓存纳入policy/schema引用摘要/函数注册版本，不含ctx/q/数据；纯计算无context硬抢占保证，节点/容器预算按最坏合法输入测量。

迁移不直接改旧配置：先只读扫描当前实际ticket_type_fields/transition_meta；D01实现发布/草稿快照后才纳入，不能冒称当前已有。报告新增拒绝原因，无问题配置不改值。坏项仍可读，仅拒实际触及该表达式的新保存/提交，评论/标签等无关操作不一刀切；下一次合法相关写才重算，不批量改历史。报告含类型/version/字段或转换/purpose/表达式hash/分析器版本/前后分析结论/reason，不求业务值、不导全文或执行字典I/O。语法/业务校验仍现400/ErrInvalidParams，DB/context故障另分，不伪装未命中字典。

C01.1 节点角色/作用域分析＋正反corpus；C01.2 保存/提交共核及输入隔离；C01.3 旧配置只读扫描。出口：upper(x)+upper值位formula引用不能借callee同名消失，let x=base; upper(x)保合法；在固定时钟/相同依赖快照下，请求伪值、旧值及字段排序不决定结果。合法now()/lookup_dict本身依当前时间/数据，不能宣称跨时间恒定。缺字段、词法嵌套、字典取消、保存/提交/转换/显隐分别验，不以禁全部函数逃避修复。

### C01准确接线与一次快照显隐

基线`610c1d76`、Expr v1.17.8。现`POST /api/v1/ticket-types/fields/replace`保留ReplaceTypeFieldsRequest的code/version/fields、field_meta.formula/visible_when字符串及成功warnings。`internal/service/ticket/type_admin.go`的`ReplaceTicketTypeFields→validateFieldInputs→checkFieldReferences`负责保存；`service.go`的Create/Update→validateCustomData→applyFormulas负责提交；`transition.go`的`buildValidatorEnv/checkValidator`供转换。当前router无公开formula preview API，D04需要时才消费同内核；validator保priority/title/status/type_code/custom_data上下文，不套formula禁链到全部系统字段。

`internal/pkg/exprcalc/exprcalc.go`保留CheckSyntax/Identifiers/ApplyFormulasContext旧入口，由内部分析结果提供自由字段/调用/原因/policy版本。解析与compileOptions共用disabled/functions快照，不能无配置parser.Parse与DisableAllBuiltins编译两套名字语义。Pinned ast.Walk为后序visitor，选择显式递归；VariableDeclaratorNode先外层Value、再加Name分析Expr，原checker禁止的重声明/函数遮蔽不放宽。CallNode直接callee按注册表，参数按值；静态Member字符串属性不是字段，对象根/动态下标/slice边界继续分析。Pointer/Predicate仅原合法域，不由支持分析开放已禁builtin/闭包/宿主方法。未知节点拒绝，字面字符串$env不误伤。MaxExprLen=2048实际为**UTF-8字节数**，文案须校正。

显隐采用有限一次快照：普通schema白名单输入形成visibility快照→仅为visible_when实际依赖的formula按需算只读服务端值→普通快照＋派生值判hidden→prune→用prune后的普通基础环境重算可见formula最终产物。客户端/历史formula值从两环境隔离，不被显隐依赖的隐藏formula不求值；禁链使其无需通用DAG/反复求不动点。缺合法依赖沿原保守可见/更严校验，基础设施失败另报依赖错误，不能用伪值/空label隐去required。此行为纠正列扫描报告，保留合法formula显隐能力。

领域层从完整当前schema及已启用字段写资格投影输入；通用ApplyFormulasContext只有公式列表，至少删其全部公式输出键，不能假称知道完整领域字段。每公式读同一不可变基础，输出不喂后续公式；入参map/ctx/tx不改，未知存量键可在原兼容载荷保留但不入env。隔离求值不等于顺手清洗整单数据，最终产物仍验type/regex/required/有限数和JSON预算。

`internal/service/ticket/exprfuncs.go`的withFormulaDictLookup保留formula读取停用项历史标签；withDictQuerier显隐/validator按原enabled规则。真正无行与DB/读完成错误/取消/超时分开，后者不伪报未命中；现TestExprLookup_ReadCompletionError文案预期随分类纠正，零成功值不变量保留。编译缓存只存程序/分析元数据，按policy/schema/function generation隔离，注册冻结，旧generation不装新key。优先用Expr MaxNodes与VM MemoryBudget，后者不是Go堆字节；2048字节、现64KiB JSON/原deadline保留，节点/深度/字典次数/range预算用最坏合法样例冻结，不冒称所有者SLA。

**C01-01–10待新增**：callee和值同名；let initializer/合法作用域及非法重声明；静态/动态节点与沙箱；保存/Create/Update/validator共核且可引用集不同；伪/旧formula隔离与顺序/入参不变；显隐服务端派生/hidden不复活；字典tx/disabled/故障分类；schema/policy/generation与并发ctx隔离；只读扫描及相关写hold；长度/节点/range/字典/JSON与fuzz坏种子。移除修复须使同名漏检、伪值或ctx后台化反例有效红，沿原至少8轮纪律。FuzzExprReferences仍PR20秒/周10分钟，全批五目标PR100秒/周50分钟，不另加全仓fuzz。

复用`internal/pkg/exprcalc/exprcalc_test.go`的TestEvalFormula_Basics/TestEvalFormula_Guardrails/TestApplyFormulas/TestApplyFormulas_RejectsInfNaN，`internal/pkg/exprcalc/context_test.go`的TestContextFunction_RuntimeIsolation/TestContextFunction_SandboxAndCompatibility，`internal/service/ticket/b17_semantic_integration_test.go`的TestB17_FormulaRefs/TestB17_FormulaDrift，`internal/service/ticket/formula_integration_test.go`的TestFormula_Fields，`internal/service/ticket/b16_batch2_integration_test.go`的TestB16_LookupDict_Formula/TestB16_FormulaVisibilityAndOutputValidation，`internal/service/ticket/expr_context_integration_test.go`的TestExprLookup_ReadCompletionError/TestExprLookup_UpdateReusesTransaction/TestExprLookup_CompiledProgramDBAndTransactionIsolation。本轮未执行这些未来反例；扫描若发现真实配置依赖旧不可信结果，给具体场景入集中议程，不自行放回旁路。

## C02：文件引用与对象生命周期

主仓 `StorageRepo.BindCreateAttachments` 已校验 bucket/key、上传人、confirmed、暂存用途并锁对象，`StorageService.Download` 仍按真实 owner 工单鉴权。目标修复是写侧语义闭合，不把任意 key 写入等同已证实下载越权。

统一操作建议 `ReconcileTicketFileRefsTx(tx, ticket, actor, oldRefs, newRefs, explicitAttachments)`，由 ticket service 拥有，storage repo 只执行参数化 SQL。输入来自已 G2 校验的 schema file 字段、richtext 稳定 URL 抽取和显式附件列表，按 bucket/key 去重；不接收调用者自报 owner。Create 与 Update 使用同一规则；Update 在锁内新鲜基线上 merge/重算后抽取，禁止事务内再向 pool 读 schema。只改 title 时不重新夺取所有旧附件所有权。

- 保留引用：对象confirmed且bucket/key→ticket_attachments当前票关联成立可保留，不要求编辑者是最初上传人。owner_kind/owner_id只是可空元数据，不作归属授权依据，不能因旧行NULL拒绝合法关系，也不能仅凭其票ID接受。不一致旧数据报安全原因，不静默抢外单对象。
- 新增引用：当前操作者上传的 confirmed 暂存 `attachment_create/richtext_create` 可在同事务落绑；已有该工单的正式对象可复用。其他工单、其他上传者暂存、avatar、pending/deleted/不存在对象均拒绝；真实 owner 的读权限不等于跨单迁移权。body 任一引用非法则工单 patch、关系、owner、ticket event 一并回滚。
- 删除字段引用：只移除该字段内容，**不自动删已绑定附件或物理对象**；显式删除仍走DeleteTicketAttachment授权，仍被当前file/richtext引用则409＋稳定reason和泛化提示，不回隐藏字段名/正文位置。ENG-D02已确认保留与显式删除；上传B追加B，不隐式替换A。引用感知自动解链不作默认。
- 锁序统一 ticket→按对象 ID 升序的 file_objects→关系；同 key 跨单竞争只有一个成功，计数去重后再校验。不在 DB tx 内 HEAD/删除对象存储；上传确认先完成 HEAD，失败的 DB 绑定保持暂存供重试/GC，GC 与绑定共享状态条件/锁，不删已绑定对象。数据库失败为 5xx，业务引用非法为 400，已被他单绑定/仍被引用删除为 409；响应不泄露外单标题/人员。

文件字段是工单自定义表单的file控件，不是仓库.md文档。单次可选一个或多个文件，成功项按对象key追加且去重，失败项逐项提示；建单前“移除”只表示不纳入本次提交，误传暂存仍按既有GC处理。已建单“删除附件”是独立明确操作，不能因上传B/编辑字段自动删除A。Markdown/富文本正文只引用图片/文件URL，删除正文引用与删除对象分开；其图片用途/引用回收另按C02对应规则，不能混成同一个按钮。

F23 `attachment_create + ticket_id`矛盾组合先观测并明确弃用，下一次兼容发布后严格400：已有单的**独立附件立即上传**使用ticket_attachment；编辑表单中随保存原子绑定的暂存仍attachment_create/richtext_create且不传ticket_id，不能因已有票就取消暂存流。禁止confirm静默把矛盾参数解释为绑定成功。合法暂存路径保留，所有权限仍经L1与ticket L2/L3，存储通用包不import工单业务。

**ENG-D02供owner理解的例子**：同一工单“整改前截图=A、整改后截图误填A”，改后者为B，只取消该字段与A的对应关系；A仍被整改前字段使用，不能删掉。字段和附件列表是同一个对象的两个入口，不是复制两份。推荐按钮明确区分“从此字段移除”与“删除工单附件”；即使以后最后一个字段引用移除，也保留附件直到显式删除/保留规则处理。当前详情file字段尚无这样的编辑UI，当前附件删除对file引用的完整保护也未落地；2026-11-11所有者已确认保留A/明确删除，并纠正上传B应默认追加；当前实现和删除保护缺口仍待C02/U03落地。

C02.1 盘点旧 orphan/跨单引用、接口用途合同；C02.2 锁内 ref reconciliation 同规 Create/Update；C02.3 删除引用保护、GC竞争及 F23 兼容期。出口：伪 key、错用途、别人暂存、他单正式对象、并发双绑、最后一个字段失败、超额度、保留别人上传的本单附件、清字段保留附件、显式删仍引用对象、GC/回滚均有真实 PG/存储证据；下载权限负例保留。

第二轮另补头像和metadata边界：旧avatar退役须当前用户合法归属，不能因profile任意key引用就删除他人confirmed未绑定对象；保留合法外链兼容。删除附件先核`bucket/ticket_id/key`当前关联，再给richtext用途提示，非法关联与未知同形404，合法本单richtext仍原400。具名`Avatar_RetireOnlyOwnedOldKey/Attachment_ForeignPurposeNotObservable`，已绑定他人头像/跨票下载/他人暂存绑定正确对照仍须通过；不要夸大成已证明可枚举或读取他人内容。

### C02准确wire、存量修复与UI生命周期

基线core `610c1d76`、UI `875df0bf`。**首期不新增请求/读字段或引用表**。`internal/model/ticket_request.go`现Create的custom_data＋attachments不变，Update custom_data仍patch/null删键、attachments增量；未来D02的patch/collect_fields只通过同一内部helper适配，不由C02抢改现wire。引用位置首期不返回；以后若真需要再做受权扩展，不把它当删除保护前置。

`internal/service/ticket/service.go`的Create已合并显式/file/richtext键，Update目前只绑显式附件。共同helper接**验证、hidden剔除及公式处理后的最终数据＋同一schema/definition快照＋tx/RowQuerier**；复用ListTicketTypeFieldsTx与validateCustomData的q，不直接把内部走pool的fileFieldKeys/richtextFieldKeys搬进Update事务。正文继续`internal/service/ticket/richtext.go`的ExtractFileRefs，不加另一套正则。只改title不夺旧附件，现票合法协作者保留同票confirmed对象不要求是原上传人；新增本人confirmed悬挂对象才执行原上传归属/用途约束。

按bucket+key全量命中并锁内验状态、用途、同票关联与MIME，缺项不能因为只查回一部分MIME就通过。派生跨字段/正文的同键去重后只绑一次、计一次额度；显式attachments自身重复仍可保原400合同。锁序接C：授权守卫→适用配置快照→ticket→稳定排序file_objects→关系；ticket patch、绑定、owner/状态、事件任一失败全回滚，网络HEAD/物理删除在外。

删除候选事务：先当前票update资格→锁票/对象→证明(bucket,ticket_id,key)关联→合法本票inline保原400→普通附件检查当前全部真实引用，有引用409→解绑与逻辑deleted同tx→commit后物理删除，失败留deleted供二扫。现`internal/service/storage/service.go`的DeleteTicketAttachment用途先判且Unlink后Mark失败只warn，尚不具此原子性；RECHECK05非法关联/未知统一404，不能先泄用途。引用检查覆盖全部真实字段，但409只泛化“仍被引用”，不回reference_locations；缺定义/解析不确定不得当零引用执行破坏动作。

旧Update可能留下“custom_data引用存在但ticket_attachments缺失”，现`internal/repository/storage_repo.go`的ListConfirmedOrphans/MarkDeleted不能证明已保护这些旧引用。上线前只读盘点，能证明本人合法悬挂/同票来源者受审补关联，归属/来源/定义不可证的hold/quarantine，相关GC不得先回收；不自动认领他票/他人键。已绑inline去引用后自动回收仍归RK-P5-16后继，不暗塞C02。

头像RECHECK04纳入`internal/service/user_service.go`的Create/Update/UpdateProfile及正式BindAvatar共享不变量：内部对象键须合法归属**目标资料用户**（管理者身份不改变上传归属），Create尚无可证目标归属则拒内部key，不回落actor上传归属；旧外链兼容不触发服务端抓取。`internal/repository/storage_avatar.go`的BindAvatar退旧键须目标合法拥有、confirmed/avatar、无其它有效引用；只读到users.avatar字符串不构成归属证明。保留users.version、同键noop及对象锁后新语句复查；不新建代他人上传接口。物理退役在commit后，未知/他人键不泄对象存在性。

UI `src/views/ticket/create/index.vue`的handleFileUpload已有单次追加，补多选逐项状态/成功去重，失败不改A；`src/components/TicketAttachments.vue`虽multiple，当前共享uploading及只比ticketId不足以防A→B→A，补target＋generation、逐项pending/错误、确认前提交锁。`src/components/RichtextField.vue`的onUploadImg当前失败会交空URL，改为只将当前编辑器成功项回调，卸载/迟到不插新目标。去Markdown引用不删对象；明确删除409保列表/字段，结果未知不自动清项/重发。

**C02-01–08待新增**：Create/Update显式/file/richtext/多处同键及patch/null同规；现票协作与新悬挂归属/失效/缺项；额度去重及中途失败零半套；关联先用途及404/inline400/引用409不泄位置；去一处/全部引用仍保附件；旧缺关联hold/合法补绑与GC/物理二扫；头像他人旧键不退役及合法/外链/CAS对照；多选部分失败/重复/换目标卸载/确认迟到/未知结果。并发双绑/GC/回滚按真实PG和既有至少8轮有效红，不拿前端按钮替服务端保护。

复用`internal/service/ticket/attachment_bind_integration_test.go`的TestCreate_TicketAttachmentBind/TestUpdate_TicketAttachmentIncrement/TestCreate_RichtextRefBind，`internal/service/ticket/update_tx_hammer_integration_test.go`的TestBK11_UpdateHammer，`internal/service/storage/gc_binding_integration_test.go`的TestGC_TicketBindingWins，`internal/service/storage/avatar_race_integration_test.go`的TestBindAvatar_GCWins/TestBindAvatar_BindingWinsGC，`internal/service/avatar_profile_integration_test.go`的TestUserAvatarProfileCAS。UI沿src/api/storage/idWire.test.ts的confirm/bind/delete、src/components/useRichtextDraft.test.ts及src/views/ticket/detail/useRenderedRichtext.test.ts借wire/世代夹具，ticket/idWire.test.ts只作相邻票请求补充，不能替代storage wire。当前无附件组件专测，需Playwright真实交互。旧绿色不证明新增保护，保留A/追加B不再待问。

## C04：事务试点、组织删除与默认拒绝

2026-11-15[第二轮现状复验](../review/reports/permission-boundary-recheck-2026-11-15.md)补三个具体授权编辑出口：角色直接绑定回显不能用三源展开（否则保存固化派生grant）；组织集合保留项不能DELETE重插丢任职角色/scope；UI整体替换须当前目标加载成功，失败/pending/迟到响应零误POST。归C04/D06与实际薄UI，优先于正式使用；不因下面DBTX仍为窄试点而把这些本地IAM正确性推至完整UAA。

试点只覆盖 `ticket.Service.Create/Update/Transition` 及其直接 repo 调用。建议主仓内部 `DBTX` 接口包含 Query/QueryRow/Exec，显式参数传递；`WithinTx(ctx, pool, fn)` 管 begin/rollback/commit，仅返回提交后可发出的效果摘要。不得把 tx 塞任意 context、在 repo 自动猜 pool/tx、自动嵌套事务或泛化到 utils 的业务接口。utils 仅在三个服务都证实需要同一种纯机制后考虑下沉；本次不做全仓重写。

service 是事务所有者，tx 窗口禁止第二条 pool 连接；生成外部随机编号、HEAD、provider API 等先在外部完成，可变业务检查与 audit/event/intent 在同 tx 核对。提交后调用非可靠 Hook；可靠动作留待 R01。commit 返回错误可能是“效果未知”，不能 blanket retry 整个函数；有操作幂等键先查询凭证再恢复，未提供凭证的历史写入返回可定位 request_id，不能对客户端暗示无副作用。只读上下文取消可安全退出，已提交操作不伪装回滚。

deadline 采用**试点命令的显式预算**：建议普通写 10 秒、锁等待 2 秒，上传/下载/导出和 SSE 不套全局同一超时；继承更短 caller deadline，tx 语句本地超时不得在连接池残留。用池容量=2 的并发回归证明无嵌套取连接与及时归还；超时后保存提交结果分类，异步意图用独立有界 worker context。超时参数是初始值，G04 测量后允许配置调整，不创造“context 可以撤销外部效果”的承诺。

`OrgRepo.Delete` 在已有组织 FOR UPDATE 锁下新增 org_roles 检查。推荐有绑定则返回稳定冲突码和计数，要求显式解绑后删，避免悄悄丢授权配置；`BindOrgRole` 与所有组织依赖写侧同用 `lockAliveOrgForShareTx` 协议，删与绑不能穿透。虚拟组删除 `DeleteVgWithOwnerCleanup` 单独核同一政策。历史软删组织关联只读盘点并由可审脚本清理/保留审计；不把历史孤儿自动恢复成生效授权。新错误码/迁移在实施时查权威占号，本文不分配号码。

资源护栏复用 `internal/pkg/resource/registry.go` 的 `Registry.Authorize/GetFilter`、`ticket.Resource.canOperate` 与 repository 空 filter 拒绝机制。新资源缺注册/未知 action/空列表过滤均拒绝；导出必须走与列表同一 parser、L1/L2 filter 和字段策略，不因后台任务或 AI 发起就绕开。`internal/router/router.go` 的 ticket export 既有链作为回归样本，B案页面GET/按钮写、路由目录对账守卫不撤。系统自动化改用显式受限服务主体与意图授权快照，ActorID 只归因不是授权凭据。

C04.4明确承接领域合同`AuthorizeForMutation`：IAM撤权与本地高风险写共享授权守卫。2026-11-15按[模块合同](../phase6/foundation-iam-module-detail.md#3-c04共同授权守卫与多实例)收敛为一个短事务guard，L1策略代际与L2/L3资源revision分开，不预建全局加主体两套epoch。普通业务取共享锁、授权变化取排他锁；只有实际改变Casbin策略或其缓存角色映射才增加L1 revision/重载，CC与类型可见性按当前DB谓词和资源revision判断，不引起全站reload。效果事务唯一锁序为guard→领域group→risk/ticket/link等资源ID（各集合排序）→最后执行fence/receipt条件写；claim/续租另用独立短事务，只锁执行行、不反取领域锁。所有影响权限的入口均列覆盖表，包含后续企业投影、接管/应急/退出，启用时复用此守卫，不因未来入口未实现阻塞无关本地业务。先以正确性试点和G04测量决定后续分片；不以缓存作原子守卫，也不靠本地锁承诺跨系统即时撤权。

C04.1 DBTX试点及故障分类；C04.2 org_roles两侧锁/历史处置；C04.3资源与导出负例及deadline；C04.4授权epoch与撤权竞争。出口：第二步audit/intent插入失败整笔回滚；池饱和不死锁；并发删组织/绑角色只有合法串行结果；未知action/export无scope不返数据；撤权先提交后旧授权业务不能提交，业务先持守卫提交则撤权等待并阻止后续写；取消/commit断链不伪称未提交。R01/D03在此事务与授权入口可用后接入，不扩大成分布式事务。

## C05：撤销会话的前后端合同

推荐保留 disabled 当前 403+30003 兼容响应，但增加可机器判定的**会话来源标志**（建议响应头 `X-Session-State: revoked`，仅 JWT/file-session 认证中间件产生，网关剥离扩展上游同名响应头并禁止业务handler伪造）。业务“目标用户已停用”使用原业务错误但不得带此标志，避免管理员修改别人失败时清自己会话。新UI识别平台认证响应的显式标志；与旧后端兼容时，遇无标志的403+30003先用独立、禁递归拦截的当前会话profile探针确认：仅探针亦拒绝该会话才终结，探针成功保留管理员会话，探针网络/5xx暂不清理且不重放原写操作。旧 UI 仍及时被后台拒绝，升级新 UI 才闭合跳转体验；不通过将任意 403 都当登出来修 F38。

401+20002 继续单飞 refresh 一次，401+20003 或认证 revoked 终结会话；403+20007 保留改密会话；权限不足普通403、429/网络/5xx保留会话。`resetState` 继续是唯一清理入口，幂等调用后跳登录并保留安全的站内 redirect；不持久化含敏感查询参数的外部 URL。

单标签已有 session generation，目标补浏览器同源多标签协调：只广播 logout/revoked/session_epoch 和不透明会话代，不广播 token；BroadcastChannel 优先，storage event 兜底。refresh 用短期跨标签互斥（Web Locks 可用则用；fallback 带过期与 owner 的 storage lease），令牌结果仍由当前会话代校验后接受；任何迟到旧 refresh/旧请求响应不得写入新登录会话。浏览器锁不是安全边界，后台 epoch 与 refresh 原子消费仍为权威；异常竞争宁可重新登录，不放宽 RT 重放检测。主动登出在网络失败时本地立即清理；后台撤权 Redis 失败保持既有 fail-closed，不伪报撤权成功。

本次不做设备管理产品：`device_id` 继续已有登录归属，列设备/单设备吊销/风控需另定明确场景；C05 固定窗交付“暂不建设、API/权限/审计前提与下一复核点”的结论，不悬置成无限触发条件。浏览器通知只能在下一次请求/页面恢复时得知撤权；无需为“即时弹走”额外建常驻推送服务。

C05.1 响应来源/错误码兼容及 UI 集中清理；C05.2 跨标签代际/迟到响应；C05.3 真栈账户撤权与不误退管理员。出口：A标签停用甲、甲B/C标签下一请求/恢复即清理；管理员停用乙返回业务错误仍在自己的会话；旧refresh在重新登录后返回不能复活旧用户；瞬时503、普通403、强制改密均不被误清；清会话后菜单、页签、query cache 无残留。mock 覆盖全部时序，真栈保留 backend+浏览器至少一条完整链。

## ENG-D01b：分档门禁合同（方向已确认、尚未启用）

所有者已确认采用分档门禁；G03实施时同步AGENTS/standards等规则、白名单与CI，并验证错分反例后启用。启用前现行完整门禁仍适用，本节是已确认方向的实施合同，**不追认当前候选免测，也不将旧253/42收据改标新SHA通过**。建议首先建立“本提交独立文档验证档”，而不是为跳过一次测试先建设全SDK/浏览器/环境闭包和24小时收据续用系统。运行收据的真实来源/失败链继续保留；制品交付的G03/G05来源约束不因此删除。

1. **按完整候选差异分类**：记录base/head/实际checkout tree与全部路径、增删改名/模式，不能只看最后一笔commit、commit type或GitHub可能截断的路径列表。文件不明、消费关系不明或混合变化默认普通档；不能由PR自己提交一个pass标记决定。
2. **文档档准入**：首版精确白名单候选为五仓`README.md`、`docs/README.md`，以及主仓`docs/review/planning-{backlog.md,backlog.json,closeout.md,decisions.json,design-status.json,document-scope.json,source-map.json}`。仅说明/导航/计划且未被代码/测试/embed/生成器消费后准入；设计稿/正式报告须逐文件登记已审配置清单，不凭目录自动准入。新增/变更白名单本身走普通档。规划JSON须校验ID/映射/依赖/投影；不把`docs/**`、`.md`扩展名作为全局免测规则。
3. **明确排除**：AGENTS、standards、现行开发/安全/操作/测试/发布规则；Swagger/OpenAPI/生成schema与`docs/docs.go`；运行示例/fixtures/testdata、源码/SQL/config/.env/deploy、依赖锁/工具/脚本/workflow/hook/Makefile等变化均回普通档。当前整批规划还含UI AGENTS/测试指南等修改，不能仅凭本节存在把整批宣称为纯文档档。
4. **离线原型单列**：首个精确对象为UI的`docs/design/prototypes/lifecycle-workspace.html`，脚本/资源也须逐项声明；在普通文档检查之外验脚本语法、修改旅程的交互/角色/窄屏/错误态与截图、无意外服务请求；它证明本地样板，不证明Vue/后端/E2E。生产运行时引用了原型/资源，或无法确认隔离，则回普通档。
5. **每个候选重新验证**：适用的diff/链接/片段/围栏、JSON/投影、来源/状态、符号锚点、脱敏和路径归属检查均绑定本候选。结论写`mode=document|prototype|ordinary`及实际命令、范围、失败/skip/限制。文档档表示本次文档证据通过，不表示本次运行了integration/acceptance/E2E或新功能已验收；旧运行结果始终标原来源。
6. **统一必需门禁**：未来quality-gate始终创建，按分类核对该档所有必跑结果与适用理由；缺失、取消、应跑而skipped不得放行。普通档继续现有门禁。改规则本身先以普通档验证错分反例，再同步主仓AGENTS/standards、UI AGENTS/testing和兄弟仓指南对应条文后启用；本节不直接改变远端保护/CI或现有纪律。

反例至少包括：`docs/docs.go`与swagger伪装文档、改AGENTS/测试指南、Markdown被embed/测试读取、新增未登记文件、同PR混进Go/SQL/lock改动、变更当前API语义却标新功能已完成、整个必需workflow被paths过滤跳过。原型文案改动可按影响选择检查，不把旧36张截图冒充本次全量重验。分类实现仍归G03，本文没有新增工具或workflow。

GitHub官方说明：顶层路径过滤跳过workflow可能令required check持续Pending，job级条件跳过则又有不同语义，因此应保留常驻聚合门禁并验证本档实际结果。[路径过滤](https://docs.github.com/en/actions/reference/workflows-and-actions/workflow-syntax#onpushpull_requestpull_request_targetpathspaths-ignore)、[required checks排障](https://docs.github.com/en/pull-requests/how-tos/merge-and-close-pull-requests/troubleshooting-required-status-checks)。选择独立文档档而非复杂收据复用是本项目的工程建议，不是GitHub替本项目批准豁免。

## 依赖、决策与交付边界

实施顺序：G01→G02/G03；C01/C02/C03 可在独立 worktree 并行，C04依赖C01，G04依赖G03，G06依赖G01，C05依赖G04；R01依赖C03/C04并先于D10/D12/A02等不可丢消费者。以上遵从总账，不新加第二套排期。G03扩展矩阵与根扩展设计联调，不将稳定公共适配层与私有providerSDK绑成一包。

需所有者确认的工作流/业务决策只有以下两项（未拍板时不得切换相应行为，但可完成其余实现设计）：

- **ENG-D01a（G02）已确认**：五仓统一PR、single-maintainer可合，G02落规则与保护；紧急bypass另记操作者与后补期限。**ENG-D01b（G03）方向已确认**：采用上节纯文档/离线原型独立验证档；同步修订AGENTS等明示规则并验错分反例后启用。旧运行收据不改标当前SHA，也不先实现复杂复用系统。启用前保持现行完整门禁，本轮已按原流程补验。
- **ENG-D02（C02）已确认**：file上传默认追加，B加入后A保留；字段移除不自动删附件，误传由用户明确移除/删除。已建单显式删除保护仍引用对象；正文图片引用按对应用途处理，不把file与richtext的删除通路混为同一实现。C02/U03编码前按此冻结引用元数据和操作文案，自动解除不是当前采用方案。

跨仓执行的业务取舍见伴随页 EXE-D01（reassign 提交语义）与 EXE-D02（cron错失策略），根控制台统一收录。其他技术选型是本页推荐方案，实施者只需在 PR 证明合同，不要求所有者替工程决策。

本页交付只增加设计文档；无 CI/保护设置、API、迁移、权限码、provider、生产开关已实施的声明。实施 PR 要附本页工作包/反例case、实际受控路径、迁移兼容与回退收据；共享总账与能力矩阵由集成批同步。设计自身复核包括源路径+符号、已实现/目标时态、跨页职责、mock/真实门禁保留和未决项完整性，不重跑已经通过的 Phase 5 业务门禁充当新功能证据。
