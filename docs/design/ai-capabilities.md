# AI能力接入与领域智能体设计

日期：2026-11-11。状态：规划修订，未实现AI/MCP/领域Agent功能。按[平台边界](platform-evolution.md)执行：已有模型/Agent/知识/SOC等供给优先；模型仅API、内网仅内部服务；保留大量mock；允许外部Agent与本平台领域Agent双轨。

本文件定义本平台必须拥有的业务连接、工具授权、领域配置与受控运行合同。唯一排期和估算见[总账](../review/planning-backlog.md)的A01–A07、I01–I03、R01–R02。P6.3/A02仅当前用户交互确认建单；无人值守、跨服务委托和MCP写工具须等I02及相应合同完成。R01在P6.1先于R3联动和任何不可丢写意图交付。

## 1. 模型与 Agent 生命周期设计建议

### 1.1 模型连接只负责调用已有能力

模型配置对象最小包含：逻辑别名、provider 类型、endpoint 引用、凭据引用、网络/数据区域、可用模型标识、能力声明、超时、并发/速率、请求/输出上限、启用与发布状态。密钥不进入 Agent 配置、普通 API 响应、提示词、工具参数或日志。

Provider 端口提供平台真正消费的生成与工具调用能力，流式、结构化输出、视觉、embedding、rerank、usage 统计按能力声明。OpenAI-compatible 只是一个可选 adapter，不能假定内部模型全部兼容；不支持工具调用的模型先用于只读摘要，不靠脆弱的提示文本模拟无限制工具权限。模型可以经企业网关、原生 HTTP/SDK 或其他现成远端接口适配。

网络按部署配置明确内网/外部路由、TLS、代理、允许目的地、重定向和 DNS 校验；内网任务不自动降级到外部模型。fallback 只在同一数据与授权区域、符合所需能力的配置间发生，并在运行中记录。Provider alias 的配置版本和实际返回的模型标识分别留证；供给方不提供真实模型修订号时记为未知，不能承诺完全可重复输出。

**窗口：P6.0（A01/A03前置合同） 合同与 mock；P6.2/A01 首个模型调用。** 首版不建设独立大模型网关产品，不规划算力、部署、训练或自建 embedding 服务。

### 1.2 Agent 接入配置与平台业务场景的生命周期

先区分外部 Agent 定义与本平台接入配置。若已有 Agent 服务拥有提示、知识、skill、运行图和评测，本平台只存其连接、稳定 Agent/version 引用、输入输出 schema、可提供的上下文、允许平台工具、委托/确认/预算与结果留存策略，不复制另一个 builder。直接调用已有模型的固定查询/建单场景可有少量版本化提示模板和确定性应用步骤，不要求先搭完整 Agent runtime。

平台接入/场景配置生命周期建议：草稿 → 合成测试/contract 验证 → 发布不可变版本 → 灰度/启停 → 退役。外部 Agent 自身发布由其属主负责，平台引用明确版本；如对方只提供可变 alias，记录兼容风险并限制自动写入场景。凭据轮换、账号停用和权限撤回属于实时安全约束，不能被版本快照冻结。

首版以一个固定只读场景为单位；外部Agent与领域Agent双轨配置在P8/A05交付。平台 invocation 至少分清受理、运行、等待确认、成功、失败、取消请求、已取消、结果未知，保存外部 run ID 和必要工具/效果记录。外部运行图/checkpoint 是其 runtime 的职责，不在平台复制。多 Agent、自动发布 Agent、长期自治与自我修改 skill 不纳入；新增接入不能直接取得配置人的所有权限。

配置发布和执行授权分开：能配置 Agent 不等于能以它读取所有人的工单；能运行 Agent 不等于能执行所有写工具。会话以当前用户委托运行，后台任务以受限持续委托运行。平台审计记录配置、外部版本、业务计划与工具效果，不保存模型私有思维过程。

**窗口：P7/A03–A04 场景与接入配置；P8/A05–A07 接企业 Agent/知识，同时交领域 Agent 最小配置与受控 host。** P6.2/A01 先用内置固定场景，不建通用 Agent builder；P8 使用成熟组件并将必要的领域增量限定为下一节。

### 1.3 领域 Agent 的最小受控 host

