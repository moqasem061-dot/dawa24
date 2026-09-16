package ui

import (
	"context"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// Whether the vendor import may offer its AI switch.
//
// Same discipline as the smart order's: never render an enabled toggle the
// platform cannot honour. A vendor who ticks a box, waits, and gets purely
// deterministic results has been told nothing about why — and every reason
// below is one an operator can go and fix.
//
// None of the checks calls the Gateway. Rendering a settings page must not wait
// on a network round trip to the service that may itself be the thing that is
// down.
func (h *UIHandler) vendorImportAIState(ctx context.Context, lang string) (bool, string) {
	if h.ingSvc == nil || !h.ingSvc.AIAvailable() {
		return false, i18n.T(lang, "vendor.ingest.ai_not_enabled")
	}
	if h.aiClient == nil || !h.aiClient.Enabled() {
		return false, i18n.T(lang, "vendor.ingest.ai_gateway_down")
	}
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		return false, i18n.T(lang, "vendor.ingest.ai_members_only")
	}
	if h.orgSvc == nil {
		return false, i18n.T(lang, "vendor.ingest.ai_check_failed")
	}
	org, err := h.orgSvc.GetOrganization(ctx, actor.OrganizationID)
	if err != nil || org == nil {
		return false, i18n.T(lang, "vendor.ingest.ai_check_failed")
	}
	if org.AIVirtualKey == "" {
		return false, i18n.T(lang, "vendor.ingest.ai_not_activated")
	}
	return true, ""
}
