package matching

import (
	"context"
	"runtime"
	"sync"

	"github.com/moqasem061-dot/dawa24/internal/shared/matchflow"
	"github.com/moqasem061-dot/dawa24/internal/shared/productmatch"
)

// decideQuestions answers every distinct question in a run.
//
// Knowledge for all of them is fetched in a few queries up front rather than
// two per question, and the deterministic decisions run on every core: a
// fifty-thousand-row file is tens of thousands of distinct questions, and one
// goroutine asking them one at a time with a database round trip each was the
// whole of a slow import.
func (s *serviceImpl) decideQuestions(
	ctx context.Context,
	sc Scope,
	sigs []string,
	rows []productmatch.Row,
	idx *productmatch.Index,
	pol productmatch.Policy,
	sourceRev int,
	catRev int64,
) ([]matchflow.RowDecision, int) {
	kr := NewKnowledgeResolver(s.repo)
	known, _ := kr.Fetch(ctx, sc, sigs) // a knowledge outage degrades to deterministic matching

	out := make([]matchflow.RowDecision, len(sigs))
	reused := make([]bool, len(sigs))
	workers := runtime.GOMAXPROCS(0)
	var wg sync.WaitGroup
	next := make(chan int, workers*2)
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range next {
				out[i], reused[i] = s.decideOne(sc, &rows[i], known[sigs[i]], kr, idx, pol, sourceRev, catRev)
			}
		}()
	}
	for i := range sigs {
		next <- i
	}
	close(next)
	wg.Wait()

	n := 0
	for _, r := range reused {
		if r {
			n++
		}
	}
	return out, n
}

// decideOne answers one question: remembered knowledge where it applies, the
// deterministic engine otherwise, and the rejections a person recorded in
// either case.
func (s *serviceImpl) decideOne(
	sc Scope,
	row *productmatch.Row,
	items []Knowledge,
	kr *KnowledgeResolver,
	idx *productmatch.Index,
	pol productmatch.Policy,
	sourceRev int,
	catRev int64,
) (matchflow.RowDecision, bool) {
	res := kr.Evaluate(sc, row, items, idx, pol, nil)
	if res.RejectAll {
		dec := rejectAllDecision(pol)
		dec.Revision = sourceRev
		dec.Versions.CatalogRevision = catRev
		return dec, true
	}
	if res.Decision != nil && res.Accepted {
		dec := *res.Decision
		dec.Revision = sourceRev
		dec.Versions.CatalogRevision = catRev
		return dec, true
	}

	dec := s.decideWithRecover(idx, row, pol, sourceRev, catRev)

	// A product a person rejected for this question is never applied to it,
	// whatever the engine thinks of it.
	if dec.ProductID != nil && containsID(res.Rejected, *dec.ProductID) {
		dec.State = matchflow.StateNeedsReview
		dec.Reasons = append([]matchflow.Reason{{Code: string(productmatch.ReasonKnowledgeRejected)}}, dec.Reasons...)
		dec.ProductID = nil
	}
	for i := range dec.Candidates {
		if containsID(res.Rejected, dec.Candidates[i].ProductID) {
			dec.Candidates[i].Eligible = false
			dec.Candidates[i].Reasons = append(dec.Candidates[i].Reasons, string(productmatch.ReasonKnowledgeRejected))
		}
	}

	// A stale or now-contradicted memory is kept as a suggestion only when the
	// engine could not settle the row itself.
	if res.Decision != nil && dec.State != matchflow.StateAccepted {
		stale := *res.Decision
		stale.Revision = sourceRev
		stale.Versions.CatalogRevision = catRev
		stale.Candidates = dec.Candidates
		if stale.State == matchflow.StateAccepted {
			stale.State = matchflow.StateNeedsReview
		}
		return stale, false
	}

	if res.Decision != nil {
		// The engine settled the row on its own evidence, and a memory for it
		// exists that no longer applies. Said so, so a reviewer can see the
		// remembered answer changed.
		dec.Flags = append(dec.Flags, "stale_knowledge")
	}
	if dec.ProductID != nil {
		if p, ok := idx.Lookup(*dec.ProductID); ok {
			dec.Versions.Target = ProductFactFingerprint(p)
		}
	}
	return dec, false
}
