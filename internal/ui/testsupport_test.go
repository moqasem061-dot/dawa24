package ui_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	attachmentsPostgres "github.com/muhiya/dawa24-store/internal/modules/attachments/postgres"
	billingPostgres "github.com/muhiya/dawa24-store/internal/modules/billing/postgres"
	catalogPostgres "github.com/muhiya/dawa24-store/internal/modules/catalog/postgres"
	chatPostgres "github.com/muhiya/dawa24-store/internal/modules/chat/postgres"
	commercePostgres "github.com/muhiya/dawa24-store/internal/modules/commerce/postgres"
	comparePostgres "github.com/muhiya/dawa24-store/internal/modules/compare/postgres"
	hrPostgres "github.com/muhiya/dawa24-store/internal/modules/hr/postgres"
	identityPostgres "github.com/muhiya/dawa24-store/internal/modules/identity/postgres"
	ingestPostgres "github.com/muhiya/dawa24-store/internal/modules/ingest/postgres"
	inventoryPostgres "github.com/muhiya/dawa24-store/internal/modules/inventory/postgres"
	notificationsPostgres "github.com/muhiya/dawa24-store/internal/modules/notifications/postgres"
	orgPostgres "github.com/muhiya/dawa24-store/internal/modules/org/postgres"
	platformadminPostgres "github.com/muhiya/dawa24-store/internal/modules/platform_admin/postgres"
	promoPostgres "github.com/muhiya/dawa24-store/internal/modules/promo/postgres"
	workflowPostgres "github.com/muhiya/dawa24-store/internal/modules/workflow/postgres"

	"github.com/muhiya/dawa24-store/internal/modules/attachments"
	"github.com/muhiya/dawa24-store/internal/modules/billing"
	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/modules/chat"
	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/modules/compare"
	"github.com/muhiya/dawa24-store/internal/modules/hr"
	"github.com/muhiya/dawa24-store/internal/modules/identity"
	"github.com/muhiya/dawa24-store/internal/modules/ingest"
	"github.com/muhiya/dawa24-store/internal/modules/inventory"
	"github.com/muhiya/dawa24-store/internal/modules/notifications"
	"github.com/muhiya/dawa24-store/internal/modules/org"
	platformadmin "github.com/muhiya/dawa24-store/internal/modules/platform_admin"
	"github.com/muhiya/dawa24-store/internal/modules/promo"
	"github.com/muhiya/dawa24-store/internal/modules/workflow"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/config"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/ui"
)

// testDB returns a real PostgreSQL connection pool for integration tests.
func testDB(t *testing.T) *database.DB {
	t.Helper()

	if testing.Short() {
		t.Skip("skipping database integration test in short mode")
	}

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = os.Getenv("TEST_DATABASE_URL")
	}
	if dbURL == "" {
		if os.Getenv("CI") == "true" {
			t.Fatal("DATABASE_URL or TEST_DATABASE_URL must be set in CI")
		}
		t.Skip("DATABASE_URL not set; skipping integration test")
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cfg := config.Database{
		URL:              dbURL,
		MaxConns:         5,
		MinConns:         1,
		MaxConnLifetime:  time.Hour,
		MaxConnIdleTime:  30 * time.Minute,
		StatementTimeout: 10 * time.Second,
	}

	db, err := database.Open(ctx, cfg)
	if err != nil {
		if os.Getenv("CI") == "true" {
			t.Fatalf("CI failed to connect to database at %s: %v", dbURL, err)
		}
		t.Skipf("cannot connect to database at %s: %v; skipping", dbURL, err)
		return nil
	}

	t.Cleanup(func() {
		db.Close()
	})

	return db
}

