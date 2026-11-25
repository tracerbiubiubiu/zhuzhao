# 文档与本地材料目录

> 用途：解释材料的唯一归属和保留决定；范围：五仓文档及本机项目档案。共同知识从[文档总入口](README.md)读取，具体规则见[文档维护约定](documentation.md#2-什么应进入-git)。本页不依赖个人电脑绝对路径。

## Git中继续维护

当前设计、指南、接口、运维、决策和正式时点结论仍随各自仓库保存；生态级范围/依赖/估算/决策统一在[planning](planning/README.md)。[迁移清单](document-migration.json)记录39份文件的旧新位置，不复制第二份内容。

规划检查读取的 JSON、按固定版本保留的来源和验证记录继续随 Git 保存。前端审计数据、设计评审和模块验收摘要如果记录了独有的发现、接口约定或复跑依据，也继续保留。正式报告保留失败/限制和历史基线；不能因原始日志已归本地便删除唯一结论。Swagger、合成展示资产和参考示例按原消费者/声明职责保留。

可复用方法、空白模板和开发/审计提示词原件由[工作资料库](toolkit/README.md)索引并随Git维护；原件不包含某轮实际基线、进度或验证结果。填写后的报告只有需长期追溯的正式结论进入review/reports，其余执行实例归本地批次。产品运行时提示词/模板、冻结输入和工具识别位置仍按实际消费者保存。

### 主仓开发前盘点（2026-11-21）

基线 `3d958c3` 在 `docs/` 下跟踪 392 项，包括 326 篇 Markdown。另有 4 篇 Markdown 位于仓库根与工具入口。文档量集中在正式报告及历史审查（含目录入口共 98 篇）、Phase 1–5（61 篇）、提案（18 篇）。这些数量用于找整理入口，单篇是否保留仍看下面的用途。

- **当前说明更新到归属页。** 当前登录、工单和权限从[模块入口](modules/README.md)读。设计决定从[规划入口](planning/README.md)和对应合同查。历史阶段保留先前实现的背景、决策和验收边界，导航直接指向当前说明。
- **按实际使用保留文件。** [旧表单原型](phase5/prototypes/README.md)还有离线验证，不能因当前 UI 已另仓实现而将原型和配套资产一起移走。[规划结论的证据清单](planning/planning-closure.json)引用的机器证据含固定校验值，应按原路径及字节保留。
- **以减少重复内容为先。** 后续有新结果时优先更新已有合同、控制台或对应专题入口。正文已由其他权威页承接、没有依赖当前树路径的消费者时，按[保留判断](documentation.md#2-什么应进入-git)合并或退出当前跟踪。只需按时点追溯的过程原件可通过固定提交与本地副本保存。

本轮将 2026-10-25 Phase 5 已填审计任务卡移出当前跟踪。它记录分批检查计划，结果归[实际审计总报告](review/reports/zhuzhao-ecosystem-full-audit-2026-11-06.md)和[B27 收口](review/reports/zhuzhao-ecosystem-audit-b27-phase5-closeout-2026-11-07.md)。[历史索引](review/archive/README.md)改指固定提交 `6c4377aa0bb04c5716cace8b637fa0787885d363` 的原文。历史盘点登记保留，校验器无需读取其现行路径。材料 ID `maintenance/docs-clarity-20261121/` 留存原件与收据，原件 SHA-256 为 `30d91f3eca72c631df3497d42bc9e67383d569c331f3d6d5128f1d8f8e601baa`。

## 本地档案中保存

本地档案根目录由维护者配置，当前机器的真实位置写在档案根`README.md`；克隆仓库不要求同时拥有该目录。以下稳定材料ID均为相对于档案根的路径：

- `references/project-comparison-audit/`：原始项目比较与调研材料；正式结论由对应审查/设计承接。
- `references/ticket-benchmark-2026-10/`：原始工单对标材料，保留参考来源。
- `references/zhuzhao-phase6-9-planning-review-2026-11-07/`：阶段规划原始审查，正式计划以当前台账为准。
- `evidence/readme-refresh-20261113/readme-refresh-2026-11-13.json`：已完成README更新的纯操作收据，本次停止Git跟踪；[正式报告](review/reports/readme-refresh-2026-11-13.md)继续保留范围、原失败及结果。
- `maintenance/docs-consolidation-20261116/`：本次逐文件库存、旧新路径、冻结输入哈希、归档验证和测试原日志；属于维护执行记录，不跟随产品仓提交。

工作资料整理的执行记录使用材料ID `maintenance/docs-toolkit-20261116/`，保存模板提取核对、链接/Git范围检查、隔离门禁和清理收据，不随产品仓提交。

B2 隔离实施的本机执行材料登记为 `g04-it1-20261122/stage3` 和 `g04-it1-20261122/d01`。前者对应第 1–3 项的候选指纹、独立复验和门禁收据，后者对应第 4 项首批定向验证。共享结论见[第 1–3 项隔离联合验收](review/reports/phase6-b2-stage3-acceptance-2026-11-22.md)。逐请求和数据库读回保留在本机执行批次。

第 6–9 项的合成执行材料使用 `evidence/phase6-b2-stage9-20261123/`，记录候选与运行指纹、门禁、实际丢响应、重启前后读回和独立复核。受控结论见[第 6–9 项隔离验证](review/reports/phase6-b2-stage9-validation-2026-11-23.md)。

完整性复核记录使用材料ID `maintenance/document-organization-recheck-20261116/`；后续入口同步与地址核对记录使用 `maintenance/document-addresses-20261116/`。它们只保存整理/验证过程，不作为新的产品事实台账。

此前工作树清理的压缩包与逐文件SHA256继续保存在独立批次`src-cleanup-20261116`，由本地档案索引定位；不重复复制大包。首次目录清理未纳入的受限参考资料，后续已按所有者要求集中到仓库外的本地私有区；其真实位置与迁移清单只记录在本地索引。

三组参考目录共937个普通文件，迁移时逐文件SHA256一致。停止跟踪的README收据SHA256为`5b474278750c9a3cbfdc1d90ffedf8f96320323e8c8ce7ad4dbae536ac99c981`，也可从主仓迁移前基线`90364d194e00f4dc776a0d86be747f02479aae16`的`docs/review/reports/readme-refresh-2026-11-13.json`恢复。本地归档只表示集中保留，不表示已经异机备份。

## 仅限本机的受限资料

所有者明确限定为本机使用的资料，原件、压缩包、解压目录、HTML及其执行清单均不得纳入Git或通过其他渠道上传，包括私有仓库、云盘、外部分析服务、分享或消息附件。此类资料由本地私有区的协作规则约束，不与普通可共享参考混放；受控文档只说明保管边界，不登记真实私有文件名、绝对路径或内容。Git/Docker/ZCode排除只作防误操作，不能替代禁止上传要求。

文档里已有的旧本地路径只保留历史来源含义，不要求克隆者拥有原件，也不建立软链接把私有资料重新暴露到仓库。需要定位时查本地索引；归档不代表异机备份或已经实施系统级网络拦截。

复核还将三份运维工具说明集中到[工具文档](ops/tools/README.md)，构建上下文、Dockerfile和依赖锁保持原位置；旧的本地过程记录已归档，具体迁移清单只留本地维护批次。

## 后续新增材料

B2 固定候选签收的本地材料 ID 为 `evidence/phase6-b2-signoff-20261125/`，登记复用收据的 SHA256、运行镜像身份、补跑结果、合成提醒读回和所有者签收记录。正式结论见[固定候选签收证据](review/reports/phase6-b2-candidate-signoff-2026-11-25.md)。

2026-11-20 两轮 Web 易用性复验使用材料 ID `evidence/phase6-web-ux-20261120/`，选定的合成渲染截图与测试收据按轮保存，并核对 SHA-256。受控结论与复现条件见[正式报告](review/reports/phase6-web-ux-audit-2026-11-20.md)。

2026-11-18错对象写入修复批使用材料ID `maintenance/pass8-target-write-20261118/`，保存合成错序红绿证据、候选基线、适用门禁及被隔离资源的回收记录。分批处置归[存量修复前移计划](planning/audit-repair-plan-2026-11-17.md#6-2026-11-18-同类复核与分批承接)，原始运行材料留本机。

2026-11-20 审计续批使用材料 ID `maintenance/audit-repair-followup-20261120/`，归档定向红绿、三仓适用门禁、隔离 Compose 停止及主线合并收据。范围和保留问题见[修复记录](review/reports/audit-repair-followup-2026-11-20.md)。

pass8复审后所有者明确要求按建议补修，材料ID`maintenance/audit-pass8-followup-20261117/`保存本批执行卡、红绿/删除关键修复变异、五仓Git来源与四组件源码/运行产物manifest、独占全门禁、浏览器/后台/恢复专项、独审与精确清理收据；最终外置封存以该ID的sealed-index及逐项源/副本SHA和权限复核为准。受控结论归[pass8补修报告](review/reports/audit-pass8-followup-2026-11-17.md)和原前移计划的具名后继表，旧审计和四批封存保留原时点，不覆盖历史validation或上传原始过程材料。

本批源码白名单须在回收最终工作树前封存。封存后最终Git清理、执行副本退役及逐项复核另记同一maintenance目录下的`audit-pass8-followup-20261117-operation-receipt.json`及SHA文件，明确绑定该sealed-index SHA；它不写入或改动已封存payload。最终工作区事实取该操作收据，代码与原始证据取不可变封存，不能把封存前仍存在工作树的快照冒称为收尾后状态。

2026-11-17存量审计复核、政策确认和修复计划的执行卡、原始门禁日志、候选/资源指纹及收据归材料ID`maintenance/audit-repair-plan-20261117/`；原始门禁及修订证据已归本地档案并核哈希，ignored工作区保留本次执行副本。受控结论归[修复计划](planning/audit-repair-plan-2026-11-17.md)及[目标账号保护合同](phase6/iam-first-batch-design.md#37-目标账号接管保护cf-06本轮已确认待实现)，不将本机路径或测试秘密复制到Git。原`FULL-AUDIT-2026-11-16`审计材料保留其批次和时点，报告修订不改写既有原始证据。

本轮推送与main承接另用材料ID`maintenance/audit-baseline-main-merge-20261117/`，用于保存三仓消息only映射、推送、PR/准确head CI、合并及后继文档的来源复核与操作收据；现势结论见[三仓合入记录](review/reports/audit-baseline-main-merge-2026-11-17.md)。原本地验收材料封存不改，不因功能分支已推送就记main或部署成功。

同日临时工作树收尾使用独立材料ID`maintenance/audit-worktree-closeout-20261117/`：保存8个旧实施/评审工作树的395项未提交状态、另5个已合并工作树的本地文件、三仓Git bundle、逐文件SHA256、实际还原验证及精确清理收据。共保全33,698个普通文件；原8树没有排除，另5树只排除两个UI根`node_modules`依赖安装目录，生产构建产物与执行记录仍保留。49项未匹配旧归档的草稿经差异复核未发现遗漏实现，清理后继结论见[合入记录§5](review/reports/audit-baseline-main-merge-2026-11-17.md#5-临时工作树独立收尾)。旧封存索引保持原SHA；新材料仅本机保留，不随Git上传。

所有者随后要求全面复核五仓工作区与分支，材料ID`maintenance/audit-workspace-final-check-20261117/`保存fresh远端快照、其余19个本地/6个远端分支的准确引用、五仓Git bundle及实际恢复/fsck、旧草稿与完整旧封存重验、精确分支清理和后继文档门禁收据。待清分支均为本地及远端main祖先，分支独有提交为零；受控结论见[合入记录§6](review/reports/audit-baseline-main-merge-2026-11-17.md#6-五仓工作区与全部剩余分支复核)。本批不改旧封存，也不将原始执行材料和被替代草稿上传Git。

同日后继产品实现与组合验收使用独立材料ID`maintenance/audit-baseline-repair-20261117/`：保存主批`repair-implementation-20261117`及相关工作树原始专项、首次失败与修正、冻结源码/运行产物manifest、门禁命令/退出码、浏览器、恢复、精确清理及本地提交收据。正式结论归[前置修复与组合验收](review/reports/audit-baseline-repair-2026-11-17.md)。归档逐文件核SHA，下载缓存/依赖安装目录不作为证据复制，具体排除及副本清单由本地索引记载；目录/文件收紧权限，不随Git上传，ignored工作区可保留执行副本。该归档为同机保留，不等于异机备份。

2026-11-25 Claude 两轮只读审计使用材料 ID `evidence/project-audit-claude-20261125/`。该目录包含：
- 第 1 轮 5 份分项原文（Phase 6 规划专项）；
- 第 2 轮 9 份分项原文（五仓全面审计）；
- 基线与来源会话说明；
- SHA256。

受控结论与主会话的复核修正见[审计报告](review/reports/phase6-plan-and-five-repo-audit-2026-11-25.md)。原文只在本机保存，不是异机备份。

先判断它是否是必须共同维护的事实/输入。是则进入归属仓`docs/`并挂到已有入口；否则使用本地档案相应分类和明确批次ID。短期执行产物可先留ignored `.codex/`，正式交付提炼结论并登记材料ID，避免在`src`下持续新增散落报告目录。
