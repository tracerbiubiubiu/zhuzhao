# 03 · 用户场景走查与测试覆盖矩阵

> **文档定位**：Phase 4 计划的多轮终验产出（2026-09-21，十三批=场景级走查轮）。上承 [00 总盘点](./00-planning-inventory.md)/[01 前端设计](./01-frontend-design.md)/[02 实施计划](./02-implementation-plan.md)——本文不重复其内容，补两块此前缺位的：**① 按「用户真实使用旅程」逐场景核对数据形状与交互约束（本轮两路代码勘察实核）**；**② 场景→测试用例覆盖矩阵**（既有门禁断言 vs 需新增断言，消灭「有场景无用例」）。本轮新发现的 8 个缺口处置见 §4（已同步回 01/02 号）。
>
> **Phase 4 一句话定位**：**核心 = 前端 zhuzhao-ui 落地（裸 API 的 IAM 是运营硬伤）+ 后端模块的能力增强、补全与修复**（菜单词表只读化/B 案按钮授权拆分/BK-18 方法整改/通知/字典/导出/验证码/PAT/MCP/P2 小件/cron 收归等技术债穿插）。

---

## 1. 角色与登录场景（S1–S2）

| # | 场景 | 走查要点（代码实核） | 约束/坑 |
|---|------|---------------------|---------|
| S1 | 首登强制改密（admin 种子 E000001/admin123） | 登录响应 `must_change_password=true` → 守卫跳改密页；期间全部业务端点 403+20007（jwt.go:93），仅 `/auth/password/update` 放行；**改密成功即轮换**（返回新 TokenPair，旧 AT 进黑名单），**改密请求须带 device_id**（漏传落 "default" 错槽互踢） | 改密后凭据复用闭环：E2E 首跑改到固定测试密码存 env |
| S2 | 登录→工作台 | 登录带 `device_id`（浏览器级 UUID，白名单 `[a-zA-Z0-9_-]{1,64}`）；三件并行 profile+menus+permissions；redirect 第一个可见菜单=/home；工作台首版=状态统计卡（按 status 各拉一次 total，权宜）+最近工单列表 | `assignee=me` 参数随 W4 补后点亮待办/已办；工单 6 态=open/assigned/in_progress/pending_verify/closed/rejected（state_machine.go，**类型可配勿写死**） |

## 2. 管理场景（S3–S7，admin/superadmin）

| # | 场景 | 走查要点 | 约束/坑 |
|---|------|----------|---------|
| S3 | 用户管理 | 搜索=**username 模糊（ILIKE）+employee_no 精确+role 精确+status**（user_handler.go:24-44）；**无 org 过滤、无 real_name/email 搜索**——按组织看人走组织成员页 | 搜索区只放真支持的参数（01 §5「不得假搜索」）；重置密码/状态启停/角色分配/组织分配均有对应端点 |
| S4 | 角色管理+AssignMenus | 勾选树 **el-tree check-strictly=true**（B 案前提）；「viewer 只读/operator 读写」推荐预设写进页面文档 | AssignMenus 替换语义——重存会按当前勾选全量重算 casbin（intent-preserving 已防旧角色降权） |
| S5 | 菜单管理（只读树） | W1 后无写接口，纯展示+角色分配入口 | — |
| S6 | 组织管理（admin 面） | 树 CRUD/move/`ticket_visibility`（**仅 update 表单、仅实体组可配**，虚拟组 400）；子节点/成员占用预检 | org 删除守卫不查 org_roles（已知残留，读侧已挡） |
| S7 | 我的组织（owner 面，静态路由） | **⚠ 本轮新发现：现成员列表响应是裸 `model.User`，不含组内角色（owner/admin/member）与 ticket_scope 字段**（org_request.go:82-88）——「我的组织」页要展示「组内角色」列拿不到数据 | **W3 新增的名册端点必须返回 org_member_role+ticket_scope**（已回写 02 §2-W3） |

## 3. 工单场景（S8–S12，operator 为主）

