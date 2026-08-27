package ticket

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
)

// TestFromTicketType_DiverseTransitions 验证不同工单类型的 transitions JSONB
// 会生成行为完全不同的状态机——证明"流程差异化=DB配置，无需改代码"的设计。
func TestFromTicketType_DiverseTransitions(t *testing.T) {
	cases := []struct {
		name     string
		typeCode string
		// 自定义 transitions（与 seed 一致但纯内存，不依赖 DB）
		transitions string
		// 合法转换路径，逐条 AssertTransition 成功
		legals [][2]string
		// 非法转换路径，逐条 return 90002
		illegals [][2]string
	}{
		{
			name:     "incident-故障: rejected可回in_progress + pending可reassigned",
			typeCode: "incident",
			transitions: `{
				"open":["assigned","closed"],
				"assigned":["in_progress","open"],
				"in_progress":["pending_verify","rejected","closed"],
				"pending_verify":["closed","reassigned"],
				"reassigned":["assigned","open"],
				"rejected":["open","in_progress"],
				"closed":["open"]
			}`,
			legals: [][2]string{
				{"open", "assigned"},
				{"open", "closed"},
				{"assigned", "in_progress"},
				{"assigned", "open"},
				{"in_progress", "pending_verify"},
				{"in_progress", "rejected"},
				{"in_progress", "closed"},
				{"pending_verify", "closed"},
				{"pending_verify", "reassigned"}, // incident 独有
				{"reassigned", "assigned"},
				{"reassigned", "open"},
				{"rejected", "in_progress"}, // incident 独有（可回返工单）
				{"rejected", "open"},
				{"closed", "open"},
			},
			illegals: [][2]string{
				{"open", "in_progress"},
				{"assigned", "closed"},
				{"closed", "assigned"},
				{"rejected", "reassigned"},
				{"pending_verify", "open"},
			},
		},
		{
			name:     "request-请求: rejected不存在 + reassigned只能回assigned",
			typeCode: "request",
			transitions: `{
				"open":["assigned","closed"],
				"assigned":["in_progress","open"],
				"in_progress":["pending_verify","closed"],
				"pending_verify":["closed","reassigned"],
				"reassigned":["assigned"],
				"closed":["open"]
			}`,
			legals: [][2]string{
				{"open", "assigned"},
				{"open", "closed"},
				{"assigned", "in_progress"},
				{"in_progress", "pending_verify"},
				{"in_progress", "closed"},
				{"pending_verify", "reassigned"},
				{"reassigned", "assigned"},
				{"closed", "open"},
			},
			illegals: [][2]string{
				{"in_progress", "rejected"},       // request 没有 rejected 态
				{"rejected", "in_progress"},       // request 没有 rejected
				{"reassigned", "open"},            // request 的 reassigned 只能回 assigned
				{"pending_verify", "in_progress"}, // DDL DEFAULT 有，但 request seed 没有
				{"assigned", "closed"},
			},
		},
		{
			name:     "change-变更工单: 全新 7 状态，无 closed→open",
			typeCode: "change",
			transitions: `{
				"draft":["reviewing"],
				"reviewing":["approved","rejected","draft"],
				"approved":["implementing"],
				"implementing":["completed","rolled_back"],
				"rolled_back":["reviewing"],
				"rejected":["draft"],
				"completed":[]
			}`,
			legals: [][2]string{
				{"draft", "reviewing"},
				{"reviewing", "approved"},
				{"reviewing", "rejected"},
				{"reviewing", "draft"},
				{"approved", "implementing"},
				{"implementing", "completed"},
				{"implementing", "rolled_back"},
				{"rolled_back", "reviewing"},
				{"rejected", "draft"},
			},
			illegals: [][2]string{
				{"draft", "approved"},
				{"reviewing", "implementing"},
				{"completed", "draft"}, // completed 是终态
				{"approved", "rolled_back"},
				{"rejected", "reviewing"}, // rejected 必须先回 draft
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tt := &model.TicketType{
				Code:        tc.typeCode,
				Transitions: json.RawMessage(tc.transitions),
			}
			sm, err := FromTicketType(tt)
			require.NoError(t, err, "FromTicketType 解析失败：%s", tc.name)

			for _, pair := range tc.legals {
				from, to := pair[0], pair[1]
				err := sm.AssertTransition(from, to)
				assert.NoErrorf(t, err, "%s 合法转换 %s→%s 应放行，但 got err=%v", tc.name, from, to, err)
			}
			for _, pair := range tc.illegals {
				from, to := pair[0], pair[1]
				err := sm.AssertTransition(from, to)
				require.Errorf(t, err, "%s 非法转换 %s→%s 应返回 90002，但成功了", tc.name, from, to)
				var ee *errcode.Error
				ok := errors.As(err, &ee)
				assert.Truef(t, ok, "%s 非法转换 %s→%s err 应是 errcode.Error：%T %v", tc.name, from, to, err, err)
				if ok {
					assert.Equalf(t, errcode.ErrTicketInvalidTransition.Code, ee.Code,
						"%s 非法转换 %s→%s 错误码应为 90002：%v", tc.name, from, to, ee)
				}
			}
		})
	}
}

// TestFromTicketType_InvalidJSON_Malformed 验证 transitions JSON 非法时返回错误
func TestFromTicketType_InvalidJSON_Malformed(t *testing.T) {
	_, err := FromTicketType(&model.TicketType{
		Code:        "bad",
		Transitions: json.RawMessage(`{broken json!!}`),
	})
	require.Error(t, err, "非法 JSON 必须报错")
}

// TestFromTicketType_UnknownSourceState 验证 transitions 里没有源状态 → 90002
func TestFromTicketType_UnknownSourceState(t *testing.T) {
	sm, err := FromTicketType(&model.TicketType{
		Code:        "x",
		Transitions: json.RawMessage(`{"open":["closed"]}`),
	})
	require.NoError(t, err)

	err = sm.AssertTransition("never_heard_of_it", "closed")
	require.Error(t, err)
	var ee *errcode.Error
	require.True(t, errors.As(err, &ee))
	assert.Equal(t, errcode.ErrTicketInvalidTransition.Code, ee.Code)
}
