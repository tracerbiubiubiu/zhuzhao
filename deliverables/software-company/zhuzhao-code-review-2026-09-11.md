# zhuzhao 全面代码检查报告

> **检查对象**：`zhuzhao`（Go 模块化单体 IAM + 工单）｜分支 `feature/phase-3`
> **检查日期**：2026-09-11
> **组织方式**：软件开发团队（主理人齐活林编排）—— QA 严过关（场景×测试）、架构师高见远（设计↔实现）、工程师寇豆码（并发×细节）三线并行，主理人独立复核交叉验证
> **性质**：**只读审查**——审查本身未修改任何项目代码 / 文档 / 迁移。
> **后续动作（2026-09-11 三轮）**：仅就 **P1-4 的重新定性**做了 **doc-only 登记**（`docs/phase1/02-auth.md` + `docs/review/11-project-control.md`），**未改任何代码**；详见 §二·补。
> **原始分报告**：`.workbuddy/codereview/{qa-test-coverage,architect-design-impl,engineer-concurrency-details}.md`
> **依据**：`docs/` 为 SSOT（`doc/` 为旧归档，未采信）

---

## 一、总体裁决（TL;DR）

**设计站得住，实现整体忠实；核心并发机制质量高；未发现 P0 级缺陷。**

- **无 P0**：无「必然数据错乱 / 可直接利用的安全漏洞 / 确定性崩溃」。
- **问题分布（经二次确认 + 三轮定性校准）**：**P1 × 3**（真实并发缺陷，**均已真机复现**）+ **P2 × 19**（健壮性/关闭竞态/一致性）+ **文档口径不一致 × 6**（低）+ **有意设计登记 × 1**。

> ✅ **二次确认（2026-09-11）**：已利用本地 PG/Redis 对 P1/P2 的关键条目做**真机复现**——P1-1（8/8 并发回调重复执行）、P1-2（59/60 轮双向重复关联）、P1-3（lost update）、P2-1（软删组织残留成员）**全部坐实**；同时**修正一条误报**（分页回显仅 `page` 上限成立、`page_size` 侧不成立），并把原 P1-5（护栏键错头）**下调为 P2**（不可利用）。详见 [二次确认报告](./zhuzhao-code-review-verification-2026-09-11.md)。
> ⚖️ **三轮定性校准（2026-09-11，所有者复议）**：原 **P1-4（RT 轮换重用即删键）撤销 P1 定性**——经查该行为（旧 RT 提交 → 当前会话失效 → 强制重登）是 **OAuth BCP 式的「RT 重用检测」有意设计**，且**已在 `docs/phase1/02-auth.md` §RT 轮换流程显式登记**（Auth0/OWASP ASVS 对照 + 可接受的 Phase 1 安全级别）；残留的真问题仅为**多标签迟到提交的误伤**（已知代价，非缺陷），已改为「有意设计 + 已知代价」登记入档（见下 §二·补）。故 **P1 由 4 条收敛为 3 条**。
- **一句话**：这不是一个"做错了"的代码库——它的并发骨架（乐观锁、状态机 CAS、事务边界、Redis 原子原语、advisory lock）**实现质量高于同级项目**；问题集中在**低频配置写路径**、**handler/并发重复提交的测试盲区**、以及**文档与实现的口径漂移**。
- **测试的根本形态**：67 测试文件 / 254 测试函数，**主链覆盖扎实且多为真并发**，但 **CI 默认跑的是最薄的两层**（`test-unit` 19.1% 覆盖、handler 包 14.6%），工单 handler（567 行）、审计写库、网关穿越、乐观锁冲突**全在 CI 看不见的地方**。

---

## 二、问题总表（三线合并去重，按严重度）

### P1 —— 真实缺陷（3 条，均已真机复现）

