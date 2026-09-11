# zhuzhao 代码检查 · 发现二次确认报告

> **目的**：对上一轮《全面代码检查报告》的每条发现做**独立二次确认**——用新鲜证据重核，并刻意寻找"可能判错"的反例。
> **日期**：2026-09-11 ｜ **性质**：只读复核 + **临时复现测试**（跑完即删，未留任何改动）
> **环境**：本轮发现本地 **PG(5432) / Redis(6379) 开发容器在线**，故集成复现走 testcontainers（真 `postgres:15-alpine` + 真迁移 000001–000025）。
> **基线**：`go build ./...` EXIT=0 ｜ `go vet ./internal/...` EXIT=0

---

## 一、确认结论总表

| 编号 | 发现 | 确认方式 | 结论 |
|---|---|---|---|
| **P1-1** | 回调幂等栅栏并发失效 | **真机复现**（8 并发同 task_id） | ✅ **坐实（100% 复现）** |
| **P1-2** | 工单关联反向判重 TOCTOU | **真机复现**（60 轮并发 A→B / B→A） | ✅ **坐实（59/60 轮）** |
| **P1-3** | 类型/字段/模板管理无乐观锁 | **真机复现**（并发 description 编辑） | ✅ **坐实（lost update）** |
| **P1-4** | RT 轮换「重用即删当前有效键」 | **真机复现**（miniredis） | ✅ 行为坐实；但**三轮复议撤销 P1 定性** → 改判「有意设计（RT 重用检测）+ 已知代价」（见 §六） |
| **P1-5** | JWT/AK-SK 互斥护栏键错头 | 代码级确证（读 2 处实现） | ✅ 事实坐实；**严重度下调 P1 → P2**（见 §三） |
| **P2-1** | 组织删除守卫未真正锁行 | **真机复现**（交错事务） | ✅ **坐实（残留成员=1）** |
| P2-2…P2-14 | 关停/健壮性/一致性各条 | 逐条重读代码 | ✅ 全部维持（细节见 §四） |
| **（原 QA P1-1）** | **工单分页回显不一致** | **真机 + 源码复核** | ❌ **部分为误报**——`page_size` 侧不成立（见 §二） |
| 主理人补充 | 文档称「tickets 有 version」失实 | 全迁移扫描 | ✅ 坐实（见 §三） |

---

## 二、❗ 修正：一条误报（分页回显）

**原报告（QA P1-1 / 工程 C-2）称**：请求 `page_size=100000` 时实查 100 条、响应回显 100000；`page_size=0` 回显 0。

**复核结论：`page_size` 部分为误报。** 依据 `internal/handler/ticket_handler.go:46-48`：
```go
if q.PageSize < 1 || q.PageSize > 100 { q.PageSize = 20 }
```
handler 已把越界 `page_size` 归一为 20，后段 `normalizePage`（`user_repo.go:625-637`）再夹到 [1,100]，**回显与实查一致**。故 `page_size=100000`/`=0` 不会不一致。

**真正的不一致只在 `page` 上限**：handler 仅夹下限（`page<1→1`，`:43-45`），未夹上限；`normalizePage` 上限 10000 → 请求 `page=999999` 时实查第 10000 页、响应仍回显 999999。

**修正**：该项目 **P1 → P2**，范围收窄为「`page` 上限未在 handler 归一」。

---

## 三、P1 逐条确认（含实证输出）

### P1-1 回调幂等栅栏并发失效 —— ✅ 100% 复现
**复现**：8 个 goroutine 同时 `signedPost` 同一 `task_id`，handler 内 `sleep(80ms)` 放大执行窗口。
**实测输出**：
```
handler 实际执行次数 = 8（幂等预期 = 1）
HTTP codes=[200 200 200 200 200 200 200 200]   job_submissions.status="succeeded"
```
**结论**：`EnsureCallbackRow`（`INSERT ON CONFLICT DO NOTHING` + 独立 `SELECT`）在并发下**全部放行**，8 个回调**全部执行**了副作用。迁移 `000021` 注释自称「at-least-once 下防重复执行副作用」——**该保证在并发下不成立**。

### P1-2 关联反向判重 TOCTOU —— ✅ 59/60 轮复现
**复现**：60 轮并发 `CreateRelation`（A→B 与 B→A，同 relation_type），每轮清场。
**实测输出**：`=== P1-2 复核结果：59/60 轮出现「双向重复关联行」 ===`（每轮 `rows=2`，双方 `err=<nil>`）
**根因坐实**：唯一索引 `uq_ticket_relations_pair (source_ticket_id, target_ticket_id, relation_type)`（`000016:16`）**是方向性的**——`(1,2)` 与 `(2,1)` 是两个不同元组，DB 拦不住；反向判重全靠应用层非事务预检。

