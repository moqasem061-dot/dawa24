package matchflow

// Asking the pair question for the proposals that would change a row.

import (
	"context"
	"sort"
	"sync/atomic"
	"time"
)

// verify runs the pair checks for one request's proposals. The result is
// keyed by index into checks.
func (r *runner) verify(ctx context.Context, batch Batch, checks []pending) map[int]Check {
	out := make(map[int]Check, len(checks))
	r.count(func(s *RunStats) { s.Checked += len(checks) })

	for start := 0; start < len(checks); start += checkChunk {
		end := min(start+checkChunk, len(checks))
		idx := make([]int, 0, end-start)
		for i := start; i < end; i++ {
			idx = append(idx, i)
		}
		for i, c := range r.verifyChunk(ctx, batch, checks, idx, 0) {
			out[i] = c
		}
	}
	return out
}

func (r *runner) verifyChunk(ctx context.Context, batch Batch, checks []pending,
	idx []int, depth int) map[int]Check {

	out := make(map[int]Check, len(idx))
	if len(idx) == 0 || ctx.Err() != nil {
		return out
	}
	cb := CheckBatch{
		Feature:        batch.Feature,
		OrganizationID: batch.OrganizationID,
		UserID:         batch.UserID,
	}
	refTo := make(map[int]int, len(idx))
	for _, i := range idx {
		p := checks[i]
		product, ok := r.describe(batch, p.target)
		if !ok {
			continue
		}
		ref := len(cb.Pairs) + 1
		refTo[ref] = i
		item := p.item
		item.CurrentGuess, item.CurrentScore, item.Options, item.Settled = nil, 0, nil, false
		item.Ref = ref
		cb.Pairs = append(cb.Pairs, Pair{Ref: ref, Item: item, Product: product})
	}
	if len(cb.Pairs) == 0 {
		return out
	}

	rem := r.runDeadline - time.Since(r.origin)
	to := AttemptDeadline(rem)
	if to <= 0 {
		return out
	}

	rc, cancel := context.WithTimeout(ctx, to)
	defer cancel()
	atomic.AddInt64(&r.checkRequests, 1)
	got, err := r.verifier.Verify(rc, cb)

	if err != nil {
		if ctx.Err() == nil && len(idx) >= splitMin && depth < maxDepth {
			half := len(idx) / 2
			for i, c := range r.verifyChunk(ctx, batch, checks, idx[:half], depth+1) {
				out[i] = c
			}
			for i, c := range r.verifyChunk(ctx, batch, checks, idx[half:], depth+1) {
				out[i] = c
			}
		}
		return out
	}
	for _, c := range got {
		i, ok := refTo[c.Ref]
		if !ok {
			continue
		}
		if _, dup := out[i]; dup {
			continue
		}
		out[i] = normalizeCheck(c)
	}
	return out
}

func (r *runner) describe(batch Batch, id int64) (CatalogEntry, bool) {
	for _, e := range batch.Catalog {
		if e.ProductID == id {
			return e, true
		}
	}
	if e, ok := r.catalog[id]; ok {
		return e, true
	}
	if r.spec.Describe != nil {
		return r.spec.Describe(id)
	}
	return CatalogEntry{}, false
}

func sortedRefs(m map[int]string) []int {
	refs := make([]int, 0, len(m))
	for ref := range m {
		refs = append(refs, ref)
	}
	sort.Ints(refs)
	return refs
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
