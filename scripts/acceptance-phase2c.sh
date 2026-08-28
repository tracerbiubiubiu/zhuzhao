#!/usr/bin/env bash
# Phase 2c 收口验收（M2c-3）：头段回归 phase2b（链式含 2a + Phase 1），再跑 2c 断言（P2-D5）
# 覆盖：D1–D9 + D11 回归（HTTP 层）——SetOwners 双轨 / 组内防提权 D2-D5 / owner 删 vg D6 /
#       工单委托 D7-D8 / ancestor owner D9；D10 HR 隔离为「字段不被 HR 触碰」静态断言（HR Job 延后）
# 运行前置：make docker-dev-reset && make dev（或等价部署）
set -euo pipefail

BASE="${BASE_URL:-http://localhost:33333/api/v1}"
HEALTH="${HEALTH_URL:-http://localhost:33333/health/ready}"

detect_container() {
  local candidate
  for candidate in "$@"; do
    if docker ps --format '{{.Names}}' | grep -qx "$candidate"; then
      echo "$candidate"
      return 0
    fi
  done
  return 1
}
PG="${PG_CONTAINER:-$(detect_container zhuzhao-dev-postgres zhuzhao-postgres || echo zhuzhao-postgres)}"
PG_USER="${PG_USER:-zhuzhao}"
PG_DB="${PG_DB:-zhuzhao}"
REDIS="${REDIS_CONTAINER:-$(detect_container zhuzhao-dev-redis zhuzhao-redis || echo zhuzhao-redis)}"

pass=0
fail=0
sectionA=0

cleanup_redis() { docker start "$REDIS" >/dev/null 2>&1 || true; }
trap cleanup_redis EXIT

json_code() { python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('code') if d else '')" 2>/dev/null || echo PARSE_ERR; }
psql_q()    { docker exec "$PG" psql -U "$PG_USER" -d "$PG_DB" -qt -A -c "$1" | tr -d '\n'; }

check() {
  local name="$1" expect="$2" got="$3"
  if [ "$got" = "$expect" ]; then
    echo "PASS $name"; pass=$((pass + 1))
  else
    echo "FAIL $name expect=$expect got=$got"; fail=$((fail + 1))
  fi
}

# ============================================================
# Section A: 头段回归——phase2b 全量（链式含 phase2a + Phase 1）
# ============================================================
echo "========== Section A: phase2b 回归（含 2a + Phase 1） =========="
if PG_CONTAINER="$PG" PG_USER="$PG_USER" PG_DB="$PG_DB" REDIS_CONTAINER="$REDIS" \
   BASE="$BASE" HEALTH="$HEALTH" bash "$(dirname -- "${BASH_SOURCE[0]}")/acceptance-phase2b.sh"; then
  sectionA=1
  echo "Section A: phase2b 回归 — ALL PASS"
else
  sectionA=0
  echo "Section A: phase2b 回归 — FAILED（继续 Section B 供排查）"
fi

echo "========== Section B: Phase 2c（组织委托 D1–D9 + D11 回归） =========="

# --- superadmin（幂等登录，与 2b 脚本同式）---
SA=$(curl -s -X POST "$BASE/auth/login" -H 'Content-Type: application/json' -d '{"employee_no":"E000001","password":"admin12345"}')
if [ "$(echo "$SA" | json_code)" != "0" ]; then
  SA=$(curl -s -X POST "$BASE/auth/login" -H 'Content-Type: application/json' -d '{"employee_no":"E000001","password":"admin123"}')
  SAT=$(echo "$SA" | python3 -c "import sys,json; print(json.load(sys.stdin)['data']['access_token'])" 2>/dev/null || true)
  curl -s -X POST "$BASE/auth/password/update" -H "Authorization: Bearer $SAT" -H 'Content-Type: application/json' \
    -d '{"old_password":"admin123","new_password":"admin12345","device_id":"phase2c"}' >/dev/null
  SA=$(curl -s -X POST "$BASE/auth/login" -H 'Content-Type: application/json' -d '{"employee_no":"E000001","password":"admin12345"}')
