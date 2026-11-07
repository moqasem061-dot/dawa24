package ui

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
)

func accountPasswordHandler() *UIHandler {
	return &UIHandler{log: slog.New(slog.NewTextHandler(io.Discard, nil))}
}

// A member whose role holds no permission at all can still open the password
// form that the forced-change hold points to.
func TestAccountPasswordPageNeedsNoPermission(t *testing.T) {
	h := accountPasswordHandler()
	req := httptest.NewRequest(http.MethodGet, "/account/password", nil)
	req = req.WithContext(authctx.WithActor(req.Context(), authctx.Actor{UserID: 9}))
	rec := httptest.NewRecorder()
	h.AccountPasswordPage(rec, req)
	body := rec.Body.String()
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, want 200", rec.Code)
	}
	for _, want := range []string{`action="/customer/password"`, `name="current_password"`, `name="new_password"`, `name="return" value="account"`} {
		if !strings.Contains(body, want) {
			t.Errorf("page lacks %s", want)
		}
	}
}

func TestAccountPasswordPageRequiresSignIn(t *testing.T) {
	h := accountPasswordHandler()
	rec := httptest.NewRecorder()
	h.AccountPasswordPage(rec, httptest.NewRequest(http.MethodGet, "/account/password", nil))
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/auth/login" {
		t.Fatalf("anonymous got %d to %q, want redirect to /auth/login", rec.Code, rec.Header().Get("Location"))
	}
}

// A failed change from the account page returns to that page, not to the
// permission-gated sessions page.
func TestPasswordChangeFailureReturnsToAccountPage(t *testing.T) {
	h := accountPasswordHandler()
	form := url.Values{"return": {"account"}, "current_password": {"old"}, "new_password": {"abcdefgh1"}, "confirm_password": {"different"}}
	req := httptest.NewRequest(http.MethodPost, "/customer/password", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(authctx.WithActor(context.Background(), authctx.Actor{UserID: 9}))
	rec := httptest.NewRecorder()
	h.TenantPasswordChangeSubmit(rec, req)
	if loc := rec.Header().Get("Location"); !strings.HasPrefix(loc, "/account/password") {
		t.Fatalf("redirected to %q, want /account/password", loc)
	}
}
