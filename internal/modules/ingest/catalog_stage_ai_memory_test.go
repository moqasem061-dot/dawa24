package ingest

import (
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/shared/matchflow"
)

// Only rows the Match Decision Memory (or a checked identifier) already
// answered stay away from the model; every name match, a full-score one
// included, is put to it.
func TestAnsweredBeforeAI(t *testing.T) {
	cases := map[matchflow.Basis]bool{
		matchflow.BasisKnowledgeHuman:    true,
		matchflow.BasisKnowledgeCurated:  true,
		matchflow.BasisKnowledgePlatform: true,
		matchflow.BasisReviewer:          true,
		matchflow.BasisIdentifier:        true,
		matchflow.BasisExact:             false,
		matchflow.BasisSimilarity:        false,
		matchflow.BasisAI:                false,
		"":                               false,
	}
	for basis, want := range cases {
		if got := answeredBeforeAI(basis); got != want {
			t.Errorf("answeredBeforeAI(%q) = %v, want %v", basis, got, want)
		}
	}
}
