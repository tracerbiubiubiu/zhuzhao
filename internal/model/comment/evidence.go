// Package comment 定义公开评论作为「可核证据引用」时的规范合同
// （Phase6 B2 验收卡二 R1：引用必须能定位稳定对象与版本，当前复测人可按
// 既有 GET 工单评论读取核对）。
//
// 适用字段的权威模型是 internal/model/ticket.go 的 TicketComment（2026-11-23
// 告知项：基础模型在 ticket.go，本包不复制全量模型，只拥有引用格式、内容
// 摘要与版本规范形的唯一实现）。公开评论只增不改（repo 仅 INSERT，随工单
// 级联删除），因此内容 SHA256 与 created_at 即可同时充当稳定版本指纹；
// 内部备注（is_internal）有更窄的可见性边界，不属于可核对象。
//
// 同一规范形由两条路径共用：公开读面把它算入 TicketComment.EvidenceVersion
// 供 UI/API 原样取用，证据核验（ticket 域 owner）按同函数重算比对——任何
// 一侧不得自行改写时间格式或摘要算法。
package comment

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strconv"
	"strings"
	"time"
)

// EvidenceRefPrefix 引用唯一合法前缀；其后必须是正十进制评论 ID（无符号、
// 无前导零、无空白）。
const EvidenceRefPrefix = "ticket-comment:"

// EvidenceRef 是证据核验面使用的引用形态。wire 侧即
// riskremediation.EvidenceRef 的 kind="reference" 行；Ref/Version 原样传输，
// 不在任何一层改写。
type EvidenceRef struct {
	Ref     string
	Version string
}

// ErrNotCanonicalRef / ErrNotCanonicalVersion 非规范形（解析失败的可判别原因；
// 服务层统一映射为参数错误，不区分具体形态以避免探测面）。
var (
	ErrNotCanonicalRef     = errors.New("ticket-comment 引用非规范形")
	ErrNotCanonicalVersion = errors.New("评论证据版本串非规范形")
)

// ParseEvidenceRef 解析唯一合法形态 ticket-comment:<正十进制ID>。
// ParseInt 接受的 "+1"、"01"、空白等形式经 FormatInt 回写比对一律拒绝。
func ParseEvidenceRef(ref string) (int64, error) {
	digits, ok := strings.CutPrefix(ref, EvidenceRefPrefix)
	if !ok {
		return 0, ErrNotCanonicalRef
	}
	id, err := strconv.ParseInt(digits, 10, 64)
	if err != nil || id <= 0 || strconv.FormatInt(id, 10) != digits {
		return 0, ErrNotCanonicalRef
	}
	return id, nil
}

// EvidenceVersionToken 计算公开评论的稳定版本规范形：
//
//	<内容 SHA256 小写十六进制>@<created_at 的 UTC RFC3339Nano>
//
// SHA256 取 content 的 UTF-8 字节（TEXT 落库即 UTF-8，读回一致）。
// 时间显式归一到 UTC：pgx 扫描 timestamptz 落在进程本地时区，JSON created_at
// 随部署时区漂移，不能直接当版本串；PostgreSQL 微秒精度经 RFC3339Nano 去尾零
// 后与解析端逐字节可复现。复测人用 GET 评论返回的 content 与 evidence_version
// 即可独立重算核对。
func EvidenceVersionToken(createdAt time.Time, content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:]) + "@" + createdAt.UTC().Format(time.RFC3339Nano)
}

// ParseEvidenceVersion 校验并拆开规范形版本串。只接受本包产出的形态：
// 小写 64 位十六进制 + 单个 "@" + UTC RFC3339Nano（时区偏移、大写十六进制、
// 未去尾零的小数秒等非规范变体经回写比对一律拒绝）。
func ParseEvidenceVersion(version string) (contentSHA256 string, createdAt time.Time, err error) {
	at := strings.IndexByte(version, '@')
	if at <= 0 || strings.Contains(version[at+1:], "@") {
		return "", time.Time{}, ErrNotCanonicalVersion
	}
	hash, stamp := version[:at], version[at+1:]
	if !lowercaseHex64(hash) {
		return "", time.Time{}, ErrNotCanonicalVersion
	}
	parsed, err := time.Parse(time.RFC3339Nano, stamp)
	if err != nil || parsed.UTC().Format(time.RFC3339Nano) != stamp {
		return "", time.Time{}, ErrNotCanonicalVersion
	}
	return hash, parsed, nil
}

func lowercaseHex64(s string) bool {
	if len(s) != sha256.Size*2 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}
