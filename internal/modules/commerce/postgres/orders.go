package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// orderColumns is the canonical order projection (063: offer_id,
// branch_id, vendor_branch_id, user_address_id, total_discount, final_price).
// All order scans use it so the column order lives in exactly one place.
const orderColumns = `id, public_id, order_number, customer_id, organization_id,
	offer_id, branch_id, vendor_branch_id, user_address_id, status,
	subtotal, discount_amount, total_discount, shipping_fee, tax_amount,
	total_amount, final_price, payment_method, payment_status, notes,
	is_negotiation, negotiation_status, COALESCE(negotiation_notes, '') AS negotiation_notes,
	rating, review, rated_at, delivered_at,
	created_at, updated_at, deleted_at`

// scanOrder scans one order row (pgx.Row covers both QueryRow and Rows).
func scanOrder(row pgx.Row) (*commerce.Order, error) {
	var o commerce.Order
	var statusStr, payStatusStr string
	var offerID *int64
	if err := row.Scan(
		&o.ID, &o.PublicID, &o.OrderNumber, &o.CustomerID, &o.OrganizationID,
		&offerID, &o.BranchID, &o.VendorBranchID, &o.UserAddressID,
		&statusStr, &o.Subtotal, &o.DiscountAmount, &o.TotalDiscount, &o.ShippingFee,
		&o.TaxAmount, &o.TotalAmount, &o.FinalPrice,
		&o.PaymentMethod, &payStatusStr, &o.Notes,
		&o.IsNegotiation, &o.NegotiationStatus, &o.NegotiationNotes,
		&o.Rating, &o.Review, &o.RatedAt, &o.DeliveredAt,
		&o.CreatedAt, &o.UpdatedAt, &o.DeletedAt,
	); err != nil {
		return nil, err
	}
	if offerID != nil {
		o.OfferID = *offerID
	}
	o.Status = commerce.OrderStatus(statusStr)
	o.PaymentStatus = commerce.PaymentStatus(payStatusStr)
	return &o, nil
}

