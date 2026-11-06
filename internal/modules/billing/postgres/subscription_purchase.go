package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/moqasem061-dot/dawa24/internal/modules/billing"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
	"github.com/moqasem061-dot/dawa24/internal/shared/apperr"
)

// supersedeLiveSubscriptions expires the live subscription rows of the owner of
// sub (its organisation, or the user when there is none).
func supersedeLiveSubscriptions(ctx context.Context, tx pgx.Tx, sub *billing.Subscription) error {
	if sub.OrganizationID != nil && *sub.OrganizationID > 0 {
		_, err := tx.Exec(ctx, `
			UPDATE billing.subscriptions
			SET status = 'expired', auto_renew = false, updated_at = now()
			WHERE organization_id = $1
			  AND status IN ('active', 'trialing', 'past_due');
		`, *sub.OrganizationID)
		return err
	}
	_, err := tx.Exec(ctx, `
		UPDATE billing.subscriptions
		SET status = 'expired', auto_renew = false, updated_at = now()
		WHERE user_id = $1
		  AND organization_id IS NULL
		  AND status IN ('active', 'trialing', 'past_due');
	`, sub.UserID)
	return err
}

// insertSubscription writes one subscription row and fills in its generated fields.
func insertSubscription(ctx context.Context, tx pgx.Tx, sub *billing.Subscription) error {
	if sub.BillingCycle == "" {
		sub.BillingCycle = "monthly"
	}
	return tx.QueryRow(ctx, `
		INSERT INTO billing.subscriptions (
			user_id, organization_id, plan_id, status, billing_cycle, auto_renew,
			starts_at, expires_at, last_renewed_at, renewal_attempts, source_system, source_id
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		RETURNING id, public_id, created_at, updated_at;
	`,
		sub.UserID, sub.OrganizationID, sub.PlanID, string(sub.Status),
		sub.BillingCycle, sub.AutoRenew, sub.StartsAt, sub.ExpiresAt,
		sub.LastRenewedAt, sub.RenewalAttempts, sub.SourceSystem, sub.SourceID,
	).Scan(&sub.ID, &sub.PublicID, &sub.CreatedAt, &sub.UpdatedAt)
}

// liveSubscriptionID returns the id of the subscription that is current for the
// owner of sub, or nil when there is none.
func liveSubscriptionID(ctx context.Context, tx pgx.Tx, sub *billing.Subscription) (*int64, error) {
	var id int64
	var err error
	if sub.OrganizationID != nil && *sub.OrganizationID > 0 {
		err = tx.QueryRow(ctx, `
			SELECT id FROM billing.subscriptions
			WHERE organization_id = $1 AND status = 'active' AND expires_at > now()
			ORDER BY starts_at DESC, id DESC LIMIT 1;`, *sub.OrganizationID).Scan(&id)
	} else {
		err = tx.QueryRow(ctx, `
			SELECT id FROM billing.subscriptions
			WHERE user_id = $1 AND status = 'active' AND expires_at > now()
			ORDER BY starts_at DESC, id DESC LIMIT 1;`, sub.UserID).Scan(&id)
	}
	if database.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &id, nil
}

func sameSubscriptionID(a, b *int64) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

// PurchaseWalletSubscription creates a subscription and records its wallet debit
// in one transaction. The wallet is locked first, so concurrent purchases for one
// wallet serialise. The subscription that was current when the caller decided the
// purchase must still be current; otherwise the purchase is refused. A failed
// debit rolls the subscription back too, so no active subscription is left
// without its payment.
func (r *Repository) PurchaseWalletSubscription(ctx context.Context, p billing.WalletSubscriptionPurchase) (*billing.Subscription, error) {
	sub := p.Subscription
	err := r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		if err := lockWalletRow(txCtx, tx, p.WalletID); err != nil {
			return err
		}
		current, err := liveSubscriptionID(txCtx, tx, &sub)
		if err != nil {
			return err
		}
		if !sameSubscriptionID(current, p.ExpectedCurrentSubID) {
			return apperr.Conflict("subscription.changed",
				"تغيّر اشتراكك أثناء معالجة الطلب. يرجى المحاولة مرة أخرى. Your subscription changed while this request was processed. Please try again.")
		}
		if err := supersedeLiveSubscriptions(txCtx, tx, &sub); err != nil {
			return err
		}
		if err := insertSubscription(txCtx, tx, &sub); err != nil {
			return err
		}
		_, err = r.recordLedgerRow(txCtx, tx, p.WalletID, billing.TxPurchase, p.Delta, p.RefType, &sub.ID, p.Description, "")
		return err
	})
	if err != nil {
		return nil, err
	}
	return &sub, nil
}