| # | 场景 | 走查要点 | 约束/坑 |
|---|------|----------|---------|
| S8 | 工单发起 | form-create 渲染 `GET /ticket-types/:code/fields` schema（**4 元数据 GET 双页绑定保 operator 可拉**——十二批红线）；priority 1–4 越界 400 | 模板卡片消费 `GET /ticket-templates` |
| S9 | 工单列表 | 过滤仅 `type_code/status/priority`（**无 keyword/日期/org**）；**⚠ 新发现：`assigned_to/created_by` 是裸用户 ID 且无批量用户名反查端点**——「处理人」列需 N+1 `GET /users/:id` | **W4 后端随批件：工单列表响应回填处理人/创建人姓名（或加批量反查端点，二选一）**（已回写 02 §2-W4） |
| S10 | 工单详情 | 评论公开/备注内部（**内部备注仅创建人/处理人/admin 可见**，service.go:493-509 服务端过滤，前端同权限分层渲染）；关联正反向判重 409/自关联 400 | Close/Assign 成功**返回 OK(c,nil) 无 body**——前端靠 query invalidation 刷新，勿依赖响应体 |
| S11 | 工单处理 | 动作=assign（open→assigned/取消分派→open）/close（过状态机，非法转换 400+90002；已关 409+90004）/update（closed 拒 409）；**in_progress/pending_verify/rejected 三态经 API 不可达**（B3 端点在翻案线）——状态筛选下拉可列 6 态，流转按钮只有分派/取消/关闭 | 状态机从 ticket_types.transitions JSONB 构建，**前端勿写死转换图**（类型可配） |
| S12 | 类型配置三件套 | W1 整改后全 POST（update/delete/fields/replace 五路径对照在 02 §2-W1）；写端点挂 `ticket_type_write_btn` | ticket_type_manage 页 B 后=4 共享 GET（绑页=导航+元数据读，写须绑按钮） |

## 4. 任务/名单/审计场景（S13–S15）

| # | 场景 | 走查要点 | 约束/坑 |
|---|------|----------|---------|
| S13 | 任务中心（页内 Tab） | 提交（params ≤64KB、timeout 0/1–86400；**callback_url 随 W0 拒收——前端一律不传，传非空 400，十八批对齐**）；**取消仅 pending 可用（否则 409）、重试仅 failed/dead 可用**——按钮禁用态按 status 渲染（task_service.go:290-379）；runs 筛选=request_id/action/status/job_id/dept[]/from/to(RFC3339) | **⚠ 新发现：死信列表 `{list,page_size}` 无 total（asynq 游标拿不到总数）**——死信 Tab 也是无 total 形态（01 §5 cursor UI 形态适用面扩大到 taskrunner 死信）；死信行不带 request_id/job_id，详情需按 task_id 二次查；**jobs 无 DELETE**（任务定义页无删除按钮）；trigger 对 enabled=false 报 409 |
| S14 | 名单页 | types/data 两页；data 行=schema-less JSONB（列渲染按 Definition.fields：name/type/required/sensitive）；**软删=status 两态（无 deleted_at），列表默认排除软删行**；restore 幂等；**deprecate 不可逆**（UI 须危险确认：废弃后名称永久保留、拒演进/插入、存量可查可导出） | cursor 分页（无 total 上一页/下一页）；**导入硬上限双约束：1GiB 字节 + 30s ReadTimeout**（前端大文件须提示分批/联系管理员调参）；导出=裸 JSON 数组 blob 旁路 |
| S15 | 审计查询 | 过滤=path/user_id/employee_no/start/end（日期 `2006-01-02`，start>end 400）；**无 method/status_code/keyword 过滤** | 审计列=id/username/method/path/status_code/duration_ms/ip/user_agent/request_body/request_id/created_at |

## 5. 会话边界场景（S16–S18）

| # | 场景 | 走查要点 |
|---|------|----------|
| S16 | 会话边界 | 401 分码（20002 过期→静默刷新 / 20003 无效→跳登录）；5xx（503+10008）**不清会话**拒绝挂起提示重试；账号锁定=**429**（勿入 401 分支）；多标签登出 BroadcastChannel 主动同步；刷新失败码族 20004/20014/20015 |

