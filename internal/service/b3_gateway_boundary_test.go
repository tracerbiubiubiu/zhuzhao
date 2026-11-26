package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
)

func TestB3ActionsRequireDurableAdmission(t *testing.T) {
	ctx := context.Background()
	var p *TaskActionPolicy
	for _, action := range []string{"ticket.evidence.report.generate", "activelist.document.update", "activelist.document.get"} {
		t.Run(action, func(t *testing.T) {
			require.ErrorIs(t, p.authorizeAction(ctx, action, nil), errcode.ErrForbidden)
			require.ErrorIs(t, p.authorizeDefinition(ctx, taskDefinition{ActionID: action}), errcode.ErrForbidden)
			_, err := p.AuthorizeCallback(ctx, "test-task", action, nil)
			require.ErrorIs(t, err, errcode.ErrForbidden)
		})
	}
}
