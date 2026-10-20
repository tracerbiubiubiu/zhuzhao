# zhuzhao 备份与恢复（2026-09-14 运维补强，M-A6/activelist 同款模式）

## 策略

| 层 | 机制 | 频率 | 保留 |
|---|------|------|------|
| 逻辑备份 | `pg_dump -Fc`（pgbackup 侧车，每日 02:00） | 每日 | 14 份（`RETAIN_COUNT` 可调） |
| 增量 | WAL 归档（postgres `archive_mode=on`，`archive_timeout=300s`） | 持续 | 随 `wal_archive` 卷 |
| 对象存储 | `rclone sync` 主→备（rustfs-mirror 侧车，phase5 S5/B5-5） | `MIRROR_INTERVAL`（dev 1h；**RPO=周期**，附件场景日级可接受——显式登记不默认） | `--backup-dir` 天目录 14 个（`RETAIN_COUNT`）+ 目标桶**版本化**（RustFS 实测支持） |

备份卷：`backups`（pg_dump 产物 zh-*.dump）、`wal_archive`（WAL 归档，pgbackup 只读挂载）。

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
