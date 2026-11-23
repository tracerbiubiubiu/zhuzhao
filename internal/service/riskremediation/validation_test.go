package riskremediation

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBoundedStableEvidenceAndVersionOverflow(t *testing.T) {
	require.False(t, validEvidence(nil, true))
	require.False(t, validEvidence([]EvidenceRef{{Kind: "file", Ref: "synthetic-object", Version: ""}}, true))
	require.True(t, validEvidence([]EvidenceRef{{Kind: "reference", Ref: "synthetic-change", Version: "v1"}}, true))
	require.False(t, advance(math.MaxInt64))
	require.True(t, advance(math.MaxInt64-1))
	require.Error(t, validCommand(Command{ActorID: 1, OperationID: "K", TicketID: 1}, true))
}
