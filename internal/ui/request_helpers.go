package ui

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/errtrack"
	"github.com/muhiya/dawa24-store/internal/platform/httpx"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/filesecurity"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/pagination"
	"github.com/muhiya/dawa24-store/internal/ui/components"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// SiteSettingsMiddleware injects live SiteSettings from database into every request context.
func (h *UIHandler) SiteSettingsMiddleware(next http.Handler) http.Handler {
	return h.siteSettingsMiddleware(next)
}

func (h *UIHandler) renderError(w http.ResponseWriter, r *http.Request, err error) {
	if err == nil {
		err = errors.New("internal server error")
	}
	ctx := r.Context()
	// Client cancellations (user navigated away, refreshed, closed tab) are normal HTTP lifecycle events.
	if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) || strings.Contains(strings.ToLower(err.Error()), "context canceled") {
		h.log.DebugContext(ctx, "client canceled request before render completed", "path", r.URL.Path)
		return
	}

	h.log.ErrorContext(ctx, "ui error rendering page", "error", err, "path", r.URL.Path)

	// Most of what a user actually hits arrives here rather than through
	// httpx.Error: this is the error page for every server-rendered screen.
	// Only genuine faults are recorded -- a validation refusal or a missing
	// record is the system working, and recording those would bury the faults.
	if status := statusForError(err); status >= 500 {
		errtrack.ReportRequest(ctx, r, err, errtrack.LevelError, status)
	}

	lang, dir := h.localeAndDir(r)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	// err.Error() renders the internal form - "conflict [order.already_confirmed]:
	// ... (detail)" for an apperr, and for anything else the raw driver text,
	// which names tables, columns and constraints. apperr.Msg is documented as
	// user-safe and LocalizedMsg gives the Arabic wording by code; anything that
	// is not an apperr gets a generic message and lives only in the log above.
	msg := h.safeMessage(err, lang)

	if h.isHTMX(r) {
		// 200 on purpose: HTMX swaps the error state into the target region. A
		// non-2xx would leave the old content in place with nothing explaining why.
		w.WriteHeader(http.StatusOK)
		if rerr := components.ErrorState(components.ErrorStateProps{
			Title:      i18n.T(lang, "errors.data_load_failed"),
			Message:    msg,
			RetryURL:   r.URL.String(),
			RetryLabel: i18n.T(lang, "common.retry"),
		}).Render(ctx, w); rerr != nil {
			h.log.ErrorContext(ctx, "render error state", "error", rerr)
		}
		return
	}

	w.WriteHeader(statusForError(err))
	if rerr := pages.ErrorPage(
		i18n.T(lang, "errors.generic_title"),
		msg,
		"/",
		lang,
		dir,
	).Render(ctx, w); rerr != nil {
		h.log.ErrorContext(ctx, "render error page", "error", rerr)
	}
}

// safeMessage returns wording that may be shown to a user.
func (h *UIHandler) safeMessage(err error, lang string) string {
	if err == nil {
		return ""
	}
	if strings.Contains(err.Error(), filesecurity.SecurityErrorMessage) {
		return filesecurity.SecurityErrorMessage
	}
	if appErr, ok := apperr.As(err); ok {
		if strings.Contains(appErr.Msg, filesecurity.SecurityErrorMessage) {
			return filesecurity.SecurityErrorMessage
		}
		return appErr.LocalizedMsg(lang)
	}
	errStr := err.Error()
	if strings.Contains(errStr, "email") && (strings.Contains(errStr, "unique") || strings.Contains(errStr, "duplicate key") || strings.Contains(errStr, "23505") || strings.Contains(errStr, "users_email_key")) {
		return i18n.T(lang, "validation.email_already_registered")
	}
	if strings.Contains(errStr, "commercial_register") && (strings.Contains(errStr, "unique") || strings.Contains(errStr, "duplicate key") || strings.Contains(errStr, "23505")) {
		return i18n.T(lang, "validation.cr_already_registered")
	}
	if strings.Contains(errStr, "city_id") || strings.Contains(errStr, "branches_city_id_fkey") {
		return i18n.T(lang, "validation.city_invalid")
	}
	if strings.Contains(errStr, "order_shipments_organization_id_fkey") || strings.Contains(errStr, "order_lines_organization_id_fkey") {
		return i18n.T(lang, "validation.supplier_missing_cart")
	}
	if strings.Contains(errStr, "orders_branch_id_fkey") || strings.Contains(errStr, "order_shipments_branch_id_fkey") {
		return i18n.T(lang, "validation.pharmacy_branch_invalid")
	}
	if strings.Contains(errStr, "orders_vendor_branch_id_fkey") {
		return i18n.T(lang, "validation.vendor_branch_invalid")
	}
	if strings.Contains(errStr, "foreign key") || strings.Contains(errStr, "23503") {
		return fmt.Sprintf(i18n.T(lang, "errors.foreign_key_mismatch_format"), errStr)
	}
	return fmt.Sprintf(i18n.T(lang, "errors.processing_error_format"), errStr)
}

