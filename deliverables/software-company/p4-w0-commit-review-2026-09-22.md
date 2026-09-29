# p4-w0 提交审查（2026-09-22）

> 范围：`bed4005`(origin/phase4) → `0231e09`(HEAD) 共 8 个 W0 提交 + 工作区未提交改动。
> 方法：逐提交读 diff + 实跑测试 + **`git worktree` 干净态对照**（区分「改动引入」与「既有失败」）。
> 结论速览：**8 个提交本身自洽、自带测试全绿**；问题集中在**未提交的工作区改动**——1 个阻断级 + 4 个次级。

---

## 一、已提交的 8 个提交：逐条裁决

| 提交 | 内容 | 裁决 | 证据 |
|---|---|---|---|
| `58cbe38` | P0-1 SK 弱值三件套（repoKnownSKs + release 拒绝 + compose `${VAR:?}`） | ✅ 成立 | `config/validate.go` 清单+判定；compose 三处无兜底；debug 放行保本地 |
| `69fccd8` | P0-3 AddMember 补 `scope=all` 全局守卫 | ✅ 成立 | 与 `SetMemberScope` 同纪律；`TestDelegation_AddMemberScopeAllGuard` 绿 |
| `6d03b3f` | P0-4 迁移 `000022` down 修复（补 3 按钮四段清理） | ✅ 成立 | 照 `000010` down 范式；role_menus→menu_apis→按钮→页面顺序正确 |
| `c9cbc95` | wire 收口（仅 Makefile 注释 + 计划文档） | ✅ 无风险 | 纯文档/注释，无代码路径 |
| `3d34b82` | CreateRelation 鉴权先行修侧信道 | ✅ 成立 | **纯两段位置互换**（`update` 双端鉴权块上移），无权限语义变更；`docs/phase2/09-ticket.md:90` 早已登记「双端 update 取严」 |
| `32987ea` | 登出 `device_id` 必填 | ✅ 成立（有文档缺口感） | `auth_service.go` 空值→`ErrInvalidParams`；负向项已在 `docs/phase4/03:50` 登记 |
| `621c186` | RecordSubmit 记账失败空吞 → `slog.Error` | ✅ 成立 | Submit/Trigger 两处均补 task_id/action/actor 线索，取舍不变 |
| `0231e09` | 回调快照一致性校验（Execute 比对 action/params → 409） | ✅ 实现成立，⚠️ **测试存在同义反复** | 见 §3.2 |

**实跑证据**：`TestD9*` 10/10 绿；`TestJobsCallback*`（service + handler 两包）全绿；`go test ./...` 全量单测绿。

---

## 二、阻断级问题：未提交的 Create 归属守卫让 26 个集成用例全红

**改动**（工作区未提交，`internal/service/ticket/service.go` Create 开头）：

```go
if req.AssignedTo != nil { return nil, errcode.New(..., "assigned_to 不允许在创建时指定…") }
if member, err := s.orgRepo.IsMember(ctx, req.OrgID, actorUserID); err != nil { ... }
else if !member {
    global, gerr := s.delegation.HasOrgManagePermission(ctx, actorUserID)
    ...
    if !global { return nil, errcode.ErrNoPermission }
}
```

**实证（决定性对照）**：

| 状态 | 命令 | 结果 |
|---|---|---|
| HEAD 干净态 | `git worktree add /tmp/zz-head HEAD` → `go test -tags=integration ./internal/service/ticket/` | **`ok`**（全绿） |
| 工作区（含改动） | 同命令 | **26 个 `--- FAIL`** |

失败首因（`TestTicket_R4_InvisibleReturns404`）：

```
authz_resource_integration_test.go:103 创建工单失败：title=B-R4-b1 actor=2
Error: 无权限
```

即 `Create` 直接对既有夹具场景返回 `70001`。失败清单覆盖 R3–R8、T6、B2_*、B2Org_*、BK5、BK11、CustomData_*、Create_*、TC2、BK18、PriorityCheck 等 26 例——**与建单归属无关的用例也被连带打红**。

### 2.1 次级：同一语义在测试套件内自相矛盾（不可同时绿）

- `delegation_authorize_integration_test.go:301`（未提交，新增）：Create 带 `assigned_to` → 断言 `ErrInvalidParams`。
- `b2_core_integration_test.go:132-135`（已提交，未同步）：Create 带 `AssignedTo: &assigned` → 断言 `require.NoError`。

实跑 `TestB2_WriteSeparation`：

```
b2_core_integration_test.go:134: Received unexpected error:
    assigned_to 不允许在创建时指定，请使用分派接口
--- FAIL: TestB2_WriteSeparation
```

### 2.2 次级：`assigned_to` 拒收会让「处理人关闭工单」在默认状态机下不可达

`b2_core_integration_test.go:128-131` 的注释（改动前既有）指出：`Assign` 对 open 单会置 `assigned`，而 **`assigned→closed` 非法**，所以「创建即分派（open+assignee）」是该能力的**唯一可达路径**。

`state_machine.go:30` 默认转换图印证：`{"open":["assigned","closed"], "assigned":["in_progress","open"], ...}` —— 无 `assigned→closed`。
`service.go:395-397` 印证：`if ticket.Status == StatusOpen && req.AssignedTo != nil { newStatus = StatusAssigned }`。

→ 拒收 `assigned_to` 后，`assigned` scope 与「处理人可关闭」（`docs/modules/ticket.md:75,80,114`）在默认配置下**无路径可达成**。需要所有者定性：是「能力有意下线」还是「需同时放开 `assigned→closed`」。

### 2.3 次级：实现口径窄于登记项

登记原文（`docs/phase4/02-implementation-plan.md:24` 与 `:152` 行 28）：

