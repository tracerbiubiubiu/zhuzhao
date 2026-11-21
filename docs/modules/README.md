# 模块导读

本目录解释当前实现：职责、关键状态、调用链、权限和失败边界。详细迁移 SQL 以源码为准，旧阶段方案保留在 phase 目录。后续已批准能力不因出现在设计稿里就视为实现。

| 想了解什么 | 文档 | 先看哪个问题 |
|---|---|---|
| 登录和会话 | [认证](auth.md) | AT/RT/PAT 怎么失效，Redis 故障怎么处理 |
| 账号和资料 | [用户](user.md) | 身份字段、版本、改密与管理保护 |
| 角色与策略 | [角色](role.md) | 继承、菜单分配事务与内存重载 |
| 组织与委托 | [组织](organization.md) | 角色来源、scope、负责人三者的区别 |
| 页面和按钮 | [菜单](menu.md) | 迁移词表如何生成导航与 API 策略 |
| 全局权限边界 | [鉴权](authz.md) | L1/L2/L3 各在哪里执行 |
| 操作与判定留痕 | [审计](audit.md) | 两条日志链与归档的可靠性 |
| 请求管线 | [中间件](middleware.md) | 全局、公开、自服务、M2M 分组 |
| 工单领域 | [工单](ticket.md) | 类型、状态、范围和动作权限 |

## 按业务场景串起当前实现

以下按候选基线 `3d958c3` 的主仓源码导读，只说明调用关系，不新增验收或部署结论。

- **用户登录后进入系统**：`internal/router/router.go` 的 `New` 将登录挂在公开认证组；`internal/service/auth_service.go` 的 `AuthService.Login` 校验验证码策略、失败锁、账号与密码，再签发 AT/RT、保存设备刷新槽。拿到令牌后的受保护请求还要经过 `internal/middleware/jwt.go` 的 `JWT` 和路由鉴权；登录成功本身不授予工单权限。会话失效与登录故障从[认证模块§5–6](auth.md#5-核心流程)查起。
- **编辑工单或改变状态**：`internal/service/ticket/service.go` 的 `Service.Update/updateCommand` 检查资源权限、当前状态与字段，事务内更新工单并写 `ticket_events`；`internal/service/ticket/transition_service.go` 的 `Service.Transition` 另校验状态机、condition 和 validator，事务内写状态及事件，提交后派发领域事件、执行 post_actions。后置动作失败不回滚已提交转换；类型配置、状态与动作范围见[工单模块](ticket.md)。
- **请求被权限拒绝**：先看凭证是否在 `JWT` 被拒，再看 `internal/middleware/casbin.go` 的 `CasbinAuth` 是否允许进入 Handler，最后看工单 `Service.authorizeCheck` 的资源判断。工单不存在或不可见返回 404，可见但无动作权限返回 403；策略投影不可用等依赖问题可能返回 503，需结合错误码和应用日志定位。自服务组仍要求有效身份和非空角色，具体资源检查留在 Service；分层入口见[鉴权](authz.md)，不要仅凭页面可见推定写权限。
- **操作留下什么记录**。工单事务写 `ticket_events`，HTTP 响应后由 `internal/middleware/audit.go` 的 `AuditLog` 尽力记录操作。资源判定另写 `policy_evaluation_logs`。`AuthService.Login` 在成功和部分失败分支调用 `LogLogin`。请求若在 JWT 阶段被拒，先按响应头 `X-Request-ID` 对应的 `request_id` 查访问日志。 [审计模块](audit.md)
- **工单变了却没收到提醒**。`internal/app/app.go` 的 `NewApp` 装配独立订阅者。查站内消息时，从 `internal/service/ticket/inbox_service.go` 的 `InboxService.OnDomainEvent` 核事件登记、收件人和入箱记录。查规则外发时，从 `internal/service/ticket/dispatcher.go` 的 `MessageDispatcher.Dispatch` 核规则、模板、偏好、节流和 `message_log`。

  当前发送器与可靠性有两个边界：`internal/pkg/channelsender/port.go` 的 `UIPStubSender.Send` 在开发态只记日志并返回成功，release 下真实发送仍未实现；`sent` 不证明外部送达。`internal/pkg/domainevent/registry.go` 的 `HookRegistry.Dispatch` 是进程内异步派发，进程崩溃可能丢事件，通知失败不会回滚工单。

<a id="代码路径约定"></a>

## 找代码的顺序

从 `internal/router/router.go` 的 `New` 找生产路由，再进入 Handler → Service → Repository。IAM 服务当前仍多为 `internal/service/*_service.go` 扁平文件，工单为 `internal/service/ticket/` 领域包；“目标目录”不等于实际已迁。公共密码/JWT/AKSK 等由独立 utils 模块维护，资源授权框架仍留主仓。

注册表目前只有工单消费资源授权；公共 Builtin 策略是可复用能力，不代表 taskrunner、组织等已经接上相同 L2。先以 [架构](../design/architecture.md) 和实际装配确认边界。

## 变更记录

| 日期 | 变更 |
|---|---|
| 2026-11-08 | 将跨阶段全形态索引改为当前实现导读，删除并未落地的目录、设备 UI 与公共包归属暗示。 |

## 跨模块演进

当前模块职责仍按本目录核对；风险、能力目录与AI为后续工作，不因规划出现即实现。[平台演进](../design/platform-evolution.md)统一五仓数据/状态写主、失败恢复和可插拔边界；[总账](../planning/planning-backlog.md)维护全部遗留的当前去向。专业能力先接入消费，不将模块地图扩成SOC/CMDB/SOAR/知识的复制清单。
