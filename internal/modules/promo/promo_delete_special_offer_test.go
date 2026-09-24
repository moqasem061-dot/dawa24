package promo

import (
	"context"
)

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
