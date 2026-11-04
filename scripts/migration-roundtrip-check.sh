#!/usr/bin/env bash
# 迁移往返数据 diff 门禁：临时库 up → 快照 → down N → up → 快照 → diff。
# SQL 与 migrate 共用 ROUNDTRIP_DSN 派生的连接；--check 只读预检供全链入口调用。
# 默认兼容本地 dev；自定义 PG_CONTAINER 必须显式给出 ROUNDTRIP_DSN。
set -euo pipefail
cd "$(dirname "$0")/.."

CHECK_ONLY=0
if [ "${1:-}" = "--check" ]; then CHECK_ONLY=1; shift; fi
N="${1:-999}"
[[ "$N" =~ ^[1-9][0-9]*$ ]] && [ "$#" -le 1 ] || {
  echo "❌ 用法：migration-roundtrip-check.sh [--check | N]（N 为正整数）" >&2; exit 2;
}
if [ "${PG_CONTAINER:-zhuzhao-dev-postgres}" != "zhuzhao-dev-postgres" ] && [ -z "${ROUNDTRIP_DSN:-}" ]; then
  echo "❌ 自定义 PG_CONTAINER 必须显式设置 ROUNDTRIP_DSN；不会回退到 dev 数据库" >&2
  exit 2
fi
MIG="${MIG:-migrate}"
for tool in psql python3 "$MIG"; do
  command -v "$tool" >/dev/null || { echo "❌ 缺少往返门禁工具：$tool" >&2; exit 2; }
done

# 不接受 libpq 的 query dbname/host 等覆盖项，保证替换 path 后仍是专属临时库。
CONNECTIONS="$(ROUNDTRIP_DSN="${ROUNDTRIP_DSN:-postgres://zhuzhao:zhuzhao_dev@localhost:5432/postgres?sslmode=disable}" python3 - <<'PYDSN'
import os, re, secrets, sys
from urllib.parse import parse_qsl, urlencode, urlsplit, urlunsplit
try:
    value = os.environ['ROUNDTRIP_DSN']
    if re.search(r'[\x00-\x20\x7f]', value) or re.search(r'%(?![0-9a-fA-F]{2})', value):
        raise ValueError()
    uri = urlsplit(value)
    if uri.scheme not in ('postgres', 'postgresql') or not uri.hostname or uri.fragment:
        raise ValueError()
    if uri.port is not None and not 1 <= uri.port <= 65535:
        raise ValueError()
    query = parse_qsl(uri.query, keep_blank_values=True, strict_parsing=True)
    if len({k for k, v in query}) != len(query) or any(k not in ('sslmode', 'connect_timeout') for k, v in query):
        raise ValueError()
    if any(k == 'connect_timeout' and (not v.isdigit() or int(v) < 1) for k, v in query):
        raise ValueError()
    if not any(k == 'connect_timeout' for k, v in query):
        query.append(('connect_timeout', '5'))
    db = 'zz_roundtrip_' + secrets.token_hex(8)
    print(urlunsplit((uri.scheme, uri.netloc, uri.path or '/postgres', urlencode(query), '')))
    print(urlunsplit((uri.scheme, uri.netloc, '/' + db, urlencode(query), '')))
    print(db)
except (ValueError, TypeError):
    print('❌ ROUNDTRIP_DSN 须为 PostgreSQL URI；query 仅支持 sslmode/connect_timeout，不允许连接或库名覆盖', file=sys.stderr)
    sys.exit(2)
PYDSN
)"
ADMIN_DSN="${CONNECTIONS%%$'\n'*}"
REST="${CONNECTIONS#*$'\n'}"
DSN="${REST%%$'\n'*}"
DB="${REST#*$'\n'}"
CAN_CREATE="$(psql -X -w -v ON_ERROR_STOP=1 "$ADMIN_DSN" -Atqc \
  'SELECT rolsuper OR rolcreatedb FROM pg_roles WHERE rolname = current_user')"
[ "$CAN_CREATE" = "t" ] || { echo "❌ 往返门禁连接账号需要 CREATEDB 权限" >&2; exit 2; }
if [ "$CHECK_ONLY" -eq 1 ]; then echo "✅ 往返门禁只读预检通过"; exit 0; fi

umask 077
TMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/zz-roundtrip.XXXXXXXX")"
DB_CREATED=0
cleanup() {
  local status=$?
  if [ "$DB_CREATED" -eq 1 ]; then
    if ! psql -X -w -v ON_ERROR_STOP=1 "$ADMIN_DSN" -c "DROP DATABASE \"$DB\";" >/dev/null; then
      echo "❌ 临时往返库清理失败：$DB" >&2
      status=1
    fi
  fi
  rm -rf -- "$TMP_DIR"
  exit "$status"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

psql -X -w -v ON_ERROR_STOP=1 "$ADMIN_DSN" -c "CREATE DATABASE \"$DB\";" >/dev/null
DB_CREATED=1
"$MIG" -path migrations -database "$DSN" up >/dev/null
LAST=$("$MIG" -path migrations -database "$DSN" version 2>&1 | awk '/^[0-9]+$/{print $1}')
[ -n "$LAST" ] || { echo "❌ 无法解析 migrate version（stderr 未输出数字）" >&2; exit 1; }
if [ "$N" = "999" ]; then N=$LAST; fi

# 28 批 SYS-2：内容级快照——逐行投影+稳定排序，等量互换/内容改写均可 diff 出
snapshot() {
  psql -X -w -v ON_ERROR_STOP=1 "$DSN" -At -F $'\x1f' -c "
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
           coalesce(v2,''), coalesce(v3,''),coalesce(v4,''),coalesce(v5,''),'','','',''
      FROM casbin_rule
    UNION ALL
    -- P4 业务表（审计修正 2026-09-30：000033 空 down 即在此盲区漏网——批一承诺兑现）
    SELECT 'notification_configs', code, name, channel, webhook_url, enabled::text,
           version::text, '','','','',''
      FROM notification_configs
    UNION ALL
    SELECT 'dict_types', code, name, enabled::text, remark, version::text,
           '','','','','',''
      FROM dict_types
    UNION ALL
    SELECT 'dict_items', type_code, code, label, sort_order::text, enabled::text,
           remark, version::text, '','','',''
      FROM dict_items
    UNION ALL
    SELECT 'personal_access_tokens', name, coalesce(secret_hash,''), coalesce(scope,''),
           coalesce(expires_at::text,''), coalesce(revoked_at::text,''), '','','','','',''
      FROM personal_access_tokens
  " | LC_ALL=C sort
}

snapshot > "$TMP_DIR/rt_before.txt"
"$MIG" -path migrations -database "$DSN" down "$N" >/dev/null
"$MIG" -path migrations -database "$DSN" up >/dev/null
snapshot > "$TMP_DIR/rt_after.txt"

if ! diff -u "$TMP_DIR/rt_before.txt" "$TMP_DIR/rt_after.txt"; then
  echo "❌ 迁移往返数据漂移（down 后 up 与原态不一致）"
  exit 1
fi
echo "✅ 往返零漂移（$N 对，menus/menu_apis/role_menus/casbin_rule/P4 四业务表 计数+明细）"