> **十六批安全审计补充（2026-09-22）**：S13 口径已拍（2026-09-22）：共享队列全可见+「只看我提交的」筛选（W5 随批 submitted_by 过滤）；S1 登出链路修复后补负向断言（空 device_id 登出=400）；SSRF 修复后 Submit 恶意 callback_url=400 负向测试归 W0。
| S17 | viewer 只读（FE3） | 运行时绑 ticket_list 页 **+ticket_read_btn 读按钮（十四批：详情静态路由入口按 `button:ticket:read` 放行，只勾页面进不了详情）** → GET /tickets=200 且 POST=403（T7 反转断言）；管理面+审计菜单不可见 |
| S18 | operator 正向 | 持 `ticket:relation`/`task:operate` 码且经其端点 200；/jobs\* 仍 403（十二批新增断言；**夹具前置：setup 先 submit 一任务使处 pending、建两张工单再断言——十四批**） |

---

## 6. 场景→测试用例覆盖矩阵

> 既有覆盖 = 四档 acceptance（phase1/2a/2b/2c）+ Go 单测/集成（`make test-unit`/`test-integration`）；新增 = W1 acceptance 断言批 + Playwright E2E（W2 起建，标准三栈）。**原则：服务端行为 Go/acceptance 盖，前端渲染与交互 Playwright 盖，契约形状 codegen 编译期盖。**

| 场景 | 既有覆盖 | 新增覆盖 | 落点 |
|------|----------|----------|------|
| S1 强制改密 | Go：auth 集成（20007 拦截已有单测） | Playwright：首登→gate 跳转→改密→新 TokenPair 生效→登出重登用新密码；二次运行凭据复用 | W2 E2E |
| S2 工作台 | — | Playwright：登录→菜单渲染→状态卡出数→最近工单列表 | W2 冒烟集 |
| S3 用户管理 | acceptance phase1（CRUD/搜索断言） | Playwright：搜索（username 模糊命中/employee_no 精确/无 org 过滤项不出现）+分页+按钮权限槽 | W3 |
| S4 角色分配 | acceptance（AssignMenus/绑定断言）+ **W1 新增：只勾页不勾按钮→写端点 403（intent-preserving 用自定义角色）** | Playwright：check-strictly 勾选（只勾页面可保存）+ 角色变更后 API 即时生效/导航重登刷新 | W1 acceptance + W3 E2E |
| S5 菜单只读 | **W1 新增：菜单页无写按钮+三写接口 404** | Playwright：只读树渲染 | W1+W3 |
| S6 组织管理 | acceptance（org CRUD/move 预检） | Playwright：树操作+ticket_visibility 仅 update 表单出现 | W3 |
| S7 我的组织 | —（新端点） | **Go 集成：名册端点 L3 判定（owner 可读/普通成员拒/含 org_member_role+ticket_scope 字段）**；Playwright：owner 见控件/成员只读 | W3 |
| S8 发起 | Go：Create 校验（priority 越界/type 停用 90003） | Playwright：模板卡片→form-create 渲染→提交成功（FE1） | W4 |
| S9 列表 | Go：List 过滤+L2 身份可见 | **Go：assignee=me 参数（W4 新增）**；Playwright：筛选组合+处理人列显示姓名（W4 后端回填后） | W4 |
| S10 详情/评论 | Go：评论分层可见性（现有集成）+relation 判重 | Playwright：备注分层渲染（viewer 不可见内部备注）+ 关联操作挂 relation 码 | W4 |
| S11 处理 | Go：状态机（90002/90004）+ close 带 comment | Playwright：分派/关闭流+非法转换按钮禁用或报错呈现 | W4 |
| S12 类型三件套 | Go：BK-18 集成（**W1 随整改更新为新 POST 路径**） | Playwright：三件套全流程无 SQL（FE2） | W4 |
| S13 任务中心 | taskrunner 自有测试（cancel/retry 前置态） | Playwright：取消（仅 pending 可点）/重试（仅 failed/dead）/死信查看→跳转任务详情；**zhuzhao 代理 409 透传断言** | W5 |
| S14 名单 | activelist 自有测试（cursor/deprecate/导入） | Playwright：cursor 翻页（无 total 形态）/软删恢复流/deprecate 危险确认弹窗/导入大文件提示 | W5 |
| S15 审计 | acceptance（审计写入断言） | Playwright：日期过滤+start>end 错误呈现 | W5 |
| S16 会话边界 | Go：401 分码/刷新码族/5xx（现有单测全覆盖） | Playwright：AT 过期静默刷新（等 30min 不现实→mock 时钟或调短 AT TTL 的测试配置）+多标签登出同步 | W2（刷新用单测盖，E2E 盖多标签） |
| S17 viewer 只读 | — | **W1 acceptance：运行时绑页 GET 200/POST 403（T7 反转）**；Playwright：viewer 登录菜单/按钮双不可见 | W1+W2 |
| S18 operator 正向 | — | **W1 acceptance：operator 三码正向+ /jobs\* 403** | W1 |

