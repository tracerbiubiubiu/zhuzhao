#!/bin/sh
# zz-commit —— 多会话安全提交包装（standards §12-11 提交隔离协议的执行工具）
#
# 用法：
#   scripts/zz-commit.sh <消息文件> <path>...            # 普通提交
#   scripts/zz-commit.sh --amend <消息文件> <path>...   # 修正上一笔（同 path 语义）
#
# 行为（防「提交其他会话文件」四步校验——dfa69c9/1418f33 索引污染实证后成文）：
#   1) 显式 add 仅声明的路径（禁 add -A / commit -a / 无路径裸 commit）
#   2) 校验暂存集合 ⊆ 声明路径集合（不等即拒绝——疑其他会话改动混入）
#   3) pathspec 提交（--only 语义：无视索引中其他会话的暂存内容）
#   4) 提交后自动 show --stat 复核输出
#
# 纪律：rebase/amend/reset 为会话独占操作，执行前确认 origin 无新提交且无
#   index.lock 竞态；推送前 fetch + 窗口差集核对（§12-9 push 前置三件）。
# 注：路径含空格的文件不支持（本仓跟踪文件无空格；如引入请改用 xargs 形态）。
set -eu

amend=""
if [ "${1:-}" = "--amend" ]; then
    amend="--amend"
    shift
fi
[ $# -ge 2 ] || { echo "usage: zz-commit.sh [--amend] <msgfile> <path>..."; exit 2; }
msgfile="${1}"
shift

paths=""
for a in "$@"; do
    paths="${paths} ${a}"
done

# 1) 只暂存声明的路径
git add -- "$@"

# 2) 暂存集合 ⊆ 声明路径集合（防卷入其他会话的文件）
staged=$(git diff --cached --name-only)
for f in ${staged}; do
    case "${paths}" in
        *"${f}"*) ;;
        *) echo "[FAIL] 暂存集含未声明路径: ${f}（疑其他会话改动混入——检查 git status 后重试）"; exit 3 ;;
    esac
done

# 3) pathspec 提交（--only 语义；${amend} 为空或 --amend——2026-11-01 修复：原先变量赋值后未传入，
#    --amend 形态静默退化为普通提交、在上一笔之上叠同名新提交）
git commit ${amend} -F "${msgfile}" -- ${paths}

# 4) 复核输出
git show --stat --format='%h %s' HEAD
