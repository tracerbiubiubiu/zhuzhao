# 02 · Phase 4 实施计划（草案 v1）

> **文档定位**：Phase 4 首个计划文档——主轴拍板落笔、批次排期、编号分配、触发项三态标注。素材来源 = [00 号总盘点](./00-planning-inventory.md)；本文只做「定计划」，不重复登记理由（引用 00 号节号）。
>
> **拍板状态**：~~本文整体为草案~~ **✅ 已转正（2026-09-21）**——§6 决策清单 13 项全部终确（1 显式拍板 + 12 按建议整体授权）；§5.2–5.3 计划评审处置同步落档。所有者定性附注：**部分建议并非「最正确」的方式，而是「最适配当前阶段」的方式**（适配单人带宽/内部系统量级/触发驱动纪律），各决策行内以 ⚖ 标注——详见 §6.1。
>
> **基准**：2026-09-21。迁移号 000001–000029 已占用，**下一号 000030**；Phase 3 干净基线（无带入 Wave）。

---

## 1. 主轴拍板（按 00 号 §7 建议组合；✅ 2026-09-21 终确：主动线=前端）

| 线 | 定位 | 理由 |
|----|------|------|
| **① 产品完整线** | **主动线** | 前端 zhuzhao-ui 是唯一无外部前置、运营杠杆最大的件（裸 API 的 IAM 是运营硬伤）；线内其余项（通知/字典/导出/验证码/PAT/MCP）均可独立成批穿插 |
| **④ 技术债清偿线** | **穿插线** | 低成本（各 ≤1–2 天），随间隙插入，含菜单词表只读化批与启动批 |
| **② 上线运维线 + ③ 外部集成线** | **信号线（不排期）** | 全部带真实触发器（上线/多实例/进内网/迁移窗口/HR 对接），提前做=投机；雷达表监听 |
| **⑤ 工单翻案线** | **挂起** | 等 §23 翻案条件；类型可见性策略 S1 设计输入已在 10 号 §10 备好 |

## 2. 批次总览

