#!/usr/bin/env bash
# Phase 1 full acceptance (docs/phase1/README.md §1.3 + 08-audit.md)
set -euo pipefail

BASE="${BASE_URL:-http://localhost:33333/api/v1}"
HEALTH="${HEALTH_URL:-http://localhost:33333/health/ready}"
PG="${PG_CONTAINER:-zhuzhao-postgres}"
PG_USER="${PG_USER:-zhuzhao}"
PG_DB="${PG_DB:-zhuzhao}"

pass=0
fail=0
skip=0

cleanup_redis() {
  docker start "${REDIS_CONTAINER:-zhuzhao-redis}" >/dev/null 2>&1 || true
}
trap cleanup_redis EXIT

json_code() { python3 -c "import sys,json; print(json.load(sys.stdin).get('code',''))"; }
json_field() { python3 -c "import sys,json; d=json.load(sys.stdin); print($1)"; }
psql_q() { docker exec "$PG" psql -U "$PG_USER" -d "$PG_DB" -qt -A -c "$1" | tr -d '\n'; }
json_body() { python3 -c 'import json,sys; print(json.dumps(json.loads(sys.argv[1])))' "$1"; }

check() {
  local name="$1" expect="$2" got="$3"
  if [ "$got" = "$expect" ]; then
    echo "PASS $name"
    pass=$((pass + 1))
  else
    echo "FAIL $name expect=$expect got=$got"
    fail=$((fail + 1))
  fi
}

skip_case() {
  echo "SKIP $1"
  skip=$((skip + 1))
}

echo "=== Phase 1 Acceptance ==="
echo "BASE=$BASE"

# --- M1 #1 ---
R=$(curl -s "$HEALTH" | python3 -c "import sys,json; print(json.load(sys.stdin).get('status',''))")
check "#1 ready" "ok" "$R"
MENUS=$(psql_q "SELECT COUNT(*) FROM menus")
check "#1 menus>=25" "1" "$([ "$MENUS" -ge 25 ] && echo 1 || echo 0)"
CASBIN=$(psql_q "SELECT COUNT(*) FROM casbin_rule WHERE v0 IN ('role::admin','role::superadmin')")
check "#1 casbin seed" "1" "$([ "$CASBIN" -ge 2 ] && echo 1 || echo 0)"

# --- login ---
SA=$(curl -s -X POST "$BASE/auth/login" -H 'Content-Type: application/json' -d '{"employee_no":"E000001","password":"admin123"}')
check "#2 login" "0" "$(echo "$SA" | json_code)"
SAT=$(echo "$SA" | python3 -c "import sys,json; print(json.load(sys.stdin)['data']['access_token'])")
SRT=$(echo "$SA" | python3 -c "import sys,json; print(json.load(sys.stdin)['data']['refresh_token'])")

# F-10：种子 admin 首登强制改密（mcp=true），需先改密再拿无 mcp 的 AT
check "mcp change" "0" "$(curl -s -X POST "$BASE/auth/password/update" -H "Authorization: Bearer $SAT" -H 'Content-Type: application/json' -d '{"old_password":"admin123","new_password":"admin12345","device_id":"acceptance"}' | json_code)"
SA=$(curl -s -X POST "$BASE/auth/login" -H 'Content-Type: application/json' -d '{"employee_no":"E000001","password":"admin12345"}')
SAT=$(echo "$SA" | python3 -c "import sys,json; print(json.load(sys.stdin)['data']['access_token'])")
SRT=$(echo "$SA" | python3 -c "import sys,json; print(json.load(sys.stdin)['data']['refresh_token'])")

check "#3 menus" "0" "$(curl -s "$BASE/user/menus" -H "Authorization: Bearer $SAT" | json_code)"
check "#4 perms" "0" "$(curl -s "$BASE/user/permissions" -H "Authorization: Bearer $SAT" | json_code)"
check "#5 users" "0" "$(curl -s "$BASE/users" -H "Authorization: Bearer $SAT" | json_code)"

