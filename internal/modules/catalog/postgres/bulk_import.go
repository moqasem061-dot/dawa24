package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// chunkSize bounds one pipelined round trip. Large enough that a 10,000-row
// import is fifty round trips rather than ten thousand, small enough that the
// per-row replay after a failure stays cheap.
const chunkSize = 500

// maxNamedFailures bounds how many offending rows the error message names.
const maxNamedFailures = 20

const insertProductQuery = `
	INSERT INTO catalog.products (
		organization_id, category_id, brand_id, branch_id, name, description,
		sku, barcode, price, discount, old_price, image, image_link, status,
		is_featured, dosage_form, scientific_name, pharmacology, active,
		concentration, unit, manufacturing_companies, institutional_work_ids
	) VALUES (
		$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16,
		$17, $18, $19, $20, $21, $22, $23::bigint[]
	) RETURNING id;
`

// updateProductQuery refreshes an existing catalogue row from an imported one.
//
// Every assignment is guarded so a sparse import cannot erase data the
// catalogue already holds: a supplier price list that carries names and prices
// but no scientific names must update the prices and leave the pharmacology
// alone. COALESCE is not enough for this — the columns are NOT NULL with an
// empty-string default — so the guard is an explicit emptiness test per column.
const updateProductQuery = `
	UPDATE catalog.products SET
		name        = CASE WHEN $2::jsonb IS NULL OR $2::jsonb = '{}'::jsonb
		                   THEN name ELSE $2::jsonb END,
		description = CASE WHEN $3::jsonb IS NULL OR $3::jsonb = '{}'::jsonb
		                   THEN description ELSE $3::jsonb END,
		sku         = CASE WHEN $4::text  = '' THEN sku      ELSE $4::text  END,
		barcode     = CASE WHEN $5::text  = '' THEN barcode  ELSE $5::text  END,
		price       = CASE WHEN $6::numeric = 0 THEN price   ELSE $6::numeric END,
		discount    = CASE WHEN $7::numeric = 0 THEN discount ELSE $7::numeric END,
		old_price   = CASE WHEN $8::numeric = 0 THEN old_price ELSE $8::numeric END,
		status      = CASE WHEN $9::text = '' THEN status ELSE $9::text END,
		dosage_form = CASE WHEN $10::text = '' THEN dosage_form ELSE $10::text END,
		scientific_name = CASE WHEN $11::text = '' THEN scientific_name ELSE $11::text END,
		active      = CASE WHEN $12::text = '' THEN active ELSE $12::text END,
		concentration = CASE WHEN $13::text = '' THEN concentration ELSE $13::text END,
		unit        = CASE WHEN $14::text = '' THEN unit ELSE $14::text END,
		manufacturing_companies = CASE WHEN $15::text = ''
		                   THEN manufacturing_companies ELSE $15::text END,
		brand_id    = COALESCE($16::bigint, brand_id),
		image       = CASE WHEN $17::text = '' THEN image ELSE $17::text END,
		image_link  = CASE WHEN $18::text = '' THEN image_link ELSE $18::text END,
		updated_at  = now()
	WHERE id = $1 AND deleted_at IS NULL;
`

// BulkUpsertProducts inserts new products and refreshes existing ones, matching
// on SKU, then barcode, then normalised Arabic name, within each product's
// organisation.
func (r *Repository) BulkUpsertProducts(
	ctx context.Context, prods []*catalog.Product, opts catalog.BulkWriteOptions,
) (catalog.BulkWriteResult, error) {
	result := catalog.BulkWriteResult{Matches: map[int]catalog.MatchReason{}}
	if len(prods) == 0 {
		return result, nil
	}

	err := r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		var err error
		result, err = bulkUpsertInTx(txCtx, tx, prods, opts)
		return err
	})

	if err != nil {
		// A rollback happened, so nothing was written whatever the counters
		// reached mid-flight. Reporting them would tell the admin rows landed
		// that did not.
		failures := result.Failures
		return catalog.BulkWriteResult{Matches: map[int]catalog.MatchReason{}, Failures: failures}, err
	}
	return result, nil
}

