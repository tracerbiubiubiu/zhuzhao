#!/usr/bin/env bash
# Phase 2a 全量验收（docs/phase2/README.md §1.1 + 02-authz-resource.md §4 R3-R8 + 09-ticket.md §7 T1-T7）
# P2-D5：头段运行 Phase 1 27 用例回归；再跑 2a 新增用例段
# 运行前置：make docker-dev-reset && make dev（或等价部署），确保服务监听 BASE_URL
set -euo pipefail

BASE="${BASE_URL:-http://localhost:33333/api/v1}"
HEALTH="${HEALTH_URL:-http://localhost:33333/health/ready}"

# F-25：容器名自动探测（与 acceptance-phase1.sh 一致）——dev compose 为
# zhuzhao-dev-*，完整部署为 zhuzhao-*；显式导出 PG_CONTAINER/REDIS_CONTAINER 时优先
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
# 清除登录限流锁（验收链对 E000001 反复登录，15min/5 次会触发 20006 锁定）
docker exec "$REDIS" redis-cli -a zhuzhao_dev --no-auth-warning del "lock:login:E000001" >/dev/null 2>&1 || true
SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"

pass=0
fail=0
skip=0

cleanup_redis() {
  docker start "$REDIS" >/dev/null 2>&1 || true
}
trap cleanup_redis EXIT

# --- 辅助函数（与 acceptance-phase1.sh 保持一致）---
json_code()   { python3 -c "import sys,json; print(json.load(sys.stdin).get('code',''))"; }
psql_q()      { docker exec "$PG" psql -U "$PG_USER" -d "$PG_DB" -qt -A -c "$1" | tr -d '\n'; }

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

# ============================================================
# Section A: Phase 1 回归段（P2-D5：头段先跑上一段 27 用例）
# ============================================================
echo ""
echo "========== Section A: Phase 1 Regression (27 cases) =========="
set +e
# shellcheck disable=SC1091
BASE_URL="${BASE_URL:-}" HEALTH_URL="${HEALTH_URL:-}" \
PG_CONTAINER="$PG" PG_USER="$PG_USER" PG_DB="$PG_DB" REDIS_CONTAINER="$REDIS" \
bash "$SCRIPT_DIR/acceptance-phase1.sh"
P1_EXIT=$?
set -e
# 不读取 phase1 脚本内部计数器（无法共享 set -e 上下文），仅用退出码做断言：
#   exit 0 = 全部通过（phase1 脚本 fail 非 0 会 exit 1，因为末尾有 [ $fail -eq 0 ]）
if [ "$P1_EXIT" -eq 0 ]; then
  echo "Section A: Phase 1 27 cases — ALL PASS"
else
  echo "Section A: Phase 1 27 cases — FAILED (exit=$P1_EXIT)"
  fail=$((fail + 1))
fi

# ============================================================
# Section B: 2a 新增用例段
#   T1–T7 + R3–R8 大量重合，按实际执行顺序合并，两边锚点同回标
#   README §1.1 4 条验收穿插其中
# ============================================================
echo ""
echo "========== Section B: Phase 2a Acceptance ========="
echo "BASE=$BASE"

# --- 健康 + 种子检查（README §1.1 前置）-------------------------------
R=$(curl -s "$HEALTH" | python3 -c "import sys,json; print(json.load(sys.stdin).get('status',''))")
check "ready" "ok" "$R"

# 2a 菜单种子：11 个菜单 code（2 level 1/2 + 9 按钮）应存在
TM=$(psql_q "SELECT COUNT(*) FROM menus WHERE code IN (
  'ticket_manage','ticket_list',
  'ticket_list_btn','ticket_create_btn','ticket_read_btn',
  'ticket_update_btn','ticket_close_btn','ticket_assign_btn',
  'ticket_delete_btn','ticket_comment_btn','ticket_note_btn')")
check "ticket menus(11)" "11" "$TM"

# 2a ticket_types 种子：incident + request 两类
TT=$(psql_q "SELECT COUNT(*) FROM ticket_types WHERE code IN ('incident','request')")
check "ticket types(2)" "2" "$TT"

