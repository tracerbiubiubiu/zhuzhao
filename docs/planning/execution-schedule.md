# 后续完整排程 — 既有能力、互通与遗漏范围的有界重排

日期：2026-11-16。本文件由仓内计算器生成；设计/计价候选，T0未启动，不是开发或部署指令。

当前实施顺序、验收及日期请先读[按交付结果排列的排期](intranet-uaa-release-schedule.md)。本页用于复算历史工作量和工作包依赖。

## 范围、来源与复现

原71父包完整承接，正式纳入CE五项后为76父包、17正式子出口、6项LS额外范围任务、13发布切片。
LS不新增父业务包，普通子出口与CE02内部迁移配额只分解父价；UAA C/P/R仍为独立增量。
本页长程总量只覆盖76包既有计价基线，不是基础版B0/B1工期，也未合并F004新增未净估细项及OBS候选补充；后继范围见[补充册](post-baseline-scope-estimates.json)。
首个本地正式核心获准先用本地账号，质量优先；1月试用是争取窗口，完整UAA P/R核心稳定后交。见[阶梯](../phase6/first-usable-release-plan.md)、[首版净估候选](local-core-release-estimate.json)和[变化对照](core-release-change-comparison.md)；本页长期DAG/日窗不是该首版顺序或日期承诺。
六项RP替换原基价，不能替换后又把净差额再加一次。
数字源为[净增册](planning-scope-estimates.json)、[UAA估算](uaa-implementation-estimate.json)和[总账](planning-backlog.json)。
切片、UAA接线及分阶段迁移配额见[排程模型](execution-schedule-model.json)；逐任务/资源槽/输入哈希见[JSON](execution-schedule.json)。
复现：`python3 scripts/calculate-planning-schedule.py --write`；检查：`python3 scripts/calculate-planning-schedule.py --check`。
测试：`python3 scripts/test-planning-schedule.py`。无需本机隐藏目录、旧ignored生成器或Git冻结版本才能计算。

## 工程量与相较上一版的变化

补充前公共292–496/含R305–518，净增102–160，当前公共394–656/含R407–678。基线、净增及当前总量分别由净增册与实际WBS求和派生，不与首版本地净价叠加。
G07A/G07B历史开发8–14日不再排；已完成的版本维护/合main不再派发或重复计价。原with/without reserve保留1–2日历史/条件容量对照，不是今日剩余交付承诺；本地Q2已扣除该维护信用。


## 当前审计候选的累计工程量

- 人工风险整改，52.5–88.5 日，保守 110.5 日。
- 自动任务、清单和公共消息组合，83.0–134.0 日，保守 156 日。
- 本地核心正式和稳定，95.5–153.0 日，保守 175 日。
- 公共 UAA 双模式，126.5–203.0 日，保守 228 日。
- 真实 UAA 纳管，139.5–225.0 日，保守 250 日。

