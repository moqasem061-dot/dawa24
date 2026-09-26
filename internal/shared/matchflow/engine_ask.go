package matchflow

// Asking the proposal question: one request, split when it fails, and
// re-asked for what the model skipped.

import (
	"context"
	"sync/atomic"
	"time"
)

// propose asks one batch, splitting it when it fails and re-asking what the
// model skipped. The result is keyed by ref.
func (r *runner) propose(ctx context.Context, batch Batch, depth int) map[int]Decision {
	out := make(map[int]Decision, len(batch.Items))
	if len(batch.Items) == 0 || ctx.Err() != nil {
		return out
	}

	ds, err := r.call(ctx, batch)
	if err != nil {
		if ctx.Err() != nil {
			return out
		}
		if len(batch.Items) >= splitMin && depth < maxDepth {
			// The commonest failures — a truncated answer, an upstream that
			// chokes on one malformed line, a request that ran long — all get
			// better with a smaller batch, and none gets better by asking the
			// same batch a third time.
			r.count(func(s *RunStats) { s.Split++ })
			half := len(batch.Items) / 2
			for ref, d := range r.propose(ctx, subBatch(batch, batch.Items[:half]), depth+1) {
				out[ref] = d
			}
			for ref, d := range r.propose(ctx, subBatch(batch, batch.Items[half:]), depth+1) {
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
			// Two answers for one ref: the first stands. A model that
			// answered twice is not more sure the second time.
			continue
		}
		if d.ProductID != nil && *d.ProductID <= 0 {
			d.ProductID = nil
		}
		// An id outside the window is kept as an answer: handle marks it
		// not-offered, Verdict refuses it, and the caller counts the refusal.
		out[d.Ref] = d
	}

	var skipped []Item
	for _, it := range batch.Items {
		if _, ok := out[it.Ref]; !ok {
			skipped = append(skipped, it)
		}
	}
	if len(skipped) > 0 {
		// Skipped items are re-asked once, and only when the model answered
		// part of the batch — the signature of an answer that ran out of room.
		// An empty result is not a partial skip: asking the same batch again
		// buys the same nothing.
		if depth == 0 && len(out) > 0 && ctx.Err() == nil {
			for ref, d := range r.propose(ctx, subBatch(batch, skipped), depth+1) {
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

// call makes one proposal request with its own timeout, retrying once.
func (r *runner) call(ctx context.Context, batch Batch) ([]Decision, error) {
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(attempt) * 1500 * time.Millisecond):
			}
		}
		rc, cancel := context.WithTimeout(ctx, requestTimeout)
		atomic.AddInt64(&r.requests, 1)
		ds, err := r.ai.Enhance(rc, batch)
		cancel()
		if err == nil {
			return ds, nil
		}
		lastErr = err
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
	}
	return nil, lastErr
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
