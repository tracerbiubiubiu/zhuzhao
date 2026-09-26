# 27 批修复验证报告（2026-09-26）

> 被验证对象：`6e9e6ba`（D-4 Swagger）+ `9fdf1e5`（七发现全处置 + 系统性止血），基线 `6609782`。
> 方法：**不采信提交信息**，逐项独立复现。**本轮只读**——除验证期间创建/删除一个临时探针文件（已删除，`git status` 干净）外，未改动仓库任何文件。
> 结论：**7 条发现中 6 条确认解决，1 条（D-7）逻辑正确但无测试；另发现"系统性止血"本身存在 3 个缺陷（其中 1 个是阻断级：默认入口跑不起来）。**
> 追加（同日复核）：§五 **自修清单（照抄级）**——按"先不动代码"的要求，只给改法与精确 DB 长度；期间新查得 D-3 遗漏面比 D-3 原报告更宽（`UpdateProfileRequest` 四个字段全裸，`UpdateUserRequest.RealName` 亦漏），以及 `Makefile` 注释错位（M-3）。

---

## 一、逐项验证结论

| 编号 | 结论 | 我的独立证据 |
|---|---|---|
| **D-1** 000031 down 忠实逆 | ✅ **已解决** | **精确判定**（不只是"down 后再 up 一致"）：`goto 30` → 快照 S30（202 行，其中 `role_menus` **72** 行）→ `goto 31` → `goto 30` → 快照 S30′（202 行，`role_menus` **72** 行）→ **内容级 `diff` 零差异**。即 `down(31) == 30 态`，满足批 25 登记「down 精确还原」。修复前为 72→70。 |
| **D-2** 000005 down 顺序 | ✅ **已解决** | 复现原失败场景：`goto 5` 时 `null_v3=2` → `migrate down 1` → `5/d casbin_nullable (9.4ms)` **成功**（原报 `column "v3" contains null values`）→ 回滚后 `null_v3=0` 且 `is_nullable=NO`，列约束正确恢复。**附带效果**：全链 `down 31` 现在全程通过（原先在 v5 卡死）。 |
| **D-3** 写请求缺 `max=` | 🟡 **行为已解决，实现不完整** | 端到端（当前 HEAD 起服务）：`POST /roles` code=51 → **400/10001**、name=101 → **400**；`POST /users` username=51 → **400**。对照 `POST /roles` code=`viewer`（合法长度）→ **409/40001「角色已存在」**，证明 400 是"超长触发"而非"全都 400"。**但**：`user_request.go` 仅补了 username/employee_no/real_name，**domain_account/user_domain/email/phone/avatar 仍未补 max**；`UpdateUserRequest` 仅补 employee_no。这些字段的 400 来自 D-5 兜底，非 model 主防线。 |
| **D-5** `22001` → 400 兜底 | ✅ **已解决** | 未补 max 的字段端到端验证：`POST /users` avatar=501 → **400/10001**、email=101 → **400**；`POST /tickets` title=201 → **400**（原均 500/10000）。`MapStringValueOverflow` 在 `writeServiceError` 中接线正确。 |
| **D-4** Swagger 三方一致 | ✅ **已解决** | ①`internal/handler/ticket_handler.go:422/445/469/521/544` 五处注解**全部为 `[post]`**；②`docs/swagger.json` 中五个新路径**各命中 1 次**，`"put"`/`"delete"` 方法**归零**；③残留 `{code}` 路径仅剩 `/ticket-templates/{code}`、`/ticket-types/{code}/fields` 两条 **GET**，属正常保留。 |
| **D-6** 架构守护收紧 | ✅ **已解决，且确为真守卫** | **反向测试**：注入 `internal/handler/zz_guard_probe.go`（import repository）→ 守护 **FAIL** 并点名该文件（`architecture_test.go:153`）→ 删除后复跑 **PASS**，工作区干净。证明不再是"无条件放行"。 |
| **D-7** 审计脱敏 `*_token` | 🟡 **逻辑正确但零测试** | `middleware/audit.go` 新增 `strings.HasSuffix(strings.ToLower(key), "_token")` 分支，逻辑正确、包可编译。但 `internal/middleware/audit_test.go` 中**无 `access_token`/`refresh_token` 用例**——同批其他项均带验证，此项未覆盖。 |
| 回归 | ✅ | `go test ./...` 13 包绿；`make guard` 7 条断言绿（verbose 确认执行）；`gofmt -l .` 无漂移。 |

