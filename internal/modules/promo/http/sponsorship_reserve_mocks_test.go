package http_test

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/moqasem061-dot/dawa24/internal/modules/promo"
)

// CreateSponsorshipRequestReserved mirrors the repository: request, then credit.
func (r stubRepo) CreateSponsorshipRequestReserved(ctx context.Context, sr *promo.SponsorshipRequest, in promo.ConsumeCredits) error {
	if err := r.CreateSponsorshipRequest(ctx, sr); err != nil {
		return err
	}
	entityID := sr.ID
	in.EntityID = &entityID
	_, err := r.ConsumeSponsorshipCredits(ctx, in)
	return err
}

// CreateSponsorshipRequestsReserved mirrors the repository: requests, then credit.
func (r stubRepo) CreateSponsorshipRequestsReserved(ctx context.Context, reqs []*promo.SponsorshipRequest, in promo.ConsumeCredits) error {
	for _, sr := range reqs {
		if err := r.CreateSponsorshipRequest(ctx, sr); err != nil {
			return err
		}
	}
	if len(reqs) == 0 {
		return nil
	}
	_, err := r.ConsumeSponsorshipCredits(ctx, in)
	return err
}

// CreateSponsorshipPurchaseWithDebit mirrors the repository: debit, then purchase.
func (r stubRepo) CreateSponsorshipPurchaseWithDebit(ctx context.Context, p *promo.SponsorshipPurchase, debit func(ctx context.Context, tx pgx.Tx) (*int64, error)) error {
	if debit != nil {
		paymentID, err := debit(ctx, nil)
		if err != nil {
			return err
		}
		p.SourceID = paymentID
	}
	return r.CreateSponsorshipPurchase(ctx, p)
}

// CreateSponsorshipRequestReserved mirrors the repository: request, then credit.
func (h happyRepo) CreateSponsorshipRequestReserved(ctx context.Context, sr *promo.SponsorshipRequest, in promo.ConsumeCredits) error {
	if err := h.CreateSponsorshipRequest(ctx, sr); err != nil {
		return err
	}
	entityID := sr.ID
	in.EntityID = &entityID
	_, err := h.ConsumeSponsorshipCredits(ctx, in)
	return err
}

// CreateSponsorshipRequestsReserved mirrors the repository: requests, then credit.
func (h happyRepo) CreateSponsorshipRequestsReserved(ctx context.Context, reqs []*promo.SponsorshipRequest, in promo.ConsumeCredits) error {
	for _, sr := range reqs {
		if err := h.CreateSponsorshipRequest(ctx, sr); err != nil {
			return err
		}
	}
	if len(reqs) == 0 {
		return nil
	}
	_, err := h.ConsumeSponsorshipCredits(ctx, in)
	return err
}

// CreateSponsorshipPurchaseWithDebit mirrors the repository: debit, then purchase.
func (h happyRepo) CreateSponsorshipPurchaseWithDebit(ctx context.Context, p *promo.SponsorshipPurchase, debit func(ctx context.Context, tx pgx.Tx) (*int64, error)) error {
	if debit != nil {
		paymentID, err := debit(ctx, nil)
		if err != nil {
			return err
		}
		p.SourceID = paymentID
	}
	return h.CreateSponsorshipPurchase(ctx, p)
}
