# 领域Agent代表场景与三路径对照

> 日期：2026-11-13。用途：补FD03的同场景样例、实际合成原型与取舍；范围：受权风险整改草案。设计状态：本轮候选；实施状态：仅执行设计模型，未实现产品Agent、未接真实模型/企业Agent。
> 权威：业务不变量归[生命周期合同](business-lifecycle-contracts.md)，公共调用归[能力合同](capability-invocation-contract.md)，排期归总账A01/A02/A05–A07。本文不新建通用Agent平台或重开已确认前提。

## 1. 为什么用这个场景

输入：“整理我负责的高危风险和已有关联的整改工单，依据我有权看的已批准处置手册，列缺口并给出需要补单的草案，先让我确认。”

它依赖本平台特有的组合：当前风险管理范围、已有整改票防重、同owner的N:1约束、类型发布版本、风险revision、当前知识引用权限及精确建单批准。SOC负责专业研判，知识平台负责原知识；本平台不能靠自然语言把一个全局模型账号变成跨组织管理员。

合成数据为risk-a（org-a、owner甲、revision17，无在办票）、risk-b（org-a、同owner，有demo-ticket-9）、risk-hidden（org-b，调用者不可见）。知识返回knowledge-public，拒绝knowledge-secret。正常结果只给risk-a草案和risk-b已有票；不泄露第三条风险/知识存在性，不自动复测、接受、关闭或调用专业处置。

## 2. 有限工具与同一业务路径

原型使用候选静态能力名及contract_version=1：risk.query、ticket.linked.query、knowledge.query、risk.dispatch.preview；确认后的risk.dispatch.apply及operation.receipt由应用层控制，不由skill声明自动授予。名字是本文合同样本，不声称已经注册API；正式目录统一后以映射冻结，不能复制第二套业务实现。

| 步骤 | 可信输出/约束 | 写主 |
|---|---|---|
| 受权查询 | 服务端注入当前主体范围；外部Agent不能指定“system”绕过 | 风险与工单用例 |
| 查已有票 | 只返回风险和工单双方当前可见的关联；有在办关系不重复补单 | 领域桥/工单 |
| 取知识 | 原服务按主体过滤；plan钉id/version/digest，apply前回读当前ACL及相同版本/内容；任一不可确认进入held | 企业知识供给 |
| preview | 同owner、配置组织/类型、最多20风险、revision/定义引用/必填完整 | 风险派单用例 |
| 用户确认 | actor＋精确plan_hash＋配置版本＋期限；改计划不得沿用旧批准 | 平台批准记录 |
| apply/receipt | 再验当前授权与revision；同operation同输入读原receipt，异参拒绝 | 领域事务/可靠执行 |

仅格式化模型可见裁剪后的上下文与工具观察，不含凭据、内部SDK或隐藏推理。plan.knowledge_refs保存封闭对象{id,version,digest}，不再只有ID；摘要、引用快照和批准hash绑定同一份来源内容。结果保存摘要、真实引用、外部run ID（若有）和效果凭证，不保存模型私有思维链。

## 3. 实际运行的三个原型

同一份[fixture](examples/ai-soc-reference/fixtures.json)和[可运行模型](examples/ai-soc-reference/verify.py)分别执行：

| 路径 | 实际运行方式 | A01正常例观察 | 能证明/不能证明 |
|---|---|---|---|
| 确定性应用步骤 | 固定四个受权查询/preview步骤＋合成格式化响应 | 5次工具入口（含apply）、1次合成模型格式化交互、1效果 | 固定旅程可表达；不证明真实模型摘要质量 |
| 企业Agent adapter | JSON序列化外部run/工具请求，经同一门面执行 | 5次工具入口、2次合成协议交互、1效果 | 外部计划也不能绕门面；不是实际厂商API/SDK联调 |
| 有限本地host | 脚本化提议模型按观察推进下一工具，最大工具预算 | 5次工具入口、4次合成提议、1效果 | 有界工具循环/批准/恢复可表达；不是已接实际LLM/ADK/LangGraph |

这些请求次数来自合成状态机，**不能用来估真实token、延迟、费用或Agent推理优劣**。没有为了显示host更聪明而给它更宽权限/不同输入。三条路径用同一业务批准、操作身份和效果账本，模型只是提议者。

原型把checkpoint、工具预算及效果receipt分别落到临时文件再恢复。A16恢复从持久文件读预算，不重置调用次数；A07提交后丢响应按receipt恢复；A08重复确认仅一个效果。临时文件原子替换只验证模型的串行崩溃/恢复语义，不代替生产PG事务、多实例claim/fence或真实outbox验证。

## 4. 比较后的裁定

第一条固定风险摘要/草案旅程采用确定性应用步骤，并接入已有模型服务的API。当前35个合成场景没有显示必须引入动态host的业务效果；固定步骤更少协议/状态面，方便单维护者核验。这是基于该有限任务形态的设计判断，不是模型质量比较结论。

保留用户要求的双轨：外部Agent能满足企业知识/分析时接其API；平台领域Agent可在A05/A07以有限配置和受控host交付，支持平台独有能力组合。不是取消领域Agent，也不先造通用builder、图语言、调度器、市场或多Agent自治。

首个实际host组件在P8按模型工具调用/结构化输出、可持久化状态、取消/版本兼容、依赖与离线包评估后锁定。新语言/独立服务只有明确收益才增设；本FD03研究没有增加产品Go或npm依赖，更没有声称企业Agent合同已适配。

