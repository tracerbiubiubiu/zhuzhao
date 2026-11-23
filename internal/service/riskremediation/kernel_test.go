package riskremediation

import (
	"context"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestRequiresCallerGuardAndAdapters(t *testing.T) {
	s := New(nil, Callbacks{})
	_, err := s.FinishTx(context.Background(), nil, FinishCommand{})
	require.Error(t, err)
}
