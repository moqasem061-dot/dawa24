package main

// Snapshotting the catalogue the corpus is matched against.
//
// A regression harness that needs a production database is not a gate: it
// cannot run in CI, it cannot run on a laptop offline, and its answers change
// under it whenever someone edits a product. So the matching projection — the
// eleven fields the scorer actually reads, for every live product — is written
// beside the files as a compressed snapshot, and every corpus run scores
// against exactly the same catalogue.

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/platform/database"
)

// catalogueSnapshot is the file the harness loads.
const catalogueSnapshot = "catalogue.jsonl.gz"

// snapshotProduct is one catalogue product as matching sees it.
//
// Field names are single letters because there are twenty thousand of them and
// the key repeats in every line: spelling them out costs about a megabyte for
// no reader's benefit, since nothing but the harness ever opens this file.
type snapshotProduct struct {
	ID     int64  `json:"i"`
	NameAR string `json:"a,omitempty"`
	NameEN string `json:"e,omitempty"`
	SKU    string `json:"k,omitempty"`
	Barode string `json:"b,omitempty"`
	Sci    string `json:"s,omitempty"`
	Form   string `json:"f,omitempty"`
	Conc   string `json:"c,omitempty"`
	Unit   string `json:"u,omitempty"`
	Maker  string `json:"m,omitempty"`
	Price  string `json:"p,omitempty"`
}

// exportCatalogue writes the matching projection of every live product.
func exportCatalogue(ctx context.Context, db *database.DB) error {
	if err := os.MkdirAll(corpusDir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(corpusDir, catalogueSnapshot)
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	zw := gzip.NewWriter(f)
	enc := json.NewEncoder(zw)

	count := 0
	err = db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(txCtx, `
			SELECT p.id,
			       COALESCE(p.name->>'ar', ''), COALESCE(p.name->>'en', ''),
			       p.sku, p.barcode, p.scientific_name, p.dosage_form,
			       p.concentration, p.unit, p.manufacturing_companies,
			       p.price::text
			FROM catalog.products p
			WHERE p.deleted_at IS NULL
			ORDER BY p.id;`)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var s snapshotProduct
			if err := rows.Scan(&s.ID, &s.NameAR, &s.NameEN, &s.SKU, &s.Barode,
				&s.Sci, &s.Form, &s.Conc, &s.Unit, &s.Maker, &s.Price); err != nil {
				return err
			}
			if err := enc.Encode(&s); err != nil {
				return err
			}
			count++
		}
		return rows.Err()
	})
	if err != nil {
		return err
	}
	if err := zw.Close(); err != nil {
		return err
	}

	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	fmt.Printf("catalogue snapshot: %d products, %d bytes at %s\n", count, info.Size(), path)
	return nil
}
