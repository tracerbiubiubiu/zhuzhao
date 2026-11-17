package config

import (
	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"
	"testing"
)

func TestMaintenanceEnvReplacesStartupGrantsAndEmptyRevokes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte("taskrunner:\n  maintenance:\n    maintainer_user_ids: [7]\n"), 0600))
	t.Setenv("TASK_MAINTAINER_USER_IDS", "42, 43")
	cfg, err := Load(path)
	require.NoError(t, err)
	require.Equal(t, []int64{42, 43}, cfg.Taskrunner.Maintenance.MaintainerUserIDs)
	t.Setenv("TASK_MAINTAINER_USER_IDS", "")
	cfg, err = Load(path)
	require.NoError(t, err)
	require.Empty(t, cfg.Taskrunner.Maintenance.MaintainerUserIDs)
	t.Setenv("TASK_MAINTAINER_USER_IDS", "admin")
	_, err = Load(path)
	require.Error(t, err)
}
