package postgres

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/shared/arabic"
	"github.com/muhiya/dawa24-store/internal/shared/productmatch"
)

// Reading the catalogue an import is matched against.
//
// Once, into memory, indexed by every identifier the resolver tries. The three
// tiers used to ask the database separately and repeatedly: item codes in
// batches of two thousand keys, barcodes the same, then a scan for the names.
// A twenty-thousand-row file was around twenty round trips, and not one of them
// could use an index — `lower(btrim(sku)) = ANY($2)` does not match the plain
// index on sku, and `name->>'ar'` has no equality index at all — so each was a
// sequential scan of the whole products table with a function call per row.
//
// That is what an administrator watching the progress bar was waiting on. It
// sat at the start of the matching band, reporting nought of twenty thousand,
// for as long as those scans took; on a catalogue large enough it did not
// finish at all, because the pool's statement timeout cancelled one of them and
// the run failed with a message about a statement rather than about a file.
//
// One scan, and the matching happens in Go. Forty thousand products is a few
// megabytes and a fraction of a second, against twenty scans of the same table.

// catalogueRow is one existing product as the resolver needs to see it: the
// identifiers it can be found by, and the attributes that tell it apart from
// another product with the same name.
type catalogueRow struct {
	id      int64
	sku     string
	barcode string
	fields  productmatch.ProductFields
}

// catalogue is an organisation's products indexed by each identifier in turn.
//
// The exact tiers keep one id per key — the lowest, which is what the previous
// min(id) queries returned. The name tier keeps every row behind a name,
// because which of them an incoming row means is a question about strengths and
// forms that only chooseByIdentity can answer.
type catalogue struct {
	bySKU     map[string]int64
	byBarcode map[string]int64
	byName    map[string][]nameCandidate
}

// lookupSKU and lookupBarcode resolve an exact identifier, folded the same way
// on both sides.
func (c *catalogue) lookupSKU(raw string) (int64, bool) {
	id, ok := c.bySKU[foldIdentifier(raw)]
	return id, ok
}

func (c *catalogue) lookupBarcode(raw string) (int64, bool) {
	id, ok := c.byBarcode[foldIdentifier(raw)]
	return id, ok
}

// candidatesFor returns the catalogue rows sharing an incoming row's name.
func (c *catalogue) candidatesFor(name string) []nameCandidate {
	return c.byName[arabic.Normalize(name)]
}

// foldIdentifier is how an item code or barcode is compared, on both sides of
// the match. It mirrors what the SQL did — lower(btrim(...)) — so a catalogue
// loaded here resolves exactly the rows the queries used to.
func foldIdentifier(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

// loadCatalogue reads one organisation's live products into the index above.
func loadCatalogue(ctx context.Context, tx pgx.Tx, orgID int64) (*catalogue, error) {
	c := &catalogue{
		bySKU:     map[string]int64{},
		byBarcode: map[string]int64{},
		byName:    map[string][]nameCandidate{},
	}

	rows, err := tx.Query(ctx, `
		SELECT p.id,
		       COALESCE(p.sku, ''),
		       COALESCE(p.barcode, ''),
		       COALESCE(p.name->>'ar', ''),
		       COALESCE(p.name->>'en', ''),
		       COALESCE(p.concentration, ''),
		       COALESCE(p.dosage_form, ''),
		       COALESCE(p.unit, '')
		FROM catalog.products p
		WHERE p.organization_id = $1
		  AND p.deleted_at IS NULL
		ORDER BY p.id
	`, orgID)
	if err != nil {
		return nil, fmt.Errorf("catalog postgres: load catalogue for matching: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var r catalogueRow
		if err := rows.Scan(&r.id, &r.sku, &r.barcode,
			&r.fields.NameAR, &r.fields.NameEN,
			&r.fields.Concentration, &r.fields.DosageForm, &r.fields.Unit); err != nil {
			return nil, fmt.Errorf("catalog postgres: scan catalogue row: %w", err)
		}
		c.add(r)
	}
	if rows.Err() != nil {
		return nil, fmt.Errorf("catalog postgres: read catalogue for matching: %w", rows.Err())
	}
	return c, nil
}

// add files one product under each identifier it carries.
//
// Rows arrive lowest id first, so keeping the first of a duplicated identifier
// is the same answer min(id) gave — and the same answer on every re-run, which
// is what stops a second import of one file resolving somewhere else.
func (c *catalogue) add(r catalogueRow) {
	if key := foldIdentifier(r.sku); key != "" {
		if _, taken := c.bySKU[key]; !taken {
			c.bySKU[key] = r.id
		}
	}
	if key := foldIdentifier(r.barcode); key != "" {
		if _, taken := c.byBarcode[key]; !taken {
			c.byBarcode[key] = r.id
		}
	}
	if key := arabic.Normalize(r.fields.NameAR); key != "" {
		c.byName[key] = append(c.byName[key], nameCandidate{id: r.id, fields: r.fields})
	}
}