| # | 问题 | 位置 | 后果 | 修法 | 复现证据 |
|---|---|---|---|---|---|
| **P1-1** | **回调幂等栅栏非原子**：`EnsureCallbackRow` = `INSERT ON CONFLICT DO NOTHING` + 独立 `GetByTaskID` 两步，无锁 | `internal/service/jobs_callback.go:51-80`；`internal/repository/job_submission_repo.go:86-103` | 同一 `task_id` 的**并发**回调（taskrunner at-least-once 重投）都读到 `status=submitted` → **两个 goroutine 同时执行 handler**，副作用重复 | 改原子抢占：一次往返 `INSERT ... ON CONFLICT (task_id) DO UPDATE ... RETURNING status`，或 `SELECT ... FOR UPDATE` 锁行后判定 | ✅ 8 并发 → handler **执行 8 次**（预期 1） |
| **P1-2** | **工单关联反向判重 TOCTOU**：`ExistsRelationBetween` 事务外预检 + `CreateRelation` 单独 INSERT；DB 唯一索引**仅防同向** | `internal/service/ticket/service.go:514-547`；`internal/repository/ticket_repo.go:408-443`；索引 `000016:16` | 并发 A→B 与 B→A 双双通过预检 → **双向重复关联行**（BK-5 声称的双向判重只挡顺序到达） | 写入纳入事务 + 规范化唯一键（`least(source,target), greatest(...)` 生成列唯一索引） | ✅ 60 轮并发 → **59 轮产生 2 行** |
| **P1-3** | **类型/字段/模板三表无乐观锁**（并发编辑静默互相覆盖，无 409） | `internal/repository/ticket_repo.go:542-564 / 595-620 / 683-705`；DDL `migrations/000010_ticket.up.sql`、`000015_*.up.sql` | 两管理员并发编辑同一类型/模板 → 后写覆盖先写，`states/transitions`/字段定义静默丢失；状态机可能偏离文档 | 三表加 `version INT NOT NULL DEFAULT 1` + `WHERE code=$1 AND version=$n`，0 行 → `ErrConcurrentModification(409)`；请求体补 `version` | ✅ 并发写 description → 两次均 200、**一方被覆盖** |

> **~~P1-4~~ → 已撤销 P1 定性（改为「有意设计」登记，见下 §二·补）**：原判「RT 轮换重用即删当前有效键」为缺陷，经复议**不成立**——该行为是 OAuth BCP 式的 RT 重用检测，**文档中已作为有意设计登记**。

### 二·补　撤销项：P1-4 重新定性为「有意设计 + 已知代价」

| 项 | 内容 |
|---|---|
| **原判** | P1-4：`GetDel` 先删再比对，提交任意旧 RT 即删当前有效键 → 强制合法用户下线（`auth_service.go:174-184`） |
| **复议结论** | **撤销 P1 定性，不按 bug 修**。理由：① 「旧 RT 提交 → 401+20004」是 **OAuth BCP / Auth0 / Google 的 RT 重用检测**正统行为（重放即盗用信号 → 会话失效强制重登），符合最佳实践；② 该设计**早已在 `docs/phase1/02-auth.md` §RT 轮换流程「RT Reuse Detection（业界对照）」显式登记**，并自评「可接受的 Phase 1 安全级别」；③ 作用域为**单设备（`refresh:{userId}:{deviceId}` 单槽位）**，不跨设备，影响可控；④ 不涉数据正确性。 |
| **残留真问题** | **多标签/迟到提交误伤**：A 刷新成功拿到新 RT 后，B 携带已轮换的旧 RT **迟到**提交（落在 issue→Set 的极短窗口）→ `GetDel` 把 A 的当前有效键一并删除 → A、B 双双重登。属**并发时序、非确定性**，是业界已知代价。 |
| **处置** | **登记不修**（有意设计 + 已知代价），已入档 `docs/phase1/02-auth.md`（补登误伤代价与演进选项）+ `docs/review/11-project-control.md`（§6 健康状态 `RT-1` / §8 随手项）。触发驱动：**若线上出现多标签体验问题反馈**，再评估「宽限窗口 / family_id」方案。 |
| **附带修正** | 原报告「建议改『先 GET 比对、命中才 DEL』」**该修法本身即会削弱盗用检测**（replay 退化为 no-op，不再吊销会话），故不可取；同时在报告中更正一条事实——**文档本就覆盖 RT 轮换/重用（此前误称『未提及』系 BSD grep 不支持 `\|` 交替所致）**。 |


