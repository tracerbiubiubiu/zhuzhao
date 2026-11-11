# 全会话关注点研究深度核验（2026-11-12）

## 结论

**不能认定所有关注点都已深入研究。** 本次从完整会话重新提取32条真实用户输入，将原始要求、后续补充、纠正及明确确认拆成65个核对原子：51项有实质规划研究证据，10项仍部分深入，4项因真实提供方或运行条件未知而合理后置。未发现完全没有承接的关注点；“被纳入、已排期、已确认”不能代替研究深度。

65不是65项独立功能，也不是新的工作包数量。51项表示已有现状证据、具体边界与可审方案，**不表示51项已实现或所有后期详设已冻结**。上一轮“完成”应理解为规划材料交付；若将它理解为所有关注点等深研究完毕，本次证据不支持。

[逐项机器索引](../focus-depth-verification.json)保留每点短原话、原消息ID、实际证据文件/小节或符号、具体已研究内容、不足与对应补研究项；完整会话、独立审读过程和原截图仅在本地ignored证据目录保存。

## 方法与范围

- 从当前聊天的三页消息读取到hasMore=false，取32条真实用户消息，排除自动goal续跑；65点覆盖全部32条。用户简答“可以”与此前明确的两项建议绑定，避免将短答扩大为全部规划批准。
- 重新核验现行五仓规划快照：主仓b2d7f099、UI1fc697d5、TR44f6b644、ALa34d6603、utils e2cf85a1。工程、产品/AI/adapter、前端/业务三路独立审读，根审查原路线、旧比较、触发挂账及来源完整性。
- 先看实际设计正文及必要源码、CI、测试配置/原型证据，再判现状核实、选择理由、具体首期、权限/失败/恢复边界与验收窗口。不以270份库存、510来源映射、169原子或绿色门禁直接推定深入。
- 本次没有启动服务、改业务代码或重跑旧测试。旧253项验收/42项E2E证明对应固定实现，不证明新增规划、全站视觉或全部关注点研究完整。
- 判定“合理后置”需要有具体边界和复核窗口，且确实依赖真实提供方或运行条件；可用合成场景先完成的本地产品研究，不因真实API未知就自动豁免。

## 已有研究较扎实的部分

工程G01–G07并非空泛建议：有五仓实际CI/工具差距、同文件共享索引局限、worktree/环境归属、组合清单、分档门禁准入/排除、覆盖率限定口径、风险导向fuzz、目录语言profile、版本和恢复窗口。尚未实施的保护/工具不被说成已生效。

业务合同对上传追加/主动删除、类型查看范围、CC与处理权、转换事务与独立后继、调度错失策略/有界恢复、风险组受限派单、离职与撤部都有实质规则、失败反例及验收出口。26项方向确认的来源闭合。

私有扩展和内网adapter已具体到一次性公共接缝、独立私有仓/服务、manifest/目录/网关、同步身份、版本协商/消费者矩阵、归一化/水位/影子切换/回退。未承诺任意未来版本永不变；未部署阶段的六个月保留方案已按用户决定退出。

全路线承接有实际取舍：保留手动运行、一次性/延迟、固定报告、通报和持久闭环等原业务效果，允许专业供给/更小实现替代重型自研，不将AI入口代替原能力。模块化单体继续是有效形态。

## 深度不足的10项：合并为8组补研究

以下只记录证据不足与建议补研究出口，**未先补完再回称“原来已研究”**。建议窗口关联现有包，不擅自宣布新增研究已包含在原粗估内；执行时需按实际范围复估。

### FD01 当前模块耦合缺事实与逐项裁定

涉及：C07、C15。

ticket.Service持OrgRepo/StorageRepo，UserService持RoleRepo，OrgRepo.Move写工单/模板；架构§3.5.4禁止类似跨域仓储依赖。

**不足**：缺实际依赖、共享写表/事务及保留、例外或窄端口的逐项理由；这些耦合不自动是bug，P9拆分复评不能替代当前检查。

