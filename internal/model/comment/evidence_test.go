package comment

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"
	"time"
)

func TestParseEvidenceRef(t *testing.T) {
	t.Parallel()
	for _, ref := range []string{"ticket-comment:1", "ticket-comment:42", "ticket-comment:123456789012345"} {
		id, err := ParseEvidenceRef(ref)
		if err != nil || id <= 0 {
			t.Fatalf("ParseEvidenceRef(%q)=%d,%v want positive id", ref, id, err)
		}
	}
	for _, ref := range []string{
		"", "ticket-comment:", "ticket-comment:0", "ticket-comment:-1", "ticket-comment:+1",
		"ticket-comment:01", "ticket-comment: 1", "ticket-comment:1 ", "ticket-comment:1x",
		"ticket-comment:99999999999999999999", "comment:1", "file:1", "TICKET-COMMENT:1",
	} {
		if id, err := ParseEvidenceRef(ref); err == nil || id != 0 {
			t.Fatalf("ParseEvidenceRef(%q)=%d,%v want ErrNotCanonicalRef", ref, id, err)
		} else if !errors.Is(err, ErrNotCanonicalRef) {
			t.Fatalf("ParseEvidenceRef(%q) err=%v want ErrNotCanonicalRef", ref, err)
		}
	}
}

func TestEvidenceVersionTokenCanonical(t *testing.T) {
	t.Parallel()
	createdAt := time.Date(2026, 11, 23, 8, 30, 5, 123456000, time.UTC)
	content := "修复完成：升级至 2.3.1，复测环境见变更记录。"
	token := EvidenceVersionToken(createdAt, content)
	sum := sha256.Sum256([]byte(content))
	want := hex.EncodeToString(sum[:]) + "@2026-11-23T08:30:05.123456Z"
	if token != want {
		t.Fatalf("token=%q want %q", token, want)
	}
}

func TestEvidenceVersionTokenTimezoneIndependent(t *testing.T) {
	t.Parallel()
	content := "same content"
	utc := time.Date(2026, 11, 23, 0, 30, 5, 500000000, time.UTC)
	shifted := utc.In(time.FixedZone("CST", 8*3600))
	if EvidenceVersionToken(utc, content) != EvidenceVersionToken(shifted, content) {
		t.Fatalf("same instant in different zones must produce one token")
	}
	token := EvidenceVersionToken(shifted, content)
	if token[len(token)-1] != 'Z' {
		t.Fatalf("token=%q must end in UTC designator Z", token)
	}
}

func TestEvidenceVersionTokenFractionTrimming(t *testing.T) {
	t.Parallel()
	whole := time.Date(2026, 11, 23, 8, 30, 5, 0, time.UTC)
	if got := EvidenceVersionToken(whole, "x"); got[len(got)-len("T08:30:05Z"):] != "T08:30:05Z" {
		t.Fatalf("zero fraction must omit decimal seconds, got %q", got)
	}
	trimmed := time.Date(2026, 11, 23, 8, 30, 5, 500000000, time.UTC)
	if got := EvidenceVersionToken(trimmed, "x"); got[len(got)-len("T08:30:05.5Z"):] != "T08:30:05.5Z" {
		t.Fatalf("trailing zeros must be trimmed once, got %q", got)
	}
}

func TestParseEvidenceVersionRoundTrip(t *testing.T) {
	t.Parallel()
	createdAt := time.Date(2026, 11, 23, 8, 30, 5, 123456000, time.UTC)
	content := "handle-with-care"
	token := EvidenceVersionToken(createdAt, content)
	hash, parsed, err := ParseEvidenceVersion(token)
	if err != nil {
		t.Fatalf("ParseEvidenceVersion(%q) err=%v", token, err)
	}
	sum := sha256.Sum256([]byte(content))
	if hash != hex.EncodeToString(sum[:]) || !parsed.Equal(createdAt) {
		t.Fatalf("round trip hash=%s parsed=%v mismatch", hash, parsed)
	}
}

func TestParseEvidenceVersionRejectsNonCanonical(t *testing.T) {
	t.Parallel()
	good := EvidenceVersionToken(time.Date(2026, 11, 23, 8, 30, 5, 0, time.UTC), "x")
	at := len(good) - len("T08:30:05Z")
	hash, stamp := good[:at], good[at+1:]
	bad := []string{
		"", "@", "@2026-11-23T08:30:05Z",
		good + "@2026-11-23T08:30:06Z",        // 双 @
		"ABCDEF" + hash[6:] + "@" + stamp,     // 大写十六进制
		hash[:63] + "@" + stamp,               // 63 位
		hash + "0@" + stamp,                   // 65 位
		hash[:62] + "g0@" + stamp,             // 非十六进制
		hash + "@2026-11-23T08:30:05+08:00",   // 时区偏移
		hash + "@2026-11-23T08:30:05.000000Z", // 未去尾零
		hash + "@2026-11-23T08:30:05.000Z",    // 毫秒变体
		hash + "@2026-11-23 08:30:05Z",        // 空格分隔
		hash + "@2026-11-23T08:30:05",         // 缺时区
		"v1:" + good,                          // 夹带前缀
	}
	for _, version := range bad {
		hash, parsed, err := ParseEvidenceVersion(version)
		if err == nil || !errors.Is(err, ErrNotCanonicalVersion) || hash != "" || !parsed.IsZero() {
			t.Fatalf("ParseEvidenceVersion(%q)=(%q,%v,%v) want ErrNotCanonicalVersion", version, hash, parsed, err)
		}
	}
	if _, _, err := ParseEvidenceVersion(good); err != nil {
		t.Fatalf("canonical token %q rejected: %v", good, err)
	}
}
