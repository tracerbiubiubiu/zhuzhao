# 历史触发项逐源合理性裁定（FD02）

日期：2026-11-12。本文是现行路线的补研究，不是实现完成或生产验收。唯一包/依赖/粗估仍见[总账](planning-backlog.md)；本次没有修改总账或控制台。完整原行、来源hash、证据和子窗口见[机器索引](legacy-trigger-disposition.json)。

后续集成已将T045纠正到U02/U06、T033分到D13/D14、T083明确到U01/U02/D10/U06；T065既有五包关联保留。上面“本次没有修改总账”指本份独立补研的写入边界，当前范围/复估以[新版执行排期](execution-schedule.md)与[集成报告](reports/near-term-research-and-schedule-2026-11-12.md)为准，不据下文“建议root同步”推断尚未处理。

## 来源与裁定口径

当前来源映射中，指向 `docs/phase6/07-backlog-and-triggers.md` 的实际来源是 **119条：P6-T001–116共116条原表行，加CARRY-TASKUX、CARRY-CROSSMETRICS与CONT-ENG-06三条**。最后一条是跨行审计追加，不能制造成07号不存在的新表行。116条已按标题/次序与当前07号原文逐行匹配；额外来源另给真实定位。119不是119个独立功能。

本次裁定：保留26、改写72、专业替代15、退出旧独立待办6。**没有取消业务功能**；“退出”只用于重复立项或已有关闭依据的旧新建待办，仍给去重指向/回归。未知接口、未定参数与缺业务样本均保留未决状态，不能当取消或已实现。

判断顺序是：原条件能否描述真实价值或技术前提；是否已触发、已拍、已并账或已有关闭证据；是否属安全/完整性必须在消费者启用前完成；是否需专业供给；再给有界条件、明确责任和决策/交付窗口。不能再以“首次诉求/再漏一次/任意小批/内网启动”等文字无限挂账。

本文读取了现行逐包正文、07号、原债台账、工单完善与风险报表方案，并抽核当前事件装配、通知薄壳、Close、字段软提示、图标清单与Cookie模板等源码。旧缺陷、旧批准、静态实现和运行验证分别保留；没有启动服务、运行新接口或复跑旧绿测试。

## 本次实质修正

- Cmd+K与英文用户并无必然关系，原映射E04报告包不相称。建议U02/P6.1先判断、U06/P6.3交获选入口；这是待root同步的归属建议。
- “P2六组”须拆子项：watcher/字段ACL/predue/digest/配置小件不能全部被E05/P10承接；软删/跨端草稿与现有体验的窗口不同。
- 首页旧1–2天纯前端估算与全局四状态副行，不再覆盖安全入口、当前处理/明确复核、任务与消息三域的新合同。
- org_roles完整性不能等出现实际孤儿危害；handler测试的旧翻案触发已经发生。二者都应按现行固定包检查点处理。
- 已记录关闭的基础显隐、通知双路径、G12/G14不重复开发；升级链、多源信封不重复立项。专业替代保留本地受权消费和闭环，不把未知供给当取消。

## 逐源裁定

### 3.1 引擎演进类

**P6-T001 · S3b 审批流 — 改写**

- 原文：[docs/phase6/07-backlog-and-triggers.md](../phase6/07-backlog-and-triggers.md) / ### 3.1 引擎演进类 / S3b 审批流; 首个真实审批场景
- 理由：首个真实审批场景是防止空造引擎的合理价值门槛，但不能继续没有审查期限；当前风险接受已有确认制，不等于需要会签引擎。
- 当前条件：P6.3用接受确认/一个真实岗位步骤比较现有确认、专业流程与有限本地审批，保留必要效果；不默认建workflow全表族。
- 现存证据：[project-evolution-strategy.md](../../docs/design/project-evolution-strategy.md) / §1–8：保留业务效果、薄实现、先决策与后交付分开; [02-r2-assessment-treatment.md](../../docs/phase6/02-r2-assessment-treatment.md) / 处置四策略、工作日候选、接受确认与误报抑制; [platform-evolution.md](../../docs/design/platform-evolution.md) / §1/2/4/5/7：专业写主、可靠交接和按窗口冻结
- 承接与窗口：E03 (P6.3出口; P8)
- 出口与未决：交代表输入/资格/驳回/撤权/恢复样例；保留者P8给子窗，未证需复杂引擎则明确保留确认制并登记复核。；真实提供方/参数/配置未定时仍按窗口复核，不称取消或完成。

**P6-T002 · SLA 层 1 — 改写**

- 原文：[docs/phase6/07-backlog-and-triggers.md](../phase6/07-backlog-and-triggers.md) / ### 3.1 引擎演进类 / SLA 层 1; 硬五条任一（对外承诺/停表/业务日历/KPI/自动升级链）
- 理由：对外承诺、停表、日历、KPI、自动升级是五种不同合同；一个开关不应把全部SLA计时引擎设为必需。
- 当前条件：P6.3分清仅日历换算与真正停表/暂停工时，各自用既有日历或薄配置；旧在途期限保持，显式重算另操作。
- 现存证据：[project-evolution-strategy.md](../../docs/design/project-evolution-strategy.md) / §1–8：保留业务效果、薄实现、先决策与后交付分开; [02-r2-assessment-treatment.md](../../docs/phase6/02-r2-assessment-treatment.md) / 处置四策略、工作日候选、接受确认与误报抑制; [04-r4-metrics-reports.md](../../docs/phase6/04-r4-metrics-reports.md) / §3/4/7/附录A：固定统计、快照、报告及跨模块限制
- 承接与窗口：E03 (P6.3出口; P8)
- 出口与未决：跨节假日/时区/暂停与日历版本样例；P8只交获选有限场景，不把等待任一信号当无限挂账。；真实提供方/参数/配置未定时仍按窗口复核，不称取消或完成。

**P6-T003 · 升级链 — 退出旧独立待办**

- 原文：[docs/phase6/07-backlog-and-triggers.md](../phase6/07-backlog-and-triggers.md) / ### 3.1 引擎演进类 / 升级链; 已被 B0-4 条件路由吞并
- 理由：原文已说明升级链被条件路由吞并，继续独立立项会重复估算和重复执行；退出的是旧独立项，不是升级业务效果。
- 当前条件：与P6-T005/P6-T020合并，由D16/D17声明哪些是本地超期路由、哪些是专业SOAR执行。
- 现存证据：[project-evolution-strategy.md](../../docs/design/project-evolution-strategy.md) / §1–8：保留业务效果、薄实现、先决策与后交付分开; [platform-evolution.md](../../docs/design/platform-evolution.md) / §1/2/4/5/7：专业写主、可靠交接和按窗口冻结; [s6-s5-debt-ledger.md](../../docs/phase5/s6-s5-debt-ledger.md) / B/C/G/H组及挂账质量判据：历史状态与触发原意
- 承接与窗口：D16 (本波开工前; P6.3)
- 支持/决策关联：D17；不是本源新增独立交付，不重复计价。
- 去重：业务效果显式归入 **P6-T005**，本行仅退出重复立项。
- 出口与未决：同一事件一次路由、上限/循环/无目标与通知回执可核；不得同时启旧升级器和新路由。；真实提供方/参数/配置未定时仍按窗口复核，不称取消或完成。

**P6-T004 · 表单条件显隐 — 退出旧独立待办**

- 原文：[docs/phase6/07-backlog-and-triggers.md](../phase6/07-backlog-and-triggers.md) / ### 3.1 引擎演进类 / 表单条件显隐; ~~「选 A 才填 B」联动诉求~~ **✅ 已随 S1 B1-6 交付销账（2026-10-28 核对）**
- 理由：旧“选A才填B”能力已记录交付；当前目标是防前后端漂移，不应把已存在visible_when重建。
- 当前条件：关闭旧基础能力新建待办，回归保留；全文法服务端预览/复杂条件能力按P6-T085继续建设。
- 现存证据：[business-lifecycle-contracts.md](../../docs/design/business-lifecycle-contracts.md) / §6：schema求值、约束与Query/prefill; [frontend-redesign.md](../../../zhuzhao-ui/docs/design/frontend-redesign.md) / FE-PRE-01–11：实际静态问题/代表方案/全站未实测限制; [s6-s5-debt-ledger.md](../../docs/phase5/s6-s5-debt-ledger.md) / B/C/G/H组及挂账质量判据：历史状态与触发原意
- 承接与窗口：D04 (本波开工前; P6.1)
- 出口与未决：已存在显隐/G2不退化；复杂表达式与提交同源，不能因关闭本行同步关闭085。；真实提供方/参数/配置未定时仍按窗口复核，不称取消或完成。

**P6-T005 · automation_rules — 改写**

- 原文：[docs/phase6/07-backlog-and-triggers.md](../phase6/07-backlog-and-triggers.md) / ### 3.1 引擎演进类 / automation_rules; 首个需管理员配置面的编排场景
- 理由：“首个管理员配置编排”只描述需求来源，未约束可配范围；平台现在明确不造通用SOAR。
- 当前条件：D16只承接工单默认边/条件/多命中/受控动作；跨系统复杂运行交专业编排，动作仍经授权。
- 现存证据：[project-evolution-strategy.md](../../docs/design/project-evolution-strategy.md) / §1–8：保留业务效果、薄实现、先决策与后交付分开; [platform-evolution.md](../../docs/design/platform-evolution.md) / §1/2/4/5/7：专业写主、可靠交接和按窗口冻结; [business-lifecycle-contracts.md](../../docs/design/business-lifecycle-contracts.md) / §2–5/7–11：定义、动作/权限、目录与风险闭环
- 承接与窗口：D16 (本波开工前; P6.3)
- 支持/决策关联：D17,R02；不是本源新增独立交付，不重复计价。
- 出口与未决：发布校验/条件全假/循环上限/CAS认领/崩溃恢复；同一业务效果只有一个编排主。；真实提供方/参数/配置未定时仍按窗口复核，不称取消或完成。

**P6-T006 · 多源内容采集 — 专业替代**

- 原文：[docs/phase6/07-backlog-and-triggers.md](../phase6/07-backlog-and-triggers.md) / ### 3.1 引擎演进类 / 多源内容采集; 桥接方案拍板
- 理由：旧“桥接方案拍板”不是可观察技术触发，且容易长成采集平台；用户已有专业日志/SOC等供给。
- 当前条件：I04在P7先两类代表源，私有adapter发送统一事实信封；保留接收、去重、可见、受权派单与失败回放。
- 现存证据：[project-evolution-strategy.md](../../docs/design/project-evolution-strategy.md) / §1–8：保留业务效果、薄实现、先决策与后交付分开; [platform-evolution.md](../../docs/design/platform-evolution.md) / §1/2/4/5/7：专业写主、可靠交接和按窗口冻结; [business-lifecycle-contracts.md](../../docs/design/business-lifecycle-contracts.md) / §8：专业来源、周期、处置与N:1整改单
- 承接与窗口：I04 (本波开工前; P7)
- 支持/决策关联：X02；不是本源新增独立交付，不重复计价。
- 出口与未决：按源ID/序号/完整性/重复/权限/恢复验证；未知provider只阻其real，不能取消接入效果。；真实提供方/参数/配置未定时仍按窗口复核，不称取消或完成。

**P6-T007 · 升级路径 outbox（C-20） — 改写**

- 原文：[docs/phase6/07-backlog-and-triggers.md](../phase6/07-backlog-and-triggers.md) / ### 3.1 引擎演进类 / 升级路径 outbox（C-20）; 「丢一条即事故」的消费者 / 跨进程消费者
- 理由：不可丢/跨进程是合理可靠性门槛，但D10等既定消费者已会使用；不能等实际丢失或只靠MQ宣称可靠。
- 当前条件：R01/P6.1先交一个不可丢消费者的事务intent或已证明完整重建；本地必需变更同tx，独立后继才异步。
- 现存证据：[cross-repo-execution-contract.md](../../docs/design/cross-repo-execution-contract.md) / C03/R01：提交、效果收据/意图、reassign三类成功含义; [business-lifecycle-contracts.md](../../docs/design/business-lifecycle-contracts.md) / §2–5/7–11：定义、动作/权限、目录与风险闭环
- 承接与窗口：R01 (本波开工前; P6.1)
- 支持/决策关联：C04；不是本源新增独立交付，不重复计价。
- 出口与未决：commit后kill、ACK丢失、重复、未知效果、撤权和回退有收据；不把日志或当前状态扫描直接当重建证明。；真实提供方/参数/配置未定时仍按窗口复核，不称取消或完成。

**P6-T008 · B-21 整合批剩余体验项 — 改写**

- 原文：[docs/phase6/07-backlog-and-triggers.md](../phase6/07-backlog-and-triggers.md) / ### 3.1 引擎演进类 / B-21 整合批剩余体验项; S2 走查结论
- 理由：S2走查结论可发现体验债，但“剩余体验”边界不清、旧整合批已拆归属；不能另开一个兜底包。
- 当前条件：U04逐项承接边属性/条件/动作/选中/问题定位与未保存保护，保留现有设计器资产，独立高级项入口裁决。
- 现存证据：[frontend-implementation-contract.md](../../../zhuzhao-ui/docs/design/frontend-implementation-contract.md) / 组件合同、六类工作区、全站映射、已确认UX01/02/03、跨阶段旅程; [frontend-redesign.md](../../../zhuzhao-ui/docs/design/frontend-redesign.md) / FE-PRE-01–11：实际静态问题/代表方案/全站未实测限制; [06-ticket-completeness.md](../../docs/phase6/06-ticket-completeness.md) / §3各件、§4 P2原触发与§5不做范围
- 承接与窗口：U04 (本波开工前; P6.3)
- 出口与未决：按字段/图同源、版本发布、长内容与键盘替代旅程逐项验，原B21不再重复计价。；真实提供方/参数/配置未定时仍按窗口复核，不称取消或完成。

### 3.2 phase6 域类

**P6-T009 · 真实漏扫 adapter — 保留**

- 原文：[docs/phase6/07-backlog-and-triggers.md](../phase6/07-backlog-and-triggers.md) / ### 3.2 phase6 域类 / 真实漏扫 adapter; 漏扫契约到位（换 mock）
- 理由：真实漏扫契约到位是real验收所需事实；但不应阻断公共合同、mock和源归一化研究。
- 当前条件：E01从P6.1列provider能力/owner/私有connector/真实子窗，D08先mock；P7至少一条具备合同的real纵切。
- 现存证据：[project-evolution-strategy.md](../../docs/design/project-evolution-strategy.md) / §1–8：保留业务效果、薄实现、先决策与后交付分开; [business-lifecycle-contracts.md](../../docs/design/business-lifecycle-contracts.md) / §8：专业来源、周期、处置与N:1整改单; [platform-evolution.md](../../docs/design/platform-evolution.md) / §1/2/4/5/7：专业写主、可靠交接和按窗口冻结
- 承接与窗口：E01 (P6.1出口起逐项核契约，P7出口首次真实验证复核; P7)
- 支持/决策关联：X02,D08；不是本源新增独立交付，不重复计价。
- 出口与未决：来源稳定ID/排序/分页/删除/授权逐项核；没有接口仍保留真实待验并在P7出口复核，不切mock假成功。；真实提供方/参数/配置未定时仍按窗口复核，不称取消或完成。

**P6-T010 · R1 第二档连接器（扫描器/SIEM webhook） — 改写**

- 原文：[docs/phase6/07-backlog-and-triggers.md](../phase6/07-backlog-and-triggers.md) / ### 3.2 phase6 域类 / R1 第二档连接器（扫描器/SIEM webhook）; 源接入诉求
- 理由：按源接入诉求拆adapter合理；扫描器/SIEM webhook不能共用一个未经鉴别的入口身份，也不应全源一口承诺。
- 当前条件：I04先SOC/日志或等效两类代表，入口签名/重放/限流与来源模型分离，每provider有有界子包。
- 现存证据：[project-evolution-strategy.md](../../docs/design/project-evolution-strategy.md) / §1–8：保留业务效果、薄实现、先决策与后交付分开; [business-lifecycle-contracts.md](../../docs/design/business-lifecycle-contracts.md) / §8：专业来源、周期、处置与N:1整改单; [cross-repo-execution-contract.md](../../docs/design/cross-repo-execution-contract.md) / C03/R01：提交、效果收据/意图、reassign三类成功含义
- 承接与窗口：I04 (本波开工前; P7)
- 支持/决策关联：X02；不是本源新增独立交付，不重复计价。
- 出口与未决：坏签名/重复投递/乱序/失败归一化/受权回放和本地可见性，real逐源留证。；真实提供方/参数/配置未定时仍按窗口复核，不称取消或完成。

**P6-T011 · R1保留期/归档 — 保留**

- 原文：[docs/phase6/07-backlog-and-triggers.md](../phase6/07-backlog-and-triggers.md) / ### 3.2 phase6 域类 / R1保留期/归档; 已拍默认365天归档/180天原始流水清理，随R1落地
- 理由：归档与历史可追溯不能等数据积累才补；原已拍365/180天是设计默认，不是实际部署已启用。
- 当前条件：随D08建立保留类别、归档/删除范围及失败可见；数据量仅用于回调容量与批次参数。
- 现存证据：[01-r1-ingest-ledger.md](../../docs/phase6/01-r1-ingest-ledger.md) / §3.1–3.6：来源、去重/周期、台账、权限、非功能与保留期; [business-lifecycle-contracts.md](../../docs/design/business-lifecycle-contracts.md) / §8：专业来源、周期、处置与N:1整改单; [project-evolution-strategy.md](../../docs/design/project-evolution-strategy.md) / §1–8：保留业务效果、薄实现、先决策与后交付分开
- 承接与窗口：D08 (本波开工前; P6.2)
- 支持/决策关联：E09；不是本源新增独立交付，不重复计价。
- 出口与未决：到期边界/失败重试/归档可读/无权不出；生产启用前核配置、备份和保留要求。；真实提供方/参数/配置未定时仍按窗口复核，不称取消或完成。

**P6-T012 · R1 批量导入 — 改写**

- 原文：[docs/phase6/07-backlog-and-triggers.md](../phase6/07-backlog-and-triggers.md) / ### 3.2 phase6 域类 / R1 批量导入; 存量人工台账迁移
- 理由：有存量迁移才需要导入工具，但“首次诉求”会无限等待；风险Excel与标准feed有不同交付。
- 当前条件：E10在P6.1明确模板/量级/来源命名空间与重复策略；可由私有一次性工具转feed，模板和行报告仍需交或明确未选原因。
- 现存证据：[business-lifecycle-contracts.md](../../docs/design/business-lifecycle-contracts.md) / §13 E10：风险Excel/工单业务导入/跨指纹merge分别裁决; [business-lifecycle-contracts.md](../../docs/design/business-lifecycle-contracts.md) / §8：专业来源、周期、处置与N:1整改单
- 承接与窗口：E10 (P6.1出口；不等待后续实施依赖完成才调查/决策; P6.E); D08 (本波开工前; P6.2); X02 (本波开工前; P6.1)
- 出口与未决：逐行错误/重复/完整快照/部分成功/断点重放；获选有限增量P6.E独立估算，不以API替代Excel效果。；真实提供方/参数/配置未定时仍按窗口复核，不称取消或完成。

**P6-T013 · 评分模型/级别映射规则 — 专业替代**

- 原文：[docs/phase6/07-backlog-and-triggers.md](../phase6/07-backlog-and-triggers.md) / ### 3.2 phase6 域类 / 评分模型/级别映射规则; R2 后续档
- 理由：评分和映射被旧文档混称模型，容易自建专业风险分析；保留本地四档及人工处置，外部评分保来源。
- 当前条件：D09消费专业分值/适用性，配置确定性源级别映射；没有可靠分值记未提供，不编制自有CVSS研判。
- 现存证据：[business-lifecycle-contracts.md](../../docs/design/business-lifecycle-contracts.md) / §8：专业来源、周期、处置与N:1整改单; [02-r2-assessment-treatment.md](../../docs/phase6/02-r2-assessment-treatment.md) / 处置四策略、工作日候选、接受确认与误报抑制; [platform-evolution.md](../../docs/design/platform-evolution.md) / §1/2/4/5/7：专业写主、可靠交接和按窗口冻结
- 承接与窗口：D09 (本波开工前; P6.2)
- 支持/决策关联：D08；不是本源新增独立交付，不重复计价。
- 出口与未决：源五级到平台四档阈值/缺值/未知枚举/版本回放与人工改级审计；不让源重推覆盖本地处置。；真实提供方/参数/配置未定时仍按窗口复核，不称取消或完成。

**P6-T014 · deadline 工作日口径 — 改写**

- 原文：[docs/phase6/07-backlog-and-triggers.md](../phase6/07-backlog-and-triggers.md) / ### 3.2 phase6 域类 / deadline 工作日口径; 日历数据源到位
- 理由：日历数据是业务日计算前提，不是完整SLA引擎前提；默认关闭不足以说明开启后的存量行为。
- 当前条件：与P6-T002合并决策，P6.3核权威日历/工作周/时区/版本与不可用行为，P8保留场景有限交付。
- 现存证据：[project-evolution-strategy.md](../../docs/design/project-evolution-strategy.md) / §1–8：保留业务效果、薄实现、先决策与后交付分开; [02-r2-assessment-treatment.md](../../docs/phase6/02-r2-assessment-treatment.md) / 处置四策略、工作日候选、接受确认与误报抑制
- 承接与窗口：E03 (P6.3出口; P8)
- 共用依据：P6-T002；本源原意和条件仍单独保留。
- 出口与未决：节假日/补班/无日历/变更与旧在途期限正反例；不开启时明确自然日，不悄悄重算旧单。；真实提供方/参数/配置未定时仍按窗口复核，不称取消或完成。

**P6-T015 · 接受审批升级（S3b） — 改写**

