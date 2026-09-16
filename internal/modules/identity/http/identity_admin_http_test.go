package http_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/identity"
	identityHttp "github.com/muhiya/dawa24-store/internal/modules/identity/http"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/config"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/platform/httpx"
)

func newAuthedRouter(repo identity.Repository) http.Handler {
	log := slog.New(slog.NewJSONHandler(io.Discard, nil))
	svc := identity.NewService(repo, nil, log)
	handler := identityHttp.NewHandler(svc, config.Session{
		CookieName: testCookieName,
		TTL:        30 * 24 * time.Hour,
		SecureOnly: false,
	}, log)

	r := chi.NewRouter()
	r.Use(httpx.RequestID)
	r.Use(httpx.Recover(log))
	r.Use(httpx.Locale)

	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			sess := &identity.Session{
				UserID:      1,
				ActiveOrgID: 1,
				Role:        "super_admin",
				Permissions: []string{"admin", "super_admin", "identity.admin"},
			}
			ctx := identityHttp.WithSession(r.Context(), sess)
			actor := authctx.Actor{
				UserID:         1,
				OrganizationID: 1,
				Role:           "super_admin",
				Permissions:    []string{"admin", "super_admin", "identity.admin"},
			}
			ctx = authctx.WithActor(ctx, actor)
			ctx = database.WithTenant(ctx, 1)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	})

	r.Post("/api/v1/auth/register", handler.Register)
	r.Post("/api/v1/auth/login", handler.Login)
	r.Post("/api/v1/auth/logout", handler.Logout)
	r.Get("/api/v1/auth/me", handler.Me)
	r.Get("/api/v1/me", handler.GetMe)
	r.Put("/api/v1/me", handler.UpdateMe)
	r.Get("/api/v1/me/addresses", handler.ListAddresses)
	r.Post("/api/v1/me/addresses", handler.CreateAddress)
	r.Put("/api/v1/me/addresses/{id}", handler.UpdateAddress)
	r.Delete("/api/v1/me/addresses/{id}", handler.DeleteAddress)
	r.Get("/api/v1/me/favorites", handler.ListFavorites)
	r.Post("/api/v1/me/favorites", handler.AddFavorite)
	r.Delete("/api/v1/me/favorites/{productId}", handler.RemoveFavorite)
	handler.RegisterAdminRoutes(r)
	return r
}

var protectedRoutes = []struct{ method, path string }{
	{http.MethodGet, "/api/v1/auth/me"},
	{http.MethodGet, "/api/v1/me"},
	{http.MethodPut, "/api/v1/me"},
	{http.MethodGet, "/api/v1/me/addresses"},
	{http.MethodPost, "/api/v1/me/addresses"},
	{http.MethodPut, "/api/v1/me/addresses/1"},
	{http.MethodDelete, "/api/v1/me/addresses/1"},
	{http.MethodGet, "/api/v1/me/favorites"},
	{http.MethodPost, "/api/v1/me/favorites"},
	{http.MethodDelete, "/api/v1/me/favorites/1"},
}

func TestProtectedRoutesRejectAnonymousCallers(t *testing.T) {
	router := newTestRouter(t)

	for _, route := range protectedRoutes {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			req := httptest.NewRequest(route.method, route.path, strings.NewReader("{}"))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			router.ServeHTTP(rec, req)

			if rec.Code != http.StatusUnauthorized {
				t.Errorf("got %d, want 401 — this endpoint is reachable without a session", rec.Code)
			}
		})
	}
}

func TestProtectedRoutesRejectGarbageSessionToken(t *testing.T) {
	router := newTestRouter(t)

	for _, route := range protectedRoutes {
		req := httptest.NewRequest(route.method, route.path, strings.NewReader("{}"))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{
			Name:  testCookieName,
			Value: "forged-token-that-was-never-issued",
		})
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s with a forged token got %d, want 401", route.method, route.path, rec.Code)
		}
	}
}

func TestUnauthorizedResponseUsesTheErrorEnvelope(t *testing.T) {
	router := newTestRouter(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	var body httpx.ErrorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not the JSON error envelope: %v (body: %s)", err, rec.Body.String())
	}
	if body.Error.Code == "" {
		t.Error("error envelope has no code")
	}
	if body.Error.RequestID == "" {
		t.Error("error envelope has no request_id")
	}
}

