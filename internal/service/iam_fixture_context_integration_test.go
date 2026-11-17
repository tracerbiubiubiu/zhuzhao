//go:build integration

package service_test

import (
	"context"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service"
)

// These legacy direct-service fixtures deliberately test L3/business effects.
// Real HTTP/L1 freshness cases use the production middleware in their own tests.
func trustedIAMTestContext() context.Context { return service.TrustedIAMInternal(context.Background()) }
