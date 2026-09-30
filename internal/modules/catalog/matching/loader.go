package matching

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
	"github.com/moqasem061-dot/dawa24/internal/shared/productmatch"
)

const (
	// DefaultPageSize is the number of products fetched per keyset page.
	DefaultPageSize = 5000
	// DefaultHardCeiling is the maximum number of catalogue products loaded into memory.
	DefaultHardCeiling = 500000
)

// LoaderStats reports statistics about the catalogue loading run.
type LoaderStats struct {
	Rows      int  `json:"rows"`
	Pages     int  `json:"pages"`
	Truncated bool `json:"truncated"`
}

// LoaderOptions configures the catalogue loader.
type LoaderOptions struct {
	PageSize    int
	HardCeiling int
}

// Loader loads master catalogue products using keyset pagination.
type Loader struct {
	db *database.DB
}

// NewLoader creates a new master catalogue loader.
func NewLoader(db *database.DB) *Loader {
	return &Loader{db: db}
}

// Load retrieves all active master catalogue products with keyset pagination by id.
func (l *Loader) Load(ctx context.Context, opts ...LoaderOptions) ([]productmatch.MasterProduct, LoaderStats, error) {
	pageSize := DefaultPageSize
	hardCeiling := DefaultHardCeiling
	if len(opts) > 0 {
		if opts[0].PageSize > 0 {
			pageSize = opts[0].PageSize
		}
		if opts[0].HardCeiling > 0 {
			hardCeiling = opts[0].HardCeiling
		}
	}

	var stats LoaderStats
	var products []productmatch.MasterProduct
	var lastID int64

	for {
		if stats.Rows >= hardCeiling {
			stats.Truncated = true
			break
		}

		batchLimit := pageSize
		if remaining := hardCeiling - stats.Rows; remaining < batchLimit {
			batchLimit = remaining
		}

		var batch []productmatch.MasterProduct
		var maxID int64

		err := l.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
			query := `
				SELECT id,
				       COALESCE(name->>'ar', ''),
				       COALESCE(name->>'en', ''),
				       COALESCE(sku, ''),
				       COALESCE(barcode, ''),
				       COALESCE(scientific_name, ''),
				       COALESCE(dosage_form, ''),
				       COALESCE(concentration, ''),
				       COALESCE(unit, ''),
				       COALESCE(company, manufacturing_companies, ''),
				       COALESCE(price::text, '0.00')
				FROM catalog.products
				WHERE id > $1 AND deleted_at IS NULL
				ORDER BY id ASC
				LIMIT $2;
			`
			rows, err := tx.Query(txCtx, query, lastID, batchLimit)
			if err != nil {
				return err
			}
			defer rows.Close()

			for rows.Next() {
				var p productmatch.MasterProduct
				if err := rows.Scan(
					&p.ID,
					&p.NameAR,
					&p.NameEN,
					&p.SKU,
					&p.Barcode,
					&p.Scientific,
					&p.DosageForm,
					&p.Concentration,
					&p.Unit,
					&p.Manufacturer,
					&p.PublicPrice,
				); err != nil {
					return err
				}
				batch = append(batch, p)
				maxID = p.ID
			}
			return rows.Err()
		})

		if err != nil {
			return nil, stats, fmt.Errorf("load catalogue batch: %w", err)
		}

		if len(batch) == 0 {
			break
		}

		stats.Pages++
		stats.Rows += len(batch)
		products = append(products, batch...)
		lastID = maxID

		if len(batch) < batchLimit {
			break
		}
	}

	return products, stats, nil
}
