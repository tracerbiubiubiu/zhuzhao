# runbook · 故障处置与运维手册


> **端口分配**：全生态端口权威表见 [ops/port-allocation.md](port-allocation.md)（冲突处理指引同页）。
> **定位**：真实事故与演练的沉淀位（`docs/README.md` 承诺的 Phase 3 欠账，2026-10-12 建档）。
> 条目纪律：**写「验证过的事实」**——每节要么实机演练过、要么真实事故复盘过；未验证的设想写 `docs/` 对应设计文档，不进这里。
> 维护：每次演练/事故后追加记录行；结论变更就地修订并注日期。

---

## 1. superadmin 密码恢复（break-glass）

> 本节是故障复原，不是普通多账号测试入口。E000001仅可按§4批准的CLI复原/acceptance/globalSetup例外收敛到既定终态；人工测试改密用专属账号。示例中的容器与用户名先核当前目标，用户名是显示字段，手工SQL尤其必须先锁定唯一用户ID与superadmin角色，禁止按可能同名的username批量更新。

### 1.1 适用场景与授权模型

- **场景**：superadmin 用户密码遗忘/疑似泄露，且无其他可用 superadmin 可走管理端 `POST /api/v1/users/password/reset` 代重置。
- **授权模型**：不走 HTTP/会话，凭**容器执行权**授权——能 `docker exec` 进 app 容器者本已具备宿主级权限，工具不新增提权面（这也是不采用 HTTP break-glass 端点的原因：那会给认证系统开新攻击面）。
- **能力边界**：仅接受**持启用中 superadmin 角色**的用户（普通用户走管理端重置，不得绕层级防护）；目标若处于禁用态则连带启用；重置后 `must_change_password=true`，下次登录强制改密。

### 1.2 首选路径：CLI 子命令

同二进制子命令分发（`deployments/Dockerfile` 零改动；2026-10-12 起的镜像自带）。

```bash
# 容器形态（演示/生产 compose，容器名 zhuzhao-app）——交互式
docker exec -it zhuzhao-app /app/zhuzhao reset-password --username admin

# 容器形态——管道式（共享终端推荐：密码不经回显也不进 shell 历史）
printf '新密码\n' | docker exec -i zhuzhao-app /app/zhuzhao \
  reset-password --username admin --password-stdin --yes

# dev 宿主形态（make dev 栈，DB/Redis 经 configs/config.yaml + env 寻址）
go run ./cmd/server reset-password --username admin
```

参数：`--username`（必填）/ `--password-stdin`（管道读一行密码）/ `--yes`（跳过交互确认）。

工具执行顺序 = 与 HTTP 侧重置完全相同的语义：目标校验 → bcrypt 置换 + `must_change_password=true` → （若被禁）连带启用 → **全端会话吊销**（disabled 标记 + 密码纪元 INCR + refresh 键清除，短退避重试 3 次）→ `audit_logs` 落行（`method='CLI'`, `path='break-glass:reset-password'`）。

### 1.3 密码输入方式注记

交互形态为**明文双录**（stdlib 无关闭回显能力，为零新增依赖的取舍）：控制台可见输入，**共享终端/录屏环境一律改用 `--password-stdin` 管道形态**。

### 1.4 会话吊销失败的对账（CLI 输出 ⚠️ 时执行）

吊销失败不阻断重置（DB 已提交），残留 token 须手工清理（对齐 `internal/service/session_revoke.go` 三步语义）：

```bash
# ① 清 refresh 键（实际键名以 scan 结果为准）
docker exec zhuzhao-redis redis-cli -a "$REDIS_PASSWORD" --scan \
  --pattern 'refresh:<user_id>:*'
# ② 密码纪元 +1（旧 RT 携旧纪元，下次 Refresh 即拒）
docker exec zhuzhao-redis redis-cli -a "$REDIS_PASSWORD" incr user:pw_epoch:<user_id>
# ③ 存量 AT 依赖 disabled 标记拒答（TTL=access_ttl，与 JWT 配置对齐）
docker exec zhuzhao-redis redis-cli -a "$REDIS_PASSWORD" set user:disabled:<user_id> 1 ex 900
```

应用日志中带 `tag=reconcile` 的 ERROR 行即吊销失败现场（`session_revoke.go` 重试耗尽时落）。

### 1.5 台账要求

- CLI 正常路径已在 `audit_logs` 落一行（`method='CLI'`），但**操作者身份审计不覆盖**——须人工台账补记：时间/操作者/目标账号/触发原因/新密码交付方式。
- CLI 输出「审计落库失败」告警时，台账为**唯一**留痕，必须补记。

### 1.6 兜底路径：手工 SQL（镜像过旧无子命令时）

> ⚠️ 此路径**绕过应用层一切留痕与会话吊销**，以下步骤**全部**为必做，缺一即留敞口。

手工路径先只读确认目标唯一ID、启用的superadmin角色、当前状态及回退信息，生成新hash后在维护窗口按该ID执行；下列为必须替换占位值的SQL轮廓。不要以可重名的显示用户名作为更新谓词。

```sql
BEGIN;
-- <verified_id> 必须是前置核验的唯一superadmin目标；不是普通测试账号操作入口
UPDATE users SET password = '<hash>', must_change_password = true,
                 status = 1, updated_at = NOW()
WHERE id = <verified_id> AND deleted_at IS NULL;
-- 核对恰为一行后补审计；任一步不符则ROLLBACK
INSERT INTO audit_logs (user_id, username, method, path, status_code, duration, user_agent, request_body)
SELECT id, username, 'CLI', 'break-glass:reset-password(sql)', 200, 0, 'manual',
       '{"trigger":"break-glass","tool":"manual-sql"}'
FROM users WHERE id = <verified_id> AND deleted_at IS NULL;
COMMIT;
```

**④ 手工会话吊销（必做——直改 SQL 不会吊销任何 token）**：按 §1.4 ①②③ 执行。

### 1.7 演练记录

| 日期 | 环境 | 范围 | 结果 |
|---|---|---|---|
| 2026-10-12 | dev 栈（交付批演练） | CLI 全链：重置→强改密→refresh 清除→纪元 +1→审计落行；非 superadmin 拒绝；禁用连带启用；目标不存在 | ✅ 6/6 用例过（`internal/pkg/admincli` 集成测试为常态门禁，随 `make test-integration` 持续回归） |
| 2026-10-12 | dev 栈（**真实使用**） | 并行会话致 admin 凭据脱离验收态——CLI 一键重置回规范值 `admin12345` 并吊销存量会话 | ✅ 复原即用；首个真实场景实战 |

