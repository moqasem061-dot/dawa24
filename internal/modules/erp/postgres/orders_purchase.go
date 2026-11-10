package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/moqasem061-dot/dawa24/internal/modules/erp"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
)

// purchaseHeaderInner reads the buyer's own order headers under the buyer's
// tenant context: commerce.orders is keyed to the buyer, so RLS shows exactly
// these rows. The import and the idempotency record are joined by lateral so a
// second row can never duplicate an order. $2 is the integration filter (0 for
// any integration of the buyer organization).
const purchaseHeaderInner = `
SELECT o.id, o.order_number, o.status,
       CASE WHEN i.acked_status IS NULL AND o.status IN ('cancelled','rejected','failed','returned','refunded') THEN 'closed' WHEN i.acked_status IS NULL THEN 'new' WHEN i.acked_status = o.status THEN 'imported' ELSE 'changed' END AS import_state,
       COALESCE(r.external_reference, ''), COALESCE(i.erp_reference, ''), i.acked_at,
       o.created_at, o.updated_at, o.total_amount,
       COALESCE(buyer.name, '{}'::jsonb),
       COALESCE(NULLIF(br.phone, ''), buyer.phone, ''),
       COALESCE(NULLIF(br.address, ''), buyer.address, ''),
       COALESCE(NULLIF(ci.name->>'en', ''), ci.name->>'ar', '')
FROM commerce.orders o
LEFT JOIN org.organizations buyer ON buyer.id = o.organization_id
LEFT JOIN org.branches br ON br.id = o.branch_id AND br.organization_id = o.organization_id
LEFT JOIN platform_admin.cities ci ON ci.id = br.city_id
LEFT JOIN LATERAL (
    SELECT x.erp_reference, x.acked_status, x.acked_at
    FROM erp.order_imports x
    WHERE x.organization_id = o.organization_id AND x.direction = 'purchase' AND x.order_id = o.id
      AND ($2 = 0 OR x.integration_id = $2)
    ORDER BY x.acked_at DESC LIMIT 1
) i ON true
LEFT JOIN LATERAL (
    SELECT y.external_reference
    FROM erp.idempotency_records y
    WHERE y.organization_id = o.organization_id AND y.scope = 'purchase_order' AND y.order_id = o.id AND y.state = 'done'
    ORDER BY y.id DESC LIMIT 1
) r ON true
WHERE o.organization_id = $1 AND o.deleted_at IS NULL`

// purchaseListSQL pages the buyer's orders by id: $1 buyer, $2 integration,
// $3 after, $4 state, $5 from, $6 to, $7 limit.
const purchaseListSQL = `
SELECT * FROM (` + purchaseHeaderInner + `
  AND o.id > $3
  AND ($5::timestamptz IS NULL OR o.created_at >= $5)
  AND ($6::timestamptz IS NULL OR o.created_at < $6)) q
WHERE ($4 = 'all' OR q.import_state = $4)
ORDER BY q.id
LIMIT $7`

// purchaseGetSQL reads one order by number: $1 buyer, $2 integration (0), $3 number.
const purchaseGetSQL = `
SELECT * FROM (` + purchaseHeaderInner + `
  AND o.order_number = $3) q
ORDER BY q.id
LIMIT 1`

func scanPurchaseHeader(row scanner) (erp.PurchaseRow, error) {
	var r erp.PurchaseRow
	var buyerName []byte
	err := row.Scan(&r.OrderID, &r.OrderNumber, &r.Status, &r.ImportState,
		&r.ExternalReference, &r.ErpReference, &r.AckedAt,
		&r.CreatedAt, &r.UpdatedAt, &r.Total,
		&buyerName, &r.Ordering.Phone, &r.Ordering.Address, &r.Ordering.City)
	if errors.Is(err, pgx.ErrNoRows) {
		return r, erp.ErrNotFound
	}
	r.BuyerName = decodeBilingual(buyerName)
	return r, err
}

// ListPurchaseOrders pages the buyer's order headers. Shipments are not read
// here: the service joins them through ListPurchaseShipmentsAsSystem (E7).
func (r *Repository) ListPurchaseOrders(ctx context.Context, buyerOrgID, integrationID int64, f erp.OrderFilter) ([]erp.PurchaseRow, error) {
	var out []erp.PurchaseRow
	err := r.db.InReadTx(database.WithTenant(ctx, buyerOrgID), func(ctx context.Context, tx pgx.Tx) error {
		cursor, err := tx.Query(ctx, purchaseListSQL, buyerOrgID, integrationID, f.After, f.State, f.From, f.To, f.Limit)
		if err != nil {
			return err
		}
		defer cursor.Close()
		for cursor.Next() {
			row, err := scanPurchaseHeader(cursor)
			if err != nil {
				return err
			}
			out = append(out, row)
		}
		return cursor.Err()
	})
	return out, err
}

