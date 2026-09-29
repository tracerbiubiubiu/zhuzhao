#!/bin/bash
# P4-W2 首日件：跨三仓本地编排（zhuzhao+activelist+taskrunner）
# 用法：bash scripts/dev-stack.sh [up|down|reset]
# 前提：三仓均在同级目录（../activelist ../taskrunner）

set -euo pipefail
cd "$(dirname "$0")/.."
CMD="${1:-up}"
ZROOT="$(pwd)"
AROOT="$ZROOT/../activelist"
TROOT="$ZROOT/../taskrunner"

# 固定 compose project name（防双栈网络别名冲突——27 批教训）
export COMPOSE_PROJECT_NAME=zhuzhao-dev

case "$CMD" in
up)
  echo "── ⓪ 外部网络（先建——taskrunner/activelist external 网首跑需已存在）──"
  docker network create zhuzhao_to_activelist 2>/dev/null || true
  docker network create zhuzhao_to_taskrunner 2>/dev/null || true
  echo "── ① zhuzhao dev 栈（PG+Redis）──"
  make docker-dev-up 2>/dev/null || docker compose -f deployments/docker-compose.yaml up -d postgres redis
  echo "── ② activelist ──"
  # 审计修正（2026-09-30）：deploy/compose.yaml 已改名（8f40ba9）——dev 联调用 compose.dev
  [ -d "$AROOT" ] && (cd "$AROOT" && docker compose -f deploy/compose.dev.yaml up -d 2>&1 | tail -1)
  echo "── ③ taskrunner ──"
  [ -d "$TROOT" ] && (cd "$TROOT" && docker compose -f deploy/compose.yaml up -d 2>/dev/null || echo "  taskrunner compose 未配置或已运行")
  echo "✅ 三栈编排完成（docker ps 查看状态）"
  ;;
down)
  [ -d "$TROOT" ] && (cd "$TROOT" && docker compose -f deploy/compose.yaml down 2>/dev/null || true)
  [ -d "$AROOT" ] && (cd "$AROOT" && docker compose -f deploy/compose.yaml down 2>/dev/null || true)
  make docker-dev-down 2>/dev/null || true
  echo "✅ 三栈已停"
  ;;
reset)
  "$0" down
  "$0" up
  ;;
*)
  echo "用法：$0 [up|down|reset]"
  exit 1
  ;;
esac
