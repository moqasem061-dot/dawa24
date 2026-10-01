package matchflow_test

import (
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/shared/matchflow"
)

func TestEligible_Rules(t *testing.T) {
	baseReview := matchflow.RowDecision{
		State: matchflow.StateNeedsReview,
		Candidates: []matchflow.CandidateDecision{
			{ProductID: 101, Eligible: true},
		},
	}

	t.Run("EligibleCandidate", func(t *testing.T) {
		ok, reason := matchflow.Eligible(baseReview, false, false)
		if !ok || reason != matchflow.ReasonEligible {
			t.Fatalf("expected eligible, got ok=%v, reason=%v", ok, reason)
		}
	})

	t.Run("HumanLocked_Ineligible", func(t *testing.T) {
		ok, reason := matchflow.Eligible(baseReview, true, false)
		if ok || reason != matchflow.ReasonHumanLocked {
			t.Fatalf("expected ReasonHumanLocked, got ok=%v, reason=%v", ok, reason)
		}
	})

	t.Run("KnowledgeHit_Ineligible", func(t *testing.T) {
		ok, reason := matchflow.Eligible(baseReview, false, true)
		if ok || reason != matchflow.ReasonKnowledgeHit {
			t.Fatalf("expected ReasonKnowledgeHit, got ok=%v, reason=%v", ok, reason)
		}
	})

	t.Run("SettledAccepted_Ineligible", func(t *testing.T) {
		d := baseReview
		d.State = matchflow.StateAccepted
		ok, reason := matchflow.Eligible(d, false, false)
		if ok || reason != matchflow.ReasonSettled {
			t.Fatalf("expected ReasonSettled, got ok=%v, reason=%v", ok, reason)
		}
	})

	t.Run("SettledConfirmed_Ineligible", func(t *testing.T) {
		d := baseReview
		d.State = matchflow.StateConfirmed
		ok, reason := matchflow.Eligible(d, false, false)
		if ok || reason != matchflow.ReasonSettled {
			t.Fatalf("expected ReasonSettled, got ok=%v, reason=%v", ok, reason)
		}
	})

	t.Run("InvalidState_Ineligible", func(t *testing.T) {
		d := baseReview
		d.State = matchflow.StateExcluded
		ok, reason := matchflow.Eligible(d, false, false)
		if ok || reason != matchflow.ReasonStateIneligible {
			t.Fatalf("expected ReasonStateIneligible, got ok=%v, reason=%v", ok, reason)
		}
	})

	t.Run("NoCandidates_Ineligible", func(t *testing.T) {
		d := baseReview
		d.Candidates = nil
		ok, reason := matchflow.Eligible(d, false, false)
		if ok || reason != matchflow.ReasonNoEligibleCandidate {
			t.Fatalf("expected ReasonNoEligibleCandidate, got ok=%v, reason=%v", ok, reason)
		}
	})

	t.Run("UnstatedCandidate_Eligible", func(t *testing.T) {
		d := matchflow.RowDecision{
			State: matchflow.StateNoCandidate,
			Candidates: []matchflow.CandidateDecision{
				{
					ProductID: 102,
					Eligible:  false,
					Diff:      map[string]string{"strength": "unstated", "form": "agree"},
				},
			},
		}
		ok, reason := matchflow.Eligible(d, false, false)
		if !ok || reason != matchflow.ReasonEligible {
			t.Fatalf("expected eligible with unstated candidate, got ok=%v, reason=%v", ok, reason)
		}
	})

	t.Run("HardConflict_Ineligible", func(t *testing.T) {
		d := matchflow.RowDecision{
			State: matchflow.StateNeedsReview,
			Candidates: []matchflow.CandidateDecision{
				{
					ProductID: 103,
					Eligible:  false,
					Diff:      map[string]string{"strength": "conflict"},
					Reasons:   []string{"strength_conflict"},
				},
			},
		}
		ok, reason := matchflow.Eligible(d, false, false)
		if ok || reason != matchflow.ReasonHardConflict {
			t.Fatalf("expected ReasonHardConflict, got ok=%v, reason=%v", ok, reason)
		}
	})

	t.Run("HardConflict_ResolvedByCorrection", func(t *testing.T) {
		d := matchflow.RowDecision{
			State: matchflow.StateNeedsReview,
			Candidates: []matchflow.CandidateDecision{
				{
					ProductID: 103,
					Eligible:  false,
					Diff:      map[string]string{"strength": "conflict", "form": "unstated"},
					Reasons:   []string{"strength_conflict"},
				},
			},
			Actor: &matchflow.DecisionActor{
				Corrections: []matchflow.Correction{
					{Attribute: "strength", Value: "500mg"},
				},
			},
		}
		ok, reason := matchflow.Eligible(d, false, false)
		if !ok || reason != matchflow.ReasonEligible {
			t.Fatalf("expected resolved hard conflict to be eligible, got ok=%v, reason=%v", ok, reason)
		}
	})
}
