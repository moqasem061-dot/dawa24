package ui_test

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/inventory"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/ui"
)

func TestVendorStockAlertsPage_E2E(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	repo := newMockInventoryRepoForWarehouseTest()
	invSvc := inventory.NewService(repo, logger)

	h := ui.NewUIHandler(
		nil, nil, nil, nil, invSvc, nil, nil, nil, nil, nil, nil, nil, nil, nil, logger,
	)

	// Setup mock warehouse with branch
	branchID := int64(10)
	wh := &inventory.Warehouse{
		ID:             1,
		OrganizationID: 100,
		BranchID:       &branchID,
		BranchName:     "فرع المعادي الرئيسي",
		Name:           "مستودع المعادي الطبي",
		Code:           "WH-MD-01",
		Address:        "شارع النصر، المعادي",
		IsActive:       true,
	}
	repo.warehouses[1] = wh

	// Low stock item: quantity 3 <= min_threshold 20
	repo.stocks = append(repo.stocks, &inventory.Stock{
		ID:               101,
		OrganizationID:   100,
		WarehouseID:      1,
		ProductID:        501,
		ProductVariantID: 701,
		Quantity:         3,
		MinThreshold:     20,
	})

	// Out of stock item: quantity 0 <= min_threshold 15
	repo.stocks = append(repo.stocks, &inventory.Stock{
		ID:               102,
		OrganizationID:   100,
		WarehouseID:      1,
		ProductID:        502,
		ProductVariantID: 702,
		Quantity:         0,
		MinThreshold:     15,
	})

	r := chi.NewRouter()
	h.RegisterVendorRoutes(r)

	// 1. Anonymous visitor is redirected to login
	{
		req := httptest.NewRequest(http.MethodGet, "/vendor/inventory/alerts", nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusSeeOther {
			t.Fatalf("expected redirect for anonymous user, got %d", rec.Code)
		}
		loc := rec.Header().Get("Location")
		if !strings.Contains(loc, "/auth/login") {
			t.Errorf("expected login redirect, got %s", loc)
		}
	}

	// 2. Authenticated vendor user can view stock alerts
	vendorActor := authctx.Actor{
		UserID:         10,
		OrganizationID: 100,
		OrgType:        "vendor",
		Permissions:    []string{"vendor.inventory.view", "vendor.inventory.adjust"},
	}

	{
		req := httptest.NewRequest(http.MethodGet, "/vendor/inventory/alerts", nil)
		ctx := authctx.WithActor(req.Context(), vendorActor)
		req = req.WithContext(ctx)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK for vendor alerts page, got %d: %s", rec.Code, rec.Body.String())
		}

		body := rec.Body.String()

		// Verify page title and header
		if !strings.Contains(body, "تنبيهات انخفاض ونفاد المخزون") {
			t.Errorf("expected page header 'تنبيهات انخفاض ونفاد المخزون' in response")
		}

		// Verify warehouse and branch names appear in the table
		if !strings.Contains(body, "مستودع المعادي الطبي") {
			t.Errorf("expected warehouse name 'مستودع المعادي الطبي' in table")
		}
		if !strings.Contains(body, "فرع المعادي الرئيسي") {
			t.Errorf("expected branch name 'فرع المعادي الرئيسي' in table")
		}

		// Verify stock badges and deficit
		if !strings.Contains(body, "3 عبوة") {
			t.Errorf("expected remaining quantity '3 عبوة' in table")
		}
		if !strings.Contains(body, "نقص 17 عبوة") {
			t.Errorf("expected deficit badge 'نقص 17 عبوة' in table")
		}
		if !strings.Contains(body, "عجز 15 عبوة") {
			t.Errorf("expected deficit badge 'عجز 15 عبوة' in table")
		}

		// Verify restock action button
		if !strings.Contains(body, "تعديل / تعبئة") {
			t.Errorf("expected restock button 'تعديل / تعبئة' in table")
		}
	}

	// 3. Warehouse alerts alias works
	{
		req := httptest.NewRequest(http.MethodGet, "/vendor/warehouses/alerts", nil)
		ctx := authctx.WithActor(req.Context(), vendorActor)
		req = req.WithContext(ctx)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK for warehouse alerts alias, got %d", rec.Code)
		}
	}
}