# menu_apis 覆盖工单 16 条路由（09-ticket §3 API 表）
MA=$(psql_q "SELECT COUNT(*) FROM menu_apis ma
  INNER JOIN menus m ON m.id = ma.menu_id
  WHERE m.code = 'ticket_list'")
check "ticket menu_apis(16)" "16" "$MA"

# 2a: admin/superadmin 获得 11 个 ticket 菜单的 role_menus 绑定
RM=$(psql_q "SELECT COUNT(DISTINCT rm.menu_id) FROM role_menus rm
  INNER JOIN roles r ON r.id = rm.role_id
  INNER JOIN menus m ON m.id = rm.menu_id
  WHERE r.code IN ('superadmin','admin')
    AND m.code IN (
      'ticket_manage','ticket_list',
      'ticket_list_btn','ticket_create_btn','ticket_read_btn',
      'ticket_update_btn','ticket_close_btn','ticket_assign_btn',
      'ticket_delete_btn','ticket_comment_btn','ticket_note_btn')")
check "role_menus ticket admin binds" "1" "$([ "$RM" -eq 11 ] && echo 1 || echo 0)"  # 1 目录+1 页面+9 按钮

# --- Superadmin 登录（复用 Phase 1 改密流程）-------------------------
SA=$(curl -s -X POST "$BASE/auth/login" -H 'Content-Type: application/json' \
  -d '{"employee_no":"E000001","password":"admin123"}')
if [ "$(echo "$SA" | json_code)" = "0" ] && \
   echo "$SA" | python3 -c "import sys,json,sys
try:
  t=json.load(sys.stdin)['data']['access_token']
except Exception:
  sys.exit(1)
" >/dev/null 2>&1; then
  SAT=$(echo "$SA" | python3 -c "import sys,json; print(json.load(sys.stdin)['data']['access_token'])")
  # 改密以解锁 mcp（与 Phase 1 脚本同样的一次性流程）
  MCPC=$(curl -s -X POST "$BASE/auth/password/update" \
    -H "Authorization: Bearer $SAT" -H 'Content-Type: application/json' \
    -d '{"old_password":"admin123","new_password":"admin12345","device_id":"phase2a"}' | json_code)
  [ "$MCPC" = "0" ] || true
fi
SA=$(curl -s -X POST "$BASE/auth/login" -H 'Content-Type: application/json' \
  -d '{"employee_no":"E000001","password":"admin12345"}')
check "sa login" "0" "$(echo "$SA" | json_code)"
SAT=$(echo "$SA" | python3 -c "import sys,json; print(json.load(sys.stdin)['data']['access_token'])")

# --- 预创建：组织 + 用户 A(创建者) + 用户 B(处理人) + 用户 V(viewer, 无 ticket 权限)
ROOT_ID=$(psql_q "SELECT id FROM organizations WHERE parent_id IS NULL ORDER BY id LIMIT 1")
[ -z "$ROOT_ID" ] && echo "WARN: no root org found, cannot create sub-org" >&2 && ROOT_ID=1

# 建子组织 "tech"（挂 root 下，先查是否已存在避免冲突）
TECH_CODE="p2a_tech_$$"
TECH_JSON=$(curl -s -X POST "$BASE/orgs" \
  -H "Authorization: Bearer $SAT" -H 'Content-Type: application/json' \
  -d "{\"code\":\"$TECH_CODE\",\"name\":\"Phase2a Tech\",\"parent_id\":\"$ROOT_ID\",\"is_virtual\":false,\"sort_order\":99}")
TECH_ID=$(echo "$TECH_JSON" | python3 -c "import sys,json; print((json.load(sys.stdin).get('data') or {}).get('id','0'))")
check "create tech org" "1" "$([ "$TECH_ID" != "0" ] && echo 1 || echo 0)"

# 2b 语义适配（策略 B 透明读，09-ticket §5.2）：A/B 同属 tech 后互为透明读，
# T2/T3 的隔离断言需 B 工单位于 A 锚点之外的独立子树，故建 tech2 承载 B 的工单
TECH2_CODE="p2a_tech2_$$"
TECH2_JSON=$(curl -s -X POST "$BASE/orgs" \
  -H "Authorization: Bearer $SAT" -H 'Content-Type: application/json' \
  -d "{\"code\":\"$TECH2_CODE\",\"name\":\"Phase2a Tech2\",\"parent_id\":\"$ROOT_ID\",\"is_virtual\":false,\"sort_order\":98}")
TECH2_ID=$(echo "$TECH2_JSON" | python3 -c "import sys,json; print((json.load(sys.stdin).get('data') or {}).get('id','0'))")
check "create tech2 org" "1" "$([ "$TECH2_ID" != "0" ] && echo 1 || echo 0)"

# 创建用户 A（创建者）+ B（处理人）+ viewer 用户
make_user() {
  local suf="$1" role_id="$2"
  local payload="{\"username\":\"p2a_$suf\",\"password\":\"pass1234\",\"employee_no\":\"E2A$suf\",\"role_ids\":[\"$role_id\"],\"org_ids\":[\"$TECH_ID\"]}"
  local resp
  resp=$(curl -s -X POST "$BASE/users" \
    -H "Authorization: Bearer $SAT" -H 'Content-Type: application/json' \
    -d "$payload")
  echo "$resp" | python3 -c "import sys,json; print((json.load(sys.stdin).get('data') or {}).get('id','0'))"
}
# role 4 = viewer (seed), role 3 = operator, role 2 = admin
ROLE_V=$(psql_q "SELECT id FROM roles WHERE code='viewer'")
ROLE_O=$(psql_q "SELECT id FROM roles WHERE code='operator'")
# 模拟管理员授权流程（对齐 Phase 1 #27 模式）：给 operator 绑定工单页面菜单 →
# 页面菜单含全部 16 条 ticket menu_apis，L1 放行后由 L2/L3 管辖（R6/T7 的 403 用例才有意义）
TICKET_PAGE_MENU=$(psql_q "SELECT id FROM menus WHERE code='ticket_list'")
curl -s -X POST "$BASE/roles/menus" -H "Authorization: Bearer $SAT" -H 'Content-Type: application/json' \
  -d "{\"role_id\":\"$ROLE_O\",\"menu_ids\":[\"$TICKET_PAGE_MENU\"]}" >/dev/null
AID=$(make_user "a_$$" "$ROLE_O")
BID=$(make_user "b_$$" "$ROLE_O")
VID=$(make_user "v_$$" "$ROLE_V")
check "create user A"       "1" "$([ "$AID" != "0" ] && echo 1 || echo 0)"
check "create user B"       "1" "$([ "$BID" != "0" ] && echo 1 || echo 0)"
check "create viewer user V" "1" "$([ "$VID" != "0" ] && echo 1 || echo 0)"

login_user() {
  local suf="$1"
  local resp
  resp=$(curl -s -X POST "$BASE/auth/login" -H 'Content-Type: application/json' \
    -d "{\"employee_no\":\"E2A$suf\",\"password\":\"pass1234\"}")
  echo "$resp" | python3 -c "import sys,json; print((json.load(sys.stdin).get('data') or {}).get('access_token',''))"
}
AAT=$(login_user "a_$$")
BAT=$(login_user "b_$$")
VAT=$(login_user "v_$$")
check "user A login ok" "1" "$([ -n "$AAT" ] && echo 1 || echo 0)"
check "user B login ok" "1" "$([ -n "$BAT" ] && echo 1 || echo 0)"
check "user V login ok" "1" "$([ -n "$VAT" ] && echo 1 || echo 0)"

# ========================================================================
# T1 / README §1.1-2: 创建工单 → 200 且 org_path 快照正确（同根组织 path）
# ========================================================================
T1=$(curl -s -X POST "$BASE/tickets" \
  -H "Authorization: Bearer $AAT" -H 'Content-Type: application/json' \
  -d "{\"type_code\":\"incident\",\"title\":\"打印机故障\",\"description\":\"3F 打印机卡纸\",\"priority\":2,\"org_id\":\"$TECH_ID\"}")
check "T1 create ticket code=0" "0" "$(echo "$T1" | json_code)"
TID_A=$(echo "$T1" | python3 -c "import sys,json; print((json.load(sys.stdin).get('data') or {}).get('id','0'))")
check "T1 created id>0" "1" "$([ "$TID_A" != "0" ] && echo 1 || echo 0)"

# DB 断言：org_path 与组织 path 前缀一致（从 organizations 快照）
EXPECTED_PATH=$(psql_q "SELECT path::text FROM organizations WHERE id=$TECH_ID")
ACTUAL_PATH=$(psql_q "SELECT org_path::text FROM tickets WHERE id=$TID_A")
check "T1 org_path snapshot" "$EXPECTED_PATH" "$ACTUAL_PATH"

# 额外：创建时写 ticket_events action=created
EV=$(psql_q "SELECT COUNT(*) FROM ticket_events WHERE ticket_id=$TID_A AND action='created'")
check "T1 ticket_event created" "1" "$EV"

# ========================================================================
# 额外 1：模板预填（2a 前移）— 通过 incident 类型创建模板
# ========================================================================
TPL=$(curl -s -X POST "$BASE/ticket-templates" \
  -H "Authorization: Bearer $SAT" -H 'Content-Type: application/json' 2>/dev/null || true)
# 注意：2a 只实现了模板列表/详情 GET（无 POST 模板管理 API，见 09-ticket §3 API 表——仅 GET）
# 所以这里不做 POST，改用 DB 直插种子模板来验证预填逻辑
TPL_CODE="p2a_inc_hi_$$"
psql_q "INSERT INTO ticket_templates (code,name,type_code,default_priority,default_fields,org_id,org_path,created_by) VALUES ('$TPL_CODE','Hi-Pri Incident','incident',1,'{\"description\":\"SLA 4h 模板\"}',$TECH_ID,'$EXPECTED_PATH',$AID)" >/dev/null 2>&1
T2_TPL=$(curl -s -X POST "$BASE/tickets" \
  -H "Authorization: Bearer $AAT" -H 'Content-Type: application/json' \
  -d "{\"type_code\":\"incident\",\"title\":\"模板工单\",\"template_code\":\"$TPL_CODE\",\"org_id\":\"$TECH_ID\"}")
check "template prefill code=0" "0" "$(echo "$T2_TPL" | json_code)"
TPL_PRI=$(echo "$T2_TPL" | python3 -c "import sys,json; print((json.load(sys.stdin).get('data') or {}).get('priority','0'))")
check "template default_priority override" "1" "$TPL_PRI"
TPL_DESC=$(echo "$T2_TPL" | python3 -c "import sys,json; print((json.load(sys.stdin).get('data') or {}).get('description',''))")
check "template default_fields description prefill" "SLA 4h 模板" "$TPL_DESC"

# ========================================================================
# T2 / R3 / README §1.1-1: assigned scope 列表仅返回 created_by 或 assigned_to
# （2b 起策略 B 同子树透明读，本组隔离断言依赖 B 工单位于 tech2 独立子树）
# ========================================================================
# 先让 B 也创建一张工单（确认 A 的列表不含 B 创建的工单）
T_B=$(curl -s -X POST "$BASE/tickets" \
  -H "Authorization: Bearer $BAT" -H 'Content-Type: application/json' \
  -d "{\"type_code\":\"request\",\"title\":\"B 申请电脑\",\"priority\":3,\"org_id\":\"$TECH2_ID\"}")  # 2b：独立子树，保持 A 不可见
TID_B=$(echo "$T_B" | python3 -c "import sys,json; print((json.load(sys.stdin).get('data') or {}).get('id','0'))")
check "user B create ticket" "1" "$([ "$TID_B" != "0" ] && echo 1 || echo 0)"

A_LIST=$(curl -s "$BASE/tickets" -H "Authorization: Bearer $AAT")
check "T2 A list code=0" "0" "$(echo "$A_LIST" | json_code)"
A_IDS=$(echo "$A_LIST" | python3 -c "import sys,json
d=json.load(sys.stdin).get('data') or {}
ids=[str(t['id']) for t in d.get('list',[])]
print(','.join(ids))")
check "T2 A list contains TID_A (自己创建)" "1" "$(echo ",$A_IDS," | grep -q ",$TID_A," && echo 1 || echo 0)"
check "T2 A list excludes TID_B (他人创建, 自己未被分派)" "1" "$(echo ",$A_IDS," | grep -qv ",$TID_B," && echo 1 || echo 0)"

# ========================================================================
# T3 / R4 / README §1.1-3: 不可见工单详情 → 404（90001，而非 403）
# ========================================================================
R4=$(curl -s -o /tmp/p2a.json -w "%{http_code}" \
  "$BASE/tickets/$TID_B" -H "Authorization: Bearer $AAT")
R4C=$(cat /tmp/p2a.json | json_code)
check "T3 A读B 工单 http=404" "404" "$R4"
check "T3 A读B 工单 code=90001" "90001" "$R4C"

# ========================================================================
# T4 / R5 / README §1.1-2: A 更新自己的工单 → 200
# ========================================================================
T4=$(curl -s -X POST "$BASE/tickets/update" \
  -H "Authorization: Bearer $AAT" -H 'Content-Type: application/json' \
  -d "{\"id\":\"$TID_A\",\"title\":\"打印机故障（更新：缺纸张）\"}")
check "T4 A 更新自己工单 code=0" "0" "$(echo "$T4" | json_code)"
T4_T=$(echo "$T4" | python3 -c "import sys,json; print((json.load(sys.stdin).get('data') or {}).get('title',''))")
check "T4 title 已变更" "1" "$([ "$T4_T" = "打印机故障（更新：缺纸张）" ] && echo 1 || echo 0)"

# ========================================================================
# R6：A 尝试更新 B 的工单（路由放行，资源级 canOperate=update 拒绝 → 403 + 70001）
# 备注：2a assigned scope 下，B 创建未分派给 A 时，A 对 TID_B 是不可见，
#       Get 先返回 404。为了命中「可见但非属主 → 403」分支，先由 admin 分派 TID_B 给 A，
#       此时 A 通过 assigned 命中 canRead=true，但 canOperate(update) 在当前实现中
#       2a 口径为「创建人 or 处理人可 update」（09 §5.1 RK-11 2b 才收窄）。
#       所以为拿到 403，使用 分派/删除 两种 canOperate 永远返回 false 的动作（属主也不放行）。
# ========================================================================
# 由 admin 将 B 的工单分派给 A（admin bypass）
AS=$(curl -s -X POST "$BASE/tickets/assign" \
  -H "Authorization: Bearer $SAT" -H 'Content-Type: application/json' \
  -d "{\"id\":\"$TID_B\",\"assigned_to\":\"$AID\"}")
check "admin assigns B's ticket to A" "0" "$(echo "$AS" | json_code)"

# A 对 B 的工单（已被分派=可见）做 DELETE：canOperate(delete)=false（即使可见也非属主/处理人能做）
R6_DEL=$(curl -s -o /tmp/p2a.json -w "%{http_code}" -X POST "$BASE/tickets/delete" \
  -H "Authorization: Bearer $AAT" -H 'Content-Type: application/json' \
  -d "{\"id\":\"$TID_B\"}")
R6C=$(cat /tmp/p2a.json | json_code)
check "R6 A 删除分派给自己但非创建的工单 http=403" "403" "$R6_DEL"
check "R6 code=70001 (ErrNoPermission)" "70001" "$R6C"

# 同样测试 assign 动作（A 自己不能分派他人）
R6_ASS=$(curl -s -o /tmp/p2a.json -w "%{http_code}" -X POST "$BASE/tickets/assign" \
  -H "Authorization: Bearer $AAT" -H 'Content-Type: application/json' \
  -d "{\"id\":\"$TID_A\",\"assigned_to\":\"$BID\"}")
R6A=$(cat /tmp/p2a.json | json_code)
check "R6 A assign A's ticket http=403" "403" "$R6_ASS"
check "R6 assign code=70001" "70001" "$R6A"

# ========================================================================
# T5: admin 分派 + 状态变 assigned（open → assigned）
# ========================================================================
T5=$(curl -s -X POST "$BASE/tickets/assign" \
  -H "Authorization: Bearer $SAT" -H 'Content-Type: application/json' \
  -d "{\"id\":\"$TID_A\",\"assigned_to\":\"$BID\"}")
check "T5 admin assigns A's ticket → B" "0" "$(echo "$T5" | json_code)"
T5_STATUS=$(psql_q "SELECT status FROM tickets WHERE id=$TID_A")
check "T5 status=assigned" "assigned" "$T5_STATUS"
T5_ASSIGNEE=$(psql_q "SELECT assigned_to FROM tickets WHERE id=$TID_A")
check "T5 assigned_to=B" "$BID" "$T5_ASSIGNEE"
# 分派事件写入 ticket_events
T5_EV=$(psql_q "SELECT COUNT(*) FROM ticket_events WHERE ticket_id=$TID_A AND action='assigned' AND to_value='$BID'")
check "T5 event assigned row" "1" "$T5_EV"

# ========================================================================
# T6: 非法 transition → 400 + 90002
# 种子 transitions：assigned 只能到 in_progress 或 open，不能直接 closed
# ========================================================================
CLOSE_BAD=$(curl -s -o /tmp/p2a.json -w "%{http_code}" -X POST "$BASE/tickets/close" \
  -H "Authorization: Bearer $SAT" -H 'Content-Type: application/json' \
  -d "{\"id\":\"$TID_A\"}")
T6C_HTTP=$(cat /tmp/p2a.json | json_code)
check "T6 assigned→closed http=400" "400" "$CLOSE_BAD"
check "T6 assigned→closed code=90002" "90002" "$T6C_HTTP"

# 合法 transition：assigned→in_progress（通过 update status 模拟，这里直接走 close 合法路径：admin 先把状态推到 open 再 close）
ADV_OPEN=$(curl -s -X POST "$BASE/tickets/assign" \
  -H "Authorization: Bearer $SAT" -H 'Content-Type: application/json' \
  -d "{\"id\":\"$TID_A\",\"assigned_to\":null}")
check "admin clear assignee (-> open)" "0" "$(echo "$ADV_OPEN" | json_code)"
LEGAL_CLOSE=$(curl -s -X POST "$BASE/tickets/close" \
  -H "Authorization: Bearer $SAT" -H 'Content-Type: application/json' \
  -d "{\"id\":\"$TID_A\",\"comment\":\"已修复\"}")
check "T6_legit open→closed code=0" "0" "$(echo "$LEGAL_CLOSE" | json_code)"
CLOSED_EV=$(psql_q "SELECT COUNT(*) FROM ticket_events WHERE ticket_id=$TID_A AND action='status_changed' AND to_value='closed'")
check "status_changed event count=1" "1" "$CLOSED_EV"
# 带 comment 关闭：应生成一条评论
CMT=$(psql_q "SELECT COUNT(*) FROM ticket_comments WHERE ticket_id=$TID_A AND is_internal=false")
check "close comment saved" "1" "$CMT"

# ========================================================================
# T7 / R8: 无 ticket:list 路由权限（viewer 角色，未绑定 ticket_list 菜单）→ 403 + 70001
# ========================================================================
T7=$(curl -s -o /tmp/p2a.json -w "%{http_code}" "$BASE/tickets" \
  -H "Authorization: Bearer $VAT")
T7C_HTTP=$(cat /tmp/p2a.json | json_code)
check "T7 viewer GET tickets http=403" "403" "$T7"
check "T7 viewer GET tickets code=70001" "70001" "$T7C_HTTP"
# 同样验证 POST /tickets 也被 Casbin L1 拦
T7P=$(curl -s -o /tmp/p2a.json -w "%{http_code}" -X POST "$BASE/tickets" \
  -H "Authorization: Bearer $VAT" -H 'Content-Type: application/json' \
  -d "{\"type_code\":\"incident\",\"title\":\"viewer 不应能建\",\"priority\":3,\"org_id\":\"$TECH_ID\"}")
T7PC=$(cat /tmp/p2a.json | json_code)
check "T7 viewer POST tickets http=403" "403" "$T7P"
check "T7 viewer POST code=70001" "70001" "$T7PC"

# ========================================================================
# R7: admin 读任意工单（admin bypass L2/L3）
# ========================================================================
R7=$(curl -s "$BASE/tickets/$TID_B" -H "Authorization: Bearer $SAT")
check "R7 admin read B's ticket code=0" "0" "$(echo "$R7" | json_code)"
R7_TITLE=$(echo "$R7" | python3 -c "import sys,json; print((json.load(sys.stdin).get('data') or {}).get('title',''))")
check "R7 admin read title matches" "B 申请电脑" "$R7_TITLE"

# ========================================================================
# 评论 + 备注（T 表未显式编号，README §1.1-2 延伸：可见工单可 comment）
# ========================================================================
# B 对 A 的工单（B 已被分派或 B 创建 B 可读）：B 对 TID_B 发评论
C1=$(curl -s -X POST "$BASE/tickets/comments" \
  -H "Authorization: Bearer $BAT" -H 'Content-Type: application/json' \
  -d "{\"ticket_id\":\"$TID_B\",\"content\":\"请问电脑型号是？\"}")
check "B comment on own ticket code=0" "0" "$(echo "$C1" | json_code)"
C1_ID=$(echo "$C1" | python3 -c "import sys,json; print((json.load(sys.stdin).get('data') or {}).get('id','0'))")
check "comment id>0" "1" "$([ "$C1_ID" != "0" ] && echo 1 || echo 0)"
# comment 查询：B 读自己的工单评论
C_LIST=$(curl -s "$BASE/tickets/$TID_B/comments" -H "Authorization: Bearer $BAT")
check "list comments code=0" "0" "$(echo "$C_LIST" | json_code)"
# 内部备注：A 对 A 创建的工单（A 是创建人 → note 权限）
N1=$(curl -s -X POST "$BASE/tickets/notes" \
  -H "Authorization: Bearer $AAT" -H 'Content-Type: application/json' \
  -d "{\"ticket_id\":\"$TID_B\",\"content\":\"内部：查一下库存是否有\"}")
# A 对 B 的工单（A 被分派过 → 处理人，2a note 口径 = 创建人 or 处理人）→ 允许
check "A note as assignee code=0" "0" "$(echo "$N1" | json_code)"
N1_ID=$(echo "$N1" | python3 -c "import sys,json; print((json.load(sys.stdin).get('data') or {}).get('id','0'))")
N1_INTERNAL=$(psql_q "SELECT is_internal FROM ticket_comments WHERE id=$N1_ID")
check "note is_internal=true" "t" "$N1_INTERNAL"
N_INTERNAL=$(psql_q "SELECT is_internal FROM ticket_comments WHERE id=$C1_ID")
check "comment public" "f" "$N_INTERNAL"  # postgres bool lowercase f/t

# ========================================================================
# 工单关联（§3 前移功能）：建立关联须对 source/target 双端走 update 级 L2/L3 鉴权（严于 PRD 的 target-only，见 09 §2 工单关联节）
# ========================================================================
# 额外创建 A 的第二张工单便于关联
TID_A2_JSON=$(curl -s -X POST "$BASE/tickets" \
  -H "Authorization: Bearer $AAT" -H 'Content-Type: application/json' \
  -d "{\"type_code\":\"incident\",\"title\":\"关联用工单 A2\",\"priority\":3,\"org_id\":\"$TECH_ID\"}")
TID_A2=$(echo "$TID_A2_JSON" | python3 -c "import sys,json; print((json.load(sys.stdin).get('data') or {}).get('id','0'))")

# A 建立 A2 → TID_A 的关联（2b RK-11：update 收窄为仅创建人，A 对 TID_B 已无
# update 权——双端正例改用 A 自己的两张工单；跨可见性负例由集成测试覆盖）
REL=$(curl -s -X POST "$BASE/tickets/relations" \
  -H "Authorization: Bearer $AAT" -H 'Content-Type: application/json' \
  -d "{\"source_ticket_id\":\"$TID_A2\",\"target_ticket_id\":\"$TID_A\",\"relation_type\":\"related\"}")
check "create relation code=0" "0" "$(echo "$REL" | json_code)"

# ========================================================================
# P2-D1：组织 move 级联 tickets.org_path
# ========================================================================
# 新建子子组织 "tech/fe"，创建一张工单，再把 fe 移到另一个父（挂 root），
# 验证该工单 org_path 同步改写
FE_CODE="p2a_fe_$$"
FE_JSON=$(curl -s -X POST "$BASE/orgs" \
  -H "Authorization: Bearer $SAT" -H 'Content-Type: application/json' \
  -d "{\"code\":\"$FE_CODE\",\"name\":\"Phase2a Frontend\",\"parent_id\":\"$TECH_ID\",\"is_virtual\":false,\"sort_order\":1}")
FE_ID=$(echo "$FE_JSON" | python3 -c "import sys,json; print((json.load(sys.stdin).get('data') or {}).get('id','0'))")
check "create org fe (child of tech)" "1" "$([ "$FE_ID" != "0" ] && echo 1 || echo 0)"
FE_OLD_PATH=$(psql_q "SELECT path::text FROM organizations WHERE id=$FE_ID")

# 创建一张工单挂 fe 下
FE_TICKET=$(curl -s -X POST "$BASE/tickets" \
  -H "Authorization: Bearer $AAT" -H 'Content-Type: application/json' \
  -d "{\"type_code\":\"request\",\"title\":\"FE 工单\",\"priority\":4,\"org_id\":\"$FE_ID\"}")
FE_TID=$(echo "$FE_TICKET" | python3 -c "import sys,json; print((json.load(sys.stdin).get('data') or {}).get('id','0'))")
check "create FE ticket id>0" "1" "$([ "$FE_TID" != "0" ] && echo 1 || echo 0)"
FE_TICKET_PATH_OLD=$(psql_q "SELECT org_path::text FROM tickets WHERE id=$FE_TID")
check "P2-D1 ticket org_path = fe old path" "$FE_OLD_PATH" "$FE_TICKET_PATH_OLD"

# Move: 把 fe 从 tech 下挪到 root（parent_id NULL 即 root）
MOVE=$(curl -s -X POST "$BASE/orgs/move" \
  -H "Authorization: Bearer $SAT" -H 'Content-Type: application/json' \
  -d "{\"id\":\"$FE_ID\",\"new_parent_id\":null}")
check "P2-D1 move fe → root code=0" "0" "$(echo "$MOVE" | json_code)"

FE_NEW_PATH=$(psql_q "SELECT path::text FROM organizations WHERE id=$FE_ID")
FE_TICKET_PATH_NEW=$(psql_q "SELECT org_path::text FROM tickets WHERE id=$FE_TID")
# fe 单节点，newPath 即自己的 code，oldPath 是 tech_code.fe_code
# 因此新路径应以 "$FE_CODE" 结尾，新工单路径应与 org 路径完全一致
check "P2-D1 move后 ticket org_path 同步" "$FE_NEW_PATH" "$FE_TICKET_PATH_NEW"
check "P2-D1 move后 org路径已改变" "1" "$([ "$FE_NEW_PATH" != "$FE_OLD_PATH" ] && echo 1 || echo 0)"

# ========================================================================
# R7 列表侧验证：admin 列表含所有 2a 中创建的工单（bypass L2 assigned 过滤）
# ========================================================================
ADMIN_LIST=$(curl -s "$BASE/tickets" -H "Authorization: Bearer $SAT")
ADMIN_IDS=$(echo "$ADMIN_LIST" | python3 -c "import sys,json
d=json.load(sys.stdin).get('data') or {}
ids=[str(t['id']) for t in d.get('list',[])]
print(','.join(ids))")
check "R7 admin list contains A's ticket" "1" "$(echo ",$ADMIN_IDS," | grep -q ",$TID_A," && echo 1 || echo 0)"
check "R7 admin list contains B's ticket" "1" "$(echo ",$ADMIN_IDS," | grep -q ",$TID_B," && echo 1 || echo 0)"
check "R7 admin list contains FE ticket" "1" "$(echo ",$ADMIN_IDS," | grep -q ",$FE_TID," && echo 1 || echo 0)"

# ========================================================================
# 汇总
# ========================================================================
echo ""
echo "========== Phase 2a Acceptance Summary =========="
echo "PASS=$pass  FAIL=$fail  SKIP=$skip"

if [ "$fail" -eq 0 ]; then
  echo "✅ Phase 2a ALL PASSED"
  exit 0
else
  echo "❌ Phase 2a HAS FAILURES ($fail)"
  exit 1
fi
