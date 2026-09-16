package ingest

// Driving the stage.
//
// catalog_enhance.go declares what the stage IS — its ports, its ceilings, its
// vocabulary and what it records. This file runs it: the cache first, then the
// planner, then as many concurrent requests as the profile allows, with every
// failure path degrading to the deterministic outcome.

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/matchflow"
)

// Run improves what it can and leaves the rest as the deterministic engine
// found it.
//
// It never returns an error. Every failure path — Gateway down, budget spent,
// malformed response — degrades to the deterministic outcome, because a vendor
// must be able to import their catalogue when the AI is unavailable.
func (e *Enhancement) Run(ctx context.Context, rows []*openRow) []AIMatch {
	if e.ai == nil || e.index == nil || len(rows) == 0 {
		return nil
	}
	e.Stats.Ran = true

	// The wall clock is enforced on the calls themselves, not merely checked
	// between them: a check in the dispatch loop passes for every batch and
	// then lets slow requests run past the limit unbounded.
	runCtx, cancel := context.WithDeadline(ctx, e.now().Add(ceilings.MaxWallClock))
	defer cancel()

	for _, r := range rows {
		if r.settled {
			e.Stats.Verified += len(r.sourceRows)
		}
	}

	// The cache answers first, so a remembered row never enters a request.
	pending := e.applyCache(ctx, rows)
	if len(pending) == 0 {
		return collect(rows)
	}

	asked := byQuestion(pending)
	adaptiveCeilings := matchflow.Adaptive(matchflow.ProfileVendor, len(rows))
	requests, ceilingHit := matchflow.Plan(e.questions(pending), adaptiveCeilings)
	if ceilingHit {
		e.Stats.CeilingHit = true
	}
	total := 0
	for _, req := range requests {
		total += rowsIn(req, asked)
	}

	var (
		wg     sync.WaitGroup
		slots  = make(chan struct{}, adaptiveCeilings.MaxConcurrent)
		saveMu sync.Mutex
		toSave []CachedDecision
	)
	for _, request := range requests {
		wg.Add(1)
		slots <- struct{}{}
		go func(req matchflow.Request) {
			defer wg.Done()
			defer func() { <-slots }()
			rows := rowsIn(req, asked)
			// A panic inside one batch must not take the vendor's whole import
			// down with it.
			defer func() {
				if rec := recover(); rec != nil {
					e.count(func(s *AIStats) { s.Rejected += rows })
				}
			}()
			// The batch is accounted whatever happens to it: a failed batch
			// still leaves its rows with their deterministic outcome, and a bar
			// that stalls on failure reads as a hung run.
			defer e.report(rows, total)

			atomic.AddInt64(&e.requests, 1)
			req.Batch.Feature = matchflow.FeatureVendorImport
			if actor, ok := authctx.From(runCtx); ok {
				if actor.OrgID > 0 {
					req.Batch.OrganizationID = actor.OrgID
				} else {
					req.Batch.OrganizationID = actor.OrganizationID
				}
				req.Batch.UserID = actor.UserID
			}
			if req.Batch.OrganizationID <= 0 {
				if tid, ok := database.TenantFrom(runCtx); ok && tid > 0 {
					req.Batch.OrganizationID = tid
				}
			}
			outcomes, err := e.ai.Enhance(runCtx, req.Batch)
			if err != nil {
				if errors.Is(err, context.DeadlineExceeded) {
					e.count(func(s *AIStats) { s.CeilingHit = true })
				}
				return
			}
			if saved := e.apply(req, asked, outcomes); len(saved) > 0 {
				saveMu.Lock()
				toSave = append(toSave, saved...)
				saveMu.Unlock()
			}
		}(request)
	}
	wg.Wait()

	e.Stats.Requests = int(atomic.LoadInt64(&e.requests))
	e.flush(ctx, toSave)
	return collect(rows)
}

// rowsIn counts the vendor's spreadsheet rows one request settles.
//
// It exceeds the item count whenever duplicates were collapsed, and it is what
// the progress bar counts — a bar measured in questions rather than rows would
// stop short of the total on any file that repeats a product.
func rowsIn(req matchflow.Request, asked map[string]*openRow) int {
	n := 0
	for _, key := range req.Keys {
		if r, ok := asked[key]; ok {
			n += len(r.sourceRows)
		}
	}
	return n
}

// collect spreads each answer over every staged row that asked for it.
func collect(rows []*openRow) []AIMatch {
	out := make([]AIMatch, 0, len(rows))
	for _, r := range rows {
		switch {
		case r.answer != nil:
			for _, n := range r.sourceRows {
				out = append(out, AIMatch{
					SourceRow: n,
					ProductID: r.answer.ProductID,
					Score:     r.answer.Score,
					Reason:    r.answer.Reason,
					Level:     aiLevelAccepted,
				})
			}
		case r.disputed && r.guess != nil:
			for _, n := range r.sourceRows {
				out = append(out, AIMatch{
					SourceRow: n,
					ProductID: *r.guess,
					Score:     r.score,
					Reason:    i18n.TDefault("ingest.ai_dispute"),
					Level:     aiLevelDisputed,
				})
			}
		}
	}
	return out
}

// report advances the progress callback by one batch.
func (e *Enhancement) report(n, total int) {
	if e.OnProgress == nil {
		return
	}
	e.OnProgress(int(atomic.AddInt64(&e.settled, int64(n))), total)
}

// count mutates the stats under the same lock apply uses.
func (e *Enhancement) count(fn func(*AIStats)) {
	e.mu.Lock()
	defer e.mu.Unlock()
	fn(&e.Stats)
}