NR=$(curl -s -X POST "$BASE/auth/refresh" -H 'Content-Type: application/json' -d "{\"refresh_token\":\"$SRT\"}")
check "#6 refresh" "0" "$(echo "$NR" | json_code)"
SAT2=$(echo "$NR" | python3 -c "import sys,json; print(json.load(sys.stdin)['data']['access_token'])")
SRT2=$(echo "$NR" | python3 -c "import sys,json; print(json.load(sys.stdin)['data']['refresh_token'])")

check "#7 logout" "0" "$(curl -s -X POST "$BASE/auth/logout" -H "Authorization: Bearer $SAT2" -H 'Content-Type: application/json' -d '{}' | json_code)"
HC=$(curl -s -o /tmp/p1.json -w "%{http_code}" "$BASE/users" -H "Authorization: Bearer $SAT2")
check "#8 post-logout http" "401" "$HC"

SA=$(curl -s -X POST "$BASE/auth/login" -H 'Content-Type: application/json' -d '{"employee_no":"E000001","password":"admin12345"}')
SAT=$(echo "$SA" | python3 -c "import sys,json; print(json.load(sys.stdin)['data']['access_token'])")

# --- #9 ---
B1=$(curl -s -X POST "$BASE/auth/login" -H 'Content-Type: application/json' -d '{"employee_no":"E000001","password":"wrong"}' | python3 -c "import sys,json; print(json.load(sys.stdin)['message'])")
B2=$(curl -s -X POST "$BASE/auth/login" -H 'Content-Type: application/json' -d '{"employee_no":"NOPE999","password":"wrong"}' | python3 -c "import sys,json; print(json.load(sys.stdin)['message'])")
check "#9 enum" "$B1" "$B2"

# --- #10 lockout ---
LOCK_EN="EL$RANDOM"
for i in 1 2 3 4 5; do
  curl -s -X POST "$BASE/auth/login" -H 'Content-Type: application/json' -d "{\"employee_no\":\"$LOCK_EN\",\"password\":\"wrong\"}" >/dev/null
done
HC=$(curl -s -o /tmp/p1.json -w "%{http_code}" -X POST "$BASE/auth/login" -H 'Content-Type: application/json' -d "{\"employee_no\":\"$LOCK_EN\",\"password\":\"wrong\"}")
CODE=$(cat /tmp/p1.json | json_code)
check "#10 http" "429" "$HC"
check "#10 code" "20006" "$CODE"

# --- #22 ---
HC=$(curl -s -o /tmp/p1.json -w "%{http_code}" "$BASE/users" -H "Authorization: Bearer $SAT" -H 'X-AK-Access-Key: x')
check "#22 http" "400" "$HC"
check "#22 code" "20008" "$(cat /tmp/p1.json | json_code)"

# --- M5 user for #11 ---
SUF=$RANDOM
curl -s -X POST "$BASE/users" -H "Authorization: Bearer $SAT" -H 'Content-Type: application/json' \
  -d "{\"username\":\"u$SUF\",\"password\":\"pass1234\",\"employee_no\":\"E9$SUF\",\"role_ids\":[\"4\"]}" >/tmp/u.json
TUID=$(cat /tmp/u.json | python3 -c "import sys,json; print(json.load(sys.stdin)['data']['id'])")
TARGET_USER_ID="$TUID"
LOGIN=$(curl -s -X POST "$BASE/auth/login" -H 'Content-Type: application/json' -d "{\"employee_no\":\"E9$SUF\",\"password\":\"pass1234\"}")
TAT=$(echo "$LOGIN" | python3 -c "import sys,json; print(json.load(sys.stdin)['data']['access_token'])")
TRT=$(echo "$LOGIN" | python3 -c "import sys,json; print(json.load(sys.stdin)['data']['refresh_token'])")
curl -s -X POST "$BASE/users/status" -H "Authorization: Bearer $SAT" -H 'Content-Type: application/json' \
  -d "{\"user_id\":\"$TUID\",\"status\":0}" >/dev/null
