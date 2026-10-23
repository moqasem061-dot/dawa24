package main

import (
	"log"
	"net/http"
	"time"

	"github.com/moqasem061-dot/dawa24/internal/modules/billing"
	"github.com/moqasem061-dot/dawa24/internal/modules/org"
	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
	"github.com/moqasem061-dot/dawa24/internal/shared/i18n"
	"github.com/moqasem061-dot/dawa24/internal/shared/money"
	"github.com/moqasem061-dot/dawa24/internal/ui/layouts"
	"github.com/moqasem061-dot/dawa24/internal/ui/pages"
)

func adminOrganizationsFixture(w http.ResponseWriter, r *http.Request) {
	actor := authctx.Actor{UserID: 1, IsStaff: true, Role: "super_admin"}
	ctx := layouts.WithPath(authctx.WithActor(r.Context(), actor), r.URL.Path)
	company := &org.Organization{ID: 26, LegalName: "مخزن بيت السلام لتوزيع الأدوية والمستلزمات الطبية", Type: org.TypeVendor,
		TradeName: i18n.New("بيت السلام", "Beit El Salam"), CommercialRegister: "350220", Status: org.StatusApproved, CreatedAt: time.Now()}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.AdminOrganizationsPage(pages.AdminOrganizationsPageData{Organizations: []*org.Organization{company}, TotalCount: 1, Page: 1, PerPage: 25}, "ar", "rtl").Render(ctx, w); err != nil {
		log.Print(err)
	}
}

func vendorPricingFixture(w http.ResponseWriter, r *http.Request) {
	actor := authctx.Actor{UserID: 1, OrganizationID: 10, OrgID: 10, OrgType: "vendor", Permissions: []string{"vendor.*"}}
	ctx := layouts.WithPath(authctx.WithActor(r.Context(), actor), r.URL.Path)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	form := pages.VendorVariantFormInput{ProductID: 7, Price: "100", Discount: "10", NameAR: "كريم", StockQty: "5", MinOrderQty: "1"}
	if err := pages.VendorProductEditor(pages.VendorVariantEditorData{Form: form}, "ar", "rtl").Render(ctx, w); err != nil {
		log.Print(err)
	}
}

func financeDepositsFixture(w http.ResponseWriter, r *http.Request) {
	actor := authctx.Actor{UserID: 1, IsStaff: true, Role: "super_admin"}
	ctx := layouts.WithPath(authctx.WithActor(r.Context(), actor), r.URL.Path)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	deposit := &billing.AdminWalletDepositView{ID: 81, UserName: "مستخدم الاختبار", UserEmail: "fixture@example.com",
		Amount: money.MustParse("2111"), Status: billing.DepositPending, PaymentMethod: "instapay", CreatedAt: time.Now()}
	if err := pages.AdminFinanceDepositsPage(pages.AdminFinanceData{Deposits: []*billing.AdminWalletDepositView{deposit}, TotalDeposits: 1, PendingDepositsCount: 1, Page: 1, PerPage: 25}, "ar", "rtl").Render(ctx, w); err != nil {
		log.Print(err)
	}
}
