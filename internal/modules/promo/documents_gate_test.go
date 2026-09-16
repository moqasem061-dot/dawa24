package promo

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

type stubPromoRepo struct {
	createErr error
	created   *Offer
}

func (s *stubPromoRepo) CreateOffer(_ context.Context, o *Offer) error {
	if s.createErr != nil {
		return s.createErr
	}
	s.created = o
	return nil
}
func (s *stubPromoRepo) GetOfferByID(context.Context, int64) (*Offer, error) { return nil, nil }
func (s *stubPromoRepo) ListActiveOffers(context.Context, int, int) ([]*Offer, error) {
	return nil, nil
}
func (s *stubPromoRepo) ListOffersForProduct(context.Context, int64) ([]*OfferProductWithOffer, error) {
	return nil, nil
}
func (s *stubPromoRepo) ListOffersForProducts(context.Context, []int64) ([]*OfferProductWithOffer, error) {
	return nil, nil
}
func (s *stubPromoRepo) ListOffers(context.Context, int, int) ([]*Offer, error) { return nil, nil }
func (s *stubPromoRepo) ListBuyerOffers(context.Context, BuyerOfferQuery) ([]*BuyerOffer, int, error) {
	return nil, 0, nil
}
func (s *stubPromoRepo) OfferVerdict(context.Context, BuyerOfferQuery) (OfferVerdict, error) {
	return OfferVerdict{}, nil
}
func (s *stubPromoRepo) ListRunningOffersByOrg(context.Context, int64, int) ([]*Offer, error) {
	return nil, nil
}
func (s *stubPromoRepo) SetOfferActive(context.Context, int64, bool) error           { return nil }
func (s *stubPromoRepo) IncrementOfferEngagement(context.Context, int64, bool) error { return nil }
func (s *stubPromoRepo) CreatePackage(context.Context, *OfferPackage) error          { return nil }
func (s *stubPromoRepo) ListPackages(context.Context) ([]*OfferPackage, error)       { return nil, nil }
func (s *stubPromoRepo) CreateSponsorship(context.Context, *OfferSponsorship) error  { return nil }
func (s *stubPromoRepo) ListActiveAds(context.Context, string) ([]*Ad, error)        { return nil, nil }
func (s *stubPromoRepo) RecordAdClick(context.Context, int64, *int64, string, string) error {
	return nil
}
func (s *stubPromoRepo) UpdatePackage(context.Context, *OfferPackage) error { return nil }
func (s *stubPromoRepo) GetPackageByID(context.Context, int64) (*OfferPackage, error) {
	return nil, nil
}
func (s *stubPromoRepo) AdminListPackages(context.Context) ([]*OfferPackage, error) {
	return nil, nil
}
func (s *stubPromoRepo) TogglePackageActive(context.Context, int64, bool) error { return nil }
func (s *stubPromoRepo) CreateSponsorshipPurchase(context.Context, *SponsorshipPurchase) error {
	return nil
}
func (s *stubPromoRepo) GetSponsorshipPurchaseByID(context.Context, int64) (*SponsorshipPurchase, error) {
	return nil, nil
}
func (s *stubPromoRepo) ListSponsorshipPurchasesByOrg(context.Context, int64) ([]*SponsorshipPurchase, error) {
	return nil, nil
}
func (s *stubPromoRepo) ListActiveSponsorshipPurchasesByOrg(context.Context, int64) ([]*SponsorshipPurchase, error) {
	return nil, nil
}
func (s *stubPromoRepo) ConsumeSponsorshipCredits(context.Context, ConsumeCredits) (*CreditEntry, error) {
	return &CreditEntry{}, nil
}
func (s *stubPromoRepo) ListCreditEntries(context.Context, int64, int, int) ([]*CreditEntry, int, error) {
	return nil, 0, nil
}
func (s *stubPromoRepo) ListOrgCreditEntries(context.Context, int64, int, int) ([]*CreditEntry, int, error) {
	return nil, 0, nil
}
func (s *stubPromoRepo) ExpireSponsorshipPurchases(context.Context) (int64, error) { return 0, nil }
func (s *stubPromoRepo) CreateSponsorshipRequest(context.Context, *SponsorshipRequest) error {
	return nil
}
func (s *stubPromoRepo) GetSponsorshipRequestByID(context.Context, int64) (*SponsorshipRequest, error) {
	return nil, nil
}
func (s *stubPromoRepo) ListSponsorshipRequestsByOrg(context.Context, int64, int, int) ([]*SponsorshipRequest, error) {
	return nil, nil
}
func (s *stubPromoRepo) ListSponsorshipRequestsByOrgWithTotal(context.Context, int64, int, int) ([]*SponsorshipRequest, int, error) {
	return nil, 0, nil
}
func (s *stubPromoRepo) ListAllSponsorshipRequests(context.Context, int, int) ([]*SponsorshipRequest, error) {
	return nil, nil
}
func (s *stubPromoRepo) ListAllSponsorshipRequestsWithTotal(context.Context, int, int) ([]*SponsorshipRequest, int, error) {
	return nil, 0, nil
}
func (s *stubPromoRepo) ListPendingSponsorshipRequests(context.Context, int, int) ([]*SponsorshipRequest, error) {
	return nil, nil
}
func (s *stubPromoRepo) UpdateSponsorshipRequestAdminStatus(context.Context, int64, AdminStatus, string, int64) error {
	return nil
}
func (s *stubPromoRepo) ActivateSponsorshipRequest(context.Context, int64, int64) (*SponsorshipRequest, error) {
	return nil, nil
}
func (s *stubPromoRepo) CancelSponsorshipRequest(context.Context, int64, int64) error { return nil }
func (s *stubPromoRepo) ExpireSponsorshipRequests(context.Context) (int64, error)     { return 0, nil }
func (s *stubPromoRepo) RankedSponsorshipsForProducts(context.Context, []int64) ([]*RankedSponsorship, error) {
	return nil, nil
}
func (s *stubPromoRepo) RankedSponsorshipsForOffers(context.Context, []int64) ([]*RankedSponsorship, error) {
	return nil, nil
}
func (s *stubPromoRepo) ListActiveRankedSponsorships(context.Context, SponsorshipItemType) ([]*RankedSponsorship, error) {
	return nil, nil
}
func (s *stubPromoRepo) IsSponsored(context.Context, SponsorshipItemType, int64) (*RankedSponsorship, error) {
	return nil, nil
}
func (s *stubPromoRepo) CreateAd(context.Context, *Ad) error { return nil }
func (s *stubPromoRepo) UpdateAd(context.Context, *Ad) error { return nil }
func (s *stubPromoRepo) GetAdByID(context.Context, int64) (*Ad, error) {
	return nil, nil
}
func (s *stubPromoRepo) ListAdsByOrg(context.Context, int64, int, int) ([]*Ad, error) {
	return nil, nil
}
func (s *stubPromoRepo) ListAdsByOrgWithTotal(context.Context, int64, int, int) ([]*Ad, int, error) {
	return nil, 0, nil
}
func (s *stubPromoRepo) ListAllAds(context.Context, int, int) ([]*Ad, error) {
	return nil, nil
}
func (s *stubPromoRepo) ListAllAdsWithTotal(context.Context, int, int) ([]*Ad, int, error) {
	return nil, 0, nil
}
func (s *stubPromoRepo) UpdateAdAdminStatus(context.Context, int64, AdminStatus, string, int64) error {
	return nil
}
func (s *stubPromoRepo) AdminToggleAd(context.Context, int64) (*Ad, error) {
	return &Ad{ID: 1, IsActive: true}, nil
}
func (s *stubPromoRepo) SubmitAdEditRequest(context.Context, int64, *AdPendingChanges) error {
	return nil
}
func (s *stubPromoRepo) ApproveAdEditRequest(context.Context, int64, int64) error {
	return nil
}
func (s *stubPromoRepo) RejectAdEditRequest(context.Context, int64, int64, string) error {
	return nil
}
func (s *stubPromoRepo) RecordAdImpression(context.Context, int64, *int64, string, string) error {
	return nil
}
func (s *stubPromoRepo) CreateHighlightSection(context.Context, *HighlightSection) error {
	return nil
}
func (s *stubPromoRepo) UpdateHighlightSection(context.Context, *HighlightSection) error {
	return nil
}
func (s *stubPromoRepo) DeleteHighlightSection(context.Context, int64, int64) error {
	return nil
}
func (s *stubPromoRepo) GetHighlightSectionByID(context.Context, int64) (*HighlightSection, error) {
	return nil, nil
}
func (s *stubPromoRepo) ListHighlightSections(context.Context) ([]*HighlightSection, error) {
	return nil, nil
}
func (s *stubPromoRepo) ListHighlightSectionsByOrg(context.Context, int64) ([]*HighlightSection, error) {
	return nil, nil
}
func (s *stubPromoRepo) AddHighlightItem(context.Context, *HighlightSectionItem) error { return nil }
func (s *stubPromoRepo) ListHighlightItems(context.Context, int64) ([]*HighlightSectionItem, error) {
	return nil, nil
}
func (s *stubPromoRepo) ExpirePromotions(context.Context) (int64, error) { return 0, nil }
func (s *stubPromoRepo) ExpirePromotionsAndCollectMedia(context.Context) ([]string, int64, error) {
	return nil, 0, nil
}
func (s *stubPromoRepo) CreateSpecialOffer(context.Context, *SpecialOffer) error {
	return nil
}
func (s *stubPromoRepo) UpdateSpecialOffer(context.Context, *SpecialOffer) error {
	return nil
}
func (s *stubPromoRepo) GetSpecialOfferByID(context.Context, int64) (*SpecialOffer, error) {
	return nil, nil
}
func (s *stubPromoRepo) ListSpecialOffersByOrg(context.Context, int64) ([]*SpecialOffer, error) {
	return nil, nil
}
func (s *stubPromoRepo) ListAllSpecialOffers(context.Context, int, int) ([]*SpecialOffer, error) {
	return nil, nil
}
func (s *stubPromoRepo) ListAllSpecialOffersWithTotal(context.Context, string, int, int) ([]*SpecialOffer, int, error) {
	return nil, 0, nil
}
func (s *stubPromoRepo) UpdateSpecialOfferAdminStatus(context.Context, int64, string, string, int64) error {
	return nil
}
func (s *stubPromoRepo) ToggleSpecialOfferStatus(context.Context, int64, bool) error {
	return nil
}
func (s *stubPromoRepo) DeleteSpecialOffer(context.Context, int64, int64) error { return nil }
func (s *stubPromoRepo) AddSpecialOfferLocation(context.Context, *SpecialOfferLocation) error {
	return nil
}
func (s *stubPromoRepo) ListSpecialOfferLocations(context.Context, int64) ([]*SpecialOfferLocation, error) {
	return nil, nil
}
func (s *stubPromoRepo) DeleteSpecialOfferLocation(context.Context, int64, int64, int64) error {
	return nil
}
func (s *stubPromoRepo) ListAdminOfferLocations(context.Context, OfferLocationsFilter) ([]*OfferLocationAdminRow, OfferLocationsStats, int, error) {
	return nil, OfferLocationsStats{}, 0, nil
}

