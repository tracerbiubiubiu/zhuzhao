# 13 — BK11 连接池死锁复盘 — 事务窗口嵌套池取的根因、修复与彻底方案（2026-10-26）

> **文档定位**：point-in-time 复盘报告——`TestBK11_CreateVsMoveHammer` CI 挂死（10-24 起 15 run 挂 7 笔）的完整证据链、三层根因、已交付修复（`039aca6`，CI 绿）与彻底方案登记（连接解析跟随 ctx，**待拍板**）。
> **影响面定性**：不止 CI flake——生产 4 核部署 ≥4 并发 Create 同样可永久挂死，属真实并发缺陷，CI 率先照出。
> **同日关联**：新增 CI 环境差坑一条——pgxpool 默认池随核数漂移致池类死锁本地永不复现（机理 §3.3，对冲 = §4 testutil 钉 4，坑注记随本复盘在案）；hotfix 已随 `039aca6` 合入 phase5。

---

## 1. 问题现象与时间线

- **签名**：`test-integration` job 恒 21m±10s 失败——`go test -tags integration -timeout 20m` 挂满超时，`running tests: TestBK11_CreateVsMoveHammer (19m56s)`。
- **时间线**：两个嵌套点随 `50f40a5`（B1-6 批1 file 字段，10-22）/`f055c60`（S6 补遗 human_key，10-23）落地后，**10-24 20:05 起出现首笔挂死**；至 10-26 定位时 15 run 挂 7（≈50%），同代码有绿有红=时序型 flake。
- **测试本体**：`internal/service/ticket/create_move_race_test.go` 锤击——每轮 3 并发 `Create` + 1 `Move`，8 轮，全程 `context.Background()` 无超时，任何一方永久阻塞即挂到全局 20m。

## 2. 定位过程（证据链）

1. CI 失败日志 → 唯一在跑测试 = BK11 锤击；
2. goroutine dump → Move goroutine 停在 `create_move_race_test.go:144`（`orgRepo.Move` 内），Create 栈停在 `service.go:533`（`fileFieldKeys` → `ListTicketTypeFields`）；
3. 代码路径 → `Service.Create` **事务窗口内**两个「走池不走 tx」的嵌套调用：
   - `service.go:325` → `fileFieldKeys` → `TicketRepo.ListTicketTypeFields`（repo 持池）——`50f40a5` 引入；
   - `service.go:334` → `keyGen.Next` → `humankey.Generator` 自持 `*pgxpool.Pool` 做 `CREATE SEQUENCE`+`nextval`——`f055c60` 引入；
4. 池配置 → `testdb_integration.go` `pgxpool.New(connStr)` 无 MaxConns → 默认 `max(4, GOMAXPROCS)`：**CI 4 核 runner=4，本地 M 系列≥8——死锁本地永不复现**；
5. 复现实验（先红）→ 池钉 4 + 未修代码，本地 `-count=5` 于 2m30s 挂死——诊断实证。

> 注：本节与 §3 引用的代码行号（`service.go:325/334/533`、`create_move_race_test.go:144`）均为**挂死现场的修复前历史行号**；修复后现行位置见 §4 与 `039aca6` diff。

## 3. 根因（三层）

### 3.1 直接机制：池耗尽循环等待

坏时序：3 个 Create 各 `Begin`（占 3 连接）并对 org B 行取 FOR SHARE（共享锁互容，全部拿到）→ Move `Begin` 占第 4 连接（池满），其 FOR UPDATE 被共享锁挡住 → 3 个 Create 走到 `fileFieldKeys` 申请第 5 连接 → 池满；唯一能释放连接的 Move 在等 Create 提交 → **循环等待，永久挂死**。ctx 无 deadline、测试无轮次超时 → 挂到 20m。

绿时序（为何 flake）：Move 的 FOR UPDATE 抢先拿到 → Create 在行锁上排队（此时尚不需要第二连接）→ Move 提交放行 → Create 串行复用释放的连接 → 全绿。两种时序五五开，CI 慢机上坏时序高频。

### 3.2 设计层：不变量只存在于命名惯例

「事务窗口禁池调用」在本仓由 `*Tx` 后缀命名惯例承载——无结构保障、无检查项。repo 双轨设计（池方法与 `*Tx` 方法并存同一 struct）使事务窗口里池方法「就在手边、能编译、测试绿」；`humankey` 库自持池连接不接受注入，调用方无法让它进事务。两个快速交付批（各「实际 ~1 天」）自然引入嵌套点，review 检查项里没有这类资源纪律。

### 3.3 环境层：两层防线缺失

- **pgxpool 无默认获取超时**——对照 HikariCP 默认 `connectionTimeout=30s`：同款死锁在 Java 生态是 30 秒显性异常+告警，在本仓表现为静默挂死；生产面 `http.Server` 仅 `ReadHeaderTimeout`（防 slowloris），handler ctx 无服务端 deadline——同类 bug 的表现形态是「挂到客户端断开」的隐疾，而非快速失败。
- **池大小随核数漂移**——本地永不复现（已修：testutil 钉 4）。

