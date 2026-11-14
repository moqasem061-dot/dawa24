// Package erpgate answers one question for the whole process: is the ERP
// integration switched on right now?
//
// Two switches decide it. The deployment switch (ERP_API_ENABLED) is read once
// at boot and says whether the integration exists on this deployment at all.
// The runtime switch is the platform feature flag "erp.enabled", which an admin
// flips from the settings page without a restart. The integration is on only
// when both are.
package erpgate

import (
	"context"
	"net/http"
	"sync/atomic"

	"github.com/moqasem061-dot/dawa24/internal/platform/features"
)

var deployed atomic.Bool

func init() { deployed.Store(true) }

// SetDeployment records the deployment switch. The composition root calls it
// once, at boot, with the value of ERP_API_ENABLED.
func SetDeployment(on bool) { deployed.Store(on) }

// DeploymentEnabled reports the deployment switch alone.
func DeploymentEnabled() bool { return deployed.Load() }

// Enabled reports whether the integration is on: deployed and not switched off
// by an admin. An unknown flag counts as on, so a deployment that has not yet
// run the migration behaves as it did before the switch existed.
func Enabled(ctx context.Context) bool {
	return deployed.Load() && features.Enabled(ctx, features.KeyERPEnabled)
}

// RequireEnabled answers 404 while the integration is off, as if the routes it
// guards did not exist. It evaluates the switch per request, so flipping it
// takes effect on the next request.
func RequireEnabled(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !Enabled(r.Context()) {
			http.NotFound(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}
