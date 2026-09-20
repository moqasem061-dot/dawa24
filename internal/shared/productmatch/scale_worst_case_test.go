package productmatch

import (
	"fmt"
	"testing"
	"time"
)

// The other end of the scale test.
//
// TestImportOfTwentyFiveThousandRows builds 3,333 distinct brands for 20,000
// products, so every row's rarest word has about six postings and the candidate
// pool is six wide. It reports 18µs a row and it is not wrong — it is the best
// case, and it was the only case anybody had measured.
//
// A real Egyptian pharmaceutical catalogue is not like that. Brand families
// repeat, generic names share every word they have, and for those rows the pool
// fills to its limit. Measured on this machine the two ends are 228 times
// apart: 0.45 seconds against 82 seconds for the same twenty thousand rows.
//
// That spread is why the matching phase had to be chunked. A pass whose cost
// varies by two orders of magnitude with the shape of somebody's catalogue
// cannot be a single call that reports nothing until it returns: on the good
// data it looks instant and on the real data it looks hung.
//
// The gate here is about SHAPE, not speed. It is sized so a slow CI box passes
// comfortably and a change that makes the pool unbounded, or that reintroduces
// a per-row allocation inside the rating loop, does not.
func TestWorstCaseCandidatePoolStaysBounded(t *testing.T) {
	if testing.Short() {
		t.Skip("scale timing skipped in -short")
	}
	const products, rows = 8_000, 4_000

	forms := []string{"أقراص", "كبسول", "شراب", "حقن", "كريم"}
	strengths := []string{"5 مجم", "10 مجم", "20 مجم", "250 مجم", "500 مجم"}

	// Every product shares one identifying word, so no token can narrow the
	// pool and every row pays the limit.
	catalogue := make([]MasterProduct, 0, products)
	for i := 0; i < products; i++ {
		catalogue = append(catalogue, MasterProduct{
			ID:            int64(i + 1),
			NameAR:        fmt.Sprintf("فيتامين مركب %s %s", strengths[i%len(strengths)], forms[i%len(forms)]),
			DosageForm:    forms[i%len(forms)],
			Concentration: strengths[i%len(strengths)],
		})
	}
	idx := NewIndex(catalogue)

	file := make([]*Row, 0, rows)
	for i := 0; i < rows; i++ {
		file = append(file, &Row{
			Name:          fmt.Sprintf("فيتامين مركب %s %s", strengths[i%len(strengths)], forms[i%len(forms)]),
			Concentration: strengths[i%len(strengths)],
			DosageForm:    forms[i%len(forms)],
		})
	}

	start := time.Now()
	MatchAll(idx, file, DefaultMatchOptions(), 0)
	elapsed := time.Since(start)

	t.Logf("worst-case pool, %d rows against %d products: %s (%s/row)",
		rows, products, elapsed.Round(time.Millisecond),
		(elapsed / time.Duration(rows)).Round(time.Microsecond))

	if budget := 2 * time.Minute; elapsed > budget {
		t.Fatalf("worst-case matching took %s, over the %s budget — the candidate "+
			"pool is no longer bounded, or the rating loop has started allocating",
			elapsed.Round(time.Second), budget)
	}
}
