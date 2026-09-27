package ui_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/moqasem061-dot/dawa24/internal/modules/org"
	"github.com/moqasem061-dot/dawa24/internal/modules/promo"
	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
	"github.com/moqasem061-dot/dawa24/internal/shared/i18n"
	"github.com/moqasem061-dot/dawa24/internal/shared/money"
	"github.com/moqasem061-dot/dawa24/internal/ui"
)

func TestOffersWorkflowAndRendering(t *testing.T) {
	now := time.Now().UTC()
	expiry := now.Add(30 * 24 * time.Hour)

	mockRepo := &mockPromoRepo{
		offers: []*promo.Offer{
			{
				ID:             101,
				OrganizationID: 50,
				Title:          i18n.New("عرض الصيف الحصري للأدوية", "Summer Medicine Offer"),
				Description:    i18n.New("خصم خاص 20% لكافة الصيدليات", "Special 20% discount"),
				DiscountType:   promo.DiscountPercentage,
				DiscountValue:  money.FromMinor(2000), // 20%
				StartsAt:       now,
				ExpiresAt:      expiry,
				IsActive:       true,
				AdminStatus:    "approved",
			},
		},
		spec: &promo.SpecialOffer{
			ID:                 101,
			OrganizationID:     50,
			OrganizationName:   "شركة الأمل للمستلزمات والأدوية",
			Title:              i18n.New("عرض الصيف الحصري للأدوية", "Summer Medicine Offer"),
			Description:        i18n.New("خصم خاص 20% لكافة الصيدليات", "Special 20% discount"),
			DiscountPercentage: 20.0,
			TotalPrice:         money.FromMinor(45000),
			StartDate:          &now,
			EndDate:            &expiry,
			Status:             "active",
			AdminStatus:        "approved",
			Products: []*promo.SpecialOfferProduct{
				{
					ID:                 1,
					OfferID:            101,
					VariantID:          26643,
					VariantName:        "يوريكودروب 80مجم 30 قرص",
					OriginalPrice:      money.FromMinor(3500),
					CustomPrice:        money.FromMinor(2800),
					DiscountPercentage: 20.0,
					Quantity:           10,
				},
			},
			Locations: []*promo.SpecialOfferLocation{
				{
					ID:        1,
					OfferID:   101,
					CityName:  "القاهرة",
					AddressAr: "مدينة نصر والتجمع الخامس",
				},
			},
		},
	}

	promoSvc := promo.NewService(mockRepo, slog.Default())
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := ui.NewUIHandler(nil, nil, nil, nil, nil, nil, nil, promoSvc, nil, nil, nil, nil, nil, nil, logger)

	r := chi.NewRouter()
	handler.RegisterPublicRoutes(r)
	r.Post("/cart/add-offer", handler.AddOfferToCartSubmit)

	testActor := authctx.Actor{
		UserID:         5,
		OrganizationID: 12,
		Role:           "pharmacy",
	}

	// 1. Test GET /offers (Public Offers Listing)
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/offers", nil)
	req = req.WithContext(authctx.WithActor(req.Context(), testActor))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200 for /offers, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !containsStr(body, "عرض الصيف الحصري للأدوية") {
		t.Fatalf("expected /offers to contain offer title")
	}
	if !containsStr(body, "شركة الأمل للمستلزمات والأدوية") {
		t.Fatalf("expected /offers to contain supplier name")
	}
	if strings.Contains(body, "Ø") || strings.Contains(body, "Ù") {
		t.Fatalf("detected mojibake characters in /offers HTML response")
	}

	// 2. Test GET /offers/101 (Offer Detail Page)
	reqDetail := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/offers/101", nil)
	reqDetail = reqDetail.WithContext(authctx.WithActor(reqDetail.Context(), testActor))
	recDetail := httptest.NewRecorder()
	r.ServeHTTP(recDetail, reqDetail)

	if recDetail.Code != http.StatusOK {
		t.Fatalf("expected status 200 for /offers/101, got %d", recDetail.Code)
	}
	bodyDetail := recDetail.Body.String()
	if !containsStr(bodyDetail, "يوريكودروب 80مجم 30 قرص") {
		t.Fatalf("expected /offers/101 to contain medicine item name")
	}
	if !containsStr(bodyDetail, "28.00") {
		t.Fatalf("expected /offers/101 to display discounted custom price")
	}

	// 3. Test GET /offers/101 as Logged-in Customer Pharmacy
	reqAuth := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/offers/101", nil)
	reqAuth = reqAuth.WithContext(authctx.WithActor(reqAuth.Context(), authctx.Actor{
		UserID:         5,
		OrganizationID: 12,
		OrgType:        string(org.TypeCustomer),
		Role:           "customer",
	}))
	recAuth := httptest.NewRecorder()
	r.ServeHTTP(recAuth, reqAuth)

	if recAuth.Code != http.StatusOK {
		t.Fatalf("expected status 200 for /offers/101 as customer, got %d", recAuth.Code)
	}
	bodyAuth := recAuth.Body.String()
	if !containsStr(bodyAuth, "action=\"/cart/add-offer\"") {
		t.Fatalf("expected /offers/101 for customer to render /cart/add-offer form")
	}

	// 4. Test POST /cart/add-offer
	formOffer := url.Values{}
	formOffer.Set("offer_id", "101")
	formOffer.Set("quantity", "1")
	reqPostOffer := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/cart/add-offer", strings.NewReader(formOffer.Encode()))
	reqPostOffer.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqPostOffer = reqPostOffer.WithContext(authctx.WithActor(reqPostOffer.Context(), authctx.Actor{
		UserID:         5,
		OrganizationID: 12,
		OrgType:        string(org.TypeCustomer),
		Role:           "customer",
		Permissions:    []string{"pharmacy.cart.use"},
	}))
	recPostOffer := httptest.NewRecorder()
	r.ServeHTTP(recPostOffer, reqPostOffer)
	if recPostOffer.Code != http.StatusSeeOther && recPostOffer.Code != http.StatusOK {
		t.Fatalf("expected status 303 or 200 for POST /cart/add-offer, got %d", recPostOffer.Code)
	}
}

