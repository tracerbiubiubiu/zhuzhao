#!/bin/sh
# zhuzhao 对象存储主→备单向同步循环（B5-5，照抄 pgbackup 侧车形态——05 号 §7 拍板 #5 B 形态）：
#   - rclone sync 主实例桶 → 备实例桶，周期 MIRROR_INTERVAL 秒（dev 默认 1h；RPO=周期，显式登记）
#   - 删除/覆盖保护 = --backup-dir（被覆盖/删除对象移入 backup-archive/<桶>-<日期>/ 前缀，
#     按名排序轮转保留 RETAIN_COUNT 个「天目录」）——不依赖服务端版本化（RustFS 版本化
#     未验；05 §7 A 形态[服务端桶复制+双端版本化]升级时替换）
#   - 失败留痕：失败不更新 last-success 标记、ERROR 日志（pgbackup 三日假备份教训——
#     静默 done 0 类；本脚本模式对齐该事故修复版）
# 环境变量（compose 注入，rclone remote 走 RCLONE_CONFIG_SRC_* / RCLONE_CONFIG_DST_*）：
#   BUCKET / MIRROR_INTERVAL / RETAIN_COUNT
# 恢复与演练步骤见同目录 README.md。
set -eu

BUCKET="${BUCKET:-zhuzhao}"
MIRROR_INTERVAL="${MIRROR_INTERVAL:-3600}"
RETAIN_COUNT="${RETAIN_COUNT:-14}"
ARCHIVE_ROOT="backup-archive"

do_sync() {
  # 首跑目标桶可能不存在：mkdir 幂等（已存在零副作用）
  rclone mkdir "dst:$BUCKET" || { echo "[mirror] ERROR mkdir dst bucket failed"; return 1; }
  # 目标桶版本化（05 §7 拍板 #5；2026-10-20 B5-5 演练实测 RustFS 支持 PutBucketVersioning）：
  # 幂等启用；失败仅 WARN 不阻断（版本化=第二重删除保护，--backup-dir 归档已兜底）
  rclone backend versioning "dst:$BUCKET" Enabled >/dev/null 2>&1 \
    || echo "[mirror] WARN enable dst versioning failed（--backup-dir 归档仍兜底）"
  # --backup-dir 保删除/覆盖（天目录轮转）；-v 记变化量
  if ! rclone sync "src:$BUCKET" "dst:$BUCKET" \
      --backup-dir "dst:$ARCHIVE_ROOT/$BUCKET-$(date +%F)" -v; then
    echo "[mirror] ERROR rclone sync failed (source $(date -u +%FT%TZ))"
    return 1
  fi
  return 0
}

rotate_archive() {
  # 按「天目录」名排序倒序保留前 RETAIN_COUNT 个（对齐 pgbackup RETAIN_COUNT 惯例；
  # 名含日期 → 名序即时序）
  rclone lsf "dst:$ARCHIVE_ROOT" --dirs-only 2>/dev/null | sort -r \
    | tail -n +$((RETAIN_COUNT + 1)) | while IFS= read -r dir; do
      echo "[mirror] 轮转删除旧归档: $dir"
      rclone purge "dst:$ARCHIVE_ROOT/$dir" || echo "[mirror] WARN purge $dir failed（下轮重试）"
    done
}

echo "[mirror] started: src:$BUCKET -> dst:$BUCKET interval=${MIRROR_INTERVAL}s retain=${RETAIN_COUNT}d"
while :; do
  if do_sync; then
    rotate_archive
    echo "[mirror] sync OK $(date -u +%FT%TZ)"
  else
    echo "[mirror] sync 失败，${MIRROR_INTERVAL}s 后重试"
  fi
  sleep "$MIRROR_INTERVAL"
done
