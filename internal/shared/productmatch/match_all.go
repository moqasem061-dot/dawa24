package productmatch

// Matching a whole file.
//
// Every importer had the same loop — take the rows, call Match on each, keep
// the result — and every one of them ran it on one core. That was defensible
// while the engine's cost was dominated by the database; it is not now. A
// twenty-five-thousand-row price list against a twenty-thousand-product
// catalogue is a few hundred million scored pairs, and it is pure CPU with no
// shared mutable state: the index is read-only once built, and every row's
// scratch space is created inside its own query.
//
// So the work divides perfectly, and the only reason it was not divided is that
// nobody had written the loop in one place.
//
// Determinism survives. Results are written back into a slice at the row's own
// index rather than appended as they finish, so the output is the input order
// whatever the scheduler does — which matters because the staging table, the
// review screen and the decision cache all key on it.

import (
	"runtime"
	"sync"
)

// MatchAll scores every row against the index, in parallel, preserving order.
//
// workers of 0 means one per core. One is the serial path, which is what a test
// wanting a repeatable profile asks for.
//
// The returned slice is parallel to rows: out[i] is the result for rows[i], and
// a nil row yields the zero MatchResult rather than a panic — a staging pass
// that dropped a row should not take the import down with it.
func MatchAll(idx *Index, rows []*Row, opts MatchOptions, workers int) []MatchResult {
	out := make([]MatchResult, len(rows))
	if idx == nil || len(rows) == 0 {
		return out
	}
	if workers <= 0 {
		workers = runtime.NumCPU()
	}
	if workers > len(rows) {
		workers = len(rows)
	}
	if workers <= 1 {
		for i, row := range rows {
			if row != nil {
				out[i] = idx.Match(row, opts)
			}
		}
		return out
	}

	// A shared cursor rather than fixed slices, because rows differ enormously
	// in cost: a row whose brand is rare scores a dozen candidates and a row
	// whose every word is common scores the pool limit. Fixed slices leave one
	// worker finishing a file the others finished a minute ago.
	//
	// Claimed in blocks, because a mutex per row on work this short costs more
	// than the row does.
	const block = 64
	var (
		wg     sync.WaitGroup
		cursor int
		mu     sync.Mutex
	)
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				mu.Lock()
				start := cursor
				cursor += block
				mu.Unlock()
				if start >= len(rows) {
					return
				}
				end := start + block
				if end > len(rows) {
					end = len(rows)
				}
				for i := start; i < end; i++ {
					if rows[i] != nil {
						out[i] = idx.Match(rows[i], opts)
					}
				}
			}
		}()
	}
	wg.Wait()
	return out
}

// RecallAll retrieves candidates for every row, in parallel, preserving order.
//
// The same argument as MatchAll and the same guarantees. It matters more here
// than it looks: retrieval reads three posting lists per row and scores a pool
// six hundred wide, so on a large file it costs about as much as matching does.
func RecallAll(idx *Index, rows []*Row, opts RecallOptions, workers int) [][]MatchCandidate {
	out := make([][]MatchCandidate, len(rows))
	if idx == nil || len(rows) == 0 {
		return out
	}
	// The retrieval indexes are built lazily behind a sync.Once. Touching them
	// once here means the workers do not all arrive at that Once together and
	// block on a build that one of them is doing.
	idx.recall()

	if workers <= 0 {
		workers = runtime.NumCPU()
	}
	if workers > len(rows) {
		workers = len(rows)
	}
	if workers <= 1 {
		for i, row := range rows {
			if row != nil {
				out[i] = idx.Recall(row, opts)
			}
		}
		return out
	}

	const block = 32
	var (
		wg     sync.WaitGroup
		cursor int
		mu     sync.Mutex
	)
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				mu.Lock()
				start := cursor
				cursor += block
				mu.Unlock()
				if start >= len(rows) {
					return
				}
				end := start + block
				if end > len(rows) {
					end = len(rows)
				}
				for i := start; i < end; i++ {
					if rows[i] != nil {
						out[i] = idx.Recall(rows[i], opts)
					}
				}
			}
		}()
	}
	wg.Wait()
	return out
}