**补研究出口**：模块×调用/共享写表清单，关联源符号和事务所有者；逐项给保留/例外/隔离与最小改动。

**建议窗口**：当前可补；最迟P6.0边界冻结及G06/X01改边界前；关联G06、X01、E07。

### FD02 旧触发项逐条合理性证据不足

涉及：C11。

119条P6-T/触发索引均有去向；169连续性原子中仅3条直接来自07触发索引。映射字段不含逐条合理性结论。

**不足**：包名、波次或总行数不能证明每条旧触发描述均深入复核；这不等于断言其它116条从未读过。

**补研究出口**：按119条去重回读原文，给保留/改写/专业替代/退出理由、证据与决策/交付窗口；同源允许共用理由。

**建议窗口**：当前可补；最迟对应P6.1/P6.3裁决窗口前；关联E01、E02、E03、E04、E05、E06、E07、E08、E09、E10。

### FD03 领域Agent缺具体必要性与完整代表样例

涉及：C23。

已有双轨配置、host/预算/确认/恢复边界和多种复用路径，但未用同一业务场景比较。

**不足**：缺证明必须Agent循环而非固定步骤的首场景，缺输入/工具/暂停/结果/恢复样例；未知模型API不阻塞本地场景研究。

**补研究出口**：一个合成平台独有场景，对比外部Agent、固定步骤和本地host，给工具序列、成本、终止与恢复样例。

**建议窗口**：P7比较时，先于A05/A07的P8范围冻结；关联A05、A07。

### FD04 自然语言计划与逐次确认未贯通

涉及：C26。

调度恢复已细；原型定时生成待确认草案，A04又要持续grant。

**不足**：离线、确认逾期、多个时隙积压、计划/权限改变、动作TTL与misfire/取消尚无完整衔接样例。

**补研究出口**：同一计划跨多个时隙的确认/过期/撤权/积压/取消状态表、回执与界面反馈，区分草案和已授权无人值守执行。

**建议窗口**：A04/I02/I03合同冻结前，可先用合成数据；关联A04、I02、I03。

### FD05 MCP首工具、授权衔接与消费窗口不足

涉及：C27。

规范/SDK引用有效，已有协议adapter与授权交集原则；总账A03/P7含外部消费，AI总纲§3把client放P8。

**不足**：缺首工具I/O/错误/transport与SDK-client组合、适用授权服务到现有IAM的具体取舍；外部消费缺代表工具且窗口冲突。

**补研究出口**：发布及被消费工具各一份合同/合成兼容例，明确授权路径，统一P7/P8范围后复估。

**建议窗口**：P6.0最小合同冻结前明确授权路径，P7/A03开工前统一范围；关联A03、I01、I02、A05。

### FD06 skill缺可信样包与暂停运行兼容例

涉及：C29。

已有来源、版本、信任、允许工具和skill不赋权原则。

**不足**：缺实际样包、输入输出/资产/评测，加载冲突、撤销升级对暂停run的影响只留到后期。

**补研究出口**：一个版本化领域skill样包及输入/预期输出/工具，覆盖加载、撤销、升级和恢复。

**建议窗口**：领域Agent场景选定后，A07/P8开工前；关联A07。

### FD07 全站视觉研究仍为静态初审与代表样板

涉及：C41、C48。

27路由有静态清单；六类HTML样板中四类对应现存home/list/detail/intel，另两类为未来Agent/设备；亮色tokens/声明对比度不等于生产全站实测。

**不足**：配置/树/任务/AL/登录等未有同等页级视觉研究，缺真实亮暗/字体/字号/比例/密度、长内容和交互状态证据。

**补研究出口**：27视图及共享组件的角色×亮暗×视口/缩放×状态走查；逐页截图、实际样式尺寸、问题或保留理由及目标映射。

**建议窗口**：U01复现入口，U02/U07建真实基线，U06改各页前补对应研究；关联U01、U02、U06、U07。

