package corpus

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/modules/ingest"
	"github.com/moqasem061-dot/dawa24/internal/shared/productmatch"
	"github.com/moqasem061-dot/dawa24/internal/shared/sheet"
	"github.com/moqasem061-dot/dawa24/internal/ui/pages"
)

// Real supplier files, staged through the matcher, stored the way the
// database stores them (payload and candidates round-tripped through JSON),
// and rendered as the vendor-import review page — lowest score first, the
// "All" tab's order. Every page must come out with its table.
func TestReviewPageRendersRealStagedRows(t *testing.T) {
	idx, err := LoadIndex()
	if err != nil {
		t.Skip(err)
	}
	entries, _ := LoadManifest()
	if len(entries) > 12 {
		entries = entries[:12]
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
		analysis, err := productmatch.Analyze(book, nil)
		if err != nil {
			continue
		}
		analysis.Complete()
		var outs []*ingest.RowOutcome
		id := int64(0)
		_, _ = productmatch.Process(book, analysis.Layout, analysis.Mapping, productmatch.DefaultProcessOptions(),
			func(batch []*productmatch.Row) error {
				for _, row := range batch {
					res := idx.Match(row, productmatch.DefaultMatchOptions())
					id++
					o := ingest.RowOutcome{ID: id, SourceRow: row.Number, Outcome: ingest.OutcomeStaged,
						MatchLevel: string(res.Level), MatchScore: res.Score, DisplayName: row.DisplayName(),
						SourceCode: row.SKU, Payload: row, Candidates: res.Candidates, Issues: row.Issues}
					if res.ProductID > 0 {
						p := res.ProductID
						o.ProductID = &p
					}
					// The database round-trip.
					pj, _ := json.Marshal(o.Payload)
					cj, _ := json.Marshal(o.Candidates)
					o.Payload, o.Candidates = nil, nil
					if err := json.Unmarshal(pj, &o.Payload); err != nil {
						t.Errorf("%s row %d payload: %v", e.File, row.Number, err)
					}
					_ = json.Unmarshal(cj, &o.Candidates)
					outs = append(outs, &o)
				}
				return nil
			})
		_ = book.Close()
		sort.SliceStable(outs, func(i, j int) bool { return outs[i].MatchScore < outs[j].MatchScore })
		last := (len(outs) - 1) / 25
		for page := 0; page*25 < len(outs); page++ {
			if page > 2 && page != last {
				continue
			}
			end := min((page+1)*25, len(outs))
			view := pages.VendorImportView{Lang: "ar",
				Session: &ingest.Session{PublicID: "p", Phase: ingest.PhaseReview, TotalRows: len(outs)},
				Rows:    outs[page*25 : end], RowTotal: len(outs), Page: page + 1, PerPage: 25}
			func() {
				defer func() {
					if rec := recover(); rec != nil {
						t.Errorf("%s page %d panicked: %v", e.File, page+1, rec)
					}
				}()
				var b bytes.Buffer
				if err := pages.VendorIngestPage(view, "ar", "rtl").Render(context.Background(), &b); err != nil {
					t.Errorf("%s page %d: %v", e.File, page+1, err)
				}
				if !bytes.Contains(b.Bytes(), []byte("review-table")) {
					t.Errorf("%s page %d: no table in output", e.File, page+1)
				}
			}()
		}
	}
}
