# B11 · 当前 L1 全角色与全路由矩阵审计（2026-11-06）

> 状态：八轮动态矩阵完成，未发现新增 P0/P1 或菜单授权与 L1 实际放行的差异。审计范围限当前 L1 链与路由分类；业务 handler 使用零值依赖作为到达哨兵，400/500 不代表业务功能通过。本报告为必要正式记录；原始矩阵、探针和过程材料仅本地 `.codex`，不纳管。控制台与会话台账由主会话统一回填。

## 1. 范围与基线

原始审计主仓 `5c03581d5999139a3e8e5ead5972c026868aa181`；本次实跑联合候选 `7389ed79bdf409e466542992d9f1befa0c1a8529` 的独立 `git archive`。`router.New`、catalog、Casbin enforcer/model、中间件、RBACService、RoleRepo 七份 L1 源文件跨两点逐字相同；完整 migrations 亦无差异。JWT 密码纪元和 gateway 413 修复属于联合候选的已审变更，未宣称与原始代码一致。归档 697 份原文件最终逐字对 Git 对象复核一致，新增审计探针仅在忽略目录归档的 `cmd/audit-b11/`。

设计/先例：`docs/review/14-b13-permission-matrix-2026-10-26.md` 全文 1–190（SHA256 `9b45cbcad886b32494442e4f585b764b392628f7fe72a06dde17bcfeea94f3f8`），将其作为 2026-10-26/DB v79 时点先例，不作为当前矩阵证明。

仅新建自有 PostgreSQL 15 + Redis 7 容器（label `audit.owner=ECO-20261104-B11`，随机绑定 127.0.0.1），逐轮重建自有 public schema，完整执行 81 份 up（包含 000002 全种子）。没有修改 dev/test 共享环境、E000001、项目源码或已有工作树索引；测试主体为新建 E81xxxx 专属账号。没有 commit/push/merge/main 操作。

## 2. 验证方法和证据边界

- 真实 `casbin.New` PG adapter 装载生产 `configs/casbin_model.conf`；角色展开走真实 `RBACService.GetRoleCodesByUserID`→`RoleRepo.GetEffectiveRoleCodes`，不 stub 角色 SQL。
- 自定义读、写、AL、父子角色通过真实 `CreateRole`/`AssignMenus` 创建授权。独立期望由菜单授权关系 `role_menus`→`menu_apis` 和设计中的路由分类产生，路径匹配使用探针自己的逐段匹配器；实际判定使用 Casbin/keyMatch2 与真实 router 链。实际 `casbin_rule` 单独存证，不用于期望 oracle。
- 对 `router.New` 的真实路由树执行 Gin 官方 `CreateTestContextOnly` + `HandleContext`。CasbinAuth 只在最终放行时设置 `c[roles]`，据此识别 L1 放行，不以“不是 403”或“500”推定放行；JWT-only 文件链、公开面和签名面使用各自认证/调用方标记。
- 每轮对 admin/viewer/无角色/继承角色的全 170 case 再走 `ServeHTTP`（680 次）交叉核对响应状态，八轮全一致。该交叉核对与明确 roles 标记共同留证。
- `/al` 使用真正本地 HTTP 上游，逐次验证出站 HMAC、请求 ID 与认证的 E81xxxx 操作者，确保客户端伪造的 `X-Operator` 未获签名背书；上游为独立哨兵，不替代 activelist 自身功能/L2 验收。
- `/internal` 两端点、`/compat/echo` 在受控配置显式启用，分别使用有效/缺失/错误签名正反验证；另证明默认关闭时这三面与 `/al` 均 404。未重新认证生产网络隔离/TLS。

锚点（均为 `7389ed79bdf409e466542992d9f1befa0c1a8529`）：`internal/router/router.go` 的 `New`（128；SelfService 212/290/309，files 262，biz 326，gateway 534）；`internal/middleware/casbin.go` 的 `CasbinAuth`（56）；`internal/casbin/enforcer.go` 的 `New`（15）；`internal/repository/role_repo.go` 的 `GetEffectiveRoleCodes`（350）；`internal/router/catalog.go` 的 `CatalogExemptRoutes`（45）、`AuditRouteCatalog`（148）、`AuditPageWriteBindings`（222）；`configs/casbin_model.conf` matcher（18）。

## 3. 当前矩阵与发现

每轮 19 主体 × 170 case = 3230 次矩阵请求；八轮共 **25840** 次，预期差异 **0**。另外共 **5440** 次 ServeHTTP 交叉，差异 **0**。三个签名面每轮有效/缺失/错签名共 9 case 全符合预期；每轮 8 个页面写绑定反向夹具、8 个双向 catalog 漂移夹具均检出，事务回滚恢复。候选正常 catalog 缺口与页面写绑定违例均 0。

当前目录精确构成：97 主仓 Casbin 绑定端点；40 需要角色的 SelfService；1 JWT-only 文件代理；3 公开认证端点；3 根级探针/指标；2 internal HMAC；1 compat HMAC；9 个网关 wildcard 方法；14 个网关具名绑定。实际注册路由 156 条，另外具体展开 14 个网关绑定形成 170 case。

`menu_apis` 为 **124 行、116 唯一(method,path)**。其中五个组织委托写端点还保留菜单绑定，但运行时属 SelfService/L3：delete、members、members/delete、members/role、owners。不能把菜单关联行数当作独立受 Casbin 控制的端点数，也不能把 catalog 豁免当作不鉴权。