## 5. 可复验的场景与结果

当前35个业务场景×3路径=105条行为检查。原25个场景覆盖preview不写；执行前撤权；批准过期/改计划；风险revision变化；提交成功丢响应；重复确认；取消；注入非法工具；预算耗尽；钉版不可用；风险源不可用；知识无主体ACL；跨组织不可见；checkpoint恢复；同owner合并；不同owner需分计划；超过20需拆批。新增10场景覆盖已提交操作从apply重放时的跨步骤读权/批准/知识变化。

**本轮独立审查发现并修正一个实际模型缺口**：原19场景×3的57条及16条skill例未覆盖“preview之后知识源撤权/改版/改内容”。原apply只验风险revision，plan引用仅有ID。新增A20–A25在加守卫前实际跑出18条失败：9条在知识失效后仍新建效果、6条receipt主体/目标失权仍成功、3条未按当前知识权投影引用及最小receipt。失败与去敏摘要见[回归记录](examples/ai-soc-reference/knowledge-recheck-regression.json)，原始失败和加守卫前源码快照保留本地审计区。这些是设计模型的红绿证据，不是产品缺陷修复。

A20–A22分别在已preview并生成精确批准后撤知识ACL、改变version、同版改变digest：apply逐条回读当前知识权威，要求同一来源仍可见且version/digest精确匹配，否则held、无新效果；不能自动换新版后沿用旧批准。digest绑定所用受权证据字节及固定选取/规范化方式，不对模型改写摘要求hash；提供方不返digest时可由薄adapter在受权取证后计算，无法再验证当前ACL/相同版本内容则held。真实产品复验仍经同一受权知识Query/adapter，并纳入当前执行的超时/流控/总预算；原型的文件权威读取不计为真实provider请求或容量证据。摘要/引用展示也按当前权限与固定来源匹配裁剪，不把失效引用当有效证据。

A23在已有提交效果后撤知识ACL，再从checkpoint恢复：当前仍有operation/目标读权时只返回{operation_id,status,effect_id}，不回传知识ID/版本、内容、plan/hash；知识失效不抹掉已合法完成的效果。A24换actor、A25撤目标读权后恢复读取均拒绝，已完成效果仍为一条。业务效果引用在真实实现中还需以同一当前资源权限回读；本模型只验证明确的当前授权守卫，不证明真实IAM多实例传播。

本次105条Agent、16个skill、46个SOC共167条行为及10个守卫变异通过。旧75条Agent及7变异保留原验证范围；新增A26–A35×3在修正前实际失败21条，随后只对已有效果复用当前读取授权并返回最小receipt，不重新验旧执行批准/知识来否认既有事实，也不产生新效果。另有独立32组直接构造交叉检查，逐字节验证账本不变；不把模型检查当作产品并发回归或真实集成验收。准确来源与首红见[本轮再核](../review/conversation-reaudit.md)。

复跑（主仓根目录）：

```bash
python3 docs/design/examples/ai-soc-reference/verify.py --output .codex/audits/ai-soc-design-20261113/prototype-results.json
node docs/design/examples/ai-soc-reference/validate-schemas.cjs ../zhuzhao-ui/node_modules/.pnpm/ajv@6.15.0/node_modules/ajv
```

[DTO样本](examples/ai-soc-reference/contract-examples.json)与[Schema](domain-agent-reference-scenario.schema.json)是设计原型合同，不是当前HTTP接口。初轮A14误期待知识ACL未成立时已经形成草案；按“未形成可发布草案”语义修正预期，原失败保留本地，不能将其写成产品缺陷修复或有效业务红绿证据。

## 6. 范围、工程量和下一门

A05公开有界范围建议校准为5–8工程日：外部连接/引用配置1–2、领域配置与发布1.5–2、最小UI1–2、合同/拒绝例1.5–2。host执行/skill不在A05重复算。A07建议7–12日，细分见[skill合同](skill-reference-package.md#7-实施与估算边界)。A06受权知识3–5日保持。

数字是给总账集成的同一范围复估输入，**本页不成为第二份工期权威**。实际企业Agent/知识/模型申请等待、私有协议特例及新增语言服务不默含；真实SDK/模型黄金集、限额/延迟与离线制品仍在实际接入门核验。

## 7. 一手研究与采纳

- [LangGraph workflows/agents](https://docs.langchain.com/oss/python/langgraph/workflows-agents)区分预定代码路径与动态选工具；本场景先用前者，不由“Agent”名字倒推动态引擎。
- [LangGraph interrupts](https://docs.langchain.com/oss/python/langgraph/interrupts)明确恢复会重进节点，副作用要幂等/去重；平台receipt不能被框架checkpoint替代。
- [ADK workflow文档](https://adk.dev/agents/workflow-agents/)核查时已将旧template workflows标为被图式结构取代；不照搬旧博客API或直接锁某框架。来源核查日2026-11-13。

## 变更记录

- 2026-11-13：补FD03完整代表场景、三路径可执行合成对照、裁定、恢复反例、预算和复估；真实模型/企业Agent/框架互操作仍未验。后继独立审查补知识引用id/version/digest、apply当前ACL/来源复核及receipt最小投影，新增6×3条实际红绿回归；原57条未覆盖这组竞争窗口。
