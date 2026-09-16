package catalog

import (
	"context"
)

// ListBuyerOffers returns a paginated list of sellable offers for a buyer.
func (s *Service) ListBuyerOffers(ctx context.Context, q BuyerOfferQuery) ([]*BuyerOffer, int, error) {
	if s.repo == nil {
		return nil, 0, nil
	}
	return s.repo.ListBuyerOffers(ctx, q)
}
