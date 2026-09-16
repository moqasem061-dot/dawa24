package ui_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/ui"
)

func TestVendorIngestWizardResumability(t *testing.T) {
	ctx := context.Background()
	actor := authctx.Actor{
		UserID:         100,
		OrganizationID: 10,
		OrgType:        "vendor",
		// The vendor dashboard is permission-gated; this double stands in for
		// a company owner, who holds their whole dashboard.
		Permissions: []string{"vendor.*"},
	}
	ctx = authctx.WithActor(ctx, actor)
	ctx = database.WithTenant(ctx, 10)

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := ui.NewUIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, logger)

	r := chi.NewRouter()
	handler.RegisterVendorRoutes(r)

	// Step 1: GET /vendor/ingest without session should render 200 OK
	req1, _ := http.NewRequestWithContext(ctx, "GET", "/vendor/ingest", nil)
	rr1 := httptest.NewRecorder()
	r.ServeHTTP(rr1, req1)
	if rr1.Code != http.StatusOK {
		t.Errorf("expected 200 OK for /vendor/ingest, got %d", rr1.Code)
	}

	// Step 2: GET /vendor/ingest/{sessionID}
	req2, _ := http.NewRequestWithContext(ctx, "GET", "/vendor/ingest/999", nil)
	rr2 := httptest.NewRecorder()
	r.ServeHTTP(rr2, req2)
	// Non-existing session redirects back to /vendor/ingest
	if rr2.Code != http.StatusSeeOther {
		t.Errorf("expected redirect for non-existent session, got %d", rr2.Code)
	}

	// Step 3: GET /vendor/ingest/{id}/catalog-search
	reqSearch, _ := http.NewRequestWithContext(ctx, "GET", "/vendor/ingest/999/catalog-search?q=panadol", nil)
	rrSearch := httptest.NewRecorder()
	r.ServeHTTP(rrSearch, reqSearch)
	if rrSearch.Code != http.StatusOK {
		t.Errorf("expected 200 OK for catalog-search, got %d", rrSearch.Code)
	}

	// Step 4: POST /vendor/ingest/{id}/rows/1/toggle
	reqToggle, _ := http.NewRequestWithContext(ctx, "POST", "/vendor/ingest/999/rows/1/toggle", nil)
	rrToggle := httptest.NewRecorder()
	r.ServeHTTP(rrToggle, reqToggle)
	if rrToggle.Code != http.StatusSeeOther {
		t.Errorf("expected 303 redirect for row toggle, got %d", rrToggle.Code)
	}

	// Step 5: POST /vendor/ingest/{id}/batch-quantity
	reqBatch, _ := http.NewRequestWithContext(ctx, "POST", "/vendor/ingest/999/batch-quantity", strings.NewReader("batch_quantity=50"))
	reqBatch.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rrBatch := httptest.NewRecorder()
	r.ServeHTTP(rrBatch, reqBatch)
	if rrBatch.Code != http.StatusSeeOther {
		t.Errorf("expected 303 redirect for batch quantity, got %d", rrBatch.Code)
	}

	// Step 6: POST /vendor/ingest/{id}/rows/1/update with quantity and price
	reqUpdate, _ := http.NewRequestWithContext(ctx, "POST", "/vendor/ingest/999/rows/1/update", strings.NewReader("display_name=Panadol&quantity=25&price=45.50"))
	reqUpdate.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rrUpdate := httptest.NewRecorder()
	r.ServeHTTP(rrUpdate, reqUpdate)
	if rrUpdate.Code != http.StatusSeeOther {
		t.Errorf("expected 303 redirect for row update, got %d", rrUpdate.Code)
	}
}
