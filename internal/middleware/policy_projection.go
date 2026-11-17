package middleware

import (
	"sync"
	"sync/atomic"

	"github.com/casbin/casbin/v3"
)

var policyProjections sync.Map

func policyProjection(enforcer *casbin.SyncedEnforcer) *atomic.Bool {
	candidate := &atomic.Bool{}
	candidate.Store(true)
	state, _ := policyProjections.LoadOrStore(enforcer, candidate)
	return state.(*atomic.Bool)
}

// MarkPolicyProjectionUnavailable closes admission before a p-changing DB
// transaction commits. A failed/uncertain commit remains conservatively closed.
func MarkPolicyProjectionUnavailable(enforcer *casbin.SyncedEnforcer) {
	if enforcer != nil {
		policyProjection(enforcer).Store(false)
	}
}

// MarkPolicyProjectionAvailable is called only after a successful complete
// reload. A controlled restart also creates a fresh enforcer after startup load.
func MarkPolicyProjectionAvailable(enforcer *casbin.SyncedEnforcer) {
	if enforcer != nil {
		policyProjection(enforcer).Store(true)
	}
}
func PolicyProjectionAvailable(enforcer *casbin.SyncedEnforcer) bool {
	return enforcer != nil && policyProjection(enforcer).Load()
}
