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
| A-3 | S6 | F-D2 rules version 乐观锁 | 000060 ✅ 已入库 **33b42a7**（v3 审计 R-1 勘误——原标 b101cc6 系错标：000060/92506/CAS 实际全在 33b42a7，b101cc6 仅含 F-1-9b/92303/台账） | ✅ **已修** |
| A-4 | — | 外审批 3 对表文档批（06/10/11/05 四稿） | b75917d/baccad6/d14678e | ✅ **已修** |

## B. 已锁定消化批次（挂而不悬——锚点已写进 11 号/段稿）

| # | 段 | 项 | 消化点 | 锚 | 量级 |
|---|---|---|---|---|---|
| B-1 | S6 | D20 changes[] 真值 diff（旧值比对+custom_data 键级粒度） | B6-7 二期 | 06 号 A.1 | +0.5 天 |
| B-2 | S6 | variables 端点补动态字段源+时间类 | B6-7 二期 | 06 号 A.6 | +0.5 天 |
| B-3 | S6 | `'*'` 免打扰跨域生效 | B6-7 二期 | 外审 F-1-12 | +0.5h |
| B-4 | S6 | acceptance 通知深面（注入/节流/fail-fast/偏好四类） | B6-7 前补课 | 06 号 §10 | +0.5 天 |
| B-5 | S6 | opt-out service 直连 db 收敛到 repo | 架构批 | 外审 F-1-12 | +0.5h |
| ~~B-6~~ | S10 | ~~MessageDispatcher 编排器抽出~~ **✅ 已落（B10-4 62c03c2——dispatcher.go 抽出+hook 薄壳+ops 双域订阅）** | — | 06 号 §1.5 | 销账 |
| ~~B-7~~ | S10 | ~~escalation 催办 + ticket_reminders 侧表 + supervisor 第四分支~~ **✅ 已落（B10-4 62c03c2——实施偏差：supervisor 不做 recipient_type 第四枚举，目标集由扫描器 emit 时解析进 cc_users 载荷，规则面零扩）** | — | 10 号 §3.4 | 销账 |
| B-8 | S3a | `run.succeeded` emit + `result.*` 命名空间 | B3-3 | 03 号 §3.3 | 随批 |
| B-9 | S5 | 内嵌图引用集解析（回收最后一段——RK-P5-16） | B6-7 二期或 phase6 | RK-P5-16 | +1 天 |
| B-10 | S6 | message_log GIN 索引 + fetcher 时机 | 触发（工单量级/首源） | S1 审计 | 随批 |
| B-11 | S5 | BindAvatar 非事务（F-A3——"无害"论证因 GC 第三段已成立） | S6 二期或随手 | 五批审计 F-A3 | +0.5h |
| B-12 | S1 | ~~P2-8 游离 godoc（ticket_handler 4 处）~~ **✅ 已修（bf4a388 删 FieldImpact 前重复块——现存 ticket_handler.go:432 为合法附着非游离——v3 审计 L-2 补标）** | 随手批 | 全量核查 | 5min |
| B-13 | S1 | P1-21 `'deleted'` 字面量在 repo + Subscribe 无 name | 随手批 | 外审 F-2-7 | ~0.5h |
| B-14 | S6 | ~~渠道命名两套冲突~~ **✅ 已修（33b42a7 批 3 对表——06 号 §3:39 两级渠道词表成文：channel=adapter 层 uip/webhook + delivery=偏好层——v3 审计 R-2 补标）** | 文档批 | 06 号 §3 | 5min |
| B-15 | S3a | ~~on_complete 直调 send() vs 通知收敛走 S6 规则面——二选一成文~~ **✅ 已拍死（33b42a7 批 3 对表——03 号 §3:48 事件优先成文，原直调表述作废——v3 审计 R-2 补标）** | S3a 开工前 | 03 号 §3.3 | 拍板 |
| B-16 | S6 | A.5③ 守卫拦截/告警（被引用字段改 key/删时规则失效——与 S1 影响分析同件复用） | B6-7 二期 | 06 号 A.5 | +0.5 天 |
| B-17 | S6 | ~~11 号计划面回填~~ **✅ 已修（33b42a7 批 3 对表——11 号 §4/§2.4/§3.3/§5.9 七关键词全回填（escalation/supervisor/ticket_reminders/MessageDispatcher/run.succeeded/manager_id/on_complete）——v3 审计 R-2 补标）** | 文档批 | 外审 F-3-6 | 15min |
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
| C-12 | S6 | ~~stub 独立 config 段（`notification.uip.*`）~~ **✅ 已落（微批 A——Notification.UIP.Endpoint 四链+BindEnv+生产态 fail-fast；契约接入面归 D-1——v3 审计 L-2 补标）** | 接 UIP 真实契约前 | 外审 F-1-14 |
| C-13 | S6 | markdown 渠道转义（`channelEscapes` 加第三族） | UIP 契约确认正文格式后 | 外审 F-2-9 |
| C-14 | S5 | cookie Secure release 强制 | 生产 HTTPS 网关前置 | 外审 F-4-7 |
| C-15 | S6 | 死信/emitEvent ctx 超时+request_id | 接入链路追踪时 | 外审 F-2-8 |
| C-16 | S6 | 节流入口廉价预判 | 通知量级异常增大时 | 外审 F-2-8 |
| C-17 | S5 | mirror 侧车失败聚合可见面（F-C1） | B10-4 |
| C-18 | S5 | ListDeleted 无索引（000049 partial idx 已建—— 态查询用）/ Mark 失败 500 残留行 | P3 量级到了 |
| C-19 | S6 | ~~settings Upsert version 乐观锁（F-D2——000060 在途收尾）~~ **✅ 已落码（000050+settings_service.go:232 CAS+expected_version 双测试——早于本台账终版即已交付，登记滞后——v3 审计 L-2 补标）** | 下个迁移顺位 |

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
| ~~E-1~~ | ~~supervisor 的 org 管理角色码~~ | ✅ **已拍（2026-10-24 所有者——方案改写）**：上级来源 = 人工指定（Create/Assign 填 `escalation_supervisor_id`）> HR `manager_id`（M-HR 后）；角色码推导**作废**（dd §31 通则：「人工指定为基线，HR 直属上级为进阶」） | 10 号 §3.4/A.5 修订 |
| ~~E-2~~ | ~~B10-4 时间源~~ | ✅ **已拍（2026-10-24 所有者确认）**：(a) `status_changed_at` 列（`UpdateStatusTx` 咽喉同写）+ 存量回填后**首跑观察模式**（只记日志不发通知，防 updated_at 近似值误报） | 10 号 A.3 |
| ~~E-3~~ | B2-4 lint 是否阻断保存 | ✅ **已拍（2026-10-24 所有者确认——2026-10-16 批量预确认项开工前再确认）** | 告警不阻断起步——与 11 号 §4/02 号 A.5 既有成议一致 |