### FD08 SOC事件本地协作职责授权缺矩阵

涉及：C61。

风险记录/整改票/SOC攻击应急事件已区分；B05不因入组授予全部事件能力，D14主要仍为范围与供给。

**不足**：缺本地事件协作角色×动作×数据范围/参与/撤权矩阵；SOC研判供给不替代本地操作授权。

**补研究出口**：薄事件协作矩阵及正反例，分清引用、协作更新、指派、结案和回传与专业写主，不造第二SOC。

**建议窗口**：D14/P6.E开工前，可先不依赖真实SOC地址；关联D14、D03、C04。

## 四项合理后置

- **C16 服务拆分**：先补当前耦合清单FD01，再以独立发布、容量或故障隔离证据裁决拆分；未部署不虚构生产负载。
- **C22 真实企业Agent合同**：连接/配置边界已定，具体版本、身份、运行和权限映射以实际企业服务为准；不以此掩盖本地域Agent的FD03。
- **C28 真实知识ACL/RAG合同**：已有实时权限交集、引用/版本/撤权设计，真实服务检索和授权语义未验；不默认自建知识平台。
- **C33 逐provider真实能力**：用户确认已有平台类别，具体API/粒度/回传能力未知。已有私有connector、mock/contract/real与逐源窗口，不等于真实接入已成立。

这些后置项保留原研究边界和复核窗，不应当作完全未研究，也不能改称已完成真实验证。

## 本次查出的现状措辞错误

旧首轮报告ENG-20写“AI/DataFetcher等已有端口/注册表”。本次核源码并对照同报告PROD-07，已有的是jobs/gateway/DataFetcher；AI/LLMProvider尚未实现。该句在本轮纠正，原句及基线保留在机器索引和本地收据；不将MCP、AI host或插件机制改写成现成能力。

另有派生可运维性建议：AI未配置/全部停用时，启动、能力发现、在途任务与回手工的端到端行为可进一步研究。它不是用户原话“无需部署模型”的同义词，未计入65点或用户缺口数量。

## 全部65点逐项核验

主证据给文件及小节/符号；完整证据与研究内容见机器索引。“实质规划”仍不代表实现或生产实测。

