package corpus

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/muhiya/dawa24-store/internal/shared/matchflow"
	"github.com/muhiya/dawa24-store/internal/shared/productmatch"
	"github.com/muhiya/dawa24-store/internal/shared/sheet"
)

// TestPlausibilityGate measures how much of each file's residue would be sent to
// the model, and how much the gate refuses to ask about.
//
// It asserts nothing. The gate's value is a cost argument, and a cost argument
// has to be re-measurable against the same files whenever the retrieval or the
// floor changes.
func TestPlausibilityGate(t *testing.T) {
	if !Available() {
		t.Skip("corpus not exported")
	}
	idx, err := LoadIndex()
	if err != nil {
		t.Fatalf("catalogue: %v", err)
	}
	entries, err := LoadManifest()
	if err != nil {
		t.Fatalf("manifest: %v", err)
	}

	ceil := matchflow.For(matchflow.ProfileOrder)
	recall := productmatch.DefaultRecallOptions()
	recall.Limit = ceil.RecallLimit
	matchOpts := productmatch.DefaultMatchOptions()

	for _, e := range entries {
		if e.Bytes < 20_000 {
			continue // the tiny hand-made CSVs say nothing about cost
		}
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
			_ = book.Close()
			continue
		}
		a.Complete()

		residue, askable := 0, 0
		_, err = productmatch.Process(book, a.Layout, a.Mapping,
			productmatch.DefaultProcessOptions(),
			func(batch []*productmatch.Row) error {
				for _, row := range batch {
					res := idx.Match(row, matchOpts)
					if res.Matched() && res.Score >= 0.85 {
						continue // settled; never reaches the stage
					}
					residue++
					for _, c := range idx.Recall(row, recall) {
						if c.Score >= ceil.MinPlausible {
							askable++
							break
						}
					}
				}
				return nil
			})
		_ = book.Close()
		if err != nil {
			continue
		}
		if residue == 0 {
			continue
		}
		requests := (askable + ceil.MaxItemsPerRequest - 1) / ceil.MaxItemsPerRequest
		before := (residue + ceil.MaxItemsPerRequest - 1) / ceil.MaxItemsPerRequest
		t.Logf("%-22s residue=%-6d asked=%-6d skipped=%-6d (%3d%%)  requests %d -> %d",
			e.File, residue, askable, residue-askable,
			(residue-askable)*100/residue, before, requests)
	}
}
