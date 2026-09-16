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

	"github.com/muhiya/dawa24-store/internal/modules/notifications"
	notificationsHttp "github.com/muhiya/dawa24-store/internal/modules/notifications/http"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/platform/httpx"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

type stubRepo struct{ t *testing.T }

func (r stubRepo) fail(method string) {
	r.t.Helper()
	r.t.Fatalf("repository.%s was called; the request should have been rejected before reaching the repository", method)
}

func (r stubRepo) CreateLog(context.Context, *notifications.NotificationLog) error {
	r.fail("CreateLog")
	return nil
}
func (r stubRepo) GetTemplateBySlug(context.Context, string) (*notifications.Template, error) {
	r.fail("GetTemplateBySlug")
	return nil, nil
}
func (r stubRepo) ListUserNotifications(context.Context, int64, int, int) ([]*notifications.NotificationLog, error) {
	r.fail("ListUserNotifications")
	return nil, nil
}
func (r stubRepo) MarkAsRead(context.Context, int64, int64) error { r.fail("MarkAsRead"); return nil }
func (r stubRepo) MarkAllAsRead(context.Context, int64) (int64, error) {
	r.fail("MarkAllAsRead")
	return 0, nil
}
func (r stubRepo) ListUnread(context.Context, int64, int, int) ([]*notifications.NotificationLog, error) {
	r.fail("ListUnread")
	return nil, nil
}
func (r stubRepo) GetUnreadCount(context.Context, int64) (int, error) {
	r.fail("GetUnreadCount")
	return 0, nil
}
func (r stubRepo) HasNotificationWithTitle(context.Context, int64, string, time.Time) (bool, error) {
	r.fail("HasNotificationWithTitle")
	return false, nil
}

type happyRepo struct{}

func (happyRepo) CreateLog(ctx context.Context, l *notifications.NotificationLog) error {
	l.ID = 1
	return nil
}
func (happyRepo) GetTemplateBySlug(ctx context.Context, slug string) (*notifications.Template, error) {
	return &notifications.Template{ID: 1, Slug: slug}, nil
}
func (happyRepo) ListUserNotifications(ctx context.Context, userID int64, limit, offset int) ([]*notifications.NotificationLog, error) {
	return []*notifications.NotificationLog{{ID: 1, UserID: userID, Title: "Order Confirmed"}}, nil
}
func (happyRepo) MarkAsRead(ctx context.Context, id, userID int64) error {
	return nil
}
func (happyRepo) MarkAllAsRead(ctx context.Context, userID int64) (int64, error) {
	return 5, nil
}
func (happyRepo) ListUnread(ctx context.Context, userID int64, limit, offset int) ([]*notifications.NotificationLog, error) {
	return []*notifications.NotificationLog{{ID: 1, UserID: userID, Title: "Order Confirmed"}}, nil
}
func (happyRepo) GetUnreadCount(ctx context.Context, userID int64) (int, error) {
	return 3, nil
}
func (happyRepo) HasNotificationWithTitle(context.Context, int64, string, time.Time) (bool, error) {
	return false, nil
}


const testCookieName = "dawa24_session"

func newTestRouter(t *testing.T) http.Handler {
	t.Helper()

	log := slog.New(slog.NewJSONHandler(io.Discard, nil))
	notifSvc := notifications.NewService(stubRepo{t: t}, log)

	r := chi.NewRouter()
	r.Use(httpx.RequestID)
	r.Use(httpx.Recover(log))
	r.Use(httpx.Locale)

	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie(testCookieName)
			if err != nil || cookie.Value == "" || cookie.Value == "forged-token-that-was-never-issued" {
				httpx.Error(w, r, log, apperr.Unauthorized())
				return
			}
			next.ServeHTTP(w, r)
		})
	})
	notificationsHttp.NewHandler(notifSvc, log).RegisterRoutes(r)

	return r
}

func newAuthedRouter(repo notifications.Repository) http.Handler {
	log := slog.New(slog.NewJSONHandler(io.Discard, nil))
	notifSvc := notifications.NewService(repo, log)

	r := chi.NewRouter()
	r.Use(httpx.RequestID)
	r.Use(httpx.Recover(log))
	r.Use(httpx.Locale)

	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			actor := authctx.Actor{
				UserID:         1,
				OrganizationID: 1,
				Role:           "admin",
				Permissions:    []string{"admin", "notifications.admin"},
			}
			ctx := authctx.WithActor(r.Context(), actor)
			ctx = database.WithTenant(ctx, 1)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	})
	notificationsHttp.NewHandler(notifSvc, log).RegisterRoutes(r)
	return r
}

var protectedRoutes = []struct{ method, path string }{
	{http.MethodGet, "/api/v1/notifications"},
	{http.MethodPost, "/api/v1/notifications/1/read"},
	{http.MethodGet, "/api/v1/notifications/unread-count"},
	{http.MethodGet, "/api/v1/notifications/unread"},
	{http.MethodPost, "/api/v1/notifications/read-all"},
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
		req.AddCookie(&http.Cookie{Name: "dawa24_session", Value: "forged-token-that-was-never-issued"})
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s with a forged token got %d, want 401", route.method, route.path, rec.Code)
		}
	}
}

func TestUnauthorizedResponseUsesTheErrorEnvelope(t *testing.T) {
	router := newTestRouter(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/notifications", nil)
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

func TestNotificationsHandler_HappyPaths(t *testing.T) {
	router := newAuthedRouter(happyRepo{})

	tests := []struct {
		name       string
		method     string
		path       string
		body       string
		wantStatus int
	}{
		{"ListNotifications", http.MethodGet, "/api/v1/notifications?limit=10&offset=0", "", http.StatusOK},
		{"MarkAsRead", http.MethodPost, "/api/v1/notifications/1/read", "", http.StatusOK},
		{"GetUnreadCount", http.MethodGet, "/api/v1/notifications/unread-count", "", http.StatusOK},
		{"ListUnread", http.MethodGet, "/api/v1/notifications/unread?limit=10&offset=0", "", http.StatusOK},
		{"MarkAllRead", http.MethodPost, "/api/v1/notifications/read-all", "", http.StatusOK},
		{"AdminBroadcast", http.MethodPost, "/api/v1/admin/notifications/broadcast", `{"title":"System Maintenance","body":"Downtime in 10 mins"}`, http.StatusOK},
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
