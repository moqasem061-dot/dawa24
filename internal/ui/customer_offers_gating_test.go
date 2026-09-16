package ui_test

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/modules/promo"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/rbac"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
	"github.com/muhiya/dawa24-store/internal/ui"
)

type mockGatingPromoRepo struct {
	promo.Repository
	offers []*promo.SpecialOffer
	locs   map[int64][]*promo.SpecialOfferLocation
}

func (m *mockGatingPromoRepo) ListActiveOffers(_ context.Context, _, _ int) ([]*promo.Offer, error) {
	var out []*promo.Offer
	for _, o := range m.offers {
		s, e := time.Now().Add(-24*time.Hour), time.Now().Add(24*time.Hour)
		if o.StartDate != nil { s = *o.StartDate }
		if o.EndDate != nil { e = *o.EndDate }
		out = append(out, &promo.Offer{
			ID: o.ID, OrganizationID: o.OrganizationID, Title: o.Title,
			DiscountType: promo.DiscountPercentage, DiscountValue: money.FromMinor(int64(o.DiscountPercentage * 100)),
			StartsAt: s, ExpiresAt: e, AdminStatus: o.AdminStatus, ProductIDs: []int64{1, 2},
		})
	}
	return out, nil
}

// ListBuyerOffers and OfferVerdict emulate the SQL offer rule's live and
// city-coverage predicates, which promo/postgres tests against a real schema,
// so these tests exercise how the handlers use the rule.
func (m *mockGatingPromoRepo) ListBuyerOffers(_ context.Context, q promo.BuyerOfferQuery) ([]*promo.BuyerOffer, int, error) {
	var out []*promo.BuyerOffer
	for _, o := range m.offers {
		if !m.live(o) || (q.Buying && !m.covers(o.ID, q.Branch.CityID)) {
			continue
		}
		out = append(out, &promo.BuyerOffer{
			ID: o.ID, OrganizationID: o.OrganizationID, Title: o.Title, TotalPrice: o.TotalPrice,
			DiscountType: promo.DiscountPercentage, DiscountValue: money.FromMinor(int64(o.DiscountPercentage * 100)),
			StartsAt: o.StartDate, ExpiresAt: o.EndDate, ProductCount: len(o.Products),
		})
	}
	return out, len(out), nil
}

func (m *mockGatingPromoRepo) OfferVerdict(_ context.Context, q promo.BuyerOfferQuery) (promo.OfferVerdict, error) {
	for _, o := range m.offers {
		if o.ID == q.OfferID {
			return promo.OfferVerdict{Found: true, Live: m.live(o), SupplierOK: true, BranchOK: true,
				Institutional: true, Covered: m.covers(o.ID, q.Branch.CityID)}, nil
		}
	}
	return promo.OfferVerdict{}, nil
}

func (m *mockGatingPromoRepo) live(o *promo.SpecialOffer) bool {
	return o.Status == "active" && o.AdminStatus == "approved"
}

func (m *mockGatingPromoRepo) covers(offerID, cityID int64) bool {
	for _, l := range m.locs[offerID] {
		if l.CityID != nil && *l.CityID == cityID {
			return true
		}
	}
	return false
}

func (m *mockGatingPromoRepo) GetSpecialOfferByID(_ context.Context, id int64) (*promo.SpecialOffer, error) {
	for _, o := range m.offers {
		if o.ID == id { return o, nil }
	}
	return nil, fmt.Errorf("not found")
}

func (m *mockGatingPromoRepo) GetOfferByID(ctx context.Context, id int64) (*promo.Offer, error) {
	sp, err := m.GetSpecialOfferByID(ctx, id)
	if err != nil { return nil, err }
	return &promo.Offer{ID: sp.ID, OrganizationID: sp.OrganizationID, Title: sp.Title, AdminStatus: sp.AdminStatus}, nil
}

func (m *mockGatingPromoRepo) ListSpecialOfferLocations(_ context.Context, offerID int64) ([]*promo.SpecialOfferLocation, error) {
	return m.locs[offerID], nil
}

func (m *mockGatingPromoRepo) RankedSponsorshipsForOffers(_ context.Context, _ []int64) ([]*promo.RankedSponsorship, error) { return nil, nil }
func (m *mockGatingPromoRepo) IncrementOfferEngagement(_ context.Context, _ int64, _ bool) error { return nil }

type mockGatingOrgRepo struct {
	org.Repository
	branches map[int64]*org.Branch
}

func (m *mockGatingOrgRepo) GetBranchByID(_ context.Context, id int64) (*org.Branch, error) {
	if b, ok := m.branches[id]; ok { return b, nil }
	return nil, fmt.Errorf("branch not found")
}

