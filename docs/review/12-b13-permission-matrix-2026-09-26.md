# 12 · B13 权限覆盖矩阵（point-in-time 审计）

> **文档定位**：全端点 × 三层鉴权（L1 Casbin / L2 行级 / L3 服务内）逐行对账——计划拍板「带日期戳 point-in-time 审计，不背活文档包袱」（02 §6 #3）。基于 W1 词表重排（000031）后的终态；下一份矩阵的合理时点=下个大版本（或词表再动时）。
>
> **基准**：2026-09-26，分支 phase4（565317b）。数据源=子代理全量勘察（79 用户面端点+3 机器面+网关），锚点均 file:line 实核。
>
> **三层模型速查**：L1=路由级 Casbin（menu_apis→(path,method) 策略；W1 后页面=GET、写=按钮）；L2=行级（resource.Registry，**现仅 ticket 注册**）；L3=服务内判定（委托/档位/守卫）。SelfService=L1 豁免标记（登录即过，L3 管辖）。

---

## 1. 矩阵总览（12 域 79+端点）

### 域 1 匿名 auth（2）

| 端点 | L1 | L3 | 注记 |
|---|---|---|---|
| POST /auth/login | 公开 | 登录锁+device_id 白名单 | catalogExempt |
| POST /auth/refresh | 公开 | RT 轮换三态+纪元 | catalogExempt |

### 域 2 selfService（6）

| 端点 | L1 | L3 | 注记 |
|---|---|---|---|
| POST /auth/logout | SelfService | device_id 必填（W0b） | exempt |
| POST /auth/password/update | SelfService | 旧密码+device_id **可空**（不对称注记） | exempt；20007 唯一放行口 |
| GET/POST /user/profile(/update) | SelfService | 仅自身+patch 限个人字段 | exempt |
| GET /user/menus·/permissions | SelfService | 本人角色；admin 全量 B4-4 | exempt |

### 域 3 orgDelegated（9，全 L3 判定）

| 端点 | L3 判定 | menu_apis |
|---|---|---|
| POST /orgs/delete | 全局∨（虚拟组∧admin/owner）+BK-20 未结工单守卫 | 装饰性行 |
| POST /orgs/members | ensureCanManage+scope=all 全局+W0b 守卫 | 装饰性行 |
| POST /orgs/members/role | ensureCanManage+owner 目标拒改 | 装饰性行 |
| POST /orgs/members/scope | ensureCanManage+scope=all 全局（BK-14） | **无行** |
| GET /orgs/roles/list | 全局∨admin/owner | **无行** |
| POST /orgs/roles/bind | **仅全局**（BK-12） | **无行** |
| POST /orgs/roles/delete | 仅全局 | **无行** |
| POST /orgs/owners | 全局∨owner 档+owner 互护 | 装饰性行 |
| POST /orgs/members/delete | ensureCanManage | 装饰性行 |

装饰性=该行在 catalogExempt 内不参与 enforce（绑定仅为 BK-22 双向对账存在）。

### 域 4 users（10）

| 端点 | L1（000031 后） | L2 | L3 |
|---|---|---|---|
| GET /users | 页面 | **无** | 超管影子过滤 |
| POST /users | create 按钮 | — | canAssignRole |
| GET /users/:id | 页面 | 无 | ensureVisible |
| POST /users/update | update 按钮 | — | ensureCanManage |
| POST /users/delete | delete 按钮 | — | ensureCanManage+自删拒+末位超管 |
| POST /users/status | status 按钮 | — | 同上族 |
| POST /users/roles | assign_role 按钮 | — | ensureCanManage+canAssignRole |
| POST /users/password/reset | reset_pwd 按钮 | — | ensureCanManage+会话吊销 |
| POST /users/orgs | assign_org 按钮 | — | **仅 ensureCanManage（无 org 侧校验——盲区 #8）** |
| GET /users/:id/orgs | 页面 | 无 | ensureVisible |

### 域 5 roles（8）：页面 GET×4/按钮写×4；L3=ensureCanManageRole（B1-2）+F-2 优先级+BK-12 父链；读端点超管影子（B2-6）。

### 域 6 orgs 明面（6）

| 端点 | L1 | L3 | 注记 |
|---|---|---|---|
| GET /orgs | 页面 | **无** | 盲区 #3：全量树 |
| POST /orgs | create 按钮 | 无鉴权类 | |
| GET /orgs/:id | 页面 | 无 | |
| POST /orgs/update | update 按钮 | **仅 IsSystem 拒** | **不对称注记：无委托路径（盲区 #7）** |
| POST /orgs/move | move 按钮 | 环/父校验（repo） | |
| GET /orgs/:id/members | 页面 | **无** | 盲区 #3：任意 org 名录 |

### 域 7 menus/audit（4）

| 端点 | L1 | L2/L3 | 注记 |
|---|---|---|---|
| GET /menus（树/:id） | 页面 | 无 | W1 只读化：写接口已删 |
| GET /audit/logs | 页面 | **无行级（参数≠行级）** | **盲区 #1：勾页=全平台审计** |

