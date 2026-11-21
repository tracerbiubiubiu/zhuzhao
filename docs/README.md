# zhuzhao 文档入口

目前可以用账号、组织和角色分配工单权限。下一批准备做人工漏洞整改（发布验收点 B2）。例如同一位风险负责人有两项漏洞待修，风险管理者可以把它们派到一张工单上。办理人分别提交修复证据，风险负责人逐项复测。 [当前能力](review/11-project-control.md) · [人工漏洞整改开工计划](planning/phase6-b2-kickoff-plan.md) · [两张验收卡](planning/phase6-b2-acceptance-cards.md)

## 先选择阅读路径

- **想知道接下来要做什么**。从[规划入口](planning/README.md)查预计先交哪项功能、工作人员还缺哪些信息、何时需要你决定。每项再链接到具体设计或历史报告。
- **遇到不熟悉的编号**。从[规划入口的编号索引](planning/README.md#常见编号怎么读)按事项种类查来源。例如 B2 是人工漏洞整改的发布验收点，B02 是抄送后可见和评论的业务决策。
- **准备开发一项功能**。按[工作包查到对应设计](planning/README.md#按工作包找合同与交接)，确认这次改哪些地方、缺什么前提、怎样验收。项目和代码规则分别见[产品边界](vision.md)、[开发指南](guides/development.md)和[工程公约](standards.md)。
- **排查或核对已交付功能**。从[运维入口](ops/README.md)找部署、故障和恢复办法，从[审查入口](review/README.md)查哪一版程序在什么环境通过了哪些检查。

例如办理人点“关闭工单”，系统先检查他的账号能否使用关闭功能，再查他是否有权看这张工单、能否关闭它。通过检查后，系统才保存状态和操作历史。 [工单模块](modules/ticket.md) · [架构导览](design/architecture.md)

试用只接收试用期间按指定整改类型创建的工单。升级前已经存在的工单，以及发布新规则以后工单用哪版规则，都各有测试。 [试用与已有工单的例子](planning/phase6-b2-kickoff-plan.md#旧票范围与定向验收)

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

**看近期开发**。Phase 1–5 的实现已合入各仓主线。Phase 6 先做账号停用后让旧登录失效，以及用户点保存时再查一次当前权限。接下来让工单记住创建时用的规则，并在进入下一步时收集整改证据。最后用两条风险验证从派单到复测的全程。实际开工日、每周可投入的时间和实施授权仍待确认。真实试用前还要确认人员、数据和环境。 [交付排期](planning/intranet-uaa-release-schedule.md) · [工作包总账](planning/planning-backlog.md)

[Phase 1](phase1/README.md)、[Phase 2](phase2/README.md)、[Phase 3](phase3/README.md)、[Phase 4](phase4/README.md)和[Phase 5](phase5/README.md)保留账号、权限、工单、前端和通用能力的设计及交付记录。[Phase 6](phase6/README.md)先准备人工整改。[Phase 7](phase7/README.md)计划让工作人员从工单调用已获准的外部专业服务。[Phase 8](phase8/README.md)计划接入智能体和专业执行平台。[Phase 9](phase9/README.md)计划让跨系统任务在中断后能查进度、恢复并由人接管。更远的候选见[路线图](roadmap.md)。

早期设计保留当时的取舍。准备开发时，从当前阶段入口查本次实际采用哪版设计，再确认接口字段、数据库变更和验收用例。Phase 5 的交付和清理见[验收记录 §8](phase5/12-scenarios-and-tests.md#8-phase-5-收尾与-phase-6-承接2026-11-11-整理)和[运行手册](ops/runbook.md)。

原排期按一名开发人员每周投入 5 天计算。 [排期的人员假设](planning/execution-schedule.json) · [B2 待确认输入](planning/owner-decision-agenda.md#详设审计后的排产选择)

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
