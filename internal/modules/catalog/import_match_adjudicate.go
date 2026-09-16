package catalog

// Asking the model about the rows the similarity tier settled, and the rows it
// could not.
//
// It runs last, in batches, and it applies only answers that name a candidate
// the model was actually shown. Everything it decides is filed in the shared
// decision cache, so an administrator re-uploading the same registry extract
// next month pays for the rows that changed and nothing else.
//
// What is new is the first population. The tier used to see only what
// similarity could not settle, so the rows it settled WRONGLY — the ones that
// overwrite a catalogue entry every pharmacy on the platform reads, at a score
// high enough that nobody looks — were the only rows in the file never examined
// twice. They are now checked, and a check the model will not confirm takes the
// row off the commit rather than replacing one machine's answer with another's.

import (
	"context"

	"github.com/muhiya/dawa24-store/internal/shared/matchflow"
	"github.com/muhiya/dawa24-store/internal/shared/productmatch"
)

// adjudicateMatches puts the pending rows to the model, in batches, and applies
// only the answers that survive every guard.
func (s *Service) adjudicateMatches(
	ctx context.Context,
	session *ImportSession,
	prods []*Product,
	matches map[int]ExistingMatch,
	forAI []pendingMatch,
	stats *MatchStats,
	progress ProgressFunc,
) {
	index, ok := s.catalogueIndex(ctx, session)
	if !ok {
		return
	}

	items := make([]MatchAdjudicationItem, 0, len(forAI))
	byRef := make(map[int64]pendingMatch, len(forAI))
	for _, p := range forAI {
		ref := int64(p.index)
		byRef[ref] = p
		items = append(items, MatchAdjudicationItem{
			Ref:          ref,
			Text:         adjudicationText(prods[p.index]),
			Candidates:   summarizeCandidates(index, p.candidates),
			Settled:      p.settled,
			CurrentGuess: p.guess,
			CurrentScore: p.score,
		})
	}

	decided := map[int64]bool{}
	var remembered []matchflow.Remembered
	ceilings := matchflow.Adaptive(matchflow.ProfileCatalog, len(items))
	batchSize := ceilings.MaxItemsPerRequest
	maxBatches := ceilings.MaxRequestsPerRun

	for start := 0; start < len(items); start += batchSize {
		if stats.AIRequests >= maxBatches {
			stats.CeilingHit = true
			break
		}
		end := start + batchSize
		if end > len(items) {
			end = len(items)
		}
		if progress != nil {
			progress.report(ImportPhaseMatching, end, len(items))
		}

		stats.AIRequests++
		session.AICalls++
		req := MatchAdjudicationRequest{
			Items:          items[start:end],
			OrganizationID: session.OrganizationID,
		}
		if session.CreatedBy != nil {
			req.UserID = *session.CreatedBy
		}
		results, err := s.adjudicator.AdjudicateMatches(ctx, req)
		if err != nil {
			// One failed batch is not a failed import: those rows keep the
			// deterministic outcome, which is "this is a new product" for an
			// unresolved row and the similarity match for a settled one.
			session.AIFallback = true
			s.log.WarnContext(ctx, "match adjudication batch failed; deterministic outcome stands",
				"session", session.PublicID, "items", end-start, "error", err)
			continue
		}
		for _, res := range results {
			p, known := byRef[res.Ref]
			if !known || decided[res.Ref] {
				continue
			}
			decided[res.Ref] = true

			j := s.judgeMatch(index, prods[p.index], p, res)
			s.recordMatch(matchflow.Verdict(j, res.ProductID), p, res, matches, stats, session)

			if matchflow.Remember(j, res.ProductID) {
				remembered = append(remembered, matchflow.Remembered{
					Key:             matchQuestionKey(matchRowNorm(prods[p.index]), p.candidates),
					NormName:        matchRowNorm(prods[p.index]),
					ChosenProductID: res.ProductID,
					Confidence:      res.Confidence,
					Reason:          res.Reason,
					PromptVersion:   matchflow.PromptVersion,
				})
			}
		}
	}
	s.rememberMatchDecisions(ctx, remembered)
}

// judgeMatch gathers what the shared rules need to decide one answer.
//
// The floor is this importer's own, and it is the highest on the platform —
// 0.90 against 0.80 — because a wrong match here overwrites the entry every
// pharmacy reads.
func (s *Service) judgeMatch(index *productmatch.Index, product *Product,
	p pendingMatch, res MatchAdjudicationResult) matchflow.Judgement {

	j := matchflow.Judgement{
		Settled:    p.settled,
		Current:    p.guess,
		Confidence: res.Confidence,
		Floor:      aiFloor,
	}
	if res.ProductID == nil || *res.ProductID <= 0 {
		return j
	}
	// A product that was not on THIS row's shortlist is refused, even though
	// the model may answer from the whole window. Widening what it can see is
	// not the same as widening what it can do to a shared catalogue.
	j.Offered = inShortlist(p.candidates, *res.ProductID)
	j.Conflicts = !index.IdentityConflict(matchRowFor(product), *res.ProductID).None()
	return j
}

// recordMatch writes one verdict onto the row that asked.
func (s *Service) recordMatch(verdict matchflow.Outcome, p pendingMatch,
	res MatchAdjudicationResult, matches map[int]ExistingMatch,
	stats *MatchStats, session *ImportSession) {

	switch verdict {
	case matchflow.OutcomeApply:
		matches[p.index] = ExistingMatch{ProductID: *res.ProductID, Reason: MatchAI}
		stats.AI++
		session.AIApplied++

	case matchflow.OutcomeReview:
		// The similarity tier applied a match and the model would not confirm
		// it. The product stays on the row so the administrator can see what
		// was nearly done to the shared catalogue; the row leaves the commit.
		if p.guess == nil {
			return
		}
		matches[p.index] = ExistingMatch{ProductID: *p.guess, Reason: MatchDisputed}
		stats.Disputed++
		if stats.Similar > 0 {
			stats.Similar--
		}
	}
}
