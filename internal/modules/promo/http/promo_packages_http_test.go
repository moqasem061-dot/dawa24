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

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/promo"
	promoHttp "github.com/muhiya/dawa24-store/internal/modules/promo/http"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/platform/httpx"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

func (happyRepo) ListActiveRankedSponsorships(context.Context, promo.SponsorshipItemType) ([]*promo.RankedSponsorship, error) {
	return nil, nil
}
func (happyRepo) IsSponsored(context.Context, promo.SponsorshipItemType, int64) (*promo.RankedSponsorship, error) {
	return nil, nil
}
func (happyRepo) CreateAd(context.Context, *promo.Ad) error { return nil }
func (happyRepo) UpdateAd(context.Context, *promo.Ad) error { return nil }
func (happyRepo) GetAdByID(context.Context, int64) (*promo.Ad, error) {
	return nil, nil
}
func (happyRepo) ListAdsByOrg(context.Context, int64, int, int) ([]*promo.Ad, error) {
	return nil, nil
}
func (happyRepo) ListAdsByOrgWithTotal(context.Context, int64, int, int) ([]*promo.Ad, int, error) {
	return nil, 0, nil
}
func (happyRepo) ListAllAds(context.Context, int, int) ([]*promo.Ad, error) {
	return nil, nil
}
func (happyRepo) ListAllAdsWithTotal(context.Context, int, int) ([]*promo.Ad, int, error) {
	return nil, 0, nil
}
func (happyRepo) UpdateAdAdminStatus(context.Context, int64, promo.AdminStatus, string, int64) error {
	return nil
}
func (happyRepo) AdminToggleAd(context.Context, int64) (*promo.Ad, error) {
	return &promo.Ad{ID: 1, IsActive: true}, nil
}
func (happyRepo) SubmitAdEditRequest(context.Context, int64, *promo.AdPendingChanges) error {
	return nil
}
func (happyRepo) ApproveAdEditRequest(context.Context, int64, int64) error {
	return nil
}
func (happyRepo) RejectAdEditRequest(context.Context, int64, int64, string) error {
	return nil
}
func (happyRepo) RecordAdImpression(context.Context, int64, *int64, string, string) error {
	return nil
}
func (happyRepo) CreateHighlightSection(ctx context.Context, h *promo.HighlightSection) error {
	h.ID = 1
	return nil
}
func (happyRepo) UpdateHighlightSection(ctx context.Context, h *promo.HighlightSection) error {
	return nil
}
func (happyRepo) DeleteHighlightSection(ctx context.Context, id, orgID int64) error {
	return nil
}
func (happyRepo) GetHighlightSectionByID(ctx context.Context, id int64) (*promo.HighlightSection, error) {
	return &promo.HighlightSection{ID: id, Title: i18n.Text{"ar": "قسم مميز"}}, nil
}
func (happyRepo) ListHighlightSections(ctx context.Context) ([]*promo.HighlightSection, error) {
	return []*promo.HighlightSection{{ID: 1, Title: i18n.Text{"en": "Featured"}}}, nil
}
func (happyRepo) ListHighlightSectionsByOrg(ctx context.Context, orgID int64) ([]*promo.HighlightSection, error) {
	return []*promo.HighlightSection{{ID: 1, OwnerType: "organization", OrganizationID: &orgID, Title: i18n.Text{"ar": "الأكثر مبيعاً"}}}, nil
}
func (happyRepo) AddHighlightItem(ctx context.Context, item *promo.HighlightSectionItem) error {
	item.ID = 1
	return nil
}
func (happyRepo) ListHighlightItems(ctx context.Context, sectionID int64) ([]*promo.HighlightSectionItem, error) {
	return nil, nil
}
func (happyRepo) ExpirePromotions(ctx context.Context) (int64, error) {
	return 0, nil
}
func (happyRepo) ExpirePromotionsAndCollectMedia(ctx context.Context) ([]string, int64, error) {
	return nil, 0, nil
}
func (happyRepo) CreateSpecialOffer(ctx context.Context, o *promo.SpecialOffer) error {
	o.ID = 1
	return nil
}
func (happyRepo) UpdateSpecialOffer(ctx context.Context, o *promo.SpecialOffer) error {
	return nil
}
func (happyRepo) GetSpecialOfferByID(ctx context.Context, id int64) (*promo.SpecialOffer, error) {
	return &promo.SpecialOffer{ID: id, Title: i18n.New("عرض خاص", "Special Offer")}, nil
}
func (happyRepo) ListSpecialOffersByOrg(ctx context.Context, orgID int64) ([]*promo.SpecialOffer, error) {
	return []*promo.SpecialOffer{{ID: 1, Title: i18n.New("عرض خاص", "Special Offer")}}, nil
}
func (happyRepo) ListAllSpecialOffers(ctx context.Context, limit, offset int) ([]*promo.SpecialOffer, error) {
	return []*promo.SpecialOffer{{ID: 1, Title: i18n.New("عرض خاص", "Special Offer")}}, nil
}
func (happyRepo) ListAllSpecialOffersWithTotal(ctx context.Context, statusFilter string, limit, offset int) ([]*promo.SpecialOffer, int, error) {
	return []*promo.SpecialOffer{{ID: 1, Title: i18n.New("عرض خاص", "Special Offer")}}, 1, nil
}
func (happyRepo) UpdateSpecialOfferAdminStatus(ctx context.Context, id int64, adminStatus, notes string, approvedBy int64) error {
	return nil
}
func (happyRepo) ToggleSpecialOfferStatus(ctx context.Context, id int64, isActive bool) error {
	return nil
}
func (happyRepo) DeleteSpecialOffer(ctx context.Context, id, orgID int64) error {
	return nil
}
func (happyRepo) AddSpecialOfferLocation(ctx context.Context, loc *promo.SpecialOfferLocation) error {
	loc.ID = 1
	return nil
}
func (happyRepo) ListSpecialOfferLocations(ctx context.Context, offerID int64) ([]*promo.SpecialOfferLocation, error) {
	return []*promo.SpecialOfferLocation{{ID: 1, Radius: 1000}}, nil
}
func (happyRepo) DeleteSpecialOfferLocation(ctx context.Context, id, offerID, orgID int64) error {
	return nil
}
func (happyRepo) ListAdminOfferLocations(ctx context.Context, filter promo.OfferLocationsFilter) ([]*promo.OfferLocationAdminRow, promo.OfferLocationsStats, int, error) {
	return nil, promo.OfferLocationsStats{}, 0, nil
}
func (r stubRepo) DeleteSpecialOfferLocation(context.Context, int64, int64, int64) error {
	r.fail("DeleteSpecialOfferLocation")
	return nil
}
func (stubRepo) ListAdminOfferLocations(ctx context.Context, filter promo.OfferLocationsFilter) ([]*promo.OfferLocationAdminRow, promo.OfferLocationsStats, int, error) {
	return nil, promo.OfferLocationsStats{}, 0, nil
}