fi
check "sa login" "0" "$(echo "$SA" | json_code)"
SAT=$(echo "$SA" | python3 -c "import sys,json; print(json.load(sys.stdin)['data']['access_token'])")

SUF=$$
ROOT_ID=$(psql_q "SELECT id FROM organizations WHERE parent_id IS NULL ORDER BY id LIMIT 1")
mkorg() {
  curl -s -X POST "$BASE/orgs" -H "Authorization: Bearer $SAT" -H 'Content-Type: application/json' \
    -d "{\"code\":\"$1\",\"name\":\"$2\",\"parent_id\":\"$3\",\"org_type\":$4,\"sort_order\":60}" \
    | python3 -c "import sys,json; print((json.load(sys.stdin).get('data') or {}).get('id','0'))"
}
mkuser() {
  curl -s -X POST "$BASE/users" -H "Authorization: Bearer $SAT" -H 'Content-Type: application/json' \
    -d "{\"username\":\"$1\",\"password\":\"pass1234\",\"employee_no\":\"$2\",\"role_ids\":[\"$3\"],\"org_ids\":[\"$4\"]}" \
    | python3 -c "import sys,json; print((json.load(sys.stdin).get('data') or {}).get('id','0'))"
}
mklogin() {
  curl -s -X POST "$BASE/auth/login" -H 'Content-Type: application/json' \
    -d "{\"employee_no\":\"$1\",\"password\":\"pass1234\"}" \
    | python3 -c "import sys,json; print(json.load(sys.stdin)['data']['access_token'])" 2>/dev/null || true
}
http_of() { curl -s -o /tmp/p2c.json -w '%{http_code}' "$@"; }
code_of() { cat /tmp/p2c.json | json_code; }

# --- 环境：P(实体) > VG(虚拟组)；owner/admin/m1/m2/admin2 五用户 ---
ROLE_V=$(psql_q "SELECT id FROM roles WHERE code='viewer'")
ROLE_O=$(psql_q "SELECT id FROM roles WHERE code='operator'")
P_ID=$(mkorg "p2c_p_$SUF" "2c P" "$ROOT_ID" 3)
VG_ID=$(mkorg "vg_2c_$SUF" "2c VG" "$P_ID" 4)
check "create P/VG" "1" "$([ "$P_ID" != "0" ] && [ "$VG_ID" != "0" ] && echo 1 || echo 0)"

# operator 角色：Section A（phase2a）已授予 operator 工单菜单并持久——L1 放行后，
# D 系列的 403/200 差异才能精确落在 L3 委托/成员关系上（D7 PRD 要求 Casbin+Authorize 双过）
OWNER=$(mkuser "p2c_own_$SUF" "C2OWN$SUF" "$ROLE_O" "$VG_ID")
ADMIN=$(mkuser "p2c_adm_$SUF" "C2ADM$SUF" "$ROLE_O" "$VG_ID")
M1=$(mkuser "p2c_m1_$SUF" "C2M1$SUF" "$ROLE_O" "$VG_ID")
M2=$(mkuser "p2c_m2_$SUF" "C2M2$SUF" "$ROLE_O" "$VG_ID")
ADM2=$(mkuser "p2c_ad2_$SUF" "C2AD2$SUF" "$ROLE_O" "$VG_ID")
check "create users" "1" "$([ "$OWNER" != "0" ] && [ "$ADMIN" != "0" ] && [ "$M1" != "0" ] && echo 1 || echo 0)"
OWNTOK=$(mklogin "C2OWN$SUF"); ADMTOK=$(mklogin "C2ADM$SUF"); M1TOK=$(mklogin "C2M1$SUF")
check "users login" "1" "$([ -n "$OWNTOK" ] && [ -n "$ADMTOK" ] && [ -n "$M1TOK" ] && echo 1 || echo 0)"

