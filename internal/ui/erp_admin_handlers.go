package ui

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
	"github.com/moqasem061-dot/dawa24/internal/shared/i18n"
	"github.com/moqasem061-dot/dawa24/internal/ui/pages"
)

// Platform-staff handlers for the integration keys (S1). The staff page
// crosses tenants on purpose: every organization's keys, with the AsStaff
// repository paths behind it (plan named exception E-family). Any other staff
// view stays inside its tenant scope.

// ErpAdminKeysPage lists every organization's keys for platform staff.
func (h *UIHandler) ErpAdminKeysPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := resolveLocale(r)
	if _, ok := authctx.From(ctx); !ok {
		erpLoginRedirect(w, r)
		return
	}
	data := pages.ErpAdminKeysPageData{}
	data.NoticeKind, data.NoticeMessage = noticeFrom(r)
	if h.erpSvc != nil {
		rows, err := h.erpSvc.ListKeysAsStaff(ctx)
		if err != nil {
			h.renderError(w, r, err)
			return
		}
		data.Rows = rows
	}
	h.renderPage(ctx, w, "render erp admin keys page", pages.ErpAdminKeysPage(data, lang, dir))
}

// ErpAdminKeyRevoke revokes any organization's key on staff request. The
// route gate carries platform.integration.revoke; the handler refuses any
// actor the middleware did not let through as staff.
func (h *UIHandler) ErpAdminKeyRevoke(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, _ := resolveLocale(r)
	back := "/admin/integrations"
	actor, ok := authctx.From(ctx)
	if !ok {
		erpLoginRedirect(w, r)
		return
	}
	if !actor.IsStaff {
		// Never confirm to a non-staff caller that the key exists.
		h.redirectWithNotice(w, r, back, "error", i18n.T(lang, "erp.err.not_found"))
		return
	}
	if h.erpSvc == nil {
		h.redirectWithNotice(w, r, back, "error", i18n.T(lang, "common.service_unavailable"))
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id < 1 {
		h.redirectWithNotice(w, r, back, "error", i18n.T(lang, "erp.err.not_found"))
		return
	}
	reason, _ := parseRevokeForm(r)
	if err := h.erpSvc.RevokeKeyAsStaff(ctx, id, reason, actor.UserID); err != nil {
		h.redirectWithNotice(w, r, back, "error", h.erpMessage(err, lang))
		return
	}
	h.redirectWithNotice(w, r, back, "success", i18n.T(lang, "erp.keys.revoked_done"))
}
