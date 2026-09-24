package http_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	platformadmin "github.com/moqasem061-dot/dawa24/internal/modules/platform_admin"
	platformadminHttp "github.com/moqasem061-dot/dawa24/internal/modules/platform_admin/http"
	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
	"github.com/moqasem061-dot/dawa24/internal/platform/httpx"
)

// An ordinary approved member must not read non-public settings (they hold the
// Gateway and AI credentials) or the contact inbox.
func TestMemberCannotReadPrivateSettingsOrContactInbox(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	r := chi.NewRouter()
	r.Use(httpx.RequestID)
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			a := authctx.Actor{UserID: 3, OrganizationID: 7, Role: "user"}
			a.Grants([]string{"vendor.order.view"})
			ctx := database.WithTenant(authctx.WithActor(req.Context(), a), 7)
			next.ServeHTTP(w, req.WithContext(ctx))
		})
	})
	platformadminHttp.NewHandler(platformadmin.NewService(happyRepo{}, log), log).RegisterRoutes(r)

	for path, want := range map[string]int{
		"/api/v1/platform/settings/gateway_configuration": http.StatusNotFound,
		"/api/v1/platform/contact":                        http.StatusForbidden,
	} {
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequestWithContext(context.Background(), http.MethodGet, path, nil))
		if rec.Code != want {
			t.Errorf("GET %s by a member: status %d; want %d", path, rec.Code, want)
		}
	}
}
