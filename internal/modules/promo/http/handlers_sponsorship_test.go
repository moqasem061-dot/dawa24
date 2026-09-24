package http_test

import (
	"context"

	"github.com/moqasem061-dot/dawa24/internal/modules/promo"
)

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
