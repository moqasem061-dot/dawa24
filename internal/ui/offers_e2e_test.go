package ui_test

import (
	"context"

	"github.com/moqasem061-dot/dawa24/internal/modules/promo"
)

type mockPromoRepo struct {
	offers []*promo.Offer
	spec   *promo.SpecialOffer
}

func (m *mockPromoRepo) CreateOffer(ctx context.Context, o *promo.Offer) error {
	o.ID = 101
	m.offers = append(m.offers, o)
	return nil
}
func (m *mockPromoRepo) GetOfferByID(ctx context.Context, id int64) (*promo.Offer, error) {
	if len(m.offers) > 0 {
		return m.offers[0], nil
	}
	return nil, nil
}
func (m *mockPromoRepo) ListActiveOffers(ctx context.Context, limit, offset int) ([]*promo.Offer, error) {
	return m.offers, nil
}
func (m *mockPromoRepo) ListOffersForProduct(ctx context.Context, productID int64) ([]*promo.OfferProductWithOffer, error) {
	return nil, nil
}
func (m *mockPromoRepo) ListOffersForProducts(ctx context.Context, productIDs []int64) ([]*promo.OfferProductWithOffer, error) {
	return nil, nil
}

// ListBuyerOffers and OfferVerdict stand in for the SQL offer rule, which
// promo/postgres tests against a real schema; here every offer passes it.
func (m *mockPromoRepo) ListBuyerOffers(ctx context.Context, q promo.BuyerOfferQuery) ([]*promo.BuyerOffer, int, error) {
	out := make([]*promo.BuyerOffer, 0, len(m.offers))
	for _, o := range m.offers {
		starts, expires := o.StartsAt, o.ExpiresAt
		out = append(out, &promo.BuyerOffer{
			ID: o.ID, OrganizationID: o.OrganizationID, Title: o.Title, Description: o.Description,
			DiscountType: o.DiscountType, DiscountValue: o.DiscountValue, MinOrderAmount: o.MinOrderAmount,
			StartsAt: &starts, ExpiresAt: &expires, ProductCount: len(o.ProductIDs),
		})
		if m.spec != nil && m.spec.ID == o.ID {
			out[len(out)-1].LegalName = m.spec.OrganizationName
		}
	}
	return out, len(out), nil
}
func (m *mockPromoRepo) OfferVerdict(ctx context.Context, q promo.BuyerOfferQuery) (promo.OfferVerdict, error) {
	return promo.OfferVerdict{Found: true, Live: true, SupplierOK: true, BranchOK: true, Institutional: true, Covered: true}, nil
}
func (m *mockPromoRepo) ListRunningOffersByOrg(ctx context.Context, orgID int64, limit int) ([]*promo.Offer, error) {
	return m.offers, nil
}
func (m *mockPromoRepo) ListOffers(ctx context.Context, limit, offset int) ([]*promo.Offer, error) {
	return m.offers, nil
}
func (m *mockPromoRepo) SetOfferActive(ctx context.Context, id int64, active bool) error {
	return nil
}
func (m *mockPromoRepo) IncrementOfferEngagement(ctx context.Context, offerID int64, isClick bool) error {
	return nil
}
func (m *mockPromoRepo) CreatePackage(ctx context.Context, p *promo.OfferPackage) error {
	return nil
}
func (m *mockPromoRepo) ListPackages(ctx context.Context) ([]*promo.OfferPackage, error) {
	return nil, nil
}
func (m *mockPromoRepo) CreateSponsorship(ctx context.Context, s *promo.OfferSponsorship) error {
	return nil
}
func (m *mockPromoRepo) ListActiveAds(ctx context.Context, position string) ([]*promo.Ad, error) {
	return nil, nil
}
func (m *mockPromoRepo) RecordAdClick(ctx context.Context, adID int64, userID *int64, ip, ua string) error {
	return nil
}
func (m *mockPromoRepo) CreateHighlightSection(ctx context.Context, h *promo.HighlightSection) error {
	return nil
}
func (m *mockPromoRepo) UpdateHighlightSection(ctx context.Context, h *promo.HighlightSection) error {
	return nil
}
func (m *mockPromoRepo) DeleteHighlightSection(ctx context.Context, id, orgID int64) error {
	return nil
}
func (m *mockPromoRepo) GetHighlightSectionByID(ctx context.Context, id int64) (*promo.HighlightSection, error) {
	return nil, nil
}
func (m *mockPromoRepo) ListHighlightSections(ctx context.Context) ([]*promo.HighlightSection, error) {
	return nil, nil
}
func (m *mockPromoRepo) ListHighlightSectionsByOrg(ctx context.Context, orgID int64) ([]*promo.HighlightSection, error) {
	return nil, nil
}
func (m *mockPromoRepo) AddHighlightItem(ctx context.Context, item *promo.HighlightSectionItem) error {
	return nil
}
func (m *mockPromoRepo) ListHighlightItems(ctx context.Context, sectionID int64) ([]*promo.HighlightSectionItem, error) {
	return nil, nil
}
func (m *mockPromoRepo) ExpirePromotions(ctx context.Context) (int64, error) {
	return 0, nil
}
func (m *mockPromoRepo) ExpirePromotionsAndCollectMedia(ctx context.Context) ([]string, int64, error) {
	return nil, 0, nil
}
func (m *mockPromoRepo) CreateSpecialOffer(ctx context.Context, o *promo.SpecialOffer) error {
	o.ID = 101
	m.spec = o
	return nil
}
func (m *mockPromoRepo) GetSpecialOfferByID(ctx context.Context, id int64) (*promo.SpecialOffer, error) {
	return m.spec, nil
}
func (m *mockPromoRepo) ListSpecialOffersByOrg(ctx context.Context, orgID int64) ([]*promo.SpecialOffer, error) {
	if m.spec != nil {
		return []*promo.SpecialOffer{m.spec}, nil
	}
	return nil, nil
}
func (m *mockPromoRepo) DeleteSpecialOffer(ctx context.Context, id, orgID int64) error {
	return nil
}
func (m *mockPromoRepo) AddSpecialOfferLocation(ctx context.Context, loc *promo.SpecialOfferLocation) error {
	return nil
}
func (m *mockPromoRepo) DeleteSpecialOfferLocation(ctx context.Context, id, offerID, orgID int64) error {
	return nil
}
func (m *mockPromoRepo) ListSpecialOfferLocations(ctx context.Context, offerID int64) ([]*promo.SpecialOfferLocation, error) {
	if m.spec != nil {
		return m.spec.Locations, nil
	}
	return nil, nil
}
func (m *mockPromoRepo) ListAllSpecialOffers(ctx context.Context, limit, offset int) ([]*promo.SpecialOffer, error) {
	if m.spec != nil {
		return []*promo.SpecialOffer{m.spec}, nil
	}
	return nil, nil
}
func (m *mockPromoRepo) ListAllSpecialOffersWithTotal(ctx context.Context, statusFilter string, limit, offset int) ([]*promo.SpecialOffer, int, error) {
	if m.spec != nil {
		return []*promo.SpecialOffer{m.spec}, 1, nil
	}
	return nil, 0, nil
}
func (m *mockPromoRepo) UpdateSpecialOfferAdminStatus(ctx context.Context, id int64, adminStatus, notes string, approvedBy int64) error {
	if m.spec != nil && m.spec.ID == id {
		m.spec.AdminStatus = adminStatus
		m.spec.AdminNotes = notes
	}
	return nil
}
func (m *mockPromoRepo) ToggleSpecialOfferStatus(ctx context.Context, id int64, isActive bool) error {
	return nil
}