## F. N/A 已销账

| 项 | 因 |
|---|---|
| rules version 乐观锁 | ✅ 已落码（000060 + UpsertRule CAS + 92506/409 + 四子测） |
| binder 接口化 | YAGNI 销账（外审 F-4-1：purpose 分支内联三处显式=实施终态） |
| exprcalc 超时注释+Now 死码 | ✅ 已清（bf4a388） |
| JWT_SECRET<32 release 拒启 | ✅ 已落码（validate.go:139-147 完整实现——旧登记「未提交」过时） |

## G. 收官审计新增（2026-10-25——分段对表×场景业界×前端全量审计产出）

> 出处：收官全量审计（六路分段取证+前端检查+真机走查）；编号 G-序号（承接 v5 审计 G 系发现位）。

| # | 段 | 项 | 级 | 量级 | 挂点 |
|---|---|---|---|---|---|
| G-1 | S1 | fetch 装配链生产雷：fetcherReg wire 第 9 参恒 nil（R1 同型第五例）——auto_fill 静默不生效，formula 引用 fetch 字段提交期 400 卡用户；错误 UX 三档未建 | P1 | 随 phase6 R3 首源批 | C-1/C-2 并账（registry 注入+适配器+超时/限流/审计一次配齐） |
| G-2 | S6 | variables 变量字典端点缺席——管理员重存种子模板（含 reminder_count/event.*）即 400；白名单硬编码与种子模板不同源 | P1 | +0.5 天 | =B-2 加重证据（随 B6-7 二期） |
| G-3 | S5 | avatar confirmed 后用户永不 bind → 对象永无 GC 覆盖（第三段仅扫 ticket purpose）——行+对象永久泄漏 | P2 | +0.5 天 | GC 第三段扩 avatar purpose（磁盘水位前） |
| G-4 | S3a | sla_due_at 仅 Transition 写刻——经 Close 到终态的单无 deadline 刻（Close/Assign 收敛批未做） | P2 | +0.5 天 | Close/Assign 收敛批随行 |
| G-5 | S3a | RunPostActions 失败仅 slog——A.3 要求的 ops.action_failed 事件未 emit（可观测缺口） | P2 | 5min | 随批（词表+inbox 登记同批） |
| G-6 | S3a | reassign 声明 at_most_once 但语义=同步强一致通道——失败静默丢转派，一致性档位需复核 | P2 | 拍板 | S3a 二期拍板 |
| G-7 | S6 | inbox 不查 opt-out——站内通知用户无法自关（与外发偏好脱节） | P2 | +0.5 天 | 通知中心权限批（业界=GitHub per-channel） |
| G-8 | S6 | ticket.action_notify（B3-3 已上线）未登记 inbox 词表——站内静默 | P3 | 5min | 随批登记 |
| G-9 | S6 | ops.cron_failed 词表行是死行（payload 无 to_assignee/cc_users——collectRecipients 恒空） | P3 | 15min | 随批（改锚点或特判 job 收件人） |
| G-10 | S6 | message_log 实落表与 A.2 漂移（biz_id VARCHAR/无 dead 态/retry_count）+死信归因字段折损 | P3 | 登记不修 | 二期表形态批 |
| G-11 | S9 | CleanupExpired 未挂 cron——过期行只懒查不物理删，长期运行只增不删 | P2 | 15min | 生产部署前必挂 |
| G-12 | ui | 铃铛 invalidateQueries 死代码（角标最长 60s 不更新）/画布 undo 后 lint 态 stale/intel 页裸 fetch 绕 api 层/附件列表失败静默 | P2/P3 | +0.5 天 | 前端随手批 |
| G-13 | 文档 | 10 号变更记录缺 SLA 0.5 交付行；errcode.md 双份已补登 21010/92600-92602（✅ 随本批修） | P3 | ✅/5min | 文档批 |
| G-14 | S1 | FieldImpact radio 选项收窄漏检+dict: 引用收窄天然失效 | P3 | 15min | B1-6 前小修 |
| G-15 | ui/S5 | storage API 六端点漏 `/api/v1` 前缀（b72b854d 建档即如此）——浏览器内附件列表拿回 index.html 渲成 3418 幽灵行、五个 POST 404：头像上传/附件上传删除/富文本渲染整面静默失效 | **P0** | ✅ ui e88173bd | 销账（+`apiPaths.test` 静态守卫，红→绿恰报六处） |
| G-16 | S9/S6-2 | intel_query/message_center 建菜单行未绑 role_menus（000031 惯例漏项）——全员不可达，铃铛 /messages 404 | **P0** | ✅ 000074 | 销账 |
| G-17 | S3a | 分派规则页（ui 45a72256）无菜单行——孤岛页；viewNames 守卫单向不报 | **P0** | ✅ 000073+000074 | 销账（ui 反向守卫同批） |
| G-18 | ui | E2E 三 spec 失配：S5 私有 login 等 submit 型按钮+history 路径（B5-2/3 自建档从未跑绿）/S10 等已被 UserPicker 替换的 ID 输入框/S12 字段配置默认设计模式——三者均被误判「超时」 | P1 | ✅ ui e88173bd | 销账 |
| G-19 | ui | UserPicker `/^\d+$/` 分流工号查询，E 前缀工号永走 username 分支——工号搜索不可用 | P2 | ✅ ui e88173bd | 销账 |
| G-20 | ui | viewNames 派生器两盲区（INSERT…SELECT 页面行只认别名 p 且一律记按钮 / UPDATE visible 不重放）+「零动作」守卫从未登记 0 形同虚设 → system_setting 无 defineOptions、system_notification 名错，两页 keep-alive 恒不命中 | P2 | ✅ 本批 | 销账 |
| G-21 | ui | 菜单 icon 映射漏 intel/message（icons.test 种子清单手工维护未同步）——侧栏图标静默空白 | P3 | ✅ 本批 | 销账 |
| G-22 | ui/CI | viewNames 同级主仓固定拉 main——特性分支种子页在 CI 不进期望名单（反向守卫会误报孤岛） | P3 | ✅ 本批（同名分支优先，push 后核 CI 转绿） | 销账待核 |
| G-23 | S6-2 | 消息中心是否默认全员可见：现 000074 仅绑 admin+superadmin；operator/viewer 及自建角色须经角色管理分配本页，否则其头部铃铛跳 /messages 仍 404（铃铛对所有登录用户渲染） | 拍板 | 所有者 | E 组候选（a=static route 兜底 / b=种子绑全部内置角色 / c=维持按角色分配+铃铛按菜单可达性隐藏） |
| G-24 | ui | SEED_ICON_NAMES / MENU_ICON_MAP / viewNames STATIC_ROUTES 三清单手工维护——派生化（从迁移种子/routes.ts 解析） | P3 | 登记 | 触发=再漏一次 |
| G-25 | ui | phase5 新能力 E2E spec 为零——五件待补：CC/标签、S2 画布、S6-2 消息中心+铃铛、S3a 转换+规则页 | P1 | 本批下一步 | 补齐后更新 AGENTS/README 计数 |

## 挂账质量判据（外审 F-4-5 推广）

> **登记了触发信号 ≠ 触发信号可观测**——C 组「触发驱动」类挂账若代码面零实现
> 观测面（端点/日志/SQL），事实上等于「永不触发」。后续登记触发驱动项须同时
> 回答：触发信号由什么代码产生？谁来读？
> **豁免（v3 审计 L-2 补）**：诉求/决策型触发（如 C-3 护栏泛化、C-4 翻案、
> C-8 危害实证）信号=所有者人判断，不要求代码观测面——判据只约束技术信号型
> 条目（量级水位/契约到达/依赖启动类）。

---

*基线：zhuzhao `2dcb496` / zhuzhao-ui `7839aea2`（2026-10-25 收官审计）——A/E/F 组全清+B/C/D 组按上表销账；G 组=收官全量审计新增 14 件（P1×2/P2×7/P3×5）**+ E2E 补齐批（2026-10-25，ui e88173bd 起）新增 11 件 G-15~G-25（P0×3/P1×2/P2×2/P3×3/拍板×1——G-15~G-22 已销，G-23 待拍，G-24 登记，G-25 下一步）**；下一迁移编号 **000075**（000073/000074 本批占用）*