> 演练捕获的操作知识：① CLI 重置后 `must_change_password=true`，如需直接清除强制改密标记，走 `POST /auth/password/update` **两步轮换**（新==旧被 B2-2 拒绝，须先改临时值再改回）；② CLI 只做连接三节最小配置校验，不受服务端启动 fail-fast（internal_jobs SK 等）牵连——该分离即本批由演练抓出后落码（config.Load 保持读+默认值，启动检查归 Validate）。

---

## 2. Phase 3 故障演练结论（2026-09-14，四项全过）

> 原始记录：`docs/review/reports/phase3-closure-report-2026-09-15.md`（阶段 E）。

1. **kill 后队列续跑**：taskrunner 容器 kill 后任务不丢，重启续跑。注意口径：**`kill` = 人工停止**——`unless-stopped` 策略不会自动拉起被人工停止的容器（编排器语义），恢复须 `docker start` / `compose up`。
2. **PG 自愈**：PG 容器重启后，zhuzhao / taskrunner 连接池自动恢复，无需重启应用。
3. **dead 重驱**：造死法 = compose `MAX_RETRY` 旋钮临时置 0 + 任务自指黑洞（`127.0.0.1:1`）；重驱后终态/重试链恢复。`MAX_RETRY` 用完即回原值。
4. **SIGTERM 排空**：收到 SIGTERM 后 0.237s 完成在途任务排空并干净退出（`ShutdownTimeout` 生效实证）。

## 3. 备份与恢复要点（2026-09-14 运维补强批）

- **每日备份**：zhuzhao/activelist 各带 pgbackup 侧车（RETAIN_COUNT 统一）。⚠️ 教训（「三日假备份」事故）：postgres 无 restart 策略时停机不自启 + 旧脚本失败静默 done 0——**备份必须定期恢复验证，不能只看文件存在**。
- **恢复演练**：zhuzhao 21 表零差异恢复已实测。坑：shell 循环 `docker exec` 采表会吞 stdin（只采到 9 表）——用 `gexec` + heredoc 单会话采全。
- **PITR**：`pg_dump` 逻辑备**不可**回放 WAL，须 `pg_basebackup` 物理基备（tar 流 stdout → 容器内落盘 + `docker cp`）；卷根 = 数据目录根勿套层；`recovery.signal` 手建 + chown 999；目标时刻若落在未归档段，活库 `pg_switch_wal` 切段后重启即解。
- **对象存储备份**（2026-10-20 B5-5）：`rustfs-mirror` 侧车 rclone sync 主→备 + 双重删除保护（`--backup-dir` 归档轮转 + 目标桶版本化）+ 失败不静默；**RPO = 同步周期**（dev 1h）——对象存储无 WAL 类日志、PITR 不可得，属显式登记口径。恢复步骤与演练记录见 [docs/ops/backup.md](backup.md)「对象存储备份与恢复」。
- **存储栈升级纪律**（2026-10-27 A-2 拍板 C-11 b 案落行）：动存储栈（RustFS 升级 / minio-go 或 rclone 客户端升级 / 镜像换版）**必跑附件全链验收**（头像上传绑定 / 附件上传下载删除 / richtext 贴图渲染三线 + `make acceptance` S5 面）——兼容劣化信号必然以报障显形，观测者=执行升级的人，本行即其检查单。
- **存储用量观测**（2026-10-27 观测批）：应用侧 `/metrics` 暴露 `zhuzhao_storage_usage_bytes`（按 purpose——`file_objects` SQL 采样）+ 越限 `ops.storage_usage_high` 事件（种子规则收件人空=管理员在消息管理面配置后生效）。运维侧备选：RustFS 原生 `/minio/v2/metrics`（MinIO 兼容端点）**兼容性未实测**——实测通过后磁盘容量水位可零开发接入抓取端，登记在案勿默认其存在。

## 4. dev 栈测试账号台账（2026-10-13 统一口径）

> **性质**：dev 栈公开测试凭据（生产凭据一律 env 注入，与本表无关）。**唯一权威 = 本表**；
> 各仓套件（后端 acceptance / ui E2E / break-glass CLI）全部收敛到同一终值，禁止私设终态。

### 4.1 账号清单

| 账号 | 工号 | 密码 | 用途与归属 | 改密许可 |
|---|---|---|---|---|
| admin（superadmin） | E000001 | `admin12345` | **全生态唯一终态**：后端四档 acceptance / ui E2E global-setup / break-glass CLI 三处收敛同值 | **仅限三处**：① phase1 幂等改密流 ② ui `e2e/global-setup.ts` 收敛逻辑 ③ break-glass CLI 复原（§1）。**人工与 agent 的多账号测试禁止改 E000001**——用 E000002+ 专属账号 |
| manager | E000002 | `Adm2#2609x`（用户自管） | 日常多账号手动测试（admin 角色，不动 E000001 的正确姿势）。**2026-11-10 复建**：此前 dev 栈卷重建后账号消失、台账未跟（10-24「密码脱靶」实为账号缺失误诊）——已按本行原值经 admin 面重建并登录验证 | 所有者自管 |
| agent_s2（agent 专属） | E800001 | `Agt1#2610x` | agent 会话 UI 走查专属（admin 角色；2026-10-24 S2 画布走查建号——E000002 台账密码已脱靶且属所有者自管，按 §4.2#2 新建 E8 段） | agent 自管；所有者可随时禁用 |
| agent_eco_chrome（仅独立 test 栈） | E800006 | 同 E800001 的公开测试口令 | 2026-11-06 ECO-B13 真实 Chrome 走查专属 admin；仅 `zhuzhao_test` 主库，未写入 dev/生产 | agent 自管；所有者可随时禁用，不由 E2E global-setup 接管 |
| agent_eco_e2e_admin（test临时准备号，已回收） | E800007 | 原E800001公开测试值；回收时未改口令 | 2026-11-06原样E2E替换身份准备：因套件固定seed用户名/组织，已禁用软删、角色清空、58个RT清除及纪元推进；不复用，证据属ECO-B08 | 已回收；E000001/E800006未变 |
| p5_accept_a（验收甲）/ p5_accept_b（验收乙） | E800002 / E800003 | `P5Accept#2611x` | 2026-11-09 phase5 验收专用（[phase5/12 号](../phase5/12-scenarios-and-tests.md) R0-2 建，角色=P5验收成员 id 311、组织=「P5验收组」id 1227）：甲入组（member/assigned）任处理人/角标/消息中心视角，乙不入组任局外人（L2 不可见/抄送可见不可改）视角。E000002 台账密码已脱靶且属所有者自管，故按 §4.2#2 用 E8 段 | 验收期 agent 自管；走查收尾随测试数据一并处置（保留或禁用由所有者拍板） |
| e2e_operator / e2e_viewer | E2E9020 / E2E9030 | `E2eOper#2609` / `E2eView#2609` | Playwright E2E 专属（global-setup 幂等建号） | 仅 `zhuzhao-ui/e2e/global-setup.ts` |
| S1 演练号 | E2E 段随建 | global-setup 派发 | 强制改密流程演练（每轮重置，无常态） | 同上 |