// --- New sponsorship and ads methods (stubs) ---
func (m *mockPromoRepo) UpdatePackage(ctx context.Context, p *promo.OfferPackage) error { return nil }
func (m *mockPromoRepo) GetPackageByID(ctx context.Context, id int64) (*promo.OfferPackage, error) {
	return nil, nil
}
func (m *mockPromoRepo) AdminListPackages(ctx context.Context) ([]*promo.OfferPackage, error) {
	return nil, nil
}
func (m *mockPromoRepo) TogglePackageActive(ctx context.Context, id int64, active bool) error {
	return nil
}

func (m *mockPromoRepo) CreateSponsorshipPurchase(ctx context.Context, p *promo.SponsorshipPurchase) error {
	return nil
}
func (m *mockPromoRepo) GetSponsorshipPurchaseByID(ctx context.Context, id int64) (*promo.SponsorshipPurchase, error) {
	return nil, nil
}
func (m *mockPromoRepo) ListSponsorshipPurchasesByOrg(ctx context.Context, orgID int64) ([]*promo.SponsorshipPurchase, error) {
	return nil, nil
}
func (m *mockPromoRepo) ListActiveSponsorshipPurchasesByOrg(ctx context.Context, orgID int64) ([]*promo.SponsorshipPurchase, error) {
	return nil, nil
}
func (m *mockPromoRepo) ConsumeSponsorshipCredits(context.Context, promo.ConsumeCredits) (*promo.CreditEntry, error) {
	return &promo.CreditEntry{}, nil
}
func (m *mockPromoRepo) ListCreditEntries(context.Context, int64, int, int) ([]*promo.CreditEntry, int, error) {
	return nil, 0, nil
}
func (m *mockPromoRepo) ListOrgCreditEntries(context.Context, int64, int, int) ([]*promo.CreditEntry, int, error) {
	return nil, 0, nil
}
func (m *mockPromoRepo) CreditTotals(context.Context, int64) (int, int, error)         { return 0, 0, nil }
func (m *mockPromoRepo) ExpireSponsorshipPurchases(ctx context.Context) (int64, error) { return 0, nil }