- 原文：[docs/phase6/07-backlog-and-triggers.md](../phase6/07-backlog-and-triggers.md) / ### 3.2 phase6 域类 / 接受审批升级（S3b）; S3b 落地
- 理由：用“S3b落地”当接受审批触发是循环依赖；接受确认业务已存在，不应等通用引擎才可操作。
- 当前条件：沿当前risk:manage确认与禁自确认，P6.3证明多级/会签业务缺口后再选择外部或最小本地环节。
- 现存证据：[business-lifecycle-contracts.md](../../docs/design/business-lifecycle-contracts.md) / §8：专业来源、周期、处置与N:1整改单; [02-r2-assessment-treatment.md](../../docs/phase6/02-r2-assessment-treatment.md) / 处置四策略、工作日候选、接受确认与误报抑制; [project-evolution-strategy.md](../../docs/design/project-evolution-strategy.md) / §1–8：保留业务效果、薄实现、先决策与后交付分开
- 承接与窗口：E03 (P6.3出口; P8)
- 共用依据：P6-T001；本源原意和条件仍单独保留。
- 出口与未决：撤权/owner交接/续期与到期竞争、确认结果持久可核；没有高级场景保留现制并非取消接受。；真实提供方/参数/配置未定时仍按窗口复核，不称取消或完成。

**P6-T016 · 看板页（自研 echarts） — 保留**

- 原文：[docs/phase6/07-backlog-and-triggers.md](../phase6/07-backlog-and-triggers.md) / ### 3.2 phase6 域类 / 看板页（自研 echarts）; 首个固定看板消费场景
- 理由：有固定受众才决定看板布局合理；echarts是已有依赖而非价值证明，统计口径必须先与台账一致。
- 当前条件：D13/P6.E先风险生命周期固定视图/受权导出，复用聚合口径；通用大屏不由风险看板暗吞。
- 现存证据：[04-r4-metrics-reports.md](../../docs/phase6/04-r4-metrics-reports.md) / §3/4/7/附录A：固定统计、快照、报告及跨模块限制; [project-evolution-strategy.md](../../docs/design/project-evolution-strategy.md) / §1–8：保留业务效果、薄实现、先决策与后交付分开
- 承接与窗口：D13 (本波开工前; P6.E)
- 出口与未决：卡片/图表/筛选明细/导出同授权与分母；没数据显示空/未知不编趋势，受众与问题可指认。；真实提供方/参数/配置未定时仍按窗口复核，不称取消或完成。

**P6-T017 · 统计 DSL — 专业替代**

- 原文：[docs/phase6/07-backlog-and-triggers.md](../phase6/07-backlog-and-triggers.md) / ### 3.2 phase6 域类 / 统计 DSL; 首个「管理层临时看数」
- 理由：临时看数需求合理，但通用统计DSL不是必需技术形式；现有白名单业务查询与报告可先满足。
- 当前条件：E04先受权白名单维度/指标/时间查询与固定报告，优先既有报告供给；不开放任意SQL或通用BI。
- 现存证据：[04-r4-metrics-reports.md](../../docs/phase6/04-r4-metrics-reports.md) / §3/4/7/附录A：固定统计、快照、报告及跨模块限制; [project-evolution-strategy.md](../../docs/design/project-evolution-strategy.md) / §1–8：保留业务效果、薄实现、先决策与后交付分开; [platform-evolution.md](../../docs/design/platform-evolution.md) / §1/2/4/5/7：专业写主、可靠交接和按窗口冻结
- 承接与窗口：E04 (P6.E出口; P8)
- 出口与未决：至少一个临时问数用例可完成、越权维度拒绝、时间边界独测；不以退出DSL形式取消运营查询效果。；真实提供方/参数/配置未定时仍按窗口复核，不称取消或完成。

**P6-T018 · 版式报告 — 改写**

- 原文：[docs/phase6/07-backlog-and-triggers.md](../phase6/07-backlog-and-triggers.md) / ### 3.2 phase6 域类 / 版式报告; 首份周报/月报成文
- 理由：首份周报是可理解场景，但不能靠等用户再次提出才排期；现有计划已保留报告完整效果。
- 当前条件：P6.E冻结首报告/受众/供给，E04/P8交白名单指标→固定Word/PDF模板→TR定时/终态→受权分发回执。
- 现存证据：[04-r4-metrics-reports.md](../../docs/phase6/04-r4-metrics-reports.md) / §3/4/7/附录A：固定统计、快照、报告及跨模块限制; [project-evolution-strategy.md](../../docs/design/project-evolution-strategy.md) / §1–8：保留业务效果、薄实现、先决策与后交付分开; [schedule-recovery-policy.md](../../docs/design/schedule-recovery-policy.md) / §2–8：时隙/策略/episode/预算/预览/停启与恢复
- 承接与窗口：E04 (P6.E出口; P8)
- 出口与未决：模板/数据/时区/版本/来源可追溯；产出成功分发失败不重跑产物，原权限变化可阻分发。；真实提供方/参数/配置未定时仍按窗口复核，不称取消或完成。

**P6-T019 · 高级搜索（custom_data 筛选） — 改写**

- 原文：[docs/phase6/07-backlog-and-triggers.md](../phase6/07-backlog-and-triggers.md) / ### 3.2 phase6 域类 / 高级搜索（custom_data 筛选）; 台账自定义字段筛选诉求
- 理由：自定义字段检索有价值，但“任意custom_data筛选”易暴露不受控SQL/字段；前端包不能独自承诺后端。
- 当前条件：U03入口选字段白名单/类型操作符、AND/OR最小档与后端过滤合同；JSONB或值表由实际样本/索引预算决定。
- 现存证据：[frontend-implementation-contract.md](../../../zhuzhao-ui/docs/design/frontend-implementation-contract.md) / 组件合同、六类工作区、全站映射、已确认UX01/02/03、跨阶段旅程; [business-lifecycle-contracts.md](../../docs/design/business-lifecycle-contracts.md) / §6：schema求值、约束与Query/prefill; [business-lifecycle-contracts.md](../../docs/design/business-lifecycle-contracts.md) / §5：类型查看/创建/参与管理和风险组职责
- 承接与窗口：U03 (本波开工前; P6.3)
- 支持/决策关联：D04,C04,G04；不是本源新增独立交付，不重复计价。
- 出口与未决：不可见字段/票不可检索、空/缺键/数组/数值语义明确；复杂筛选和导出同模型并有查询预算。；真实提供方/参数/配置未定时仍按窗口复核，不称取消或完成。

**P6-T020 · 规则化自动建单 — 改写**

- 原文：[docs/phase6/07-backlog-and-triggers.md](../phase6/07-backlog-and-triggers.md) / ### 3.2 phase6 域类 / 规则化自动建单; S3a 动作库演进 × D20 对表
- 理由：规则化建单已是风险闭环消费，不再只等S3a动作库演进；自动资格、可靠创建与用户创建权需分离。
- 当前条件：D08/D10先可信来源与受权人工派单，D16/D17按明确发布规则开放自动动作，复用R01及固定目标类型组织。
- 现存证据：[business-lifecycle-contracts.md](../../docs/design/business-lifecycle-contracts.md) / §8：专业来源、周期、处置与N:1整改单; [cross-repo-execution-contract.md](../../docs/design/cross-repo-execution-contract.md) / C03/R01：提交、效果收据/意图、reassign三类成功含义; [business-lifecycle-contracts.md](../../docs/design/business-lifecycle-contracts.md) / §5：类型查看/创建/参与管理和风险组职责
- 承接与窗口：D08 (本波开工前; P6.2); D10 (本波开工前; P6.2); D16 (本波开工前; P6.3); R01 (本波开工前; P6.1)
- 支持/决策关联：D17；不是本源新增独立交付，不重复计价。
- 出口与未决：重复发现不双票、无owner待处理、目标失效/撤权/unknown有收据；不把任意来源直接当建单授权。；真实提供方/参数/配置未定时仍按窗口复核，不称取消或完成。

**P6-T021 · 威胁情报→资产自动建单 — 专业替代**

- 原文：[docs/phase6/07-backlog-and-triggers.md](../phase6/07-backlog-and-triggers.md) / ### 3.2 phase6 域类 / 威胁情报→资产自动建单; 资产平台就绪（所有者明示先不做）
- 理由：TI→资产适用性关联属于专业结论，且旧原文明确先不做；不能因资产可查就默认自动建票。
- 当前条件：保留已定不做首期自动联动；I04/P7核专业TI/SOC/CMDB能否给适用性和来源，再P6.E/P7入口给受控建议/人工确认方案。
- 现存证据：[platform-evolution.md](../../docs/design/platform-evolution.md) / §1/2/4/5/7：专业写主、可靠交接和按窗口冻结; [business-lifecycle-contracts.md](../../docs/design/business-lifecycle-contracts.md) / §8：专业来源、周期、处置与N:1整改单; [project-evolution-strategy.md](../../docs/design/project-evolution-strategy.md) / §1–8：保留业务效果、薄实现、先决策与后交付分开
- 承接与窗口：I04 (本波开工前; P7)
- 支持/决策关联：D07,D17；不是本源新增独立交付，不重复计价。
- 出口与未决：无适用性不派单、不自建漏洞关联；保留后续效果候选，改为自动副作用须独立审批/幂等/撤权合同。；真实提供方/参数/配置未定时仍按窗口复核，不称取消或完成。

**P6-T022 · 资产解析步（D29） — 改写**

- 原文：[docs/phase6/07-backlog-and-triggers.md](../phase6/07-backlog-and-triggers.md) / ### 3.2 phase6 域类 / 资产解析步（D29）; phase7 前置批
- 理由：资产解析不必整包等P7；R3可以null降级，但薄CMDB引用/来源新鲜度应在公共Query合同中先清楚。
- 当前条件：D07/P6.2交稳定asset_ref/来源/未解析状态和有界Query；真实CMDB能力在E01/X02逐源验，完整资产历史投影另按D13。
- 现存证据：[business-lifecycle-contracts.md](../../docs/design/business-lifecycle-contracts.md) / §6：schema求值、约束与Query/prefill; [business-lifecycle-contracts.md](../../docs/design/business-lifecycle-contracts.md) / §8：专业来源、周期、处置与N:1整改单; [platform-evolution.md](../../docs/design/platform-evolution.md) / §1/2/4/5/7：专业写主、可靠交接和按窗口冻结
- 承接与窗口：D07 (本波开工前; P6.2)
- 支持/决策关联：X02,D13；不是本源新增独立交付，不重复计价。
- 出口与未决：未解析/陈旧/同名/无权限不猜ID，人工值与权威引用分开；null不阻核心风险但不伪解析成功。；真实提供方/参数/配置未定时仍按窗口复核，不称取消或完成。

**P6-T023 · 风险域权限矩阵（端点×L1 码×L2 锚[主锚/副锚/owner]×L3 字段取值） — 保留**

- 原文：[docs/phase6/07-backlog-and-triggers.md](../phase6/07-backlog-and-triggers.md) / ### 3.2 phase6 域类 / 风险域权限矩阵（端点×L1 码×L2 锚[主锚/副锚/owner]×L3 字段取值）; ~~R1 立项批拍板~~ **✅ 已拍（2026-11-01，0575719）**：两码维持 / 评论归 read / 确认制衡开（≥2 名管理角色） / 副锚裁剪
- 理由：此处已拍是设计确认，不是等待触发；端点×两码×主副锚/owner和业务制衡是风险上线前必要输入。
- 当前条件：D03与D08/D09/D10冻结同一矩阵；将虚拟组org_roles、单条/批量派单和类型策略联验。
- 现存证据：[business-lifecycle-contracts.md](../../docs/design/business-lifecycle-contracts.md) / §5：类型查看/创建/参与管理和风险组职责; [business-lifecycle-contracts.md](../../docs/design/business-lifecycle-contracts.md) / §8：专业来源、周期、处置与N:1整改单; [01-r1-ingest-ledger.md](../../docs/phase6/01-r1-ingest-ledger.md) / §3.1–3.6：来源、去重/周期、台账、权限、非功能与保留期
- 承接与窗口：D03 (本波开工前; P6.1); D08 (本波开工前; P6.2); D09 (本波开工前; P6.2); D10 (本波开工前; P6.2)
- 出口与未决：主/副锚、owner、只读/管理、禁自确认、范围裁剪与撤权反例；不能只验菜单可见。；真实提供方/参数/配置未定时仍按窗口复核，不称取消或完成。

**P6-T024 · 非功能量级假设（风险单/年·事件流水/日·聚合并发·保留期默认值） — 改写**

- 原文：[docs/phase6/07-backlog-and-triggers.md](../phase6/07-backlog-and-triggers.md) / ### 3.2 phase6 域类 / 非功能量级假设（风险单/年·事件流水/日·聚合并发·保留期默认值）; ~~R1 立项批拍板~~ **✅ 已拍参考值（2026-11-01，0575719）**：5,000 单/年·2,000 条/日·20 QPS·系数 8·并发 10·归档 365 天·流水 180 天对齐审计——容量假设初值，观测面实测回调
- 理由：参考量级已拍可保留作容量输入，不能当当前生产负载或性能通过值；等待“量级起来”会遗漏最初索引/恢复预算。
- 当前条件：G04/P6.1用5k/年、2k流水/日、20QPS等带来源初值造合成数据，D08/D13落索引和查询预算后再回调。
- 现存证据：[01-r1-ingest-ledger.md](../../docs/phase6/01-r1-ingest-ledger.md) / §3.1–3.6：来源、去重/周期、台账、权限、非功能与保留期; [engineering-execution-design.md](../../docs/design/engineering-execution-design.md) / G03/G04/G06/C01–C05：当前差距、失败反例与固定出口; [04-r4-metrics-reports.md](../../docs/phase6/04-r4-metrics-reports.md) / §3/4/7/附录A：固定统计、快照、报告及跨模块限制
- 承接与窗口：G04 (本波开工前; P6.1)
- 支持/决策关联：D08,D13；不是本源新增独立交付，不重复计价。
- 出口与未决：基准标数据分布/并发/硬件，锁/导出/聚合与保留期分测；未实测明确未知。；真实提供方/参数/配置未定时仍按窗口复核，不称取消或完成。

**P6-T025 · 合规映射（D26）/数据分类（D27） — 专业替代**

- 原文：[docs/phase6/07-backlog-and-triggers.md](../phase6/07-backlog-and-triggers.md) / ### 3.2 phase6 域类 / 合规映射（D26）/数据分类（D27）; 触发 / X5 契约
- 理由：合规框架与分类权威已应来自专业GRC/资产供给，本地无需再造分类评级；预留列不是完成分类。
- 当前条件：E06在P6.E核代码表/来源/权限/版本；平台留引用和处置证据，缺供给显示未分类并可人工薄协作。
- 现存证据：[platform-evolution.md](../../docs/design/platform-evolution.md) / §1/2/4/5/7：专业写主、可靠交接和按窗口冻结; [project-evolution-strategy.md](../../docs/design/project-evolution-strategy.md) / §1–8：保留业务效果、薄实现、先决策与后交付分开; [04-r4-metrics-reports.md](../../docs/phase6/04-r4-metrics-reports.md) / §3/4/7/附录A：固定统计、快照、报告及跨模块限制
- 承接与窗口：E06 (P6.E出口; P8)
- 出口与未决：未知分类不扩大可见性，映射版本与历史来源可追；真实供给未定不等于取消合规证据效果。；真实提供方/参数/配置未定时仍按窗口复核，不称取消或完成。

**P6-T026 · 工单批量导入 — 改写**

- 原文：[docs/phase6/07-backlog-and-triggers.md](../phase6/07-backlog-and-triggers.md) / ### 3.2 phase6 域类 / 工单批量导入; 触发
- 理由：工单业务批量导入与配置包、批量标签不同；泛“触发”没有样本/权限/回滚依据。
- 当前条件：E10/P6.1取固定类型发布版、允许字段与creator/owner/附件白名单，逐行调用工单用例；P6.E获选有限CLI或页。
- 现存证据：[business-lifecycle-contracts.md](../../docs/design/business-lifecycle-contracts.md) / §13 E10：风险Excel/工单业务导入/跨指纹merge分别裁决; [business-lifecycle-contracts.md](../../docs/design/business-lifecycle-contracts.md) / §3：发布/实例钉版/预检/同名最小迁版; [business-lifecycle-contracts.md](../../docs/design/business-lifecycle-contracts.md) / §5：类型查看/创建/参与管理和风险组职责
- 承接与窗口：E10 (P6.1出口；不等待后续实施依赖完成才调查/决策; P6.E); D08 (本波开工前; P6.2); U03 (本波开工前; P6.3); X02 (本波开工前; P6.1)
- 出口与未决：非法身份/旧schema/部分成功/重复行/响应丢失重放；必须有每行receipt，不直接写表。；真实提供方/参数/配置未定时仍按窗口复核，不称取消或完成。

**P6-T027 · 每日敞口快照表 — 保留**

- 原文：[docs/phase6/07-backlog-and-triggers.md](../phase6/07-backlog-and-triggers.md) / ### 3.2 phase6 域类 / **每日敞口快照表**; 非触发——建议随 R4 第一档（不做则历史趋势永久丢失；性质同审计归档）
- 理由：每日快照保存会被更新覆盖的历史事实，不能以“等趋势需求出现”延迟采集；但需先定授权维度与保留。
- 当前条件：D13/P6.E先固定可解释矩阵/日界/版本/受权读取，复用维护调度；后加维度不重造虚假历史。
- 现存证据：[04-r4-metrics-reports.md](../../docs/phase6/04-r4-metrics-reports.md) / §3/4/7/附录A：固定统计、快照、报告及跨模块限制; [project-evolution-strategy.md](../../docs/design/project-evolution-strategy.md) / §1–8：保留业务效果、薄实现、先决策与后交付分开
- 承接与窗口：D13 (本波开工前; P6.E)
- 出口与未决：重复EOD幂等、漏日标缺口、迟到修正可追、跨组织快照不泄露；指标可与原台账口径核。；真实提供方/参数/配置未定时仍按窗口复核，不称取消或完成。

**P6-T028 · IOC 库与封禁清单分发（轻量 TIP——只做权威清单源不自建执行器，D4 同哲学） — 专业替代**

- 原文：[docs/phase6/07-backlog-and-triggers.md](../phase6/07-backlog-and-triggers.md) / ### 3.2 phase6 域类 / **IOC 库与封禁清单分发**（轻量 TIP——只做权威清单源不自建执行器，D4 同哲学）; R5 立项批评估实体 / phase8 连接器（分发）
- 理由：旧“轻量TIP权威库”与已有专业TIP/SOC写主冲突；保留受控IOC消费/分发效果，不复制第二权威。
- 当前条件：E06取带来源/置信度/有效期的清单引用，经允许工具/网关分发；本地保存发布/接收回执，不自建情报运营。
- 现存证据：[platform-evolution.md](../../docs/design/platform-evolution.md) / §1/2/4/5/7：专业写主、可靠交接和按窗口冻结; [project-evolution-strategy.md](../../docs/design/project-evolution-strategy.md) / §1–8：保留业务效果、薄实现、先决策与后交付分开; [ai-capabilities.md](../../docs/design/ai-capabilities.md) / §1–5：已有模型API、双轨Agent/受权工具/知识/skill边界
- 承接与窗口：E06 (P6.E出口; P8)
- 支持/决策关联：I02；不是本源新增独立交付，不重复计价。
- 出口与未决：过期/撤回/无权IOC不下发；清单版本/目标/回执明确，未知目标能力不写成功。；真实提供方/参数/配置未定时仍按窗口复核，不称取消或完成。

**P6-T029 · 对内通告发布中心（F19 是「收」本件是「发」：通告实体+触达统计+S6 多通道） — 专业替代**

- 原文：[docs/phase6/07-backlog-and-triggers.md](../phase6/07-backlog-and-triggers.md) / ### 3.2 phase6 域类 / **对内通告发布中心**（F19 是「收」本件是「发」：通告实体+触达统计+S6 多通道）; 首次对内广播需求
- 理由：对内发布有业务价值，但不必新建完整通告中心；现有消息/工单与企业发布供给可承载薄链。
- 当前条件：E06/P8先收文/分责/回复，内发用S6或既有发布平台，保留对象、受众、确认/触达口径。
- 现存证据：[project-evolution-strategy.md](../../docs/design/project-evolution-strategy.md) / §1–8：保留业务效果、薄实现、先决策与后交付分开; [business-lifecycle-contracts.md](../../docs/design/business-lifecycle-contracts.md) / §9：事件/通知最小合同及B08阈值配置; [platform-evolution.md](../../docs/design/platform-evolution.md) / §1/2/4/5/7：专业写主、可靠交接和按窗口冻结
- 承接与窗口：E06 (P6.E出口; P8)
- 支持/决策关联：D11；不是本源新增独立交付，不重复计价。
- 出口与未决：撤权/错误受众/重复发布/分发部分失败可追；触达不冒充阅读，保留需自建的最小缺口。；真实提供方/参数/配置未定时仍按窗口复核，不称取消或完成。

**P6-T030 · 桌面演练形态（R5 和平态姊妹——Campaign 加「演练/实战」类型标记，不单列模块） — 专业替代**

- 原文：[docs/phase6/07-backlog-and-triggers.md](../phase6/07-backlog-and-triggers.md) / ### 3.2 phase6 域类 / **桌面演练形态**（R5 和平态姊妹——Campaign 加「演练/实战」类型标记，不单列模块）; R5 立项批设计内一并
- 理由：演练是协作场景而非SOC检测或独立平台；把“R5立项时再看”改为固定供给/价值裁决。
- 当前条件：E06/P6.E核场景来源、演练与实战标识，P8先引用/岗位响应/证据/复盘；复杂评分运营由已有专业供给。
- 现存证据：[project-evolution-strategy.md](../../docs/design/project-evolution-strategy.md) / §1–8：保留业务效果、薄实现、先决策与后交付分开; [platform-evolution.md](../../docs/design/platform-evolution.md) / §1/2/4/5/7：专业写主、可靠交接和按窗口冻结; [04-r4-metrics-reports.md](../../docs/phase6/04-r4-metrics-reports.md) / §3/4/7/附录A：固定统计、快照、报告及跨模块限制
- 承接与窗口：E06 (P6.E出口; P8)
- 支持/决策关联：D14；不是本源新增独立交付，不重复计价。
- 出口与未决：演练通知/动作与实战隔离，合成标记不污染真实指标；保留有限本地协作，未知供给不自动取消。；真实提供方/参数/配置未定时仍按窗口复核，不称取消或完成。

**P6-T031 · 安全度量台（指标字典+目标值+趋势+管理层订阅推送） — 改写**

