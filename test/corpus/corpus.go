// Package corpus runs real import files through the matching engine and
// reports what it resolved.
//
// It exists because every claim about matching in this repository — a floor, a
// weight, a veto, a retrieval strategy — was until now argued from a comment.
// A comment cannot be re-checked. This can: `go test ./test/corpus -run
// TestCorpusReport -v` reads twenty-four files that real admins, vendors and
// pharmacies actually uploaded, scores them against a snapshot of the real
// catalogue, and prints the match rate per file and per tier.
//
// Nothing here asserts a number. Numbers belong in the baseline file the report
// writes, so a change to the engine shows up as a diff a human reads rather
// than as a test that fails with no explanation of which direction it moved.
package corpus

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/muhiya/dawa24-store/internal/shared/productmatch"
	"github.com/muhiya/dawa24-store/internal/shared/sheet"
)

// Entry is one corpus file, as recorded by `cli corpus-export`.
type Entry struct {
	System     string   `json:"system"`
	Source     string   `json:"source"`
	File       string   `json:"file"`
	Bytes      int      `json:"bytes"`
	Rows       int      `json:"rows,omitempty"`
	Duplicates []string `json:"duplicates,omitempty"`
}

// snapshotProduct mirrors the compact shape the exporter writes.
type snapshotProduct struct {
	ID     int64  `json:"i"`
	NameAR string `json:"a"`
	NameEN string `json:"e"`
	SKU    string `json:"k"`
	Barode string `json:"b"`
	Sci    string `json:"s"`
	Form   string `json:"f"`
	Conc   string `json:"c"`
	Unit   string `json:"u"`
	Maker  string `json:"m"`
	Price  string `json:"p"`
}

// Dir is where the exported corpus lives, relative to this package.
const Dir = "."

// Available reports whether the corpus has been exported.
//
// It is checked rather than assumed so a fresh clone runs the rest of the suite
// green: the corpus is 4 MB of real customer files, and a developer who has not
// pulled it should get a skip, not a failure.
func Available() bool {
	_, err := os.Stat(filepath.Join(Dir, "manifest.json"))
	return err == nil
}