// newRealUIHandler builds a UIHandler wired to real services over a real database.
func newRealUIHandler(t *testing.T, db *database.DB) http.Handler {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	catRepo := catalogPostgres.NewRepository(db)
	orgRepo := orgPostgres.NewRepository(db)
	ingRepo := ingestPostgres.NewRepository(db)
	commRepo := commercePostgres.NewRepository(db)
	invRepo := inventoryPostgres.NewRepository(db)
	idRepo := identityPostgres.NewRepository(db)
	notifRepo := notificationsPostgres.NewRepository(db)
	promoRepo := promoPostgres.NewRepository(db)
	adminRepo := platformadminPostgres.NewRepository(db)
	billRepo := billingPostgres.NewRepository(db)
	chatRepo := chatPostgres.NewRepository(db)
	wfRepo := workflowPostgres.NewRepository(db)
	hrRepo := hrPostgres.NewRepository(db)
	attachRepo := attachmentsPostgres.NewRepository(db)

	orgSvc := org.NewService(orgRepo, logger)
	catSvc := catalog.NewService(catRepo, logger)
	// The import wizard stages a parsed file for review before writing it; the
	// handler reports itself unavailable without a store, so the harness has to
	// wire the same one the composition root does.
	catSvc.SetImportStore(catRepo)
	ingSvc := ingest.NewService(ingRepo, logger)
	commSvc := commerce.NewService(commRepo, logger)
	invSvc := inventory.NewService(invRepo, logger)
	idSvc := identity.NewService(idRepo, nil, logger)
	notifSvc := notifications.NewService(notifRepo, logger)
	promoSvc := promo.NewService(promoRepo, logger)
	adminSvc := platformadmin.NewService(adminRepo, logger)
	billSvc := billing.NewService(billRepo, logger)
	chatSvc := chat.NewService(chatRepo, logger)
	wfSvc := workflow.NewService(wfRepo, logger)
	hrSvc := hr.NewService(hrRepo, logger)
	attachSvc := attachments.NewService(attachRepo, nil, logger)

	handler := ui.NewUIHandler(
		catSvc,
		orgSvc,
		ingSvc,
		commSvc,
		invSvc,
		idSvc,
		notifSvc,
		promoSvc,
		adminSvc,
		billSvc,
		chatSvc,
		wfSvc,
		hrSvc,
		attachSvc,
		logger,
	)

	compareRepo := comparePostgres.NewRepository(db)
	compareSvc := compare.NewService(compareRepo, logger)
	handler.SetCompareService(compareSvc)

	return newRealUIHandlerRouter(handler)
}

func newRealUIHandlerRouter(handler *ui.UIHandler) http.Handler {
	r := chi.NewRouter()
	handler.RegisterAdminRoutes(r)
	handler.RegisterVendorRoutes(r)
	handler.RegisterCustomerRoutes(r)
	handler.RegisterPreApprovalRoutes(r)
	handler.RegisterApprovedSharedRoutes(r)
	handler.RegisterCustomerSharedRoutes(r)
	handler.RegisterVendorSharedRoutes(r)
	handler.RegisterBuyingRoutes(r)
	handler.RegisterPublicRoutes(r)
	return r
}

// doGET drives the handler over HTTP as a given actor.
func doGET(t *testing.T, h http.Handler, path string, actor authctx.Actor) *httptest.ResponseRecorder {
	t.Helper()
	ctx := context.Background()
	if actor.UserID != 0 || actor.OrganizationID != 0 || actor.Role != "" {
		ctx = authctx.WithActor(ctx, actor)
	}

	req, err := http.NewRequestWithContext(ctx, "GET", path, nil)
	if err != nil {
		t.Fatalf("failed to create GET request: %v", err)
	}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// doPOST drives the handler over HTTP as a given actor with url-encoded form values.
func doPOST(t *testing.T, h http.Handler, path string, form url.Values, actor authctx.Actor) *httptest.ResponseRecorder {
	t.Helper()
	ctx := context.Background()
	if actor.UserID != 0 || actor.OrganizationID != 0 || actor.Role != "" {
		ctx = authctx.WithActor(ctx, actor)
	}

	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}

	req, err := http.NewRequestWithContext(ctx, "POST", path, body)
	if err != nil {
		t.Fatalf("failed to create POST request: %v", err)
	}
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func actorFor(orgID int64, orgType string, isStaff bool, role string) authctx.Actor {
	return authctx.Actor{
		UserID:         1000 + orgID,
		OrganizationID: orgID,
		OrgType:        orgType,
		IsStaff:        isStaff,
		Role:           role,
	}
}

// --- Seed Helpers with Cleanup ---