HC=$(curl -s -o /tmp/p1.json -w "%{http_code}" -H "Authorization: Bearer $TAT" "$BASE/users")
check "#11 http" "403" "$HC"
check "#11 code" "30003" "$(cat /tmp/p1.json | json_code)"
HC=$(curl -s -o /tmp/p1.json -w "%{http_code}" -X POST "$BASE/auth/refresh" -H 'Content-Type: application/json' -d "{\"refresh_token\":\"$TRT\"}")
check "#11b http" "401" "$HC"
check "#11b code" "20004" "$(cat /tmp/p1.json | json_code)"
curl -s -X POST "$BASE/users/status" -H "Authorization: Bearer $SAT" -H 'Content-Type: application/json' \
  -d "{\"user_id\":\"$TUID\",\"status\":1}" >/dev/null

# --- #12 ---
HC=$(curl -s -o /tmp/p1.json -w "%{http_code}" -X POST "$BASE/users/status" -H "Authorization: Bearer $SAT" -H 'Content-Type: application/json' -d '{"user_id":"1","status":0}')
check "#12 http" "403" "$HC"
check "#12 code" "30006" "$(cat /tmp/p1.json | json_code)"

# --- #13 #21 admin user ---
AEN="E8$SUF"
curl -s -X POST "$BASE/users" -H "Authorization: Bearer $SAT" -H 'Content-Type: application/json' \
  -d "{\"username\":\"a$SUF\",\"password\":\"pass1234\",\"employee_no\":\"$AEN\",\"role_ids\":[\"2\"]}" >/dev/null
AAT=$(curl -s -X POST "$BASE/auth/login" -H 'Content-Type: application/json' -d "{\"employee_no\":\"$AEN\",\"password\":\"pass1234\"}" | python3 -c "import sys,json; print(json.load(sys.stdin)['data']['access_token'])")
HC=$(curl -s -o /tmp/p1.json -w "%{http_code}" -X POST "$BASE/users/password/reset" -H "Authorization: Bearer $AAT" -H 'Content-Type: application/json' -d '{"user_id":"1","password":"newpass1234"}')
check "#13 http" "403" "$HC"
check "#13 code" "30005" "$(cat /tmp/p1.json | json_code)"
SUPER_ROLE=$(psql_q "SELECT id FROM roles WHERE code='superadmin'")
HC=$(curl -s -o /tmp/p1.json -w "%{http_code}" -X POST "$BASE/users/roles" -H "Authorization: Bearer $AAT" -H 'Content-Type: application/json' -d "{\"user_id\":\"$TUID\",\"role_ids\":[\"$SUPER_ROLE\"]}")
check "#21 http" "403" "$HC"
check "#21 code" "30009" "$(cat /tmp/p1.json | json_code)"

# --- B1-2 目标校验：admin（非 superadmin）改系统角色菜单 → 403 + 40004 ---
VIEWER_ROLE_ID=$(psql_q "SELECT id FROM roles WHERE code='viewer'")
HC=$(curl -s -o /tmp/p1.json -w "%{http_code}" -X POST "$BASE/roles/menus" -H "Authorization: Bearer $AAT" -H 'Content-Type: application/json' -d "{\"role_id\":\"$VIEWER_ROLE_ID\",\"menu_ids\":[]}")
check "B1-2 sys-menu http" "403" "$HC"
check "B1-2 sys-menu code" "40004" "$(cat /tmp/p1.json | json_code)"
# --- B1-2 目标校验：admin 删同级 admin 用户 → 403 + 30010（通用防提权码） ---
PEER_ID=$(psql_q "SELECT id FROM users WHERE employee_no='$AEN' AND deleted_at IS NULL")
HC=$(curl -s -o /tmp/p1.json -w "%{http_code}" -X POST "$BASE/users/delete" -H "Authorization: Bearer $AAT" -H 'Content-Type: application/json' -d "{\"user_id\":\"$PEER_ID\"}")
check "B1-2 peer-del http" "403" "$HC"
check "B1-2 peer-del code" "30010" "$(cat /tmp/p1.json | json_code)"

