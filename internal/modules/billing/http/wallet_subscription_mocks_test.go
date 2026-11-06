package http_test

import (
	"context"

	"github.com/moqasem061-dot/dawa24/internal/modules/billing"
)

// PurchaseWalletSubscription mirrors the repository: subscription first, then its debit.
func (r stubRepo) PurchaseWalletSubscription(ctx context.Context, p billing.WalletSubscriptionPurchase) (*billing.Subscription, error) {
	sub := p.Subscription
	if err := r.CreateSubscription(ctx, &sub); err != nil {
		return nil, err
	}
	if _, err := r.RecordTransaction(ctx, p.WalletID, billing.TxPurchase, p.Delta, p.RefType, &sub.ID, p.Description); err != nil {
		return nil, err
	}
	return &sub, nil
}

// PurchaseWalletSubscription mirrors the repository: subscription first, then its debit.
func (h happyRepo) PurchaseWalletSubscription(ctx context.Context, p billing.WalletSubscriptionPurchase) (*billing.Subscription, error) {
	sub := p.Subscription
	if err := h.CreateSubscription(ctx, &sub); err != nil {
		return nil, err
	}
	if _, err := h.RecordTransaction(ctx, p.WalletID, billing.TxPurchase, p.Delta, p.RefType, &sub.ID, p.Description); err != nil {
		return nil, err
	}
	return &sub, nil
}