### 4.2 纪律

1. **E000001 密码终态恒为 `admin12345`**——历史冲突根因 = ui E2E 曾把终态设为 `E2eAdmin#2609`、后端 acceptance 认 `admin12345`，谁后跑谁赢（2026-10-13 一天内三次互改实证）；已翻转为全生态收敛 admin12345（`global-setup.ts` main ① 注释）。
2. **新增测试账号**：工号用手动段 `E8xxxx`（E2E 段 E2E9xxx 归 Playwright）；密码模式参考 `{Role}{N}#{YYMM}x`（≥8 位含大小写数字符号为测试口令约定；当前绑定只强制min=8，不代表复杂度校验已实现），登记进本表。
3. **改密后必须清强制改密标记**（否则 20007 门拦后续操作）：走 `/auth/password/update` **两步轮换**（新==旧被 B2-2 拒，先临时值再回终值）——break-glass CLI 只负责重置，标记清除是独立一步。
4. agent 会话开工多账号测试前**先读本表**；发现 E000001 脱离终态，用 §1 CLI 复原而非再设新值。

### 4.3 节点超时催办观察模式（S10 B10-4——上线后必关）

> **当前提交基线（2026-11-11）**。`29d78a69` 已将 `configs/config.yaml` 的 `tickets.timeout_observe` 固化为 false。存量迁移或新环境首次开启前，仍可显式设 true 观察候选并对账，再关闭观察启用发送。按实际配置来源、环境变量和构建核对生效值，不能凭提交默认推断现有进程状态。
> 首次接入存量数据或重新开启观察时按以下步骤核对。已关闭观察的环境直接核对第 4 步。

1. `GET /api/v1/system/cron-status`（admin）→ 确认 `timeout_scan` 任务存在且 `observe: true`；**顺带核每个任务 `started_at` 非空**（空=已注册未启动——2026-11-01 观测批三 cron 曾注册未拉起、11 个 tick 零执行；f51cd4c 后此字段即「注册≠启动」探测点）
2. 等首个扫描周期（默认 5 分钟）→ 查服务端日志 `timeout_scan: overdue candidate (observe, not sent)` 确认无误报
3. `configs/config.yaml` 置 `tickets.timeout_observe: false`（或 env `TICKETS_TIMEOUT_OBSERVE=false`）→ 重启
4. 复查 cron-status → `observe: false` → 下个周期起实际催办

不关的后果：所有超时催办/SLA deadline 通知**只记日志不发送**——功能静默失效。

## 5. 存量协议兼容层接入（aksk/compat，2026-10-13 建档·10-14 已预接线默认关）

> **性质**：存量协议兼容层接入与验证手册。utils `aksk/compat` 已交付并发布（v0.4.2），zhuzhao 侧已预接线（§5.1，默认关）；本节 = 内网调用场景就位后的密钥配置与上线前比对流程。背景决策：[phase1/02-auth §存量迁移翻案追记](../phase1/02-auth.md)。

### 5.1 预接线状态（2026-10-14 已完成）与剩余两步

**已预接线（默认关）**：config 段 `compat_aksk`（`Enabled`/`Keys` 密钥环；`Validate` 对「启用而密钥环空/含空 SK」fail-closed 拒启）+ `/compat` 专用路由组 + **`POST /compat/echo` 判定靶**（验签通过回显 caller/request_id，无业务副作用）。目标服务 = zhuzhao 主仓。

**剩余两步**：

1. **填密钥开开关**（存量 AK/SK 对出带同步进配置卷，SK 勿入 git；`SkipBody` 先保持默认 false，待 5.2 判定）：

   ```yaml
   compat_aksk:
     enabled: true
     keys:
       LEGACY_AK_1: "<SK 出带同步>"
   ```

2. **真实请求抓包 → §5.2 判定表**（重放靶 = `/compat/echo`）；业务端点随真实场景就位后同组并列。

**纪律**：只挂 `/compat` 专用路由组，**不与内部 `aksk.GinMiddleware` 混挂**（standards §3.11 豁免条款；存量签名不覆盖 method/path，混挂扩大跨端点重放面；路由组只隔离组外——组内须实质单端点或幂等/无副作用）。

### 5.2 上线前向量比对（硬前置）

1. 抓一条**新鲜**真实存量请求（≤5 分钟内，时间窗 ±5min），留存四个 `x-auth-*` 头 + 原始 body 字节。
2. 原样重放（curl 模板，body 与抓包逐字节一致；靶 = 预接线判定端点）：

   ```bash
   curl -v -X POST "http://<服务>/compat/echo" \
     -H "x-auth-access-key: $AK"  -H "x-auth-timestamp: $TS" \
     -H "x-auth-signature: $SIG"  -H "x-auth-body-hash: $BH" \
     --data-binary @captured_body.bin
   ```