- 原文：[docs/phase6/07-backlog-and-triggers.md](../phase6/07-backlog-and-triggers.md) / ### 3.2 phase6 域类 / **安全度量台**（指标字典+目标值+趋势+管理层订阅推送）; 首个管理层周期汇报需求
- 理由：指标字典/目标/趋势/管理订阅是多个增量，不能因“周期汇报”一触发就建完整安全度量台。
- 当前条件：D13先本地可证指标；E04/P6.E选受众与首报告，P8在首报告后渐进补字典/目标/趋势/订阅。
- 现存证据：[project-evolution-strategy.md](../../docs/design/project-evolution-strategy.md) / §1–8：保留业务效果、薄实现、先决策与后交付分开; [04-r4-metrics-reports.md](../../docs/phase6/04-r4-metrics-reports.md) / §3/4/7/附录A：固定统计、快照、报告及跨模块限制
- 承接与窗口：D13 (本波开工前; P6.E)
- 支持/决策关联：E04；不是本源新增独立交付，不重复计价。
- 出口与未决：每指标定义分母/时间/可见范围/缺数；无目标不报达标，专业MTTD无来源起点不算。；真实提供方/参数/配置未定时仍按窗口复核，不称取消或完成。

**P6-T032 · 渗透组发现接入（PT 由专项组负责——平台不做 PT 工作台，只接收其发现走风险事件单[2026-11-03 所有者拍板]） — 专业替代**

- 原文：[docs/phase6/07-backlog-and-triggers.md](../phase6/07-backlog-and-triggers.md) / ### 3.2 phase6 域类 / **渗透组发现接入**（PT 由专项组负责——平台不做 PT 工作台，只接收其发现走风险事件单[2026-11-03 所有者拍板]）; 渗透组交付格式契约到位
- 理由：PT发现由专项组产生，平台不做渗透工作台的既定边界正确；真实格式契约仅阻该源real。
- 当前条件：I04/X02以规范发现信封接入PT交付，可从受审文件/私有connector起，保留来源和证据索引。
- 现存证据：[business-lifecycle-contracts.md](../../docs/design/business-lifecycle-contracts.md) / §8：专业来源、周期、处置与N:1整改单; [platform-evolution.md](../../docs/design/platform-evolution.md) / §1/2/4/5/7：专业写主、可靠交接和按窗口冻结; [project-evolution-strategy.md](../../docs/design/project-evolution-strategy.md) / §1–8：保留业务效果、薄实现、先决策与后交付分开
- 承接与窗口：I04 (本波开工前; P7)
- 支持/决策关联：X02；不是本源新增独立交付，不重复计价。
- 出口与未决：稳定来源ID/重复/归属/敏感证据授权、部分导入/回放；不把接收格式等同重做PT扫描。；真实提供方/参数/配置未定时仍按窗口复核，不称取消或完成。

**P6-T033 · 实体时间线视图 — 改写**

- 原文：[docs/phase6/07-backlog-and-triggers.md](../phase6/07-backlog-and-triggers.md) / ### 3.2 phase6 域类 / **实体时间线视图**; 首个联动分析场景
- 理由：按risk_id合流有用，但风险实体时间线不能被D14的SOC事件视图暗中替代；两者共享呈现，不混写主。
- 当前条件：D13保留风险/票/任务/通知关联投影，D14再加SOC事件引用；统一correlation、源时间和本地时间。
- 现存证据：[business-lifecycle-contracts.md](../../docs/design/business-lifecycle-contracts.md) / §8：专业来源、周期、处置与N:1整改单; [cross-repo-execution-contract.md](../../docs/design/cross-repo-execution-contract.md) / C03/R01：提交、效果收据/意图、reassign三类成功含义; [04-r4-metrics-reports.md](../../docs/phase6/04-r4-metrics-reports.md) / §3/4/7/附录A：固定统计、快照、报告及跨模块限制
- 承接与窗口：D13 (本波开工前; P6.E); D14 (本波开工前; P6.E)
- 出口与未决：无权子对象不泄露、顺序/重复/缺失/时区明确；外部未接动作展示覆盖缺口，不编完整时间线。；真实提供方/参数/配置未定时仍按窗口复核，不称取消或完成。

**P6-T034 · 处置链路耗时分解 — 保留**

- 原文：[docs/phase6/07-backlog-and-triggers.md](../phase6/07-backlog-and-triggers.md) / ### 3.2 phase6 域类 / **处置链路耗时分解**; 管理层问「一条高危 30 天时间花在哪了」
- 理由：“30天花在哪”是有效管理问题，阶段耗时属于本地生命周期事实，适合固定聚合，不需AI/统计DSL。
- 当前条件：D13以领域入态事件分阶段，明确等待评估/派单/处理/复核及并行/重开口径。
- 现存证据：[04-r4-metrics-reports.md](../../docs/phase6/04-r4-metrics-reports.md) / §3/4/7/附录A：固定统计、快照、报告及跨模块限制; [business-lifecycle-contracts.md](../../docs/design/business-lifecycle-contracts.md) / §8：专业来源、周期、处置与N:1整改单; [cross-repo-execution-contract.md](../../docs/design/cross-repo-execution-contract.md) / C03/R01：提交、效果收据/意图、reassign三类成功含义
- 承接与窗口：D13 (本波开工前; P6.E)
- 出口与未决：缺事件记未知、重开区间分离、跨时区和未结事项口径明确，汇总可追到受权明细。；真实提供方/参数/配置未定时仍按窗口复核，不称取消或完成。

**P6-T035 · 通知→响应转化率 — 改写**

- 原文：[docs/phase6/07-backlog-and-triggers.md](../phase6/07-backlog-and-triggers.md) / ### 3.2 phase6 域类 / **通知→响应转化率**; 降噪/节流阈值需要有数据校准
- 理由：通知→响应只能证明本地行为关联，不能直接等同通知因果效果或以24h硬指标考核所有类型。
- 当前条件：D13先定义消息类型、送达/阅读可得性、响应动作和窗口；专业消息源无receipt时标覆盖率。
- 现存证据：[04-r4-metrics-reports.md](../../docs/phase6/04-r4-metrics-reports.md) / §3/4/7/附录A：固定统计、快照、报告及跨模块限制; [business-lifecycle-contracts.md](../../docs/design/business-lifecycle-contracts.md) / §9：事件/通知最小合同及B08阈值配置
- 承接与窗口：D13 (本波开工前; P6.E)
- 支持/决策关联：D11；不是本源新增独立交付，不重复计价。
- 出口与未决：多次通知去重、actor自回执/无响应/迟到/节流/optout分母分开；不把没发送当无响应。；真实提供方/参数/配置未定时仍按窗口复核，不称取消或完成。

**P6-T036 · 超期根因分布 — 改写**

- 原文：[docs/phase6/07-backlog-and-triggers.md](../phase6/07-backlog-and-triggers.md) / ### 3.2 phase6 域类 / **超期根因分布**; 管理动作需要靶点（人/系统/级别/阶段）
- 理由：按人/系统/级别/阶段可做超期分布，但事件数据通常不足以自动证明根因；不能给人贴因果标签。
- 当前条件：D13先叫“超期分布/待核原因”，原因需人工/受控选项与证据；真实根因分析取专业结论。
- 现存证据：[04-r4-metrics-reports.md](../../docs/phase6/04-r4-metrics-reports.md) / §3/4/7/附录A：固定统计、快照、报告及跨模块限制; [business-lifecycle-contracts.md](../../docs/design/business-lifecycle-contracts.md) / §8：专业来源、周期、处置与N:1整改单
- 承接与窗口：D13 (本波开工前; P6.E)
- 出口与未决：截止口径、在途/已结分母、无人负责/缺源标未知，授权聚合不泄漏个人域。；真实提供方/参数/配置未定时仍按窗口复核，不称取消或完成。

**P6-T037 · 回归 × 修复方式交叉 — 保留**

- 原文：[docs/phase6/07-backlog-and-triggers.md](../phase6/07-backlog-and-triggers.md) / ### 3.2 phase6 域类 / **回归 × 修复方式交叉**; 回归数据积累后（回归标记已拍）
- 理由：数据积累决定图表是否有意义，不应决定是否保存已拍回归/处置事实；空样本可先给诚实空态。
- 当前条件：D08先周期/回归与处置方式事实，D13固定回归×修复方式交叉且注明样本窗。
- 现存证据：[business-lifecycle-contracts.md](../../docs/design/business-lifecycle-contracts.md) / §8：专业来源、周期、处置与N:1整改单; [04-r4-metrics-reports.md](../../docs/phase6/04-r4-metrics-reports.md) / §3/4/7/附录A：固定统计、快照、报告及跨模块限制
- 承接与窗口：D13 (本波开工前; P6.E)
- 支持/决策关联：D08；不是本源新增独立交付，不重复计价。
- 出口与未决：真回归新周期与accepted到期重审分开，零样本不推结论，历史映射版本可追。；真实提供方/参数/配置未定时仍按窗口复核，不称取消或完成。

**P6-T038 · 误报 × 扫描器质量 — 改写**

- 原文：[docs/phase6/07-backlog-and-triggers.md](../phase6/07-backlog-and-triggers.md) / ### 3.2 phase6 域类 / **误报 × 扫描器质量**; 抑制名单数据积累
- 理由：误报结论的本地计数可以保留，但“扫描器质量排名”不是平台可单独证明的专业评分。
- 当前条件：D13只聚合已确认误报/来源/样本覆盖，专业评分取源；无统一检测范围不得横比成供应商优劣。
- 现存证据：[business-lifecycle-contracts.md](../../docs/design/business-lifecycle-contracts.md) / §8：专业来源、周期、处置与N:1整改单; [04-r4-metrics-reports.md](../../docs/phase6/04-r4-metrics-reports.md) / §3/4/7/附录A：固定统计、快照、报告及跨模块限制; [platform-evolution.md](../../docs/design/platform-evolution.md) / §1/2/4/5/7：专业写主、可靠交接和按窗口冻结
- 承接与窗口：D13 (本波开工前; P6.E)
- 支持/决策关联：D09；不是本源新增独立交付，不重复计价。
- 出口与未决：同事件重推/抑制样本/重复源去重，分母与标注过程可追；人工误报不能被新扫描结果静默覆盖。；真实提供方/参数/配置未定时仍按窗口复核，不称取消或完成。

**P6-T039 · 跨资产扩散 → 补丁式批量处置 — 专业替代**

- 原文：[docs/phase6/07-backlog-and-triggers.md](../phase6/07-backlog-and-triggers.md) / ### 3.2 phase6 域类 / **跨资产扩散 → 补丁式批量处置**; 同源漏洞跨 owner 扩散成常态（N:1 合并按 owner 分组的进阶）
- 理由：跨资产适用性/扩散检测优先专业平台，本地应保留同源候选分组和按owner批准派单，而不是自建分析/补丁执行。
- 当前条件：I04取专业结论，D10按owner/类型/组织分组，U03仅受控批量确认，写副作用经I02/当前授权。
- 现存证据：[business-lifecycle-contracts.md](../../docs/design/business-lifecycle-contracts.md) / §8：专业来源、周期、处置与N:1整改单; [platform-evolution.md](../../docs/design/platform-evolution.md) / §1/2/4/5/7：专业写主、可靠交接和按窗口冻结; [project-evolution-strategy.md](../../docs/design/project-evolution-strategy.md) / §1–8：保留业务效果、薄实现、先决策与后交付分开
- 承接与窗口：I04 (本波开工前; P7); D08 (本波开工前; P6.2); D09 (本波开工前; P6.2); D10 (本波开工前; P6.2); U03 (本波开工前; P6.3); I02 (本波开工前; P7)
- 出口与未决：不同owner不误合、每票≤20风险、逐行权限/部分失败/重复回执、目标失效；不凭同CVE自动认定可修复。；真实提供方/参数/配置未定时仍按窗口复核，不称取消或完成。

**P6-T040 · 升级链有效性对比 — 改写**

- 原文：[docs/phase6/07-backlog-and-triggers.md](../phase6/07-backlog-and-triggers.md) / ### 3.2 phase6 域类 / **升级链有效性对比**; 升级单 vs 未升级单响应时间对比
- 理由：比较升级与未升级响应时间有选择偏差，不应承诺因果有效性；现有reminder记录可做描述性分组。
- 当前条件：D13先记录升级原因/时点、同窗口分层比较，明确统计不证明升级导致改进。
- 现存证据：[04-r4-metrics-reports.md](../../docs/phase6/04-r4-metrics-reports.md) / §3/4/7/附录A：固定统计、快照、报告及跨模块限制; [business-lifecycle-contracts.md](../../docs/design/business-lifecycle-contracts.md) / §9：事件/通知最小合同及B08阈值配置
- 承接与窗口：D13 (本波开工前; P6.E)
- 出口与未决：无样本/多级升级/重复催办去重、配置变更和actor范围可追；保留分析口径，不另建升级引擎。；真实提供方/参数/配置未定时仍按窗口复核，不称取消或完成。

**P6-T041 · C-6 图数上限收紧本体 — 改写**

- 原文：[docs/phase6/07-backlog-and-triggers.md](../phase6/07-backlog-and-triggers.md) / ### 3.2 phase6 域类 / C-6 图数上限收紧本体; 磁盘水位信号亮起（观测面已建：gauge+阈值 cron）
- 理由：水位触发技术上合理且观测面已有，但阈值没配置时不能等一个永不亮的信号；图数不等于存储字节。
- 当前条件：E02/P6.3固定核图数/对象大小/容量与阈值配置，P7决定维持现限额或收紧；所有者已定观测不需重拍。
- 现存证据：[s6-s5-debt-ledger.md](../../docs/phase5/s6-s5-debt-ledger.md) / B/C/G/H组及挂账质量判据：历史状态与触发原意; [app.go](../../internal/app/app.go) / NewApp：ticket.notify/ops.notify与InboxService.SetEventRegistry；storage_usage_sample/notify_volume_check; [05-s5-object-storage-richmedia.md](../../docs/phase5/05-s5-object-storage-richmedia.md) / 现有存储用途、权限、引用和GC合同
- 承接与窗口：E02 (P6.3出口; P7)
- 支持/决策关联：G05；不是本源新增独立交付，不重复计价。
- 出口与未决：当前storage_usage_sample绝对值/未配容量可解释，逼近阈值有合成告警；收紧不误删存量引用。；真实提供方/参数/配置未定时仍按窗口复核，不称取消或完成。

**P6-T042 · C-16 节流廉价预判本体 — 改写**

- 原文：[docs/phase6/07-backlog-and-triggers.md](../phase6/07-backlog-and-triggers.md) / ### 3.2 phase6 域类 / C-16 节流廉价预判本体; 通知量级异常信号（ok/dead 计数+量级 cron 已建）
- 理由：发送计数可发现“成功的骚扰”，但廉价预判是否必要应按真实热点路径测量，不能只等用户投诉或dead。
- 当前条件：D11先定义发送预算/幂等/偏好/节流顺序及WARN独立通道，固定在开工验入口成本与压测。
- 现存证据：[s6-s5-debt-ledger.md](../../docs/phase5/s6-s5-debt-ledger.md) / B/C/G/H组及挂账质量判据：历史状态与触发原意; [app.go](../../internal/app/app.go) / NewApp：ticket.notify/ops.notify与InboxService.SetEventRegistry；storage_usage_sample/notify_volume_check; [business-lifecycle-contracts.md](../../docs/design/business-lifecycle-contracts.md) / §9：事件/通知最小合同及B08阈值配置
- 承接与窗口：D11 (本波开工前; P6.3)
- 支持/决策关联：G04；不是本源新增独立交付，不重复计价。
- 出口与未决：大量成功消息也触发观测；不能预判错拦合法新版本，unknown和已发送receipt不被重置。；真实提供方/参数/配置未定时仍按窗口复核，不称取消或完成。

**P6-T043 · C-1 / C-2 后端三件 / C-3 — 改写**

- 原文：[docs/phase6/07-backlog-and-triggers.md](../phase6/07-backlog-and-triggers.md) / ### 3.2 phase6 域类 / C-1 / C-2 后端三件 / C-3; （非触发项——ID 索引补登：C-1 + C-2 后端三件（prefill 端点/params 泛化/external_http）= Week 0「R3 前置四件」行、C-3 =「BK-21 泛化」行，2026-10-28 外审核出总索引缺行；**C-2「前端交互」（编辑时预填运行时 UI/三档错误 UX）不在四件内、无承接——05 号 §1.2 M1 注，R3 立项批圈范围时计入**，2026-11-01 卫生批补）
- 理由：本行混四个后端能力和前端三档错误UX，原Week0漏UX是范围缺口；用一个D07映射不能证明全落。
- 当前条件：C04先拒绝空filter；D07做多参/超时/限流/审计/prefill，U03/对应表单同批接未配置/失败/可手填。
- 现存证据：[business-lifecycle-contracts.md](../../docs/design/business-lifecycle-contracts.md) / §6：schema求值、约束与Query/prefill; [engineering-execution-design.md](../../docs/design/engineering-execution-design.md) / C04：DBTX试点、org_roles删除双方锁、默认拒绝; [frontend-implementation-contract.md](../../../zhuzhao-ui/docs/design/frontend-implementation-contract.md) / 组件合同、六类工作区、全站映射、已确认UX01/02/03、跨阶段旅程
- 承接与窗口：D07 (本波开工前; P6.2); C04 (本波开工前; P6.1); U03 (本波开工前; P6.3)
- 出口与未决：不返回任意URL/私有secret、取消context有效、源错不变mock绿；字段失败与提交流程可恢复。；真实提供方/参数/配置未定时仍按窗口复核，不称取消或完成。

**P6-T044 · 跨指纹人工合并（P1-5） — 改写**

- 原文：[docs/phase6/07-backlog-and-triggers.md](../phase6/07-backlog-and-triggers.md) / ### 3.2 phase6 域类 / **跨指纹人工合并（P1-5）**; 首个多源同事件诉求（多 CVE/多扫描器口径）
- 理由：多指纹同事件是合理人工合并场景，但“业界标配”与首次诉求都不足以默认数据合并。
- 当前条件：E10/P6.1取不同owner/活动链接/接受结论冲突样本，P6.E只交获准的主从回链与逐项补偿。
- 现存证据：[business-lifecycle-contracts.md](../../docs/design/business-lifecycle-contracts.md) / §13 E10：风险Excel/工单业务导入/跨指纹merge分别裁决; [business-lifecycle-contracts.md](../../docs/design/business-lifecycle-contracts.md) / §8：专业来源、周期、处置与N:1整改单
- 承接与窗口：E10 (P6.1出口；不等待后续实施依赖完成才调查/决策; P6.E); D08 (本波开工前; P6.2); X02 (本波开工前; P6.1)
- 出口与未决：merged_into保留双方ID/审计，来源后续归属、统计与活动票不丢；不以N:1整改单代替merge。；真实提供方/参数/配置未定时仍按窗口复核，不称取消或完成。

**P6-T045 · 全局命令面板（Cmd+K——搜菜单/页面/工单号直达跳转） — 改写**

- 原文：[docs/phase6/07-backlog-and-triggers.md](../phase6/07-backlog-and-triggers.md) / ### 3.2 phase6 域类 / **全局命令面板（Cmd+K——搜菜单/页面/工单号直达跳转）**; 外部协作者/英文用户出现（与 i18n 翻案同触发器）或「侧栏逐级找页」走查反馈成串
- 理由：Cmd+K是查找入口体验，与英文用户无必然关系；现映射E04报告包不相称，不能随报告延期到P8。
- 当前条件：建议U02/P6.1用权限过滤菜单/工单号任务样本决定是否保留，U06/P6.3交获选最薄跳转；正式归属由root同步总账。
- 现存证据：[frontend-implementation-contract.md](../../../zhuzhao-ui/docs/design/frontend-implementation-contract.md) / 组件合同、六类工作区、全站映射、已确认UX01/02/03、跨阶段旅程; [frontend-redesign.md](../../../zhuzhao-ui/docs/design/frontend-redesign.md) / FE-PRE-01–11：实际静态问题/代表方案/全站未实测限制
- 承接与窗口：U06 (本波开工前; P6.3)
- 支持/决策关联：E04,U02；不是本源新增独立交付，不重复计价。
- 当前条件：建议U02/P6.1做命令面板价值/权限任务评审，U06/P6.3交获选最薄入口；原E04映射只保来源追溯，尚未改总账
- 出口与未决：无权菜单/工单不出搜索结果，键盘焦点/空/失败/跳转稳定；列设置/主题/相对时间/未保存逐项裁定不打包暗批。；真实提供方/参数/配置未定时仍按窗口复核，不称取消或完成。

**P6-T046 · S8通用数据大屏 — 改写**

- 原文：[docs/phase6/07-backlog-and-triggers.md](../phase6/07-backlog-and-triggers.md) / ### 3.2 phase6 域类 / **S8通用数据大屏**; 数据积累+固定受众双触发
- 理由：通用大屏与风险看板独立的边界可保留，但“双触发”须有固定受众和问题清单，不能只等数据积累。
- 当前条件：E04/P6.E冻结受众/数据源/视图，P8在固定报告之后选择一个只读大屏；没有价值明确保留当前列表而非建空屏。
- 现存证据：[04-r4-metrics-reports.md](../../docs/phase6/04-r4-metrics-reports.md) / §3/4/7/附录A：固定统计、快照、报告及跨模块限制; [project-evolution-strategy.md](../../docs/design/project-evolution-strategy.md) / §1–8：保留业务效果、薄实现、先决策与后交付分开; [frontend-implementation-contract.md](../../../zhuzhao-ui/docs/design/frontend-implementation-contract.md) / 组件合同、六类工作区、全站映射、已确认UX01/02/03、跨阶段旅程
- 承接与窗口：E04 (P6.E出口; P8)
- 出口与未决：指标/范围/时间/刷新/未知/陈旧清楚，专用只读scope、无公开全公司URL；不建BI设计器。；真实提供方/参数/配置未定时仍按窗口复核，不称取消或完成。

**P6-T047 · 风险单讨论流/评论 — 改写**

