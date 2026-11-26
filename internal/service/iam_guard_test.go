package service

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tracerbiubiubiu/zhuzhao/internal/middleware"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

func TestC05IAMCredentialAccountPolicy(t *testing.T) {
	current, prior, legacy := int64(7), int64(6), int64(0)
	for _, tt := range []struct {
		name    string
		purpose middleware.IAMRequestPurpose
		status  int
		deleted bool
		want    bool
	}{
		{name: "current session", purpose: middleware.IAMRequestPurpose{SessionEpoch: &current}, status: 1, want: true},
		{name: "prior session epoch", purpose: middleware.IAMRequestPurpose{SessionEpoch: &prior}, status: 1},
		{name: "legacy session", purpose: middleware.IAMRequestPurpose{SessionEpoch: &legacy}, status: 1},
		{name: "independent PAT", purpose: middleware.IAMRequestPurpose{PATID: 42}, status: 1, want: true},
		{name: "missing facts", status: 1},
		{name: "ambiguous facts", purpose: middleware.IAMRequestPurpose{SessionEpoch: &current, PATID: 42}, status: 1},
		{name: "inactive account", purpose: middleware.IAMRequestPurpose{SessionEpoch: &current}},
		{name: "deleted account", purpose: middleware.IAMRequestPurpose{PATID: 42}, status: 1, deleted: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			account := repository.AccountSecuritySnapshot{Status: tt.status, Deleted: tt.deleted, SessionEpoch: current, ProjectionEpoch: prior}
			require.Equal(t, tt.want, currentC05AccountCredential(tt.purpose, account))
		})
	}
}
