# 五批代码审计报告（2026-10-21，双线合并版）

> **性质**：五批「审计+测试」双轨代码审计的正式归档（§12-10 产出归档纪律）。本线 = 会话 A（基线 HEAD `c3f1ef9`，范围 HEAD~17..HEAD 18 笔）；并行线 = 会话 B（同一范围独立执行，其 F 系发现已于 `6db1f24` 回声批登记处置）。本报告为两线并账后的统一台账——**F 编号以先登记者为准，本线续编不撞车**（F-C2/F-D3 为本线新增）。
>
> **基线漂移**：审计期间并行线推进 13 笔至 `96bbdbb`（其中 6 笔与审计发现直接相关），本线批 C/D/E 于 `96bbdbb` 复核并逐笔核实修复真实性。P2 三项（F-A1/F-D3/F-D2）已按所有者「按建议」于收口批 `ed53008` 修复。

## 一、分批判定总表

| 批 | 范围（提交） | 判定 | 本线发现 | 并行线发现（处置） |
|---|---|---|---|---|
| A | S5 评审修复链：8a36d66/610fbcf/07b3a01/f255d62/f890b2c/45cd22b/03b0e41 | **通过** | F-A1 P2 / F-A2 P3 / F-A3 P3；七项原评审（P0-1/P1-1/P1-2/P2-1/P2-2/P2-4/P2-5 + M-3/4/5/7/10）**全闭环** | F-A7 GC 二扫无索引（→000049 已修）/ F-A6 Mark 失败降级（已修） |
| B | richtext/B5-4：37941ec/7c29ee5 | **通过** | custom_data 事务外基线并发丢改 + 渲染外链图放行 + custom_data 无总量上限（三项均被并行线 `1d4eb7e`/`710e3e6` 先修——本线核实修复语义：LockCustomDataTx FOR UPDATE 新鲜基线 / 自定义 Image 渲染器仅稳定 URL / GFM 层收口） | F-B1（已修 1d4eb7e，含并发回归闸 5 轮对） |
| C | B5-5 前置件：2ad0937/7182204 | **通过** | F-C2 P3（侧车 env 无校验） | F-C1 侧车无 healthcheck/机器可读 last-success（**挂 B10-4**） |
| D | B5-6 KV：c3f1ef9 + 5fc7f54/9bee2c7 | **通过** | **F-D3 P2**（共享 StorageConfig 数据竞争 + wire/app 指针分裂致 KV 到不了 GC） | F-D1 boot 越界跳过（已修 9bee2c7）/ F-D2 version 乐观锁（挂起→**本批收口**） |
| E | 迁移+文档：70a33af/02e9122/9b4b446 + docs 四笔 | **通过** | F-E1 P3（08 §7 口径残留→doc-fix 已修） | F-E1′（大半撤销——D24/D25 系在档、08 §8 L104 属时序日志） |

**P0 = 0；P1 = 0。**

## 二、P 级清单（终态去重，含处置结果）

| 编号 | 级 | 内容 | 终态 |
|---|---|---|---|
| F-A1 | P2 | GC 一扫「先删对象后清行」——List 与 Remove 之间撞并发 confirm，刚确认对象被物理删（行 confirmed 在库、对象 404） | **已修**（`ed53008`：先条件清行让位 + 墓碑走二扫；两用例：让位断言/墓碑收敛） |
| F-D3 | P2 | ①运行时 Apply 无同步 mutate 共享 StorageConfig（与并发请求读 = data race）②wire 值拷贝与 app &cfg.Storage 分裂——KV 改 orphan_gc_days 到不了 GC（隐藏功能缺口） | **已修**（`ed53008`：ConfigStore atomic.Pointer 快照，三服务+GC 共享单例） |
| F-D2 | P2 | KV 无乐观锁——两 superadmin 并发改同键后写静默覆盖 | **已修**（`ed53008`：000050 version 列 + CAS；stale 期望 400 + 值不被覆盖用例） |
| F-A2 | P3 | confirm 按 TicketID 分流与 presign 按 purpose 分流不对称（avatar confirm 带误 ticket_id 触发无谓 L2） | 开放（0.2 天，随手） |
| F-A3 | P3 | 并发 BindAvatar 留「已归属未挂头像」永久 confirmed 孤儿 | 开放（第三 GC 落地自然覆盖） |
| F-C2 | P3 | 侧车 MIRROR_INTERVAL/RETAIN_COUNT 无启动校验——非法值 sleep 报错后立即重试 = 忙循环 | 开放（0.1 天，随手） |
| F-C1 | P3 | 侧车无 healthcheck/机器可读 last-success（成败只在容器日志） | **挂 B10-4**（并行线登记） |
| F-E1 | P3 | 08 §7 P1 行旧口径 000048 残留 | **已修**（doc-fix 批） |

