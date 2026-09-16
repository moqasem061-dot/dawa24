package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/telegram"
)

// ClaimDeliveries leases due messages for active links. A lease that expires
// without a report is claimable again, up to the shared attempt limit, so a
// crashed run neither loses a message nor retries it for ever.
func (r *Repository) ClaimDeliveries(ctx context.Context, limit int, lease time.Duration) ([]telegram.Outgoing, error) {
	var out []telegram.Outgoing
	err := r.db.InTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if err := r.ExpireLeases(ctx, tx); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, r.DueDeliveriesSQL()+`d.id, k.chat_id, d.text;`, limit, lease.Seconds())
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var o telegram.Outgoing
			if err := rows.Scan(&o.ID, &o.ChatID, &o.Text); err != nil {
				return err
			}
			out = append(out, o)
		}
		return rows.Err()
	})
	return out, err
}