---

## 二、本轮**新发现**：「系统性止血」本身有 3 个缺陷

### 🔴 SYS-1 [阻断] `make guard-roundtrip` 默认调用**必然失败**
- **实测**：`make guard-roundtrip` →
  ```
  31
  error: can't read limit argument N
  make: *** [guard-roundtrip] Error 1
  ```
- **根因**（已隔离验证）：`migrate version` 把版本号输出到 **stderr**（实测 `[stdout]` 为空、`[stderr]` 为 `31`），而脚本用 `LAST=$(... version | awk '{print $1}')` 捕获 **stdout** → `LAST` 恒为空 → `N=""` → `migrate down ""` → 报错（golang-migrate 的该错误文案是固定字面量 `can't read limit argument N`，与变量无关）。
- **对照**：显式传参 `bash scripts/migration-roundtrip-check.sh 31` → ✅ `往返零漂移（31 对…）`。
- **影响**：门禁的**唯一对外入口（Makefile 目标）开箱即废**；提交信息称"全 31 对零漂移验证过"，只可能来自显式传参，与"门禁固化"的意图不符。
- **修法**：`LAST=$($MIG -path migrations -database "$DSN" version 2>&1 | awk '/^[0-9]+$/{print $1}')`（或 `2>&1` 后过滤数字行）。

### 🟡 SYS-2 [P3] 所谓「明细快照」实为**纯计数**，抓不到等量漂移
- 脚本中三条"明细"实为：
  ```sql
  SELECT 'menus_detail', count(*) FROM (SELECT code||parent_id||... FROM menus ORDER BY code) t
  ```
  即对**投影求行数**——`menus_detail` 恒等于 `menus` 行数；`ma_detail`/`rm_detail` 同理仅为计数。
- **后果**：**行数不变而内容改变**的漂移（例如两个角色绑定互换、name/permission 被改写）**对门禁完全不可见**。它能抓到 D-1 只是因为 D-1 恰好是净 −2 行。
- **对照**：我用**内容级**快照（逐行拼接 + 全量 `diff`）验证同一场景——229 行零差异。建议把脚本快照改为内容级（`string_agg` 或逐行导出后 `diff`），才配得上"明细"二字。

### 🟡 SYS-3 [P3] 门禁未强制 + 硬编码绝对路径
- `Makefile` 中 `guard-roundtrip` 是**独立目标**，未挂进 `guard`（默认架构门禁）也未见接入 `acceptance`/CI → **不跑也不会有任何反馈**，"固化"仅停留在"存在"。
- `scripts/migration-roundtrip-check.sh` 硬编码 `MIG=/Users/bujibuji/code/bin/migrate` → **他人机器/CI 必然失败**（本机恰好存在该路径）。应改 `command -v migrate` 或从 PATH 取。
- **附带（M-3，仅观感）**：`Makefile:100` 的注释「架构守护：分层依赖 / 命名 / 技术债门禁」原属 `guard:` 目标，新增 `guard-roundtrip` 时被插在注释与 `guard:` 之间，导致该注释现在**贴在了 `guard-roundtrip:` 头上**（`100` 注释 → `101 guard-roundtrip:` → `104 guard:`）。建议把注释回贴 `guard:`，并给 `guard-roundtrip` 另写一行说明。

---

## 三、需要你决策的两处「元问题」

