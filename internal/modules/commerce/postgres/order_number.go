package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// NextOrderNumber draws the next order number from commerce.order_number_seq.
// The sequence is not tenant data, but the call still runs in a transaction so
// it follows the module's rule for every database access. It implements
// commerce.OrderNumberAllocator.
func (r *Repository) NextOrderNumber(ctx context.Context) (string, error) {
	var number string
	err := r.db.InTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT nextval('commerce.order_number_seq')::text`).Scan(&number)
	})
	return number, err
}