func validOffer() *Offer {
	now := time.Now().UTC()
	return &Offer{
		Title:         i18n.New("عرض يناير", "January Offer"),
		DiscountType:  DiscountPercentage,
		DiscountValue: money.MustParse("10.00"),
		StartsAt:      now,
		ExpiresAt:     now.Add(24 * time.Hour),
	}
}

// TestCreateOffer_DocumentsGate verifies the §4.2 gate: a vendor with missing
// mandatory documents cannot publish an offer, and the gate consults the
// tenant bound to the context.
func TestCreateOffer_DocumentsGate(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	t.Run("gate blocks missing documents", func(t *testing.T) {
		repo := &stubPromoRepo{}
		svc := NewService(repo, logger)
		blocked := false
		svc.SetRequiredDocsChecker(func(_ context.Context, orgID int64, orgType string) error {
			blocked = orgID == 7 && orgType == "vendor"
			return errors.New("documents.incomplete: mandatory documents missing")
		})

		ctx := database.WithTenant(context.Background(), 7)
		if _, err := svc.CreateOffer(ctx, validOffer()); err == nil {
			t.Fatal("offer creation must be refused when the documents gate fails")
		}
		if !blocked {
			t.Fatal("the gate must have been consulted with the vendor org")
		}
		if repo.created != nil {
			t.Fatal("offer must not be persisted when blocked")
		}
	})

	t.Run("gate passes and offer is published", func(t *testing.T) {
		repo := &stubPromoRepo{}
		svc := NewService(repo, logger)
		svc.SetRequiredDocsChecker(func(context.Context, int64, string) error { return nil })

		ctx := database.WithTenant(context.Background(), 7)
		offer, err := svc.CreateOffer(ctx, validOffer())
		if err != nil {
			t.Fatalf("offer creation must proceed when documents are complete: %v", err)
		}
		assertOfferSaved(t, repo, offer)
	})

	t.Run("no checker means no gate", func(t *testing.T) {
		repo := &stubPromoRepo{}
		svc := NewService(repo, logger)

		ctx := database.WithTenant(context.Background(), 7)
		if _, err := svc.CreateOffer(ctx, validOffer()); err != nil {
			t.Fatalf("offer creation must proceed without a checker installed: %v", err)
		}
	})
}

func assertOfferSaved(t *testing.T, repo *stubPromoRepo, offer *Offer) {
	t.Helper()
	if repo.created == nil {
		t.Fatal("offer must be persisted")
	}
	if !offer.IsActive {
		t.Fatal("newly created offers publish as active")
	}
	if offer.OrganizationID != 7 {
		t.Fatalf("offer must carry the tenant org, got %d", offer.OrganizationID)
	}
}

func (s *stubPromoRepo) CreditTotals(context.Context, int64) (int, int, error) { return 0, 0, nil }
func (s *stubPromoRepo) ListCreditAccounts(context.Context, string, int, int) ([]*CreditAccount, int, error) {
	return nil, 0, nil
}
func (s *stubPromoRepo) ListPurchasesForOrg(context.Context, int64, int, int) ([]*SponsorshipPurchase, int, error) {
	return nil, 0, nil
}
