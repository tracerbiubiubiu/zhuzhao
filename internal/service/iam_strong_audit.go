package service

import (
	"crypto/sha256"
	"encoding/hex"
)

// ConfigureIAMAuditDelivery selects the collector once during application wiring.
func ConfigureIAMAuditDelivery(users *UserService, roles *RBACService, endpoint string) {
	key := collectorKey(endpoint)
	users.auditCollector, roles.auditCollector = key, key
}

func collectorKey(endpoint string) string {
	if endpoint == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(endpoint))
	return hex.EncodeToString(sum[:])
}