> ⑨**工单 Create 补归属校验+assigned_to 拒收（P0-5…）**——Create 零 L2 写侧校验…→ **补组织归属/委托校验**+assigned_to 直接拒收

登记明确要求「归属/**委托**校验」，而实现只用了：
1. `orgRepo.IsMember`（**直接**成员，`org_repo.go:81`）；
2. `delegation.HasOrgManagePermission`（`org:%` 闸门）作全局豁免。

**缺少的恰是委托那一支**：上级部门 owner / supervisor、祖先 owner、经 L2 锚点（`org_path <@ ANY(anchors)`）可见者，对下级 org 既非直接成员、也不持全局 `org:%` → 一律 70001。这与 ticket 模块其余部分统一采用的锚点/委托语义不一致，正是 26 例失败的共同根因。

**已排除的假设**（实测，非推断）：superadmin/admin **确实**经 `role_menus` 持有 5 个 `org:%` 菜单（本地 dev 库实查），故全局豁免对 SAT 有效——phase2c 的 BK20 路径（SAT 向已清空成员的 VG2 建单）**未破**。phase2c 的 OWNER 也确以 `mkuser … "$VG_ID"` 建为成员，同样未破。

### 2.4 次级：未提交的夹具修补无效（基于错误假设）

`authz_resource_integration_test.go`（未提交 +9 行）把三用户挂到 `setupTicket2a` 的 `orgID`（`p2a_it_*`），但 R3–R8 等用例实际在 `childOrgID(t, "rN")`（**另一个** org `p2a_*`）建单（`authz_resource_integration_test.go:126-139`）。挂错 org → 26 例仍红。修补需覆盖每一处 `childOrgID`，或改为满足委托语义。

---

## 三、其余问题

### 3.1 未提交的 `go.mod`/`go.sum` 不 tidy

```
$ go mod tidy -diff    # Go 1.26.6
require →  - github.com/google/subcommands v1.2.0 // indirect
           - github.com/pmezard/go-difflib v1.0.0 // indirect
```

`tidy` 会删掉这两条。属噪声改动，建议 `git restore go.mod go.sum`。（全量单测在含该改动时仍绿，故非功能性阻断。）

### 3.2 `0231e09` 快照比对的 `params` 为字节精确比较，且测试同义反复

实现（`jobs_callback.go:61-72`）：`bodyParams := string(in.Params)`；仅 `"" → "{}"` 归一；`row.Params != bodyParams` 即 409。

仓库侧归一（`job_submission_repo.go`）：`COALESCE(NULLIF($4,''), '{}')` —— 仅此一种。

**风险（未覆盖）**：两个非等价形态会误判 409 于**合法**回调——
- 回调 body 显式 `"params": null` → `json.RawMessage` 原样为 `"null"`，**不等于**归一后的 `"{}"`；
- 同一 JSON 语义但键序/空白不同（zhuzhao `RecordSubmit` 存客户端原始字节，taskrunner 若经 map 再序列化回传即不同字节）。

**为何未被测试发现**：`jobs_callback_snapshot_integration_test.go` 与 `jobs_integration_test.go` 全部走**回调补录**路径（无 `RecordSubmit` 前置）——该路径下 `row.Params` 由回调 body 自身写入，比对是**同义反复**。真实生产链路「Submit 落档(api origin) → 回调」**零覆盖**。`TestJobsCallbackAuditArchiveE2E`（`jobs_integration_test.go:211`，`params=nil`）亦为纯回调路径。

建议：补一条 `RecordSubmit` 前置 + 回调的用例；或把比对收窄为 `action` 单字段（安全判别力在 action），`params` 改语义化 JSON 比较。

### 3.3 文档未回填（SSOT 漂移）

| 位置 | 现状 | 应补 |
|---|---|---|
| `docs/phase2/09-ticket.md:401` | Create 行 L2 列写「—／create 恒 true」 | 新增归属校验，语义已变 |
| `docs/phase2/09-ticket.md:447-455` | Create 流程 7 步，无归属校验步 | 补第 2.5 步 |
| `docs/modules/auth.md:190-199`（§5.3 登出流程） | 未标 `device_id` 必填 | 随 `32987ea` 补 |
| `docs/phase1/02-auth.md:139-146`（登出黑名单） | 同上 | 同上 |
| `internal/service/ticket/service.go:531` | 注释「对 target 走 L2/L3 鉴权」与代码「双端」不符 | 顺手修正（`3d34b82` 未顺带改） |

### 3.4 次级：`009` 类守卫口径不统一

`ticket Create` 用裸 `HasOrgManagePermission`；而 `org_service.go:38-52 isGlobalOrgAdmin` 是「先查角色码（admin/superadmin）→ 再回退 `HasOrgManagePermission`」。本次实测 SAT 两者皆真，故未爆；但两处口径不同，后续若角色码与菜单绑定漂移会分叉。建议 Create 复用同一判定入口。

---

## 四、建议处置顺序

1. **阻断**：决定 Create 守卫的语义（是否补「委托/锚点」支），修好后重跑 `./internal/service/ticket/` 集成包（应回到 `ok`）。
2. **阻断**：同步 `b2_core_integration_test.go:132` 与 `delegation_authorize_integration_test.go:301` 的矛盾断言；定性「处理人关闭」能力归属。
3. **阻断**：修 `assigned_to` 拒收带来的能力不可达（或明确记录为有意下线）。
4. 撤销/补齐 `go.mod`/`go.sum`（`go mod tidy`）。
5. `0231e09` 补「Submit → 回调」用例，并复核 `params` 字节一致性。
6. 文档回填（§3.3 五处）。

> 注：`test` 门禁方面——**HEAD 提交态本身是绿的**，红灯全部来自工作区未提交改动。
