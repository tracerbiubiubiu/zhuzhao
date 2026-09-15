# ops/runbook — 故障处置与运维手册

> **来源纪律**：全部条目来自真实事故与故障演练（2026-09 联调期，非推演）。每条 = 现象 → 根因 → 处置 → 预防。新事故按同结构追加；处置过的新套路回填本文件。

---

## 0. 环境速查（两套栈，先分清再动手）

| | 演示栈 | dev 栈 |
|---|---|---|
| compose | `deployments/docker-compose.yaml`（zhuzhao 侧） | `deployments/docker-compose.dev.yaml` + `go run ./cmd/server` |
| 入口 | `zhuzhao-app:33333` | 同 :33333（**与演示栈互斥，先停一个**） |
| 数据库 | `zhuzhao-postgres`（volume 持久） | `zhuzhao-dev-postgres:5432`（docker-dev-reset 可重置） |
| **E000001 口令** | `Phase3Demo#2026`（bcrypt 已恢复） | 随 acceptance 漂移：admin123 → admin12345（phase1 脚本强制改密）→ 验证残留 B8Verify#2026 |
| taskrunner | 独立三容器栈：`taskrunner/deploy/compose.prod.yaml`（**compose 已改名，必须显式 -f**） | 不部署（回调链经 Go 集成测试） |

- SK 配对：zhuzhao `GATEWAY_SK` ↔ activelist `ACTIVELIST_CALLER_ZHUZHAO_SK`；zhuzhao `internal_jobs` SK ↔ taskrunner `TASKRUNNER_SELF_SK`；taskrunner `CALLER_ZHUZHAO_SK` ↔ zhuzhao internal_jobs 侧。本地默认 `dev-gateway-sk`/`dev-self-sk` 系，生产一律覆盖。
- **两库口令勿混**：演示库与 dev 库是两套 PG，口令各自独立（混用 = 20001 假性密码错误）。

---

## 1. 常见故障处置

### 1.1 登录返回 20006「账号已锁定」

- **现象**：登录 401 + code 20006。
- **根因**：同一工号 15 分钟窗口内失败 ≥5 次（Redis `lock:login:{工号}`）。误用旧口令连试、验收脚本断言失败重试都会触发。
- **处置**：`docker exec zhuzhao-dev-redis redis-cli -a zhuzhao_dev del "lock:login:E000001"`（或等 TTL）。
- **预防**：脚本断言登录失败时**先清锁**；演示口令变更后同步更新脚本。

### 1.2 dev server 拒启：`internal_jobs.enabled=true 但 taskrunner_sk 未配置`

- **现象**：启动即 `config: ... 拒绝启动`（fail-closed）。
- **根因**：E 阶段后 `internal_jobs.enabled` 默认开启，`INTERNAL_JOBS_SK` 未注入。
- **处置**：`INTERNAL_JOBS_SK=<非空> go run ./cmd/server`。
- **说明**：这是验签端点不允许裸奔的设计行为（fail-closed），不是故障。

### 1.3 容器健康但日志静默（非 root × bind mount）

- **现象**：容器正常服务，但文件日志与 stdout **双双无输出**。
- **根因**：容器以非 root（uid 100）运行，宿主 bind mount 目录属主覆盖镜像内 chown；utils logger 的 MultiWriter 中 lumberjack 失败即断，stdout 一并丢失。
- **处置**：宿主目录 `chown -R 100:<gid>`；taskrunner 已带启动可写探针（R5：`TASKRUNNER_LOG_DIR` 不可写即 Fatal 拒启，报错含 uid 指引）。
- **预防**：部署清单写明宿主目录属主预授权；日志可见性列入发布后首查项。

### 1.4 备份目录多日无新文件（activelist pgbackup「假备份」事故）

- **现象**：备份目录三天无新增（且存在 0 字节残留文件）。
- **根因**：postgres 容器无 restart 策略（停机后不自启）× 旧备份脚本失败静默退出 × 0 字节残留未清理。
- **处置**：compose 补 `restart: unless-stopped`；0 字节残留重做机制；`RETAIN_COUNT` 统一保留策略。
- **预防**：恢复演练**周期化**（备份的存在性 ≠ 可恢复性）；`pg_restore -l` 预检先行。

