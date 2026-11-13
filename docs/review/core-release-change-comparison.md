# 首个可用版本：以前怎样、现在补什么、为什么做

日期：2026-11-14。面向所有者：本页看变化与取舍，[主计划](../phase6/first-usable-release-plan.md)看首版范围、阶梯和日期风险。2027年1月试用、2月1日前可用是已确认目标；下列新增变化仍待实施，不能据设计落档声称已接通或单席能够按期完成。

依据本轮[业务四链复核](reports/core-release-chain-recheck-2026-11-14.md)、[三层/UAA复核](reports/core-release-uaa-recheck-2026-11-14.md)、[UI与试用复核](reports/core-release-ui-trial-plan-2026-11-14.md)及其源码符号。旧[2026-11-12路线对照](roadmap-change-comparison-2026-11-12.md)保留当时比较，不倒写成今日首版范围。

“基础必做”指已启用场景能正常使用且不损坏数据、不越权、失败可处理的下限；“场景必需”随本次选定联动落地；更完整的厂商功能和罕见复杂分支留后继，不把产品功能全集当基础门槛。

## 十二项变化

1. **已有底座与日常界面｜基础必做。** 现状：Phase5已有工单、TR、AL和消息页面，但普通Logo落点、部分失败反馈和新增业务入口仍有缺口。首版：固定可入网试验的组合，修正常首页，复用现有组件交最小业务UI。收益：普通人能完成任务；代价：需改关联页并做真实角色回归。状态：旧页面已有，新接线/修正待做；[UI合同](../../../zhuzhao-ui/docs/design/frontend-implementation-contract.md)与[基础边界](../design/foundation-release-boundaries.md)。全站U06视觉推广后续逐页交。

2. **私有adapter与核心更新｜内网二开基础必做。** 现状：已有部分接口/签名机制，尚无完整可运行的外置接缝，不能宣称HR等只换endpoint就好。首版：公司协议、SDK、字段映射和凭据留私有进程/仓，经稳定API/Port调用核心；所用adapter按受审配置登记，完整私有应用宿主治理不挡首链。收益：核心升级少带公司分叉，厂商变化集中改adapter；代价：多一处配置、运行及合同测试。状态：目标待实现；[扩展兼容](../design/extension-compatibility.md)、[内网迁移](../design/intranet-adapter-migration.md)。

3. **同一能力的一条受控调用路径｜所选能力基础必做。** 现状：HTTP、工单动作与未来后台入口的授权/结果接线覆盖不同。首版：本次人工、工单、worker、adapter入口复用同一领域用例和结果合同。收益：换入口不改变权限和完成含义；代价：需抽取本批必要端口与关联身份。状态：共同runtime未交付；[能力调用合同](../design/capability-invocation-contract.md)。158项存量迁移和MCP产品化按原路线继续，不挡首条链。

4. **从有菜单到对每个对象/动作判权｜基础必做。** 现状：工单已有L1路由功能、L2资源范围、L3动作资格；签名网关不等于风险、AL和任务都已接同样数据权限。首版：逐入口验风险派单、任务结果、AL行/字段与消息的当前资格，计数/导出同范围。收益：普通账号可安全协作；代价：后台查询/写入都要接权限并验负例。状态：既有工单机制复用，其余所选链待补；[生命周期合同](../design/business-lifecycle-contracts.md)、[UAA复核](reports/core-release-uaa-recheck-2026-11-14.md)。

5. **本地IAM与公司UAA双模式｜企业启用必做。** 现状：本地账号/角色/菜单授权可用，真实SSO、三feed与来源投影未做，57例离线模型仅证合同。首版：独立模式继续可用；企业模式对接受管账号/角色/资源，纳管部分只读，停用/撤权及待生效可查。收益：公司治理与本地业务权限各有写主；代价：真实协议映射、会话/策略收敛和公司联调不可省。状态：公共与私有产品接线待做，真实条件未定；[UAA合同](../design/uaa-governance-integration.md)。

