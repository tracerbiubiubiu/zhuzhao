package directorymock

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReadPrimaryTransferAcceptsOnlySyntheticExternalReferences(t *testing.T) {
	const synthetic = `{"provider_ref":"mock-directory","scope_ref":"synthetic","operation_id":"transfer-1",
		"employee_external_id":"employee-1","identity_generation":1,
		"from_department_external_id":"dept-a","to_department_external_id":"dept-b","source_revision":2}`
	event, err := ReadPrimaryTransfer(strings.NewReader(synthetic))
	require.NoError(t, err)
	require.Equal(t, PrimaryTransfer{
		ProviderRef: "mock-directory", ScopeRef: "synthetic", OperationID: "transfer-1",
		EmployeeExternalID: "employee-1", IdentityGeneration: 1,
		FromDepartmentExternalID: "dept-a", ToDepartmentExternalID: "dept-b", SourceRevision: 2,
	}, event)

	cases := map[string]string{
		"local ID":        `{"user_id":5,` + synthetic[1:],
		"other provider":  strings.Replace(synthetic, "mock-directory", "hr", 1),
		"duplicate key":   `{"source_revision":3,` + synthetic[1:],
		"same department": strings.Replace(synthetic, "dept-b", "dept-a", 1),
		"no generation":   strings.Replace(synthetic, `"identity_generation":1`, `"identity_generation":0`, 1),
		"oversized":       strings.Repeat(" ", maxTransferBytes) + synthetic,
	}
	for name, input := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := ReadPrimaryTransfer(strings.NewReader(input))
			require.ErrorIs(t, err, ErrInvalidTransfer)
		})
	}
}
