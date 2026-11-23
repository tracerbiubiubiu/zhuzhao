package risk

import (
	"errors"
	"strings"
)

// 来源序号：规范无符号十进制串（详设 §13——拒符号/小数/指数/前导零；
// 比较按长度、同长字典序，不经 JS number）。adapter 仅在证明源等价语义后归一化。

var errNonCanonicalSequence = errors.New("source_sequence 不是规范无符号十进制串")

func validateCanonicalSequence(s string) error {
	if s == "" {
		return errNonCanonicalSequence
	}
	if len(s) > 1 && s[0] == '0' {
		return errNonCanonicalSequence // 前导零
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return errNonCanonicalSequence // 符号/小数/指数/分隔符/非 ASCII 数字
		}
	}
	return nil
}

func compareCanonicalSequence(a, b string) int {
	if len(a) != len(b) {
		if len(a) < len(b) {
			return -1
		}
		return 1
	}
	return strings.Compare(a, b)
}