### 1.5 演示库 schema 漂移（迁移缺版）

- **现象**：回调幂等栅栏 SQL 报错（缺列）；或容器日志全静默（伴生日志目录问题）。
- **根因**：演示库 schema_migrations 停在旧版本（000026–000029 未应用），与代码期望不一致。
- **处置**：`SELECT version FROM schema_migrations` 对照迁移目录；缺版时引导版本号后用 migrate 镜像手工前滚（镜像拉取被网络阻断时的绕法见 1.8）。
- **预防**：每次重建/升级后核对 `schema_migrations` 与 `migrations/` 最高版本一致。

### 1.6 `docker compose` 裸命令不命中/拉错文件

- **现象**：`docker compose up -d` 无反应，或把部署态栈当开发态拉起。
- **根因**：activelist 的 compose 已改名双文件（`compose.prod.yaml` 部署态 / `compose.dev.yaml` 开发态）——裸命令不再命中任何文件。
- **处置/预防**：**永远显式 `-f`**：部署态 `-f deploy/compose.prod.yaml`，开发态 `-f deploy/compose.dev.yaml`。

### 1.7 端口 33333 冲突（acceptance 与演示栈互斥）

- **现象**：acceptance 起服务失败，或演示栈端口被占。
- **处置**：acceptance 占用 33333——跑验收前先停演示 app（`docker compose -f deployments/docker-compose.yaml stop app`），验收后 `start app` 恢复；另注意 dev server 须带 `INTERNAL_JOBS_SK`。
- **预防**：两套栈入口互斥写进验收前置清单（已入标准环境纪律）。

### 1.8 git push `Empty reply from server`

- **现象**：push 偶发失败（网络对 GitHub 阻断/瞬断）。
- **处置**：`sleep` 后重试即可（未发现需要特殊代理配置的场合）；提交都已本地落库，无丢失风险。

---

## 2. 故障演练复现手册（E 阶段四项，均已实证）

| # | 演练 | 复现步骤 | 预期（已实证） |
|---|---|---|---|
| 1 | **硬杀续跑（at-least-once）** | 提交任务 → `docker kill taskrunner-taskrunner-1`（硬杀） → 处理中的任务 → `docker compose start` | 任务 pending 存活 Redis 队列，拉起即续跑 succeeded。注：kill 属人工停止，`unless-stopped` 不自动拉（编排器口径） |
| 2 | **PG 闪断自愈** | `docker restart taskrunner-postgres-1` | healthy → 新任务提交 succeeded（连接池自愈） |
| 3 | **dead 重驱** | compose `TASKRUNNER_MAX_RETRY` 旋钮置 0（造 dead 靶标）→ 重驱 API：dead→pending→再执行→dead；不存在任务返回 10004 | 重驱链路语义正确 |
| 4 | **SIGTERM 排空** | `docker stop`（SIGTERM） | asynq graceful shutdown 全序列 0.237s 干净退出 → 回 healthy |

---

## 3. 验收标准环境纪律（历史事故固化）

1. 跑 acceptance 前先停演示 app（33333 互斥），用 `make docker-dev-reset` 重置标准环境；
2. dev server 必带 `INTERNAL_JOBS_SK`（见 1.2）；
3. docker-dev-reset 后如跑 taskrunner 相关测试，重建 scratch 库；
4. **门禁判定禁只数 ok 行数**——必须看整体退出码/FAIL 行（utils 编译破损漏检事故）；
5. 事故改环境（口令/迁移）后立即回写 runbook 本节。

---

## 4. 已知限制与观察项（登记不修，等触发）

- 回调 fence 失配返回 2xx「幂等受理」后，若接管执行失败无自动重驱（fence 固有取舍；观察任务失败率）。
- Logout × 并发 Refresh 的单设备幸存窗口（毫秒级；02-auth 已留痕）。
- activelist `statement_timeout` 配置 <5s 时，锁等待以 57014 而非 55P03 到达 → 500 而非 409（配置口径：须 >5s 或 0）。
- activelist 大导入上传真实上限 = ReadTimeout 30s（WriteTimeout 300s 只管响应写出段）；更大导入走运维通道。
