package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// The enriched read of one shipment: everything a person standing at a
// pharmacy door needs on one screen.
//
// It is declared once and reached two ways — by tracking reference and by id —
// because the two used to be the same forty-line SELECT written twice, and the
// courier columns would have made it three. The projection and the scan travel
// together so a column added to one cannot be forgotten in the other.

// shipmentDetailSelect is the projection and the joins, without a WHERE. The
// caller appends its own predicate and ordering.
const shipmentDetailSelect = `
	SELECT s.id, s.public_id, s.order_id, s.organization_id, s.branch_id, s.shipment_number,
	       s.status, s.subtotal, s.shipping_fee, s.total_amount, s.tracking_number,
	       s.carrier_name, s.delivery_code, s.delivery_attempts, s.delivery_locked_until,
	       s.delivery_notes, s.collected_amount_minor, s.delivered_by_courier_at,
	       s.courier_user_id, s.courier_assigned_at, s.courier_assigned_by,
	       s.shipped_at, s.delivered_at, s.created_at, s.updated_at,
	       COALESCE(ord.order_number, ''),
	       COALESCE(ord.payment_method, 'cod'),
	       COALESCE(ord.payment_status, 'unpaid'),
	       COALESCE(ord.notes, ''),
	       COALESCE(ord.is_negotiation, false),
	       COALESCE(ord.negotiation_status, 'none'),
	       COALESCE(ord.negotiation_notes, ''),
	       COALESCE(vendor_org.name, '{"ar":"مورد معتمد","en":"Approved Supplier"}'::jsonb) AS vendor_name,
	       COALESCE(cust_org.name, '{"ar":"صيدلية معتمدة","en":"Approved Pharmacy"}'::jsonb) AS customer_org_name,
	       COALESCE(b.name, '{"ar":"الفرع الرئيسي","en":"Main Branch"}'::jsonb) AS branch_name,
	       COALESCE(b.address, '') AS branch_address,
	       COALESCE(b.phone, '') AS branch_phone,
	       COALESCE(b.manager_name, '') AS manager_name,
	       COALESCE(b.latitude, city.latitude) AS branch_latitude,
	       COALESCE(b.longitude, city.longitude) AS branch_longitude,
	       COALESCE(b.google_maps_url, '') AS branch_google_maps_url,
	       COALESCE(NULLIF(courier.name->>'ar', ''), NULLIF(courier.name->>'en', ''), COALESCE(courier.email, '')) AS courier_name,
	       COALESCE(courier.phone, '') AS courier_phone
	FROM commerce.order_shipments s
	JOIN commerce.orders ord ON ord.id = s.order_id
	LEFT JOIN org.organizations vendor_org ON vendor_org.id = s.organization_id
	LEFT JOIN org.organizations cust_org ON cust_org.id = ord.organization_id
	LEFT JOIN identity.users courier ON courier.id = s.courier_user_id
	LEFT JOIN org.branches b ON b.id = COALESCE(
		ord.branch_id,
		(SELECT mb.id FROM org.branches mb WHERE mb.organization_id = ord.organization_id AND mb.is_main = true AND mb.deleted_at IS NULL LIMIT 1),
		(SELECT mb.id FROM org.branches mb WHERE mb.organization_id = ord.organization_id AND mb.deleted_at IS NULL ORDER BY mb.id ASC LIMIT 1)
	)
	LEFT JOIN platform_admin.cities city ON city.id = b.city_id`

// scanShipmentDetail reads one row of shipmentDetailSelect.
func scanShipmentDetail(row pgx.Row) (*commerce.OrderShipment, error) {
	var s commerce.OrderShipment
	var statusStr, payStatusStr string
	err := row.Scan(
		&s.ID, &s.PublicID, &s.OrderID, &s.OrganizationID, &s.BranchID, &s.ShipmentNumber,
		&statusStr, &s.Subtotal, &s.ShippingFee, &s.TotalAmount, &s.TrackingNumber,
		&s.CarrierName, &s.DeliveryCode, &s.DeliveryAttempts, &s.DeliveryLockedUntil,
		&s.DeliveryNotes, &s.CollectedAmountMinor, &s.DeliveredByCourierAt,
		&s.CourierUserID, &s.CourierAssignedAt, &s.CourierAssignedBy,
		&s.ShippedAt, &s.DeliveredAt, &s.CreatedAt, &s.UpdatedAt,
		&s.OrderNumber, &s.PaymentMethod, &payStatusStr, &s.Notes,
		&s.IsNegotiation, &s.NegotiationStatus, &s.NegotiationNotes,
		&s.VendorName, &s.CustomerOrgName, &s.CustomerBranchName, &s.CustomerBranchAddress,
		&s.CustomerBranchPhone, &s.CustomerManagerName,
		&s.CustomerBranchLatitude, &s.CustomerBranchLongitude, &s.CustomerBranchGoogleMapsURL,
		&s.CourierName, &s.CourierPhone,
	)
	if err != nil {
		return nil, err
	}
	s.Status = commerce.OrderStatus(statusStr)
	s.PaymentStatus = commerce.PaymentStatus(payStatusStr)
	return &s, nil
}

// shipmentDetail runs one enriched read and loads its lines. Callers supply
// their own predicate, so ownership is expressed in SQL rather than checked
// after the row has already been read.
func (r *Repository) shipmentDetail(ctx context.Context, query string, args ...any) (*commerce.OrderShipment, error) {
	var s *commerce.OrderShipment
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		row, err := scanShipmentDetail(tx.QueryRow(txCtx, query, args...))
		if err != nil {
			return err
		}
		s = row
		return nil
	})
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, apperr.NotFound("shipment")
		}
		return nil, fmt.Errorf("commerce postgres: get shipment detail: %w", err)
	}

	lines, err := r.shipmentLines(ctx, []int64{s.ID})
	if err != nil {
		return nil, err
	}
	s.Lines = lines[s.ID]
	return s, nil
}

// shipmentLines loads the order lines of one or more shipments, keyed by
// shipment id. One query for a page of parcels rather than one per parcel.
func (r *Repository) shipmentLines(ctx context.Context, shipmentIDs []int64) (map[int64][]*commerce.OrderLine, error) {
	out := make(map[int64][]*commerce.OrderLine, len(shipmentIDs))
	if len(shipmentIDs) == 0 {
		return out, nil
	}
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		const query = `
			SELECT id, order_id, shipment_id, organization_id, product_id, product_variant_id,
			       product_name, variant_name, sku, unit_price, quantity, discount_amount, total_price,
			       cost_price, COALESCE(cost_discount_percentage, 0.00),
			       COALESCE(list_price, 0), COALESCE(original_price, 0), COALESCE(original_discount, 0)
			FROM commerce.order_lines
			WHERE shipment_id = ANY($1)
			ORDER BY shipment_id ASC, id ASC;
		`
		rows, err := tx.Query(txCtx, query, shipmentIDs)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var l commerce.OrderLine
			if err := rows.Scan(
				&l.ID, &l.OrderID, &l.ShipmentID, &l.OrganizationID, &l.ProductID, &l.ProductVariantID,
				&l.ProductName, &l.VariantName, &l.SKU, &l.UnitPrice, &l.Quantity, &l.DiscountAmount, &l.TotalPrice,
				&l.CostPrice, &l.CostDiscountPercentage,
				&l.ListPrice, &l.OriginalPrice, &l.OriginalDiscount,
			); err != nil {
				return err
			}
			out[l.ShipmentID] = append(out[l.ShipmentID], &l)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("commerce postgres: get shipment lines: %w", err)
	}
	return out, nil
}
