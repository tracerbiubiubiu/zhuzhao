# zhuzhao 备份与恢复（2026-09-14 运维补强，M-A6/activelist 同款模式）

## 策略

| 层 | 机制 | 频率 | 保留 |
|---|------|------|------|
| 逻辑备份 | `pg_dump -Fc`（pgbackup 侧车，每日 02:00） | 每日 | 14 份（`RETAIN_COUNT` 可调） |
| 增量 | WAL 归档（postgres `archive_mode=on`，`archive_timeout=300s`） | 持续 | 随 `wal_archive` 卷 |
| 对象存储 | `rclone sync` 主→备（rustfs-mirror 侧车，phase5 S5/B5-5） | `MIRROR_INTERVAL`（dev 1h；**RPO=周期**，附件场景日级可接受——显式登记不默认） | `--backup-dir` 天目录 14 个（`RETAIN_COUNT`）+ 目标桶**版本化**（RustFS 实测支持） |
| 审计 JSONL | `audit_archive` 导出目录的专属卷；单独离线备份 | 按实际归档/备份窗口执行，本批不新增调度 | 本地文件按 mtime 保留 395 天（默认）；离线副本另行登记保留/销毁策略 |

备份卷：`backups`（pg_dump 产物 zh-*.dump）、`wal_archive`（WAL 归档，pgbackup 只读挂载）。

## 审计 JSONL 持久化与离线恢复（2026-11-07）

`AuditArchiveJob.Handle`（`internal/service/audit_archive.go`）将过期的 `audit_logs`、
`policy_evaluation_logs` 单批导出为**普通 JSONL**，文件 `Sync` 成功后删除同批 DB 行。
这些文件不在后续 `pg_dump` 中，也不属于 RustFS 附件桶，必须独立备份。
生产 Compose 的 `app_archive`、test Compose 的 `zt_app_archive` 均挂到
`/app/data/archive`；实际卷名随 Compose project 前缀生成，以容器 Mounts 为准。
镜像预建目录归属 `33333:33333`，新命名卷使用 Docker 默认 copy-up 保留该归属，
无需把目录放宽到 777。已有宿主/卷数据或自定义运行用户需先核归属与写权限。

默认 `audit.archive.out_dir=data/archive`，容器工作目录 `/app`。如果通过
`APP_AUDIT_ARCHIVE_OUT_DIR` 覆盖目录，须同步调整挂载目标、备份路径和恢复路径；
否则实际产物仍可能落入容器写层。native 进程用启动日志中的绝对目录为准，
不能把宿主目录当成 Docker named volume。新增挂载只在 app **重建**后生效；
重建前先备份原容器写层内的既有文件，再迁移到新卷，禁止直接删除原容器。

### 备份与 fresh 卷恢复

先停实际归档写入方：app 进程内 ticker 和 taskrunner 回调均不能在快照期间写入。
可在维护窗口停止 app，使用实际部署的 `-f`/`-p`/env 组合；本批不会自动停止或重建环境。
以下使用已在场 PostgreSQL 镜像的 GNU tar，源卷只读挂载，不安装包或增加常驻服务。
归档含个人信息，产物保存在受限本地目录，随备份一起保存 SHA256 与文件
`SHA/条数/UID/GID/mode/mtime` 清单；不上传到未登记的目的地。

```sh
set -eu
# 设置实际容器名（test 对应 zt-app、zt-postgres）及已存在的受保护本地目录
archive_app=zhuzhao-app
archive_pg=zhuzhao-postgres
archive_backup=/protected/local/audit-archive-YYYYMMDD.tar
archive_volume="$(docker inspect "$archive_app" --format \
  '{{range .Mounts}}{{if eq .Destination "/app/data/archive"}}{{.Name}}{{end}}{{end}}')"
archive_tar_image="$(docker inspect "$archive_pg" --format '{{.Image}}')"
test -n "$archive_volume" # 无 named volume 时先处理旧写层数据，勿导出空卷
umask 077

# 源卷只读，PAX 保留纳秒 mtime 与数字 UID/GID；GNU tar 要先核版本
docker run --rm --network none --read-only --entrypoint tar \
  "$archive_tar_image" --version
docker run --rm --network none --read-only --user 33333:33333 \
  --mount "type=volume,src=$archive_volume,dst=/archive,readonly" \
  --entrypoint tar "$archive_tar_image" \
  --format=pax --numeric-owner -cpf - -C /archive . > "$archive_backup"
# 上一命令须 exit 0，再记哈希并核对清单，不能仅以 tar 文件存在判断成功
shasum -a 256 "$archive_backup"

# 只恢复到新建空卷，先核对备份哈希；恢复容器临时 root 用于保留所有者
archive_restore_volume=zhuzhao-archive-restore-YYYYMMDD
if docker volume inspect "$archive_restore_volume" >/dev/null 2>&1; then
  echo "恢复卷已存在，请改用未占用名称" >&2
  exit 1
fi
docker volume create "$archive_restore_volume"
docker run --rm -i --network none --read-only --user 0:0 \
  --mount "type=volume,src=$archive_restore_volume,dst=/archive" \
  --entrypoint tar "$archive_tar_image" --same-owner -xpf - -C /archive \
  < "$archive_backup"
```

