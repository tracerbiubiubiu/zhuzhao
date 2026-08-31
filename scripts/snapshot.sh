#!/usr/bin/env bash
# 项目现状快照：一条命令看清整个项目全貌。
# 用途：AI 快速迭代后，用机器扫描代替人工逐行检查，30 秒重建全局掌控感。
#
# 用法：bash scripts/snapshot.sh [--diff] [--md]
#   --diff  仅显示与上次快照的差异（需先跑一次全量生成基线）
#   --md    输出 Markdown（便于贴进 PR / 文档）

# 注意：不用 set -e —— 本脚本全是「可选统计」，grep 无匹配返回 1 属正常，
# 一旦 set -e 会因某条统计无结果而静默中断（历史坑：trap 还会删掉缓冲文件，导致零输出）。
set -uo pipefail
cd "$(dirname "$0")/.."

SNAP_DIR=".codebuddy/snapshots"
mkdir -p "$SNAP_DIR"
BUF=$(mktemp)
DIFF_MODE=0
MD_MODE=0
for arg in "$@"; do
  case "$arg" in
    --diff) DIFF_MODE=1 ;;
    --md)   MD_MODE=1 ;;
  esac
done

# 输出缓冲：--diff 模式需要先生成完整快照再比对
# 故意不用 trap 清理：异常退出时保留缓冲，便于排查（BUFFER 残留不影响功能）。
out() { printf '%s\n' "$*" >> "$BUF"; }
hr()  { out ""; out "─────────────────────────────────────────────────────────"; out ""; }

# ── 0. 版本与分支状态 ───────────────────────────────────────────
out "项目快照  $(date '+%Y-%m-%d %H:%M:%S')"
out "分支      $(git branch --show-current)  @  $(git log -1 --format='%h %s')"
DIRTY=$(git status --porcelain | wc -l | tr -d ' ')
[ "$DIRTY" -gt 0 ] && out "未提交    ${DIRTY} 个文件改动" || out "工作区    干净"

# ── 1. 代码规模 ─────────────────────────────────────────────────
hr
out "【1】代码规模"
NONTEST=$(find . -name '*.go' -not -name '*_test.go' -not -path './vendor/*' | wc -l | tr -d ' ')
TESTS=$(find . -name '*_test.go' -not -path './vendor/*' | wc -l | tr -d ' ')
LOC=$(find . -name '*.go' -not -path './vendor/*' -exec cat {} + | wc -l | tr -d ' ')
out "  业务文件 ${NONTEST}   测试文件 ${TESTS}   总行数 ${LOC}   测试比 $((TESTS * 100 / (NONTEST + TESTS)))%"

# ── 2. 分层结构（每层文件数，快速看有没有“长歪”） ────────────────
hr
out "【2】分层结构"
for d in handler service repository model middleware router pkg casbin config; do
  [ -d "internal/$d" ] || continue
  n=$(find "internal/$d" -name '*.go' -not -name '*_test.go' | wc -l | tr -d ' ')
  l=$(find "internal/$d" -name '*.go' -not -name '*_test.go' -exec cat {} + 2>/dev/null | wc -l | tr -d ' ')
  printf '  %-14s %2s 文件  %6s 行\n' "$d" "$n" "$l" >> "$BUF"
done