- 原文：[docs/phase6/07-backlog-and-triggers.md](../phase6/07-backlog-and-triggers.md) / ### 3.2 phase6 域类 / **风险单讨论流/评论**; 风险单侧协同讨论诉求出现（处置协同现状由整改工单承载）
- 理由：风险协作当前由整改工单承载；再加风险评论可能分裂事实，不应只因“讨论诉求”重复造聊天流。
- 当前条件：D08入口用一个未派单风险讨论场景比较事件备注/关联票/本地只读+评论；确需增量再定独立作者/编辑/权限规则。
- 现存证据：[business-lifecycle-contracts.md](../../docs/design/business-lifecycle-contracts.md) / §8：专业来源、周期、处置与N:1整改单; [business-lifecycle-contracts.md](../../docs/design/business-lifecycle-contracts.md) / §5：类型查看/创建/参与管理和风险组职责; [business-lifecycle-contracts.md](../../docs/design/business-lifecycle-contracts.md) / §9：事件/通知最小合同及B08阈值配置
- 承接与窗口：D08 (本波开工前; P6.2)
- 支持/决策关联：U03；不是本源新增独立交付，不重复计价。
- 出口与未决：风险读/管理/owner权限明确，通知不泄密；不把工程备注当评论全功能，也不删除现有协作路径。；真实提供方/参数/配置未定时仍按窗口复核，不称取消或完成。

**P6-T048 · 处置建议库（风险类型→建议模板） — 专业替代**

- 原文：[docs/phase6/07-backlog-and-triggers.md](../phase6/07-backlog-and-triggers.md) / ### 3.2 phase6 域类 / **处置建议库**（风险类型→建议模板）; 同类风险反复处置诉求出现
- 理由：按类型的处置建议可先复用已有runbook/知识服务与版本模板，不必新建自有知识库。
- 当前条件：A06/P8留来源/版本/有效期/ACL的建议引用；本地允许薄模板选择，模型建议不当专业事实。
- 现存证据：[ai-capabilities.md](../../docs/design/ai-capabilities.md) / §1–5：已有模型API、双轨Agent/受权工具/知识/skill边界; [platform-evolution.md](../../docs/design/platform-evolution.md) / §1/2/4/5/7：专业写主、可靠交接和按窗口冻结; [project-evolution-strategy.md](../../docs/design/project-evolution-strategy.md) / §1–8：保留业务效果、薄实现、先决策与后交付分开
- 承接与窗口：A06 (本波开工前; P8)
- 支持/决策关联：R05；不是本源新增独立交付，不重复计价。
- 出口与未决：撤权/旧版本/缺来源/无适用建议中性展示，禁止建议自动赋予操作权；需求不足保留引用候选并在P7比较。；真实提供方/参数/配置未定时仍按窗口复核，不称取消或完成。

**P6-T049 · CVSS 自动评分导入 — 专业替代**

- 原文：[docs/phase6/07-backlog-and-triggers.md](../phase6/07-backlog-and-triggers.md) / ### 3.2 phase6 域类 / **CVSS 自动评分导入**; 真实扫描器源给出 CVSS 分值时映射（关联既有登记：README §2 R2 ◆评分模型后续档）
- 理由：导入来源CVSS和自行计算CVSS不同；真实源提供分值只触发映射，不授权本地生成评分。
- 当前条件：D08/09收源分值、版本、向量/适用性可选和平台级别映射；无值显示未提供。
- 现存证据：[business-lifecycle-contracts.md](../../docs/design/business-lifecycle-contracts.md) / §8：专业来源、周期、处置与N:1整改单; [02-r2-assessment-treatment.md](../../docs/phase6/02-r2-assessment-treatment.md) / 处置四策略、工作日候选、接受确认与误报抑制; [platform-evolution.md](../../docs/design/platform-evolution.md) / §1/2/4/5/7：专业写主、可靠交接和按窗口冻结
- 承接与窗口：D08 (本波开工前; P6.2)
- 支持/决策关联：D09；不是本源新增独立交付，不重复计价。
- 出口与未决：非法范围/未知评分版本/迟到新旧值、人工改级/本地状态不被重推覆写。；真实提供方/参数/配置未定时仍按窗口复核，不称取消或完成。

**P6-T050 · 多源采集信封 — 退出旧独立待办**

- 原文：[docs/phase6/07-backlog-and-triggers.md](../phase6/07-backlog-and-triggers.md) / ### 3.2 phase6 域类 / 多源采集信封; 同 3.1 多源采集行
- 理由：本行自称同3.1多源采集，是同一信封的另一入口；保留两套会令身份/幂等/完整性漂移。
- 当前条件：去重指向P6-T006；RISK来源事实合同与I04私有adapter共用版本信封，不取消多源接入。
- 现存证据：[business-lifecycle-contracts.md](../../docs/design/business-lifecycle-contracts.md) / §8：专业来源、周期、处置与N:1整改单; [project-evolution-strategy.md](../../docs/design/project-evolution-strategy.md) / §1–8：保留业务效果、薄实现、先决策与后交付分开
- 承接与窗口：I04 (本波开工前; P7)
- 支持/决策关联：X02；不是本源新增独立交付，不重复计价。
- 去重：业务效果显式归入 **P6-T006**，本行仅退出重复立项。
- 出口与未决：两个来源进入同一规范合同、重复/乱序/能力协商反例一致，不能各造字段表。；真实提供方/参数/配置未定时仍按窗口复核，不称取消或完成。

**P6-T051 · P1-6 通知矩阵（时机×级别→收件人集合） — 保留**

- 原文：[docs/phase6/07-backlog-and-triggers.md](../phase6/07-backlog-and-triggers.md) / ### 3.2 phase6 域类 / **P1-6 通知矩阵（时机×级别→收件人集合）**; R2/R3 立项批（首个级别分档通知诉求）——**spec 首实例已入（2026-11-02 所有者）：高级别风险→沿组织树上溯抄送一级部门负责人（resolveOrgHead，见 01 号 §3.1）**
- 理由：高级别风险上溯一级部门负责人已是明确需求，不再等待首次分档通知；尚待的是阈值/组织锚点/频次配置。
- 当前条件：D15在首条风险通知启用前冻结B08实际映射与缺负责人策略，复用当前owner与授权收件人解析。
- 现存证据：[business-lifecycle-contracts.md](../../docs/design/business-lifecycle-contracts.md) / §9：事件/通知最小合同及B08阈值配置; [03-r3-ticket-linkage.md](../../docs/phase6/03-r3-ticket-linkage.md) / §3.3–3.5：N:1/owner/通知及逐项审计; [business-lifecycle-contracts.md](../../docs/design/business-lifecycle-contracts.md) / §7：DirectorySync与离职/撤部
- 承接与窗口：D15 (本波开工前; P6.2)
- 出口与未决：owner变更/无负责人/上溯层级/抑制/静默例外、越域收件人负例；无合法人留待办不广播。；真实提供方/参数/配置未定时仍按窗口复核，不称取消或完成。

**P6-T052 · P0-5 轻量系统台账 — 专业替代**

- 原文：[docs/phase6/07-backlog-and-triggers.md](../phase6/07-backlog-and-triggers.md) / ### 3.2 phase6 域类 / **P0-5 轻量系统台账**; ✅ 已拍（2026-10-28 所有者）——随 R1 立项批
- 理由：薄系统目录有用，但已有CMDB权威，旧“轻台账”不可扩大成第二资产维护系统。
- 当前条件：D07存稳定ref与必要展示快照；人工改的是本地补充或有期限override，权威原值保留来源。
- 现存证据：[business-lifecycle-contracts.md](../../docs/design/business-lifecycle-contracts.md) / §6：schema求值、约束与Query/prefill; [platform-evolution.md](../../docs/design/platform-evolution.md) / §1/2/4/5/7：专业写主、可靠交接和按窗口冻结; [business-lifecycle-contracts.md](../../docs/design/business-lifecycle-contracts.md) / §8：专业来源、周期、处置与N:1整改单
- 承接与窗口：D07 (本波开工前; P6.2)
- 支持/决策关联：X02；不是本源新增独立交付，不重复计价。
- 出口与未决：无匹配/陈旧/同名/人工覆盖来源清楚，不能猜资产或把本地值反写成CMDB权威。；真实提供方/参数/配置未定时仍按窗口复核，不称取消或完成。

**P6-T053 · P1-7 内部事件面归属 — 保留**

- 原文：[docs/phase6/07-backlog-and-triggers.md](../phase6/07-backlog-and-triggers.md) / ### 3.2 phase6 域类 / **P1-7 内部事件面归属**; ✅ 已拍（2026-10-28 所有者拍 b）——公约层=[standards §14](../standards.md)（已落）；装配守卫随 Week 0（05 号 §1.2 行）
- 理由：事件公约已落不等于订阅装配守卫存在；漏订阅会导致整链静默，属于工程准入而非等业务触发。
- 当前条件：G03固定每域期望订阅集并启动/测试校验；可靠交接仍由R01，不将进程内总线当outbox。
- 现存证据：[engineering-execution-design.md](../../docs/design/engineering-execution-design.md) / G03/G04/G06/C01–C05：当前差距、失败反例与固定出口; [app.go](../../internal/app/app.go) / NewApp：ticket.notify/ops.notify与InboxService.SetEventRegistry；storage_usage_sample/notify_volume_check; [cross-repo-execution-contract.md](../../docs/design/cross-repo-execution-contract.md) / C03/R01：提交、效果收据/意图、reassign三类成功含义
- 承接与窗口：G03 (本波开工前; P6.0)
- 出口与未决：缺notify/inbox等必要装配显式失败，禁止零消费者绿；不把文档§14存在自销实现。；真实提供方/参数/配置未定时仍按窗口复核，不称取消或完成。

**P6-T054 · R5 事件响应台账（D30——攻防演练/护网承载） — 专业替代**

- 原文：[docs/phase6/07-backlog-and-triggers.md](../phase6/07-backlog-and-triggers.md) / ### 3.2 phase6 域类 / **R5 事件响应台账（D30——攻防演练/护网承载）**; R1–R4 收口（X9 已销 2026-11-01：按正常节奏，不为演练调排期）
- 理由：R1–R4收口不是必须自建SOC台账的理由；用户已定专业SOC发现研判，本地保岗位协作与证据。
- 当前条件：D14/P6.E接SOC事件/战役ID、时间线、动作结果与本地应急工单；具体本地职责矩阵需同包冻结。
- 现存证据：[platform-evolution.md](../../docs/design/platform-evolution.md) / §1/2/4/5/7：专业写主、可靠交接和按窗口冻结; [project-evolution-strategy.md](../../docs/design/project-evolution-strategy.md) / §1–8：保留业务效果、薄实现、先决策与后交付分开; [frontend-implementation-contract.md](../../../zhuzhao-ui/docs/design/frontend-implementation-contract.md) / 组件合同、六类工作区、全站映射、已确认UX01/02/03、跨阶段旅程
- 承接与窗口：D14 (本波开工前; P6.E)
- 支持/决策关联：C04；不是本源新增独立交付，不重复计价。
- 出口与未决：来源与补录分标、无权限证据不泄露、断链/撤权/接管/回传未知明确；不计算缺起点MTTD。；真实提供方/参数/配置未定时仍按窗口复核，不称取消或完成。

**P6-T055 · 09 类型级可见性策略（ticket_types.visibility_policy 三策略 + participants 面） — 保留**

- 原文：[docs/phase6/07-backlog-and-triggers.md](../phase6/07-backlog-and-triggers.md) / ### 3.2 phase6 域类 / **09 类型级可见性策略**（ticket_types.visibility_policy 三策略 + participants 面）; **已确认纳入 Phase 6**（2026-11-09 确认方向。代拍项 P1–P5 与排期在阶段入口确定。批内前置为 06 号件 1 和件 3）
- 理由：类型可见性已拍为业务策略，不应再当“触发后考虑”；其live权限与定义快照是不同轴。
- 当前条件：D03/P6.1交旧inherit/新相关人、CC读评、all不穿透、独立参与管理及风险owner通路。
- 现存证据：[business-lifecycle-contracts.md](../../docs/design/business-lifecycle-contracts.md) / §5：类型查看/创建/参与管理和风险组职责; [business-lifecycle-contracts.md](../../docs/design/business-lifecycle-contracts.md) / §4：转换采集/资格/capabilities/首页责任投影
- 承接与窗口：D03 (本波开工前; P6.1)
- 出口与未决：列表/详情/导出/计数/搜索同predicate，类型收紧plan/CAS和角色失效反例；不另增is_sensitive。；真实提供方/参数/配置未定时仍按窗口复核，不称取消或完成。

### 3.3 通知与渠道类

**P6-T056 · 件 13 邮件 SMTP adapter — 专业替代**

- 原文：[docs/phase6/07-backlog-and-triggers.md](../phase6/07-backlog-and-triggers.md) / ### 3.3 通知与渠道类 / 件 13 邮件 SMTP adapter; 外部人员/无钉钉用户到达诉求
- 理由：外部无钉钉用户的到达需求有效，但渠道应先经企业UIP/消息供给；不必立即再写SMTP直连。
- 当前条件：E01/P6.1列邮件供给/身份/收件范围，P7真实子包；缺供给才比较已有ChannelSender的有限SMTP adapter。
- 现存证据：[s6-s5-debt-ledger.md](../../docs/phase5/s6-s5-debt-ledger.md) / B/C/G/H组及挂账质量判据：历史状态与触发原意; [business-lifecycle-contracts.md](../../docs/design/business-lifecycle-contracts.md) / §9：事件/通知最小合同及B08阈值配置; [project-evolution-strategy.md](../../docs/design/project-evolution-strategy.md) / §1–8：保留业务效果、薄实现、先决策与后交付分开
- 承接与窗口：E01 (P6.1出口起逐项核契约，P7出口首次真实验证复核; P7)
- 支持/决策关联：D11；不是本源新增独立交付，不重复计价。
- 出口与未决：桩与real分别验，错误收件/HTML转义/重试/unknown/回执；未接真实邮件不能称到达完成。；真实提供方/参数/配置未定时仍按窗口复核，不称取消或完成。

**P6-T057 · 件 15 完整映射迁移档 — 改写**

- 原文：[docs/phase6/07-backlog-and-triggers.md](../phase6/07-backlog-and-triggers.md) / ### 3.3 通知与渠道类 / 件 15 完整映射迁移档; 删状态后需迁移在途单
- 理由：删状态是完整迁版的合理场景，但最小同名迁版已与D01合并；保留旧实例不迁也可运行。
- 当前条件：D18/P7先单类型CLI from→to mapping，发布hash/逐票CAS/权限/游标恢复，不强迫全量迁到最新。
- 现存证据：[business-lifecycle-contracts.md](../../docs/design/business-lifecycle-contracts.md) / §3：发布/实例钉版/预检/同名最小迁版; [project-evolution-strategy.md](../../docs/design/project-evolution-strategy.md) / §1–8：保留业务效果、薄实现、先决策与后交付分开
- 承接与窗口：D18 (P7对应包开工前冻结最小场景/合同; P7); D01 (本波开工前; P6.1); D04 (本波开工前; P6.1)
- 出口与未决：字段不兼容列冲突、撤权/并发逐项报告；迁版不重跑后动作或重置SLA，补偿是新命令。；真实提供方/参数/配置未定时仍按窗口复核，不称取消或完成。

**P6-T058 · H-5 收件人轴泛化 — 保留**

- 原文：[docs/phase6/07-backlog-and-triggers.md](../phase6/07-backlog-and-triggers.md) / ### 3.3 通知与渠道类 / H-5 收件人轴泛化; R1 首条风险通知规则前
- 理由：风险首条通知需要owner轴是已存在消费，不能等另一泛化框架；工单creator与当前risk_owner不总相同。
- 当前条件：D15只给风险域具名解析当前owner/上级/负责人，授权/来源时效与B08同冻结；不开放任意收件人表达式。
- 现存证据：[business-lifecycle-contracts.md](../../docs/design/business-lifecycle-contracts.md) / §9：事件/通知最小合同及B08阈值配置; [business-lifecycle-contracts.md](../../docs/design/business-lifecycle-contracts.md) / §8：专业来源、周期、处置与N:1整改单; [business-lifecycle-contracts.md](../../docs/design/business-lifecycle-contracts.md) / §7：DirectorySync与离职/撤部
- 承接与窗口：D15 (本波开工前; P6.2)
- 共用依据：P6-T051；本源原意和条件仍单独保留。
- 出口与未决：owner交接旧人不收详情，缺人有待办，通知发送时再验读权；不以历史created_by永久找负责人。；真实提供方/参数/配置未定时仍按窗口复核，不称取消或完成。

**P6-T059 · UIP 真实发送 — 保留**

- 原文：[docs/phase6/07-backlog-and-triggers.md](../phase6/07-backlog-and-triggers.md) / ### 3.3 通知与渠道类 / UIP 真实发送; UIP 契约到手
- 理由：真实UIP契约是认证/消息体/delivery/receipt的必要条件，当前生产fail-fast不能当真实已发送。
- 当前条件：E01从P6.1分provider合同与子交付，D11先mock/contract，P7单条真实通道纵切。
- 现存证据：[s6-s5-debt-ledger.md](../../docs/phase5/s6-s5-debt-ledger.md) / B/C/G/H组及挂账质量判据：历史状态与触发原意; [business-lifecycle-contracts.md](../../docs/design/business-lifecycle-contracts.md) / §9：事件/通知最小合同及B08阈值配置; [project-evolution-strategy.md](../../docs/design/project-evolution-strategy.md) / §1–8：保留业务效果、薄实现、先决策与后交付分开
- 承接与窗口：E01 (P6.1出口起逐项核契约，P7出口首次真实验证复核; P7)
- 支持/决策关联：D11；不是本源新增独立交付，不重复计价。
- 出口与未决：非2xx/超时未知/重复/收据/节流、身份和数据范围真实验；供给不到仍明确待验与复核，不自动取消。；真实提供方/参数/配置未定时仍按窗口复核，不称取消或完成。

**P6-T060 · UIP 通道差距（群机器人/语音）+定时批量推送 — 改写**

- 原文：[docs/phase6/07-backlog-and-triggers.md](../phase6/07-backlog-and-triggers.md) / ### 3.3 通知与渠道类 / UIP 通道差距（群机器人/语音）+定时批量推送; UIP 深化谈判
- 理由：群机器人/语音/批量推送是三类能力，不能“深化谈判”一词全包或默认现UIP具备。
- 当前条件：E01逐通道声明supported/unsupported/unknown；先选择保留受众与时效，批量推送复用TR调度与D11授权收件人。
- 现存证据：[project-evolution-strategy.md](../../docs/design/project-evolution-strategy.md) / §1–8：保留业务效果、薄实现、先决策与后交付分开; [business-lifecycle-contracts.md](../../docs/design/business-lifecycle-contracts.md) / §9：事件/通知最小合同及B08阈值配置; [schedule-recovery-policy.md](../../docs/design/schedule-recovery-policy.md) / §2–8：时隙/策略/episode/预算/预览/停启与恢复
- 承接与窗口：E01 (P6.1出口起逐项核契约，P7出口首次真实验证复核; P7)
- 支持/决策关联：D11,I03；不是本源新增独立交付，不重复计价。
- 出口与未决：一条真实到达不代销其它渠道，定时推送错失/取消/范围变化有receipt，没回执不称已读。；真实提供方/参数/配置未定时仍按窗口复核，不称取消或完成。

**P6-T061 · D-1 delivery 字段定形 — 改写**

- 原文：[docs/phase6/07-backlog-and-triggers.md](../phase6/07-backlog-and-triggers.md) / ### 3.3 通知与渠道类 / D-1 delivery 字段定形; UIP 契约
- 理由：delivery应由提供方合同定形，但当前channel/delivery两层语义已成文，不应再次重命名所有旧配置。
- 当前条件：E01核偏好层delivery与adapter channel映射、可用枚举/兼容默认；未知通道不静默发送。
- 现存证据：[s6-s5-debt-ledger.md](../../docs/phase5/s6-s5-debt-ledger.md) / B/C/G/H组及挂账质量判据：历史状态与触发原意; [business-lifecycle-contracts.md](../../docs/design/business-lifecycle-contracts.md) / §9：事件/通知最小合同及B08阈值配置; [project-evolution-strategy.md](../../docs/design/project-evolution-strategy.md) / §1–8：保留业务效果、薄实现、先决策与后交付分开
- 承接与窗口：E01 (P6.1出口起逐项核契约，P7出口首次真实验证复核; P7)
- 支持/决策关联：D11；不是本源新增独立交付，不重复计价。
- 出口与未决：旧合法配置语义保留，未知/停用delivery拒绝或明确降级；日志记录实际渠道/返回收据而非模型推测。；真实提供方/参数/配置未定时仍按窗口复核，不称取消或完成。

**P6-T062 · 通知双路径收敛 — 退出旧独立待办**

- 原文：[docs/phase6/07-backlog-and-triggers.md](../phase6/07-backlog-and-triggers.md) / ### 3.3 通知与渠道类 / 通知双路径收敛; B6-7 既定
- 理由：B6-7双路径收敛在文档已记录关闭，当前TicketNotifyHook为MessageDispatcher薄壳且app挂ticket/ops；不重开旧直调方案。
- 当前条件：退出旧“再收敛”新建待办，保留同链回归；可靠投递/偏好/应发实发对账仍由D11/R01独立处理。
- 现存证据：[notify_hook.go](../../internal/service/ticket/notify_hook.go) / TicketNotifyHook为MessageDispatcher薄壳；不等于已可靠投递; [app.go](../../internal/app/app.go) / NewApp：ticket.notify/ops.notify与InboxService.SetEventRegistry；storage_usage_sample/notify_volume_check; [s6-s5-debt-ledger.md](../../docs/phase5/s6-s5-debt-ledger.md) / B/C/G/H组及挂账质量判据：历史状态与触发原意
- 承接与窗口：D11 (本波开工前; P6.3)
- 支持/决策关联：R01；不是本源新增独立交付，不重复计价。
- 出口与未决：事件入口覆盖同渲染发送日志链，旧on_complete直send不复活；静态链存在不冒称故障重放已通过。；真实提供方/参数/配置未定时仍按窗口复核，不称取消或完成。

**P6-T063 · activelist 变更事件摄入 — 改写**

- 原文：[docs/phase6/07-backlog-and-triggers.md](../phase6/07-backlog-and-triggers.md) / ### 3.3 通知与渠道类 / activelist 变更事件摄入; 首个「列表变更驱动工单/通知」场景
- 理由：“首个列表变更场景”是价值筛选但不能把所有AL写自动变成可信主仓事件；现没有通用摄入不算bug。
- 当前条件：R06/P7先选一AL类型/变更动作，定义提交后信封/世代/幂等/允许消费者，主仓按当前身份授权消费。
- 现存证据：[project-evolution-strategy.md](../../docs/design/project-evolution-strategy.md) / §1–8：保留业务效果、薄实现、先决策与后交付分开; [cross-repo-execution-contract.md](../../docs/design/cross-repo-execution-contract.md) / C03/R01：提交、效果收据/意图、reassign三类成功含义; [platform-evolution.md](../../docs/design/platform-evolution.md) / §1/2/4/5/7：专业写主、可靠交接和按窗口冻结
- 承接与窗口：R06 (本波开工前; P7); I04 (本波开工前; P7)
- 支持/决策关联：R01；不是本源新增独立交付，不重复计价。
- 出口与未决：全量replace与单行变更区分，重放不重复建单/通知，失败可恢复且不让AL替代业务权威。；真实提供方/参数/配置未定时仍按窗口复核，不称取消或完成。

