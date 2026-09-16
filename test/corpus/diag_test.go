package corpus

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/muhiya/dawa24-store/internal/shared/productmatch"
	"github.com/muhiya/dawa24-store/internal/shared/sheet"
)

// TestDiagFile prints what one corpus file resolved to, row by row.
//
// It is a diagnostic rather than a gate: run it with CORPUS_DIAG=<file> when a
// change to the engine moves a rate and the question is which rows moved. It
// asserts nothing and skips unless asked for by name.
func TestDiagFile(t *testing.T) {
	name := os.Getenv("CORPUS_DIAG")
	if name == "" || !Available() {
		t.Skip("set CORPUS_DIAG=<file> to dump one corpus file")
	}
	idx, err := LoadIndex()
	if err != nil {
		t.Fatalf("catalogue: %v", err)
	}
	content, err := os.ReadFile(filepath.Join(Dir, "files", name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	book, err := sheet.Open(content, name)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = book.Close() }()

	a, err := productmatch.Analyze(book, nil)
	if err != nil {
		t.Fatalf("analyse: %v", err)
	}
	a.Complete()

	opts := productmatch.DefaultMatchOptions()
	limit := 400
	shown := 0
	_, err = productmatch.Process(book, a.Layout, a.Mapping,
		productmatch.DefaultProcessOptions(),
		func(batch []*productmatch.Row) error {
			for _, row := range batch {
				if shown >= limit {
					return nil
				}
				res := idx.Match(row, opts)
				best := ""
				if len(res.Candidates) > 0 {
					best = res.Candidates[0].Name
				}
				shown++
				t.Logf("%-6s %.2f | %-45s | %s", res.Level, res.Score,
					trunc(row.DisplayName(), 45), trunc(best, 45))
			}
			return nil
		})
	if err != nil {
		t.Fatalf("process: %v", err)
	}
}