# ── 3. 数据库：表 + 迁移 ─────────────────────────────────────────
hr
out "【3】数据库"
out "  迁移 $(ls migrations/*.up.sql 2>/dev/null | wc -l | tr -d ' ') 个，最新：$(ls migrations/*.up.sql 2>/dev/null | tail -1 | xargs basename)"
# 兼容 CREATE TABLE / CREATE TABLE IF NOT EXISTS 两种写法
TABLES=$(grep -rhoiE 'CREATE TABLE (IF NOT EXISTS )?[a-z_]+' migrations/*.up.sql 2>/dev/null \
  | sed -E 's/.*CREATE TABLE (IF NOT EXISTS )?//I' | sort -u)
out "  数据表（$(printf '%s\n' "$TABLES" | grep -c . | tr -d ' ') 张）："
printf '%s\n' "$TABLES" | grep . | sed 's/^/    /' >> "$BUF"
out ""
EXTS=$(grep -rhoiE 'CREATE EXTENSION( IF NOT EXISTS)?\s+[A-Za-z_"]+' migrations/*.up.sql 2>/dev/null \
  | sed -E 's/.*EXTENSION( IF NOT EXISTS)?[ \t]+//I' | tr -d '"' | sort -u | tr '\n' ' ')
out "  扩展：${EXTS:-（无）}"

# ── 4. 路由清单（按模块分组，这是“系统能干什么”的直接答案） ────────
hr
out "【4】路由清单"
ROUTE_TOTAL=$(grep -coE '(GET|POST|PUT|DELETE|PATCH)\("' internal/router/router.go 2>/dev/null || echo 0)
out "  共 ${ROUTE_TOTAL} 条。缩进=Group 嵌套层级，可直观看清模块边界；"
out "  完整 URL = 从最近的 Group(\"...\") 前缀往下拼（此处保留源码缩进，避免解析失真）。"
out ""
# 提取 Group / 路由行，保留 tab 缩进（2 空格/tab），零解析风险。
grep -nE 'Group\("|(GET|POST|PUT|DELETE|PATCH)\("' internal/router/router.go 2>/dev/null \
  | sed -E 's/^([0-9]+):\t*/  /; s/\t/  /g' \
  | sed -E 's/^([ ]*)([0-9]+)[ ]*/\1/' >> "$BUF"

# ── 5. 权限模型（Casbin：谁能对什么做什么） ───────────────────────
hr
out "【5】权限模型"
MODEL=$(find . -name '*.conf' -path '*casbin*' -o -name 'rbac_model.conf' 2>/dev/null | head -1)
[ -n "$MODEL" ] && out "  模型文件：${MODEL}"
# 注意：xargs 无法调用 shell 函数（历史坑：`xargs -I{} out "..."` 报 "out: No such file"），
# 必须先落到变量再传给 out。
POLICY_WRITES=$(grep -rn "AddPolicy\|AddGroupingPolicy" internal/ 2>/dev/null | wc -l | tr -d ' ')
out "  策略写入点：${POLICY_WRITES} 处"
out "  资源动作码（router 中出现的权限码）："
grep -rhoE '"[a-z_]+:(list|read|create|update|delete|approve|assign|close)"' internal/router/ internal/handler/ 2>/dev/null \
  | sort -u | sed 's/^/    /' >> "$BUF"

# ── 6. 测试与验收门禁 ────────────────────────────────────────────
hr
out "【6】测试与门禁"
for f in scripts/acceptance-*.sh; do
  [ -e "$f" ] || continue
  printf '  %s\n' "$(basename "$f")" >> "$BUF"
done
out "  集成测试函数：$(grep -rhoE '^func (Test[A-Za-z0-9_]+)' internal/ --include='*_test.go' 2>/dev/null | wc -l | tr -d ' ') 个"

# ── 7. 技术债雷达（这些是“AI 悄悄欠下的账”的探测器） ────────────────
hr
out "【7】技术债雷达"
# count_pat 统计非测试代码中的模式。
# 只统计 *_test.go 之外的文件：测试里的 panic/mock 密码属正常，计入会制造假警报
# （历史坑：一度报「panic 9 处」，实际业务代码 0 处，测试占 8 处）。
count_pat() {
  local label="$1" pat="$2"
  local n
  n=$(grep -rInE "$pat" internal/ 2>/dev/null | grep -v '_test\.go' | wc -l | tr -d ' ')
  printf '  %-22s %s\n' "$label" "$n" >> "$BUF"
}
count_pat "TODO/FIXME"      'TODO|FIXME'
count_pat "XXX/HACK"        'XXX|HACK'
count_pat "裸 fmt.Print"    'fmt\.Print(ln)?\('
count_pat "context.TODO"    'context\.TODO'
count_pat "硬编码密码/密钥" '(password|secret|apikey|api_key)\s*[:=]\s*"[^"]{8,}"'

# panic 单列：启动期 fail-fast 是正确做法（配置非法应快速失败），
# 只有业务路径（handler/service/repository）的 panic 才是缺陷。
PANIC_BIZ=$(grep -rIn 'panic(' internal/handler internal/service internal/repository 2>/dev/null | grep -v '_test\.go' | wc -l | tr -d ' ')
PANIC_ALL=$(grep -rIn 'panic(' internal/ 2>/dev/null | grep -v '_test\.go' | wc -l | tr -d ' ')
printf '  %-22s %s（业务层 %s / 启动期等 %s）\n' "panic(" "$PANIC_ALL" "$PANIC_BIZ" "$((PANIC_ALL - PANIC_BIZ))" >> "$BUF"
if [ "$PANIC_BIZ" -gt 0 ]; then
  out "    ⚠️ 业务层 panic 明细（应改为返回 error）："
  grep -rIn 'panic(' internal/handler internal/service internal/repository 2>/dev/null \
    | grep -v '_test\.go' | head -12 | sed 's/^/      /' >> "$BUF"
else
  out "    ✅ 业务层无 panic（启动期 fail-fast 属正常）"
fi
if [ "$PANIC_ALL" -gt "$PANIC_BIZ" ]; then
  out "    启动期 panic（配置校验等，属 fail-fast 正常做法）："
  grep -rIn 'panic(' internal/ 2>/dev/null | grep -v '_test\.go' \
    | grep -vE 'internal/(handler|service|repository)/' | head -8 | sed 's/^/      /' >> "$BUF"
fi
out "  未处理 TODO 明细："
TODO_N=$(grep -rInE 'TODO|FIXME' internal/ 2>/dev/null | grep -v '_test\.go' | wc -l | tr -d ' ')
if [ "$TODO_N" -gt 0 ]; then
  grep -rInE 'TODO|FIXME' internal/ 2>/dev/null | grep -v '_test\.go' | head -12 | sed 's/^/    /' >> "$BUF"
else
  out "    （无）"
fi

# ── 8. 文档与代码一致性（文档说了但代码没有 = 失控前兆） ─────────────
hr
out "【8】文档 / 代码一致性"
out "  Phase 文档："
for p in phase1 phase2 phase3; do
  [ -d "docs/$p" ] || continue
  printf '    %-8s %s 个文档\n' "$p" "$(ls docs/$p/*.md 2>/dev/null | wc -l | tr -d ' ')" >> "$BUF"
done
out "  文档断链（引用了但文件不存在）——文档腐化信号："
# 关键：链接是相对「引用文件所在目录」解析的，必须带上来源文件才能算对路径。
# （历史坑：一律按 docs/ 前缀解析，会把 ../ 开头的链接全部误报为断链。）
find docs -name '*.md' -print0 2>/dev/null | while IFS= read -r -d '' src; do
  srcdir=$(dirname "$src")
  # 提取该文件中的 md 链接（跳过 http(s) 与页内锚点）
  grep -oE '\]\([^)]+\.md[^)]*\)' "$src" 2>/dev/null | sed -E 's/^\]\(//; s/\)$//' | while IFS= read -r link; do
    case "$link" in
      http://*|https://*|'#'*) continue ;;
    esac
    target="${link%%#*}" # 去掉 #锚点
    [ -z "$target" ] && continue
    if [ ! -e "$srcdir/$target" ]; then
      echo "    ✗ $src  →  $link"
    fi
  done
done | sort -u > /tmp/zz_broken.$$ 2>/dev/null
if [ -s /tmp/zz_broken.$$ ]; then
  cat /tmp/zz_broken.$$ >> "$BUF"
  out "    断链数：$(wc -l < /tmp/zz_broken.$$ | tr -d ' ')"
else
  out "    （无断链）"
fi
rm -f /tmp/zz_broken.$$ 2>/dev/null

# ── 输出 / 差异比对 ──────────────────────────────────────────────
if [ "$DIFF_MODE" -eq 1 ]; then
  BASE="$SNAP_DIR/latest.txt"
  if [ -f "$BASE" ]; then
    echo "=== 与上次快照的差异 ==="
    # 时间戳必然变化，比对前剔除，否则 diff 永远「有变化」而失去价值。
    grep -v '^项目快照 ' "$BASE" > /tmp/zz_base.$$ 2>/dev/null
    grep -v '^项目快照 ' "$BUF"   > /tmp/zz_cur.$$  2>/dev/null
    if diff /tmp/zz_base.$$ /tmp/zz_cur.$$; then
      echo "✅ 与上次快照一致（规模/分层/表/路由/权限/技术债均无变化）"
    fi
    rm -f /tmp/zz_base.$$ /tmp/zz_cur.$$ 2>/dev/null
  else
    echo "无基线，输出全量："
    cat "$BUF"
  fi
else
  cp "$BUF" "$SNAP_DIR/latest.txt"
  cp "$BUF" "$SNAP_DIR/snapshot-$(date '+%Y%m%d-%H%M%S').txt"
  cat "$BUF"
fi

if [ "$MD_MODE" -eq 1 ]; then
  echo ""
  echo "> Markdown 版已存至 $SNAP_DIR/latest.md"
  { echo "# 项目快照"; echo '```'; cat "$BUF"; echo '```'; } > "$SNAP_DIR/latest.md"
fi