### P2 —— 健壮性 / 关闭竞态 / 一致性（19 条，摘要）

| # | 问题 | 位置 | 影响 |
|---|---|---|---|
| P2-1 | 组织删除守卫**注释称 `FOR UPDATE` 锁行，实无**（三条 COUNT 子查询无锁；且该 SQL **无 FROM 子句**，直接补锁在语法上就不成立） | `internal/repository/org_repo.go:340-352` | READ COMMITTED 下并发加成员可绕过 → 软删组织残留成员行；B4-5 声称已消灭的窗口仍有残余。**⚠️ 修法见下注——「给守卫补 `FOR UPDATE`」无效** |
| P2-2 | 优雅关停**未 join 判定日志管道**：`Shutdown()` 只关 HTTP，不等待 pump/flusher drain，`main.go` 随后关 Redis/PG | `internal/app/app.go:67,84-99`；`internal/pkg/audit/policyeval.go:143-158,166-170` | 在途判定行可能静默丢失（fail-open 可接受，但属"不可控丢"）；建议 `sync.WaitGroup` join |
| P2-3 | flusher 落库失败时 Redis `processing` **无界增长** | `internal/pkg/audit/policyeval.go:177-218` | DB 持续不可用 → 每 tick 仍 `LMove` 新行、插入失败不 trim → List 线性膨胀占内存 |
| P2-4 | 限流 Lua 返回值**断言缺长度校验**：`vals, _ := res.([]any)` 后直接 `vals[0]` | `internal/middleware/ratelimit.go:86-87` | 类型异常 → index panic → Recovery 转 500（本应 fail-close 503） |
| P2-5 | RBAC 继承不变量 **check-then-act**（守卫在事务外） | `internal/service/rbac_service.go:124-138`；`internal/repository/role_repo.go:152-175,385-399` | 并发角色更新可破坏 `child.priority ≤ parent.priority` 单调性/成环（环已被 review 判定"收敛不发散"） |
| P2-6 | Casbin 内存策略**多实例不一致**：`StopAutoLoadPolicy` 实为 no-op（从未 Start），`reloadPolicy` 只刷本实例 | `internal/casbin/enforcer.go:30-37`；`internal/service/rbac_service.go:331-349` | 多副本下被撤销权限在他副本继续放行，直至重启（部署文档已把多实例列为 Phase 3，单副本无碍） |
| P2-7 | 会话吊销 `SCAN + DEL` **非原子** | `internal/service/session_revoke.go:12-32` | SCAN 后新建的 RT 键漏删；**被 `user:disabled` 键兜住**，可接受 |
| P2-8 | 限流时间源用应用侧 `time.Now()` | `internal/middleware/ratelimit.go:79` | 多实例时钟漂移影响精度（注释已声明升级路径 = `redis TIME`） |
| P2-9 | `RecordSubmit` **幂等分支与注释不符**：`ON CONFLICT DO NOTHING RETURNING` 冲突时无行 → 返回错误，与「返回既有行、不算错」矛盾 | `internal/repository/job_submission_repo.go:50-66` | 当前两处调用方均吞错，无可见影响；函数契约被违反，未来调用方会踩坑 |
| P2-10 | 工单列表**分页 `page` 上限未在 handler 归一**（⚠️ **二次确认修正**：`page_size` 侧**不成立**——handler 已把越界值归一为 20，与 repo 一致） | `internal/handler/ticket_handler.go:43-45`（仅夹下限）；`internal/repository/user_repo.go:629-631`（repo 夹上限 10000） | 请求 `page=999999` 实查第 10000 页、响应回显 999999 → 回显与实查不一致 |
| P2-11 | 审计按天筛选**时区边界用 UTC** | `internal/handler/audit_handler.go:38-52`；`internal/repository/audit_log_repo.go:100-124` | 东八区查询"某天"边界偏移 8 小时，漏/多数据 |
| P2-12 | 错误被吞（**多为已文档化取舍**）：`rand.Read` err、`io.ReadAll` err、`MarkFailed` err、凭证记账 err | `internal/middleware/logger.go:63`、`audit.go:53`、`jobs_callback.go:67`、`taskrunner_service.go:84,100` | 仅 `logger.go:63` 值得加 Error 日志（极端下 request_id 全零） |
| P2-13 | 同步审计写造成**尾延迟放大**（响应已发出后仍占用至多 3s） | `internal/middleware/audit.go:84-88` | DB 抖动期 goroutine/连接被放大占用（F-5 已知取舍） |
| P2-14 | `InsertPolicyEvals` 参数随 BatchSize 线性放大（每行 9 占位符） | `internal/repository/audit_log_repo.go:149-170` | batch_size > ~7281 触发 PG 65535 参数上限；建议配置侧设上限 |
| P2-15 | **JWT/AK-SK 互斥护栏键错头**（原 P1-5，**二次确认下调**）：护栏检测 `X-AK-Access-Key`，而定版 AK/SK（utils `aksk`）用 `Authorization: HMAC Credential=...` | `internal/middleware/jwt.go:117-118` vs `zhuzhao-utils@v0.2.0/aksk/aksk.go:39-45`；文档 `architecture §5.4`、`api/errcode.md:245` | 「Bearer + AK/SK 互斥 → 400/20008」对真实方案**真空**；但**不可利用**（AK/SK 走 `/internal/jobs` 独立路由组，与 JWT 路由不相交），属未上线方案的陈旧占位 |

