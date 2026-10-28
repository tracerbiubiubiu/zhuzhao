# runbook · 故障处置与运维手册


> **端口分配**：全生态端口权威表见 [ops/port-allocation.md](ops/port-allocation.md)（冲突处理指引同页）。
> **定位**：真实事故与演练的沉淀位（`docs/README.md` 承诺的 Phase 3 欠账，2026-10-12 建档）。
> 条目纪律：**写「验证过的事实」**——每节要么实机演练过、要么真实事故复盘过；未验证的设想写 `docs/` 对应设计文档，不进这里。
> 维护：每次演练/事故后追加记录行；结论变更就地修订并注日期。

---

## 1. superadmin 密码恢复（break-glass）

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

```sql
-- ① 生成 bcrypt（任意工具，cost ≥ 10）：
--    htpasswd -bnBC 12 "" '新密码' | tr -d ':\n'
-- ② 置换密码 + 强制改密（+ 被禁时启用）
UPDATE users SET password = '<hash>', must_change_password = true, updated_at = NOW()
WHERE username = '<user>' AND deleted_at IS NULL;
UPDATE users SET status = 1 WHERE username = '<user>' AND deleted_at IS NULL; -- 仅被禁时
-- ③ 补审计行（尽力而为；无论如何人工台账必须补记）
INSERT INTO audit_logs (user_id, username, method, path, status_code, duration, user_agent, request_body)
SELECT id, username, 'CLI', 'break-glass:reset-password(sql)', 200, 0, 'manual',
       '{"trigger":"break-glass","tool":"manual-sql"}'
FROM users WHERE username = '<user>' AND deleted_at IS NULL;
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

> 原始记录：`deliverables/software-company/phase3-closure-report-2026-09-15.md`（阶段 E）。

1. **kill 后队列续跑**：taskrunner 容器 kill 后任务不丢，重启续跑。注意口径：**`kill` = 人工停止**——`unless-stopped` 策略不会自动拉起被人工停止的容器（编排器语义），恢复须 `docker start` / `compose up`。
2. **PG 自愈**：PG 容器重启后，zhuzhao / taskrunner 连接池自动恢复，无需重启应用。
3. **dead 重驱**：造死法 = compose `MAX_RETRY` 旋钮临时置 0 + 任务自指黑洞（`127.0.0.1:1`）；重驱后终态/重试链恢复。`MAX_RETRY` 用完即回原值。
4. **SIGTERM 排空**：收到 SIGTERM 后 0.237s 完成在途任务排空并干净退出（`ShutdownTimeout` 生效实证）。

## 3. 备份与恢复要点（2026-09-14 运维补强批）

- **每日备份**：zhuzhao/activelist 各带 pgbackup 侧车（RETAIN_COUNT 统一）。⚠️ 教训（「三日假备份」事故）：postgres 无 restart 策略时停机不自启 + 旧脚本失败静默 done 0——**备份必须定期恢复验证，不能只看文件存在**。
- **恢复演练**：zhuzhao 21 表零差异恢复已实测。坑：shell 循环 `docker exec` 采表会吞 stdin（只采到 9 表）——用 `gexec` + heredoc 单会话采全。
- **PITR**：`pg_dump` 逻辑备**不可**回放 WAL，须 `pg_basebackup` 物理基备（tar 流 stdout → 容器内落盘 + `docker cp`）；卷根 = 数据目录根勿套层；`recovery.signal` 手建 + chown 999；目标时刻若落在未归档段，活库 `pg_switch_wal` 切段后重启即解。
- **对象存储备份**（2026-10-20 B5-5）：`rustfs-mirror` 侧车 rclone sync 主→备 + 双重删除保护（`--backup-dir` 归档轮转 + 目标桶版本化）+ 失败不静默；**RPO = 同步周期**（dev 1h）——对象存储无 WAL 类日志、PITR 不可得，属显式登记口径。恢复步骤与演练记录见 [deployments/backup/README.md](../deployments/backup/README.md)「对象存储备份与恢复」。
- **存储栈升级纪律**（2026-10-27 A-2 拍板 C-11 b 案落行）：动存储栈（RustFS 升级 / minio-go 或 rclone 客户端升级 / 镜像换版）**必跑附件全链验收**（头像上传绑定 / 附件上传下载删除 / richtext 贴图渲染三线 + `make acceptance` S5 面）——兼容劣化信号必然以报障显形，观测者=执行升级的人，本行即其检查单。
- **存储用量观测**（2026-10-27 观测批）：应用侧 `/metrics` 暴露 `zhuzhao_storage_usage_bytes`（按 purpose——`file_objects` SQL 采样）+ 越限 `ops.storage_usage_high` 事件（种子规则收件人空=管理员在消息管理面配置后生效）。运维侧备选：RustFS 原生 `/minio/v2/metrics`（MinIO 兼容端点）**兼容性未实测**——实测通过后磁盘容量水位可零开发接入抓取端，登记在案勿默认其存在。

## 4. dev 栈测试账号台账（2026-10-13 统一口径）

> **性质**：dev 栈公开测试凭据（生产凭据一律 env 注入，与本表无关）。**唯一权威 = 本表**；
> 各仓套件（后端 acceptance / ui E2E / break-glass CLI）全部收敛到同一终值，禁止私设终态。

### 4.1 账号清单

| 账号 | 工号 | 密码 | 用途与归属 | 改密许可 |
|---|---|---|---|---|
| admin（superadmin） | E000001 | `admin12345` | **全生态唯一终态**：后端四档 acceptance / ui E2E global-setup / break-glass CLI 三处收敛同值 | **仅限三处**：① phase1 幂等改密流 ② ui `e2e/global-setup.ts` 收敛逻辑 ③ break-glass CLI 复原（§1）。**人工与 agent 的多账号测试禁止改 E000001**——用 E000002+ 专属账号 |
| manager | E000002 | `Adm2#2609x`（用户自管） | 日常多账号手动测试（admin 角色，不动 E000001 的正确姿势） | 所有者自管 |
| agent_s2（agent 专属） | E800001 | `Agt1#2610x` | agent 会话 UI 走查专属（admin 角色；2026-10-24 S2 画布走查建号——E000002 台账密码已脱靶且属所有者自管，按 §4.2#2 新建 E8 段） | agent 自管；所有者可随时禁用 |
| e2e_operator / e2e_viewer | E2E9020 / E2E9030 | `E2eOper#2609` / `E2eView#2609` | Playwright E2E 专属（global-setup 幂等建号） | 仅 `zhuzhao-ui/e2e/global-setup.ts` |
| S1 演练号 | E2E 段随建 | global-setup 派发 | 强制改密流程演练（每轮重置，无常态） | 同上 |

