# 开发与操作指南

面向开源访客的说明从[主仓README](../../README.md)进入；维护公开入口、命名和社区准备见[开源展示指南](open-source-presentation.md)。参与代码或文档贡献见[贡献指南](../contributing.md)。

从 [开发指南](development.md) 开始：准备工具 → 确认环境归属 → 依赖与迁移 → 启动 → 验证。需要部署容器、恢复数据或处置账号时转 [运维入口](../ops/README.md)。

指南说明“怎么做”；模块内部原理见 [模块导读](../modules/README.md)，API 返回约定见 [契约入口](../api/README.md)。前端、任务服务和动态数据服务的本仓步骤分别由其自身 docs 维护。

## 执行前与结果定位

本目录的主仓 `make` 命令从**含 Makefile 的主仓根目录**执行；部署页显式 `cd deployments` 后的 Compose 命令才从该目录执行。先区分宿主开发、容器部署与测试编排，再使用对应页的一整组配置、目标库和端口。

- **工具前置**：开发指南列出 Go 与 Python 3；依赖启动还需 Docker Compose v2，宿主 `make migrate-up` 需 PATH 中已有 `migrate`。`Makefile` 的 `tools-install` 只安装 Wire/Swagger，不安装迁移 CLI；容器迁移另用[部署页](../ops/deployment.md)的专用镜像。
- **启动预期**：依赖健康、迁移退出成功且版本非 dirty 后，再观察应用监听与直连 `/health/ready`。`internal/router/router.go` 的 `New` 在 DB、Redis Ping 均成功时返回 HTTP 200 和 `{"status":"ok"}`；失败返回 503，并以 `component=db` 或 `redis` 定位。UI 和上游链路继续按部署页分别核对。
- **失败入口**：命令找不到先核工具/PATH，迁移失败查迁移输出与目标库版本，监听失败查[端口表](../ops/port-allocation.md)和[单实例限制](../ops/deployment.md#单实例运行限制与存储公开入口2026-11-17存量修复候选)。已经收到业务错误时，保留错误码、`request_id` 与目标入口，按[模块调用链](../modules/README.md#按业务场景串起当前实现)定位应用日志和审计；日志目录取实际 `log.dir`，不把其他环境的历史路径当当前值。

## 带着具体任务阅读

- **第一次启动系统**：先按[开发指南](development.md)准备主仓依赖并显式迁移，再按[部署与联调](../ops/deployment.md)接入 UI、taskrunner、activelist。主仓启动成功只覆盖主仓就绪条件，上游仍需各自验证。
- **新增一个管理页面**：先看[菜单](../modules/menu.md)的迁移/角色绑定顺序，再看 UI 仓的页面与组件指南。导航能显示、接口能调用、具体数据可操作是三个检查点。
- **排查“看得到但改不了”**：从[权限地图](../modules/authz.md)区分路由、可见性和动作，再按 request_id 关联[操作与判定日志](../modules/audit.md)。不要先扩大角色或清 Redis 状态。
- **判断一项功能是否已完成**：看[控制台](../review/11-project-control.md)的当前能力和债，再读对应报告的候选与验证范围；后续规划从[路线图](../roadmap.md)进入。
