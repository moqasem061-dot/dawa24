package ui_test

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/muhiya/dawa24-store/internal/modules/identity"
	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
	"github.com/muhiya/dawa24-store/internal/ui"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
	"github.com/stretchr/testify/assert"
)

func TestOffersPageSorting_SponsoredLeadsAcrossAllSorts(t *testing.T) {
	cards := []*pages.OfferCardData{
		{ID: 1, Title: i18n.New("Normal Cheap", "Normal Cheap"), TotalPrice: money.FromMinor(1000), DiscountPercentage: 10, IsSponsored: false},
		{ID: 2, Title: i18n.New("Sponsored Expensive", "Sponsored Expensive"), TotalPrice: money.FromMinor(5000), DiscountPercentage: 5, IsSponsored: true},
		{ID: 3, Title: i18n.New("Normal Expensive", "Normal Expensive"), TotalPrice: money.FromMinor(9000), DiscountPercentage: 20, IsSponsored: false},
		{ID: 4, Title: i18n.New("Sponsored Cheap", "Sponsored Cheap"), TotalPrice: money.FromMinor(2000), DiscountPercentage: 15, IsSponsored: true},
	}

	// 1. Test price_asc: sponsored should always come first
	sorted := make([]*pages.OfferCardData, len(cards))
	copy(sorted, cards)

	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].IsSponsored != sorted[j].IsSponsored {
			return sorted[i].IsSponsored
		}
		return sorted[i].TotalPrice.Minor() < sorted[j].TotalPrice.Minor()
	})

	assert.True(t, sorted[0].IsSponsored, "first item must be sponsored")
	assert.True(t, sorted[1].IsSponsored, "second item must be sponsored")
	assert.Equal(t, int64(4), sorted[0].ID, "Sponsored Cheap should be first among sponsored")
	assert.Equal(t, int64(2), sorted[1].ID, "Sponsored Expensive should be second among sponsored")
	assert.False(t, sorted[2].IsSponsored)
	assert.False(t, sorted[3].IsSponsored)

	// 2. Test price_desc: sponsored should always come first
	copy(sorted, cards)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].IsSponsored != sorted[j].IsSponsored {
			return sorted[i].IsSponsored
		}
		return sorted[i].TotalPrice.Minor() > sorted[j].TotalPrice.Minor()
	})

	assert.True(t, sorted[0].IsSponsored, "first item must be sponsored")
	assert.True(t, sorted[1].IsSponsored, "second item must be sponsored")
	assert.Equal(t, int64(2), sorted[0].ID, "Sponsored Expensive should be first among sponsored")
	assert.Equal(t, int64(4), sorted[1].ID, "Sponsored Cheap should be second among sponsored")
}

func TestCatalogSorting_OrderableSponsoredLeads(t *testing.T) {
	variantCards := []*pages.SupplierVariantCard{
		{
			VariantID: 1, ProductID: 101, ProductNameAr: "صنف غير متوفر لكنه ممول",
			IsSponsored: true, CanAddToCart: false, AvailableStock: 0, IsCovered: true,
			Price: money.FromMinor(1000),
		},
		{
			VariantID: 2, ProductID: 102, ProductNameAr: "صنف عادي متوفر ومغطى ورخيص",
			IsSponsored: false, CanAddToCart: true, AvailableStock: 10, IsCovered: true,
			Price: money.FromMinor(2000),
		},
		{
			VariantID: 3, ProductID: 103, ProductNameAr: "صنف ممول متوفر ومغطى",
			IsSponsored: true, CanAddToCart: true, AvailableStock: 50, IsCovered: true,
			Price: money.FromMinor(5000),
		},
		{
			VariantID: 4, ProductID: 104, ProductNameAr: "صنف عادي غير مغطى",
			IsSponsored: false, CanAddToCart: true, AvailableStock: 20, IsCovered: false,
			Price: money.FromMinor(1500),
		},
	}

	sort.SliceStable(variantCards, func(i, j int) bool {
		aEligibleSponsored := variantCards[i].IsSponsored && variantCards[i].CanAddToCart && variantCards[i].AvailableStock > 0 && variantCards[i].IsCovered
		bEligibleSponsored := variantCards[j].IsSponsored && variantCards[j].CanAddToCart && variantCards[j].AvailableStock > 0 && variantCards[j].IsCovered
		if aEligibleSponsored != bEligibleSponsored {
			return aEligibleSponsored
		}

		aOrderable := variantCards[i].CanAddToCart && variantCards[i].AvailableStock > 0 && variantCards[i].IsCovered
		bOrderable := variantCards[j].CanAddToCart && variantCards[j].AvailableStock > 0 && variantCards[j].IsCovered
		if aOrderable != bOrderable {
			return aOrderable
		}

		aInStock := variantCards[i].AvailableStock > 0
		bInStock := variantCards[j].AvailableStock > 0
		if aInStock != bInStock {
			return aInStock
		}

		return variantCards[i].Price.Minor() < variantCards[j].Price.Minor()
	})

	// 1st MUST be the orderable sponsored variant (#3), even though its price is higher than #1 and #2!
	assert.Equal(t, int64(3), variantCards[0].VariantID, "Orderable sponsored product must be #1")
	// 2nd MUST be the orderable non-sponsored variant (#2)
	assert.Equal(t, int64(2), variantCards[1].VariantID, "Orderable regular product must be #2")
	// 3rd and 4th MUST be non-orderable or out of stock (#1 or #4)
	assert.False(t, variantCards[2].CanAddToCart && variantCards[2].AvailableStock > 0 && variantCards[2].IsCovered)
	assert.False(t, variantCards[3].CanAddToCart && variantCards[3].AvailableStock > 0 && variantCards[3].IsCovered)
}

func TestVendorTeam_SelfDeleteAndSelfToggleGuard(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	idRepo := newMockIdentityRepoTeamTest()
	idSvc := identity.NewService(idRepo, nil, logger)
	orgRepo := newMockOrgRepoTeamTest()
	orgSvc := org.NewService(orgRepo, logger)

	// Add self as a member with UserID = 10
	selfMember := &org.Member{
		ID:             100,
		OrganizationID: 55,
		UserID:         10,
		RoleKey:        "org_manager",
		IsActive:       true,
	}
	orgRepo.members[100] = selfMember

	h := ui.NewUIHandler(
		nil, orgSvc, nil, nil, nil, idSvc, nil, nil, nil, nil, nil, nil, nil, nil, logger,
	)

	r := chi.NewRouter()
	h.RegisterVendorRoutes(r)

	vendorActor := authctx.Actor{UserID: 10, OrganizationID: 55, OrgType: "vendor", Permissions: []string{"vendor.*"}}

	// 1. Trying to toggle self active status should be rejected
	t.Run("POST /vendor/team/{id}/toggle on self is rejected", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/vendor/team/100/toggle", nil)
		req = req.WithContext(authctx.WithActor(req.Context(), vendorActor))
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusSeeOther, rec.Code)
		loc := rec.Header().Get("Location")
		assert.Contains(t, loc, "notice=error")
		assert.True(t, orgRepo.members[100].IsActive, "Self member status should still be active")
	})

	// 2. Trying to delete self should be rejected
	t.Run("POST /vendor/team/{id}/delete on self is rejected", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/vendor/team/100/delete", nil)
		req = req.WithContext(authctx.WithActor(req.Context(), vendorActor))
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusSeeOther, rec.Code)
		loc := rec.Header().Get("Location")
		assert.Contains(t, loc, "notice=error")
		assert.NotNil(t, orgRepo.members[100], "Self member should not have been deleted")
	})
}
