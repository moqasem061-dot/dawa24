package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/moqasem061-dot/dawa24/internal/modules/commerce"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
	"github.com/moqasem061-dot/dawa24/internal/shared/apperr"
	"github.com/moqasem061-dot/dawa24/internal/shared/i18n"
	"github.com/moqasem061-dot/dawa24/internal/shared/money"
)

// processOrderLineEditsTx validates and updates line items for a pending order within a transaction.
func processOrderLineEditsTx(
	txCtx context.Context,
	tx pgx.Tx,
	order *commerce.Order,
	lines []commerce.OrderLineEditItem,
	defaultOrgID int64,
) error {
	for _, l := range lines {
		if l.IsDeleted {
			if l.ID > 0 {
				_, _ = tx.Exec(txCtx, `DELETE FROM commerce.order_lines WHERE id = $1 AND order_id = $2;`, l.ID, order.ID)
			}
			continue
		}

		if l.Quantity <= 0 {
			return apperr.Validation("quantity", i18n.TDefault("w4_mod.1_152"), nil)
		}

		if l.ID <= 0 {
			return apperr.Validation("order.line_not_editable",
				i18n.TDefault("customer.order.no_manual_lines"), nil)
		}

		var dbProductID, dbVariantID, dbOfferProductID *int64
		var dbUnitPrice, dbOldDiscount, dbOriginalDiscount, dbListPrice money.Amount
		var dbOldQty int
		var dbProductName string
		var dbOrgID int64

		err := tx.QueryRow(txCtx, `
			SELECT product_id, product_variant_id, offer_product_id, organization_id, unit_price, quantity, discount_amount, COALESCE(original_discount, 0),
			       COALESCE(product_name->>'ar', product_name->>'en', ''), COALESCE(list_price, 0)
			FROM commerce.order_lines
			WHERE id = $1 AND order_id = $2
			FOR UPDATE;
		`, l.ID, order.ID).Scan(&dbProductID, &dbVariantID, &dbOfferProductID, &dbOrgID, &dbUnitPrice, &dbOldQty, &dbOldDiscount, &dbOriginalDiscount, &dbProductName, &dbListPrice)
		if err != nil {
			if database.IsNotFound(err) {
				continue
			}
			return fmt.Errorf("fetch line %d: %w", l.ID, err)
		}

		if dbProductName == "" {
			dbProductName = l.ProductName
		}

		if dbOfferProductID != nil && *dbOfferProductID > 0 {
			var offerCustomQty, maxQtyPerOrder int
			_ = tx.QueryRow(txCtx, `
				SELECT COALESCE(custom_qty, 1), COALESCE(max_qty_per_order, 0)
				FROM promo.offer_products
				WHERE id = $1;
			`, *dbOfferProductID).Scan(&offerCustomQty, &maxQtyPerOrder)

			if maxQtyPerOrder > 0 && l.Quantity > maxQtyPerOrder {
				return apperr.Validation("max_qty_exceeded", fmt.Sprintf(i18n.TDefault("w4_mod.s_d_d_153"), dbProductName, l.Quantity, maxQtyPerOrder), nil)
			}
			if offerCustomQty > 1 && l.Quantity < offerCustomQty {
				return apperr.Validation("min_offer_qty", fmt.Sprintf(i18n.TDefault("w4_mod.s_d_154"), dbProductName, offerCustomQty), nil)
			}
		}

		if dbVariantID != nil && *dbVariantID > 0 {
			var minOrderQty int
			_ = tx.QueryRow(txCtx, `
				SELECT COALESCE(min_order_qty, 0)
				FROM catalog.product_variants
				WHERE id = $1 AND deleted_at IS NULL;
			`, *dbVariantID).Scan(&minOrderQty)
			if minOrderQty > 0 && l.Quantity < minOrderQty {
				return apperr.Validation("min_order_qty", fmt.Sprintf(i18n.TDefault("w4_mod.s_d_155"), dbProductName, minOrderQty), nil)
			}

			var availableStock int
			checkOrgID := dbOrgID
			if checkOrgID == 0 {
				checkOrgID = defaultOrgID
			}
			_ = tx.QueryRow(txCtx, `
				SELECT COALESCE(SUM(quantity), 0)
				FROM inventory.stocks
				WHERE product_variant_id = $1 
				  AND ($2::bigint = 0 OR organization_id = $2)
				  AND deleted_at IS NULL;
			`, *dbVariantID, checkOrgID).Scan(&availableStock)
			if availableStock > 0 && l.Quantity > availableStock {
				return apperr.Validation("stock_exceeded", fmt.Sprintf(i18n.TDefault("w4_mod.s_d_d_156"), dbProductName, l.Quantity, availableStock), nil)
			}
		}

		unitDiscount := money.Zero
		if dbOriginalDiscount.IsPositive() && dbListPrice.IsPositive() {
			unitDiscount = dbListPrice.ApplyPercent(dbOriginalDiscount.Minor())
		} else if dbOldQty > 0 && dbOldDiscount.IsPositive() {
			discMinor := dbOldDiscount.Minor() / int64(dbOldQty)
			unitDiscount = money.FromMinor(discMinor)
		} else if dbListPrice.IsPositive() && dbListPrice.Minor() > dbUnitPrice.Minor() {
			unitDiscount, _ = dbListPrice.Sub(dbUnitPrice)
		}

		if (unitDiscount.IsZero() || dbListPrice.IsZero() || dbListPrice.Minor() <= dbUnitPrice.Minor()) && dbVariantID != nil && *dbVariantID > 0 {
			var catPrice, catEffPrice money.Amount
			var catDiscPct float64
			if err := tx.QueryRow(txCtx, `
				SELECT price, COALESCE(effective_price, 0), COALESCE(discount_percentage, 0.00)
				FROM catalog.product_variants
				WHERE id = $1 AND deleted_at IS NULL;
			`, *dbVariantID).Scan(&catPrice, &catEffPrice, &catDiscPct); err == nil {
				if catPrice.IsPositive() && catPrice.Minor() > dbUnitPrice.Minor() {
					dbListPrice = catPrice
					unitDiscount, _ = catPrice.Sub(dbUnitPrice)
				} else if catDiscPct > 0 && dbListPrice.IsPositive() {
					unitDiscount = dbListPrice.ApplyPercent(int64(catDiscPct * 100))
				} else if catPrice.IsPositive() && catEffPrice.IsPositive() && catPrice.Minor() > catEffPrice.Minor() {
					dbListPrice = catPrice
					unitDiscount, _ = catPrice.Sub(catEffPrice)
				}
			}
		}

		if unitDiscount.IsZero() && dbOfferProductID != nil && *dbOfferProductID > 0 {
			var customDiscPct float64
			var customPrice money.Amount
			if err := tx.QueryRow(txCtx, `
				SELECT COALESCE(custom_price, 0), COALESCE(custom_discount_percentage, 0.00)
				FROM promo.offer_products
				WHERE id = $1;
			`, *dbOfferProductID).Scan(&customPrice, &customDiscPct); err == nil {
				if customDiscPct > 0 && dbListPrice.IsPositive() {
					unitDiscount = dbListPrice.ApplyPercent(int64(customDiscPct * 100))
				} else if customPrice.IsPositive() && dbListPrice.IsPositive() && dbListPrice.Minor() > customPrice.Minor() {
					unitDiscount, _ = dbListPrice.Sub(customPrice)
				}
			}
		}

		if unitDiscount.IsPositive() && (dbListPrice.IsZero() || dbListPrice.Minor() <= dbUnitPrice.Minor()) {
			dbListPrice, _ = dbUnitPrice.Add(unitDiscount)
		}
		if dbListPrice.IsZero() {
			dbListPrice = dbUnitPrice
		}

		lineDiscount, _ := unitDiscount.MulInt(int64(l.Quantity))
		lineTotal, _ := dbUnitPrice.MulInt(int64(l.Quantity))

		_, err = tx.Exec(txCtx, `
			UPDATE commerce.order_lines
			SET quantity = $1,
				unit_price = $2,
				discount_amount = $3,
				total_price = $4,
				list_price = $5
			WHERE id = $6 AND order_id = $7;
		`, l.Quantity, dbUnitPrice, lineDiscount, lineTotal, dbListPrice, l.ID, order.ID)
		if err != nil {
			return fmt.Errorf("update line %d: %w", l.ID, err)
		}
	}
	return nil
}