> 另有 4 条「文档口径/滞后」（低）：`registry.Authorize` 文档写 Handler 实为 Service、`TicketEngine` Port 文档超前未落地、`authz.md` 伪代码滞后、`service` 分层口径两文档不一（见第四节）。

> **⚠️ P2-1 修法更正（2026-09-11；真 PG 15 锁矩阵实测 + testcontainers 端到端复现，全部可复现）**——**「给守卫 SELECT 加 `FOR UPDATE`」关不了窗**：
> ① **冲突矩阵（实测）**：软删 `UPDATE organizations SET deleted_at=…` 取 **`FOR NO KEY UPDATE`**；INSERT 外键检查取 **`FOR KEY SHARE`**——**二者不冲突**。守卫改 `FOR UPDATE` 确会使 FK 插入阻塞，但**软删不使外键目标失效**（行物理仍在、FK 也不看 `deleted_at`），阻塞解除后 INSERT 仍成功。**端到端证伪**：基线残留 `deleted=true members=1`；「仅守卫加 `FOR UPDATE`」重跑**仍 `members=1`**。照搬既有 `FindByIDForShareTx` 的 **`FOR SHARE` 也挡不住**。
> ② **真修法 = 两侧锁协议（无需迁移）**：**Delete 侧**在守卫前对 org 行取 **`FOR UPDATE`** 作为「删除中」的**事务性认领**（随提交/回滚自动释放，**零 schema 改动**；故**不建议**复用 `status`（0=禁用是独立语义、会混淆），也不建议新增 `deleting` 标记列），再跑三 COUNT 守卫与软删；**AddMember 侧**在每个 `INSERT INTO user_orgs` 的**同事务内、插入前**对 org 行取 **`FOR SHARE`** 并复核 `deleted_at IS NULL`，0 行则 `ErrOrgNotFound`。两锁互斥（`FOR UPDATE` ⊥ `FOR SHARE`）——双向交错连续 **25 × 2 轮零残留**。
> ③ **⚠️ 两侧都必须改**：只改 Delete 侧（B 证）或**只改 AddMember 侧（E 证）均仍残留**；单侧锁一律不闭合。
> ④ **落点共 4 处** `INSERT INTO user_orgs`：`AddMemberWithRole`(`org_repo.go:154`)、`AddMember`(`:107`)、`SetUserOrgsTx`(`:238`，已有存活谓词、**只缺锁**)、`SetOwnersTx`(`:564`)；同型守卫 `DeleteVgWithOwnerCleanup`(`:629-671`) 亦需加认领。**漏一处即重新开窗。**
> ⑤ **可选最小单语句**：PG 允许 `INSERT INTO user_orgs(…) SELECT … FROM organizations o WHERE o.id=$2 AND o.deleted_at IS NULL FOR SHARE`（实测语法 OK 且闭合）——但仍须 Delete 侧同时 `FOR UPDATE` 认领。
> ⑥ **触发器方案（备选）不推荐**：`BEFORE INSERT` 在 FK 检查**之前**执行，触发器内普通读在 READ COMMITTED 下同样看不见未提交软删 → 必须**自身取冲突锁**（等于把锁搬家）；`AFTER` 触发器晚于 FK 检查、不可用。

