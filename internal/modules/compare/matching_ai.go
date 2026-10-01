package compare

// The AI stage for comparison files.
//
// The same stage every other importer runs — internal/shared/matchflow — so a
// comparison file gets the same proposal pass, the same pair check, the same
// identity guard and the same wall clock as a smart order or a vendor price
// list. It runs inside the detached staging pass (see RegisterAndStage), before
// the rows are written, so the rows are stored with their final match and no
// second write is needed.

import (
	"context"
	"math"
	"sync"

	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
	"github.com/moqasem061-dot/dawa24/internal/shared/matchflow"
	"github.com/moqasem061-dot/dawa24/internal/shared/productmatch"
)

var compareCeilings = matchflow.For(matchflow.ProfileSaving)

// compareVerifiable reports whether a settled result rests on a name, and is
// therefore worth a second opinion. Identifier tiers are the file's own claim
// and a model cannot improve on them.
func compareVerifiable(level productmatch.MatchLevel) bool {
	switch level {
	case productmatch.MatchBarcode, productmatch.MatchCode, productmatch.MatchExact:
		return false
	}
	return true
}

// compareQuestion is one distinct question and every row that asks it.
type compareQuestion struct {
	rows    []int
	row     *productmatch.Row
	settled bool
	current *int64
}

func (s *Service) refineWithAI(ctx context.Context, idx *productmatch.Index,
	rows []*CompareFileRow, matchRows []*productmatch.Row, results []productmatch.MatchResult) {

	recall := productmatch.DefaultRecallOptions()
	recall.Limit = compareCeilings.RecallLimit
	found := productmatch.RecallAll(idx, matchRows, recall, 0)

	asked := make(map[string]*compareQuestion)
	var questions []matchflow.Question
	for i, r := range rows {
		res := results[i]
		settled := res.Matched() && res.Level.Settled()
		if settled && !compareVerifiable(res.Level) {
			continue
		}
		var current *int64
		if settled {
			id := res.ProductID
			current = &id
		}

		window := make([]matchflow.CatalogEntry, 0, len(found[i])+1)
		ids := make([]int64, 0, len(found[i])+1)
		plausible := settled
		for _, c := range found[i] {
			if c.Score >= compareCeilings.MinPlausible {
				plausible = true
			}
			if e, ok := compareEntry(idx, c.ProductID); ok {
				window = append(window, e)
				ids = append(ids, c.ProductID)
			}
		}
		if !plausible {
			continue
		}
		if current != nil && !containsID(ids, *current) {
			if e, ok := compareEntry(idx, *current); ok {
				window = append(window, e)
				ids = append(ids, *current)
			}
		}
		if len(window) == 0 {
			continue
		}

		key := matchflow.DecisionKey(productmatch.NormalizeText(r.RawName), ids)
		if q, dup := asked[key]; dup {
			q.rows = append(q.rows, i)
			continue
		}
		asked[key] = &compareQuestion{rows: []int{i}, row: matchRows[i],
			settled: settled, current: current}
		questions = append(questions, matchflow.Question{
			Key:    key,
			Window: window,
			Item: matchflow.Item{
				Text:         r.RawName,
				Strength:     productmatch.InferConcentration(r.RawName),
				DosageForm:   productmatch.InferDosageForm(r.RawName),
				PackSize:     productmatch.InferPackSize(r.RawName),
				SKU:          r.SKU,
				CurrentGuess: current,
				CurrentScore: res.Score,
				Settled:      settled,
				Loose:        res.Loose,
			},
			Risk: matchflow.Risk(settled, res.Level == productmatch.MatchAmbiguous, res.Score),
		})
	}
	if len(questions) == 0 {
		return
	}

	orgID, userID := compareAttribution(ctx, rows)
	var mu sync.Mutex
	matchflow.Run(ctx, s.enhancer, matchflow.Spec{
		Profile:        matchflow.ProfileSaving,
		Rows:           len(rows),
		Questions:      questions,
		Feature:        matchflow.FeatureCompareTool,
		OrganizationID: orgID,
		UserID:         userID,
		Screen: func(key string, productID int64) bool {
			q, ok := asked[key]
			return ok && idx.IdentityConflict(q.row, productID).None()
		},
		Describe: func(id int64) (matchflow.CatalogEntry, bool) { return compareEntry(idx, id) },
		OnAnswer: func(a matchflow.Answer) {
			mu.Lock()
			defer mu.Unlock()
			q, ok := asked[a.Key]
			if !ok {
				return
			}
			d := a.Decision
			j := matchflow.Judgement{
				Settled:    q.settled,
				Current:    q.current,
				Offered:    a.Offered,
				Confidence: d.Confidence,
				Floor:      compareCeilings.MinApplyConfidence,
				Verified:   a.Verified(),
			}
			if d.ProductID != nil {
				j.Conflicts = !idx.IdentityConflict(q.row, *d.ProductID).None()
			}
			switch matchflow.Verdict(j, d.ProductID) {
			case matchflow.OutcomeApply:
				id := *d.ProductID
				for _, i := range q.rows {
					rows[i].MatchedProductID = &id
					rows[i].MatchConfidence = math.Round(d.Confidence * 100)
					rows[i].MatchMethod = MatchMethodAI
				}
			case matchflow.OutcomeReview:
				// Two methods disagree about a match the engine applied. A
				// comparison priced against the wrong product misleads more
				// than an unmatched row, which the user can correct.
				for _, i := range q.rows {
					rows[i].MatchedProductID = nil
					rows[i].MatchConfidence = 0
					rows[i].MatchMethod = MatchMethodUnmatched
				}
			}
		},
	})
}

func compareEntry(idx *productmatch.Index, id int64) (matchflow.CatalogEntry, bool) {
	p, ok := idx.Lookup(id)
	if !ok {
		return matchflow.CatalogEntry{}, false
	}
	return matchflow.CatalogEntry{
		ProductID: p.ID, NameAR: p.NameAR, NameEN: p.NameEN,
		Scientific: p.Scientific, DosageForm: p.DosageForm,
		Concentration: p.Concentration, Manufacturer: p.Manufacturer,
	}, true
}

// compareAttribution names who pays for the stage: the file's organisation,
// and the uploader where the context still carries them.
func compareAttribution(ctx context.Context, rows []*CompareFileRow) (int64, int64) {
	var orgID, userID int64
	if actor, ok := authctx.From(ctx); ok {
		orgID, userID = actor.OrganizationID, actor.UserID
	}
	if orgID <= 0 {
		for _, r := range rows {
			if r.OrganizationID != nil && *r.OrganizationID > 0 {
				orgID = *r.OrganizationID
				break
			}
		}
	}
	if orgID <= 0 {
		if tid, ok := database.TenantFrom(ctx); ok && tid > 0 {
			orgID = tid
		}
	}
	return orgID, userID
}

func containsID(ids []int64, id int64) bool {
	for _, v := range ids {
		if v == id {
			return true
		}
	}
	return false
}