func containsStr(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && (s == substr || len(s) >= len(substr) && stringContains(s, substr))
}

func stringContains(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestVendorOfferEditAndAdminModerationWorkflow(t *testing.T) {
	now := time.Now().UTC()
	expiry := now.Add(30 * 24 * time.Hour)

	mockRepo := &mockPromoRepo{
		spec: &promo.SpecialOffer{
			ID:                 101,
			OrganizationID:     50,
			OrganizationName:   "شركة الأمل للمستلزمات والأدوية",
			Title:              i18n.New("عرض الصيف الحصري للأدوية", "Summer Medicine Offer"),
			Description:        i18n.New("خصم خاص 20% لكافة الصيدليات", "Special 20% discount"),
			DiscountPercentage: 20.0,
			TotalPrice:         money.FromMinor(45000),
			StartDate:          &now,
			EndDate:            &expiry,
			Status:             "active",
			AdminStatus:        "pending",
			Products: []*promo.SpecialOfferProduct{
				{
					ID:                 1,
					OfferID:            101,
					VariantID:          26643,
					VariantName:        "يوريكودروب 80مجم 30 قرص",
					OriginalPrice:      money.FromMinor(3500),
					CustomPrice:        money.FromMinor(2800),
					DiscountPercentage: 20.0,
					Quantity:           10,
				},
			},
			Locations: []*promo.SpecialOfferLocation{
				{
					ID:        1,
					OfferID:   101,
					CityName:  "القاهرة",
					AddressAr: "مدينة نصر والتجمع الخامس",
				},
			},
		},
	}

	promoSvc := promo.NewService(mockRepo, slog.Default())
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := ui.NewUIHandler(nil, nil, nil, nil, nil, nil, nil, promoSvc, nil, nil, nil, nil, nil, nil, logger)

	r := chi.NewRouter()
	r.Get("/vendor/offers/{id}/edit", handler.VendorOfferEditPage)
	r.Post("/vendor/offers/{id}/edit", handler.VendorOfferEditSubmit)
	r.Post("/admin/offers/{id}/request-changes", handler.AdminOfferRequestChangesSubmit)
	r.Post("/admin/offers/{id}/reject", handler.AdminOfferRejectSubmit)
	r.Post("/admin/offers/{id}/approve", handler.AdminOfferApproveSubmit)

	vendorActor := authctx.Actor{
		UserID:         5,
		OrganizationID: 50,
		Role:           "supplier",
		OrgType:        string(org.TypeVendor),
		Permissions:    []string{"vendor.offers.manage"},
	}

	adminActor := authctx.Actor{
		UserID:         1,
		OrganizationID: 1,
		Role:           "superadmin",
		Permissions:    []string{"admin.offers.manage", "platform.admin"},
	}

	// 1. Vendor opens edit page: GET /vendor/offers/101/edit
	reqEdit := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/vendor/offers/101/edit", nil)
	reqEdit = reqEdit.WithContext(authctx.WithActor(reqEdit.Context(), vendorActor))
	recEdit := httptest.NewRecorder()
	r.ServeHTTP(recEdit, reqEdit)
	if recEdit.Code != http.StatusOK {
		t.Fatalf("expected status 200 for vendor edit page, got %d", recEdit.Code)
	}

	// 2. Admin requests changes: POST /admin/offers/101/request-changes
	formChanges := url.Values{}
	formChanges.Set("notes", "يرجى تعديل نسبة الخصم وإرفاق البانر بدقة")
	reqChanges := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/admin/offers/101/request-changes", strings.NewReader(formChanges.Encode()))
	reqChanges.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqChanges = reqChanges.WithContext(authctx.WithActor(reqChanges.Context(), adminActor))
	recChanges := httptest.NewRecorder()
	r.ServeHTTP(recChanges, reqChanges)
	if recChanges.Code != http.StatusSeeOther {
		t.Fatalf("expected status 303 for request changes, got %d", recChanges.Code)
	}
	if mockRepo.spec.AdminStatus != "changes_requested" {
		t.Fatalf("expected AdminStatus changes_requested, got %s", mockRepo.spec.AdminStatus)
	}
	if mockRepo.spec.AdminNotes != "يرجى تعديل نسبة الخصم وإرفاق البانر بدقة" {
		t.Fatalf("expected AdminNotes to be saved, got %q", mockRepo.spec.AdminNotes)
	}

	// 3. Admin rejects with notes: POST /admin/offers/101/reject
	formReject := url.Values{}
	formReject.Set("notes", "عرض لا يطابق الشروط التنظيمية")
	reqReject := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/admin/offers/101/reject", strings.NewReader(formReject.Encode()))
	reqReject.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqReject = reqReject.WithContext(authctx.WithActor(reqReject.Context(), adminActor))
	recReject := httptest.NewRecorder()
	r.ServeHTTP(recReject, reqReject)
	if recReject.Code != http.StatusSeeOther {
		t.Fatalf("expected status 303 for reject offer, got %d", recReject.Code)
	}
	if mockRepo.spec.AdminStatus != "rejected" {
		t.Fatalf("expected AdminStatus rejected, got %s", mockRepo.spec.AdminStatus)
	}
	if mockRepo.spec.AdminNotes != "عرض لا يطابق الشروط التنظيمية" {
		t.Fatalf("expected AdminNotes to be saved on reject, got %q", mockRepo.spec.AdminNotes)
	}

	// 4. Admin approves: POST /admin/offers/101/approve
	reqApprove := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/admin/offers/101/approve", nil)
	reqApprove = reqApprove.WithContext(authctx.WithActor(reqApprove.Context(), adminActor))
	recApprove := httptest.NewRecorder()
	r.ServeHTTP(recApprove, reqApprove)
	if recApprove.Code != http.StatusSeeOther {
		t.Fatalf("expected status 303 for approve offer, got %d", recApprove.Code)
	}
	if mockRepo.spec.AdminStatus != "approved" {
		t.Fatalf("expected AdminStatus approved, got %s", mockRepo.spec.AdminStatus)
	}

	// 5. Vendor edits the already approved offer: POST /vendor/offers/101/edit
	formEdit := url.Values{}
	formEdit.Set("title_ar", "عرض الصيف المعدل")
	formEdit.Set("title_en", "Summer Offer Updated")
	formEdit.Set("status", "active")
	formEdit.Set("total_price", "420.00")
	formEdit.Set("discount_percentage", "25")
	formEdit.Set("selected_variants", "26643")
	formEdit.Set("qty_26643", "5")
	formEdit.Set("custom_price_26643", "84.00")

	reqVendorEdit := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/vendor/offers/101/edit", strings.NewReader(formEdit.Encode()))
	reqVendorEdit.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqVendorEdit = reqVendorEdit.WithContext(authctx.WithActor(reqVendorEdit.Context(), vendorActor))
	recVendorEdit := httptest.NewRecorder()
	r.ServeHTTP(recVendorEdit, reqVendorEdit)
	if recVendorEdit.Code != http.StatusSeeOther {
		t.Fatalf("expected status 303 for vendor edit submit, got %d", recVendorEdit.Code)
	}
	if mockRepo.spec.AdminStatus != "pending" {
		t.Fatalf("expected AdminStatus to reset to pending after vendor edit, got %s", mockRepo.spec.AdminStatus)
	}

	// 6. Admin re-approves the updated offer: POST /admin/offers/101/approve
	reqReApprove := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/admin/offers/101/approve", nil)
	reqReApprove = reqReApprove.WithContext(authctx.WithActor(reqReApprove.Context(), adminActor))
	recReApprove := httptest.NewRecorder()
	r.ServeHTTP(recReApprove, reqReApprove)
	if recReApprove.Code != http.StatusSeeOther {
		t.Fatalf("expected status 303 for admin re-approve, got %d", recReApprove.Code)
	}
	if mockRepo.spec.AdminStatus != "approved" {
		t.Fatalf("expected AdminStatus approved after re-approval, got %s", mockRepo.spec.AdminStatus)
	}
}
func (m *mockPromoRepo) ListCreditAccounts(context.Context, string, int, int) ([]*promo.CreditAccount, int, error) {
	return nil, 0, nil
}
func (m *mockPromoRepo) ListPurchasesForOrg(context.Context, int64, int, int) ([]*promo.SponsorshipPurchase, int, error) {
	return nil, 0, nil
}
