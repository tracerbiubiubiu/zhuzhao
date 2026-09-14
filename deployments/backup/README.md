# zhuzhao 备份与恢复（2026-09-14 运维补强，M-A6/activelist 同款模式）

## 策略

| 层 | 机制 | 频率 | 保留 |
|---|------|------|------|
| 逻辑备份 | `pg_dump -Fc`（pgbackup 侧车，每日 02:00） | 每日 | 14 份（`RETAIN_COUNT` 可调） |
| 增量 | WAL 归档（postgres `archive_mode=on`，`archive_timeout=300s`） | 持续 | 随 `wal_archive` 卷 |

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
