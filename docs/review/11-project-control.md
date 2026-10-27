# 11 · 项目能力总览图（Project Control）

> **用途**：一张地图快速掌握整个项目的能力、关键细节与当前健康状态，用于对 AI 快速迭代保持掌控。**每次代码改动后应同步更新本文**（见 `AGENTS.md`）。
>
> **当前测试基线（单一真相源——2026-10-27 全景审计 B-1 立约）**：四档 acceptance **249/0**（2026-10-26 `24920b3` 时点全量）。约定：历史批次行保留时点值**不回改**，此处为唯一当前值——其后批次增减用例须随批刷新本行；各文档/报告引用四档数字一律以此为准，勿复制历史行时点值。
>
> 更新日期：**2026-10-15**（**文档对账批**：§3 API 地图对齐路由实态——/menus「CRUD」反向断言修正[代码 P4-W1 起只读]+P4 增量端点补行+`router.go` 过期行号勘误+头部日期滚动恢复）。历史批注——2026-09-27（**W2 出口闭合批回填**：① Phase 4 实施态落档——W0 安全批/W1 词表只读化（000030/31）已推（至 6609782）；W2 前端壳层双侧交付（主仓 d513825=Swagger 61 路径 P1-8 闭合+dev-stack.sh；zhuzhao-ui 22c024aa=壳层四件+登录/首登改密/工作台+system 占位+vitest 43 例）+ 本批出口闭合三件（**E2E 起建**：Playwright 打标准三栈+globalSetup 幂等建号/角色预设/admin 凭据闭环，S1 强制改密/S2 冒烟/FE3 viewer 只读三 spec；**codegen 链**：swagger2openapi 7.0.8 + openapi-typescript 7.13.0 → `pnpm codegen`，生成物随仓；**zhuzhao-ui CI 上线**：lint/typecheck/build + test 双 job）；② §1 矩阵补前端行；③ 12-frontend §4/§5 回标（views 目录/API 布局/FE 状态随 W2 落定）；④ 主仓 ci.yml 触发分支补 `phase4`（W0 B-4 欠账一行）+ swag info 块补 @title/@version（swagger2openapi 转换前提）｜ 分支：`phase4`（Phase 4 **实施期**——原「规划期未动代码」口径作废；W1 批四档 acceptance 全绿 91+67+26+34 FAIL=0）｜ 文档体系见 [docs/roadmap.md](../roadmap.md)
> 2026-10-15 文档批次（phase5 设计稿批）：**phase5 十段段级设计稿建档（01–10 号）+ README 转「总纲/索引」两层**——每稿统一骨架（定位由来/现状锚点[2026-10-15 逐段代码核实]/拍板点带建议值/边界/影响面/量级/验收/文档衔接），未新增决策；核对增量回写（任务面三 Tab 现状勘误 + 水位挂账收窄[前端近似水位条已落、精确队列深度留挂账]/三处锚点勘误/S10 抄送×`ticket_participants` 对表项——防同语义双表）；§6 树 phase5 状态同步；全部在 `phase5` 分支（README §5 分支策略：启动切分支），排期仍待所有者拍板
> 2026-10-20 批次（S5 代码评审修复批）：**两轮只读评审（基线 a78f8cb/a81c17e）P0×1+P1×3+P2×5 全清**——P0=自服务面五端点挂 biz 无 SelfService 标记致普通角色全 403（catalogExempt 仅启动对账豁免非运行时跳过；修复=标记组挂载[router 笔]+acceptance #S5-7 viewer 回归闸）；P1=附件数 ≤20 零执行点/建单绑定并发双绑窗口/richtext 落绑语义未拍（a 案拍定——建单贴图与附件同经 attachments 列表落绑）；P2=删除口径（先标 deleted+GC 二扫）/吞错映射/桶隔离/cookie Secure 配置化/审计白名单[storage 三笔]；§2.2 B5-2 行翻 ✅、B5-4 登记编辑增量绑定；评审报告对话输出未落档（所有者核验后定）
> 2026-10-22 批次（**B1-6 能力面扩展批立项**——所有者"全加"拍定，排 S6 一期前）：**六件**=①radio 第 9 类型（与 select 同数据模型不同渲染 el-radio-group）+ ②rating/score 第 10 类型（与 number 同模型 el-slider/el-rate）+ ③col_span 多列布局（schema 可选字段 1–24，el-row/el-col 栅格）+ ④分组/折叠（schema 可选 group 字符串，el-collapse 分段——纯前端）+ ⑤**条件显隐翻案**（01 号 §3.5 原"唯一显式不做"→`field_meta.visible_when` 表达式+watch 联动+G2 跳过隐藏字段——exprcalc 已有底座减为集成件）+ ⑥**函数注册表**（exprcalc AddFunction 白名单——首批 risk_score/sla_deadline/lookup_dict/now/upper 5 函数，新计算能力=加 Go 函数非新字段类型——业界 Jira Expressions/Airtable functions 同款）；量级 ~4–5 天；01 号 §2 字段类型行同步**十种**
> 2026-10-23 批次（**S6 一期收口**）：B6-1 DomainEvent/DomainHook 事件面基建 → B6-2 ChannelSender+UIP 桩 → B6-3 模板层（000053 迁移+msgrender+CRUD+管理页）→ B6-4 用户通知偏好（000056 optouts 表+opt-out 过滤）→ B6-5 触发挂接（TicketNotifyHook rules 路由+Assign/Close emit）→ B6-6 发送记录（000054 message_log/message_rules+管理面 API+000055 menu_apis 种子）；四档 acceptance 241/0+双仓 CI 绿；含 B6-6 增量（notification 全链集成测试：事件→rules→模板→渲染→UIP 桩→发送→message_log）
> 2026-10-23 批次（**S6 外审全量审计修复批**——P1×7 收口，详录 06 号变更记录）：①收件人解析消费 message_rules 收件人列（修复前旁路 DDL 硬编码）②拍板 A=created 删硬编码特判走规则驱动 ③closed 链路静默丢失修复（Close payload 补 to_assignee+Create 补 actor+两端钉测）④渲染渠道转义族映射+值级转义（修复前零转义+幻渠道名 email_html 双问题）⑤拍板 B 过渡=message_log 列表 body 截断脱敏 ⑥死信 payload 扩展+92004 ⑦human_key（000057+internal/pkg/humankey）+死信进程内直调 ⑧四象限 fail 档差距挂 B6-3 增强批（测试名实对齐）；**挂账新增**：stub config 化/opt-out 自服务端点/channel 白名单守卫[拍板 C]/删除守卫+引用校验+审计/D20 updated+changes[]/rules version/variables+preview 端点/acceptance 通知用例（现零条）——归 **B6-3 增强批+B6-7 前补课批**。验证=b65 四子测（新增 closed 全链+未指派静默回归锚点）-race 绿+22 包单测
> 2026-10-23 批次（**S6 微批 A——审计残余收口**，所有者「按这个方案来」）：①**92506 ErrMsgRuleVersionConflict** + **迁移 000060** message_rules.version 乐观锁（upsert CAS：陈旧 version→0 行→409 不静默覆盖；四子测集成含变异锚）②**channel 错配运行时守卫**（遵所有者 F-3-2 两级渠道词表保留 uip/webhook 白名单——错配模板显式失败+log failed，替代原「白名单收 uip」方案；路由层落地后守卫自然 no-op）③**notification.uip.endpoint config 段**（BindEnv NOTIFICATION_UIP_ENDPOINT+config.yaml 字面值+装配接线——生产态 fail-fast 自此前可达）④**92004 漏映射补登记**（台账声称已登记但 errors.go/全码表双缺——500 映射+清单 73）⑤P3 三件随批清：resolveRecipients 死码兼容层删除/Subscribe 重名防护（06 A.1「重复名 panic 装配期暴露」落地+13 调用点具名化）/errors.go:33 注释错位修正；**拍板登记**：①binder 已由所有者销账（05 号 F-4-1 YAGNI）②variables 端点维持 SelfService+待通知中心权限码落地一并收紧④发送超时 ctx 挂 UIP 契约接入批（现对桩无意义）。验证=rules CAS/b65 错配子测先红后绿+service/repository/ticket 三包全量集成 -race 绿+全量单测绿+lint 0+**000060 往返零漂移**。06/11 号变更记录待所有者批 3 对表未提交件入库后补行
> 2026-10-24 批次（**S2 流程图设计器全批交付——phase5 下一段开工即收（🔵 待所有者走查）**）：B2-1 spike 对称验证落定 **LogicFlow 2.2.5 维持**（vue-flow 1.48.2 对照——包体 ~1/3 但连线/标签编辑/校验须自写；判据与实施偏差四件详 [02 号变更记录](../phase5/02-s2-flow-graph-designer.md)）→ B2-2 mapper/lint/layout 纯函数+StateNode+壳（**纯前端零迁移零端点**——类型编辑框画布/JSON 源码双模式共存，壳层 props/expose 去耦）→ B2-3 改图在途单告警（stuckStates 判据含零出边死路扩展）→ B2-4 图检查角标（告警不阻断——E-3 拍板落实施）；验证=ui 单测 158（+21：**默认六状态图往返幂等零 diff 承重**）+lint/typecheck/build 绿+**真机走查全链**（画布渲染/改名/锚点连线/重复连线拦截/双向同步/带告警保存/重开回显一致/在途告警「open·待处理（1 单）」+取消零变化——临时类型已清理）；zhuzhao 主仓零代码改动（文档四件：02 号状态+变更记录/11 号 §0/§1/§2.5/runbook §4.1 E800001 agent 走查号建号登记）
> 2026-10-24 批次（**S10 工单协作开工——B10-1 抄送交付**）：**迁移 000062** ticket_cc（role 预留 cc/participant 防 ticket_participants 双表）+ 种子模板/规则（cc 通知走 S6 规则面零新通知代码）；**L2 特例通道「抄送即可见」**（拍板点 1 预确认落地）——GetFilter 第四 OR 支（列表/导出/关联查询全路径自动生效，BK-21 正交）+ Authorize 可见段 cc 支，**只进读可见集不进 canOperate**（抄送人改单 403——能看不能改）；授权口径=自抄送/自摘/列表 read 面、抄他人/摘他人 update 权；三端点 selfService+catalogExempt（B5-3 附件面同款）；resolveRecipients 增 cc 档+规则面枚举/词表放行 ticket.cc_added。验证=TestB101 六子测（cc 四向+幂等+通知到人）+集成三包+22 包单测+lint+往返 62 对零漂移+**四档 acceptance 246/0（119+67+26+34）**；前端详情页抄送区（ui 34c3b751，typecheck/lint/test/build 绿）。段状态 ⬜ 开工中（B10-2~5 未启）；B10-4 前仍有台账 E-1/E-2 待拍（建议值在台账）
> 2026-10-24 批次（**S10 B10-2 标签 + B10-3 角标交付**）：**迁移 000063** tags 通用表（entity_type 泛型——phase6 风险事件复用零改；默认共享[拍板点 6]）；tag 授权口径=挂/自摘读可见、摘他人 update 权；ListTickets 增 tag 筛选（EXISTS 子查询——可见性由 GetFilter 管线约束不旁路 L2）；三端点 selfService+catalogExempt。B10-3=BadgeService 计算器注册制（ticket 域注册 todo[assignee=me 非终态]；phase6 risk 注册零改协议并入）+ GET /badges sections 开放 map（单 section 失败记日志跳过）+ 前端 NavbarBadge 60s 轮询。验证=TestB102 五子测+TestB103 联动+集成三包+22 包单测+lint+往返 63 对零漂移+四档 246/0；ui typecheck/lint/test/build 绿（caa80fe+857001b6）。段内余 B10-4（开工前拍 E-1/E-2）、B10-5
> 2026-10-24 批次（**S10 B10-4 节点超时+催办后端交付**——E-1/E-2 已拍[所有者场景走查]）：迁移 000064 四件套（transition_meta JSONB / status_changed_at[E-2 a 案 UpdateStatusTx 咽喉同写] / escalation_supervisor_id[E-1 人工指定档——dd §31 通则：人工指定为基线 HR 直属上级为进阶] / ticket_reminders 水位侧表[F-3-7]）+ 活跃单部分索引 + 种子模板/规则两枚；timeout_scanner 骑 CronScanner（D18 维护型）：escalation 水位判定（无段单发/间隔上限缺省 240min·3 次/间隔校验 ≥5min 防 S6 节流窗互吞）→ ticket.timeout 事件（cc_users=assignee∪人工指定上级——一条种子规则全员可达）；观察模式 config 默认 true（存量回填近似值防误报——cron-status 端点可见标记，确认后关）；**cron 观测面**：CronRegistry + GET /system/cron-status（audit:read 码）+ 失败→DomainEvent{ops, ops.cron_failed}→S6 规则告警（静默失败收口）；**MessageDispatcher 抽出**（B-6 销账——hook 薄壳+ticket/ops 双域订阅，ops 模板 {{event.*}} 根键）。验证=TestB104 六+三子测+纯函数单测+集成三包+22 包+往返 64 对零漂移+四档 246/0（62c03c2）。**S10 段收尾（2026-10-24 同日）**：B10-4 前端编辑面（ui 9d117dd4——类型页「超时与催办」per-state 表单面+发起/分派指定催办上级 UserPicker）+ B10-5 导出（zhuzhao dcdd757+ui 5a2bf84d——excelize v2.11 新依赖[替代 csv 不符拍板点 4]；BK-21 同源三件套 parseListQuery/resolveListFilter/buildTicketWhere；流式 StreamWriter+5 万钳制+RFC 5987 文件名+导出自审计手动落行；v1 级独立链免通用审计 body 捕获[files 先例]；迁移 000065 绑 ticket_list+2a 断言校准 9→10 设计内增长）。**S10 五批前后端全清——段状态 🔵 待所有者走查（清单见 11 号 §0）**
> 2026-10-24 批次（**S4+S9 双段全清——B4-1/B4-2/B4-3 结论件+B9-1 交付**）：**S4**=jobs.Registry 扩 ActionMeta+`GET /jobs/actions` 本地直读（000066 绑 task_center/task:read——operator 提交选单口径）+**提交 fail-fast 21010**（Submit/CreateJob 未注册动作即 400——原回调期 404 后置收口；Trigger 不拦防误伤多执行端）+operator 动作可搜索下拉（加载失败回退手填）+**CronBuilder**（预设四类/专家双模式/双向反解析——产物 cron_spec 五段 taskrunner 零改动；presets 17 测含六段带秒必拒）；**B4-3 死信对账结论=不成立**（死信=asynq archived Redis 队列残留 vs runs=job_runs PG 审计事实——两存储天然不对齐，维持三 Tab，04 号 §3.5 销账关闭）。**S9**=迁移 000067 intel_cache+intel:query 码+threatintel 包（Provider 端口+HTTP adapter 独立映射层+两态桩 stub 默认 true）+查询链路全文级（singleflight[x/sync 升直连]/懒过期 stale+异步刷新/404 入缓存/429 降级矩阵 92601→503）+`GET /intel/query`；errcode 92600-92602（全码表 77）；**实施偏差**：rate.Limiter 挂真实契约批（config 键已挂，stub 态 YAGNI）。验证=TestB91 八子测/22 包单测/四档 246/0/往返 67 对零漂移；ui 四件绿（test 175）。下一编号 000068 导出
> 2026-10-24 批次（**S3a 语义引擎全批交付——B3-1/B3-2/B3-3+SLA0.5 一日四笔**）：**B3-1**=POST /tickets/transition 统一转换（L2/L3 transition 动作[与 close 同面]→AssertTransition→condition fail-closed 403→validator expr→单事务 UpdateStatusTx 咽喉→emit ticket.transitioned）+available 按用户过滤（000068）；transition_meta "from>to" 转换级键复用 B10-4 列（">" 区分键形，两形共存校验分层）。**B3-2**=assignment_rules（000069/070）+OnAfterCreate priority 首个命中走 Assign 全链+dispatch_missed 事件+四 CRUD（ticket:type:manage 零新码）。**B3-3**=ActionRegistry（Delivery 一致性声明两档/链深 3/注册 error 化[架构守卫 panic·Printf 两守卫先红后绿]）+首批四动作 notify·cc·reassign·spawn_task（**系统身份重放语义**——post-function 以创建者身份执行写面动作）+post_actions 挂 Transition commit 后。**SLA0.5**=000071 sla_due_at 激活+sla_due_hours 配置+deadline_scan（ticket.deadline 事件/观察模式同开关）。**实施偏差三件**：Close/Assign 收敛封装挂后批/notify 无种子规则默认静默/run.succeeded 反向链=B-8 挂账（taskrunner 契约面）。验证=TestB31 六+TestB32 六+TestB33 五子测全绿+集成三包+22 包单测+四档 247/0+往返 71 对零漂移。**S3a 段状态 🔵 待所有者走查（清单 11 号 §0）——phase5 仅余 S6 二期**
> 2026-10-24 批次（**S6 二期 B6-7 站内消息中心交付——phase5 十段全部交付完毕 🎉**）：**迁移 000072**=notifications 表族（title+link 入站即渲染——读面零渲染逻辑/行级 read_at 第一档）+message_center 菜单（inbox:read 码）+三读面路由；InboxService=DomainHook **ticket/ops 双域订阅**（「消息中心=事件系统的消费者」定性落地；收件人锚点 to_assignee∪cc_users 去重；词表 seedTemplates 十动作收敛防全量事件灌 inbox）+读面三端点（inbox 分页/单条已读幂等非本人 404/全部已读）+**unread 并入 badges sections**（RegisterUnread——badge 计算器注册制零改协议兑现）+CleanupExpired 保留策略（90 天封顶）；前端=消息中心页双 tab（通知=事件快照/待办=状态派生——语义分离落地）+深链直跳+未读红点+情报查询页落成（viewNames 守卫抓 S9 种子无落成页面——补页先红后绿）。验证=TestB67 四子测+集成两包+22 包单测+四档 247/0+往返 72 对零漂移；ui 四件绿（test 177）。**SSE 实时通道=分期口径触发驱动**（轮询→SSE→弹窗，不入基线）。**phase5 收官待办**：①全段 🔵 走查（S2/S4+S9/S10/S3a/S6-2 五段清单在 11 号 §0）②23+3=26 笔未推送待指令③phase6 硬前置（S1/S2/S6+S10）全部就绪
> 2026-10-25 批次（**E2E 补齐批——phase5 首次落浏览器跑 E2E，三 spec 失配顺藤摸出四处生产级虚标**）：起因=phase5 全程未装 Playwright 浏览器、未跑既有 21 spec、未为新能力补 spec；装浏览器首跑 28 用例 19 绿；排查 3 条「超时」实为选择器与实现脱节（非慢）。**连带根因四件**：①ui `src/api/storage` 六端点全部漏 `/api/v1` 前缀（b72b854d 建档即如此）——浏览器内附件列表 GET 命中 vite SPA fallback 拿回 index.html，v-for 把 3418 字符渲染成 3418 行幽灵附件；五个 POST 全 404——头像上传/附件上传删除/富文本渲染**整面静默失效**（单测 mock request、E2E 未落浏览器、B5-2/3 只断可见三层全漏）→ 修前缀 + `apiPaths.test` 静态门禁（红→绿恰报六处）；②**三菜单页零 role_menus 绑定**（000067 intel_query/000072 message_center 建行漏绑 000031 惯例；GetUserMenus 无通配旁路）→ 情报查询/消息中心全员不可达、铃铛 goInbox 跳 /messages 404——且 S3a 分派规则页（ui 45a72256）**连菜单行都没有**（viewNames 守卫单向只查「种子→页面」）→ **迁移 000073**（规则页菜单行+四端点绑定）+ **000074**（三页绑 admin+superadmin）；③ui UserPicker 以 `/^\d+$/` 分流工号查询，E 前缀工号永走 username 分支——placeholder 承诺的工号搜索不可用 → 两路并查合并；④viewNames 派生器两盲区（INSERT…SELECT 页面行只认别名 p 且一律记按钮 / UPDATE visible 不重放；「零动作」守卫从未登记 0 形同虚设）→ system_setting/system_notification 两页 keep-alive 名从未被检——实际一无 defineOptions 一名错 → 修派生器+两页名+**反向守卫**（每个 views/**/index.vue 须 ∈ 种子页 ∪ routes.ts）；另 icon 映射漏 intel/message（侧栏空白）、CI 同级主仓固定拉 main 改同名分支优先。验证=三 spec 6 用例 23.8s 全绿（S10 180s 挂→7.9s）；全量 28 用例 **23 绿/5 为 activelist+taskrunner 未起**（fe4×2 仅 502 /al、w5×3）；ui lint 0 错/typecheck/vitest 21 文件 182 用例；**四档 acceptance 246/0（119+67+26+34）+往返 74 对零漂移**（此前批次行「247/0」为无分项的传抄数——脚本 check 数自 B10-1 至今未变，246 为唯一带分项记录）。提交=ui e88173bd（E2E+根因①③）；本批 zhuzhao/ui 第二笔随文档。**新 E2E spec 五件下一步补**（CC/标签、S2 画布、S6-2 消息中心+铃铛、S3a 转换+规则页）。
> 2026-10-25 批次二程（**E2E 五件补齐 + 规则页两缺陷 + L1 越权修复 000075**）：新 spec p5-s10-cc-tags / p5-s2-flow-designer / p5-s6-message-center / p5-s3a-transition / p5-s3a-rules 全绿（E2E 26 spec / 33 用例，上游 5 件外全绿）。写 spec 时核出：①规则页 `orgOptions` 为 IIFE 快照（orgTreeQuery 在途时永久空）→ **冷加载下 UI 建不了任何规则**（目标组织必填无选项）→ 改 computed；②**L1 越权（P1）**：000070 把 /assignment-rules 三写端点挂 ticket_type_manage 页面行、000073 照抄——违反 W1 B 案（页面=读 API、写端点挂按钮，000018 `ticket_type_write_btn` 先例）→ viewer/operator 预设只绑页面即持三写 casbin（viewer POST 实测 **400 过 L1 非 403**，可建/停/删自动分派规则）→ **迁移 000075** 新按钮行承载三写 + 页面行摘除 + 非管理角色三写 casbin 清除；规则页 el-switch 补 v-permission；p5-s3a-rules 钉 viewer 三写 403；③available 按人过滤实证：无处理人时 in_progress 不渲染（设计如此，spec 据此改写）；④五 spec 连跑打穿每用户 20rps 桶（429/10007）→ e2e helpers `api()` 仅 429 退避重试一次 + `mustData` 信封断言替代裸解构。验证=四档 **246/0** + 往返 **75 对**零漂移；ui 六件门禁绿。**对账建议（G-26）**：扫全部 menu_apis「页面行(menu_type=2)挂 POST」= B 案漏项清单，本批仅修本域。
> 2026-10-25 批次三程（**所有者「按建议修改一版」——G-23 实施 + G-26 对账收口 + 防复发两小件**）：**G-23**=inbox/read/read-all 三端点移 selfService 组（严格自数据：服务端限 recipient=self、非本人 404，与 /badges 同性质；catalogExempt 同批）+ ui `/messages` 常量路由（Layout 下常驻页、ROUTE_EXEMPT/NO_RESET 白名单、name=组件名供 keep-alive）+ **迁移 000076** 菜单行 visible=false 惰性化（不删行——000048 惯例防 ID 漂移；用户树过滤不可见节点故不与常量路由双注册）+ 解绑；入口=铃铛（000072 原设计），侧栏项随之消失。**G-26 对账**=全库扫「页面行(menu_type≠3)挂 POST」得 4 处：system_dict×6 / system_notification×8（**对应按钮行 dict_write_btn/notification_write_btn 建行即空绑**，前端写按钮按这两码渲染而 API 随页面放行——两页仅绑 admin/superadmin 故为潜伏）/ message_center×2（随 G-23 消化）/ ticket_list transition×1（000068 设计内，维持）→ **迁移 000077** 写端点迁按钮行 + 000031 ② intent-preserving + 非管理残留 casbin 清除。**防复发**=ui `pnpm gate` 六件门禁一键串行（audit:prod 与 CI 同参）+ testutil `ZHUZHAO_TEST_PG_IMAGE` 可覆盖镜像名。验证=dev 后端重启载新路由后 viewer inbox 200/read-all 200、dict/notification 写 403；四档 **246/0** + 往返 **77 对**零漂移；router/repository 集成包绿（catalog 对账含新豁免）；ui vitest 182/typecheck/lint 绿；E2E 全量 33：28 绿 / 5 上游未起 / **0 flaky**（429 退避生效），p5-s6 补 viewer 铃铛可达尾段。**待拍清零**：G-23 已按建议实施（a 变体=selfService+常量路由+菜单惰性化）。
> 2026-10-25 批次四程（**所有者「可以」——B 案结构性守卫 + G-23 删行收尾**）：**B 案守卫三道**=`router.AuditPageWriteBindings`+`pageWriteAllow`（唯一例外 ticket_list transition，000068）→ ①app.go 启动 **fail-fast**（与 BK-22 同址——写端点挂页面行的迁移在 dev 栈拒启即被抓）②`TestCatalogPageRows_NoWriteBindings`（CI test-integration job 必跑；**红→绿实证**：摘掉 allowlist 即报 transition）③acceptance phase1 `D2 B案 页面行零写端点`（SQL 红/绿在回滚事务内实证）——约定「页面=读、写挂按钮」自此可执行；router 包补 `TestMain` 持共享池（第二条集成测试加入后首条 Cleanup 关池致 closed pool——修）。**000078** 删 message_center 幽灵行（000076 只 visible=false 过于保守：管理端 GetTree 含隐藏节点→角色分配树可勾无效；复核往返语义删行同样零漂移）+ inbox:read 退役；ui 派生器补 **DELETE 形态**（000031 删三按钮行此前亦未解析——幻影行被 menuType 过滤无感，页面行若再 INSERT 同 code 会被首写胜吞）+ icons 撤 message。验证=后端重启载新 fail-fast 通过；四档 **247/0（120+67+26+34）**（phase1 +1=D2 B案）+ 往返 **78 对**；router（TestMain 化，双守卫）/repository/handler 集成绿；ui vitest 182/typecheck/lint 绿；E2E fe3/p5-s6/s4 角色树/s5 菜单树绿。AGENTS.md 迁移规范补 B 案条文。
> 2026-10-26 批次五（**通知面收口批（所有者「做」）——模板/规则管理面编辑化 + notify 动作模板参数生效**）：①**ui 通知配置页只读→编辑面**（S6 一期 polish 欠项销账）：消息模板 Tab 新建/编辑/删除（is_system 种子置灰只读；channel=uip 一期单渠道固定）+ 订阅规则 Tab 新建/编辑/删除（事件=后端词表下拉、模板下拉、收件人四档、fixed 数字校验；upsert 同 biz_type+event_action=改绑）——「多模板」自此对管理员是真能力；E2E 新 spec p5-s6-notify-admin，E2E **27 spec / 34 用例**。②**notify 动作 template 参数生效**（修一虚标：参数写在动作文档里但从未被读）：NotifyAction 读 params.template → EmitActionNotify 透传 payload.template_code → dispatcher 覆盖规则默认模板（**规则仍是开关与兜底**，码不存在走统一 not-found 拒发）；事件词表补 ticket.action_notify（此前规则面无法为该事件配路由）；集成测试加「覆盖」子测+种子复原改造（原 INSERT DO NOTHING 会撞 000079 种子、cleanup 会误删种子）。③**迁移 000079**：ticket_action_notify 种子模板+规则（cc 档收件人）——**翻案**「notify 无种子默认静默」的 S3a 接受偏差登记（2026-10-26 拍板：管理员显式配动作而全程静默是坑不是克制；删规则即回静默，开关语义不变）。④acceptance phase2c D6 免疫：结单前先 transition 回 open——dev 栈走查数据含 incident 全量分派规则（新单秒变 assigned、不可直达 closed 而挂 D6B；与三 E2E spec 假红同根）。ui 顺修：表单载荷剔 id（model.ID json:",string" 空串致 ShouldBindJSON「参数错误」——E2E 首跑抓出）。验证=四档 **249/0（122+67+26+34）**+往返 **79 对**；ticket 集成包 -race 绿（B33 含新覆盖锚）；ui lint/typecheck/vitest **22 文件 189 用例**绿；E2E 全量 **34/34**；dev 后端已重启载新代码。
> 2026-10-26 批次（**外审 v4 二十条对称验证 + 修复批**——所有者「验证一下这个报告…继续按段推进」）：门禁三项（build/vet/单测）属实；**20 条发现 19 条确认、1 条判设计内**（#18 标签摘除：后来者的「挂」是 ON CONFLICT 无操作，摘 A 挂的标签需 update 权=拍板点 6 口径，登记 UX 注记）；#10（Drain wg.Add/Wait 竞态 + app.go 注释 3s≠实际 5s）落在**所有者未提交的 registry.go/app.go 在途改动**中，仅报不改。**修复 16 条**：P1 #1 `notify_hook` 深链 `/ticket/detail/`→`/tickets/`（全仓唯一 link 产生点，六枚种子模板 `{{ticket.link}}` 此前点击全 404；renderer_test fixture 同改——曾把错值固化成断言）；#2 新增 `Service.AssignAsSystem`（鉴权在规则创建期行使；修前以 rule.CreatedBy 过 L2/L3，非该单可操作人建的规则一律 403 静默失效）；#3 post_actions 链深改经 ctx 透传（修前恒传 0，maxDepth 守卫形同虚设——递归单测修前栈溢出）；#4 催办扫描「已催满/间隔未到」下推 SQL `NOT EXISTS`（修前 LIMIT 500 被催满单占满→第 501+ 永不催）；#5 导出审计 `request_id` 键修正+`WithoutCancel`+行数入 ResponseSummary+前置到 f.Write 之前；#6 NotifyAction 收件人 float64→int64（`fmt.Sprint(1e6)="1e+06"`；与 CCAction 共享 idsFromParam）；#8 inbox `renderTitle` 单次扫描防二次展开 + updated/commented 不给 actor 自己落站内信（assigned/cc_added 保留——TestB67 既定语义；全局 actor 排除/接 message_rules 属拍板 G-7 并账）；#9 `sla_due_at` 写入并入状态事务且无 meta 显式清 NULL（修前 commit 后裸 Exec 吞错）；#11 `AddTicketCCRequest.UserIDs` 补 `max=20`（swagger 同步 maxItems）；#12 intel timeout 非法值降级补 Warn；#13 规则 keyword LIKE 转义 `\ % _`+`ESCAPE`（修前 keyword=`%` 全量命中）；#14 删死类型 AvailableTransition；#16 删 DispatchService 死字段与 fmt 压 unused 黑魔法；#17 phase5/11 §5「下一编号 000061」→000079；#20 先发后记水位的 at-least-once 权衡落注。另修 **TestB67 既有 flaky**（两事件紧挨 Dispatch，goroutine 落库序不定——3 跑 1 挂，与本批无关）：第一条落库后再发第二条。**不改**：#7 登记 AGENTS+runbook §6（回滚含写端点回挂的 down 须同批回退代码）；#15 lookup 别名为显式语义非重复；#19 SystemAsCreator 委托放大点落 03 号注。验证=build/vet/22 包单测绿 + 新增 4 单测（depth 递归/ids/renderTitle/selfNoise）+ ticket·repository 集成绿 + 四档 **249/0（122+67+26+34）**（phase1 +2 来自所有者 9dbc2a0 三场景收口）+ 往返 78 对 + E2E p5 四 spec/s10 绿；dev 后端已重启。swagger 重生成（仅 +maxItems）。**验证回声（同日 13:57，所有者「验证一下是否修改完毕」）**：16 处修正逐一在 2fac388 落点核实；对运行后端实测 CC 21 人→400、导出审计 request_id 有值——但 `response_summary` 仍 null，顺藤核出**既有缺陷**：`AuditService.entryToModel` 漏拷 `ResponseSummary`（000036 加列、中间件 capWriter 一直采、repo INSERT 也写 $10，唯独映射层漏）→ **37,150 行审计 response_summary 非空 0 行**，中间件采的响应摘要自 P4-8 起全部静默丢弃；一行修补后导出行 `rows=638`、`/api/v1/runs` 等普通行开始带摘要。另：我重启后端时只复刻 INTERNAL_JOBS_SK、丢了所有者 11:56 起栈带的 TASKRUNNER_SK/BASE_URL（W5 上游已起），当即从 taskrunner 进程取回 CALLER_ZHUZHAO_SK 复刻重启，/runs 与 /al 实测 200——教训：复刻启动 env 须全量不挑键。
> 2026-10-22 批次（**外审回声批**）：S1 收口后外部审计发现 P0×4+P1×8——修复三笔：zhuzhao f32464c（dict select 展开/impact SelfService/DisableAllBuiltins/formula required/swagger 去污染）+ ui 3c622a03（richtext XSS 服务端消毒）+ ui fb3c38c9（richtext 编辑器接入发起页——组件孤岛修复）
> 2026-10-22 批次（**S1 段收口**）：B1-1→B1-5 五批全交付——spike 维持自研→FormDesigner 三层→字典挂键+守卫+UserPicker→formula（field_meta 000051+提交链重算）→DataFetcher 端口→FieldImpact+详情页回显；四档 241/0+前端五件绿；实际 1 天
> 2026-10-22 批次（**走查定稿批 + 独立复核落档 + B1-7 语义收口批立项**——三笔文档批）：①所有者场景走查拍定六项（N1 终局=禁 formula 引用 formula / 字段锁死兼容性判据版[待实施] / auto_fill ③⁺ 全链定稿[实现随 R3] / impact 归位管理组[撤 SelfService] / 详情回显差距行勘误 / A.6 测试矩阵定稿）；②全面审计 A/B 批独立复核全属实落档（P0/F-B-1 已修实证 + N1/N2/N3 登记，N 系前缀防撞）；③**B1-7 语义收口批立项**（~1–1.5 天排 B1-6 前——锁死守卫/禁链式+引用存在性/类型漂移/$env token 级/impact 归位/92304 入 allErrCodes/ui 两件）；随批：B1-6 行增量补登（**file 第 11 类型**、量级统一 ~5–6 天）、phase6 R3 前置批登记（③⁺ 四件 ~3–4 天）、01 号头注「迁移零」/「十种」两处勘误、file×附件交互五点（01 号 §3.5 建议口径）；**「S1 收口」两层语义钉死：本行上行 ✅ = 五批交付，语义收口=B1-7、能力面=B1-6 增量批**
> 2026-10-22 批次（**P1-1/P2-6 修复批 + 文档一致性收口**——全量核查报告[24 笔未推送]采纳执行，所有者「按建议来」）：①**expandDictOptions 错误臂返 `[]`**（P1-1：DB 故障原返回原文→字面量单选项→一切合法 select 值被 400 拒；三臂回归测试先红后绿——与「码不存在→空集放行」为同函数两互补臂，后者 B1-7 保存期存在性收口）②validateCustomData 三重 isEmpty 死代码合并（P2-6，行为零变化）；随批文档五件：类型漂移方案拍定=提交期重算后类型校验硬 400（替换「保存期预判」不可实施方案）/11 号 B1-7 行三路 F-B-x 前缀化+A.5 保存校验②③补挂载体+§3 迁移计数统一（S5 六枚、下一编号 000052）/05 号两处「四枚」→六枚/01 号 A.6 补充组+TestCatalogExempt 措辞勘误+92303 悬空引用删注+③⁺ mapping 目标键不重复注+prototypes Sortable 1.14.0 版本差注。验证=ticket 包全量集成 -race 绿 + lint EXIT=0 + commit-lint
> 2026-10-22 批次（**B1-7 语义收口批三批全落 ✅——S1 语义收口告毕**）：批1=$env token 级拒绝+92304 入全码表（9194f6c）；批2=锁死守卫[硬锁 type/key/删+软提示 message 承载]/禁链式+引用存在性/字典码存在性/fetch 三校验/类型漂移提交期硬 400/impact 归位管理组+**迁移 000052**（menu_apis+casbin 绑定——BK-22 对账 fail-fast 拦截后补）（f657aa8）；批3=ui detail 字典 label 映射+e2e 硬断言（ui 1a94cf46）。验证：b17 15 子测红→绿、ticket 全量集成 -race、四档 acceptance 241/0 复跑、双仓 CI 绿；「S1 收口」两层语义的第一层（五批交付）之外——语义收口层今告毕，余能力面层=B1-6
> 2026-10-21 批次（**S5 段收口** + S1 拍板落档）：**W1–3 S5 ✅ 收口**——B5-1→B5-6 六批后端全收官 + B5-2/3/4/6 前端六组件交付（zhuzhao-ui b72b854d + 238f4769 + 3bf34c8c）；**四档 acceptance 244/0 终态**（phase1 114 含 #S5-8 渲染 SSOT 七断言 + 2a 68 + 2b 27 + 2c 35）；双仓 CI 绿（zhuzhao 7 笔 / zhuzhao-ui 5 笔含 audit 漏洞清零——vite 8.3.4 + follow-redirects 1.16.1）；迁移终态 **五枚 000045–000049**（000047=owner ALTER / 000048=system_settings KV / 000049=deleted 部分索引——双审计拍板批 2026-10-21 增第五枚；11:151 与 README:167 曾同批修正为四枚口径）；**挂账**：binder 接口化未落（confirm 仍 if 硬分支——重构级不阻塞，随 S1 开工批顺带）。**S1 拍板落档（按修正后拍）**：①S1-1 确认已拍自研（10-15）+ 对标措辞校正（Jira=ProForma 收购）；②S1-2 四源白名单确认 + 限流/审计 A 类登记入 A.4（per-source fail-fast 不排队 + operator 必填）；③S1-3 按源三分治——internal_db=发起人身份（回写即探测）/ activelist=服务身份（无权限检查）/ asset-platform·external_http=服务身份+白名单回填（待 X5）——01:166 重写
> 2026-10-20 批次（phase5 规划研究回灌批——doc-only）：**phase5 计划随行件与端口预留回灌**（S5 file_objects owner 列 + 前缀占名 + confirm/GC 绑定器、S3a 动作参数化、S4 ParamsSchema 钉标 + kind/owner_service、S9 缓存层独立包、S2 壳层去耦、S6 事件分类学、README §6 +5 端口）——依据 deliverables/software-company/secops-platform-vision-2026-10/08-phase5-adjustments.md；同批补 S10 调度分工注（D18）、phase5 README §2 S4 备忘注、phase3/17 号 UAA source 列方案 a 已拍注；S5 owner 两列经**新 ALTER 迁移**（A2 先建先占——**同日实落 000047**，B5-6 顺位 000048）落、000045 不改；phase6–11 规划研究稿见同目录 README（未立项）；**同日三轮审计（六维/合并卡/三审）校正随批**：05 号变更记录同日行序归正 + 000045 迁移注释残留记账、11 号 §5.10 D18 调度分工边界成文、本文件 §4 登记 000048 预留；研究族 10 份 + overview 索引段同窗入库（deliverables 另笔）
> 2026-10-20 批次（B5-4 后端首件：迁移 000047 owner 随行件实落）：**§4 迁移地图 +000047 行、下一编号翻 000048**（A2 先建先占——B5-6 KV 原口径 000047 让位；golang-migrate 不允许事后插入更低版本）；写侧三路径同批（ticket 态 confirm/建单落绑内联 `ticket/<id>`、avatar bind `SetObjectOwner` 填 `user/<uid>` 且归属先于 users.avatar 回写）；FileObject +owner_kind/owner_id（omitempty 出站，swagger 重生成）；testutil 清单同步。验证=往返门禁 2 对零漂移 + storage/ticket/repository 三包集成（新增 owner 断言）+ **四档 acceptance 237/0（107+68+27+35）** + lint/gofmt/16 包单测绿；dev 库 migrate-up v47。详见 05/11 号变更记录行
> 2026-10-20 批次（B5-4 后端二/三：richtext 第八种 + UpdateTicket 附件增量绑定）：richtext 枚举三处扩（binding oneof / validFieldTypes / validateFieldValue）+ MD 源文本 256 KiB 上限 + 单测钉 8 种只增不改；`UpdateTicketRequest.attachments` 增量落绑（05 号 A.3 a 案登记项——`bindAttachmentsTx` 共用件，UPDATE 先行持行锁、失败整事务回滚，集成四子场景）；01 号 S1 字段类型行同步八种；**范围发现**：custom_data 建单后无更新路径（仅 INSERT）——richtext 编辑流需 custom_data patch（G2 重校验+changes 审计），登记 B5-4 拍板项非随行件。验证=lint/gofmt/16 包单测 + ticket 包集成 -race + 四档 acceptance 237/0 复跑 + swagger 重生成
> 2026-10-20 批次（B5-4 后端四：渲染 SSOT + custom_data 增量 patch——后端面收官）：**新依赖 2 项**——goldmark v1.8.6（CommonMark+GFM Table；原间接依赖升直连）+ bluemonday v1.0.27（UGC 白名单消毒，携 gorilla/css；替代 blackfriday[停维护]/自写白名单[不值]——理由随提交正文）；新端点 1——`POST /api/v1/tickets/richtext/render`（登录态自服务 + catalogExempt 登记；§3 API 面 modules/ticket.md 已同步两行）；UpdateTicket 扩 custom_data 增量 patch（键覆盖/null 删键/合并后 G2 整体重校验/仅非终态；D20 changes[] 随 S6 B6-1 事件面，ticket_events 无 payload 列不为此加迁移）；**B5-3 行翻 ✅ 后端收口**（判据"403"→"404 同形"对齐实现——评审 P2-4）。验证=lint（gofmt 回拷后完整重跑）+16 包单测 + ticket/handler/router 三包集成 + 四档 acceptance 237/0 + 新端点真机冒烟
> 2026-10-20 批次（B5-5 备份随行收口——S5 段仅余 B5-6 可切出件）：**新增 compose 服务 1**（rustfs-mirror 侧车，rclone/rclone 1.75.2 钉死、profile=storage——sync 主→备 + `--backup-dir` 天目录轮转 + **目标桶版本化幂等启用：RustFS 实测支持 PutBucketVersioning**、失败不静默）；**cron 最小件**（CronScheduler lock 查询失败 Warn 留痕[原 :82-84 静默跳过] + `CronStatus` 数据面 + panic 转失败记录——audit_archive/storage_gc 同受益，B10-4 聚合端点消费位预留）；**恢复演练五步实测**（`rclone check` 24/0 哈希对账、损→恢字节一致、删除归档保护、DB↔对象双向对账负向演示——引用集以 confirmed 为准）；**RPO 显式登记 = 同步周期**（dev 1h，对象存储无 WAL、PITR 不可得）；backup/README 增「对象存储备份与恢复」节 + runbook §3 增行
> 2026-10-21 批次（五批代码审计收口批——双线合并）：**三项 P2 修复**（F-A1 GC 一扫让位+墓碑 / F-D3 ConfigStore 原子快照+wire·app 指针分裂合一 / F-D2 KV version 乐观锁=**迁移 000050**）+ standards §3-15 周期任务归属公约（D18 成文）+ 研究稿六处校正与拍板回填；**审计报告归档** deliverables/software-company/zhuzhao-batch-audit-2026-10-21.md（双线 F 系并账台账）；P3 开放三项（F-A2/F-A3/F-C2）移交后续随手批。四档 acceptance 未跑（owner 侧执行后回填）。
> 2026-10-27 立项（L 组·文档架构整理——phase5 合回 main 后执行）：**deliverables/ 24 文件归位 docs/**（审计终稿 11 篇→review/、研究稿 10 篇+任务卡→proposal/、overview 保留修 5 处 .workbuddy 死链）+ 22 文件 36 处 deliverables/ 引用改写（11 号/standards/runbook 三 SSOT 必核）——外部报告核验后立项（4 处修正：json-contract 归 review/overview 保留不拆/24 非 23/成本重估），doc-only 单批；并行会话在途不搬。台账 L-1~L-4 同步登记。
> 2026-10-15 文档批次（phase5 拍板批）：**S5 七枚拍板全闭环**（下载形态=统一后端代理+302 升级缝/权限码复用 ticket:update/MD 编辑器/限制基线/备份直接做·必做件/限制配置化=分层 KV+按 key 编辑层级）+ 排期 Wave 序已拍（W0→S5→S1→S6一期→S2→S10→S4→S9→S6二期）+ 19 项随段拍板制 + **standards §3-1 扩写落档**（第三方协议方法辖域界定+仅 GET/POST 立法理由——全生态公约变更，首批应用例=phase5 S5）；文档人工检查中，W0 启动待另令
> 2026-10-12 文档批次（break-glass 批）：**superadmin 密码恢复应急通道落码**——`zhuzhao reset-password` 子命令（同二进制分发零镜像改动：目标校验须持启用 superadmin 角色 + 复用 HTTP 侧重置语义[bcrypt+must_change_password+全端会话吊销 `RevokeUserSessionsWithRetry` 导出] + 禁用连带启用 + audit_logs 落行 method='CLI'）；**config 职责拆分**（服务端启动 fail-fast[rate_limit/internal_jobs SK/gateway SK] 自 Load 迁 Validate——演练实证应急通道被无关启动检查卡死；Load 保持读+默认值，server 路径 Load→Validate 行为不变）；`docs/runbook.md` 建档（Phase 3 承诺欠账补建：§1 恢复流程 CLI 首选/手工 SQL 兜底含清 Redis 会话 + §2 四项演练结论沉淀 + §3 备份/PITR 要点）；§1 矩阵补行；起因=superadmin 设计评估发现「最后一个 superadmin 忘密码零应用层通道」缺口（DB 直改不清会话/不留痕）；实战=同日并行会话致 admin 凭据脱离验收态，CLI 一键复原
> 2026-10-14 文档批次（v0.4.2 发行与四仓 pin 齐平批）：**utils tag v0.4.2 打标推送**（发行内容=compat 收口[Transport/豁免收紧]/Int64Slice MarshalJSON/jwt ErrTokenExpired 哨兵/postgres MaxIdleConns 移除）+ **三仓 pin 齐平**（zhuzhao `1333f3d` 含 OwnerUserIDs 字符串化生效与 MaxIdleConns 消费端同步；activelist `ad590ab`/taskrunner `9e7343e` 纯 go.mod——影响面核查=两仓零处触及变更面，双 CI 含集成+race 绿）；同批发现并修复：standards §5 与 runbook §5 性质行「v0.4.2 待发」口径过时（发布后未同步）、swagger 对 Int64Slice 出站类型表达背离（number[] vs 实发 string[]，加 swaggertype:"array,string" 七处并重生成）、W4 测试三处值类型字段上恒真 NotNil 清理
> 2026-10-14 文档批次（二轮一致性审计收口批，四仓联动）：**全仓统一核查遗留欠账清零**——zhuzhao：runbook §5.2 判定表对齐 onFail=nil 实现（分档键改服务端日志 err 三档；响应恒统一文案）+ §5 标题残留清理 + config.yaml 补 compat_aksk 样例段 + §3 API 地图补 `/compat/echo` 行 + 外部审查残留批行回填；activelist：include_deleted 口径三处对齐（头注「已被取代」反向矛盾修正）+ 契约审计护栏测试补齐（游标字符串化集成断言/meta·repository 两包建测）；taskrunner：GET /v1/runs 补 submitted_by 文档 + 护栏测试（params null 三入口/RunView.ID 字符串化/`?id=` 筛选承重/source_ip/notify 恒出）；zhuzhao-ui：任务中心 runs 筛选补重置按钮（对齐筛选栏配方）+ 注释勘误
> 2026-10-14 文档批次（存量兼容层预接线批）：**compat 能力整体预接线 zhuzhao 主仓（默认关，用户拍板「先迁能力、有内网调用场景再验证」）**——config 段 `compat_aksk`（Enabled/Keys 密钥环，Validate 对「启用而密钥环空/含空 SK」fail-closed）+ `/compat` 专用路由组 + `POST /compat/echo` 判定靶（runbook §5.2 判定表重放靶，验签通过回显 caller/request_id）；纪律三重落码（只挂专用组/组内实质单端点/SkipBody 默认 false 待抓包判定）；**批 3 收敛为两输入**（存量 AK/SK 出带同步 + 真实请求抓包，runbook §5.1 已改「预接线状态+剩余两步」口径）；utils v0.4.2 已打标已推送 + zhuzhao pin 已升（OwnerUserIDs 字符串化生效/MaxIdleConns 口径修正消费端同步）
> 2026-10-14 文档批次（JSON 契约审计批）：**三仓 JSON 序列化契约全量审计 + B1/B2/B3 修复 + §3-14 立约**——utils Int64Slice 补 MarshalJSON（发送侧 ID 数组字符串化，v0.4.2 收编待打标）/zhuzhao role_ids·menu_ids 出站字符串化 + relation_type 回显规范化 + 回填四字段统一值类型 + Role.ParentID 对齐 null + 死信载荷成对恒出/activelist after_id·MaxID 字符串化 + Data nil 归一 + Sensitive 恒出 + fields 兜底/taskrunner RunView.ID ,string + PATCH params 显式 null 400 + NextRun·source_ip·死信载荷口径统一 + callback 死分支清理；**standards §3 增第 14 条 JSON 序列化形态公约**；遗留登记（params 双形态=ui 协同决策项/JoinedAt 三形态/Children 叶子/Create 无回填等七项）见 deliverables/json-contract-audit-2026-10-14.md
> 2026-10-14 文档批次（外部审查残留批）：**casbin 两处 error 日志补 request_id**（386e3d2 取角色失败分支 + 9e5f2aa enforce error 分支——故障注入实测按响应 request_id 反查服务端日志在 casbin 路径断链）+ W4-2b 无工号回填分支钉测（8be2ad0，外审再提契约不对称的最小动作）+ ListRuns 注释对齐与致谢页外链显示名勘误（1c62348）+ DeadLetterNotifyPayload gofmt 补跑（9a0ebe6，CI 抓漏）；均小修不入 §1 矩阵，随批留痕
> 2026-10-13 文档批次（致谢页批）：**新建 `docs/acknowledgments.md`**——借鉴/参考项目集中致谢与溯源（§1 已实际借鉴 11 项：qingtao-aksk/easy-workflow/aifei-go/RuoYi-Go 双版/go-wind-admin/ginfast/ecmdb/axiaoxin-ratelimiter/eflow/eiam；§2 仅评估未采用 6 项：Dagu GPL 红线/gin 限流生态弃养反例/gin-vue-admin/phase5 选型串）；**页首四条纪律**（借设计不借代码·无 LICENSE 禁拷贝/go.mod 依赖不登记/公司内部参考不入页/新借鉴随批登记）；许可自本地克隆 LICENSE 文件核验（ginfast 实为 MIT 文本；eflow/eiam 无 LICENSE 标⚠）；指针三处=主 README §致谢/docs README 树/standards §13 SSOT 索引；起因=aksk/compat 批引出的出处礼仪诉求（同日 compat 批翻案追记已落）
> 2026-10-13 文档批次（存量协议兼容层批）：**utils 新增 `aksk/compat` 兼容层（未提交，批 3 挂载待目标服务+存量 AK/SK）**——存量 `x-auth-*` 调用方（qingtao/aksk 衍生实现）兼容口子：出站签名+入站验签+gin 中间件，独立重实现不引原库（原库时间窗不校验/nil-Body 隐患，2026-08-14 评估结论「不直接依赖」维持）；golden 向量由存量实现现场生成并经其服务端校验器反向验证+双向交叉验证四方向全 PASS；我方端补强=±5min 时间窗强制/恒时比较/读体上限/空 SK fail-closed/caller=AK 归因；**翻案落档三处**（02-auth §存量迁移「无兼容模式」→迁移窗口内有限并存+迁移后整包删除/dd §14 行/phase1 README 行）+standards §3.11 豁免登记+utils 清单 10→11 包（v0.4.2 待发）+16 号 §9 服务鉴权行与决策记录追记｜ 代码在 zhuzhao-utils 仓（make lint exit 0，compat 20 用例全绿）；**同日能力对账**：对端四包全量枚举比对——线协议/签名/验签/中间件全量迁移，有意排除=对端密钥管理面（生成/Mongo 明文存储/签发端点[无鉴权]）/HTTP 200 包装/一站式默认中间件，形态差异=KeyGetter 函数注入未预留（config 密钥环固定，DB 动态密钥触发时补）+Sign 无构造期空密钥校验（config 层 fail-closed 承接），详见 compat 包注释「能力对账」段
> 2026-10-13 文档批次（用户展示统一批）：**「人名(工号)」结构化展示契约落地**——后端 enrichUserNames 重构 userRefsByIDs 共享 helper（查询加 employee_no，回填 name+no 独立字段非合并串）+ 工单/评论作者回填（TicketComment/Note 此前裸 ID）+ **迁移 000044 审计操作人快照列**（employee_no/real_name 写入时经 users 定格——行内自描述，归档后无 users 可 join；增量列不动 username，存量行 NULL 读端兜底）+ model 三处加 *_no 字段（工单端点为泛型 @Success 不入 swagger——本批 swagger 增量仅审计 method 筛选参数）+ swag 重生成；前端 UserNameText 统一组件（三态降级）+ 工单列表/详情（含补创建人展示）/评论作者/审计操作人四处接入（MyOrg/用户管理已是姓名+工号独立列天然合规不动）；§4 迁移地图 42→44 对（补 000043 并行批行）+下一编号 000045、§1 W4 行契约增补指针；消歧器=工号拍板（域账号覆盖率/敏感性均劣）；dd §27 站内消息中心翻案同日另落
> 2026-10-12 文档批次（000042 修复批+同步收口）：**000041/000042 迁移重写收口**（B=down 守卫换 menu_apis 来源维度[与 000041 同款]/C=up 补 menu_apis 来源行——首版只插 casbin 经不起 AssignMenus 重放，UI 保存菜单即 403 复发；down 顺序承重=先删来源行再守卫删 casbin）+000041 up 补 DISTINCT+testutil 清单补 000040-42（D2-31 欠账 ×3）+AssignMenus 重放回归测试入 `role_repo_integration_test.go`；**四档 acceptance 90×4 全绿收口**（2a 断言校准 ticket_list 页面 GET 8→9=设计内增长；operator/viewer 测试绑定清除种子态复原）；§4 迁移地图 39→42 对+下一编号 000043、§6 BK-23 补 provenance 盲区注记（roundtrip 对来源类缺陷结构性盲=000042 二次实证）+**BK-24 登记**（GET /orgs 读面超陈述，触发=进内网前安全审查随 UAA 批拍板）、B13 矩阵 orgs 行 2026-10-12 追记（L1 面扩至工单菜单角色）、phase2 README §2.4 编号表改指向本文件 §4（终结双维护）、dd §24 补 break-glass 实物化追记
> 2026-10-09 文档批次（doc-only）：phase5 规划扩至十段（S10 工单协作与提醒——抄送/标签/角标/节点超时/导出+BK-21，场景差距核查四线合并产出）+ §4 迁移谱系对账（P4-2 实落 000033 单号、下一编号 000041）+ phase6 立项登记与前置扩展（S1/S2/S6+S10）+ UAA 全移权评估与角色壳方案（**已立项 M-UAA**，公司指南下发触发达成，[phase3/17](../phase3/17-uaa-integration.md)）
> 2026-09-22 文档批次（doc-only）：Phase 4 规划批最小回填（十九批治理）——00-03 号规划定稿、十六批安全审计登记（→W0）、虚标勘误四处、迁移谱系（W1 占 000030/31）、§6 TC1 回标（详见 phase4/ 各文件）
> 2026-09-10 文档批次：四仓文档对账同步批（doc-only）。
> 2026-09-20 文档批次（doc-only）：① 工单类型级可见性策略设计输入落档（10 号新增 §10 = S1 设计+四铁律+演进阶梯 T0–T7；11-authz §5 触发表补行 #7 + 中间档位说明；12-frontend 补 §3.6 配置页规格——均随 §23 翻案批取用，不驱动当前开发）；② Phase 4 规划素材盘点起步（新建 `phase4/00-planning-inventory.md`：85 登记点归五主轴+触发信号雷达表+能力对标检查补录 §8），本文件 §7 导航树补 phase4 行。
> 2026-09-02 文档批次：Phase 3 待编写文档（03/06/07/08/09）全部补齐 + 13-implementation-plan 建档 + ops/deployment.md（B10）补齐（doc-only，未动代码/门禁）。

---

## 0. 项目一句话

Go 编写的**模块化单体 IAM + 工单系统**：三层鉴权（路由 RBAC → 资源级 ltree scope → 属主/委托）+ 资源级工单业务，单实例 Docker Compose（PG + Redis）部署，文档驱动开发（docs/ 与代码强同步）。

**技术栈**：Go + Gin + pgx + Casbin + Wire DI + Redis + PostgreSQL（ltree）+ Docker Compose。

**规模**：75 个非测试 Go 源文件（`internal/`+`cmd/`）｜ 71 个测试文件 ｜ 268 个测试函数（含集成/性能）｜ 58 个迁移文件（29 对）｜ 13 份 review 文档（含本文件；**统计时点 2026-09-14**，随代码漂移——改动后请重算或更新时点）。

---

## 1. 模块 × 能力 × 状态矩阵（核心）

| 模块 | 能力 | 状态 | 关键代码位置 |
|------|------|------|-------------|
| **认证** | 登录、双 Token（AT/RT）、RT 轮换、登出、黑名单、登录限流、会话吊销、改密 | ✅ Phase 1 | `internal/service/auth_service.go` `session_revoke.go` |
| **用户** | CRUD、启用/禁用、密码修改/重置、角色绑定、组织绑定、超管保护 | ✅ Phase 1 | `internal/service/user_service.go` |
| **角色** | CRUD、菜单分配、Casbin 策略同步、角色优先级（防提权） | ✅ Phase 1 | `internal/service/rbac_service.go` |
| **组织** | 树形 CRUD、ltree 路径、move、成员管理、owner/成员角色、虚拟组、scope | ✅ Phase 1+2 | `internal/service/org_service.go` `org_delegation.go` |
| **菜单** | CRUD、菜单树、权限码、前端数据 | ✅ Phase 1 | `internal/service/menu_service.go` |
| **审计** | 操作日志中间件、同步写、登录审计、事件表 FK 去 CASCADE | ✅ Phase 1 + 2c（000014 去 CASCADE + deleted 事件） | `internal/middleware/audit.go` `internal/service/audit_service.go` |
| **鉴权 L1** | 路由级 Casbin RBAC（BFS 角色展开 + 超管通配） | ✅ Phase 1 | `internal/middleware/casbin.go` `internal/casbin/enforcer.go` |
| **鉴权 L2/L3** | 资源级鉴权：属主/assigned/scope/虚拟组/BFS 三源/组织内委托 | ✅ Phase 2 | `internal/service/ticket/resource.go` `scope_resolver.go` |
| **工单** | CRUD、状态机（open/assigned/in_progress/pending_verify/closed/rejected）、分派、评论/备注、关联、类型/模板、可见性；**W4 随批件（2026-09-29）**：`assignee=me` 列表筛选（P1-c，工作台待办/已办数据源）+ 列表/详情姓名回填 `created_by_name`/`assignee_name`（03 S9，处理人列 N+1 消除；**2026-10-13 升结构化**：新增 `created_by_no/assignee_no` 工号消歧字段 + 评论作者同款回填，前端 UserNameText「人名(工号)」——见 03 S9/S10 契约增补）+ Update 字段级 COALESCE（十六批 Med，nil 不覆盖） | ✅ Phase 2 | `internal/service/ticket/service.go` `state_machine.go` |
| **工单模板/关联** | ticket_templates（org_path ltree）、ticket_relations | ✅ Phase 2a（迁移 000015/000016） | `migrations/000015*` `000016*` |
| **基础设施** | Wire DI、配置、优雅关闭、健康检查、迁移、限流、安全头 | ✅ Phase 1 | `internal/app/` `internal/pkg/` |
| **平台策略库（批次 A）** | 内置行级策略：`org-member`/`owner-only`/`role-gated` + `Builtin()` 一行注册 + schema fail-fast（`RequireSchema`） | 🟡 **库就绪待接线**（2026-09-04；**生产装配零消费者**，仅测试引用；首个消费者 = M-E；工单手写策略与 builtin 双路并存不合流） | `internal/pkg/resource/builtin.go` |
| **判定日志 L2（B11①，E-①）** | registry.Authorize 统一埋点 + channel→Redis List→processing→批量落库（fail-open）；request_id 全链路贯通（ctx 注入 + audit_logs/ticket_events/policy_evaluation_logs 加列，迁移 000020） | ✅ 2026-09-04（03-audit-l2 §2/§3；P3 拍板异步） | `internal/pkg/audit/policyeval.go` `internal/pkg/resource/registry.go` `internal/pkg/reqid/` |
| **内网回调端点（E-②）** | `/internal/jobs/callback`（AK/SK 验签 **utils aksk.GinMiddleware + response.AKSKFail() 统一形态**【2026-09-16 验签统一批：信封+分档中文文案，替代原零依赖默认响应】+ 专用拓扑，默认关；**action_id 在 body，C10**——早期曾拟 `/internal/jobs/<action_id>` 路径参数，未采用）；`pkg/jobs` 动作注册表；`job_submissions` 一表两用（提交凭证 + 回调幂等栅栏，迁移 000021）；P6 未知动作 404 / P7 错误映射（ErrAbort→409、其他→500） | ✅ 2026-09-04（16 号 §3）；2026-09-16 接入生态统一中间件形态 | `internal/handler/jobs_handler.go` `internal/pkg/jobs/` `internal/repository/job_submission_repo.go` |
| **审计归档（B11②，E-③）** | `audit_archive` 首个预置动作：audit_logs + policy_evaluation_logs 超期导出 JSONL→**单批导出成功后按同批 id 删行**（fsync 后删，崩溃窗口仅重复不丢）；保留期默认 180 天可配/params 可覆盖；单表失败跳过、任一失败→5xx 可重试可重入 | ✅ 2026-09-04（03 §4；本地卷 P4；注册进 jobs Registry） | `internal/service/audit_archive.go` |
| **任务管理代理（E-④）** + **契约改造（E-⑦）** | 三层校验后代理 taskrunner API（提交/状态/执行记录/任务定义 CRUD/触发/取消/重试/死信）；出站 aksk 签名 + request_id/actor/source_ip 透传；提交/触发同步落 job_submissions 凭证（E5）；权限码 task:submit/read/manage + 菜单（000022）；**E-⑦**：C10 四路由改造 + C11 runs dept 多值过滤/task 响应补 dept + 负向测试 | ✅ E-④ 2026-09-04 / E-⑦ 2026-09-07；**全链实机贯通 + taskrunner 容器化部署（2026-09-14，B+C/D 阶段：共享网服务名寻址，不可达时 502 透出）** | `internal/pkg/taskrunner/` `internal/service/taskrunner_service.go` `internal/handler/taskrunner_handler.go` |
| **网关反代（批次 B/E13）** | 前缀→上游注册表 / ReverseProxy / StripPrefix / AK/SK 出站签名 / 身份断言（X-Operator/X-Request-ID）/ 点段路径拒绝 / 502+10008 错误映射；根级挂载全链 JWT→限流→审计跳body→CasbinAuth | ✅ 2026-09-09（16 号批次 B）；**部署批与 E2E 联调 2026-09-14 闭环**（activelist 栈 compose 双网络/双副本/pgbackup+WAL，`/al` 签名透传贯通） | `internal/gateway/` `internal/router/router.go` |
| **API 限流（07 §2）** | Redis Lua 令牌桶：键取 user_id/ClientIP（登录后/匿名二选一）+ 路由精确覆盖 + 429+Retry-After + Redis 错误 fail-close 503 | ✅ 2026-09-09；**演示栈已启用（2026-09-14，20/40）**：50 并发锤 `/al` 实证 40×200+10×429+Retry-After、补桶节拍与跨路由桶隔离 | `internal/middleware/ratelimit.go` |
| **BK-22 路由↔menu_apis 对账** | 双向审计（missing_binding/dead_binding）+ 豁免集（探针/internal/公开认证/自服务/orgDelegated/网关前缀）+ wire 启动 fail-fast；发现跑抓出 audit/logs 权限面缺失 → 000025 | ✅ 2026-09-09 | `internal/router/catalog.go` |
| **通知配置（P4-2）** | notification_configs 表 + 管理 API 四端点 + /internal/notify/dead-letter 死信入口 + taskrunner 终败通知 hook（E-⑥ 形态）+ webhook 全链闭环实测 | ✅ 2026-09-29 | `internal/service/notification_service.go` `migrations/000033` |
| **字典/系统参数（P4-3）** | 字典类型+字典项 CRUD + 消费端点 GET /user/dicts/:code/items（登录可读）+ 前端管理页 | ✅ 2026-09-29 | `internal/service/dict_service.go` `migrations/000034` |
| **PAT 个人令牌（P4-6）** | personal_access_tokens 表 + 签发/吊销/列表端点 + 前端管理页 | ✅ 2026-09-29 | `internal/service/pat_service.go` `migrations/000035` |
| **P2 运维小件（P4-8）** | panic 聚合查询 GET /audit/panics + 路由对账 GET /audit/reconcile + 指标采集预留 | ✅ 2026-09-29（随 W5 前端 audit 页同批交付） | `internal/repository/panic_repo.go` + `internal/handler/ops_handler.go`（原写 panic_service.go 系路径腐烂，2026-10-15 勘误） `migrations/000036` |
| **自服务组织端点** | GET /user/orgs（富化行 org_member_role+ticket_scope+joined_at，catalogExempt） | ✅ 2026-09-29 | `internal/handler/org_handler.go` |
| **角色回显端点** | GET /users/:id/roles（role_ids 数组——前端分配角色对话框反查，替代 N+1） | ✅ 2026-10-03 | `internal/handler/user_handler.go` `migrations/000039` |
| **break-glass 应急通道（admincli）** | `zhuzhao reset-password` 子命令：superadmin 密码恢复（目标校验须持启用 superadmin 角色 + 复用 HTTP 侧重置语义[bcrypt+must_change_password+全端会话吊销] + 禁用连带启用 + audit_logs 落行 method='CLI'）；同二进制子命令分发零镜像改动；流程与兜底 SQL 见 `docs/runbook.md` §1 | ✅ 2026-10-12（集成 4 用例常态门禁 + dev 栈演练） | `cmd/server/reset_password.go` `internal/pkg/admincli/` `internal/service/session_revoke.go` |
| **前端控制台 zhuzhao-ui（P4-W2–W5+P4-9）** | **全线交付（2026-09-29 收官）**：壳层四件+登录/强制改密/个人中心+暗色主题+CI+codegen；system 四页+工单全域+al/task/audit 三域+P4-9 部署件；E2E 19 spec/21 用例打真五栈；单测 15 文件/115 用例；四轮审计 31 项全部清账 | ✅ P4-W2–W5+P4-9 全交付（2026-09-29）；审计修复链 2026-10-03 终态 | `../zhuzhao-ui` |

### 未实现 / 延后（明确不做）
- **附件**（file_objects/ticket_attachments）— 2b-ext 延后，迁移编号启动时按 A2 取下一可用号（现 **000026**；000017 已被 IW1 占用）
- **✅ Phase 3 收口（2026-09-15，口径 A）**：项目内工程收口——M-E/M-A 交付、~~M-HR(预留) 交付~~ **M-HR 未落码（2026-09-22 勘误：预留接口三件全无，对接时一并开工——见 phase4/00 §2.3 行内勘误）**（见 §1 矩阵各行）；M-UAA/M-Mig 🚦 外部前置待触发。剩余可观测性/多实例/高可用/安全增强/ops/前端 = **随部署形态或上线触发**（非工程在办项；**2026-09-27 注：前端已转 Phase 4 主动线且 W2 壳层已交付——见 §1 前端行，本句仅存 Phase 3 时点史实**）；执行结构 = **Wave W0–W4**（README §2.1.0，2026-08-31 确认，历史结构）。**文档已全量就绪（2026-09-02）**：01 / 02 / 03 / 06 / 07 / 08 / 09 / 10（含 7-0 决议）/ 11 / 12 / 13-implementation-plan（执行计划）已编写；ops/deployment.md（B10）已补齐（见 §8 B9）
- **⚠ 2026-09-02 重定位（design-decisions §23）**：**工单自研暂缓（内部引擎优先，自研兜底）**——项目将迁移公司内部并对接内部工单平台/引擎，Phase 2 工单现状封版（仅保数据安全修复，如 BK-20）；工单业务（SLA/通知/审批流/分派/报表）、BranchedStateEngine、7a–7e、B3 推进端点、审批/报表前端**全部暂缓自研**（10/12 号转对接参考）。**Phase 3 主线 = M-E 事件/任务总线（taskrunner，Asynq + 审计归档首预置动作）→ M-A activelist 独立实现 → M-HR HR 同步 → M-SSO 单点登录（🚦，§24）→ M-Mig 迁移准备**（排期见 13 §1）；M1/M5 随部署形态 🚦。工单对接形态与 Phase 2 资产处置 = 迁移时拍板（🚦）；翻案条件见 §23。**✅ 2026-10-09 §23 翻案**（dd §29）：工单自研恢复为主线；S3 拆 S3a（转换级语义+分派+动作库，排基线交付后）/S3b（审批流，等场景）；M-UAA 立项（SSO+MQ 数据面）
- **⚠ 2026-09-02 SSO（design-decisions §24）**：登录对接公司 SSO，**OAuth2.0 授权码模式**预留接口版（`SSOProvider` 接口 + `/auth/sso/login`·`/auth/sso/callback` + 身份映射对账键同 HR + 登录审计 method）；**鉴权三层零改动**（callback 签发自有 JWT/RT）；JIT 默认关（仅限已同步账号）、本地密码兜底并存。排位 **M-SSO→M-UAA**（13 §1，🚦→**触发达成已立项**，公司指南下发对接形态明确；蓝本=[phase3/17](../phase3/17-uaa-integration.md)，量级 3–5 周全含=MQ 数据面 2–4 周+SSO 件 2–3 人日，17 号 §7 引用规则）
- **⚠ 2026-09-03 activelist 职责收敛（ADR-003 修订）**：activelist 收窄为**动态数据模型平台**（类型注册/Schema 演进/动态校验/CRUD/存储），**事件与审计移交 zhuzhao**（事件 = zhuzhao Asynq 业务操作点显式发布；审计 = zhuzhao 侧记录），进程 3→1，**独立部署保留**；业界对标确认同类开源（NocoBase/Teable/Twenty 等）均连带事件/审计/UI，自研薄层合理；**共享 utils**：zhuzhao `internal/pkg` 抽独立共享项目（`crypto/errcode/jsonutil/resource/validate/response` 零依赖直抽；`jwt/logger/postgres/redis` 需 config 解耦），zhuzhao 与 activelist 共用，M-A 前置 🚦（详见 ADR-003 修订节 + 13 §9 U-B）
- **微服务拆分 / gRPC / CQRS / RS256** — 明确不做（无需求）；~~AK-SK~~ ✅ **服务间 AK/SK HMAC 签名已拍板并实现**（2026-09-03/04：utils `aksk` 包 v0.2.0 已发布（2026-09-08），内部服务通信用；外部 M2M AK/SK 管理面仍 🚦，见 09 号分层注记）

---

## 2. 权限模型速查

### 三层鉴权（从粗到细）

| 层 | 机制 | 说明 |
|----|------|------|
| **L1 路由级** | Casbin RBAC | 中间件 BFS 展开角色 → `role::superadmin` / `role::admin` 通配所有；否则匹配 `(role, path, method)` |
| **L2 资源级** | ltree scope | 工单 `org_path <@ ANY(锚点)`（包含于用户 scope 锚点集，锚点经组织表实时解析）；虚拟组/兄弟读写分离 |
| **L3 属主级** | 属主/委托 | created_by / assigned_to 直判；org owner / org_member_role=admin 组织内委托；组内防提权 |

### 权限码清单（Casbin seed 管理类）

`user:create/update/delete/status/assign_role/assign_org/reset_password` ｜ `role:create/update/delete/assign_menu` ｜ `org:create/update/delete/move/member` ｜ `menu:create/update/delete`

> 工单操作不走静态权限码，走 **L2/L3 资源级动态判定**（create/read/update/close/assign/delete/comment/note）。

---

## 3. API 地图（`/api/v1`）

| 分组 | 端点 | 说明 |
|------|------|------|
| `/metrics` `/health/*` | Prometheus 文本 + 存活/就绪探针 | 公开（探针豁免信封） |
| `/auth` | captcha（GET，P4-7）+ login / refresh | 公开 |
| 自助（登录后） | logout、auth/password/update、user/profile(+update)、user/menus、user/permissions、user/dicts/:code/items、user/pats（列表/签发/吊销）、user/orgs | 本人 |
| `/orgs`（委托） | delete、members、members/role、members/list、members/scope、members/delete、owners、roles/list、roles/bind、roles/delete | org admin/owner 操作 |
| `/users` | CRUD + status + roles(+按 id 查询) + password/reset + orgs(+按 id 查询) | 管理端 |
| `/roles` | CRUD + menus + permissions | 管理端 |
| `/orgs` | 树 CRUD + move + members | 管理端 |
| `/menus` | **只读**（GET 列表/详情——写接口已随 P4-W1 词表只读化删除，菜单行走种子迁移） | 管理端读 |
| `/dicts` `/dict-items` | 字典类型/项 CRUD（P4-3 管理面） | 管理端 |
| `/notifications` | 通知配置 CRUD（P4-2 管理面） | 管理端 |
| `/audit/logs` `/audit/panics` `/audit/reconcile` | 审计日志查询 + panic 聚合 + 路由对账（P4-8） | 管理端 |
| `/tickets` | CRUD + close + assign + comments + notes + relations | 工单 |
| `/ticket-types` `/ticket-templates` | 元数据（类型/字段/模板）：**读 + 管理**（BK-18 已补 7 写端点：2 POST + 5 PUT/DELETE 整改后均为 POST，`router.go:361-378`） | 读：无专属码；管理：`ticket:type:manage`（L1 通配 admin/superadmin，operator 经类型配置页 AssignMenus 放行） |
| `/v1/tasks` `/v1/runs` `/v1/jobs` `/v1/dead-letters` | 任务管理代理（E-④，三层校验后出站 taskrunner；tasks 组 `/cancel`·`/retry` 动词段 = taskrunner 豁免契约镜像） | task:submit/read/manage |
| `/internal/jobs/callback` `/internal/notify/dead-letter` | 内网回调 + 死信通知入口（AK/SK 验签 GinMiddleware+AKSKFail 统一形态，action 在 body；C10） | taskrunner 内网，默认关 |
| `/compat/echo` | 存量协议兼容判定靶（aksk/compat 验签 GinMiddleware，验签过回显 caller/request_id；standards §3.11 豁免登记） | 存量调用方内网，默认关 |
| `/al/*`（网关反代） | activelist 透传（JWT→限流→审计跳body→Casbin→身份断言→AK/SK 出站签名；menu_apis 000024 权限面） | 批次 B/E13 |

健康检查：`/health/live` `/health/ready`（Phase 1 起）。

---

## 4. 数据库迁移地图（79 对）

| 迁移 | 用途 | 阶段 |
|------|------|------|
| 000001 | 初始化：用户/角色/组织/菜单/审计等基础表 | P1 |
| 000002 | seed 初始数据 | P1 |
| 000003-000005 | Casbin 列调整 | P1 |
| 000006 | 唯一索引部分化（users/menus 等，软删友好） | P1 |
| 000007 | seed admin/MCP | P1 |
| 000008 | user_orgs 主键修正 | P1 |
| 000009 | Phase 1 硬化 | P1 |
| 000010 | **ticket 工单表** | 2a |
| 000011 | ticket_visibility 可见性 | 2b |
| 000012 | org_enhance 虚拟组/scope | 2b |
| 000013 | org_delegation owner/成员角色 | 2c |
| 000014 | 审计事件 FK 去 CASCADE | 2c |
| 000015 | ticket_templates | 2a |
| 000016 | ticket_relations | 2a |
| 000017 | ticket_visibility CHECK 放开 `project_isolated`（BK-13，IW1） | 2b |
| 000018 | ticket_type_admin：validate_regex + 类型配置页菜单/menu_apis（BK-18，IW3） | 2a-ext |
| 000019 | org_is_virtual：org_type 四值枚举（1/2/3 实体细分 + 4 虚拟）收敛为 `is_virtual` 布尔——行为消费点仅区分实体/虚拟，1/2/3 细分零代码消费（层级由 path/nlevel 表达）；API 契约 `org_type` → `is_virtual`（前端暂缓期无消费方） | 模型收敛 |
| 000020 | policy_eval_request_id：判定日志表 `policy_evaluation_logs`（B11①/E-①）+ `audit_logs`/`ticket_events` 补 `request_id`（03 §3.4 全链路关联） | M-E |
| 000021 | job_submissions：一表两用（E-④ 提交凭证 + E-② 回调幂等栅栏） | M-E |
| 000022 | task_admin_menus：任务管理菜单 + 权限码 task:submit/read/manage（E-④） | M-E |
| 000023 | job_submissions_params：提交入参快照列（E-④；zhuzhao 权威全量 × taskrunner 排障快照双侧记） | M-E |
| 000024 | activelist_menus：名单管理权限面（目录/菜单/按钮 5 + menu_apis 14 路由，/al 前缀网关侧路径，keyMatch2 :param；批次 B 权限面） | 批次 B |
| 000025 | audit_menu：审计日志菜单 + audit:read 码 + menu_apis 绑定（BK-22 对账发现 GET /api/v1/audit/logs 无权限面注册，补齐） | 批次 B |
| 000026 | job_submissions_claimed_at：回调幂等栅栏原子抢占时刻列（P1-1，并发审查批次；running 陈旧 10 分钟可重认领自愈） | 并发批次 |
| 000027 | ticket_config_version：类型/字段/模板三表 `version` 乐观锁列（P1-3，并发审查批次；version 可选 CAS，nil 保持旧 patch 语义） | 并发批次 |
| 000028 | ticket_relations_normalized：规范化对部分唯一索引（P1-2，并发审查批次；先软删历史双向重复行再建唯一索引，DB 兜底反向判重） | 并发批次 |
| 000029 | ticket_relations_source_index：补 source 前导部分索引（R4，第二轮审查——000028 规范化删除方向索引后，OR 谓词与 ListRelations source 臂退化顺序扫描） | 批次 8 |
| 000030 | BK-18 五端点 PUT/DELETE→POST 整改 | P4-W1 |
| 000031 | B 案词表重排+三新按钮+role_menus 补绑 | P4-W1 |
| 000032 | activelist 三端点标识入 body（整改②） | P4-W5 前置 |
| 000033 | notification_configs 表+菜单（P4-2） | P4 穿插池 |
| 000034 | dict 表+菜单+页面行（P4-3） | P4 穿插池 |
| 000035 | personal_access_tokens（P4-6 PAT） | P4 穿插池 |
| 000036 | ops panic 聚合表+菜单（P4-8） | P4 穿插池 |
| 000037 | system_menu_nest（system 目录嵌挂修正，P1-1） | P4-W3 |
| 000038 | dict_items_route（字典项路由重挂） | P4-W3 |
| 000039 | user_roles_route（GET /users/:id/roles 页面读绑定，P2-5） | P4 收尾批 |
| 000040 | al_data 菜单改名「名单数据」→「活动列表」（2026-10-08 用户拍板） | P4 收尾后调整 |
| 000041 | orgs 读放开：GET /orgs 绑「工单管理」目录（menu_apis+casbin；down 含忠实逆守卫——无其他菜单来源才删，2026-10-12 复核批补） | 权限修复批 |
| 000042 | orgs 读放开·绑定面修正：子页面（ticket_list/ticket_type_manage）补 menu_apis 来源行+casbin 存量补授（重写版：down=menu_apis 维度守卫+顺序承重；AssignMenus 重放回归测试随批） | 权限修复批 |
| 000043 | al 域命名统一：菜单「名单数据」系→「活动列表」系（顶级/活动数据/活动类型；用户拍板，2026-10-13 并行批） | 权限修复批同期 |
| 000044 | 审计操作人快照列（employee_no/real_name，写入时经 users 定格）——行内自描述可归因（归档后无 users 可 join，原仅 username=本地账号名非登录键）；增量列方案不动 username | 用户展示统一批 |
| 000045 | `file_objects` 对象元数据表（phase5 S5 B5-1，05 号 A.3 全文级）——对象本体在 RustFS 本表只存元数据；相对 10 号蓝本三处实施级修正：purpose 显式列（含建单前 \*_create 态）/presign 即写 pending+GC 部分索引/etag 快照列（拍板 1b：代理 If-Match 防重放+304 数据源一列两用）；ticket_attachments 关联表随 B5-3（000046 对账取号） | phase5 B5-1 批5 |
| 000046 | `ticket_attachments` 工单附件关联表（phase5 S5 B5-3，05 号 A.3/10 号 §3 蓝本）——tickets 级联删行（对象本体 GC 收）/file_objects 不级联/UNIQUE(ticket_id,file_object_id) 防重绑；建单前绑定（attachment_create 悬挂态）由 CreateTicket 同事务落绑本表 | phase5 B5-3 批1 |
| 000047 | `file_objects` 追加 `owner_kind VARCHAR(30)` / `owner_id TEXT` 两可空列（phase5 S5 随行件——05 号 A.3 随行件迁移段）：通用归属引用（ticket/user，后续 script_package/run/ingest/export 复用），零回填、无 FK、不预建索引；写侧同批——ticket 态 confirm/建单落绑内联填 ticket、avatar bind 填 user；**A2 先建先占**（B5-6 KV 原口径 000047 顺位 000048） | phase5 B5-4 后端首件（2026-10-20） |
| 000048 | `system_settings` KV 表（phase5 S5 B5-6 配置化）+ 设置管理页 menu_apis 种子（system_setting/system_setting_update）；存储限制为首个消费者（五键 catalog：max_upload_bytes/max_attachments_per_ticket/max_presign_batch/allowed_mime_types/orphan_gc_days——最后一键 2026-10-21 收口后评审批补） | phase5 B5-6（2026-10-20；000047 让位顺位） |
| 000049 | `file_objects` deleted 态部分索引 idx_file_objects_deleted（GC 二扫 ListDeleted 收敛扫描——双审计 F-A7；维护成本极低直接建收口） | phase5 S5 双审计拍板批（2026-10-21） |
| 000050 | `system_settings` 追加 `version BIGINT NOT NULL DEFAULT 1`（F-D2 乐观锁——并行审计挂起项收口：Update 走读版本→条件更新 version+1 的 CAS，expected_version 非空时严格校验并发冲突 400；存量行回填 1） | 五批审计 P2 收口批（2026-10-21） |
| 000051 | `ticket_type_fields` 追加 `field_meta` JSONB 列（phase5 S1 B1-3——01 号附录 A.2 方案 b：formula/扩展元数据承载列，与 field_options 分列防混载；零回填 NULL 兼容） | phase5 S1 B1-3（2026-10-22） |
| 000052 | impact 端点归位管理组绑定——`menu_apis` 补 `/ticket-types/:code/fields/impact` GET + casbin 存量回填（授权可重建 000042 同款模板——B1-7 批2） | phase5 S1 B1-7 批2（2026-10-22） |
| 000053 | `message_templates` 消息模板表（code UNIQUE/subject+body 占位符/variable_schema/version 乐观锁）+ **种子模板三枚**（created/assigned/closed 通知——仅编号+标题+链接防越 L2） | phase5 S6 B6-3（2026-10-22） |
| 000054 | `message_log` 发送记录（biz_type+biz_id 域无关键/body 渲染正文存档）+ `message_rules` 订阅规则轻表（biz_type+event_action UNIQUE/recipient_type/fixed_recipients）+ **种子规则二枚**（assigned/closed→assignee）+ 模板管理端点 menu_apis 正式绑定（撤 catalogExempt 过渡态） | phase5 S6 B6-5/B6-6（2026-10-22） |
| 000055 | 消息管理四端点（logs/rules GET·POST·delete）menu_apis + casbin 正式绑定（撤 catalogExempt 误挂——000052 同款模板） | phase5 S6 B6-6（2026-10-22） |
| 000056 | `notification_optouts` 用户通知偏好轻表（UNIQUE(user_id,biz_type,event_action)——'*'=全域静默档） | phase5 S6 B6-4（2026-10-22） |
| 000057 | `tickets.human_key` VARCHAR 列+按日唯一索引（TK-yyyymmdd-NNNN——`internal/pkg/humankey` 前缀注册表生成器，CREATE SEQUENCE 幂等+冲突重试 ≤3；phase6 RISK- 同件复用） | phase5 S6 §5.3 随批件（2026-10-23 f055c60） |
| 000058 | `message_templates.is_system` 列+种子模板标 true（B6-3 增强批——种子只读语义：Update/Delete 双拦 92503） | phase5 S6 B6-3 增强批（2026-10-23 c309ca7） |
| 000059 | 消息管理 **8 端点**（/message-templates+rules+logs 域）menu_apis+casbin 自 ticket_type_manage **重绑 system_notification**（拍板 B 终版——通知中心独立权限面；system_notification 菜单 API 总数 12=4 通知端点+8 消息端点——v3 审计 R-3 勘误：原"12 端点"系菜单面口径混入端点面） | phase5 S6 外审 F-2-4（2026-10-23） |
| 000060 | `message_rules.version` 乐观锁列（微批 A——06 号 A.5 设计原文欠账收口；upsert 携陈旧 version → 92506/409 不静默覆盖；对齐 000050/模板 CAS 全仓惯例） | phase5 S6 微批 A（2026-10-23） |
| 000061 | `message_rules.template_code` 补 FK → message_templates(code)（Y-3——删除守卫语句级原子≠事务级无窗口，FK 封悬挂引用穿透；防御性 DELETE 兜底存量悬挂行后 ADD CONSTRAINT；down 对称 DROP） | phase5 S6 v3 审计 P2 收口（2026-10-23 d0f3052） |
| 000062 | `ticket_cc` 抄送表（phase5 S10 B10-1——10 号附录 A.3 ①：role 预留 cc/participant 防 ticket_participants 双表[拍板点 2]；UNIQUE 幂等 + idx_ticket_cc_user 供 GetFilter 第四 OR 支反查）+ 种子模板 ticket_cc_notify + 种子规则 ticket.cc_added→recipient_type=cc（S6 规则面零新通知代码） | phase5 S10 B10-1（2026-10-24 111d007） |
| 000063 | `tags` 通用标签表（phase5 S10 B10-2——10 号附录 A.3 ②：entity_type+entity_id 泛型挂载，phase6 风险事件复用零改；默认共享[拍板点 6]；UNIQUE 幂等 + entity/tag 双索引） | phase5 S10 B10-2（2026-10-24 caa80fe） |
| 000064 | 节点超时+催办四件套（phase5 S10 B10-4）：`ticket_types.transition_meta` JSONB（state 级 timeout/escalation——S3a 后 from>to 键复用）+ `tickets.status_changed_at`（E-2 拍定 UpdateStatusTx 咽喉同写；存量 updated_at 近似回填）+ `tickets.escalation_supervisor_id`（E-1 拍定人工指定催办上级——dd §31）+ `ticket_reminders` 水位侧表（F-3-7）+ 活跃单部分索引 + 种子模板/规则两枚（ticket.timeout→cc / ops.cron_failed）+ cron-status menu_apis 绑 audit_log | phase5 S10 B10-4（2026-10-24 62c03c2） |
| 000065 | 工单导出端点绑定：GET /tickets/export 绑 ticket_list 页面行（S10 B10-5——查询类 GET 与列表同受众；BK-21 导出不越查询可见性） | phase5 S10 B10-5（2026-10-24 dcdd757） |
| 000066 | 动作清单只读端点绑定：GET /jobs/actions 绑 task_center 页面行（S4 B4-1——task:read 码受众=operator 提交选单口径；与 /jobs 数据面 admin 专属语义分离） | phase5 S4 B4-1（2026-10-24 80adb2c） |
| 000067 | `intel_cache` 情报缓存表（S9 B9-1——UNIQUE(intel_type,归一化 query_value)+expires_at 普通索引[P0-3 口径]）+ intel:query 码+intel_query 菜单行（本段唯一新码） | phase5 S9 B9-1（2026-10-24 1d6cf39） |
| 000068 | 统一转换端点绑定：POST /tickets/transition + GET /tickets/:id/transitions/available 绑 ticket_list（S3a B3-1——L1 与既有操作同受众） | phase5 S3a B3-1（2026-10-24 3d3f320） |
| 000069 | `assignment_rules` 自动分派规则表（S3a B3-2——03 号 A.2 DDL：type_code+priority 命中索引/keyword LIKE/决议③ 写侧校验） | phase5 S3a B3-2（2026-10-24 5070b59） |
| 000070 | 分派规则 CRUD 路由绑定：/assignment-rules 四端点绑 ticket_type_manage（S3a B3-2——零新码权限沿用） | phase5 S3a B3-2（2026-10-24 5070b59） |
| 000071 | SLA 层 0.5：tickets.sla_due_at 激活（模型 A——死字段复活存量清零）+ 过期部分索引 + ticket.deadline 种子模板/规则（S3a 随窗件——gap2 确认 W15 顺带） | phase5 S3a（2026-10-24 39ea17f） |
| 000072 | `notifications` 站内通知表族（S6 二期 B6-7——title+link 入站即渲染/行级 read_at 第一档/未读部分索引）+ message_center 菜单行（inbox:read 码）+ 三读面路由绑定 | phase5 S6 二期 B6-7（2026-10-24 8df6105） |
| 000073 | 分派规则管理页菜单行 `ticket_assignment_rules`（ticket_manage 下，权限沿用 ticket:type:manage）+ 四端点再绑本页（S3a B3-2 补遗——页面 ui 45a72256 落成却无种子成孤岛页，E2E 补齐批核出） | phase5 E2E 补齐批（2026-10-25） |
| 000074 | phase5 三页 role_menus 回补：intel_query / message_center / ticket_assignment_rules 绑 admin+superadmin（000031 惯例——000067/000072 建行漏绑致全员不可达、铃铛 /messages 404） | phase5 E2E 补齐批（2026-10-25） |
| 000075 | 分派规则三写端点迁按钮行 `ticket_assignment_rules_write_btn`（权限码沿用 ticket:type:manage，绑 admin+superadmin）+ 从 ticket_type_manage / ticket_assignment_rules 两页面行摘除 + 非管理角色三写 casbin 清除——**L1 越权修复**：000070 把写端点挂页面行违反 W1 B 案「页面=读/写挂按钮」，viewer 预设即持建/停/删规则权（POST 实测 400 过 L1 非 403） | phase5 E2E 补齐批二程（2026-10-25） |
| 000076 | 消息中心自服务化（G-23 实施）：message_center 菜单行 visible=false 惰性化（不删行防自增 ID 漂移）+ 摘除 inbox 三端点绑定 + 解绑 role_menus + 非管理角色残留 casbin 清除；同批 router 把 inbox/read/read-all 移 selfService 组（严格自数据——与 /badges 同性质）+ catalogExempt；ui 加 /messages 常量路由（铃铛对全员渲染，路由不能随菜单绑定） | phase5 E2E 补齐批三程（2026-10-25） |
| 000077 | 字典/通知配置写端点迁按钮行（G-26 对账收口）：system_dict 页面行 6 写 → `dict_write_btn`(dict:manage)、system_notification 页面行 8 写 → `notification_write_btn`(notification:manage)——两按钮行建行即空绑；含 000031 ② intent-preserving + 非管理角色残留写 casbin 清除。重扫后页面行挂 POST 仅余 ticket_list transition（000068 设计内「与既有操作同受众」） | phase5 E2E 补齐批三程（2026-10-25） |
| 000078 | 删 message_center 菜单行（G-23 收尾）：000076 惰性化留下的"幽灵项"在角色分配树可见可勾却无效；复核往返语义后补删（行在两次全量 up 终态均不存在=零漂移，000073 删叶子行先例）+ inbox:read 码随行退役；down 回种 000076 终态（visible=false 零绑定） | phase5 E2E 补齐批四程（2026-10-25） |
| 000079 | notify 动作默认路由种子：ticket_action_notify 模板 + ticket.action_notify 规则（cc 档收件人）——**翻案**「无种子默认静默」为接受偏差的 S3a 登记（2026-10-26 拍板：显式配动作而静默是坑；删规则即静默）；同批代码使动作 params.template 透传覆盖规则默认模板 | phase5 通知面收口批（2026-10-26） |

> **编号冲突已拍板（A2，2026-08-31）**：2b-ext 附件与 Phase 3 SLA 都曾规划 `000017`，规则 = **谁先启动谁占用，后者整体重排**。当前 **000017–000044 已占用**（000017–000029 见上表：IW1/IW3/并发审查批次/批次 8 回补；~~下一编号 000030~~ **Phase 4 谱系（2026-10-08 对账更新，明细见上表 30–39 行）**：000030/000031 = BK-18 整改+词表重排（W1 两连号）/000032 = activelist 整改②/000033 = 通知（P4-2 实落**单号**，非十四批预测的两连号——ticket_events 消费位未落）/000034–000036 = 字典/PAT/ops（穿插池）/000037–000039 = 菜单嵌套与路由种子（W3/收尾批）——**P4 全交付合回 main**；~~下一编号 000041~~ **000041/000042 = orgs 读放开两连（2026-10-12 权限修复批）/000043 = al 域菜单改名（2026-10-13 并行批）/000044 = 审计操作人快照（2026-10-13 展示统一批），明细见上表——~~000058 = is_system / 000059 = message_rebind / 000060 = message_rules_version，当前下一编号 000061~~ **000045 = file_objects（2026-10-19 phase5 S5 B5-1 批5）/000046 = ticket_attachments（2026-10-19 phase5 S5 B5-3 批1）/000047 = file_objects owner_kind/owner_id 两可空列（2026-10-20 phase5 S5 B5-4 后端首件——A2 先建先占），000048 = system_settings KV（B5-6 配置化，原口径 000047 让位顺位）/000049 = file_objects deleted 部分索引（2026-10-21 双审计拍板批），000050 = system_settings version 列（F-D2 乐观锁）/000051 = field_meta JSONB 列（S1 B1-3）/000052 = impact 归位管理组绑定（menu_apis+casbin，B1-7 批2 2026-10-22）/~~当前下一编号 000053~~ **000053 = message_templates 消息模板表+种子模板三枚（S6 B6-3 2026-10-22）/000054 = message_log+message_rules 表族+种子规则+模板端点 menu_apis（S6 B6-5/B6-6）/000055 = 消息管理四端点 menu_apis+casbin（S6 B6-6）/000056 = notification_optouts 用户通知偏好（S6 B6-4）/000057 = tickets.human_key 列+唯一索引（S6 §5.3 随批件 2026-10-23），000058 = message_templates.is_system 列+种子标 true（B6-3 增强批），~~当前下一编号 000059~~ **000059 = 消息管理 8 端点重绑 system_notification（菜单 API 面 12=4 通知+8 消息）（外审 F-2-4）/000060 = message_rules.version 乐观锁（微批 A）/000061 = message_rules.template_code FK（v3 审计 Y-3）/000062 = ticket_cc 抄送表+种子（S10 B10-1）/000063 = tags 标签表（S10 B10-2）/000064 = 节点超时四件套+种子（S10 B10-4）/000065 = 导出端点绑定（S10 B10-5）/000066 = 动作清单端点绑定（S4 B4-1）/000067 = intel_cache+intel:query（S9 B9-1）/000068 = 转换端点绑定（S3a B3-1）/000069-70 = assignment_rules 表+路由（S3a B3-2）/000071 = sla_due_at 激活+deadline 种子（S3a SLA0.5）/000072 = notifications 表族+消息中心路由（S6 二期 B6-7），~~当前下一编号 000073~~ **000073 = 分派规则页菜单行（孤岛页补路由）/000074 = phase5 三页 role_menus 回补/000075 = 分派规则写端点迁按钮行（L1 越权修复）/000076 = 消息中心自服务化（G-23）/000077 = 字典·通知写端点迁按钮行（G-26 对账）/000078 = 删 message_center 幽灵行+inbox:read 退役（E2E 补齐批 2026-10-25 三程/四程），当前下一编号 000080**（000079=notify 动作种子，通知面收口批 2026-10-26）**（2026-10-23 微批 A 对账——000059 行漏登一并补；同日 v3 审计 L-3 补 000061 行）；Phase 3 SLA（10-ticket-business §2 旧规划编号）启动时按此规则重排。

---

## 5. 测试与门禁体系（你的"一键验证"）

| 门禁 | 命令 | 覆盖 |
|------|------|------|
| 单元测试 | `make test-unit` | 121 个测试函数（2026-09-14 时点） |
| 集成测试 | `make test-integration` | 需 PG/Redis |
| 覆盖率 | `make test-cover` | 覆盖率报告 |
| **全量验收** | `make acceptance` | **四档链式门禁**（经 2c 脚本）：phase1（27 用例）→ 2a（R3-R8+T1-T7）→ 2b（R9-R10 虚拟组/scope/BFS）→ 2c（D1-D9 委托） |
| 分档入口 | `make acceptance-2a` / `-2b` / `-2c` | 单档运行（-2c 含全部上游回归） |
| 其他 | `make lint` `build` `swag` `benchmark` | 静态检查/构建/文档 |

**用法建议**：AI 迭代完成后，你只跑 `make acceptance` 看绿灯 + `make test-cover` 看覆盖率趋势，即可验收，无需读 diff。

---

## 6. 当前健康状态（遗留问题追踪）

> 状态基准：2026-08-29 静态验证；权威清单见 `docs/review/archive/10-phase2-comprehensive-verification.md`。

| 编号 | 问题 | 状态 |
|------|------|------|
| CC1 | Close TOCTOU（UpdateStatusTx 无条件） | ✅ **已修复**（`WHERE id AND status=$2`） |
| CC2 | Assign TOCTOU（UpdateAssignedToTx 无条件） | ✅ **已修复**（`AND status<>'closed'`） |
| CC3 | DeleteOrgDelegated 非原子 | ✅ **已修复**（`DeleteVgWithOwnerCleanup` 单事务） |
| HC3 | CreateTx 未映射 FK 违规（raw 500） | ✅ **已修复**（`MapForeignKeyViolation`） |
| MC2 | Org Move 未级联 ticket_templates.org_path | ✅ **已修复**（Move 含 templates 级联） |
| MC3 | IsAncestorOwner 未用 ticketOrgPath | ✅ **已修复** |
| P0 | RemoveMember 不清理 owner_user_ids | ✅ **已修复**（owner 三处同步清理） |
| **HC1** | Comment/Note 不写 ticket_events（无审计事件） | ✅ **已修复（2026-08-31，A4）**：CreateComment/CreateNote 事务化并同事务写 comment/note 事件；TestHC1_CommentNoteWriteEvents |
| **TC1** | delete 成功路径断言 | ✅ **已闭环（2026-08-31 A7，本行 2026-09-22 回标）**：`TestTicket_Delete_AdminSucceeds` 已入集成基线（此前「Go 层仍缺」表述与 §8 A7 已完成状态同文件矛盾——十九批治理修正） |
| **TC2** | 缺 relation 集成测试 | ✅ **已补**（`TestD9_CreateRelation`：正向 / 同向 409 / 删后建联 400） |
| HC2 | Delete 无 "deleted" 事件 | ✅ **已修复**（000014 SET NULL + Delete 同事务写 deleted 事件，随库存活；回归断言通过） |
| EC1 | Swagger 未重新生成 | ✅ **已修复**（orgs/owners 等 2c 端点已入 docs.go/swagger.json） |
| **OP1** | org_path 快照竞态（Create×Move 并发 → 旧 path 快照） | ✅ **已修复（BK-11 ①，2026-08-31）**：`OrgRepo.FindByIDForShareTx` 事务内 FOR SHARE 锁 org 行 + `ticket.Service.Create` 重构（org 读取移入事务）；回归：锁窗口阻塞验证 + Move×Create 锤击（变异验证去锁必失败）。② 已拍板（2026-08-31）保留镜像列，JOIN 备选弃用 |
| **BK-12** | `org_roles` / `roles.parent_id` 写侧管理接口缺失 | ✅ **已实施（2026-08-31）**：org_roles 绑定/解绑/列表（仅全局管理员，系统角色禁绑）+ parent_id 写侧（单调 child ≤ parent + 环检测）；BFS 源 2/3 全闭环；**收尾修复（2026-09-01）**：Bind/List 加 org 预检（不存在/软删 → 404 ErrOrgNotFound）+ Bind FK 23503 兜底 400（TestBK12_BindOrgRole_OrgGuard）；残留登记：org 删除守卫不查 org_roles（孤儿绑定行，读侧已挡无权限效果）；详见 00 §9 |
| **BK-13** | 工单可见性默认方向：兄弟虚拟组透明可读 vs 业务要求「默认只看自己 + 可配置」（`project_isolated`） | ✅ **已实施（IW1，2026-08-31）**：迁移 000017 + org update 配置 API + **L2 委托轴** + D12/委托轴测试；详见 00 §9 |
| **BK-14** | 成员 scope（`ticket_scope`）无配置面（AddMember 仅收 org_member_role，无任何 API 写 scope） | ✅ **已实施（IW1，2026-08-31）**：AddMember 扩展 + /orgs/members/scope + scope=all 仅全局管理员 + 审计；详见 00 §9 |
| **BK-15** | 代码卫生：UpdateAssignedTo 死包装 + Delete/UpdateAssignedToTx 叠注释（旧「admin only」过时） | ✅ **已修复（2026-08-31）**：删死代码 + 注释合并更正 |
| **BK-16** | ~~Delete 不校验 RowsAffected~~ | ⚪ **误报关闭（2026-08-31）**：校验自 Phase 2a 即存在（66e2c39），审计读码截断所致；详见 00 §9 |
| **BK-17** | 角色展开每请求双查（中间件 + service 各一次 BFS SQL） | ✅ **已实施（IW1，2026-08-31）**：request context 透传缓存（RolesFromContext + GetRoleCodesByUserID 命中即返回）；详见 00 §9 |
| **BK-18** | 类型/字段/模板管理闭环（只读 API，增删改仅 SQL；custom_data 无 schema 校验） | ✅ **已实施（IW3，2026-08-31）**：迁移 000018 + 7 管理端点 + G2 schema 校验 + TestBK18×2；前端照 12-frontend 施工（另排期）；详见 00 §9。随手项：~~类型/字段/模板三表无 version 乐观锁（并发编辑可互相覆盖，2026-09-01 登记）~~ → ✅ 已由 **并发-P1-3** 收口（迁移 000027，2026-09-11）；字段级加密评估不做（触发条件驱动） |
| **BK-19** | 工单 handler 层零 Go 测试（TC-1，中） | 🔶 **已登记（2026-08-31），随工单封版后置（2026-09-02 §23）**——工单现状封版，handler 测试不再作为主链前置；翻案/对接时再评估（~0.5–1 天：httptest 绑定/L1 拒绝/正常路径）；详见 00 §9 |
| **BK-20** | 禁删有未结工单的组织（守卫）+ 软删组织委托残留处置（2026-09-02 登记） | ✅ 守卫**已实施（2026-09-03）**（ErrOrgHasOpenTickets 50013→409 + 集成 TestBK20 + acceptance D6 三断言；全门禁绿）；残留部分**登记不修**——已结工单的委托可见性=档案连续性（三处委托 SQL 无 `deleted_at` 属设计内，显式断开杠杆=删除前 SetOwners 清空）；语义 SSOT = design-decisions §21；详见 00 §9 |
| **BK-21** | IW4 护栏未泛化：fail-closed 哨兵 + AST 守护仅覆盖 ticket_repo 一处，新资源接 L2 时漏接 GetFilter 仍=静默全量（2026-09-08 OPA/ReBAC 复核清点，11-authz §9.3） | 🔶 **已登记（2026-09-08），触发驱动**——随首个新资源接 L2 / 导出功能一起实施（哨兵泛化 registry 层或 AST 守护扩展至全部 repo.List 调用点）；详见 00 §9 |
| **BK-22** | 路由↔menu_apis 一致性对账：menu_apis 迁移行与 router.go 实际路由靠人工对齐，接错=混权（有路由无码=裸奔、有码无路由=死策略）；RuoYi-Go 双项目核验实锤同款痛点（Kun 版 120 条路由权限串硬编码与 sys_menu 种子人肉同步） | ✅ **已实施（2026-09-09）**：internal/router/catalog.go 双向审计（豁免集+网关前缀跨仓边界）+ wire 层启动 fail-fast（缺口拒启）+ 集成发现跑（真抓出 audit/logs 权限面缺失 → 000025 补菜单/audit:read/绑定）；Form B 时转上报校验器（design-decisions §26.2）；详见 00 §9 |
| **RT-1** | RT 轮换重用检测：提交**非当前槽位**的签名有效 RT（含已轮换旧 RT）→ `GetDel` 删除该键（**连带使当前有效 RT 失效**）→ hash 不匹配 → 401；含多标签迟到提交误伤代价 | ✅ **设计内（2026-09-11 定性；2026-09-15 错误码状态拆分）**：旧 RT 被重放本身即盗用信号，拒绝并使当前会话失效、强制重登，符合 OAuth BCP / Auth0 / Google 的 reuse detection 最佳实践；作用域**单设备（用户+deviceId 单槽位）**，不跨设备、不涉数据正确性；多标签误伤属已知代价（触发驱动评估宽限窗口/family_id）。**衍生观察（2026-09-18 登记，触发驱动不修）**：20015 命中当前不落审计事件/WARN 日志（安全信号仅存在于响应码，运维侧不可见），且多标签良性迟到提交亦命中 20015——加观测须先有降噪前提（宽限窗口/family_id），届时同框评估。**对外码 2026-09-15 起三态分离**：重放 hash 不符 → **20015**、密码纪元不匹配（C3）→ **20014**、解析失败/空槽/账号状态（防枚举）→ 20004。详见 `docs/phase1/02-auth.md` §RT 轮换流程 |
| **并发-P1-1** | 回调幂等栅栏 TOCTOU：`EnsureCallbackRow` INSERT ON CONFLICT DO NOTHING + 独立 SELECT 两段式，并发同 task_id 回调全部读到 submitted → handler 重复执行（真机 8 并发 → 副作用 8 次） | ✅ **已修复（2026-09-11）**：单语句原子抢占 `ClaimCallbackRow`（`INSERT ... ON CONFLICT (task_id) DO UPDATE SET status='running', claimed_at=NOW() WHERE status IN ('submitted','failed') OR running 陈旧` RETURNING）；迁移 000026 补 `claimed_at`（陈旧 running 10 分钟可重认领自愈）；集成测试 8 goroutine 锤击仅 1 次副作用 |
| **并发-P1-2** | 工单关联反向判重缺失：`uq_ticket_relations_pair` 方向性唯一索引 + service 层事务外预检 → 并发 A→B / B→A 双双入库（真机 59/60 轮 2 行残留） | ✅ **已修复（2026-09-11）**：迁移 000028 规范化对部分唯一索引 `uq_ticket_relations_normalized(LEAST,GREATEST,relation_type) WHERE deleted_at IS NULL`（先软删历史重复行再建索引）；23505 → `ErrConflict` 409；集成测试 60 轮并发零残留 |
| **并发-P1-3** | 类型/字段/模板并发编辑 lost update：三表无 version，后写静默覆盖先写（BK-18 随手项 2026-09-01 登记的正式收口） | ✅ **已修复（2026-09-11）**：迁移 000027 三表补 `version`；`UpdateTicketType` / `ReplaceTypeFields`（以父类型 version 为权威谓词）/ `UpdateTicketTemplate` 支持**可选 version CAS**（nil = 旧 patch 语义；携 version 命中才写，0 行 → 409 `ErrConcurrentModification`，ErrNoRows 区分 404）；集成测试 lost-update 场景验证 409 |
| **并发-P2-1** | 组织软删 × 成员插入竞态窗口：软删 UPDATE 取 `FOR NO KEY UPDATE` 与 INSERT FK 检查 `FOR KEY SHARE` 不冲突，单侧加锁无法闭合（实测证伪「仅守卫 FOR UPDATE」「仅 AddMember 侧」两方案）；软删不使 FK 目标行失效 → 迟到 INSERT 仍成功 | ✅ **已修复（2026-09-11）**：**两侧锁协议**（无迁移）：Delete/DeleteVgWithOwnerCleanup 守卫前对 org 行取 `FOR UPDATE` 事务性认领（随提交/回滚释放，勿加 deleting 列/勿复用 status）+ AddMember 侧 4 处 `INSERT INTO user_orgs` 同事务取 `FOR SHARE` 复核 `deleted_at IS NULL`（`org_repo.go` AddMember/AddMemberWithRole/SetUserOrgsTx/SetOwnersTx；**严禁改用 FOR KEY SHARE**——与软删 NO KEY UPDATE 不冲突）；集成测试 25×2 轮并发删除×加成员零残留 |
| **BK-23** | 迁移 up/down 验证方法盲区：现行 down 验证只断言「down 过程不报错」，**无 up→down→再 up 的 A/B 数据快照比对**（批 29 全链 down 验证 31 对→0 仍抓不到 D-1 类问题=实证，code-audit-2026-09-26）；同系触发驱动盲区=blind-spots-2026-09-22 **B-1**（回调幂等栅栏租约无心跳/无执行时长上限——长任务跨 10 分钟窗口可被并发重认领双执行，`audit_archive` 首跑积压必踩）/**B-2**（49 个 CREATE INDEX 非 CONCURRENTLY，在线迁移锁表维度零登记）。**provenance 盲区补记（2026-10-12 000042 B/C 二次实证）**：快照 roundtrip 对「授权来源类」缺陷**结构性盲**——down 误删他源规则后再 up 会原样重建，快照零漂移判 PASS（000041 down 误删/000042 首版两缺陷均实测过环）；辅助手段=down 守卫语义对拍（menu_apis 来源维度）+ AssignMenus 重放测试（已随 000042 重写批落 `role_repo_integration_test.go` 为常态门禁） | 🔶 **已登记（2026-10-10，文档审计循环收口批）**，触发驱动：BK-23=下次迁移窗口或迁移规范修订时补「快照比对」验证法并接入门禁；B-1=长回调任务/归档积压场景出现时补租约续期+时长上限；B-2=大表索引/在线迁移诉求出现时按 blind-spots §B-2 建议实施 |
| **BK-24** | GET /orgs 读面超陈述（0373011 复核批挂账②）：非 admin 开放组织树读后，响应携带 `owner_user_ids`/成员计数等管理面字段——信息暴露面大于「发起工单选组织」所需。收窄方式待设计拍板：非 admin 精简 select 分支 或 响应脱敏（字段白名单） | 🔶 **已登记（2026-10-12）**，触发驱动：进内网前安全审查时收口（与 M-UAA 批同窗，随数据最小化一起拍板）；L3 行级面不变（B13 盲区 #3 定性维持） |
| **BK-25** | utils 下版窗口（v0.4.3）小批（2026-10-15 全仓审计挂账）：README 依赖边界声明滞后（response 实依赖 aksk，:25 仅称 errcode）/错误消息包名前缀统一（jwt·redis·postgres·jsonutil 无前缀，aksk·compat 为范本）/aksk·compat 两 gin fail() 码值字面量补 why 注释/crypto·errcode·logger 三包零测试（CheckDummyPassword 时延语义无回归保护）/jwt AT 过期哨兵用例/response 补限流 429 信封助手（zhuzhao ratelimit.go 手拼 gin.H 随批收编） | ✅ **已实施（2026-10-15 清欠批，utils `1c27ca2` + zhuzhao 同批）**——main 直提不动 tag（消费方 pin v0.4.2 零影响），随 v0.4.3 发版收编；429 助手一项豁绝：现有 `response.Fail(c,status,code,msg)` 已支持任意状态码+request_id，无需新增助手，ratelimit.go 直接收编 Fail+Abort |
| **并发-P3-1** | Create 事务窗口嵌套池取死锁：`fileFieldKeys`/`keyGen.Next` 在 `Begin` 后再向连接池要第二个连接，锤击（3 Create+1 Move）在 4 连接池（pgxpool 默认 `max(4,GOMAXPROCS)`——CI 4 核即 4、本地多核 ≥8 永不复现）构成池耗尽循环等待——2026-10-24 起 CI 15 run 挂 7 笔（`TestBK11_CreateVsMoveHammer` 20m 挂死签名）；**生产 4 核部署 ≥4 并发 Create 同样可永久挂死**（B1-6 批1 `50f40a5`/S6 补遗 `f055c60` 两批引入嵌套点） | ✅ **已修复（Create=2026-10-26 `039aca6`；Update 同类残留=2026-10-27 随批）**：Create 两池调用提事务外 + testutil 池钉 4 + 锤击 30s 兜底（先红后绿）；**Update 残留**（`validateCustomData`/`applyFormulas` 在锁内窗口走池读——6 并发×MaxConns=4 自闭环死锁，外审核出+锤击实证 round 0 即挂）同批修：repo 增 `RowQuerier`+`ListTicketTypeFieldsTx`（pgx.Tx/池双满足），两 helper 接取数通道参数（Create 传池/Update 传 tx），`TestBK11_UpdateHammer` 先红（ctx deadline）后绿（0.12s）；复盘 SSOT = [13 号](./13-bk11-pool-deadlock-2026-10-26.md)。**待拍板 3 项**：dbtx 连接解析跟随 ctx（彻底方案，repo 签名零改动，建议至少 ticket 域试点）/ 请求级 deadline 中间件（互补件——pgx 无默认获取超时，同类死锁线上形态=挂到客户端断开）/ humankey 计数行表化+schema 读缓存（挂账随批） |

---

## 7. 文档导航（docs/ 体系）

```
docs/
├── design/        # 为什么这样设计（决策与权衡）
├── proposal/      # 具体方案是什么
├── modules/       # 模块完整设计（跨阶段）
├── phase1/2/3/4/5/6/  # 每阶段实施计划（phase3 已收口；**phase4 = ✅ 已收口（2026-09/10）**：批次总览七件[W0–W5 六波+P4-9]+穿插池五件+PAT 全交付，phase4 分支已合回 main 删除；**phase5 = 排期已拍（2026-10-15 所有者按建议 Wave 序：W0 启动批 → S5 → S1 → S6 一期 → S2 → S10 → S4 → S9 → **S3a 语义引擎[W13–15]** → S6 二期，一期收口 ≈12–13 周/含二期+引擎 ≈16–17 周；十段设计稿已建 01–10 号+实施计划 [11 号](../phase5/11-implementation-plan.md) ✅ **已批准转执行态（2026-10-16）+ W0 ✅ 已收口（2026-10-19 所有者确认，W1 开工 B5-1）**，`phase5` 分支）**：前端可视化配置与通用能力底座线十段——表单设计器与字段自动化/流程状态图设计器/流程语义配置化（已解冻拆 S3a/S3b——S3a 2026-10-19 升格排期 W13–15 纳入收官）/管理面人性化（taskrunner 任务面）/对象存储与富媒体（RustFS——2026-10-16 拍定原 MinIO：头像上传/工单附件/富文本字段，IW2 附件件收编）/消息通知（统一消息平台+模板+工单触发，P4-2 通道延伸；通知配置页自 S4 迁入）/AI 助手（形态未定 fastclaw 参考·概要登记，长线=AI 协助业务）/数据大屏（🚦 触发驱动后置）/威胁情报缓存代理（对已有情报应用补 TTL 7/30 天缓存能力，消费方=phase6 R1 情报源）/工单协作与提醒（抄送/标签/角标/节点超时/导出+BK-21——2026-10-09 场景差距分析产出）；**phase6 = 规划草案未排期**：风险管理模块（首个业务模块——风险单汇聚去重/评估分级/四大处置策略/整改工单联动/自动关闭；**硬前置=phase5 S1/S2/S6+S10**；BK-21 随批），2026-10-08 立项登记，SSOT=phase6/README；**Week 0 启动批已拍（2026-10-26）**：前置件并入立项批不单开过渡批（R3 前置四件+HR mock 指派轴+BK-21 泛化+mock IdP，约 7–10 天，不进核心切片计时，出口标准=R1 开工时接入/指派/L2 三块地基全绿；登录/登出审计 0.5 天不锁定可随批先行），见 phase6/README §7）
├── roadmap.md     # 各阶段总览（Phase 4/5 段已同步）
├── adr/           # 架构决策（001 L1 事件 / 002 Asynq / 003 activelist）
└── review/        # 验证报告（本文件 = 能力总览，01-10 = 历史 review）
```

**掌握流程**：遇到能力问题 → 查本文件矩阵定位模块 → 按需深入 `modules/` 对应文档 → 变更后回填本文件。

---

### 7.x B13 权限覆盖矩阵（2026-09-26 完成）

| 文档 | 状态 |
|------|------|
| [12-b13-permission-matrix-2026-09-26.md](./12-b13-permission-matrix-2026-09-26.md) | ✅ point-in-time 快照：79 端点×三层全对账；8 项盲区/不对称定性（审计/用户/组织读=管理面全局视图设计内；orgs update 不对称注记；users/orgs 建议 W3 后补 org 侧校验）；BK-21 泛化未到触发点；IAM 四平面边界清单（§26.1 输入交付） |

## 8. 遗留问题分类：Phase 3 前置 vs 随行（2026-08-31 整理；2026-09-01 增补 B11/IW4）

> 本节取代原「下一步」清单，把全部已知未决项归入两档：**A 档 = Phase 3 启动前/启动时完成**（门禁与拍板，不做会让启动本身踩坑）；**B 档 = 随 Phase 3 对应子能力一起**（提前做无收益）。代码级 backlog 详情见 [phase2/00 §9](../phase2/00-implementation-plan.md)。**Phase 3 启动时从 [phase3/00-startup-checklist.md](../phase3/00-startup-checklist.md) 进入检查流程**（本节 + §6 是其数据源）。
>
> **Phase 3 执行结构（2026-08-31 确认）= Wave W0–W4**（详见 [phase3/README §2.1.0](../phase3/README.md)）：**W0** 启动门禁（本节 A 档 + 检查单 IW1/IW3）→ **W1** 可运维基座（Step 1/2/3；[02-multi-instance](../phase3/02-multi-instance.md) 已编写，含 Casbin Watcher 移植方案）→ **W2** 工单业务（Step 7；[10-ticket-business](../phase3/10-ticket-business.md) 已含 7-0 决议，前端规格见 [12-frontend](../phase3/12-frontend.md)；本节 B 档为其随行项）→ **W3** 加固收尾 → **W4** activelist 集成。
> **⚠ 2026-09-02 重定位（design-decisions §23，推翻 §22.5）**：工单自研暂缓（内部引擎优先），Wave W2 工单业务闭环暂缓自研（10/12 号转对接参考）；现行主链 = **M-E 事件/任务总线 → M-A activelist 独立实现 → M-HR HR 同步 → M-Mig 迁移准备**（W1 随部署形态 🚦；排期见 [13-implementation-plan](../phase3/13-implementation-plan.md) §1）。B 档中工单随行项（B1/B3/B4 等）随 §23 后置/暂缓，审计归档（B11②）改随 **M-E** 首个预置任务。

### A 档：Phase 3 启动前/启动时完成

| # | 事项 | 说明 | 量级 |
|---|------|------|------|
| A1 | **文档修正包** | ① phase3 README §1.4 前置矛盾（2b-ext 延后项列为「2b 验收」前置）、§5 状态行（10/11 已编写仍标待编写）；② **review/10 C1–C4 处置**（2026-08-31 逐条核验）：C1「Step 9 应为 8-10」三次裁定驳回维持（wontfix，注记防再犯）、C2 `NewResource` 三参签名已修（注记闭环）、**C4 §2.2 ScopeResolver 接口段与现行实现不符（真开放）**——按 `ReadAnchorPaths`/`ResolveScope` 重写、C3 `docs/ops/deployment.md` 缺 → 归 B10；③ 09 合集抽查回注：F-01/02（VISION 措辞/迁移现状）、F-03（activelist 链）、F-18（Redis requirepass 经 compose 注入）经核实**均已修**（09 行未回标属历史文档常态，此处记录防重复排查） | 半天 | → ✅ **已完成（2026-08-31）**：README §1.4/§5 已修正、review/10 C1–C4 处置注记全落
| A2 | **迁移编号 000017 归属拍板** | 2b-ext 附件（phase2/00 §Step 6）与 Phase 3 SLA（10-ticket-business §2，占用 000017–000021）都规划 000017。规则建议：**谁先启动谁占用，后者启动时整体重排**——✅ **已拍板（2026-08-31）**：谁先启动谁占用，后者整体重排 | 已关闭 | 决策 |
| A3 | **BK-11 ② 数据结构拍板** | `tickets.org_path` 保留镜像列（FOR SHARE 已兜底）vs 去列改运行时 JOIN + write-once `created_org_id`；已登记 [phase3/README §4](../phase3/README.md)，✅ **已拍板（2026-08-31）**：保留镜像列；`created_org_id` 留 Step 7e 按需 | 已关闭 | 决策 |
| A4 | **HC1：comment/note 补 ticket_events** | 事件流是 Step 7 SLA/通知/报表的统一输入，补全属地基（唯一遗留的 §6 未修项）；service 层两处 + 事件常量 + 测试 | ~半天 | → ✅ **已完成（2026-08-31）**：comment/note 事件已补（TestHC1_CommentNoteWriteEvents）
| A5 | **BK-5：relation 反向判重** | DB 唯一索引只挡同向，A→B 与 B→A 可共存；报表/SLA 引用关联数据前收口数据质量（应用层 EXISTS 检查） | ~1–2h | → ✅ **已完成（2026-08-31）**：ExistsRelationBetween 双向判重 + TestBK5_RelationReverseDedup
| A6 | **散落决策落档与断链收口**（2026-08-31 全量扫描 phase1/2/3 新发现） | ① **SoD 延后决策未落档**：11-authz-architecture-review §4 已给结论（「延后 + 届时优先动态 SoD」）但从未写入 design-decisions；且其建议编号 P2-D7 已被 00 计划的三轨拆分复用——落档时用新编号并注明别名；② **phase2/12·13 号断链**：14 号文档 5 处引用 `12-phase1-backlog-and-phase2-review.md`、`13-project-plan-multi-round-verification.md`、`13-plan-remediation-actions.md`，文件从未入库（git 历史无删除记录）——修正引用为实际归宿（review/09 等）或加「已并入」注记；③ review §7 item6「DB 错误注入 → 拒绝」测试用例未落（顺手项） | ~1h | → ✅ **已完成（2026-08-31）**：SoD 落 design-decisions §20、14 号断链注记、错误注入用例落 02-authz §4
| A7 | **TC1-Go：补全局 admin 删单成功集成测试** | 现有 Go 覆盖为委托删（vg owner/vg admin/ancestor owner）；全局 admin bypass 删单成功仅 2c 脚本覆盖（SAT 建删删）——补 Go 层测试进 `make test-integration` 基线（CI 可跑）。AGENTS.md 遗留节已同步校准（TC2/HC2 标已修） | ~15min | → ✅ **已完成（2026-08-31）**：TestTicket_Delete_AdminSucceeds 已入集成基线

> 其余待决策点（K8s vs Compose、Redis/PG HA、部署级分离时机、审批流引擎选型）已在 [phase3/README §4](../phase3/README.md) 维护，启动时逐项过表，此处不重复。

### B 档：随 Phase 3 对应子能力一起

| # | 事项 | 随哪个子能力 |
|---|------|-------------|
| B1 | **Step 7 设计期拍板清单**：① SLA 暂停态语义、通知「主管」定义、邮件通知矩阵（原评估 B1/B3/B6）；② **10 号文档设计深度缺口（2026-08-31 外评证实）**：§2.5「标记+Enqueue 同事务 vs 只 Enqueue」二选一（SLA 正确性核心）、§7.2 signal 双写二选一、responded_at 是否含内部备注、`min_level` 职级数据源（users 无 level 列，悬空）、§5 分派深度（keyword 算法/同优先级/target 从属/无命中兜底/Hook 事务边界）、§6 报表深度（权限码/缓存失效/指标口径/分页）、§8 TB 负向用例（§2.5 四必坑 0 覆盖）；③ **2026-08-31 生态调研吸收**：发起人撤回（Revoke）业务设计缺失（eflow WITHDRAWING 栅栏模式可对标）、`workflow_definitions` 版本/发布快照（编辑版/发布版分离）、审批人策略模型 `Assignee{rule,values}`（部门领导/分管规则替代职级，解 min_level 悬空） | Step 7 设计期逐项拍板 |
| B2 | 权限码 seed：ticket:approve / notification:* / workflow:manage 均未设计 | Step 7（与 7c/7d 同步设计 + seed 迁移） |
| B3 | in_progress / pending_verify 状态推进端点（BK-10 已拍板归 Phase 3） | Step 7 工单业务深化 |
| B4 | BranchedStateEngine 引擎本体 | Step 7c（~~**硬交付**~~ **随 §23 暂缓**：工单自研暂缓（2026-09-02），引擎本体不再实施；翻案恢复见 design-decisions §23）。设计期消费 A6 的 SoD 延后决策（审批流互斥优先**动态 SoD**） |
| B5 | BK-11 ② 实施（去列 JOIN 或保留快照的落地） | 随 A3 拍板结果，在 Step 7 动工前实施 |
| B6 | ~~BK-12：org_roles / parent_id 写侧~~ | ✅ 已实施（2026-08-31，IW3 附带）：绑定/解绑/列表三端点 + parent_id 单调规则 + TestBK12；HR 同步启动时无需再补写侧 |
| B7 | CORS AllowAll 转轨收紧（09 合集 F-21） | Step 5 security-enhance + 上线检查单 |
| B9 | 文档补齐状态：**已编写（2026-08-31）**：01 / 02-multi-instance / 10（含 7-0 决议）/ 11 / 12-frontend；**已编写（2026-09-02，原 5 份待编写全部补齐）**：03-audit-l2（含 B11①②）/ 06-ha / 07-security-enhance / 08-ops + **ops/deployment.md**（B10）/ 09-platform；另 13-implementation-plan（执行计划，里程碑/人日估算/🚦 触发项/⚠️ 不确定项）同日建档 | 全部文档就绪，启动时按 Wave 取用；**M2 硬依赖 = Asynq 底座（2026-09-02 §22.1 修订，M1 降 🚦）**；**参考实现**：02 的 Casbin Watcher 直接移植 eiam `ioc/casbin.go`（redis-watcher + StartAutoLoadPolicy 双保险）、Asynq 任务建模仿 etask（RetryConfig 指数退避/补偿器/job 化 Wire 注册） |
| B10 | `docs/ops/deployment.md` 补编写（ops 骨架 README 已在，review/10 C3） | 随 Step 6 ops / 部署文档批 |
| B11 | **审计治理两件（2026-09-01 go-wind-admin 调研吸收）**：① **L2/L3 策略评估日志**——判定日志表 + `resource.Authorize`/`scope_resolver.resolve` 埋点（actor/资源/动作/scope 轴/结果/原因/trace_id），补 L2 拒绝无留痕盲区；② **审计归档**——audit_logs + 判定日志表超期导出 JSONL、导出成功后删行（保留期默认 180 天等保口径、可配置） | ①✅ **已实施（2026-09-04，E-①）**：迁移 000020 + EvalHook 埋点 + L2 writer（管道拍板 2026-09-03 异步）+ request_id 三列贯通；②✅ **已实施（2026-09-04，E-②/E-③）+ 已按周期跑通（2026-09-14 E 阶段收官：cron 任务定义 + AL5 真数据闭环——超期行导出 JSONL 并删行；故障演练四项全过）**；**B11 全闭环** |
| B12 | **BK-21 IW4 护栏泛化**：fail-closed 哨兵 + AST 守护从 ticket_repo 泛化到 registry 层通用机制（或 AST 扩展覆盖全部 repo.List 调用点）；导出功能为高危场景（绕过分页全量拉取）必须接 L2（2026-09-08 OPA/ReBAC 复核清点，11-authz §9.3） | 随首个新资源接 L2 / 导出功能（新资源经 Builtin/手写 Resource 接入时一并落） |
| B13 | **权限覆盖矩阵审计（提议待拍板，2026-09-08）**：全端点 × 三层 × 「设计内豁免 vs 遗漏」逐行对账，产出覆盖矩阵 review 文档；顺带产出 ① IAM 平面边界清单（design-decisions §26.1 三步走①的输入）② 全文档 `internal/*` 路径引用对现状核对（F-1 路径腐烂教训推广） | 半天–1 天，doc-only；拍板后排期 |
| B14 | **BK-22 路由↔menu_apis 一致性对账**（启动期/架构测试双向 fail-fast；Form B 能力目录时转上报校验器，design-decisions §26.2） | 随批次 B（activelist API 入 menu_apis 同批落地），~半天 |

### 独立窗口（已触发，Phase 2 范畴，不属于 Phase 3 前置或随行）

| # | 事项 | 说明 |
|---|------|------|
| IW1 | ~~多虚拟组可见性场景闭环：BK-13 + BK-14~~ | ✅ **已实施（2026-08-31）**：000017 CHECK + org update 配置 API + L2 委托轴 + scope 配置面全链 + D12/委托轴测试（原编号 W1，随 2026-08-31 编号治理改 IW；详见 00 §2.3） |
| IW2 | **2b-ext 剩余两件**（附件 / auth-enhance；**HR 同步已升 Phase 3 主链 M-HR**（原 M2.5），2026-09-02 §22.2/§23.2） | 附件 / auth-enhance 按需独立启动（附件先于 Phase 3 启动 → 按迁移号规则核对 A2）；HR 同步随 M-HR（预留接口版 HRFetcher + 引擎 + mock adapter；⚠ 2026-09-22 勘误：**三件均未落码**，全仓无 HRFetcher——对接时一并开工），**两个待设计项随启动拍板**（hr-directory-sync.md §3.2/§4.3 的 2026-09-01 场景登记：离职在途工单处置策略、部门撤销 × tickets.org_path 级联完整性）。**2026-10-08：附件/存储件收编为 Phase 5 S5 实施载体**（SSOT=[phase5/README](../phase5/README.md) §P5-S5，迁移号启动时按 §4 对账占用——原预留 000034 已被 P4-3 字典占用，000058 = is_system / 000059 = message_rebind / 000060 = message_rules_version / 000061 = message_rules FK（v3 审计 Y-3），当前下一编号 000080（000079 = action_notify_seed，2026-10-26））；auth-enhance 维持本窗口触发驱动 |
| IW3 | ~~BK-18：类型/字段/模板管理闭环~~ | ✅ **后端已实施（2026-08-31）**：迁移 000018 + 7 管理端点 + G2 校验 + TestBK18×2；前端照 12-frontend 施工（另排期） |
| IW4 | ~~行级过滤护栏（fail-closed）~~（2026-09-01 go-wind-admin 调研吸收） | ✅ **已实施（2026-09-01）**：`resource.Filter.Unscoped` 显式豁免（admin bypass / ticket_scope=all 两处显式化）+ `ticket_repo.List` 入口 fail-closed 哨兵（无谓词且未豁免 → 报错，漏接 L2 从静默全量变测试期报错）+ `TestGuard_TicketRepoListCallSites` AST 守护（repo.List 调用点锁定 ticket 包）+ 测试 4 个；全门禁绿（lint / 13 包单测+集成 `-race` / acceptance 27+66+26+32 FAIL=0） |

> **随手项（任意时点）**：BK-9（测试死代码清理）、A6 ③（错误注入测试用例）、09 F-31④（relation 越权负向用例——现有 `TestD9_CreateRelation` 只覆盖正向/同向 409/删后 404）、09 F-32（audit/user service 分支级单测，低优——集成已兜底核心分支）、可选 Q5 组织赋角注记（11-authz §5 Q5 不变量补一句「组织赋角（org_roles/parent_id）使 token 快照原理上不可行」，doc-only，2026-09-01 登记）。**新增（2026-09-08）：在线用户管理面（触发驱动；触发条件 = 出现强制下线/会话审计运维诉求——安全事件响应或账号共享治理，或 M-UAA 上线引入多端会话管理；两条件均未出现前不动，2026-09-09 补写触发条件对齐「暂缓 ≠ 搁置」纪律）**——会话吊销原语全现成（`user:disabled` + SCAN `refresh:<uid>:*` + `blacklist:at:<jti>`，session_revoke.go / auth_service.go:326），缺的仅管理端点：基础版 GET/DELETE `/monitor/online-users` + 权限码 seed，0.5–1 天；按设备精确强退需 RT value 改 JSON 存 AT jti（+1 天）。**新增（2026-09-11 审查修正批，文档体系低优治理）**：① **ADR 覆盖不足**——现仅 3 篇（事件机制/Asynq/activelist 集成），建议补 5–8 篇重大不可逆决策（三层鉴权、ltree 选型、双 Token + 黑名单、迁移编号治理、ticket_visibility、虚拟组）；② **体量偏重**——`docs/` 约 2.6 万行 > 代码 2.3 万行，`design/architecture.md`（1,899 行）/ `design-decisions.md`（1,414 行）建议按职责拆分（对齐 standards §4「单文件 >200 行评估拆分」精神）；③ **RT 重用检测的多标签误伤**（触发驱动；触发条件 = 出现多标签体验问题的真实反馈）——届时评估**宽限窗口** / **family_id** 方案；未触发前不修（注意「先 GET 比对、命中才 DEL」会削弱盗用检测，须权衡）。详见 `docs/phase1/02-auth.md` §RT 轮换流程 / 本文件 §6 RT-1。
