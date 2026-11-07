package ui

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/modules/identity"
	"github.com/moqasem061-dot/dawa24/internal/shared/i18n"
)

// S17: a driver or constraint message is never shown. The bilingual apperr
// message still is.
func TestSafeMessageHidesRawErrorText(t *testing.T) {
	h := &UIHandler{}
	raw := []string{
		`pq: relation "org.branches" does not exist`,
		`insert or update on table "order_lines" violates foreign key constraint "order_lines_variant_id_fkey" (SQLSTATE 23503)`,
		`repo: update branch: ERROR: column "city_code" of relation "org.branches" does not exist`,
	}
	for _, text := range raw {
		for _, lang := range []string{"ar", "en"} {
			msg := h.safeMessage(errors.New(text), lang)
			if strings.Contains(msg, "org.branches") || strings.Contains(msg, "order_lines") || strings.Contains(msg, "SQLSTATE") || strings.Contains(msg, "relation") {
				t.Fatalf("lang %s: raw driver text leaked into %q", lang, msg)
			}
		}
	}
	if got, want := h.safeMessage(errors.New(raw[0]), "en"), i18n.T("en", "errors.500_internal"); got != want {
		t.Fatalf("generic message = %q, want %q", got, want)
	}
	if got, want := h.safeMessage(errors.New(raw[1]), "en"), i18n.T("en", "errors.400_bad_request"); got != want {
		t.Fatalf("foreign key message = %q, want %q", got, want)
	}
}

// S25: formulas are neutralised, plain numbers such as negative prices are not changed.
func TestCSVSafeNeutralisesFormulasOnly(t *testing.T) {
	cases := map[string]string{
		`=HYPERLINK("http://x.invalid","Panadol")`: `'=HYPERLINK("http://x.invalid","Panadol")`,
		`+1+cmd|' /C calc'!A0`:                     `'+1+cmd|' /C calc'!A0`,
		`-1+1`:                                     `'-1+1`,
		`@SUM(1+1)`:                                `'@SUM(1+1)`,
		"\tcmd":                                    "'\tcmd",
		`-12.50`:                                   `-12.50`,
		`+3`:                                       `+3`,
		`Panadol 500mg`:                            `Panadol 500mg`,
		``:                                         ``,
	}
	for in, want := range cases {
		if got := csvSafe(in); got != want {
			t.Fatalf("csvSafe(%q) = %q, want %q", in, got, want)
		}
	}
}

// S19: a registered and an unregistered email get the same redirect, so the
// form does not reveal which accounts exist.
func TestForgotPasswordAnswersAlikeForKnownAndUnknownEmail(t *testing.T) {
	h, _, repo, _ := setupTestMFAHandler(t)
	repo.usersByEm["known@dawa24.test"] = &identity.User{ID: 9, Email: "known@dawa24.test", Status: identity.StatusActive}

	locations := map[string]string{}
	for _, email := range []string{"known@dawa24.test", "unknown@dawa24.test"} {
		form := url.Values{"email": {email}}
		req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/auth/forgot", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()
		h.ForgotSubmit(w, req)
		if w.Code != http.StatusSeeOther {
			t.Fatalf("%s: status %d, want 303", email, w.Code)
		}
		locations[email] = w.Header().Get("Location")
	}
	for email, loc := range locations {
		if !strings.HasPrefix(loc, "/auth/forgot/verify?email=") || !strings.HasSuffix(loc, "&notice=sent") {
			t.Fatalf("%s: redirect %q is not the generic verify page", email, loc)
		}
		if strings.Contains(loc, "error=") {
			t.Fatalf("%s: redirect %q carries an error", email, loc)
		}
	}
}
