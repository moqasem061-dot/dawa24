package promo

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// CreateSponsorshipRequestReserved mirrors the repository: request, then credit.
func (m *mockPromoRepo) CreateSponsorshipRequestReserved(ctx context.Context, sr *SponsorshipRequest, in ConsumeCredits) error {
	if err := m.CreateSponsorshipRequest(ctx, sr); err != nil {
		return err
	}
	entityID := sr.ID
	in.EntityID = &entityID
	_, err := m.ConsumeSponsorshipCredits(ctx, in)
	return err
}

// CreateSponsorshipRequestsReserved mirrors the repository: requests, then credit.
func (m *mockPromoRepo) CreateSponsorshipRequestsReserved(ctx context.Context, reqs []*SponsorshipRequest, in ConsumeCredits) error {
	for _, sr := range reqs {
		if err := m.CreateSponsorshipRequest(ctx, sr); err != nil {
			return err
		}
	}
	if len(reqs) == 0 {
		return nil
	}
	_, err := m.ConsumeSponsorshipCredits(ctx, in)
	return err
}

// CreateSponsorshipPurchaseWithDebit mirrors the repository: debit, then purchase.
func (m *mockPromoRepo) CreateSponsorshipPurchaseWithDebit(ctx context.Context, p *SponsorshipPurchase, debit func(ctx context.Context, tx pgx.Tx) (*int64, error)) error {
	if debit != nil {
		paymentID, err := debit(ctx, nil)
		if err != nil {
			return err
		}
		p.SourceID = paymentID
	}
	return m.CreateSponsorshipPurchase(ctx, p)
}
