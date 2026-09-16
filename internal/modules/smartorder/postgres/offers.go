package postgres

import (
	"context"
	"os"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/smartorder"
	"github.com/muhiya/dawa24-store/internal/platform/database"
)

// LoadOffers returns every vendor variant of the given products, in ONE query.
//
// Two decisions are load-bearing here.
//
// **Where the data comes from.** Stock is read from inventory.stocks and the
// offer from catalog.product_variants — never from catalog.product_index's
// stock_quantity column. That column was the literal 0 for all 28,786 rows in
// production, so a query filtering on it returned nothing, for every product,
// silently. The read model is excellent at *finding* a product by name; it is
// not the authority on whether anyone has it.
//
// **Why it is one query for the whole file.** FR-017a forbids per-row work. A
// ten-thousand-line import resolves to at most ten thousand product ids, which
// go in as a single array and come back as one result set. Looping this per line
// is the difference between three minutes and an hour.
//
// Cross-tenant read: a marketplace buyer must see other organisations' products,
// which is the explicit invariant of a multi-vendor catalogue. It runs under
// database.AsSystem for that reason, and the buyer's own organisation is
// excluded in the WHERE clause rather than left to the caller.
func (r *Repository) LoadOffers(ctx context.Context, buyerOrgID int64, productIDs []int64) ([]smartorder.Offer, error) {
	if len(productIDs) == 0 {
		return nil, nil
	}

	var out []smartorder.Offer
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(txCtx, `
			SELECT
				v.product_id,
				v.id                                    AS variant_id,
				v.organization_id                       AS vendor_org_id,
				COALESCE(v.branch_id, (SELECT b.id FROM org.branches b WHERE b.organization_id = v.organization_id AND b.deleted_at IS NULL ORDER BY b.is_main DESC, b.id ASC LIMIT 1), 0) AS branch_id,
				(v.price * 100)::bigint                 AS price_minor,
				(v.discount * 100)::bigint              AS discount_bps,
				COALESCE(v.unit, p.unit, '')            AS unit,
				GREATEST(COALESCE(v.min_order_qty, 1), 1) AS min_order_qty,
				COALESCE(st.qty, 0)                     AS stock_qty,
				EXISTS (
					SELECT 1
					FROM org.organization_followers f
					JOIN org.members m ON m.user_id = f.user_id
					WHERE f.organization_id = v.organization_id
					  AND m.organization_id = $2
				)                                       AS is_followed,
				(o.status = 'approved'
				 AND o.type IN ('vendor', 'supplier', 'company')
				 AND o.deleted_at IS NULL) AS vendor_active,
				(v.status = 'active'
				 AND p.status = 'active'
				 AND v.deleted_at IS NULL AND p.deleted_at IS NULL) AS product_active,
				COALESCE(p.institutional_work_ids, '{}'::bigint[]) AS institutional_work_ids,
				v.branch_id                             AS variant_branch_id
			FROM catalog.product_variants v
			JOIN catalog.products p      ON v.product_id = p.id
			JOIN org.organizations o     ON v.organization_id = o.id
			LEFT JOIN LATERAL (
				SELECT COALESCE(SUM(s.quantity), 0)::int AS qty
				FROM inventory.stocks s
				WHERE s.product_variant_id = v.id
			) st ON TRUE
			WHERE v.product_id = ANY($1::bigint[])
			  AND v.organization_id <> $2;`,
			productIDs, buyerOrgID)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var o smartorder.Offer
			if err := rows.Scan(
				&o.ProductID, &o.VariantID, &o.VendorOrgID, &o.BranchID,
				&o.PriceMinor, &o.DiscountBps, &o.Unit, &o.MinOrderQty,
				&o.StockQty, &o.IsFollowed, &o.VendorActive, &o.ProductActive,
				&o.InstitutionalWorkIDs, &o.VariantBranchID,
			); err != nil {
				return err
			}
			out = append(out, o)
		}
		return rows.Err()
	})
	return out, err
}

