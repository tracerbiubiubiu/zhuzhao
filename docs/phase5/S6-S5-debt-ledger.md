# Phase 5 挂账统一台账（2026-10-23 终版——全仓口径）

> 本文件为 phase5 **全部段**未清项的唯一权威台账（外审 F-2-10 修复）。
> 整理规则：按「消化时机」分五组（A 即时可清 / B 已锁批次 / C 触发驱动 /
> D 外部依赖 / E 待拍板）——组内按段排。已修项不列；N/A 项列并标因。
> 代码实态逐项 grep 交叉核验（基线 zhuzhao `f3e321d` / ui `9ada8da4`）。

## A. 即时可清（零设计，随最近一批带走）

| # | 段 | 项 | 出处 | 量级 |
|---|---|---|---|---|
| A-1 | S6 | F-1-9b Update 期 placeholders 校验旁路（Create 有 Update 无） | 外审 F-1-9 半截 | ✅ **已修**（本批） |
| A-2 | S1 | 92303 errcode 预留注记 | 01 号 A.6 原登记 | ✅ **已修**（本批） |
| A-3 | S6 | F-D2 rules version 乐观锁 | 000060 ✅ 已入库 b101cc6 | ✅ **已修** |
| A-4 | — | 外审批 3 对表文档批（06/10/11/05 四稿） | b75917d/baccad6/d14678e | ✅ **已修** |

## B. 已锁定消化批次（挂而不悬——锚点已写进 11 号/段稿）

| # | 段 | 项 | 消化点 | 锚 | 量级 |
|---|---|---|---|---|---|
| B-1 | S6 | D20 changes[] 真值 diff（旧值比对+custom_data 键级粒度） | B6-7 二期 | 06 号 A.1 | +0.5 天 |
| B-2 | S6 | variables 端点补动态字段源+时间类 | B6-7 二期 | 06 号 A.6 | +0.5 天 |
| B-3 | S6 | `'*'` 免打扰跨域生效 | B6-7 二期 | 外审 F-1-12 | +0.5h |
| B-4 | S6 | acceptance 通知深面（注入/节流/fail-fast/偏好四类） | B6-7 前补课 | 06 号 §10 | +0.5 天 |
| B-5 | S6 | opt-out service 直连 db 收敛到 repo | 架构批 | 外审 F-1-12 | +0.5h |
| B-6 | S10 | MessageDispatcher 编排器抽出（notify_hook 内联→独立件） | B10-4 首件 | 06 号 §1.5 | +0.5–1 天 |
| B-7 | S10 | escalation 催办 + ticket_reminders 侧表 + supervisor 第四分支 | B10-4 | 10 号 §3.4 / 06 号 A.5 | ~2 天 |
| B-8 | S3a | `run.succeeded` emit + `result.*` 命名空间 | B3-3 | 03 号 §3.3 | 随批 |
| B-9 | S5 | 内嵌图引用集解析（回收最后一段——RK-P5-16） | B6-7 二期或 phase6 | RK-P5-16 | +1 天 |
| B-10 | S6 | message_log GIN 索引 + fetcher 时机 | 触发（工单量级/首源） | S1 审计 | 随批 |
| B-11 | S5 | BindAvatar 非事务（F-A3——"无害"论证因 GC 第三段已成立） | S6 二期或随手 | 五批审计 F-A3 | +0.5h |
| B-12 | S1 | P2-8 游离 godoc（ticket_handler 4 处） | 随手批 | 全量核查 | 5min |
| B-13 | S1 | P1-21 `'deleted'` 字面量在 repo + Subscribe 无 name | 随手批 | 外审 F-2-7 | ~0.5h |
| B-14 | S6 | 渠道命名两套冲突（设计文档 email/dingtalk vs 实现 uip/webhook） | 文档批 | 06 号 §3 | 5min |
| B-15 | S3a | 03 号 §3.3 on_complete 直调 send() vs 通知收敛走 S6 规则面——二选一成文 | S3a 开工前 | 03 号 §3.3 | 拍板 |
| B-16 | S6 | A.5③ 守卫拦截/告警（被引用字段改 key/删时规则失效——与 S1 影响分析同件复用） | B6-7 二期 | 06 号 A.5 | +0.5 天 |
| B-17 | S6 | 11 号计划面回填（escalation/on_complete/supervisor/三入口等七关键词在 11 号零登记） | 文档批 | 外审 F-3-6 | 15min |
| B-18 | S10 | last_reminder_at 落列决策 + recipients_spec JSONB vs 两列口径 | B10-4 开工前 | 外审 F-3-7/8 | 拍板 |
| B-19 | — | B13 权限覆盖矩阵审计（全端点×三层——半天 doc-only） | phase5 收官前 | 09 月提议至今 | +0.5 天 |

## C. 触发驱动（有明确触发信号，信号不到不动）

