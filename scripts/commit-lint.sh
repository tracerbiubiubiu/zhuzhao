#!/bin/sh
# commit-lint —— §12-9 提交信息格式扫描器（发现链第 1 道：push 决策点必跑）
#
# 用法：
#   scripts/commit-lint.sh                  # 默认扫待推送窗口 origin/<当前分支>..HEAD
#   scripts/commit-lint.sh HEAD~40          # 扫近 40 笔（单点参数自动补 ..HEAD）
#   make commit-lint                        # 同第一行
#   make commit-lint RANGE=HEAD~40          # 同第二行
#
# 校验规则（docs/standards.md §12-9 + AGENTS.md 三节）：
#   ① 主题行 = type(scope): 摘要（type 枚举小写；scope 可选小写）
#   ② 摘要 <=50 字（去前缀按字符计，中文友好；2026-10-30 所有者拍 40→50）
#   ③ 摘要结尾不加句号
#   ④ 主题行与正文之间必须空行
#   ⑤ 正文至少有一行非空内容（AGENTS.md 三节必附：改动摘要/影响面/验证证据）
#   ⑥ swagger 新鲜度（窗口级）：触及 internal/model|handler 的窗口，本地跑
#      make swag 后 docs 生成物必须零漂移（对齐 CI lint 形态——红窗三次实证
#      f6a9cd7/c59f30b/4fa17e1 均为「注解/模型改了、生成物没进 paths」）
#   ⑦ 披露一致性（WARN，文件级）——正文声称的「NN 号」须有对应 docs/phase*/NN-*.md
#      变更（自动化 dc9acd7 类「声称同步某文件但文件不在提交」；内容级断言——
#      声称的具体小节/量级注是否真改——仍需人工 grep，e4ea567 教训；WARN 级不 FAIL，
#      引用型提法会误报，升格 FAIL 前先观察一个月）
#   ⑨ fix 正文三节结构（WARN）：各节须存在且有内容，支持无 scope 的 fix。
#      先观察误报/漏报，不以新规则阻断存量提交；证据真实性仍须人工复核。
#
# 退出码：0 全绿 / 1 有违例（可复用于 CI 兜底道）。
# 违例处置：push 前发现 -> git rebase -i <base> 对应笔 reword（本地历史可改窗口）；
#          已 push 存量不追溯（§12-9 原口径）。
#
# ⚠ 实现注记：执行字符串一律 ASCII + 变量引用用 ${var} 花括号形式——
#   macOS 自带 bash 3.2 在 set -u + UTF-8 locale 下，双引号内 $var 紧贴
#   多字节字符（emoji/中文标点）会被误拼进变量名报 unbound（实测两次）。
#   中文只出现在注释行（bash 词法忽略，无风险）。

set -u
export LC_ALL=en_US.UTF-8
script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)

range=""
explicit_range=0
if [ -n "${1:-}" ]; then
    range="${1}"
    explicit_range=1
else
    branch=$(git rev-parse --abbrev-ref HEAD 2>/dev/null || true)
    if [ -n "${branch}" ] && [ "${branch}" != "HEAD" ] &&
       git show-ref --verify --quiet "refs/remotes/origin/${branch}" 2>/dev/null; then
        range="origin/${branch}..HEAD"
    else
        # 初推检查固定启用基线之后全部提交，不猜近20笔。
        baseline=$(cat "${script_dir}/commit-policy-baseline") || exit 1
        range="${baseline}..HEAD"
    fi
fi
# 单点参数语义修正：HEAD~20 应为「近 20 笔」——rev-list 单点会列全部祖先
case "${range}" in
    *..*) ;;
    *) range="${range}..HEAD" ;;
esac
if ! git rev-list -1 "${range}" >/dev/null 2>&1; then
    echo "[FAIL] commit-lint: invalid range '${range}'"
    exit 1
fi

violations=0
total=0
[ "$(git rev-parse --is-shallow-repository)" = false ] || {
    echo "[FAIL] shallow history; fetch --unshallow"; exit 1;
}
if [ "${explicit_range}" -eq 1 ]; then
    commits=$(git rev-list "${range}") || exit 1
else
    baseline=$(cat "${script_dir}/commit-policy-baseline") || exit 1
    printf '%s\n' "${baseline}" | grep -qE '^[0-9a-f]{40}$' || {
        echo "[FAIL] invalid adoption baseline"; exit 1;
    }
    git cat-file -e "${baseline}^{commit}" || exit 1
    commits=$(git rev-list "${range}" --not "${baseline}") || exit 1
    echo "[INFO] adoption baseline ancestors excluded: ${baseline}"