### 3.4 工单域 P2/基建类

**P6-T064 · 通知事件元数据读取 — 改写**

- 原文：[docs/phase6/07-backlog-and-triggers.md](../phase6/07-backlog-and-triggers.md) / ### 3.4 工单域 P2/基建类 / 通知事件元数据读取; 新事件域接入或需消除手维护词表
- 理由：元数据端点与字段字典/visible-fields不同；旧“新域接入”可促成价值但当前手维护确有漂移成本。
- 当前条件：D11先一个受权只读事件目录，生成或复用后端validEventActions并带域/版本/可用变量；UI不另造词表。
- 现存证据：[message_admin_service.go](../../internal/service/message_admin_service.go) / validEventActions；UI手词表与已存在守卫不是元数据读端点; [s6-s5-debt-ledger.md](../../docs/phase5/s6-s5-debt-ledger.md) / B/C/G/H组及挂账质量判据：历史状态与触发原意; [business-lifecycle-contracts.md](../../docs/design/business-lifecycle-contracts.md) / §9：事件/通知最小合同及B08阈值配置
- 承接与窗口：D11 (本波开工前; P6.3)
- 支持/决策关联：G06；不是本源新增独立交付，不重复计价。
- 出口与未决：后端未知键拒绝，目录与路由/渲染词表差异有守卫；不能返回私有配置或给目录读取者执行权。；真实提供方/参数/配置未定时仍按窗口复核，不称取消或完成。

**P6-T065 · 06 号 P2 六组 — 改写**

- 原文：[docs/phase6/07-backlog-and-triggers.md](../phase6/07-backlog-and-triggers.md) / ### 3.4 工单域 P2/基建类 / 06 号 P2 六组; 各自触发
- 理由：“各自触发”把多个不同能力混在E05会掩盖已到期的UI/权限工作；不能把全部P2推P10。
- 当前条件：拆为watcher→U03、常驻字段ACL→D04、predue/digest→D11；软删/服务端草稿→E05；类型克隆→U04，评价/其它批量→U03/E05逐项裁决。
- 现存证据：[06-ticket-completeness.md](../../docs/phase6/06-ticket-completeness.md) / §3各件、§4 P2原触发与§5不做范围; [frontend-implementation-contract.md](../../../zhuzhao-ui/docs/design/frontend-implementation-contract.md) / 组件合同、六类工作区、全站映射、已确认UX01/02/03、跨阶段旅程; [business-lifecycle-contracts.md](../../docs/design/business-lifecycle-contracts.md) / §5：类型查看/创建/参与管理和风险组职责
- 承接与窗口：U03 (本波开工前; P6.3); U04 (本波开工前; P6.3); D04 (本波开工前; P6.1); D11 (本波开工前; P6.3); E05 (P7出口; P10); D03 (本波开工前; P6.1)
- 支持/决策关联：D02；不是本源新增独立交付，不重复计价。
- 当前条件：先D04/D02/U03/U04/D11既定P6窗口处理已到期子项，只有软删/服务端草稿/外部协作等候选沿E05/P7裁决、P10获准增量；不全部等E05
- 出口与未决：每子项有独立使用例/权限/失败与窗口；自助watcher不扩大可见性，草稿非业务draft态，未选候选不默取消。；真实提供方/参数/配置未定时仍按窗口复核，不称取消或完成。

**P6-T066 · M6 在线用户管理面 / 登录失败解锁管理面 — 改写**

- 原文：[docs/phase6/07-backlog-and-triggers.md](../phase6/07-backlog-and-triggers.md) / ### 3.4 工单域 P2/基建类 / M6 在线用户管理面 / 登录失败解锁管理面; 安全运营诉求（phase6 承担风险管理职能后实际出现）——随任意小批
- 理由：在线会话与解锁不是“随任意小批”的无边界管理功能；已有禁用/撤销/解锁安全守卫应先保留。
- 当前条件：E03/P6.3分别核安全运营场景，P8只读在线会话/受权撤销或审计解锁最薄一项；不复制IAM运营平台。
- 现存证据：[engineering-execution-design.md](../../docs/design/engineering-execution-design.md) / C05：撤会话、安全状态与前端恢复合同; [project-evolution-strategy.md](../../docs/design/project-evolution-strategy.md) / §1–8：保留业务效果、薄实现、先决策与后交付分开; [engineering-execution-design.md](../../docs/design/engineering-execution-design.md) / G03/G04/G06/C01–C05：当前差距、失败反例与固定出口
- 承接与窗口：E03 (P6.3出口; P8)
- 出口与未决：普通人不可枚举会话；撤销后旧AT/RT行为、误解锁/最后管理员和审计明确；Redis不可用不假成功。；真实提供方/参数/配置未定时仍按窗口复核，不称取消或完成。

**P6-T067 · 软删替代物理删 — 改写**

- 原文：[docs/phase6/07-backlog-and-triggers.md](../phase6/07-backlog-and-triggers.md) / ### 3.4 工单域 P2/基建类 / 软删替代物理删; 合规保留期诉求
- 理由：保留期与审计回放是可理解需要，物理删导致链断应先盘点，不等投诉才研究；但不自动批准全软删。
- 当前条件：E05/P7选保留类别/法律业务需求/恢复语义，比较软删、不可变审计快照与归档；获准增量P10有子估算。
- 现存证据：[06-ticket-completeness.md](../../docs/phase6/06-ticket-completeness.md) / §3各件、§4 P2原触发与§5不做范围; [project-evolution-strategy.md](../../docs/design/project-evolution-strategy.md) / §1–8：保留业务效果、薄实现、先决策与后交付分开; [cross-repo-execution-contract.md](../../docs/design/cross-repo-execution-contract.md) / C03/R01：提交、效果收据/意图、reassign三类成功含义
- 承接与窗口：E05 (P7出口; P10)
- 出口与未决：删除后审计可追、权限与复原防冲突、附件/关联/统计清楚；调查完成不声称软删已交。；真实提供方/参数/配置未定时仍按窗口复核，不称取消或完成。

**P6-T068 · B-10 message_log GIN — 改写**

- 原文：[docs/phase6/07-backlog-and-triggers.md](../phase6/07-backlog-and-triggers.md) / ### 3.4 工单域 P2/基建类 / B-10 message_log GIN; 工单量级/首源
- 理由：GIN是查询优化实现选项，工单量级/首源不是足够建索引依据；索引有写放大与迁移成本。
- 当前条件：D11在首消息日志查询合同里列实际谓词/数据分布，G04用EXPLAIN和延迟样本判断GIN、普通索引或维持现状。
- 现存证据：[s6-s5-debt-ledger.md](../../docs/phase5/s6-s5-debt-ledger.md) / B/C/G/H组及挂账质量判据：历史状态与触发原意; [engineering-execution-design.md](../../docs/design/engineering-execution-design.md) / G03/G04/G06/C01–C05：当前差距、失败反例与固定出口; [business-lifecycle-contracts.md](../../docs/design/business-lifecycle-contracts.md) / §9：事件/通知最小合同及B08阈值配置
- 承接与窗口：D11 (本波开工前; P6.3)
- 支持/决策关联：G04；不是本源新增独立交付，不重复计价。
- 出口与未决：非法/慢查询预算、索引前后查询/写成本与回滚可核；不以“无量级”永久不复核。；真实提供方/参数/配置未定时仍按窗口复核，不称取消或完成。

**P6-T069 · B-9 内嵌图回收 — 改写**

- 原文：[docs/phase6/07-backlog-and-triggers.md](../phase6/07-backlog-and-triggers.md) / ### 3.4 工单域 P2/基建类 / B-9 内嵌图回收; 触发
- 理由：引用GC只在证明最后一个引用移除后才可删，不能单靠存储水位；用户已确认移除字段不自动删对象。
- 当前条件：E02/P6.3先盘存量孤儿和用途/引用集，P7决定保留保守GC或有限reconciler，复用C02锁条件。
- 现存证据：[engineering-execution-design.md](../../docs/design/engineering-execution-design.md) / C02：字段/正文/显式附件引用、用途/归属/GC与删除保护; [05-s5-object-storage-richmedia.md](../../docs/phase5/05-s5-object-storage-richmedia.md) / 现有存储用途、权限、引用和GC合同; [s6-s5-debt-ledger.md](../../docs/phase5/s6-s5-debt-ledger.md) / B/C/G/H组及挂账质量判据：历史状态与触发原意
- 承接与窗口：E02 (P6.3出口; P7)
- 支持/决策关联：C02；不是本源新增独立交付，不重复计价。
- 出口与未决：并发绑定/正文与字段多引用/上传成功未提交/回收失败，绝不误删已绑定对象；观察不等于enforce。；真实提供方/参数/配置未定时仍按窗口复核，不称取消或完成。

**P6-T070 · G-31 标签回告 — 改写**

- 原文：[docs/phase6/07-backlog-and-triggers.md](../phase6/07-backlog-and-triggers.md) / ### 3.4 工单域 P2/基建类 / G-31 标签回告; 用户反馈
- 理由：“用户再反馈”没有必要：同标签归属导致添加无变化与摘他人403是已知可解释语义，不是权限缺陷。
- 当前条件：U03批量/单项回执统一added/unchanged与原创建者信息的合法展示，摘他人仍update，不因体验改权限。
- 现存证据：[tag_service.go](../../internal/service/ticket/tag_service.go) / Service.AddTags/RemoveTag：read协作、自摘与摘他人差异; [frontend-implementation-contract.md](../../../zhuzhao-ui/docs/design/frontend-implementation-contract.md) / 组件合同、六类工作区、全站映射、已确认UX01/02/03、跨阶段旅程; [business-lifecycle-contracts.md](../../docs/design/business-lifecycle-contracts.md) / §5：类型查看/创建/参与管理和风险组职责
- 承接与窗口：U03 (本波开工前; P6.3)
- 出口与未决：A先挂/B再挂不转归属，自摘和他摘分验，重复点击结果明确可恢复。；真实提供方/参数/配置未定时仍按窗口复核，不称取消或完成。

**P6-T071 · G-24 ui 三清单派生 — 改写**

- 原文：[docs/phase6/07-backlog-and-triggers.md](../phase6/07-backlog-and-triggers.md) / ### 3.4 工单域 P2/基建类 / G-24 ui 三清单派生; 再漏一次
- 理由：“再漏一次”是被动事故触发；已有三份手工清单应在固定规范窗口决定单源或一致性守卫。
- 当前条件：G06/P6.1核迁移解析器/route registry边界，能派生则派生，不能可靠派生就固定权威与双向校验。
- 现存证据：[icons.test.ts](../../../zhuzhao-ui/src/icons.test.ts) / SEED_ICON_NAMES手维护注；src/icons.ts的MENU_ICON_MAP及router/viewNames.test.ts的STATIC_ROUTES仍存在; [engineering-execution-design.md](../../docs/design/engineering-execution-design.md) / G03/G04/G06/C01–C05：当前差距、失败反例与固定出口
- 承接与窗口：G06 (本波开工前; P6.1)
- 出口与未决：新菜单/图标/静态路由的应过应拒绝例；不可仅复制同名清单或把解析盲区当生成成功。；真实提供方/参数/配置未定时仍按窗口复核，不称取消或完成。

**P6-T072 · G-4 sla_due_at Close 刻 — 改写**

- 原文：[docs/phase6/07-backlog-and-triggers.md](../phase6/07-backlog-and-triggers.md) / ### 3.4 工单域 P2/基建类 / G-4 sla_due_at Close 刻; Close/Assign 收敛批
- 理由：Close/Assign收敛是合理工程机会，但SLA入态属于所有写入口不变量，不应依赖未来收敛才修。
- 当前条件：D02复用统一入态核覆盖Transition/Close/Assign/创建/转办撤回；实施前核当前分支，不照旧G4文字认定全部仍缺。
- 现存证据：[service.go](../../internal/service/ticket/service.go) / Service.Close/assignAuthorized；当前Close/Assign与Transition入态分支须逐个核; [business-lifecycle-contracts.md](../../docs/design/business-lifecycle-contracts.md) / §4：转换采集/资格/capabilities/首页责任投影; [s6-s5-debt-ledger.md](../../docs/phase5/s6-s5-debt-ledger.md) / B/C/G/H组及挂账质量判据：历史状态与触发原意
- 承接与窗口：D02 (本波开工前; P6.1)
- 出口与未决：配置有/无SLA与终态清理、并发状态变化、失败回滚，同一入态deadline/水位一致。；真实提供方/参数/配置未定时仍按窗口复核，不称取消或完成。

**P6-T073 · G-6 reassign 一致性档位 — 改写**

- 原文：[docs/phase6/07-backlog-and-triggers.md](../phase6/07-backlog-and-triggers.md) / ### 3.4 工单域 P2/基建类 / G-6 reassign 一致性档位; S3a 二期拍板
- 理由：旧reassign一概“同步强一致”或“一律异步”都过宽，用户已接受按业务成功含义区分。
- 当前条件：C04/R01同tx做必需本地状态+责任；独立后继持久intent；外部结果前提先待回执。
- 现存证据：[cross-repo-execution-contract.md](../../docs/design/cross-repo-execution-contract.md) / C03/R01：提交、效果收据/意图、reassign三类成功含义; [engineering-execution-design.md](../../docs/design/engineering-execution-design.md) / C04：DBTX试点、org_roles删除双方锁、默认拒绝
- 承接与窗口：R01 (本波开工前; P6.1)
- 出口与未决：目标停用失败无半状态、后继失败可见、ACK/重复/人工改派后旧重试不覆盖；不设普通用户同步异步万能开关。；真实提供方/参数/配置未定时仍按窗口复核，不称取消或完成。

**P6-T074 · G-10 message_log 表形态 — 改写**

- 原文：[docs/phase6/07-backlog-and-triggers.md](../phase6/07-backlog-and-triggers.md) / ### 3.4 工单域 P2/基建类 / G-10 message_log 表形态; 二期表形态批
- 理由：“二期表形态/登记不修”缺退出条件，biz_id类型/dead状态/retry_count须按真实消费者区分，不能机械照旧DDL重做。
- 当前条件：D11入口清点当前message_log字段、死信回执/查询需要与D12结果关系，选择保留、加字段或投影并说明兼容。
- 现存证据：[s6-s5-debt-ledger.md](../../docs/phase5/s6-s5-debt-ledger.md) / B/C/G/H组及挂账质量判据：历史状态与触发原意; [business-lifecycle-contracts.md](../../docs/design/business-lifecycle-contracts.md) / §9：事件/通知最小合同及B08阈值配置; [cross-repo-execution-contract.md](../../docs/design/cross-repo-execution-contract.md) / C03/R01：提交、效果收据/意图、reassign三类成功含义
- 承接与窗口：D11 (本波开工前; P6.3)
- 支持/决策关联：D12；不是本源新增独立交付，不重复计价。
- 出口与未决：旧日志可读、未知业务ID/string、失败归因与重试次数可核，不把展示缺列自动判业务丢失。；真实提供方/参数/配置未定时仍按窗口复核，不称取消或完成。

**P6-T075 · G-12 UI随手批 — 退出旧独立待办**

- 原文：[docs/phase6/07-backlog-and-triggers.md](../phase6/07-backlog-and-triggers.md) / ### 3.4 工单域 P2/基建类 / G-12 UI随手批; 已由后续B20等复验关闭，按债台账逐项核范围
- 理由：G12已由B20固定候选修复回报关闭，当前附件已有原位失败/重试；旧0.5天待办不应复活。
- 当前条件：退出旧一次性UI修复批，G04/U07保旧负例；本次全站视觉/交互改造范围仍由U06。
- 现存证据：[zhuzhao-ecosystem-audit-b20-ui-feedback-2026-11-07.md](../../docs/review/reports/zhuzhao-ecosystem-audit-b20-ui-feedback-2026-11-07.md) / 固定候选UI反馈与红绿反证；不冒称本次运行; [frontend-redesign.md](../../../zhuzhao-ui/docs/design/frontend-redesign.md) / FE-PRE-01–11：实际静态问题/代表方案/全站未实测限制
- 承接与窗口：G04 (本波开工前; P6.1)
- 支持/决策关联：U07；不是本源新增独立交付，不重复计价。
- 出口与未决：保留原红→绿/准确SHA范围；新回归重现才建新缺陷，不借此宣称所有新设计验收通过。；真实提供方/参数/配置未定时仍按窗口复核，不称取消或完成。

**P6-T076 · G-14 FieldImpact软提示 — 退出旧独立待办**

- 原文：[docs/phase6/07-backlog-and-triggers.md](../phase6/07-backlog-and-triggers.md) / ### 3.4 工单域 P2/基建类 / G-14 FieldImpact软提示; 已由B26代码复验关闭
- 理由：G14已有B26报告和softFieldWarnings分支；关闭的是旧软提示缺口，不是停用字典/硬删公式所有情形。
- 当前条件：退出原15min重复开发，G04保radio/字典切换/空集合回归；字典硬删与新快照预检另在D01/D04处理。
- 现存证据：[zhuzhao-ecosystem-audit-b26-field-warnings-2026-11-07.md](../../docs/review/reports/zhuzhao-ecosystem-audit-b26-field-warnings-2026-11-07.md) / 固定候选FieldImpact范围与剩余限制；不冒称本次运行; [type_admin.go](../../internal/service/ticket/type_admin.go) / ReplaceTicketTypeFields/softFieldWarnings；当前代码已有radio/选项/dict相关分支; [business-lifecycle-contracts.md](../../docs/design/business-lifecycle-contracts.md) / §3：发布/实例钉版/预检/同名最小迁版
- 承接与窗口：G04 (本波开工前; P6.1)
- 支持/决策关联：D04；不是本源新增独立交付，不重复计价。
- 出口与未决：不将软提示改硬锁，不要求无历史快照时编旧值；未覆盖外部字典变更明确留白。；真实提供方/参数/配置未定时仍按窗口复核，不称取消或完成。

**P6-T077 · SSE 实时通道 — 改写**

- 原文：[docs/phase6/07-backlog-and-triggers.md](../phase6/07-backlog-and-triggers.md) / ### 3.4 工单域 P2/基建类 / SSE 实时通道; 30s 轮询不可接受确认
- 理由：30s轮询是否不可接受需端到端延迟/容量与受众证据，不应先建SSE再找场景；轮询可作为有效首期。
- 当前条件：E04/P6.E比较推送/轮询成本、身份/重连/游标与更新频率，P8获选视图接同一授权数据。
- 现存证据：[project-evolution-strategy.md](../../docs/design/project-evolution-strategy.md) / §1–8：保留业务效果、薄实现、先决策与后交付分开; [frontend-implementation-contract.md](../../../zhuzhao-ui/docs/design/frontend-implementation-contract.md) / 组件合同、六类工作区、全站映射、已确认UX01/02/03、跨阶段旅程; [04-r4-metrics-reports.md](../../docs/phase6/04-r4-metrics-reports.md) / §3/4/7/附录A：固定统计、快照、报告及跨模块限制
- 承接与窗口：E04 (P6.E出口; P8)
- 支持/决策关联：U07；不是本源新增独立交付，不重复计价。
- 出口与未决：断线补查、撤权停止、重复更新/多标签页与陈旧指示；SSE是形式选择，不取消及时更新业务效果。；真实提供方/参数/配置未定时仍按窗口复核，不称取消或完成。

**P6-T078 · 存储第二 driver + 上传三重校验 — 改写**

- 原文：[docs/phase6/07-backlog-and-triggers.md](../phase6/07-backlog-and-triggers.md) / ### 3.4 工单域 P2/基建类 / 存储第二 driver + 上传三重校验; 附件 2b-ext 开工
- 理由：第二driver是兼容/供给选择，上传三重校验是正确性约束，二者不应以“附件扩期开工”绑一起。
- 当前条件：C02先保用途/对象/引用写校验；E02/P6.3评S3兼容路径/当前SDK问题，P7只为真实不兼容交有限替代。
- 现存证据：[engineering-execution-design.md](../../docs/design/engineering-execution-design.md) / C02：字段/正文/显式附件引用、用途/归属/GC与删除保护; [05-s5-object-storage-richmedia.md](../../docs/phase5/05-s5-object-storage-richmedia.md) / 现有存储用途、权限、引用和GC合同; [project-evolution-strategy.md](../../docs/design/project-evolution-strategy.md) / §1–8：保留业务效果、薄实现、先决策与后交付分开
- 承接与窗口：E02 (P6.3出口; P7)
- 支持/决策关联：C02；不是本源新增独立交付，不重复计价。
- 出口与未决：签名上传/HEAD/类型/大小/confirm/下载/GC跨驱动合同；未证不兼容保当前SDK，不能把暂缓driver等于暂缓校验。；真实提供方/参数/配置未定时仍按窗口复核，不称取消或完成。

**P6-T079 · C-14 cookie Secure — 改写**

- 原文：[docs/phase6/07-backlog-and-triggers.md](../phase6/07-backlog-and-triggers.md) / ### 3.4 工单域 P2/基建类 / C-14 cookie Secure; 生产 HTTPS 网关前置
- 理由：HTTPS场景Secure属性是生产安全准入，不能只写“前置已到才看”；开发HTTP默认false不自动是生产漏洞。
- 当前条件：E09/P6.3列所选网关/HTTPS拓扑、可信代理/域/path与Cookie终态，首次生产前验证；release应有配置检查。
- 现存证据：[file_cookie_secure_template_test.go](../../internal/config/file_cookie_secure_template_test.go) / TestLoad_FileCookieSecureTemplates：当前字段可配置且模板默认false；不证明部署HTTPS终态; [project-evolution-strategy.md](../../docs/design/project-evolution-strategy.md) / §1–8：保留业务效果、薄实现、先决策与后交付分开; [05-s5-object-storage-richmedia.md](../../docs/phase5/05-s5-object-storage-richmedia.md) / 现有存储用途、权限、引用和GC合同
- 承接与窗口：E09 (P6.3出口建立缺口，P7出口复核; P7)
- 支持/决策关联：C05；不是本源新增独立交付，不重复计价。
- 出口与未决：真实HTTPS图片/下载/登录会话与Secure、SameSite/Path检查；HTTP开发样本单列，不把模板test当部署已安全。；真实提供方/参数/配置未定时仍按窗口复核，不称取消或完成。