// statusForError maps an error onto a response code. A full page load that
// returns 200 for a failure is invisible to uptime checks and to the browser.
func statusForError(err error) int {
	switch apperr.KindOf(err) {
	case apperr.KindNotFound:
		return http.StatusNotFound
	case apperr.KindValidation:
		return http.StatusBadRequest
	case apperr.KindUnauthorized:
		return http.StatusUnauthorized
	case apperr.KindForbidden:
		return http.StatusForbidden
	case apperr.KindConflict:
		return http.StatusConflict
	default:
		return http.StatusInternalServerError
	}
}

// redirectWithNotice sends the user on with a message to show when they land.
//
// Form posts here redirect after handling, which is correct — it stops a
// refresh from resubmitting. But it also throws away everything the handler
// learned, which is why a failed save was indistinguishable from a successful
// noticeFrom reads the flash a redirect left in the query string.
//
// Two spellings are in circulation: redirectWithNotice writes notice/msg,
// while several admin screens write notice_type/notice_msg. Reading both in
// one place is why a page cannot pick the wrong pair and silently render no
// banner after a save that did happen.
func noticeFrom(r *http.Request) (kind, message string) {
	q := r.URL.Query()
	kind = strings.TrimSpace(q.Get("notice_type"))
	if kind == "" {
		kind = strings.TrimSpace(q.Get("notice"))
	}
	message = strings.TrimSpace(q.Get("notice_msg"))
	if message == "" {
		message = strings.TrimSpace(q.Get("msg"))
	}
	return kind, message
}

// redirectWithNotice sends the caller on with a message to show when they land.
//
// When the target path is the page the caller is already on, the query string
// they arrived with is carried over. A list screen is a query — filters, a
// search term, a page number, a sort — and throwing that away after every
// action is why long sessions on /admin/users were unusable: approve one user
// and you are back at page 1 of an unfiltered list, scrolled to the top.
//
// Only same-path, same-origin referers are trusted, and only parameters the
// target does not already set are copied, so an explicit ?tab=x in the call
// still wins.
func (h *UIHandler) redirectWithNotice(w http.ResponseWriter, r *http.Request, path, kind, message string) {
	path = safeLocalRedirect(path, "/")
	u, err := url.Parse(path)
	if err != nil {
		if h.isHTMX(r) {
			w.Header().Set("HX-Redirect", path)
			w.WriteHeader(http.StatusOK)
			return
		}
		http.Redirect(w, r, path, http.StatusSeeOther)
		return
	}

	q := u.Query()

	if ref := r.Referer(); ref != "" {
		if refURL, err := url.Parse(ref); err == nil {
			isSameHost := refURL.Host == "" ||
				strings.EqualFold(refURL.Host, r.Host) ||
				(r.Header.Get("X-Forwarded-Host") != "" && strings.EqualFold(refURL.Host, r.Header.Get("X-Forwarded-Host")))

			cleanRefPath := strings.TrimSuffix(refURL.Path, "/")
			cleanTarget := strings.TrimSuffix(u.Path, "/")

			if isSameHost && cleanRefPath == cleanTarget && cleanRefPath != "" {
				for k, vals := range refURL.Query() {
					switch strings.ToLower(k) {
					case "notice", "msg", "notice_type", "notice_msg":
						continue
					}
					if !q.Has(k) {
						for _, v := range vals {
							q.Add(k, v)
						}
					}
				}
			}
		}
	}

	q.Set("notice", kind)
	q.Set("msg", message)
	u.RawQuery = q.Encode()
	target := u.String()

	if h.isHTMX(r) {
		w.Header().Set("HX-Redirect", target)
		w.WriteHeader(http.StatusOK)
		return
	}
	http.Redirect(w, r, target, http.StatusSeeOther)
}

// clientIP returns the trusted client IP for the request based on configured proxy hops.
func (h *UIHandler) clientIP(r *http.Request) string {
	return httpx.ClientIP(r, h.trustedProxyHops)
}