# --- D1 SetOwners（SAT 全局）：owner_user_ids + 双轨 org_member_role=owner ---
D1=$(curl -s -X POST "$BASE/orgs/owners" -H "Authorization: Bearer $SAT" -H 'Content-Type: application/json' \
  -d "{\"org_id\":\"$VG_ID\",\"owner_user_ids\":[\"$OWNER\"]}")
check "D1 set owners code=0" "0" "$(echo "$D1" | json_code)"
check "D1 owner_user_ids 含 owner" "1" \
  "$(psql_q "SELECT CASE WHEN $OWNER = ANY(owner_user_ids) THEN 1 ELSE 0 END FROM organizations WHERE id=$VG_ID")"
check "D1 双轨 org_member_role=owner" "owner" \
  "$(psql_q "SELECT org_member_role FROM user_orgs WHERE org_id=$VG_ID AND user_id=$OWNER")"

# --- D2 owner 任命 m1 → admin ---
D2_RAW=$(curl -s -X POST "$BASE/orgs/members/role" -H "Authorization: Bearer $OWNTOK" -H 'Content-Type: application/json' \
  -d "{\"org_id\":\"$VG_ID\",\"user_id\":\"$M1\",\"org_member_role\":\"admin\"}")
check "D2 owner appoints admin (0)" "0" "$(echo "$D2_RAW" | json_code)"
check "D2 m1 role=admin" "admin" "$(psql_q "SELECT org_member_role FROM user_orgs WHERE org_id=$VG_ID AND user_id=$M1")"
# 环境补齐：ADMIN/ADM2 由 owner 任命为 admin（D3–D5 的「组 admin」主体）
D2A_RAW=$(curl -s -X POST "$BASE/orgs/members/role" -H "Authorization: Bearer $OWNTOK" -H 'Content-Type: application/json' \
  -d "{\"org_id\":\"$VG_ID\",\"user_id\":\"$ADMIN\",\"org_member_role\":\"admin\"}")
check "D2 owner appoints ADMIN (0)" "0" "$(echo "$D2A_RAW" | json_code)"
D2B_RAW=$(curl -s -X POST "$BASE/orgs/members/role" -H "Authorization: Bearer $OWNTOK" -H 'Content-Type: application/json' \
  -d "{\"org_id\":\"$VG_ID\",\"user_id\":\"$ADM2\",\"org_member_role\":\"admin\"}")
check "D2 owner appoints ADM2 (0)" "0" "$(echo "$D2B_RAW" | json_code)"

# --- D3 admin 调用 SetMemberRole → 50010；请求 owner → 400 ---
D3_HTTP=$(curl -s -o /tmp/p2c_d3.json -w '%{http_code}' -X POST "$BASE/orgs/members/role" -H "Authorization: Bearer $ADMTOK" -H 'Content-Type: application/json' -d "{\"org_id\":\"$VG_ID\",\"user_id\":\"$M2\",\"org_member_role\":\"admin\"}")
check "D3 admin calls (403)" "403" "$D3_HTTP"
check "D3 code=50010" "50010" "$(cat /tmp/p2c_d3.json | json_code)"
check "D3 request owner → 400" "400" \
  "$(http_of -X POST "$BASE/orgs/members/role" -H "Authorization: Bearer $OWNTOK" -H 'Content-Type: application/json' \
    -d "{\"org_id\":\"$VG_ID\",\"user_id\":\"$M2\",\"org_member_role\":\"owner\"}")"

# --- D4 admin 移除 member m2 → 200；D5 admin 移除 admin → 50009 ---
D4_RAW=$(curl -s -X POST "$BASE/orgs/members/delete" -H "Authorization: Bearer $ADMTOK" -H 'Content-Type: application/json' \
  -d "{\"org_id\":\"$VG_ID\",\"user_id\":\"$M2\"}")
check "D4 admin removes member (0)" "0" "$(echo "$D4_RAW" | json_code)"
D5_HTTP=$(curl -s -o /tmp/p2c_d5.json -w '%{http_code}' -X POST "$BASE/orgs/members/delete" -H "Authorization: Bearer $ADMTOK" -H 'Content-Type: application/json' -d "{\"org_id\":\"$VG_ID\",\"user_id\":\"$ADM2\"}")
check "D5 admin removes admin (403)" "403" "$D5_HTTP"
cp /tmp/p2c_d5.json /tmp/p2c.json  # 供 code_of 读取
check "D5 code=50009" "50009" "$(code_of)"

