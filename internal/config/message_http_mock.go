package config

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/channelsender"
)

type MessageHTTPMockConfig struct {
	Origin     string `mapstructure:"origin"`
	Profile    string `mapstructure:"profile"`
	AK         string `mapstructure:"ak"`
	SK         string `mapstructure:"sk"`
	CallbackAK string `mapstructure:"callback_ak"`
	CallbackSK string `mapstructure:"callback_sk"`
}

var mockProfileName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.:-]{0,127}$`)

func (c MessageHTTPMockConfig) Open() (*channelsender.HTTPMessageClient, error) {
	if c == (MessageHTTPMockConfig{}) {
		return nil, nil
	}
	if !mockProfileName.MatchString(c.Profile) || !mockProfileName.MatchString(c.AK) ||
		!mockProfileName.MatchString(c.CallbackAK) || strings.TrimSpace(c.SK) == "" ||
		strings.TrimSpace(c.CallbackSK) == "" || c.CallbackSK == c.SK {
		return nil, errors.New("notification.http_mock requires a profile and distinct outbound and callback credentials")
	}
	return channelsender.NewHTTPMessageClient(c.Origin, c.AK, c.SK, 5*time.Second)
}

// Binding identifies the destination and signing identities, allowing rotation
// of their secrets without changing the destination of an existing delivery.
func (c MessageHTTPMockConfig) Binding() string {
	wire := strings.Join([]string{strings.TrimSuffix(c.Origin, "/"), c.AK, c.CallbackAK}, "\x00")
	digest := sha256.Sum256([]byte(wire))
	return hex.EncodeToString(digest[:])
}
