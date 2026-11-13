# 架构与设计解释

近期先读[基础版与每期交付边界](foundation-release-boundaries.md)：现有底座可入网试验、最小adapter/私有应用接缝优先，完整企业治理和高阶增强按启用能力验收；当前说明不代表这些接缝已实现。

后继F004的主责、范围派单、业务管理员、时长度量、通知和AL/设备/仪表盘权限见[工单运营补充](ticket-operations-supplement.md)：F006已确认五组方向，实际配置与新增净估仍按窗口处理。F006另确认Q01–Q06方向，要求[UAA离线mock](examples/uaa-reference-mock/README.md)及[统一日志审计设计](unified-observability-audit.md)；不要求导出公司原文，不表示产品或真实接入已经完成。

> **用途**：定位当前架构解释、目标设计及后继决策。**状态**：各文档分别标注设计/实施/验证状态；本索引不证明能力已交付。**权威**：当前事实查[模块](../modules/README.md)/[控制台](../review/11-project-control.md)，目标合同按下方归属引用。

## 先区分当前实现和目标设计

- **理解现有系统**：先读[系统架构](architecture.md)，按问题进入[权限继承与级联](rbac-inheritance-and-cascade.md)及模块说明。图中的服务关系不表示已批准全面微服务拆分。
- **修改后续能力**：先看[规划收口/设计深度](../review/planning-closeout.md)，再读相应共同/领域合同，最后回读原Phase来源；候选字段/schema不等于当前API。
- **查为什么这样选**：[ADR](../adr/README.md)给决策及后继；[设计决策](design-decisions.md)保留讨论时间序，[旧系统对比](system-comparison.md)保留设计来源。旧“当前”一词须结合日期和后继解释。

## 第一次阅读时的术语

- **IAM**：身份与访问管理，回答是谁、有哪些功能资格及可操作哪些数据。
- **L1/L2/L3**：分别检查入口资格、资源可见性和当前动作；能打开页面不代表能读任意工单，更不代表可删除。实际例外见[权限地图](../modules/authz.md)。
- **SSOT**：单项事实或规则的唯一维护源；摘要引用它，避免多份正文漂移。
- **AK/SK**：识别与验签服务请求，不代替浏览器用户或业务资源授权。公共协议与数据写主见[五仓边界](architecture.md#13-项目与生态边界)。
- **已确认/已实现/已验证/已部署**：分别是方向/代码/准确候选检查/指定环境运行，不能互相推导。

## 用一个请求串起文档

浏览器打开或操作工单，经[中间件](../modules/middleware.md)认证/入口检查，再由[工单用例](../modules/ticket.md)核资源与动作权限，最后通过repository读写PG。附件、任务和通知各有调用/提交后边界；工单事务成功不代表独立后继全部成功。排障先定位段落，再按request_id找其日志与错误。

## Phase5之后的现行演进

这些是后续方向/合同入口，实施状态仍回到各工作包和准确收据：

- [平台边界](platform-evolution.md)、[全项目渐进演进](project-evolution-strategy.md)：专业供给、数据写主、三类流程、扩展与拆分；原规划价值见[连续性审计](../review/roadmap-capability-continuity.md)。
- [能力互通/共同执行路径](capability-invocation-contract.md)、[生态一致性](ecosystem-consistency-contract.md)：核心/插件共同调用与三条链、统一语义和允许差异；新范围见[互通增量](../review/capability-interconnect-supplement.md)。
- [AI能力](ai-capabilities.md)：已有模型API、外部与平台领域Agent双轨、MCP、受权知识和skill；不建设模型部署/通用知识平台。
- [五仓工程治理](engineering-governance.md)：隔离、CI/PR、mock分层、质量/目录和发布；[总账](../review/planning-backlog.md)维护唯一范围/依赖/粗估。

## 详设候选与新增边界

| 场景 | 权威设计入口 | 实施前仍需核什么 |
|---|---|---|
| 工程、事务与权限 | [工程施工](engineering-execution-design.md)、[当前模块事务/边界](current-module-boundaries.md) | 真实入口/守卫、当前批准确切片和测试预期 |
| 命令执行、调度与结果 | [跨仓执行](cross-repo-execution-contract.md)、[调度恢复](schedule-recovery-policy.md) | 凭证/租约/unknown、具体发生次/恢复配置及真实故障链 |
| 工单、风险、人员生命周期 | [业务生命周期](business-lifecycle-contracts.md) | 定义/实例/权限、事务和候选接口；准确DTO/迁移按批冻结 |
| 私有模块和内网数据接入 | [扩展兼容](extension-compatibility.md)、[adapter迁移](intranet-adapter-migration.md) | manifest/支持组合、写主/来源和所选真实provider |
| 企业统一认证 | [UAA治理/双模式](uaa-governance-integration.md) | 独立与企业分别验、来源/撤权、最小UI/运维和真实激活条件 |
| 统一日志与业务审计 | [日志审计合同](unified-observability-audit.md)、[standards §6](../standards.md#6-可观测性) | 可信上下文、敏感值/日志权限、本地持久与跨仓交接、查询/归档、实际多实例反例；不把trace当业务成功 |
| AI/MCP与计划确认 | [AI调用](ai-invocation-contract.md)、[MCP首批](mcp-first-contract.md)/[Schema样本](mcp-first-contract.schema.json)、[定时AI确认](scheduled-ai-confirmation.md) | 精确批准/grant、数据ACL、协议/SDK互操作及适用预算 |
| 设备薄模块 | [设备合同](device-module-contract.md) | 目录/状态/人工入口；不承诺异构设备SSO |
| 版本与数据升级 | [版本现代化](dependency-modernization.md)、[恢复契约](component-upgrade-data-contract.md) | 准确制品/数据组合、停写/恢复和回退边界；[工具说明](../../deployments/tools/README.md) |

[规划收口](../review/planning-closeout.md)区分详设候选、组件原型、逐页范围和边界；不是全部远期API/DDL已冻结。[新旧对照](../review/roadmap-change-comparison-2026-11-12.md)解释取舍，[执行排期](../review/execution-schedule.md)给已计价基线，[UAA补估](../review/uaa-implementation-estimate.md)负责独立数字源；CE及旧子范围已扣重复估，当前完整排程另见[净增注册表](../review/planning-scope-estimates.json)；有研究证据不等于产品已经实现。

## 本轮补齐的可检查材料

- [逐操作库存](capability-operation-catalog.md)、[三链精确合同](capability-interconnect-detail.md)/[Schema](capability-interconnect-detail.schema.json)：现态枚举、目标消息/水位/授权/恢复和最小UI运维分开。
- [旧子范围](legacy-scope-completion.md)：字段ACL、类型化检索、CmdK、真值diff、第二专业源及生产MCP身份桥。
- [领域Agent比较](domain-agent-reference-scenario.md)、[skill样包](skill-reference-package.md)、[SOC职责](soc-collaboration-authorization.md)及[可执行样例](examples/README.md)：合成模型与真实SDK证据分别说明，不冒称企业接通。

## 变更记录

- 2026-11-13：按当前实现/后续目标/决策来源分流，按场景列合同归属，删除重复数字摘要；保留原阶段、生成物和合同路径。
