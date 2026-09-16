package integration_test

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/platform/config"
	"github.com/muhiya/dawa24-store/internal/platform/gateway"
	"github.com/muhiya/dawa24-store/internal/platform/httpx"

	billingHttp "github.com/muhiya/dawa24-store/internal/modules/billing/http"
	catalogHttp "github.com/muhiya/dawa24-store/internal/modules/catalog/http"
	commerceHttp "github.com/muhiya/dawa24-store/internal/modules/commerce/http"
	hrHttp "github.com/muhiya/dawa24-store/internal/modules/hr/http"
	identityHttp "github.com/muhiya/dawa24-store/internal/modules/identity/http"
	ingestHttp "github.com/muhiya/dawa24-store/internal/modules/ingest/http"
	inventoryHttp "github.com/muhiya/dawa24-store/internal/modules/inventory/http"
	notificationsHttp "github.com/muhiya/dawa24-store/internal/modules/notifications/http"
	orgHttp "github.com/muhiya/dawa24-store/internal/modules/org/http"
	platformadminHttp "github.com/muhiya/dawa24-store/internal/modules/platform_admin/http"
	promoHttp "github.com/muhiya/dawa24-store/internal/modules/promo/http"
	workflowHttp "github.com/muhiya/dawa24-store/internal/modules/workflow/http"

	"github.com/muhiya/dawa24-store/internal/modules/billing"
	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/modules/hr"
	"github.com/muhiya/dawa24-store/internal/modules/identity"
	"github.com/muhiya/dawa24-store/internal/modules/ingest"
	"github.com/muhiya/dawa24-store/internal/modules/inventory"
	"github.com/muhiya/dawa24-store/internal/modules/notifications"
	"github.com/muhiya/dawa24-store/internal/modules/org"
	platformadmin "github.com/muhiya/dawa24-store/internal/modules/platform_admin"
	"github.com/muhiya/dawa24-store/internal/modules/promo"
	"github.com/muhiya/dawa24-store/internal/modules/workflow"
)

func buildTestRouter(log *slog.Logger) http.Handler {
	r := chi.NewRouter()
	r.Use(httpx.RequestID)
	r.Use(httpx.Recover(log))
	r.Use(httpx.Logger(log))
	r.Use(httpx.SecurityHeaders)
	r.Use(httpx.Locale)

	// Liveness
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		httpx.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	// Mount all handlers
	sessionCfg := config.Session{CookieName: "dawa24_session", TTL: 24 * time.Hour}
	identityHttp.NewHandler(identity.NewService(nil, nil, log), sessionCfg, log).RegisterRoutes(r)
	catalogHttp.NewHandler(catalog.NewService(nil, log), log).RegisterRoutes(r)
	inventoryHttp.NewHandler(inventory.NewService(nil, log), log).RegisterRoutes(r)
	commerceHttp.NewHandler(commerce.NewService(nil, log), log).RegisterRoutes(r)
	billingHttp.NewHandler(billing.NewService(nil, log), log).RegisterRoutes(r)
	ingestHttp.NewHandler(ingest.NewService(nil, log), log).RegisterRoutes(r)
	promoHttp.NewHandler(promo.NewService(nil, log), log).RegisterRoutes(r)
	workflowHttp.NewHandler(workflow.NewService(nil, log), log).RegisterRoutes(r)
	hrHttp.NewHandler(hr.NewService(nil, log), log).RegisterRoutes(r)
	platformadminHttp.NewHandler(platformadmin.NewService(nil, log), log).RegisterRoutes(r)
	notificationsHttp.NewHandler(notifications.NewService(nil, log), log).RegisterRoutes(r)
	orgHttp.NewHandler(org.NewService(nil, log), log).RegisterRoutes(r)

	return r
}

func TestRouterEndpointsMounting(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	router := buildTestRouter(logger)

	endpoints := []struct {
		method string
		path   string
	}{
		{"GET", "/health"},
		{"POST", "/api/v1/auth/login"},
		{"POST", "/api/v1/auth/register"},
		{"GET", "/api/v1/catalog/search"},
		{"GET", "/api/v1/catalog/categories"},
		{"GET", "/api/v1/inventory/warehouses"},
		{"POST", "/api/v1/commerce/checkout"},
		{"GET", "/api/v1/billing/plans"},
		{"GET", "/api/v1/promo/offers"},
		{"GET", "/api/v1/promo/packages"},
		{"GET", "/api/v1/promo/ads"},
		{"POST", "/api/v1/workflow/priority-requests"},
		{"GET", "/api/v1/hr/work-times"},
		{"GET", "/api/v1/platform/settings/public"},
		{"GET", "/api/v1/platform/countries"},
		{"GET", "/api/v1/notifications/unread-count"},
		{"GET", "/api/v1/org/organizations"},
	}

	for _, ep := range endpoints {
		req := httptest.NewRequest(ep.method, ep.path, nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		// Assert route is recognized (not a 404 Route Not Found)
		if rec.Code == http.StatusNotFound {
			t.Errorf("route %s %s was not found (404)", ep.method, ep.path)
		}
	}
}

var _ = config.Config{}
var _ = gateway.Client(nil)
var _ = time.Second