恢复后以 app 实际用户核 JSONL 可逐行解析、条数/哈希与清单一致、UID/GID/mode/mtime
保持，再在维护窗口显式把 app 指向恢复卷并验证。保留原卷供回退，勿在原卷上覆盖试恢复。
本次所用 Docker 的 `docker cp` 默认 tar 流把文件所有者改成 root、mtime 截断到秒，
隔离负向演练已检出；
需保真时使用上面的 PAX 路径。生产首次接卷前的旧写层文件，同样须在停写后以
PAX/数字所有者保真迁移并核对，不能以新增空卷遮住尚未备份的文件。

### 保留范围与验证边界

- DB 默认保留 180 天、单批 5000 行；文件默认保留 395 天。现有清理器仅扫描
  `out_dir` **顶层 `.jsonl` 文件的 mtime**，不按文件名日期，不递归，不清理离线 tar；
  恢复时重置 mtime 会改变清理时间。离线副本的期限、责任人与销毁方式需另行登记，
  不因本批加卷而自动轮转。卷仍在同宿主机，不能等同异机备份。
- 应用 `cron_enabled=false` 只关闭进程内 ticker；taskrunner 的 manual/cron 作业定义
  仍须独立核对。本批不启用作业、不改保留天数；`task_runs` / `execution_logs`
  的永久保留不在此次归档范围。
- 2026-11-07 独立 PG（81 份真实迁移）+ 自有卷演练：真实 service 导出 3+2 条合成过期行，
  DB 各保留 1 条新行；容器替换及 PAX 备份→fresh 卷恢复后，JSONL 解析/条数/SHA、
  UID/GID/mode/纳秒 mtime 一致。仅合成夹具验证 400 天旧 mtime 文件删除，
  1 天新 mtime 的旧文件名、非 JSONL 和子目录文件保留。常驻 dev/test 未应用或触发归档；
  当前环境实际备份是否启用，仍须按运行容器与日志核验，不能从本策略表推定。

## 恢复步骤（pg_dump → 全量恢复）

```sh
# 0. 前置核验（毁库前必做）：备份归档可读
docker exec zhuzhao-pgbackup pg_restore -l /backups/zh-YYYYMMDD.dump | head

# 1. 停写
docker compose -f deployments/docker-compose.yaml stop app

# 2. 清库重建
docker compose -f deployments/docker-compose.yaml exec postgres \
  psql -U zhuzhao -d postgres -c "DROP DATABASE zhuzhao;" -c "CREATE DATABASE zhuzhao OWNER zhuzhao;"

# 3. 恢复（管道退出码用 pipefail 检查）
docker exec zhuzhao-pgbackup cat /backups/zh-YYYYMMDD.dump | \
  docker compose -f deployments/docker-compose.yaml exec -T postgres \
  pg_restore -U zhuzhao -d zhuzhao --no-owner --role=zhuzhao

# 4. 起栈抽验（/health/ready + 登录 + 关键表计数与毁库前基线比对）
docker compose -f deployments/docker-compose.yaml start app
```

> **迁移记账随库走**：`schema_migrations` 在恢复产物内，恢复后勿再跑 `--profile migrate`（重复迁移会失败于对象已存在）。

## PITR（WAL 时点恢复）

> **⚠ 2026-09-14 实测勘误**：PITR 的基准备份必须是**物理基备（`pg_basebackup`）**——
> `pg_dump` 是逻辑导出，恢复出的集群 LSN 与原库 WAL 时间线不接续，**不能用归档 WAL
> 回放**（activelist 侧 backup/README.md 的蓝本同此误，已同步勘误）。
> 物理基备做法：

```sh
# 1. 物理基备（容器内本地执行，落 wal_archive 同盘卷或临时目录）
docker exec zhuzhao-postgres pg_basebackup -D /tmp/base -Fp -Xs -P -U zhuzhao

# 2. 目标时刻前的变更照常发生（WAL 由 archive_command 持续归档）

# 3. 临时实例回放到目标时刻（挂同一 wal_archive 卷）
docker run -d --name zh-pitr --network zhuzhao_default \
  -v zhuzhao_wal_archive:/wal_archive:ro -v <基备目录>:/var/lib/postgresql/data \
  -e POSTGRES_PASSWORD=dummy postgres:15 \
  postgres -c restore_command='cp /wal_archive/%f %p' \
           -c recovery_target_time='2026-09-14 18:00:00+08' \
           -c recovery_target_action='promote'

# 4. 若报「recovery ended before configured recovery target was reached」：
#    目标时刻落在当前【部分段】（未归档）——活库 pg_switch_wal() 强制切段后重启临时实例
```