### 测试缺口（QA 报告，3 P0 + 9 P1）

- **P0-3 · 工单 handler 零 Go 测试**：`internal/handler/ticket_handler.go`（567 行，全项目最大 handler）无任何测试；`make test-unit/integration` 均不跑验收脚本 → CI 无法捕获 handler 回归。已登记 BK-19。
- **P0-2 · 乐观锁冲突路径零断言**：user/role/org/menu 四表 `version` 机制**已实现但无一测试**断言「陈旧 version → 409」；唯一引用在 `handler/errors_test.go:27`（错误码表）。
- **P0-1 · 回调幂等无并发测试**：唯一幂等用例是**顺序三步**（`jobs_integration_test.go:157-187`），从未并发触发同 `task_id` 回调。
- **P1**：分页回显未 clamp、网关点段穿越未测、JWT Redis 故障语义未测、并发写工单/类型模板未测、委托环并发未测、审计中间件端到端写库未测、L1 越权仅覆盖 1 条路由、`create_move_race` 在 goroutine 内用 `require`（不安全）、`client_test` 断言过弱。
- **量化**：`make test-cover`（仅单测）总覆盖 **19.1%**、`internal/handler` 包均值 **≈14.6%**；`internal/app`/`casbin`/`model`/`config.Load`/`reqid` **零覆盖**；全项目 **0 处 `t.Skip`**（加分项）、**0 处 `t.Parallel()`**（`-race` 只覆盖测试内自建 goroutine）。

---

## 三、按你的三个问题作答

### 3.1 使用场景、测试用例与设计是否完善？

- **使用场景完善**：从 `docs/review/11 §1` 能力矩阵 + `router.go` 反推，**35 个场景中主链全部落地**；未实现项（附件/Phase 3 全量/SSO）文档已显式登记，**不存在"文档吹了代码没有"的幽灵能力**。
- **测试用例**：**主链扎实**——登录/双 Token/并发刷新/三层鉴权/工单状态机/委托都有测试，且相当多是**真并发**（goroutine+WaitGroup，非伪并发）；**0 处 `t.Skip`**。
- **不完善之处（3 类系统性薄弱）**：① 并发用例只覆盖"双写竞态"，**未覆盖"重复提交/乐观锁冲突"**；② **handler 层（HTTP 契约/L1 越权负向）在 CI 里近乎空白**，靠验收脚本兜底；③ `app`/`casbin`/`model`/`config.Load` 零覆盖。

### 3.2 细节实现是否符合要求？

- **符合**：15 项核心能力与设计逐条对齐（三层鉴权、状态机、资源 Registry、ltree 委托、迁移、网关、AK/SK、错误码、request_id）；`go vet` 零告警；**25 处 `db.Begin` 全部配 `defer Rollback`**，多写操作均同事务。
- **不符合（口径级）**：分层意图被穿透——`service` 直持 `pgxpool` + 片状裸 SQL，且 `service → middleware` 反向依赖（4 个 service 文件取 `RoleFetcher/AuditLogger`）；`handler` 引 `repository` 类型（`AuditListQuery`/`UserListQuery`）。
- **文档滞后**：`registry.Authorize` 文档说 Handler 调用、实现是 Service（实现更合理）；`TicketEngine` Port 文档超前；`authz.md` 伪代码签名过时。