func (m *mockGatingOrgRepo) GetBranchInstitutionalWorks(_ context.Context, _ int64) ([]*org.InstitutionalWork, error) {
	return []*org.InstitutionalWork{{ID: 1}}, nil
}

func (m *mockGatingOrgRepo) GetConnectedInstitutionalWorkIDs(_ context.Context, ids []int64) ([]int64, error) {
	return ids, nil
}

func (m *mockGatingOrgRepo) ListBranches(_ context.Context, orgID int64) ([]*org.Branch, error) {
	var out []*org.Branch
	for _, b := range m.branches {
		if b.OrganizationID == orgID { out = append(out, b) }
	}
	return out, nil
}

func (m *mockGatingOrgRepo) GetOrganizationByID(_ context.Context, id int64) (*org.Organization, error) {
	return &org.Organization{ID: id, LegalName: fmt.Sprintf("Vendor Org %d", id), Status: org.StatusApproved}, nil
}

type mockGatingProbe struct {
	variants map[int64]commerce.VariantAvailability
	vendors  map[int64]commerce.VendorAvailability
}

func (p *mockGatingProbe) Variant(_ context.Context, id int64) (commerce.VariantAvailability, error) {
	if v, ok := p.variants[id]; ok { return v, nil }
	return commerce.VariantAvailability{}, fmt.Errorf("variant %d not found", id)
}

func (p *mockGatingProbe) Vendor(_ context.Context, id int64) (commerce.VendorAvailability, error) {
	if v, ok := p.vendors[id]; ok { return v, nil }
	return commerce.VendorAvailability{ID: id, IsVendor: true, Approved: true}, nil
}

func (p *mockGatingProbe) CustomerBranch(_ context.Context, id int64) (commerce.BranchAvailability, error) {
	cairo := int64(1)
	lat, lon := 30.0444, 31.2357
	return commerce.BranchAvailability{
		ID: id, OrganizationID: 100, CityID: &cairo, Latitude: &lat, Longitude: &lon,
		InstitutionalWorks: []string{"pharmacy"},
	}, nil
}

func (p *mockGatingProbe) VendorCovers(_ context.Context, _, _ int64, _, _ float64, _ time.Weekday, _ *int64) (bool, error) { return true, nil }
func (p *mockGatingProbe) VendorInstitutionalConnection(_ context.Context, _, _, _ int64) (bool, error) { return true, nil }

func (p *mockGatingProbe) VariantsByIDs(_ context.Context, ids []int64) (map[int64]commerce.VariantAvailability, error) {
	out := make(map[int64]commerce.VariantAvailability, len(ids))
	for _, id := range ids {
		if v, ok := p.variants[id]; ok { out[id] = v }
	}
	return out, nil
}

func (p *mockGatingProbe) VendorsByIDs(_ context.Context, ids []int64) (map[int64]commerce.VendorAvailability, error) {
	out := make(map[int64]commerce.VendorAvailability, len(ids))
	for _, id := range ids { out[id] = commerce.VendorAvailability{ID: id, IsVendor: true, Approved: true} }
	return out, nil
}

func (p *mockGatingProbe) VendorInstitutionalConnections(_ context.Context, _ int64, lines []commerce.AvailabilityLine) (map[int64]bool, error) {
	out := make(map[int64]bool, len(lines))
	for _, l := range lines { out[l.VariantID] = true }
	return out, nil
}

type mockGatingCommRepo struct {
	commerce.Repository
	cart *commerce.Cart
}

func (m *mockGatingCommRepo) GetOrCreateCart(_ context.Context, userID int64) (*commerce.Cart, error) {
	if m.cart == nil {
		m.cart = &commerce.Cart{ID: 1, UserID: userID}
	}
	return m.cart, nil
}

func (m *mockGatingCommRepo) AddToCartItem(_ context.Context, cartID int64, item *commerce.CartItem) error {
	item.ID = int64(len(m.cart.Items) + 1)
	item.CartID = cartID
	m.cart.Items = append(m.cart.Items, item)
	return nil
}

func (m *mockGatingCommRepo) GetCartWithItems(_ context.Context, _ int64) (*commerce.Cart, error) {
	return m.cart, nil
}

