package app

import (
	"os"
	"strings"
	"testing"
)

// TestG5_ActionFailedEmitterWired G-5 装配守卫（R1 教训——装配层遗漏是最隐蔽的
// 虚报形态：机制在、测试绿、生产不执行）。providers.go 必须含 actionReg.SetEmitter
// 接线——撤线即红。磁盘读守卫先例=TestCatalogPageRows_NoWriteBindings（读迁移）。
func TestG5_ActionFailedEmitterWired(t *testing.T) {
	b, err := os.ReadFile("providers.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "actionReg.SetEmitter(") {
		t.Fatal("G-5 回归：providers.go 丢失 actionReg.SetEmitter 接线——ops.action_failed 将退化为仅日志（S6 规则面/inbox 均不可达）")
	}
}
