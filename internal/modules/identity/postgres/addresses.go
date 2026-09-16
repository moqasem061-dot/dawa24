package postgres

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/identity"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// writeAddressHistory appends one row to the address audit trail, in the same
// transaction as the change it describes.
func writeAddressHistory(ctx context.Context, tx pgx.Tx, userID, addressID int64, event string, snapshot any) error {
	b, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}
	const query = `INSERT INTO identity.user_address_histories (user_id, address_id, event, snapshot) VALUES ($1, $2, $3, $4);`
	_, err = tx.Exec(ctx, query, userID, addressID, event, b)
	return err
}

// CreateAddress saves a new user shipping/billing address.
func (r *Repository) CreateAddress(ctx context.Context, addr *identity.UserAddress) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		if addr.IsDefault {
			_, err := tx.Exec(txCtx, `UPDATE identity.user_addresses SET is_default = false WHERE user_id = $1;`, addr.UserID)
			if err != nil {
				return err
			}
		}
		query := `
			INSERT INTO identity.user_addresses (
				user_id, title, recipient, phone, city_id, address, building, floor, apartment, is_default, created_at, updated_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, now(), now())
			RETURNING id, created_at, updated_at;
		`
		if err := tx.QueryRow(txCtx, query,
			addr.UserID, addr.Title, addr.Recipient, addr.Phone, addr.CityID,
			addr.Address, addr.Building, addr.Floor, addr.Apartment, addr.IsDefault,
		).Scan(&addr.ID, &addr.CreatedAt, &addr.UpdatedAt); err != nil {
			return err
		}
		return writeAddressHistory(txCtx, tx, addr.UserID, addr.ID, "created", addr)
	})
}

// GetAddressByID retrieves a user's address.
func (r *Repository) GetAddressByID(ctx context.Context, id, userID int64) (*identity.UserAddress, error) {
	var a identity.UserAddress
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, user_id, title, recipient, phone, city_id, address, building, floor, apartment, is_default, created_at, updated_at
			FROM identity.user_addresses
			WHERE id = $1 AND user_id = $2;
		`
		var building, floor, apt *string
		err := tx.QueryRow(txCtx, query, id, userID).Scan(
			&a.ID, &a.UserID, &a.Title, &a.Recipient, &a.Phone, &a.CityID,
			&a.Address, &building, &floor, &apt, &a.IsDefault, &a.CreatedAt, &a.UpdatedAt,
		)
		if err != nil {
			if database.IsNotFound(err) {
				return apperr.NotFound("user_address")
			}
			return err
		}
		if building != nil {
			a.Building = *building
		}
		if floor != nil {
			a.Floor = *floor
		}
		if apt != nil {
			a.Apartment = *apt
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &a, nil
}

// ListAddresses returns all addresses saved by a user.
func (r *Repository) ListAddresses(ctx context.Context, userID int64) ([]*identity.UserAddress, error) {
	var list []*identity.UserAddress
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, user_id, title, recipient, phone, city_id, address, building, floor, apartment, is_default, created_at, updated_at
			FROM identity.user_addresses
			WHERE user_id = $1
			ORDER BY is_default DESC, id DESC;
		`
		rows, err := tx.Query(txCtx, query, userID)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var a identity.UserAddress
			var building, floor, apt *string
			if err := rows.Scan(
				&a.ID, &a.UserID, &a.Title, &a.Recipient, &a.Phone, &a.CityID,
				&a.Address, &building, &floor, &apt, &a.IsDefault, &a.CreatedAt, &a.UpdatedAt,
			); err != nil {
				return err
			}
			if building != nil {
				a.Building = *building
			}
			if floor != nil {
				a.Floor = *floor
			}
			if apt != nil {
				a.Apartment = *apt
			}
			list = append(list, &a)
		}
		return rows.Err()
	})
	return list, err
}

// UpdateAddress modifies a user's address.
func (r *Repository) UpdateAddress(ctx context.Context, addr *identity.UserAddress) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		if addr.IsDefault {
			_, err := tx.Exec(txCtx, `UPDATE identity.user_addresses SET is_default = false WHERE user_id = $1;`, addr.UserID)
			if err != nil {
				return err
			}
		}
		query := `
			UPDATE identity.user_addresses
			SET title = $1, recipient = $2, phone = $3, city_id = $4, address = $5,
			    building = $6, floor = $7, apartment = $8, is_default = $9, updated_at = now()
			WHERE id = $10 AND user_id = $11;
		`
		tag, err := tx.Exec(txCtx, query,
			addr.Title, addr.Recipient, addr.Phone, addr.CityID, addr.Address,
			addr.Building, addr.Floor, addr.Apartment, addr.IsDefault, addr.ID, addr.UserID,
		)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return apperr.NotFound("user_address")
		}
		return writeAddressHistory(txCtx, tx, addr.UserID, addr.ID, "updated", addr)
	})
}

// DeleteAddress removes an address.
func (r *Repository) DeleteAddress(ctx context.Context, id, userID int64) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(txCtx, `DELETE FROM identity.user_addresses WHERE id = $1 AND user_id = $2;`, id, userID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return apperr.NotFound("user_address")
		}
		return writeAddressHistory(txCtx, tx, userID, id, "deleted", map[string]any{"address_id": id})
	})
}

// AddFavorite adds a product to a user's favorites.
func (r *Repository) AddFavorite(ctx context.Context, userID, productID int64) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			INSERT INTO identity.user_favorites (user_id, product_id, created_at)
			VALUES ($1, $2, now())
			ON CONFLICT (user_id, product_id) DO NOTHING;
		`
		_, err := tx.Exec(txCtx, query, userID, productID)
		return err
	})
}

// RemoveFavorite removes a product from a user's favorites.
func (r *Repository) RemoveFavorite(ctx context.Context, userID, productID int64) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(txCtx, `DELETE FROM identity.user_favorites WHERE user_id = $1 AND product_id = $2;`, userID, productID)
		return err
	})
}

// ListFavorites returns all favorited product IDs for a user.
func (r *Repository) ListFavorites(ctx context.Context, userID int64) ([]int64, error) {
	var ids []int64
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `SELECT product_id FROM identity.user_favorites WHERE user_id = $1 ORDER BY created_at DESC;`
		rows, err := tx.Query(txCtx, query, userID)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var pid int64
			if err := rows.Scan(&pid); err != nil {
				return err
			}
			ids = append(ids, pid)
		}
		return rows.Err()
	})
	return ids, err
}

// ListAddressHistory returns the append-only change trail for a user's addresses.
func (r *Repository) ListAddressHistory(ctx context.Context, userID int64, limit int) ([]*identity.UserAddressHistory, error) {
	var list []*identity.UserAddressHistory
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		const query = `SELECT id, user_id, address_id, event, snapshot, changed_at FROM identity.user_address_histories WHERE user_id = $1 ORDER BY changed_at DESC LIMIT $2;`
		if limit <= 0 || limit > 100 {
			limit = 20
		}
		rows, err := tx.Query(txCtx, query, userID, limit)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var h identity.UserAddressHistory
			if err := rows.Scan(&h.ID, &h.UserID, &h.AddressID, &h.Event, &h.Snapshot, &h.ChangedAt); err != nil {
				return err
			}
			list = append(list, &h)
		}
		return rows.Err()
	})
	return list, err
}
