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

启用基线：主仓3ca6402、UI bebfe523、TR6d0e2ad、AL83909b8、utils563785a，完整SHA随各仓baseline文件纳管。自动窗口排除其祖先，显式历史审计仍严格；不移动历史水位，不重写996b399/f3f2077或UI6a95fd2a。主仓保留Swagger⑥与披露/同类扫描/三节WARN⑦⑧⑨，生成器失败不再因无diff误绿。

安装仅写worktree-local hooksPath，不把三个canonical main切换到缺文件的相对路径。遗留main仍借用主仓绝对钩子时，仅在钩子物理位置确在本仓之外、本仓无core/基线、外部来源具有core三条件下明确WARN沿原行为；本仓标准/嵌套钩子缺文件均FAIL。原重题诊断补变量边界，避免macOS bash将全角括号吞入变量名产生非法UTF-8。

独立复核实际发现并修正两类接线问题：对`-F`原文strip-comments可能换掉非法主题并误拒字面Markdown；`!=标准.githooks`不能证明钩子在仓外。最终42项共享反例包含真实Git pathspec提交、无关staged守恒、四种legacy兼容边界、初推/force/多ref/删除、精确CI SHA、缺对象与浅克隆拒绝，五仓各42项全绿。主仓另10条旧WARN/正文回归和5条wrapper/Swagger反例全绿；三兄弟Go1.26.10 lint/test/build、UI lint/typecheck/240单测/build通过。CI已接线但尚未push，因此没有本轮远端CI运行结论。

独立路径提交绑定：taskrunner `b51d224f3ec2e6e4caa45d4095dbd7fc92191eeb`（10文件）；activelist `2ebafca1f1f52ec1cf5c950e25f854c7c6875bbe`（10文件）；utils `46a5605d94bffc719ca43e0f5312214c00847be5`（10文件）；UI `63c4f4f16d789a004cc3bc5de0c539c19ff04828`（11文件）。各仓六个共享源/钩子逐字一致；主仓配套工具与规范提交 `03e0146195ecca6f705329870e209a361efe9559`（16文件），不循环自引报告提交SHA。

## 第三批 RR-DOC-01：UI清欠登记与真实词表

主仓S6的A.9补齐原6a95fd2a的M-2、M-4与richtext降级项事实，S10回链既有两状态top-K待办分页，UI工程规范反向引用；通知配置注释改指真实A.9。phase6/07只补事件元数据触发登记，原件18 visible_when不动；当前不存在事件词表读取端点，未新增API或批准实施批次。

同类核对发现UI `EVENT_OPTIONS`为13项，后端`validEventActions`为14项，缺既有`ops.storage_usage_high`选项。本批补齐，并新增UI `eventOptions.test.ts`以TypeScript AST读取生产Vue数组、与后端生产map完整集合核对（含重复/空集合/解析失败守卫）；缺主仓源文件不得skip，不只比较计数。仍有手维护/响应生成schema缺口，均如实登记，不宣称已实现元数据端点或完整codegen接线。

本批旧13项词表实红、补齐绿；等数量错键变异红、缺主仓源码直接ENOENT失败。含新选项的原样 `pnpm gate` 再次全部通过：31文件241测、E2E41/41，零retry/flaky/skip及HTTP429；独占 `make acceptance` 再次253/0/0，81对迁移往返零漂移。Go354/AL44/TR43运行源逐SHA匹配原构建receipt，实际三ELF SHA与Go1.26.10元数据核验通过。主仓753/UI296冻结manifest保持，canonical UI与快照无差异，共享node元数据保持，全部本轮容器/卷/网络已精确回收，既有容器无增删改。

UI修复提交 `22d9fafa011e30f8af8c71a790221f3317e83034` 的五文件与本轮冻结源相同。RR-DOC-01代码/登记复验关闭；当前仅本地分批提交，未push、未合main或发布。所有原失败与最终结果分列，第三批以ignored `joint-doc/final-summary.json` 为准。

## 验证与环境边界

UI原六件gate与主仓四档acceptance均在本轮独占栈执行通过。实际三个Go容器ELF与既有构建receipt的完整Go/go.mod/go.sum集合逐项相等，元数据Go1.26.10；没有把不同HEAD冒称重新构建。main746文件/UI288文件冻结manifest门禁前后无漂移，canonical UI与快照相同，共享node_modules元数据保持；本轮8容器及私有卷/网络已精确回收。

准备失败单列保留：Playwright --list在globalSetup前缺state；旧UI wrapper要求已不存在的系统HTTP代理，尚未启动gate。第二次wrapper依据scutil实际无代理状态执行原gate成功，未改产品/测试断言或系统网络设置，最终状态以ignored joint/final-summary.json为准，不覆盖首轮result.json。

主仓普通lint首轮生成工具明确拒绝系统Go1.26.6；显式1.26.10后新工具缓存安装超时，改用已有、由工具入口核验版本/来源的固定缓存后lint/unit/cover及完整integration全部通过（integration987.18秒），覆盖率20.4%。未降低go.mod下限或修改全局PATH；Go源码未改。

原始执行卡、基线、命令日志、快照与资源归属仅在 ignored `.codex/audits/report-followup-20261108/`。共享 dev/test、账号、原卷和其它会话资源不变。E000001仅允许独占 acceptance/globalSetup 的既定幂等例外流，不手工改共享凭据。