## 4. 已交付修复（`039aca6`，2026-10-26）

- **根治面**：两处池调用提到 `Begin` 前——与既有 `applyFetches/validateCustomData/applyFormulas` 三个 pre-tx schema 读同规；`nextval` 本非事务性（回滚烧号语义不变），事务内保留 human_key 写行 UPDATE——原子性不变，repo 层零改动。
- **测试加固**：testutil 池显式 `MaxConns=4`（本地与 CI 同为最严格口径，同类回归本地即暴露）；BK11 锤击每轮 30s 兜底超时（回归以带定位的失败浮出，不再吃满 CI 时限）。
- **验证**：先红（钉 4+未修代码 2m30s 挂死复现）后绿（同条件 `-count=5` 全绿 7.7s，40 次锤击交错全过）；全包 integration `-count=1` 23 包 ok；unit/gofmt/build/vet ✓；CI run `40301487450` 一次转绿（1m46s，恢复正常时长即证明挂死消除）。
- 修复后全仓事务窗口启发式扫描：无残留池调用。

## 5. 修复评估：正确的最小解，但只堵住第一层

- 与业界首选完全一致（HikariCP 文档第一律「绝不持一个池连接再申请第二个——单连接单元工作」）；两个已知取舍无害（schema 读出事务与既有 pre-tx 读一致；失败路径烧号）。
- **边界**：不变量仍是纪律不是结构——下一个快速批同样能把池调用放回事务窗口（B1-6 即先例）。

## 6. 业界做法对照（三档防线）

