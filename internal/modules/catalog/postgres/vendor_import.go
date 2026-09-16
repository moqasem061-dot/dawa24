package postgres

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/platform/database"
)

// Bulk variant persistence for the vendor catalogue import.
//
// Both operations run inside one transaction per call, so a batch either lands
// whole or not at all, and a failure on one row is reported against that row
// rather than taking the batch down with it.

// ListVariantKeys loads every live variant of one organisation, reduced to the
// fields the importer matches on.
//
// One query for a vendor's whole catalogue is deliberate. The alternative â€” a
// lookup per spreadsheet row â€” is what made the previous importer take minutes
// on a file that this reads in seconds, and it issued those lookups against
// three different keys, so a nine-thousand-row file cost twenty-seven thousand
// round trips before it wrote anything.
func (r *Repository) ListVariantKeys(ctx context.Context, orgID int64) ([]catalog.VariantKey, error) {
	var out []catalog.VariantKey
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(txCtx, `
			SELECT id, COALESCE(product_id, 0), sku, barcode, unit, batch_number, branch_id,
			       status = 'active'
			FROM catalog.product_variants
			WHERE organization_id = $1 AND deleted_at IS NULL
			ORDER BY id`, orgID)
		if err != nil {
			return fmt.Errorf("catalog postgres: list variant keys: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			var k catalog.VariantKey
			if err := rows.Scan(&k.ID, &k.ProductID, &k.SKU, &k.Barcode,
				&k.Unit, &k.BatchNumber, &k.BranchID, &k.Active); err != nil {
				return fmt.Errorf("catalog postgres: scan variant key: %w", err)
			}
			out = append(out, k)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// Every value below arrives as text and is cast in SQL.
//
// That is deliberate rather than fussy. Postgres infers a parameter's type from
// where it sits, and the previous statement sat a jsonb name inside
// COALESCE(NULLIF($3, â€), name) â€” which asks it to match text against jsonb
// and is refused outright with "COALESCE types text and jsonb cannot be
// matched". Every UPDATE in every import failed on it, was caught by the
// per-row isolation path, and was reported to the vendor as "could not save
// this item, an unexpected error" â€” nine hundred times in a row on a file whose
// items all already existed. Nothing was ever updated and no balance ever moved.
//
// Typing every parameter as text and casting it explicitly removes the
// inference entirely: what the statement does no longer depends on what
// Postgres guessed a placeholder meant.
//
// The empty string is the "say nothing about this column" marker, which is what
// makes a partial file safe. A price list that carries names and prices but no
// barcode, batch or expiry column must not wipe the identifiers future imports
// match on, and a supplier who exports no cost price must not have last month's
// cost erased.
const insertVariantSQL = `
	INSERT INTO catalog.product_variants (
		organization_id, product_id, name, sku, barcode, price, cost_price,
		cost_discount_percentage,
		discount, unit, image, status, is_featured, is_negotiable, batch_number,
		expiry_date, min_order_qty, branch_id, variant_type
	) VALUES (
		$1,
		NULLIF($2::text, '')::bigint,
		COALESCE(NULLIF($3::text, '')::jsonb, '{}'::jsonb),
		$4::text, $5::text,
		COALESCE(NULLIF($6::text, '')::numeric, 0),
		NULLIF($7::text, '')::numeric,
		COALESCE(NULLIF($8::text, '')::numeric, 0),
		COALESCE(NULLIF($9::text, '')::numeric, 0),
		$10::text, $11::text,
		COALESCE(NULLIF($12::text, ''), 'active'),
		COALESCE(NULLIF($13::text, '')::boolean, false),
		COALESCE(NULLIF($14::text, '')::boolean, false),
		$15::text,
		NULLIF($16::text, '')::date,
		COALESCE(NULLIF($17::text, '')::int, 1),
		COALESCE(
			NULLIF($18::text, '')::bigint,
			(SELECT b.id FROM org.branches b
			  WHERE b.organization_id = $1 AND b.deleted_at IS NULL
			  ORDER BY b.is_main DESC, b.id ASC LIMIT 1)),
		'standard'
	)
	RETURNING id`

// updateVariantSQL refreshes an existing variant.
//
// Status is passed rather than assumed. An import's "publish immediately"
// switch decides what NEW rows get; on an update the caller sends â€ so the
// variant keeps the status it has, because an unticked box on a routine price
// refresh must not delist a vendor's whole catalogue.
const updateVariantSQL = `
	UPDATE catalog.product_variants
	SET name         = COALESCE(NULLIF($3::text,  '')::jsonb, name),
	    sku          = COALESCE(NULLIF($4::text,  ''), sku),
	    barcode      = COALESCE(NULLIF($5::text,  ''), barcode),
	    price        = COALESCE(NULLIF($6::text,  '')::numeric, price),
	    cost_price   = COALESCE(NULLIF($7::text,  '')::numeric, cost_price),
	    cost_discount_percentage =
	                   COALESCE(NULLIF($8::text,  '')::numeric, cost_discount_percentage),
	    discount     = COALESCE(NULLIF($9::text,  '')::numeric, discount),
	    unit         = COALESCE(NULLIF($10::text, ''), unit),
	    image        = COALESCE(NULLIF($11::text, ''), image),
	    is_negotiable= COALESCE(NULLIF($12::text, '')::boolean, is_negotiable),
	    batch_number = COALESCE(NULLIF($13::text, ''), batch_number),
	    expiry_date  = COALESCE(NULLIF($14::text, '')::date, expiry_date),
	    min_order_qty= COALESCE(NULLIF($15::text, '')::int, min_order_qty),
	    branch_id    = COALESCE(
	                     NULLIF($16::text, '')::bigint,
	                     branch_id,
	                     (SELECT b.id FROM org.branches b
	                       WHERE b.organization_id = $2 AND b.deleted_at IS NULL
	                       ORDER BY b.is_main DESC, b.id ASC LIMIT 1)),
	    product_id   = COALESCE(NULLIF($17::text, '')::bigint, product_id),
	    status       = COALESCE(NULLIF($18::text, ''), status),
	    updated_at   = now()
	WHERE id = $1 AND organization_id = $2 AND deleted_at IS NULL
	RETURNING id`

// errRowRejected marks a batch in which at least one statement was refused. It
// never reaches a caller; it rolls the batch back so the rows can be retried
// one at a time.
var errRowRejected = errors.New("catalog postgres: a row in the batch was rejected")

// BulkWriteVariants inserts and updates a batch of variants.
//
// The happy path is one pipelined round trip for the whole batch. Postgres
// aborts a transaction at the first failed statement, though, so a single bad
// row would otherwise take five hundred good ones with it â€” and "one row in
// this file has a duplicate code" must not mean "none of your catalogue
// imported". When the batch is refused, it is rolled back and rewritten row by
// row, each in its own transaction, so the cost of isolating the bad row falls
// only on the file that has one.
func (r *Repository) BulkWriteVariants(
	ctx context.Context, orgID int64, rows []catalog.VariantWriteRow,
) (catalog.VariantWriteResult, error) {
	result, err := r.batchVariants(ctx, orgID, rows)
	if err == nil {
		return result, nil
	}
	if !errors.Is(err, errRowRejected) {
		return result, fmt.Errorf("catalog postgres: bulk write variants: %w", err)
	}
	return r.writeVariantsOneByOne(ctx, orgID, rows)
}

// batchVariants writes the whole batch in one pipelined transaction.
func (r *Repository) batchVariants(
	ctx context.Context, orgID int64, rows []catalog.VariantWriteRow,
) (catalog.VariantWriteResult, error) {
	result := catalog.VariantWriteResult{
		IDs:          make(map[int]int64, len(rows)),
		InsertedRefs: make(map[int]bool, len(rows)),
	}
	if len(rows) == 0 {
		return result, nil
	}

	err := r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		batch := &pgx.Batch{}
		queued := make([]catalog.VariantWriteRow, 0, len(rows))
		for _, row := range rows {
			if row.Variant == nil {
				continue
			}
			queued = append(queued, row)
			queueVariant(batch, orgID, row.Variant)
		}

		br := tx.SendBatch(txCtx, batch)
		ids := make(map[int]int64, len(queued))
		var rejected error
		for _, row := range queued {
			var id int64
			if err := br.QueryRow().Scan(&id); err != nil && rejected == nil {
				rejected = err
			}
			ids[row.Ref] = id
		}
		if closeErr := br.Close(); closeErr != nil && rejected == nil {
			rejected = closeErr
		}
		if rejected != nil {
			return errRowRejected
		}

		for _, row := range queued {
			result.IDs[row.Ref] = ids[row.Ref]
			if row.Variant.ID > 0 {
				result.Updated++
			} else {
				row.Variant.ID = ids[row.Ref]
				result.InsertedRefs[row.Ref] = true
				result.Inserted++
			}
		}
		return nil
	})
	if err != nil {
		return catalog.VariantWriteResult{
			IDs: map[int]int64{}, InsertedRefs: map[int]bool{},
		}, err
	}
	return result, nil
}

// writeVariantsOneByOne is the isolation path: every row gets its own
// transaction, so a rejection costs exactly that row.
func (r *Repository) writeVariantsOneByOne(
	ctx context.Context, orgID int64, rows []catalog.VariantWriteRow,
) (catalog.VariantWriteResult, error) {
	result := catalog.VariantWriteResult{
		IDs:          make(map[int]int64, len(rows)),
		InsertedRefs: make(map[int]bool, len(rows)),
	}
	for _, row := range rows {
		if row.Variant == nil {
			continue
		}
		var id int64
		err := r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
			batch := &pgx.Batch{}
			queueVariant(batch, orgID, row.Variant)
			br := tx.SendBatch(txCtx, batch)
			if err := br.QueryRow().Scan(&id); err != nil {
				_ = br.Close()
				return err
			}
			return br.Close()
		})
		// An update that matched nothing means the variant was deleted between
		// the index being loaded and the write. The vendor's row is still valid,
		// so it is inserted rather than reported as a failure they can do
		// nothing about.
		if err != nil && row.Variant.ID > 0 && database.IsNotFound(err) {
			row.Variant.ID = 0
			err = r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
				batch := &pgx.Batch{}
				queueVariant(batch, orgID, row.Variant)
				br := tx.SendBatch(txCtx, batch)
				if scanErr := br.QueryRow().Scan(&id); scanErr != nil {
					_ = br.Close()
					return scanErr
				}
				return br.Close()
			})
		}
		if err != nil {
			result.Failures = append(result.Failures, catalog.VariantWriteFailure{
				Ref:     row.Ref,
				Message: writeFailureMessage(err),
			})
			continue
		}
		result.IDs[row.Ref] = id
		if row.Variant.ID > 0 {
			result.Updated++
		} else {
			row.Variant.ID = id
			result.InsertedRefs[row.Ref] = true
			result.Inserted++
		}
	}
	return result, nil
}

