package postgres

import (
	"context"
	"fmt"
	"strconv"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// PERF OPTIMIZATION NOTE (Priority 10):
// hydrateOrderDetails executes 3 queries per order (customer org/branch, shipments, order lines).
// When invoked in loops (e.g. ListVendorNegotiationOrdersWithTotal for 25 orders), this triggers
// an N+1 query pattern (1 + 25x3 = 76 queries).
// Future refactor: bulk-hydrate using WHERE order_id = ANY($1) in 3 batch queries for all orders,
// then map back to orders in Go memory.
func hydrateOrderDetails(txCtx context.Context, tx pgx.Tx, o *commerce.Order) error {
	if o == nil {
		return nil
	}

	// 1. Hydrate customer branch & organization info
	queryCustomer := `
		SELECT COALESCE(org.name, '{"ar":"","en":""}'::jsonb),
		       COALESCE(b.name, '{"ar":"","en":""}'::jsonb),
		       COALESCE(b.address, ''),
		       COALESCE(b.phone, ''),
		       COALESCE(b.manager_name, '')
		FROM commerce.orders ord
		LEFT JOIN org.organizations org ON org.id = ord.organization_id
		LEFT JOIN org.branches b ON b.id = ord.branch_id
		WHERE ord.id = $1;
	`
	_ = tx.QueryRow(txCtx, queryCustomer, o.ID).Scan(
		&o.CustomerOrgName,
		&o.CustomerBranchName,
		&o.CustomerBranchAddress,
		&o.CustomerBranchPhone,
		&o.CustomerManagerName,
	)

	// 2. Load shipments
	queryShipments := `
		SELECT s.id, s.public_id, s.order_id, s.organization_id, s.branch_id, s.shipment_number,
		       s.status, s.subtotal, s.shipping_fee, s.total_amount, s.tracking_number,
		       s.carrier_name, s.delivery_code, s.delivery_attempts, s.delivery_locked_until,
		       s.delivery_notes, s.collected_amount_minor, s.delivered_by_courier_at,
		       s.shipped_at, s.delivered_at, s.created_at, s.updated_at,
		       COALESCE(org.name, '{"ar":"مورد معتمد","en":"Approved Supplier"}'::jsonb) AS vendor_name
		FROM commerce.order_shipments s
		LEFT JOIN org.organizations org ON org.id = s.organization_id
		WHERE s.order_id = $1
		ORDER BY s.id ASC;
	`
	sRows, err := tx.Query(txCtx, queryShipments, o.ID)
	if err == nil {
		defer sRows.Close()
		shipmentMap := make(map[int64]*commerce.OrderShipment)
		for sRows.Next() {
			var s commerce.OrderShipment
			var statusStr string
			if err := sRows.Scan(
				&s.ID, &s.PublicID, &s.OrderID, &s.OrganizationID, &s.BranchID,
				&s.ShipmentNumber, &statusStr, &s.Subtotal, &s.ShippingFee,
				&s.TotalAmount, &s.TrackingNumber, &s.CarrierName,
				&s.DeliveryCode, &s.DeliveryAttempts, &s.DeliveryLockedUntil,
				&s.DeliveryNotes, &s.CollectedAmountMinor, &s.DeliveredByCourierAt,
				&s.ShippedAt, &s.DeliveredAt, &s.CreatedAt, &s.UpdatedAt,
				&s.VendorName,
			); err == nil {
				s.Status = commerce.OrderStatus(statusStr)
				s.OrderNumber = o.OrderNumber
				s.CustomerOrgName = o.CustomerOrgName
				s.CustomerBranchName = o.CustomerBranchName
				s.CustomerBranchAddress = o.CustomerBranchAddress
				s.CustomerBranchPhone = o.CustomerBranchPhone
				s.CustomerManagerName = o.CustomerManagerName
				s.PaymentMethod = o.PaymentMethod
				s.PaymentStatus = o.PaymentStatus
				s.Notes = o.Notes
				s.IsNegotiation = o.IsNegotiation
				s.NegotiationStatus = o.NegotiationStatus
				s.NegotiationNotes = o.NegotiationNotes

				o.Shipments = append(o.Shipments, &s)
				shipmentMap[s.ID] = &s
			}
		}

		// 3. Load order lines with real-time stock & constraint metadata
		var orgIDParam int64
		if o.OrganizationID != nil {
			orgIDParam = *o.OrganizationID
		}

		queryLines := `
			SELECT l.id, l.order_id, l.shipment_id, l.organization_id, l.product_id,
			       l.product_variant_id, l.product_name, l.variant_name, l.sku,
			       l.offer_product_id, l.unit_price, l.quantity, l.discount_amount,
			       l.total_price, l.cost_price, COALESCE(l.cost_discount_percentage, 0.00),
			       l.list_price, l.original_price, l.original_discount,
			       l.is_negotiated, COALESCE(l.proposed_unit_price, 0) as proposed_unit_price, l.rating,
			       COALESCE(stk.available_stock, 0) as available_stock,
			       COALESCE(pv.min_order_qty, 1) as min_order_qty,
			       COALESCE(op.max_qty_per_order, 0) as max_qty_per_order
			FROM commerce.order_lines l
			LEFT JOIN catalog.product_variants pv ON pv.id = l.product_variant_id AND pv.deleted_at IS NULL
			LEFT JOIN promo.offer_products op ON op.id = l.offer_product_id
			LEFT JOIN LATERAL (
			    SELECT COALESCE(SUM(s.quantity), 0)::int AS available_stock
			    FROM inventory.stocks s
			    WHERE s.product_variant_id = l.product_variant_id
			      AND (s.organization_id = l.organization_id OR ($2::bigint > 0 AND s.organization_id = $2))
			      AND s.deleted_at IS NULL
			) stk ON true
			WHERE l.order_id = $1
			ORDER BY l.id ASC;
		`
		lRows, err := tx.Query(txCtx, queryLines, o.ID, orgIDParam)
		if err == nil {
			defer lRows.Close()
			for lRows.Next() {
				var l commerce.OrderLine
				if err := lRows.Scan(
					&l.ID, &l.OrderID, &l.ShipmentID, &l.OrganizationID, &l.ProductID,
					&l.ProductVariantID, &l.ProductName, &l.VariantName, &l.SKU,
					&l.OfferProductID, &l.UnitPrice, &l.Quantity, &l.DiscountAmount,
					&l.TotalPrice, &l.CostPrice, &l.CostDiscountPercentage,
					&l.ListPrice, &l.OriginalPrice, &l.OriginalDiscount,
					&l.IsNegotiated, &l.ProposedUnitPrice, &l.Rating,
					&l.AvailableStock, &l.MinOrderQty, &l.MaxQtyPerOrder,
				); err == nil {
					o.Lines = append(o.Lines, &l)
					if sh, ok := shipmentMap[l.ShipmentID]; ok {
						sh.Lines = append(sh.Lines, &l)
					}
				}
			}
		}
	}
	return nil
}

// GetOrderByID retrieves order details.
func (r *Repository) GetOrderByID(ctx context.Context, id int64) (*commerce.Order, error) {
	var o *commerce.Order
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT ` + orderColumns + `
			FROM commerce.orders
			WHERE id = $1 AND deleted_at IS NULL;
		`
		var err error
		o, err = scanOrder(tx.QueryRow(txCtx, query, id))
		if err != nil {
			if database.IsNotFound(err) {
				return apperr.NotFound("order")
			}
			return err
		}
		return hydrateOrderDetails(txCtx, tx, o)
	})
	if err != nil {
		return nil, err
	}
	return o, nil
}

// GetOrderByNumber retrieves an order by its public business identifier.
func (r *Repository) GetOrderByNumber(ctx context.Context, number string) (*commerce.Order, error) {
	var o *commerce.Order
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT ` + orderColumns + `
			FROM commerce.orders
			WHERE order_number = $1 AND deleted_at IS NULL;
		`
		var err error
		o, err = scanOrder(tx.QueryRow(txCtx, query, number))
		if err != nil {
			if database.IsNotFound(err) {
				return apperr.NotFound("order")
			}
			return err
		}
		return hydrateOrderDetails(txCtx, tx, o)
	})
	if err != nil {
		return nil, err
	}
	return o, nil
}