### M-1 提交信息再次虚报（与刚定性的 `1a04958` 同类）
`9fdf1e5` 提交信息称「D-3 users/roles/**tickets title** 补 max= 对齐 DB varchar（…201 ticket title 端到端 400 实证，原 500）」。
**但 `internal/model/ticket_request.go` 自 `6609782` 起从未被修改**（`git diff 6609782..HEAD -- internal/model/ticket_request.go` 为空），`CreateTicketRequest.Title` 至今只有 `binding:"required"`。
`POST /tickets` title=201 返回 400 是 **D-5 的 22001 兜底**生效，**不是** `max=` 主防线。
→ 现象已修好（这点没问题），但**归因写错了**。鉴于批 25 刚为"提交信息虚报"定过性，建议同口径留痕。

### M-2 D-5 引入了一处分层让步
`internal/handler/errors.go` 现在 `import repository` 并**调用** `repository.MapStringValueOverflow`——这是 handler→repository 的**真实调用**（非类型复用），并因此被加进 D-6 的白名单。而白名单的注释理由仍写「仅查询参数结构体复用」。两处语义已不一致。
备选设计：把 22001 映射放在 service 层出口，或由 errcode 层统一承接，可避免破格。
（另注：`MapStringValueOverflow` 把**所有** 22001 一律判为 400；若未来出现"服务端生成值超长"的场景，会被误报为客户端的 4xx。当前无此场景，属提示。）

---

## 四、复现命令（供你自行核对）

```bash
# D-2 逐对回滚（原失败点）
DB=zz_p5; docker exec zhuzhao-dev-postgres psql -U zhuzhao -q -c "DROP DATABASE IF EXISTS $DB" -c "CREATE DATABASE $DB"
DSN="postgres://zhuzhao:zhuzhao_dev@localhost:5432/$DB?sslmode=disable"; MIG=/Users/bujibuji/code/bin/migrate
$MIG -path migrations -database "$DSN" goto 5
$MIG -path migrations -database "$DSN" down 1            # 修复前：column "v3" contains null values

# SYS-1 门禁默认入口（当前必失败）
make guard-roundtrip
bash scripts/migration-roundtrip-check.sh 31             # 显式传参才通

# D-3/D-5 端到端（需先起服务）
make dev &            # 需 INTERNAL_JOBS_SK 非空
curl -s --noproxy '*' -H "Authorization: Bearer $TOK" -H 'Content-Type: application/json' \
  -X POST http://localhost:33333/api/v1/tickets \
  -d "{\"type_code\":\"incident\",\"title\":\"$(python3 -c 'print("a"*201)')\",\"org_id\":\"1\"}"   # 期望 400/10001
```

---

## 五、自修清单（照抄级，2026-09-26 复核补）

> 按你的选择「先不动代码」，此处只给改法与精确参数。**本清单不含任何已落地的仓库改动**。

### SYS-1 一行修法

`scripts/migration-roundtrip-check.sh:21` 改为：

```bash
LAST=$($MIG -path migrations -database "$DSN" version 2>&1 | awk '/^[0-9]+$/{print $1}')
[ -n "$LAST" ] || { echo "❌ 无法解析 migrate version（stderr 未输出数字）"; exit 1; }
```

加第二行的意义：把「静默取空」变成**显式失败**——否则下次 migrate 改输出格式时又会退化成今天这个「跑不起来但看起来在跑」的状态。

### SYS-2 内容级快照（替换 `snapshot()`）

```bash
snapshot() {
  docker exec "$PG_C" psql -U zhuzhao -d "$DB" -At -F $'\x1f' -c "
    SELECT 'menus', code, coalesce(parent_id::text,''), coalesce(menu_type::text,''),
           coalesce(path,''), coalesce(component,''), coalesce(permission,''), coalesce(name,'')
      FROM menus
    UNION ALL
    SELECT 'menu_apis', m.code, coalesce(ma.api_path,''), coalesce(ma.api_method::text,''),'','','',''
      FROM menu_apis ma JOIN menus m ON m.id = ma.menu_id
    UNION ALL
    SELECT 'role_menus', r.code, m.code,'','','','',''
      FROM role_menus rm JOIN roles r ON r.id = rm.role_id JOIN menus m ON m.id = rm.menu_id
    UNION ALL
    SELECT 'casbin_rule', coalesce(ptype,''), coalesce(v0,''), coalesce(v1,''), coalesce(v2,''), coalesce(v3,''),'',''
      FROM casbin_rule
  " | LC_ALL=C sort
}
```

要点：① 逐行输出而非 `count(*)`——**增/删/改任一行都会产生 `diff` 的 `<`/`>` 行**，与 SYS-2 描述的等量漂移盲区正面对冲；② 分隔符用 `\x1f`（不用 `|`，避免字段值含 `|` 时产生歧义拼接）；③ `LC_ALL=C sort` 保证两次快照排序稳定；④ `removed` 行数本身就是计数，原先那 4 条 `count` 行可一并删掉。

### SYS-3 + M-3 收口

- 脚本第 12 行：`MIG=${MIG:-$(command -v migrate || echo /Users/bujibuji/code/bin/migrate)}` —— 允许环境变量覆盖、PATH 优先，仅在本机兜底。
- `Makefile:100-104` 把注释归位：`# 架构守护…` 贴回 `guard:`，`guard-roundtrip` 单写一行 `# 迁移往返数据 diff（依赖开发 PG 容器）`。
- 若要真"固化"：`acceptance:` 目标链尾追加 `bash scripts/migration-roundtrip-check.sh`，或让 `guard:` 依赖它（注意它有**容器依赖**，进 CI 需先起 PG，否则应只在 `acceptance` 链内）。

### D-3 遗留字段（精确到 DB 长度，含本轮新查的两处）

`migrations/000001_init.up.sql:7-15` 定义：`domain_account(100) / user_domain(255) / real_name(100) / email(100) / phone(20) / avatar(500)`。据此：

| 结构体 | 待补字段 | 追加 tag |
|---|---|---|
| `CreateUserRequest` | `DomainAccount` / `UserDomain` / `Email` / `Phone` / `Avatar` | `omitempty,max=100` / `255` / `100` / `20` / `500` |
| `UpdateUserRequest` | `DomainAccount` / `UserDomain` / **`RealName`** / `Email` / `Phone` / `Avatar` | 同上（**`RealName` 在此处也漏了 `max=100`**） |
| **`UpdateProfileRequest`**（报告 D-3 未点名，本轮新查） | `RealName` / `Email` / `Phone` / `Avatar` | `omitempty,max=100 / 100 / 20 / 500` |
| `CreateTicketRequest` | `Title` → `required,max=200`；`TypeCode` → `required,max=50` | 兑现 `9fdf1e5` 提交信息里的承诺 |

说明：指针字段配 `omitempty` 时，`validator` 对 `nil` 短路跳过、对**非 nil 空串**仍会校验 → 与 B2-3 的「传空串显式清空」语义**不冲突**，可安全加。`000010_ticket.up.sql:36` 明确 `title VARCHAR(200) NOT NULL`。

### D-7 测试补法

`internal/middleware/audit_test.go` 补表驱动用例：正例 `access_token` / `refresh_token` / `Access_Token`（验大小写无关）、`_token`；反例 `token`（**无下划线前缀，不应脱敏**）、`tokens`——反例是防止把实现写成 `strings.Contains(key,"token")` 的宽松匹配。

### M-1 / M-2 留痕建议

- **M-1**：同 `1a04958` 口径，在 `docs/phase4/02-implementation-plan.md` §5.5 或 11 号文登记「`9fdf1e5` 提交信息把 ticket title 的 400 归因于本批 `max=`，实为 D-5 的 22001 兜底（`ticket_request.go` 自 `6609782` 未改）」。
- **M-2**：两个选项——(a) 保留现设计，但把 `architecture_test.go` 白名单注释改为「查询参数结构体复用 + errcode 映射」，消除语义不一致；(b) 把 `MapStringValueOverflow` 的调用挪到 service 出口或 errcode 层统一承接，恢复 handler→repository 零调用。

---

## 六、一句话结论

**你确实把 7 条发现里的实质问题都修掉了**——D-1 是**精确的忠实逆**（内容级零差异）、D-2 在原失败点通过、D-4/D-6 经反向测试确认为真修复、D-3/D-5 端到端从 500 变 400。**但**：(1) 配套的"系统性门禁"**默认跑不起来**，且其"明细"只是计数，撑不起它声称的防护力；(2) D-7 无测试；(3) 提交信息对 ticket title 的归因有误。建议优先修 SYS-1（一行改动）并顺手把 SYS-2 换成内容级快照，否则这个门禁会给人"已验证"的错觉。

**补充**：D-3 的遗漏面比原报告更宽——除 `CreateUserRequest` 五个字段外，`UpdateUserRequest.RealName` 与**整个 `UpdateProfileRequest`（用户自助改资料路径）**同样无 `max=`；逐字段精确长度见 **§五**。该清单已按"先不动代码"约定写成可照抄的改法，未改动仓库任何文件。

---

## 附：处置回执（2026-09-26 当日闭环，`ed9fac3`）

SYS-1（stderr 捕获+解析空显式失败）/SYS-2（内容级逐行快照）/SYS-3（PATH 优先+Makefile 注释归位 M-3+挂 acceptance 链尾）/M-1（Title/TypeCode/Update 指针补 max= 兑现+虚报定性 02 行 31）/D-3 遗漏面（user_request 19 处含 UpdateProfileRequest 四指针+email 格式）/D-7（正反例测试，含 tokens 反例）/M-2a（白名单注释如实）——`make guard-roundtrip` 默认入口 31 对零漂移验证过。本档为闭环验证档案。
