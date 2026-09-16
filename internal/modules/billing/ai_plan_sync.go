package billing

import (
	"context"
	"log/slog"
)

// Keeping a tenant's AI quota in step with what they pay for.
//
// A billing plan carries an ai_plan_id: the plan on the AI Gateway whose rate
// limits and budget window the organisation's key spends against. Creating,
// upgrading, downgrading or renewing a subscription changes which plan that is.
//
// Until this existed, nothing told the Gateway. UpdateOrganizationPlan had been
// written and had no callers anywhere in the repository, so an organisation
// kept whatever AI quota it happened to be provisioned with the first time —
// a pharmacy on the free tier that upgraded to enterprise paid enterprise money
// and went on being rate-limited as free, with nothing in any screen to explain
// why.
//
// It is a port rather than a direct call because billing must not import the
// organisation module, and the Gateway identity needs both. The composition
// root supplies the implementation.

// AIPlanSync propagates an organisation's entitlement to the AI Gateway.
type AIPlanSync interface {
	// SyncPlan moves the organisation onto the Gateway plan its current
	// subscription entitles it to.
	SyncPlan(ctx context.Context, orgID int64) error
}

// SetAIPlanSync installs the propagation port. Leaving it unset is supported:
// the Gateway is an enhancement, and a deployment without one still bills,
// subscribes and renews exactly as before.
func (s *Service) SetAIPlanSync(sync AIPlanSync) {
	s.aiPlans = sync
}

// syncAIPlan pushes an organisation's new entitlement to the Gateway.
//
// Failure is logged and swallowed on purpose. The subscription has already been
// written and is the source of truth; a Gateway that could not be reached must
// not fail a payment the user has made, and the tenant key provisioner
// re-resolves the plan on its own cache expiry regardless. What must not happen
// is failing silently — hence the log line, which names the organisation and
// the error an operator can act on.
func (s *Service) syncAIPlan(ctx context.Context, orgID *int64) {
	if s.aiPlans == nil || orgID == nil || *orgID <= 0 {
		return
	}
	if err := s.aiPlans.SyncPlan(ctx, *orgID); err != nil {
		s.log.WarnContext(ctx, "subscription changed but AI gateway plan was not updated",
			"org_id", *orgID, "error", err, slog.String("impact", "tenant keeps previous AI quota until the next resolve"))
	}
}