// UpdateOrderStatus transitions order status and records an entry in the audit history.
func (r *Repository) UpdateOrderStatus(
	ctx context.Context,
	orderID int64,
	toStatus commerce.OrderStatus,
	history commerce.OrderStatusHistory,
) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		var currentStatus string
		err := tx.QueryRow(txCtx, `SELECT status FROM commerce.orders WHERE id = $1 FOR UPDATE;`, orderID).Scan(&currentStatus)
		if err != nil {
			if database.IsNotFound(err) {
				return apperr.NotFound("order")
			}
			return err
		}

		if !commerce.IsValidStatusTransition(commerce.OrderStatus(currentStatus), toStatus) {
			return apperr.Validation("order.invalid_transition", fmt.Sprintf("Cannot transition order from %s to %s", currentStatus, toStatus), nil)
		}

		queryUpdate := `UPDATE commerce.orders SET status = $2, updated_at = now() WHERE id = $1;`
		if _, err := tx.Exec(txCtx, queryUpdate, orderID, string(toStatus)); err != nil {
			return err
		}

		if toStatus == commerce.StatusCancelled {
			_, _ = tx.Exec(txCtx, `
				UPDATE commerce.order_shipments 
				SET status = 'cancelled', updated_at = now() 
				WHERE order_id = $1 AND status NOT IN ('delivered', 'completed', 'shipped', 'in_transit', 'out_for_delivery');
			`, orderID)
		}

		queryHistory := `
			INSERT INTO commerce.order_status_history (
				order_id, shipment_id, from_status, to_status, notes, changed_by_user_id
			) VALUES ($1, $2, $3, $4, $5, $6);
		`
		_, err = tx.Exec(txCtx, queryHistory,
			orderID, history.ShipmentID, currentStatus, string(toStatus), history.Notes, history.ChangedByUserID,
		)
		if err != nil {
			return err
		}

		auditAction := "commerce.order.status_change"
		if toStatus == commerce.StatusRefunded {
			auditAction = "commerce.refund"
		}
		var actorID int64
		if history.ChangedByUserID != nil {
			actorID = *history.ChangedByUserID
		}
		_ = database.WriteAudit(txCtx, tx, database.AuditEntry{
			ActorUserID: actorID,
			Action:      auditAction,
			EntityType:  "order",
			EntityID:    strconv.FormatInt(orderID, 10),
			Before:      map[string]any{"status": currentStatus},
			After:       map[string]any{"status": string(toStatus), "notes": history.Notes},
		})

		return nil
	})
}

