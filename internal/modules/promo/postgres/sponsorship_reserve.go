package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/moqasem061-dot/dawa24/internal/modules/promo"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
)

// Batch 2 (S24): a sponsorship request and the credit that pays for it are written
// in one transaction. The credit UPDATE locks the purchase row, so concurrent
// submissions serialise: credits used never pass credits bought, and a request
// whose credit is refused is never left pending.

func insertSponsorshipRequestTx(ctx context.Context, tx pgx.Tx, sr *promo.SponsorshipRequest) error {
	return tx.QueryRow(ctx, `
		INSERT INTO promo.sponsorship_requests (
			organization_id, purchase_id, package_id, item_type, item_id,
			credits_used, admin_status, starts_at, expires_at, status
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING id, public_id, created_at, updated_at;
	`,
		sr.OrganizationID, sr.PurchaseID, sr.PackageID, string(sr.ItemType), sr.ItemID,
		sr.CreditsUsed, string(sr.AdminStatus), sr.StartsAt, sr.ExpiresAt, string(sr.Status),
	).Scan(&sr.ID, &sr.PublicID, &sr.CreatedAt, &sr.UpdatedAt)
}

func insertSponsorshipPurchaseTx(ctx context.Context, tx pgx.Tx, p *promo.SponsorshipPurchase) error {
	return tx.QueryRow(ctx, `
		INSERT INTO promo.sponsorship_purchases (
			organization_id, package_id, credits_total, credits_used,
			starts_at, expires_at, status, auto_renew, billing_cycle,
			amount, payment_id, source_system, source_id
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		RETURNING id, public_id, created_at, updated_at;
	`,
		p.OrganizationID, p.PackageID, p.CreditsTotal, p.CreditsUsed,
		p.StartsAt, p.ExpiresAt, string(p.Status), p.AutoRenew, p.BillingCycle,
		p.Amount, p.PaymentID, p.SourceSystem, p.SourceID,
	).Scan(&p.ID, &p.PublicID, &p.CreatedAt, &p.UpdatedAt)
}

// CreateSponsorshipRequestReserved creates one request and takes its credit in one
// transaction. If the credit is refused, the request is not created either.
func (r *Repository) CreateSponsorshipRequestReserved(ctx context.Context, sr *promo.SponsorshipRequest, in promo.ConsumeCredits) error {
	if err := in.Validate(); err != nil {
		return err
	}
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		if err := insertSponsorshipRequestTx(txCtx, tx, sr); err != nil {
			return err
		}
		entityID := sr.ID
		in.EntityID = &entityID
		_, err := consumeCreditsTx(txCtx, tx, in)
		return err
	})
}

// CreateSponsorshipRequestsReserved creates a batch of requests and takes their
// combined credit in one transaction: all of them, or none.
func (r *Repository) CreateSponsorshipRequestsReserved(ctx context.Context, reqs []*promo.SponsorshipRequest, in promo.ConsumeCredits) error {
	if len(reqs) == 0 {
		return nil
	}
	if err := in.Validate(); err != nil {
		return err
	}
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		for _, sr := range reqs {
			if err := insertSponsorshipRequestTx(txCtx, tx, sr); err != nil {
				return err
			}
		}
		_, err := consumeCreditsTx(txCtx, tx, in)
		return err
	})
}

// CreateSponsorshipPurchaseWithDebit records a purchase and, when debit is given,
// the wallet payment for it, in one transaction. The payment id returned by debit
// is stored on the purchase. If either step fails, neither is kept.
//
// The transaction runs as system: the wallet and its ledger rows carry no
// organization, so under a vendor's tenant context row-level security hides them
// and every debit would read a zero balance. The purchase row still names its
// organization (set by the service from the caller's own tenant).
func (r *Repository) CreateSponsorshipPurchaseWithDebit(
	ctx context.Context,
	p *promo.SponsorshipPurchase,
	debit func(ctx context.Context, tx pgx.Tx) (*int64, error),
) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		if debit != nil {
			paymentID, err := debit(txCtx, tx)
			if err != nil {
				return err
			}
			p.SourceID = paymentID
		}
		return insertSponsorshipPurchaseTx(txCtx, tx, p)
	})
}
