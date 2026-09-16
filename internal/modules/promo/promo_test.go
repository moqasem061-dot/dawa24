package promo

import (
	"context"

	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

type mockPromoRepo struct {
	offers                        map[int64]*Offer
	packages                      map[int64]*OfferPackage
	sponsorships                  map[int64]*OfferSponsorship
	ads                           map[int64]*Ad
	sections                      map[int64]*HighlightSection
	nextID                        int64
	CreateSponsorshipPurchaseFunc func(context.Context, *SponsorshipPurchase) error
	ExpirePromotionsFunc          func(context.Context) ([]string, int64, error)
}

func newMockPromoRepo() *mockPromoRepo {
	return &mockPromoRepo{
		offers:       map[int64]*Offer{},
		packages:     map[int64]*OfferPackage{},
		sponsorships: map[int64]*OfferSponsorship{},
		ads:          map[int64]*Ad{},
		sections:     map[int64]*HighlightSection{},
		nextID:       1,
	}
}

func (m *mockPromoRepo) CreateOffer(_ context.Context, o *Offer) error {
	o.ID = m.nextID
	m.nextID++
	m.offers[o.ID] = o
	return nil
}

func (m *mockPromoRepo) GetOfferByID(_ context.Context, id int64) (*Offer, error) {
	o, ok := m.offers[id]
	if !ok {
		return nil, apperr.NotFound("offer")
	}
	return o, nil
}

func (m *mockPromoRepo) ListActiveOffers(_ context.Context, limit, offset int) ([]*Offer, error) {
	var list []*Offer
	for _, o := range m.offers {
		if o.IsActive {
			list = append(list, o)
		}
	}
	return list, nil
}

func (m *mockPromoRepo) ListOffersForProduct(_ context.Context, productID int64) ([]*OfferProductWithOffer, error) {
	var list []*OfferProductWithOffer
	for _, o := range m.offers {
		if !o.IsActive {
			continue
		}
		for _, pid := range o.ProductIDs {
			if pid == productID {
				list = append(list, &OfferProductWithOffer{
					Offer: o,
					Product: &OfferProduct{
						ProductID: pid,
						CustomQty: 1,
					},
				})
			}
		}
	}
	return list, nil
}

func (m *mockPromoRepo) ListOffersForProducts(_ context.Context, productIDs []int64) ([]*OfferProductWithOffer, error) {
	wanted := make(map[int64]struct{}, len(productIDs))
	for _, id := range productIDs {
		wanted[id] = struct{}{}
	}
	var list []*OfferProductWithOffer
	for _, o := range m.offers {
		if !o.IsActive {
			continue
		}
		for _, pid := range o.ProductIDs {
			if _, ok := wanted[pid]; ok {
				list = append(list, &OfferProductWithOffer{
					Offer:   o,
					Product: &OfferProduct{ProductID: pid, CustomQty: 1},
				})
			}
		}
	}
	return list, nil
}

func (m *mockPromoRepo) ListBuyerOffers(context.Context, BuyerOfferQuery) ([]*BuyerOffer, int, error) {
	return nil, 0, nil
}

func (m *mockPromoRepo) OfferVerdict(context.Context, BuyerOfferQuery) (OfferVerdict, error) {
	return OfferVerdict{}, nil
}

func (m *mockPromoRepo) ListRunningOffersByOrg(context.Context, int64, int) ([]*Offer, error) {
	return nil, nil
}

func (m *mockPromoRepo) ListOffers(_ context.Context, _, _ int) ([]*Offer, error) {
	var list []*Offer
	for _, o := range m.offers {
		list = append(list, o)
	}
	return list, nil
}

func (m *mockPromoRepo) SetOfferActive(_ context.Context, _ int64, _ bool) error {
	return nil
}

func (m *mockPromoRepo) IncrementOfferEngagement(_ context.Context, offerID int64, isClick bool) error {
	o, ok := m.offers[offerID]
	if !ok {
		return apperr.NotFound("offer")
	}
	if isClick {
		o.ClicksCount++
	} else {
		o.ViewsCount++
	}
	return nil
}

func (m *mockPromoRepo) CreatePackage(_ context.Context, p *OfferPackage) error {
	p.ID = m.nextID
	m.nextID++
	m.packages[p.ID] = p
	return nil
}

func (m *mockPromoRepo) ListPackages(_ context.Context) ([]*OfferPackage, error) {
	var list []*OfferPackage
	for _, p := range m.packages {
		list = append(list, p)
	}
	return list, nil
}

func (m *mockPromoRepo) CreateSponsorship(_ context.Context, s *OfferSponsorship) error {
	s.ID = m.nextID
	m.nextID++
	m.sponsorships[s.ID] = s
	return nil
}

func (m *mockPromoRepo) ListActiveAds(_ context.Context, position string) ([]*Ad, error) {
	var list []*Ad
	for _, a := range m.ads {
		if a.IsActive && (position == "" || a.Position == position) {
			list = append(list, a)
		}
	}
	return list, nil
}

func (m *mockPromoRepo) RecordAdClick(_ context.Context, adID int64, userID *int64, ip, ua string) error {
	if a, ok := m.ads[adID]; ok {
		a.Clicks++
	}
	return nil
}
func (m *mockPromoRepo) UpdatePackage(_ context.Context, _ *OfferPackage) error { return nil }
func (m *mockPromoRepo) GetPackageByID(_ context.Context, id int64) (*OfferPackage, error) {
	if p, ok := m.packages[id]; ok {
		return p, nil
	}
	return nil, apperr.NotFound("package")
}
func (m *mockPromoRepo) AdminListPackages(_ context.Context) ([]*OfferPackage, error) {
	return nil, nil
}
func (m *mockPromoRepo) TogglePackageActive(_ context.Context, _ int64, _ bool) error {
	return nil
}
func (m *mockPromoRepo) CreateSponsorshipPurchase(ctx context.Context, p *SponsorshipPurchase) error {
	if m.CreateSponsorshipPurchaseFunc != nil {
		return m.CreateSponsorshipPurchaseFunc(ctx, p)
	}
	return nil
}
func (m *mockPromoRepo) GetSponsorshipPurchaseByID(_ context.Context, _ int64) (*SponsorshipPurchase, error) {
	return nil, nil
}
func (m *mockPromoRepo) ListSponsorshipPurchasesByOrg(_ context.Context, _ int64) ([]*SponsorshipPurchase, error) {
	return nil, nil
}
func (m *mockPromoRepo) ListActiveSponsorshipPurchasesByOrg(_ context.Context, _ int64) ([]*SponsorshipPurchase, error) {
	return nil, nil
}
func (m *mockPromoRepo) ConsumeSponsorshipCredits(context.Context, ConsumeCredits) (*CreditEntry, error) {
	return &CreditEntry{}, nil
}
func (m *mockPromoRepo) ListCreditEntries(context.Context, int64, int, int) ([]*CreditEntry, int, error) {
	return nil, 0, nil
}
func (m *mockPromoRepo) ListOrgCreditEntries(context.Context, int64, int, int) ([]*CreditEntry, int, error) {
	return nil, 0, nil
}
func (m *mockPromoRepo) ExpireSponsorshipPurchases(_ context.Context) (int64, error) {
	return 0, nil
}
func (m *mockPromoRepo) CreateSponsorshipRequest(_ context.Context, _ *SponsorshipRequest) error {
	return nil
}
func (m *mockPromoRepo) GetSponsorshipRequestByID(_ context.Context, _ int64) (*SponsorshipRequest, error) {
	return nil, nil
}
func (m *mockPromoRepo) ListSponsorshipRequestsByOrg(_ context.Context, _ int64, _, _ int) ([]*SponsorshipRequest, error) {
	return nil, nil
}
func (m *mockPromoRepo) ListSponsorshipRequestsByOrgWithTotal(_ context.Context, _ int64, _, _ int) ([]*SponsorshipRequest, int, error) {
	return nil, 0, nil
}
func (m *mockPromoRepo) ListAllSponsorshipRequests(_ context.Context, _, _ int) ([]*SponsorshipRequest, error) {
	return nil, nil
}
func (m *mockPromoRepo) ListAllSponsorshipRequestsWithTotal(_ context.Context, _, _ int) ([]*SponsorshipRequest, int, error) {
	return nil, 0, nil
}
func (m *mockPromoRepo) ListPendingSponsorshipRequests(_ context.Context, _, _ int) ([]*SponsorshipRequest, error) {
	return nil, nil
}
func (m *mockPromoRepo) UpdateSponsorshipRequestAdminStatus(_ context.Context, _ int64, _ AdminStatus, _ string, _ int64) error {
	return nil
}
func (m *mockPromoRepo) ActivateSponsorshipRequest(_ context.Context, _ int64, _ int64) (*SponsorshipRequest, error) {
	return nil, nil
}
func (m *mockPromoRepo) CancelSponsorshipRequest(_ context.Context, _, _ int64) error { return nil }
func (m *mockPromoRepo) ExpireSponsorshipRequests(_ context.Context) (int64, error) {
	return 0, nil
}
func (m *mockPromoRepo) RankedSponsorshipsForProducts(_ context.Context, _ []int64) ([]*RankedSponsorship, error) {
	return nil, nil
}
func (m *mockPromoRepo) RankedSponsorshipsForOffers(_ context.Context, _ []int64) ([]*RankedSponsorship, error) {
	return nil, nil
}
func (m *mockPromoRepo) ListActiveRankedSponsorships(_ context.Context, _ SponsorshipItemType) ([]*RankedSponsorship, error) {
	return nil, nil
}
func (m *mockPromoRepo) IsSponsored(_ context.Context, _ SponsorshipItemType, _ int64) (*RankedSponsorship, error) {
	return nil, nil
}
func (m *mockPromoRepo) CreateAd(_ context.Context, _ *Ad) error { return nil }
func (m *mockPromoRepo) UpdateAd(_ context.Context, _ *Ad) error { return nil }
func (m *mockPromoRepo) GetAdByID(_ context.Context, _ int64) (*Ad, error) {
	return nil, nil
}
func (m *mockPromoRepo) ListAdsByOrg(_ context.Context, _ int64, _, _ int) ([]*Ad, error) {
	return nil, nil
}
func (m *mockPromoRepo) ListAdsByOrgWithTotal(_ context.Context, _ int64, _, _ int) ([]*Ad, int, error) {
	return nil, 0, nil
}
func (m *mockPromoRepo) ListAllAds(_ context.Context, _, _ int) ([]*Ad, error) {
	return nil, nil
}
func (m *mockPromoRepo) ListAllAdsWithTotal(_ context.Context, _, _ int) ([]*Ad, int, error) {
	return nil, 0, nil
}
func (m *mockPromoRepo) UpdateAdAdminStatus(_ context.Context, _ int64, _ AdminStatus, _ string, _ int64) error {
	return nil
}
func (m *mockPromoRepo) AdminToggleAd(_ context.Context, id int64) (*Ad, error) {
	return &Ad{ID: id, IsActive: true}, nil
}
func (m *mockPromoRepo) SubmitAdEditRequest(_ context.Context, _ int64, _ *AdPendingChanges) error {
	return nil
}
func (m *mockPromoRepo) ApproveAdEditRequest(_ context.Context, _ int64, _ int64) error {
	return nil
}
func (m *mockPromoRepo) RejectAdEditRequest(_ context.Context, _ int64, _ int64, _ string) error {
	return nil
}
func (m *mockPromoRepo) RecordAdImpression(_ context.Context, _ int64, _ *int64, _, _ string) error {
	return nil
}

func (m *mockPromoRepo) CreateHighlightSection(_ context.Context, h *HighlightSection) error {
	h.ID = m.nextID
	m.nextID++
	m.sections[h.ID] = h
	return nil
}

func (m *mockPromoRepo) ListHighlightSections(_ context.Context) ([]*HighlightSection, error) {
	var list []*HighlightSection
	for _, s := range m.sections {
		list = append(list, s)
	}
	return list, nil
}

func (m *mockPromoRepo) ListHighlightSectionsByOrg(_ context.Context, orgID int64) ([]*HighlightSection, error) {
	var list []*HighlightSection
	for _, s := range m.sections {
		if s.OwnerType == "organization" && s.OrganizationID != nil && *s.OrganizationID == orgID {
			list = append(list, s)
		}
	}
	return list, nil
}

func (m *mockPromoRepo) UpdateHighlightSection(_ context.Context, s *HighlightSection) error {
	m.sections[s.ID] = s
	return nil
}

func (m *mockPromoRepo) DeleteHighlightSection(_ context.Context, id, orgID int64) error {
	delete(m.sections, id)
	return nil
}

func (m *mockPromoRepo) GetHighlightSectionByID(_ context.Context, id int64) (*HighlightSection, error) {
	return m.sections[id], nil
}

func (m *mockPromoRepo) AddHighlightItem(_ context.Context, item *HighlightSectionItem) error {
	item.ID = m.nextID
	m.nextID++
	return nil
}

func (m *mockPromoRepo) ListHighlightItems(_ context.Context, _ int64) ([]*HighlightSectionItem, error) {
	return nil, nil
}

func (m *mockPromoRepo) ExpirePromotions(_ context.Context) (int64, error) {
	return 0, nil
}

func (m *mockPromoRepo) ExpirePromotionsAndCollectMedia(ctx context.Context) ([]string, int64, error) {
	if m.ExpirePromotionsFunc != nil {
		return m.ExpirePromotionsFunc(ctx)
	}
	return nil, 0, nil
}

func (m *mockPromoRepo) CreateSpecialOffer(_ context.Context, o *SpecialOffer) error {
	o.ID = 1
	return nil
}
func (m *mockPromoRepo) UpdateSpecialOffer(_ context.Context, _ *SpecialOffer) error {
	return nil
}
func (m *mockPromoRepo) GetSpecialOfferByID(_ context.Context, id int64) (*SpecialOffer, error) {
	return &SpecialOffer{ID: id, Title: i18n.New("عرض تجريبي", "Demo Special Offer")}, nil
}
func (m *mockPromoRepo) ListSpecialOffersByOrg(_ context.Context, _ int64) ([]*SpecialOffer, error) {
	return []*SpecialOffer{{ID: 1, Title: i18n.New("عرض تجريبي", "Demo Special Offer")}}, nil
}
func (m *mockPromoRepo) ListAllSpecialOffers(_ context.Context, _, _ int) ([]*SpecialOffer, error) {
	return []*SpecialOffer{{ID: 1, Title: i18n.New("عرض تجريبي", "Demo Special Offer")}}, nil
}
func (m *mockPromoRepo) ListAllSpecialOffersWithTotal(_ context.Context, _ string, _, _ int) ([]*SpecialOffer, int, error) {
	return []*SpecialOffer{{ID: 1, Title: i18n.New("عرض تجريبي", "Demo Special Offer")}}, 1, nil
}
func (m *mockPromoRepo) UpdateSpecialOfferAdminStatus(_ context.Context, _ int64, _, _ string, _ int64) error {
	return nil
}
func (m *mockPromoRepo) ToggleSpecialOfferStatus(_ context.Context, _ int64, _ bool) error {
	return nil
}
func (m *mockPromoRepo) DeleteSpecialOffer(_ context.Context, _, _ int64) error {
	return nil
}
func (m *mockPromoRepo) AddSpecialOfferLocation(_ context.Context, loc *SpecialOfferLocation) error {
	loc.ID = 1
	return nil
}
func (m *mockPromoRepo) ListSpecialOfferLocations(_ context.Context, _ int64) ([]*SpecialOfferLocation, error) {
	return []*SpecialOfferLocation{{ID: 1, Radius: 1000}}, nil
}
func (m *mockPromoRepo) DeleteSpecialOfferLocation(_ context.Context, _, _, _ int64) error {
	return nil
}
func (m *mockPromoRepo) ListAdminOfferLocations(context.Context, OfferLocationsFilter) ([]*OfferLocationAdminRow, OfferLocationsStats, int, error) {
	return nil, OfferLocationsStats{}, 0, nil
}

func (m *mockPromoRepo) CreditTotals(context.Context, int64) (int, int, error) { return 0, 0, nil }
func (m *mockPromoRepo) ListCreditAccounts(context.Context, string, int, int) ([]*CreditAccount, int, error) {
	return nil, 0, nil
}
func (m *mockPromoRepo) ListPurchasesForOrg(context.Context, int64, int, int) ([]*SponsorshipPurchase, int, error) {
	return nil, 0, nil
}
