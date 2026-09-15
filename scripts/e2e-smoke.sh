#!/usr/bin/env bash
# 双栈 E2E 冒烟自检：login → RT 轮换 → 旧 RT 失效 → /al 反代读+写。
# 用法：BASE_URL/E2E_EMPLOYEE_NO/E2E_PASSWORD/E2E_DEVICE 可用 env 覆盖
#   例：BASE_URL=http://localhost:33333 E2E_PASSWORD='Phase3Demo#2026' ./scripts/e2e-smoke.sh
# 退出码：0=全绿；1=任一断言失败。只读+建冒烟类型，不改既有数据。
set -uo pipefail

BASE="${BASE_URL:-http://localhost:33333}"
EMP="${E2E_EMPLOYEE_NO:-E000001}"
PASS="${E2E_PASSWORD:?'需注入 E2E_PASSWORD（演示栈 Phase3Demo#2026 / dev 库见 runbook 环境速查）'}"
DEVICE="${E2E_DEVICE:-smoke-$(date +%s)}"
TYPE_NAME="smoke_$(date +%s)"
PASS_COUNT=0

ok()   { PASS_COUNT=$((PASS_COUNT+1)); echo "✓ $1"; }
fail() { echo "✗ $1"; exit 1; }
code() { python3 -c "import json,sys;print(json.load(sys.stdin).get('code'))" 2>/dev/null; }

J() { python3 -c "import json,sys;d=json.load(sys.stdin);print(d$1)" 2>/dev/null; }

# ── 1. 登录 ──
R=$(curl -s -X POST "$BASE/api/v1/auth/login" -H 'Content-Type: application/json' \
  -d "{\"employee_no\":\"$EMP\",\"password\":\"$PASS\",\"device_id\":\"$DEVICE\"}")
[ "$(echo "$R" | code)" = "0" ] || fail "login 失败: $(echo "$R" | head -c 120)"
AT=$(echo "$R" | J "['data']['access_token']")
RT1=$(echo "$R" | J "['data']['refresh_token']")
[ -n "$AT" ] && [ -n "$RT1" ] || fail "login 未返回双 Token"
ok "login（工号 ${EMP}，设备 ${DEVICE}）"

# ── 2. RT 轮换：刷 RT1 → RT2，旧 RT1 复用应 401 ──
R=$(curl -s -X POST "$BASE/api/v1/auth/refresh" -H 'Content-Type: application/json' \
  -d "{\"refresh_token\":\"$RT1\"}")
[ "$(echo "$R" | code)" = "0" ] || fail "refresh RT1 失败"
RT2=$(echo "$R" | J "['data']['refresh_token']")
[ -n "$RT2" ] && [ "$RT2" != "$RT1" ] || fail "RT 未轮换"
R=$(curl -s -o /dev/null -w "%{http_code}" -X POST "$BASE/api/v1/auth/refresh" \
  -H 'Content-Type: application/json' -d "{\"refresh_token\":\"$RT1\"}")
[ "$R" = "401" ] || fail "旧 RT1 复用未被拒（HTTP $R，应 401——单槽轮换语义）"
ok "RT 轮换 + 旧 RT 复用 401"

# ── 3. 网关反代读：/al 类型列表 ──
R=$(curl -s -o /tmp/e2e_smoke_read.json -w "%{http_code}" \
  "$BASE/al/api/v1/admin/types" -H "Authorization: Bearer $AT" \
  -H "X-Request-ID: req-smoke-read")
[ "$R" = "200" ] || fail "GET /al/admin/types HTTP $R"
[ "$(cat /tmp/e2e_smoke_read.json | code)" = "0" ] || fail "activelist 信封 code≠0"
RID=$(python3 -c "import json;print((json.load(open('/tmp/e2e_smoke_read.json')).get('request_id') or '')[:8])" 2>/dev/null || true)
[ -n "$RID" ] || fail "响应缺 request_id（信封贯通断言）"
ok "网关反代读（/al 信封 code=0，request_id=${RID}…）"

# ── 4. 网关反代写：建冒烟类型（动态建表全链）──
R=$(curl -s -o /tmp/e2e_smoke_write.json -w "%{http_code}" -X POST \
  "$BASE/al/api/v1/admin/types" -H "Authorization: Bearer $AT" \
  -H "X-Request-ID: req-smoke-write" -H 'Content-Type: application/json' \
  -d "{\"type_name\":\"$TYPE_NAME\",\"fields\":[{\"name\":\"env\",\"type\":\"string\",\"required\":false}]}")
[ "$R" = "200" ] || fail "POST /al/admin/types HTTP $R"
[ "$(cat /tmp/e2e_smoke_write.json | code)" = "0" ] || fail "建类型信封 code≠0"
ok "网关反代写（动态建表 ${TYPE_NAME}）"

echo "── E2E 冒烟全绿（$PASS_COUNT 项断言）──"
exit 0
