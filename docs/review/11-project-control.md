# 11 · 项目能力总览图（Project Control）

> **2026-11-08文档二次复核**：按原始要求重核迁移消费者、内容一致性、现行状态、示例前置与阅读体验，修正初轮遗漏并补说明；本轮验证进度与覆盖边界见[二次复核记录](reports/documentation-recheck-2026-11-08.md)。无业务代码、配置、依赖或迁移变化。
>
> **首次归位快照**：原225份文档逐项审读/处置，当轮274份Markdown、2532处本地引用检查零问题，11张图解析渲染通过；初红、默认skip及当时未推送边界保留在[首次整理记录](reports/documentation-reorganization-2026-11-08.md)。随后按所有者指令推送17笔，五仓精确HEAD CI通过；本轮二次复核另判，未合main或更新共享运行服务。维护约定见[documentation.md](../documentation.md)。

> **用途**：从能力矩阵、迁移地图和健康状态掌握当前项目；改动后同步本页。原逐日流水见[历史记录](archive/project-control-history-2026-11-08.md)，不再与当前摘要混排。

## 当前摘要（2026-11-11）

- **工作边界**。Phase5已于2026-11-11验收收尾并整合五仓main；代码基线主仓5c0e248d/UI73a18d39/TR340c7a6/AL93ecbcb/utils b027089。本轮重规划以原6–9功能为基线覆盖全部后续路线与新增能力，Phase6只是近期切片；首轮及本批详设已在独立codex分支落档，功能尚未实施。当前审计步骤3/详设候选与4/增量复核；旧待答字样已集中收敛到决策登记，非全部设计完成；[收口状态](planning-closeout.md)区分五步与逐包成熟度。资源清理终态见 [12 号 §8.4](../phase5/12-scenarios-and-tests.md#84-环境数据与工作区收口2026-11-11-已完成)。
- **测试与证据**。11-11 固定快照四档 253/0/0、81 对迁移往返及 UI 244 单测/42 E2E 通过，范围和旧代码对照见 [12 号 §7](../phase5/12-scenarios-and-tests.md#7-结果汇总2026-11-11-收尾复核)。主仓 114762d1 与 UI 3833ca2c 准确提交 CI 已成功，关闭状态文档的最终提交另留收据。清理后开发主应用、UI 与两上游健康检查通过。
- **问答确认与细化**：ENG-D02默认追加上传/保留旧文件、用户明确删除，EXE-D01按本地业务不变量同事务/独立后继异步均已确认。C03/I03按任务错失策略已细化[恢复合同](../design/schedule-recovery-policy.md)，数值仍候选，超限hold已获EXE-D03确认；C03基础6–10日/I03恢复与跨仓观测9–15日，粗估以总账为准；见[决策册](planning-decisions.json)。
- **首页方向已确认（2026-11-12）**：UX01默认单侧栏+身份顶栏、页签/页脚不默认占首屏；UX02按当前处理/明确复核责任统计，与列表/角标同源去重，发起/CC不自动计，任务/消息按权限保留。原R3未保证改assigned_to，不为统计自动改派；U01先入口、U02普通责任、D10同批复核。两项方向均已确认但未实现，UX03仍待决；见[领域合同](../design/business-lifecycle-contracts.md)。
- **交接决策准备（2026-11-12）**：B06按批准继任映射自动交接、B07按已确认接管plan迁挂的默认值仍待owner；无映射不猜上级/祖先，历史不重写。只修规划候选，未执行HR同步或权限迁移。
- **全路线与渐进承接**：O10确认以现有规模尽可能承接、优化扩展原6–9及更远能力；[169条连续性审计](roadmap-capability-continuity.md)补U08/I05/D17/D18等有限出口，主入口为[全项目路线](../roadmap.md)。
- **分档门禁方向已确认**：ENG-D01b采用文档/离线原型/完整档，规则与混合变更走完整档；G03同步并验证后启用，现行规则尚未改变。本轮五仓完整门禁已补验。见[工程施工](../design/engineering-execution-design.md)。
- **类型查看范围**：B01新类型参与人/旧类型现有范围、B02合法CC读评论均已确认；B03普通scope=all不越过类型范围、B04独立授权管理参与人/抄送均已确认；B05风险派单简化建议待确认。“敏感类型”不是新增is_sensitive字段，详见[领域合同](../design/business-lifecycle-contracts.md)。
- **组织赋角补核**：虚拟组可复用org_roles；现有角色展开尚未按组织status/deleted_at过滤，停组不等于已撤派生L1。C04/D06统一生命周期撤权与回归，详见[领域合同](../design/business-lifecycle-contracts.md)；未在本批修源码。
- **能力与债**：§1列已交付能力，§6区分代码验证和运行承接。dbtx ticket域试点已批准但未实施；新资源L2护栏、通知增强等按[Phase5债台账](../phase5/s6-s5-debt-ledger.md)与[Phase6启动批](../phase6/05-launch-batch.md)取用。B23/BK11表达式修复不等于通用dbtx已落地。
- **数据与文档**：迁移保持81对，下一编号 **000082**，新增前按§4与磁盘双核。正式证据统一到[reports](reports/README.md)，已拍规划来源在[secops-platform](../proposal/secops-platform/README.md)，原始过程材料仍只在ignored `.codex/`。维护规则见[documentation.md](../documentation.md)。
- **追溯入口**：[B27最终运行与安全收口](reports/zhuzhao-ecosystem-audit-b27-phase5-closeout-2026-11-07.md) → [11-08后续复验](reports/zhuzhao-report-followup-2026-11-08.md)；首次全量覆盖与固定基线见[11-06总报告](reports/zhuzhao-ecosystem-full-audit-2026-11-06.md)。原失败、NOT_RUN、旧HEAD及未部署表述保留其时点含义。

---

## 0. 项目一句话

Go 编写的**模块化单体 IAM + 工单系统**：三层鉴权（路由 RBAC → 资源级 ltree scope → 属主/委托）+ 资源级工单业务，单实例 Docker Compose（PG + Redis）部署，文档驱动开发（docs/ 与代码强同步）。

**技术栈**：Go + Gin + pgx + Casbin + Wire DI + Redis + PostgreSQL（ltree）+ Docker Compose。

**规模口径**：迁移为162个文件（81对），下一编号000082；源码/测试数量以对应固定报告manifest为准，不用旧统计冒充当前计数。整理前154个生产Go源/148个测试文件/436函数及review数量的2026-10-28原统计保留在[历史规模](archive/project-control-history-2026-11-08.md#旧导航与规模原文)。当前文档入口与分类见[review索引](README.md)。

---

## 1. 模块 × 能力 × 状态矩阵（核心）

| 模块 | 能力 | 状态 | 关键代码位置 |
|------|------|------|-------------|
| **全项目路线重规划/详设** | 71包/510来源/174依赖；28详设候选+5组件原型+2逐页范围+36边界/处置；私有扩展/内网adapter、无SSO设备和版本升级已入账 | 📋 设计评审，owner部分待决；实现均未交付 | [收口](planning-closeout.md)、[需求闭环审计](reports/pre-phase6-requirements-audit-2026-11-11.md) |
| **文档维护** | 五仓各自docs统一分类、正式报告与规划归位、历史与当前状态分层；补查行内路径、示例前置与图示 | ✅ 2026-11-08首次整理及二次复核内容/静态/必要本地门禁收口；历史失败、缓存命中和未覆盖专项如实分列 | [维护约定](../documentation.md)、[二次复核](reports/documentation-recheck-2026-11-08.md)、[review索引](README.md) |
| **提交规范门禁** | 五仓同源消息规则、逐ref pre-push、CI事件精确窗口、worktree安装与固定历史基线 | ✅ 2026-11-08本地五仓各42反例通过；首次文档候选五仓已推且对应CI全绿，本轮新修订另核；五仓main已承接Phase5；本轮规划另审 | `scripts/commit-policy.py`、`.githooks/`、各仓CI；[RR-GIT-01](reports/zhuzhao-report-followup-2026-11-08.md) |
| **后续重规划** | 五仓工程/目录/模块、专业供给、AI双轨、全站设计与全部遗留去向 | 📋 规划已落档；当前71包尚未实施，真实提供方与视觉待验分列 | [路线](../roadmap.md)、[总账](planning-backlog.md)、[报告](reports/pre-phase6-replanning-2026-11-11.md) |
| **认证** | 登录、双 Token（AT/RT）、RT 轮换、登出、黑名单、登录限流、会话吊销、改密 | ✅ Phase 1 | `internal/service/auth_service.go` `session_revoke.go` |
| **用户** | CRUD、启用/禁用、密码修改/重置、角色绑定、组织绑定、超管保护 | ✅ Phase 1 | `internal/service/user_service.go` |
| **角色** | CRUD、菜单分配、Casbin 策略同步、角色优先级（防提权） | ✅ Phase 1 | `internal/service/rbac_service.go` |
| **组织** | 树形 CRUD、ltree 路径、move、成员管理、owner/成员角色、虚拟组、scope | ✅ Phase 1+2 | `internal/service/org_service.go` `org_delegation.go` |
| **菜单** | 菜单树/详情、用户菜单/权限码；词表与API绑定由迁移维护，生产运行时菜单CRUD已退役 | ✅ Phase 1底座 + Phase 4 W1只读边界 | `internal/service/menu_service.go` |
| **审计** | 操作日志中间件、同步写、登录审计、事件表 FK 去 CASCADE | ✅ Phase 1 + 2c（000014 去 CASCADE + deleted 事件） | `internal/middleware/audit.go` `internal/service/audit_service.go` |
| **鉴权 L1** | 路由级 Casbin RBAC（BFS 角色展开 + 超管通配） | ✅ Phase 1 | `internal/middleware/casbin.go` `internal/casbin/enforcer.go` |
| **鉴权 L2/L3** | 资源级鉴权：属主/assigned/scope/虚拟组/BFS 三源/组织内委托 | ✅ Phase 2 | `internal/service/ticket/resource.go` `scope_resolver.go` |
| **工单** | CRUD、状态机（open/assigned/in_progress/pending_verify/closed/rejected）、分派、评论/备注、关联、类型/模板、可见性；**W4 随批件（2026-09-29）**：`assignee=me` 列表筛选（P1-c，工作台待办/已办数据源）+ 列表/详情姓名回填 `created_by_name`/`assignee_name`（03 S9，处理人列 N+1 消除；**2026-10-13 升结构化**：新增 `created_by_no/assignee_no` 工号消歧字段 + 评论作者同款回填，前端 UserNameText「人名(工号)」——见 03 S9/S10 契约增补）+ Update 字段级 COALESCE（十六批 Med，nil 不覆盖） | ✅ Phase 2 | `internal/service/ticket/service.go` `state_machine.go` |
| **工单模板/关联** | ticket_templates（org_path ltree）、ticket_relations | ✅ Phase 2a（迁移 000015/000016） | `migrations/000015*` `000016*` |
| **跨环境配置晋级** | 已发布工单类型、流程、字段和模板的配置包，含审批绑定、版本化发布和补偿 | 提案，未实施。原件 7 登记保留为重规划输入，确认实施后在首次生产配置晋级前验收 | [配置晋级方案](../proposal/config-promotion.md)、[件 7](../phase6/06-ticket-completeness.md#37-件-7-类型定义包导入导出) |
| **基础设施** | Wire DI、配置、优雅关闭、健康检查、迁移、限流、安全头 | ✅ Phase 1 | `internal/app/` `internal/pkg/` |
| **平台策略库（批次 A）** | 内置行级策略：`org-member`/`owner-only`/`role-gated` + `Builtin()` 注册；`RequireSchema`须由消费者显式调用，Register不自动查库 | 🟡 **库就绪待接线**：生产装配零消费者；原M-E消费预期已因独立库边界取消，首个本库新资源需要L2时触发；工单手写策略与builtin双路并存 | `internal/pkg/resource/builtin.go`；[当前边界](../modules/authz.md) |
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
| **break-glass 应急通道（admincli）** | `zhuzhao reset-password` 子命令：superadmin 密码恢复（目标校验须持启用 superadmin 角色 + 复用 HTTP 侧重置语义[bcrypt+must_change_password+全端会话吊销] + 禁用连带启用 + audit_logs 落行 method='CLI'）；同二进制子命令分发零镜像改动；流程与兜底 SQL 见 `docs/ops/runbook.md` §1 | ✅ 2026-10-12（集成 4 用例常态门禁 + dev 栈演练） | `cmd/server/reset_password.go` `internal/pkg/admincli/` `internal/service/session_revoke.go` |
| **前端控制台 zhuzhao-ui（P4-W2–W5+P4-9）** | **全线交付（2026-09-29 收官）**：壳层四件+登录/强制改密/个人中心+暗色主题+CI+codegen；system 四页+工单全域+al/task/audit 三域+P4-9 部署件；E2E 19 spec/21 用例打真五栈；单测 15 文件/115 用例；四轮审计 31 项全部清账 | ✅ P4-W2–W5+P4-9 全交付（2026-09-29）；审计修复链 2026-10-03 终态 | `../zhuzhao-ui` |
| **对象存储与富媒体（P5-S5）** | storage.Client 抽象（minio-go）+ file_objects/ticket_attachments 表族 + 头像/附件/富文本字段 + 软删 GC 四段（pending/deleted/反连接回收/avatar reaper observe） | ✅ 既有能力已交付；2026-11-08 RR-UI-01：UI bebfe523详情缓存清空/移除与异步身份修复已原gate/253/81复验 | `internal/service/storage/` `migrations/000045–000050`；[B06报告](reports/zhuzhao-ecosystem-audit-b06-storage-transactions-2026-11-05.md)；[本轮复验](reports/zhuzhao-report-followup-2026-11-08.md) |
| **表单字段设计器（P5-S1）** | field_meta JSONB（formula/visible_when/auto_fetch ③⁺ 全链设计定稿）+ 十一种字段类型（B1-6 file 第 11 类型） + 字段影响分析 + B1-7 语义收口（锁死守卫/禁链式） | ✅ 2026-10-22~26（五批+B1-7）。公式同名绕过与求值输入隔离作为 C1 移交 Phase 6，保留 P1 复核优先级。 | `internal/service/ticket/` `migrations/000051/000052`；[Phase6 件 26](../phase6/06-ticket-completeness.md#327-件-26-公式引用校验与提交重算) |
| **消息通知一期（P5-S6）** | DomainEvent 事件面（进程内 HookRegistry+Drain 闭闸）+ ChannelSender 端口（UIP 两态桩）+ 模板/rules 路由/message_log 死信 + 用户偏好 + human_key 编号 | ✅ 既有能力已交付；2026-11-08 RR-DOC-01：UI14事件选项/跨仓集合守卫及A.9登记已原gate复验 | `internal/pkg/domainevent/` `internal/pkg/channelsender/` `migrations/000053–000061`；[06号A.9](../phase5/06-s6-message-notification.md) |
| **站内消息中心（P5-S6 二期）** | notifications 表族 + inbox 双域订阅 + 读面三端点（自服务）+ 消息中心双 tab 页 + unread 并入角标 | ✅ 2026-10-24（🔵 待走查） | `internal/service/ticket/inbox_service.go` `migrations/000072` |
| **流程图设计器（P5-S2）** | LogicFlow 画布/JSON 双模式 + mapper 往返幂等（21 测）+ lint 三规则 + 在途告警（纯前端零迁移） | ✅ 2026-10-24（ 待走查） | zhuzhao-ui（02 号附录 A） |
| **工单协作与提醒（P5-S10）** | 抄送（L2 特例通道——可见不可改）/标签（entity_type 泛型）/角标（计算器注册制）/节点超时+催办/Excel 导出（BK-21 同源三件套） | ✅ 2026-10-24（五批全清；🔵 待走查） | `internal/service/ticket/` `migrations/000062-000065` |
| **任务面人性化（P5-S4）** | 动作清单端点+提交 fail-fast 21010 + CronBuilder 双模式 17 测 + 死信对账结论=三 Tab 维持 | ✅ 2026-10-24（🔵 待走查） | `internal/pkg/jobs/` `migrations/000066` |
| **威胁情报缓存代理（P5-S9）** | intel_cache TTL 7/30 天 config 化 + singleflight + 429 降级矩阵（8 子测）+ intel_cleanup cron | ✅ 2026-10-24（🔵 待走查） | `internal/pkg/threatintel/` `migrations/000067` |
| **语义引擎（P5-S3a）** | transition 端点 + condition/validator（transition_meta "from>to" 键）+ assignment_rules 自动分派 + ActionRegistry 四动作（notify/cc/reassign/spawn_task）+ SLA0.5 deadline 激活 | ✅ 2026-10-24（🔵 待走查） | `internal/service/ticket/` `migrations/000068-000071` |

### 未实现 / 延后（明确不做）
- ~~**附件**（file_objects/ticket_attachments）— 2b-ext 延后，迁移编号启动时按 A2 取下一可用号（现 **000026**；000017 已被 IW1 占用）~~ **✅ 已交付（Phase 5 S5——000045 file_objects / 000046 ticket_attachments / 000047 owner 列 / 000049 部分索引；2026-10-19~21，见 §4 地图与 [phase5/05 号](../phase5/05-s5-object-storage-richmedia.md)）**
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

`user:create/update/delete/status/assign_role/assign_org/reset_password` ｜ `role:create/update/delete/assign_menu` ｜ `org:create/update/delete/move/member`

> 上列仅为用户/角色/组织管理类速查，完整当前L1词表以 `menus`/`menu_apis` 迁移和路由目录为准。菜单生产写端点已退役，不再列菜单CRUD权限码。工单业务路由先过 **L1路由RBAC**（如ticket:list/create/update等），再过 **L2可见性/L3动作权**；cc/tags等已登记selfService的入口豁免L1业务码，仍由服务端检查读可见或update动作权，不能把自服务豁免推广到全部工单操作。

---

## 3. API 分组速查（`/api/v1`）

本表用于定位IAM、工单与任务代理分组，不穷举Phase5全部增量端点。精确路径与资源读写语义以`internal/router/router.go`的`New`路由注册及对应模块合同为准，不能将本表未列出的端点视作不存在。

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

## 4. 数据库迁移地图（81 对）

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
| 000080 | 存储用量越限告警种子：ops_storage_usage_alert 模板 + ops.storage_usage_high 规则（fixed+空收件人=管理员配置后生效，同 000064 惯例；事件源=storage_usage_sample cron 阈值跃迁） | phase5 收官观测批（2026-10-27，C-6/G-3/C-16） |
| 000081 | 设置恢复默认端点绑定：POST /api/v1/system/settings/reset 挂 system_setting_reset 按钮行（000048 惯例三件套：按钮行+menu_apis+role_menus admin/superadmin）+ 删 system_settings 覆盖行回 config 出厂值（幂等） | phase5 设置页走查批（2026-11-03，0081f2d——本行 2026-11-04 补登） |

> **编号规则**：A2（2026-08-31）=谁先启动谁占用，后者整体重排。000001–000081实际占用见上表，**下一编号000082**；新迁移前同时核磁盘与本表。历次让位/重排的原文保留在[编号演进历史](archive/project-control-history-2026-11-08.md#迁移编号演进原文)，不以旧“下一编号”指导新建。

---

## 5. 测试与门禁体系（你的"一键验证"）

| 门禁 | 命令 | 覆盖 |
|------|------|------|
| 单元测试 | `make test-unit` | 默认build标签、全internal包、race；数量依受测快照，历史121函数为2026-09-14时点 |
| 集成测试 | `make test-integration` | 需 PG/Redis |
| 覆盖率 | `make test-cover` | 覆盖率报告 |
| **全量验收** | `make acceptance` | **四档链式门禁**（经 2c 脚本）：phase1（27组、当前124断言）→ 2a（68）→ 2b（26）→ 2c（35）；精确范围见本页当前测试基线 |
| 分档入口 | `make acceptance-2a` / `-2b` / `-2c` | 单档运行（-2c 含全部上游回归） |
| 其他 | `make lint` `build` `swag` `benchmark` | 静态检查/构建/文档 |

**用法建议**：AI 迭代完成后，你只跑 `make acceptance` 看绿灯 + `make test-cover` 看覆盖率趋势，即可验收，无需读 diff。

---

## 6. 当前健康状态（遗留问题追踪）

### 2026-11-11 全项目路线重规划

专业供给优先、mock保留、模型仅既有API和领域Agent双轨已落[现行设计](../design/platform-evolution.md)；首轮61包/494来源已由当前71包/510来源继续校准，实施/接入/取舍见[总账](planning-backlog.md)。C1–C3保持开放并重锚P6.0；R01可靠意图先于不可丢消费者。全站27页有设计迁移清单，尚未改造；模型部署/重复专业平台不在本轮范围；通用DAG自研与强制SOAR迁回的旧技术形式建议用更小协同替代，业务效果仍须承接/审阅。本轮现有实现回归已通过：主仓lint/233单测/531含integration测试、253验收与81对往返；UI244单测/42真实E2E。默认tag单测覆盖率20.4%，仅作当前口径记录，不宣称因文档改动改善。当前没有新增API/权限/迁移或风险/AI实现。验证与未覆盖见[本轮报告](reports/pre-phase6-replanning-2026-11-11.md)。

### 2026-11-11 Phase 5 已收尾

阶段交付与当前工作区清理已完成，13 组余项及待验记录保持移交状态。清理 10 个测试容器、5 个空网络、5 个验证镜像、1 个空卷、45 项临时产物和 51 处隔离缓存。三库备份和原测试数据卷保留，当前 6 个 dev 容器及原生服务健康。完整证据在本机 `.codex/audits/phase5-final-closeout-20261111/`，后续进入全项目重规划。

### 2026-11-11 Phase 5 收尾整理与后续重规划

- 所有者已完成一轮前端初验，[12 号](../phase5/12-scenarios-and-tests.md) 保留 97 步的历史结果、自动补证和未验边界。自动复验固定快照主仓 0707e9c6、UI 3833ca2c 的 253/81 与 UI 244/42 通过，后续配置 29d78a69 的超时观察及发送模式另有定向证据。上一轮文档提交 ba02a8de 的远端 lint、单测、覆盖率、全量集成和提交规范检查通过。
- 按所有者后续指示，前次 13 组代码修复条件统一移交 Phase 6。C1 公式、C2 文件引用与归属、C3 回调响应及记账保留 P1 优先级，当前范围、后续核验线索和触发条件见 12 号 §8.2。延期项保持未修状态，C2 尚未证实越权下载，C3 需分别考虑实际执行成功、在途重放与记账失败。
- 本阶段先收口工作区、内容、文档位置及提交推送。五仓 8 个已注册工作区和 10 个本地分支的遗漏、脏文件及远端可达性已核对。Phase 5 收尾后进行全项目重新设计规划，旧接口、波次、方案和估算作为输入；配置晋级提案保留 proposal 位置，实施前再确认完整契约。

### 2026-11-10 Phase 5 走查修正

- 工单建单和自定义字段更新统一显隐与公式求值的字段快照。隐藏公式不执行，计算产物校验必填、类型、正则和最终大小。字典成功返回空启用集时只允许延续旧值，字段公式和显隐或流转的停用项查询分开。
- 字段配置只有成功加载当前版本与字段后才允许提交。设计和专家模式共享草稿，保留已有 MIME 白名单、静态选项值和未编辑元数据。流程编辑保留状态级 SLA，建单、分派与其他状态入口的截止时间收敛仍未修，按后续指示作为 K2 移交 Phase 6。
- 发起页切换类型时立即隔离旧字段，详情按当前工单与类型归属读取数据。活动列表全量替换确认绑定当前内容，过期或失败的文件读取不能提交旧内容。
- 本地修复快照通过主仓 lint、单测和全量竞态集成、四档 253 条验收和 81 对迁移往返。默认单测覆盖率按一位小数为 20.3%。UI 六件门禁通过，含 244 条单测和 42 条 E2E。走查缺陷记录与验收通过分列。硬删字典项后的公式兼容、文件字段更新绑定、回调成功状态等剩余问题继续登记。阶段决策仍按 [Phase 6 总纲](../phase6/README.md) 执行。

### 2026-11-08报告复核后修复

| 发现 | 当前状态 | 验证与边界 |
|---|---|---|
| RR-UI-01详情richtext缓存 | UI bebfe523生产修复、十条回归与独占联合门禁通过，代码复验关闭 | 两目标各八轮变异稳定红；gate240/41、253/81过，未推送/合main/部署共享服务；[复验报告](reports/zhuzhao-report-followup-2026-11-08.md) |
| RR-GIT-01提交规范 | 五仓自包含三钩与CI真实窗口已接线，本地各42反例通过 | 固定基线祖先不追溯；主仓另10+5回归；未推送所以远端CI未验证，canonical main仍未承接新门禁 |
| RR-DOC-01登记/事件词表 | UI 22d9fafa，S6 A.9补M-2/M-4/richtext事实，注释改指真实位置，词表14键一致 | 新guard旧红/修绿、等数量错键红、缺源失败；第三批原gate241/41及253/81通过，无新端点/迁移；未push/main/部署 |
| 依赖/归档收口复核 | utils远端引用保护已登记；23文件/22批号及B01/03/04、B17/B22归属已补；镜像线索未冒充实证 | 冷下载/38源码、五仓11笔严格窗口及本机147392成员校验、13件恢复/5旧bundle通过；原Go四门禁/253验收/81往返通过，20.4%；最新候选恢复另留收据，同盘非异地；不改矩阵/迁移/pin |

### 2026-11-06全面审计承接

以下分支验证与主线/生产承接分开；不将后面的历史底座行理解为当前全项目零缺陷。

| 审计组 | 当前验证状态 | 主线/运行边界 |
|---|---|---|
| B02撤权/B09过期成员，B06业务 | dc3含F13–F20等后笔已固定正常门禁、253/81和真实13/15专项；对应代码关闭 | 保留test已承接最新60ad/ALb3ed/TR69d/UI152组合（B27）；主线未合/生产未发，旧专项/已接受边界逐报告保留 |
| B07 AL/TR | test实际ALb3ed/TR69d；主仓/AL正式pin cd759，AL60全PG/race196通过，新组合253/81过，F10关闭 | utils仅codex分支已推；main/tag不动、生产未发；AL不使用出站Transport及TR不升无关依赖的边界保留 |
| B08前端 | 原14项限定范围关闭；最新test UI152/60ad六件+41首过及Chrome代表走查通过 | 本轮UI阶段0HTTP429；W5旧概率性负控边界仍见B18。dev旧实例已更新运行源，不把它继续记为旧F08开放实例；未合main/生产或全站认证 |
| B11当前L1矩阵 | 19主体八轮25840请求/5440交叉差异0，catalog反例有效 | 限L1到达与认证分类，业务L2/L3看各批；不复用旧14号为当前证明 |
| B12错误码契约 | 77当前定义补全，AST定义/清单/登记/消息/HTTP双向守卫，4da89fa固定专项和四类各8轮变异有效 | 只改登记与测试，无生产HTTP变更，已dc3完整门禁关闭，内容完整性/路径归一分列；主线仍待承接 |
| B14资料/安全状态 | f917c64原/修复/变异及原生专项过，最终dc3完整门禁/253/81通过，代码复验关闭 | 保留test已承接最新60ad组合，未合main/生产；头像同类F20的原专项同轮关闭，不扩成所有未来并发安全保证 |
| B13环境/启动 | dev保卷恢复4宿主服务，test固定版本在线，真实Chrome代表走查已核；启动候选760a583固定679文件零差异、19桩回归过/五类变异各8红 | 端口表准确登记test33334/UI48081/RustFS59000及第二dev33335须核归属；候选真实启停/构建与完整门禁边界保留，未合main，不以healthy/命令桩代替验收 |
| B15横切边界 | 原四P2中工具sum/回滚前置已B18准确CI复验，W5已b5cbf15专项验过；B19存储输出ID统一string并接生成类型，真实HTTP/JS与8轮变异、Go及253/81通过 | ID wire为破坏性变更，两仓已同步test承接；初版B20画布失败已修，最终41与准确主4db/UI152 CI通过，仓外numeric消费者仍须协调。down79的23503保全业务数据但可留dirty，P1候选撤销；不把预检等同回滚许可，详B15/B18/B19 |
| B16五仓安全验证 | UI生产依赖本轮0报告；首轮34 GO-ID已分层复验，F01/P2以bcdf133专项/稳定变异及完整门禁关闭，test配置源已切正确模板 | 主/AL stripped仅模块精度，TR符号不等于调用图；两fix提交元数据缺项已如实补录，不改已推历史，main未动 |
| B17安全依赖修复 | 主603/ALdb6/TR1e31/utils5637准确CI、原253/81、新3ELF/4source复扫、最低版本96次ABA均过；Go1.26.10及net62/quic59.1，主另Excelize2.11.1/compress1.20.2 | F02在代码/test关闭，F03已升依赖、唯一OpenPGP模块提示按未导入边界保留；保留test与原生dev均已承接Go1.26.10，main/生产未切，无新正式发布 |
| B24冷启动TCP就绪 | 三仓6 Compose/8 pg_isready补显式TCP；默认/全profiles展开、8轮旧误绿/新真实AL ready对照及三仓准确CI通过 | 本机5PG已同卷承接，生产模板未现场部署；不改变重启/时序/数据，不以TCP替代迁移和应用ready |
| B25归档持久化 | prod/test app增加JSONL专属卷；真实导出3+2旧行、替换容器及PAX→fresh卷恢复内容/元数据一致，395天mtime边界专项过 | 原目录均0文件，未证明已丢数据；test已接归档卷，备份/作业未自动启用，不改保留期或taskrunner执行日志；源规范/专项与组合夹具门禁分列 |
| B20前端反馈 | UI15225863：badge实际订阅、画布重复load后撤销重做、附件失败恢复；最终29/230与41/41原样门禁通过，实际0retry/flaky/skip | 初版40过1失败及尾钩缺helper保留；UI0个429，三认证读均负控，外部testutil生命周期变化分列，准确UI152 CI及test/Chrome代表承接已B27核 |
| B26字段软提示 | G-14完整值集合/radio/字典切换与读取不确定提示，正式354源完整Go及新60ad组合253/81通过 | 原故障策略和非阻断保存不变，同字典项外部变更无历史快照；准确4db CI、UI批和dev/test均B27承接；不混入通用dbtx试点 |
| 文档与历史债 | B10五组校准、130篇全文基线及最终差异、101编号债逐行映射均已核；B21/L-5两主体已按固定源码回填并经独立语义/链接复核 | L-5主体在Phase5文档分支关闭；跨文件P2/P3与L-1～4搬树仍各有边界，其余未来/已接受取舍不当当前回归；Go源码/真实ELF补扫已B16/B17完成，许可证/完整供应链认证不扩大 |

### 历史底座记录


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
| **BK-21** | IW4 护栏未泛化：fail-closed 哨兵 + AST 守护仅覆盖 ticket_repo 一处，新资源接 L2 时漏接 GetFilter 仍=静默全量（2026-09-08 OPA/ReBAC 复核清点，11-authz §9.3） | 🔶 **已排位（2026-10-27 回填——原「触发驱动」已被 phase6 Week 0 基建显式排位[2026-10-26 拍——时点为四件，后扩七件]超越：BK-21 泛化随 Week 0，R1 开工前落地）**（哨兵泛化 registry 层或 AST 守护扩展至全部 repo.List 调用点）；详见 00 §9 |
| **BK-22** | 路由↔menu_apis 一致性对账：menu_apis 迁移行与 router.go 实际路由靠人工对齐，接错=混权（有路由无码=裸奔、有码无路由=死策略）；RuoYi-Go 双项目核验实锤同款痛点（Kun 版 120 条路由权限串硬编码与 sys_menu 种子人肉同步） | ✅ **已实施（2026-09-09）**：internal/router/catalog.go 双向审计（豁免集+网关前缀跨仓边界）+ wire 层启动 fail-fast（缺口拒启）+ 集成发现跑（真抓出 audit/logs 权限面缺失 → 000025 补菜单/audit:read/绑定）；Form B 时转上报校验器（design-decisions §26.2）；详见 00 §9 |
| **RT-1** | RT 轮换重用检测：提交**非当前槽位**的签名有效 RT（含已轮换旧 RT）→ `GetDel` 删除该键（**连带使当前有效 RT 失效**）→ hash 不匹配 → 401；含多标签迟到提交误伤代价 | ✅ **设计内（2026-09-11 定性；2026-09-15 错误码状态拆分）**：旧 RT 被重放本身即盗用信号，拒绝并使当前会话失效、强制重登，符合 OAuth BCP / Auth0 / Google 的 reuse detection 最佳实践；作用域**单设备（用户+deviceId 单槽位）**，不跨设备、不涉数据正确性；多标签误伤属已知代价（触发驱动评估宽限窗口/family_id）。**衍生观察（2026-09-18 登记，触发驱动不修）**：20015 命中当前不落审计事件/WARN 日志（安全信号仅存在于响应码，运维侧不可见），且多标签良性迟到提交亦命中 20015——加观测须先有降噪前提（宽限窗口/family_id），届时同框评估。**对外码 2026-09-15 起三态分离**：重放 hash 不符 → **20015**、密码纪元不匹配（C3）→ **20014**、解析失败/空槽/账号状态（防枚举）→ 20004。详见 `docs/phase1/02-auth.md` §RT 轮换流程 |
| **并发-P1-1** | 回调幂等栅栏 TOCTOU：`EnsureCallbackRow` INSERT ON CONFLICT DO NOTHING + 独立 SELECT 两段式，并发同 task_id 回调全部读到 submitted → handler 重复执行（真机 8 并发 → 副作用 8 次） | ✅ **已修复（2026-09-11）**：单语句原子抢占 `ClaimCallbackRow`（`INSERT ... ON CONFLICT (task_id) DO UPDATE SET status='running', claimed_at=NOW() WHERE status IN ('submitted','failed') OR running 陈旧` RETURNING）；迁移 000026 补 `claimed_at`（陈旧 running 10 分钟可重认领自愈）；集成测试 8 goroutine 锤击仅 1 次副作用 |
| **并发-P1-2** | 工单关联反向判重缺失：`uq_ticket_relations_pair` 方向性唯一索引 + service 层事务外预检 → 并发 A→B / B→A 双双入库（真机 59/60 轮 2 行残留） | ✅ **已修复（2026-09-11）**：迁移 000028 规范化对部分唯一索引 `uq_ticket_relations_normalized(LEAST,GREATEST,relation_type) WHERE deleted_at IS NULL`（先软删历史重复行再建索引）；23505 → `ErrConflict` 409；集成测试 60 轮并发零残留 |
| **并发-P1-3** | 类型/字段/模板并发编辑 lost update：三表无 version，后写静默覆盖先写（BK-18 随手项 2026-09-01 登记的正式收口） | ✅ **已修复（2026-09-11）**：迁移 000027 三表补 `version`；`UpdateTicketType` / `ReplaceTypeFields`（以父类型 version 为权威谓词）/ `UpdateTicketTemplate` 支持**可选 version CAS**（nil = 旧 patch 语义；携 version 命中才写，0 行 → 409 `ErrConcurrentModification`，ErrNoRows 区分 404）；集成测试 lost-update 场景验证 409 |
| **并发-P2-1** | 组织软删 × 成员插入竞态窗口：软删 UPDATE 取 `FOR NO KEY UPDATE` 与 INSERT FK 检查 `FOR KEY SHARE` 不冲突，单侧加锁无法闭合（实测证伪「仅守卫 FOR UPDATE」「仅 AddMember 侧」两方案）；软删不使 FK 目标行失效 → 迟到 INSERT 仍成功 | ✅ **已修复（2026-09-11）**：**两侧锁协议**（无迁移）：Delete/DeleteVgWithOwnerCleanup 守卫前对 org 行取 `FOR UPDATE` 事务性认领（随提交/回滚释放，勿加 deleting 列/勿复用 status）+ AddMember 侧 4 处 `INSERT INTO user_orgs` 同事务取 `FOR SHARE` 复核 `deleted_at IS NULL`（`org_repo.go` AddMember/AddMemberWithRole/SetUserOrgsTx/SetOwnersTx；**严禁改用 FOR KEY SHARE**——与软删 NO KEY UPDATE 不冲突）；集成测试 25×2 轮并发删除×加成员零残留 |
| **BK-23** | 迁移 up/down 验证方法盲区：现行 down 验证只断言「down 过程不报错」，**无 up→down→再 up 的 A/B 数据快照比对**（批 29 全链 down 验证 31 对→0 仍抓不到 D-1 类问题=实证，code-audit-2026-09-26）；同系触发驱动盲区=blind-spots-2026-09-22 **B-1**（回调幂等栅栏租约无心跳/无执行时长上限——长任务跨 10 分钟窗口可被并发重认领双执行，`audit_archive` 首跑积压必踩）/**B-2**（49 个 CREATE INDEX 非 CONCURRENTLY，在线迁移锁表维度零登记）。**provenance 盲区补记（2026-10-12 000042 B/C 二次实证）**：快照 roundtrip 对「授权来源类」缺陷**结构性盲**——down 误删他源规则后再 up 会原样重建，快照零漂移判 PASS（000041 down 误删/000042 首版两缺陷均实测过环）；辅助手段=down 守卫语义对拍（menu_apis 来源维度）+ AssignMenus 重放测试（已随 000042 重写批落 `role_repo_integration_test.go` 为常态门禁） | 🔶 **已登记（2026-10-10，文档审计循环收口批）**，触发驱动：BK-23=下次迁移窗口或迁移规范修订时补「快照比对」验证法并接入门禁；B-1=长回调任务/归档积压场景出现时补租约续期+时长上限；B-2=大表索引/在线迁移诉求出现时按 blind-spots §B-2 建议实施 |
| **BK-24** | GET /orgs 读面超陈述（0373011 复核批挂账②）：非 admin 开放组织树读后，响应携带 `owner_user_ids`/成员计数等管理面字段——信息暴露面大于「发起工单选组织」所需。收窄方式待设计拍板：非 admin 精简 select 分支 或 响应脱敏（字段白名单） | 🔶 **已登记（2026-10-12）**，触发驱动：进内网前安全审查时收口（与 M-UAA 批同窗，随数据最小化一起拍板）；L3 行级面不变（B13 盲区 #3 定性维持） |
| **BK-25** | utils 下版窗口（v0.4.3）小批（2026-10-15 全仓审计挂账）：README 依赖边界声明滞后（response 实依赖 aksk，:25 仅称 errcode）/错误消息包名前缀统一（jwt·redis·postgres·jsonutil 无前缀，aksk·compat 为范本）/aksk·compat 两 gin fail() 码值字面量补 why 注释/crypto·errcode·logger 三包零测试（CheckDummyPassword 时延语义无回归保护）/jwt AT 过期哨兵用例/response 补限流 429 信封助手（zhuzhao ratelimit.go 手拼 gin.H 随批收编） | ✅ **已实施（2026-10-15 清欠批，utils `1c27ca2` + zhuzhao 同批）**——main 直提不动 tag（消费方 pin v0.4.2 零影响），随 v0.4.3 发版收编；429 助手一项豁绝：现有 `response.Fail(c,status,code,msg)` 已支持任意状态码+request_id，无需新增助手，ratelimit.go 直接收编 Fail+Abort |
| **并发-P3-1** | Create 事务窗口嵌套池取死锁：`fileFieldKeys`/`keyGen.Next` 在 `Begin` 后再向连接池要第二个连接，锤击（3 Create+1 Move）在 4 连接池（pgxpool 默认 `max(4,GOMAXPROCS)`——CI 4 核即 4、本地多核 ≥8 永不复现）构成池耗尽循环等待——2026-10-24 起 CI 15 run 挂 7 笔（`TestBK11_CreateVsMoveHammer` 20m 挂死签名）；**生产 4 核部署 ≥4 并发 Create 同样可永久挂死**（B1-6 批1 `50f40a5`/S6 补遗 `f055c60` 两批引入嵌套点） | ✅ **已修复（Create=2026-10-26 `039aca6`；Update 同类残留=2026-10-27 随批）**：Create 两池调用提事务外 + testutil 池钉 4 + 锤击 30s 兜底（先红后绿）；**Update 残留**（`validateCustomData`/`applyFormulas` 在锁内窗口走池读——6 并发×MaxConns=4 自闭环死锁，外审核出+锤击实证 round 0 即挂）同批修：repo 增 `RowQuerier`+`ListTicketTypeFieldsTx`（pgx.Tx/池双满足），两 helper 接取数通道参数（Create 传池/Update 传 tx），`TestBK11_UpdateHammer` 先红（ctx deadline）后绿（0.12s）；复盘 SSOT = [13 号](./13-bk11-pool-deadlock-2026-10-26.md)。**后续状态**：dbtx ticket域试点已于2026-10-28批准、传播器与推广仍未实施；请求deadline未单独批准，随试点评估；humankey表化/schema缓存仍挂账。`resolveClosedOrMissing`及表达式q/ctx残留已由B06/B23独立修复，不能把它们继续计为未修或扩称传播器已落。有效决议见[13号§8](./13-bk11-pool-deadlock-2026-10-26.md#8-决策状态)。 |

---

## 7. 文档导航（docs/ 体系）

```
docs/
├── README.md / documentation.md  # 阅读入口 / 五仓维护约定
├── standards.md                 # 生态工程公约
├── design/ / adr/                # 架构解释 / 已定架构决策
├── modules/                     # 跨阶段模块说明
├── proposal/                    # 方案；secops-platform保留研究与已拍来源
├── phase1/ … phase9/            # 阶段状态、实施边界与启动条件
├── api/ / ops/                  # 调用契约 / 可执行运维说明
└── review/
    ├── 11-project-control.md    # 当前能力、迁移、健康状态
    ├── 13-bk11-pool-deadlock-2026-10-26.md  # 复盘及有效dbtx决议
    ├── audit-playbook.md        # 可复用审查方法
    ├── reports/                 # 正式固定时点证据
    └── archive/                 # 历史审查、计划和旧流水
```

Phase5当前收口与main/生产承接看顶部和§6；Phase6实际启动须遵“Phase5收口→经授权合main并核验→从main切出”顺序。Phase7–9是后续总纲，不能从目录存在推断已立项。各阶段具体状态回到其README，原周数和日历说明已保留在历史记录。

**掌握流程**：遇到能力问题 → 查本文件矩阵定位模块 → 按需深入 `modules/` 对应文档 → 变更后回填本文件。

---

### 7.x B13 权限覆盖矩阵（2026-09-26 完成）

| 文档 | 状态 |
|------|------|
| [12-b13-permission-matrix-2026-09-26.md](reports/12-b13-permission-matrix-2026-09-26.md) | ✅ point-in-time 快照：79 端点×三层全对账；8 项盲区/不对称定性（审计/用户/组织读=管理面全局视图设计内；orgs update 不对称注记；users/orgs 建议 W3 后补 org 侧校验）；BK-21 泛化未到触发点；IAM 四平面边界清单（§26.1 输入交付） |

## 8. 遗留问题分类：Phase 3 前置 vs 随行（历史来源）

以下保留历史分类与关闭依据，不再承担现行排期；所有仍开放项与触发条件已在[总账](planning-backlog.md)及来源映射重锚，不能据旧“随手/按需”无限搁置。

> **历史分流（2026-08-31至09-02）**：本节保留当时「下一步」清单及后来处置出处，**不是当前全部开放债清单**；B3等后继已由Phase5交付，不能因下表仍保留原计划而重开。当前状态先看§6，再看Phase5/6权威台账。本节当时将事项归入两档：**A 档 = Phase 3 启动前/启动时完成**（门禁与拍板，不做会让启动本身踩坑）；**B 档 = 随 Phase 3 对应子能力一起**（提前做无收益）。代码级 backlog 详情见 [phase2/00 §9](../phase2/00-implementation-plan.md)。**Phase 3 启动时从 [phase3/00-startup-checklist.md](../phase3/00-startup-checklist.md) 进入检查流程**（本节 + §6 是其数据源）。
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
| B12 | **BK-21 IW4 护栏泛化**：fail-closed 哨兵 + AST 守护从 ticket_repo 泛化到 registry 层通用机制（或 AST 扩展覆盖全部 repo.List 调用点）；导出功能为高危场景（绕过分页全量拉取）必须接 L2（2026-09-08 OPA/ReBAC 复核清点，11-authz §9.3） | 随首个新资源接 L2 / 导出功能（新资源经 Builtin/手写 Resource 接入时一并落）——**2026-10-27 回填：已随 phase6 Week 0 基建显式排位（2026-10-26 拍，先于触发到达）** |
| B13 | **权限覆盖矩阵审计（提议待拍板，2026-09-08）**：全端点 × 三层 × 「设计内豁免 vs 遗漏」逐行对账，产出覆盖矩阵 review 文档；顺带产出 ① IAM 平面边界清单（design-decisions §26.1 三步走①的输入）② 全文档 `internal/*` 路径引用对现状核对（F-1 路径腐烂教训推广） | 半天–1 天，doc-only；拍板后排期 |
| B14 | **BK-22 路由↔menu_apis 一致性对账**（启动期/架构测试双向 fail-fast；Form B 能力目录时转上报校验器，design-decisions §26.2） | 随批次 B（activelist API 入 menu_apis 同批落地），~半天 |

### 独立窗口（已触发，Phase 2 范畴，不属于 Phase 3 前置或随行）

| # | 事项 | 说明 |
|---|------|------|
| IW1 | ~~多虚拟组可见性场景闭环：BK-13 + BK-14~~ | ✅ **已实施（2026-08-31）**：000017 CHECK + org update 配置 API + L2 委托轴 + scope 配置面全链 + D12/委托轴测试（原编号 W1，随 2026-08-31 编号治理改 IW；详见 00 §2.3） |
| IW2 | **2b-ext 剩余两件**（附件 / auth-enhance；**HR 同步已升 Phase 3 主链 M-HR**（原 M2.5），2026-09-02 §22.2/§23.2） | 附件 / auth-enhance 按需独立启动（附件先于 Phase 3 启动 → 按迁移号规则核对 A2）；HR 同步随 M-HR（预留接口版 HRFetcher + 引擎 + mock adapter；⚠ 2026-09-22 勘误：**三件均未落码**，全仓无 HRFetcher——对接时一并开工），**两个待设计项随启动拍板**（hr-directory-sync.md §3.2/§4.3 的 2026-09-01 场景登记：离职在途工单处置策略、部门撤销 × tickets.org_path 级联完整性）。**2026-10-08：附件/存储件收编为 Phase 5 S5 实施载体**（SSOT=[phase5/README](../phase5/README.md) §P5-S5，迁移号启动时按 §4 对账占用——原预留 000034 已被 P4-3 字典占用，000058 = is_system / 000059 = message_rebind / 000060 = message_rules_version / 000061 = message_rules FK（v3 审计 Y-3），当前下一编号 000082（000079 = action_notify_seed 2026-10-26；000080 = storage_usage_alert 种子，C-6 观测批 2026-10-27；000081 = settings_reset_bind，设置页走查批 2026-11-03））；auth-enhance 维持本窗口触发驱动 |
| IW3 | ~~BK-18：类型/字段/模板管理闭环~~ | ✅ **后端已实施（2026-08-31）**：迁移 000018 + 7 管理端点 + G2 校验 + TestBK18×2；前端照 12-frontend 施工（另排期） |
| IW4 | ~~行级过滤护栏（fail-closed）~~（2026-09-01 go-wind-admin 调研吸收） | ✅ **已实施（2026-09-01）**：`resource.Filter.Unscoped` 显式豁免（admin bypass / ticket_scope=all 两处显式化）+ `ticket_repo.List` 入口 fail-closed 哨兵（无谓词且未豁免 → 报错，漏接 L2 从静默全量变测试期报错）+ `TestGuard_TicketRepoListCallSites` AST 守护（repo.List 调用点锁定 ticket 包）+ 测试 4 个；全门禁绿（lint / 13 包单测+集成 `-race` / acceptance 27+66+26+32 FAIL=0） |

> **随手项（任意时点）**：BK-9（测试死代码清理）、A6 ③（错误注入测试用例）、09 F-31④（relation 越权负向用例——现有 `TestD9_CreateRelation` 只覆盖正向/同向 409/删后 404）、09 F-32（audit/user service 分支级单测，低优——集成已兜底核心分支）、可选 Q5 组织赋角注记（11-authz §5 Q5 不变量补一句「组织赋角（org_roles/parent_id）使 token 快照原理上不可行」，doc-only，2026-09-01 登记）。**新增（2026-09-08）：在线用户管理面（触发驱动；触发条件 = 出现强制下线/会话审计运维诉求——安全事件响应或账号共享治理，或 M-UAA 上线引入多端会话管理；两条件均未出现前不动，2026-09-09 补写触发条件对齐「暂缓 ≠ 搁置」纪律）**——会话吊销原语全现成（`user:disabled` + SCAN `refresh:<uid>:*` + `blacklist:at:<jti>`，session_revoke.go / auth_service.go:326），缺的仅管理端点：基础版 GET/DELETE `/monitor/online-users` + 权限码 seed，0.5–1 天；按设备精确强退需 RT value 改 JSON 存 AT jti（+1 天）。**新增（2026-09-11 审查修正批，文档体系低优治理）**：① **ADR 覆盖不足**——现仅 3 篇（事件机制/Asynq/activelist 集成），建议补 5–8 篇重大不可逆决策（三层鉴权、ltree 选型、双 Token + 黑名单、迁移编号治理、ticket_visibility、虚拟组）；② **体量偏重**——`docs/` 约 2.6 万行 > 代码 2.3 万行，`design/architecture.md`（1,899 行）/ `design-decisions.md`（1,414 行）建议按职责拆分（对齐 standards §4「单文件 >200 行评估拆分」精神）；③ **RT 重用检测的多标签误伤**（触发驱动；触发条件 = 出现多标签体验问题的真实反馈）——届时评估**宽限窗口** / **family_id** 方案；未触发前不修（注意「先 GET 比对、命中才 DEL」会削弱盗用检测，须权衡）。详见 `docs/phase1/02-auth.md` §RT 轮换流程 / 本文件 §6 RT-1。