并行线已修不再列：F-B1（1d4eb7e）、F-D1（9bee2c7）、F-A7（92a4b1f/000049）、F-A6、渲染外链图与 custom_data 封顶（710e3e6）。

## 三、测试矩阵（收口批后终态）

| 命令 | 结果 |
|---|---|
| `go test ./internal/... -race -count=1`（16 包） | 全 ok |
| `go test ./internal/{service,storage,ticket,handler,repository}/... -tags integration -race -count=1` | 全 ok（含 RustFS e2e、GC 新旧用例、settings F-D2 两用例） |
| 迁移 000050 独立 testcontainers up→down→up | 零漂移 |
| `make lint` | 绿（gofmt 漂移修复后完整重跑） |
| acceptance 四档 | **未跑**（需 dev 栈，owner 侧执行） |

## 四、批 B 深读记录（正式版，此前按令压后）

- **渲染安全**：goldmark 默认档（内嵌原始 HTML 整体丢弃）+ GFM Table；bluemonday UGC 二道；`AllowRelativeURLs(true)` 放行 `/api/v1/files` 稳定 URL。并行线 `710e3e6` 再加自定义 Image 渲染器（外链/协议相对图降级 alt 文本——探针实证 bluemonday 把 `//host` 归相对 URL 放行且属性策略 OR 合并无法收紧，产出点收口是唯一可靠位）。双保险成立。
- **custom_data patch**：传入键覆盖/null 删键/合并后整体 G2 重校验（防基线外字段绕进、required 被清空）；未知键放行=向前兼容（Create 既有语义）。
- **编辑增量绑定与建单一致性**（a 案）：同走 `bindAttachmentsTx`→`BindCreateAttachments`（FOR UPDATE 行锁 + purpose 翻正式态条件更新 + RowsAffected=0 拒）；UpdateTx 的 UPDATE 先持工单行锁再 bind——计数校验在锁内，无 TOCTOU。**并发丢键窗口**（事务外 GetByID 基线）由 `1d4eb7e` LockCustomDataTx 收口（本线核实：FOR UPDATE 新鲜基线 + G2 校验先于写，含 5 轮对并发回归闸）。
- **第八类型契约**：`richtext` 入八类型白名单（`type_admin.go`）；入站只收字符串 + 256KiB 上限；S1 调色板/UI 渲染归 B5-4 前端批。

## 五、与 §12-10 检查类型学对表

- 三档分层：本审计=**深读轮**（五批逐批静态+动态）为主，叠**横切一致性扫描**（外审核验/旧口径扫描/迁移编号三处同口径）；docs 四笔=锚点抽查。
- 可复用模式：**级联残留映射对照**（D24/D25 悬空核验、08 §7→§8 口径级联=F-E1）；**变更记录错位行检测**（SHA 留痕区 vs 正文辖域）。
- **并行会话收尾互检**：本实践本身——两线 F 系并账、编号续编、修复笔逐笔核实。
- 发现分级与处置：P2 全清（3/3 修）、P3 开放 3 + 挂起 1（处置时限在册）；外审截断规则不适用（实施期核验走分级处置）。

## 六、收尾核对

- 开工快照：HEAD `c3f1ef9` 工作树干净 → 终态：三笔新提交（`8ac5541` doc-fix / `ed53008` P2 收口 / `17e2506` standards）+ 工作树干净；测试产物零残留（testcontainers 自动清理、迁移验证容器已 rm）。
- 待办移交：F-A2/F-A3/F-C2 三项 P3 开放（合计 0.5 天）；acceptance 四档 owner 侧跑；push 单独指令。
