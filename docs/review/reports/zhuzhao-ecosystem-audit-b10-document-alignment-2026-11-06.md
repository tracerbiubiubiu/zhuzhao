# 生态审计 B10：现行设计与能力说明校准（2026-11-06）

> **历史路径口径（2026-11-08 整理）**：审计事实、行号与文件位置按原时点名称保留；不以今天的目录迁移改写历史快照。可点击导航指向当前文档位置。

> **报告口径（2026-11-08整理）**：本文保留注明时点的基线、原失败、发现编号和复验范围；代码路径是定位提示，历史内容以文内固定提交为准。后续变化见[正式报告索引](README.md)，当前能力、开放债与运行承接见[项目控制台](../11-project-control.md)。报告通过不等于main或生产已承接。
## 范围、基线与出口

业务基线 `5c03581d5999139a3e8e5ead5972c026868aa181`，本批落笔前文档快照 `84de799df8bed8daa7da130bb78c179152cd5c6f`。仅校准五组已证实的现行说明，不扩大源码重组、不重写历史决策或实施日志。事实对齐并指向当前维护源后结束本批；P2/P3措辞不再追加抛光轮。此出口不代替全面实现审计的逐模块核验。

## B10-F01 / P2：engine子包被写成已经落地

原快照锚点：[docs/phase6/00-flow-engine-v2.md:114](https://github.com/tracerbiubiubiu/zhuzhao/blob/5c03581d5999139a3e8e5ead5972c026868aa181/docs/phase6/00-flow-engine-v2.md#L114)。

原 [phase6/00 §7](https://github.com/tracerbiubiubiu/zhuzhao/blob/5c03581d5999139a3e8e5ead5972c026868aa181/docs/phase6/00-flow-engine-v2.md#L114) 声称“engine子包化已随B3-1落”。原快照 `internal/service/ticket/` 无engine子目录；`state_machine.go`、`transition_service.go`、`timeout_scanner.go` 都是package ticket。`internal/architecture_test.go` 的跨领域依赖守卫不等同于不存在的专属engine包。

本批承认未实施，是否随B0整理另作收益评估；phase5/03 A.6、S10 A.1标明原计划目标路径，历史记录保留。包组织偏差本身不证明运行时越权，也不据此强行重构。

## B10-F02 / P2：ADR导航误称轮询已实施

原快照锚点：[docs/README.md:217](https://github.com/tracerbiubiubiu/zhuzhao/blob/5c03581d5999139a3e8e5ead5972c026868aa181/docs/README.md#L217)。

原 `docs/README.md` 的ADR001导航写“轮询实装”；但 `docs/adr/ADR-001-event-mechanism-l1-steady-state.md` 状态已明确轮询未实施，当前实际为 `internal/pkg/domainevent/registry.go` 的 `HookRegistry.Dispatch/Drain` 进程内分发。

仅更正当前导航：ticket_events事务留痕已落，轮询未落，当前at-most-once；可靠重放升级仍为C-20。未将历史选型决策改写为当前可靠性承诺。

## B10-F03 / P2：事件生产者清单多报标签

原快照锚点：[docs/standards.md:250](https://github.com/tracerbiubiubiu/zhuzhao/blob/5c03581d5999139a3e8e5ead5972c026868aa181/docs/standards.md#L250)。

原 `docs/standards.md` §14将标签列为当前DomainEvent生产者；实际 `internal/service/ticket/tag_service.go` 的 `AddTags/RemoveTag` 只写tagRepo，没有发布DomainEvent。

删除活清单中该错误断言，明确标签和内部备注不发布；保留真实生命周期/CC、timeout和ops来源。主线 `service.go` 的 `emitEvent` 与独立修复分支 `emitTicketEvent` 分列；公开评论原漏线及92adc20补线见[B06-F11](zhuzhao-ecosystem-audit-b06-flow-reminders-2026-11-06.md)，未冒称主线已经升级。

## B10-F04 / P1·P2：控制台与菜单长期稿沿用已退役权限形态

原快照锚点：[docs/modules/menu.md:15](https://github.com/tracerbiubiubiu/zhuzhao/blob/5c03581d5999139a3e8e5ead5972c026868aa181/docs/modules/menu.md#L15)。

原快照锚点：[docs/review/11-project-control.md:86](https://github.com/tracerbiubiubiu/zhuzhao/blob/5c03581d5999139a3e8e5ead5972c026868aa181/docs/review/11-project-control.md#L86)。

11号控制台§1仍列菜单CRUD、§2仍列menu:create/update/delete，并概括“工单操作不走静态权限码”；`docs/modules/menu.md` 仍指导POST /menus登记及运行时API绑定。实际 `internal/router/router.go` 的菜单组只有GET树/详情，角色菜单分配是另一仍有效入口；工单业务路由先过L1，selfService豁免不代表全部工单绕过L1。

能力矩阵/权限速查失真按本项目P1；长期历史模块稿缺边界标识按P2合并处理，不重复计根因。本批更正11号当前矩阵与速查，模块稿添加只读边界并标历史接口/流程，旧参考doc树不再作为设计依据。L-5增加menu校准范围，ticket/architecture主体回填保持开放，不借局部修订销掉整项历史债。

## B10-F05 / P2：旧部署示例仍被导航成当前入口

原快照锚点：[docs/ops/README.md:12](https://github.com/tracerbiubiubiu/zhuzhao/blob/5c03581d5999139a3e8e5ead5972c026868aa181/docs/ops/README.md#L12)。

原快照锚点：[docs/ops/deployment.md:13](https://github.com/tracerbiubiubiu/zhuzhao/blob/5c03581d5999139a3e8e5ead5972c026868aa181/docs/ops/deployment.md#L13)。

`docs/ops/deployment.md` 的2026-09-02示例仍列Go≥1.22、旧env键、根目录裸compose、旧二进制命令和8080；`docs/ops/README.md` 仍称runbook待编写。实际根go.mod为1.26.5，`internal/config/config.go` 的 `Load` 显式绑定APP_PORT等；现行操作在deployments/README、docs/runbook与端口分配表。

保留历史示例并明确不可直接照抄启动，将当前入口指向上述维护源；导航改为已建runbook的真实路径。未复制第三套env清单，未改冻结configs基线。

## 验证证据与限制

- 以固定hash读取上述原断言，逐一对目录、包声明、真实路由、配置Load与事件出口；源码锚点以文件路径+符号/章节为主。
- 原始断言、源码匹配和链接校验清单位于本地 `.codex/audits/ECO-20261104/evidence/ECO-20261104-b10/`，不入Git/Docker。
- 本批仅文档，没有数据库迁移、接口、权限行为或配置变更。业务联合12401aa此前全量门禁已过；不把该结果冒称本次重新执行门禁，也不以文档校准证明其它模块实现无缺陷。
- 五组当前说明完成校准即结束B10；其它已读/未读、实现证据和历史债对账继续由全面审计主线承接。
