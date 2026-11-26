package directorymock

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReadManagerResolutionRestrictsSyntheticReferences(t *testing.T) {
	const input = `{"provider_ref":"mock-directory","scope_ref":"synthetic","operation_id":"resolve-1",
		"employee_external_id":"employee-1","identity_generation":2,
		"manager_external_id":"employee-2","manager_identity_generation":1}`
	event, err := ReadManagerResolution(strings.NewReader(input))
	require.NoError(t, err)
	require.Equal(t, ManagerResolution{
		ProviderRef: "mock-directory", ScopeRef: "synthetic", OperationID: "resolve-1",
		EmployeeExternalID: "employee-1", IdentityGeneration: 2,
		ManagerExternalID: "employee-2", ManagerIdentityGeneration: 1,
	}, event)
	require.False(t, validRef(managerOperationKey(event.ScopeRef, event.OperationID).id))

	for name, badInput := range map[string]string{
		"internal key":  strings.Replace(input, "resolve-1", "manager/resolve-1", 1),
		"other source":  strings.Replace(input, "mock-directory", "hr", 1),
		"no generation": strings.Replace(input, `"manager_identity_generation":1`, `"manager_identity_generation":0`, 1),
		"local ID":      `{"manager_user_id":5,` + input[1:],
		"duplicate key": `{"manager_external_id":"other",` + input[1:],
	} {
		t.Run(name, func(t *testing.T) {
			_, err := ReadManagerResolution(strings.NewReader(badInput))
			require.ErrorIs(t, err, ErrInvalidManager)
		})
	}
}
