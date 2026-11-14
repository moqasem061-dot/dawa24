package ui

import (
	"context"
	"net/http"
	"strings"

	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
	"github.com/moqasem061-dot/dawa24/internal/platform/erpgate"
	"github.com/moqasem061-dot/dawa24/internal/platform/features"
	"github.com/moqasem061-dot/dawa24/internal/shared/i18n"
	"github.com/moqasem061-dot/dawa24/internal/ui/pages"
)

// setERPFlag stores the ERP runtime switch. It is a variable so a test can
// observe the call without a database; production uses the feature-flag
// engine, which writes the flag and its audit entry in one transaction.
var setERPFlag = func(ctx context.Context, enabled bool, actorID int64) error {
	return features.Set(ctx, features.KeyERPEnabled, enabled, actorID)
}

// erpSwitchState is what the settings card renders.
func erpSwitchState(ctx context.Context) pages.AdminERPSwitch {
	return pages.AdminERPSwitch{Deployed: erpgate.DeploymentEnabled(), Enabled: erpgate.Enabled(ctx)}
}

// AdminERPIntegrationSubmit turns the ERP integration on or off at runtime
// (POST /admin/settings/erp-integration). The route sits in the settings
// update group, so it needs the same permission as every other platform
// setting. The change is stored with its audit entry and takes effect in this
// process at once; other server instances follow on their next flag reload.
func (h *UIHandler) AdminERPIntegrationSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	const back = "/admin/settings?tab=features"

	actor, ok := authctx.From(ctx)
	if !ok || !actor.IsStaff {
		http.NotFound(w, r)
		return
	}
	if !erpgate.DeploymentEnabled() {
		h.redirectWithNotice(w, r, back, "error", i18n.T(lang, "erp.switch.deployment_off"))
		return
	}
	var enabled bool
	switch strings.TrimSpace(r.PostFormValue("enabled")) {
	case "true":
		enabled = true
	case "false":
		enabled = false
	default:
		h.redirectWithNotice(w, r, back, "error", i18n.T(lang, "erp.switch.failed"))
		return
	}
	if err := setERPFlag(ctx, enabled, actor.UserID); err != nil {
		h.log.ErrorContext(ctx, "erp runtime switch: save failed", "error", err, "enabled", enabled)
		h.redirectWithNotice(w, r, back, "error", i18n.T(lang, "erp.switch.failed"))
		return
	}
	h.log.InfoContext(ctx, "erp runtime switch changed", "enabled", enabled, "actor", actor.UserID)
	if enabled {
		h.redirectWithNotice(w, r, back, "success", i18n.T(lang, "erp.switch.saved_on"))
		return
	}
	h.redirectWithNotice(w, r, back, "success", i18n.T(lang, "erp.switch.saved_off"))
}
