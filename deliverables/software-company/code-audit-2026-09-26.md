# zhuzhao 全项目代码核查报告（2026-09-26）

> 基线：`phase4` @ `6609782`，工作区干净。**本轮只读，未修改任何代码/文档/迁移。**
> 方法：三层独立核查 + 交叉验证（架构师审计 → QA 独立复核 → 主理人复核决定性证据）。
> 原则：**结论与数字均由至少两方独立复现**；对「某内容不存在」的结论一律换专用检索工具二次确认（本机 BSD `grep` 的 `\|` 交替会**静默返回空**，本轮已导致 2 次误判并被工具复核纠正）。

---

## 一、门禁实测（两方独立复跑，结果一致）

| 门禁 | 主理人 | QA 独立复跑 | 结论 |
|---|---|---|---|
| `go build ./...` | 通过 | exit 0 | ✅ |
| `go vet ./...` | 通过 | exit 0 | ✅ |
| `go test ./...`（13 包单测） | 全绿 | 13 包全 ok | ✅ |
| `go test -tags=integration ./...`（13 包） | 全绿 | 13 包全 ok | ✅ |
| `make guard`（`internal/architecture_test.go`） | 7/7 PASS | 7/7 PASS（verbose 逐条） | ✅ |
| `gofmt -l .` | 无漂移 | 0 行漂移 | ✅ |
| **四档 acceptance**（`make acceptance`） | 未跑 | **2a 67 / 2b 26 / 2c 34，FAIL=0**，phase1 作链式前置必过 | ✅ |

**基线判定：当前代码在自动化门禁层面是健康的。** 所有发现均为**门禁未覆盖**的盲区（迁移往返数据一致性与 Swagger 产物一致性都没有专用断言）。

---

## 二、发现清单（已去重合并，严重度为**主理人裁定**）

### D-1｜P2｜迁移 `000031` down 非忠实逆：`role_menus` 漂移 −6/+4
- **位置**：`migrations/000031_menu_wordbook_split.down.sql:4-31`（⑤ 只重建 `menus`+`menu_apis`，**从不重建 `role_menus`**；⑥ 撤销清单**漏 `al_manage`/`task_manage`**），对照 `.up.sql:181-187`（⑤ 先删 role_menus 三连删）、`:189-200`（⑥ 对 admin/superadmin **全量**补绑）。
- **实证**（QA 临时库 + 主理人复核）：up→down 往返后
  - **永久丢失 6 行**：admin/superadmin × `system_menu_create/update/delete`
  - **多出 4 行**：admin/superadmin × `al_manage`/`task_manage`
  - 净 `role_menus` 72 → 70
  - **`menus` / `menu_apis` 两表往返完全一致（无漂移）** ← QA 补充核验，缩小了修复面
- **严重度上调理由（主理人）**：架构师/QA 均判 P3，但批 25 登记（`docs/phase4/02-implementation-plan.md:25`）对本迁移有**明文验收要求**：
  > 「**000031 内嵌「路由→按钮」逐行对照表（SQL 注释），down 精确还原，验收断言按表逐行核**」
  
  实测**未精确还原**、且**逐行核验显然未执行**（否则必红）。故本质是**登记项验收未达标**，非普通小漂移 → **P2**。

### D-2｜P2｜迁移 `000005` down 顺序倒置：逐对回滚直接失败
- **位置**：`migrations/000005_casbin_nullable.down.sql:2-9`（先 `SET NOT NULL`）vs `:10-13`（后回填 `NULL→''`）——**顺序倒置**。对照 `.up.sql`：up 把 `''` 全转成 NULL，故 down 时刻必有 NULL。
- **主理人独立实证**（全新临时库，apply 000001..000005 up）：
  ```
  casbin_rule: total=2, null_v2=0, null_v3=2, null_v5=2
  执行 000005 down → ERROR: column "v3" of relation "casbin_rule" contains null values
  ```
  → **`migrate down 1`（逐对回滚）即失败**，落在项目自认的**支持范围内**。
