# 可信领域skill样包、加载与恢复合同

> 日期：2026-11-13。用途：补FD06的可检查样包、评测和加载/撤销/升级恢复例；范围：风险整改摘要skill，可信指令与只读资产。设计状态：候选；实施状态：合成模型已运行，产品loader/runtime未实现。
> 权威：业务权限与效果归[共同调用](capability-invocation-contract.md)、[AI调用](ai-invocation-contract.md)和[代表场景](domain-agent-reference-scenario.md)，排期归A05/A06/A07。skill不是权限、执行器或市场。

## 1. 完整样包及载体职责

受控样包在[skills/risk-remediation-brief](examples/ai-soc-reference/skills/risk-remediation-brief/README.md)：

- [SKILL.md](examples/ai-soc-reference/skills/risk-remediation-brief/SKILL.md)：兼容Agent Skills的name、description、compatibility、字符串metadata及操作步骤；指令要求使用当前主体受权数据、已有票防重和精确preview。
- [manifest.json](examples/ai-soc-reference/skills/risk-remediation-brief/manifest.json)：本平台分发/信任合同，含语义版本、publisher、发布状态、合同版本、所需能力及文件SHA256、禁止脚本/网络执行和钉版恢复策略。不是对Agent Skills规范增加必需字段。
- [fixtures.json](examples/ai-soc-reference/fixtures.json)及[verify.py](examples/ai-soc-reference/verify.py)：独立预期与可执行设计模型。
- [Schema](skill-reference-package.schema.json)：机器校验manifest；unknown字段/未批准能力/脚本开关不能静默通过。

