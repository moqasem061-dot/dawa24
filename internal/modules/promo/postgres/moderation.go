package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/promo"
	"github.com/muhiya/dawa24-store/internal/platform/database"
)

// ListOffers returns all offers (any lifecycle state) for admin moderation.
func (r *Repository) ListOffers(ctx context.Context, limit, offset int) ([]*promo.Offer, error) {
	var offers []*promo.Offer
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		const query = `
			SELECT ` + offerColumns + `
			FROM promo.offers
			WHERE deleted_at IS NULL
			ORDER BY id DESC
			LIMIT $1 OFFSET $2;
		`
		if limit <= 0 || limit > 100 {
			limit = 20
		}
		rows, err := tx.Query(txCtx, query, limit, offset)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			o, err := scanOffer(rows)
			if err != nil {
				return err
			}
			offers = append(offers, o)
		}
		return rows.Err()
	})
	return offers, err
}

// SetOfferActive activates or deactivates an offer.
func (r *Repository) SetOfferActive(ctx context.Context, id int64, active bool) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(txCtx, `UPDATE promo.offers SET is_active = $1, updated_at = now() WHERE id = $2;`, active, id)
		return err
	})
}
