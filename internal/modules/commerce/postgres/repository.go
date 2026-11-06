package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/moqasem061-dot/dawa24/internal/platform/database"
	"github.com/moqasem061-dot/dawa24/internal/shared/money"
)

// Repository implements commerce.Repository using PostgreSQL.
type Repository struct {
	db       *database.DB
	refunder WalletOrderRefunder
	payer    WalletOrderPayer
}

// NewRepository creates a new PostgreSQL commerce repository.
func NewRepository(db *database.DB) *Repository {
	return &Repository{db: db}
}

// WalletOrderRefunder credits a wallet from inside the commerce transaction, so
// that a cancellation and its refund commit together. The billing repository
// implements it; the composition root wires it with WithWalletRefunder. The
// commerce module never imports billing.
type WalletOrderRefunder interface {
	CreditOrderRefund(ctx context.Context, tx pgx.Tx, orderID int64, capAmount *money.Amount, key, description string, actorID int64) (money.Amount, error)
	OrderNetDebit(ctx context.Context, tx pgx.Tx, orderID int64) (money.Amount, error)
}

// WithWalletRefunder sets the refunder used for wallet-paid orders.
func (r *Repository) WithWalletRefunder(refunder WalletOrderRefunder) *Repository {
	r.refunder = refunder
	return r
}

// WalletOrderPayer debits a buyer's wallet for one supplier shipment, inside the
// commerce transaction that confirms the shipment. The billing repository
// implements it; the composition root wires it with WithWalletPayer.
type WalletOrderPayer interface {
	DebitOrderShipment(ctx context.Context, tx pgx.Tx, walletUserID, orderID, shipmentID int64, amount money.Amount, description string, actorUserID int64) error
}

// WithWalletPayer sets the payer used to charge wallet-paid shipments.
func (r *Repository) WithWalletPayer(payer WalletOrderPayer) *Repository {
	r.payer = payer
	return r
}