### 3.3 是否考虑了常见状况与并发？

**是，且质量高于一般项目**——已核查"机制健全"项：`Move` 全局 advisory lock + 事务内 `FOR UPDATE` + ltree 环检测 + 行数 CAS + 级联同步；`Create` 事务内 `FOR SHARE` 快照 path；状态迁移 CAS；`superadmin 最后一人保护` advisory lock；Redis `GetDel`/`Pipeline`/Lua 原子；无资源泄漏、无 SQL 注入/路径穿越面。

**但仍有 3 个真实缺口**（见 P1 表，均经真机复现），集中在：**回调幂等（P1-1）**、**关联判重（P1-2）**、**配置类写路径无乐观锁（P1-3）**。

> **RT 轮换**一项经复议**移出缺口清单**——属有意设计（见 §二·补）：机制健全侧的 `GetDel` 原子语义本就不错，问题只是其**误伤代价**未登记，已入档。

---

## 四、主理人独立复核（交叉验证）

三份分报告的关键结论我**逐条回验**，结果如下：

| 成员结论 | 我的独立复核 | 结论 |
|---|---|---|
| 工程 P1-2 回调幂等竞态 | 读 `EnsureCallbackRow` = INSERT+独立 SELECT，无原子抢占；确认 `MarkSucceeded` 在 handler 之后 → 窗口=整个 handler 执行期 | ✅ **坐实** |
| 工程 P1-3 关联 TOCTOU | 读 `service.go:514-547` 查重与插入分离、`ticket_repo.go:408-443` 唯一索引仅同向 | ✅ **坐实** |
| 工程 P2-7 RT 重用即删 | 读 `auth_service.go:174-184`：`GetDel` 先删后比对，确认旧 RT 可删当前有效键 | ✅ 事实坐实；但**三轮复议撤销 P1 定性**（属 OAuth BCP 重用检测的有意设计，doc 已登记）→ 归「有意设计」 |
| 工程 P2-1 org Delete 注释不实 | 读 `org_repo.go:340-352`：注释称 `FOR UPDATE`，SQL 确无 | ✅ **坐实** |
| 架构 I-2 护栏键错头 | 读 `jwt.go:117-119`（只查 `X-AK-Access-Key`）+ utils `aksk.go:8-45`（`Authorization: HMAC`） | ✅ **坐实**（非漏洞：路由组不相交） |
| 架构 I-4 handler 引 repository 类型 | 读 `audit_handler.go:25`/`user_handler.go:26`：仅用 `*ListQuery` 类型、**未直调 repo 方法** | ✅ **坐实**（性质轻，非穿透） |
| QA P0-2 乐观锁零断言 | `grep ErrConcurrentModification` 测试侧仅 `errors_test.go:27`（码表），无冲突断言 | ✅ **坐实** |
| QA P0-3 工单 handler 零测试 | `internal/handler/` 确无 `ticket_handler_test.go`（文件 16KB/567 行） | ✅ **坐实** |
| QA P0-1 回调幂等顺序测试 | 读 `jobs_integration_test.go:157-187`：`w1→w2→w3` 顺序调用，无 goroutine | ✅ **坐实** |
| QA P1-8 goroutine 内用 require | 读 `create_move_race_test.go:128,137` | ✅ **坐实** |

**主理人补充发现（三线均未报）**：
- **文档 ↔ 实现不一致**：`docs/phase2/00-implementation-plan.md:361`（BK-18 随手项）称「organizations/roles/tickets **均有 version** + ErrConcurrentModification」，但 **`tickets` 表全仓无 `version` 列**（`000010` 建表未加、无 `ALTER` 补列）——工单实走 **status-CAS**。**该文档表述失实，建议更正**（改为"organizations/roles/menus/users 有 version；tickets 走状态 CAS"）。