主体覆盖：4 种子角色；无角色；operator+viewer；自定义工单读/写；AL 读/写；多角色并集；父角色继承；空自定义角色；停用/软删角色；组织精确节点赋角；不继承父组织角色；已过期组织成员；匿名。全部实际 BFS 角色集合与手工期望一致。

- superadmin/admin 按既有 model bypass 允许全部 97 主仓绑定与 14 AL 绑定；该 L1 通配不能替代业务层影子超管/L2/L3限制。
- 全新种子 operator/viewer 未分配菜单，主仓业务面与 AL 绑定均拒绝，40 条角色自服务均过 L1，符合种子声明；部署/E2E 中已配置的角色菜单不等同于本次 fresh seed。
- 自定义工单读角色允许 12 个主仓绑定（含既有例外 POST transition）；工单写角色仅允许其两项按钮授权。AL 读角色允许 6 项、数据写允许 5 项；其余拒绝。
- 多角色按并集、child→parent 按角色链联合；组织角色仅精确节点、父组织不继承、过期成员不授予角色，停用/软删角色不参与。
- 无角色不能进入 40 条角色自服务/97 主仓业务/14 AL 绑定；文件代理是既定仅 JWT 专链，没有“有角色”要求，该链到达仍不能证明有权限读取任意对象。
- 用户 Bearer（含 admin）不能替代机器 HMAC；缺失/错误签名401，有效签名产生正确 caller 标记。internal 的有效签名+空业务 body 返回400是业务参数拒绝，不是业务执行成功；compat 有效签名200为只读 echo。

未发现本批新增 P0/P1，也没有据本批证据新增权限或扩大授权。14号历史正文仍混用44/45、委托8/10等时点数字，本批按当前枚举解释，导航/措辞债不冒充运行时越权。其余 L2/L3 能力沿用各批独立证据；本批不覆盖所有真实业务输入、PAT/密码处置、生产运行权限快照或生产网络边界。

## 4. 留证与复现

证据根目录：`.codex/audits/ECO-20261104/evidence/ECO-20261104-b11/`。

- `plan.json`：窗口、七份跨点源哈希、设计读取锚点、边界。
- `results.json`：八轮汇总、每轮 exit0/时长、697 文件归档等价和探针 SHA256。
- `stable-01-results/` 至 `stable-08-results/`：每轮完整 `matrix.json`/`matrix.csv`、角色集合、独立菜单期望、实际 Casbin policies、目录、44豁免、124绑定、签名正反、680次交叉、默认禁用404、catalog正反夹具。
- `principal-summary.json`：各主体允许/拒绝数，区分 L1 业务绑定、自服务、gateway。
- `b11-probe.go`、`fixed-archive/cmd/audit-b11/main.go`、`run.py`：精确探针与执行命令；重放须为新自有 PG/Redis 更新资源清单/端口，不连接已清理资源或共享栈。
- 首轮 `run-first.log` 的 org_type 已退役字段是审计夹具错误；第二轮 `run-second.log` 的74个 gateway 到达失败源于 httptest Background ctx.Done=nil 导致 Gin CloseNotify 与 Recorder 不兼容。仅修正探针为带 deadline、可取消的请求 context 后，八轮均绿；失败原证据保留，不作产品发现。
- 本批无源码变更，不因纯审计而重复运行 make acceptance；已有正常门禁事实由 root 联合候选批次报告承载。


## 已核固定源码链接与清理

以下L1文件在原5c与联合7389逐字相同，链接采用已存在的原基线对象，不假定未推送7389已可由GitHub访问。

- [internal/router/router.go:128](https://github.com/tracerbiubiubiu/zhuzhao/blob/5c03581d5999139a3e8e5ead5972c026868aa181/internal/router/router.go#L128) 的 `func New(`。

- [internal/middleware/casbin.go:56](https://github.com/tracerbiubiubiu/zhuzhao/blob/5c03581d5999139a3e8e5ead5972c026868aa181/internal/middleware/casbin.go#L56) 的 `func CasbinAuth(`。

- [internal/casbin/enforcer.go:15](https://github.com/tracerbiubiubiu/zhuzhao/blob/5c03581d5999139a3e8e5ead5972c026868aa181/internal/casbin/enforcer.go#L15) 的 `func New(`。

- [internal/repository/role_repo.go:350](https://github.com/tracerbiubiubiu/zhuzhao/blob/5c03581d5999139a3e8e5ead5972c026868aa181/internal/repository/role_repo.go#L350) 的 `func (r *RoleRepo) GetEffectiveRoleCodes`。

- [internal/router/catalog.go:45](https://github.com/tracerbiubiubiu/zhuzhao/blob/5c03581d5999139a3e8e5ead5972c026868aa181/internal/router/catalog.go#L45) 的 `func CatalogExemptRoutes(`。

- [internal/router/catalog.go:148](https://github.com/tracerbiubiubiu/zhuzhao/blob/5c03581d5999139a3e8e5ead5972c026868aa181/internal/router/catalog.go#L148) 的 `func AuditRouteCatalog(`。

- [internal/router/catalog.go:222](https://github.com/tracerbiubiubiu/zhuzhao/blob/5c03581d5999139a3e8e5ead5972c026868aa181/internal/router/catalog.go#L222) 的 `func AuditPageWriteBindings(`。

自有PG/Redis按精确ID和owner标签核对后删除，cleanup.json确认资源缺失；dev/test常驻环境、业务源码与索引均未改动。主仓7389固定正常全量门禁及前端f104×7389完整41项另批已通过，此处不把它们合算为B11业务执行覆盖。
