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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/muhiya/dawa24-store/internal/modules/promo"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
	"github.com/muhiya/dawa24-store/internal/ui"
)

// mockAdminPromoRepo is an in-memory test double for promo.Service admin operations.
type mockAdminPromoRepo struct {
	promo.Repository
	packages []*promo.OfferPackage
	requests []*promo.SponsorshipRequest
	ads      []*promo.Ad
}

func newMockAdminPromoRepo() *mockAdminPromoRepo {
	return &mockAdminPromoRepo{
		packages: []*promo.OfferPackage{
			{
				ID:           1,
				PublicID:     "pkg_gold",
				Name:         i18n.New("الباقة الذهبية", "Gold Package"),
				Description:  i18n.New("تثبيت 10 أصناف في الصدارة", "Pin 10 items in top results"),
				Price:        money.FromMajor(1500),
				DurationDays: 30,
				Credits:      10,
				MaxOffers:    20,
				TierLevel:    3,
				SortOrder:    1,
				BadgeColor:   "#f59e0b",
				IsFeatured:   true,
				IsActive:     true,
				CreatedAt:    time.Now(),
				UpdatedAt:    time.Now(),
			},
		},
		requests: []*promo.SponsorshipRequest{
			{
				ID:             101,
				OrganizationID: 42,
				PackageID:      1,
				ItemType:       "product",
				ItemID:         901,
				CreditsUsed:    1,
				AdminStatus:    promo.AdminPending,
				CreatedAt:      time.Now(),
			},
			{
				ID:             102,
				OrganizationID: 43,
				PackageID:      1,
				ItemType:       "special_offer",
				ItemID:         902,
				CreditsUsed:    1,
				AdminStatus:    promo.AdminPending,
				CreatedAt:      time.Now(),
			},
		},
		ads: []*promo.Ad{
			{
				ID:          501,
				Title:       "بنر الأدوية الترويجية",
				TitleAr:     "بنر الأدوية الترويجية",
				TitleEn:     "Promo Medicines Banner",
				Position:    "home_top",
				AdminStatus: promo.AdminPending,
				Impressions: 1200,
				Clicks:      85,
				CTR:         7.08,
				CreatedAt:   time.Now(),
			},
		},
	}
}

func (m *mockAdminPromoRepo) CreatePackage(ctx context.Context, p *promo.OfferPackage) error {
	p.ID = int64(len(m.packages) + 1)
	p.PublicID = "pkg_auto"
	p.CreatedAt = time.Now()
	p.UpdatedAt = time.Now()
	m.packages = append(m.packages, p)
	return nil
}

func (m *mockAdminPromoRepo) UpdatePackage(ctx context.Context, p *promo.OfferPackage) error {
	for i, existing := range m.packages {
		if existing.ID == p.ID {
			m.packages[i] = p
			return nil
		}
	}
	return nil
}

func (m *mockAdminPromoRepo) GetPackageByID(ctx context.Context, id int64) (*promo.OfferPackage, error) {
	for _, p := range m.packages {
		if p.ID == id {
			return p, nil
		}
	}
	return nil, nil
}

func (m *mockAdminPromoRepo) AdminListPackages(ctx context.Context) ([]*promo.OfferPackage, error) {
	return m.packages, nil
}

func (m *mockAdminPromoRepo) ListPackages(ctx context.Context) ([]*promo.OfferPackage, error) {
	var list []*promo.OfferPackage
	for _, p := range m.packages {
		if p.IsActive {
			list = append(list, p)
		}
	}
	return list, nil
}

func (m *mockAdminPromoRepo) TogglePackageActive(ctx context.Context, id int64, active bool) error {
	for _, p := range m.packages {
		if p.ID == id {
			p.IsActive = active
			return nil
		}
	}
	return nil
}

func (m *mockAdminPromoRepo) GetSponsorshipRequestByID(ctx context.Context, id int64) (*promo.SponsorshipRequest, error) {
	for _, r := range m.requests {
		if r.ID == id {
			return r, nil
		}
	}
	return nil, nil
}

func (m *mockAdminPromoRepo) ListAllSponsorshipRequests(ctx context.Context, limit, offset int) ([]*promo.SponsorshipRequest, error) {
	return m.requests, nil
}

func (m *mockAdminPromoRepo) ListAllSponsorshipRequestsWithTotal(ctx context.Context, limit, offset int) ([]*promo.SponsorshipRequest, int, error) {
	return m.requests, len(m.requests), nil
}