- **严重度上调理由（主理人）**：批 29 登记（`docs/phase4/02-implementation-plan.md:153`）的豁免范围**写死为 `000001-004`**：
  > 「②**000004** casbin_column_pt down 数据条件债（v3 列含 NULL 时不可回滚）：**定性=全链 down 到 0 非支持路径**（支持范围=逐对 down+关键区段可逆），**000001-004 深层回滚债登记不做**」
  
  `000005` **既不在豁免编号区间内**，**性质也不同**——000004 是「数据条件债（不可逆）」，而 000005 是**纯粹的语句顺序错误（可逆、可修）**：把 `:10-13` 的 UPDATE 提到 `SET NOT NULL` 之前即正确回滚。故**不可用既有定性豁免** → **P2**。

### D-3｜P3｜写请求缺 `max` 绑定 → PG `22001` 未映射 → **HTTP 500 + 10000**
- **根因链**：`model/user_request.go`（`max=` 出现 **0** 次）、`model/role_request.go`（**0** 次）未对齐 DB varchar；`repository/pgerr.go:13/:39` **只映射 23505/23503**；`handler/errors.go:80-91` 兜底 → `InternalError`。
- **实测对照**（QA，当前 HEAD 起服务端到端）：

  | 请求 | 结果 |
  |---|---|
  | `POST /roles` code=51 字符 | **500 / 10000** ❌ |
  | `POST /roles` name=101 字符 | **500 / 10000** ❌ |
  | `POST /users` username=51 字符 | **500 / 10000** ❌ |
  | `POST /tickets` title=201 字符 | **500 / 10000** ❌ |
  | `POST /orgs` name=101（已修域，对照） | **400 / 10001** ✅ |
  | `POST /ticket-types` code=51（已修域，对照） | **400 / 10001** ✅ |

- **域覆盖矩阵**（会 500 = 无 `max`）：users.create/update、roles.create/update、tickets.create/update 的 `title`；**已修**：login(`employee_no`)、org、ticket-types/templates/fields；**天然免疫**（TEXT 列）：roles.description、tickets.description、comments/notes.content。
- 说明：`org_request.go` 有 3 处 `max=`（R2-ORG-06 已修）、`ticket_request.go` 有 10 处（集中在类型/模板/字段段）——**同一文件内覆盖不均**是本次漏改的形态。
- **无越权/无信息泄漏**（错误体为通用文案），但**客户端输入错误计 5xx**，污染告警与指标。

### D-4｜P2｜Swagger 产物 / 路由 / 注解三方不一致 + 提交信息不实
- **位置**：`internal/handler/ticket_handler.go:422/445/469/521/544`（五处 `@Router` 仍 `[put]`/`[delete]`）vs `internal/router/router.go:297-302`（全 POST）。
- **产物**：`docs/swagger.json`、`docs/docs.go`、`docs/swagger.yaml` 中旧 `{code}` PUT/DELETE **全命中**，新 `/ticket-types/update|delete|fields/replace`、`/ticket-templates/update|delete` **零命中**。
- **决定性验证（QA）**：`swag init -o /tmp/qa_swag` 重生成 → 与已提交产物 **byte-identical（diff 为空）**。
  → **结论：只跑 `make swag` 修复不了，必须先改 5 处注解**。
- **提交信息不实**：`git log -1 -- docs/swagger.*` = `1a04958`，而 `git show 1a04958 -- docs/swagger.json` 实际**只 +33 行 definitions，零 path 改动**，与提交信息「swag 重生成（POST 五端点+删三写路由）」矛盾。
- **裁定**：产物漂移并入已登记 **P1-8**（同域）；但**「提交信息不实」单列留痕**（对应批 25 教训「提交信息必须披露、不得虚报」，另有 `25bda2e` 静默回退先例）。

