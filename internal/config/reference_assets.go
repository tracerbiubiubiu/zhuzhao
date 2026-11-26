package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/spf13/viper"
	"github.com/tracerbiubiubiu/zhuzhao/pkg/referencequery"
)

// ReferenceAssetsConfig registers the single synthetic read-only profile.
type ReferenceAssetsConfig struct {
	Enabled         bool          `mapstructure:"enabled"`
	ExtensionID     string        `mapstructure:"extension_id"`
	Profile         string        `mapstructure:"profile"`
	ConfigRevision  string        `mapstructure:"config_revision"`
	SourceID        string        `mapstructure:"source_id"`
	AppOrigin       string        `mapstructure:"app_origin"`
	OutboundAK      string        `mapstructure:"outbound_ak"`
	OutboundKeyFile string        `mapstructure:"outbound_key_file"`
	CallbackAK      string        `mapstructure:"callback_ak"`
	CallbackKeyFile string        `mapstructure:"callback_key_file"`
	ReaderRoles     []string      `mapstructure:"reader_roles"`
	Timeout         time.Duration `mapstructure:"timeout"`
}

func loadReferenceAssets(path string, cfg *ReferenceAssetsConfig) error {
	file := viper.New()
	file.SetConfigFile(path)
	if err := file.ReadInConfig(); err != nil {
		return fmt.Errorf("reference_assets: read config: %w", err)
	}
	if raw := file.Get("reference_assets"); raw == nil {
		return nil
	}
	sub := file.Sub("reference_assets")
	if sub == nil {
		return fmt.Errorf("reference_assets: expected an object")
	}
	if err := sub.UnmarshalExact(cfg); err != nil {
		return fmt.Errorf("reference_assets: invalid settings: %w", err)
	}
	return nil
}

var referenceIdentifier = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.:-]{0,63}$`)

const ReferenceAssetsPage = "/extensions/reference-assets"

func referenceRouteConflict(prefix string) bool {
	for _, route := range []string{ReferenceAssetsPage, referencequery.QueryPath, referencequery.AdmissionPath} {
		if prefix == route || strings.HasPrefix(route, prefix+"/") || strings.HasPrefix(prefix, route+"/") {
			return true
		}
	}
	return false
}

func (c ReferenceAssetsConfig) Validate() error {
	if (c.ExtensionID != "" && c.ExtensionID != referencequery.ExtensionID) ||
		(c.Profile != "" && c.Profile != referencequery.Profile) {
		return fmt.Errorf("reference_assets: unknown extension or profile")
	}
	roles := make(map[string]bool, len(c.ReaderRoles))
	for _, role := range c.ReaderRoles {
		if !referenceIdentifier.MatchString(role) || roles[role] {
			return fmt.Errorf("reference_assets: reader roles must be distinct identifiers")
		}
		roles[role] = true
	}
	if !c.Enabled {
		return nil
	}
	if c.ExtensionID == "" || c.Profile == "" || !referenceIdentifier.MatchString(c.ConfigRevision) ||
		!referenceIdentifier.MatchString(c.SourceID) || len(roles) == 0 || len(roles) > 32 ||
		c.Timeout <= 0 || c.Timeout > 5*time.Second || !referenceIdentifier.MatchString(c.OutboundAK) ||
		!referenceIdentifier.MatchString(c.CallbackAK) || c.OutboundAK == c.CallbackAK {
		return fmt.Errorf("reference_assets: enabled profile requires identity, revision, source, reader roles, distinct caller IDs and a bounded timeout")
	}
	return nil
}

func (c ReferenceAssetsConfig) Open() (*referencequery.Peer, []byte, error) {
	if err := c.Validate(); err != nil || !c.Enabled {
		return nil, nil, err
	}
	appKey, appErr := referenceKey(c.OutboundKeyFile)
	callbackKey, callbackErr := referenceKey(c.CallbackKeyFile)
	if appErr != nil || callbackErr != nil || appKey == callbackKey {
		return nil, nil, fmt.Errorf("reference_assets: each caller requires a distinct readable key file")
	}
	peer, err := referencequery.NewPeer(c.AppOrigin, c.OutboundAK, appKey, c.Timeout)
	if err != nil {
		return nil, nil, fmt.Errorf("reference_assets: invalid application origin or caller configuration")
	}
	return peer, []byte(callbackKey), nil
}

func referenceKey(path string) (string, error) {
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("invalid key file reference")
	}
	stat, err := os.Stat(path)
	if err != nil || !stat.Mode().IsRegular() || stat.Size() > 4096 {
		return "", fmt.Errorf("invalid key file reference")
	}
	data, err := os.ReadFile(path)
	key := strings.TrimSpace(string(data))
	if err != nil || len(key) < 32 || len(key) > 4096 {
		return "", fmt.Errorf("invalid caller key")
	}
	return key, nil
}
