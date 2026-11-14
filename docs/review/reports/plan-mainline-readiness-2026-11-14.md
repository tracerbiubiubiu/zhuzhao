# 关键模块与大阶段规划合主线前复核

日期：2026-11-14；复核基线：主仓候选 `c335061695ecbf4191388d1bd2569e18651913ae`。
复核范围：关键特性、模块承接和大阶段方向；不重新估价，不逐项冻结所有未来DTO/DDL，也不作全产品重新审计。
输入：[路线图](../../roadmap.md)、[总账](../planning-backlog.json)、[原会话复核](../conversation-reaudit.json)、[后继输入](../planning-followups.json)、[首版计划](../../phase6/first-usable-release-plan.md)及[模块交接](../../phase6/module-design-handoff.md)。

## 结论与本次合入范围

本次抽查未发现应阻止规划同步main的关键模块遗漏或大阶段方向问题；结论来自下列业务语义与权威承接核对，不能依据条目数量推导全部未来细节均无遗漏。
用户最新明确授权五仓“规划与已验证升级一起合入”main；授权范围仍不含部署、共享环境切换或规划中新业务实现。
本报告基线的主仓候选相对本地 `origin/main...HEAD` 涉及217个文件，包含继承的工具链、依赖、容器、CI与部署模板修改，不能作为纯文档分支交付。
升级既有验证引用[版本维护记录](version-modernization-2026-11-12.md)；准确五仓候选、实际测试、PR检查及main结果须另以本轮收据绑定，本报告不提前宣称已合main或已发布。

## 已覆盖的关键分组

- **工程与单维护者质量**：G01–G06承接worktree/运行资源归属、同文件并发隔离、固定五仓CI、必需检查、风险测试/fuzz及目录规则同步。
  权威：[工程施工](../../design/engineering-execution-design.md)G01–G06、[工程治理](../../design/engineering-governance.md)“单维护者的最小质量闭环”、[模块交接](../../phase6/module-design-handoff.md)§3；真实行为红→绿与现行完整适用门禁继续有效。
- **风险、工单、TR、AL与消息闭环**：首条旅程明确AL变化触发获权TR后继，列表刷新或通知不能替代该效果；各边分别定义写主、当前授权、operation/receipt、可靠交接与失败恢复。
  权威：[首版计划](../../phase6/first-usable-release-plan.md)§2、[模块交接](../../phase6/module-design-handoff.md)§1六条边、总账D08–D12/D15/R01及CE02–CE05。
- **统一能力、外部API与MCP后继**：核心和可插拔能力共用受控用例、结果/事件合同；手动、工单、计划、事件与AI入口逐步接线，MCP薄映射同一能力。
  权威：[调用合同](../../design/capability-invocation-contract.md)、[互通详设](../../design/capability-interconnect-detail.md)、总账CE01–CE05/I01/U08/A03。
- **完整本地三层权限与UAA双模式**：本地核心保留账号/角色/菜单和完整L1/L2/L3；先留UAA-C身份、source和稳定资源接缝，完整P/R在核心稳定后承接，企业治理不缩成纯SSO。
  权威：[首版计划](../../phase6/first-usable-release-plan.md)§1/§4、[模块交接](../../phase6/module-design-handoff.md)§2、[UAA合同](../../design/uaa-governance-integration.md)；独立模式正式运行不等真实UAA。
- **私有adapter、长期mock与正式数据**：私有SDK/字段/凭据留内网；首次正式消费者即保护兼容，验证旧参考adapter×新core；正式UID、归属、附件/审计、receipt及在途效果不能按测试卷重建。
  权威：[首版计划](../../phase6/first-usable-release-plan.md)§5、[扩展兼容](../../design/extension-compatibility.md)、[内网适配](../../design/intranet-adapter-migration.md)、总账X01/X02/E01。
- **HTTP消息与日志审计**：真实公共HTTP渠道仍属核心，区分受理/送达/失败/待核实；日志、业务历史和专门审计职责分开，统一能力沿OBS01/P6.0、OBS02/P6.1、OBS03/P7渐进交付。
  权威：[首版计划](../../phase6/first-usable-release-plan.md)§2、[统一日志审计](../../design/unified-observability-audit.md)§3/§8、后继F006/F007/F013；既有日志或mock不代验新增能力。