func TestIdentityHandler_HappyPaths(t *testing.T) {
	router := newAuthedRouter(happyRepo{})

	tests := []struct {
		name       string
		method     string
		path       string
		body       string
		wantStatus int
	}{
		{"Register", http.MethodPost, "/api/v1/auth/register", `{"email":"newuser@example.com","password":"password123","name_ar":"مستخدم","name_en":"User"}`, http.StatusCreated},
		{"Logout", http.MethodPost, "/api/v1/auth/logout", "", http.StatusNoContent},
		{"Me", http.MethodGet, "/api/v1/auth/me", "", http.StatusOK},
		{"GetMe", http.MethodGet, "/api/v1/me", "", http.StatusOK},
		{"UpdateMe", http.MethodPut, "/api/v1/me", `{"name_ar":"مستخدم","name_en":"Updated User","phone":"01000000000","timezone":"Africa/Cairo","language":"en"}`, http.StatusOK},
		{"ListAddresses", http.MethodGet, "/api/v1/me/addresses", "", http.StatusOK},
		{"CreateAddress", http.MethodPost, "/api/v1/me/addresses", `{"title":"Office","recipient":"User","phone":"01000000000","address":"456 Nile St","city_id":1}`, http.StatusCreated},
		{"UpdateAddress", http.MethodPut, "/api/v1/me/addresses/1", `{"title":"Office 2","recipient":"User","phone":"01000000000","address":"456 Nile St","city_id":1}`, http.StatusOK},
		{"DeleteAddress", http.MethodDelete, "/api/v1/me/addresses/1", "", http.StatusOK},
		{"ListFavorites", http.MethodGet, "/api/v1/me/favorites", "", http.StatusOK},
		{"AddFavorite", http.MethodPost, "/api/v1/me/favorites", `{"product_id":1}`, http.StatusOK},
		{"RemoveFavorite", http.MethodDelete, "/api/v1/me/favorites/1", "", http.StatusOK},
		{"AdminUsers", http.MethodGet, "/api/v1/admin/identity/users", "", http.StatusOK},
		{"AdminGetUser", http.MethodGet, "/api/v1/admin/identity/users/1", "", http.StatusOK},
		{"AdminSuspend", http.MethodPost, "/api/v1/admin/identity/users/1/suspend", "", http.StatusOK},
		{"AdminReactivate", http.MethodPost, "/api/v1/admin/identity/users/1/reactivate", "", http.StatusOK},
		{"AdminResetMFA", http.MethodPost, "/api/v1/admin/identity/users/1/reset-mfa", "", http.StatusOK},
		{"AdminAssignRole", http.MethodPut, "/api/v1/admin/identity/users/1/role", `{"role":"vendor"}`, http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var bodyReader io.Reader
			if tt.body != "" {
				bodyReader = strings.NewReader(tt.body)
			}
			req := httptest.NewRequest(tt.method, tt.path, bodyReader)
			if tt.body != "" {
				req.Header.Set("Content-Type", "application/json")
			}
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			if rec.Code != tt.wantStatus {
				t.Errorf("%s %s got status %d, want %d (body: %s)", tt.method, tt.path, rec.Code, tt.wantStatus, rec.Body.String())
			}
		})
	}
}

func (r stubRepo) AdminCountUsers(_ context.Context) (int, error) {
	r.fail("AdminCountUsers")
	return 0, nil
}

func (r stubRepo) SearchUsers(_ context.Context, _ string, _ string, _ int) ([]*identity.User, error) {
	r.fail("SearchUsers")
	return nil, nil
}

func (r stubRepo) DefaultOrgForUser(_ context.Context, _ int64) (int64, error) {
	r.fail("DefaultOrgForUser")
	return 0, nil
}

func (r stubRepo) DefaultOrgInfoForUser(_ context.Context, _ int64) (int64, string, string, error) {
	r.fail("DefaultOrgInfoForUser")
	return 0, "", "", nil
}

func (r stubRepo) RegisterOrganization(_ context.Context, _ *identity.User, _ identity.RegisterOrgInput) (*identity.RegisterOrgResult, error) {
	r.fail("RegisterOrganization")
	return nil, nil
}

func (r stubRepo) ListAddressHistory(_ context.Context, _ int64, _ int) ([]*identity.UserAddressHistory, error) {
	r.fail("ListAddressHistory")
	return nil, nil
}

func (r stubRepo) GetPreferences(_ context.Context, _ int64) (*identity.UserPreferences, error) {
	r.fail("GetPreferences")
	return nil, nil
}

func (r stubRepo) UpdatePreferences(_ context.Context, _ *identity.UserPreferences) error {
	r.fail("UpdatePreferences")
	return nil
}

func (r stubRepo) ListSessionPlans(_ context.Context) ([]*identity.SessionPlan, error) {
	r.fail("ListSessionPlans")
	return nil, nil
}

func (r stubRepo) GetSessionPlanByID(_ context.Context, _ int64) (*identity.SessionPlan, error) {
	r.fail("GetSessionPlanByID")
	return nil, nil
}

func (r stubRepo) SetMaxLoginSessions(_ context.Context, _ int64, _ int) error {
	r.fail("SetMaxLoginSessions")
	return nil
}

func (r stubRepo) ListPlatformRoles(ctx context.Context) ([]*identity.PlatformRole, error) {
	return nil, nil
}

func (r stubRepo) GetPlatformRole(ctx context.Context, key string) (*identity.PlatformRole, error) {
	return nil, nil
}

func (r stubRepo) CreatePlatformRole(ctx context.Context, role *identity.PlatformRole, actorID int64) error {
	return nil
}

func (r stubRepo) UpdatePlatformRole(ctx context.Context, role *identity.PlatformRole, actorID int64) error {
	return nil
}

func (r stubRepo) DeletePlatformRole(ctx context.Context, key string) error { return nil }

func (happyRepo) ListPlatformRoles(ctx context.Context) ([]*identity.PlatformRole, error) {
	return nil, nil
}

func (happyRepo) GetPlatformRole(ctx context.Context, key string) (*identity.PlatformRole, error) {
	return &identity.PlatformRole{Key: key}, nil
}

func (happyRepo) CreatePlatformRole(ctx context.Context, role *identity.PlatformRole, actorID int64) error {
	return nil
}

func (happyRepo) UpdatePlatformRole(ctx context.Context, role *identity.PlatformRole, actorID int64) error {
	return nil
}

func (happyRepo) DeletePlatformRole(ctx context.Context, key string) error { return nil }