**覆盖原则补三条**：① Playwright 用例按上表「新增覆盖」列建 spec 清单（预计 ~18 个 spec，W2 建冒烟 4 个、逐 Wave 扩）；② W1 的 acceptance 断言（只读回归/operator 正向/名册 L3/44→47 行对照表逐行核）在四档脚本内加节，**不新建第五档**；③ 契约形状（信封/分页/int64-string）由 codegen 编译期盖，不重复写 E2E。

## 7. 测试基建与门禁口径（汇总）

| 层 | 工具 | 归属仓 | 触发时机 |
|----|------|--------|----------|
| 服务端行为 | Go 单测+集成（-race） | zhuzhao/taskrunner/activelist/utils 各自 | 每后端批（AGENTS.md 口径） |
| 契约与迁移 | 四档 acceptance（含 W1 新增断言节） | zhuzhao | 每后端批门禁 |
| 前端逻辑 | Vitest（composables/动态路由解析/tokenStorage/单飞刷新） | zhuzhao-ui | 每前端批 |
| 端到端 | Playwright（标准三栈，~18 spec 渐进建） | zhuzhao-ui | 每前端 Wave 出口 |
| 供应链 | govulncheck（四仓）/ pnpm audit | 各仓 | 随 lint 同批 |

## 8. 本轮（十三批）新发现缺口处置一览

| # | 缺口 | 处置 | 归落 |
|---|------|------|------|
| 1 | 工单响应 assigned_to/created_by 裸 ID、无批量用户名反查 → 列表页 N+1 | W4 后端随批：列表响应回填姓名**或**批量反查端点（二选一，实施时定） | 02 §2-W4（已回写） |
| 2 | P4-2 触发源：ticket_events **无读 API、无 processed 列**（000010 注释承诺的 Phase 3 迁移未实施）、无 webhook 发射代码 | P4-2 立项时补：事件消费面（轮询 DB 或服务内 emit hook）+ processed/消费位迁移（占号随批） | 02 §4 P4-2（已回写） |
| 3 | 组织成员列表不含组内角色/ticket_scope → 「我的组织」页数据不全 | W3 名册端点响应**必须含 org_member_role+ticket_scope** | 02 §2-W3（已回写） |
| 4 | taskrunner 死信 `{list,page_size}` **无 total** | W5 死信 Tab 按无 total 形态设计；01 §5 cursor UI 适用面=al 域+死信 | 01 §5/02 §2-W5（已回写） |
| 5 | 取消仅 pending/重试仅 failed·dead/jobs 无 DELETE/trigger 对 disabled 409 | W5 按钮禁用态与操作集设计依据（本文 S13）+ Playwright 断言 | 本文+02 §2-W5 |
| 6 | deprecate 不可逆（名称永久占用） | W5 危险确认交互+S14 用例 | 本文 |
| 7 | 导入双上限（1GiB+30s ReadTimeout） | W5 上传交互提示 | 本文 |
| 8 | in_progress/pending_verify/rejected 三态 API 不可达（B3 在翻案线） | W4 状态筛选列 6 态、流转按钮仅三种——勿按 6 态做流转 UI | 本文 S11+01 §8-W4（已回写） |

---

*产出方法：2026-09-21 十三批=双 Explore 子代理场景级勘察（zhuzhao 状态机/事件面/profile/组织/过滤参数 + taskrunner 死信/cancel·retry 前置/jobs 字段 + activelist 类型/软删/导入导出约束），叠加前十二轮契约核验与业界对照（RBAC 按钮授权=GitHub 两档语义/迁移原子性/DoD/触发驱动雷达）。*