| # | 段 | 项 | 触发信号 | 出处 |
|---|---|---|---|---|
| C-1 | S1 | phase6 R3 前置批（DataFetcher 超时/限流/审计，~3–4 天） | 接首个外部源前**必须**（否则 P1 升 P0） | 01 号 §3.3③⁺ |
| C-2 | S1 | auto_fill 实现批（prefill 端点/params 泛化/external_http/前端交互） | phase6 R3 首源就位 | 01 号 §3.3③⁺ |
| C-3 | — | BK-21 GetFilter 护栏泛化 | 首个新资源接 L2 / 导出功能 | 工程约束 |
| C-4 | — | BK-19 handler 层测试 | 工单封版翻案时 | phase2 遗留 |
| C-5 | S6 | RT-1 观测面（20015 审计/WARN） | 先有降噪前提（宽限窗口/family_id） | 06 号 §5.2 |
| C-6 | S5 | RK-P5-15 richtext 图数上限 + 触发信号观测面 | 磁盘水位/贴图密集（**需先补观测**——见判据） | 05 号 §3 |
| C-7 | S5 | RK-P5-14 附件独立权限码 | 工单域权限细化诉求 | 05 号 A.7 |
| C-8 | — | BK-12 残留（org 删除守卫不查 org_roles） | 出现孤儿绑定行实际危害时 | AGENTS 遗留 |
| C-9 | S1 | spec JSONB 收件人模型 / form-create runtime 渲染引擎 | ≥2 来源组合 / 渲染联动变重时 | 06 号 A.5 注 |
| C-10 | S1 | manager_id 显式汇报线 | M-HR 启动 | HR 同步待设计 |
| C-11 | S5 | aws-sdk-go 备选 | rclone/RustFS 兼容劣化 | RK-P5-13 |
| C-12 | S6 | stub 独立 config 段（`notification.uip.*`） | 接 UIP 真实契约前 | 外审 F-1-14 |
| C-13 | S6 | markdown 渠道转义（`channelEscapes` 加第三族） | UIP 契约确认正文格式后 | 外审 F-2-9 |
| C-14 | S5 | cookie Secure release 强制 | 生产 HTTPS 网关前置 | 外审 F-4-7 |
| C-15 | S6 | 死信/emitEvent ctx 超时+request_id | 接入链路追踪时 | 外审 F-2-8 |
| C-16 | S6 | 节流入口廉价预判 | 通知量级异常增大时 | 外审 F-2-8 |
| C-17 | S5 | mirror 侧车失败聚合可见面（F-C1） | B10-4 |
| C-18 | S5 | ListDeleted 无索引（000049 partial idx 已建—— 态查询用）/ Mark 失败 500 残留行 | P3 量级到了 |
| C-19 | S6 | settings Upsert version 乐观锁（F-D2——000060 在途收尾） | 下个迁移顺位 |

## D. 外部依赖 / 远期

| # | 项 | 依赖 |
|---|---|---|
| D-1 | UIP 契约确认 → `delivery` 字段定形 + `channelEscapes` 重映射（含 C-13 重审） | 企业统一消息平台 |
| D-2 | M-UAA（内网 MQ + 契约四问） | 内网启动 |
| D-3 | S6 二期 inbox（notifications 表族+消息中心页） | S6 一期 ✅ + S3a ✅ |
| D-4 | S8 数据大屏 / S3b 首个审批场景 | 触发条件各自 |
| D-5 | S5 A.6 前端组件路径回填实施形态 | 05 号文档批 |

## E. 待拍板（下次开工前需所有者一句话）

| # | 项 | 拍板时点 | 建议 |
|---|---|---|---|
| E-1 | supervisor 的 org 管理角色码（B10-4 前拍） | S10 开工前 | 现无 org_admin/org_owner 则种子 |
| E-2 | B10-4 时间源 | S10 开工前 | 已荐 (a) `status_changed_at` 列 |
| E-3 | B2-4 lint 是否阻断保存 | S2 开工前 | 告警不阻断起步（11 号 §4 已拍） |

## F. N/A 已销账

| 项 | 因 |
|---|---|
| rules version 乐观锁 | ✅ 已落码（000060 + UpsertRule CAS + 92506/409 + 四子测） |
| binder 接口化 | YAGNI 销账（外审 F-4-1：purpose 分支内联三处显式=实施终态） |
| exprcalc 超时注释+Now 死码 | ✅ 已清（bf4a388） |
| JWT_SECRET<32 release 拒启 | ✅ 已落码（validate.go:139-147 完整实现——旧登记「未提交」过时） |

## 挂账质量判据（外审 F-4-5 推广）

> **登记了触发信号 ≠ 触发信号可观测**——C 组「触发驱动」类挂账若代码面零实现
> 观测面（端点/日志/SQL），事实上等于「永不触发」。后续登记触发驱动项须同时
> 回答：触发信号由什么代码产生？谁来读？

---

*基线：zhuzhao `f3e321d` / zhuzhao-ui `9ada8da4`（2026-10-23 双仓 CI 绿）——A 组 4 项全清；下一迁移编号 000061*
