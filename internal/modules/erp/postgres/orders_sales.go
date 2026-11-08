package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/moqasem061-dot/dawa24/internal/modules/erp"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
)

// salesHeaderInner is the E2 statement (plan.md E2). commerce.orders is keyed to
// the buyer, so a supplier cannot read the header, the buyer's branch or the
// buyer organization under WithTenant; it reads them here as system, and every
// row is pinned to the supplier by s.organization_id = $1 (trap 2). The branch
// join also requires the branch to belong to the buyer organization. The
// import state is computed from the stored acknowledgement of this shipment.
const salesHeaderInner = `
SELECT s.id, s.order_id, o.order_number, s.shipment_number, s.status,
       CASE WHEN i.acked_status IS NULL THEN 'new' WHEN i.acked_status = s.status THEN 'imported' ELSE 'changed' END AS import_state,
       o.payment_method, o.payment_status, COALESCE(o.notes, ''),
       COALESCE(i.erp_reference, ''), i.acked_at,
       o.created_at, o.updated_at,
       s.subtotal, s.shipping_fee, s.total_amount,
       COALESCE(o.organization_id, 0), o.customer_id,
       COALESCE(co.name, '{}'::jsonb), COALESCE(b.name, '{}'::jsonb),
       COALESCE(CASE WHEN o.branch_id IS NULL THEN co.address ELSE b.address END, ''),
       COALESCE(CASE WHEN o.branch_id IS NULL THEN co.phone ELSE b.phone END, '')
FROM commerce.order_shipments s
JOIN commerce.orders o ON o.id = s.order_id
LEFT JOIN org.organizations co ON co.id = o.organization_id
LEFT JOIN org.branches b ON b.id = o.branch_id AND b.organization_id = o.organization_id
LEFT JOIN LATERAL (
    SELECT x.erp_reference, x.acked_status, x.acked_at
    FROM erp.order_imports x
    WHERE x.organization_id = s.organization_id AND x.direction = 'sales' AND x.shipment_id = s.id
    ORDER BY x.acked_at DESC LIMIT 1
) i ON true
WHERE s.organization_id = $1 AND o.deleted_at IS NULL`

// salesListSQL pages the supplier's shipments by id (trap 27). The state
// filter sits on the computed import_state column.
const salesListSQL = `
SELECT * FROM (` + salesHeaderInner + `
  AND s.id > $2
  AND ($5::timestamptz IS NULL OR o.created_at >= $5)
  AND ($6::timestamptz IS NULL OR o.created_at < $6)) q
WHERE ($3 = 'all' OR q.import_state = $3)
ORDER BY q.id
LIMIT $4`

const salesGetSQL = `
SELECT * FROM (` + salesHeaderInner + `
  AND o.order_number = $2) q
ORDER BY q.id
LIMIT 1`

// scanSalesHeader reads one row of the E2 statement, in its column order.
func scanSalesHeader(row scanner) (erp.SalesRow, orderRef, error) {
	var (
		r        erp.SalesRow
		ref      orderRef
		custName []byte
		branch   []byte
	)
	err := row.Scan(&r.ShipmentID, &r.OrderID, &r.OrderNumber, &r.ShipmentNumber, &r.Status,
		&r.ImportState, &r.PaymentMethod, &r.PaymentStatus, &r.Notes,
		&r.ErpReference, &r.AckedAt,
		&r.CreatedAt, &r.UpdatedAt,
		&r.Subtotal, &r.ShippingFee, &r.Total,
		&ref.CustomerOrgID, &ref.CustomerUserID,
		&custName, &branch,
		&r.Customer.Address, &r.Customer.Phone)
	if errors.Is(err, pgx.ErrNoRows) {
		return r, ref, erp.ErrNotFound
	}
	if err != nil {
		return r, ref, err
	}
	ref.OrderID = r.OrderID
	r.Customer.Name = decodeBilingual(custName)
	r.Customer.BranchName = decodeBilingual(branch)
	return r, ref, nil
}

// ListSalesShipmentsAsSystem pages the supplier's own shipments (plan.md E2).
// It returns at most f.Limit rows, ordered by shipment id; the lines come from a
// second statement under the supplier's tenant context.
func (r *Repository) ListSalesShipmentsAsSystem(ctx context.Context, orgID, integrationID int64, f erp.OrderFilter) ([]erp.SalesRow, error) {
	var rows []erp.SalesRow
	err := r.db.InReadTx(database.AsSystem(ctx), func(ctx context.Context, tx pgx.Tx) error {
		var refs []orderRef
		var err error
		rows, refs, err = querySalesHeaders(ctx, tx, salesListSQL, orgID, f.After, f.State, f.Limit, f.From, f.To)
		if err != nil {
			return err
		}
		return applyCustomerCodes(ctx, tx, orgID, rows, refs)
	})
	if err != nil {
		return nil, err
	}
	return r.attachSalesLines(ctx, orgID, integrationID, rows)
}

