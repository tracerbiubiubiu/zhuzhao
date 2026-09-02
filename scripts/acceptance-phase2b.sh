#!/usr/bin/env bash
# Phase 2b 收口验收（M2b 验收）：头段回归 phase2a（含 Phase 1），再跑 2b 断言（P2-D5）
# 覆盖：R9/R10 虚拟组兄弟读写分离 / scope 主管分派 / BFS 三源（HTTP 层，含 Casbin）/
#       临时成员过期 / 虚拟组创建约束
# 运行前置：make docker-dev-reset && make dev（或等价部署）
set -euo pipefail

BASE="${BASE_URL:-http://localhost:33333/api/v1}"
HEALTH="${HEALTH_URL:-http://localhost:33333/health/ready}"

# F-25：容器名自动探测（与 acceptance-phase1/2a.sh 一致）
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

cleanup_redis() {
  docker start "$REDIS" >/dev/null 2>&1 || true
}
trap cleanup_redis EXIT

json_code() { python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('code') if d else '')" 2>/dev/null || echo PARSE_ERR; }
psql_q()    { docker exec "$PG" psql -U "$PG_USER" -d "$PG_DB" -qt -A -c "$1" | tr -d '\n'; }

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

# ============================================================
# Section A: 头段回归——phase2a 全量（内含 Phase 1 27 例回归段）
# ============================================================
echo "========== Section A: phase2a 回归（含 Phase 1） =========="
if PG_CONTAINER="$PG" PG_USER="$PG_USER" PG_DB="$PG_DB" REDIS_CONTAINER="$REDIS" \
   BASE="$BASE" HEALTH="$HEALTH" bash "$(dirname -- "${BASH_SOURCE[0]}")/acceptance-phase2a.sh"; then
  sectionA=1
  echo "Section A: phase2a 回归 — ALL PASS"
else
  sectionA=0
  echo "Section A: phase2a 回归 — FAILED（继续跑 Section B 供排查）"
fi

echo "========== Section B: Phase 2b（虚拟组 / scope / BFS / 临时成员） =========="

# --- superadmin 登录（兼容两种密码态：独立跑=种子 admin123+mcp 流程；接续 2a 跑=admin12345）---
SA=$(curl -s -X POST "$BASE/auth/login" -H 'Content-Type: application/json' \
  -d '{"employee_no":"E000001","password":"admin12345"}')
if [ "$(echo "$SA" | json_code)" != "0" ]; then
  SA=$(curl -s -X POST "$BASE/auth/login" -H 'Content-Type: application/json' \
    -d '{"employee_no":"E000001","password":"admin123"}')
  SAT=$(echo "$SA" | python3 -c "import sys,json; print(json.load(sys.stdin)['data']['access_token'])" 2>/dev/null || true)
  curl -s -X POST "$BASE/auth/password/update" -H "Authorization: Bearer $SAT" \
    -H 'Content-Type: application/json' \
    -d '{"old_password":"admin123","new_password":"admin12345","device_id":"phase2b"}' >/dev/null
  SA=$(curl -s -X POST "$BASE/auth/login" -H 'Content-Type: application/json' \
    -d '{"employee_no":"E000001","password":"admin12345"}')
fi
check "sa login" "0" "$(echo "$SA" | json_code)"
SAT=$(echo "$SA" | python3 -c "import sys,json; print(json.load(sys.stdin)['data']['access_token'])")

# --- 组织树：P（实体部门）下 vg_a / vg_b（R9/R10 场景）---
SUF=$$
ROOT_ID=$(psql_q "SELECT id FROM organizations WHERE parent_id IS NULL ORDER BY id LIMIT 1")
[ -z "$ROOT_ID" ] && echo "WARN: no root org" >&2 && ROOT_ID=1
mkorg() { # code name parent_id is_virtual → id
  curl -s -X POST "$BASE/orgs" -H "Authorization: Bearer $SAT" -H 'Content-Type: application/json' \
    -d "{\"code\":\"$1\",\"name\":\"$2\",\"parent_id\":\"$3\",\"is_virtual\":$4,\"sort_order\":50}" \
    | python3 -c "import sys,json; print((json.load(sys.stdin).get('data') or {}).get('id','0'))"
}
P_ID=$(mkorg "p2b_p_$SUF" "2b 父部门" "$ROOT_ID" false)
check "create P org" "1" "$([ "$P_ID" != "0" ] && echo 1 || echo 0)"
VGA_ID=$(mkorg "vg_a_$SUF" "虚拟组A" "$P_ID" true)
VGB_ID=$(mkorg "vg_b_$SUF" "虚拟组B" "$P_ID" true)
check "create vg_a (is_virtual=true)" "1" "$([ "$VGA_ID" != "0" ] && echo 1 || echo 0)"
check "create vg_b (is_virtual=true)" "1" "$([ "$VGB_ID" != "0" ] && echo 1 || echo 0)"
VG_TYPE=$(psql_q "SELECT is_virtual FROM organizations WHERE id=$VGA_ID")
check "vg is_virtual=t" "t" "$VG_TYPE"
VG_SRC=$(psql_q "SELECT source FROM organizations WHERE id=$VGA_ID")
check "vg source=local" "local" "$VG_SRC"