// LoadManifest reads the list of exported files.
func LoadManifest() ([]Entry, error) {
	raw, err := os.ReadFile(filepath.Join(Dir, "manifest.json"))
	if err != nil {
		return nil, err
	}
	var out []Entry
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// LoadIndex builds the matching index from the catalogue snapshot.
func LoadIndex() (*productmatch.Index, error) {
	f, err := os.Open(filepath.Join(Dir, "catalogue.jsonl.gz"))
	if err != nil {
		return nil, err
	}
	defer f.Close()

	zr, err := gzip.NewReader(f)
	if err != nil {
		return nil, err
	}
	defer zr.Close()

	masters := make([]productmatch.MasterProduct, 0, 20000)
	sc := bufio.NewScanner(zr)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		var p snapshotProduct
		if err := json.Unmarshal(sc.Bytes(), &p); err != nil {
			return nil, err
		}
		masters = append(masters, productmatch.MasterProduct{
			ID: p.ID, NameAR: p.NameAR, NameEN: p.NameEN, SKU: p.SKU,
			Barcode: p.Barode, Scientific: p.Sci, DosageForm: p.Form,
			Concentration: p.Conc, Unit: p.Unit, Manufacturer: p.Maker,
			PublicPrice: p.Price,
		})
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return productmatch.NewIndex(masters), nil
}

// FileReport is what one corpus file resolved to.
type FileReport struct {
	Entry Entry `json:"entry"`
	// Error is a file the engine could not read at all, which is a result
	// worth recording rather than a reason to stop.
	Error string `json:"error,omitempty"`

	Columns  []string `json:"columns,omitempty"`
	DataRows int      `json:"data_rows"`
	Parsed   int      `json:"parsed"`
	Rejected int      `json:"rejected"`
	// Duplicates counts rows collapsed onto an identity already seen in the
	// same file. A large figure here is not a supplier writing the same product
	// twice; it is the identity key being too coarse to tell two products
	// apart, and it silently removes rows before matching ever sees them.
	Duplicates int `json:"duplicates"`

	// Levels counts rows by the match level the engine reached.
	Levels map[string]int `json:"levels"`
	// Settled is the share the engine would apply without asking, as a
	// percentage of parsed rows. It is the single number this whole corpus
	// exists to move.
	SettledPct int `json:"settled_pct"`
	// MatchedPct includes the tiers that need review, which is what the
	// wizard's own "matched" counter shows.
	MatchedPct int `json:"matched_pct"`
}

// Run scores one corpus file against the index.
func Run(idx *productmatch.Index, e Entry) FileReport {
	rep := FileReport{Entry: e, Levels: map[string]int{}}

	content, err := os.ReadFile(filepath.Join(Dir, "files", e.File))
	if err != nil {
		rep.Error = err.Error()
		return rep
	}
	book, err := sheet.Open(content, e.File)
	if err != nil {
		rep.Error = err.Error()
		return rep
	}
	defer func() { _ = book.Close() }()

	analysis, err := productmatch.Analyze(book, nil)
	if err != nil {
		rep.Error = err.Error()
		return rep
	}
	analysis.Complete()
	if analysis.Mapping != nil {
		for _, c := range analysis.Mapping.Columns {
			if c != nil && c.Field != "" {
				rep.Columns = append(rep.Columns, string(c.Field))
			}
		}
		sort.Strings(rep.Columns)
	}

	opts := productmatch.DefaultMatchOptions()
	result, err := productmatch.Process(book, analysis.Layout, analysis.Mapping,
		productmatch.DefaultProcessOptions(),
		func(batch []*productmatch.Row) error {
			for _, row := range batch {
				res := idx.Match(row, opts)
				rep.Levels[string(res.Level)]++
			}
			return nil
		})
	if err != nil {
		rep.Error = err.Error()
		return rep
	}

	rep.DataRows = result.Stats.DataRows
	rep.Parsed = result.Stats.Parsed
	rep.Rejected = result.Stats.Rejected
	rep.Duplicates = result.Stats.Duplicates
	rep.SettledPct = pct(settled(rep.Levels), rep.Parsed)
	rep.MatchedPct = pct(rep.Parsed-rep.Levels[string(productmatch.MatchNone)], rep.Parsed)
	return rep
}

// settled counts the levels the engine applies without asking.
func settled(levels map[string]int) int {
	n := 0
	for _, l := range []productmatch.MatchLevel{
		productmatch.MatchBarcode, productmatch.MatchCode,
		productmatch.MatchExact, productmatch.MatchStrong,
	} {
		n += levels[string(l)]
	}
	return n
}

func pct(n, total int) int {
	if total <= 0 {
		return 0
	}
	return n * 100 / total
}

// Format renders one report as a single line for the console.
func (r FileReport) Format() string {
	if r.Error != "" {
		return fmt.Sprintf("%-22s %-10s  ERROR: %s", r.Entry.File, r.Entry.System, r.Error)
	}
	return fmt.Sprintf("%-22s %-10s rows=%-6d parsed=%-6d rej=%-6d dup=%-4d settled=%3d%% matched=%3d%%  %s",
		r.Entry.File, r.Entry.System, r.DataRows, r.Parsed, r.Rejected, r.Duplicates,
		r.SettledPct, r.MatchedPct, levelSummary(r.Levels))
}

// levelSummary renders the tier breakdown in a stable order.
func levelSummary(levels map[string]int) string {
	order := []productmatch.MatchLevel{
		productmatch.MatchBarcode, productmatch.MatchCode, productmatch.MatchExact,
		productmatch.MatchStrong, productmatch.MatchReview, productmatch.MatchAmbiguous,
		productmatch.MatchNone,
	}
	out := ""
	for _, l := range order {
		if n := levels[string(l)]; n > 0 {
			out += fmt.Sprintf("%s=%d ", l, n)
		}
	}
	return out
}