### D-5｜P3｜pgerr 映射码清单缺口（D-3 的纵深兜底）
- `repository/pgerr.go` 仅 23505（唯一）/23503（外键）。缺 **22001（字符串超长，D-3 直接根因）**、22003、23514(CHECK)、23502(NOT NULL)。
- 建议：至少补 `22001 → ErrInvalidParams(400)` 作为兜底，防未来新字段再漏。

### D-6｜P3｜架构守护断言强度与注释不符（主理人发现，QA/架构师未报）
- `internal/architecture_test.go:141-147`：对 `handler → repository` 是**无条件放行**（命中即 `ok = true` 并仅打日志），而 `:103-104` 注释声称「**仅允许复用查询参数结构体，不允许直接做数据访问**」。
- 现状核查（主理人）：非测试代码中 handler 只用到 `repository.UserListQuery`（`user_handler.go:26`）与 `repository.AuditListQuery`（`audit_handler.go:25`）——**当前无实际违规**。
- 性质：**守卫挡不住未来违规**（handler 若持有注入的 repo 并调其方法，`NoDBInHandler` 也拦不住，因为它只查 DB 驱动 import）。属"安全感的假象"，P3。

### D-7｜P3｜审计脱敏键清单未覆盖 `*_token`
- `middleware/audit.go:118` `sensitiveKeys` 精确匹配 `{password, old_password, new_password, secret, token}`，**不含 `access_token`/`refresh_token`**。
- **当前无害**：`/auth/login`、`/auth/refresh` 只在 auth 组（`router.go:136-141`，仅 RateLimit），AuditLog 挂 `authed` 组（`:144-149`）→ `refresh_token` 不进审计。
- 风险：未来若审计面端点回传 `*_token` 会明文入库。属**前瞻性加固**。

---

## 三、系统性判断（主理人综合）——比单条缺陷更值得关注

**同一缺陷类已至少第 6 个迁移文件命中：** `000010`（作为正确范式被引用）→ `000022`（23503）→ `000018`/`000024`/`000025`（role_menus，批 25 补）→ **`000031`（D-1，本轮）** → **`000005`（D-2，本轮）**。

批 29 已做过「全链 down 验证（31 对→0）」，**却抓不到 D-1**——因为该验证只断言「down 过程不报错」，**没有做 up→down→再 up 的往返数据比对**。于是 D-1 这种「能跑通但数据悄悄漂移」的缺陷**对现有验证方法全盲**。

**建议：把"往返数据 diff"固化为门禁**（对关键表 `menus`/`menu_apis`/`role_menus`/`casbin_rule` 在临时库做 up→down→A/B 快照比对）。否则「逐对补齐」是无限重复劳动，且第 7 次复发只是时间问题。

---

## 四、已核实无问题面（两方交叉确认，供后续免复）

- 迁移 **000030** up/down **完全对称**，五条 UPDATE 全用 `(api_path, api_method)` 复合定位，GET 行未被误伤（红线遵守）。
- 迁移 **000031 up** ③ 段 `AND ma.api_method != 'GET'`（`:159`，同 path 跨 method 红线）**在位**；⑤ 段删三 POST 保留 GET 树/详情正确；⑥ 段补绑逻辑本身正确（错只错在 **down**）。
- **casbin 保留字提权不可行**：`validate.Identifier` 字符集不含 `:`（无法构造 `role::admin`），且 `roles.code` 唯一约束已占用 `admin`。
- **审计链路**：`context.WithoutCancel` + 独立 3s 超时（`audit.go:84`）；`maskSensitive` 递归 + 大小写不敏感；`refresh_token` 不入审计（见 D-7）。
- **审计仓储无注入**：`audit_log_repo.go` 的动态表名有 `archiveTables` 白名单（`:176-193`）。
- **SSRF 四口收口**：`taskrunner_handler.go:57-58` 非空 `callback_url` → 400；回调地址由 `self_base_url` 服务端拼（`taskrunner_service.go:58-72`）。
- **AK/SK 弱值**：release 拒 repo-known 三件套；`InternalJobs.Enabled` 且 SK 空拒启；Gateway 配 upstreams 但 AK/SK 缺失拒启。
- **BK-22 / wire**：对账在 `app.NewApp` 构造期 fail-fast；`menu_apis` 硬删无软删漏读。
- **空吞错误**：非测试代码真·忽略 error 仅 4 处（`gateway.go:108`、`audit.go:53`、`logger.go:83`、`testutil`），均属可接受。