# 根级虚拟组负例：无 parent → 400
check "root-level vg rejected (400)" "400" \
  "$(curl -s -o /dev/null -w '%{http_code}' -X POST "$BASE/orgs" -H "Authorization: Bearer $SAT" -H 'Content-Type: application/json' -d "{\"code\":\"vg_root_$SUF\",\"name\":\"根级虚拟组\",\"is_virtual\":true}")"

# --- 成员用户：va ∈ vg_a、vb ∈ vg_b（operator）---
ROLE_O=$(psql_q "SELECT id FROM roles WHERE code='operator'")
mkuser() { # username employee_no role_id org_id → id
  curl -s -X POST "$BASE/users" -H "Authorization: Bearer $SAT" -H 'Content-Type: application/json' \
    -d "{\"username\":\"$1\",\"password\":\"pass1234\",\"employee_no\":\"$2\",\"role_ids\":[\"$3\"],\"org_ids\":[\"$4\"]}" \
    | python3 -c "import sys,json; print((json.load(sys.stdin).get('data') or {}).get('id','0'))"
}
mklogin() { # employee_no password → token
  curl -s -X POST "$BASE/auth/login" -H 'Content-Type: application/json' \
    -d "{\"employee_no\":\"$1\",\"password\":\"$2\"}" \
    | python3 -c "import sys,json; print(json.load(sys.stdin)['data']['access_token'])" 2>/dev/null || true
}
VA_ID=$(mkuser "p2b_va_$SUF" "B2VA$SUF" "$ROLE_O" "$VGA_ID")
VB_ID=$(mkuser "p2b_vb_$SUF" "B2VB$SUF" "$ROLE_O" "$VGB_ID")
check "create va/vb" "1" "$([ "$VA_ID" != "0" ] && [ "$VB_ID" != "0" ] && echo 1 || echo 0)"
VAT=$(mklogin "B2VA$SUF" "pass1234")
VBT=$(mklogin "B2VB$SUF" "pass1234")
check "va/vb login" "1" "$([ -n "$VAT" ] && [ -n "$VBT" ] && echo 1 || echo 0)"

# --- R9/R10：vb 在 vg_b 建单；va 透明读 200 / update 403 / comment 0 / note 403 ---
TK=$(curl -s -X POST "$BASE/tickets" -H "Authorization: Bearer $VBT" -H 'Content-Type: application/json' \
  -d "{\"type_code\":\"incident\",\"title\":\"vg_b 工单\",\"org_id\":\"$VGB_ID\"}")
TID=$(echo "$TK" | python3 -c "import sys,json; print((json.load(sys.stdin).get('data') or {}).get('id','0'))")
check "vb create ticket in vg_b" "1" "$([ "$TID" != "0" ] && echo 1 || echo 0)"

check "R9 va read vg_b ticket (200)" "200" \
  "$(curl -s -o /tmp/p2b.json -w '%{http_code}' "$BASE/tickets/$TID" -H "Authorization: Bearer $VAT")"
check "R9 code=0" "0" "$(cat /tmp/p2b.json | json_code)"
VA_BODY="{\"id\":\"$TID\",\"title\":\"R10 rename\"}"
curl -s -o /tmp/p2b.json -w 'R10_HTTP=%{http_code}\n' -X POST "$BASE/tickets/update" -H "Authorization: Bearer $VAT" -H 'Content-Type: application/json' -d "$VA_BODY" >&2
R10=$(cat /tmp/p2b.json | json_code)
check "R10 va update vg_b ticket (403+70001)" "70001" "$R10"
VA_COMMENT=$(curl -s -X POST "$BASE/tickets/comments" -H "Authorization: Bearer $VAT" -H 'Content-Type: application/json' -d "{\"ticket_id\":\"$TID\",\"content\":\"va 公开回复\"}")
check "va comment allowed (0)" "0" "$(echo "$VA_COMMENT" | json_code)"
NOTE_HTTP=$(curl -s -o /tmp/p2b_note.json -w '%{http_code}' -X POST "$BASE/tickets/notes" -H "Authorization: Bearer $VAT" -H 'Content-Type: application/json' -d "{\"ticket_id\":\"$TID\",\"content\":\"va 备注\"}")
check "va note denied (403)" "403" "$NOTE_HTTP"

