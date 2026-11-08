#!/bin/bash
# Compose 依赖/部署辅助：main/AL 的宿主应用、SQLite taskrunner、vite 另行启动。
# 用法：bash scripts/dev-stack.sh [up|down|reset]；reset 保留数据卷。
set -euo pipefail
cd "$(dirname "$0")/.."
CMD="${1:-up}"
ZROOT="$(pwd -P)"
AROOT="${ACTIVELIST_DIR:-${ZROOT}/../activelist}"
TROOT="${TASKRUNNER_DIR:-${ZROOT}/../taskrunner}"
ZPROJECT="${ZHUZHAO_DEV_PROJECT:-zhuzhao-dev}"
# 保留既有开发卷归属：AL 原 compose 默认 project=deploy，TR name=taskrunner。
APROJECT="${ACTIVELIST_DEV_PROJECT:-deploy}"
TPROJECT="${TASKRUNNER_PROJECT:-taskrunner}"
WAIT_TIMEOUT="${DEV_STACK_WAIT_TIMEOUT:-90}"

fail() { echo "ERROR: $*" >&2; exit 1; }
case "${CMD}" in up|down|reset) ;; *) fail "用法：$0 [up|down|reset]" ;; esac
for project in "${ZPROJECT}" "${APROJECT}" "${TPROJECT}"; do
  [[ "${project}" =~ ^[a-z0-9][a-z0-9_-]*$ ]] || fail "无效 Compose project：${project}"
done
[[ "${ZPROJECT}" != "${APROJECT}" && "${ZPROJECT}" != "${TPROJECT}" && "${APROJECT}" != "${TPROJECT}" ]] || fail "三组 Compose project 必须不同"
[[ "${WAIT_TIMEOUT}" =~ ^[1-9][0-9]*$ ]] || fail "DEV_STACK_WAIT_TIMEOUT 必须为正整数"
[ -d "${AROOT}" ] || fail "activelist 仓库不存在：${AROOT}（可设 ACTIVELIST_DIR）"
[ -d "${TROOT}" ] || fail "taskrunner 仓库不存在：${TROOT}（可设 TASKRUNNER_DIR）"
AROOT="$(cd "${AROOT}" && pwd -P)"
TROOT="$(cd "${TROOT}" && pwd -P)"
ZFILE="${ZROOT}/deployments/docker-compose.dev.yaml"
AFILE="${AROOT}/deploy/compose.dev.yaml"
TFILE="${TROOT}/deploy/compose.yaml"
for file in "${ZFILE}" "${AFILE}" "${TFILE}"; do
  [ -f "${file}" ] || fail "Compose 文件不存在：${file}"
done
command -v docker >/dev/null || fail "需要 Docker Compose v2（docker compose）"
docker compose version >/dev/null
docker info >/dev/null

compose() {
  local root="$1" project="$2" file="$3"
  shift 3
  (cd "${root}" && docker compose -p "${project}" -f "${file}" "$@")
}

# 在任何 network/up/down 前核配置、project 与固定容器归属；不自动接管旧栈。
check_project_owner() {
  local project="$1" file="$2" ids id sources
  ids="$(docker ps -a --filter "label=com.docker.compose.project=${project}" --format '{{.ID}}')"
  for id in ${ids}; do
    sources="$(docker container inspect --format '{{index .Config.Labels "com.docker.compose.project.config_files"}}' "${id}")"
    local source owned=false
    local -a config_sources
    [ -n "${sources}" ] || fail "project ${project} 缺少 Compose 来源标签（${id}），不会接管"
    IFS=, read -r -a config_sources <<< "${sources}"
    for source in "${config_sources[@]}"; do
      if [ "${source}" = "${file}" ] || [ "${source}" -ef "${file}" ]; then
        owned=true
        break
      fi
    done
    [ "${owned}" = true ] || fail "project ${project} 已由其它 Compose 文件占用（${id}）；先核归属与数据卷，脚本不会接管"
  done
}
check_fixed_owner() {
  local name="$1" project="$2" service="$3" owner
  if owner="$(docker container inspect --format '{{index .Config.Labels "com.docker.compose.project"}}|{{index .Config.Labels "com.docker.compose.service"}}' "${name}" 2>/dev/null)"; then
    [ "${owner}" = "${project}|${service}" ] || fail "容器 ${name} 归属 ${owner}，与 ${project}|${service} 不同；先核数据卷，脚本不会接管"
  fi
}
compose "${ZROOT}" "${ZPROJECT}" "${ZFILE}" config --quiet
compose "${AROOT}" "${APROJECT}" "${AFILE}" config --quiet
compose "${TROOT}" "${TPROJECT}" "${TFILE}" config --quiet
check_project_owner "${ZPROJECT}" "${ZFILE}"
check_project_owner "${APROJECT}" "${AFILE}"
check_project_owner "${TPROJECT}" "${TFILE}"
check_fixed_owner zhuzhao-dev-postgres "${ZPROJECT}" postgres
check_fixed_owner zhuzhao-dev-redis "${ZPROJECT}" redis
check_fixed_owner activelist-dev-postgres "${APROJECT}" postgres

up() {
  for network in zhuzhao_to_activelist zhuzhao_to_taskrunner; do
    docker network inspect "${network}" >/dev/null 2>&1 || docker network create "${network}" >/dev/null
  done
  echo "── ① zhuzhao dev 依赖：PG/Redis（${ZPROJECT}）──"
  compose "${ZROOT}" "${ZPROJECT}" "${ZFILE}" up -d --wait --wait-timeout "${WAIT_TIMEOUT}" postgres redis
  echo "── ② activelist dev 依赖：PG（${APROJECT}）──"
  compose "${AROOT}" "${APROJECT}" "${AFILE}" up -d --wait --wait-timeout "${WAIT_TIMEOUT}" postgres
  echo "── ③ taskrunner 容器部署栈（${TPROJECT}，仅内网）──"
  compose "${TROOT}" "${TPROJECT}" "${TFILE}" up -d --wait --wait-timeout "${WAIT_TIMEOUT}"
  echo "✅ 三组 Compose 资源就绪；宿主 app/AL/taskrunner/vite 与回调链另行检查（docs/ops/deployment.md）"
}
down() {
  compose "${TROOT}" "${TPROJECT}" "${TFILE}" down
  compose "${AROOT}" "${APROJECT}" "${AFILE}" down
  compose "${ZROOT}" "${ZPROJECT}" "${ZFILE}" down
  echo "✅ 三组 Compose 已停；数据卷与共享外部网络保留"
}
case "${CMD}" in
  up) up ;;
  down) down ;;
  reset) down; up ;;
esac