func (m *mockPromoRepo) CreateSponsorshipRequest(ctx context.Context, r *promo.SponsorshipRequest) error {
	return nil
}
func (m *mockPromoRepo) GetSponsorshipRequestByID(ctx context.Context, id int64) (*promo.SponsorshipRequest, error) {
	return nil, nil
}
func (m *mockPromoRepo) ListSponsorshipRequestsByOrg(ctx context.Context, orgID int64, limit, offset int) ([]*promo.SponsorshipRequest, error) {
	return nil, nil
}
func (m *mockPromoRepo) ListSponsorshipRequestsByOrgWithTotal(ctx context.Context, orgID int64, limit, offset int) ([]*promo.SponsorshipRequest, int, error) {
	return nil, 0, nil
}
func (m *mockPromoRepo) ListAllSponsorshipRequests(ctx context.Context, limit, offset int) ([]*promo.SponsorshipRequest, error) {
	return nil, nil
}
func (m *mockPromoRepo) ListAllSponsorshipRequestsWithTotal(ctx context.Context, limit, offset int) ([]*promo.SponsorshipRequest, int, error) {
	return nil, 0, nil
}
func (m *mockPromoRepo) ListPendingSponsorshipRequests(ctx context.Context, limit, offset int) ([]*promo.SponsorshipRequest, error) {
	return nil, nil
}
func (m *mockPromoRepo) UpdateSponsorshipRequestAdminStatus(ctx context.Context, id int64, status promo.AdminStatus, notes string, reviewerID int64) error {
	return nil
}
func (m *mockPromoRepo) ActivateSponsorshipRequest(ctx context.Context, id int64, reviewerID int64) (*promo.SponsorshipRequest, error) {
	return nil, nil
}
func (m *mockPromoRepo) CancelSponsorshipRequest(ctx context.Context, id, orgID int64) error {
	return nil
}
func (m *mockPromoRepo) ExpireSponsorshipRequests(ctx context.Context) (int64, error) { return 0, nil }

func (m *mockPromoRepo) RankedSponsorshipsForProducts(ctx context.Context, productIDs []int64) ([]*promo.RankedSponsorship, error) {
	return nil, nil
}
func (m *mockPromoRepo) RankedSponsorshipsForOffers(ctx context.Context, offerIDs []int64) ([]*promo.RankedSponsorship, error) {
	return nil, nil
}
func (m *mockPromoRepo) ListActiveRankedSponsorships(ctx context.Context, itemType promo.SponsorshipItemType) ([]*promo.RankedSponsorship, error) {
	return nil, nil
}
func (m *mockPromoRepo) IsSponsored(ctx context.Context, itemType promo.SponsorshipItemType, itemID int64) (*promo.RankedSponsorship, error) {
	return nil, nil
}

func (m *mockPromoRepo) CreateAd(ctx context.Context, a *promo.Ad) error            { return nil }
func (m *mockPromoRepo) UpdateAd(ctx context.Context, a *promo.Ad) error            { return nil }
func (m *mockPromoRepo) GetAdByID(ctx context.Context, id int64) (*promo.Ad, error) { return nil, nil }
func (m *mockPromoRepo) ListAdsByOrg(ctx context.Context, orgID int64, limit, offset int) ([]*promo.Ad, error) {
	return nil, nil
}
func (m *mockPromoRepo) ListAllAds(ctx context.Context, limit, offset int) ([]*promo.Ad, error) {
	return nil, nil
}
func (m *mockPromoRepo) ListAdsByOrgWithTotal(ctx context.Context, orgID int64, limit, offset int) ([]*promo.Ad, int, error) {
	return nil, 0, nil
}
func (m *mockPromoRepo) ListAllAdsWithTotal(ctx context.Context, limit, offset int) ([]*promo.Ad, int, error) {
	return nil, 0, nil
}
func (m *mockPromoRepo) UpdateAdAdminStatus(ctx context.Context, id int64, status promo.AdminStatus, notes string, reviewerID int64) error {
	return nil
}
func (m *mockPromoRepo) SubmitAdEditRequest(ctx context.Context, id int64, changes *promo.AdPendingChanges) error {
	return nil
}
func (m *mockPromoRepo) ApproveAdEditRequest(ctx context.Context, id int64, reviewerID int64) error {
	return nil
}
func (m *mockPromoRepo) RejectAdEditRequest(ctx context.Context, id int64, reviewerID int64, notes string) error {
	return nil
}
func (m *mockPromoRepo) RecordAdImpression(ctx context.Context, adID int64, userID *int64, ip, ua string) error {
	return nil
}
func (m *mockPromoRepo) AdminToggleAd(ctx context.Context, id int64) (*promo.Ad, error) {
	return &promo.Ad{ID: id, IsActive: true}, nil
}
func (m *mockPromoRepo) UpdateSpecialOffer(ctx context.Context, o *promo.SpecialOffer) error {
	return nil
}
func (m *mockPromoRepo) ListAdminOfferLocations(ctx context.Context, filter promo.OfferLocationsFilter) ([]*promo.OfferLocationAdminRow, promo.OfferLocationsStats, int, error) {
	return nil, promo.OfferLocationsStats{}, 0, nil
}