| Wave | 线 | 内容 | 量级 | 出口标准 |
|------|----|------|------|----------|
| **P4-W0 启动批** | ④+拍板 | 00 §4 四拍板项落地（03 号 D2/D4 定案、B13 定性、scope 枚举映射表回标）+ P2-14 `InsertPolicyEvals` batch_size 上限 + 00 §8.1 三项定性（F-23 审计防篡改重定性 / govulncheck 入 Makefile / 密码历史·邮箱重置归档定性） | ≤2 天 | 四档 acceptance + 全门禁绿 |
| **P4-W1 菜单词表只读化** | ④ | [00 §2.4](./00-planning-inventory.md) 方案 B 全清单：删 3 写接口 + **迁移 000030+000031 两连号（十一批拆分：30=BK-18 整改、31=词表重排+按钮补种+绑定，序号即执行顺序；以下条目按所属迁移落位）**（**含 menu_apis 三行清理：system_menu × POST /menus·/menus/update·/menus/delete** + **system_menu_create/update/delete 三按钮行删除**（00 §2.4 清单原有、十批复审补登）——残留即孤儿绑定，catalog.go `AuditRouteCatalog` 双向对账启动即报）+ 死码随迁扩容（ErrMenuAlreadyExists 60001/ErrMenuHasChildren 60003/ErrMenuIsSystem 60004 + handler/errors.go 对应 HTTP 映射行 + **docs/api/errcode.md 删行** + **pgerr.go:26 菜单唯一约束映射处置（改通用冲突，防 raw 500）** + **errors_test.go:47 allErrCodes 码表断言同步（len 58→55）**；ErrMenuNotFound 60002 留——AssignMenus 预检在用）+ swag + 文档四处（07-menu 定位改写+契约句改写+**audit:list→audit:read 勘误**/§26.2 注记/11 号矩阵/迁移地图）+ **resource.go:66 注释随手修正**（「路由级已校验 ticket:create」→ B 后语义对齐「路由级=页面/按钮绑定含 POST /tickets」）。**+ B 案词表拆分（2026-09-21 所有者取 B，研究方案细化六点）**：① menu_apis 挂载点重排——页面行只留 GET、写端点挂既有按钮行，ticket/task/al 三域 + system/audit 同批统一（admin/superadmin 策略来自 000002 通配、rbac_service.go:307-312 跳过 menu_apis 路径=零影响——十二批行号勘误；模型统一后任意自定义角色天然支持只读）；② 补缺失按钮（**task_center 实为六写路由**——POST /tasks·/tasks/cancel·/tasks/retry·/jobs·/jobs/update·/jobs/trigger，十批复审补全）：`ticket_relation_btn(ticket:relation)`、`task_operate_btn(task:operate)` 挂 relations 与 cancel/retry；**`POST /tasks`（submit）归既有 `task_submit_btn`**（十二批点名补全，六写路由全数有主：tasks→submit_btn/cancel+retry→operate_btn/jobs 三写→manage_btn）；**/jobs·/jobs/update·/jobs/trigger 三端点显式归 `task_manage_btn`**；**ticket_type_manage 七写路由零按钮（000018 通篇无 type=3 行）——M1 已拍（2026-09-21）：补同码按钮 `ticket_type_write_btn(ticket:type:manage)`（**复用既有 permission 字面值、不新增 permission 定义**——十一批表述精确化；**粒度代价登记**：7 写端点挂一按钮=写权限全有全无，不能只给「改字段」不给「删类型」，内部系统可接受、非既有缺陷**）**——前端 W4/W5 关联/取消/重试按钮渲染同用新码；③ intent-preserving 迁移：非 admin 角色若现有 casbin 含写路由，按新形态补 role_menus 按钮绑定（防重存角色静默降权——AssignMenus 替换语义+每次重算 casbin；当前 operator/viewer 零绑定=实际空转，纯防御；**十批 P0-3：该分支无既有覆盖，acceptance 构造含写路由自定义角色验证一遍，或首次真实触发先 dry-run**）；④ 07-menu 契约句改写：「**页面菜单=该页读 API，写 API 挂对应按钮行**」（F-3 注释引用的原文同步改；模型复原=页面 GET+按钮写，F-3 本意「勾全即能写」保持——勾页面+全部按钮仍得全 API）；⑤ 不回退 F-3 已删过滤、不触发 catalog 拦截（AuditRouteCatalog 只对账路由↔menu_apis 集合、不校验 menu_type，catalog.go:95）；⑥ 规模修正（十一批）：**换挂 ≈47 行**（system 17/ticket 15=8 写+7 类型写/task 7=六写+GET /jobs/al 8）**+4 个元数据 GET 双页绑定**（GET /ticket-types·/:code/fields·/ticket-templates·/:code 在 ticket_list **保留** + ticket_type_manage 页面行**复制**绑定（menu_apis M:N 天然支持，casbin 按 (role,path,method) 去重）——**移走会断 operator 发起表单的 schema 拉取，只增不删**）；**GET /jobs 一并归 `task_manage_btn`**（「/jobs\* 管理面除外」语义完整性——只勾页面的角色不得见 job 定义，task_center 页面只留 GET /tasks/:id·/runs·/dead-letters）；ticket_type_manage 页 B 后 menu_apis=4 共享 GET（**绑页=仅导航与元数据读，写权限须绑按钮——推荐分配预设须体现「绑页+绑按钮」**）；**000031 内嵌「路由→按钮」逐行对照表（SQL 注释），down 精确还原，验收断言按表逐行核；对照表注记：system_org 组 POST /orgs/delete·/orgs/members·/orgs/members/delete 三行同时在 catalogExempt（SelfService 跳 Casbin）——换挂后生成的 casbin 规则永不参与 enforce，绑定仅声明用、真实边界在 L3（十二批装饰性注记）**。**+ role_menus 绑定缺口修复（五批复审 P0）**：admin/superadmin 五菜单绑定 + acceptance 断言。**+ 验收账号与权限基座（原「验收种子」，2026-09-21 拍板 P1+研究方案修正+十批 H1 采纳，十一批改名）**：**测试账号不进 migrations/（迁移落包括生产在内的每个环境，flag=false 已知凭证是比 admin123 更低的安全水位）**——E2E/Playwright setup 幂等建号：admin 登录后 POST /users 创建 operator/viewer 两个 `must_change_password=false` 账号（重复运行幂等）；**admin 凭据闭环：首跑经强制改密改到固定测试密码（存 E2E env）后续复用——改密即轮换，不闭环则二次运行登录失败**；运行时调 POST /roles/menus 分配（先例=phase2a:179），断言注意防抹种子；尊重 000002:11-12 角色描述既有契约（「默认未绑定权限，需管理员显式分配（只读菜单）后生效」——D2-45/#25 自然保持绿；且 viewer 描述本就承诺只读菜单=B 案兑现种子承诺）；「viewer=业务只读/operator=业务读写（/jobs* 管理面除外）」作为**推荐分配预设**写入 W3 文档（**十二批补三新按钮归属：`ticket:relation`→operator（关联工单=常规协作动作，与 comment/note 同类）；`task:operate`→operator（operator 提交的任务应能取消/失败重试；jobs 管理面仍经 /jobs* 归 task:manage 挡在 admin）；`ticket:type:write`→admin 专属——不补则 operator 在 W4 看不到关联按钮、W5 看不到取消/重试按钮，联调时易误判前端 bug**）；权限语义（所有者口径）：审计+system 域 admin/superadmin 专属，operator/viewer 均不绑。**+ BK-18 方法整改（单仓直接改，2026-09-21 拍板）**：5 端点 PUT/DELETE→POST zhuzhao 风格，**旧→新路径对照（十一批补全，替换原「等」字）**：`PUT /ticket-types/:code → POST /ticket-types/update`（code 入 body）、`DELETE /ticket-types/:code → POST /ticket-types/delete`、`PUT /ticket-types/:code/fields → POST /ticket-types/fields/replace`（整组替换语义，非 update）、`PUT /ticket-templates/:code → POST /ticket-templates/update`、`DELETE /ticket-templates/:code → POST /ticket-templates/delete`；+ menu_apis **api_path+api_method 双同步** + Go 集成测试更新 + swag + **standards §3-5 豁免条目删除**。**⚠ 同 path 跨 method 迁移红线（十二批新发现，唯一有安全后果的一处）**：`/api/v1/ticket-types/:code/fields` 等三条 path 在 menu_apis 存在跨页多行（000010:144 GET 绑 ticket_list=operator 元数据只读 / 000018:22 PUT 绑类型页=管理写）——**迁移 UPDATE 一律按 `(api_path, api_method)` 复合条件定位**，漏 method 条件会把 operator 的 GET 改成 POST 写权限（静默提权且 AuditRouteCatalog 不报错——改后 path 仍有路由，表象只是「operator 多了奇怪权限」）；出口加反向断言：**ticket_list 必须仍持 4 条 GET 且不得持有 fields/replace 等写端点**。**迁移拆分（十一批采纳）：BK-18 URL/method 整改=000030，词表重排+按钮补种+role_menus 绑定=000031——migrate 按序号天然保证「先整改后重排」的钉死顺序，down 粒度减半；IW2 附件批按 A2 让位从 000032 起（规则本就覆盖）** | **4–5 天**（十一批核算上调，原 2.5–3 未含后续扩容：删接口 0.5+两迁移含对照表与 down 还原 1.5–2+BK-18 测试 swag 文档 1+intent/死码五处 1+出口验收 0.5–1） | 四档 acceptance+全门禁绿 + **部署原子性（H2）：migrate-up 与新二进制同一步连续执行——catalog 对账启动期 fail-fast 拒启（wire_gen.go:135），顺序错乱窗口内重启即宕机；运行中旧进程不重启不受影响（gate 只在启动期跑）** + BK-22 启动对账平 + 新 5 菜单 /user/menus(admin) 可见性对账平 + **新增只读回归断言：运行时给 viewer 绑 ticket_list 页 → GET /tickets=200 且 POST /tickets=403**（T7 反转增强）+ **operator 正向断言（十二批补）：operator 持 ticket:relation/task:operate 码且经其端点 200；/jobs* 仍 403** |
| **P4-W2 前端壳层** | ① | [01 号 §8](./01-frontend-design.md)：底座模板裁剪（**vue-element-plus-admin v3 种子拷入，备胎 pure-admin-thin——01 号 §9.1 定案**，首日锁版本跑通构建=栈兼容验证）+ 壳层四件自写 + 登录登出 + **首登强制改密流（20007 gate + profile/改密页）** + **个人中心页（profile 自服务：资料编辑+自愿改密入口，01 §8-W2 十一批显式化——十二批 02 行同步）** + 菜单渲染 + 动态路由 + 三范例页 + **首页工作台（简单仪表盘——2026-09-21 拍板；首版=状态统计+最近工单零后端改动，`assignee=me` 参数随 P4-W4 补后点亮待办/已办）** + 明暗主题切换（01 §9-4 拍板）+ **首日项：跨三仓 compose 编排+external network 预建脚本（本仓栈仅 pg/pgbackup/migrate/redis/app，activelist/taskrunner 在各自仓——十批 P1-4 修正口径；固定 compose project name/端口段，防双栈网络别名冲突致门禁假失败的事故复发）+ 12-frontend 回标清单（§4 views/§5 FE 口径随实际落定回写）** | 1–1.5 周 | FE3（viewer：业务只读可见/管理面+审计不可见）+ 强制改密流 E2E + 前端 E2E 冒烟绿（前端门禁首跑；**E2E 打标准三栈 compose，不用 stub——2026-09-21 拍板 P2**） |
| **P4-W3 前端 system 域** | ① | 用户/角色（AssignMenus 勾选树，**el-tree 必须 `check-strictly=true`——级联勾选会让「只勾页面、不勾按钮」的只读授权选不出来，B 案 UI 前提；「viewer 只读/operator 读写（/jobs* 除外）」作为推荐分配预设写入角色页文档**）/菜单（只读树+角色分配）/组织——**两面拆分（2026-09-21 拍板 P0-2）**：**管理面**=system_org 菜单（admin 专属）：全组织树 CRUD/move/`ticket_visibility`（仅 update 表单）；**自服务面**=「我的组织」静态路由页（01 §3.2④ 机制，任何登录用户可达）：普通成员只读组织信息、owner/admin 经 L3 判定见成员管理/组内赋权/owner 控件——委托九端点 SelfService 刻意跳 Casbin（router.go:163），静态页是非 admin 委托者唯一可达入口 + **委托组成员名册查询端点**（GET /orgs/members/list SelfService 版，L3 判定 owner/admin 可读——现成员列表挂 biz 组需 Casbin，owner 页内只见角色不见人，~0.5 天随批；**响应必须含 org_member_role+ticket_scope——现 /orgs/:id/members 返回裸 model.User 无此二字段，「我的组织」组内角色列拿不到数据（十三批场景走查，[03 号 §8](./03-scenarios-and-tests.md)）**；**catalogExempt 逐行登记+理由注释——catalog.go:40 是精确匹配 map（形似通配实为逐条），新 /api/v1 路由无登记即 missing_binding 拒启，十批 P1-a；含 Go 集成测试，后端件照走后端门禁**） | 1–2 周+0.5–1 天 | FE2 口径（管理全流程无 SQL）；API 权限即时生效 + 导航重登刷新；owner 账号过「我的组织」委托操作 E2E |
| **P4-W4 前端 ticket 域** | ① | 工单发起（form-create 动态表单，菜单外静态路由）/列表/详情（静态路由）/评论/备注/关联（按钮码 `ticket:relation` 随 B 案补种）+ 处理动作=close/assign/update/**delete**（九批复审补 ticket:delete；后端无 approve 端点，审批随翻案批）+ 类型/字段/模板管理三件套（历史 PUT/DELETE 已随 W1 整改为 POST）+ **assignee=me 列表查询参数（后端件：TicketListQuery 加字段+repo 条件+集成测试 ~0.5 天——点亮 W2 工作台待办/已办，十批 P1-c）** + **处理人/创建人姓名回填（后端随批二选一：列表响应回填姓名 或 加批量反查端点——现响应是裸用户 ID 且无批量反查，列表页处理人列=N+1 请求，十三批场景走查）** | 2–3 周 | FE1 + FE2 + 新路由菜单种子同 PR 对账 |
| **P4-W5 前端 al/task/audit 域** | ① | 名单页（`/al/api/v1` 反代：types/data，cursor 分页；**含导出（blob 流式旁路）/导入入口——九批复审补**）/任务中心（tasks/runs/死信/jobs 页内 Tab；**死信只读**，重试走 `POST /tasks/retry`——取消/重试按钮挂 `task:operate` 码，B 案补种；**按钮禁用态按后端前置态渲染：取消仅 pending、重试仅 failed/dead（409 透传），jobs 无 DELETE，trigger 对 enabled=false 409（十三批 S13）**；**死信 Tab 亦无 total（`{list,page_size}`）——无 total 形态适用面=al 域+死信（十三批）**；jobs Tab 保留——cron 收归后 taskrunner 执行职责不变，服务手动触发与未来定时任务）/审计日志 | ~1 周 | 权限码可见性验证 + 页面清单与菜单种子对账（豁免静态路由清单） |
| **P4-9 前端部署件** | ① | nginx 静态容器 + 反代（`/api` 与 `/al/api` 精确 location，其余 fallback index.html——前端 history 路由 `/al/data` 与反代前缀同居 `/al`）+ 三栈 compose 接入 + 后端 `trusted_proxies` 启用（现为空=不信任代理，反代后 ClientIP 全变 nginx IP→限流桶全站共享）。**排位：P4-W5 收尾或紧随（十批小项 1 补）** | 1–2 天 | 冒烟经 nginx 全链绿 |
| **穿插池** | ①④ | P4-2 通知（本体含渠道配置持久化+管理 API；†配置页触发驱动后补——2026-09-21 拍板：配置能力现在加、页面后加）→ P4-3 字典（†system 域管理页）→ P4-4 导出（†前端导出交互，与 BK-21 同批）→ P4-7 验证码（前端登录插槽已在 01 §6 预留）→ P4-8 P2 小件（†panic 聚合/指标/对账极简查询页——2026-09-21 拍板要 UI，**随 P4-W5 audit 域同批交付**；候选追加：**工单聚合统计端点**——工作台首版按状态多拉 total 是权宜，终态化登记触发驱动，十批低优 2；登录日志独立表（与业务审计分表差异化留存，P2 遗留建议）一并入候选）→ P4-6 PAT（†个人中心管理页）（**P4-5 MCP 读侧已提排至 P4-W1/W2 并行窗口——2026-09-21 拍板 P2-7：纯后端不抢契约面，把快照/AuditRouteCatalog/权限码表暴露为只读 MCP tool，直接缓解单人带宽这一唯一高风险项**）；附件（IW2）独立窗口，启动先拍三决策点（**含补附件前端入口规格**——12-frontend 现无此件）；**④线漏排四行同池（2026-09-21 拍板 P5 分件处置）**：**cron 调度收归评估+实施**（**1.5–2 天**含 taskrunner 跨仓退役——七批复审估时修正：zhuzhao 侧先行可交付，taskrunner 侧 job 定义退役并行尾随不阻塞；**退役 smoke checklist（taskrunner 无等价 acceptance 门禁，十批 P1-6）；/jobs* 端点终态冻结=保留（W1 重排前置条件）**；头位——所有者质疑成立：审计数据与归档内务全在 zhuzhao 本地，闹钟外置 taskrunner 属既成依赖；方向=收归进程内 ticker + PG advisory lock 单实例锁 + 配置开关，taskrunner 侧 job 定义退役）/文档治理批（~1 天，**提前：P4-W1 收口后立即执行，不等穿插间隙**——00 §6 回标搭 W1 便车，ADR 补篇+两大文档拆分随本批；**子项全清单见 00 §2.4 文档治理行（含 S-2 策略库标注/S-6 版本滞后/phase2-00:361 更正——十批复审补指针）**）/随手项打包（≤1 天，尾位；角色复制按钮随 P4-W3 角色页）/activelist 契约整改②（**先不改，挪穿插池末位**，P4 收尾前豁免登记或整改二选一）+ **B13 执行本体**（0.5–1 天矩阵——**不走间隙规则：W1 收口后立即执行（与文档治理批同窗），W2 启动前必须完成**；W0 仅定性，七批复审指认「穿插池定位 vs W2 前硬时限」自相矛盾，已消解）。④线各件加「P4 收尾前必须清」软时限。†=含前端联动面，随本批同步交付（页面工作量已在量级内），**交付时点不早于其依赖的前端基座 Wave：字典页≥W3、PAT 页≥W2（profile 页扩展）、导出交互≥W5、P4-8 UI=W5——九批复审钉死，替换原「随最近 Wave」歧义表述** | 各 0.5–5 天 | 每批独立过门禁 |

**排位理由**：菜单只读化（P4-W1）压在前端 P4-W3（system 域角色分配页消费 `GET /menus`）之前，保证前端只见终态接口，避免按将废接口开发；前端 P4-W2–W5 为主动线主体，串行推进（单人假设）；穿插池项均无相互依赖，随前端 Wave 间隙插入。

## 3. 触发项三态标注（00 号 §3 雷达表定计划口径）

| 态 | 项 |
|----|----|
| **主动做**（进 Wave/穿插池） | 前端全部（P4-W2–W5 + P4-9 部署件）、菜单只读化（P4-W1）、govulncheck/P2-14/§8.1 定性（P4-W0）、BK-21（随 P4-4 导出批）、登录验证码（P4-7，兼上线前置属性）、**④线漏排四行**（cron 调度评估/文档治理批/随手项打包/activelist 契约整改②，归穿插池） |
| **持续监听**（信号线，不排期） | ② 全部（B7 CORS/B9 Watcher/M1 基座/L1 缓存/HA 三件/决策清单三项）、③ 全部（M-SSO/M-Mig/M-HR/taskrunner 后置项/PG 备份口径）、**主审计切异步（判定日志量超预期时；启用须补停机 drain）**、RT-1 衍生、runbook §4 四条、在线用户管理面、ReBAC 触发表、MFA/密码合规、E-⑤/E-⑥、密钥管理面 |
| **明确不做**（本期显式关闭） | 配置热重载（00 §2.2 建议标不做）、微前端/SSR/移动端（01 号 §1；~~i18n~~ 已升格可选项随 §5.4 定案，十批复审勘误）、Keycloak-Ory 替换（§26.1 既有）、OPA 迁移（既有）、通用流程引擎（16 号红线）、**运行时菜单 CRUD**（P4-W1 关闭；重建触发=多租户自定义菜单，已登记 00 §3 信号组 C） |

## 4. 编号分配

- **新 namespace `P4-`**（现有 W/IW/BK/AB/F/C/U/RT/CC/HC/MC/EC/OP/TC/P0/并发-Px 全占用）。⚠ 与外审 P0/P1、代码审查 P2 系列区分：`P4-`=Phase 4 立项项，`P0/P1/P2-`=审查发现编号，引用写全前缀防混。
- **P4 立项项分配**：

| 编号 | 项 | 量级 | 排位 |
|------|----|------|------|
| P4-1 | 菜单词表只读化（方案 B+词表拆分+BK-18 整改 000030+词表重排 000031 两连号+验收账号基座） | 4–5 天 | P4-W1 |
| P4-2 | 通知通道（webhook 优先；渠道接口配置化=00 §2.1 既有口径，**含配置持久化+管理 API 面（无页面），配置页触发驱动后补——2026-09-21 拍板**；死信告警不依赖工单解封；**触发源注意（十三批场景走查）：ticket_events 无读 API、无 processed 列（000010 注释承诺的 Phase 3 迁移未实施）、无发射代码——实施须补事件消费面（轮询或 emit hook）+ 消费位迁移（占号随批）**） | 2–4 天 | 穿插池 |
| P4-3 | 字典/系统参数（只做业务枚举/运维参数，不碰权限策略面） | 2–3 天 | 穿插池 |
| P4-4 | 用户面导出（excelize 固定列；**必须接 L2**，与 BK-21 同批） | 1–2 天+BK-21 | 穿插池 |
| P4-5 | MCP server 读侧（AI 代运营；透传鉴权模式） | 3–5 天 | **提排 P4-W1/W2 并行窗口（2026-09-21 拍板 P2-7，原穿插池后位）** |
| P4-6 | 个人 API token PAT（与 09 号 api_keys 管理面合并规划；勿抄 gva signing key 方案） | 2–3 天 | 穿插池后位 |
| P4-7 | 登录验证码（Redis store 必须；随上线前置） | ~1 天 | 穿插池 |
| P4-8 | P2 小件打包（审计响应体/panic 落库+聚合查询/只读对账端点/指标采集；**含极简查询 UI——2026-09-21 所有者拍板要 UI**，随 P4-W5 同批交付） | 2–3 天 | 穿插池 |

- **既有编号随批沿用**：IW2（附件）、BK-21、B13、P2-14、RT-1 等。
- **P4-9**：前端部署件（§2 批次表）。④线漏排四行沿用 00 §2.4 行名**不新增编号**（评估/纯文档/随手打包类，无跨文档引用诉求）。
- **迁移号**：000030 起。**P4-W1 占 000030（BK-18 整改）+000031（词表重排）两连号（十一批拆分）**；**IW2 附件批让位从 000032 起、P4-2 notification_configs 顺延 000033（A2=谁先启动谁占用，先启动者取 32，十二批分列消歧）**。

## 5. 门禁纪律（不变）

每 Wave/穿插批交付前：后端批跑全四档 acceptance + 全门禁绿（AGENTS.md 口径）；前端批按 [01 号 §7](./01-frontend-design.md) 前端门禁（vue-tsc/ESLint/Vitest/Playwright 冒烟）单独口径，两套门禁分仓各跑互不掺和。文档同步与 11 号回填随各批交付。

## 5.1 Phase 4 完成定义与风险登记（2026-09-21 计划评审补）

**完成定义（阶段级出口，超出 wave 级）**：

1. P4-W2–W5 四波全绿（**FE1–FE3** 通过 + 前端门禁稳定运行；~~FE1–FE4~~ FE4 审批链路随 §1 主轴⑤翻案批复活时验收——载体页审批操作/审批人配置在翻案线，本期无 Wave 认领）；
2. 穿插池完成 ≥5 件（**池内件=①线 P4-2/3/4/6/7/8 六件+④线四行+B13（P4-5 已提排 W1/W2 窗口）**；至少通知/字典/导出/验证码/P2 小件落地）；
3. 触发信号雷达表（00 号 §3）复核一次——三态标注随季度实际情况重审；
4. 量化目标三条：**FE1–FE3 全绿；管理全流程无 SQL（用户/角色/菜单/组织/工单类型配置全走 UI）；死信告警通道上线**（P4-2 交付）。

**风险登记**：

| 风险 | 缓解 |
|------|------|
| 底座模板栈过新（Vite 8/vue-router 5/Pinia 4 插件兼容） | P4-W2 首日锁版本跑通构建=一次性验证；备胎 pure-admin-thin 同机制切换（01 §9.1） |
| 单人带宽（季度级前端 + 穿插池串行） | 穿插池全部可让路（见 §2 排位理由）；隐性缓冲显式化（**十一批再修：P4-1 上调 4–5 天+文档治理/B13 同窗，前置段 ≈1.7–2 周，总量 ≈9.7–12.5 周，对 13 周季度缓冲 ≈4–25%；P4-5 MCP 提排占约 0.5–1 周缓冲（单人「并行」实耗日历）——穿插项让路优先级：完成定义硬线五件 > P4-6/P4-8 > 其余**），P4-W4 深水期误判延期时不砍质量砍穿插件 |
| P4-W4（ticket 域）form-create 设计器深水区（自定义字段拖拽/校验联动复杂度） | P4-W4 首日先做渲染器（只读回显）再上设计器；设计器复杂度超预期时降级为 JSON 源码模式（12-frontend §3.2 本就有此兜底形态） |
| 穿插批与前端波并行的契约冲突 | **穿插批只增端点不改既有契约**；P4-W1 已把菜单契约冻结在前端动工前 |

**分支策略（对齐 standards §12.7）**：规划文档批合回 main 后，`phase4` 长命分支退役；实施期每 Wave 开自己的短分支（如 `p4-w1-menu-readonly`），合入即删。

## 5.2 计划评审落档（2026-09-21，业界对照检查）

> 评审结论：骨架成立；2 项内部不一致 + 6 项业界对照缺失项，处置如下（§5.3）。§6 决策清单 13 项已于同日全部终确（§6 表 + §6.1 权宜标注）。

### 5.3 评审发现处置表

| # | 发现 | 处置 | 归落 |
|---|------|------|------|
| 1 | 长命 phase4 分支 vs §12.7 短命分支公约冲突 | **采纳修复**：规划文档合 main 后 phase4 分支退役，每 Wave 短分支 | §5.1 分支策略 |
| 2 | al 域 keyset 游标分页被 ProTable 偏移分页假设漏掉（含 cursor `+` urlencode 实锤坑） | **采纳修复**：ProTable 双分页模式（offset=zhuzhao / cursor=al），urlencode 在 al 适配层内解决 | 01 号 §5 修订 |
| 3 | 前后端契约类型手工同步（业界标准=OpenAPI→TS 生成，契约漂移编译期拦截） | **采纳补上**：P4-W2 壳层件之一（`make swag && pnpm codegen` 链，openapi-typescript 或 orval，~半天） | 01 号 §3.3/§7 + P4-W2 |
| 4 | zhuzhao-ui 仓引导裸奔（无 AGENTS.md/README/LICENSE/版本锁定） | **采纳补上**：P4-W2 首日 repo scaffolding（前端版 AGENTS.md 含变更评审三节适配） | 01 号 §7 + P4-W2 |
| 5 | 前端供应链扫描缺口（npm 侧零防护，与 govulncheck 四仓不对称） | **采纳补上**：`pnpm audit` 入前端门禁；renovate 可选 | 01 号 §7 |
| 6 | 阶段级出口 + 风险登记缺失 | **采纳补上**：本节 §5.1 | §5.1 |
| 7 | 缓冲未显式化 | **采纳**：§5.1 风险表单人带宽行注记 | §5.1 |
| 8 | 量化目标缺失 | **采纳**：完成定义第 4 条（三条，不搞 OKR 全家桶） | §5.1 |
| 9 | Wave 制 vs sprint 制 | **维持**：单人+AI 串行、outcome 驱动 wave + 出口标准，业界对小团队恰是不用多人 sprint 仪式 | — |
| 10 | 前端错误上报（Sentry/GlitchTip 类） | **触发驱动**：01 号 errorHandler 已兜底，外部可访问或用户规模上来再接 | 01 号 §6 注记 |
| 11 | Storybook/视觉回归/浏览器兼容矩阵 | **明确不做**：内部管理系统过重 | 01 号 §1「明确不做」 |
| 12 | i18n | **升格为可选项**：所有者 2026-09-21 提出加回——见 §5.4 定案 | §5.4 |

### 5.4 i18n 决策（2026-09-21 所有者提出，当日定案）

**结论：P4-W2 壳层期不引入 i18n；P4-W3（system 域）完成后视进度决策（默认做，落 P4-W4 前间隙或穿插池）。** 理由与方式：

- **为什么现在不做**：① 已选底座 vue-element-plus-admin 的 meta.title 走 i18n key（如 `router.dashboard`）——但其 locale 文件仅 2×103 行、耦合浅（评估结论），**剥离比全量接入便宜**，留到接入时处理正合适，现在做是给壳层换血期添变量；② 菜单 `name` 是 DB 中文种子（000002 等），页面名来自后端——i18n 要么菜单树加多语列（动后端+迁移），要么前端对 DB 值做 key 映射（**推荐后者**，零后端改动）；
- **接入方式（到时执行）**：vue-i18n + locale JSON 按域分文件（`locales/system.json`、`locales/ticket.json`）；DB 值（菜单名/错误码文案/枚举名）经**集中映射层**（`common/i18n/dbKeys.ts`，key=后端原值中文，缺 key 回退原值）——后端零改动，映射缺口=渲染原值，不会破功能；日期/数字格式经 dayjs locale；
- **业界对照**：管理端 i18n 业界常态是「框架能力预留 + 按需启用」——Element Plus/vue-i18n 均原生支持，晚接不付惩罚性成本；反之过早接入会拖慢每个页面（所有文案过 key）。内部单语系统先落地功能、确认真实多语诉求（外部协作者/英文用户出现）再启用，是合理排序而非欠账；
- **触发检查点**：P4-W3（system 域）收口时看进度——正常或有余量 → P4-W4 前间隙接入（量约 1–2 天）；P4-W4 深水 → 推迟到穿插池（不阻塞完成定义）。
- **P4-W2 现场二选一注记（2026-09-21 前端复查补）**：既然默认要接 i18n，「P4-W2 剥离模板 i18n + P4-W3 后重建自有体系」存在双重工作——若 P4-W2 实看代码确认模板 locale 耦合确实浅（评估结论 13 文件 $t / 2×103 行），可改为**保留骨架、只替换 locale 内容为自有 zh 包**（省一道剥离工；DB 菜单名不经 $t 直渲染，映射层方案不变）。两案 P4-W2 拿到代码后定，本文档不预锁。

## 5.5 复审落档（2026-09-21，第二轮四批评审收敛）

> 功能缺口/契约细节/内部矛盾/动态路由四批评审 + 计划漏排复查，全部经代码核验后落位：01 号正文就地修（其 §11 留追溯表），本表只录**计划层**处置。同日另一批「编号错位 C1」在终确批已修复不重录。

| # | 发现 | 处置 | 归落 |
|---|------|------|------|
| 1 | 00 §2.4 技术债线四行漏排（cron 调度评估/文档治理批/随手项打包/activelist 契约整改②）——§1 称④为穿插线但 §2 穿插池只装①线七件，且 §3 三态未标（违背本 doc「杜绝暂缓≠搁置歧义」原则） | 批次表穿插池补行归位；§3 三态补标「主动做」 | §2/§3 |
| 2 | FE4 审批链路无 Wave 认领，完成定义「FE1–FE4 全绿」无法闭环（审批载体页随主轴⑤挂起） | 完成定义改 FE1–FE3，FE4 标随翻案批 | §5.1 |
| 3 | 前端部署件（nginx）无编号无波次——三仓 compose 零先例，且含两个实坑：`trusted_proxies` 空=反代后限流桶全站共享；前端路由 `/al/data` 与反代 `/al/api` 同居 `/al` 需 location 精确分流 | 立项 **P4-9**（1–2 天，W5 收尾或紧随） | §2/§4 |
| 4 | 穿插池七件的前端联动面未登记（字典页/PAT 页/导出交互等，量级疑似只算后端）；P4-5 MCP 读侧无前端面、P4-7 已有插槽 | 穿插池行加 † 标注；IW2 启动前置补「附件前端入口规格」 | §2 |
| 5 | §3 持续监听清单漏「主审计切异步」（信号组 D 残余，D4 拍板进 W0 后剩余触发项） | 补行 | §3 |
| 6 | W2–W5 范围与出口措辞校准（强制改密流 W2/组织委托面 W3/静态路由+处理动作语义 W4/死信只读+对账豁免 W5） | 随 01 号 §8 修订同步批次表 | §2 |
| 7 | **新菜单缺 role_menus 绑定（后端缺陷，五批复审最高优先，五方向交叉确认）**：000018/22/24/25 只插 menus+menu_apis 零 role_menus；GetUserMenus 经 ListByRoleIDs INNER JOIN 且无 admin 旁路（GetUserPermissions 有 B4-4=不对称，admin「有 route: 码无路由」）；acceptance 只断言过 ticket 11 个绑定（phase2a:111-121）——ticket_type_manage/task_center/al_data/al_types/audit_log 连 admin 都不可见，W4/W5 页面全不可达 | **并入 P4-W1 迁移同批**（~~000030~~ **000031——十二批勘误：role_menus 绑定属词表重排不属 BK-18 整改**）：补 admin/superadmin 五菜单绑定 + acceptance 断言；W1 出口扩写「含新 5 菜单 /user/menus(admin) 可见性对账平」 | §2 P4-W1 |
| 8 | 401 需分码：20002=过期（可静默刷新）vs 20003=签名错/黑名单（须跳登录，B2-1）——01 §3.3 原把 401 笼统走刷新；al 导出为裸 JSON 数组流旁路不走信封；device_id 白名单 `[a-zA-Z0-9_-]{1,64}`；constantRoutes 常量路由组未定义 | 01 号 §3.2⑤/§3.3 就地修 | 01 §3.2/§3.3 |
| 9 | B13 执行本体无批次承载（W0 只写「定性」，§6 #3 说 P4-W2 前执行 0.5–1 天矩阵） | 归穿插池带硬时限 | §2 |
| 10 | 七批复审（代码级精度 6 处）：① W1 行未显式提 menu_apis 三行清理——**000002:114/116/117**（113/115 为 GET 保留行，十批勘误）孤儿绑定→`AuditRouteCatalog`（catalog.go:95）双向对账启动报错（00 §2.4 改动清单原有、行内压缩丢失）② viewer/operator 只插 role_menus 不够——Casbin 策略为存储行（roleRepo.AssignMenus 服务路径同事务生成），种子须 role_menus+casbin_rule 两表同插，否则菜单可见 API 全 403 ③ P4-2 迁移号未分配 ④ B13「穿插池」定位与「W2 前完成」自相矛盾 ⑤ P4-8 UI「随最近 Wave」歧义 ⑥ cron 估时未含 taskrunner 跨仓退役 | 就地修：W1 行显式 menu_apis 清理+两表同插口径（元组格式 (role::code,path,method)，非按钮码）；§4 补 P4-2 号按 A2 取（~~当前 000031~~ **十二批勘误：W1 占 30/31 后 IW2=32/P4-2=33 分列**）；B13 改「W1 收口即执行、不走间隙」；P4-8 UI 钉死 W5；cron 调 1.5–2 天 | §2/§4 |
| 11 | 第八/九批 P0 与豁免复核拍板（2026-09-21）：**P0-2** 委托 UI 对非 admin 委托者不可达（SelfService 刻意跳 Casbin vs system 域 admin 专属）→「我的组织」静态自服务页两面拆分 + 补委托组成员名册 SelfService 端点；**BK-18 PUT/DELETE** → 所有者定规则「多仓先豁免、单仓直接改」，核实单仓涉及 → 随 W1 整改（5 端点改 POST+menu_apis method 同步+standards §3-5 删豁免）；**P0-1** 只读语义=B 案（写端点 menu_apis 挂既有按钮行——评审 A 案孪生页解不了 operator/task_center 提权），已向所有者澄清「读写区分语义三层有两层、唯独 L1 未承载，B=焊接既有语义」**（P0-1 已由本表 #12 当日定案取 B，本行「待最终确认」字样作废——十批勘误）** | §2 W1/W3 |
| 12 | **P0-1 定案取 B（2026-09-21 所有者确认，研究方案六点细化全部采信）**：① 挂载点重排 ticket/task/al+system/audit 同批统一（admin 通配零影响）② 补两行缺失按钮 `ticket_relation_btn`/`task_operate_btn`（POST /tickets/relations、/tasks/cancel·retry 原无按钮可挂——修正我方「种子现成」误判）③ intent-preserving casbin 迁移（防 AssignMenus 替换语义重存角色静默降权）④ 验收种子修正：只种两个测试账号**不种角色绑定**（尊重 000002:11-12「默认未绑定」描述契约，D2-45/#25 保持绿；E2E 运行时 POST /roles/menus 分配，先例 phase2a:179）——viewer 种子描述本就承诺「只读菜单」=B 兑现种子承诺，D 案出局的规范级理由 ⑤ T7 反转为只读回归断言（GET 200/POST 403）⑥ 07-menu 契约句改写「页面菜单=该页读 API，写 API 挂对应按钮行」；W3 硬要求 el-tree `check-strictly=true`（级联勾选=只读授权选不出来） | §2 W1/W3/W4/W5 + 01 §8 |
| 12-a | 十批复审（跨仓与验收工程化，全部采信）：H1 测试账号出迁移（生产库弱凭证风险——改 Playwright setup 幂等建号，~~待所有者确认~~（已由 #13 拍板落地））/H2 部署原子性（catalog fail-fast）/M1+P0-1 ticket_type_manage 七写零按钮+task 六写清单补全（M1 二选一~~待拍~~（已由 #13 拍板落地推荐项））/M4 死码两引用点（pgerr.go:26+errors_test.go:47）/P1-a catalogExempt 精确匹配/P1-c assignee=me 后端承载/P1-4 compose 表述打架（01 错 02 对）/P1-5 缓冲算术/P1-6 taskrunner 退役无门禁补 smoke/低优 ~~44 行~~（**47 行，十一批已改 W1 行，此处同步**）对照表+聚合统计端点登记触发/文档治理子项全清单指针 | §2 W1/W2/W3/W4/穿插池 + §5.1 + 01 §1/§5/§11 |
| 13 | **十批三项拍板落地（2026-09-21 所有者「按建议」）**：**H1 采纳**——测试账号不进 migrations/，E2E setup 幂等建号（admin→POST /users 建 operator/viewer flag=false）+运行时 /roles/menus 分配；**M1 采纳推荐**——ticket_type_manage 补同码按钮 `ticket_type_write_btn(ticket:type:manage)`；**P2-7 采纳**——P4-5 MCP 读侧提排至 P4-W1/W2 并行窗口（单人带宽直接缓解）。§5.5 12-a 行「待确认/待拍」字样就此作废 | §2 W1/穿插池/§4 |
| 14 | **十一批终验勘误与补强（2026-09-21，全部核验后落）**：①高危——01 §3.4 `button:{code}` 改 `button:{permission}`（menu_service.go「button:"+m.Permission」，照 menus.code 拼会按钮全不渲染且酷似正常无权限）；② 语义两点——GET /jobs 一并归 task_manage_btn（「/jobs* 除外」完整性）+4 元数据 GET 双页绑定（ticket_list 保留+类型页复制，移走断 operator 发起表单）；③ 迁移拆分——000030（BK-18 整改）/000031（词表重排）两连号，序号即顺序、down 粒度减半，IW2/P4-2 按 A2 让位 32 起；④ P4-1 量级上调 4–5 天+§5.1 缓冲再修（4–25%+MCP 占用注记）；⑤ 旧→新路径五行对照补全（fields/replace 命名钉死）；⑥ 杂项——验收种子改名「验收账号与权限基座」/admin 凭据幂等闭环（首跑改密到固定测试密码）/M1「零新码」表述精确化+粒度代价登记/W2 补个人中心页/§3.2「四特例+路由三层」计数勘误/01 §11 compose 残留与 #10–14 指针（十二批勘误：#14 自述与 01 §11 实际落位不符）/00 §2.4 三新按钮/§5.5 行序重排（12-a 划改）。**P4-9 排位：评审提出时确实缺失、十一批终验时已补（十二批措辞修正，原「误报」定性不准）** | §2/§4/§5.1 + 01 §3.2/§3.4/§7/§8/§11 + 00 §2.4 |
| 15 | **十二批迁移安全与回溯勘误（2026-09-21，全部核验后落）**：① **同 path 跨 method 迁移红线**——/ticket-types/:code/fields 等三 path 在 menu_apis 跨页多行（000010:144 GET vs 000018:22 PUT），UPDATE 漏 method 条件会把 operator 的 GET 改成写权限（静默提权，AuditRouteCatalog 抓不到——改后 path 仍有路由）→ 迁移 SQL 一律 (api_path, api_method) 复合定位 + 出口反向断言（ticket_list 仍持 4 GET 且无写端点）；② 迁移号回溯四处勘误（#7→000031/#10→32·33 分列/00:203 附件号→000032/01 §5 死码→000031）；③ operator 补 ticket:relation+task:operate 两码归属（预设段+出口正向断言——不补则 W4/W5 按钮不渲染易误判前端 bug）；④ 六写路由全点名（POST /tasks 归 submit_btn）；⑤ 豁免行装饰性注记（catalogExempt 三行绑定不参与 enforce）；⑥ rbac 行号 306-310→307-312；⑦ #14 自述勘误（#10–14）+「误报」措辞改「已补」；⑧ 00 §2.4 争号句作废+对照表归 000031；B13 口径注记（不走间隙、W1 后即执行，非池内间隙件——完成定义「池内件」计数不含 B13） | §2 W1/§2 W2/§4 + 01 §5/§8 + 00 §2.4/§6 |
| 16 | **十三批场景级走查（2026-09-21，双勘察代理实核，产出 [03 号](./03-scenarios-and-tests.md) 场景×测试矩阵）**：8 个新缺口全部回写——①工单响应裸用户 ID 无批量反查（列表页 N+1）→W4 后端随批姓名回填或批量端点；②P4-2 触发源 ticket_events 无读 API 无 processed 列（000010 注释承诺的 Phase 3 迁移未实施）→实施补消费面+迁移；③组织成员列表无组内角色/scope 字段→W3 名册端点必含；④taskrunner 死信无 total→无 total UI 扩面；⑤cancel 仅 pending/retry 仅 failed·dead/jobs 无 DELETE→W5 禁用态依据；⑥deprecate 不可逆→危险确认；⑦导入双上限 1GiB+30s→上传提示；⑧工单三态 API 不可达→流转 UI 仅三种。测试覆盖：18 场景×（既有 acceptance/Go ∪ 新增 W1 断言+Playwright spec）矩阵成文 | 03 号全文 + §2 W3/W4/W5 + §4 P4-2 + 01 §5/§8 |

## 6. 决策清单（✅ 2026-09-21 全部终确）

> ⚖ 标记 = 该建议属「最适配当前」而非「理论上最优」——所有者终确时的定性附注（§头部引）。带 ⚖ 的行附「更正确的方式」与未选原因，避免将来误把权宜当定论重审时无从对照；触发条件命中时优先按 ⚖ 列升级。

| # | 决策 | 终确结论 | 状态 |
|---|------|----------|------|
| 1 | 主轴组合 | ①主动（前端）+ ④穿插 + ②③信号 + ⑤挂起 | ✅ 终确（显式拍板：主动线=前端） |
| 2 | W0（P4-W0）启动批清单 | 四拍板项 + P2-14 + §8.1 三定性，≤2 天 | ✅ 终确（按建议） |
| 3 | B13 权限覆盖矩阵审计 | **做**（0.5–1 天 doc-only；P4-W2 前执行；定位=带日期戳 point-in-time 审计，不背活文档包袱） | ✅ 终确（按建议）⚖ 更正确=矩阵由自动化持续保障（IAM Access Analyzer 形态）；未选原因=一次性人工审计对单人内部系统性价比最高，持续保障已由 BK-22 盖 L1 面 |
| 4 | F-23 审计防篡改 | **显式关闭 + 信任边界声明**（防 DBA 级篡改不在威胁模型；具象触发=等保 2.0 三级；AL5 归档「导出删行」与触发器禁删冲突佐证须触发后认真设计） | ✅ 终确（按建议）⚖ 更正确=hash 链/WORM 完整防篡改；未选原因=内部系统无合规要求时收益不抵复杂度，且选型依赖合规级别未知 |
| 5 | P4- namespace 与编号分配 | 采纳（P4-1…P4-8，附件沿用 IW2；**P4-9 前端部署件为后续扩编——十批复审补登**） | ✅ 终确（按建议） |
| 6 | 菜单只读化排位 | P4-W1（前端 system 域之前，契约先冻结） | ✅ 终确（按建议） |
| 7 | 穿插池顺序 | 通知→字典→导出(+BK-21)→验证码→P2 小件→PAT→MCP；默认序非死排期，真实信号可插队；附件 IW2 独立窗口 | ✅ 终确（按建议） |
| 8 | 前端 P4-W2 启动时点 | P4-W1 收口即启；穿插池随间隙插入不让路 | ✅ 终确（按建议） |
| 9 | D4 判定日志口径 | 默认全开 + 拒绝永不采样 + 超量对成功判定降采样 | ✅ 终确（按建议） |
| 10 | govulncheck 范围 | 四仓全做；`make vulncheck` 进 AGENTS.md 交付前验证清单 | ✅ 终确（按建议）⚖ 更正确=CI 自动化扫描+renovate 全套；未选原因=无 CI 平台现状下本地门禁已闭环，CI 信号线触发后再升级 |
| 11 | 密码历史/邮箱重置 | **移挂起**（密码历史触发=合规/客户问卷明确要求，NIST 800-63B 反向注记；邮箱重置触发=SSO 未覆盖的外部协作者账号成规模） | ✅ 终确（按建议）⚖ 更正确（若合规要求）=密码历史+泄露库校验（haveibeenpwned/k-anonymity 形态）；未选原因=NIST 800-63B 现行建议明确不推荐历史防重用，无要求不反向加码 |
| 12 | P2-14 修法 | 启动期按列数反推上限断言（防呆，半天）；CopyFrom 改造登记触发项（触发=写入成瓶颈） | ✅ 终确（按建议）⚖ 更正确=直接 CopyFrom（PG COPY 无参数上限，高吞吐标准形态）；未选原因=当前量级用不上，触发驱动纪律优先 |
| 13 | D2 fail-open | 维持 fail-open；补丢弃计数观测（drop counter + WARN） | ✅ 终确（按建议）⚖ 更正确=fail-close+持久队列（零丢失观测）；未选原因=观测管道反噬业务路径是业界大忌，判定日志丢几条不损鉴权正确性 |

### 6.1 ⚖ 权宜标注总览（「最适配当前」vs「更正确的方式」）

> 所有者 2026-09-21 终确定性：部分建议并非最正确、而是最适配当前（单人带宽/内部系统量级/触发驱动纪律）。本节集中列出全部 ⚖ 项，**触发条件命中时优先按右列升级，勿把权宜当定论**：

| # | 当前采纳（适配） | 更正确的方式 | 升级触发 |
|---|------------------|--------------|----------|
| 3 | 一次性人工矩阵审计 | 自动化持续权限保障（Access Analyzer 形态） | 端点数量增长到人工对账不经济，或出现多维护者 |
| 4 | 显式关闭+信任边界 | hash 链/WORM 防篡改 | 等保三级/合规审计明确要求 |
| 10 | 本地 make vulncheck | CI 全自动扫描+依赖更新机器人 | 有 CI 平台（信号线触发） |
| 11 | 挂起不动 | 密码历史+泄露库校验 | 合规/客户问卷明确要求 |
| 12 | 启动断言防呆 | pgx.CopyFrom 写入 | 判定日志量级使批量插入成瓶颈 |
| 13 | fail-open+丢弃计数 | fail-close+持久化队列零丢失 | 观测数据被用于合规取证场景 |
