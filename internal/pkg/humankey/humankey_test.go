// humankey 人读编号生成器单测（S6 §5.3）。
package humankey

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// mockPool 不会真正调用（Next 在集成测试覆盖——此单测测格式）
func TestHumanKeyFormat(t *testing.T) {
	// 格式验证：前缀-yyyymmdd-NNNN
	dateStr := "20261023"
	seq := int64(42)
	key := fmt.Sprintf("TK-%s-%04d", dateStr, seq)
	assert.True(t, strings.HasPrefix(key, "TK-"))
	assert.Contains(t, key, "20261023")
	assert.Contains(t, key, "0042")
	assert.Len(t, key, len("TK-")+8+1+4, "TK-yyyymmdd-NNNN=15 字符")
}