**P6-T080 · C-15 死信/emitEvent ctx — 改写**

- 原文：[docs/phase6/07-backlog-and-triggers.md](../phase6/07-backlog-and-triggers.md) / ### 3.4 工单域 P2/基建类 / C-15 死信/emitEvent ctx; 接入链路追踪
- 理由：链路追踪不是给异步工作独立有界context与request关联的唯一理由；请求结束后副作用仍要有生命周期。
- 当前条件：I03/P7核emit/dead-letter各入口的独立预算/关联ID与receipt；R01不可丢发送先行覆盖，不等可视化trace平台。
- 现存证据：[cross-repo-execution-contract.md](../../docs/design/cross-repo-execution-contract.md) / C03/R01：提交、效果收据/意图、reassign三类成功含义; [engineering-execution-design.md](../../docs/design/engineering-execution-design.md) / G03/G04/G06/C01–C05：当前差距、失败反例与固定出口; [project-evolution-strategy.md](../../docs/design/project-evolution-strategy.md) / §1–8：保留业务效果、薄实现、先决策与后交付分开
- 承接与窗口：I03 (本波开工前; P7)
- 支持/决策关联：R01；不是本源新增独立交付，不重复计价。
- 出口与未决：请求取消/超时、关机drain、迟到ACK/未知效果与死信可定位；context不能冒称撤销已发生外部效果。；真实提供方/参数/配置未定时仍按窗口复核，不称取消或完成。

**P6-T081 · 品牌 logo 上传（设置页「品牌」区） — 改写**

- 原文：[docs/phase6/07-backlog-and-triggers.md](../phase6/07-backlog-and-triggers.md) / ### 3.4 工单域 P2/基建类 / 品牌 logo 上传（设置页「品牌」区）; **所有者已明示（2026-10-30 问询「logo 是否支持设置上传」）**，排窗=phase6 主线间隙独立管理面小批，不进 Week 0 已拍范围
- 理由：品牌上传是明确管理需求，不应借“主线间隙”无限等待；匿名读入口是新安全面，要独立定purpose/缓存/范围。
- 当前条件：U06/P6.3冻结brand_logo管理与默认恢复，匿名只读必要品牌资产；favicon为独立候选不暗批。
- 现存证据：[frontend-implementation-contract.md](../../../zhuzhao-ui/docs/design/frontend-implementation-contract.md) / 组件合同、六类工作区、全站映射、已确认UX01/02/03、跨阶段旅程; [05-s5-object-storage-richmedia.md](../../docs/phase5/05-s5-object-storage-richmedia.md) / 现有存储用途、权限、引用和GC合同; [engineering-execution-design.md](../../docs/design/engineering-execution-design.md) / C02：字段/正文/显式附件引用、用途/归属/GC与删除保护
- 承接与窗口：U06 (本波开工前; P6.3)
- 出口与未决：恶意MIME/大小/尺寸/未confirm对象/缓存破除/恢复默认；登录未认证读不泄露配置或任意对象。；真实提供方/参数/配置未定时仍按窗口复核，不称取消或完成。

**P6-T082 · 双列侧栏次级栏 — 改写**

- 原文：[docs/phase6/07-backlog-and-triggers.md](../phase6/07-backlog-and-triggers.md) / ### 3.4 工单域 P2/基建类 / 双列侧栏次级栏; 2026-11-02已拍/已落收起态保留图标列，叶子项tooltip、rail已有文字不重复tooltip；早版“整体隐藏”已否决
- 理由：当前收起态保留列是已实施资产，UX01选择默认单侧栏并不授权删除旧布局；hover flyout尚是候选。
- 当前条件：U02/P6.1核四布局兼容等级/退出迁移与flyout任务价值，U06按获选最薄实现逐页推广。
- 现存证据：[frontend-implementation-contract.md](../../../zhuzhao-ui/docs/design/frontend-implementation-contract.md) / 组件合同、六类工作区、全站映射、已确认UX01/02/03、跨阶段旅程; [frontend-redesign.md](../../../zhuzhao-ui/docs/design/frontend-redesign.md) / FE-PRE-01–11：实际静态问题/代表方案/全站未实测限制
- 承接与窗口：U06 (本波开工前; P6.3); U02 (本波开工前; P6.1)
- 出口与未决：键盘/触屏/窄屏/焦点/长菜单可达，旧存储布局配置可恢复；不能将本行“已拍”推广为flyout已完成。；真实提供方/参数/配置未定时仍按窗口复核，不称取消或完成。

**P6-T083 · 首页工作台改版（身份欢迎条+按域合并卡+角色渐进） — 改写**

- 原文：[docs/phase6/07-backlog-and-triggers.md](../phase6/07-backlog-and-triggers.md) / ### 3.4 工单域 P2/基建类 / 首页工作台改版（身份欢迎条+按域合并卡+角色渐进）; **所有者已明示（2026-10-30 走查：六卡全工单态要合并+补任务/消息域+身份组织信息）**，排窗=**随 phase6 执行（2026-10-30 所有者拍板：不做间隙小批，统一入 phase6；具体波次随启动批/完善批排期归位）**
- 理由：旧纯前端1–2天/全局四状态副行已不足以涵盖安全首页和责任口径；原八次列表查询也需门控/错误状态。
- 当前条件：U01/P6.0先可达，U02冻结本人当前处理/明确复核及三域卡合同，U06迁页；D10同批接R3复核，不靠自动改assigned_to。
- 现存证据：[index.vue](../../../zhuzhao-ui/src/views/Home/index.vue) / onMounted：8次列表取数；assignee两态待办；失败统计归零的当前静态事实; [frontend-implementation-contract.md](../../../zhuzhao-ui/docs/design/frontend-implementation-contract.md) / 组件合同、六类工作区、全站映射、已确认UX01/02/03、跨阶段旅程; [business-lifecycle-contracts.md](../../docs/design/business-lifecycle-contracts.md) / §4：转换采集/资格/capabilities/首页责任投影
- 承接与窗口：U01 (本波开工前; P6.0); U02 (本波开工前; P6.1); U06 (本波开工前; P6.3); D10 (本波开工前; P6.2)
- 出口与未决：甲处理乙复核同票去重、任务task:read与submitted_by=me、消息保留、空菜单无业务请求、失败不归零。；真实提供方/参数/配置未定时仍按窗口复核，不称取消或完成。

**P6-T084 · 附件/协作按钮按单能力下发（viewerCan\* 统一驱动——「按钮可见但操作失败」正解） — 保留**

- 原文：[docs/phase6/07-backlog-and-triggers.md](../phase6/07-backlog-and-triggers.md) / ### 3.4 工单域 P2/基建类 / 附件/协作按钮按单能力下发（viewerCan\* 统一驱动——「按钮可见但操作失败」正解）; **所有者已明示（2026-10-30 走查指认+最优解拍板：写权自动带附件权模型不动、判定结果作为数据下发）**，排窗=随 phase6 执行（06 号 §3.18 件 17）
- 理由：“按钮可见操作失败”有明确现状，统一按票capability是正确接缝；不能用全局角色启发式替代资源判断。
- 当前条件：D02输出update/attach/comment/manage等能力与当前转换，UI缺字段failclosed；实际写仍重验。
- 现存证据：[business-lifecycle-contracts.md](../../docs/design/business-lifecycle-contracts.md) / §4：转换采集/资格/capabilities/首页责任投影; [frontend-implementation-contract.md](../../../zhuzhao-ui/docs/design/frontend-implementation-contract.md) / 组件合同、六类工作区、全站映射、已确认UX01/02/03、跨阶段旅程; [06-ticket-completeness.md](../../docs/phase6/06-ticket-completeness.md) / §3各件、§4 P2原触发与§5不做范围
- 承接与窗口：D02 (本波开工前; P6.1)
- 支持/决策关联：U03；不是本源新增独立交付，不重复计价。
- 出口与未决：透明读/跨部门/撤权/403或409后刷新能力、输入保留；设置min_editor_tier不套工单能力。；真实提供方/参数/配置未定时仍按窗口复核，不称取消或完成。

**P6-T085 · visible_when 服务端求值端点（复合表达式静默丢值正解——前端单条件解析器防漂移升级） — 保留**

- 原文：[docs/phase6/07-backlog-and-triggers.md](../phase6/07-backlog-and-triggers.md) / ### 3.4 工单域 P2/基建类 / visible_when 服务端求值端点（复合表达式静默丢值正解——前端单条件解析器防漂移升级）; **所有者已明示（2026-10-30 扫荡审计指认 M-5+批准 SSOT 求值方案）**，排窗=随 phase6 执行（06 号 §3.19 件 18）
- 理由：前端局部解析器无法完整复刻expr全文法，服务端同源预览必要，但不是把基础visible_when重新立项。
- 当前条件：D04复用求值端口/预算/context和schema版本，UI防抖、序列守卫，非法表达式与提交同failclosed。
- 现存证据：[business-lifecycle-contracts.md](../../docs/design/business-lifecycle-contracts.md) / §6：schema求值、约束与Query/prefill; [engineering-execution-design.md](../../docs/design/engineering-execution-design.md) / C01：AST角色/作用域与输入隔离、旧配置扫描; [frontend-implementation-contract.md](../../../zhuzhao-ui/docs/design/frontend-implementation-contract.md) / 组件合同、六类工作区、全站映射、已确认UX01/02/03、跨阶段旅程
- 承接与窗口：D04 (本波开工前; P6.1)
- 支持/决策关联：U04；不是本源新增独立交付，不重复计价。
- 出口与未决：复杂AND/OR/函数/字典/迟到返回/缺定义、隐藏值处理与G2一致；不能把预览当最终授权。；真实提供方/参数/配置未定时仍按窗口复核，不称取消或完成。

**P6-T086 · 列表筛选组合与反选（状态多选/排除模式——「组合+反选」筛选体验） — 保留**

- 原文：[docs/phase6/07-backlog-and-triggers.md](../phase6/07-backlog-and-triggers.md) / ### 3.4 工单域 P2/基建类 / 列表筛选组合与反选（状态多选/排除模式——「组合+反选」筛选体验）; **所有者已明示（2026-11-10 phase5 走查指认：待办角标口径「排除已关闭/已驳回」人工无法复现——状态单选无排除模式；明示「应该有组合和反选的感觉，加到 phase6 优化项」）**
- 理由：状态集合排除已是当前后端合同，用户要可复现待办口径是现需求，不再等笼统优化批。
- 当前条件：U03/P6.3以多选/排除首场景接状态集合，chips常驻；任意跨字段逻辑树另按有界查询预算裁决。
- 现存证据：[frontend-implementation-contract.md](../../../zhuzhao-ui/docs/design/frontend-implementation-contract.md) / 组件合同、六类工作区、全站映射、已确认UX01/02/03、跨阶段旅程; [business-lifecycle-contracts.md](../../docs/design/business-lifecycle-contracts.md) / §4：转换采集/资格/capabilities/首页责任投影; [business-lifecycle-contracts.md](../../docs/design/business-lifecycle-contracts.md) / §5：类型查看/创建/参与管理和风险组职责
- 承接与窗口：U03 (本波开工前; P6.3)
- 出口与未决：首页/角标深链与手工筛选/导出同结果，未知/自定义终态与清除条件，切筛选清选择。；真实提供方/参数/配置未定时仍按窗口复核，不称取消或完成。

**P6-T087 · 沙箱持续回归与 expr-lang 引用守卫（F1/F4） — 改写**

- 原文：[docs/phase6/07-backlog-and-triggers.md](../phase6/07-backlog-and-triggers.md) / ### 3.4 工单域 P2/基建类 / 沙箱持续回归与 expr-lang 引用守卫（F1/F4）; **phase5 走查问询实证（2026-11-10，[12 号 §5.2 F1/F4](../phase5/12-scenarios-and-tests.md)）**：所测探针和源码核对表明当前仅 `internal/pkg/exprcalc` 一处引用 expr 库且无守卫；phase6 流程条件/自动化规则/R3 预填任一开工前落
- 理由：沙箱既有正反例应持续守卫，不应到首外部源才防绕过统一入口；但现有单一引用不证明未来不漂移。
- 当前条件：C01/G04在表达式新增消费者前固定AST/输入隔离和依赖边界检查，用应拒绝例证明守卫生效。
- 现存证据：[engineering-execution-design.md](../../docs/design/engineering-execution-design.md) / C01：AST角色/作用域与输入隔离、旧配置扫描; [engineering-execution-design.md](../../docs/design/engineering-execution-design.md) / G03/G04/G06/C01–C05：当前差距、失败反例与固定出口
- 承接与窗口：C01 (本波开工前; P6.0)
- 支持/决策关联：G04；不是本源新增独立交付，不重复计价。
- 出口与未决：函数名与值位同名、let/闭包、伪公式值、预算/未知AST/合法函数不退化；不通过禁全部函数逃避。；真实提供方/参数/配置未定时仍按窗口复核，不称取消或完成。

**P6-T088 · 画布选中反馈与右键属性（F15/F16） — 保留**

- 原文：[docs/phase6/07-backlog-and-triggers.md](../phase6/07-backlog-and-triggers.md) / ### 3.4 工单域 P2/基建类 / 画布选中反馈与右键属性（F15/F16）; 所有者在 Phase5 走查反馈选中态弱、需要查看节点入出边及连线指向，随 Phase6 B0-2 交互评审
- 理由：选中反馈、入出边与指向辨识直接影响配置正确性，是已知任务需求；右键不能成为唯一可达入口。
- 当前条件：U04统一选中/hover/问题定位/属性区，保留键盘与可读列表替代、未保存和图更新同步。
- 现存证据：[frontend-implementation-contract.md](../../../zhuzhao-ui/docs/design/frontend-implementation-contract.md) / 组件合同、六类工作区、全站映射、已确认UX01/02/03、跨阶段旅程; [06-ticket-completeness.md](../../docs/phase6/06-ticket-completeness.md) / §3各件、§4 P2原触发与§5不做范围; [frontend-redesign.md](../../../zhuzhao-ui/docs/design/frontend-redesign.md) / FE-PRE-01–11：实际静态问题/代表方案/全站未实测限制
- 承接与窗口：U04 (本波开工前; P6.3)
- 出口与未决：长节点/多边/缩放、键盘/触屏、撤销重做/JSON往返及影响预览；避免只做色彩高亮自销。；真实提供方/参数/配置未定时仍按窗口复核，不称取消或完成。

**P6-T089 · 工单编号可发现性（K4/F35） — 保留**

- 原文：[docs/phase6/07-backlog-and-triggers.md](../phase6/07-backlog-and-triggers.md) / ### 3.4 工单域 P2/基建类 / 工单编号可发现性（K4/F35）; Phase5 走查发现列表与详情缺人读编号，关键词搜索未覆盖 TK- 编号
- 理由：人读编号后端已生成，发现/搜索路径仍需统一；缺展示是体验范围，不等于全编号未实现。
- 当前条件：U03列表/详情/消息展示，搜索明确标题/数字ID/TK号；历史无号兼容，通知载荷归D11。
- 现存证据：[frontend-implementation-contract.md](../../../zhuzhao-ui/docs/design/frontend-implementation-contract.md) / 组件合同、六类工作区、全站映射、已确认UX01/02/03、跨阶段旅程; [06-ticket-completeness.md](../../docs/phase6/06-ticket-completeness.md) / §3各件、§4 P2原触发与§5不做范围; [business-lifecycle-contracts.md](../../docs/design/business-lifecycle-contracts.md) / §9：事件/通知最小合同及B08阈值配置
- 承接与窗口：U03 (本波开工前; P6.3)
- 支持/决策关联：D11；不是本源新增独立交付，不重复计价。
- 出口与未决：旧无编号单可查，编号不取代string资源ID，越权编号查询同形；消息深链与列表命中一致。；真实提供方/参数/配置未定时仍按窗口复核，不称取消或完成。

**P6-T090 · 情报陈旧结果反馈（K5） — 改写**

- 原文：[docs/phase6/07-backlog-and-triggers.md](../phase6/07-backlog-and-triggers.md) / ### 3.4 工单域 P2/基建类 / 情报陈旧结果反馈（K5）; 正常缓存到期与上游配额故障共用误导文案
- 理由：陈旧可能来自正常TTL、刷新失败或配额，统一归因配额是错误推断；应忠实消费机器事实而非发明原因。
- 当前条件：U05/P6.3区分获取/观测/到期、stale与refresh状态、来源错误及明确重查；现本机元数据不冒历史快照。
- 现存证据：[frontend-implementation-contract.md](../../../zhuzhao-ui/docs/design/frontend-implementation-contract.md) / 组件合同、六类工作区、全站映射、已确认UX01/02/03、跨阶段旅程; [frontend-redesign.md](../../../zhuzhao-ui/docs/design/frontend-redesign.md) / FE-PRE-01–11：实际静态问题/代表方案/全站未实测限制
- 承接与窗口：U05 (本波开工前; P6.3)
- 出口与未决：有效/陈旧可用/上游失败/无缓存分别示例，结果绑定请求key、无评分中性展示。；真实提供方/参数/配置未定时仍按窗口复核，不称取消或完成。

**P6-T091 · 表单渲染与配置增强（F5/F6/F7/F10） — 保留**

- 原文：[docs/phase6/07-backlog-and-triggers.md](../phase6/07-backlog-and-triggers.md) / ### 3.4 工单域 P2/基建类 / 表单渲染与配置增强（F5/F6/F7/F10）; Phase5 走查提出实时公式预览、tips 说明块、字段字数上限与自由排版
- 理由：实时公式、tips、max_length、自由排版是四项不同能力，部分类型与保存保真已修，不能整组重建。
- 当前条件：D04冻结长度单位/类型/公式服务端预览，U04做结构化控件与真实预览；已有字段类型保留往返。
- 现存证据：[business-lifecycle-contracts.md](../../docs/design/business-lifecycle-contracts.md) / §6：schema求值、约束与Query/prefill; [frontend-implementation-contract.md](../../../zhuzhao-ui/docs/design/frontend-implementation-contract.md) / 组件合同、六类工作区、全站映射、已确认UX01/02/03、跨阶段旅程; [06-ticket-completeness.md](../../docs/phase6/06-ticket-completeness.md) / §3各件、§4 P2原触发与§5不做范围
- 承接与窗口：D04 (本波开工前; P6.1); U04 (本波开工前; P6.3)
- 支持/决策关联：U03；不是本源新增独立交付，不重复计价。
- 出口与未决：emoji/中文长度、required/显隐/公式同源、旧meta不改写、长表单末端可达。；真实提供方/参数/配置未定时仍按窗口复核，不称取消或完成。

**P6-T092 · 布局旧数据与绑定校验（F12 余项 / F8） — 改写**

- 原文：[docs/phase6/07-backlog-and-triggers.md](../phase6/07-backlog-and-triggers.md) / ### 3.4 工单域 P2/基建类 / 布局旧数据与绑定校验（F12 余项 / F8）; 件 21 启用 `col_span/group` 投影前，件 22 开工前
- 理由：旧col_span=1可能用户有意，F8原“HTTP拒11类”已被反证；不能以旧结论强制批量修数据或删绑定枚举。
- 当前条件：D04/U04先只读扫描旧布局来源，保留/显式迁改；元素级dive/合法类型服务规则一并核。
- 现存证据：[business-lifecycle-contracts.md](../../docs/design/business-lifecycle-contracts.md) / §6：schema求值、约束与Query/prefill; [06-ticket-completeness.md](../../docs/phase6/06-ticket-completeness.md) / §3各件、§4 P2原触发与§5不做范围; [type_admin.go](../../internal/service/ticket/type_admin.go) / ReplaceTicketTypeFields/softFieldWarnings；当前代码已有radio/选项/dict相关分支
- 承接与窗口：D04 (本波开工前; P6.1); U04 (本波开工前; P6.3)
- 支持/决策关联：U03；不是本源新增独立交付，不重复计价。
- 出口与未决：未改字段原样往返、非法元素拒绝、11合法类HTTP/UI均可达，真实缺口才补，不自动重置用户布局。；真实提供方/参数/配置未定时仍按窗口复核，不称取消或完成。

**P6-T093 · 配置包读写往返前置（F11） — 保留**

- 原文：[docs/phase6/07-backlog-and-triggers.md](../phase6/07-backlog-and-triggers.md) / ### 3.4 工单域 P2/基建类 / 配置包读写往返前置（F11）; B3 件 7 类型定义包接入前
- 理由：空options往返是配置包入口真实兼容约束；没有首次生产晋级前完整隔离预演不能据代码片段称解决。
- 当前条件：D05依赖D01已发布快照，计划绑定包/目标/批准/基线；GET空/缺省/合法选项与导入都按同合同。
- 现存证据：[business-lifecycle-contracts.md](../../docs/design/business-lifecycle-contracts.md) / §3：发布/实例钉版/预检/同名最小迁版; [business-lifecycle-contracts.md](../../docs/design/business-lifecycle-contracts.md) / §13 E10：风险Excel/工单业务导入/跨指纹merge分别裁决; [business-lifecycle-contracts.md](../../docs/design/business-lifecycle-contracts.md) / §2–5/7–11：定义、动作/权限、目录与风险闭环
- 承接与窗口：D05 (本波开工前; P6.3)
- 出口与未决：源→包→目标往返、引用变更冲突、成功审计同tx、逐项补偿/灾备分开；不是工单业务导入。；真实提供方/参数/配置未定时仍按窗口复核，不称取消或完成。

**P6-T094 · 管理员建单组织范围（F21） — 改写**

