# Fix-body structure check. Warnings only; semantic evidence remains a review task.
BEGIN {
    titles[1] = "改动摘要"
    titles[2] = "影响面"
    titles[3] = "验证证据"
    labels[1] = "changes"
    labels[2] = "impact"
    labels[3] = "evidence"
    current = 0
}
{
    line = $0
    sub(/\r$/, "", line)
    sub(/^[[:space:]]*#+[[:space:]]+/, "", line)
    gsub(/\*\*/, "", line)
    sub(/^[[:space:]]+/, "", line)
    sub(/[[:space:]]+$/, "", line)

    # Footers must not turn an otherwise empty evidence section into a full one.
    if (line ~ /^(Refs|Fixes|BREAKING CHANGE|批次)[[:space:]]*:/) {
        current = 0
        next
    }

    heading = 0
    for (i = 1; i <= 3; i++) {
        if (index(line, titles[i]) != 1) continue
        rest = substr(line, length(titles[i]) + 1)
        sub(/^[[:space:]]+/, "", rest)
        if (rest == "" || index(rest, ":") == 1 || index(rest, "：") == 1) {
            heading = i
            break
        }
    }
    if (heading) {
        current = heading
        seen[current]++
        if (index(rest, ":") == 1) rest = substr(rest, 2)
        else if (index(rest, "：") == 1) rest = substr(rest, length("：") + 1)
        if (rest ~ /[^[:space:]]/) content[current] = 1
        next
    }

    if (current && line !~ /^[[:space:]#*>_`-]*$/) content[current] = 1
}
END {
    for (i = 1; i <= 3; i++) {
        if (!seen[i])
            printf "[WARN] %s: fix body missing section: %s\n", commit, labels[i]
        else if (!content[i])
            printf "[WARN] %s: fix body empty section: %s\n", commit, labels[i]
        if (seen[i] > 1)
            printf "[WARN] %s: fix body duplicate section: %s\n", commit, labels[i]
    }
}
