# 报告复核后分批修复 — 2026-11-08

## 范围与固定基线

用户授权依次修复 richtext 清空缓存与旧响应覆盖、补回归并跑门禁，再补主仓及三个 Go 兄弟提交规范、UI 登记和悬空锚点。仅基于实际 Phase5 工作分支，不合 main，不 push、不发布或改写已推历史，不替换共享运行服务。

修复前固定端点：主仓 `3ca640226814768a9c3b240d281569f017dccfb3`；UI `152258638abaa3bd3daa656255a794c5422a0e58`；taskrunner `6d0e2ad592eccf0c12bb525958784200c478a45e`；activelist `83909b86a697dd2fc69a71819a5e1327c9be1d03`；utils `563785a83ce2aeb12c5f2b65c42defb58a48c331`。主仓/UI 为 phase5，三兄弟为 codex/phase5-audit。各仓开工工作区干净；未用 canonical main 代替审计候选。

## 第一批 RR-UI-01：详情 richtext 新鲜度

原 UI `src/views/ticket/detail/index.vue` 的 `renderedHtml` watcher 过滤空源而未删旧缓存，且 await 后无请求身份检查；源码抽取 Vue 运行时探针分别复现清空后残留与旧请求覆盖新 HTML。不是跨工单泄漏，亦不声称当前详情编辑框有 custom_data 清空入口。

生产缓存提为同目录 `useRenderedRichtext.ts` 的 `useRenderedRichtext`，详情实际调用此实现。每字段源清空/移除时同步移除缓存与请求身份；异步成功/失败结果仅对仍持有身份的请求写回，独立字段互不取消，scope 销毁撤销所有身份。逻辑取消不改变 render API 签名或服务端消毒职责，不引入新依赖、迁移或权限。

新增十条直接测试生产 composable 的契约回归：同源复用、已渲染清空、在途清空、schema 移除、乱序响应、同 tick A→B→A、移除后同源重加、多字段独立、卸载、渲染失败不回退原始 HTML。旧逻辑等价抽出时八红二绿，修复后十绿；完整单测由29文件230例增至30文件240例。清空/移除资格失效、请求身份两目标变异各八轮稳定红，独占副本恢复前后十绿。

UI修复提交 `bebfe523d2a84f7d017bc46fcaf05d38e883844c` 的六文件与联合门禁冻结源逐字节一致。原样 `pnpm gate` 六件全部通过，真实E2E 41/41、零retry/flaky/skip，后端及Node helper全窗HTTP429为0；独占 `make acceptance` 为253/0/0，81对迁移往返零漂移。**RR-UI-01代码复验关闭**，未推送、合main或部署共享服务。

同类扫描检查详情富文本与未被页面引用的 `RichtextView.vue`；后者不在本批消费路径，未以本次缓存修复替代其未来启用前的安全复核。

## 第二批 RR-GIT-01：五仓提交消息与真实窗口

主仓、UI和三个Go兄弟均纳管同源 `scripts/commit-policy.py`、`test-commit-policy.py`、`install-git-hooks.sh` 与三钩。本地commit-msg在旧重题早退之前校验消息原文，pre-push逐ref检查Git提供的旧/新OID；CI独立job用push before/after或PR base/head并fetch-depth 0，缺对象只允许精确补fetch，不使用猜测窗口。新merge同样要求CC主题和非空正文。

最终豁免基线：主仓3ca6402、UI15225863、TR6d0e2ad、AL83909b8、utils563785a，均为本轮开工已推快照，完整SHA随各仓baseline文件纳管。自动窗口排除其祖先，显式历史审计仍严格；不移动历史水位，不重写996b399/f3f2077或UI6a95fd2a。主仓保留Swagger⑥与披露/同类扫描/三节WARN⑦⑧⑨，生成器失败不再因无diff误绿。

安装仅写worktree-local hooksPath，不把三个canonical main切换到缺文件的相对路径。遗留main仍借用主仓绝对钩子时，仅在钩子物理位置确在本仓之外、本仓无core/基线、外部来源具有core三条件下明确WARN沿原行为；本仓标准/嵌套钩子缺文件均FAIL。原重题诊断补变量边界，避免macOS bash将全角括号吞入变量名产生非法UTF-8。

