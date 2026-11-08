package ui

import (
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/moqasem061-dot/dawa24/internal/modules/erp"
	"github.com/moqasem061-dot/dawa24/internal/modules/erp/provider"
	"github.com/moqasem061-dot/dawa24/internal/shared/i18n"
)

// formError names one form field that failed validation and the message to
// show the user. Field follows the field spelling the ERP service uses in its
// ERP_VALIDATION details (for example "warehouseId", "ratePerMinute").
type formError struct {
	Field   string
	Message string
}

// parseCreateKeyForm reads the new-key form into the service input.
//
// Ownership is never read here: ActorIsOwner is set by the handler from the
// signed-in actor (authctx.Actor.IsOwner), never from a submitted field, and
// so are OrganizationID and ActorUserID. Empty optional fields fall back to
// the documented defaults (provider, scopes, rate limit).
func parseCreateKeyForm(r *http.Request, vendor bool) (erp.CreateKeyInput, []formError) {
	lang := langOf(r)
	_ = r.ParseForm()
	// i18n keys carry printf verbs in their catalog text; the alias keeps
	// calls to it away from the printf analyzer, which can only see the
	// English-less Go literal (same idiom as internal/modules/erp/http).
	translate := i18n.T
	var in erp.CreateKeyInput
	var errs []formError
	add := func(field, message string) {
		errs = append(errs, formError{Field: field, Message: message})
	}

	in.Name = strings.TrimSpace(r.Form.Get("name"))
	if n := utf8.RuneCountInString(in.Name); n < 1 || n > 80 {
		add("name", translate(lang, "erp.keys.name_length"))
	}

	in.Provider = strings.TrimSpace(r.Form.Get("provider"))
	switch {
	case in.Provider == "":
		in.Provider = provider.DefaultProvider
	default:
		known := false
		for _, name := range provider.Names() {
			if name == in.Provider {
				known = true
				break
			}
		}
		if !known {
			add("provider", translate(lang, "erp.keys.provider_invalid"))
		}
	}

	if id, ok := parseID(r.Form.Get("warehouse_id")); ok {
		in.WarehouseID = id
	} else if r.Form.Get("warehouse_id") != "" {
		add("warehouseId", translate(lang, "erp.keys.warehouse_invalid"))
	}
	if id, ok := parseID(r.Form.Get("branch_id")); ok {
		in.BranchID = id
	} else if r.Form.Get("branch_id") != "" {
		add("branchId", translate(lang, "erp.keys.branch_invalid"))
	}
	if id, ok := parseID(r.Form.Get("buying_user_id")); ok {
		in.BuyingUserID = &id
	} else if r.Form.Get("buying_user_id") != "" {
		add("buyingUserId", translate(lang, "erp.keys.buying_user_invalid"))
	}

	scopes := formScopes(r)
	switch {
	case len(scopes) == 0:
		in.Scopes = erp.DefaultScopes(vendor)
	default:
		for _, scope := range scopes {
			if !erp.ValidScope(scope) {
				add("scopes", translate(lang, "erp.keys.scope_invalid"))
				break
			}
		}
		in.Scopes = scopes
	}

	in.RateLimitPerMinute = erp.DefaultRatePerMinute
	if raw := strings.TrimSpace(r.Form.Get("rate_limit")); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < erp.MinRatePerMinute || n > erp.MaxRatePerMinute {
			add("ratePerMinute", translate(lang, "erp.keys.rate_range", erp.MinRatePerMinute, erp.MaxRatePerMinute))
		} else {
			in.RateLimitPerMinute = n
		}
	}
	return in, errs
}

// parseRotateForm reads the rotation grace in hours; empty means the default.
func parseRotateForm(r *http.Request) (time.Duration, []formError) {
	translate := i18n.T
	lang := langOf(r)
	_ = r.ParseForm()
	raw := strings.TrimSpace(r.Form.Get("grace_hours"))
	if raw == "" {
		return erp.RotateGraceDefault, nil
	}
	hours, err := strconv.Atoi(raw)
	grace := time.Duration(hours) * time.Hour
	if err != nil || grace < 0 || grace > erp.RotateGraceMax {
		return erp.RotateGraceDefault, []formError{{
			Field:   "graceHours",
			Message: translate(lang, "erp.keys.grace_range", int(erp.RotateGraceMax.Hours())),
		}}
	}
	return grace, nil
}

// parseRevokeForm reads the optional revocation reason.
func parseRevokeForm(r *http.Request) (string, []formError) {
	_ = r.ParseForm()
	return strings.TrimSpace(r.Form.Get("reason")), nil
}

// parseLimitsForm reads the per-minute request limit; it is required and must
// sit inside the documented range.
func parseLimitsForm(r *http.Request) (int, []formError) {
	translate := i18n.T
	lang := langOf(r)
	_ = r.ParseForm()
	raw := strings.TrimSpace(r.Form.Get("rate_limit"))
	n, err := strconv.Atoi(raw)
	if err != nil || n < erp.MinRatePerMinute || n > erp.MaxRatePerMinute {
		return 0, []formError{{
			Field:   "ratePerMinute",
			Message: translate(lang, "erp.keys.rate_range", erp.MinRatePerMinute, erp.MaxRatePerMinute),
		}}
	}
	return n, nil
}

// parseID reads a positive integer form field.
func parseID(raw string) (int64, bool) {
	if strings.TrimSpace(raw) == "" {
		return 0, false
	}
	id, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || id < 1 {
		return 0, false
	}
	return id, true
}

// formScopes collects every selected scope, whether the form sent them as
// repeated fields or as one comma-separated value.
func formScopes(r *http.Request) []string {
	var out []string
	for _, raw := range r.Form["scopes"] {
		for _, part := range strings.Split(raw, ",") {
			if scope := strings.TrimSpace(part); scope != "" {
				out = append(out, scope)
			}
		}
	}
	return out
}
