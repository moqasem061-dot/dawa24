package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// GetOfferDetailsForOrderLine fetches the bundle/offer information and all included items
// for an order line that was purchased under a promotional offer (WO-16).
func (r *Repository) GetOfferDetailsForOrderLine(ctx context.Context, orderID, lineID int64) (*commerce.OrderLineOfferDetails, error) {
	var details commerce.OrderLineOfferDetails
	details.LineID = lineID

	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		var (
			offerProductID *int64
			nameAr, nameEn string
			unitPrice      money.Amount
			quantity       int
			discountAmount money.Amount
			totalPrice     money.Amount
			listPrice      money.Amount
			originalPrice  money.Amount
		)
		err := tx.QueryRow(txCtx, `
			SELECT offer_product_id, COALESCE(product_name->>'ar', ''), COALESCE(product_name->>'en', ''),
			       unit_price, quantity, discount_amount, total_price,
			       COALESCE(list_price, 0), COALESCE(original_price, 0)
			FROM commerce.order_lines
			WHERE id = $1 AND order_id = $2
		`, lineID, orderID).Scan(
			&offerProductID, &nameAr, &nameEn,
			&unitPrice, &quantity, &discountAmount, &totalPrice,
			&listPrice, &originalPrice,
		)
		if err != nil {
			if database.IsNotFound(err) {
				return apperr.NotFound("order_line")
			}
			return err
		}

		details.UnitPrice = unitPrice
		details.Quantity = quantity
		details.DiscountAmount = discountAmount
		details.TotalPrice = totalPrice

		if listPrice.IsPositive() {
			details.ListPrice = listPrice
		} else if originalPrice.IsPositive() {
			details.ListPrice = originalPrice
		} else if quantity > 0 {
			unitDisc := discountAmount.Minor() / int64(quantity)
			details.ListPrice = money.FromMinor(unitPrice.Minor() + unitDisc)
		} else {
			details.ListPrice = unitPrice
		}

		var offerID int64
		if offerProductID != nil && *offerProductID > 0 {
			offerID = *offerProductID
		} else {
			_ = tx.QueryRow(txCtx, `SELECT id FROM promo.offers WHERE (title->>'ar' = $1 OR title->>'en' = $2) LIMIT 1`, nameAr, nameEn).Scan(&offerID)
			if offerID <= 0 {
				_ = tx.QueryRow(txCtx, `SELECT COALESCE(offer_id, 0) FROM commerce.orders WHERE id = $1`, orderID).Scan(&offerID)
			}
		}

		var startsAt, expiresAt *time.Time
		if offerID > 0 {
			err = tx.QueryRow(txCtx, `
				SELECT o.id, o.title, o.description, o.discount_type, o.discount_value,
				       COALESCE(org.name->>'ar', org.name->>'en', ''),
				       o.starts_at, o.expires_at
				FROM promo.offers o
				LEFT JOIN org.organizations org ON org.id = o.organization_id
				WHERE (o.id = $1 OR o.id IN (SELECT offer_id FROM promo.offer_products WHERE id = $1)) LIMIT 1
			`, offerID).Scan(
				&details.OfferID, &details.Title, &details.Description,
				&details.DiscountType, &details.DiscountValue, &details.VendorName,
				&startsAt, &expiresAt,
			)
			if err != nil && !database.IsNotFound(err) {
				return err
			}
			details.StartsAt = startsAt
			details.ExpiresAt = expiresAt
		}

		// Fallback for Title if offer record had no title or wasn't found
		if details.Title.Get("ar") == "" && details.Title.Get("en") == "" {
			details.Title = i18n.Text{"ar": nameAr, "en": nameEn}
		}

		if details.OfferID > 0 {
			rows, err := tx.Query(txCtx, `
				SELECT COALESCE(op.product_id, pv.product_id, 0), op.variant_id, COALESCE(op.custom_qty, 1),
				       COALESCE(op.custom_price, 0), COALESCE(op.custom_discount_percentage, 0),
				       COALESCE(p.name, jsonb_build_object('ar', pv.name->>'ar', 'en', pv.name->>'en')),
				       COALESCE(pv.name->>'ar', pv.name->>'en', ''), COALESCE(pv.sku, '')
				FROM promo.offer_products op
				LEFT JOIN catalog.product_variants pv ON (pv.id = op.variant_id OR (op.variant_id IS NULL AND pv.product_id = op.product_id))
				LEFT JOIN catalog.products p ON p.id = COALESCE(op.product_id, pv.product_id)
				WHERE op.offer_id = $1 ORDER BY op.id ASC;
			`, details.OfferID)
			if err == nil {
				defer rows.Close()
				for rows.Next() {
					var it commerce.OrderLineOfferItem
					if err := rows.Scan(
						&it.ProductID, &it.VariantID, &it.Quantity,
						&it.CustomPrice, &it.CustomDiscountPercent,
						&it.ProductName, &it.VariantName, &it.SKU,
					); err == nil {
						details.Items = append(details.Items, it)
					}
				}
			}
		}

		return nil
	})
	if err != nil {
		return nil, err
	}
	return &details, nil
}