// matchableStatuses is which catalogue products the matcher may resolve to.
//
// Live products, plus the legacy 'pending' rows. The catalogue no longer has a
// review queue — an administrator importing a product is the act that approves
// it, and a vendor import cannot create one at all — so nothing produces
// 'pending' any more and the rows that carry it are simply older.
//
// Excluding them was measured to be ruinous: on 2026-08-26 every one of the
// 3,592 *pharmaceutical* products was 'pending' while the 28,786 'active' ones
// were cosmetics, so a pharmacy's order list matched two rows out of eight
// hundred and the few it did match were toiletries. LoadOffers applies the same
// pair, or a matched line would come back with no supplier and no reason.
//
// SMARTORDER_MATCH_STATUSES overrides it, for testing against a catalogue in an
// unusual state without editing product statuses as a side effect.
func matchableStatuses() []string {
	if raw := strings.TrimSpace(os.Getenv("SMARTORDER_MATCH_STATUSES")); raw != "" {
		var out []string
		for _, s := range strings.Split(raw, ",") {
			if s = strings.TrimSpace(s); s != "" {
				out = append(out, s)
			}
		}
		if len(out) > 0 {
			return out
		}
	}
	return []string{"active", "pending"}
}

// ProductNames resolves catalogue product names for the results and review
// screens, in one query for the whole page.
//
// Arabic first, English as a fallback: the catalogue is Arabic-primary and a
// row with only an English name is rare but must still render as something a
// person can read rather than as a bare id.
func (r *Repository) ProductNames(ctx context.Context, ids []int64) (map[int64]string, error) {
	out := make(map[int64]string, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(txCtx, `
			SELECT p.id,
			       COALESCE(NULLIF(p.name->>'ar', ''), NULLIF(p.name->>'en', ''), '') AS label
			FROM catalog.products p
			WHERE p.id = ANY($1::bigint[]);`, ids)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var id int64
			var label string
			if err := rows.Scan(&id, &label); err != nil {
				return err
			}
			out[id] = label
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// LoadMatchIndex loads the catalogue projection the in-memory matcher scores
// against.
//
// Thirty thousand of these cost a few megabytes; thirty thousand full products
// would not fit the same budget. Loading once per run and scoring in memory is
// what keeps a ten-thousand-row file inside its time budget without issuing a
// query per row.
func (r *Repository) LoadMatchIndex(ctx context.Context) ([]smartorder.IndexedProduct, error) {
	var out []smartorder.IndexedProduct
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(txCtx, `
			SELECT
				p.id,
				COALESCE(p.name->>'ar', '')            AS name_ar,
				COALESCE(p.name->>'en', '')            AS name_en,
				COALESCE(p.sku, '')                    AS sku,
				COALESCE(p.barcode, '')                AS barcode,
				COALESCE(p.scientific_name, '')        AS scientific,
				COALESCE(p.dosage_form, '')            AS dosage_form,
				COALESCE(p.concentration, '')          AS concentration,
				COALESCE(p.unit, '')                   AS unit,
				COALESCE(NULLIF(p.manufacturing_companies, ''), p.company, '') AS manufacturer
			FROM catalog.products p
			WHERE p.deleted_at IS NULL AND p.status = ANY($1::text[]);`, matchableStatuses())
		if err != nil {
			return err
		}
		defer rows.Close()

		// Pre-sized for the measured catalogue so the slice does not regrow
		// thirty thousand times on every run.
		out = make([]smartorder.IndexedProduct, 0, 32000)
		for rows.Next() {
			var p smartorder.IndexedProduct
			if err := rows.Scan(&p.ID, &p.NameAR, &p.NameEN, &p.SKU, &p.Barcode,
				&p.Scientific, &p.DosageForm, &p.Concentration, &p.Unit, &p.Manufacturer); err != nil {
				return err
			}
			out = append(out, p)
		}
		return rows.Err()
	})
	return out, err
}
