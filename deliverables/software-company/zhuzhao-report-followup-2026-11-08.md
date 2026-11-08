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

## 验证与环境边界

UI原六件gate与主仓四档acceptance均在本轮独占栈执行通过。实际三个Go容器ELF与既有构建receipt的完整Go/go.mod/go.sum集合逐项相等，元数据Go1.26.10；没有把不同HEAD冒称重新构建。main746文件/UI288文件冻结manifest门禁前后无漂移，canonical UI与快照相同，共享node_modules元数据保持；本轮8容器及私有卷/网络已精确回收。

准备失败单列保留：Playwright --list在globalSetup前缺state；旧UI wrapper要求已不存在的系统HTTP代理，尚未启动gate。第二次wrapper依据scutil实际无代理状态执行原gate成功，未改产品/测试断言或系统网络设置，最终状态以ignored joint/final-summary.json为准，不覆盖首轮result.json。

主仓普通lint首轮生成工具明确拒绝系统Go1.26.6；显式1.26.10后新工具缓存安装超时，改用已有、由工具入口核验版本/来源的固定缓存后lint/unit/cover及完整integration全部通过（integration987.18秒），覆盖率20.4%。未降低go.mod下限或修改全局PATH；Go源码未改。

原始执行卡、基线、命令日志、快照与资源归属仅在 ignored `.codex/audits/report-followup-20261108/`。共享 dev/test、账号、原卷和其它会话资源不变。E000001仅允许独占 acceptance/globalSetup 的既定幂等例外流，不手工改共享凭据。
