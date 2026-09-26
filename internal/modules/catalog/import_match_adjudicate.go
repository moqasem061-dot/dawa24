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
	"sync"

	"github.com/moqasem061-dot/dawa24/internal/shared/matchflow"
	"github.com/moqasem061-dot/dawa24/internal/shared/productmatch"
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
	if !ok || len(forAI) == 0 {
		return
	}

	// One question per distinct row text and shortlist: an administrator's
	// registry extract repeats a product across packs and batches, and each
	// repetition is the same question.
	byKey := make(map[string][]pendingMatch, len(forAI))
	questions := make([]matchflow.Question, 0, len(forAI))
	for _, p := range forAI {
		norm := matchRowNorm(prods[p.index])
		key := matchQuestionKey(norm, p.candidates)
		if _, seen := byKey[key]; !seen {
			questions = append(questions, adjudicationQuestion(index, prods[p.index], p, key))
		}
		byKey[key] = append(byKey[key], p)
	}

	var userID int64
	if session.CreatedBy != nil {
		userID = *session.CreatedBy
	}
	var (
		mu         sync.Mutex
		remembered []matchflow.Remembered
		done       int
	)
	run := matchflow.Run(ctx, s.adjudicator, matchflow.Spec{
		Profile:        matchflow.ProfileCatalog,
		Rows:           len(forAI),
		Questions:      questions,
		Feature:        matchflow.FeatureCatalogImport,
		OrganizationID: session.OrganizationID,
		UserID:         userID,
		Screen: func(key string, productID int64) bool {
			group := byKey[key]
			return len(group) > 0 &&
				index.IdentityConflict(matchRowFor(prods[group[0].index]), productID).None()
		},
		Describe: func(id int64) (matchflow.CatalogEntry, bool) {
			p, ok := index.Lookup(id)
			if !ok {
				return matchflow.CatalogEntry{}, false
			}
			return catalogEntryOf(p), true
		},
		OnAnswer: func(a matchflow.Answer) {
			mu.Lock()
			defer mu.Unlock()
			group := byKey[a.Key]
			if len(group) == 0 {
				return
			}
			res := MatchAdjudicationResult{ProductID: a.Decision.ProductID,
				Confidence: a.Decision.Confidence, Reason: a.Decision.Reason}
			lead := group[0]
			j := s.judgeMatch(index, prods[lead.index], lead, res)
			j.Offered = a.Offered
			j.Verified = a.Verified()
			verdict := matchflow.Verdict(j, res.ProductID)
			for _, p := range group {
				s.recordMatch(verdict, p, res, matches, stats, session)
			}
			if matchflow.Remember(j, res.ProductID) {
				remembered = append(remembered, matchflow.Remembered{
					Key:             a.Key,
					NormName:        matchRowNorm(prods[lead.index]),
					ChosenProductID: res.ProductID,
					Confidence:      res.Confidence,
					Reason:          res.Reason,
					PromptVersion:   matchflow.PromptVersion,
				})
			}
		},
		OnDone: func(keys []string) {
			if progress == nil {
				return
			}
			mu.Lock()
			for _, k := range keys {
				done += len(byKey[k])
			}
			processed := min(len(matches)+done, len(prods))
			mu.Unlock()
			progress.report(ImportPhaseMatching, processed, len(prods))
		},
	})

	stats.AIRequests += run.Requests + run.CheckRequests
	session.AICalls += run.Requests + run.CheckRequests
	stats.CeilingHit = stats.CeilingHit || run.CeilingHit
	if run.Failed > 0 {
		session.AIFallback = true
	}
	s.rememberMatchDecisions(ctx, remembered)
}

// adjudicationQuestion renders one pending row as a question: its text, the
// parsed hints the other importers already send, and its shortlist as the
// window. The hints used to be missing here, so the catalogue import was the
// one importer whose model had to re-derive strength and form from a joined
// string.
func adjudicationQuestion(index *productmatch.Index, product *Product, p pendingMatch,
	key string) matchflow.Question {

	row := matchRowFor(product)
	window := make([]matchflow.CatalogEntry, 0, len(p.candidates))
	for _, c := range p.candidates {
		if m, ok := index.Lookup(c.ProductID); ok {
			window = append(window, catalogEntryOf(m))
			continue
		}
		window = append(window, matchflow.CatalogEntry{
			ProductID: c.ProductID, NameAR: c.Name, Scientific: c.Scientific,
			DosageForm: c.DosageForm, Concentration: c.Concentration,
			Manufacturer: c.Manufacturer,
		})
	}
	return matchflow.Question{
		Key:    key,
		Window: window,
		Item: matchflow.Item{
			Text:         adjudicationText(product),
			Brand:        row.Name,
			Strength:     row.Concentration,
			DosageForm:   row.DosageForm,
			Manufacturer: row.Manufacturer,
			Scientific:   row.Scientific,
			SKU:          row.SKU,
			Barcode:      row.Barcode,
			CurrentGuess: p.guess,
			CurrentScore: p.score,
			Settled:      p.settled,
		},
		Risk: matchflow.Risk(p.settled, false, p.score),
	}
}

func catalogEntryOf(m *productmatch.MasterProduct) matchflow.CatalogEntry {
	return matchflow.CatalogEntry{
		ProductID:     m.ID,
		NameAR:        m.NameAR,
		NameEN:        m.NameEN,
		Scientific:    m.Scientific,
		DosageForm:    m.DosageForm,
		Concentration: m.Concentration,
		Manufacturer:  m.Manufacturer,
	}
}

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