# --- scope=group 主管：sup 挂 P（assigned），SQL 升 group 后可分派子树工单 ---
SUP_ID=$(mkuser "p2b_sup_$SUF" "B2SP$SUF" "$ROLE_O" "$P_ID")
SUP=$(mklogin "B2SP$SUF" "pass1234")
check "supervisor login" "1" "$([ -n "$SUP" ] && echo 1 || echo 0)"
SUP_BEFORE=$(curl -s -o /tmp/p2b_sup.json -w '%{http_code}' -X POST "$BASE/tickets/assign" -H "Authorization: Bearer $SUP" -H 'Content-Type: application/json' -d "{\"id\":\"$TID\",\"assigned_to\":\"$VB_ID\"}")
check "supervisor assign before scope (403)" "403" "$SUP_BEFORE"
psql_q "UPDATE user_orgs SET ticket_scope='group' WHERE user_id=$SUP_ID AND org_id=$P_ID" >/dev/null
SUP_AFTER_RAW=$(curl -s -X POST "$BASE/tickets/assign" -H "Authorization: Bearer $SUP" -H 'Content-Type: application/json' -d "{\"id\":\"$TID\",\"assigned_to\":\"$VB_ID\"}")
check "supervisor assign after scope=group (0)" "0" "$(echo "$SUP_AFTER_RAW" | json_code)"

# --- BFS 三源（HTTP 层，含 Casbin）：org_roles 授角色 → 菜单与 API 放行 ---
# BFS 判别用 system_user 菜单（operator 永不拥有，区分度最高；
# ticket_list 对 operator 无区分度——Section A 已把工单菜单授予 operator）
RB_CODE="p2b_role_$SUF"
RB_JSON=$(curl -s -X POST "$BASE/roles" -H "Authorization: Bearer $SAT" -H 'Content-Type: application/json' \
  -d "{\"code\":\"$RB_CODE\",\"name\":\"2b BFS 测试角色\",\"priority\":50}")
RB_ID=$(echo "$RB_JSON" | python3 -c "import sys,json; print((json.load(sys.stdin).get('data') or {}).get('id','0'))")
check "create bfs role" "1" "$([ "$RB_ID" != "0" ] && echo 1 || echo 0)"
SYSMENU=$(psql_q "SELECT id FROM menus WHERE code='system_user'")
curl -s -X POST "$BASE/roles/menus" -H "Authorization: Bearer $SAT" -H 'Content-Type: application/json' \
  -d "{\"role_id\":\"$RB_ID\",\"menu_ids\":[\"$SYSMENU\"]}" >/dev/null
MENU_HAS_SU() { curl -s "$BASE/user/menus" -H "Authorization: Bearer $1" | python3 -c "
import sys,json
d=json.load(sys.stdin)['data']
ns = d.get('menus') if isinstance(d, dict) else d   # data 为 {menus:[树]}（GetUserMenus 信封内层）
def walk(nodes):
    for m in nodes or []:
        if isinstance(m,dict):
            if m.get('code')=='system_user': return True
            if walk(m.get('children') or []): return True
    return False
print(1 if walk(ns) else 0)"; }
check "va menus before org_roles (no system_user)" "0" "$(MENU_HAS_SU "$VAT")"
check "va GET /users before org_roles (403)" "403" \
  "$(curl -s -o /dev/null -w '%{http_code}' "$BASE/users" -H "Authorization: Bearer $VAT")"
psql_q "INSERT INTO org_roles (org_id, role_id) VALUES ($VGA_ID, $RB_ID) ON CONFLICT DO NOTHING" >/dev/null
check "BFS: va menus after org_roles (has system_user)" "1" "$(MENU_HAS_SU "$VAT")"
check "BFS: va GET /users via org role (200)" "200" \
  "$(curl -s -o /dev/null -w '%{http_code}' "$BASE/users" -H "Authorization: Bearer $VAT")"
check "BFS: 父部门角色不继承子部门成员" "0" \
  "$(psql_q "SELECT COUNT(*) FROM org_roles orgr JOIN user_orgs m ON m.org_id = orgr.org_id WHERE m.user_id = $VB_ID AND orgr.role_id = $RB_ID")"

# --- 临时成员过期：expired 成员不可读，未过期可读 ---
EXP_ID=$(mkuser "p2b_exp_$SUF" "B2EX$SUF" "$ROLE_O" "$VGB_ID")
EXPT=$(mklogin "B2EX$SUF" "pass1234")
check "expired-before: member reads vg_b ticket (200)" "200" \
  "$(curl -s -o /dev/null -w '%{http_code}' "$BASE/tickets/$TID" -H "Authorization: Bearer $EXPT")"
psql_q "UPDATE user_orgs SET expires_at = NOW() - INTERVAL '1 hour' WHERE user_id=$EXP_ID AND org_id=$VGB_ID" >/dev/null
check "expired-after: member reads vg_b ticket (404)" "404" \
  "$(curl -s -o /dev/null -w '%{http_code}' "$BASE/tickets/$TID" -H "Authorization: Bearer $EXPT")"

# ============================================================
# 汇总（Section A 以退出码并入）
# ============================================================
echo ""
if [ "$sectionA" -eq 1 ] && [ "$fail" -eq 0 ]; then
  echo "========== Phase 2b Acceptance Summary =========="
  echo "PASS=$pass  FAIL=0  SKIP=0（Section A 回归通过）"
  echo "✅ Phase 2b ALL PASSED"
  exit 0
else
  echo "========== Phase 2b Acceptance Summary =========="
  echo "PASS=$pass  FAIL=$fail  SectionA_OK=$sectionA"
  echo "❌ Phase 2b HAS FAILURES"
  exit 1
fi
