package repository

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDictDeleteInvalidConditionsDoNotQuery(t *testing.T) {
	r := &DictRepo{}
	for _, targets := range [][]DictTypeDeleteTarget{
		{{ID: 0, Version: 1}}, {{ID: 1, Version: 0}}, {{ID: -1, Version: 1}},
		{{ID: 1, Version: -1}}, {{ID: 1, Version: 1}, {ID: 1, Version: 1}},
	} {
		require.ErrorIs(t, r.DeleteTypeByCode(context.Background(), "synthetic", targets...), ErrDictDeleteInvalid)
	}
	for _, versions := range [][]int64{{0}, {-1}, {1, 2}} {
		require.ErrorIs(t, r.DeleteItemByID(context.Background(), 1, versions...), ErrDictDeleteInvalid)
	}
}
