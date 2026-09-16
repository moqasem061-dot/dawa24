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

func (m *mockAdminPromoRepo) ListAdminSponsorshipRows(ctx context.Context, f promo.AdminSponsorshipFilter) ([]*promo.AdminSponsorshipRow, int, error) {
	now := time.Now()
	expires := now.Add(30 * 24 * time.Hour)
	purchaseID := int64(88)
	rows := []*promo.AdminSponsorshipRow{
		{
			ID:               101,
			PublicID:         "SR-101",
			OrganizationID:   42,
			OrganizationName: i18n.New("صيدلية النور", "Al-Noor"),
			OrganizationType: "pharmacy",
			ProductID:        901,
			ProductName:      i18n.New("بانادول إكسترا", "Panadol Extra"),
			ProductSKU:       "PAN-01",
			PackageID:        1,
			PackageName:      i18n.New("الباقة الذهبية", "Gold Package"),
			TierLevel:        3,
			PurchaseID:       &purchaseID,
			ItemID:           901,
			CreditsUsed:      1,
			CreditsTotal:     10,
			Amount:           money.FromMajor(150),
			AdminStatus:      string(m.requests[0].AdminStatus),
			Status:           "active",
			StartsAt:         &now,
			ExpiresAt:        &expires,
			CreatedAt:        now,
			Impressions:      1250,
			Clicks:           95,
		},
	}
	return rows, len(rows), nil
}

func (m *mockAdminPromoRepo) AdminSponsorshipCounts(ctx context.Context, f promo.AdminSponsorshipFilter) (promo.AdminSponsorshipCounts, error) {
	return promo.AdminSponsorshipCounts{
		Total:    1,
		Active:   1,
		Pending:  0,
		Expired:  0,
		Rejected: 0,
	}, nil
}

func (m *mockAdminPromoRepo) GetAdminSponsorshipRow(ctx context.Context, id int64) (*promo.AdminSponsorshipRow, error) {
	now := time.Now()
	expires := now.Add(30 * 24 * time.Hour)
	purchaseID := int64(88)
	return &promo.AdminSponsorshipRow{
		ID:               id,
		PublicID:         "SR-101",
		OrganizationID:   42,
		OrganizationName: i18n.New("صيدلية النور", "Al-Noor"),
		OrganizationType: "pharmacy",
		ProductID:        901,
		ProductName:      i18n.New("بانادول إكسترا", "Panadol Extra"),
		ProductSKU:       "PAN-01",
		PackageID:        1,
		PackageName:      i18n.New("الباقة الذهبية", "Gold Package"),
		TierLevel:        3,
		PurchaseID:       &purchaseID,
		ItemID:           901,
		CreditsUsed:      1,
		CreditsTotal:     10,
		Amount:           money.FromMajor(150),
		AdminStatus:      string(m.requests[0].AdminStatus),
		Status:           "active",
		StartsAt:         &now,
		ExpiresAt:        &expires,
		CreatedAt:        now,
		Impressions:      1250,
		Clicks:           95,
	}, nil
}

func (m *mockAdminPromoRepo) GetSponsorshipPurchaseByID(ctx context.Context, id int64) (*promo.SponsorshipPurchase, error) {
	return &promo.SponsorshipPurchase{
		ID:               id,
		PublicID:         "PUR-88",
		OrganizationID:   42,
		PackageID:        1,
		CreditsTotal:     10,
		CreditsUsed:      1,
		CreditsRemaining: 9,
		Amount:           money.FromMajor(150),
		Status:           promo.PurchaseActive,
		CreatedAt:        time.Now(),
	}, nil
}

func (m *mockAdminPromoRepo) GetAdminSponsorshipCreditEntries(ctx context.Context, purchaseID, requestID int64) ([]*promo.CreditEntry, error) {
	return []*promo.CreditEntry{
		{
			ID:             1,
			PublicID:       "TX-001",
			OrganizationID: 42,
			PurchaseID:     purchaseID,
			Delta:          -1,
			BalanceAfter:   9,
			Reason:         promo.CreditSponsorshipRequested,
			Note:           "خصم رصيد تثبيت منتج",
			CreatedAt:      time.Now(),
		},
	}, nil
}

