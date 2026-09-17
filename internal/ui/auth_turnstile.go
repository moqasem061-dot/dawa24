package ui

import (
	"context"
	"net/http"
	"time"
)

// verifyTurnstile verifies the Cloudflare Turnstile token from the form submission.
// If turnstile is not enabled or keys are omitted (e.g. dev/test), it safely passes.
func (h *UIHandler) verifyTurnstile(r *http.Request, action string) error {
	if h.turnstile == nil || !h.turnstile.Enabled() {
		return nil
	}

	token := r.PostFormValue("cf-turnstile-response")
	clientIP := h.clientIP(r)
	host := r.Host

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	return h.turnstile.Verify(ctx, token, clientIP, action, host)
}

// turnstileSiteKey returns the public site key if Turnstile is configured and enabled.
func (h *UIHandler) turnstileSiteKey() string {
	if h.turnstile == nil || !h.turnstile.Enabled() {
		return ""
	}
	return h.turnstile.SiteKey()
}