// GetPurchaseOrder reads one buyer order by number under the buyer's tenant context.
func (r *Repository) GetPurchaseOrder(ctx context.Context, buyerOrgID int64, orderNumber string) (erp.PurchaseRow, error) {
	var out erp.PurchaseRow
	err := r.db.InReadTx(database.WithTenant(ctx, buyerOrgID), func(ctx context.Context, tx pgx.Tx) error {
		var err error
		out, err = scanPurchaseHeader(tx.QueryRow(ctx, purchaseGetSQL, buyerOrgID, 0, orderNumber))
		return err
	})
	return out, err
}

const purchaseShipmentsSQL = `
SELECT s.id, s.order_id, s.shipment_number, s.status, COALESCE(sup.name, '{}'::jsonb), s.subtotal, s.shipping_fee
FROM commerce.order_shipments s
JOIN commerce.orders o ON o.id = s.order_id
LEFT JOIN org.organizations sup ON sup.id = s.organization_id
WHERE o.organization_id = $1 AND s.order_id = ANY($2) AND o.deleted_at IS NULL
ORDER BY s.id`

// purchaseLinesSQL reads the lines of those shipments. Lines carry the variant's
// numeric id and barcode. The external code is the buyer's own item code for the
// variant (erp.item_codes), pinned to the buyer organization: this statement runs
// as system (E7), so the join must not rely on RLS. It is empty when none is stored.
const purchaseLinesSQL = `
SELECT ol.shipment_id, ol.id, COALESCE(v.id::text, ''), COALESCE(v.barcode, ''),
       ol.product_name, ol.quantity, ol.unit_price, ol.discount_amount, ol.total_price,
       COALESCE(c.external_code, ''), COALESCE(ol.list_price, 0)
FROM commerce.order_lines ol
JOIN commerce.order_shipments s ON s.id = ol.shipment_id
JOIN commerce.orders o ON o.id = s.order_id
LEFT JOIN catalog.product_variants v ON v.id = ol.product_variant_id
LEFT JOIN erp.item_codes c ON c.organization_id = $1 AND c.variant_id = ol.product_variant_id
WHERE o.organization_id = $1 AND s.order_id = ANY($2) AND o.deleted_at IS NULL
ORDER BY ol.id`

// ListPurchaseShipmentsAsSystem reads the supplier shipments and their lines of
// the buyer's orders (plan.md E7). Both statements join the order and pin it to
// the buyer organization, so only this buyer's orders are returned.
func (r *Repository) ListPurchaseShipmentsAsSystem(ctx context.Context, buyerOrgID int64, orderIDs []int64) ([]erp.PurchaseShipment, error) {
	if len(orderIDs) == 0 {
		return nil, nil
	}
	var out []erp.PurchaseShipment
	err := r.db.InReadTx(database.AsSystem(ctx), func(ctx context.Context, tx pgx.Tx) error {
		cursor, err := tx.Query(ctx, purchaseShipmentsSQL, buyerOrgID, orderIDs)
		if err != nil {
			return err
		}
		index := map[int64]int{}
		for cursor.Next() {
			var id int64
			var supplier []byte
			var sh erp.PurchaseShipment
			if err := cursor.Scan(&id, &sh.OrderID, &sh.ShipmentNumber, &sh.Status, &supplier, &sh.Subtotal, &sh.ShippingFee); err != nil {
				cursor.Close()
				return err
			}
			sh.SupplierName = decodeBilingual(supplier)
			index[id] = len(out)
			out = append(out, sh)
		}
		cursor.Close()
		if err := cursor.Err(); err != nil {
			return err
		}
		lines, err := tx.Query(ctx, purchaseLinesSQL, buyerOrgID, orderIDs)
		if err != nil {
			return err
		}
		defer lines.Close()
		for lines.Next() {
			var shipmentID int64
			var name []byte
			var line erp.OrderLine
			if err := lines.Scan(&shipmentID, &line.LineID, &line.VariantPublicID, &line.Barcode,
				&name, &line.Quantity, &line.UnitPrice, &line.DiscountAmount, &line.TotalPrice,
				&line.ExternalProductCode, &line.ListPrice); err != nil {
				return err
			}
			line.Name = decodeBilingual(name)
			if at, ok := index[shipmentID]; ok {
				out[at].Lines = append(out[at].Lines, line)
			}
		}
		return lines.Err()
	})
	return out, err
}