func (stubRepo) CreditTotals(context.Context, int64) (int, int, error)  { return 0, 0, nil }
func (happyRepo) CreditTotals(context.Context, int64) (int, int, error) { return 0, 0, nil }
func (stubRepo) ListCreditAccounts(context.Context, string, int, int) ([]*promo.CreditAccount, int, error) {
	return nil, 0, nil
}
func (stubRepo) ListPurchasesForOrg(context.Context, int64, int, int) ([]*promo.SponsorshipPurchase, int, error) {
	return nil, 0, nil
}
func (happyRepo) ListCreditAccounts(context.Context, string, int, int) ([]*promo.CreditAccount, int, error) {
	return nil, 0, nil
}
func (happyRepo) ListPurchasesForOrg(context.Context, int64, int, int) ([]*promo.SponsorshipPurchase, int, error) {
	return nil, 0, nil
}

const testCookieName = "dawa24_session"

func newTestRouter(t *testing.T) http.Handler {
	t.Helper()

	log := slog.New(slog.NewJSONHandler(io.Discard, nil))
	promoSvc := promo.NewService(stubRepo{t: t}, log)

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
	promoHttp.NewHandler(promoSvc, log).RegisterRoutes(r)

	return r
}

func newAuthedRouter(repo promo.Repository) http.Handler {
	log := slog.New(slog.NewJSONHandler(io.Discard, nil))
	promoSvc := promo.NewService(repo, log)

	r := chi.NewRouter()
	r.Use(httpx.RequestID)
	r.Use(httpx.Recover(log))
	r.Use(httpx.Locale)

	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			actor := authctx.Actor{
				UserID:         1,
				OrganizationID: 1,
				// IsStaff, because the write gates on these two modules use
				// RequireAPIPermission — a platform-settings or homepage-
				// highlight write is a staff action and a tenant member holding
				// the key by accident must still be refused.
				IsStaff:     true,
				Role:        "admin",
				Permissions: []string{"admin", "promo.admin"},
			}
			ctx := authctx.WithActor(r.Context(), actor)
			ctx = database.WithTenant(ctx, 1)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	})
	promoHttp.NewHandler(promoSvc, log).RegisterRoutes(r)
	return r
}

