package ingest

import (
	"context"
	"log/slog"
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/shared/matchflow"
	"github.com/moqasem061-dot/dawa24/internal/shared/productmatch"
)

type scoreMatcher struct {
	Matcher
	decisions []matchflow.RowDecision
}

func (m scoreMatcher) Prepare(context.Context, productmatch.PrepareRequest) (productmatch.PrepareResult, error) {
	return productmatch.PrepareResult{Decisions: m.decisions}, nil
}

func TestStagePreservesMatcherScores(t *testing.T) {
	for _, state := range []matchflow.State{matchflow.StateAccepted, matchflow.StateNeedsReview} {
		t.Run(string(state), func(t *testing.T) {
			id := int64(1)
			decision := matchflow.RowDecision{V: 1, State: state, Basis: matchflow.BasisSimilarity, ProductID: &id,
				Candidates: []matchflow.CandidateDecision{{ProductID: 1, Score: 0.72}, {ProductID: 2, Score: 0.41}}}
			store := &aiTrackingStore{}
			svc := NewService(nil, slog.Default())
			svc.SetImportStore(store)
			svc.SetMatcher(scoreMatcher{decisions: []matchflow.RowDecision{decision}})
			run := stagingRun{svc: svc, session: &Session{ID: 1, OrganizationID: 10},
				index:  productmatch.NewIndex([]productmatch.MasterProduct{{ID: 1, NameAR: "Panadol"}, {ID: 2, NameAR: "Concor"}}),
				byNorm: make(map[string]*openRow), bucketOf: make(map[int]matchBucket)}
			if err := run.stage(context.Background(), []*productmatch.Row{{Number: 1, Name: "Panadol"}}); err != nil {
				t.Fatal(err)
			}
			staged := store.stagedRows[0]
			if staged.MatchScore != 0.72 || staged.Candidates[0].Score != 0.72 || staged.Candidates[1].Score != 0.41 {
				t.Fatalf("scores changed: %+v", staged)
			}
			if state == matchflow.StateNeedsReview && staged.ProductID != nil {
				t.Fatal("suggestion was silently linked")
			}
		})
	}
}

func TestStoredDecisionRestoresOldCandidateScores(t *testing.T) {
	row := &RowOutcome{Candidates: []productmatch.MatchCandidate{{ProductID: 1, Score: 1}},
		MatchDecision: &matchflow.RowDecision{Candidates: []matchflow.CandidateDecision{{ProductID: 1, Score: 0.42}}}}
	row.RestoreDecisionScores()
	if row.Candidates[0].Score != 0.42 {
		t.Fatal("legacy inflated confidence was retained")
	}
	if row.ProductID != nil {
		t.Fatal("restoring a score linked an unsettled suggestion")
	}
}