func (m *mockAdminPromoRepo) GetAdminSponsorshipEvents(ctx context.Context, itemID int64) ([]*promo.AdminSponsorshipEvent, error) {
	return []*promo.AdminSponsorshipEvent{
		{
			EventType: "impression",
			ID:        1001,
			IPAddress: "192.168.1.1",
			Detail:    "top_search",
			CreatedAt: time.Now(),
		},
		{
			EventType: "click",
			ID:        2001,
			IPAddress: "192.168.1.2",
			Detail:    "catalog_card",
			CreatedAt: time.Now(),
		},
	}, nil
}

func TestAdminAdvProducts_PageRenders(t *testing.T) {
	mockRepo := newMockAdminPromoRepo()
	promoSvc := promo.NewService(mockRepo, slog.Default())
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := ui.NewUIHandler(nil, nil, nil, nil, nil, nil, nil, promoSvc, nil, nil, nil, nil, nil, nil, logger)

	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			actor := authctx.Actor{
				UserID:      1,
				IsStaff:     true,
				Role:        "super_admin",
				Permissions: []string{"*"},
			}
			ctx := authctx.WithActor(req.Context(), actor)
			next.ServeHTTP(w, req.WithContext(ctx))
		})
	})
	r.Get("/admin/adv-products", handler.AdminAdvProductsPage)
	r.Get("/admin/adv-products/{id}", handler.AdminAdvProductDetailPage)
	r.Post("/admin/adv-products/{id}/approve", handler.AdminAdvProductApproveSubmit)
	r.Post("/admin/adv-products/{id}/reject", handler.AdminAdvProductRejectSubmit)
	r.Post("/admin/adv-products/new", handler.AdminAdvProductCreateSubmit)

	t.Run("GET /admin/adv-products renders hub with correct title, filters and table rows", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/admin/adv-products?tab=all&q=بانادول&tier_level=3", nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		require.Equal(t, http.StatusOK, rec.Code)
		body, _ := io.ReadAll(rec.Body)
		bodyStr := string(body)

		assert.Contains(t, bodyStr, "رعاية المنتجات")
		assert.Contains(t, bodyStr, "Product Sponsorships")
		assert.Contains(t, bodyStr, "إجمالي المنتجات المروجة")
		assert.Contains(t, bodyStr, "الرعايات النشطة حالياً")
		assert.Contains(t, bodyStr, "بانادول إكسترا")
		assert.Contains(t, bodyStr, "صيدلية النور")
		assert.Contains(t, bodyStr, "1250")
	})

	t.Run("GET /admin/adv-products/101 renders full detail page with telemetry and ledger", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/admin/adv-products/101", nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		require.Equal(t, http.StatusOK, rec.Code)
		body, _ := io.ReadAll(rec.Body)
		bodyStr := string(body)

		assert.Contains(t, bodyStr, "تفاصيل الرعاية #101")
		assert.Contains(t, bodyStr, "بانادول إكسترا")
		assert.Contains(t, bodyStr, "صيدلية النور")
		assert.Contains(t, bodyStr, "سجل حركات الرصيد")
		assert.Contains(t, bodyStr, "TX-001")
		assert.Contains(t, bodyStr, "سجل الأحداث والتفاعل")
	})

	t.Run("POST /admin/adv-products/101/approve approves sponsorship request", func(t *testing.T) {
		form := url.Values{}
		form.Set("notes", "معتمد وموافق عليه")

		req := httptest.NewRequest(http.MethodPost, "/admin/adv-products/101/approve", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusSeeOther, rec.Code)
		assert.Contains(t, rec.Header().Get("Location"), "/admin/adv-products")

		// Verify status in mock
		sr, err := mockRepo.GetSponsorshipRequestByID(context.Background(), 101)
		require.NoError(t, err)
		require.NotNil(t, sr)
		assert.Equal(t, promo.AdminApproved, sr.AdminStatus)
	})

	t.Run("POST /admin/adv-products/101/reject rejects sponsorship request", func(t *testing.T) {
		// Reset status to pending
		mockRepo.requests[0].AdminStatus = promo.AdminPending

		form := url.Values{}
		form.Set("notes", "البيانات غير مكتملة")

		req := httptest.NewRequest(http.MethodPost, "/admin/adv-products/101/reject", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusSeeOther, rec.Code)
		assert.Contains(t, rec.Header().Get("Location"), "/admin/adv-products")

		// Verify status in mock
		sr, err := mockRepo.GetSponsorshipRequestByID(context.Background(), 101)
		require.NoError(t, err)
		require.NotNil(t, sr)
		assert.Equal(t, promo.AdminRejected, sr.AdminStatus)
	})
}
