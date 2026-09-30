package matchflow_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/moqasem061-dot/dawa24/internal/shared/matchflow"
)

func TestRowDecision_RoundTrip(t *testing.T) {
	prodID := int64(48213)
	relID := int64(99)
	now := time.Now().UTC().Truncate(time.Second)

	original := matchflow.RowDecision{
		State:     matchflow.StateNeedsReview,
		Basis:     matchflow.BasisSimilarity,
		ProductID: &prodID,
		Granularity: matchflow.GranularityPair{
			Required: matchflow.GranularityPack,
			Achieved: matchflow.GranularityProduct,
		},
		Pack: matchflow.PackDecision{
			Outcome:        matchflow.PackOutcomeNeedsRelationship,
			RelationshipID: &relID,
		},
		Candidates: []matchflow.CandidateDecision{
			{
				ProductID: 48213,
				Rank:      1,
				Eligible:  true,
				Diff:      map[string]string{"strength": "agree", "pack": "unstated"},
				Reasons:   []string{"similarity"},
			},
			{
				ProductID: 48214,
				Rank:      2,
				Eligible:  false,
				Diff:      map[string]string{"strength": "conflict"},
				Reasons:   []string{"strength_conflict"},
			},
		},
		Reasons: []matchflow.Reason{
			{Code: "pack_unresolved"},
		},
		Flags: []string{"retrieval_incomplete"},
		Versions: matchflow.Versions{
			Policy:          "pm-2026.10.1",
			Normalizer:      "n7",
			CatalogRevision: 91822,
		},
		Revision: 3,
		AI: matchflow.AIDecision{
			State:    matchflow.AIStateCompleted,
			Outcome:  "abstained",
			Attempts: []int64{9912},
		},
		Actor: &matchflow.DecisionActor{
			UserID: "usr_123",
			At:     now,
			Corrections: []matchflow.Correction{
				{Attribute: "strength", Value: "500 mg", Reason: "supplier typo"},
			},
		},
	}

	bytes, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	parsed, err := matchflow.ParseRowDecision(bytes)
	if err != nil {
		t.Fatalf("ParseRowDecision failed: %v", err)
	}

	if parsed.V != 1 {
		t.Errorf("expected V=1, got %d", parsed.V)
	}
	if parsed.State != original.State {
		t.Errorf("expected state %v, got %v", original.State, parsed.State)
	}
	if parsed.Basis != original.Basis {
		t.Errorf("expected basis %v, got %v", original.Basis, parsed.Basis)
	}
	if parsed.ProductID == nil || *parsed.ProductID != prodID {
		t.Errorf("expected product_id %d, got %v", prodID, parsed.ProductID)
	}
	if parsed.Granularity != original.Granularity {
		t.Errorf("expected granularity %+v, got %+v", original.Granularity, parsed.Granularity)
	}
	if parsed.Pack.Outcome != original.Pack.Outcome || *parsed.Pack.RelationshipID != relID {
		t.Errorf("expected pack %+v, got %+v", original.Pack, parsed.Pack)
	}
	if len(parsed.Candidates) != len(original.Candidates) {
		t.Errorf("expected %d candidates, got %d", len(original.Candidates), len(parsed.Candidates))
	}
	if len(parsed.Reasons) != len(original.Reasons) {
		t.Errorf("expected %d reasons, got %d", len(original.Reasons), len(parsed.Reasons))
	}
	if len(parsed.Flags) != len(original.Flags) {
		t.Errorf("expected %d flags, got %d", len(original.Flags), len(parsed.Flags))
	}
	if parsed.Versions != original.Versions {
		t.Errorf("expected versions %+v, got %+v", original.Versions, parsed.Versions)
	}
	if parsed.Revision != original.Revision {
		t.Errorf("expected revision %d, got %d", original.Revision, parsed.Revision)
	}
	if parsed.AI.State != original.AI.State || parsed.AI.Outcome != original.AI.Outcome {
		t.Errorf("expected AI %+v, got %+v", original.AI, parsed.AI)
	}
	if parsed.Actor == nil || parsed.Actor.UserID != "usr_123" {
		t.Errorf("expected actor %+v, got %+v", original.Actor, parsed.Actor)
	}
}

func TestParseRowDecision_ContractExample(t *testing.T) {
	contractJSON := []byte(`{
		"v": 1,
		"state": "needs_review",
		"basis": "similarity",
		"product_id": 48213,
		"granularity": {"required": "pack", "achieved": "product"},
		"pack": {"outcome": "unstated"},
		"candidates": [
			{"product_id": 48213, "rank": 1, "eligible": true,
			 "diff": {"strength": "agree", "form": "agree", "pack": "unstated"},
			 "reasons": ["similarity"]},
			{"product_id": 48214, "rank": 2, "eligible": false,
			 "diff": {"strength": "conflict"}, "reasons": ["strength_conflict"]}
		],
		"reasons": [{"code": "pack_unresolved"}],
		"flags": ["retrieval_incomplete"],
		"versions": {"policy": "pm-2026.10.1", "normalizer": "n7", "catalog_revision": 91822},
		"revision": 3,
		"ai": {"state": "completed", "outcome": "abstained", "attempts": [9912]},
		"actor": null
	}`)

	parsed, err := matchflow.ParseRowDecision(contractJSON)
	if err != nil {
		t.Fatalf("ParseRowDecision failed: %v", err)
	}

	if parsed.V != 1 {
		t.Errorf("expected V=1, got %d", parsed.V)
	}
	if parsed.State != matchflow.StateNeedsReview {
		t.Errorf("expected state needs_review, got %v", parsed.State)
	}
	if parsed.Actor != nil {
		t.Errorf("expected null actor, got %+v", parsed.Actor)
	}
}

func TestParseRowDecision_UnsupportedVersion(t *testing.T) {
	invalidVersionJSON := []byte(`{
		"v": 2,
		"state": "accepted",
		"basis": "exact"
	}`)

	_, err := matchflow.ParseRowDecision(invalidVersionJSON)
	if err == nil {
		t.Fatalf("expected error for V=2, got nil")
	}
}