func (m *mockAdminPromoRepo) ListSponsorshipRequestsByOrgWithTotal(ctx context.Context, orgID int64, limit, offset int) ([]*promo.SponsorshipRequest, int, error) {
	return m.requests, len(m.requests), nil
}

func (m *mockAdminPromoRepo) ActivateSponsorshipRequest(ctx context.Context, requestID int64, reviewerID int64) (*promo.SponsorshipRequest, error) {
	for _, r := range m.requests {
		if r.ID == requestID {
			r.AdminStatus = promo.AdminApproved
			return r, nil
		}
	}
	return nil, nil
}

func (m *mockAdminPromoRepo) UpdateSponsorshipRequestAdminStatus(ctx context.Context, requestID int64, status promo.AdminStatus, notes string, reviewerID int64) error {
	for _, r := range m.requests {
		if r.ID == requestID {
			r.AdminStatus = status
			return nil
		}
	}
	return nil
}

func (m *mockAdminPromoRepo) AdminListAds(ctx context.Context, limit, offset int) ([]*promo.Ad, error) {
	return m.ads, nil
}

func (m *mockAdminPromoRepo) ListAllAds(ctx context.Context, limit, offset int) ([]*promo.Ad, error) {
	return m.ads, nil
}

func (m *mockAdminPromoRepo) ListAllAdsWithTotal(ctx context.Context, limit, offset int) ([]*promo.Ad, int, error) {
	return m.ads, len(m.ads), nil
}

func (m *mockAdminPromoRepo) ListAdsByOrgWithTotal(ctx context.Context, orgID int64, limit, offset int) ([]*promo.Ad, int, error) {
	return m.ads, len(m.ads), nil
}

func (m *mockAdminPromoRepo) UpdateAdAdminStatus(ctx context.Context, id int64, status promo.AdminStatus, notes string, reviewerID int64) error {
	for _, a := range m.ads {
		if a.ID == id {
			a.AdminStatus = status
			return nil
		}
	}
	return nil
}

func setupTestPromoHandler(repo promo.Repository) (*ui.UIHandler, *chi.Mux) {
	promoSvc := promo.NewService(repo, slog.Default())
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := ui.NewUIHandler(nil, nil, nil, nil, nil, nil, nil, promoSvc, nil, nil, nil, nil, nil, nil, logger)

	r := chi.NewRouter()
	r.Get("/admin/offers-packages", handler.AdminOffersPackagesHubPage)
	r.Post("/admin/offers-packages/new", handler.AdminOfferPackageCreateSubmit)
	r.Post("/admin/offers-packages/{id}/edit", handler.AdminOfferPackageEditSubmit)
	r.Post("/admin/offers-packages/{id}/toggle", handler.AdminOfferPackageToggleSubmit)
	r.Post("/admin/offers-packages/sponsorships/{id}/approve", handler.AdminSponsorshipRequestApproveSubmit)
	r.Post("/admin/offers-packages/sponsorships/{id}/reject", handler.AdminSponsorshipRequestRejectSubmit)
	r.Post("/admin/ads/{id}/approve", handler.AdminAdApproveSubmit)
	r.Post("/admin/ads/{id}/reject", handler.AdminAdRejectSubmit)
	r.Get("/admin/ad-plan", handler.AdminAdPlansPage)
	r.Get("/admin/offers-packages/packages", handler.AdminOfferPackagesListPage)

	return handler, r
}

func adminContext(req *http.Request) *http.Request {
	actor := authctx.Actor{
		UserID:  1,
		Role:    "admin",
		IsStaff: true,
		IsOwner: true,
	}
	return req.WithContext(authctx.WithActor(req.Context(), actor))
}

func TestAdminOffersPackages_HubPageRenders(t *testing.T) {
	repo := newMockAdminPromoRepo()
	_, r := setupTestPromoHandler(repo)

	req := httptest.NewRequest(http.MethodGet, "/admin/offers-packages?tab=packages", nil)
	req = adminContext(req)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, "منظومة باقات الرعايات والعروض الترويجية")
	assert.Contains(t, body, "الباقة الذهبية")
	assert.Contains(t, body, "باقات الرعايات المتاحة")
	assert.Contains(t, body, "مراجعة طلبات الرعاية")
	assert.Contains(t, body, "مراجعة الإعلانات والبنرات")
}