平台自建 Agent 仅面向本平台独有的安全生命周期任务，例如汇总本人风险/工单、结合已授权情报提出整改草案、将自然语言变成可确认的工单/计划任务。其配置包括目的、模型连接、提示版本、工具/skill 引用、业务上下文、最大轮次/时长/工具调用/token预算、确认策略、允许数据区域、输出 schema 和启停状态。

host 最小职责为读取已发布配置、构建受权上下文、调用已有模型 API、验证结构化工具建议、经统一门面执行允许的工具、记录观察结果、达到预算/完成条件即停。写操作先生成精确草案与批准凭证；模型不能直接调用 repo 或取得管理员密钥。优先复用成熟 SDK/执行方案，不开发通用图语言、插件市场、任意脚本内核、无限多 Agent 协作或知识索引。

跨请求/人工等待必须保存配置版本、已完成 step 的结果引用、下一步状态、待确认摘要及 run/attempt；恢复时不重执行已完成副作用，模型新生成的计划不能沿用旧批准。运行记录的恢复是有限状态持久化，不以此默认建设通用 DAG 引擎。TR 仍是计划任务的唯一调度主。

领域 Agent 与外部 Agent 共用业务工具授权、输入输出、运行关联和效果审计；外部 Agent 调用平台工具也必须走相同门面。两轨的配置/API 可以统一入口，但外部运行真相和自有 run 真相分别由各属主持有。

### 1.4 复用方案的有界比较，不先锁框架

P8 开工前用同一组模型适配、工具授权、暂停确认、恢复不重复副作用、预算耗尽、取消和版本升级样例选型。优先级为企业 Agent API 覆盖通用场景；本平台领域 host 选成熟组件的最小子集；不自行开发可视化 Agent 图语言或通用调度内核。