3. **判定表**（当前挂载 `onFail=nil` = utils 默认响应：鉴权失败恒为统一文案 `401 {"code":10002,"message":"未授权：AK/SK 签名验证失败"}`，读体超限另为 413/400 + `code 10001`——**响应不分档，分档看 zhuzhao 服务端日志** `compat: verify failed` 的 `err` 字段；如需响应侧分档，经 `GinMiddleware` 自定义 `onFail` 按本包哨兵 `Err*` 实现，勿复用内部 `response.AKSKFail`——两包哨兵不同源，`errors.Is` 恒 false）：

   | 服务端日志 err 分档 | 结论 / 动作 |
   |---|---|
   | （无失败记录，200） | 主路径吻合（复算派），可直接上线 |
   | `signature mismatch`，`SkipBody=true` 后 200 | 对端为 skipBody 形态（body 不入签）——**固定开启 `Verifier.SkipBody`** 再上线 |
   | `timestamp out of allowed window` | 双方时钟差超窗，先校时再比 |
   | `unknown credential` | 密钥环没配对，查 config |
   | `signature mismatch`（开关两态都败） | 线协议与冻结向量不符（分隔符/编码/拼接差异）——**停，回设计对账**，勿强上 |

### 5.3 回退

摘路由组中间件即恢复原状；config 密钥环段保留无副作用（不挂载不生效）。兼容层无 DB、无迁移，删除 = 删依赖 + 删路由组。

## 6. 迁移回滚与 B 案守卫耦合（2026-10-26 外审 #7 登记）

> 自 `45dcb25`（B 案三道守卫）起，app 启动会审计 menu_apis：页面/目录行（menu_type≠3）承载非 GET 绑定即 **拒启**（例外仅 `pageWriteAllow`）。000075/000076/000077 三对迁移的 down 按「回种原状」原则会把写端点**回挂页面行**——所以：
>
> - **只回滚 DB 不回滚代码 = 回滚后 app 拒启**（错误信息列出违例行与处置）。
> - 正确序：①回退代码到 `45dcb25` 之前（或临时在 `pageWriteAllow` 登记待回滚的绑定）→ ②`migrate down` → ③启动。反向（先 down 再回代码）同样可行，但中间窗口服务不可启。
> - 往返门禁（`scripts/migration-roundtrip-check.sh`）在全新库上 down→up，不经 app 启动，故不受影响；这条耦合只在**运维回滚**路径出现。

### 6.1 模板种子回滚前的只读依赖预检（B15-F03）

定制规则引用待撤的种子模板时，down 000079会被000061外键拒绝：业务数据保全，但migrate检查点可能成为v78 dirty。相同风险还涉及down 000062/000064/000071/000080；不得自动删除用户配置或盲目force来换取回滚成功，历史SQL保持。

前置为Python3、PATH中的`psql`及显式目标库只读账号。账号需要CONNECT、public schema USAGE、schema_migrations/message_rules SELECT及默认catalog可见性，无需写权限。通过安全环境注入`ROLLBACK_PREFLIGHT_DSN`后执行；不把口令原文写入终端历史、提交或审计附件：

```sh
python3 scripts/migration-template-preflight.py --target-version 78
```

工具不取本地默认DSN、不接收argv口令；URI须显式包含用户、单TCP host与数据库，仅允许sslmode/connect_timeout参数，清除继承PG*覆盖。返回0仅表示所选窗口没有存活模板外键引用阻断，**不是整个回滚许可**；1表示存在阻断引用，须由所有者决定保留模板或重新绑定定制规则后重查；2表示未证实（dirty、权限/连接错误、范围缺失、未知FK、历史SQL漂移等），不得当作通过。

支持clean current到本地已有且>=61、低于current的target；跨过000061外键/000060版本列边界显式拒绝。五类seed-down按真实DELETE谓词顺序模拟，79带template_code限定；整份已核down文件SHA变化、新增未建模消息迁移或入站FK变化均须先重新核验策略。工具仅执行一个REPEATABLE READ READ ONLY事务并ROLLBACK，不执行migrate/down/force或业务写入；快照不排斥后续写入，实际回滚须协调维护窗口并紧邻down重跑。模板正文删除影响另行审阅。

target<=74时输出`requires_code_rollback=true`，仍须遵守本节B案代码回退耦合。失败down后先核实际数据及dirty检查点，禁止仅凭目标号force。脚本回归入口为`python3 scripts/test-migration-template-preflight.py`；真实PG与项目门禁证据在对应分批审计报告中单列，不能用只读预检代替回滚演练。

## 7. 多会话协作与验收环境纪律（2026-10-26 立约）

2026-11-14 本日收尾（交接，不代表生产发布）：五仓保留main及codex/capability-interconnect-design候选，以[七条输入对账](../review/reports/daily-closeout-2026-11-14.md)核实际PR/远端和旧分支清理；[模块首包](../phase6/module-design-handoff.md)承接下一阶段。旧工作树重要ignored内容与历史SHA已本地备份，原main及当前候选证据/日常环境保留；合main、业务开发和真实企业激活未自动执行。

> 来源=两会并行开发当周的三次互相干扰事故：端口互杀（互相 `make dev` 杀对方服务）/ Redis 于验收中途被重启 / **T7 假红**（一会的 viewer 角色页面绑定实验污染另一会的验收断言）。

