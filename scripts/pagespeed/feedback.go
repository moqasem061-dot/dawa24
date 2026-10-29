package main

import (
	"log"
	"net/http"

	"github.com/moqasem061-dot/dawa24/internal/modules/catalog"
	"github.com/moqasem061-dot/dawa24/internal/modules/inventory"
	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
	"github.com/moqasem061-dot/dawa24/internal/shared/i18n"
	"github.com/moqasem061-dot/dawa24/internal/ui/layouts"
	"github.com/moqasem061-dot/dawa24/internal/ui/pages"
)

func stockAlertsFixture(w http.ResponseWriter, r *http.Request) {
	stocks := []*inventory.Stock{
		{ID: 101, WarehouseID: 1, ProductVariantID: 7, Quantity: 0, MinThreshold: 10},
		{ID: 102, WarehouseID: 2, ProductVariantID: 7, Quantity: 3, MinThreshold: 10},
	}
	data := pages.VendorStockAlertsData{
		Stocks: stocks, AllLowStocks: stocks, Total: 2, Page: 1, PerPage: 25, TotalPages: 1,
		Warehouses: []*inventory.Warehouse{{ID: 1, Name: "مخزن المعادي", BranchName: "فرع المعادي"}, {ID: 2, Name: "مخزن الجيزة", BranchName: "فرع الجيزة"}},
		Variants:   []*catalog.ProductVariant{{ID: 7, Name: i18n.Text{"ar": "بانادول 500 مجم", "en": "Panadol 500 mg"}}},
	}
	ctx := layouts.WithPath(r.Context(), r.URL.Path)
	ctx = authctx.WithActor(ctx, authctx.Actor{UserID: 1, OrganizationID: 10, OrgType: "vendor", Permissions: []string{"vendor.inventory.view", "vendor.inventory.adjust"}})
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.VendorStockAlerts(data, "ar", "rtl", false).Render(ctx, w); err != nil {
		log.Print(err)
	}
}

func branchMapFixture(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	ctx := layouts.WithPath(r.Context(), r.URL.Path)
	if err := pages.VendorBranchesPage(pages.VendorBranchesData{}, "ar", "rtl").Render(ctx, w); err != nil {
		log.Print(err)
	}
}