// queueVariant appends one insert or update to a batch.
//
// Every argument is rendered to text here, matching the statements above. An
// empty string means "this file said nothing about that column", which the
// UPDATE reads as "leave it alone" and the INSERT reads as "use the default".
func queueVariant(batch *pgx.Batch, orgID int64, v *catalog.ProductVariant) {
	name := textJSON(v.Name)
	price := amountText(v.Price)
	cost := ""
	if v.CostPrice != nil && !v.CostPrice.IsZero() {
		cost = v.CostPrice.String()
	}
	costDiscount := ""
	if v.CostDiscountPercentage > 0 {
		costDiscount = strconv.FormatFloat(v.CostDiscountPercentage, 'f', -1, 64)
	}
	// A discount is only meaningful beside the price it applies to. Writing a
	// zero discount from a file that carried no price at all would silently
	// clear a discount the vendor set by hand on the product screen.
	discount := ""
	if price != "" {
		discount = amountTextOrZero(v.Discount)
	}
	minOrderQty := ""
	if v.MinOrderQty > 0 {
		minOrderQty = strconv.Itoa(v.MinOrderQty)
	}

	if v.ID > 0 {
		batch.Queue(updateVariantSQL,
			v.ID, orgID, name, v.SKU, v.Barcode, price, cost, costDiscount,
			discount, v.Unit, v.Image, boolText(v.IsNegotiable),
			v.BatchNumber, dateText(v.ExpiryDate), minOrderQty,
			idText(v.BranchID), idText(nullableID(v.ProductID)), string(v.Status))
		return
	}
	batch.Queue(insertVariantSQL,
		orgID, idText(nullableID(v.ProductID)), name, v.SKU, v.Barcode,
		price, cost, costDiscount, discount, v.Unit, v.Image, string(v.Status),
		boolText(v.IsFeatured), boolText(v.IsNegotiable), v.BatchNumber,
		dateText(v.ExpiryDate), minOrderQty, idText(v.BranchID))
}

