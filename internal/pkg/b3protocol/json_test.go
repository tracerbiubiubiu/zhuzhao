package b3protocol

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestB3WireJSONRejectsAmbiguousFields(t *testing.T) {
	for _, raw := range []string{
		`{"operation_id":"first","operation_id":"second"}`,
		`{"Operation_id":"case alias"}`,
		`{"operation_id":"\ud800"}`,
		`{"operation_id":"first"}{"operation_id":"second"}`,
	} {
		var params TaskParams
		require.Error(t, DecodeBytes([]byte(raw), &params), raw)
	}
	var params TaskParams
	require.NoError(t, DecodeBytes([]byte(`{"operation_id":"\ud83d\udca1"}`), &params))
	require.Equal(t, "💡", params.OperationID)
}