---

## 五、建议处置顺序

| 序 | 项 | 动作 | 成本 |
|---|---|---|---|
| 1 | **D-2**（000005 down） | 把 `down.sql:10-13` 的 UPDATE 提到 `:2-9` 的 `SET NOT NULL` **之前**；临时库验证逐对回滚通过 | 极小 |
| 2 | **D-1**（000031 down） | 补两处：down ⑤ 末尾重建 `role_menus`（admin/superadmin × `system_menu_*`，`ON CONFLICT DO NOTHING`）；down ⑥ 清单补 `al_manage`/`task_manage` | 小 |
| 3 | **D-4**（Swagger） | 改 `ticket_handler.go` 5 处注解为 POST + code 入 body → `make swag` → CI 加 `git diff --exit-code docs/` | 小；**W2 首日前置（P1-8 已登记）** |
| 4 | **D-3**（500） | 补 users/roles/tickets 的 `max=` 对齐 DB **且** 补 `pgerr.go` 的 `22001 → 400` | 小 |
| 5 | **D-6/D-7** | 收紧 `LayerDependency` 的 handler→repository 豁免（改为只放行查询结构体）；`sensitiveKeys` 补 `*_token` 前缀匹配 | 小 |
| 6 | **系统性** | 把「迁移往返数据 diff」加进 `make guard` 或验收脚本 | 中（一次投入，长期止血） |
| 7 | **留痕** | D-4 的「提交信息不实」按批 25 教训在本批登记中留痕 | 极小 |

---

## 六、方法与限制

- **只读**：全程未修改仓库任何文件；两方均确认结束时 `git status` 干净。
- 临时资源：QA 与主理人各自建立的临时库（`zhuzhao_qa_*` / `zz_lead_tmp`）**已全部 DROP**，未触碰 dev 库（`zhuzhao`）。
- **未执行 `docker-dev-reset`**（会清空本地 dev 数据卷）：四档 acceptance 在**现有 dev 库**上直接跑通，故无需 reset。
- 环境注意（非代码问题）：本机 shell 有 `HTTP_PROXY=http://127.0.0.1:62718`，`curl` 访问 localhost 会走代理返回 **502**，需 `--noproxy '*'` 绕过；`make dev` 需 `INTERNAL_JOBS_SK` 非空（否则拒启，runbook §1.2）——这两条建议写进 runbook，避免后续验收误判。
- **未覆盖**：跨仓（taskrunner/activelist）代码未在本轮范围内；四档 acceptance 未做 `docker-dev-reset` 后的纯净重放（现有库脏但脚本按幂等/唯一后缀设计，结果可信）。

---

## 附：三方结论一致性

| 编号 | 架构师（R1） | QA（R2） | 主理人（终审） |
|---|---|---|---|
| F-1 → D-1 | P3 | 确证，数字吻合 | **P2 上调**（登记要求「down 精确还原」未达标） |
| — → D-2 | 未报 | 新发现 P3 | **P2 上调**（豁免区间 `000001-004` 不含 000005；性质为可修的顺序错） |
| F-2 → D-3 | P3 | 端到端确证 + 扩到工单域 | P3 维持（含 D-5 兜底建议） |
| F-3 → D-4 | P2 | 确证 + 重生成验证 | P2 维持（产物并入 P1-8；「提交信息不实」单列） |
| — → D-6 | 未报 | 未报 | P3 新发现（guard 强度） |
| — → D-7 | 未报 | 新观察 | P3（前瞻加固） |