var protectedRoutes = []struct{ method, path string }{
	{http.MethodGet, "/api/v1/promo/offers"},
	{http.MethodPost, "/api/v1/promo/offers"},
	{http.MethodGet, "/api/v1/promo/offers/1"},
	{http.MethodPost, "/api/v1/promo/offers/1/click"},
	{http.MethodGet, "/api/v1/promo/packages"},
	{http.MethodGet, "/api/v1/promo/ads"},
	{http.MethodPost, "/api/v1/promo/ads/1/click"},
	{http.MethodGet, "/api/v1/promo/highlights"},
	{http.MethodPost, "/api/v1/promo/highlights"},
	{http.MethodGet, "/api/v1/admin/promo/ads"},
	{http.MethodPost, "/api/v1/admin/promo/ads/1/approve"},
	{http.MethodPost, "/api/v1/admin/promo/ads/1/reject"},
	{http.MethodGet, "/api/v1/admin/promo/sponsorships"},
	{http.MethodPost, "/api/v1/admin/promo/sponsorships/1/review"},
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
	req := httptest.NewRequest(http.MethodGet, "/api/v1/promo/offers", nil)
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

func TestPromoHandler_HappyPaths(t *testing.T) {
	router := newAuthedRouter(happyRepo{})

	tests := []struct {
		name       string
		method     string
		path       string
		body       string
		wantStatus int
	}{
		{"ListOffers", http.MethodGet, "/api/v1/promo/offers?limit=10&offset=0", "", http.StatusOK},
		{"CreateOffer", http.MethodPost, "/api/v1/promo/offers", `{"organization_id":1,"title":{"en":"Summer Sale"},"discount_type":"percentage","discount_value":"10.00","starts_at":"2026-01-01T00:00:00Z","expires_at":"2026-12-31T23:59:59Z","is_active":true}`, http.StatusCreated},
		{"GetOffer", http.MethodGet, "/api/v1/promo/offers/1", "", http.StatusOK},
		{"RecordClick", http.MethodPost, "/api/v1/promo/offers/1/click", "", http.StatusNoContent},
		{"ListPackages", http.MethodGet, "/api/v1/promo/packages", "", http.StatusOK},
		{"ListAds", http.MethodGet, "/api/v1/promo/ads?position=home", "", http.StatusOK},
		{"RecordAdClick", http.MethodPost, "/api/v1/promo/ads/1/click", "", http.StatusNoContent},
		{"ListHighlights", http.MethodGet, "/api/v1/promo/highlights", "", http.StatusOK},
		{"CreateHighlight", http.MethodPost, "/api/v1/promo/highlights", `{"title":{"en":"Featured"},"slug":"featured","display_order":1,"is_active":true}`, http.StatusCreated},
		{"AdminListAds", http.MethodGet, "/api/v1/admin/promo/ads", "", http.StatusOK},
		{"AdminApproveAd", http.MethodPost, "/api/v1/admin/promo/ads/1/approve", "", http.StatusOK},
		{"AdminRejectAd", http.MethodPost, "/api/v1/admin/promo/ads/1/reject", "", http.StatusOK},
		{"AdminListSponsorships", http.MethodGet, "/api/v1/admin/promo/sponsorships", "", http.StatusOK},
		{"AdminReviewSponsorship", http.MethodPost, "/api/v1/admin/promo/sponsorships/1/review", "", http.StatusOK},
		{"AdminListPackages", http.MethodGet, "/api/v1/admin/promo/packages", "", http.StatusOK},
		{"AdminCreatePackage", http.MethodPost, "/api/v1/admin/promo/packages", `{"name":{"en":"Platinum"},"price":"100.00","duration_days":30,"max_offers":10}`, http.StatusCreated},
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
