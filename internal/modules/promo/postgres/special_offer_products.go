package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/promo"
)

// The bundle manifest of a special offer, read in one place.
//
// This query lived inline in three functions — GetSpecialOfferByID,
// ListSpecialOffersByOrg and AdminListSpecialOffers — and all three copies
// selected `prod.base_price`. catalog.products has no such column; it has
// `price`. Postgres answered 42703 every time, and all three call sites
// discarded the error (`pRows, _ := tx.Query(...)`), so every special offer in
// the platform loaded with an empty product list and nothing said why.
//
// One copy, and the error is returned.
const specialOfferProductsSQL = `
	SELECT p.id, p.offer_id,
	       COALESCE(p.product_id, pv.product_id, 0),
	       COALESCE(p.variant_id, pv.id, 0),
	       COALESCE(prod.name->>'ar', prod.name->>'en', pv.sku, ''),
	       COALESCE(pv.price, prod.price, 0),
	       COALESCE(p.custom_price, 0),
	       COALESCE(p.custom_discount_percentage, 0),
	       COALESCE(p.custom_discount_amount, 0),
	       COALESCE(NULLIF(p.custom_qty, 0), 1),
	       p.created_at
	FROM promo.offer_products p
	LEFT JOIN catalog.product_variants pv ON pv.id = p.variant_id
	LEFT JOIN catalog.products prod ON prod.id = COALESCE(p.product_id, pv.product_id)
	WHERE p.offer_id = $1
	ORDER BY p.id ASC;`

// loadSpecialOfferProducts reads one offer's bundle inside an open transaction.
func loadSpecialOfferProducts(
	ctx context.Context, tx pgx.Tx, offerID int64,
) ([]*promo.SpecialOfferProduct, error) {
	rows, err := tx.Query(ctx, specialOfferProductsSQL, offerID)
	if err != nil {
		return nil, fmt.Errorf("promo postgres: load offer products: %w", err)
	}
	defer rows.Close()

	var out []*promo.SpecialOfferProduct
	for rows.Next() {
		var p promo.SpecialOfferProduct
		if err := rows.Scan(
			&p.ID, &p.OfferID, &p.ProductID, &p.VariantID, &p.VariantName,
			&p.OriginalPrice, &p.CustomPrice, &p.DiscountPercentage,
			&p.DiscountAmount, &p.Quantity, &p.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("promo postgres: scan offer product: %w", err)
		}
		out = append(out, &p)
	}
	return out, rows.Err()
}

// insertSpecialOfferProducts writes a bundle, resolving each variant's product
// inside the statement.
//
// promo.offer_products.product_id is NOT NULL, and the resolution used to be a
// bare subquery: a variant id that named nothing produced NULL and aborted the
// whole transaction with 23502, losing the offer the vendor had just filled in.
// The join form skips an unknown variant instead, and the caller is told how
// many rows it actually wrote.
func insertSpecialOfferProducts(
	ctx context.Context, tx pgx.Tx, offerID, organizationID int64,
	products []*promo.SpecialOfferProduct,
) (int, error) {
	const stmt = `
		INSERT INTO promo.offer_products (
			offer_id, product_id, variant_id, custom_price,
			custom_discount_percentage, custom_discount_amount, custom_qty
		)
		SELECT $1, v.product_id, v.id, $3, $4, $5, $6
		FROM catalog.product_variants v
		WHERE v.id = $2
		  AND v.deleted_at IS NULL
		  AND v.organization_id = $7;`

	written := 0
	for _, p := range products {
		if p == nil || p.VariantID <= 0 {
			continue
		}
		qty := p.Quantity
		if qty <= 0 {
			qty = 1
		}
		tag, err := tx.Exec(ctx, stmt,
			offerID, p.VariantID, p.CustomPrice,
			p.DiscountPercentage, p.DiscountAmount, qty, organizationID,
		)
		if err != nil {
			return written, fmt.Errorf("promo postgres: insert offer product: %w", err)
		}
		written += int(tag.RowsAffected())
	}
	return written, nil
}
