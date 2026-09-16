package http_test

import (
	"context"
	"testing"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/promo"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

type stubRepo struct{ t *testing.T }

func (r stubRepo) fail(method string) {
	r.t.Helper()
	r.t.Fatalf("repository.%s was called; the request should have been rejected before reaching the repository", method)
}

func (r stubRepo) CreateOffer(context.Context, *promo.Offer) error { r.fail("CreateOffer"); return nil }
func (r stubRepo) GetOfferByID(context.Context, int64) (*promo.Offer, error) {
	r.fail("GetOfferByID")
	return nil, nil
}
func (r stubRepo) ListActiveOffers(context.Context, int, int) ([]*promo.Offer, error) {
	r.fail("ListActiveOffers")
	return nil, nil
}
func (r stubRepo) ListOffersForProduct(context.Context, int64) ([]*promo.OfferProductWithOffer, error) {
	r.fail("ListOffersForProduct")
	return nil, nil
}
func (r stubRepo) ListOffersForProducts(context.Context, []int64) ([]*promo.OfferProductWithOffer, error) {
	r.fail("ListOffersForProducts")
	return nil, nil
}
func (r stubRepo) ListBuyerOffers(context.Context, promo.BuyerOfferQuery) ([]*promo.BuyerOffer, int, error) {
	r.fail("ListBuyerOffers")
	return nil, 0, nil
}
func (r stubRepo) OfferVerdict(context.Context, promo.BuyerOfferQuery) (promo.OfferVerdict, error) {
	r.fail("OfferVerdict")
	return promo.OfferVerdict{}, nil
}
func (r stubRepo) ListRunningOffersByOrg(context.Context, int64, int) ([]*promo.Offer, error) {
	r.fail("ListRunningOffersByOrg")
	return nil, nil
}

func (r stubRepo) ListOffers(context.Context, int, int) ([]*promo.Offer, error) {
	r.fail("ListOffers")
	return nil, nil
}
func (r stubRepo) SetOfferActive(context.Context, int64, bool) error {
	r.fail("SetOfferActive")
	return nil
}
func (r stubRepo) IncrementOfferEngagement(context.Context, int64, bool) error {
	r.fail("IncrementOfferEngagement")
	return nil
}
func (r stubRepo) CreatePackage(context.Context, *promo.OfferPackage) error {
	r.fail("CreatePackage")
	return nil
}
func (r stubRepo) ListPackages(context.Context) ([]*promo.OfferPackage, error) {
	r.fail("ListPackages")
	return nil, nil
}
func (r stubRepo) CreateSponsorship(context.Context, *promo.OfferSponsorship) error {
	r.fail("CreateSponsorship")
	return nil
}
func (r stubRepo) ListActiveAds(context.Context, string) ([]*promo.Ad, error) {
	r.fail("ListActiveAds")
	return nil, nil
}
func (r stubRepo) RecordAdClick(context.Context, int64, *int64, string, string) error {
	r.fail("RecordAdClick")
	return nil
}
func (stubRepo) UpdatePackage(context.Context, *promo.OfferPackage) error { return nil }
func (stubRepo) GetPackageByID(context.Context, int64) (*promo.OfferPackage, error) {
	return nil, nil
}
func (stubRepo) AdminListPackages(context.Context) ([]*promo.OfferPackage, error) {
	return nil, nil
}
func (stubRepo) TogglePackageActive(context.Context, int64, bool) error { return nil }
func (stubRepo) CreateSponsorshipPurchase(context.Context, *promo.SponsorshipPurchase) error {
	return nil
}
func (stubRepo) GetSponsorshipPurchaseByID(context.Context, int64) (*promo.SponsorshipPurchase, error) {
	return nil, nil
}
func (stubRepo) ListSponsorshipPurchasesByOrg(context.Context, int64) ([]*promo.SponsorshipPurchase, error) {
	return nil, nil
}
func (stubRepo) ListActiveSponsorshipPurchasesByOrg(context.Context, int64) ([]*promo.SponsorshipPurchase, error) {
	return nil, nil
}
func (stubRepo) ConsumeSponsorshipCredits(context.Context, promo.ConsumeCredits) (*promo.CreditEntry, error) {
	return &promo.CreditEntry{}, nil
}
func (stubRepo) ListCreditEntries(context.Context, int64, int, int) ([]*promo.CreditEntry, int, error) {
	return nil, 0, nil
}
func (stubRepo) ListOrgCreditEntries(context.Context, int64, int, int) ([]*promo.CreditEntry, int, error) {
	return nil, 0, nil
}
func (stubRepo) ExpireSponsorshipPurchases(context.Context) (int64, error) { return 0, nil }
func (stubRepo) CreateSponsorshipRequest(context.Context, *promo.SponsorshipRequest) error {
	return nil
}
func (stubRepo) GetSponsorshipRequestByID(context.Context, int64) (*promo.SponsorshipRequest, error) {
	return nil, nil
}
func (stubRepo) ListSponsorshipRequestsByOrg(context.Context, int64, int, int) ([]*promo.SponsorshipRequest, error) {
	return nil, nil
}
func (stubRepo) ListSponsorshipRequestsByOrgWithTotal(context.Context, int64, int, int) ([]*promo.SponsorshipRequest, int, error) {
	return nil, 0, nil
}
func (stubRepo) ListAllSponsorshipRequests(context.Context, int, int) ([]*promo.SponsorshipRequest, error) {
	return nil, nil
}
func (stubRepo) ListAllSponsorshipRequestsWithTotal(context.Context, int, int) ([]*promo.SponsorshipRequest, int, error) {
	return nil, 0, nil
}
func (stubRepo) ListPendingSponsorshipRequests(context.Context, int, int) ([]*promo.SponsorshipRequest, error) {
	return nil, nil
}
func (stubRepo) UpdateSponsorshipRequestAdminStatus(context.Context, int64, promo.AdminStatus, string, int64) error {
	return nil
}
func (stubRepo) ActivateSponsorshipRequest(context.Context, int64, int64) (*promo.SponsorshipRequest, error) {
	return nil, nil
}
func (stubRepo) CancelSponsorshipRequest(context.Context, int64, int64) error { return nil }
func (stubRepo) ExpireSponsorshipRequests(context.Context) (int64, error)     { return 0, nil }
func (stubRepo) RankedSponsorshipsForProducts(context.Context, []int64) ([]*promo.RankedSponsorship, error) {
	return nil, nil
}
func (stubRepo) RankedSponsorshipsForOffers(context.Context, []int64) ([]*promo.RankedSponsorship, error) {
	return nil, nil
}
func (stubRepo) ListActiveRankedSponsorships(context.Context, promo.SponsorshipItemType) ([]*promo.RankedSponsorship, error) {
	return nil, nil
}
func (stubRepo) IsSponsored(context.Context, promo.SponsorshipItemType, int64) (*promo.RankedSponsorship, error) {
	return nil, nil
}
func (stubRepo) CreateAd(context.Context, *promo.Ad) error { return nil }
func (stubRepo) UpdateAd(context.Context, *promo.Ad) error { return nil }
func (stubRepo) GetAdByID(context.Context, int64) (*promo.Ad, error) {
	return nil, nil
}
func (stubRepo) ListAdsByOrg(context.Context, int64, int, int) ([]*promo.Ad, error) {
	return nil, nil
}
func (stubRepo) ListAdsByOrgWithTotal(context.Context, int64, int, int) ([]*promo.Ad, int, error) {
	return nil, 0, nil
}
func (stubRepo) ListAllAds(context.Context, int, int) ([]*promo.Ad, error) {
	return nil, nil
}
func (stubRepo) ListAllAdsWithTotal(context.Context, int, int) ([]*promo.Ad, int, error) {
	return nil, 0, nil
}
func (stubRepo) UpdateAdAdminStatus(context.Context, int64, promo.AdminStatus, string, int64) error {
	return nil
}
func (stubRepo) AdminToggleAd(context.Context, int64) (*promo.Ad, error) {
	return &promo.Ad{ID: 1, IsActive: true}, nil
}
func (stubRepo) SubmitAdEditRequest(context.Context, int64, *promo.AdPendingChanges) error {
	return nil
}
func (stubRepo) ApproveAdEditRequest(context.Context, int64, int64) error {
	return nil
}
func (stubRepo) RejectAdEditRequest(context.Context, int64, int64, string) error {
	return nil
}
func (stubRepo) RecordAdImpression(context.Context, int64, *int64, string, string) error {
	return nil
}
func (r stubRepo) CreateHighlightSection(context.Context, *promo.HighlightSection) error {
	r.fail("CreateHighlightSection")
	return nil
}
func (r stubRepo) UpdateHighlightSection(context.Context, *promo.HighlightSection) error {
	r.fail("UpdateHighlightSection")
	return nil
}
func (r stubRepo) DeleteHighlightSection(context.Context, int64, int64) error {
	r.fail("DeleteHighlightSection")
	return nil
}
func (r stubRepo) GetHighlightSectionByID(context.Context, int64) (*promo.HighlightSection, error) {
	r.fail("GetHighlightSectionByID")
	return nil, nil
}
func (r stubRepo) ListHighlightSections(context.Context) ([]*promo.HighlightSection, error) {
	r.fail("ListHighlightSections")
	return nil, nil
}
func (r stubRepo) ListHighlightSectionsByOrg(context.Context, int64) ([]*promo.HighlightSection, error) {
	r.fail("ListHighlightSectionsByOrg")
	return nil, nil
}
func (r stubRepo) AddHighlightItem(context.Context, *promo.HighlightSectionItem) error {
	r.fail("AddHighlightItem")
	return nil
}
func (r stubRepo) ListHighlightItems(context.Context, int64) ([]*promo.HighlightSectionItem, error) {
	r.fail("ListHighlightItems")
	return nil, nil
}
func (r stubRepo) ExpirePromotions(context.Context) (int64, error) {
	r.fail("ExpirePromotions")
	return 0, nil
}
func (r stubRepo) ExpirePromotionsAndCollectMedia(context.Context) ([]string, int64, error) {
	r.fail("ExpirePromotionsAndCollectMedia")
	return nil, 0, nil
}
func (r stubRepo) CreateSpecialOffer(context.Context, *promo.SpecialOffer) error {
	r.fail("CreateSpecialOffer")
	return nil
}
func (r stubRepo) UpdateSpecialOffer(context.Context, *promo.SpecialOffer) error {
	r.fail("UpdateSpecialOffer")
	return nil
}
func (r stubRepo) GetSpecialOfferByID(context.Context, int64) (*promo.SpecialOffer, error) {
	r.fail("GetSpecialOfferByID")
	return nil, nil
}
func (r stubRepo) ListSpecialOffersByOrg(context.Context, int64) ([]*promo.SpecialOffer, error) {
	r.fail("ListSpecialOffersByOrg")
	return nil, nil
}
func (r stubRepo) ListAllSpecialOffers(context.Context, int, int) ([]*promo.SpecialOffer, error) {
	r.fail("ListAllSpecialOffers")
	return nil, nil
}
func (r stubRepo) ListAllSpecialOffersWithTotal(context.Context, string, int, int) ([]*promo.SpecialOffer, int, error) {
	r.fail("ListAllSpecialOffersWithTotal")
	return nil, 0, nil
}
func (r stubRepo) UpdateSpecialOfferAdminStatus(context.Context, int64, string, string, int64) error {
	r.fail("UpdateSpecialOfferAdminStatus")
	return nil
}
func (r stubRepo) ToggleSpecialOfferStatus(context.Context, int64, bool) error {
	r.fail("ToggleSpecialOfferStatus")
	return nil
}
func (r stubRepo) DeleteSpecialOffer(context.Context, int64, int64) error {
	r.fail("DeleteSpecialOffer")
	return nil
}
func (r stubRepo) AddSpecialOfferLocation(context.Context, *promo.SpecialOfferLocation) error {
	r.fail("AddSpecialOfferLocation")
	return nil
}
func (r stubRepo) ListSpecialOfferLocations(context.Context, int64) ([]*promo.SpecialOfferLocation, error) {
	r.fail("ListSpecialOfferLocations")
	return nil, nil
}

type happyRepo struct{}

func (happyRepo) CreateOffer(ctx context.Context, o *promo.Offer) error {
	o.ID = 1
	return nil
}
func (happyRepo) GetOfferByID(ctx context.Context, id int64) (*promo.Offer, error) {
	return &promo.Offer{
		ID:             id,
		OrganizationID: 1,
		Title:          i18n.Text{"en": "Summer Sale"},
		DiscountType:   promo.DiscountPercentage,
		DiscountValue:  money.MustParse("10.00"),
		StartsAt:       time.Now().Add(-time.Hour),
		ExpiresAt:      time.Now().Add(24 * time.Hour),
		IsActive:       true,
	}, nil
}
func (happyRepo) ListActiveOffers(ctx context.Context, limit, offset int) ([]*promo.Offer, error) {
	return []*promo.Offer{{
		ID:             1,
		OrganizationID: 1,
		Title:          i18n.Text{"en": "Summer Sale"},
		IsActive:       true,
	}}, nil
}

func (happyRepo) ListOffersForProduct(ctx context.Context, productID int64) ([]*promo.OfferProductWithOffer, error) {
	return nil, nil
}
func (happyRepo) ListOffersForProducts(context.Context, []int64) ([]*promo.OfferProductWithOffer, error) {
	return nil, nil
}
func (happyRepo) ListBuyerOffers(context.Context, promo.BuyerOfferQuery) ([]*promo.BuyerOffer, int, error) {
	return nil, 0, nil
}
func (happyRepo) OfferVerdict(context.Context, promo.BuyerOfferQuery) (promo.OfferVerdict, error) {
	return promo.OfferVerdict{}, nil
}
func (happyRepo) ListRunningOffersByOrg(context.Context, int64, int) ([]*promo.Offer, error) {
	return nil, nil
}

func (happyRepo) ListOffers(ctx context.Context, limit, offset int) ([]*promo.Offer, error) {
	return []*promo.Offer{{ID: 1, OrganizationID: 1, Title: i18n.Text{"en": "Summer Sale"}, IsActive: true}}, nil
}
func (happyRepo) SetOfferActive(ctx context.Context, id int64, active bool) error {
	return nil
}
func (happyRepo) IncrementOfferEngagement(ctx context.Context, id int64, isClick bool) error {
	return nil
}
func (happyRepo) CreatePackage(ctx context.Context, p *promo.OfferPackage) error {
	p.ID = 1
	return nil
}
func (happyRepo) ListPackages(ctx context.Context) ([]*promo.OfferPackage, error) {
	return []*promo.OfferPackage{{ID: 1, Name: i18n.Text{"en": "Gold"}}}, nil
}
func (happyRepo) CreateSponsorship(ctx context.Context, s *promo.OfferSponsorship) error {
	s.ID = 1
	return nil
}
func (happyRepo) ListActiveAds(ctx context.Context, position string) ([]*promo.Ad, error) {
	return []*promo.Ad{{ID: 1, Title: "Ad 1", IsActive: true}}, nil
}
func (happyRepo) RecordAdClick(ctx context.Context, adID int64, userID *int64, ip, ua string) error {
	return nil
}
func (happyRepo) UpdatePackage(context.Context, *promo.OfferPackage) error { return nil }
func (happyRepo) GetPackageByID(context.Context, int64) (*promo.OfferPackage, error) {
	return nil, nil
}
func (happyRepo) AdminListPackages(context.Context) ([]*promo.OfferPackage, error) {
	return nil, nil
}
func (happyRepo) TogglePackageActive(context.Context, int64, bool) error { return nil }
func (happyRepo) CreateSponsorshipPurchase(context.Context, *promo.SponsorshipPurchase) error {
	return nil
}
func (happyRepo) GetSponsorshipPurchaseByID(context.Context, int64) (*promo.SponsorshipPurchase, error) {
	return nil, nil
}
func (happyRepo) ListSponsorshipPurchasesByOrg(context.Context, int64) ([]*promo.SponsorshipPurchase, error) {
	return nil, nil
}
func (happyRepo) ListActiveSponsorshipPurchasesByOrg(context.Context, int64) ([]*promo.SponsorshipPurchase, error) {
	return nil, nil
}
func (happyRepo) ConsumeSponsorshipCredits(context.Context, promo.ConsumeCredits) (*promo.CreditEntry, error) {
	return &promo.CreditEntry{}, nil
}
func (happyRepo) ListCreditEntries(context.Context, int64, int, int) ([]*promo.CreditEntry, int, error) {
	return nil, 0, nil
}
func (happyRepo) ListOrgCreditEntries(context.Context, int64, int, int) ([]*promo.CreditEntry, int, error) {
	return nil, 0, nil
}
func (happyRepo) ExpireSponsorshipPurchases(context.Context) (int64, error) { return 0, nil }
func (happyRepo) CreateSponsorshipRequest(context.Context, *promo.SponsorshipRequest) error {
	return nil
}
func (happyRepo) GetSponsorshipRequestByID(context.Context, int64) (*promo.SponsorshipRequest, error) {
	return &promo.SponsorshipRequest{ID: 1, AdminStatus: "pending"}, nil
}
func (happyRepo) ListSponsorshipRequestsByOrg(context.Context, int64, int, int) ([]*promo.SponsorshipRequest, error) {
	return nil, nil
}
func (happyRepo) ListSponsorshipRequestsByOrgWithTotal(context.Context, int64, int, int) ([]*promo.SponsorshipRequest, int, error) {
	return nil, 0, nil
}
func (happyRepo) ListAllSponsorshipRequests(context.Context, int, int) ([]*promo.SponsorshipRequest, error) {
	return nil, nil
}
func (happyRepo) ListAllSponsorshipRequestsWithTotal(context.Context, int, int) ([]*promo.SponsorshipRequest, int, error) {
	return nil, 0, nil
}
func (happyRepo) ListPendingSponsorshipRequests(context.Context, int, int) ([]*promo.SponsorshipRequest, error) {
	return nil, nil
}
func (happyRepo) UpdateSponsorshipRequestAdminStatus(context.Context, int64, promo.AdminStatus, string, int64) error {
	return nil
}
func (happyRepo) ActivateSponsorshipRequest(context.Context, int64, int64) (*promo.SponsorshipRequest, error) {
	return &promo.SponsorshipRequest{ID: 1, AdminStatus: "approved", Status: "active"}, nil
}
func (happyRepo) CancelSponsorshipRequest(context.Context, int64, int64) error { return nil }
func (happyRepo) ExpireSponsorshipRequests(context.Context) (int64, error)     { return 0, nil }
func (happyRepo) RankedSponsorshipsForProducts(context.Context, []int64) ([]*promo.RankedSponsorship, error) {
	return nil, nil
}
func (happyRepo) RankedSponsorshipsForOffers(context.Context, []int64) ([]*promo.RankedSponsorship, error) {
	return nil, nil
}
