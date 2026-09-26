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
MIG=${MIG:-$(command -v migrate || echo /Users/bujibuji/code/bin/migrate)}
N="${1:-999}"

cleanup() { docker exec "$PG_C" psql -U zhuzhao -d postgres -c "DROP DATABASE IF EXISTS $DB;" >/dev/null 2>&1 || true; }
trap cleanup EXIT

docker exec "$PG_C" psql -U zhuzhao -d postgres -c "DROP DATABASE IF EXISTS $DB;" -c "CREATE DATABASE $DB;" >/dev/null

$MIG -path migrations -database "$DSN" up >/dev/null
LAST=$($MIG -path migrations -database "$DSN" version 2>&1 | awk '/^[0-9]+$/{print $1}')
[ -n "$LAST" ] || { echo "❌ 无法解析 migrate version（stderr 未输出数字）"; exit 1; }
if [ "$N" = "999" ]; then N=$LAST; fi

# 28 批 SYS-2：内容级快照——逐行投影+稳定排序，等量互换/内容改写均可 diff 出
snapshot() {
  docker exec "$PG_C" psql -U zhuzhao -d "$DB" -At -F $'\x1f' -c "
    SELECT 'menus', code, coalesce(parent_id::text,''), coalesce(menu_type::text,''),
           coalesce(path,''), coalesce(component,''), coalesce(permission,''), coalesce(name,''),
           coalesce(sort_order::text,''), coalesce(icon,''), coalesce(visible::text,''), coalesce(is_system::text,'')
      FROM menus
    UNION ALL
    SELECT 'menu_apis', m.code, coalesce(ma.api_path,''), coalesce(ma.api_method::text,''),
           '','','','','','','',''
      FROM menu_apis ma JOIN menus m ON m.id = ma.menu_id
    UNION ALL
    SELECT 'role_menus', r.code, m.code,
           '','','','','','','','',''
      FROM role_menus rm JOIN roles r ON r.id=rm.role_id JOIN menus m ON m.id=rm.menu_id
    UNION ALL
    SELECT 'casbin_rule', coalesce(ptype,''), coalesce(v0,''), coalesce(v1,''),
           coalesce(v2,''), coalesce(v3,''),'','','','','',''
      FROM casbin_rule
  " | LC_ALL=C sort
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