// BulkCommitProducts is the whole clear-and-add commit in one transaction: the
// archive of the existing catalogue and the write of the new one land together
// or not at all.
//
// Splitting them across two transactions meant a failure in the write left the
// catalogue archived with nothing imported — every live product soft-deleted
// and no replacement on file. archiveOrg of zero skips the archive, which is
// what every non-destructive mode passes.
func (r *Repository) BulkCommitProducts(
	ctx context.Context, prods []*catalog.Product, opts catalog.BulkWriteOptions,
	archiveOrg int64,
) (archived int64, result catalog.BulkWriteResult, err error) {
	result = catalog.BulkWriteResult{Matches: map[int]catalog.MatchReason{}}
	if len(prods) == 0 && archiveOrg <= 0 {
		return 0, result, nil
	}

	err = r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		if archiveOrg > 0 {
			tag, err := tx.Exec(txCtx, `
				UPDATE catalog.products
				SET deleted_at = now(), updated_at = now()
				WHERE organization_id = $1 AND deleted_at IS NULL
			`, archiveOrg)
			if err != nil {
				return fmt.Errorf("catalog postgres: archive catalogue: %w", err)
			}
			archived = tag.RowsAffected()
		}
		if len(prods) == 0 {
			return nil
		}
		var upsertErr error
		result, upsertErr = bulkUpsertInTx(txCtx, tx, prods, opts)
		return upsertErr
	})
	if err != nil {
		failures := result.Failures
		return 0, catalog.BulkWriteResult{Matches: map[int]catalog.MatchReason{}, Failures: failures}, err
	}
	return archived, result, nil
}

// bulkUpsertInTx is BulkUpsertProducts' body, callable inside a transaction the
// caller owns — which is what lets the archive and the write share one.
func bulkUpsertInTx(
	ctx context.Context, tx pgx.Tx, prods []*catalog.Product, opts catalog.BulkWriteOptions,
) (catalog.BulkWriteResult, error) {
	result := catalog.BulkWriteResult{Matches: map[int]catalog.MatchReason{}}

	defaultOrgID, err := resolveDefaultOrg(ctx, tx)
	if err != nil {
		return result, err
	}

	for _, p := range prods {
		applyWriteDefaults(p, defaultOrgID)
	}

	brandsCreated, err := resolveBrands(ctx, tx, prods, opts.CreateBrands)
	if err != nil {
		return result, err
	}
	result.BrandsCreated = brandsCreated

	categoriesCreated, err := resolveCategories(ctx, tx, prods, opts.CreateCategories)
	if err != nil {
		return result, err
	}
	result.CategoriesCreated = categoriesCreated

	existing, err := resolveExistingProducts(ctx, tx, prods)
	if err != nil {
		return result, err
	}

	var toInsert, toUpdate []int
	for i, p := range prods {
		if m, ok := existing[i]; ok {
			p.ID = m.id
			result.Matches[i] = m.reason
			toUpdate = append(toUpdate, i)
			continue
		}
		toInsert = append(toInsert, i)
	}

	inserter := &batchWriter{tx: tx, prods: prods, res: &result, queue: queueInsert, scan: scanInsert}
	if err := inserter.write(ctx, toInsert); err != nil {
		return result, err
	}
	updater := &batchWriter{tx: tx, prods: prods, res: &result, queue: queueUpdate, scan: scanUpdate}
	return result, updater.write(ctx, toUpdate)
}

// applyWriteDefaults fills the values the NOT NULL columns require. The products
// table defaults these at the column level, but a parameter binding of the zero
// value overrides the default, so they are set explicitly here.
//
// Status is deliberately not among them. An empty status means the file did not
// state one, which the insert reads as "active" and the update reads as "leave
// it alone" — so re-importing a supplier's price list cannot reactivate a
// product an admin deliberately took off the catalogue.
func applyWriteDefaults(p *catalog.Product, defaultOrgID int64) {
	if p.OrganizationID <= 0 {
		p.OrganizationID = defaultOrgID
	}
	if p.Price.IsZero() {
		p.Price = money.Zero
	}
	if p.Discount.IsZero() {
		p.Discount = money.Zero
	}
	if p.OldPrice.IsZero() {
		p.OldPrice = money.Zero
	}
	if p.InstitutionalWorkIDs == nil {
		p.InstitutionalWorkIDs = []int64{}
	}
}