| 档 | 业界做法 | 本仓现状 |
|----|---------|---------|
| 惯例层 | 「单连接单元工作/事务最短化」——各连接池官方共识第一律 | ✅ 本次修复 + `*Tx` 命名惯例 |
| 结构层 | Spring `@Transactional` 传播：嵌套调用自动加入当前事务复用同连接（`REQUIRES_NEW` 是显式例外=Java 圈同款死锁经典源）；Go 对应 [avito-tech/go-transaction-manager](https://github.com/avito-tech/go-transaction-manager)（tx 存 `context.Context`，repo 从 ctx 取连接，`Propagation*` 语义同 Spring）；sqlc 的 `DBTX`/`WithTx` 模式 | ❌ 双轨 repo 无约束——bug 是设计邀请来的 |
| 兜底层 | HikariCP `connectionTimeout` 默认 30s + `leakDetectionThreshold` 泄漏检测 + 池指标；请求级 deadline（超时中间件） | ❌ pgx 池无默认获取超时 + 请求无 deadline |

另：`fileFieldKeys` 每次建单读类型字段 schema——定义/配置类数据业界标配进程内缓存（流程引擎缓存已部署流程定义是 Jira/ServiceNow 常态），缓存化后该热路径读从根上消失。

## 7. 彻底方案：连接解析跟随 ctx（自研轻量事务传播器——登记待拍板）

### 7.1 机制

`Begin` 把事务挂进派生 ctx；包装类型 `*dbtx.DB` 的 `Query/QueryRow/Exec` 解析连接时：ctx 里有活事务 → **复用该事务连接**（打 Warn 暴露 auto-join）；无事务/事务已关 → 走池。`Commit/Rollback` 置 closed 后，同 ctx 再调用自动回落池——post-commit 的 `emitEvent` 类场景天然正确。此后「事务窗口内调池方法」**从 bug 变成正确行为**——嵌套获取死锁在构造上不可能。

```go
// internal/pkg/dbtx（示意，~100 行）
func (d *DB) Begin(ctx) (pgx.Tx, context.Context, error)   // 事务挂进派生 ctx
func (d *DB) QueryRow(ctx, sql, args...) Row {
    if b := txFrom(ctx); b != nil && b.open {
        log.Warn("pool call auto-joined tx——建议上移出事务窗口")
        return b.tx.QueryRow(ctx, sql, args...)
    }
    return d.pool.QueryRow(ctx, sql, args...)
}
```

### 7.2 效果判定（四方案对比）

| 方案 | 防线形式 | 结论 |
|------|---------|------|
| hotfix（读上移，已交付） | 人保证不犯 | 下批还能犯 |
| 架构测试/depguard 检查 | 机器告诉你犯了 | 事后检测、启发式可能漏报 |
| TxRepo 类型隔离 | 编译期不让犯 | 更强但 28 个持池点全改接口、跨域可绕 |
| **ctx 连接解析** | **犯不了也无须不犯** | 错误写法自动变对——不变量从纪律变物理 |

正确性兜底与坏味道暴露兼得：每个意外 auto-join 都有 Warn，CI 可断言零 Warn——事务窗口仍应保持最短，只是不再以死锁为代价。

### 7.3 实施条件与改造面

本仓条件异常好：**ctx 已全链路穿透**（ticket_repo 抽查 26/26 查询首参 ctx）→ **repo 方法签名零改动**，仅字段类型 `*pgxpool.Pool → *dbtx.DB`。改造面 = 28 个持池点（19 repo + service/pkg）+ 装配点 + testutil，机械替换 ≈ 0.5–1 天，可按域渐进。

### 7.4 边界（三条）

①事务内真需独立连接 → `context.WithoutValue(ctx, txKey{})` 显式逃逸（当前无此模式）；②auto-join ≠ 鼓励——意外读拉长持锁，Warn 盯住+CI 零 Warn 断言；③并发 goroutine 各自 `Begin` 持各自派生 ctx，天然互不串事务（BK11 三并发 Create 语义不变）。

### 7.5 自研不引库理由

核心机制 ~100 行；[avito 库](https://github.com/avito-tech/go-transaction-manager)带的是 Spring 式七种传播语义/ChainedMW 等用不上的泛化——符合本仓「抄模式不抄架构」惯例（03 号 A.6 同哲学）；想省自研维护引库亦为正当选项。

### 7.6 落地三批（建议）

1. **试点批**：`dbtx` 包 + ticket 域切换（service/repos/humankey）+ BK11 补负向测试（故意在事务内调池读——断言**不挂死**且 Warn 在案）+ 全量 integration 回归；
2. **推广批**：其余 19 repo 机械替换 + testutil；「连接解析跟随 ctx」写入 standards 数据访问节；
3. **互补批**：请求级超时中间件——传播器消灭「嵌套获取」这一类死锁，deadline 兜所有资源类挂死（行锁等待/外部调用），**互补而非替代**。

## 8. 决策状态

| 项 | 状态 |
|----|------|
| hotfix + 测试钉池 + 锤击超时（`039aca6`） | ✅ 已合，CI 绿 |
| 彻底方案 dbtx 连接解析（§7） | ⬜ 待拍板——建议至少做试点批 |
| 请求级 deadline 中间件（§7.6 批3） | ⬜ 待拍板（独立小件） |
| humankey 计数行表化（消每日 SEQUENCE 目录膨胀，顺带可进事务） | ⬜ 挂账，随下次动 humankey 顺带 |
| fileFieldKeys 类型 schema 读进程内缓存（§6 观察——热路径省一次 DB 往返、消一个嵌套风险点） | ⬜ 挂账，随 dbtx 推广批评估或独立小件 |

## 9. 文档衔接与参考资料

- 修复提交 `039aca6`（验证证据全文）/ 改动文件：`service.go`、`create_move_race_test.go`、`testdb_integration.go`
- [03 号 A.6](../phase5/03-s3-flow-semantics.md)（架构纪律工具化先例——depguard 惯例）· [11 号项目控制台](./11-project-control.md)（遗留问题 SSOT）
- 业界：[HikariCP 池死锁/嵌套获取共识](https://github.com/brettwooldridge/HikariCP/wiki/About-Pool-Sizing) · [avito-tech/go-transaction-manager](https://github.com/avito-tech/go-transaction-manager) · [trm 传播语义](https://pkg.go.dev/github.com/avito-tech/go-transaction-manager/trm) · [SQL Transactions in Go: The Good Way](https://blog.thibaut-rousseau.com/blog/sql-transactions-in-go-the-good-way)

## 变更记录

- 2026-10-26 初稿：当日定位→先红复现→修复（`039aca6` CI 绿）→业界对照→彻底方案登记，一气呵成。
- 2026-10-27 增补：**同类残留 Update 修毕**——初稿只修 Create 两嵌套点，外审核出 `Update` 锁内窗口的 `validateCustomData`/`applyFormulas` 仍走池读（`ListTicketTypeFields` 无 tx 变体）；比 Create 更直接：N≥MaxConns 个并发 Update（带 custom_data）各持 1 连接各要第 2 个，**自闭环死锁无需第三方**（生产 4 核默认池=4 即 4 并发中招）。修法=repo `RowQuerier` 接口 + `ListTicketTypeFieldsTx`（`039aca6` 预取方案的姊妹件：不预取、改走 tx 连接，保 F-B1 锁内合并语义不动）；`TestBK11_UpdateHammer`（6 并发×10 轮×MaxConns=4，每轮 5s 兜底）先红（round 0 ctx deadline）后绿（0.12s）。
- 2026-10-27 增补二：**humankey 并发引导竞态**（双锚验证回声）——`TestBK11_CreateVsMoveHammer` 隔离跑（每次全新容器）首轮 6/6 红：当日首个并发 `Next()` 同建 `seq_<prefix>_<date>`，`CREATE SEQUENCE IF NOT EXISTS` 对 pg_class 目录检查非原子撞 23505（duplicate key pg_class_relname_nsp_index）——039aca6 把 keyGen 提事务外使并发窗口更齐（原在 tx 内同样存在，窗口更散）。生产暴露面=每日首两个并发建单同时落地（概率低但真）；修=23505 视同「已被并发方建成」继续 nextval，隔离跑 ×5 全绿。
