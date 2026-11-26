package service

import (
	"github.com/tracerbiubiubiu/zhuzhao/internal/middleware"
)

// ConfigureIAMAuditDelivery selects the collector once during application wiring.
func ConfigureIAMAuditDelivery(users *UserService, roles *RBACService, key string) {
	users.auditCollector, roles.auditCollector = key, key
	users.localPolicyAvailable = func() bool { return middleware.PolicyProjectionAvailable(roles.enforcer) }
}