> 实测（2026-09-14）：T1 插行→基备→T2 插行，回放至 T1/T2 之间目标时刻——
> T1 行在、T2 行不在，`recovery stopping before commit … <T2 时刻>` 精确命中。
> 基备解包坑：卷根必须=数据目录根（勿多套一层）；`recovery.signal` 须手建；
> 解包后 chown 归属 postgres 运行用户（uid 999）。

## 注意

- 备份失败**不静默**：pgbackup 日志可见（`docker compose logs pgbackup`）；失败当日每 10 分钟自动重试、失败日不轮转、成功才标记当日完成；
- **常见失败根因**：postgres 未运行（pg_dump 报 `could not translate host name "postgres"`）——postgres 已配 `restart: unless-stopped` 自愈；手工停库则备份持续重试失败直至库恢复，属预期；
- `backups` / `wal_archive` 卷建议纳入宿主机级外部备份（卷快照/同步），防单机盘损；
- 保留期调整：`RETAIN_COUNT`（份数，默认 14）。

## 对象存储备份与恢复（B5-5，2026-10-20）

### 形态

`rustfs-mirror` 侧车（`rclone/rclone:1.75.2` 钉死，`--profile storage` 随 rustfs 同启）：
`rclone sync` 主实例桶 → `rustfs-backup` 备实例桶 + **双 重删除保护**——① `--backup-dir` 归档前缀
（被覆盖/删除对象移入 `backup-archive/<桶>-<日期>/`，天目录按名序轮转保留 `RETAIN_COUNT` 个）；
② 目标桶**版本化**（RustFS 实测支持 `PutBucketVersioning`，脚本启动幂等启用——失败仅 WARN
不阻断，①仍兜底）。失败不静默（ERROR 日志 + 不写 last-success；对齐 pgbackup 三日假备份教训）。
remote 经 `RCLONE_CONFIG_SRC_* / DST_*` env 注入（无配置文件）。**RPO = 同步周期**
（`MIRROR_INTERVAL`，dev 默认 3600s；无 WAL 类日志可 ship，PITR 不可得——05 号 §7 显式登记）。
升级触发（05 §7 拍板 #5）：跨主机目标就位 / 量级上来 → 切服务端桶复制 + 双端版本化（A 形态）。

### 恢复步骤（单对象/前缀回填）

```sh
# 从备实例回搬（容器内 remote 已配好）：
docker exec zhuzhao-rustfs-mirror rclone copy "dst:zhuzhao/<前缀>/" "src:zhuzhao/<前缀>/"
# 若目标侧已被 sync 归档走（主侧删过、备侧移入归档天目录），从归档取：
docker exec zhuzhao-rustfs-mirror rclone copy \
  "dst:backup-archive/zhuzhao-YYYY-MM-DD/<前缀>/" "src:zhuzhao/<前缀>/"
# 整桶灾后重建：rclone sync dst:zhuzhao src:zhuzhao（方向反转，先停 app 写）
```

### 演练记录（2026-10-20 dev 栈实测，五步全过）

1. **全量同步**：侧车首启 22 对象全量 + `rclone check` **24 matching / 0 differences**（哈希级）；
2. **增量**：新写 `drill/restore-*.txt` → sync → 备侧在位；
3. **损→恢**：主侧删该对象 → `rclone copy dst→src` 回搬 → 内容字节一致（`drill-payload-*`）；
4. **删除保护**：主侧再删 → sync → 对象落入 `backup-archive/zhuzhao-2026-10-20/`（归档可取回）；
5. **DB↔对象联合对账**（负向演示）：
   ```sh
   diff <(docker exec zhuzhao-dev-postgres psql -U zhuzhao -d zhuzhao -At \
     -c "SELECT object_key FROM file_objects WHERE status='confirmed' AND bucket='zhuzhao' ORDER BY 1;") \
        <(docker exec zhuzhao-rustfs-mirror rclone lsf src:zhuzhao --recursive --files-only | sort)
   ```
   实测双向检出：DB-only = 引用悬空（旧库快照+新桶场景可查；pending 态未上传属正常瞬态，
   对账引用集应以 `confirmed` 为准）、桶-only = 无主对象（GC 域）。

### 注意（对象面）

- 演练残留：`dst:backup-archive/zhuzhao-2026-10-20/` 含演练对象（dev 栈，随天目录轮转自然过期）；
- `--profile storage` 未起则备份不在位——上生产时 rustfs/rustfs-backup/rustfs-mirror 三件同 profile；
- 版本化使 `rclone sync` 在备侧产生历史版本（叠加归档双重占空间）——空间水位纳入日常巡检。
