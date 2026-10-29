package ui_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/moqasem061-dot/dawa24/internal/modules/inventory"
	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
	"github.com/moqasem061-dot/dawa24/internal/ui"
)

type stockAdjustmentRepo struct {
	*mockTransferRepo
	tenant  int64
	failure error
}

func (r *stockAdjustmentRepo) AdjustStock(ctx context.Context, id int64, delta int, movement inventory.StockMovement) (*inventory.Stock, error) {
	r.tenant, _ = database.TenantFrom(ctx)
	if r.failure != nil {
		return nil, r.failure
	}
	return r.mockTransferRepo.AdjustStock(ctx, id, delta, movement)
}

func TestAlertAdjustmentTargetsStockAndReportsFailure(t *testing.T) {
	for _, tc := range []struct {
		name, delta string
		owner       int64
		failure     error
		changed     bool
	}{
		{"restock", "20", 55, nil, true}, {"deduct", "-2", 55, nil, true},
		{"zero", "0", 55, nil, false}, {"invalid", "2.5", 55, nil, false},
		{"overflow", "9999999999999999999999", 55, nil, false},
		{"other tenant", "20", 99, nil, false}, {"database failure", "20", 55, errors.New("write rejected"), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &stockAdjustmentRepo{mockTransferRepo: newMockTransferRepo(), failure: tc.failure}
			repo.stocks[1] = &inventory.Stock{ID: 1, OrganizationID: tc.owner, WarehouseID: 10, ProductVariantID: 7, Quantity: 5}
			repo.stocks[2] = &inventory.Stock{ID: 2, OrganizationID: 55, WarehouseID: 11, ProductVariantID: 7, Quantity: 30}
			logger := slog.New(slog.NewTextHandler(io.Discard, nil))
			h := ui.NewUIHandler(nil, nil, nil, nil, inventory.NewService(repo, logger), nil, nil, nil, nil, nil, nil, nil, nil, nil, logger)
			router := chi.NewRouter()
			h.RegisterVendorRoutes(router)
			form := url.Values{"delta": {tc.delta}, "reason": {"receipt"}, "redirect_url": {"/vendor/inventory/alerts"}}
			req := httptest.NewRequestWithContext(context.Background(), "POST", "/vendor/inventory/1/adjust", strings.NewReader(form.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			req = req.WithContext(authctx.WithActor(req.Context(), authctx.Actor{UserID: 1, OrganizationID: 55, OrgType: "vendor", Permissions: []string{"vendor.inventory.adjust"}}))
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			location, err := url.Parse(rec.Header().Get("Location"))
			if err != nil {
				t.Fatal(err)
			}
			kind := "error"
			if tc.changed {
				kind = "success"
			}
			if location.Path != "/vendor/inventory/alerts" || location.Query().Get("notice") != kind {
				t.Fatalf("unexpected redirect %d %s", rec.Code, location)
			}
			if repo.stocks[2].Quantity != 30 {
				t.Fatal("another warehouse's stock changed")
			}
			if tc.changed && (repo.tenant != 55 || len(repo.movements) != 1 || repo.movements[0].StockID != 1 || repo.movements[0].UserID == nil || *repo.movements[0].UserID != 1) {
				t.Fatal("tenant or audit movement missing")
			}
			if !tc.changed && (repo.stocks[1].Quantity != 5 || len(repo.movements) != 0) {
				t.Fatal("failed adjustment changed stock")
			}
		})
	}
}