**主理人调整记录**：① 工程报告原将 RT 重用列为 P2，我曾据「可用性影响 + 破坏重用检测语义」上调为 **P1-4**；**三轮复议（所有者提出）后撤销该上调**——「旧 RT → 会话失效」恰是 OAuth BCP 推荐的重用检测行为，且 `docs/phase1/02-auth.md` 已作有意设计登记，故**不再计为缺陷**，改为入档登记。② 同时更正我方一处**事实误判**：此前称「docs 未提及 RT 轮换/重用」有误（BSD grep 不支持 `\|` 交替导致漏检），文档 §RT 轮换流程 本已覆盖该设计。

---

## 五、修复路线图（建议）

**第一批（P1，安全/正确性，建议优先）**
1. 回调幂等改**原子抢占**（P1-1）—— 与 QA 补的并发回调测试同批。
2. 关联写入**事务化 + 规范化唯一索引**（P1-2）。
3. 类型/字段/模板三表**加 version + CAS**（P1-3）—— 与 BK-19 handler 测试同批（文档已登记同批计划）。

> ~~RT 轮换改先比对后删（原 P1-4）~~ —— **撤销**：该改法会削弱盗用检测；RT 重用检测属有意设计，**不修**，仅登记已知代价（触发驱动再评估宽限窗口）。

**第二批（测试补盲，与第一批同批最省）**
- 并发回调用例、乐观锁冲突断言、工单 handler httptest、网关点段穿越、JWT Redis 故障、审计端到端写库、L1 越权表驱动、修 `create_move_race` 的 goroutine-`require`。

**第三批（P2 健壮性，触发/随行）**
- 关停 join（P2-2）、processing 上限（P2-3）、限流返回值校验（P2-4）、**org 删除守卫「两侧锁协议」**（P2-1：**不是「补把锁」**——Delete 侧对 org 行取 `FOR UPDATE` 认领 + 4 处 `INSERT INTO user_orgs` 同事务 `FOR SHARE` 复核，**无需迁移**，见 §二 P2-1 注）、分页 `page` 上限归一（P2-10）、`RecordSubmit` 契约（P2-9）等。

**第四批（文档一致性，L2 口径，低）**
- 更正「tickets 有 version」表述；`registry.Authorize` 调用层、`TicketEngine` Port、`authz.md` 伪代码、`service` 分层口径（两份 SSOT 对齐）；互斥护栏 `X-AK-*` 与定版 `aksk` 口径对齐（P2-15）。

---

## 六、未覆盖 / 限制（诚实边界）

1. **首轮未运行门禁/测试**：三线均为**只读静态审查**（QA 复用既有 `coverage.out`；工程跑了 `go vet` EXIT=0）。**二次确认轮已补真机复现**（testcontainers 真 PG + miniredis，临时脚本跑完即删）——P1-1/P1-2/P1-3/P2-1 均实证坐实（原 P1-4 已撤销定性、改为登记）；但 `make acceptance` 四档全量、`-race` 全量仍未跑。
2. **外部依赖未读源码**：`zhuzhao-utils`（jwt/aksk/redis/crypto 等 10 包）在彼仓，登录锁定 Lua/验签/bcrypt/JWT 算法 pin 仅按调用点推断（架构线例外读了 `aksk` 用于 I-2 判定）。
3. **未做安全渗透**：AK/SK 时间窗、常量时间比较、JWT typ-alg 防混淆等按实现表面对齐，未攻击性验证。
4. **前端/Phase 3 暂缓项**未评估实现（未实现）。
5. **并发用例确定性未复跑**：`create_move_race` 依赖 300ms 锁窗口等，慢/快 CI 上可能 flaky，建议 CI 加 `-count=3` 观察。
6. **`doc/` 旧归档**按指令未采信；`swagger` 与路由未逐条对账。

---

*本报告由软件开发团队（主理人 + QA + 架构 + 工程）产出；三份分报告与主理人独立复核笔记见 `.workbuddy/codereview/`。*
