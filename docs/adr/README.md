# 架构决策记录

架构决策记录（ADR）说明选择某种设计的理由、适用范围和代价。阅读时先看“状态”与后继修订，再看原始背景；是否已经实现和验证，另查[项目控制台](../review/11-project-control.md)。例如，ADR-001 采纳了事件源方案，但不能据此认定轮询消费者已经实现。

| 决策 | 状态与阅读边界 |
|---|---|
| [ADR-001 事件事实源](adr-001-event-mechanism-l1-steady-state.md) | `ticket_events` 已在事务内写入；原定轮询未实现，现有分发由进程内 `HookRegistry` 直接调用，可靠投递的后续工作见 outbox 待办 |
| [ADR-002 异步任务执行器](adr-002-asynq-async-task-executor.md) | 保留历史采纳记录；主仓未引入 Asynq，独立 taskrunner 用其执行任务，不替代工单事件事实源 |
| [ADR-003 activelist集成](adr-003-activelist-integration-form.md) | 保留历次职责调整的历史镜像；当前 PostgreSQL（PG）设计与独立服务契约由 activelist 仓库的 docs 维护 |
| [ADR-004 类型/模板/流程](adr-004-ticket-type-template-flow.md) | 已批准类型为根、模板从属于类型的单向关系；定义快照与模板选择界面等后续工作仍需各自实现和验证 |
| [ADR-005 统一能力与源事实](adr-005-capability-invocation-and-source-facts.md) | 新增需求的设计候选；只调整 activelist（AL）源变更事实的责任边界，不表示已实现或已计入原有工作量 |

讨论过程保留在[设计决策](../design/design-decisions.md)，形成独立 ADR 后互相引用。跨仓镜像需写来源与核对日期，以便区分原始结论和后续修订；不要把镜像当作另一份当前实施清单。

## 新记录模板

用小写文件名 `adr-NNN-topic.md`，标题保留ADR编号。正文至少包括日期、背景、决策、理由、后果和后继链接；设计效力（候选/已采纳/被替代）与实施状态（未实施/部分实施/已实施）分别写明，验证/部署另附可核对的结果记录。新模板不追溯改写旧决定，修订保留原日期及覆盖范围，格式见[维护约定](../documentation.md#页头和效力)。