# --- B2 断言组 ---
# B2-2 改密新旧相同 → 400 + 10001
HC=$(curl -s -o /tmp/p1.json -w "%{http_code}" -X POST "$BASE/auth/password/update" -H "Authorization: Bearer $AAT" -H 'Content-Type: application/json' -d '{"old_password":"pass1234","new_password":"pass1234"}')
check "B2-2 same-pwd http" "400" "$HC"
check "B2-2 same-pwd code" "10001" "$(cat /tmp/p1.json | json_code)"
# B2-6 影子超管：admin 读 superadmin 角色详情/菜单/策略 → 404（防推断）
HC=$(curl -s -o /tmp/p1.json -w "%{http_code}" "$BASE/roles/$SUPER_ROLE" -H "Authorization: Bearer $AAT")
check "B2-6 sa-role http" "404" "$HC"
HC=$(curl -s -o /tmp/p1.json -w "%{http_code}" "$BASE/roles/$SUPER_ROLE/menus" -H "Authorization: Bearer $AAT")
check "B2-6 sa-menus http" "404" "$HC"
HC=$(curl -s -o /tmp/p1.json -w "%{http_code}" "$BASE/roles/$SUPER_ROLE/permissions" -H "Authorization: Bearer $AAT")
check "B2-6 sa-perms http" "404" "$HC"
# B2-5 AssignMenus 不存在菜单 → 404 + 60002
HC=$(curl -s -o /tmp/p1.json -w "%{http_code}" -X POST "$BASE/roles/menus" -H "Authorization: Bearer $SAT" -H 'Content-Type: application/json' -d "{\"role_id\":\"$VIEWER_ROLE_ID\",\"menu_ids\":[\"999999\"]}")
check "B2-5 bad-menu http" "404" "$HC"
check "B2-5 bad-menu code" "60002" "$(cat /tmp/p1.json | json_code)"

# --- #14 mcp ---
curl -s -X POST "$BASE/users/password/reset" -H "Authorization: Bearer $SAT" -H 'Content-Type: application/json' \
  -d "{\"user_id\":\"$TUID\",\"password\":\"mcp123456\"}" >/dev/null
MAT=$(curl -s -X POST "$BASE/auth/login" -H 'Content-Type: application/json' -d "{\"employee_no\":\"E9$SUF\",\"password\":\"mcp123456\"}" | python3 -c "import sys,json; print(json.load(sys.stdin)['data']['access_token'])")
HC=$(curl -s -o /tmp/p1.json -w "%{http_code}" -H "Authorization: Bearer $MAT" "$BASE/users")
check "#14 block http" "403" "$HC"
check "#14 block code" "20007" "$(cat /tmp/p1.json | json_code)"
HC=$(curl -s -o /tmp/p1.json -w "%{http_code}" -H "Authorization: Bearer $MAT" "$BASE/user/menus")
check "#14b http" "403" "$HC"
check "#14b code" "20007" "$(cat /tmp/p1.json | json_code)"
check "#14 update" "0" "$(curl -s -X POST "$BASE/auth/password/update" -H "Authorization: Bearer $MAT" -H 'Content-Type: application/json' -d '{"old_password":"mcp123456","new_password":"finalpass1234"}' | json_code)"