独立复核实际发现并修正两类接线问题：对`-F`原文strip-comments可能换掉非法主题并误拒字面Markdown；`!=标准.githooks`不能证明钩子在仓外。最终42项共享反例包含真实Git pathspec提交、无关staged守恒、四种legacy兼容边界、初推/force/多ref/删除、精确CI SHA、缺对象与浅克隆拒绝，五仓各42项全绿。主仓另10条旧WARN/正文回归和5条wrapper/Swagger反例全绿；三兄弟Go1.26.10 lint/test/build、UI lint/typecheck/240单测/build通过。CI已接线但尚未push，因此没有本轮远端CI运行结论。

独立路径提交绑定：taskrunner `b51d224f3ec2e6e4caa45d4095dbd7fc92191eeb`（10文件）；activelist `2ebafca1f1f52ec1cf5c950e25f854c7c6875bbe`（10文件）；utils `46a5605d94bffc719ca43e0f5312214c00847be5`（10文件）；UI `63c4f4f16d789a004cc3bc5de0c539c19ff04828`（11文件）。各仓六个共享源/钩子逐字一致；主仓配套工具与规范提交 `03e0146195ecca6f705329870e209a361efe9559`（16文件），不循环自引报告提交SHA。

## 第三批 RR-DOC-01：UI清欠登记与真实词表

主仓S6的A.9补齐原6a95fd2a的M-2、M-4与richtext降级项事实，S10回链既有两状态top-K待办分页，UI工程规范反向引用；通知配置注释改指真实A.9。phase6/07只补事件元数据触发登记，原件18 visible_when不动；当前不存在事件词表读取端点，未新增API或批准实施批次。

同类核对发现UI `EVENT_OPTIONS`为13项，后端`validEventActions`为14项，缺既有`ops.storage_usage_high`选项。本批补齐，并新增UI `eventOptions.test.ts`以TypeScript AST读取生产Vue数组、与后端生产map完整集合核对（含重复/空集合/解析失败守卫）；缺主仓源文件不得skip，不只比较计数。仍有手维护/响应生成schema缺口，均如实登记，不宣称已实现元数据端点或完整codegen接线。

本批旧13项词表实红、补齐绿；等数量错键变异红、缺主仓源码直接ENOENT失败。含新选项的原样 `pnpm gate` 再次全部通过：31文件241测、E2E41/41，零retry/flaky/skip及HTTP429；独占 `make acceptance` 再次253/0/0，81对迁移往返零漂移。Go354/AL44/TR43运行源逐SHA匹配原构建receipt，实际三ELF SHA与Go1.26.10元数据核验通过。主仓753/UI296冻结manifest保持，canonical UI与快照无差异，共享node元数据保持，全部本轮容器/卷/网络已精确回收，既有容器无增删改。

UI修复提交 `22d9fafa011e30f8af8c71a790221f3317e83034` 的五文件与本轮冻结源相同。RR-DOC-01代码/登记复验关闭；当前仅本地分批提交，未push、未合main或发布。所有原失败与最终结果分列，第三批以ignored `joint-doc/final-summary.json` 为准。

UI收尾提交 `bf61c863169cbe659e5af47e32167375c1f98fc5` 只将豁免基线从安装前本地bebfe523收紧为开工已推15225863，让本轮richtext提交也纳入自动扫描；没有前移水位或增加豁免，不改变第三批已验收业务源。最终真实pending/pre-push/CI载荷窗口另实核。

## 验证与环境边界

UI原六件gate与主仓四档acceptance均在本轮独占栈执行通过。实际三个Go容器ELF与既有构建receipt的完整Go/go.mod/go.sum集合逐项相等，元数据Go1.26.10；没有把不同HEAD冒称重新构建。main746文件/UI288文件冻结manifest门禁前后无漂移，canonical UI与快照相同，共享node_modules元数据保持；本轮8容器及私有卷/网络已精确回收。

准备失败单列保留：Playwright --list在globalSetup前缺state；旧UI wrapper要求已不存在的系统HTTP代理，尚未启动gate。第二次wrapper依据scutil实际无代理状态执行原gate成功，未改产品/测试断言或系统网络设置，最终状态以ignored joint/final-summary.json为准，不覆盖首轮result.json。

主仓普通lint首轮生成工具明确拒绝系统Go1.26.6；显式1.26.10后新工具缓存安装超时，改用已有、由工具入口核验版本/来源的固定缓存后lint/unit/cover及完整integration全部通过（integration987.18秒），覆盖率20.4%。未降低go.mod下限或修改全局PATH；Go源码未改。

原始执行卡、基线、命令日志、快照与资源归属仅在 ignored `.codex/audits/report-followup-20261108/`。共享 dev/test、账号、原卷和其它会话资源不变。E000001仅允许独占 acceptance/globalSetup 的既定幂等例外流，不手工改共享凭据。

