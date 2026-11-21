# zhuzhao 文档入口

系统目前能用账号、组织和角色管理工单。下一批优先准备人工漏洞整改（发布验收点 B2）。一个派单人把同一负责人名下的两条风险放进一张整改票，办理人逐项提交证据，负责人逐项复测。此项仍处于开工准备。 [当前能力](review/11-project-control.md) · [人工漏洞整改开工计划](planning/phase6-b2-kickoff-plan.md) · [两张验收卡](planning/phase6-b2-acceptance-cards.md)

## 先选择阅读路径

- **所有者看进度与待决**：从[规划入口](planning/README.md)查当前范围、工作包、设计深度、待决与交付出口。按需进入具体合同，不必逐篇读历史报告。
- **遇到不熟悉的编号**。从[规划入口的编号索引](planning/README.md#常见编号怎么读)按事项种类查来源。例如 B2 是人工漏洞整改的发布验收点，B02 是抄送后可见和评论的业务决策。
- **准备某个工作包开工与交接**：从[按工作包找合同与交接](planning/README.md#按工作包找合同与交接)定位本批有效设计、开工条件与交付物；通用说明用[能力交付评审模板](toolkit/templates/capability-delivery-review.md)，不照搬某一阶段的输入或结论。
- **开发者修改一个模块**：[产品边界](vision.md) → [开发指南](guides/development.md) → [工程公约](standards.md) → 对应[当前模块](modules/README.md)、Phase/工作包及[目标合同](design/README.md)。编码前冻结本批预期，按公约测试先行。
- **运维或审计核实事实**：[运维入口](ops/README.md)或[审查入口](review/README.md) → 准确源码/制品/环境与验证记录 → 原始证据/历史报告。旧报告只证明其固定时点。

第一次接手可先看[架构导览](design/architecture.md)，再沿[工单模块](modules/ticket.md)查一次变更。比如经办人提交状态转换时，入口资格、工单可见范围和当前动作资格在哪儿检查，状态和历史在哪儿提交。研发计划中的旧票钉版、逐项复测等规则查[工单详设](phase6/ticket-module-detail.md)。

## 按任务找文档

| 任务 | 入口 | 内容边界 |
|---|---|---|
| 本地开发、生成、测试 | [guides/](guides/README.md) | 可操作步骤、前置和预期结果 |
| 理解当前模块 | [modules/](modules/README.md) | 当前调用链、数据与权限，不把目标目录当已迁移 |
| 查架构、目标合同和决策 | [design/](design/README.md)、[adr/](adr/README.md) | 当前解释与目标设计分别标注；采纳不等于实施 |
| 查当前API | [api/](api/README.md) | 响应/错误、Swagger及准确消费者合同；候选接口另标状态 |
| 部署、备份、故障处理 | [ops/](ops/README.md)、[工具构建与扫描](ops/tools/README.md) | 开发/测试/生产范围；测试账号唯一台账在runbook §4 |
| 看当前实现或复核证据 | [review/](review/README.md) | 当前控制、审查方法与固定时点报告 |
| 复用审计方法、模板、提示词 | [toolkit/](toolkit/README.md) | 方法、空白原件与执行实例分开维护 |
| 查后续计划及原始来源 | [planning/](planning/README.md)、[proposal/](proposal/README.md) | 当前交付顺序与来源承接，不直接作为完成证明 |

## 阶段导航

**看近期开发**。Phase 1–5 的实现已合入各仓主线。Phase 6 先做账号撤权和实际业务写入前的权限复核，再交付工单定义发布与证据转换，随后联通第一条人工风险整改旅程。实际开工日、实施授权与真实试用输入仍待绑定。后续能力及其工程量按[交付排期](planning/intranet-uaa-release-schedule.md)和[规划总账](planning/planning-backlog.md)核对。

[Phase1](phase1/README.md) → [Phase2](phase2/README.md) → [Phase3](phase3/README.md) → [Phase4](phase4/README.md)为已交付阶段的设计与记录；[Phase5](phase5/README.md)保留已合main的能力和债务出处。[Phase6](phase6/README.md)是近期可信底座/风险/体验切片，[Phase7](phase7/README.md)承接受控能力与专业集成，[Phase8](phase8/README.md)承接领域Agent/专业执行，[Phase9](phase9/README.md)承接持久协同；P10及更远候选见路线图，不为目录完整性建立空阶段稿。

旧Phase分模块稿保留来源与细节，适用/替代关系从现行阶段入口、共同合同和总账核对。设计候选不等于所有DTO/DDL/测试均已冻结；各模块开工前仍有自己的冻结和测试出口。Phase5交付与清理的固定记录见[验收记录§8](phase5/12-scenarios-and-tests.md#8-phase-5-收尾与-phase-6-承接2026-11-11-整理)及[运行手册](ops/runbook.md)。

旧排程按单席、每周 5 个有效工程日计算。B2 按约一人产能准备，实际开工日、每周可用时段和实施授权仍需确认。 [排程资源说明](planning/execution-schedule.json) · [B2 待确认输入](planning/owner-decision-agenda.md#详设审计后的排产选择)

## 五仓各自维护，结构统一

- **zhuzhao**：核心应用、IAM、工单、权限、网关及生态公约/规划，本树为权威。
- **zhuzhao-ui**：[本地文档](../../zhuzhao-ui/docs/README.md) / [UI文档（main）](https://github.com/tracerbiubiubiu/zhuzhao-ui/tree/main/docs)，维护前端开发、组件、测试和发布。
- **zhuzhao-utils**：[本地文档](../../zhuzhao-utils/docs/README.md) / [公共库文档（main）](https://github.com/tracerbiubiubiu/zhuzhao-utils/tree/main/docs)，维护具体公共包、接入和版本纪律。
- **taskrunner**：[本地文档](../../taskrunner/docs/README.md) / [任务服务文档（main）](https://github.com/tracerbiubiubiu/taskrunner/tree/main/docs)，维护受理、调度、投递、运行记录和部署。
- **activelist**：[本地文档](../../activelist/docs/README.md) / [动态清单文档（main）](https://github.com/tracerbiubiubiu/activelist/tree/main/docs)，维护PostgreSQL模型、Schema、数据操作和部署。

本地入口适用于五仓同级检出的布局，读取对应工作树内容。Web 入口指向各仓主线。查特定候选时，以交接记录列出的仓库、分支和提交为准。组件的配置、API 与部署以归属仓的说明为准。

## 维护规则

[工程公约](standards.md)管代码/API/权限/迁移和测试先行；[文档维护约定](documentation.md)管格式、分类、状态与迁移；[致谢](acknowledgments.md)记录参考来源。根AGENTS是协作入口。可复用方法、空白模板和开发/审计提示词统一从[工作资料库](toolkit/README.md)进入；填写后的实例按文档规则归正式报告或本地批次。

当前解释、目标设计、机器事实和历史报告分别维护；Markdown摘要引用或校验唯一JSON事实，不再独立报价/计数。正式报告在review/reports，必要历史在review/archive；本地参考、执行卡、原日志和临时输出按[材料目录](materials.md)归本地档案或ignored执行目录。Swagger生成物保留现有消费路径，不手改。分档门禁和远端保护未因文档整理自动启用。

## 变更记录

| 日期 | 变更 |
|---|---|
| 2026-11-08 | 五仓各自docs、统一分类，区分当前/未来/历史；保留阶段和Swagger路径。 |
| 2026-11-11 | 同步Phase5收尾和问题移交，后续原稿作为重规划输入。 |
| 2026-11-16 | 当前规划迁入planning，审查与历史保留review；增加Git/本地材料边界和迁移清单，补齐五仓本地入口、工作资料与运维工具导航。 |
| 2026-11-13 | 按所有者/开发/运维审计重排阅读路径，校准阶段/资源与未计价增量，明确候选和main入口。 |

全项目后续重规划主读[路线图](roadmap.md) → [渐进演进](design/project-evolution-strategy.md) → [能力承接审计](planning/roadmap-capability-continuity.md) → 唯一总账；Phase6仅为近期一段。比较方案变化读[路线对照](planning/roadmap-change-comparison-2026-11-12.md)，查看已计价先后读[执行排期](planning/execution-schedule.md)，当前完整范围同时包含CE、旧漏估子范围与既有包复估；历史日窗不作为新承诺。