fi
for h in ${commits}; do
    total=$((total + 1))
    subject=$(git log -1 --format=%s "${h}")
    body=$(git log -1 --format=%B "${h}")
    reasons=""

    # ①-⑤与commit-msg/pre-push/五仓CI共用同一消息规则。
    if checked=$(printf '%s\n' "${body}" | python3 "${script_dir}/commit-policy.py" message - --raw 2>&1); then
        :
    else
        reasons="${checked}
    [1-5] shared message policy failed"
    fi

    # ⑦ 披露一致性（WARN，文件级）——正文声称的「NN 号」须有对应 docs/phase*/NN-*.md 变更
    #    （内容级断言仍需人工 grep；WARN 不计入 violations）
    changed_files=$(git show --name-only --format= "${h}" 2>/dev/null)
    claim_refs=$(printf '%s' "${body}" | grep -oE '[0-9]{1,2} 号' | sed 's/ 号//' | sort -u || true)
    if [ -n "${claim_refs}" ]; then
        missing=""
        for nn in ${claim_refs}; do
            case "${changed_files}" in
                *"docs/phase"*/"${nn}-"*|*"docs/review/${nn}-"*|*"docs/phase"*/"${nn}_"*) ;;
                *) missing="${missing} ${nn} 号" ;;
            esac
        done
        if [ -n "${missing}" ]; then
            echo "[WARN] ${h}: body cites 号 refs with no matching file change:${missing} (verify manually)"
        fi
    fi

    # ⑧ 修复批同类扫描行（WARN）——standards §12「同类扫描」行立法（2026-10-29 六条接力链复盘）：
    #    fix 类提交正文须含「同类扫描：」行；docs 批同等适用但留人工纪律（登记/演进类 docs 批
    #    非修复语义，机械强校验噪声大——先 fix 面试点）。中文出注释/单引号模式，echo 保持 ASCII
    #    （脚本头实现注记：$var 紧贴多字节字符在 bash 3.2 + UTF-8 下有 unbound 前科）
    case "${subject}" in
        fix\(*|fix:\ *)
            printf '%s' "${body}" | grep -q '同类扫描' || \
                echo "[WARN] ${h}: fix-type commit body lacks 'tonglei-scan' line (standards §12 same-class scan rule - skip if not a fix batch)"
            if ! printf '%s\n' "${body}" | awk -v commit="${h}" -f "${script_dir}/check-fix-body.awk"; then
                reasons="${reasons}
    [9] fix body checker could not run"
            fi
            ;;
    esac

    if [ -n "${reasons}" ]; then
        violations=$((violations + 1))
        echo "[FAIL] $(git log -1 --format='%h %s' "${h}")${reasons}"
    fi
done

if [ "${violations}" -gt 0 ]; then
    echo "----"
    echo "[FAIL] commit-lint: ${violations}/${total} commits violate 12-9"
    echo "       fix before push: git rebase -i <base> then reword; pushed history stays untouched"
    exit 1
fi
if [ "${total}" -eq 0 ]; then
    echo "[OK] commit-lint: no commits in range (all pushed?)"
    exit 0
fi

# ⑥ swagger 新鲜度（窗口级一次性，对齐 CI lint 形态）——触及注解/模型面的窗口，
#    本地重跑 swag 后 docs 生成物必须零漂移；漂移 = 生成物过期（第三次红窗实证后前移）。
swagger_stale=0
changed_api=$(git diff --name-only "${range}" -- internal/model internal/handler 2>/dev/null)
if [ -n "${changed_api}" ]; then
    swag_bin=$(command -v swag || true)
    if [ -z "${swag_bin}" ]; then
        gopath=$(go env GOPATH 2>/dev/null || true)
        if [ -n "${gopath}" ] && [ -x "${gopath}/bin/swag" ]; then
            swag_bin="${gopath}/bin/swag"
        fi
    fi
    if [ -z "${swag_bin}" ]; then
        echo "[WARN] swagger gate skipped: swag CLI not found (go install github.com/swaggo/swag/cmd/swag@v1.16.4)"
    else
        if ! "${swag_bin}" init -g cmd/server/main.go -o docs --parseDependency --parseInternal >/dev/null 2>&1; then
            echo "[FAIL] swagger gate: generator failed"
            exit 1
        fi
        stale=$(git diff --name-only -- docs/docs.go docs/swagger.json docs/swagger.yaml)
        if [ -n "${stale}" ]; then
            swagger_stale=1
            echo "[FAIL] swagger gate: artifacts stale for this window (api files changed, docs/ regenerated differs) --"
            echo "       run 'make swag' (already done) and ADD the docs/ delta to your commit:"
            git diff --stat -- docs/docs.go docs/swagger.json docs/swagger.yaml | sed 's/^/       /'
        fi
    fi
fi

if [ "${swagger_stale}" -ne 0 ]; then
    echo "[FAIL] commit-lint: ${total} commits format-ok but swagger artifacts stale (rule 6)"
    exit 1
fi
echo "[OK] commit-lint: ${total} commits all pass (12-9)"
exit 0
