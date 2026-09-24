package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/moqasem061-dot/dawa24/internal/modules/commerce"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
)

// ListVendorNegotiationOrdersWithTotal retrieves orders with price negotiation for a vendor organization.
func (r *Repository) ListVendorNegotiationOrdersWithTotal(
	ctx context.Context,
	vendorOrgID int64,
	status string,
	limit, offset int,
) ([]*commerce.Order, int, error) {
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	if offset < 0 {
		offset = 0
	}

	var (
		orders []*commerce.Order
		total  int
	)
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		const countSQL = `
			SELECT count(DISTINCT o.id)
			FROM commerce.orders o
			JOIN commerce.order_shipments s ON s.order_id = o.id
			WHERE s.organization_id = $1
			  AND o.is_negotiation = true
			  AND o.deleted_at IS NULL
			  AND ($2::text = '' OR $2 = 'all' OR o.negotiation_status = $2);
		`
		if err := tx.QueryRow(txCtx, countSQL, vendorOrgID, status).Scan(&total); err != nil {
			return err
		}

		query := `
			SELECT DISTINCT o.id, o.public_id, o.order_number, o.customer_id, o.organization_id,
				o.offer_id, o.branch_id, o.vendor_branch_id, o.user_address_id, o.status,
				o.subtotal, o.discount_amount, o.total_discount, o.shipping_fee, o.tax_amount,
				o.total_amount, o.final_price, o.payment_method, o.payment_status, o.notes,
				o.is_negotiation, o.negotiation_status, COALESCE(o.negotiation_notes, '') AS negotiation_notes,
				o.rating, o.review, o.rated_at, o.delivered_at,
				o.created_at, o.updated_at, o.deleted_at
			FROM commerce.orders o
			JOIN commerce.order_shipments s ON s.order_id = o.id
			WHERE s.organization_id = $1
			  AND o.is_negotiation = true
			  AND o.deleted_at IS NULL
			  AND ($2::text = '' OR $2 = 'all' OR o.negotiation_status = $2)
			ORDER BY o.created_at DESC, o.id DESC
			LIMIT $3 OFFSET $4;
		`
		rows, err := tx.Query(txCtx, query, vendorOrgID, status, limit, offset)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			o, err := scanOrder(rows)
			if err != nil {
				return err
			}
			orders = append(orders, o)
		}
		if err := rows.Err(); err != nil {
			return err
		}

		for _, o := range orders {
			if err := hydrateOrderDetails(txCtx, tx, o); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, 0, fmt.Errorf("commerce postgres: list vendor negotiation orders: %w", err)
	}
	return orders, total, nil
}

// CountOrderLines counts the lines of many orders in one statement.
func (r *Repository) CountOrderLines(ctx context.Context, orderIDs []int64) (map[int64]int, error) {
	out := make(map[int64]int, len(orderIDs))
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(txCtx, `SELECT order_id, count(*) FROM commerce.order_lines WHERE order_id = ANY($1) GROUP BY order_id`, orderIDs)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var id int64
			var n int
			if err := rows.Scan(&id, &n); err != nil {
				return err
			}
			out[id] = n
		}
		return rows.Err()
	})
	return out, err
}