# --- D7/D8 工单委托：m1(admin) 建?——用 owner 建单，admin（非创建人）凭委托 update/close；
#     member m2 已被移除，复建一个 member 用户 ---
M3=$(mkuser "p2c_m3_$SUF" "C2M3$SUF" "$ROLE_O" "$VG_ID")
M3TOK=$(mklogin "C2M3$SUF")
TK=$(curl -s -X POST "$BASE/tickets" -H "Authorization: Bearer $M3TOK" -H 'Content-Type: application/json' \
  -d "{\"type_code\":\"incident\",\"title\":\"D7 工单\",\"org_id\":\"$VG_ID\"}")
TID=$(echo "$TK" | python3 -c "import sys,json; print((json.load(sys.stdin).get('data') or {}).get('id','0'))")
check "D7 member creates ticket" "1" "$([ "$TID" != "0" ] && echo 1 || echo 0)"

# D7：vg admin（ADMTOK，非创建人）update → 200
D7_RAW=$(curl -s -X POST "$BASE/tickets/update" -H "Authorization: Bearer $ADMTOK" -H 'Content-Type: application/json' \
  -d "{\"id\":\"$TID\",\"title\":\"admin rename by delegation\"}")
check "D7 vg admin updates ticket (0)" "0" "$(echo "$D7_RAW" | json_code)"
# D8：member（M3TOK 是创建人——需非创建人 member；owner 建第二单，M3 试改 → 403）
TK2=$(curl -s -X POST "$BASE/tickets" -H "Authorization: Bearer $OWNTOK" -H 'Content-Type: application/json' \
  -d "{\"type_code\":\"incident\",\"title\":\"D8 工单\",\"org_id\":\"$VG_ID\"}")
TID2=$(echo "$TK2" | python3 -c "import sys,json; print((json.load(sys.stdin).get('data') or {}).get('id','0'))")
D8_HTTP=$(curl -s -o /tmp/p2c_d8.json -w '%{http_code}' -X POST "$BASE/tickets/update" -H "Authorization: Bearer $M3TOK" -H 'Content-Type: application/json' -d "{\"id\":\"$TID2\",\"title\":\"member cross update\"}")
check "D8 member updates others (403)" "403" "$D8_HTTP"

# --- D9 ancestor owner：P 设 owner（复用 OWNER?需另一实体 owner 用户）---
POWN=$(mkuser "p2c_pown_$SUF" "C2PN$SUF" "$ROLE_O" "$P_ID")
D9S_RAW=$(curl -s -X POST "$BASE/orgs/owners" -H "Authorization: Bearer $SAT" -H 'Content-Type: application/json' \
  -d "{\"org_id\":\"$P_ID\",\"owner_user_ids\":[\"$POWN\"]}")
check "D9 set P owner (0)" "0" "$(echo "$D9S_RAW" | json_code)"
PTOK=$(mklogin "C2PN$SUF")
D9_RAW=$(curl -s -X POST "$BASE/tickets/update" -H "Authorization: Bearer $PTOK" -H 'Content-Type: application/json' \
  -d "{\"id\":\"$TID2\",\"title\":\"ancestor owner rename\"}")
check "D9 ancestor owner updates vg ticket (0)" "0" "$(echo "$D9_RAW" | json_code)"

