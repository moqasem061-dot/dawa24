package authctx

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"

	"github.com/muhiya/dawa24-store/internal/platform/httpx"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

func isJSONOrAJAXRequest(r *http.Request) bool {
	return strings.Contains(r.Header.Get("Accept"), "application/json") || r.Header.Get("X-Requested-With") == "XMLHttpRequest"
}

// redirectUnauthorized redirects an authenticated caller to their permitted home dashboard
// with an authorization notice instead of showing a blank 404 page.
func redirectUnauthorized(w http.ResponseWriter, r *http.Request, actor Actor) {
	if isJSONOrAJAXRequest(r) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": false,
			"error":   "ليس لديك الصلاحية الكافية لتنفيذ هذه العملية.",
		})
		return
	}

	target := "/"
	if actor.IsCustomer() {
		target = "/customer/dashboard"
	} else if actor.IsVendor() {
		if actor.Can("vendor.dashboard.view") {
			target = "/vendor/dashboard"
		} else if actor.Can("vendor.delivery.view") {
			target = "/vendor/delivery"
		} else {
			target = "/vendor/dashboard"
		}
	} else if actor.IsStaff {
		target = "/admin/dashboard"
	}

	if r.URL.Path == target {
		target = "/"
	}

	dest := target + "?auth_notice=forbidden"

	http.SetCookie(w, &http.Cookie{
		Name:     "auth_flash",
		Value:    "forbidden",
		Path:     "/",
		MaxAge:   15,
		HttpOnly: false,
		SameSite: http.SameSiteLaxMode,
	})

	if r.Header.Get("HX-Request") == "true" {
		currentURL := r.Header.Get("HX-Current-URL")
		if currentURL != "" {
			if parsed, err := url.Parse(currentURL); err == nil && (parsed.Path == target || parsed.Path == target+"/") {
				w.WriteHeader(http.StatusForbidden)
				return
			}
		}
		if r.Header.Get("HX-Boosted") != "true" && r.Method == http.MethodGet {
			targetElem := r.Header.Get("HX-Target")
			if targetElem != "" && targetElem != "main-content" && targetElem != "body" {
				w.WriteHeader(http.StatusForbidden)
				return
			}
		}

		http.SetCookie(w, &http.Cookie{
			Name:     "auth_flash",
			Value:    "forbidden",
			Path:     "/",
			MaxAge:   15,
			HttpOnly: false,
			SameSite: http.SameSiteLaxMode,
		})
		w.Header().Set("HX-Redirect", dest)
		w.WriteHeader(http.StatusNoContent)
		return
	}

	http.Redirect(w, r, dest, http.StatusSeeOther)
}

// redirectToLogin sends the caller to sign in, preserving where they were
// heading so a successful login lands back on the page they wanted.
func redirectToLogin(w http.ResponseWriter, r *http.Request) {
	if isJSONOrAJAXRequest(r) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": false,
			"error":   "يرجى تسجيل الدخول أولاً للمتابعة.",
		})
		return
	}

	q := url.Values{}
	q.Set("redirect", r.URL.Path)
	http.Redirect(w, r, "/auth/login?"+q.Encode(), http.StatusSeeOther)
}

// notFound renders a minimal 404 via httpx unified error page.
// It deliberately does not say whether the page exists under another audience.
func notFound(w http.ResponseWriter, r *http.Request) {
	httpx.Error(w, r, nil, apperr.New(apperr.KindNotFound, "route.not_found", "الصفحة غير موجودة."))
}