1. **端口分治**：33333=标准 dev 实例；33334 历史用于实验二实例，但当前由独立 `zhuzhao_test` 占用，禁止再裸起 `APP_PORT=33334 make dev`。实验须在端口表另取未占用端口，并隔离数据库/Redis；只换 HTTP 端口不构成数据隔离。不抢不杀其他会话进程，验收另守下条独占纪律。
2. **验收独占**：跑 acceptance 前确认无他会在跑（`pgrep -fl acceptance-phase`）；验收期间不对共享 dev 栈做 docker 重启/种子数据/角色绑定变更。
3. **验收夹具自愈**：脚本对「角色零绑定→403」类断言一律用**运行时自建临时角色**（`p2a_viewer_*` / `p2c_viewer_*`，跑完按本轮夹具归属精确回收，不为清夹具重置共享卷），不得依赖 seed `viewer` 角色的绑定状态——它可能被任何会话的功能实验合法修改。phase1 仍用 seed viewer 做角色停用语义夹具，但它自带绑定清理守卫且居于链首。
4. **共享库直改须自清**：实验性 role_menus/种子数据改动在实验结束或验收窗口前按改前快照精确还原；`docker-dev-reset` 会删卷，不能用来清理共享栈实验。
5. **验证环境存疑即重跑**：acceptance 出现「断言值与响应体自相矛盾」「时序相关偶发红」时，先疑环境（另一会在写同一栈）再疑代码——NOPE999 锁污染/T7是已确认的环境案例；不能据此把所有失败都归环境，保留原失败并核源码与响应。
6. **并行会话在途台账**（2026-10-28 立——standards §12-10「会间交接注」登记处）：两会以上并行时，各会话收尾在下表落一行、收口清行；后继会话开工先读。三起立法实证：fe720e1（文档行覆写）/1418f33（staged 索引互扫）/dcf005c（amend 吞并他人提交）。

  | 会话主题 | 在途文件 | 未推送笔数 | 落笔时间 |
  |---|---|---|---|
  | Phase6前五仓重规划 | 五仓独立`codex/pre-phase6-replanning`工作区，仅文档/受控清单；root集成，子审查按仓归属；共享main保持只读。当前无功能实现、远端设置变更或生产发布 | 未推送；提交状态以实时Git为准 | 2026-11-11 |
  | B09来源陈旧下人工协作确认 | 主仓`codex/capability-interconnect-design`窄文档后继；root唯一写入/显式路径提交，子审查只读。F001来源、策略与当前入口已同步，无在途路径/索引占用；专属验收资源已精确回收，29原容器指纹不变 | 只本地提交，不推送/合main；准确提交见Git | 2026-11-13 |
  | F013本地正式先行与质量排期 | 主仓候选同分支；估算/交接子编辑已归还路径，root唯一集成提交/推送，独审只读。无其它会话索引占用；未启动业务实现、部署或改共享资源 | 续用已授权草稿PR；准确提交/远端/CI以Git及收据为准 | 2026-11-14 |
  | F014规划与已验升级合主线 | 子编辑已归还报告路径，root唯一提交/推送及PR集成，独审只读；无其它路径/索引占用。所有者已确认五仓规划与已验证升级一起合main，业务开发/部署/共享栈切换仍不在范围 | 各PR及准确SHA/CI记录实际合入，现有排期仅参考 | 2026-11-14 |

2026-11-13能力互通设计交接：五仓各自隔离的`codex/capability-interconnect-design`基于上一设计/版本候选，仅改docs；子审查按文件归属编辑，root为唯一提交者。本轮只本地提交，不推送/合main/启业务开发或切共享栈；补充方向和未计价范围见[增量册](../review/capability-interconnect-supplement.md)。本轮结束后无文档路径/索引在途占用；后继仍核实时Git，不提交其他会话文件。运行门禁和真实集成不由文档核对代验。

2026-11-13改名前文档整理交接（非在途占用）：继续在五仓隔离候选分支按文件归属处理导航、页头效力和三个Go兄弟仓AGENTS，root单一集成/路径提交；源文件与历史证据路径保留。只校准当前总账资源说明，价格/依赖/排程及冻结hash不改；静态核对不替代运行验收。文档交接后释放路径占用，新产品/仓库名称、module/import、运行资源及物理目录迁移另行处理；未推送、合main或启动业务开发。