// SetLanguage persists the chosen UI language in the dawa24_lang cookie and
// returns the user to where they were. Signed-in users get the same choice
// written to their profile preference via UpdateProfile when they save settings.
func (h *UIHandler) SetLanguage(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")
	if code != "en" {
		code = "ar"
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "dawa24_lang",
		Value:    code,
		Path:     "/",
		MaxAge:   86400 * 365,
		SameSite: http.SameSiteLaxMode,
	})
	back := r.Header.Get("Referer")
	if back == "" {
		back = "/"
	}
	http.Redirect(w, r, back, http.StatusSeeOther)
}

// resolveLocale resolves the request language and text direction.
//
// Precedence: query ?lang= → dawa24_lang cookie → Accept-Language → Arabic.
// Arabic is the default and the primary language of the marketplace.
func resolveLocale(r *http.Request) (string, string) {
	if r == nil {
		return "ar", "rtl"
	}
	if lang := r.URL.Query().Get("lang"); lang != "" {
		return dirForLang(lang)
	}
	if cookie, err := r.Cookie("dawa24_lang"); err == nil && cookie.Value != "" {
		return dirForLang(cookie.Value)
	}
	if header := r.Header.Get("Accept-Language"); header != "" {
		if lang := acceptLanguage(header); lang != "" {
			return dirForLang(lang)
		}
	}
	return "ar", "rtl"
}

// langOf is the language alone, for callers that do not need the direction.
func langOf(r *http.Request) string {
	lang, _ := resolveLocale(r)
	return lang
}

// pageLimit is the rows-per-page for a list screen. It defers to
// pagination.RowsPerPage so the control offers -- and the server honours -- the
// same four sizes everywhere; this used to default to 20 and cap at 100, which
// was a fourth answer alongside three others in the handlers.
func (h *UIHandler) pageLimit(r *http.Request) int {
	return pagination.RowsPerPage(r)
}

func (h *UIHandler) pageOffset(r *http.Request) int {
	off, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	if off < 0 {
		return 0
	}
	return off
}

func (h *UIHandler) isHTMX(r *http.Request) bool {
	return r.Header.Get("HX-Request") == "true" && r.Header.Get("HX-Boosted") != "true"
}

// localeAndDir resolves the request language and text direction.
func (h *UIHandler) localeAndDir(r *http.Request) (string, string) {
	return resolveLocale(r)
}

// dirForLang returns the language and the matching text direction. Unknown
// values fall back to Arabic rather than erroring — language is a display
// preference, never a request failure.
func dirForLang(lang string) (string, string) {
	if lang == "en" {
		return "en", "ltr"
	}
	return "ar", "rtl"
}

// acceptLanguage maps an Accept-Language header onto a supported language by
// taking the first weighted entry, ignoring the rest. It is a best effort: a
// browser sending "fr-CH, fr;q=0.9" simply gets Arabic.
func acceptLanguage(header string) string {
	for _, part := range strings.Split(header, ",") {
		lang := strings.TrimSpace(strings.SplitN(part, ";", 2)[0])
		if lang == "en" || lang == "ar" {
			return lang
		}
	}
	return ""
}

// redirectToSettingsTab permanently redirects a retired settings sub-page to
// its tab or dedicated surface.
func redirectToSettingsTab(tab string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		actor, hasActor := authctx.From(ctx)

		if tab == "organization" {
			if hasActor && actor.IsVendor() {
				http.Redirect(w, r, "/vendor/organization", http.StatusMovedPermanently)
				return
			}
		}
		if tab == "wallet" || tab == "payments" {
			if hasActor && actor.UserID > 0 {
				http.Redirect(w, r, walletDestFor(actor), http.StatusSeeOther)
				return
			}
			http.Redirect(w, r, "/customer/wallet", http.StatusSeeOther)
			return
		}
		if tab == "security" || tab == "sessions" {
			if hasActor && actor.IsVendor() {
				http.Redirect(w, r, "/vendor/sessions", http.StatusMovedPermanently)
				return
			}
			http.Redirect(w, r, "/customer/sessions", http.StatusMovedPermanently)
			return
		}
		if tab == "profile" || tab == "preferences" {
			if hasActor && actor.IsVendor() {
				http.Redirect(w, r, "/vendor/dashboard", http.StatusSeeOther)
				return
			}
		}
		http.Redirect(w, r, "/settings?tab="+tab, http.StatusMovedPermanently)
	}
}