# --- #18-20 org ---
# #6b 组织树形态断言：GET /orgs 返回树形结构（顶层含 children 嵌套）
ORG_TREE_OK=$(curl -s "$BASE/orgs" -H "Authorization: Bearer $SAT" | python3 -c "
import sys,json
d=json.load(sys.stdin)['data']
roots=[o for o in d if o.get('parent_id') is None]
nested=[o for o in d if 'children' in o and o['children']]
print(1 if roots and nested else 0)")
check "#6b org-tree" "1" "$ORG_TREE_OK"
# #6c 管理端菜单树形态断言：GET /menus 树形且含按钮节点（menu_type=3，供角色分配勾选）
MENU_TREE_OK=$(curl -s "$BASE/menus" -H "Authorization: Bearer $SAT" | python3 -c "
import sys,json
d=json.load(sys.stdin)['data']
def walk(nodes):
    for n in nodes:
        if n.get('menu_type')==3: return True
        if walk(n.get('children') or []): return True
    return False
print(1 if walk(d) else 0)")
check "#6c menu-tree-buttons" "1" "$MENU_TREE_OK"
BODY18=$(json_body "{\"org_id\":\"2\",\"user_id\":\"$TARGET_USER_ID\"}")
check "#18" "0" "$(curl -s -X POST "$BASE/orgs/members" -H "Authorization: Bearer $SAT" -H 'Content-Type: application/json' -d "$BODY18" | json_code)"
HC=$(curl -s -o /tmp/p1.json -w "%{http_code}" -X POST "$BASE/orgs/members/delete" -H "Authorization: Bearer $SAT" -H 'Content-Type: application/json' -d '{"org_id":"2","user_id":"99999"}')
check "#19 http" "404" "$HC"
check "#19 code" "50007" "$(cat /tmp/p1.json | json_code)"
BODY20=$(json_body "{\"user_id\":\"$TARGET_USER_ID\",\"org_ids\":[\"2\"],\"primary_org_id\":\"2\"}")
HC=$(curl -s -o /tmp/p1.json -w "%{http_code}" -X POST "$BASE/users/orgs" -H "Authorization: Bearer $SAT" -H 'Content-Type: application/json' -d "$BODY20")
check "#20 http" "200" "$HC"
check "#20 code" "0" "$(cat /tmp/p1.json | json_code)"

# --- B3 断言组 ---
# B3-1 AddMember 幂等不降级：先设 primary（org 2），再重复添加同 org 不带 is_primary → 200 且 primary 保持
BODY31=$(json_body "{\"org_id\":\"2\",\"user_id\":\"$TARGET_USER_ID\",\"is_primary\":true}")
check "B3-1 set-primary" "0" "$(curl -s -X POST "$BASE/orgs/members" -H "Authorization: Bearer $SAT" -H 'Content-Type: application/json' -d "$BODY31" | json_code)"
check "B3-1 idempotent" "0" "$(curl -s -X POST "$BASE/orgs/members" -H "Authorization: Bearer $SAT" -H 'Content-Type: application/json' -d "$BODY18" | json_code)"
B31_PRIMARY=$(psql_q "SELECT COUNT(*) FROM user_orgs WHERE user_id=$TARGET_USER_ID AND is_primary")
check "B3-1 primary-kept" "1" "$B31_PRIMARY"
# B3-4 SetUserOrgs 重复 org_id → 去重成功（修复前主键冲突 500）
BODY34=$(json_body "{\"user_id\":\"$TARGET_USER_ID\",\"org_ids\":[\"2\",\"2\"],\"primary_org_id\":\"2\"}")
HC=$(curl -s -o /tmp/p1.json -w "%{http_code}" -X POST "$BASE/users/orgs" -H "Authorization: Bearer $SAT" -H 'Content-Type: application/json' -d "$BODY34")
check "B3-4 dup-orgids http" "200" "$HC"
B34_ROWS=$(psql_q "SELECT COUNT(*) FROM user_orgs WHERE user_id=$TARGET_USER_ID AND org_id=2")
check "B3-4 no-dup-rows" "1" "$B34_ROWS"

# --- #23 #24 ---
check "#23" "1" "$(curl -s "$BASE/users?username=admin" -H "Authorization: Bearer $SAT" | python3 -c "import sys,json; t=json.load(sys.stdin)['data']['total']; print(1 if t>=1 else 0)")"
check "#24" "1" "$(curl -s "$BASE/users?employee_no=E000001" -H "Authorization: Bearer $SAT" | python3 -c "import sys,json; print(json.load(sys.stdin)['data']['total'])")"

# --- #15 no role ---
curl -s -X POST "$BASE/users" -H "Authorization: Bearer $SAT" -H 'Content-Type: application/json' \
  -d "{\"username\":\"nr$SUF\",\"password\":\"pass1234\",\"employee_no\":\"EN$SUF\"}" >/dev/null
NAT=$(curl -s -X POST "$BASE/auth/login" -H 'Content-Type: application/json' -d "{\"employee_no\":\"EN$SUF\",\"password\":\"pass1234\"}" | python3 -c "import sys,json; print(json.load(sys.stdin)['data']['access_token'])")
HC=$(curl -s -o /tmp/p1.json -w "%{http_code}" "$BASE/users" -H "Authorization: Bearer $NAT")
check "#15 http" "403" "$HC"
check "#15 code" "70003" "$(cat /tmp/p1.json | json_code)"
HC=$(curl -s -o /tmp/p1.json -w "%{http_code}" "$BASE/user/menus" -H "Authorization: Bearer $NAT")
check "#15b http" "403" "$HC"

# --- #25 #26 viewer zero menu (fresh casbin) ---
docker exec "$PG" psql -U "$PG_USER" -d "$PG_DB" -c "DELETE FROM role_menus WHERE role_id=(SELECT id FROM roles WHERE code='viewer'); DELETE FROM casbin_rule WHERE v0='role::viewer';" >/dev/null
VEN="E7$SUF"
curl -s -X POST "$BASE/users" -H "Authorization: Bearer $SAT" -H 'Content-Type: application/json' \
  -d "{\"username\":\"v$SUF\",\"password\":\"pass1234\",\"employee_no\":\"$VEN\",\"role_ids\":[\"4\"]}" >/dev/null
# reload casbin: restart not feasible in script — use AssignMenus empty via API not available; kill and note
# For accurate #25/#26, query enforcer: assign empty menus to reload viewer rules
VIEWER_ROLE=$(psql_q "SELECT id FROM roles WHERE code='viewer'")
curl -s -X POST "$BASE/roles/menus" -H "Authorization: Bearer $SAT" -H 'Content-Type: application/json' -d "{\"role_id\":\"$VIEWER_ROLE\",\"menu_ids\":[]}" >/dev/null
VAT=$(curl -s -X POST "$BASE/auth/login" -H 'Content-Type: application/json' -d "{\"employee_no\":\"$VEN\",\"password\":\"pass1234\"}" | python3 -c "import sys,json; print(json.load(sys.stdin)['data']['access_token'])")
MEN=$(curl -s "$BASE/user/menus" -H "Authorization: Bearer $VAT" | python3 -c "import sys,json; print(len(json.load(sys.stdin)['data']['menus']))")
check "#25 menus empty" "0" "$MEN"
HC=$(curl -s -o /tmp/p1.json -w "%{http_code}" "$BASE/users" -H "Authorization: Bearer $VAT")
check "#26 http" "403" "$HC"
check "#26 code" "70001" "$(cat /tmp/p1.json | json_code)"

# --- #27 ---
USER_MENU=$(psql_q "SELECT id FROM menus WHERE code='system_user'")
BODY27=$(json_body "{\"role_id\":\"$VIEWER_ROLE\",\"menu_ids\":[\"$USER_MENU\"]}")
check "#27 assign" "0" "$(curl -s -X POST "$BASE/roles/menus" -H "Authorization: Bearer $SAT" -H 'Content-Type: application/json' -d "$BODY27" | json_code)"
VAT=$(curl -s -X POST "$BASE/auth/login" -H 'Content-Type: application/json' -d "{\"employee_no\":\"$VEN\",\"password\":\"pass1234\"}" | python3 -c "import sys,json; print(json.load(sys.stdin)['data']['access_token'])")
HC=$(curl -s -o /tmp/p1.json -w "%{http_code}" "$BASE/users" -H "Authorization: Bearer $VAT")
check "#27 GET http" "200" "$HC"
check "#27 GET code" "0" "$(cat /tmp/p1.json | json_code)"
HC=$(curl -s -o /tmp/p1.json -w "%{http_code}" -X POST "$BASE/users" -H "Authorization: Bearer $VAT" -H 'Content-Type: application/json' -d "{\"username\":\"xp$SUF\",\"password\":\"xpass1234\",\"employee_no\":\"EX$SUF\"}")
check "#27 POST http" "200" "$HC"
check "#27 POST code" "0" "$(cat /tmp/p1.json | json_code)"

# --- #16 concurrent refresh (before #17) ---
SA=$(curl -s -X POST "$BASE/auth/login" -H 'Content-Type: application/json' -d '{"employee_no":"E000001","password":"admin12345"}')
CRT=$(echo "$SA" | python3 -c "import sys,json; print(json.load(sys.stdin)['data']['refresh_token'])")
curl -s -X POST "$BASE/auth/refresh" -H 'Content-Type: application/json' -d "{\"refresh_token\":\"$CRT\"}" >/tmp/r1.json &
curl -s -X POST "$BASE/auth/refresh" -H 'Content-Type: application/json' -d "{\"refresh_token\":\"$CRT\"}" >/tmp/r2.json &
wait
C1=$(cat /tmp/r1.json | json_code)
C2=$(cat /tmp/r2.json | json_code)
OK=0
[ "$C1" = "0" ] && [ "$C2" = "20004" ] && OK=1
[ "$C2" = "0" ] && [ "$C1" = "20004" ] && OK=1
check "#16 refresh race" "1" "$OK"

# --- M6 audit ---
LOGIN_CNT=$(psql_q "SELECT COUNT(*) FROM audit_logs WHERE path='/api/v1/auth/login' AND created_at > NOW()-INTERVAL '5 minutes'")
check "M6 login audit" "1" "$([ "$LOGIN_CNT" -ge 1 ] && echo 1 || echo 0)"
LB=$(psql_q "SELECT request_body FROM audit_logs WHERE path='/api/v1/auth/login' ORDER BY id DESC LIMIT 1")
check "M6 login pwd mask" "1" "$(echo "$LB" | python3 -c "import sys,json; b=json.loads(sys.stdin.read()); print(1 if b.get('password')=='***' else 0)")"
BEFORE=$(psql_q "SELECT COUNT(*) FROM audit_logs WHERE path='/api/v1/users' AND method='GET'")
curl -s "$BASE/users" -H "Authorization: Bearer $SAT" >/dev/null
AFTER=$(psql_q "SELECT COUNT(*) FROM audit_logs WHERE path='/api/v1/users' AND method='GET'")
check "M6 GET audited" "1" "$([ "$AFTER" -gt "$BEFORE" ] && echo 1 || echo 0)"
check "M6 audit list" "0" "$(curl -s "$BASE/audit/logs?page_size=5" -H "Authorization: Bearer $SAT" | json_code)"
HC=$(curl -s -o /tmp/p1.json -w "%{http_code}" "$BASE/audit/logs" -H "Authorization: Bearer $VAT")
check "M6 viewer audit 403" "403" "$HC"

# --- B1-1 角色禁用生效链：禁用 viewer → 成员同 Token 即时 403（Casbin 每请求查 DB） ---
docker exec "$PG" psql -U "$PG_USER" -d "$PG_DB" -c "UPDATE roles SET status=0 WHERE code='viewer';" >/dev/null
HC=$(curl -s -o /tmp/p1.json -w "%{http_code}" "$BASE/users" -H "Authorization: Bearer $VAT")
check "B1-1 disabled http" "403" "$HC"
check "B1-1 disabled code" "70003" "$(cat /tmp/p1.json | json_code)"
# 重新启用 → 同一 Token 立即恢复访问（casbin_rule 未清除，策略保留）
docker exec "$PG" psql -U "$PG_USER" -d "$PG_DB" -c "UPDATE roles SET status=1 WHERE code='viewer';" >/dev/null
HC=$(curl -s -o /tmp/p1.json -w "%{http_code}" "$BASE/users" -H "Authorization: Bearer $VAT")
check "B1-1 re-enable http" "200" "$HC"

# --- stub negative (org create) ---
ORGCODE="org_$RANDOM"
HC=$(curl -s -o /tmp/p1.json -w "%{http_code}" -X POST "$BASE/orgs" -H "Authorization: Bearer $SAT" -H 'Content-Type: application/json' -d "{\"code\":\"$ORGCODE\",\"name\":\"测试组织\",\"parent_id\":\"1\",\"org_type\":2}")
check "org create" "200" "$HC"

# --- #17 redis down (last: restores redis via trap) ---
AT_FOR_17=$SAT
REDIS_C="${REDIS_CONTAINER:-zhuzhao-redis}"
docker stop "$REDIS_C" >/dev/null 2>&1 || true
sleep 1
HC=$(curl -s -o /tmp/p1.json -w "%{http_code}" "$BASE/users" -H "Authorization: Bearer $AT_FOR_17")
CODE=$(cat /tmp/p1.json | json_code)
check "#17 http" "503" "$HC"
check "#17 code" "10008" "$CODE"

echo ""
echo "=== SUMMARY pass=$pass fail=$fail skip=$skip ==="
[ "$fail" -eq 0 ]
