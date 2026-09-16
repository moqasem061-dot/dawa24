package corpus

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/muhiya/dawa24-store/internal/shared/productmatch"
	"github.com/muhiya/dawa24-store/internal/shared/sheet"
)

// TestBarcodeLabels counts how many rows in each corpus file carry a barcode
// that resolves uniquely in the catalogue — i.e. how many labelled examples
// real vendor files can supply.
func TestBarcodeLabels(t *testing.T) {
	if os.Getenv("TRUTH_PROBE") == "" {
		t.Skip("set TRUTH_PROBE=1")
	}
	idx, err := LoadIndex()
	if err != nil {
		t.Fatal(err)
	}
	entries, err := LoadManifest()
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		content, err := os.ReadFile(filepath.Join(Dir, "files", e.File))
		if err != nil {
			continue
		}
		book, err := sheet.Open(content, e.File)
		if err != nil {
			continue
		}
		a, err := productmatch.Analyze(book, nil)
		if err != nil {
			book.Close()
			continue
		}
		a.Complete()
		rows, labelled := 0, 0
		opts := productmatch.DefaultMatchOptions()
		opts.TrustBarcode = true
		_, err = productmatch.Process(book, a.Layout, a.Mapping, productmatch.DefaultProcessOptions(),
			func(batch []*productmatch.Row) error {
				for _, row := range batch {
					rows++
					if row.Barcode == "" {
						continue
					}
					res := idx.Match(row, opts)
					if res.Level == productmatch.MatchBarcode {
						labelled++
					}
				}
				return nil
			})
		book.Close()
		if err != nil || rows == 0 {
			continue
		}
		t.Logf("%-22s rows=%-6d barcode-labelled=%-6d (%d%%)", e.File, rows, labelled, labelled*100/rows)
	}
}