| ID | 用户关注点 | 深度判定 | 主证据或补研究 |
|---|---|---|---|
| C01 | 五仓工程化、CI及自动检查缺口 | 实质规划 | [engineering-execution-design.md · 已有底座及本次差距；G02；G03候选manifest/作业集合和资源预算](../../design/engineering-execution-design.md)；[engineering-governance.md · §1基线判断、§3 CI与合入、§7验收与未决](../../design/engineering-governance.md) |
| C02 | 并行agent误带他人修改提交及同文件隔离 | 实质规划 | [engineering-execution-design.md · G01任务、工作树与运行资源独占](../../design/engineering-execution-design.md)；[engineering-governance.md · §2并行协作：隔离先行](../../design/engineering-governance.md) |
| C03 | 正规项目差距，不能只回答用户举例 | 实质规划 | [pre-phase6-replanning-2026-11-11.md · ENG-02/04/06/07/08/19/20](pre-phase6-replanning-2026-11-11.md)；[engineering-governance.md · §3合入、§4测试、§5供应链/发布/运维、§6目录](../../design/engineering-governance.md) |
| C04 | 测试覆盖率的现态、口径与改进价值 | 实质规划 | [engineering-execution-design.md · G04 mock、正确性测试与第一批fuzz；C01/C02/C04/C05出口](../../design/engineering-execution-design.md)；[Makefile · test-unit/test-integration/test-cover](../../../Makefile) |
| C05 | 关键节点fuzz的必要性、目标与预算 | 实质规划 | [engineering-execution-design.md · G04五个首批target、性质、PR/每周预算、corpus保留](../../design/engineering-execution-design.md)；[pre-phase6-replanning-2026-11-11.md · ENG-07四Go仓无顶层Fuzz/Benchmark](pre-phase6-replanning-2026-11-11.md) |
| C06 | 项目定位、设定和边界清晰度 | 实质规划 | [vision.md · 现行补充、§1.0平台定位声明](../../vision.md)；[platform-evolution.md · §1首要目标、§2能力供给与数据写主](../../design/platform-evolution.md) |
| C07 | 逐模块深入检查现有能力与设计边界 | 部分深入 | **FD01**；[pre-phase6-replanning-2026-11-11.md · §2](pre-phase6-replanning-2026-11-11.md)；[architecture.md · §3.2–3.5](../../design/architecture.md) |
| C08 | 整体目标、优先级与后续规划清晰度 | 实质规划 | [roadmap.md · §1–7](../../roadmap.md)；[project-evolution-strategy.md · §1–8](../../design/project-evolution-strategy.md) |
| C09 | Ficus及其它项目比较是否实质用于本轮 | 实质规划 | [planning-source-map.json · CMP-30/HIST-81 entries](../planning-source-map.json)；[roadmap-capability-continuity.json · 169 records](../roadmap-capability-continuity.json) |
| C10 | 执行后自行审计、证据与不确定项披露 | 实质规划 | [pre-phase6-requirements-audit-2026-11-11.md · §2/§5–12](pre-phase6-requirements-audit-2026-11-11.md)；[planning-design-status.json](../planning-design-status.json) |
| C11 | 旧触发条件和挂账描述本身是否合理 | 部分深入 | **FD02**；[07-backlog-and-triggers.md · §3](../../phase6/07-backlog-and-triggers.md)；[planning-source-map.json · 119 P6-T/trigger entries](../planning-source-map.json) |
| C12 | 全部挂账排期，不能无限触发后再说 | 实质规划 | [planning-source-map.json · P6-T entries](../planning-source-map.json)；[planning-backlog.md · §3–5](../planning-backlog.md) |
| C13 | 五仓目录缺少哪些必要目录或模块 | 实质规划 | [pre-phase6-replanning-2026-11-11.md · ENG-19实际目录/迁移方式差异](pre-phase6-replanning-2026-11-11.md)；[engineering-governance.md · §6目录与新仓一致性](../../design/engineering-governance.md) |
| C14 | 五仓及新增仓目录/文档/代码风格一致性 | 实质规划 | [engineering-execution-design.md · G06 policy-bundle.lock；G06语言与机械文件profile](../../design/engineering-execution-design.md)；[standards.md · §4语言/依赖方向/命名，生态SSOT](../../standards.md) |
| C15 | 现有模块实际耦合与架构调整必要性 | 部分深入 | **FD01**；[platform-evolution.md · §3五仓/模块写主、§6先消除跨域直查直写](../../design/platform-evolution.md)；[architecture.md · §3.2/3.3职责和依赖方向；§3.5.4跨域调用规则](../../design/architecture.md) |
| C16 | 服务拆分与微服务的渐进演进条件 | 合理后置 | [platform-evolution.md · §6可插拔与服务拆分](../../design/platform-evolution.md)；[project-evolution-strategy.md · §1怎样选择；§6 E07运行证据裁决；§8增量出口](../../design/project-evolution-strategy.md) |
| C17 | 内部模块可插拔，尽量不改主仓 | 实质规划 | [extension-compatibility.md · §1现状、§2四种扩展、§3一次性接缝、§4授权、§7出口](../../design/extension-compatibility.md)；[wire.go · 现行编译装配](../../../internal/app/wire.go) |
| C18 | 核心升级与旧私有消费者接口稳定/适配责任 | 实质规划 | [extension-compatibility.md · §1承诺边界、§5版本、§6部署前后矩阵](../../design/extension-compatibility.md)；[platform-contracts.md · §4公共版本/消费者写主](../../../../zhuzhao-utils/docs/api/platform-contracts.md) |
| C19 | mock迁入内网：HR/风险adapter隔离真实内部代码 | 实质规划 | [intranet-adapter-migration.md · §1–7](../../design/intranet-adapter-migration.md)；[business-lifecycle-contracts.md · §6.3 Query、§7 DirectorySync DTO/状态、§8发现来源](../../design/business-lifecycle-contracts.md) |
| C20 | 大量mock接口长期保留 | 实质规划 | [platform-evolution.md · §1保留前提](../../design/platform-evolution.md)；[intranet-adapter-migration.md · 开头、§1共同契约、§6/§7逐源验证](../../design/intranet-adapter-migration.md) |
| C21 | 已有模型API接入，内网仅内部模型，无模型部署 | 实质规划 | [ai-capabilities.md · §1.1模型连接](../../design/ai-capabilities.md)；[ai-invocation-contract.md · §2Provider与数据区、§5预算、§7分层验收](../../design/ai-invocation-contract.md) |
| C22 | 智能体配置与已有Agent/平台接入 | 合理后置 | [ai-capabilities.md · §1.2接入配置/版本/alias风险、§1.4候选比较](../../design/ai-capabilities.md)；[ai-invocation-contract.md · §2对象、§4身份、§5外部run与unknown、§7真实待验](../../design/ai-invocation-contract.md) |
| C23 | 基于平台特有能力自建领域Agent | 部分深入 | **FD03**；[ai-capabilities.md · §1.2–1.4、§5](../../design/ai-capabilities.md)；[ai-invocation-contract.md · §1后期领域host、§5恢复、§7剩余](../../design/ai-invocation-contract.md) |
| C24 | 自然语言查数据 | 实质规划 | [ai-capabilities.md · §2.1](../../design/ai-capabilities.md)；[ai-invocation-contract.md · §1/A01、§3Query catalog、§5/§7](../../design/ai-invocation-contract.md) |
| C25 | 自然语言下发工单 | 实质规划 | [ai-capabilities.md · §2.2](../../design/ai-capabilities.md)；[ai-invocation-contract.md · §1/A02、§3逻辑operations、§4精确确认](../../design/ai-invocation-contract.md) |
| C26 | 自然语言计划任务的完整用户旅程 | 部分深入 | **FD04**；[ai-capabilities.md · §2.3计划任务](../../design/ai-capabilities.md)；[ai-invocation-contract.md · §3/schedule.preview/confirm、§4默认逐次确认、§5等待/取消](../../design/ai-invocation-contract.md) |
| C27 | MCP接口改造与能力暴露/消费边界 | 部分深入 | **FD05**；[ai-capabilities.md · §3 MCP/Action/身份委托、§7官方依据](../../design/ai-capabilities.md)；[ai-invocation-contract.md · §3协议adapter、§4授权、§7尚无conformance](../../design/ai-invocation-contract.md) |
| C28 | RAG的接入方式与已有知识平台复用 | 合理后置 | [ai-capabilities.md · §4.1 RAG接入既有知识平台](../../design/ai-capabilities.md)；[ai-invocation-contract.md · §6知识、§7验收](../../design/ai-invocation-contract.md) |
| C29 | skill的角色、版本和执行权限边界 | 部分深入 | **FD06**；[ai-capabilities.md · §4.2 skill、§7官方格式](../../design/ai-capabilities.md)；[ai-invocation-contract.md · §6知识/skill](../../design/ai-invocation-contract.md) |
| C30 | 工作流、Agent流程、工单流程能否统一 | 实质规划 | [ai-capabilities.md · §5三类语义与取舍](../../design/ai-capabilities.md)；[platform-evolution.md · §4流程统一与§5可靠性](../../design/platform-evolution.md) |
| C31 | AI原生演进能承接现有底座而非AI页面孤岛 | 实质规划 | [ai-capabilities.md · §2三个切片、§3Action合同、§5三种流程](../../design/ai-capabilities.md)；[project-evolution-strategy.md · §3/U08、I01、§5/A05–A07](../../design/project-evolution-strategy.md) |
| C32 | 数据聚合与安全全生命周期，专业平台优先不造大而全 | 实质规划 | [vision.md · 现行补充、§1.0平台定位声明](../../vision.md)；[platform-evolution.md · §1首要目标、§2能力供给与数据写主](../../design/platform-evolution.md) |
| C33 | 已有专业供给与未知真实API的核验边界 | 合理后置 | [platform-evolution.md · §2供给与写主表、§4流程统一、§6可插拔](../../design/platform-evolution.md)；[project-evolution-strategy.md · §3/U08、§5/R03/R04/E04/E06、§6/R02/R05](../../design/project-evolution-strategy.md) |
| C34 | 内部设备台账模块边界，不重复CMDB | 实质规划 | [device-module-contract.md · §1/2/5](../../design/device-module-contract.md)；[frontend-implementation-contract.md · 设备投影与管理入口](../../../../zhuzhao-ui/docs/design/frontend-implementation-contract.md) |
| C35 | 设备状态大屏/仪表盘与来源时效 | 实质规划 | [device-module-contract.md · §1/2/5](../../design/device-module-contract.md)；[frontend-implementation-contract.md · 设备投影与管理入口](../../../../zhuzhao-ui/docs/design/frontend-implementation-contract.md) |
| C36 | 多厂商设备无SSO，首期目录/人工登录入口 | 实质规划 | [device-module-contract.md · §3/4](../../design/device-module-contract.md)；[frontend-implementation-contract.md · 设备投影与管理入口](../../../../zhuzhao-ui/docs/design/frontend-implementation-contract.md) |
| C37 | Go/Node/PG/Redis/容器/开源组件升级，自行打包不再公司对齐 | 实质规划 | [dependency-modernization.md · §1候选与官方支持周期、§2单源版本、§3分批矩阵、§4数据真实回退、§5离线分发](../../design/dependency-modernization.md)；[engineering-execution-design.md · G03自行打包版本与离线制品](../../design/engineering-execution-design.md) |
| C38 | 未正式部署，可协调替换旧合同，不背六个月兼容承诺 | 实质规划 | [extension-compatibility.md · §6部署前替换与部署后兼容验证](../../design/extension-compatibility.md)；[cross-repo-execution-contract.md · 新旧组合、回滚与迁移](../../design/cross-repo-execution-contract.md) |
| C39 | 五仓统一PR，允许单维护者检查后合入 | 实质规划 | [engineering-execution-design.md · G02 main合入纪律与例外；ENG-D01a](../../design/engineering-execution-design.md)；[planning-decisions.json · ENG-D01a owner_confirmed](../planning-decisions.json) |
| C40 | 分档门禁相对现状的区别、排除项和生效窗口 | 实质规划 | [engineering-execution-design.md · ENG-D01b：分档门禁合同六条及错分反例](../../design/engineering-execution-design.md)；[AGENTS.md · 现行门禁纪律：make acceptance](../../../AGENTS.md) |
| C41 | 全站前端重规划，不能限于点名页面 | 部分深入 | **FD07**；[frontend-redesign.md · FE-PRE-08/09/10/11](../../../../zhuzhao-ui/docs/design/frontend-redesign.md)；[frontend-implementation-contract.md · StateFeedback/组件合同/全站实施映射](../../../../zhuzhao-ui/docs/design/frontend-implementation-contract.md) |
| C42 | 普通用户默认首页、Logo404与安全落点 | 实质规划 | [frontend-redesign.md · FE-PRE-01/02](../../../../zhuzhao-ui/docs/design/frontend-redesign.md)；[index.vue · RouterLink to="/"](../../../../zhuzhao-ui/src/components/Logo/index.vue) |
| C43 | 各页新建主操作位置一致 | 实质规划 | [frontend-redesign.md · FE-PRE-03](../../../../zhuzhao-ui/docs/design/frontend-redesign.md)；[frontend-implementation-contract.md · PageHeader与SectionHeader/工单列表](../../../../zhuzhao-ui/docs/design/frontend-implementation-contract.md) |
| C44 | 卡头说明一致、去除灰色长工程文案 | 实质规划 | [frontend-redesign.md · FE-PRE-03](../../../../zhuzhao-ui/docs/design/frontend-redesign.md)；[index.vue · 工具行/target_user表单提示](../../../../zhuzhao-ui/src/views/ticket/rules/index.vue) |
| C45 | 工单列表多选批量标签 | 实质规划 | [frontend-redesign.md · FE-PRE-05](../../../../zhuzhao-ui/docs/design/frontend-redesign.md)；[frontend-implementation-contract.md · ProTable与批量动作条/工单批量标签的API与权限接线草案](../../../../zhuzhao-ui/docs/design/frontend-implementation-contract.md) |
| C46 | 搜索/筛选/展开的视觉与语义统一 | 实质规划 | [frontend-redesign.md · FE-PRE-04](../../../../zhuzhao-ui/docs/design/frontend-redesign.md)；[index.vue · controlEls/applyCollapse/切换按钮](../../../../zhuzhao-ui/src/components/FilterBar/index.vue) |
| C47 | 情报历史左右栏、回填后明确重查、当前结果/本机记录 | 实质规划 | [frontend-redesign.md · FE-PRE-06](../../../../zhuzhao-ui/docs/design/frontend-redesign.md)；[frontend-implementation-contract.md · 情报主从工作区](../../../../zhuzhao-ui/docs/design/frontend-implementation-contract.md) |
| C48 | 全站配色、字体字号、间距比例和密度 | 部分深入 | **FD07**；[frontend-redesign.md · FE-PRE-07](../../../../zhuzhao-ui/docs/design/frontend-redesign.md)；[frontend-implementation-contract.md · 单一推荐视觉体系](../../../../zhuzhao-ui/docs/design/frontend-implementation-contract.md) |
| C49 | 默认单侧栏+身份顶栏，页签页脚不占默认首屏 | 实质规划 | [frontend-implementation-contract.md · 单一推荐视觉体系/信息架构和入口合同/UX01](../../../../zhuzhao-ui/docs/design/frontend-implementation-contract.md)；[Layout.vue · layout计算/四布局分支/TagsView](../../../../zhuzhao-ui/src/layout/Layout.vue) |
| C50 | 首页当前处理/复核责任，甲乙责任切换与去重 | 实质规划 | [business-lifecycle-contracts.md · §4 首页责任投影](../../design/business-lifecycle-contracts.md)；[frontend-implementation-contract.md · 公共工作台](../../../../zhuzhao-ui/docs/design/frontend-implementation-contract.md) |
| C51 | 首页任务与消息按权限保留，全局态势另视图 | 实质规划 | [frontend-implementation-contract.md · 公共工作台](../../../../zhuzhao-ui/docs/design/frontend-implementation-contract.md)；[home-review-verification.json · member lacks task-read/task click/empty menu](../../../../../github.com/tracerbiubiubiu/zhuzhao/.codex/audits/pre-phase6-replanning-20261111/design-deepening/ux-home-review/home-review-verification.json) |
| C52 | 文件字段上传追加B保留A，手动删A，多选及Markdown澄清 | 实质规划 | [engineering-execution-design.md · C02/ENG-D02](../../design/engineering-execution-design.md)；[frontend-implementation-contract.md · 文件上传的追加与主动删除](../../../../zhuzhao-ui/docs/design/frontend-implementation-contract.md) |
| C53 | 定时任务可选错失策略并充分细化 | 实质规划 | [schedule-recovery-policy.md · §1–4/§8](../../design/schedule-recovery-policy.md)；[cron.go · Loop.FireDue](../../../../taskrunner/internal/service/cron.go) |
| C54 | 逐次补齐超限暂停并保留余项 | 实质规划 | [schedule-recovery-policy.md · §4 overflow_policy/§5/§6/§7](../../design/schedule-recovery-policy.md) |
| C55 | 状态转换/转派成功含义，本地事务与异步后继 | 实质规划 | [cross-repo-execution-contract.md · reassign 的明确业务取舍](../../design/cross-repo-execution-contract.md)；[transition_service.go · Service.Transition commit后RunPostActions](../../../internal/service/ticket/transition_service.go) |
| C56 | 新工单类型仅参与人、旧类型保留范围 | 实质规划 | [business-lifecycle-contracts.md · §5.1/5.2/§10 B01](../../design/business-lifecycle-contracts.md) |
| C57 | 抄送仅查看评论，不附送处理权 | 实质规划 | [business-lifecycle-contracts.md · §5.2授权矩阵/§4.3 capabilities/§10 B02](../../design/business-lifecycle-contracts.md) |
| C58 | all数据范围不越类型限制，不另造敏感字段 | 实质规划 | [business-lifecycle-contracts.md · §5.2 BK-21/查看范围必要性与B03说明](../../design/business-lifecycle-contracts.md) |
| C59 | 独立授权业务管理者管理参与人/抄送 | 实质规划 | [business-lifecycle-contracts.md · §5.2/5.3/§10 B04](../../design/business-lifecycle-contracts.md) |
| C60 | 虚拟风险组管理/录入/受限派单，不额外通用建单权 | 实质规划 | [business-lifecycle-contracts.md · §5.4/§8.4/B05业界参照/§10 B05](../../design/business-lifecycle-contracts.md)；[role_repo.go · RoleRepo.GetEffectiveRoleCodes](../../../internal/repository/role_repo.go) |
| C61 | 风险记录与攻击应急事件区别，SOC事件职责授权 | 部分深入 | **FD08**；[business-lifecycle-contracts.md · §5.4 风险事件术语段/B05](../../design/business-lifecycle-contracts.md)；[planning-backlog.json · work_items D14](../planning-backlog.json) |
| C62 | 离职批准映射后自动逐项交接，安全禁用先行 | 实质规划 | [business-lifecycle-contracts.md · §7.1–7.3/7.5/§10 B06](../../design/business-lifecycle-contracts.md) |
| C63 | 撤部预览确认plan后批量接管，历史/权限保护 | 实质规划 | [business-lifecycle-contracts.md · §7.4/7.5/§10 B07](../../design/business-lifecycle-contracts.md)；[hr-directory-sync.md · §4.3](../../proposal/hr-directory-sync.md) |
| C64 | 审计阶段、当前完成程度及owner何时参与 | 实质规划 | [planning-closeout.md · 五步/参与窗口](../planning-closeout.md)；[planning-decisions.json](../planning-decisions.json) |
| C65 | 原6/7/8/9及更远功能承接、优化扩展与适合规模的渐进路线 | 实质规划 | [roadmap.md · §1–7](../../roadmap.md)；[project-evolution-strategy.md · §1–8](../../design/project-evolution-strategy.md) |

## 核验后的交付含义与下一步

本轮完成的是所有关注点的研究深度核验，答案是覆盖较完整、深度不齐。51项已有实质研究，10项对应8组应补研究，4项合理后置；核验完成不等于补研究已经完成。

优先补可独立完成的耦合清单、触发合理性及MCP范围/授权取舍；全站视觉与薄事件职责在对应批前补；领域Agent、NL计划确认和skill用同一合成场景贯通，按P7/P8深化。无需现在提供内网地址/凭据或一次拍完未来参数，也不要求立即拆服务/重写平台。

长期mock、专业供给优先、已有模型API与内网限制、私有adapter隔离、多厂商设备人工入口、统一PR与渐进演进等原则继续有效。本次未重开这些决定、修改71包粗估或实施新功能。