- **工单职责、数据权限与运营**：覆盖风险组受限派单、类型查看范围、CC查看评论但无附送办理权、业务管理者受限干预、单主责多知会、轮询/认领、响应/节点/处理/闭环时长及流程通知。
  权威：[生命周期](../../design/business-lifecycle-contracts.md)§4–§9、[运营补充](../../design/ticket-operations-supplement.md)、后继F004/F006；AL行字段、设备信息及角色统计均有当前授权约束。
- **附件、调度与组织交接**：文件上传默认追加并保留旧附件，显式手删与引用保护分开；错失三策略和超限hold保留余项；离职先禁用再按批准映射交接，撤部预览plan后批量接管。
  权威：[工程施工](../../design/engineering-execution-design.md)C02、[调度恢复](../../design/schedule-recovery-policy.md)、[生命周期](../../design/business-lifecycle-contracts.md)§7及总账C03/D06/I03/I05。
- **专业供给、AI与后续业务承接**：复用SOC/SIEM、CMDB、TIP、SOAR和已有模型API，保留领域Agent/外部平台双轨、RAG、skill、自然语言查询/建单/计划及统一工作流入口，不造大而全平台。
  权威：[路线图](../../roadmap.md)§3–§6、[能力连续性](../roadmap-capability-continuity.md)、总账I04/A01–A07/R02–R05/E03–E08；P7–P10及更远能力有明确承接窗口。
- **设备、全站体验与渐进架构**：设备目录/状态/受权管理入口按异构设备人工登录交付；首页、多选标签、筛选搜索、情报左右栏及全站迁移继续承接，模块化单体按运行证据渐进拆分。
  权威：总账X03/U01–U07/E07、[设备合同](../../design/device-module-contract.md)、[一致性合同](../../design/ecosystem-consistency-contract.md)及路线图§2/§5。
- **组件维护与公开展示**：G07A/B承接Go/Node/PG/Redis/容器升级和恢复演练；中英文README、合成展示与社区入口已归属，在线demo、公开准备及正式改名保留后续窗口。
  权威：[版本现代化](../../design/dependency-modernization.md)、[展示方案](../../guides/open-source-presentation.md)、后继F005及[确认议程](../owner-decision-agenda.md)Q07–Q13；不把品牌建议当已执行改名。

## 留给具名详设窗口的事项

- **当前模块编码前**：按模块交接冻结本批可信Context、写主、输入/输出、当前授权、事务/可靠交接及用例；准确DDL核现行迁移地图，不要求今天冻结全部远期细节。
- **D03/LS01、D02对应写路径开工前**：绑定实际类型/组织/角色/字段与管理动作，冻结最小持久接单/节点事实；P6.1出口前冻结首组指标、暂停和重开口径。
- **E03与D11/P6.3窗口**：确定轮询/认领类型、候选池、无人及超时处置、关键结果/节点通知矩阵；所选增量按窗口扣重复估，不视作旧价全包。
- **首次真实AL敏感数据及X03启用前**：冻结所需最小type/row/field纵切、受权统计与地址/可选用户名范围；相关保护未具备时保持合成数据或关闭该真实入口。
- **核心真实消息及首次生产前**：绑定HTTP合同/鉴权/回执和实测条件、实际流程/字段/动作/订阅、部署目标/人员、审计名单及保留期；适用制品、迁移、恢复和生产证据门仍须通过。
- **IAM高风险写详设前**：两项修改在本地必需审计失败时是否同事务回滚仍待答；已确认的业务历史与专门审计分权不代替该写失败政策。
- **UAA-P/R及企业激活前**：核真实认证/三feed、映射、来源与接管范围、撤权新鲜度、维护恢复和受控退出；公司材料留内网，不能以合成mock代销真实验收。
- **旧账与远期包窗口**：E10在P6.1核存量与迁入/合并范围，获选建设P6.E另估；其余专业提供方、复杂协同、生产拓扑及改名按各包/议程冻结，不以“待触发”无限挂账。

## 日期、状态与本次验证限界

1月仅为争取试用窗口，质量及实际内网可用性优先；T0、真实联调等待和后续子范围尚未全部绑定，现有工程日/周次均为参考，不新增硬日期承诺。
首版本地核心是当前交付切片；AI、设备、高级设计器、完整迁版/恢复台及其它后继能力没有因本次裁范围退役，未交Phase6功能仍按总账承接。
本次仅定向阅读文档/总账并核候选范围，未运行产品测试、未更改业务代码、未提交、未合并、未部署；覆盖判断不构成新业务已实现或既有升级再次通过门禁的证明。
本报告只新增本文件；本轮测试、候选差异、PR与main的最终事实由集成会话核验并给出独立收据，若候选改变应按实际变化复核受影响边界。