- **Go SDK 路径**：Google ADK Go 提供模型接口与自定义 Agent 运行入口，可作为与现有 Go 代码同语言的候选；其模型数据结构引用 genai 类型，不能据“有接口”就声称内网模型零适配或无锁定成本。仅通过 adapter 隔离并验证实际模型/工具/会话能力后选择，不假定必须用 Google 模型或云服务。[官方模型接口](https://github.com/google/adk-go/blob/main/model/llm.go)、[官方自定义 Agent 文档](https://github.com/google/adk-docs/blob/main/docs/agents/custom-agents.md)
- **独立 Python/TS 组件路径**：LangGraph/LangChain 有现成 Agent 与持久化/暂停恢复能力，适合确实超出固定应用步骤的领域 host；代价是新增语言、依赖与受控运行进程。恢复会重进节点，节点内已发生的副作用仍需平台幂等，不能因框架有 checkpoint 就删除业务凭证。[官方中断 API](https://reference.langchain.com/python/langgraph/types/interrupt)、[官方图 API](https://github.com/langchain-ai/docs/blob/main/src/oss/langgraph/graph-api.mdx)
- **确定性应用步骤路径**：P6/P7 的只读查询、草案生成和确认后提交，直接复用现有模型 Provider、业务门面和 TR 即可，不为了这些固定切片引入完整 Agent runtime。此路径仍须版本、错误/幂等、权限和最小等待状态，不以“简单”豁免质量约束。

**2026-11-13后继补研**：已按[领域Agent代表场景](domain-agent-reference-scenario.md)运行确定性步骤、企业Agent协议stub、有限host模型三条路径；当前35个场景×3为105个Agent检查，另有skill/SOC检查。先前A20–A25补知识来源撤权/版本/digest与receipt当前权限；本次A26–A35再补apply入口重放旧效果的当前读权/最小投影和不重复执行，旧57/75条不冒称包含后继竞争窗口。当前固定摘要/草案首选确定性步骤，保留A05/A07双轨及有限host范围。没有调用真实模型/企业Agent或安装实际Agent框架，不能声称任何SDK/厂商已适配；真实质量/配额/取消/登录仍归接入门。候选名称不是新增依赖清单。

## 2. 自然语言查询、建单和计划任务的三个独立切片

### 2.1 查询：先走结构化业务接口

首场景选已有数据，例如“我负责的未关闭高优先级工单有哪些”或“这个 IOC 的情报是什么”。模型只形成受限 QueryPlan：资源、白名单字段、已支持的过滤/排序/分页/聚合、时间窗和输出格式。服务端校验并注入当前调用者范围，再调用已有业务查询。

不以 text-to-SQL 访问生产数据库，不开放任意 URL 抓取或通用 SQL 工具。未知字段、歧义时间、无权范围和缺失数据须返回可解释结果；计数/导出/摘要也使用相同行级范围。需要明细证据时带业务链接与查询条件，不能让模型编造记录或把不可见对象存在性当提示。

**验收**：合成多组织账号、撤权、空结果、分页、时间边界、非法过滤、长输入与无权全局统计；模型输出格式错误时没有查询副作用。

### 2.2 建单：自然语言生成可审阅的业务草案

自然语言提取类型、组织、标题、描述与字段。业务服务按当前发布 schema 做完整校验和预填；缺少必填字段或存在歧义时收集补充信息。展示最终工单草案和影响范围，用户确认后执行。确认绑定 actor、目标、类型/定义版本、输入摘要、有效期与幂等键；草案改变或权限撤回后重新校验。

Agent 的计划状态是智能层自己的草案，不新增工单 `draft` 业务状态作为实现捷径。创建成功后返回真实工单编号和链接；响应丢失时按操作 ID 查询，不重新生成一张单。批量创建按行报告与业务批次幂等，不因模型重试重复建单。

**验收**：缺字段、类型不可见、越组织、重复确认、确认过期、类型更新、处理中撤权、提交成功但返回丢失，均与手工建单同一规则。

### 2.3 计划任务：先生成明确调度计划，再进入唯一调度主

“以后每天九点查询并给某组建单”需要分清任务定义、触发时刻、业务动作、通知与停止条件。草案应明确 IANA 时区、开始/结束、频率、未来若干触发时间、目标范围、最大次数/预算、重复触发策略、异常与停用规则。含糊自然语言不得静默变成永久 recurring job。

使用 taskrunner 做业务调度，维护扫描仍用主仓 cron；不再给 AI 另建调度器。确认后持久化受限委托与任务版本；每次执行重新核验委托是否有效、资源权限和动作约束。用户停用或授权取消时暂停任务，不能无限沿用创建时角色快照；允许的持续执行权限必须由明确 grant 表达，不能要求未来每次都复用过期浏览器 token。

**验收**：重复 tick、错过执行、时区边界、计划更新与旧任务、取消竞争、授权撤回、外部结果未知、任务成功但结果通知失败分别留证。

**实施窗口：查询为P6.2/A01；当前用户交互确认建单为P6.3/A02；持续委托与自然语言计划任务为P7/A04。** 后两者以前述统一权限与可靠意图合同为前置，不能只在聊天界面加一个 function-calling 开关。

## 3. MCP、Action 与身份委托的推荐边界

MCP server 是协议 adapter；核心资产是平台自己的有版本能力合同。一个能力可由表单、MCP、工单动作或 Agent 调用。MCP 不替代 REST API，也不要求把所有管理路由改写。外部 MCP client 对接只在需要调用受信外部工具时加入，并单独管理服务器地址、身份、允许工具、版本和出站范围。

Action 合同建议分四部分：

- 定义：稳定 ID、版本、owner、输入输出 schema、短/长执行模式、业务风险级别和兼容性。
- 策略：调用者资格、委托类型、目标资源约束、确认要求、频率/预算、凭据 scope。
- 路由：执行端、超时、允许重试类别、幂等约定和取消能力。业务目录的写主在主仓能力模块，TR 只持有执行路由投影及运行事实。
- 结果：稳定 invocation/operation ID、运行与 attempt、权威终态、结构化结果/摘要、日志/工件引用、未知外部效果及对账入口。

MCP tools/list 与 tools/call 复用同一授权策略，但调用时仍重新执行三层/领域校验。工具 readOnly/destructive 等注解帮助体验，不能作为可信权限。权限有效集合应是用户实时权限、受限服务主体、Agent 配置允许集、委托 grant 和本次目标约束的交集；无人值守任务只按显式 grant 运行，不冒充一次真人点击。

身份至少区分发起人、实际执行主体、on-behalf-of、调用来源、批准凭证、run/step/attempt 和 request/trace。现有 X-Operator 是审计断言，不是完整 grant。新增签名字段或上下文必须纳入签名/可信传输合同；不得仅在 JSON body 声明管理员身份。

MCP 通用客户端的 HTTP 授权按当时适用的 MCP/OAuth 规范实现并验证 audience、资源与 scope；现有业务 JWT/内部 HMAC 不自动等价于完整通用 MCP 授权。协议专用路由与响应保持 MCP 自身语义，按 standards 第三方协议防腐层登记，不把 JSON-RPC 强行装进平台业务信封，也不为此翻改全平台 GET/POST 公约。

**版本研究结论（2026-11-11）**：官方当前入口为 2026-07-28，已有每请求元数据、传输等变化，官方 Go SDK 列有跨版本兼容表。实施时钉所支持规范/SDK/客户端组合和 conformance 用例；不根据旧博客生成协议实现。[MCP 规范](https://modelcontextprotocol.io/specification/2026-07-28)、[官方 Go SDK](https://github.com/modelcontextprotocol/go-sdk)、[传输规范](https://modelcontextprotocol.io/specification/2026-07-28/basic/transports)

长任务先把 invocation 查询、授权取结果和取消能力做稳；客户端支持时再映射 MCP Tasks 扩展，不要求首批所有客户端都支持。MCP transport cancel、runtime cancel request 与外部副作用已取消不是同一事实。[Tasks 扩展](https://modelcontextprotocol.io/extensions/tasks/overview)

**窗口：P6.0定最小身份/能力合同；P7/A03-R同时交两个只读发布工具和一个外部只读消费样本，A03-W在精确批准/receipt门后交有限确认写适配；P8/A05–A07扩真实企业Agent、逐主体ACL/缓存、长任务及Tasks/Skills。** 不再将外部MCP消费整体后移P8。准确首批工具、Schema、SDK/transport、OAuth与本地IAM路径见[首批合同](mcp-first-contract.md)及[受控样本](mcp-first-contract.schema.json)。官方SDK的合成HTTP回环互操作已有独立参考证据；产品能力、真实企业AS/本地身份桥和真实专业提供方联调仍未完成，不能将两种验证混写成全部未验或全部已接通。内部UI第一条查询仍可直接消费同一能力层。

## 4. RAG 与 skill：优先复用供给，保留实时权限

### 4.1 RAG 接入既有知识平台，不重建知识检索产品

用户已确认企业知识平台存在，具体 API/ACL/引用/配额合同未知。工单列表、计数、风险状态与计划任务走实时业务 API；操作手册、已批准处置知识和非结构化证据问答优先调用企业知识 API，不再建设另一套解析、chunking、embedding、向量存储、rerank 与知识发布平台。

平台保留薄知识连接配置：知识集合/来源引用、数据分类与允许区域、调用者/委托映射、查询范围、引用版本、必要缓存与结果证据。知识源和索引的写主仍在外部服务。契约需明确检索时按谁的权限过滤、源删除/撤权何时生效、是否返回可核验 source/version、缓存与引用读取如何再授权。

若外部 API 仅支持一个全局服务账号且无法裁剪来源，不能向普通员工开放跨组织检索。可先限制为已批准的公共/共享知识集合，或退为受权深链，在 P8 接入窗口明确能力缺口；不能把所有内容取回来后靠 prompt 保密。平台业务资源输送给外部检索时仍须按实时权限最小化、分类与出站策略裁剪，AL 无统一行级模型的数据不能默认全部索引。

引用须真实且当前可见，无证据时返回不足。外部知识/工具输出可能含间接 prompt injection，作为不可信数据，不赋予修改权限或执行工具的能力。派生摘要和缓存继承来源分类与权限；源失效后阻止继续使用相关结果。

**窗口：P7/A03–A04 固定知识平台合同与合成检索集；P8/A05–A07 接首个受权知识场景。** 若 API 不可用，保留 mock + contract 与受权深链；记录缺口和下一次核验，不直接改成自建 RAG。任何本地索引例外须证明现有平台无法覆盖且有明确业务收益，另列决策，不纳入默认排期。

### 4.2 skill 是领域操作知识包，不是权限或市场

领域 Agent 可采用 `SKILL.md` 元数据、操作说明和引用资产的可移植格式，承载本平台业务步骤、例子和约束；企业 Agent 平台已有的 skill 管理优先引用，不镜像另造市场。工具仍走统一受权 Action/Query，skill 不能赋权。少量领域 skill 可直接在受控版本库维护，不必先开发复杂管理界面。

skill 记录来源、owner、版本摘要、兼容性、评测、发布/停用与依赖。首版仅接可信指令/资产；脚本内容复用专业执行/SOAR 或经另行证明有缺口的隔离 runner，绝不在主进程直接执行下载包。`allowed-tools` 是格式声明，不是授权；规范当前也将其标为实验字段。[Agent Skills 格式](https://agentskills.io/specification)

MCP Skills 扩展是可选分发/发现方式，不代替版本、信任和执行权限。按实际客户端需求决定是否接，不为协议完整度而建设目录市场。[Skills Over MCP 官方说明](https://modelcontextprotocol.io/community/working-groups/skills-over-mcp)

**2026-11-13后继补研**：[可信skill样包及恢复合同](skill-reference-package.md)已补完整SKILL.md/manifest、输入输出、来源/digest/钉版、撤销/升级/暂停恢复及16个合成行为例；[SOC职责矩阵](soc-collaboration-authorization.md)补事件协作授权和46个合成反例。两者不是产品loader或真实SOC接通证据。

**窗口：P7/A03–A04 复核实际能力目录/知识供给，P8/A05–A07 实现可信 skill 与产品回归。** 研究样包不再挂为未开始；真实SDK/知识ACL与撤销传播仍按接入门验证。公共 marketplace、自动安装、自我修改、通用技能编排明确退出默认路线。

## 5. 工单流程、专业编排与 Agent 流程如何统一

建议统一业务入口、工具授权和运行关联，显式保留三类语义：

- **工单 FSM**：本平台管理业务对象状态、责任人、转换资格、字段/证据、人工复核与 SLA。
- **确定性任务工作流**：专业编排/SOAR 管依赖、并行、等待、重试与恢复；本平台只触发所需链并消费状态/结果。TR 继续承担自身既有计划任务和 handler 调度，不扩成另一 SOAR。
- **Agent 运行**：外部企业 Agent 由其 runtime 负责；本平台领域 Agent 的有限 host 管有界模型/工具循环、预算、确认和最小恢复状态，不自带通用 DAG 语言。

共享部分包括：稳定能力 schema、身份委托、批准引用、业务输入输出、错误词表、run/trace 关联、结果/日志/工件受权引用、可见状态和证据。业务状态写主、专业执行推进和模型决策分别归属，不通过合表/万能画布消除差异。

历史 D22“一套画布+Action词表+两执行语义”改为“共享设计系统、能力合同和运行关联；按工作类型显示表单/状态图/受权深链/Agent 配置”。**不默认自建任务流程画布，不强制 FSM、SOAR 与 Agent 共用引擎。** 专业流程可跳至其系统编辑，平台展示调用与结果及其对安全生命周期的影响。

工单可触发一个专业 workflow 或 Agent invocation；Agent 可提出新工单/整改计划，业务写仍经本平台授权和不变量。每链指定一个编排主和一个重试主，防 SOAR、on_success、TR 与 Agent 同时续链。模型和外部 API 作为有记录步骤，恢复时不重复已完成副作用；变更计划不能复用旧批准。

Temporal 对确定性 workflow 与非确定性 Activity 的区分可供合同设计借鉴，但不构成引入 Temporal 或自建其替代品的决定。[Temporal workflow 定义](https://docs.temporal.io/workflow-definition)

**窗口：P6.0（A01/A03前置合同） 冻结语义词表；P8/A05–A07 复用成熟组件交有限领域 host；P9 完成与专业编排的恢复/取消/升级协同。** 旧 Phase9 自研 DAG 与 Phase10 强制 SOAR 迁回不再是默认交付，必须在明确缺口、成本与业务收益证据后另行裁定。

## 6. 评测、可靠性与验收出口

普通 CI 应始终能使用合成 mock：固定模型回复、错误/超时/限流/截断、schema 非法、tool-call 重放、恶意文档、权限撤回、模型不支持能力。mock 只替代供给行为，不绕过授权/状态机/事务/幂等。contract suite 验证 provider、MCP、Action/Query、dispatch/report 和结果 schema；real 验证单列环境、模型、配额、日期与真实未知。

模型质量评测单列：意图识别、工具和参数正确性、必要澄清、拒绝无权操作、引用真实性、检索相关性、总结遗漏、任务达成与预算/延迟。用版本化合成黄金集，在 P6.2/A01 建基线与阈值，新增权限与攻击回归作为硬门槛。单次演示或“都通过”不代表任意 prompt 安全；评测集与实际上线质量限制应可见。

首个写工具上线前必须覆盖：重复确认/响应丢失不重复副作用、输入/目标/版本变更使批准失效、实时撤权、远端成功本地未知、陈旧 report/attempt、取消与完成竞争、预算耗尽、进程退出后的恢复。事件重投、任务重执行、业务重试和模型重试分别命名并选唯一重试主。

观测关联 UI request → Agent run → model step → tool invocation → TR attempt → 业务事件/工单；默认只记录必要元数据、计划摘要、输出引用与效果，不把凭据、敏感文档和完整提示自动发送到第三方 tracing。费用有 provider usage 则记录事实，无 usage 时明确估计；持续委托必须有用户可见的停用、暂停与异常入口。

配置晋级复用既有“不可变发布版本、目标侧依赖校验、计划绑定 baseline、失败补偿”的原则，但不把全部 AI 配置直接塞进当前一次性工单定义包。P7/A03–A04 另定模型/Agent/skill 的非秘密配置包与 secretRef 映射合同，运行中版本保持稳定，权限与密钥吊销实时生效。

## 7. 公开一手研究与采纳取舍

检索仅使用公开通用概念，没有上传源码、设计、真实接口或内部资料。研究结果支持设计原则，不证明本项目已经满足相应规范。

- **MCP 官方规范与 SDK**：采纳协议 adapter、显式能力/版本兼容、独立授权与结构化工具结果；不手写一套“类似 MCP”的 API，不把 SDK 本身当业务授权。[规范](https://modelcontextprotocol.io/specification/2026-07-28)、[工具](https://modelcontextprotocol.io/specification/2026-07-28/server/tools)、[Go SDK](https://github.com/modelcontextprotocol/go-sdk)
- **MCP 安全实践**：采纳 audience/资源隔离、禁止 token passthrough、收敛出站目的地与每次工具授权；不把外部 MCP server 的工具注解当安全事实。[官方授权](https://modelcontextprotocol.io/specification/2026-07-28/basic/authorization)、[安全实践](https://modelcontextprotocol.io/docs/2026-07-28/tutorials/security/security_best_practices)
- **Agent Skills**：采纳可移植的指令/资产格式和渐进加载；不把格式中的工具声明当权限，不自动执行公共包脚本。[格式规范](https://agentskills.io/specification)
- **Anthropic 有效 Agent 模式**：采纳按可预测流程与开放决策区分、从小而有界场景验证；不因该文提及某框架就引入依赖。该页自身说明 2024 年工具版图已变化，本轮仅采用稳定的模式区分。[原文](https://www.anthropic.com/research/building-effective-agents)
- **LangGraph 持久化模式**：采纳短期运行检查点与跨运行存储分离、显式暂停恢复；不默认引入 Python/托管服务，也不把 checkpoint 等同业务事务或副作用恰好一次。[官方持久化](https://docs.langchain.com/oss/python/langgraph/persistence)
- **Go ADK 模型与自定义运行接口**：纳入领域 host 的同语言复用候选；显式评估 genai 类型耦合、内网模型 adapter 和会话持久化成本，不默认上 Google 模型或云。[模型接口](https://github.com/google/adk-go/blob/main/model/llm.go)、[自定义 Agent](https://github.com/google/adk-docs/blob/main/docs/agents/custom-agents.md)
- **Temporal 确定性与 Activity**：采纳把模型和外部副作用隔离为有记录步骤、定义版本与恢复兼容；只借鉴恢复原则，不默认引入引擎或重建其替代品。[官方定义](https://docs.temporal.io/workflow-definition)
- **微软 RAG 文档级权限模式**：采纳查询时授权与权限同步的合同要求；向既有知识服务核验能力，自定义 IAM 的 live 授权仍由本平台执行。不自建重复索引，不把 Azure preview 写为依赖。[文档权限概览](https://learn.microsoft.com/azure/search/search-document-level-access-overview)
- **OWASP 过度代理风险**：采纳最小工具能力、最小权限、危险动作确认、监控与测试；不声称提示词过滤可以根除注入，也不增加与本项目无关的大型合规建设。[OWASP Excessive Agency](https://genai.owasp.org/llmrisk/llm062025-excessive-agency/)

具体A01/A02逻辑操作、schema/精确确认/receipt/错误与有界运行合同见[AI调用详设候选](ai-invocation-contract.md)；后期实际SDK/提供方/状态存储按各包窗口冻结，非全部已完成。