func setupGatingTestFixture() (*ui.UIHandler, authctx.Actor) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	cairoCity := int64(1)
	alexCity := int64(2)

	now := time.Now()
	start := now.Add(-24 * time.Hour)
	end := now.Add(48 * time.Hour)

	// Offer 1: Covered (Cairo), Available (stock 100)
	offer1 := &promo.SpecialOffer{
		ID:                 101,
		OrganizationID:     20,
		Title:              i18n.New("عرض القاهرة المتوفر", "Cairo Available Offer"),
		DiscountPercentage: 15,
		TotalPrice:         money.FromMinor(10000),
		Status:             "active",
		AdminStatus:        "approved",
		StartDate:          &start,
		EndDate:            &end,
		Products: []*promo.SpecialOfferProduct{{ID: 1, VariantID: 1001, Quantity: 2, CustomPrice: money.FromMinor(5000)}},
	}

	// Offer 2: Out-of-coverage (Alexandria only, branch is in Cairo)
	offer2 := &promo.SpecialOffer{
		ID:                 102,
		OrganizationID:     20,
		Title:              i18n.New("عرض الإسكندرية خارج التغطية", "Alexandria Out of Coverage Offer"),
		DiscountPercentage: 20,
		TotalPrice:         money.FromMinor(15000),
		Status:             "active",
		AdminStatus:        "approved",
		StartDate:          &start,
		EndDate:            &end,
		Products: []*promo.SpecialOfferProduct{{ID: 2, VariantID: 1002, Quantity: 2, CustomPrice: money.FromMinor(7500)}},
	}

	// Offer 3: Covered (Cairo), but Unavailable (Stock 0)
	offer3 := &promo.SpecialOffer{
		ID:                 103,
		OrganizationID:     20,
		Title:              i18n.New("عرض القاهرة غير متوفر المخزون", "Cairo Out of Stock Offer"),
		DiscountPercentage: 25,
		TotalPrice:         money.FromMinor(8000),
		Status:             "active",
		AdminStatus:        "approved",
		StartDate:          &start,
		EndDate:            &end,
		Products: []*promo.SpecialOfferProduct{{ID: 3, VariantID: 1003, Quantity: 5, CustomPrice: money.FromMinor(1600)}},
	}

	locs := map[int64][]*promo.SpecialOfferLocation{
		101: {{ID: 1, OfferID: 101, CityID: &cairoCity, Status: "active"}},
		102: {{ID: 2, OfferID: 102, CityID: &alexCity, Status: "active"}},
		103: {{ID: 3, OfferID: 103, CityID: &cairoCity, Status: "active"}},
	}

	promoRepo := &mockGatingPromoRepo{
		offers: []*promo.SpecialOffer{offer1, offer2, offer3},
		locs:   locs,
	}
	promoSvc := promo.NewService(promoRepo, logger)

	branchCairo := &org.Branch{
		ID:             55,
		OrganizationID: 100,
		Name:           i18n.New("فرع القاهرة", "Cairo Branch"),
		CityID:         &cairoCity,
		Status:         "active",
	}

	orgRepo := &mockGatingOrgRepo{
		branches: map[int64]*org.Branch{55: branchCairo},
	}
	orgSvc := org.NewService(orgRepo, logger)

	probe := &mockGatingProbe{
		variants: map[int64]commerce.VariantAvailability{
			1001: {ID: 1001, OrganizationID: 20, StockQty: 100, MinOrderQty: 1, Active: true},
			1002: {ID: 1002, OrganizationID: 20, StockQty: 50, MinOrderQty: 1, Active: true},
			1003: {ID: 1003, OrganizationID: 20, StockQty: 0, MinOrderQty: 1, Active: true},
		},
		vendors: map[int64]commerce.VendorAvailability{
			20: {ID: 20, IsVendor: true, Approved: true},
		},
	}
	commSvc := commerce.NewService(&mockGatingCommRepo{}, logger)
	commSvc.SetAvailabilityProbe(probe)

	handler := ui.NewUIHandler(nil, orgSvc, nil, commSvc, nil, nil, nil, promoSvc, nil, nil, nil, nil, nil, nil, logger)

	buyerActor := authctx.Actor{
		UserID:         50,
		OrganizationID: 100,
		BranchID:       &branchCairo.ID,
		Role:           "customer",
		OrgType:        "customer",
		Scope:          rbac.ScopePharmacy,
	}

	return handler, buyerActor
}

