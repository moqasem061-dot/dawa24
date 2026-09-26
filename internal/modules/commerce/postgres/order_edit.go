package postgres

import (
	"context"
	"fmt"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/moqasem061-dot/dawa24/internal/modules/commerce"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
	"github.com/moqasem061-dot/dawa24/internal/shared/apperr"
	"github.com/moqasem061-dot/dawa24/internal/shared/i18n"
	"github.com/moqasem061-dot/dawa24/internal/shared/money"
)

// UpdateCustomerPendingOrder modifies items, quantities and recalculates totals for a pending order.
func (r *Repository) UpdateCustomerPendingOrder(
	ctx context.Context,
	order *commerce.Order,
	lines []commerce.OrderLineEditItem,
	changedByUserID int64,
) (*commerce.Order, error) {
	err := r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		// 1. Lock the order row and verify status is still pending
		var currentStatus string
		var oldSubtotal, oldDiscount, shippingFee, oldTax money.Amount
		var orderBranchID *int64
		err := tx.QueryRow(txCtx, `
			SELECT status, subtotal, discount_amount, shipping_fee, tax_amount, branch_id
			FROM commerce.orders
			WHERE id = $1 FOR UPDATE;
		`, order.ID).Scan(&currentStatus, &oldSubtotal, &oldDiscount, &shippingFee, &oldTax, &orderBranchID)
		if err != nil {
			if database.IsNotFound(err) {
				return apperr.NotFound("order")
			}
			return err
		}

		if commerce.OrderStatus(currentStatus) != commerce.StatusPending {
			return apperr.Forbidden("order.locked", fmt.Sprintf(i18n.TDefault("w4_mod.s_151"), currentStatus))
		}

		// Calculate initial tax percentage rate if tax was present on the order
		// The order's tax rate is kept as the exact ratio oldTax/oldTaxable.
		oldTaxable, err := oldSubtotal.Sub(oldDiscount)
		if err != nil {
			return fmt.Errorf("order taxable amount: %w", err)
		}
		hasTaxRate := oldTax.IsPositive() && oldTaxable.IsPositive()

		// Get primary shipment ID and vendor organization ID if existing
		var defaultShipmentID, defaultOrgID int64
		_ = database.Soft(txCtx, tx, func() error {
			return tx.QueryRow(txCtx, `
			SELECT id, organization_id FROM commerce.order_shipments WHERE order_id = $1 ORDER BY id ASC LIMIT 1;
		`, order.ID).Scan(&defaultShipmentID, &defaultOrgID)
		})

		if defaultOrgID == 0 && order.OrganizationID != nil {
			defaultOrgID = *order.OrganizationID
		}

		for _, l := range lines {
			if !l.IsDeleted && l.ID <= 0 {
				return apperr.Validation("order.line_not_editable", i18n.TDefault("customer.order.no_manual_lines"), nil)
			}
		}

		// 2. Process line edits with stock and price validation
		if err := processOrderLineEditsTx(txCtx, tx, order, lines, defaultOrgID); err != nil {
			return err
		}

		// 3. Count remaining active lines
		var remainingCount int
		_ = database.Soft(txCtx, tx, func() error {
			return tx.QueryRow(txCtx, `SELECT COUNT(*) FROM commerce.order_lines WHERE order_id = $1;`, order.ID).Scan(&remainingCount)
		})
		if remainingCount == 0 {
			return apperr.Validation("order.empty", i18n.TDefault("w4_mod.w4str_157_157"), nil)
		}

		// 3b. The per-branch quota, re-measured against the edited quantities.
		editBranchID := int64(0)
		if orderBranchID != nil {
			editBranchID = *orderBranchID
		}
		editedDemands, err := orderQuotaDemandsTx(txCtx, tx, order.ID)
		if err != nil {
			return err
		}
		if err := enforceOrderQuotas(txCtx, tx, editBranchID, editedDemands, order.ID); err != nil {
			return err
		}

		// 4. Recalculate order totals from all active lines in DB
		newSubtotal := money.Zero
		newTotalDiscount := money.Zero
		rows, err := tx.Query(txCtx, `
			SELECT unit_price, quantity, discount_amount, COALESCE(list_price, 0), total_price
			FROM commerce.order_lines 
			WHERE order_id = $1;
		`, order.ID)
		if err != nil {
			return err
		}
		for rows.Next() {
			var up, da, lp, tp money.Amount
			var qty int
			if err := rows.Scan(&up, &qty, &da, &lp, &tp); err != nil {
				rows.Close()
				return fmt.Errorf("scan order line pricing: %w", err)
			}
			var lineGross money.Amount
			var err error
			if lp.IsPositive() && lp.Minor() > up.Minor() {
				lineGross, err = lp.MulInt(int64(qty))
			} else if da.IsPositive() {
				lineGross, err = tp.Add(da)
			} else {
				lineGross, err = up.MulInt(int64(qty))
			}
			if err == nil {
				newSubtotal, err = newSubtotal.Add(lineGross)
			}
			if err == nil {
				newTotalDiscount, err = newTotalDiscount.Add(da)
			}
			if err != nil {
				// Overflow: writing a truncated total would misprice the order.
				rows.Close()
				return fmt.Errorf("recompute order totals: %w", err)
			}
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return fmt.Errorf("iterate order lines: %w", err)
		}
		rows.Close()

		newTaxable, err := newSubtotal.Sub(newTotalDiscount)
		if err != nil {
			return fmt.Errorf("recompute taxable amount: %w", err)
		}
		if newTaxable.IsNegative() {
			newTaxable = money.Zero
		}

		newTaxAmount := money.Zero
		if hasTaxRate && newTaxable.IsPositive() {
			newTaxAmount, err = newTaxable.MulRatio(oldTax.Minor(), oldTaxable.Minor())
			if err != nil {
				return fmt.Errorf("recompute order tax: %w", err)
			}
		} else if oldTax.IsPositive() {
			newTaxAmount = oldTax
		}

		newTotalAmount := newTaxable
		newTotalAmount, _ = newTotalAmount.Add(shippingFee)
		newTotalAmount, _ = newTotalAmount.Add(newTaxAmount)

		// 5. Update master order row
		_, err = tx.Exec(txCtx, `
			UPDATE commerce.orders
			SET subtotal = $1,
				discount_amount = $2,
				total_discount = $2,
				tax_amount = $3,
				total_amount = $4,
				final_price = $4,
				updated_at = now()
			WHERE id = $5;
		`, newSubtotal, newTotalDiscount, newTaxAmount, newTotalAmount, order.ID)
		if err != nil {
			return err
		}

		// 6. Recalculate every shipment from its own lines.
		if _, err := tx.Exec(txCtx, `
			UPDATE commerce.order_shipments s
			SET subtotal     = COALESCE(t.subtotal, 0),
			    total_amount = COALESCE(t.net_amount, 0)
			                   + COALESCE(s.shipping_fee, 0),
			    updated_at   = now()
			FROM (
				SELECT sh.id,
				       COALESCE(SUM(CASE 
				           WHEN COALESCE(l.list_price, 0) > l.unit_price THEN l.list_price * l.quantity 
				           ELSE l.total_price + COALESCE(l.discount_amount, 0) 
				       END), 0) AS subtotal,
				       COALESCE(SUM(l.total_price), 0) AS net_amount,
				       COALESCE(SUM(l.discount_amount), 0) AS discount
				FROM commerce.order_shipments sh
				LEFT JOIN commerce.order_lines l ON l.shipment_id = sh.id
				WHERE sh.order_id = $1
				GROUP BY sh.id
			) t
			WHERE s.id = t.id AND s.order_id = $1;
		`, order.ID); err != nil {
			return fmt.Errorf("recalculate shipments: %w", err)
		}

		// 5. Record status history audit log
		historyQuery := `
			INSERT INTO commerce.order_status_history (
				order_id, shipment_id, from_status, to_status, notes, changed_by_user_id
			) VALUES ($1, $2, $3, $4, $5, $6);
		`
		var shipmentIDPtr *int64
		if defaultShipmentID > 0 {
			shipmentIDPtr = &defaultShipmentID
		}
		var userPtr *int64
		if changedByUserID > 0 {
			userPtr = &changedByUserID
		}
		_ = database.SoftExec(txCtx, tx, historyQuery, order.ID, shipmentIDPtr, currentStatus, currentStatus, i18n.TDefault("w4_mod.s_361_361"), userPtr)

		// 6. Audit logs for customer and vendor(s)
		orderIDStr := strconv.FormatInt(order.ID, 10)
		if order.OrganizationID != nil && *order.OrganizationID > 0 {
			_ = database.WriteAudit(txCtx, tx, database.AuditEntry{
				OrganizationID: order.OrganizationID,
				ActorUserID:    changedByUserID,
				Action:         "order.updated",
				EntityType:     "order",
				EntityID:       orderIDStr,
				Before: map[string]any{
					"subtotal": oldSubtotal.String(),
				},
				After: map[string]any{
					"edited_lines": len(lines),
					"notes":        i18n.TDefault("w4_mod.s_361_361"),
				},
			})
		}

		vRows, qErr := tx.Query(txCtx, `SELECT DISTINCT organization_id FROM commerce.order_shipments WHERE order_id = $1`, order.ID)
		if qErr == nil {
			defer vRows.Close()
			for vRows.Next() {
				var vOrgID int64
				if err := vRows.Scan(&vOrgID); err == nil && vOrgID > 0 {
					_ = database.WriteAudit(txCtx, tx, database.AuditEntry{
						OrganizationID: &vOrgID,
						ActorUserID:    changedByUserID,
						Action:         "order.updated",
						EntityType:     "order",
						EntityID:       orderIDStr,
						After: map[string]any{
							"edited_lines": len(lines),
							"notes":        i18n.TDefault("w4_mod.s_361_361"),
						},
					})
				}
			}
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return r.GetOrderByID(ctx, order.ID)
}