func TestAdminOffersPackages_CreateNewPackage(t *testing.T) {
	repo := newMockAdminPromoRepo()
	_, r := setupTestPromoHandler(repo)

	form := url.Values{}
	form.Set("name_ar", "الباقة الماسية المميزة")
	form.Set("name_en", "Diamond VIP Package")
	form.Set("desc_ar", "تثبيت 25 صنف في الصدارة")
	form.Set("desc_en", "Pin 25 items at top")
	form.Set("price", "3500.00")
	form.Set("duration_days", "60")
	form.Set("credits", "25")
	form.Set("tier_level", "5")
	form.Set("max_offers", "50")
	form.Set("badge_color", "#8b5cf6")
	form.Set("is_featured", "true")
	form.Set("is_active", "true")

	req := httptest.NewRequest(http.MethodPost, "/admin/offers-packages/new", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = adminContext(req)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusSeeOther, rec.Code)
	loc := rec.Header().Get("Location")
	assert.Contains(t, loc, "/admin/offers-packages")
	assert.Contains(t, loc, "tab=packages")
	assert.Contains(t, loc, "notice=success")

	require.Len(t, repo.packages, 2)
	created := repo.packages[1]
	assert.Equal(t, "الباقة الماسية المميزة", created.Name.Get(i18n.AR))
	assert.Equal(t, "Diamond VIP Package", created.Name.Get(i18n.EN))
	assert.Equal(t, 5, created.TierLevel)
	assert.Equal(t, 25, created.Credits)
	assert.True(t, created.IsFeatured)
	assert.True(t, created.IsActive)
}

func TestAdminOffersPackages_EditPackage(t *testing.T) {
	repo := newMockAdminPromoRepo()
	_, r := setupTestPromoHandler(repo)

	form := url.Values{}
	form.Set("name_ar", "الباقة الذهبية المعدلة")
	form.Set("name_en", "Updated Gold Package")
	form.Set("desc_ar", "وصف محدث")
	form.Set("desc_en", "Updated desc")
	form.Set("price", "1800.00")
	form.Set("duration_days", "45")
	form.Set("credits", "15")
	form.Set("tier_level", "4")
	form.Set("max_offers", "30")
	form.Set("badge_color", "#0284c7")
	form.Set("is_featured", "false")
	form.Set("is_active", "true")

	req := httptest.NewRequest(http.MethodPost, "/admin/offers-packages/1/edit", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = adminContext(req)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusSeeOther, rec.Code)
	loc := rec.Header().Get("Location")
	assert.Contains(t, loc, "/admin/offers-packages")
	assert.Contains(t, loc, "tab=packages")
	assert.Contains(t, loc, "notice=success")

	updated := repo.packages[0]
	assert.Equal(t, "الباقة الذهبية المعدلة", updated.Name.Get(i18n.AR))
	assert.Equal(t, 15, updated.Credits)
	assert.Equal(t, 4, updated.TierLevel)
}

func TestAdminOffersPackages_ToggleActive(t *testing.T) {
	repo := newMockAdminPromoRepo()
	_, r := setupTestPromoHandler(repo)

	form := url.Values{}
	form.Set("active", "false")

	req := httptest.NewRequest(http.MethodPost, "/admin/offers-packages/1/toggle", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = adminContext(req)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusSeeOther, rec.Code)
	assert.False(t, repo.packages[0].IsActive)
}

func TestAdminOffersPackages_LegacyAdPlanRedirect(t *testing.T) {
	repo := newMockAdminPromoRepo()
	_, r := setupTestPromoHandler(repo)

	req := httptest.NewRequest(http.MethodGet, "/admin/ad-plan", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusMovedPermanently, rec.Code)
	assert.Equal(t, "/admin/offers-packages?tab=packages", rec.Header().Get("Location"))
}

func TestAdminOffersPackages_SponsorshipApproveAndReject(t *testing.T) {
	repo := newMockAdminPromoRepo()
	_, r := setupTestPromoHandler(repo)

	// Approve request 101
	req := httptest.NewRequest(http.MethodPost, "/admin/offers-packages/sponsorships/101/approve", nil)
	req = adminContext(req)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusSeeOther, rec.Code)
	assert.Contains(t, rec.Header().Get("Location"), "tab=requests")
	assert.Equal(t, promo.AdminApproved, repo.requests[0].AdminStatus)

	// Reject request 102
	reqReject := httptest.NewRequest(http.MethodPost, "/admin/offers-packages/sponsorships/102/reject", strings.NewReader("notes=does+not+comply"))
	reqReject.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqReject = adminContext(reqReject)
	recReject := httptest.NewRecorder()
	r.ServeHTTP(recReject, reqReject)

	assert.Equal(t, http.StatusSeeOther, recReject.Code)
	assert.Contains(t, recReject.Header().Get("Location"), "tab=requests")
	assert.Equal(t, promo.AdminRejected, repo.requests[1].AdminStatus)
}
