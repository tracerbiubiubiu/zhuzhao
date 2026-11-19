package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
)

func TestDictDeleteInvalidConditionsDoNotReachRepository(t *testing.T) {
	s := NewDictService(nil, nil)
	for _, targets := range [][]DictTypeDeleteTarget{
		{{ID: 0, Version: 1}}, {{ID: 1, Version: 0}}, {{ID: -1, Version: 1}},
		{{ID: 1, Version: -1}}, {{ID: 1, Version: 1}, {ID: 1, Version: 1}},
	} {
		require.ErrorIs(t, s.DeleteType(context.Background(), "synthetic", targets...), errcode.ErrInvalidParams)
	}
	for _, versions := range [][]int64{{0}, {-1}, {1, 2}} {
		require.ErrorIs(t, s.DeleteItem(context.Background(), 1, versions...), errcode.ErrInvalidParams)
	}
}
