package ticket

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStateContractRepair_ReservedStates(t *testing.T) {
	for _, states := range []string{`["new","assigned","closed"]`, `["open","closed"]`, `["open","assigned","done"]`, `["open","assigned","closed",""]`, `["open","assigned","closed","open"]`} {
		t.Run(states, func(t *testing.T) {
			require.Error(t, validateTypeGraph(json.RawMessage(states), json.RawMessage(`{}`)), states)
		})
	}
	require.NoError(t, validateTypeGraph(json.RawMessage(`["open","assigned","closed","review"]`), json.RawMessage(`{"open":["assigned"],"assigned":["review"],"review":["closed"]}`)))
}