- 原文：[docs/phase6/07-backlog-and-triggers.md](../phase6/07-backlog-and-triggers.md) / ### 3.4 工单域 P2/基建类 / 管理员建单组织范围（F21）; 所有者走查发现管理员代其他组织建单须先入组
- 理由：管理员范围是授权决策，不应以“代建体验不便”默认入任意组织；当前创建同分支/全局管理分支需据实保留。
- 当前条件：D03/P6.1冻结create_policy×组织创建范围，creation-options仅返回合法选项；特殊领域派单是具名受限用例。
- 现存证据：[business-lifecycle-contracts.md](../../docs/design/business-lifecycle-contracts.md) / §5：类型查看/创建/参与管理和风险组职责; [business-lifecycle-contracts.md](../../docs/design/business-lifecycle-contracts.md) / §2–5/7–11：定义、动作/权限、目录与风险闭环
- 承接与窗口：D03 (本波开工前; P6.1)
- 出口与未决：普通人/组织管理者/globaladmin分别验跨组织，客户端created_by不可自报；不新增未批准的普遍冒用身份。；真实提供方/参数/配置未定时仍按窗口复核，不称取消或完成。

**P6-T095 · 附件用途防误用（F23） — 改写**

- 原文：[docs/phase6/07-backlog-and-triggers.md](../phase6/07-backlog-and-triggers.md) / ### 3.4 工单域 P2/基建类 / 附件用途防误用（F23）; `attachment_create` 用于建单前悬挂对象，其 confirm 接收 `ticket_id` 时会造成已绑定错觉
- 理由：attachment_create携ticket_id的歧义是契约缺口，但写入任意key不自动证明下载越权；兼容期必须盘消费者。
- 当前条件：C02先记录矛盾参数使用并明确弃用，已有票用ticket_attachment；下一兼容发布后严格拒绝，统一引用归属。
- 现存证据：[engineering-execution-design.md](../../docs/design/engineering-execution-design.md) / C02：字段/正文/显式附件引用、用途/归属/GC与删除保护; [index.vue](../../../zhuzhao-ui/src/views/ticket/create/index.vue) / handleFileUpload：files[0]成功后追加key；不把已有追加误报为全缺
- 承接与窗口：C02 (本波开工前; P6.0)
- 出口与未决：旧合法暂存不回退，错purpose/未confirm/外单/他人暂存拒绝且不泄露外单，GC竞争不误删。；真实提供方/参数/配置未定时仍按窗口复核，不称取消或完成。

**P6-T096 · 存储 MIME 默认与运行时配置（F22） — 改写**

- 原文：[docs/phase6/07-backlog-and-triggers.md](../phase6/07-backlog-and-triggers.md) / ### 3.4 工单域 P2/基建类 / 存储 MIME 默认与运行时配置（F22）; Phase6 重规划时核对当前开发 KV 与出厂默认的差异
- 理由：开发KV扩充不能当出厂支持；MIME支持需真实格式与内容检测，不是把所有扩展名加入默认白名单。
- 当前条件：E02/P6.3核当前默认/运行覆盖/字段白名单/图像与附件用途，P7选择明确支持矩阵和文案。
- 现存证据：[engineering-execution-design.md](../../docs/design/engineering-execution-design.md) / C02：字段/正文/显式附件引用、用途/归属/GC与删除保护; [05-s5-object-storage-richmedia.md](../../docs/phase5/05-s5-object-storage-richmedia.md) / 现有存储用途、权限、引用和GC合同; [s6-s5-debt-ledger.md](../../docs/phase5/s6-s5-debt-ledger.md) / B/C/G/H组及挂账质量判据：历史状态与触发原意
- 承接与窗口：E02 (P6.3出口; P7)
- 出口与未决：签名/实际内容不符、空MIME、多后缀、大小及字段更严格范围；配置扩容不突破硬底线。；真实提供方/参数/配置未定时仍按窗口复核，不称取消或完成。

**P6-T097 · 设置反馈、层级与人员查找（F24–F27） — 保留**

- 原文：[docs/phase6/07-backlog-and-triggers.md](../phase6/07-backlog-and-triggers.md) / ### 3.4 工单域 P2/基建类 / 设置反馈、层级与人员查找（F24–F27）; 所有者走查发现部分保存失败无具体原因、无权设置仍可编辑、MIME 输入受 IME 干扰、姓名无法搜索
- 理由：部分成功/编辑层级/IME/搜索是四个现有管理任务，不需等新风险域；系统设置层级不同于工单capability。
- 当前条件：U06/P6.3分键反馈保成功、min_editor_tier门控、MIME文本IME处理、人员搜索字段与序列合同。
- 现存证据：[frontend-implementation-contract.md](../../../zhuzhao-ui/docs/design/frontend-implementation-contract.md) / 组件合同、六类工作区、全站映射、已确认UX01/02/03、跨阶段旅程; [frontend-redesign.md](../../../zhuzhao-ui/docs/design/frontend-redesign.md) / FE-PRE-01–11：实际静态问题/代表方案/全站未实测限制; [business-lifecycle-contracts.md](../../docs/design/business-lifecycle-contracts.md) / §5：类型查看/创建/参与管理和风险组职责
- 承接与窗口：U06 (本波开工前; P6.3)
- 出口与未决：保存后读失败不重复写，层级撤权/组合输入/姓名工号/乱序结果与失败不冒无匹配逐项验。；真实提供方/参数/配置未定时仍按窗口复核，不称取消或完成。

**P6-T098 · 发送记录可读性与深链（F29/F30） — 保留**

- 原文：[docs/phase6/07-backlog-and-triggers.md](../phase6/07-backlog-and-triggers.md) / ### 3.4 工单域 P2/基建类 / 发送记录可读性与深链（F29/F30）; 收件人为裸 ID，工单业务 ID 无跳转
- 理由：裸ID与无深链是可读性问题，姓名/工号需当前授权和历史用户语义；不能为了好看泄露已不可见票。
- 当前条件：D11联U06做受权display ref/工单链接，历史失效用户可中性回显，当前对象无权不提供详情。
- 现存证据：[business-lifecycle-contracts.md](../../docs/design/business-lifecycle-contracts.md) / §9：事件/通知最小合同及B08阈值配置; [frontend-implementation-contract.md](../../../zhuzhao-ui/docs/design/frontend-implementation-contract.md) / 组件合同、六类工作区、全站映射、已确认UX01/02/03、跨阶段旅程; [business-lifecycle-contracts.md](../../docs/design/business-lifecycle-contracts.md) / §5：类型查看/创建/参与管理和风险组职责
- 承接与窗口：D11 (本波开工前; P6.3)
- 支持/决策关联：U06；不是本源新增独立交付，不重复计价。
- 出口与未决：无编号/已删除/离职/撤权/跨域消息均可理解，日志读权不赠予业务读权。；真实提供方/参数/配置未定时仍按窗口复核，不称取消或完成。

**P6-T099 · 顶栏待办与消息入口辨识（F32） — 改写**

- 原文：[docs/phase6/07-backlog-and-triggers.md](../phase6/07-backlog-and-triggers.md) / ### 3.4 工单域 P2/基建类 / 顶栏待办与消息入口辨识（F32）; 所有者走查未能分辨待办与站内信入口
- 理由：入口区分是任务可发现性问题，不能靠合并图标就偷换消息与当前责任待办口径。
- 当前条件：U02/U06按已确认三域首页与顶栏身份入口，给名称/计数/目标筛选一致模型；图标是否合并用任务样本定。
- 现存证据：[frontend-implementation-contract.md](../../../zhuzhao-ui/docs/design/frontend-implementation-contract.md) / 组件合同、六类工作区、全站映射、已确认UX01/02/03、跨阶段旅程; [business-lifecycle-contracts.md](../../docs/design/business-lifecycle-contracts.md) / §4：转换采集/资格/capabilities/首页责任投影; [index.vue](../../../zhuzhao-ui/src/views/Home/index.vue) / onMounted：8次列表取数；assignee两态待办；失败统计归零的当前静态事实
- 承接与窗口：U06 (本波开工前; P6.3)
- 支持/决策关联：U02；不是本源新增独立交付，不重复计价。
- 出口与未决：普通用户能区分未读和待办，甲乙责任与消息已读独立，空菜单安全落点且无越权请求。；真实提供方/参数/配置未定时仍按窗口复核，不称取消或完成。

**P6-T100 · 状态级字段展示与编辑权限（F33② / F34） — 改写**

- 原文：[docs/phase6/07-backlog-and-triggers.md](../phase6/07-backlog-and-triggers.md) / ### 3.4 工单域 P2/基建类 / 状态级字段展示与编辑权限（F33② / F34）; 处理流程需要字段在特定状态展示，或按角色和状态决定编辑范围
- 理由：转换采集不等于常驻读写ACL；可见when也不等于授权，旧矩阵必须区分三者避免前端隐藏当安全。
- 当前条件：D04/D02先读字段/写字段/collect字段与domain资格字段各自权威，D03再与类型可见性求交；最小场景冻结优先级。
- 现存证据：[business-lifecycle-contracts.md](../../docs/design/business-lifecycle-contracts.md) / §6：schema求值、约束与Query/prefill; [business-lifecycle-contracts.md](../../docs/design/business-lifecycle-contracts.md) / §4：转换采集/资格/capabilities/首页责任投影; [business-lifecycle-contracts.md](../../docs/design/business-lifecycle-contracts.md) / §5：类型查看/创建/参与管理和风险组职责
- 承接与窗口：U03 (本波开工前; P6.3); U04 (本波开工前; P6.3); D04 (本波开工前; P6.1); D11 (本波开工前; P6.3); E05 (P7出口; P10); D03 (本波开工前; P6.1)
- 支持/决策关联：D02；不是本源新增独立交付，不重复计价。
- 出口与未决：接口直写/模型patch/取数不能写受管risk_owner，转换先验写前资格；无权字段不经摘要/导出泄露。；真实提供方/参数/配置未定时仍按窗口复核，不称取消或完成。

**P6-T101 · 任务失败记录命名与知晓机制（F37） — 改写**

- 原文：[docs/phase6/07-backlog-and-triggers.md](../phase6/07-backlog-and-triggers.md) / ### 3.4 工单域 P2/基建类 / 任务失败记录命名与知晓机制（F37）; 所有者反馈死信术语难懂，红点缺少已知但暂缓处理语义
- 理由：死信命名/红点知晓是体验治理，不应删除失败事实或以已读标记冒充恢复成功。
- 当前条件：U06联I03区分failed/dead/unknown与acknowledged，知晓只对操作者/合法scope生效，重试入口不被静默藏掉。
- 现存证据：[frontend-implementation-contract.md](../../../zhuzhao-ui/docs/design/frontend-implementation-contract.md) / 组件合同、六类工作区、全站映射、已确认UX01/02/03、跨阶段旅程; [cross-repo-execution-contract.md](../../docs/design/cross-repo-execution-contract.md) / C03/R01：提交、效果收据/意图、reassign三类成功含义; [project-evolution-strategy.md](../../docs/design/project-evolution-strategy.md) / §1–8：保留业务效果、薄实现、先决策与后交付分开
- 承接与窗口：U06 (本波开工前; P6.3)
- 支持/决策关联：I03；不是本源新增独立交付，不重复计价。
- 出口与未决：知晓后失败仍可查、权限撤销/多人读状态/重试新attempt清楚；跨仓词表与E2E锚点同批更新。；真实提供方/参数/配置未定时仍按窗口复核，不称取消或完成。

**P6-T102 · 通知测试发送（H-6 走查增补） — 保留**

- 原文：[docs/phase6/07-backlog-and-triggers.md](../phase6/07-backlog-and-triggers.md) / ### 3.4 工单域 P2/基建类 / 通知测试发送（H-6 走查增补）; 所有者需要在预览外验证选定通道真实到达
- 理由：预览不证明真实到达，测试发送应是明确受控副作用，不能沿用纯渲染权限。
- 当前条件：D11先桩preview/test两合同；E01真实通道仅允许受权测试收件人/限频/确认，audit与结果回执单独记录。
- 现存证据：[business-lifecycle-contracts.md](../../docs/design/business-lifecycle-contracts.md) / §9：事件/通知最小合同及B08阈值配置; [s6-s5-debt-ledger.md](../../docs/phase5/s6-s5-debt-ledger.md) / B/C/G/H组及挂账质量判据：历史状态与触发原意; [cross-repo-execution-contract.md](../../docs/design/cross-repo-execution-contract.md) / C03/R01：提交、效果收据/意图、reassign三类成功含义
- 承接与窗口：D11 (本波开工前; P6.3)
- 支持/决策关联：E01；不是本源新增独立交付，不重复计价。
- 出口与未决：任意收件人/模板注入/秘钥泄露/重复/超时unknown拒或可对账；测试不污染正式运营指标。；真实提供方/参数/配置未定时仍按窗口复核，不称取消或完成。

**P6-T103 · 活动流与能力下发排期建议（件 6 / 件 17） — 改写**

- 原文：[docs/phase6/07-backlog-and-triggers.md](../phase6/07-backlog-and-triggers.md) / ### 3.4 工单域 P2/基建类 / 活动流与能力下发排期建议（件 6 / 件 17）; 所有者强调时间线重要，走查多次发现按钮可见但操作失败
- 理由：时间线和按单能力是详情改版的前置事实，不能只把它们列“优先评估”再让件23先做壳子。
- 当前条件：D02先capability与转换事件/字段真值diff合同；U03详情消费评论/事件/字段变更活动，来源和本地记录分标。D12只在真实任务结果需要展示时作为供给，不能代替件6的ticket_events payload。
- 现存证据：[business-lifecycle-contracts.md](../../docs/design/business-lifecycle-contracts.md) / §4：转换采集/资格/capabilities/首页责任投影; [frontend-implementation-contract.md](../../../zhuzhao-ui/docs/design/frontend-implementation-contract.md) / 组件合同、六类工作区、全站映射、已确认UX01/02/03、跨阶段旅程; [cross-repo-execution-contract.md](../../docs/design/cross-repo-execution-contract.md) / C03/R01：提交、效果收据/意图、reassign三类成功含义
- 承接与窗口：D02 (本波开工前; P6.1); U03 (本波开工前; P6.3)
- 支持/决策关联：R01；不是本源新增独立交付，不重复计价。
- 出口与未决：日志/评论/字段diff可追且不存全文secret，能力缺失failclosed；设置页用自身min_editor_tier。；真实提供方/参数/配置未定时仍按窗口复核，不称取消或完成。

**P6-T104 · 头像请求关联、交互与限制说明（F18–F20） — 改写**

- 原文：[docs/phase6/07-backlog-and-triggers.md](../phase6/07-backlog-and-triggers.md) / ### 3.4 工单域 P2/基建类 / 头像请求关联、交互与限制说明（F18–F20）; 所有者提出圆形参考圈、初始裁剪框与限制提示诉求
- 理由：头像双发是待复现线索，裁剪参考圈/初始框是候选，不能把旧发现等同当前main仍有请求bug。
- 当前条件：U06/P6.3先关联单次操作→请求ID→对象，区分原图解码与裁剪后大小、GIF原样；再评参考圈和比例。
- 现存证据：[frontend-implementation-contract.md](../../../zhuzhao-ui/docs/design/frontend-implementation-contract.md) / 组件合同、六类工作区、全站映射、已确认UX01/02/03、跨阶段旅程; [05-s5-object-storage-richmedia.md](../../docs/phase5/05-s5-object-storage-richmedia.md) / 现有存储用途、权限、引用和GC合同; [engineering-execution-design.md](../../docs/design/engineering-execution-design.md) / C02：字段/正文/显式附件引用、用途/归属/GC与删除保护
- 承接与窗口：U06 (本波开工前; P6.3)
- 出口与未决：取消/重复确认/大原图小产物/会话切换/旧version冲突可追；已修共享version不回退。；真实提供方/参数/配置未定时仍按窗口复核，不称取消或完成。

**P6-T105 · 公式、文件字段和任务回调约束（C1–C3，P1 风险记录） — 改写**

- 原文：[docs/phase6/07-backlog-and-triggers.md](../phase6/07-backlog-and-triggers.md) / ### 3.4 工单域 P2/基建类 / 公式、文件字段和任务回调约束（C1–C3，P1 风险记录）; Phase6 全项目重规划时复核当前可达路径、影响和必要性
- 理由：C1/C2/C3风险性质不同，旧统称假成功会掩盖已有幂等和下载校验；应按可达路径和事务重核。
- 当前条件：C01值位引用/输入隔离，C02写侧对象归属，C03运行重放/终态/记账/fence/unknown分支各交正反例。
- 现存证据：[engineering-execution-design.md](../../docs/design/engineering-execution-design.md) / C01：AST角色/作用域与输入隔离、旧配置扫描; [engineering-execution-design.md](../../docs/design/engineering-execution-design.md) / C02：字段/正文/显式附件引用、用途/归属/GC与删除保护; [cross-repo-execution-contract.md](../../docs/design/cross-repo-execution-contract.md) / C03/R01：提交、效果收据/意图、reassign三类成功含义
- 承接与窗口：C01 (本波开工前; P6.0); C02 (本波开工前; P6.0); C03 (本波开工前; P6.0)
- 出口与未决：旧同名公式/他单key/运行中重放/已成功幂等/记账失败分别分类，不把全部分支先定bug。；真实提供方/参数/配置未定时仍按窗口复核，不称取消或完成。

**P6-T106 · SLA、通知、配置反馈与会话余项（K2/K3/K6、F2/F3/F13/F14/F17/F31/F38） — 改写**

- 原文：[docs/phase6/07-backlog-and-triggers.md](../phase6/07-backlog-and-triggers.md) / ### 3.4 工单域 P2/基建类 / SLA、通知、配置反馈与会话余项（K2/K3/K6、F2/F3/F13/F14/F17/F31/F38）; 所有者 2026-11-11 将可延后事项统一带入 Phase6
- 理由：多种走查余项必须按现存事实拆分，不是“通知包”一个责任人能包全部；旧报告不证明当前main仍有同错。
- 当前条件：D02管SLA入态，D11管改派节流/通知号，U04管配置冲突/风险提示/改图，C05管停用会话，C01管validator引用。
- 现存证据：[business-lifecycle-contracts.md](../../docs/design/business-lifecycle-contracts.md) / §4：转换采集/资格/capabilities/首页责任投影; [business-lifecycle-contracts.md](../../docs/design/business-lifecycle-contracts.md) / §9：事件/通知最小合同及B08阈值配置; [frontend-implementation-contract.md](../../../zhuzhao-ui/docs/design/frontend-implementation-contract.md) / 组件合同、六类工作区、全站映射、已确认UX01/02/03、跨阶段旅程; [engineering-execution-design.md](../../docs/design/engineering-execution-design.md) / C05：撤会话、安全状态与前端恢复合同; [engineering-execution-design.md](../../docs/design/engineering-execution-design.md) / C01：AST角色/作用域与输入隔离、旧配置扫描
- 承接与窗口：D11 (本波开工前; P6.3); D02 (本波开工前; P6.1); U04 (本波开工前; P6.3); C05 (本波开工前; P6.1); C01 (本波开工前; P6.0)
- 出口与未决：每项先重现/反证当前SHA，再相应包交失败恢复；K/F已修部分只回归，不重复开发。；真实提供方/参数/配置未定时仍按窗口复核，不称取消或完成。

**P6-T107 · 审计归档与 utils 补丁承接 — 改写**

- 原文：[docs/phase6/07-backlog-and-triggers.md](../phase6/07-backlog-and-triggers.md) / ### 3.4 工单域 P2/基建类 / 审计归档与 utils 补丁承接; Phase6 全项目重规划时核对运行与依赖现状
- 理由：登录/登出审计已存在，归档默认与真实冷存储是运维效果；utils补丁则是消费者差异核验，不能混成旧升级要求。
- 当前条件：E09首次生产前验归档/备份/恢复与失败可见，R07核2b95538相对当前pin的实际差异和hook启用后决定同步。
- 现存证据：[project-evolution-strategy.md](../../docs/design/project-evolution-strategy.md) / §1–8：保留业务效果、薄实现、先决策与后交付分开; [engineering-execution-design.md](../../docs/design/engineering-execution-design.md) / G03/G04/G06/C01–C05：当前差距、失败反例与固定出口; [s6-s5-debt-ledger.md](../../docs/phase5/s6-s5-debt-ledger.md) / B/C/G/H组及挂账质量判据：历史状态与触发原意
- 承接与窗口：E09 (P6.3出口建立缺口，P7出口复核; P7); R07 (本波开工前; P7)
- 出口与未决：不删被pin远端历史；空缓存消费者构建与包装错误hook反例，归档失败不报就绪；未启用补丁不武断升为漏洞。；真实提供方/参数/配置未定时仍按窗口复核，不称取消或完成。

### 3.5 外部依赖类

**P6-T108 · D-2 M-UAA — 保留**

- 原文：[docs/phase6/07-backlog-and-triggers.md](../phase6/07-backlog-and-triggers.md) / ### 3.5 外部依赖类 / D-2 M-UAA; 内网启动
- 理由：UAA同时管理认证与受管权限；公共规则可先定，真实映射与批准在联调激活前核验，不等待迁移当天。
- 当前条件：E01-UAA-C在P6.0冻结公共管理权/编码/撤权及合成矩阵，P在P6.1交公共port与mock，R从P6.1联调且公司UAA纳管的企业模式首次正式激活前必过；其他real不能替代。
- 现存证据：[17-uaa-integration.md](../../docs/phase3/17-uaa-integration.md) / UAA contract (2026-11-12)，L2/L3仍由数据属主执行; [business-lifecycle-contracts.md](../../docs/design/business-lifecycle-contracts.md) / §7：DirectorySync与离职/撤部; [project-evolution-strategy.md](../../docs/design/project-evolution-strategy.md) / §1–8：保留业务效果、薄实现、先决策与后交付分开
- 承接与窗口: E01 (UAA-C/P6.0；P/P6.1；R/P6.1起联调且公司UAA纳管的企业模式首次正式激活前；其他provider沿原窗口; UAA跨P6.0/P6.1/公司UAA纳管的企业模式首次正式激活前；其他P7)
- 支持/决策关联：D06,X01,X02；共享部分逐项扣重，I02非人主体不替代企业用户接入。
- 出口与未决：中央管辖授权与本地非受管授权区分；通用数据属性满足条件时中央管范围值，本地执行L2/L3；三源撤权、活跃会话和故障新鲜度可验，不自动退回本地放权。真实提供方/参数/配置未定时按窗口复核，不称取消或完成。

**P6-T109 · M-HR（manager_id 轴/离职处置） — 改写**