### P1-3 类型管理无乐观锁 —— ✅ lost update 复现
**复现**：两管理员并发改同一类型的 `description`（避开 states/transitions 校验）。
**实测输出**：
```
A(err=<nil>) 提交「管理员A的描述」 / B(err=<nil>) 提交「管理员B的描述」
最终落库 description="管理员A的描述"
```
**结论**：两次并发写**均成功、无 409**，B 的修改被静默覆盖。坐实根因：DDL 无 `version` 列 + SQL 无 version 谓词 + 请求体无 version 字段（`ticket_repo.go:542-564`）。

> 附带发现：该 `/ticket-types` 更新对 `states/transitions` 有**一致性校验**（非法 states 被 400 拒绝）——这是**正面**证据，说明管理面有防呆，只是缺并发冲突检测。

### P1-4 RT 轮换「重用即删当前有效键」 —— ✅ 行为复现，⚠️ **定性经三轮复议撤销 P1**
**复现**：miniredis；键 `refresh:<uid>:<dev>` 存入**当前有效 RT** 的 hash；复刻 `auth_service.go:174-184` 关键两行提交**旧 RT**。
**实测输出**：`函数返回 invalid=true（预期 true）；当前有效键是否仍存在=false`
**行为结论**：旧 RT 提交 → `GetDel` 先删掉当前有效键 → 比对失败返回 invalid，**该设备当前会话被强制失效**。（多标签场景可自触发：A 刷新成功后，B 携旧 RT 迟到提交 → 双双失效。）

> ⚠️ **定性更正（2026-09-11 三轮，所有者复议）**：本行为**不是缺陷**——它是 **OAuth BCP / Auth0 / Google 的「RT 重用检测」有意设计**（重放即盗用信号 → 会话失效强制重登），且 **`docs/phase1/02-auth.md` §RT 轮换流程「RT Reuse Detection（业界对照）」早已登记**。残留真问题仅为**多标签迟到提交误伤**（已知代价），已改为「有意设计 + 已知代价」入档。**详见 §七。**

### P1-5 JWT/AK-SK 互斥护栏键错头 —— ✅ 事实坐实，**严重度下调为 P2**
- 事实确认：`internal/middleware/jwt.go:117-118` 只查 `X-AK-Access-Key`；`X-AK` 全仓仅出现在 `jwt.go:28,118` 与 `jwt_test.go`；而定版 utils `aksk` 用 `Authorization: HMAC Credential=...`（`aksk.go:39-45`）。故互斥护栏对真实方案真空——**事实成立**。
- **下调理由**：该护栏**不可利用**——AK/SK 走独立路由组（`/internal/jobs`），与 JWT 路由组不相交，无放行面。且代码注释自述「M2M 未上线（20009 待 Phase 3）」，属**未上线方案的陈旧占位**。性质是**文档/代码口径滞后**，非运行时 P1 级缺陷。
- **结论**：归入「文档一致性」类，**P2**；随架构线建议（按 Authorization scheme 统一判定）一并处理。

---

## 四、P2 逐条确认

| # | 结论 | 复核依据 |
|---|---|---|
| **P2-1 组织删除守卫未锁行** | ✅ **真机坐实** | 交错复现：未提交成员插入 + 并发 `Delete` → `Delete err=<nil>、组织已软删=true、残留成员行=1`；且 `Delete` **未被阻塞**（证明守卫 SELECT 无 `FOR UPDATE`、UPDATE 未被 FK 的 `FOR KEY SHARE` 挡住——该「不冲突」已由 §八 的 T1 独立实测确认）。**与文档「B4-5 已消灭该窗口」矛盾**；**修法已更正为「两侧锁协议」**（见 §八） |
| P2-2 关停未 join 判定日志管道 | ✅ 维持 | `app.go:67,84-99` 无 WaitGroup；`policyeval.go` drain 与 `wire_gen cleanup` 并发 |
| P2-3 flusher 失败致 processing 无界增长 | ✅ 维持 | `policyeval.go:177-218`：插入失败 `return` 不 trim，下轮继续 `LMove` 灌入 |
| P2-4 限流 Lua 返回值缺长度校验 | ✅ 维持（**低概率、防御性**） | `ratelimit.go:86-87` 无 `len(vals)` 检查；正常路径恒返回 2 元组，属加固项 |
| P2-5 RBAC 继承不变量 check-then-act | ✅ 维持 | `rbac_service.go:124-138` 守卫在事务外；`role_repo.Update:152-175` 单独提交（行内 version 保护不了跨行不变量） |
| P2-6 Casbin 多实例策略陈旧 | ✅ 维持（单副本无碍） | `enforcer.go:30-37`：仅启动 `LoadPolicy`，从未 `StartAutoLoadPolicy`；`cleanup` 的 `StopAutoLoadPolicy`（:35）为 no-op |
| P2-7 会话吊销 SCAN+DEL 非原子 | ✅ 维持（**已缓解**） | 由 `user:disabled` 键兜底，残留 RT 仍被拒 |
| P2-8 限流时间源用应用侧 `time.Now()` | ✅ 维持 | `ratelimit.go:79` |
| P2-9 `RecordSubmit` 契约与注释不符 | ✅ 维持 | `job_submission_repo.go:51-66`：`ON CONFLICT DO NOTHING ... RETURNING` 冲突时无行 → `ErrNoRows` → 报错，与注释「返回既有行、不算错」矛盾（现被调用方吞掉，无可见影响） |
| P2-10 审计按天筛选 UTC 边界 | ✅ 维持 | `audit_handler.go:39,47` `time.Parse("2006-01-02",…)` → UTC 零点；东八区偏移 8 小时 |
| P2-11~14（错误吞掉/同步审计尾延迟/参数放大 等） | ✅ 维持 | 逐条重读，多为已文档化取舍 |