### 4.2 纪律

1. **E000001 密码终态恒为 `admin12345`**——历史冲突根因 = ui E2E 曾把终态设为 `E2eAdmin#2609`、后端 acceptance 认 `admin12345`，谁后跑谁赢（2026-10-13 一天内三次互改实证）；已翻转为全生态收敛 admin12345（`global-setup.ts` main ① 注释）。
2. **新增测试账号**：工号用手动段 `E8xxxx`（E2E 段 E2E9xxx 归 Playwright）；密码模式参考 `{Role}{N}#{YYMM}x`（≥8 位含大小写数字符号，过 min=8 复杂度），登记进本表。
3. **改密后必须清强制改密标记**（否则 20007 门拦后续操作）：走 `/auth/password/update` **两步轮换**（新==旧被 B2-2 拒，先临时值再回终值）——break-glass CLI 只负责重置，标记清除是独立一步。
4. agent 会话开工多账号测试前**先读本表**；发现 E000001 脱离终态，用 §1 CLI 复原而非再设新值。

### 4.3 节点超时催办观察模式（S10 B10-4——上线后必关）

> **催办通知默认只记日志不发送**（`tickets.timeout_observe: true`——存量数据回填近似值防误报）。
> dev 栈重启后按以下步骤确认并关闭：

1. `GET /api/v1/system/cron-status`（admin）→ 确认 `timeout_scan` 任务存在且 `observe: true`
2. 等首个扫描周期（默认 5 分钟）→ 查服务端日志 `timeout_scan: overdue candidate (observe, not sent)` 确认无误报
3. `configs/config.yaml` 置 `tickets.timeout_observe: false`（或 env `TICKETS_TIMEOUT_OBSERVE=false`）→ 重启
4. 复查 cron-status → `observe: false` → 下个周期起实际催办

不关的后果：所有超时催办/SLA deadline 通知**只记日志不发送**——功能静默失效。

## 5. 存量协议兼容层接入（aksk/compat，2026-10-13 建档·10-14 已预接线默认关）

> **性质**：存量协议兼容层接入与验证手册。utils `aksk/compat` 已交付并发布（v0.4.2），zhuzhao 侧已预接线（§5.1，默认关）；本节 = 内网调用场景就位后的密钥配置与上线前比对流程。背景决策：[phase1/02-auth §存量迁移翻案追记](./phase1/02-auth.md)。

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

## 7. 多会话协作与验收环境纪律（2026-10-26 立约）

> 来源=两会并行开发当周的三次互相干扰事故：端口互杀（互相 `make dev` 杀对方服务）/ Redis 于验收中途被重启 / **T7 假红**（一会的 viewer 角色页面绑定实验污染另一会的验收断言）。

1. **端口分治**：33333=标准实例（`make dev`），只跑四档 acceptance 与联调；33334=实验二实例（`APP_PORT=33334 make dev`，88e0d19 机制），一切 schema/数据/角色绑定实验在此进行。两会各占其一，不抢不杀。
2. **验收独占**：跑 acceptance 前确认无他会在跑（`pgrep -fl acceptance-phase`）；验收期间不对共享 dev 栈做 docker 重启/种子数据/角色绑定变更。
3. **验收夹具自愈**：脚本对「角色零绑定→403」类断言一律用**运行时自建临时角色**（`p2a_viewer_*` / `p2c_viewer_*`，跑完残留随 `docker-dev-reset` 清），不得依赖 seed `viewer` 角色的绑定状态——它可能被任何会话的功能实验合法修改。phase1 仍用 seed viewer 做角色停用语义夹具，但它自带绑定清理守卫且居于链首。
4. **共享库直改须自清**：实验性 role_menus/种子数据改动在实验结束或验收窗口前还原（`docker-dev-reset` 一步到位）。
5. **验证环境存疑即重跑**：acceptance 出现「断言值与响应体自相矛盾」「时序相关偶发红」时，先疑环境（另一会在写同一栈）再疑代码——本仓历史上所有 acceptance 假红（NOPE999 锁污染/T7）均系环境类。
6. **并行会话在途台账**（2026-10-28 立——standards §12-10「会间交接注」登记处）：两会以上并行时，各会话收尾在下表落一行、收口清行；后继会话开工先读。三起立法实证：fe720e1（文档行覆写）/1418f33（staged 索引互扫）/dcf005c（amend 吞并他人提交）。

   | 会话主题 | 在途文件 | 未推送笔数 | 落笔时间 |
   |---|---|---|---|
   | （收尾自填；收口后清行） | | | |