2026-11-11 前次交接快照（运行终态由下方正式收尾段更新）。11-08 文档与 11-10 代码修复均已有提交，旧「工作树待提交」与「新档待建」在途行撤下。当时 11-11 自动复验的私有栈已回收，共享 dev/zt、验收账号和数据保留。文档整理、提交清单与远端 CI 收据保存在本机 ignored `.codex/audits/phase5-docs-closeout-20261111/`，本行仅保留后续工作交接。P5 数据处置仅作保留建议，未执行删除或共享卷重置。已记录但尚未修复的问题见 [12 号 §8.2](../phase5/12-scenarios-and-tests.md#82-phase-6-统一重规划与问题移交)。


2026-11-11 合并完成落档（验收走查会话）：**Phase 5 已合入 main 并全量推送**——主仓 `ed081384`（phase5 收官合入，548 笔）、UI `73a18d39`（160 笔）、taskrunner `340c7a6`、activelist `93ecbcb`、utils `b027089`（三兄弟审计分支收官合入）；五仓 main CI 全绿（主仓全门禁 12 分钟套件）；主仓/UI `phase5` 分支已删（本地+远端），三兄弟 `codex/phase5-audit` 保留（审计证据与 utils pin cd759a04 溯源）。过程备注：utils 提交策略不含 merge 类型（用 chore(main) 合并信息）、ui 主题超限曾致一次无合并误推（4377236a=3 行前端规范文档，无害已随入库）、HTTPS 推送空响应复现——三兄弟与主仓 remote 已切 SSH 形态。合并不改运行环境：dev 栈/常驻进程/验收数据按 §8.4 口径保留。

2026-11-11 所有者后续安排。Phase 5 收尾后将进行全项目重新设计规划，可延后事项统一移交 Phase 6。当前全部 8 个已注册工作区和 10 个本地分支已核对，无漏提文件、索引锁或仅存本地的提交。独立历史验证仓与固定源证据继续保留。清单、文档位置核对和提交推送收据存本机 ignored `.codex/audits/phase5-workspace-closeout-20261111/`；本轮仅改文档，运行环境与数据处置延续既有归属记录。

2026-11-11 正式收尾。已删除 zt 九容器与从未启动的 cool_keller，三库逻辑备份经归档目录读取校验，原 zt 八卷和恢复映射保留。另删除五个空网络、五个验证镜像、一个从未启动容器的空卷。45 项临时产物压缩留证并校验 13,530 文件后清理，51 处隔离缓存/产物回收。开发六容器、原生服务、p5-accept 活跃日志、SDK、备份、bundle 与依赖保护引用保留。恢复映射及备份位于本机 ignored `.codex/audits/phase5-final-closeout-20261111/protected/`，完整清单与健康检查同目录上级，最终提交和 CI 以准确 SHA 收据核对。历史 start/stop 记录按当时已有容器理解，恢复旧测试环境需依据原配置与明确卷映射重新创建容器。

2026-11-10 共享资源事件与启停登记（验收走查会话）：①**zt 测试栈 11-09 停、11-10 复起**——11-09 按 root「清理不必要测试环境」指令 `docker stop` 九容器（未删，卷/配置原样），未及时落本台账致并行会话四档受阻；11-10 `docker start` 原样拉起（33334/48081，九容器全 Up）——**纪律补课：共享栈启停须即日落本台账**，恢复优先 `docker start` 现有容器（绕开 `zhuzhao_test` 正式 compose 与 `zz-preserve-phase5` 接管模板的同名互斥，二者归属裁决留 root：日常启停走 start/stop，正式 compose 仅重建用，接管模板维持 root-gated）；②**dev 库漂移清理**：`incident.impact_scope` required 11-03 被实验会话置 true（审计 10:13–10:45 fields/replace）致四档建单 10001——经 root 授权已改回 `required=false`（**字段本身保留**，原实验其余字段零触碰；验收同款建单复现通过后删除验证单）；根修建议=实验一律自建前缀类型（如 `p5_*`），禁改共享类型（12 号 §5.2 后续登记）。

2026-11-08提交检查安装（RR-GIT-01）：Go四仓在当前目标工作树运行`make hooks`，UI运行`pnpm hooks`，需要Python3；installer仅设置当前worktree的相对`.githooks`，保留common配置供尚未承接的main使用，不手动把所有仓统一指向主仓绝对路径。消息在commit-msg检查，pre-push/CI再按真实新增窗口检查存库文本。固定`commit-policy-baseline`只豁免其祖先，不自动前移；显式历史审计不豁免。新增merge也须规范主题与正文；规则、缺对象处置与遗留外仓钩子兼容见standards §12-9。主仓`make test-commit-lint`与兄弟/ UI相应入口验证反例，不能以空待推窗口代替检查器测试。

2026-11-07后续工具链安排（所有者明确）：Go1.27、Node24/26 LTS的整体升级，须等Phase5先合入main后，再基于main独立实施与验证；当前Phase5不提前切换。已完成的Go1.26.10安全修复继续保留。本机系统Go仍1.26.6；原生1.26.10已按官方toolchain模块/开启sumdb验证取得，仅保存在ignored `.codex`，未改全局PATH或系统安装。执行本地命令时显式选择已验证SDK；工具CLI的`GO`覆盖不替代其它Make目标所需的PATH设置。早先“SDK获取失败”段是历史时点，不再作为当前状态。

2026-11-08依赖引用保护：主仓与activelist的utils pin为`v0.4.4-0.20261106080630-cd759a040245`，所需提交完整值`cd759a0402455731f805ba507bf8401447e00e29`。清理保护项为utils远端`refs/heads/codex/phase5-audit`；本轮保持现有pin与引用，不新增tag。删除/改写承载引用前，须从消费者实际go.mod重新清点全部pin，并实时确认每个提交仍由拟保留的远端分支/tag可达；用独立临时模块缓存验证精确版本下载、sum及源码一致，不清共享缓存。只有新的长期远端引用已存在且验证完成后才能解除旧引用保护，正式发布仍按main/tag流程另批。pseudo-version固定commit，不随分支前移；代理曾命中、go.sum或本机bundle均不代替长期源码托管。[Go模块引用规则](https://go.dev/ref/mod#pseudo-versions)与[官方代理保留边界](https://proxy.golang.org/#faq)为依据，本轮结果见[followup](../review/reports/zhuzhao-report-followup-2026-11-08.md#收口复核与交付候选2026-11-08)。

2026-11-08镜像拉取/工具缓存经验：先分清镜像源、网络/代理、工具获取与产品行为，保留原命令、错误、固定版本及退出码。B03实际记录migrate v4.17.0镜像拉取缓慢、替代源未成功，临时使用的宿主CLI虽`-version`显示dev，`go version -m`实为v4.19.1；该绕行不等于原v4.17.0容器路径通过。此前“daocloud/镜像源EOF”仅为未取得原始日志的历史线索，不据此指定替代镜像源或宣称已复现根因。后续Wire/Swag使用[scripts/go-tool.py](../../scripts/go-tool.py)的`main`/`verify`维护独立固定缓存，入口核验工具模块/版本/校验和及SDK来源；坏缓存拒绝执行，安装失败如实退出，不降版本、关闭sum校验或修改全局PATH。构建路径有变化时另记实际产物来源；迁移/绑定种子成功后再看应用就绪，操作序见§9及B24。

2026-11-07当前操作边界：主仓/UI已将审计候选正常合流至实际phase5（09d224d/01c5dea），后续修改基于phase5；三个Go兄弟仍用Phase5用途的 `codex/phase5-audit`。root为本批唯一提交/推送者，只推已核分支，不推main、不准备main PR、不打新tag/正式release、不部署生产或切phase6。Phase6按新顺序等Phase5合main后再从main创建。本地56分支收敛到10，46旧heads有archive ref及五份完整bundle，40旧工作树同hash detach、54目录与原证据保留；临时独立验证工作树另计，结束只收回本轮自建资源。禁止全局clean/reset/prune；证据、备份、原卷和其他会话资源保留。分支逐项清理/恢复收据仅主仓ignored `.codex/audits/ECO-20261104/security-20261107/branch-cleanup/`，不纳Git/Docker。B16/B17破坏性验收仅用自有隔离栈；保留test三应用与F01配置源承接另见下一段，业务数据和原生dev保持。

2026-11-07 B16/B17收口交接（非在途占用）：主603c1b6/ALdb6a17b/TR1e31e3e/utils563785a准确CI全绿，四source/三实际ELF复扫只余未消费OpenPGP模块提示，253/81在自有8容器完成并精确回收。保留test仅3Go应用承接已验收ELF及F01正确只读配置源，端口/网络/所有原卷/Env值保持，含UI在内的23个非目标容器不变；3PG备份、旧images/反向override及失败回退收据仅ignored本轮`b17-environment/`。共享环境未跑acceptance、未改E000001或其它账号角色。原生dev/main/生产未切换，后继仍先核实际PID/image/配置来源；本机原生SDK获取失败，不能把旧Go1.26.6当新门禁工具，完整原Make目标已由准确603的Go1.26.10 Ubuntu CI验证。Go最小版本前置按go.mod，不降低floor绕过；后续本地SDK/工具整理继续Phase5，不提前建Phase6。

2026-11-06审计交接已收口（非在途占用）：[五仓总报告](../review/reports/zhuzhao-ecosystem-full-audit-2026-11-06.md)。test=app dc3/AL60/TR eb3/UI c9，固定来源与镜像已核；dev保留原工作树/原卷，旧UI F08仍有。临时验收/PG/浏览器夹具已精确回收，常驻测试环境留供使用，其它会话cmpfull资源保留。代码/正式文档均路径提交、未合main；仅utils修复分支已推。Go漏洞可达扫描NOT_RUN，原日志/备份/截图与环境重启辅助仅主仓ignored `.codex`，不纳Git/Docker。后继操作先核当前端口/PID/镜像与下节环境来源，不用此历史交接替代实时归属检查。

### 7.1 验收连接与临时文件隔离（2026-11-04）

`make acceptance` 首先调用 `scripts/migration-roundtrip-check.sh --check`，只读检查往返连接与建库权限，再运行四档 API 验收和迁移往返。宿主须具备 `psql`（libpq 客户端）、`migrate`、`python3`；`MIG` 可指定 migrate 可执行文件路径。缺工具、连接失败、无 CREATEDB 权限或 DSN 非法时，整链在 API/容器写操作之前退出。

迁移往返的 SQL 和 migrate 都使用 `ROUNDTRIP_DSN`，不再一侧走 Docker、一侧猜宿主端口。默认兼容 dev：`postgres://zhuzhao:zhuzhao_dev@localhost:5432/postgres?sslmode=disable`。指定非默认 `PG_CONTAINER` 时，必须显式提供 `ROUNDTRIP_DSN`；其中数据库是只读预检/建删临时库所用的管理库，所有迁移和快照均改连随机生成的专属临时库。URI 的用户名/口令中保留字符须百分号编码；query 仅接受 `sslmode`、正整数 `connect_timeout`，缺省连接超时为 5 秒，拒绝 `dbname`、`host`、`options` 等覆盖项。

隔离栈运行前显式设置 `BASE_URL`、`HEALTH_URL`、`PG_CONTAINER`、`PG_USER`、`PG_DB`、`REDIS_CONTAINER` 及 `ROUNDTRIP_DSN`，按 [端口表](port-allocation.md) 核对实际映射。四档中的 SQL 仍按 `PG_CONTAINER` 运行，Redis 仍沿用现有验收口令 `zhuzhao_dev`；往返预检只证明自身连接可用，**不自动证明 API/PG/Redis 属于同一栈**，也不替代本节验收独占纪律。

四档与往返每次调用各用 `mktemp -d` 建私有目录（权限受 `umask 077` 约束）；`TMPDIR` 只选择父目录，链中子阶段不共享具体目录。正常退出、失败及 INT/TERM 清理本次目录，往返只删除已成功创建的专属库，不再先 DROP 同名库。SIGKILL/宿主崩溃不能保证执行清理；疑有残留时先核本轮证据和归属，禁止按公共 `/tmp` 文件名或库名前缀批量清他人材料。

复核入口：`python3 scripts/test-acceptance-isolation.py`（外部命令全部为桩，不连接真实环境，CI 同步执行）；真实数据库复核须另在专属环境执行往返。初次真实复核已在独立 PostgreSQL 15、自定义端口/账号及含保留字符的口令下完成；另以原业务基线运行原样 `make acceptance` 整链通过。一次性环境用后清理，未接入共享 dev/test；这不替代 R01 认证候选正式依赖版本的独立门禁。

## 8. 中间件控制台口径（2026-11-02 拍板）

**中间件控制台一律外部化：挂成熟外部工具，不内嵌业务应用、不自研。** 理由：内嵌=主动扩攻击面（IAM 平台尤其忌）+ 非核心方向永久维护负担 + 重复造 RedisInsight/Kibana 级轮子；业界主流为零。重开触发条件（三条）：①中间件实例×环境多到远超一只手；②zhuzhao 转型多团队内部平台产品（届时拿 CloudBeaver/CloudDM 类现成货，仍不自研）；③出现「工单驱动的数据操作审批」需求（与工单域有化学反应，属 phase 级新立项）。

- **Redis**：`docker compose -f deployments/docker-compose.dev.yaml --profile tools up -d redisinsight` → http://localhost:5540 （tools profile 默认不起，不进主栈；连接 redis:6379 / 密码 zhuzhao_dev，首连 UI 输入一次）
- **asynq 队列深查**：官方 asynqmon 三连实证不适用（停更 3 年+ / 无 arm64 镜像 / 不兼容 asynq v0.26 格式，详见 docker-compose.dev.yaml 注释）——现状用 zhuzhao-ui jobs/死信三 Tab + `redis-cli --scan --pattern 'asynq:{jobs}:*'`；未来真需要时给 taskrunner 加 `asynq.Inspector` 只读端点（其内部已实例化，小批可落）。
- **PG / RustFS**：桌面客户端直连 5432（pgAdmin 等）；RustFS Console 自带（storage profile，9001）。

## 9. 开发/测试环境恢复与来源核对（2026-11-06）

> **当前运行状态（2026-11-11 收尾后）**。日常开发六容器及原生 app/UI/AL/TR 保留，旧 zt 容器已移除。下文旧测试镜像与启动记录用于追溯来源，恢复时使用本轮 protected 恢复映射、原配置和保留的数据卷创建容器，再核对镜像、挂载、密钥配对及健康端点。三库新备份已验证归档目录可读，本轮没有做恢复演练。

> 2026-11-07当前源码：主仓实际phase5已承接B13启动隔离与端口拒抢修复；下文11-06恢复二进制仍可能较旧，不能由当前HEAD推断运行产物已升级。先按§7核占用和实际容器/卷/PID来源，准备go.mod要求的SDK，再使用已验证入口；不接管其他进程或用新空卷冒充数据恢复。

1. 先核 §7 占用，再按容器 labels 的 Compose project、service、实际卷名确认归属。现有开发主库/Redis 为 `zhuzhao-dev`，AL开发库历史默认项目 `deploy`、卷 `deploy_pgdata`；改 project 名会改变默认卷名，不能把新空库当作恢复旧数据。保留未知或其他会话资源，禁止全局 prune、按前缀清理和端口互杀。
2. 优先恢复已有容器/原卷；主PG与AL PG先逻辑备份，TR使用SQLite时同时备份其库及待提交日志状态，不能复制仍在写入的单个db文件。确认依赖健康、显式迁移成功且schema非dirty，再启动应用；主dev Compose只有依赖，AL dev Compose只有PG，TR生产Compose不发布宿主端口，不能互相替代。标准入口与拓扑见 [部署与联调](deployment.md)。
3. 开发全链分别核 API33333、AL18081、TR18080、Vite4000；Vite须使用仓库 `pnpm dev` 对应的 `--mode base`，不能用无mode命令造成标题/配置缺失。AK/SK三对必须同源，环境差异走env，不改冻结yaml。进程就绪后还要验证正常账号登录、真实业务读取；当前来源和已知问题见 [B13报告](../review/reports/zhuzhao-ecosystem-audit-b13-environment-chrome-2026-11-06.md)。
4. 独立测试入口为48081/API33334；数据和开发栈隔离。原始test迁移服务当前无profile，但app没有迁移完成依赖；首拉按“依赖→显式迁移成功→应用”执行。生产主Compose的migrate有profile，普通up不含迁移，二者不可混记。`/health/ready` 是API直连探针，不是 `/api/health/ready`。
5. 记录“源码hash/工作区→依赖pin→构建参数→镜像或二进制摘要→运行实例→探针/Chrome”的对应关系。`latest`标签、本仓HEAD、独立修复分支不相互替代；前端构建时明确开发/测试徽标。开发使用localhost、测试可用127.0.0.1区分浏览器host作用域，避免不同端口共享同名Cookie；浏览器临时视口/主题检查结束后恢复。

本轮现场清单、私有env、PID、备份、截图及重启辅助仅存在忽略的 `.codex/audits/ECO-20261104/environment-recovery-20261106/`，不入Git/Docker；长期规则以本节和部署入口为准。需要沿用现场进程时，先运行该目录 `devctl.py status`，`start`只恢复记录的构建产物并拒绝接管其他端口/PID，不自动升级源码或覆盖数据。

### 9.1 PostgreSQL空卷初始化的TCP就绪判定

容器内健康探针显式指定`pg_isready -h 127.0.0.1`，保留原用户/库及时序。空卷初始化的临时PostgreSQL只开放Unix socket，无host探针可能提前返回0，下游跨容器TCP仍拒绝连接；`service_healthy`与后续自动重启不能弥补这个判定差异。

三仓6份Compose的8处探针按此修正，未改镜像、角色、端口、卷或依赖。TCP可接入后，仍须依次确认显式迁移/绑定种子成功与应用`/readyz`或`/health/ready`，不把它当完整验收。专项8轮fresh卷的旧误绿/新真实AL就绪对照、初期fixture失败及精确资源回收见[B24报告](../review/reports/zhuzhao-ecosystem-audit-b24-postgres-readiness-2026-11-07.md)。该专项时点只完成配置验证；本机5PG与新业务组合已由B27承接复核，生产模板部署仍另验。

### 9.2 审计JSONL卷与恢复

prod/test app分别以app_archive/zt_app_archive挂现有`/app/data/archive`，out_dir覆盖须同步挂载；新挂载须重建才生效。重建前停实际写入方并保真备份旧写层，不能直接换空卷。JSONL独立于pg_dump和RustFS，PAX只读备份→fresh卷恢复的步骤见[backup README](backup.md#审计-jsonl-持久化与离线恢复2026-11-07)，真实导出/替换/恢复证据及限制见[B25报告](../review/reports/zhuzhao-ecosystem-audit-b25-archive-persistence-2026-11-07.md)。默认180天DB/395天文件mtime不变；taskrunner两类执行日志永久保留不在本范围。

2026-11-07承接前盘点：native archive与test写层均0文件，但test尚无持久挂载；app cron=false不代表manual回调禁用。5PG archive_mode均off、无运行pgbackup，不能把文档策略当运行备份。native三Go实际仍1.26.6，test为1.26.10旧B18组合；后续须核真实ELF、备份和挂载再更新，不靠启动hash推断当前产物。原生TR绝对SQLite路径与canonical CWD须同时保留。未知owner容器/他会话资源不清理。

### 9.3 本机Phase5维护后运行基线（2026-11-07）

主仓4db/UI152的代码与准确CI、原253/81及完整前端门禁通过后，dev/test已承接新产物；见[B27收口报告](../review/reports/zhuzhao-ecosystem-audit-b27-phase5-closeout-2026-11-07.md)。dev入口4000/API33333/AL18081/TR18080、test UI48081/API33334/RustFS59000保持；本机test PG原有效51692在本地运行闭包暂时钉住，不是仓库固定端口分配。不要对共享运行服务裸up/down、全局prune或删卷。

当前本机native PID、精确二进制/Env/CWD与ready入口在ignored `.codex/audits/ECO-20261104/phase5-closeout-20261107/environment-finalization/current-dev-manifest.json`；服务已运行时先核该清单与真实监听归属。重启须保留canonical CWD及TR绝对SQLite路径，Env用已校准的业务JSON，不混入新ambient值；KERN_PROCARGS2尾部apple执行元数据不是真实环境配置。旧11-06 devctl及一次性维护脚本基于旧PID，只作历史证据，不能直接拿来当前restart。其它机器仍按正式Make/Compose与各仓目标分支准备环境，不复制本机秘密或把本地闭包当生产模板。

启动顺序为PG/Redis/S3就绪→AL/TR→main→UI；本机运行闭包只使用已验证的不可变镜像、同字节RO配置快照与原卷/网络，build上下文不从兄弟canonical main误取代码。5PG真实TCP/SQL与schema、归档33333权限和SQLite2/290已独立核验。已作停写后的独立5PG+SQLite备份，跨库非原子，不自动恢复覆盖数据；原卷、旧产物和未知owner资源保留。

所有原生Go服务实际1.26.10，SDK只在ignored项目缓存、系统Go1.26.6/全局PATH没有替换；本地新编译须显式选可信1.26.10 SDK。Node仍22.14，整体Go1.27与Node24/26 LTS升级等Phase5合main后另批。E000001两栈只读bcrypt核验为既定终态且mcp=false，本次没有使用其登录/改密；Chrome只用既有E800006会话。


2026-11-12版本维护本地候选收口（非共享栈占用）：所有者取消公司云数据库版本约束，五仓独立`codex/prephase6-version-upgrade`工作树已验证Go1.27.4/Node26.13.1、PG18/Redis8及组件组合。原开发容器、旧卷、原main工作树与系统SDK未切换；新PG/Redis空卷不能当迁移完成。先按`docs/design/component-upgrade-data-contract.md`完成备份恢复/校验，再按实际环境批准的窗口切换；本轮候选、完整门禁与恢复结果见版本维护交付记录，旧Phase5收据不证明新组合。


2026-11-12补记：所有者已自行更新brew基础Go1.27.4、nvm Node26.13.1，已在仓外以基础二进制确认，不是模块自动切SDK的结果。本轮工作使用独立固定SDK验收，未代改包管理配置。版本候选、原门禁与数据恢复收据见[维护记录](../review/reports/version-modernization-2026-11-12.md)；共享旧环境/原卷没有自动切换。
