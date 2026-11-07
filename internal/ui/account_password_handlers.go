package ui

import (
	"net/http"

	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
	"github.com/moqasem061-dot/dawa24/internal/ui/pages"
)

// AccountPasswordPage shows the password form to any signed-in member. It asks
// for no permission: a member whose account must change its password has to
// reach it whatever their role allows.
func (h *UIHandler) AccountPasswordPage(w http.ResponseWriter, r *http.Request) {
	actor, ok := authctx.From(r.Context())
	if !ok || actor.UserID == 0 {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}
	lang, dir := h.localeAndDir(r)
	kind, msg := noticeFrom(r)
	if kind != "error" {
		msg = ""
	}
	h.renderPage(r.Context(), w, "render account password page", pages.AccountPasswordPage(lang, dir, actor.IsVendor(), msg))
}