// GetSalesShipmentAsSystem reads one sales order of the supplier (plan.md E2).
func (r *Repository) GetSalesShipmentAsSystem(ctx context.Context, orgID int64, orderNumber string) (erp.SalesRow, error) {
	var out []erp.SalesRow
	err := r.db.InReadTx(database.AsSystem(ctx), func(ctx context.Context, tx pgx.Tx) error {
		rows, refs, err := querySalesHeaders(ctx, tx, salesGetSQL, orgID, orderNumber)
		if err != nil {
			return err
		}
		out = rows
		return applyCustomerCodes(ctx, tx, orgID, rows, refs)
	})
	if err != nil {
		return erp.SalesRow{}, err
	}
	if len(out) == 0 {
		return erp.SalesRow{}, erp.ErrNotFound
	}
	lines, err := r.attachSalesLines(ctx, orgID, 0, out)
	if err != nil {
		return erp.SalesRow{}, err
	}
	return lines[0], nil
}

// querySalesHeaders runs one header statement with its arguments in the order
// the statement numbers them.
func querySalesHeaders(ctx context.Context, tx pgx.Tx, sql string, args ...any) ([]erp.SalesRow, []orderRef, error) {
	cursor, err := tx.Query(ctx, sql, args...)
	if err != nil {
		return nil, nil, err
	}
	defer cursor.Close()
	var rows []erp.SalesRow
	var refs []orderRef
	for cursor.Next() {
		row, ref, err := scanSalesHeader(cursor)
		if err != nil {
			return nil, nil, err
		}
		rows = append(rows, row)
		refs = append(refs, ref)
	}
	return rows, refs, cursor.Err()
}

// applyCustomerCodes writes the resolved customer code onto each row. The
// lookup runs inside the caller's transaction.
func applyCustomerCodes(ctx context.Context, tx pgx.Tx, orgID int64, rows []erp.SalesRow, refs []orderRef) error {
	codes, err := resolveCustomerCodes(ctx, tx, orgID, refs)
	if err != nil {
		return err
	}
	for i := range rows {
		code := codes[rows[i].OrderID]
		rows[i].Customer.ErpCode = code.ErpCode
		rows[i].Customer.Mapped = code.Mapped
		rows[i].Customer.UnmappedReason = code.UnmappedReason
	}
	return nil
}

// attachSalesLines loads the lines of the given shipments under the supplier's
// tenant context, so RLS on order lines and product links does the filtering.
func (r *Repository) attachSalesLines(ctx context.Context, orgID, integrationID int64, rows []erp.SalesRow) ([]erp.SalesRow, error) {
	if len(rows) == 0 {
		return rows, nil
	}
	ids := make([]int64, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ShipmentID)
	}
	byShipment, err := r.loadSalesLines(ctx, orgID, integrationID, ids)
	if err != nil {
		return nil, err
	}
	for i := range rows {
		rows[i].Lines = byShipment[rows[i].ShipmentID]
	}
	return rows, nil
}

const salesLinesSQL = `
SELECT ol.shipment_id, ol.id, COALESCE(v.public_id::text, ''), COALESCE(pl.external_code, ''), COALESCE(v.barcode, ''),
       ol.product_name, ol.quantity, ol.unit_price, ol.discount_amount, ol.total_price
FROM commerce.order_lines ol
LEFT JOIN catalog.product_variants v ON v.id = ol.product_variant_id
LEFT JOIN LATERAL (
    SELECT l.external_code
    FROM erp.product_links l
    WHERE l.variant_id = ol.product_variant_id AND l.organization_id = $1 AND ($2 = 0 OR l.integration_id = $2)
    ORDER BY l.id DESC LIMIT 1
) pl ON true
WHERE ol.organization_id = $1 AND ol.shipment_id = ANY($3)
ORDER BY ol.id`

// loadSalesLines reads the supplier's own lines for the shipments. The
// integration filter is 0 for "any integration of this organization".
func (r *Repository) loadSalesLines(ctx context.Context, orgID, integrationID int64, shipmentIDs []int64) (map[int64][]erp.OrderLine, error) {
	out := map[int64][]erp.OrderLine{}
	err := r.db.InReadTx(database.WithTenant(ctx, orgID), func(ctx context.Context, tx pgx.Tx) error {
		cursor, err := tx.Query(ctx, salesLinesSQL, orgID, integrationID, shipmentIDs)
		if err != nil {
			return err
		}
		defer cursor.Close()
		for cursor.Next() {
			var shipmentID int64
			var name []byte
			var line erp.OrderLine
			if err := cursor.Scan(&shipmentID, &line.LineID, &line.VariantPublicID, &line.ExternalProductCode, &line.Barcode,
				&name, &line.Quantity, &line.UnitPrice, &line.DiscountAmount, &line.TotalPrice); err != nil {
				return err
			}
			line.Name = decodeBilingual(name)
			out[shipmentID] = append(out[shipmentID], line)
		}
		return cursor.Err()
	})
	return out, err
}