- 原文：[docs/phase6/07-backlog-and-triggers.md](../phase6/07-backlog-and-triggers.md) / ### 3.5 外部依赖类 / M-HR（manager_id 轴/离职处置）; 内网启动
- 理由：manager/离职是既定风险负责人和安全禁用消费者，不能等“内网启动”才研究；HR具体API仅影响real。
- 当前条件：D06/P6.1先规范目录/完整性/乱序/停用/已批映射交接，E01/X02按provider真实验证。
- 现存证据：[business-lifecycle-contracts.md](../../docs/design/business-lifecycle-contracts.md) / §7：DirectorySync与离职/撤部; [hr-directory-sync.md](../../docs/proposal/hr-directory-sync.md) / 规范化目录/完整性/增量/离职撤部旧方案与新合同优先级; [project-evolution-strategy.md](../../docs/design/project-evolution-strategy.md) / §1–8：保留业务效果、薄实现、先决策与后交付分开
- 承接与窗口：D06 (本波开工前; P6.1)
- 支持/决策关联：E01,X02；不是本源新增独立交付，不重复计价。
- 出口与未决：分页失败不批量离职、同工号不串号、先停登录后逐项交接、撤部plan确认与历史归属保留。；真实提供方/参数/配置未定时仍按窗口复核，不称取消或完成。

**P6-T110 · ~~C-21 任务结果留存/落 activelist~~ C-21+ 任务数据面（输入/输出绑定+result 留存——件① result 列先行） — 保留**

- 原文：[docs/phase6/07-backlog-and-triggers.md](../phase6/07-backlog-and-triggers.md) / ### 3.5 外部依赖类 / ~~C-21 任务结果留存/落 activelist~~ **C-21+ 任务数据面**（输入/输出绑定+result 留存——件① result 列先行）; ~~phase7/8 脚本平台立项~~ **已升格（2026-10-27）+排期建议（2026-10-28 三波；正式拍板随 phase5 合 main）**
- 理由：任务结果/输入输出绑定已明确为核心，不应再等脚本平台；结果持久化与映射权限/体积/来源各有合同。
- 当前条件：D12/P6.3先权威终态与结构化结果、白名单输入输出和工单回写；TR执行历史、AL业务数据与对象工件写主分开。
- 现存证据：[cross-repo-execution-contract.md](../../docs/design/cross-repo-execution-contract.md) / C03/R01：提交、效果收据/意图、reassign三类成功含义; [project-evolution-strategy.md](../../docs/design/project-evolution-strategy.md) / §1–8：保留业务效果、薄实现、先决策与后交付分开; [business-lifecycle-contracts.md](../../docs/design/business-lifecycle-contracts.md) / §2–5/7–11：定义、动作/权限、目录与风险闭环
- 承接与窗口：D12 (本波开工前; P6.3)
- 支持/决策关联：I03；不是本源新增独立交付，不重复计价。
- 出口与未决：running重放不冒成功、终态回调/记账/fence/未知、映射失败不重跑已有效果，日志/对象预算明确。；真实提供方/参数/配置未定时仍按窗口复核，不称取消或完成。

**P6-T111 · 能力目录（taskrunner capability_registry） — 保留**

- 原文：[docs/phase6/07-backlog-and-triggers.md](../phase6/07-backlog-and-triggers.md) / ### 3.5 外部依赖类 / 能力目录（taskrunner capability_registry）; ~~触发驱动~~ **已拍提前提入 phase6（2026-10-29）**
- 理由：能力目录已提前，原三闸门关闭；不过执行路由目录不等于业务授权目录，TR不能代主仓判资源权。
- 当前条件：I01/P7同批完整注册/发现/路由/SDK参考consumer，主仓持业务Schema/策略，TR只路由投影，名称一次定。
- 现存证据：[project-evolution-strategy.md](../../docs/design/project-evolution-strategy.md) / §1–8：保留业务效果、薄实现、先决策与后交付分开; [platform-evolution.md](../../docs/design/platform-evolution.md) / §1/2/4/5/7：专业写主、可靠交接和按窗口冻结; [cross-repo-execution-contract.md](../../docs/design/cross-repo-execution-contract.md) / C03/R01：提交、效果收据/意图、reassign三类成功含义
- 承接与窗口：I01 (本波开工前; P7)
- 支持/决策关联：I02,R07；不是本源新增独立交付，不重复计价。
- 出口与未决：未知版本/未注册/撤销/无权工具拒绝，生成合同不泄secret，旧consumer兼容与真实调用闭环。；真实提供方/参数/配置未定时仍按窗口复核，不称取消或完成。

**P6-T112 · AI 目录数据模型 — 改写**

- 原文：[docs/phase6/07-backlog-and-triggers.md](../phase6/07-backlog-and-triggers.md) / ### 3.5 外部依赖类 / AI 目录数据模型; D8/D10/D17
- 理由：“D8/D10/D17”是旧依赖代号，不能继续当AI数据目录实体已存在；风险数据载体与Agent配置是不同写主。
- 当前条件：A05/P8将已有Agent引用/本地领域配置与权限范围、连接/版本/alias风险定清，A01先固定只读切片，不做模型部署或通用AI目录站。
- 现存证据：[ai-capabilities.md](../../docs/design/ai-capabilities.md) / §1–5：已有模型API、双轨Agent/受权工具/知识/skill边界; [project-evolution-strategy.md](../../docs/design/project-evolution-strategy.md) / §1–8：保留业务效果、薄实现、先决策与后交付分开; [platform-evolution.md](../../docs/design/platform-evolution.md) / §1/2/4/5/7：专业写主、可靠交接和按窗口冻结
- 承接与窗口：A05 (本波开工前; P8)
- 支持/决策关联：A01,A07；不是本源新增独立交付，不重复计价。
- 出口与未决：配置发布不授执行权，未知外部版本/模型能力明确；领域host必要性与skill样包另补，不用包名销功能。；真实提供方/参数/配置未定时仍按窗口复核，不称取消或完成。

**P6-T113 · C-7 附件独立权限码 — 改写**

- 原文：[docs/phase6/07-backlog-and-triggers.md](../phase6/07-backlog-and-triggers.md) / ### 3.5 外部依赖类 / C-7 附件独立权限码; 权限细化诉求
- 理由：当前附件update资源授权有理由，已知主要缺口是前端按单能力；没有明确细粒度职责不应先添独立权限码。
- 当前条件：E02/P6.3比较现capability与是否存在仅上传/仅删/仅下载职责；保留当前写模型，确证需要再提受限purpose/动作码。
- 现存证据：[business-lifecycle-contracts.md](../../docs/design/business-lifecycle-contracts.md) / §4：转换采集/资格/capabilities/首页责任投影; [engineering-execution-design.md](../../docs/design/engineering-execution-design.md) / C02：字段/正文/显式附件引用、用途/归属/GC与删除保护; [s6-s5-debt-ledger.md](../../docs/phase5/s6-s5-debt-ledger.md) / B/C/G/H组及挂账质量判据：历史状态与触发原意
- 承接与窗口：E02 (P6.3出口; P7)
- 共用依据：P6-T084；本源原意和条件仍单独保留。
- 出口与未决：透明读不显示写钮，直接API仍拒；不同用途/不同票/撤权回归，不能因UI可见错误削弱授权。；真实提供方/参数/配置未定时仍按窗口复核，不称取消或完成。

**P6-T114 · C-8 BK-12 残留 — 改写**

- 原文：[docs/phase6/07-backlog-and-triggers.md](../phase6/07-backlog-and-triggers.md) / ### 3.5 外部依赖类 / C-8 BK-12 残留; 孤儿绑定实际危害
- 理由：“出现孤儿绑定实际危害”对组织权限完整性过晚；当前C04已提出双边锁与org_roles删除守卫，不需等事故。
- 当前条件：C04/P6.1先只读盘历史软删/孤儿，删除与BindOrgRole共享存续锁；有绑定显式冲突并保留审计。
- 现存证据：[engineering-execution-design.md](../../docs/design/engineering-execution-design.md) / C04：DBTX试点、org_roles删除双方锁、默认拒绝; [business-lifecycle-contracts.md](../../docs/design/business-lifecycle-contracts.md) / §5：类型查看/创建/参与管理和风险组职责; [business-lifecycle-contracts.md](../../docs/design/business-lifecycle-contracts.md) / §7：DirectorySync与离职/撤部
- 承接与窗口：C04 (本波开工前; P6.1)
- 支持/决策关联：D06；不是本源新增独立交付，不重复计价。
- 共用依据：P6-T043；本源原意和条件仍单独保留。
- 出口与未决：并发删/绑最多一成，虚拟组同策略；停组派生角色过滤与其他权源保留由D06/C04联验。；真实提供方/参数/配置未定时仍按窗口复核，不称取消或完成。

**P6-T115 · C-9 spec 收件人/渲染引擎 — 改写**

- 原文：[docs/phase6/07-backlog-and-triggers.md](../phase6/07-backlog-and-triggers.md) / ### 3.5 外部依赖类 / C-9 spec 收件人/渲染引擎; ≥2 来源组合
- 理由：“≥2来源”不是引入recipients_spec或form-create的充分条件；收件解析与渲染器是两种独立成本。
- 当前条件：D11/D15仅具名owner/负责人解析，U04开工比较现有渲染与成熟库的有限能力；不先造通用表达式收件人或低代码。
- 现存证据：[business-lifecycle-contracts.md](../../docs/design/business-lifecycle-contracts.md) / §9：事件/通知最小合同及B08阈值配置; [frontend-implementation-contract.md](../../../zhuzhao-ui/docs/design/frontend-implementation-contract.md) / 组件合同、六类工作区、全站映射、已确认UX01/02/03、跨阶段旅程; [business-lifecycle-contracts.md](../../docs/design/business-lifecycle-contracts.md) / §6：schema求值、约束与Query/prefill
- 承接与窗口：D11 (本波开工前; P6.3); U04 (本波开工前; P6.3)
- 支持/决策关联：D15；不是本源新增独立交付，不重复计价。
- 出口与未决：两个域正反例、变量来源授权/缺值与渲染往返/禁用只读；供给不足列缺口而非无限注册。；真实提供方/参数/配置未定时仍按窗口复核，不称取消或完成。

**P6-T116 · C-4 BK-19 handler 测试 — 改写**

- 原文：[docs/phase6/07-backlog-and-triggers.md](../phase6/07-backlog-and-triggers.md) / ### 3.5 外部依赖类 / C-4 BK-19 handler 测试; 工单封版翻案
- 理由：工单自研恢复的原触发早已发生，不能继续“等翻案”；现有handler测试存在也不代表全层系统覆盖。
- 当前条件：G04/P6.1以当前路由/高风险DTO/绑定/鉴权/错误映射列具体缺口，优先负例而非追求全部镜像测试。
- 现存证据：[engineering-execution-design.md](../../docs/design/engineering-execution-design.md) / G03/G04/G06/C01–C05：当前差距、失败反例与固定出口; [business-lifecycle-contracts.md](../../docs/design/business-lifecycle-contracts.md) / §5：类型查看/创建/参与管理和风险组职责; [business-lifecycle-contracts.md](../../docs/design/business-lifecycle-contracts.md) / §6：schema求值、约束与Query/prefill
- 承接与窗口：G04 (本波开工前; P6.1)
- 出口与未决：HTTP元素dive/非法字段/404同形/403/409/分页导出边界可定位；已覆盖不重写，未测明确计口径。；真实提供方/参数/配置未定时仍按窗口复核，不称取消或完成。

### §4.2

**CARRY-TASKUX · 任务控制台分类两栏/执行页参数历史/定义分权/AL深链 — 保留**

- 原文：[docs/phase6/07-backlog-and-triggers.md](../phase6/07-backlog-and-triggers.md) / §4.2 / 任务控制台分类两栏/执行页参数历史/定义分权/AL深链; - [phase5/04 号 §3.7](../phase5/04-s4-task-console-ux.md) **任务控制台 UX 二期**（2026-10-28 设计定形——分类任务库两栏/执行页左参数右运行历史/定义管理分权〔task:manage 专属〕/AL 绑定深链/日志导出；四子菜单迁移；执行详情区依赖 C-21+ 件①；分期批1 UI 主批+菜单迁移/批2 category 列/批3 随件①）。
- 理由：任务库分类两栏/执行参数历史/定义管理分权/AL深链是明确用户旅程；不因执行详情等D12结果而把已有任务页改造无限后置。
- 当前条件：U06/P6.3先分类/执行/管理入口与task:manage、AL深链；D12同批结果合同，恢复策略新控件归I03/P7，不生成可保存假字段。
- 现存证据：[04-s4-task-console-ux.md](../../docs/phase5/04-s4-task-console-ux.md) / §3.7任务控制台UX二期与执行详情依赖; [frontend-implementation-contract.md](../../../zhuzhao-ui/docs/design/frontend-implementation-contract.md) / 组件合同、六类工作区、全站映射、已确认UX01/02/03、跨阶段旅程; [cross-repo-execution-contract.md](../../docs/design/cross-repo-execution-contract.md) / C03/R01：提交、效果收据/意图、reassign三类成功含义
- 承接与窗口：U06 (本波开工前; P6.3); D12 (本波开工前; P6.3); I02 (本波开工前; P7)
- 支持/决策关联：U02；不是本源新增独立交付，不重复计价。
- 出口与未决：执行权限不凭role名、只读运行不获定义管理；参数/历史/结果同运行ID，失败/unknown/深链过滤可回查。；真实提供方/参数/配置未定时仍按窗口复核，不称取消或完成。

**CARRY-CROSSMETRICS · 跨模块指标目录与AL类型条数/任务定义调用数 — 改写**

- 原文：[docs/phase6/07-backlog-and-triggers.md](../phase6/07-backlog-and-triggers.md) / §4.2 / 跨模块指标目录与AL类型条数/任务定义调用数; - [phase6/04 号附录 A](../phase6/04-r4-metrics-reports.md) **跨模块指标目录**（2026-10-28 登记不排期——七域计数/指标盘点三档分级；★点名 = AL 每类型数据条数 + 任务定义级调用计数；实施随各域小批，DSL 立项时收编词表）。
- 理由：跨模块目录原“登记不排期/随域小批”会留下无人交付的计数；AL类型条数和任务定义调用数是点名能力，不应只存在总目录。
- 当前条件：D13/P6.E冻结共同指标ID/分母/授权/时间；AL事实由R06/P7、任务事实由I03/P7交，UI消费标来源，不提前把P7事实当P6已实现。
- 现存证据：[04-r4-metrics-reports.md](../../docs/phase6/04-r4-metrics-reports.md) / §3/4/7/附录A：固定统计、快照、报告及跨模块限制; [project-evolution-strategy.md](../../docs/design/project-evolution-strategy.md) / §1–8：保留业务效果、薄实现、先决策与后交付分开; [cross-repo-execution-contract.md](../../docs/design/cross-repo-execution-contract.md) / C03/R01：提交、效果收据/意图、reassign三类成功含义
- 承接与窗口：D13 (本波开工前; P6.E); I03 (本波开工前; P7); R06 (本波开工前; P7)
- 出口与未决：AL类型条数按真实类型scope，任务定义调用数不是运行状态数；无total/游标源不伪总数，历史缺口与schema版本可追。；真实提供方/参数/配置未定时仍按窗口复核，不称取消或完成。

### entries[source_id=CONT-ENG-06]

**CONT-ENG-06 · E01真实接入需有逐提供方交付子包，不能以契约裁决关闭功能 — 改写**

- 原文：[docs/review/planning-source-map.json](planning-source-map.json) / entries[source_id=CONT-ENG-06]; 跨行审计补登记; P6-T009,P6-T056,P6-T059,P6-T060,P6-T061,P6-T108,P6-T109
- 理由：这是后来追加的跨行审计承接，不是原表第117个触发；指出“核契约就把功能关账”不合理，真实接入效果必须有逐provider交付。
- 当前条件：其他provider从P6.1核合同并P7至少一real；UAA另有C/P/R跨阶段窗口，R为企业模式首次正式激活硬出口，不能用其他real代验。C新增2–3日，P/R未计价待拆重复估。
- 现存证据：[project-evolution-strategy.md](../../docs/design/project-evolution-strategy.md) / §1–8：保留业务效果、薄实现、先决策与后交付分开; [platform-evolution.md](../../docs/design/platform-evolution.md) / §1/2/4/5/7：专业写主、可靠交接和按窗口冻结; [hr-directory-sync.md](../../docs/proposal/hr-directory-sync.md) / 规范化目录/完整性/增量/离职撤部旧方案与新合同优先级; [17-uaa-integration.md](../../docs/phase3/17-uaa-integration.md) / UAA contract (2026-11-12)，L2/L3仍由数据属主执行; [business-lifecycle-contracts.md](../../docs/design/business-lifecycle-contracts.md) / §9：事件/通知最小合同及B08阈值配置
- 承接与窗口: E01 (UAA-C/P6.0；P/P6.1；R/P6.1起联调且公司UAA纳管的企业模式首次正式激活前；其他provider沿原窗口; UAA跨P6.0/P6.1/公司UAA纳管的企业模式首次正式激活前；其他P7)
- 支持/决策关联：X02,D06,D11；不是本源新增独立交付，不重复计价。
- 出口与未决：mock/contract/real分别报；每provider须保留具体功能效果与失败/恢复回执，未知API不降mock成功，不以E01调查完成代销接入。UAA真实验收必须单列且早于企业模式首次正式激活上线，不因别的provider成功关闭。真实提供方/参数/配置未定时按窗口复核，不称取消或完成。

## 来源归属同步与开工范围冻结事项

以下区分当前必须校准的来源归属，与已有多包承接在开工前仍需细分的范围。P6-T045已由root同步；P6-T033/P6-T083补来源角色分配。P6-T065已有五包related保持，不要求为了汇总美观抹平多包。其余项保留具体出口与子估算要求，不把支持依赖一律计成新增任务。

### P6-T045

- 旧→新：E04 → U06; Cmd+K属于前端查找入口，E04保留旧映射追溯。
- 来源映射：主归属改U06，U02/P6.1做价值与权限裁决，U06/P6.3交获选入口。
- 总账/范围：U02明示入口候选逐项裁决；U06接获选Cmd+K，不全部等待报告包。
- 估算影响：旧2–3天只作历史。获选需U06独立子估算，不能默认塞入4–7天或E04 6–10天。

### P6-T033

- 旧→新：D14 → D13,D14; risk_id实体投影归D13，SOC事件协作归D14，共用呈现不混写主。
- 来源映射：按风险/票/通知/任务关联投影与SOC事件时间线分别列D13/D14。
- 总账/范围：D13显式保留本地实体时间线，D14保SOC证据和本地协作。
- 估算影响：旧1天不能证明已含跨域授权/缺失事件/关联查询；D13开工补有限子估算，与D14组件去重。

### P6-T065,P6-T100

- 旧→新：E05（065）/D04（100） → U03,U04,D04,D11,E05,D03; P2混合行按子效果分配；完整常驻字段ACL不同于collect_fields/visible_when。
- 来源映射：065拆watcher→U03、常驻字段ACL→D04、predue/digest→D11、类型克隆→U04、发起范围→D03、软删/服务端草稿→E05，其余小件按U03入口裁决；100共用字段ACL裁定。
- 总账/范围：已有watcher/类型克隆/predue/digest仅明确映射。D04/P6.1冻结最小常驻字段读写场景/优先级，获选有限实现建议P6.3独立子窗；完整多轴矩阵须另审范围，不能无期限挂账。
- 估算影响：完整状态×字段×角色ACL不在D04当前3–5天明确scope中，须子估算/窗口；不重复计算已列U03/U04/D11的效果。

### P6-T083

- 旧→新：U01 → U01,U02,U06,D10; 安全入口、普通责任合同、三域页面、R3复核责任分开承接。
- 来源映射：U01/P6.0安全入口，U02/P6.1普通责任与IA，U06/P6.3三域页，D10/P6.2复核投影。
- 总账/范围：当前这些包已承接确认方向，此处补来源分配；不新增重复工作包。
- 估算影响：不因归属纠正自动新增工期，但旧纯前端1–2天不能沿用为全部scope估算。

### P6-T019

- 旧→新：U03 → U03; U03仍为前后端检索效果owner；字段语义/授权/性能为支持，不拆重复功能。
- 来源映射：明确custom_data首组字段白名单、操作符和后端过滤/索引预算，不只写组合筛选。
- 总账/范围：U03开工冻结最小类型化查询，未选任意逻辑树不取消最小检索效果。
- 估算影响：任意custom_data检索/值表可能大增量，不能默认含U03 4–7天；首组字段需子估算。

### P6-T091,P6-T092

- 旧→新：D04 → D04,U04; D04管元素/求值/旧数据语义，U04管结构化配置/预览/布局，消费页同合同接。
- 来源映射：增加U04关联及语义/配置表现分项验收。
- 总账/范围：U04当前已列件19–25与旧数据兼容，仅校准来源，不暗批复杂自由二维布局。
- 估算影响：已列有限scope不重复计价，复杂自由布局另裁决。

### P6-T103

- 旧→新：D02 → D02,U03; capabilities/转换事件在D02，详情活动流表现由U03；D12任务结果不能代替ticket_events payload。
- 来源映射：加U03关联，按件6/17/23拆验收。
- 总账/范围：D02明确转换事件/字段真值diff，U03明示活动流/能力消费，可靠intent仍R01计一次。
- 估算影响：已有详情范围不重计；真值diff若未包含当前D02估算，须开工给子估算，不能沿旧1天默含。

### P6-T063

- 旧→新：R06 → R06,I04; AL事实producer与主仓受权消费分开，不用当前R06并发/导入scope代销事件。
- 来源映射：R06保主，I04接主仓摄入；先一类型/一变更场景，替代方案须保效果。
- 总账/范围：R06/I04加信封/世代/幂等/恢复参考链，是否需AL端持久发出及跨仓可靠边界在开工冻结。
- 估算影响：R06 5–8天未显式列事件producer，I04代表源也不默认含它；持久producer需独立子估算。

## 与执行台账的接续

本文不重估71包，不授权新增实现。JSON中的integration_changes给需root同步的来源、包、窗口及估算影响；其余额外包只是支持/决策关联。保留功能的实际交付仍需对应单元/合同/真实小栈/界面证据，真实API未知只阻其real放行。

本次静态核验要求是119来源集合相等、116原表标题与原文hash匹配、每源有裁定/理由/证据/条件/包/窗口/出口、去重无悬挂和所有证据文件可解析。本轮没有启动服务或运行全量门禁，未标业务实现完成。