日期、目标环境和外部等待另列。累计值有重叠，按[交付排期](intranet-uaa-release-schedule.md#2-最早与保守排期)阅读。

## 首版本地核心的机器投影（原对账底稿）

2026-11-16 首版净估对账底稿：**97–151 个有效工程日**（未取整 96.5–150.5；S2 共用假设不成立时 99–154）。这是单席工程量候选，不是日历承诺。

同一生成器读取[首版净估册](local-core-release-estimate.json)和[目标](first-usable-release-target.json)，复用既有WBS、来源父包及必交旅程；净价不追加到76包总价。
B0既有制品/mock技术预演见[首版阶梯](../phase6/first-usable-release-plan.md#4-按质量出口交付)，不反向等待全部新增能力。真实业务试用及正式启用仍须具体P1/能力/环境证据门。
以下只演算所需输入就绪时的有效工程日；真实消息窗口、试用环境和稳定验收仍待证据，未知等待不是0，正式日期和激活批准均未给出。场景/出口的依赖闭包重叠，其工日不能相加。

### 净额与必交旅程

- `S`：37–60工程日。
- `RISK`：11.0–17.0工程日。
- `TD`：13–19工程日。
- `AD`：9–13工程日。
- `MH`：7–11工程日。
- `MR`：2–3工程日。
- `UI`：10.5–16.5工程日。
- `RELEASE`：7–11工程日。
- [/must_have/0](first-usable-release-target.json#/must_have/0) 风险域和风险到整改工单人工闭环，逐风险责任/证据/复核和有界N:1；目标`R1, R2, R3, R4, R5, U2, U3`。
- [/must_have/1](first-usable-release-target.json#/must_have/1) 既有工单及所选派单/证据/复核，工单列表有限多选加标签；目标`S3, R4, U4, U8`。
- [/must_have/2](first-usable-release-target.json#/must_have/2) 工单→taskrunner→原任务结果/activelist受权更新；目标`TD01, TD02, TD03, TD04, TD05, U4`。
- [/must_have/3](first-usable-release-target.json#/must_have/3) activelist变化→获权TR后继任务；仅消息/刷新不代验；目标`AD01, AD02, AD03, AD04, U5`。
- [/must_have/4](first-usable-release-target.json#/must_have/4) 公共HTTP sender及一条实际公司消息映射/实测，明确受理与送达；真实条件未知不代验；目标`MH01, MH02, MH03, MH04, MR1, U6`。
- [/must_have/5](first-usable-release-target.json#/must_have/5) 完整本地账号/角色/菜单管理及L1/L2/L3，必要UAA-C来源/稳定键/私有adapter接缝；完整P/R后继；目标`S1, U1`。
- [/must_have/6](first-usable-release-target.json#/must_have/6) 当前授权/幂等/必要持久交接/失败结果/业务与审计历史/薄UI；目标`S1, S2, S3, TD05, MH02, MH04, U7`。
- [/must_have/7](first-usable-release-target.json#/must_have/7) 核心公共合同与私有adapter隔离；旧参考adapter×新核心兼容验收；目标`S1, MH01, Q1`。
- [/must_have/8](first-usable-release-target.json#/must_have/8) 首次本地正式生产适用制品/漏洞/运行配置/备份恢复/回退与故障接管证据；目标`Q1, Q2`。

### 同一WBS的依赖编排

父包仅作来源追踪，不追加父价或完整长期祖先；每行净额计一次。
- `S2`（来源`C04, R01, CE01, CE02`）：4–6日；直接依赖`无`。
- `S1`（来源`C04, C05, D06, E01`）：30–49日；直接依赖`S2`。
- `S3`（来源`C02, C04`）：3–5日；直接依赖`无`。
- `U1`（来源`U01, U02, U07`）：0.5–1日；直接依赖`S1`。
- `R1`（来源`D08`）：3–4.5日；直接依赖`S1, S3`。
- `R2`（来源`D03, D08`）：2–3日；直接依赖`R1`。
- `R3`（来源`D09`）：1.5–2.5日；直接依赖`R2`。
- `R4`（来源`D10`）：3–4.5日；直接依赖`R3`。
- `R5`（来源`D10`）：1.5–2.5日；直接依赖`R4`。
- `U2`（来源`D08, D09, U02, U07`）：3–4日；直接依赖`R5, U1`。
- `U3`（来源`D10, U03`）：0.5–1日；直接依赖`R4, U2`。
- `U8`（来源`U03`）：2–3日；直接依赖`S1`。
- `TD01`（来源`C03, D12, I01, I02`）：2–3日；直接依赖`S1`。
- `MH01`（来源`D11, E01`）：2–3日；直接依赖`S2`。
- `TD03`（来源`R06, CE03`）：3–5日；直接依赖`S1, TD01`。
- `TD02`（来源`C03, D12, CE02`）：2–3日；直接依赖`S2, TD01`。
- `MH02`（来源`D11, D15`）：1–2日；直接依赖`MH01, S1`。
- `TD04`（来源`R06, CE03`）：3–4日；直接依赖`S2, TD03`。
- `MH03`（来源`D11, R01`）：2–3日；直接依赖`MH02, S2`。
- `TD05`（来源`C03, D12, R06, CE03`）：3–4日；直接依赖`TD02, TD04`。
- `MH04`（来源`D11, D15`）：2–3日；直接依赖`MH03`。
- `U4`（来源`D02, D12, U03`）：1.5–2.5日；直接依赖`S3, TD05`。
- `U6`（来源`U06, D11, D15`）：0.5–1日；直接依赖`MH04`。
- `AD01`（来源`CE04`）：2–3日；直接依赖`TD04`。
- `AD02`（来源`CE04, CE05`）：2–3日；直接依赖`AD01`。
- `AD03`（来源`CE04, CE05`）：3–4日；直接依赖`AD02, S2, TD03`。
- `AD04`（来源`CE04, CE05`）：2–3日；直接依赖`AD03, TD05`。
- `U5`（来源`U06, CE03, CE04`）：0.5–1日；直接依赖`AD04, TD05`。
- `U7`（来源`U07`）：2–3日；直接依赖`U2, U3, U4, U5, U6`。
- `Q1`（来源`G03, G04, R07`）：4–6日；直接依赖`AD04, MH04, TD05, U7, U8`。
- `MR1`（来源`E01, D11`）：2–3日；直接依赖`MH04`。
- `Q2`（来源`G05, E09`）：3–5日；直接依赖`MR1, Q1`。

### 条件与出口

- 1个有效研发席位条件工程量端点：96.5–150.5有效工作日；不含未知外部等待，不是日历承诺。
- 2个有效研发席位条件工程量端点：70.5–111有效工作日；不含未知外部等待，不是日历承诺。
- [MH01_MR1](owner-decision-agenda.json#/execution_actions/MH01_MR1)：pending_real_inputs_then_binding；证据状态not_supplied，不自动标为已就绪。
- [Q09_unattended_activation](owner-decision-agenda.json#/execution_actions/Q09_unattended_activation)：pending_real_inputs_then_owner_review；证据状态not_supplied，不自动标为已就绪。
- [Q12_trial](owner-decision-agenda.json#/execution_actions/Q12_trial)：pending_real_inputs_then_binding；证据状态not_supplied，不自动标为已就绪。
- [Q12_core_stability](owner-decision-agenda.json#/execution_actions/Q12_core_stability)：engineering_candidate_pending_real_trial_acceptance；证据状态not_yet_implemented_or_trialed，不自动标为已就绪。
- **risk_ticket_trial**：目标`U2, U3`；条件`Q12_trial`；[证据要求](../phase6/first-usable-release-plan.md#41-b0预演与真实业务试用准入)。计算结论仅为`not_verified_by_schedule`。
- **public_contract_core**：目标`Q1`；条件`无外部输入门；仍需产品验收`；[证据要求](../phase6/first-usable-release-plan.md#4-按质量出口交付)。计算结论仅为`not_verified_by_schedule`。
- **formal_local**：目标`Q2`；条件`MH01_MR1, Q09_unattended_activation, Q12_trial`；[证据要求](../phase6/first-usable-release-plan.md#41-b0预演与真实业务试用准入)。计算结论仅为`not_verified_by_schedule`。
- **uaa_capacity_transition**：目标`Q2`；条件`Q12_core_stability`；[证据要求](../phase6/first-usable-release-plan.md#42-核心稳定的过渡出口)。计算结论仅为`not_verified_by_schedule`。

机器检查净价/子分配、逐行单计、父包映射、缺依赖/环、必交原文hash与覆盖、外部动作/出口绑定及现势摘要；不证明映射语义、产品可用、安全缺陷修复或生产验收。
受控摘要在[控制台](../review/11-project-control.md)和[路线图](../roadmap.md)共同核验，历史区保留原时点；净价改变须同步两区间及目标JSON。

## 长期76父包工程量对照

- 公共范围，R待外部条件：不含条件储备394–656日；保留历史/条件维护储备对照395–658日。
  - 1席参考：累计 395 / 658 个有效工作日结束，约 79.0 / 131.6 个有效研发周。
  - 2席参考：累计 200 / 332 个有效工作日结束，约 40.0 / 66.4 个有效研发周。
- 选定R且各阶段外部等待为0的条件对照：不含条件储备407–678日；保留历史/条件维护储备对照408–680日。
  - 1席参考：累计 408 / 680 个有效工作日结束，约 81.6 / 136.0 个有效研发周。
  - 2席参考：累计 206 / 343 个有效工作日结束，约 41.2 / 68.6 个有效研发周。

UAA P的后台、四页最小UI、运维、CI与质量仍只算25–39；R真实UI/运维及正常切换观察仍只算13–22。
U02/U06/UI新功能、CE与LS各按净增册扣重，不把完整前端或新增调用层误算作旧价免费附送。

## 资源与时间含义

主参考1个有效研发席位，每周5工程日；2席仅独立交付能力具备时对比，不等于两个agent。
实际开工日记为 T0，所有相对日窗从开工起按累计有效工作日表示。本页用于长期相对估算，首版以[交付排期](intranet-uaa-release-schedule.md)为准。
每项低/高输入分别跑一次排程；下文“低 / 高”不是置信区间或任意混合耗时的保证上界。
条件对照保留原维护先行排序；已完成维护不重新派发。优先基础U01/U02/C02/X01-B/X02-Q及真实祖先，保留S01/S02安全工作与E10有限调查，再交D10/D15风险首链。公开UAA P正任务优先级7.5，选定且外部就绪的企业对照在基础后优先P/R；这些是原长期容量对照，不代表首版本地顺序或激活获批。
阶段不是全串行大门；日窗可交错。单席总工期等于所选工日和；双席逐槽验证不重叠。

## 全部13发布切片

以下含CE/LS及RP替换后的净范围，剔除已做G07；UAA独立增量与条件维护容量单列，不塞进S09的E01旧基价。

- **S01 P6.0 隔离、有效检查与安全入口**：7–13工程日；1席累计 8→36 日 / 14→59 日；2席累计 1→18 日 / 2→29 日。
- **S02 P6.0 正确性与受支持构建组合**：11–19工程日；1席累计 1→42 日 / 2→69 日；2席累计 1→24 日 / 2→39 日。
- **S03 P6.1 授权、定义与领域一致性**：29–47工程日；1席累计 3→71 日 / 6→117 日；2席累计 3→36 日 / 6→60 日。
- **S04 P6.1 扩展、体验样板与数据组件**：41–66工程日；1席累计 11→111 日 / 19→182 日；2席累计 4→57 日 / 7→93 日。
- **S05 P6.2 风险首链与受权查询**：23–38工程日；1席累计 71→114 日 / 117→188 日；2席累计 34→58 日 / 55→97 日。
- **S06 P6.3 工单、结果、配置与受控写入**：74–123工程日；1席累计 114→188 日 / 188→311 日；2席累计 57→95 日 / 93→158 日。
- **S07 P6.E 本地指标与SOC薄协作**：11–18工程日；1席累计 188→199 日 / 311→329 日；2席累计 94→104 日 / 155→171 日。
- **S08 P7 Action、身份、恢复与手动消费**：46–77工程日；1席累计 224→270 日 / 368→445 日；2席累计 111→140 日 / 180→230 日。
- **S09 P7 专业源、平台治理与实际就绪**：68–114工程日；1席累计 270→338 日 / 445→559 日；2席累计 114→171 日 / 190→282 日。
- **S10 P8 专业执行与领域智能**：23–39工程日；1席累计 338→361 日 / 559→598 日；2席累计 168→183 日 / 279→307 日。
- **S11 P8 固定报告、通报与选定协作**：14–25工程日；1席累计 361→375 日 / 598→623 日；2席累计 179→188 日 / 293→313 日。
- **S12 P9 持久闭环、接管与架构复评**：16–27工程日；1席累计 375→391 日 / 623→650 日；2席累计 188→200 日 / 312→332 日。
- **S13 P10 经裁定的更远增量**：4–8工程日；1席累计 391→395 日 / 650→658 日；2席累计 190→194 日 / 316→324 日。

## 基础目标与风险首链的独立出口

以下只按具名目标及真实祖先计算联合范围；两组共享依赖，工日不能相加，日窗也不代表批准生产或企业模式激活。基础目标不是等待全部远期76包完成。
- **基础接缝/首页与附件正确性**（U01, U02, C02, X01-B, X02-Q）：含真实祖先35–57工程日；1席累计 0→35 日 / 0→57 日；2席累计 0→19 日 / 0→32 日。
- **风险首链**（D10, D15）：含真实祖先75–122工程日；1席累计 0→91 日 / 0→149 日；2席累计 0→54 日 / 0→87 日。

## 按Phase汇总的工程窗

本节按任务实际所属Phase汇总，含跨阶段UAA C/P与CE02-L配额；不把CE02父包全部20–30日塞进P6.1。

- P6.0：20–35工程日；1席累计 1→42 日 / 2→69 日；2席累计 1→24 日 / 2→39 日。
- P6.1：95–152工程日；1席累计 3→224 日 / 6→368 日；2席累计 3→114 日 / 6→190 日。
- P6.2：23–38工程日；1席累计 71→114 日 / 117→188 日；2席累计 34→58 日 / 55→97 日。
- P6.3：74–123工程日；1席累计 114→188 日 / 188→311 日；2席累计 57→95 日 / 93→158 日。
- P6.E：11–18工程日；1席累计 188→199 日 / 311→329 日；2席累计 94→104 日 / 155→171 日。
- P7：114–191工程日；1席累计 224→338 日 / 368→559 日；2席累计 111→171 日 / 180→282 日。
- P8：37–64工程日；1席累计 338→375 日 / 559→623 日；2席累计 168→188 日 / 279→313 日。
- P9：16–27工程日；1席累计 375→391 日 / 623→650 日；2席累计 188→200 日 / 312→332 日。
- P10：4–8工程日；1席累计 391→395 日 / 650→658 日；2席累计 190→194 日 / 316→324 日。
- pre_business：1–2工程日；1席累计 0→1 日 / 0→2 日；2席累计 0→1 日 / 0→2 日。

## 关键出口与分阶段迁移

- E01-UAA-C：1席累计 16 / 27 日；2席累计 9 / 15 日。
- E01-UAA-P：1席累计 224 / 368 日；2席累计 114 / 190 日。
- X02-F：1席累计 66 / 108 日；2席累计 31 / 50 日。
- CE01：1席累计 94 / 154 日；2席累计 41 / 69 日。
- CE02-S：1席累计 100 / 163 日；2席累计 47 / 78 日。
- CE02-D：1席累计 119 / 196 日；2席累计 62 / 101 日。
- D12：1席累计 123 / 203 日；2席累计 66 / 108 日。
- CE02-L：1席累计 389 / 650 日；2席累计 200 / 332 日。

CE01因C04实际在P6.1；CE02-S6–9日/P6.1，CE02-D5–8日/P6.3，CE02-L9–13日最终在P9收口。
L内部配额为P6.1 2–3、P6.3 3–4、P7 2–3、P9 2–3日，随IAM/组织、工单/通知/情报/AL、Action/MCP/适配、持久闭环分批迁移。
新增或触及模块当批接入，P9只是存量最终库存/旁路验收；当前候选能力runtime_verified均不能因已排程变为true。
D12只依CE02-D可靠入口，不反依I01；I01/I02/I03消费D12。CE03/04/05在P7，LS01–04在P6.3，LS05/06在P7。
X01-C2–3/X01-B3–4/X01-H3–5仍合计原8–12日；B为配置后重启的受信接缝，H在S09/P7交完整管理增强。X02-Q独立依X01-C，不等D06/feed；X03只依U02、X01-B和X02-Q，资源授权不因此削弱。

## UAA在现有13切片中的跨阶段硬出口

完整UAA P/R的实际主研发产能投入须通过既有`local_core.delivery_exits.uaa_capacity_transition`，对应`Q12_core_stability`业务/工程证据，当前仍为`not_verified_by_schedule`。这不是新增有价任务，不要求整个P6.3完成，也不阻协议联系、条件索取和公共mock准备；下方长期日窗不是该门已通过的证明。
公共范围保留mock/contract，不因公司资源未到而阻断独立IAM和公共开发；真实成功不能由合成用例代验。
UAA测试进入须P、测试应用/协议、网络和专属测试权限；R内部才产出影子差异、私有制品/runbook。
正式apply前另需映射/影子差异批准、正式资源/窗口、应急退出与适用公共生产门；apply后观察才是R整体收口。
不把R整体收口倒置为自己apply前提，也不编造pre_apply具体天数或正式激活日。
真实专业数据源/第二类adapter、标准OAuth AS/主体桥、SOAR/知识/Agent等依赖各自提供方合同、身份与权限。
外部排队、修复、审批和窗口等待不在工程日内；LS05/06已计有限公共实现，真实条件未具备时保持未激活。
未选Office/供应商门户/草稿等整套产品、获准后三类导入/merge建设仍按净增册条件另估，不由E05/E10小包包干。
独立首次生产仍验G05/E09适用硬项；公司UAA纳管模式加R.pre_apply。早期首发只前移硬证据，不机械等全部P7/I03。
详细条件和阶段见[真实接入条件](uaa-real-integration-conditions.json)；各提供方激活窗口由对应Owner确认。

## 全部正工时任务派发顺序

下列保留公共含历史/条件维护储备的单席对照，不是今日剩余派单；当前首版净额和顺序见上方本地核心投影。双席及含R记录保留JSON，聚合节点零工日不另计价。

- 1. `MAINTENANCE-CLOSEOUT`（pre_business，maintenance）：1–2日，累计 0→1 日 / 0→2 日；依赖`无`。
- 2. `C01`（P6.0，C01）：2–4日，累计 1→3 日 / 2→6 日；依赖`MAINTENANCE-CLOSEOUT`。
- 3. `C04`（P6.1，C04）：5–8日，累计 3→8 日 / 6→14 日；依赖`C01, MAINTENANCE-CLOSEOUT`。
- 4. `G01`（P6.0，G01）：2–3日，累计 8→10 日 / 14→17 日；依赖`MAINTENANCE-CLOSEOUT`。
- 5. `G03-Q`（P6.0，G03）：1–2日，累计 10→11 日 / 17→19 日；依赖`G01, MAINTENANCE-CLOSEOUT`。
- 6. `G06`（P6.1，G06）：3–5日，累计 11→14 日 / 19→24 日；依赖`G01, MAINTENANCE-CLOSEOUT`。
- 7. `E01-UAA-C`（P6.0，E01）：2–3日，累计 14→16 日 / 24→27 日；依赖`MAINTENANCE-CLOSEOUT`。
- 8. `G03-C`（P6.0，G03）：2–3日，累计 16→18 日 / 27→30 日；依赖`G01, G03-Q, MAINTENANCE-CLOSEOUT`。
- 9. `X01-C`（P6.1，X01）：2–3日，累计 18→20 日 / 30→33 日；依赖`C04, E01-UAA-C, G03, G06, MAINTENANCE-CLOSEOUT`。
- 10. `X02-Q`（P6.1，X02）：2–3日，累计 20→22 日 / 33→36 日；依赖`MAINTENANCE-CLOSEOUT, X01-C`。
- 11. `U01`（P6.0，U01）：1–3日，累计 22→23 日 / 36→39 日；依赖`MAINTENANCE-CLOSEOUT`。
- 12. `U02`（P6.1，U02）：6–9日，累计 23→29 日 / 39→48 日；依赖`MAINTENANCE-CLOSEOUT, U01`。
- 13. `X01-B`（P6.1，X01）：3–4日，累计 29→32 日 / 48→52 日；依赖`C04, G03, G06, MAINTENANCE-CLOSEOUT, X01-C`。
- 14. `C02`（P6.0，C02）：3–5日，累计 32→35 日 / 52→57 日；依赖`MAINTENANCE-CLOSEOUT`。
- 15. `G02`（P6.0，G02）：1–2日，累计 35→36 日 / 57→59 日；依赖`G01, G03-Q, MAINTENANCE-CLOSEOUT`。
- 16. `C03`（P6.0，C03）：6–10日，累计 36→42 日 / 59→69 日；依赖`MAINTENANCE-CLOSEOUT`。
- 17. `D01`（P6.1，D01）：7–11日，累计 42→49 日 / 69→80 日；依赖`C01, C04, MAINTENANCE-CLOSEOUT`。
- 18. `D04`（P6.1，D04）：3–5日，累计 49→52 日 / 80→85 日；依赖`C01, D01, MAINTENANCE-CLOSEOUT`。
- 19. `D02`（P6.1，D02）：3–5日，累计 52→55 日 / 85→90 日；依赖`D01, D04, MAINTENANCE-CLOSEOUT`。
- 20. `D03`（P6.1，D03）：4–7日，累计 55→59 日 / 90→97 日；依赖`C04, D02, MAINTENANCE-CLOSEOUT`。
- 21. `D06`（P6.1，D06）：4–6日，累计 59→63 日 / 97→103 日；依赖`C04, E01-UAA-C, MAINTENANCE-CLOSEOUT`。
- 22. `X02-F`（P6.1，X02）：3–5日，累计 63→66 日 / 103→108 日；依赖`D06, E01-UAA-C, MAINTENANCE-CLOSEOUT, X01-C`。
- 23. `E10`（P6.1，E10）：2–4日，累计 66→68 日 / 108→112 日；依赖`D01, D03, MAINTENANCE-CLOSEOUT, X02`。
- 24. `R01`（P6.1，R01）：3–5日，累计 68→71 日 / 112→117 日；依赖`C03, C04, MAINTENANCE-CLOSEOUT`。
- 25. `D07`（P6.2，D07）：3–5日，累计 71→74 日 / 117→122 日；依赖`D04, D06, MAINTENANCE-CLOSEOUT, X02-Q`。
- 26. `D08`（P6.2，D08）：7–10日，累计 74→81 日 / 122→132 日；依赖`D01, D03, D06, D07, MAINTENANCE-CLOSEOUT, X02-F`。
- 27. `D15`（P6.2，D15）：1–2日，累计 81→82 日 / 132→134 日；依赖`D06, D08, MAINTENANCE-CLOSEOUT, R01`。
- 28. `D09`（P6.2，D09）：4–7日，累计 82→86 日 / 134→141 日；依赖`D08, D15, MAINTENANCE-CLOSEOUT`。
- 29. `D10`（P6.2，D10）：5–8日，累计 86→91 日 / 141→149 日；依赖`D02, D08, D09, MAINTENANCE-CLOSEOUT, R01`。
- 30. `CE01`（P6.1，CE01）：3–5日，累计 91→94 日 / 149→154 日；依赖`C04, MAINTENANCE-CLOSEOUT`。
- 31. `CE02-S`（P6.1，CE02）：6–9日，累计 94→100 日 / 154→163 日；依赖`C04, CE01, MAINTENANCE-CLOSEOUT, X01-C`。
- 32. `G04`（P6.1，G04）：4–7日，累计 100→104 日 / 163→170 日；依赖`G03, MAINTENANCE-CLOSEOUT`。
- 33. `CE02-L-P61`（P6.1，CE02）：2–3日，累计 104→106 日 / 170→173 日；依赖`CE02-S, D03, MAINTENANCE-CLOSEOUT`。
- 34. `C05`（P6.1，C05）：2–4日，累计 106→108 日 / 173→177 日；依赖`G04, MAINTENANCE-CLOSEOUT`。
- 35. `U07`（P6.1，U07）：3–5日，累计 108→111 日 / 177→182 日；依赖`G04, MAINTENANCE-CLOSEOUT, U02`。
- 36. `A01`（P6.2，A01）：3–6日，累计 111→114 日 / 182→188 日；依赖`D03, D07, MAINTENANCE-CLOSEOUT, U02`。
- 37. `CE02-D`（P6.3，CE02）：5–8日，累计 114→119 日 / 188→196 日；依赖`C03, CE02-S, MAINTENANCE-CLOSEOUT, R01`。
- 38. `D12`（P6.3，D12）：4–7日，累计 119→123 日 / 196→203 日；依赖`C03, CE02-D, D02, MAINTENANCE-CLOSEOUT, R01`。
- 39. `A02`（P6.3，A02）：3–5日，累计 123→126 日 / 203→208 日；依赖`A01, D01, D02, MAINTENANCE-CLOSEOUT, R01`。
- 40. `D11`（P6.3，D11）：3–6日，累计 126→129 日 / 208→214 日；依赖`D02, MAINTENANCE-CLOSEOUT`。
- 41. `U06`（P6.3，U06）：12–19日，累计 129→141 日 / 214→233 日；依赖`D11, D12, MAINTENANCE-CLOSEOUT, U02`。
- 42. `LS01`（P6.3，D04）：11–17日，累计 141→152 日 / 233→250 日；依赖`C04, D01, D03, D04, MAINTENANCE-CLOSEOUT, U02`。
- 43. `U03`（P6.3，U03）：4–7日，累计 152→156 日 / 250→257 日；依赖`D02, D03, MAINTENANCE-CLOSEOUT, U02`。
- 44. `CE02-L-P63`（P6.3，CE02）：3–4日，累计 156→159 日 / 257→261 日；依赖`CE02-D, CE02-L-P61, D12, MAINTENANCE-CLOSEOUT, U03, U06`。
- 45. `LS02`（P6.3，U03）：5–9日，累计 159→164 日 / 261→270 日；依赖`D04, LS01, MAINTENANCE-CLOSEOUT, U02`。
- 46. `U04`（P6.3，U04）：5–8日，累计 164→169 日 / 270→278 日；依赖`D01, D04, MAINTENANCE-CLOSEOUT, U02`。
- 47. `D05`（P6.3，D05）：4–7日，累计 169→173 日 / 278→285 日；依赖`D01, D04, MAINTENANCE-CLOSEOUT`。
- 48. `X03`（P6.3，X03）：4–7日，累计 173→177 日 / 285→292 日；依赖`MAINTENANCE-CLOSEOUT, U02, X01-B, X02-Q`。
- 49. `D16`（P6.3，D16）：3–5日，累计 177→180 日 / 298→303 日；依赖`D01, D02, D04, D06, MAINTENANCE-CLOSEOUT, R01`。
- 50. `LS03`（P6.3，U06）：3–4日，累计 180→183 日 / 303→307 日；依赖`D03, MAINTENANCE-CLOSEOUT, U01, U02`。
- 51. `LS04`（P6.3，D02）：3–6日，累计 183→186 日 / 292→298 日；依赖`D02, LS01, MAINTENANCE-CLOSEOUT`。
- 52. `U05`（P6.3，U05）：2–4日，累计 186→188 日 / 307→311 日；依赖`MAINTENANCE-CLOSEOUT, U02`。
- 53. `D13-M`（P6.E，D13）：3–5日，累计 188→191 日 / 311→316 日；依赖`D08, D09, D10, MAINTENANCE-CLOSEOUT`。
- 54. `D13-T`（P6.E，D13）：1–2日，累计 191→192 日 / 316→318 日；依赖`D08, D09, D10, D11, D12, MAINTENANCE-CLOSEOUT`。
- 55. `D14`（P6.E，D14）：7–11日，累计 192→199 日 / 318→329 日；依赖`D10, D13, MAINTENANCE-CLOSEOUT`。
- 56. `UAA-P-PB02`（P6.1，E01）：3–4日，累计 199→202 日 / 329→333 日；依赖`C04, D06, E01-UAA-C, MAINTENANCE-CLOSEOUT`；实际主产能准入`local_core.delivery_exits.uaa_capacity_transition`。
- 57. `UAA-P-PB03`（P6.1，E01）：1–2日，累计 202→203 日 / 333→335 日；依赖`MAINTENANCE-CLOSEOUT, UAA-P-PB02, X01-C`；实际主产能准入`local_core.delivery_exits.uaa_capacity_transition`。
- 58. `UAA-P-PB01`（P6.1，E01）：3–4日，累计 203→206 日 / 335→339 日；依赖`C04, E01-UAA-C, MAINTENANCE-CLOSEOUT`；实际主产能准入`local_core.delivery_exits.uaa_capacity_transition`。
- 59. `UAA-P-PB05`（P6.1，E01）：3–4日，累计 206→209 日 / 339→343 日；依赖`MAINTENANCE-CLOSEOUT, UAA-P-PB02, UAA-P-PB03, X02-F`；实际主产能准入`local_core.delivery_exits.uaa_capacity_transition`。
- 60. `UAA-P-PB04`（P6.1，E01）：1–2日，累计 209→210 日 / 343→345 日；依赖`MAINTENANCE-CLOSEOUT, UAA-P-PB02, X02-F`；实际主产能准入`local_core.delivery_exits.uaa_capacity_transition`。
- 61. `UAA-P-PB06`（P6.1，E01）：1–2日，累计 210→211 日 / 345→347 日；依赖`MAINTENANCE-CLOSEOUT, UAA-P-PB02, X02-F`；实际主产能准入`local_core.delivery_exits.uaa_capacity_transition`。
- 62. `UAA-P-PB07`（P6.1，E01）：2–3日，累计 211→213 日 / 347→350 日；依赖`MAINTENANCE-CLOSEOUT, UAA-P-PB01, UAA-P-PB04, UAA-P-PB05, UAA-P-PB06`；实际主产能准入`local_core.delivery_exits.uaa_capacity_transition`。
- 63. `UAA-P-PB08`（P6.1，E01）：2–4日，累计 213→215 日 / 350→354 日；依赖`MAINTENANCE-CLOSEOUT, UAA-P-PB01, UAA-P-PB02, UAA-P-PB03, UAA-P-PB04, UAA-P-PB05, UAA-P-PB06, UAA-P-PB07`；实际主产能准入`local_core.delivery_exits.uaa_capacity_transition`。
- 64. `UAA-P-PU01`（P6.1，E01）：2–3日，累计 215→217 日 / 354→357 日；依赖`E01-UAA-C, MAINTENANCE-CLOSEOUT`；实际主产能准入`local_core.delivery_exits.uaa_capacity_transition`。
- 65. `UAA-P-PU02`（P6.1，E01）：2–3日，累计 217→219 日 / 357→360 日；依赖`E01-UAA-C, MAINTENANCE-CLOSEOUT`；实际主产能准入`local_core.delivery_exits.uaa_capacity_transition`。
- 66. `UAA-P-PO01`（P6.1，E01）：2–3日，累计 219→221 日 / 360→363 日；依赖`MAINTENANCE-CLOSEOUT, UAA-P-PB08, UAA-P-PU01, UAA-P-PU02`；实际主产能准入`local_core.delivery_exits.uaa_capacity_transition`。
- 67. `UAA-P-PU03`（P6.1，E01）：1–2日，累计 221→222 日 / 363→365 日；依赖`MAINTENANCE-CLOSEOUT, UAA-P-PB08, UAA-P-PU01, UAA-P-PU02`；实际主产能准入`local_core.delivery_exits.uaa_capacity_transition`。
- 68. `UAA-P-PC01`（P6.1，E01）：1–2日，累计 222→223 日 / 365→367 日；依赖`MAINTENANCE-CLOSEOUT, UAA-P-PB08, UAA-P-PO01, UAA-P-PU03`；实际主产能准入`local_core.delivery_exits.uaa_capacity_transition`。
- 69. `UAA-P-PQ01`（P6.1，E01）：1–1日，累计 223→224 日 / 367→368 日；依赖`MAINTENANCE-CLOSEOUT, UAA-P-PC01`；实际主产能准入`local_core.delivery_exits.uaa_capacity_transition`。
- 70. `I01`（P7，I01）：5–9日，累计 224→229 日 / 368→377 日；依赖`C03, D01, D12, MAINTENANCE-CLOSEOUT`。
- 71. `I02`（P7，I02）：4–7日，累计 229→233 日 / 377→384 日；依赖`D03, I01, MAINTENANCE-CLOSEOUT`。
- 72. `I03`（P7，I03）：9–15日，累计 233→242 日 / 384→399 日；依赖`C03, D12, I01, I02, MAINTENANCE-CLOSEOUT`。
- 73. `A03-R`（P7，A03）：7–11日，累计 242→249 日 / 399→410 日；依赖`A01, I01, I02, MAINTENANCE-CLOSEOUT`。
- 74. `I05`（P7，I05）：4–7日，累计 249→253 日 / 410→417 日；依赖`C03, I02, I03, MAINTENANCE-CLOSEOUT`。
- 75. `A04`（P7，A04）：4–7日，累计 253→257 日 / 417→424 日；依赖`A02, I02, I03, I05, MAINTENANCE-CLOSEOUT`。
- 76. `A03-W`（P7，A03）：2–3日，累计 257→259 日 / 424→427 日；依赖`A01, A02, A03-R, C04, I01, I02, MAINTENANCE-CLOSEOUT, R01`。
- 77. `LS06`（P7，A03）：8–13日，累计 259→267 日 / 427→440 日；依赖`A03-R, C04, MAINTENANCE-CLOSEOUT, X01-C`。
- 78. `U08`（P7，U08）：3–5日，累计 267→270 日 / 440→445 日；依赖`D12, I01, I02, I03, MAINTENANCE-CLOSEOUT, U02`。
- 79. `CE05`（P7，CE05）：7–11日，累计 270→277 日 / 445→456 日；依赖`CE02-D, G04, I02, I03, MAINTENANCE-CLOSEOUT`。
- 80. `R06`（P7，R06）：5–8日，累计 277→282 日 / 456→464 日；依赖`I01, MAINTENANCE-CLOSEOUT`。
- 81. `CE03`（P7，CE03）：9–14日，累计 282→291 日 / 464→478 日；依赖`CE02-D, CE05, I01, I02, I03, MAINTENANCE-CLOSEOUT, R06`。
- 82. `I04`（P7，I04）：4–8日，累计 291→295 日 / 478→486 日；依赖`D07, D08, I01, MAINTENANCE-CLOSEOUT`。
- 83. `CE04`（P7，CE04）：13–20日，累计 295→308 日 / 493→513 日；依赖`CE03, CE05, MAINTENANCE-CLOSEOUT, R01`。
- 84. `D17`（P7，D17）：4–7日，累计 308→312 日 / 486→493 日；依赖`D08, D10, D11, D15, I04, MAINTENANCE-CLOSEOUT, R01`。
- 85. `G05`（P7，G05）：4–7日，累计 312→316 日 / 513→520 日；依赖`G03, MAINTENANCE-CLOSEOUT`。
- 86. `CE02-L-P7`（P7，CE02）：2–3日，累计 316→318 日 / 527→530 日；依赖`A03, CE02-L-P63, CE03, CE04, CE05, MAINTENANCE-CLOSEOUT`。
- 87. `LS05`（P7，I04）：4–7日，累计 318→322 日 / 520→527 日；依赖`I04, MAINTENANCE-CLOSEOUT, X02-F`。
- 88. `D18`（P7，D18）：3–5日，累计 322→325 日 / 536→541 日；依赖`C04, D01, D04, MAINTENANCE-CLOSEOUT`。
- 89. `E01-BASE`（P7，E01）：3–6日，累计 325→328 日 / 530→536 日；依赖`A01, D06, D11, I02, MAINTENANCE-CLOSEOUT`。
- 90. `E09`（P7，E09）：3–5日，累计 328→331 日 / 541→546 日；依赖`G05, I03, MAINTENANCE-CLOSEOUT`。
- 91. `X01-H`（P7，X01）：3–5日，累计 331→334 日 / 546→551 日；依赖`C04, G03, G06, MAINTENANCE-CLOSEOUT, X01-B`。
- 92. `E02`（P7，E02）：2–4日，累计 334→336 日 / 551→555 日；依赖`C02, G05, MAINTENANCE-CLOSEOUT`。
- 93. `R07`（P7，R07）：2–4日，累计 336→338 日 / 555→559 日；依赖`G06, I01, MAINTENANCE-CLOSEOUT`。
- 94. `A05`（P8，A05）：5–8日，累计 338→343 日 / 565→573 日；依赖`A03, A04, MAINTENANCE-CLOSEOUT`。
- 95. `R04`（P8，R04）：3–6日，累计 343→346 日 / 559→565 日；依赖`I02, I03, I04, MAINTENANCE-CLOSEOUT`。
- 96. `A06`（P8，A06）：3–5日，累计 346→349 日 / 573→578 日；依赖`A05, I02, I04, MAINTENANCE-CLOSEOUT`。
- 97. `A07`（P8，A07）：7–12日，累计 349→356 日 / 578→590 日；依赖`A05, A06, I03, MAINTENANCE-CLOSEOUT, R01`。
- 98. `R03`（P8，R03）：5–8日，累计 356→361 日 / 590→598 日；依赖`I01, I02, I03, MAINTENANCE-CLOSEOUT`。
- 99. `E04`（P8，E04）：6–10日，累计 361→367 日 / 598→608 日；依赖`D11, D13, I03, MAINTENANCE-CLOSEOUT, U06`。
- 100. `E06`（P8，E06）：5–9日，累计 367→372 日 / 608→617 日；依赖`D11, D14, I04, MAINTENANCE-CLOSEOUT`。
- 101. `E03`（P8，E03）：3–6日，累计 372→375 日 / 617→623 日；依赖`C05, D09, MAINTENANCE-CLOSEOUT, R04`。
- 102. `R02`（P9，R02）：6–10日，累计 375→381 日 / 623→633 日；依赖`D17, I02, I03, MAINTENANCE-CLOSEOUT, R01, R04`。
- 103. `R05`（P9，R05）：4–7日，累计 381→385 日 / 633→640 日；依赖`D11, D14, MAINTENANCE-CLOSEOUT, R02`。
- 104. `E08`（P9，E08）：2–3日，累计 385→387 日 / 640→643 日；依赖`I04, MAINTENANCE-CLOSEOUT, R04`。
- 105. `CE02-L-P9`（P9，CE02）：2–3日，累计 387→389 日 / 647→650 日；依赖`CE02-L-P7, E08, MAINTENANCE-CLOSEOUT, R02, R05`。
- 106. `E07`（P9，E07）：2–4日，累计 389→391 日 / 643→647 日；依赖`G06, I01, MAINTENANCE-CLOSEOUT, R02`。
- 107. `E05`（P10，E05）：4–8日，累计 391→395 日 / 650→658 日；依赖`D03, I04, MAINTENANCE-CLOSEOUT, R04`。

## 验证与后续详设

仓内测试覆盖价格替换/去重、原范围保留、缺依赖、环、资源不重叠、端点、确定性及R待条件公共路线。
本轮先用旧范围应用行为跑红，再补实现到绿；红绿原记录留本机ignored，正式代码和测试在仓内。
每包开工仍冻结准确DTO/schema、迁移和正负边界/授权用例，测试先行；远期设计允许分批深化。
排程计算/文档绿不代替原完整acceptance或UI六件门禁，真实运行与部署没有因本页生成而完成。
