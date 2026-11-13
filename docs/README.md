# zhuzhao 文档入口

> **用途**：按阅读任务进入五仓资料；本页是导航，不是功能验收表。**范围**：当前实现、后续设计与历史证据。**权威**：[控制台](review/11-project-control.md)、[规划收口](review/planning-closeout.md)及各模块合同；状态以对应源码、候选和收据为准。

## 先选择阅读路径

- **所有者看进度与待决**：[规划收口](review/planning-closeout.md) → [模块设计深度](review/planning-closeout.md#各phase和模块细化到哪里) → [三节短审阅卡](phase6/implementation-handoff.md#单维护者短审阅卡)。再按需要进入具体合同，不必逐篇读历史报告。
- **开发者修改一个模块**：[产品边界](vision.md) → [开发指南](guides/development.md) → [工程公约](standards.md) → 对应[当前模块](modules/README.md)、Phase/工作包及[目标合同](design/README.md)。编码前冻结本批预期，按公约测试先行。
- **运维或审计核实事实**：[运维入口](ops/README.md)或[审查入口](review/README.md) → 准确源码/制品/环境与验证记录 → 原始证据/历史报告。旧报告只证明其固定时点。

第一次接手可先读[架构导览](design/architecture.md)，了解五仓、三层鉴权及调用边界；要判断某项能力是否已实现，再查控制台和实际候选。

## 按任务找文档

| 任务 | 入口 | 内容边界 |
|---|---|---|
| 本地开发、生成、测试 | [guides/](guides/README.md) | 可操作步骤、前置和预期结果 |
| 理解当前模块 | [modules/](modules/README.md) | 当前调用链、数据与权限，不把目标目录当已迁移 |
| 查架构、目标合同和决策 | [design/](design/README.md)、[adr/](adr/README.md) | 当前解释与目标设计分别标注；采纳不等于实施 |
| 查当前API | [api/](api/README.md) | 响应/错误、Swagger及准确消费者合同；候选接口另标状态 |
| 部署、备份、故障处理 | [ops/](ops/README.md) | 开发/测试/生产范围；测试账号唯一台账在runbook §4 |
| 看当前状态或复核证据 | [review/](review/README.md) | 当前控制、规划权威、审查方法与固定时点报告 |
| 查后续计划及原始来源 | [路线图](roadmap.md)、[总账](review/planning-backlog.md)、[proposal/](proposal/README.md) | 当前交付顺序与来源承接，不直接作为完成证明 |

## 阶段导航

**当前阅读口径（2026-11-13）**：Phase5已整合五仓main；P6–P10及更远路线已形成分层设计/排期候选，设计深度不一，仍在补充场景，业务T0未启动。原71包/UAA为已计价基线，[互通五项增量](review/capability-interconnect-supplement.md)尚未复估，不能把旧日窗当已含新增范围。

[Phase1](phase1/README.md) → [Phase2](phase2/README.md) → [Phase3](phase3/README.md) → [Phase4](phase4/README.md)为已交付阶段的设计与记录；[Phase5](phase5/README.md)保留已合main的能力和债务出处。[Phase6](phase6/README.md)是近期可信底座/风险/体验切片，[Phase7](phase7/README.md)承接受控能力与专业集成，[Phase8](phase8/README.md)承接领域Agent/专业执行，[Phase9](phase9/README.md)承接持久协同；P10及更远候选见路线图，不为目录完整性建立空阶段稿。

旧Phase分模块稿保留来源与细节，适用/替代关系从现行阶段入口、共同合同和总账核对。设计候选不等于所有DTO/DDL/测试均已冻结；各模块开工前仍有自己的冻结和测试出口。Phase5交付与清理的固定记录见[验收记录§8](phase5/12-scenarios-and-tests.md#8-phase-5-收尾与-phase-6-承接2026-11-11-整理)及[运行手册](ops/runbook.md)。

规划产能已按单席/每周5工程日确认，实际开工日T0尚未启动且无严格截止；当前口径以[排程资源说明](review/execution-schedule.json)和[所有者决策](review/planning-decisions.json)为准。源码整合、实际部署、功能开工各自留证，不从目录名或排期推断。

## 五仓各自维护，结构统一

- **zhuzhao**：核心应用、IAM、工单、权限、网关及生态公约/规划，本树为权威。
- **zhuzhao-ui**：[UI文档](https://github.com/tracerbiubiubiu/zhuzhao-ui/tree/main/docs)，维护前端开发、组件、测试和发布。
- **zhuzhao-utils**：[公共库文档](https://github.com/tracerbiubiubiu/zhuzhao-utils/tree/main/docs)，维护具体公共包、接入和版本纪律。
- **taskrunner**：[任务服务文档](https://github.com/tracerbiubiubiu/taskrunner/tree/main/docs)，维护受理、调度、投递、运行记录和部署。
- **activelist**：[动态清单文档](https://github.com/tracerbiubiubiu/activelist/tree/main/docs)，维护PostgreSQL模型、Schema、数据操作和部署。

以上Web入口指向已整合main；尚未合main的设计/版本候选应按准确分支和SHA查阅，不能由main链接证明候选已发布。跨仓只引用归属合同。各仓职责一致，公共库/UI/Go服务采用各自适用目录，不要求物理同构。仓库及产品新名称仍待确定，本次沿用现名。

## 维护规则

[工程公约](standards.md)管代码/API/权限/迁移和测试先行；[文档维护约定](documentation.md)管格式、分类、状态与迁移；[致谢](acknowledgments.md)记录参考来源。根AGENTS是协作入口。

当前解释、目标设计、机器事实和历史报告分别维护；Markdown摘要引用或校验唯一JSON事实，不再独立报价/计数。正式报告在review/reports，必要历史在review/archive；本地参考、执行卡、原日志和临时输出仍进ignored目录。Swagger生成物保留现有消费路径，不手改。分档门禁和远端保护未因文档整理自动启用。

## 变更记录

| 日期 | 变更 |
|---|---|
| 2026-11-08 | 五仓各自docs、统一分类，区分当前/未来/历史；保留阶段和Swagger路径。 |
| 2026-11-11 | 同步Phase5收尾和问题移交，后续原稿作为重规划输入。 |
| 2026-11-13 | 按所有者/开发/运维审计重排阅读路径，校准阶段/资源与未计价增量，明确候选和main入口。 |

全项目后续重规划主读[路线图](roadmap.md) → [渐进演进](design/project-evolution-strategy.md) → [能力承接审计](review/roadmap-capability-continuity.md) → 唯一总账；Phase6仅为近期一段。比较方案变化读[路线对照](review/roadmap-change-comparison-2026-11-12.md)，查看已计价先后读[执行排期](review/execution-schedule.md)，再合并尚未计价增量。