6. **专业结论变为可管的风险闭环｜本次业务必交。** 现状：有工单底座，没有运行中的风险实体、周期、关联和页面。首版：一个来源/规范化输入，风险评估、责任、受限派单、整改证据、复核与逐风险结论，保留有界N:1。收益：知道谁处理、处理到哪、凭什么关闭；代价：这是新增业务与迁移，不能按改URL估工。状态：待实现；[业务四链§2](reports/core-release-chain-recheck-2026-11-14.md#2-风险源--风险--整改单)。专业发现/研判继续取外部平台，不自建SOC。

7. **工单定义、字段和证据可追溯｜所选写路径基础必做。** 现状：配置和执行仍有快照、字段采集、附件引用等待完善点。首版：本批所用定义有稳定版本/摘要或服务端冻结配置守卫，转换校验、当前负责人资格和证据引用一致；要改配置须显式发布/迁移，修改/流转失败不留半套事实。收益：配置更新不悄悄改变在途票，证据不误删；代价：补所选配置的版本/冻结守卫、事务与兼容，不先建完整迁版框架。状态：既有流程/附件复用，目标仍待实施；[生命周期合同](../design/business-lifecycle-contracts.md)、[工程详设](../design/engineering-execution-design.md)。完整设计器与复杂迁版后交。

8. **任务从“已提交”到效果可证明｜异步链基础必做。** 现状：spawn_task能提交，但丢返回关联，字段映射/结果回贴不完整，任意2xx可能被当成功。首版：固定少量Action与输入，持久意图、原task/operation关联、效果receipt及工单结果投影；未知先对账。收益：重启不丢必要动作，重试不盲目重复效果；代价：多保存状态、处理提交未知。状态：TR队列/重试已有，业务接线待补；[跨仓执行](../design/cross-repo-execution-contract.md)、[四链§3](reports/core-release-chain-recheck-2026-11-14.md#3-工单任务--tr--工单结果al更新)。

9. **AL从通用CRUD到受权业务写｜本次联动必交。** 现状：AL已有CRUD/CAS，未有worker可复用的受权后端client；AK/SK不能证明用户可改哪些行/列。首版：只更新所选结构和允许字段，检查当前资格、version及原操作回执。收益：任务结果能安全汇入台账，冲突可解释；代价：要补行/字段范围与去敏，不能直接写AL库。状态：CRUD复用，受权port待做；[四链§3](reports/core-release-chain-recheck-2026-11-14.md#3-工单任务--tr--工单结果al更新)、[互通详设](../design/capability-interconnect-detail.md)。

10. **AL变化能可靠触发TR后继｜本次联动必交。** 现状：没有源变化事实、持久订阅和事件到任务登记，刷新列表或查updated_at不能代替。首版：一条受审订阅，AL源提交后触发一个获权TR后继；建议先选只读复核，避免写回同AL行自激。收益：少一次人工搬运且断线可恢复；代价：需事件去重、因果关联和有限速率。状态：待实现；[四链§4](reports/core-release-chain-recheck-2026-11-14.md#4-al变化--获权任务)。只发通知不能冒称AL→TR已打通，通用规则画布后交。

11. **消息从桩成功到公司HTTP回执｜本次联动必交。** 现状：普通业务装UIPStubSender，release真实发送未实现；已有死信webhook不是公司业务消息sender。首版：一条私有HTTP映射，复用模板/收件规则，保存发送意图、幂等及受理/失败结果。收益：知道真正发到哪一步，通知失败不抹掉已提交业务；代价：要核公司鉴权、业务码及超时查询，条件未定。状态：接口/页面已有，真实sender待做；[四链§5](reports/core-release-chain-recheck-2026-11-14.md#5-业务消息--公司http--发送回执)。不把受理显示成送达/已读。

12. **日志与审计成为业务可用的证据｜基础薄切必做。** 现状：有slog、HTTP审计、执行和发送记录，但不覆盖全部非HTTP/事务事实，部分query/body有敏感值风险。首版：本批日志先去敏，用共同关联找原业务/任务，普通人只看获权历史，专门审计独立授权；失败有薄反馈。收益：能排障又不借日志泄露数据；代价：要明确记录用途、写入和读取边界。状态：旧机制保留，新纵切待做；[日志审计合同](../design/unified-observability-audit.md)。大导出、外送、全链trace和多worker后交，两IAM审计失败回滚仍按待确认项处理。

## “随核心升级少改adapter”的准确承诺

首次先把公开合同、版本和参考adapter真正交付；以后核心在同一受支持合同内升级，原adapter应通过合同测试继续使用，通常不用改或只改endpoint等配置。公司接口变了，修改私有映射；核心新增可选字段，旧消费者按合同忽略，不强迫使用新功能。

若核心改变必填、枚举、默认语义、权限或完成含义，必须显式评审兼容或新合同版本；确有破坏变化时仍可能改adapter，不能保证永不修改。当前未正式部署允许协调替换旧合同；首次正式消费者登记后按实际支持范围给迁移安排，不恢复未采纳的固定6个月承诺。参见[扩展兼容§5–6](../design/extension-compatibility.md#5-兼容边界与合同版本)。

后继加分包括AI/RAG/skill、MCP产品化、高级仪表盘、完整插件治理/配置台、多实例和微服务。它们有价值且保留路线，但不解决本次尚缺的业务事实、当前授权、真实收发与效果回执，故不作首版共同前置；未来启用时再验相应合同。单实例也必须处理并发和重启。

## 全部路线怎样读

这12项解释当前首版的具体补强，不代表全项目所有细节已经审完。[原能力连续性](roadmap-capability-continuity.md)追溯旧功能承接，[旧路线对照](roadmap-change-comparison-2026-11-12.md)保留当时变化，[现行路线](../roadmap.md)说明Phase6–9及更远分期；不因本次首版收敛而取消后继业务效果。

工程化、多agent独立worktree、TDD、目录/命名与一致性继续按[工程治理](../design/engineering-governance.md)和[公约](../standards.md)推进；远期准确DTO与其余细节仍在对应包开工前冻结。后续新增项同样记录现状、变化、收益/代价、基础或增强分类及实现状态，沿现有总账承接，不另建一份平行大全。

## 官方依据支持什么，本项目选择什么

- [OWASP对象级授权](https://api-security.owasp.org/editions/2023/en/0xa1-broken-object-level-authorization/)支持每次对目标对象执行动作都核资格；本项目选择沿已有三层接所选链，不因此自建通用权限平台。
- [AWS transactional outbox](https://docs.aws.amazon.com/prescriptive-guidance/latest/cloud-design-patterns/transactional-outbox.html)解释数据库写与后继通知双写、重复消息及幂等消费者；本项目只为不可丢后继做持久交接，复用PG/Redis/队列，不强迫增加MQ或把每个GET事件化。
- [Microsoft API指南](https://github.com/microsoft/api-guidelines/blob/vNext/azure/Guidelines.md)支持兼容演进；[anti-corruption layer](https://learn.microsoft.com/en-us/azure/architecture/patterns/anti-corruption-layer)支持语义映射隔离，也提示延迟和维护成本。本项目采用版本合同/私有映射，不照搬Azure URL规则、云组件或审批体系。
- [Grafana backend plugins](https://grafana.com/developers/plugin-tools/key-concepts/backend-plugins/)展示独立子进程经协议交互的实例；本项目选择有限进程外HTTP接缝，不引入它的插件框架。以上依据支持机制，不证明本项目已经实现、已获真实授权或某厂商全部feature都是基础必做。

本页不新增报价或缩减旧总量；首版净估/扣重与试用余量按主计划统一收口。已审计、已规划、已实现、已运行和已进公司环境始终分别说明。