// UpdateOrderPaymentStatus modifies the payment status of an order.
func (r *Repository) UpdateOrderPaymentStatus(ctx context.Context, orderID int64, paymentStatus commerce.PaymentStatus) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `UPDATE commerce.orders SET payment_status = $2, updated_at = now() WHERE id = $1;`
		res, err := tx.Exec(txCtx, query, orderID, string(paymentStatus))
		if err != nil {
			return fmt.Errorf("commerce postgres: update payment status: %w", err)
		}
		if res.RowsAffected() == 0 {
			return apperr.NotFound("order")
		}
		return nil
	})
}

// ListOrdersByCustomer retrieves customer orders.
func (r *Repository) ListOrdersByCustomer(ctx context.Context, customerID int64, limit, offset int) ([]*commerce.Order, error) {
	orders, _, err := r.ListOrdersByCustomerWithTotal(ctx, customerID, limit, offset)
	return orders, err
}

// ListOrdersByCustomerWithTotal retrieves paginated customer orders along with the total count.
func (r *Repository) ListOrdersByCustomerWithTotal(ctx context.Context, customerID int64, limit, offset int) ([]*commerce.Order, int, error) {
	var orders []*commerce.Order
	var total int
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		countSQL := `
			SELECT count(*)
			FROM commerce.orders
			WHERE customer_id = $1 AND deleted_at IS NULL;
		`
		if err := tx.QueryRow(txCtx, countSQL, customerID).Scan(&total); err != nil {
			return err
		}

		if limit <= 0 || limit > 100 {
			limit = 25
		}
		if offset < 0 {
			offset = 0
		}

		query := `
			SELECT ` + orderColumns + `
			FROM commerce.orders
			WHERE customer_id = $1 AND deleted_at IS NULL
			ORDER BY created_at DESC, id DESC
			LIMIT $2 OFFSET $3;
		`
		rows, err := tx.Query(txCtx, query, customerID, limit, offset)
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
		return rows.Err()
	})
	return orders, total, err
}

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
