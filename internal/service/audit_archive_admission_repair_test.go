package service

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tracerbiubiubiu/zhuzhao/internal/config"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/jobs"
)

func TestAuditArchiveAdmissionRepair_DefaultParamsDoNotGrantMaintenance(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // An old unguarded Handle reaches the execution boundary and returns cancellation.
	for name, entry := range map[string]context.Context{
		"bare":       ctx,
		"actor text": WithTaskPrincipal(ctx, 42, "system"),
	} {
		for _, raw := range []json.RawMessage{nil, json.RawMessage(`{}`), json.RawMessage(`{"retention_days":180}`)} {
			t.Run(name+string(raw), func(t *testing.T) {
				job := NewAuditArchiveJob(nil, 180, 5000, t.TempDir(), 395, slog.New(slog.NewTextHandler(io.Discard, nil)))
				require.ErrorIs(t, job.Handle(entry, raw), jobs.ErrAbort, "default parameters and a system actor label cannot manufacture execution admission")
			})
		}
	}
}

func TestAuditArchiveAdmissionRepair_FixedBackgroundDefinitionAndParams(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	cfg := config.ArchiveConfig{CronEnabled: true, CronTick: "30s", RetentionDays: 180, BatchRows: 5000, OutDir: t.TempDir(), FileRetentionDays: 395}
	disabled := cfg
	disabled.CronEnabled = false
	_, err := NewFixedAuditArchiveJob(nil, disabled, logger)
	require.Error(t, err, "only an explicitly enabled reviewed startup definition creates background authority")
	invalid := cfg
	invalid.CronTick = "broken"
	_, err = NewFixedAuditArchiveJob(nil, invalid, logger)
	require.Error(t, err)
	job, err := NewFixedAuditArchiveJob(nil, cfg, logger)
	require.NoError(t, err)
	// Subsequent mutation of the caller's config cannot alter the fixed capability.
	cfg.RetentionDays = 1
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, raw := range []json.RawMessage{nil, json.RawMessage(`{}`), json.RawMessage(`{"retention_days":180}`)} {
		require.ErrorIs(t, job.Handle(ctx, raw), context.Canceled, "approved fixed execution reaches its effect boundary with the original definition")
	}
	for _, raw := range []json.RawMessage{
		json.RawMessage(`{"retention_days":1}`),
		json.RawMessage(`{"approval_id":"claimed-by-caller"}`),
		json.RawMessage(`{"actor":"system"}`),
		json.RawMessage(`{"retention_days":180,"RETENTION_DAYS":1}`),
	} {
		require.ErrorIs(t, job.Handle(ctx, raw), jobs.ErrAbort, "a fixed backend capability cannot consume caller-defined retention, approval, or actor authority")
	}
}