## 收口复核与交付候选（2026-11-08）

所有者同意保全utils引用、补批次归档说明与镜像经验、验证ignored材料备份，并冻结五仓交付候选。本批只改主仓本报告、11-06总报告、runbook与review/11；消费者pin、业务实现、配置、迁移、权限和API均保持。本轮继续不push、不合main、不打tag或发布，不改共享运行服务。历史三批验收与部署时点仍分别保留。

### 依赖引用保护与冷下载

主仓与AL消费`v0.4.4-0.20261106080630-cd759a040245`，精确提交为`cd759a0402455731f805ba507bf8401447e00e29`；TR仍为v0.4.2。本批在[runbook §7](../../docs/runbook.md#7-多会话协作与验收环境纪律2026-10-26-立约)登记utils远端`refs/heads/codex/phase5-audit`为清理保护项。保护解除要求实时清点消费者pin、验证承载提交的新长期远端引用，并重新验证精确版本可下载；不会因本机bundle或proxy缓存命中就解除。正式main/tag发布走后续流程，不用库HEAD代替消费者版本。

冷下载仅用本轮独立临时模块缓存与已验Go1.26.10 SDK，不清理共享缓存、不引入replace/GOWORK。下载与五仓SSH远端查询收据仅存ignored `.codex/audits/closeout-20261108/pin-and-repos/`；冷下载成功证明本轮时点可获取及内容一致，不是第三方永久托管承诺。

本轮全空模块缓存经官方proxy/sumdb下载成功，主仓/AL的module及go.mod两项h1均匹配；38份Go/go.mod/go.sum与目标commit逐字相同，`go mod verify`通过。SDK实际为Go1.26.10，其二进制SHA与既有已验收据一致。SSH fetch实时确认utils受保护远端tip为`563785a83ce2aeb12c5f2b65c42defb58a48c331`且包含所需commit；本地main不含、本地tag无包含。额外`ls-remote`全量分支/tag查询两次遭SSH reset，保留失败、不再重试，不宣称已实时排除其他远端tag。

### 编号归档与运维经验

[总报告“批次编号与归档口径”](./zhuzhao-ecosystem-full-audit-2026-11-06.md#批次编号与归档口径2026-11-08补记)补齐23个文件/22个批号、B06续批、B17→B16、B22→B18和B01/B03/B04本地原件映射。批次覆盖、产品发现、报告份数分别记录，不因缺同名文件推断漏跑。`deliverables/software-company/evidence/`当前仅有六个本地空批目录、无文件，`git ls-files`亦为0，目录存在不代表原始证据已入库；正式结论入Git、原始材料ignored的边界保持。

runbook登记B03实际migrate镜像拉取与宿主CLI版本差异、固定工具缓存入口的核验方式。特定“daocloud/EOF”说法在已有本地`environment-startup-review.md`中明确缺原始证据，本批仅登记为未证实线索，不编造根因或把替代源写成已验标准方案。

### 原始材料备份与恢复边界

本轮备份范围为`.codex/audits/ECO-20261104/`及`.codex/audits/report-followup-20261108/`的报告原件、执行卡、日志、manifest、截图、数据库备份及既有归档/bundle。276项排除记录包含依赖/构建/Vite缓存、快照Git管理目录、临时ELF/Mach-O与镜像/build-stage tar；这些运行产物副本未纳本包，原件全部保留，不宣称本包覆盖全部运行镜像。备份位于仓库外本机专用目录，目录700、归档与manifest600；只在本机处理，不上传运行数据或秘密。

主包为本机`~/.codex/backups/zhuzhao-closeout-20261108/evidence-20261108T091538Z.tar.gz`，1,038,614,302字节，SHA256为`ceaa590f5ee72d09c8c6ce281307cc82fb33dab90b523a1fd6783faf6655b02e`。147,392成员（131,606普通文件、3链接、15,783目录）逐成员SHA/链接校验通过，捕获期间源零变化。13件实际恢复样本逐SHA相同，覆盖B01/B03/B04、11-08执行卡/收据/门禁日志及五份bundle；五份bundle各在空bare仓验证通过。首轮最终化遇系统Python无`hashlib.file_digest`，改用兼容流式SHA后重新完整校验，原失败保留。

精确utils下载的`.info/.mod/.zip/.ziphash`四件共83,703字节另保存至本轮`pin-and-repos/module-artifacts/`，manifest绑定目标commit、h1及原pin收据SHA；本轮门禁/提交收尾后，连同新的命令日志与窗口收据补入独立补充包，不递归纳入模块缓存或重写主包。下载包只作本机恢复材料，不替代远端引用保护。

旧归档bundle与本轮待推候选分开保存；收尾另按五仓显式当前分支创建无前置bundle，在新空bare仓验证、恢复并检查本轮待推提交可达性及对象完整性。主仓本批提交后的准确HEAD、五个新包路径/SHA与恢复结果以本轮`backup/candidate-bundles.json`为准，不把旧五份bundle当作已覆盖11-08新提交；补充包引用这些仓外包的路径/SHA，不递归复制大包。

逐成员SHA/符号链接验证、独立恢复抽样及本轮交付收据均记录在ignored `.codex/audits/closeout-20261108/`，主包及manifest/排除清单/收据也存仓外备份目录。该备份与工作树在同一台机器/同盘，能够防范工作目录误删，但不构成异地容灾或生产恢复演练。

### 候选窗口与验证状态

五仓固定分支、完整HEAD、SSH fetch时点与待推提交以本轮ignored收据为准；主仓本批纯文档提交位于下列开工HEAD之后，最终提交/窗口收据另存，不在自己的正文循环引用自身hash。后续任何推送仍须重新fetch、核真实待推集合与提交规范，推送后再核对应精确提交的远端CI；本地门禁不替代远端CI、main整合或生产部署。

2026-11-08 17:14:59–17:15:05（Asia/Shanghai）五仓SSH fetch全部成功，目标分支`FETCH_HEAD`与tracking tip逐项相同，采集时各仓behind均0。固定待推窗口（左端为已fetch远端，右端为本轮开工本地HEAD）：

- 主仓`phase5`：`3ca640226814768a9c3b240d281569f017dccfb3..059b0a129a69ef6453260356560de995f4f42e02`，4笔；本批纯文档提交另追加于此右端。
- UI `phase5`：`152258638abaa3bd3daa656255a794c5422a0e58..bf61c863169cbe659e5af47e32167375c1f98fc5`，4笔。
- taskrunner `codex/phase5-audit`：`6d0e2ad592eccf0c12bb525958784200c478a45e..b51d224f3ec2e6e4caa45d4095dbd7fc92191eeb`，1笔。
- activelist `codex/phase5-audit`：`83909b86a697dd2fc69a71819a5e1327c9be1d03..2ebafca1f1f52ec1cf5c950e25f854c7c6875bbe`，1笔。
- utils `codex/phase5-audit`：`563785a83ce2aeb12c5f2b65c42defb58a48c331..46a5605d94bffc719ca43e0f5312214c00847be5`，1笔。

上述11笔逐仓以同源[scripts/commit-policy.py](../../scripts/commit-policy.py)的`main`解析`range --strict`、`select_commits`选取精确窗口及`check_commits`检查，全部通过，未使用历史基线豁免。五个候选均未进入已fetch的`origin/main`；这是本轮时点关系，不替代将来推送前的重新核对。

本轮以已验Go1.26.10 SDK及工具入口核验的固定缓存执行原样`make lint`、`make test-unit`、`make test-cover`、`make test-integration`，全部通过；integration 643.688秒，覆盖率20.4%（与上一批披露的舍入值相同），Swagger无漂移。独占8容器栈原样`make migrate-up`到81且dirty=false，原样`make acceptance` 43.152秒，四档124+68+26+35合计253/0/0，81对迁移往返零漂移，前后schema均81|f。

主354/AL44/TR43个Go/go.mod/go.sum条目及实际ELF逐项匹配既有构建receipt，本轮复用已核产物，不冒称重新构建。753文件冻结快照自身前后零漂移；canonical仅四份授权Markdown有本批修改与结果回填，脚本/配置/迁移/业务源未变，不宣称整个工作树与冻结副本逐字相同。8容器及私有卷/网络精确回收，既有容器增删改为0；integration的临时Ryuk自动退出，无手工清理。本轮收据以`gates/main/results.json`、`gates/main/cleanup-post-reaper.json`、`gates/acceptance/final-summary.json`及`cleanup-final.json`为准，冻结后文档差异另记，不覆盖原时点manifest。

本轮未重复UI门禁、安全扫描、生产部署或异地灾备演练，原结果仍属于各自固定时点。最终路径提交后的消息窗口、五仓候选恢复和本轮日志补充包结果另存本轮ignored收据；未push所以没有本轮远端CI结论。
