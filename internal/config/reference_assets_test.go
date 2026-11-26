package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadRejectsUnknownReferenceAssetsSetting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte("reference_assets:\n  enabled: false\n  unregistered_key: true\n"), 0600))
	_, err := Load(path)
	require.ErrorContains(t, err, "reference_assets")
}