---

## 五、主理人补充发现的确认

**文档失实坐实**：全迁移扫描各表 `version` 列分布 → **`users` / `roles` / `organizations` / `menus` 有 `version`；`tickets` 无**（`000010_ticket.up.sql` 无该列，全仓无 `ALTER` 补列）。
而 `docs/phase2/00-implementation-plan.md:361`（BK-18 随手项）写「与全仓惯例（organizations/roles/**tickets** 均有 version + ErrConcurrentModification）不一致」——**`tickets` 部分失实**（工单实走 **status-CAS**，是有意设计）。建议更正为「organizations/roles/menus/users 有 version；tickets 走状态 CAS」。

---

## 六、三轮复议：P1-4 撤销定性（2026-09-11，所有者提出）

**触发**：所有者对 P1-4 的定性提出异议——认为「RT 重用 → 会话失效」是**安全特性**而非缺陷。

**复议核查**（架构线，doc-only）：
1. 读 `internal/service/auth_service.go`：
   - `Refresh`（145-187）：`GetDel(key)` → hash 不匹配 → 401；`issueTokenPair`（291-312）：`Set(key, hash(newRT))`。
   - `refreshKey`（326）= `refresh:%d:%s`（userID, deviceID）→ **单设备单槽位**，非跨设备令牌族。
2. 读 `docs/phase1/02-auth.md` §RT 轮换流程（70-92）：**已存在** `>` 注记「**RT Reuse Detection（业界对照）**」——明确写「Auth0 / OWASP ASVS 推荐…吊销整个 token family；Phase 1 用 `GetDel` 原子读删…**不追踪 family**…这是**可接受的 Phase 1 安全级别**；Phase 2+ 可嵌 `family_id`」。
3. 测试用例表（745-755）**已列**：「旧 RT 失效 → 401 + 20004」「并发双刷新 → 仅一次 200」。

**复议结论**：
| 维度 | 判定 |
|---|---|
| 「旧 RT → 401 + 20004」 | ✅ **有意设计**，OAuth BCP 重用检测正统行为，**doc 已登记** → 不计缺陷 |
| 「非当前槽位 RT → 连带废掉当前有效会话」 | ⚠️ 属**重用检测的安全代价**（强制重登），可接受 |
| 「多标签迟到提交 → 双双重登」 | ⚠️ **真残留问题，但属已知代价**（并发时序、非确定性）→ **登记不修** |
| 原报告建议「先 GET 比对、命中才 DEL」 | ❌ **该修法不可取**：会使 replay 退化为 no-op，**削弱盗用检测**（与最佳实践相悖） |

**处置（已执行，doc-only）**：
- `docs/phase1/02-auth.md`：就地修订表述（原「key 不存在」→「GetDel 删键 + hash 不匹配」），补登**多标签误伤代价**与 **Phase 2+ 演进选项**（宽限窗口 / family_id，并注明宽限窗口削弱检测的取舍）。
- `docs/review/11-project-control.md`：§6 健康状态新增 `RT-1`（✅ 设计内，2026-09-11 定性）；§8 随手项追加触发驱动条目（触发条件 = 出现多标签体验问题的真实反馈）。
- **未改任何代码 / 迁移**（`git diff --stat` 仅 `docs/` 两份）。

**自纠**：本轮同时更正前文一处**事实误判**——此前称「docs 未提及 RT 轮换/重用」（§一总表 / 首轮报告）有误，实为 **macOS BSD `grep` 不支持 BRE `\|` 交替**导致漏检；`docs/phase1/02-auth.md` §RT 轮换流程 本已覆盖该设计。

---

## 七、方法与边界

- **本轮复现手段**：临时集成测试（testcontainers 真 PG）+ miniredis 单测，**跑完已全部删除**，工作树无残留（`git status` 仅交付物）。
- **仍未验证**：`make acceptance` 四档全量、`-race` 全量；P1-1/P1-2 的**线上触发概率**取决于 taskrunner 重投策略与前端并发行为（本报告证明的是**机制不设防**，非"线上必然发生"）。
- **未覆盖**：`zhuzhao-utils` 外部包（除 `aksk` 外）未读源码。

---

## 八、P2-1 修法更正：锁语义实测（2026-09-11 · 后续轮）

**触发**：所有者指出报告隐含的 P2-1 修法（「给守卫 SELECT 补 `FOR UPDATE`」）**是错的**。

**证据一 · 锁冲突关系**（本机开发库 PG 15.18；A 会话持锁 + `pg_sleep`，B 会话 `lock_timeout=400ms` 试探是否阻塞）：

| # | A 侧持锁 | B 侧试探 | 结果 | 含义 |
|---|---|---|---|---|
| **T1** | 软删 `UPDATE … SET deleted_at`（**`FOR NO KEY UPDATE`**） | `INSERT INTO user_orgs`（FK 检查 = **`FOR KEY SHARE`**） | **未阻塞（不冲突）** | 软删**挡不住**外键插入 → 该窗口本就敞着 |
| **T2** | 守卫 `SELECT … FOR UPDATE` | 同上 `INSERT` | **阻塞（冲突）** | `FOR UPDATE` 确实使插入阻塞（`CONTEXT` 原文：`… "organizations" x WHERE "id" = $1 FOR KEY SHARE OF x`） |
| **T4** | 守卫 `SELECT … FOR SHARE` | 同上 `INSERT` | **未阻塞（不冲突）** | 照搬既有 `FindByIDForShareTx` 的 `FOR SHARE` **也挡不住** |
| **T3** | 软删（`FOR NO KEY UPDATE`） | AddMember 显式 `FOR SHARE` | **阻塞（冲突）** | ✅ AddMember 侧该用的锁 |
| **T5** | 软删（`FOR NO KEY UPDATE`） | AddMember 显式 `FOR KEY SHARE` | **未阻塞（不冲突）** | ⚠️ 易踩的坑：`FOR KEY SHARE` **无效** |

**证据二 · 端到端交错复现**（testcontainers 真 `postgres:15-alpine` + 真迁移 000001–000025；临时集成测试跑完已删）：

| 用例 | 场景 | 实测结果 |
|---|---|---|
| **A 基线** | 原实现 | `deleted=true members=1` ← 残留成立 |
| **B 「仅守卫加 FOR UPDATE」** | 守卫重构为 `SELECT … FROM organizations o WHERE o.id=$1 FOR UPDATE` | `deleted=true members=1` ← **仍残留（证伪「补锁」）** |
| **C1 两侧协议 · Delete 先到** | Delete 认领 `FOR UPDATE` → 守卫 → 软删 | 25 轮**零残留** |
| **C2 两侧协议 · AddMember 先到** | AddMember 先提交 → Delete 守卫拦下 | 25 轮**零残留**（组织不被删） |
| **D PG 语法** | `INSERT … SELECT … FOR {KEY SHARE / SHARE / NO KEY UPDATE / UPDATE}` | 均 **OK**（最小单语句修法可行） |
| **E 「仅改 AddMember 侧」**（Delete 不认领） | AddMember `SELECT … FOR KEY SHARE` + 存活谓词 | `rows=1 deleted=true members=1` ← **仍残留** |

**结论**：**单侧加锁一律不闭合**（B、E 双证）。**真修法 = 两侧锁协议（无需迁移）**：Delete 侧对 org 行取 **`FOR UPDATE`** 作**事务性认领**（随提交/回滚自动释放，**不必新增 `deleting` 状态列**；`status` 另有语义、勿复用）→ 再跑三 COUNT 守卫与软删；AddMember 侧在**每个 `INSERT INTO user_orgs` 的同事务内、插入前**取 **`FOR SHARE`** 复核 `deleted_at IS NULL`（0 行 → `ErrOrgNotFound`）。`FOR UPDATE ⊥ FOR SHARE`（矩阵）→ C1/C2 零残留。**落点共 4 处**：`org_repo.go:107 / 154 / 238 / 564`；同型守卫 `DeleteVgWithOwnerCleanup`(`:629-671`) 亦需认领。触发器方案须**自身取冲突锁**，否则 `BEFORE INSERT` 的普通读同样漏看未提交软删。

> 本轮**未改动任何生产代码、未新增迁移/文件**；临时测试已删除（`git status` 仅剩既有 docs/deliverables 改动）。
