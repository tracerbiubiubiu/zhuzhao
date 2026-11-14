# 架构决策记录

ADR 记录“为什么选择这条边界”，不是完成清单。先读状态与后继修订，再读原始背景；当前能力另外查[项目控制台](../review/11-project-control.md)。

| 决策 | 状态与阅读边界 |
|---|---|
| [ADR-001 事件事实源](adr-001-event-mechanism-l1-steady-state.md) | 事务内ticket_events已落；原轮询未实现，当前HookRegistry直发，可靠性升级见outbox债务 |
| [ADR-002 异步任务执行器](adr-002-asynq-async-task-executor.md) | 保留历史采纳记录；主仓未引Asynq，独立taskrunner用其执行任务，不替代工单事实源 |
| [ADR-003 activelist集成](adr-003-activelist-integration-form.md) | 历次职责收敛镜像；当前PG与独立服务契约由activelist仓docs维护 |
| [ADR-004 类型/模板/流程](adr-004-ticket-type-template-flow.md) | 已批准单向关系；快照与模板UI接入等后续件不因批准即视为交付 |
| [ADR-005 统一能力与源事实](adr-005-capability-invocation-and-source-facts.md) | 新增需求的设计候选；限域校准AL事实源责任，不表示已实现或已计入旧预算 |

讨论过程保留在[设计决策](../design/design-decisions.md)，独立决策形成ADR后互相引用。跨仓镜像需写来源与核对日期，不复制成另一套当前实施清单。

## 新记录模板

用小写文件名 `adr-NNN-topic.md`，标题保留ADR编号。正文至少包括日期、背景、决策、理由、后果和后继链接；设计效力（候选/已采纳/被替代）与实施状态（未实施/部分实施/已实施）分别写明，验证/部署另给准确收据。新模板不追溯改写旧决定，修订保留原日期及覆盖范围，格式见[维护约定](../documentation.md#页头和效力)。
