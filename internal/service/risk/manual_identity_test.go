package risk

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
)

// validateManualFindingIdentity 成对契约的纯逻辑单测（离线）：
// finding_key 与 asset_ref 二者成对，各自长度/空白约束；均缺省保持后兼容。
func TestValidateManualFindingIdentity(t *testing.T) {
	cases := []struct {
		name       string
		findingKey string
		ref        *model.RiskAssetRef
		wantErr    error
	}{
		{"both absent stays compatible", "", nil, nil},
		{"pair accepted", "CVE-2026-0001", &model.RiskAssetRef{Provider: "mock-cmdb", ExternalID: "asset-01"}, nil},
		{"finding without asset rejected", "CVE-2026-0001", nil, errcode.ErrInvalidParams},
		{"asset without finding rejected", "", &model.RiskAssetRef{Provider: "mock-cmdb", ExternalID: "asset-01"}, errcode.ErrInvalidParams},
		{"blank finding rejected", "   ", &model.RiskAssetRef{Provider: "mock-cmdb", ExternalID: "asset-01"}, errcode.ErrInvalidParams},
		{"blank provider rejected", "CVE-2026-0001", &model.RiskAssetRef{Provider: " ", ExternalID: "asset-01"}, errcode.ErrInvalidParams},
		{"blank external id rejected", "CVE-2026-0001", &model.RiskAssetRef{Provider: "mock-cmdb", ExternalID: ""}, errcode.ErrInvalidParams},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateManualFindingIdentity(tc.findingKey, tc.ref)
			if tc.wantErr == nil {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, tc.wantErr)
		})
	}

	t.Run("oversize inputs rejected", func(t *testing.T) {
		long := string(make([]rune, 300))
		require.ErrorIs(t, validateManualFindingIdentity(long, &model.RiskAssetRef{Provider: "p", ExternalID: "a"}), errcode.ErrInvalidParams)
		overProvider := string(make([]rune, 150))
		require.ErrorIs(t, validateManualFindingIdentity("k", &model.RiskAssetRef{Provider: overProvider, ExternalID: "a"}), errcode.ErrInvalidParams)
		overExternal := string(make([]rune, 300))
		require.ErrorIs(t, validateManualFindingIdentity("k", &model.RiskAssetRef{Provider: "p", ExternalID: overExternal}), errcode.ErrInvalidParams)
		// 边界值本身合法（列宽 200/100/200）。
		okFinding := string(make([]rune, 200))
		okProvider := string(make([]rune, 100))
		okExternal := string(make([]rune, 200))
		require.NoError(t, validateManualFindingIdentity(okFinding, &model.RiskAssetRef{Provider: okProvider, ExternalID: okExternal}))
	})
}
