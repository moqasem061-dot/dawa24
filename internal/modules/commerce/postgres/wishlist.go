package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/platform/database"
)

// AddToWishlist adds a product to a user's wishlist if not already present.
func (r *Repository) AddToWishlist(ctx context.Context, userID int64, productID int64) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			INSERT INTO commerce.wishlists (user_id, product_id)
			VALUES ($1, $2)
			ON CONFLICT (user_id, product_id) DO NOTHING;
		`
		_, err := tx.Exec(txCtx, query, userID, productID)
		return err
	})
}

// RemoveFromWishlist removes a product from a user's wishlist.
func (r *Repository) RemoveFromWishlist(ctx context.Context, userID int64, productID int64) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `DELETE FROM commerce.wishlists WHERE user_id = $1 AND product_id = $2;`
		_, err := tx.Exec(txCtx, query, userID, productID)
		return err
	})
}

// ListWishlist returns all items currently in a user's wishlist.
func (r *Repository) ListWishlist(ctx context.Context, userID int64) ([]*commerce.WishlistItem, error) {
	var items []*commerce.WishlistItem
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, public_id, user_id, product_id, created_at
			FROM commerce.wishlists
			WHERE user_id = $1
			ORDER BY created_at DESC;
		`
		rows, err := tx.Query(txCtx, query, userID)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var w commerce.WishlistItem
			if err := rows.Scan(&w.ID, &w.PublicID, &w.UserID, &w.ProductID, &w.CreatedAt); err != nil {
				return err
			}
			items = append(items, &w)
		}
		return rows.Err()
	})
	return items, err
}