### 域 8 tasks 代理（10）

全组纯 L1 透传（taskrunner_service.go:111 自述）——L1 绑定：GET /tasks/:id·/runs·/dead-letters=页面；POST /tasks=submit 按钮；cancel/retry=operate 按钮（000031 新）；**GET/POST /jobs×4=manage 按钮**（/jobs* 整体管理面，000031 特殊重排）。**盲区 #4：读端点无 per-user 过滤**（共享队列口径已拍板，params 敏感约定=E-⑤ 触发项）。

### 域 9 tickets（12）——L2 唯一完整域

每端点 authorizeCheck(action)→resource.go 三轴（属主∨锚点∨scope∨委托）→canOperate 白名单（update=创建人/close=处理人/assign=主管/delete=admin/note=创建人·处理人/comment=可见者）；列表=GetFilter SQL 行级（IW4 哨兵+AST 守护）；Create 的归属约束（IsInOrgBranch）在 **L3** 非 L2；侧信道修复（relations 鉴权先行，W0b）。

### 域 10 ticketMeta（11）

4 GET=ticket_list+ticket_type_manage 双页绑定（000031 ④）；7 写=ticket_type_write_btn（000030 POST 化）。L2/L3 无（全局配置，设计内）。

### 域 11 机器面（3）

探针 2（无鉴权，设计内）+ /internal/jobs/callback（AK/SK+Enabled 开关+release SK 弱值拒绝+**W0b 快照一致性三防线**）。

### 域 12 网关 /al

JWT→限流→审计(skip body)→Casbin(keyMatch2 含 :typeName/:id 模板)→身份断言覆盖→AK/SK 出站。L1=000024+000031 重排后（读=页面/写=按钮）；行级属 activelist 侧（v1 边界口径）。

---

## 2. 设计内豁免清单（17 项）

catalog.go:40-58 全量：auth 2 公开+selfService 6+orgDelegated 9。网关前缀整体双向豁免（catalog.go:107-115）。

## 3. 盲区与不对称登记（8 项，按敏感度排序）

| # | 端点面 | 现状 | 定性 |
|---|---|---|---|
| 1 | GET /audit/logs | 参数过滤≠行级 | **产品语义决策**：审计=全局视图（当前 L1 已限 admin）vs 行级（按 org/user 过滤）——建议维持现状+前端提示「审计为全局视图」 |
| 2 | GET /users 三读 | 无 org 行级 | 同上定性：用户管理=管理面全局视图（L1 页面=admin 勾选） |
| 3 | GET /orgs 树+/:id/members | 全量 | 组织树全量可见=设计内（组织结构非机密；成员名录稍敏感——**W3「我的组织」名册端点已规划 L3 判定**，管理面名录维持 L1） |
| 4 | tasks 读 3 端点 | 纯 L1 | **已拍板共享队列口径**（02 §5.5 行 20）；params 敏感=E-⑤ 触发项 |
| 5 | roles 读 3 端点 | 仅超管影子 | 设计内 |
| 6 | ticketMeta 4 GET | L1-only | 低敏感设计内 |
| 7 | POST /orgs/update·/orgs（create） | 无委托校验 | **真不对称**（对照 delegated delete 的 L3）——定性：明面组=全局管理语义（L1 按钮 org:update/create 即闸门），委托组=组内自治语义；**非漏挂**但矩阵注记 |
| 8 | POST /users/orgs | 仅目标用户档位校验 | **与 ticket Create W0b 防线不同构**——L1 按钮（user:assign_org）即唯一闸门；建议下批补 org 侧校验（对照 IsInOrgBranch 形态）或显式定性「分配组织=全局管理动作」 |

另两处注记：UpdatePassword device_id 可空 vs Logout 必填（**不对称——建议下批对齐**）；users/roles 超管影子过滤=L3 效果但不在 Registry 体系。

## 4. BK-21 覆盖范围确认

运行期哨兵仅 ticket_repo.List（IW4）；AST 守护仅扫 ticketRepo.List 接收者。**users/audit/orgs 的 repo.List 无等价防线**——泛化已登记触发式（00 §2.4 BK-21 行：触发=首新资源接 L2/导出）。B13 结论：**当前 L2 仅 ticket 一域，泛化未到触发时点**。

## 5. IAM 平面边界清单（§26.1 输入）

- **身份平面**（认证）：auth 2+selfService 6=8 端点，全 exempt
- **管理平面**（IAM 自管）：users/roles/menus/orgs 明面=34 端点，L1 按钮闸门+L3 档位守卫
- **业务平面**：tickets 12+ticketMeta 11+tasks 10+audit 1=34 端点
- **机器平面**：/internal callback+网关 /al
- 交付物语义（§26.1 拆分讨论的输入）：管理平面自足（不依赖业务平面端点）；业务平面依赖管理平面的角色/组织原语。

---

*产出方法：2026-09-26 子代理全量勘察（router.go 79 端点逐个对照三层代码锚点+迁移后 L1 绑定）+ 盲区定性回填。本档为 point-in-time 快照，词表再动时重生。*
