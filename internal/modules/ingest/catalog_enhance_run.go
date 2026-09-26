package ingest

// Driving the stage.
//
// catalog_enhance.go declares what the stage IS — its ports, its ceilings, its
// vocabulary and what it records. This file runs it: the cache first, then the
// planner, then as many concurrent requests as the profile allows, with every
// failure path degrading to the deterministic outcome.

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
	"github.com/moqasem061-dot/dawa24/internal/shared/i18n"
	"github.com/moqasem061-dot/dawa24/internal/shared/matchflow"
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

	for _, r := range rows {
		if r.settled {
			e.Stats.Verified += len(r.sourceRows)
		}
	}

	pending := e.applyCache(ctx, rows)
	if len(pending) == 0 {
		return collect(rows)
	}

	asked := byQuestion(pending)
	total := countRows(pending)
	orgID, userID := e.OrgID, e.UserID
	if orgID <= 0 {
		if actor, ok := authctx.From(ctx); ok {
			orgID, userID = actor.OrganizationID, actor.UserID
		} else if tid, ok := database.TenantFrom(ctx); ok && tid > 0 {
			orgID = tid
		}
	}

	var (
		saveMu sync.Mutex
		toSave []CachedDecision
	)
	stats := matchflow.Run(ctx, e.ai, matchflow.Spec{
		Profile:        matchflow.ProfileVendor,
		Rows:           countRows(rows),
		Questions:      e.questions(pending),
		Feature:        matchflow.FeatureVendorImport,
		OrganizationID: orgID,
		UserID:         userID,
		Screen: func(key string, productID int64) bool {
			r, ok := asked[key]
			return ok && e.index.IdentityConflict(r.row, productID).None()
		},
		Describe: func(id int64) (matchflow.CatalogEntry, bool) {
			_, ok := e.index.Lookup(id)
			return e.describe(id), ok
		},
		OnAnswer: func(a matchflow.Answer) {
			if saved, ok := e.apply(asked, a); ok {
				saveMu.Lock()
				toSave = append(toSave, saved)
				saveMu.Unlock()
			}
		},
		OnDone: func(keys []string) {
			n := 0
			for _, k := range keys {
				if r, ok := asked[k]; ok {
					n += len(r.sourceRows)
				}
			}
			e.report(n, total)
		},
	})

	e.mu.Lock()
	e.Stats.Engine = stats
	e.Stats.Requests = stats.Requests + stats.CheckRequests
	e.Stats.CeilingHit = e.Stats.CeilingHit || stats.CeilingHit
	e.mu.Unlock()

	e.flush(ctx, toSave)
	return collect(rows)
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
