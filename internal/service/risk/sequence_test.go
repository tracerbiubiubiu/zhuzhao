package risk

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// 规范无符号十进制串校验（详设 §13：拒符号/小数/指数/前导零；不经 JS number）。
func TestCanonicalSequenceValidation(t *testing.T) {
	valid := []string{"0", "1", "42", "9", "10", "123456789012345678901234567890"}
	for _, s := range valid {
		require.NoError(t, validateCanonicalSequence(s), "expected valid: %q", s)
	}
	invalid := []string{
		"", " 1", "1 ", "+1", "-1", "1.0", "1.5", "1e3", "1E3",
		"01", "007", "0x10", "1_000", "١٢", "nan", "true", "18446744073709551616a",
	}
	for _, s := range invalid {
		require.Error(t, validateCanonicalSequence(s), "expected invalid: %q", s)
	}
}

// 比较按长度、同长字典序（详设 §13）："9" < "10"、"99" < "100"、等串相等。
func TestCompareCanonicalSequence(t *testing.T) {
	require.Equal(t, -1, compareCanonicalSequence("9", "10"))
	require.Equal(t, -1, compareCanonicalSequence("99", "100"))
	require.Equal(t, 1, compareCanonicalSequence("10", "9"))
	require.Equal(t, 0, compareCanonicalSequence("42", "42"))
	require.Equal(t, -1, compareCanonicalSequence(
		"123456789012345678901234567890", "123456789012345678901234567891"))
}