# --- D6 owner 删空虚拟组：先清成员（SAT 全局），owner 删 → 200；有成员时 → 409 ---
VG2=$(mkorg "vg_2c6_$SUF" "2c VG6" "$P_ID" 4)
G6M=$(mkuser "p2c_g6_$SUF" "C2G6$SUF" "$ROLE_V" "$VG2")
D6_HTTP=$(curl -s -o /tmp/p2c_d6.json -w '%{http_code}' -X POST "$BASE/orgs/delete" -H "Authorization: Bearer $SAT" -H 'Content-Type: application/json' -d "{\"org_id\":\"$VG2\"}")
check "D6 vg with member → 409" "409" "$D6_HTTP"
curl -s -X POST "$BASE/orgs/members/delete" -H "Authorization: Bearer $SAT" -H 'Content-Type: application/json' \
  -d "{\"org_id\":\"$VG2\",\"user_id\":\"$G6M\"}" >/dev/null
# SAT 设置 VG2 的 owner 为 OWNER 用户，owner 删空组
curl -s -X POST "$BASE/orgs/owners" -H "Authorization: Bearer $SAT" -H 'Content-Type: application/json' \
  -d "{\"org_id\":\"$VG2\",\"owner_user_ids\":[\"$OWNER\"]}" >/dev/null
D6B_RAW=$(curl -s -X POST "$BASE/orgs/delete" -H "Authorization: Bearer $OWNTOK" -H 'Content-Type: application/json' -d "{\"org_id\":\"$VG2\"}")
check "D6 owner deletes empty vg (0)" "0" "$(echo "$D6B_RAW" | json_code)"

# --- D11 回归：vg member 对兄弟 vg 工单可读不可改（策略 B；2c 下委托者也不能跨 vg）---
VG3=$(mkorg "vg_2c11_$SUF" "2c VG11" "$P_ID" 4)
V3M=$(mkuser "p2c_v3_$SUF" "C2V3$SUF" "$ROLE_O" "$VG3")
V3TOK=$(mklogin "C2V3$SUF")
TK3=$(curl -s -X POST "$BASE/tickets" -H "Authorization: Bearer $V3TOK" -H 'Content-Type: application/json' \
  -d "{\"type_code\":\"incident\",\"title\":\"D11 工单\",\"org_id\":\"$VG3\"}")
TID3=$(echo "$TK3" | python3 -c "import sys,json; print((json.load(sys.stdin).get('data') or {}).get('id','0'))")
D11R_HTTP=$(curl -s -o /dev/null -w '%{http_code}' "$BASE/tickets/$TID3" -H "Authorization: Bearer $ADMTOK")
check "D11 vg admin reads sibling vg (200)" "200" "$D11R_HTTP"
D11U_HTTP=$(curl -s -o /dev/null -w '%{http_code}' -X POST "$BASE/tickets/update" -H "Authorization: Bearer $ADMTOK" -H 'Content-Type: application/json' -d "{\"id\":\"$TID3\",\"title\":\"cross vg update\"}")
check "D11 vg admin updates sibling vg (403)" "403" "$D11U_HTTP"

# --- D10 静态断言：owner_user_ids / org_member_role 为 IAM 本地字段（HR Job 延后，
#     迁移与字段无 HR 写入路径；此处断言 user_orgs.source≠hr 的行才可能携带 org_member_role≠member?）
#     简化为：确认两列存在且默认值行为正确（HR 对账路径 2b-ext 落地后补动态回归）
check "D10 columns exist (owner_user_ids)" "1" \
  "$(psql_q "SELECT count(*) FROM information_schema.columns WHERE table_name='organizations' AND column_name='owner_user_ids'")"
check "D10 columns exist (org_member_role)" "1" \
  "$(psql_q "SELECT count(*) FROM information_schema.columns WHERE table_name='user_orgs' AND column_name='org_member_role'")"

# ============================================================
echo ""
if [ "$sectionA" -eq 1 ] && [ "$fail" -eq 0 ]; then
  echo "========== Phase 2c Acceptance Summary =========="
  echo "PASS=$pass  FAIL=0  SKIP=0（Section A 回归通过）"
  echo "✅ Phase 2c ALL PASSED"
  exit 0
else
  echo "========== Phase 2c Acceptance Summary =========="
  echo "PASS=$pass  FAIL=$fail  SectionA_OK=$sectionA"
  echo "❌ Phase 2c HAS FAILURES"
  exit 1
fi
