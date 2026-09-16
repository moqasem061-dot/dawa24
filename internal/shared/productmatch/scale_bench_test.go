package productmatch

import (
	"testing"
	"time"
)

// The stated workload, timed.
//
// "Some vendors upload 25,000 products at once" is a requirement, and a
// requirement nobody measures is a hope. This is the whole file end to end —
// build the index once, then match every row — reported as wall clock and as
// rows per second, so a change to retrieval or scoring that doubles the cost
// shows up as a number rather than as a support ticket.
//
// It is a Test rather than a Benchmark because what matters is the total for
// one import, not the per-operation average: an import that takes four seconds
// and an import that takes four minutes are different products, and `go test
// -bench` reports neither.

// TestImportOfTwentyFiveThousandRows times the stated workload.
func TestImportOfTwentyFiveThousandRows(t *testing.T) {
	if testing.Short() {
		t.Skip("scale timing skipped in -short")
	}
	const (
		products = 20_000 // the live catalogue's size
		rows     = 25_000 // the largest file a vendor has uploaded
	)

	catalogue := syntheticCatalogue(products)
	start := time.Now()
	idx := NewIndex(catalogue)
	build := time.Since(start)

	file := syntheticRows(rows, syntheticCatalogue(products))
	opts := DefaultMatchOptions()

	start = time.Now()
	results := MatchAll(idx, file, opts, 0)
	match := time.Since(start)

	settled := 0
	for _, r := range results {
		if r.Level.Settled() {
			settled++
		}
	}

	t.Logf("catalogue %d products: index built in %s", products, build.Round(time.Millisecond))
	t.Logf("file %d rows: matched in %s (%s/row, %.0f rows/sec), settled %d%%",
		rows, match.Round(time.Millisecond),
		(match / time.Duration(rows)).Round(time.Microsecond),
		float64(rows)/match.Seconds(), settled*100/rows)

	// The gate is deliberately loose and is about SHAPE rather than speed: a
	// change that makes matching quadratic in the catalogue, or that reintroduces
	// a per-row query, blows through this by an order of magnitude on any
	// machine. A slow CI box does not.
	if budget := 5 * time.Minute; match > budget {
		t.Errorf("matching %d rows took %s, over the %s an import is allowed",
			rows, match, budget)
	}
}

// BenchmarkMatchAllParallel measures the per-row cost with every core working,
// which is how an import actually runs.
func BenchmarkMatchAllParallel(b *testing.B) {
	catalogue := syntheticCatalogue(20_000)
	idx := NewIndex(catalogue)
	file := syntheticRows(2_000, syntheticCatalogue(20_000))
	opts := DefaultMatchOptions()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = MatchAll(idx, file, opts, 0)
	}
	b.ReportMetric(float64(len(file)*b.N)/b.Elapsed().Seconds(), "rows/sec")
}

// BenchmarkMatchAllSerial is the same work on one core, for the comparison that
// says whether the parallelism is buying anything.
func BenchmarkMatchAllSerial(b *testing.B) {
	catalogue := syntheticCatalogue(20_000)
	idx := NewIndex(catalogue)
	file := syntheticRows(2_000, syntheticCatalogue(20_000))
	opts := DefaultMatchOptions()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = MatchAll(idx, file, opts, 1)
	}
	b.ReportMetric(float64(len(file)*b.N)/b.Elapsed().Seconds(), "rows/sec")
}
