# S5+S6 挂账台账（2026-10-23 统一整理）

> 本文件为 S5/S6 段全部未清项的**唯一权威台账**（外审 F-2-10 修复——原挂账散布
> review/11 头注流水段+各号设计稿差距注，无法按触发信号管理、无法逐项销账）。
> 整理规则：按「段/编号/项/级别/登记日/代码实态/触发信号/挂点批次」八列——
> 已修项不列（在各自变更记录留痕）；N/A 项列并标因。

## 一、S6 一期挂账

| # | 项 | 级别 | 登记日 | 代码实态 | 触发信号 | 挂点批次 |
|---|---|---|---|---|---|---|
| S6-D1 | D20 `changes[]` 非真 diff（仅请求键集、`OldVal` 恒空、custom_data 无键级粒度） | P2 | 10-23 外审 F-1-10 | `buildChanges` 只判 `req.* != nil`，无旧值比对 | S6 二期消费方需要变更历史时 | B6-7 二期 |
| S6-D2 | variables 端点仅内置 6 条（缺 S1 field_meta 动态字段源 + 时间类变量） | P2 | 10-23 外审 F-1-11 | `Variables` handler 硬编码 6 条，无 DB 查询 | 模板编辑器变量 picker 上线时 | B6-7 二期 |
| S6-D3 | opt-out 自服务 service 直连 db（绕 repo 层——同表双实现） | P2 | 10-23 外审 F-1-12 | `NotificationPrefService` 持 `pgxpool` 写 SQL | 架构守护扩到 service→db 时 | 架构批或随手 |
| S6-D4 | `'*'` 全域免打扰仅域内生效（`biz_type = $2 AND event_action IN ($3,'*')`）——跨域不生效 | P3 | 10-23 外审 F-1-12 | 查询按 biz_type 精确匹配，`'*'` 只在同域命中 | phase6 风险域通知上线时 | B6-7 二期 |
| S6-D5 | stub 两态无独立 config 段（用 `cfg.Server.Mode != "release"` 间接推导） | P3 | 10-23 外审 F-1-14 | `app.go:64` 从 gin mode 推导；无 `notification.uip.*` 段 | 生产忘设 mode=release → 静默用桩 | 接 UIP 真实契约前 |
| S6-D6 | acceptance 通知深面缺四类（注入/节流/fail-fast/偏好过滤——现有 5 条仅 200 冒烟） | P3 | 10-23 外审 F-1-15 | `phase1.sh` #MSG-1~5 全为 HTTP 200 断言 | S6 二期验收前 | B6-7 前补课 |
| S6-D7 | markdown 渠道转义缺失（`channelEscapes` 仅 uip/webhook 二族，无 markdown 分支） | P3 | 10-23 外审 F-2-9 | `[点我](http://evil)` 在 webhook 渠道直接成为可点链接 | UIP 契约确认正文格式后 | UIP 对接批 |
| S6-D8 | rules version 乐观锁 N/A——upsert 语义（last-writer-wins）为设计终态，配置数据无并发编辑冲突场景 | N/A | 10-23 | `ON CONFLICT DO UPDATE` 无 CAS | — | — |
| S6-D9 | 死信回调 ctx 无超时 + `emitEvent` 用 `context.Background()`（丢 request_id） | P3 | 10-23 外审 F-2-8 | Send 5s 超时已加；死信/emit 侧未加 | 接入链路追踪时 | 随手批 |
| S6-D10 | 风暴期节流移到偏好后=每个重复事件需查 DB+渲染才被节流（入口廉价预判缺） | P3 | 10-23 外审 F-2-8 | throttle 在 opt-out 过滤之后 | 通知量级异常增大时 | 性能批 |

## 二、S5 挂账

| # | 项 | 级别 | 登记日 | 代码实态 | 触发信号 | 挂点批次 |
|---|---|---|---|---|---|---|
| S5-D1 | F-A3 BindAvatar 非事务（5 步逐条调用；"无害"论证因第三段 GC 落地已成立） | P2 | 10-21 五批审计 | `service.go:292-335` 无事务包裹 | 头像绑定中途失败的孤儿对象回收 | S6 二期或随手 |
| S5-D2 | 内嵌图「编辑从正文删引用」场景回收（需 custom_data 引用集解析——RK-P5-16 已登记） | P2 | 10-23 外审 F-4-4 | 第三段反连接已覆盖 2/3 场景；此场景需解析 MD 正文 | 编辑删图后对象确认未回收 | RK-P5-16→B6-7 二期 |
| S5-D3 | RK-P5-15 触发信号不可观测（磁盘水位/贴图量级——代码零实现） | P3 | 10-23 外审 F-4-5 | 无 disk_usage/贴图计数端点 | —（不可观测=永不触发） | ops 面批 |
| S5-D4 | `file_cookie_secure` 两栈均 false，无 release 强制/告警 | P3 | 10-23 外审 F-4-7 | `config.yaml:75` / `config.test.yaml:80` 均为 false | 生产 HTTPS 网关前置时 | 部署批 |
| S5-D5 | A.6 前端组件路径与实际全不一致（`Upload/FileUploader` 不存在等——未回填实施形态） | P3 | 10-23 外审 F-4-8 | 能力内联在 `RichtextField`/`TicketAttachments`/`StorageAvatarUpload` | 文档批 | 05 号回填 |

## 三、S1 挂账（跨段）

| # | 项 | 级别 | 登记日 | 代码实态 | 触发信号 | 挂点批次 |
|---|---|---|---|---|---|---|
| S1-D1 | P2-8 游离 godoc（ticket_handler 4 处 `ListTicketTypeFields`——FieldImpact 前后重复） | P3 | 10-22 全量核查 | `ticket_handler.go` 4 处命中（已从 5 减到 4） | — | 随手批 |
| S1-D2 | P1-21 `'deleted'` 字面量在 repo 层（常量在 service——import 方向不可达） | P3 | 10-23 外审 F-2-7 | `ticket_repo.go:388` 写死 `'deleted'` | — | 随手批 |
| S1-D3 | P1-21 `HookRegistry.Subscribe` 无 name 参数（A.1 规格"重复名 panic"结构不存在） | P3 | 10-23 外审 F-2-7 | 无 name 传参 | 多订阅者调试需求出现时 | B6-7 二期 |

## 四、挂账质量判据（外审 F-4-5 推广）

> **登记了触发信号 ≠ 触发信号可观测**——「触发驱动」类挂账若代码面零实现
> 观测面（端点/日志/SQL），事实上等于「永不触发」。后续登记触发驱动项
> 须同时回答：「触发信号由什么代码产生？谁来读？」

---

*整理基线：zhuzhao `b743c7f` / zhuzhao-ui `9ada8da4`（2026-10-23 双仓 CI 绿）*
