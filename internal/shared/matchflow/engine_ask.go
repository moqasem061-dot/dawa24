package matchflow

// Asking the proposal question: one request, split when it fails, and
// re-asked for what the model skipped, strictly bounded by attempt counts and deadlines.

import (
	"context"
	"sync/atomic"
	"time"
)

// propose asks one batch, splitting it when it fails and re-asking what the
// model skipped. Each question's attempts are counted and bounded by maxAttemptsPerQuestion (3).
func (r *runner) propose(ctx context.Context, batch Batch, keys map[int]string, depth int) map[int]Decision {
	out := make(map[int]Decision, len(batch.Items))
	if len(batch.Items) == 0 || ctx.Err() != nil {
		return out
	}

	var eligible []Item
	for _, it := range batch.Items {
		k := keys[it.Ref]
		if r.recordAttempt(k) {
			eligible = append(eligible, it)
		}
	}
	if len(eligible) == 0 {
		return out
	}
	batch.Items = eligible

	ds, err := r.call(ctx, batch)
	if err != nil {
		if ctx.Err() != nil {
			return out
		}
		if len(batch.Items) >= splitMin && depth < maxDepth {
			r.count(func(s *RunStats) { s.Split++ })
			half := len(batch.Items) / 2
			for ref, d := range r.propose(ctx, subBatch(batch, batch.Items[:half]), keys, depth+1) {
				out[ref] = d
			}
			for ref, d := range r.propose(ctx, subBatch(batch, batch.Items[half:]), keys, depth+1) {
				out[ref] = d
			}
			return out
		}
		r.count(func(s *RunStats) { s.Failed++ })
		return out
	}

	valid := make(map[int]bool, len(batch.Items))
	for _, it := range batch.Items {
		valid[it.Ref] = true
	}
	for _, d := range ds {
		if !valid[d.Ref] {
			continue
		}
		if _, dup := out[d.Ref]; dup {
			// Duplicate ref: first stands.
			continue
		}
		if d.ProductID != nil && *d.ProductID <= 0 {
			d.ProductID = nil
		}
		out[d.Ref] = d
	}

	var skipped []Item
	for _, it := range batch.Items {
		if _, ok := out[it.Ref]; !ok {
			skipped = append(skipped, it)
		}
	}
	if len(skipped) > 0 {
		if depth == 0 && len(out) > 0 && ctx.Err() == nil {
			for ref, d := range r.propose(ctx, subBatch(batch, skipped), keys, depth+1) {
				out[ref] = d
			}
		}
		n := 0
		for _, it := range skipped {
			if _, ok := out[it.Ref]; !ok {
				n++
			}
		}
		if n > 0 && depth == 0 {
			r.count(func(s *RunStats) { s.Missing += n })
		}
	}
	return out
}

// call makes one proposal request with timeout calculated by AttemptDeadline.
func (r *runner) call(ctx context.Context, batch Batch) ([]Decision, error) {
	rem := r.runDeadline - time.Since(r.origin)
	to := AttemptDeadline(rem)
	if to <= 0 {
		return nil, context.DeadlineExceeded
	}

	rc, cancel := context.WithTimeout(ctx, to)
	defer cancel()
	atomic.AddInt64(&r.requests, 1)
	return r.ai.Enhance(rc, batch)
}

// subBatch is a batch restricted to some of its items, carrying only the
// catalogue rows those items were offered.
func subBatch(b Batch, items []Item) Batch {
	need := make(map[int64]bool)
	for _, it := range items {
		for _, id := range it.Options {
			need[id] = true
		}
		if it.CurrentGuess != nil {
			need[*it.CurrentGuess] = true
		}
	}
	out := Batch{
		Items:          append([]Item(nil), items...),
		Feature:        b.Feature,
		OrganizationID: b.OrganizationID,
		UserID:         b.UserID,
	}
	for _, e := range b.Catalog {
		if need[e.ProductID] {
			out.Catalog = append(out.Catalog, e)
		}
	}
	return out
}