Agent Skills规范要求SKILL.md前言与正文，name匹配目录；allowed-tools目前是实验字段。这里采纳可移植结构，其声明只影响可用工具的提示，不能向当前主体授予能力。[官方格式](https://agentskills.io/specification)

## 2. 输入、输出与效果

输入为当前用户明确的风险范围/任务目标，以及由服务端组装的授权上下文、模型配置/skill版本、查询预算；不接收任意SQL、URL、shell或外部自称管理员字段。

输出分开：受权风险摘要、已有票引用、知识source/version/digest、待补资料及配置范围内的整改plan。模型措辞不成为SOC研判；缺合法引用不编造内容。该样包自身只有query/preview，用户确认后的apply由平台应用流程执掌；不因SKILL写“创建”就启动写调用。

样包发布必须经过形状检查、可信来源/digest核验、依赖能力解析、评测；发布时间不是授予时间。运行可用能力为当前主体权限、服务主体、委托、已发布配置允许集与本次资源约束的交集。发布者、运行者和批准者分开审计。

## 3. 加载、停用及撤销

| 阶段 | 必查内容 | 失败的可见结果 |
|---|---|---|
| 导入草稿 | 名称/路径/大小/文件清单、禁止绝对路径/逃逸/链接、只读资产类型、manifest形状 | 拒绝且留原因，不执行内容 |
| 候选验证 | 受信版本库/已审提交或发行来源、内容digest、工具合同与黄金样例 | 保持草稿，不能运行 |
| 发布 | 不可变版本与digest、评测收据、owner/来源和依赖快照 | 不以更新alias覆盖旧版本 |
| 开始/恢复 | 当前未撤销、主体与grant有效、配置/能力/输入合同可用 | reject或held，不改latest |
| 紧急停用 | 持久标记并阻止新run/下一工具调用；传播到多实例的实时失效通道 | 运行暂停/取消请求，已提交效果仍保留 |

原型使用受控文件digest与demo-trusted-repo允许集作信任锚，**不是实际供应商签名验证**。生产按已审提交/可信制品来源保持相同效力；真正签名、凭据及内网分发接线在G05/X02复用，不另造公钥/供应链平台。没有可信来源或只有下载地址时不自动安装。

自修改、自动拉取latest、下载脚本执行、公共市场和不经批准添加工具退出默认范围。专业脚本能力走专业执行平台与单独授权；本样包没有scripts目录，也不启动子进程。

## 4. 版本、暂停与升级例

checkpoint至少绑定run/step、精确skill版本/digest、模型配置版本、输入与plan hash、剩余预算和已完成效果引用。业务receipt独立存在，恢复先查当前安全状态，再接续未完成步骤。

| 情景 | 决定 |
|---|---|
| 1.0.0暂停后仍存在且未撤销 | 钉1.0.0原digest恢复，预算不重置 |
| 发布兼容1.0.1 | 新run可选新版本；旧run仍用原版，不自动切换 |
| 原版文件丢失/被retire为不可执行 | held_version；由管理者恢复受信制品或明确重规划 |
| 包被安全撤销 | 停止下一步，不以旧批准复活；已完成效果不删 |
| 输入合同变为2 | held_contract；转换/重规划与新批准后再开新步骤 |
| 同版内容digest变了 | reject_digest；不能以“只是文案”偷换运行语义 |
| 重新规划改了风险revision/目标 | 原批准hash不匹配，拒绝apply |
| 调用者离职、移出职责或委托撤销 | 当前授权拒绝；包仍published也不例外 |

兼容转换由平台维护并明确前后输入，不要求旧私有模块跟每次提示文字变化。包版本、能力合同、配置版本、业务revision和授权epoch分开。无法证明兼容时held，不能靠模型猜补继续。

## 5. 知识/RAG的受权边界

实时业务计数/风险状态走业务Query，不复制到知识索引。文档问答优先企业知识API：需要按主体/集合裁剪、来源版本/有效期、引用回读再授权及撤权缓存失效。

只有全局服务账号而不能按主体裁剪时：首期只能接明确批准的公共/共享集合，或给受权深链，不能把全库喂给模型再靠prompt保密。引用缓存继承原分类；未知/被撤来源使建议进入held/不足，允许查看已授权业务事实但不能伪装成有知识证据的建议。间接注入文本从未进入权限或工具允许集。

本样例以知识无主体ACL时“不形成可发布整改草案”为保守场景条件；它不强制所有自然语言查询都依赖RAG。知识服务不可用不得自动改成自建embedding/向量库。

代表plan现钉{id,version,digest}，apply前重新按当前actor回读知识ACL与固定版本/摘要；preview后撤权、换版或同版变内容均held，必须重新取合法证据/预览/批准。原16条skill加载/恢复例没有覆盖这个执行时窗口；后继以[代表场景A20–A25](domain-agent-reference-scenario.md#5-可复验的场景与结果)的18条实际红绿回归验证，不把16条旧例重复算作该覆盖。已提交效果的receipt按当前operation/目标权限返回最小状态，不带知识引用或旧plan；撤知识权不伪装业务效果不存在。

## 6. 实际验证

16个skill行为样例已运行：合法加载、内容篡改、非受信发布者、启动前撤销、同digest恢复、兼容patch仍钉旧版、原版缺失、破坏性合同、暂停后撤销、allowed-tools越权、路径逃逸、可执行资产、主体撤权、计划变更沿旧批准、越域知识引用、全局账号供给不合格。

manifest与示例DTO另经现有UI锁定的Ajv形状校验；全部研究样例合计76个正样本、9个负样本通过。Schema不证明live授权/信任，行为模型不证明真实SDK或模型正确性。运行命令见[样例README](examples/ai-soc-reference/README.md)。

## 7. 实施与估算边界

A07最小公开范围建议7–12工程日：

| 交付 | 日数区间 | 复用/不重复项 |
|---|---:|---|
| 有限host/step/receipt接线 | 2–3 | 复用CE02/R01/I03，不新建调度或业务幂等引擎 |
| 可信skill加载/钉版/撤销 | 1–2 | 复用G05来源及共同授权 |
| 等待/unknown/恢复的最小UI | 1–2 | 与业务同批，不默含旧全站U06 |
| 黄金集/注入/撤权/恢复证据 | 2–3 | 本轮fixture作输入，需产品真实栈接线回归 |
| 实际组件锁定与运行接线 | 1–2 | 不含新语言独立服务或真实提供方特例 |

A05配置/最小UI5–8、A06知识接入3–5归其各包，不重复计价。本文数值是交总账的复估依据，唯一正式数字仍由根规划集成；真实提供方等待、全面Agent市场/多Agent/脚本内核不在范围。

仍须在实际产品编码/接入时固定SDK/可信制品/持久化细节、时间/token/inflight限额、真实知识ACL和撤销传播预算，并运行产品授权/恢复测试。本轮已交具体样包和设计反例，不能将这些未来真实门写成已联调通过。

## 变更记录

- 2026-11-13：补FD06可信样包、Schema、16个可执行行为例、RAG授权降级与版本恢复合同；保留专业供给与无脚本默认边界。
