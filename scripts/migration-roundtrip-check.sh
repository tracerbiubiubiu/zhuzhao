#!/bin/bash
# 迁移往返数据 diff 门禁（27 批系统性止血）：
#   临时库 up 全量 → 快照 → down N → up N → 快照 → diff。
# 抓「down 能跑通但数据悄悄漂移」类缺陷（D-1 同型——批 29 全链验证对之全盲）。
# 用法：bash scripts/migration-roundtrip-check.sh [N]（N=从最新往回验的对数，默认全部）
set -euo pipefail
cd "$(dirname "$0")/.."

PG_C="${PG_CONTAINER:-zhuzhao-dev-postgres}"
DB="zz_roundtrip_$$"
DSN="postgres://zhuzhao:zhuzhao_dev@localhost:5432/$DB?sslmode=disable"
MIG=/Users/bujibuji/code/bin/migrate
N="${1:-999}"

cleanup() { docker exec "$PG_C" psql -U zhuzhao -d postgres -c "DROP DATABASE IF EXISTS $DB;" >/dev/null 2>&1 || true; }
trap cleanup EXIT

docker exec "$PG_C" psql -U zhuzhao -d postgres -c "DROP DATABASE IF EXISTS $DB;" -c "CREATE DATABASE $DB;" >/dev/null

$MIG -path migrations -database "$DSN" up >/dev/null
LAST=$($MIG -path migrations -database "$DSN" version | awk '{print $1}')
if [ "$N" = "999" ]; then N=$LAST; fi

snapshot() {
  docker exec "$PG_C" psql -U zhuzhao -d "$DB" -At -c "
    SELECT 'menus', count(*) FROM menus
    UNION ALL SELECT 'menu_apis', count(*) FROM menu_apis
    UNION ALL SELECT 'role_menus', count(*) FROM role_menus
    UNION ALL SELECT 'casbin_rule', count(*) FROM casbin_rule
    UNION ALL SELECT 'menus_detail', count(*) FROM (SELECT code||parent_id||menu_type||path||component||permission FROM menus ORDER BY code) t
    UNION ALL SELECT 'ma_detail', count(*) FROM (SELECT m.code||ma.api_path||ma.api_method FROM menu_apis ma JOIN menus m ON m.id=ma.menu_id ORDER BY 1) t
    UNION ALL SELECT 'rm_detail', count(*) FROM (SELECT r.code||':'||m.code FROM role_menus rm JOIN roles r ON r.id=rm.role_id JOIN menus m ON m.id=rm.menu_id ORDER BY 1) t
    ORDER BY 1;"
}

snapshot > /tmp/rt_before.txt
$MIG -path migrations -database "$DSN" down "$N" >/dev/null
$MIG -path migrations -database "$DSN" up >/dev/null
snapshot > /tmp/rt_after.txt

if ! diff -u /tmp/rt_before.txt /tmp/rt_after.txt; then
  echo "❌ 迁移往返数据漂移（down 后 up 与原态不一致）"
  exit 1
fi
echo "✅ 往返零漂移（$N 对，menus/menu_apis/role_menus/casbin_rule 计数+明细）"
