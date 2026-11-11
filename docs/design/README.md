# 架构与设计解释

先读 [系统架构](architecture.md)，再根据问题进入 [权限继承与级联](rbac-inheritance-and-cascade.md)。[设计决策](design-decisions.md) 保留讨论和裁决的时间顺序；旧阶段“当前”一词必须结合日期及后继修订理解。[旧系统对比](system-comparison.md) 是设计来源，不是当前验收表。

需要快速查最终结论时读 [ADR](../adr/README.md)；需要判断是否已交付时读 [项目控制台](../review/11-project-control.md) 和 [模块说明](../modules/README.md)。图中的服务边界是当前部署/调用关系，不代表引入微服务拆分计划。

## 第一次阅读时的术语

- **IAM**：身份与访问管理。本项目用用户、角色、组织回答“是谁、有哪些功能权限、能操作哪些数据”。
- **L1 / L2 / L3**：依次检查接口入口、具体资源是否可见、当前动作是否允许。例如能打开工单页面，不代表能读取任意工单，也不代表能删除已读到的工单；实际例外看[权限地图](../modules/authz.md)。
- **SSOT**：某一事项唯一维护的权威来源。摘要可以引用它，字段、规则或状态发生变化时回到原文件修改，避免多份正文各自漂移。
- **AK/SK**：服务调用方标识和签名密钥，用于识别、校验服务间请求；它与浏览器用户的 JWT、业务资源授权分别处理。公共协议由 utils 维护，业务归属见[五仓边界](architecture.md#13-项目与生态边界)。
- **已批准 / 已实现 / 已验证 / 已部署**：分别表示方向已定、代码存在、指定候选通过了所列检查、指定环境已运行对应产物。阅读阶段稿的勾选时先分清这四种状态。

## 用一个请求串起文档

以“打开并操作一张工单”为例：浏览器先经主仓认证和路由鉴权，再由工单 Service 判断可见性与动作权限，最后通过 Repository 读写 PostgreSQL。附件、任务和通知各有独立调用或提交后处理，不因工单事务成功就全部成功。

按[中间件](../modules/middleware.md) → [工单权限与动作](../modules/ticket.md) → 对应阶段稿的当前摘要阅读。排查失败时先确定发生在哪一段，再查该段的日志和错误码；服务之间的边界见[架构总览](architecture.md)。

## Phase5之后的现行演进

- [平台边界与演进](platform-evolution.md)：专业供给、数据写主、三类流程、插件与拆分。
- [AI能力与领域Agent](ai-capabilities.md)：已有模型API、外部与本地领域Agent双轨、MCP、受权知识、skill与运行合同。
- [五仓工程治理](engineering-governance.md)：worktree、CI、mock分层、测试、目录和发布。
- [排期总账](../review/planning-backlog.md)：跨仓唯一工作项、依赖与条件项处置。

## 详设候选与新增边界

- [工程施工](engineering-execution-design.md)、[跨仓执行](cross-repo-execution-contract.md)、[业务生命周期](business-lifecycle-contracts.md)与[AI调用](ai-invocation-contract.md)。
- [私有扩展兼容](extension-compatibility.md)、[内网adapter迁移](intranet-adapter-migration.md)、[设备目录与人工入口](device-module-contract.md)、[版本现代化](dependency-modernization.md)。
- [规划收口与审计步骤](../review/planning-closeout.md)：哪些已审、哪些是候选/边界、哪些需要owner，不能把全部工作包当全部详设已完成。