// masterCatalogOrgEN is the canonical English name of the organisation that
// owns the master catalogue. Migration 128 seeds it; resolveDefaultOrg creates
// it on a deployment that predates the migration.
const masterCatalogOrgEN = "Dawa24 Master Catalog"

// masterCatalogOrgWhere is the one definition of "the master catalogue's
// organisation" every lookup shares.
const masterCatalogOrgWhere = `
	deleted_at IS NULL AND status = 'approved'
	AND name->>'en' = 'Dawa24 Master Catalog'`

// resolveDefaultOrg finds the organisation that owns the master catalogue,
// creating it only when the instance genuinely has none.
//
// The organisation is identified by its canonical name, never by "whichever
// tenant sorts first". The previous lowest-id fallback once landed the whole
// imported catalogue inside a customer pharmacy's tenant — and a second run
// resolving to a different lowest id would have duplicated it rather than
// updated it.
func resolveDefaultOrg(ctx context.Context, tx pgx.Tx) (int64, error) {
	orgID, err := lookupDefaultOrg(ctx, tx)
	if err != nil {
		return 0, err
	}
	if orgID > 0 {
		return orgID, nil
	}

	err = tx.QueryRow(ctx, `
		INSERT INTO org.organizations (name, type, status)
		VALUES ('{"ar":"دوا 24 - الكتالوج المعتمد","en":"Dawa24 Master Catalog"}'::jsonb, 'company', 'approved')
		RETURNING id
	`).Scan(&orgID)
	if err != nil {
		return 0, fmt.Errorf("catalog postgres: create master catalog organization: %w", err)
	}
	return orgID, nil
}

// lookupDefaultOrg finds the organisation that owns the master catalogue without
// creating one, so it is safe inside a read-only transaction.
//
// It returns zero when no canonical organisation exists — never a substitute.
// Preview and commit share this resolver, so both would agree an empty catalogue
// means "everything inserts", which is the honest answer until migration 128
// has seeded the real organisation.
func lookupDefaultOrg(ctx context.Context, tx pgx.Tx) (int64, error) {
	var orgID int64
	err := tx.QueryRow(ctx,
		`SELECT id FROM org.organizations WHERE `+masterCatalogOrgWhere+` LIMIT 1`).Scan(&orgID)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("catalog postgres: resolve default organization: %w", err)
	}
	return orgID, nil
}

// resolveBrands binds each product's manufacturer to a brand row, registering
// manufacturers the catalogue has not seen before.
//
// The previous version issued one INSERT per unrecognised manufacturer inside
// the product loop and ignored the error, so a failed brand insert aborted the
// surrounding transaction invisibly and every subsequent statement failed with
// a message about the wrong thing.
func resolveBrands(
	ctx context.Context, tx pgx.Tx, prods []*catalog.Product, allowCreate bool,
) (int, error) {
	brands, err := loadBrandIndex(ctx, tx)
	if err != nil {
		return 0, err
	}

	created := 0
	for _, p := range prods {
		if p.ManufacturingCompanies == "" || (p.BrandID != nil && *p.BrandID > 0) {
			continue
		}
		key := catalog.NormalizeKey(p.ManufacturingCompanies)
		// A manufacturer of one or two folded characters is a stray cell, not a
		// company; registering it would litter the brand list permanently.
		if len([]rune(key)) < 3 {
			continue
		}

		// An existing manufacturer is reused whatever the toggle says. The
		// toggle governs creating one, not linking to one that is already
		// there — refusing to link would leave the product unbranded and the
		// next import would ask the same question again.
		id, known := brands[key]
		if !known {
			if !allowCreate {
				continue
			}
			if id, err = insertBrand(ctx, tx, p.ManufacturingCompanies); err != nil {
				return created, err
			}
			// Cached immediately, so a file naming the same new manufacturer on
			// a thousand rows creates it once.
			brands[key] = id
			created++
		}
		p.BrandID = &id
	}
	return created, nil
}
