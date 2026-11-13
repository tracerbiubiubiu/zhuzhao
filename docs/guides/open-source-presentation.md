# 开源展示、命名与社区上手

日期：2026-11-13。用途：维护者面向陌生访客的展示与贡献入口；本轮更新五仓中英README、公开合成截图与社区模板。远端元数据、正式发布、在线演示和新品牌未执行，不把建议当已有产品。

## 工程化和目录方案在哪里

这些已有具体方案和窗口，不能再说只有原则；也不能说全部机制已启用。

| 事项 | 权威方案/工作项 | 当前边界 |
| --- | --- | --- |
| 并行修改误提交 | 工程施工G01：独立worktree、显式路径、单集成者、运行资源归属 | hooks/路径提交已有；更强机械守卫仍待实施 |
| PR与自动检查 | 工程治理/施工G02–G03：主线保护、固定组合、准确check身份、分档 | 现CI与planning-check已有；完整acceptance/E2E自动CI、远端保护、分档未全面启用 |
| 测试质量 | G04：原测试先行、权限/并发/幂等反例、分层覆盖率、五个fuzz目标与预算 | 原纪律继续；持续fuzz和完整风险基线仍在P6.1 |
| 版本和发布 | G07维护候选、G05：锁版本/制品、升级恢复、离线交付 | 本地版本候选已验证；不冒称main/正式发布已承接 |
| 五仓/新仓目录 | G06：Go服务/Go库/UI/合同/私有connector分类模板 | 按真实职责建目录，不生成空模块或强迫同一目录树 |
| 命名/代码/文档/API/消息/视觉 | 生态一致性合同、G06及对应业务包 | 共同合同已有；formatter/bundle与产品全站推广未完成 |

实际窗口与工日仍以[总账](../review/planning-backlog.md)和[执行排期](../review/execution-schedule.md)为准；详见[工程施工](../design/engineering-execution-design.md)、[工程治理](../design/engineering-governance.md)、[一致性合同](../design/ecosystem-consistency-contract.md)。

## 名称应统一到产品族，但先选品牌

当前主品牌仍为zhuzhao，所有者已提出更名但未选择新名称。本轮只让五仓展示和导航一致，不改GitHub仓、Go module/import、镜像、二进制或数据库名。

建议采用短、可读、可搜索的品牌名，再统一为`<brand>`、`<brand>-ui`、`<brand>-taskrunner`、`<brand>-activelist`、`<brand>-utils`。taskrunner和activelist已有独立职责，README用“任务执行/调度”“结构化动态清单”解释；utils是公共组件库，不能为营销改称尚不存在的产品API SDK。只有明确独立产品定位时再改变后缀，不为凑整齐改所有Go包名。

新名称确定后一次性核仓库、模块/依赖消费者、镜像、链接、Action和本地worktree。GitHub重定向不保证所有消费者无需修改。具体迁移边界见[文档命名约定](../documentation.md#仓库名称与改名边界)。

## 本轮已做的展示改动

- 五仓README先给自身价值、当前能力与成熟度、实际启动/用法，再给文档与贡献。
- 五仓中文与英文互相链接，主仓作为产品入口，兄弟仓解释职责并回链主仓。
- 主仓使用两张真实Vue/合成数据截图，说明当前UI为中文；不展示未来AI/新首页为已实现。
- README对照专业平台职责，不编造竞品性能、体积或“转化提升25%”。
- 贡献、问题报告与PR模板要求可复现、脱敏、明确范围和实际验证，不增加形式上的第二位维护者。

README里先提供真实源码启动，明确仍需UI/上游。全栈一键演示和公开在线演示尚未建好；不会放占位URL或承诺5秒首次拉起。

## 下一轮公开展示准备窗口

下列事项在**首次正式对外演示/发布准备**时逐项审阅，先于大范围推广，不等待整个P6–P10结束。未具备则如实保持未发布，不长期写“以后有需求再说”。新开发项先给有限设计/估算，不默认纳入旧价。

1. **体验入口**：先选已有工单旅程，固定五仓组合和合成数据，实现可复跑体验包；再决定是否提供隔离的只读在线演示。初次下载时长、清理/重置、无真实密钥和失败提示要实测；不把审阅夹具当正式全栈体验。实现与预算在该准备窗口先冻结，归G03/G05及所选旅程，不本轮暗建托管服务。
2. **About、topics与预览图**：下表是可审候选，本轮未写远端。只标已有能力，不堆尚未实现的AI/MCP关键词；品牌确定后制作分享预览图，先核各平台尺寸和文字。
3. **安全渠道**：在对外推广前选定并验证私密报告入口，再发布SECURITY政策；不编造邮箱、响应SLA或尚未开启的私密报告链接。
4. **可下载版本**：按实际验收候选发release/changelog，清晰列当前功能、配置前提与已知限制；不把规划分支叫稳定版。utils已有v0.4.3，其他仓本次只读时无release，不能假装已有整个平台版本。
5. **真实用例和反馈**：每次交付讲一个可复现的用户场景，向对应安全/运维/开发群体分享；发布真实可接手的good-first-issue。发布后约两周复盘访问、clone、有效问题和Star趋势，关注是否有人成功跑通，不能把不同访客的粗比率当严格归因转化率。

| 仓库 | 建议About | 建议topics |
| --- | --- | --- |
| zhuzhao | Self-hosted IAM and ticketing foundation for security lifecycle collaboration. | security-operations, ticketing, iam, rbac, golang, self-hosted |
| zhuzhao-ui | Vue workspace for zhuzhao tickets, tasks, identity and data. | vue, typescript, element-plus, ticketing |
| taskrunner | Go task execution and cron scheduling with retries and signed callbacks. | golang, task-runner, cron, redis |
| activelist | Typed structured lists with validation, import/export and service-signed APIs. | golang, postgresql, dynamic-schema, data-management |
| zhuzhao-utils | Shared Go building blocks for signing, errors and service responses. | golang, hmac, shared-library |

远端只读核查时五仓About仍为占位、topics/homepage为空，issues开启、discussions关闭，main无保护规则；本地README/模板提交不改变这些设置。不开没有维护计划的渠道，不替所有者向社区发帖、建宣传issue或发送消息。

## 依据与效果边界

GitHub说明README应解释项目用途、价值和上手方法；topics帮助发现；社交预览图改善链接识别。这些是可执行的展示手段，不是确定的Star增长公式。[GitHub仓库展示](https://docs.github.com/en/repositories/managing-your-repositorys-settings-and-features/customizing-your-repository)、[topics](https://docs.github.com/en/repositories/managing-your-repositorys-settings-and-features/customizing-your-repository/classifying-your-repository-with-topics)、[社交预览](https://docs.github.com/en/repositories/managing-your-repositorys-settings-and-features/customizing-your-repository/customizing-your-repositorys-social-media-preview)。

传播建议来自面向真实目标用户、提供上下文和持续反馈的实践，仍须本项目验证；未找到能直接支持本项目“对比表提升25%转化”的可靠依据，不把用户举例的数字当事实。[Open Source Guides：寻找用户](https://opensource.guide/finding-users/)、[指标](https://opensource.guide/metrics/)。