// TestOffersPage_CoverageAndGating verifies that out-of-coverage offers are completely
// filtered out and hidden from buyers, while guest users can browse active offers.
func TestOffersPage_CoverageAndGating(t *testing.T) {
	handler, buyerActor := setupGatingTestFixture()

	req := httptest.NewRequest(http.MethodGet, "/offers", nil)
	ctx := authctx.WithActor(req.Context(), buyerActor)
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	handler.OffersPage(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rr.Code)
	}

	body := rr.Body.String()

	// 1. In-coverage Cairo offer must appear
	if !strings.Contains(body, "عرض القاهرة المتوفر") {
		t.Errorf("expected Cairo offer 101 to appear on offers page")
	}

	// 2. Out-of-coverage Alexandria offer must NOT appear from the start
	if strings.Contains(body, "عرض الإسكندرية خارج التغطية") {
		t.Errorf("out-of-coverage Alexandria offer 102 must NOT appear on offers page for Cairo buyer")
	}

	// 3. Out-of-coverage badge must NOT appear anywhere on the page
	if strings.Contains(body, "خارج التغطية") {
		t.Errorf("expected NO 'خارج التغطية' badge on offers page")
	}

	// 4. Covered badge is displayed for the covered offer
	if !strings.Contains(body, "مشمول بالتغطية") {
		t.Errorf("expected 'مشمول بالتغطية' badge for covered offer")
	}

	// 5. Guest user (not logged in as a buyer) can still browse active offers
	guestReq := httptest.NewRequest(http.MethodGet, "/offers", nil)
	guestRR := httptest.NewRecorder()
	handler.OffersPage(guestRR, guestReq)
	if guestRR.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for guest, got %d", guestRR.Code)
	}
	guestBody := guestRR.Body.String()
	if !strings.Contains(guestBody, "عرض القاهرة المتوفر") {
		t.Errorf("expected Cairo offer to appear for guest")
	}
	if !strings.Contains(guestBody, "عرض الإسكندرية خارج التغطية") {
		t.Errorf("expected Alexandria offer to appear for guest browsing public catalog")
	}
}

func TestOfferDetailPage_CoverageAndCartAction(t *testing.T) {
	handler, buyerActor := setupGatingTestFixture()

	// Subtest 1: In-coverage offer opens with green banner and enabled cart button
	t.Run("covered offer displays success alert and enabled button", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/offers/101", nil)
		ctx := authctx.WithActor(req.Context(), buyerActor)
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", "101")
		req = req.WithContext(context.WithValue(ctx, chi.RouteCtxKey, rctx))

		rr := httptest.NewRecorder()
		handler.OfferDetailPage(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", rr.Code)
		}
		body := rr.Body.String()
		if !strings.Contains(body, "عرض القاهرة المتوفر") {
			t.Errorf("expected offer title on detail page")
		}
		if !strings.Contains(body, "فرع صيدليتك مشمول في نطاق توصيل هذا العرض") {
			t.Errorf("expected covered banner on detail page")
		}
		if !strings.Contains(body, "إضافة العرض إلى السلة والشراء الآن") {
			t.Errorf("expected enabled add-to-cart button")
		}
	})

	// Subtest 2: Out-of-coverage offer opens with warning banner and disabled button
	t.Run("out-of-coverage offer displays warning alert and disabled button", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/offers/102", nil)
		ctx := authctx.WithActor(req.Context(), buyerActor)
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", "102")
		req = req.WithContext(context.WithValue(ctx, chi.RouteCtxKey, rctx))

		rr := httptest.NewRecorder()
		handler.OfferDetailPage(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", rr.Code)
		}
		body := rr.Body.String()
		if !strings.Contains(body, "عرض الإسكندرية خارج التغطية") {
			t.Errorf("expected offer title on detail page")
		}
		if !strings.Contains(body, "فرع صيدليتك خارج نطاق التغطية الجغرافية لهذا العرض") {
			t.Errorf("expected out-of-coverage warning banner")
		}
		if !strings.Contains(body, "غير متاح لفرعك (خارج التغطية)") {
			t.Errorf("expected disabled add-to-cart button")
		}
	})
}

func TestOfferAddToCart_CoverageAndAvailabilityGating(t *testing.T) {
	handler, buyerActor := setupGatingTestFixture()

	// Subtest 1: Out-of-coverage bundle cannot be added to cart
	t.Run("out-of-coverage offer rejected at cart-add", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/cart/add-offer", strings.NewReader("offer_id=102&quantity=1"))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		ctx := authctx.WithActor(req.Context(), buyerActor)
		req = req.WithContext(ctx)

		rr := httptest.NewRecorder()
		handler.AddOfferToCartSubmit(rr, req)

		if rr.Code != http.StatusSeeOther {
			t.Fatalf("expected 303 redirect, got %d", rr.Code)
		}
		loc := rr.Header().Get("Location")
		if !strings.Contains(loc, "notice=error") {
			t.Errorf("expected error notice on redirect, got %s", loc)
		}
	})

	// Subtest 2: Products inside offer do not gate cart-add (bundle is ordered as promotional unit)
	t.Run("in-coverage offer succeeds at cart-add regardless of catalog stock", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/cart/add-offer", strings.NewReader("offer_id=103&quantity=1"))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		ctx := authctx.WithActor(req.Context(), buyerActor)
		req = req.WithContext(ctx)

		rr := httptest.NewRecorder()
		handler.AddOfferToCartSubmit(rr, req)

		if rr.Code != http.StatusSeeOther {
			t.Fatalf("expected 303 redirect, got %d", rr.Code)
		}
		loc := rr.Header().Get("Location")
		if !strings.Contains(loc, "notice=success") {
			t.Errorf("expected success notice on redirect, got %s", loc)
		}
	})
}