// CreateOrder writes the master order, its vendor shipment partitions, and line item snapshots atomically.
//
// PERF OPTIMIZATION NOTE (Priority 10):
// CreateOrder executes individual INSERTs per shipment and per line item within this transaction.
// For large orders (15+ items across vendors), this generates 45-75 SQL round-trips while holding
// the transaction open and quota advisory locks. Future refactor: batch line inserts using
// unnest($1::uuid[], ...) or pgx.Batch to collapse statements into a single network round-trip.
func (r *Repository) CreateOrder(
	ctx context.Context,
	order *commerce.Order,
	shipments []*commerce.OrderShipment,
	lines []*commerce.OrderLine,
) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		// 0. The per-branch quota, before anything is written.
		//
		// commerce.CheckAvailability has already refused an over-quota line at
		// the cart and again at checkout. Neither of those is atomic with this
		// insert, so two baskets can pass the gate at the same instant and both
		// arrive here for the last units. The rule is applied once more under a
		// lock held for this transaction; it is the only place where "the
		// branch may take this much" and "the lines are written" are one act.
		var quotaBranchID int64
		if order.BranchID != nil {
			quotaBranchID = *order.BranchID
		}
		if err := enforceOrderQuotas(txCtx, tx, quotaBranchID, orderLineDemands(lines), 0); err != nil {
			return err
		}

		// 1. Insert master order
		queryOrder := `
			INSERT INTO commerce.orders (
				order_number, customer_id, organization_id, offer_id, branch_id,
				vendor_branch_id, user_address_id, status, subtotal,
				discount_amount, total_discount, shipping_fee, tax_amount, total_amount,
				final_price, payment_method, payment_status, notes,
				is_negotiation, negotiation_status, negotiation_notes
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21)
			RETURNING id, public_id, created_at, updated_at;
		`
		var offerID *int64
		if order.OfferID > 0 {
			offerID = &order.OfferID
		}
		var branchID *int64
		if order.BranchID != nil && *order.BranchID > 0 {
			branchID = order.BranchID
		}
		var vendorBranchID *int64
		if order.VendorBranchID != nil && *order.VendorBranchID > 0 {
			vendorBranchID = order.VendorBranchID
		}
		var userAddressID *int64
		if order.UserAddressID != nil && *order.UserAddressID > 0 {
			userAddressID = order.UserAddressID
		}
		var orgID *int64
		if order.OrganizationID != nil && *order.OrganizationID > 0 {
			orgID = order.OrganizationID
		}

		negStatus := string(order.NegotiationStatus)
		if negStatus == "" {
			negStatus = "none"
		}

		err := tx.QueryRow(txCtx, queryOrder,
			order.OrderNumber, order.CustomerID, orgID, offerID,
			branchID, vendorBranchID, userAddressID, string(order.Status),
			order.Subtotal, order.DiscountAmount, order.TotalDiscount, order.ShippingFee,
			order.TaxAmount, order.TotalAmount, order.FinalPrice,
			order.PaymentMethod, string(order.PaymentStatus), order.Notes,
			order.IsNegotiation, negStatus, order.NegotiationNotes,
		).Scan(&order.ID, &order.PublicID, &order.CreatedAt, &order.UpdatedAt)
		if err != nil {
			return fmt.Errorf("commerce postgres: insert order: %w", err)
		}

		// 2. Insert shipments map
		shipmentIDMap := make(map[int64]int64) // key: vendorOrgID, val: shipment.ID
		for seq, s := range shipments {
			s.OrderID = order.ID
			s.ShipmentNumber = commerce.GenerateShipmentNumber(order.OrderNumber, seq+1)
			s.Status = order.Status
			if s.DeliveryCode == "" {
				s.DeliveryCode = commerce.GenerateDeliveryCode()
			}
			if s.TrackingNumber == "" {
				s.TrackingNumber = commerce.GenerateTrackingNumber(order.OrderNumber, seq+1)
			}

			var sBranchID *int64
			if s.BranchID != nil && *s.BranchID > 0 {
				sBranchID = s.BranchID
			}

			queryShipment := `
				INSERT INTO commerce.order_shipments (
					order_id, organization_id, branch_id, shipment_number,
					status, subtotal, shipping_fee, total_amount, tracking_number, delivery_code
				) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
				RETURNING id, public_id, created_at, updated_at;
			`
			err := tx.QueryRow(txCtx, queryShipment,
				s.OrderID, s.OrganizationID, sBranchID, s.ShipmentNumber,
				string(s.Status), s.Subtotal, s.ShippingFee, s.TotalAmount,
				s.TrackingNumber, s.DeliveryCode,
			).Scan(&s.ID, &s.PublicID, &s.CreatedAt, &s.UpdatedAt)
			if err != nil {
				return fmt.Errorf("commerce postgres: insert shipment: %w", err)
			}
			shipmentIDMap[s.OrganizationID] = s.ID
		}

		// 3. Insert line snapshots
		for _, line := range lines {
			line.OrderID = order.ID
			if line.ShipmentID == 0 {
				line.ShipmentID = shipmentIDMap[line.OrganizationID]
			}

			var pID *int64
			if line.ProductID != nil && *line.ProductID > 0 {
				pID = line.ProductID
			}
			var pvID *int64
			if line.ProductVariantID != nil && *line.ProductVariantID > 0 {
				pvID = line.ProductVariantID
			}
			var opID *int64

			pName := line.ProductName
			vName := line.VariantName
			sku := line.SKU

			if pName.IsEmpty() && pvID != nil && *pvID > 0 {
				var fetchedPName, fetchedVName, fetchedSKU string
				var fetchedPID int64
				var fetchedCostPrice *money.Amount
				var fetchedCostDiscount float64
				err := tx.QueryRow(txCtx, `
					SELECT COALESCE(NULLIF(p.name->>'ar', ''), NULLIF(p.name->>'en', ''), pv.sku, ''),
					       COALESCE(NULLIF(pv.name->>'ar', ''), NULLIF(pv.name->>'en', ''), ''),
					       COALESCE(pv.sku, ''),
					       p.id,
					       pv.cost_price,
					       COALESCE(pv.cost_discount_percentage, 0.00)
					FROM catalog.product_variants pv
					JOIN catalog.products p ON p.id = pv.product_id
					WHERE pv.id = $1`, *pvID).Scan(&fetchedPName, &fetchedVName, &fetchedSKU, &fetchedPID, &fetchedCostPrice, &fetchedCostDiscount)
				if err == nil {
					if pName.IsEmpty() && fetchedPName != "" {
						pName = i18n.New(fetchedPName, fetchedPName)
					}
					if vName.IsEmpty() && fetchedVName != "" {
						vName = i18n.New(fetchedVName, fetchedVName)
					}
					if sku == "" {
						sku = fetchedSKU
					}
					if pID == nil && fetchedPID > 0 {
						pID = &fetchedPID
					}
					if line.CostPrice == nil && fetchedCostPrice != nil && fetchedCostPrice.IsPositive() {
						line.CostPrice = fetchedCostPrice
						line.CostDiscountPercentage = fetchedCostDiscount
					}
				}
			}

			if pName.IsEmpty() && pID != nil && *pID > 0 {
				var fetchedPName, fetchedSKU string
				err := tx.QueryRow(txCtx, `
					SELECT COALESCE(NULLIF(name->>'ar', ''), NULLIF(name->>'en', ''), sku, ''),
					       COALESCE(sku, '')
					FROM catalog.products
					WHERE id = $1`, *pID).Scan(&fetchedPName, &fetchedSKU)
				if err == nil {
					if pName.IsEmpty() && fetchedPName != "" {
						pName = i18n.New(fetchedPName, fetchedPName)
					}
					if sku == "" {
						sku = fetchedSKU
					}
				}
			}

			if pName.IsEmpty() {
				pName = i18n.New(i18n.TDefault("w4_mod.24_150"), "Dawa24 Product")
			}

			// Validate or resolve offer_product_id against promo.offer_products to prevent FK violations (SQLSTATE 23503)
			if line.OfferProductID != nil && *line.OfferProductID > 0 {
				var foundID int64
				err := tx.QueryRow(txCtx, `
					SELECT id FROM promo.offer_products
					WHERE id = $1
					LIMIT 1
				`, *line.OfferProductID).Scan(&foundID)
				if err == nil && foundID > 0 {
					opID = &foundID
				} else {
					err = tx.QueryRow(txCtx, `
						SELECT id FROM promo.offer_products
						WHERE offer_id = $1 AND ($2::bigint IS NULL OR product_variant_id = $2 OR product_id = $3)
						LIMIT 1
					`, *line.OfferProductID, pvID, pID).Scan(&foundID)
					if err == nil && foundID > 0 {
						opID = &foundID
					}
				}
			} else if order.OfferID > 0 {
				var foundID int64
				err := tx.QueryRow(txCtx, `
					SELECT id FROM promo.offer_products
					WHERE offer_id = $1 AND ($2::bigint IS NULL OR product_variant_id = $2 OR product_id = $3)
					LIMIT 1
				`, order.OfferID, pvID, pID).Scan(&foundID)
				if err == nil && foundID > 0 {
					opID = &foundID
				}
			}

			queryLine := `
				INSERT INTO commerce.order_lines (
					order_id, shipment_id, organization_id, product_id,
					product_variant_id, product_name, variant_name, sku,
					offer_product_id, unit_price, quantity, discount_amount,
					total_price, cost_price, cost_discount_percentage,
					list_price, original_price, original_discount,
					is_negotiated, proposed_unit_price
				) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20)
				RETURNING id, created_at;
			`
			err := tx.QueryRow(txCtx, queryLine,
				line.OrderID, line.ShipmentID, line.OrganizationID, pID,
				pvID, pName, vName, sku,
				opID, line.UnitPrice, line.Quantity, line.DiscountAmount,
				line.TotalPrice, line.CostPrice, line.CostDiscountPercentage,
				line.ListPrice, line.OriginalPrice, line.OriginalDiscount,
				line.IsNegotiated, line.ProposedUnitPrice,
			).Scan(&line.ID, &line.CreatedAt)
			if err != nil {
				return fmt.Errorf("commerce postgres: insert order line: %w", err)
			}
		}

		// 4. Initial status history
		queryHistory := `
			INSERT INTO commerce.order_status_history (order_id, to_status, notes)
			VALUES ($1, $2, 'Order placed successfully');
		`
		_, err = tx.Exec(txCtx, queryHistory, order.ID, string(order.Status))
		return err
	})
}
