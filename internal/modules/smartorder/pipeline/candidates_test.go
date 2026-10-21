package pipeline

import (
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/modules/smartorder"
	"github.com/moqasem061-dot/dawa24/internal/shared/matchflow"
)

func TestMethodForDecision(t *testing.T) {
	tests := []struct {
		name string
		dec  matchflow.RowDecision
		cols matchflow.ColumnProjection
		want smartorder.MatchMethod
	}{
		{
			name: "AI decision maps to MethodAI",
			dec:  matchflow.RowDecision{Basis: matchflow.BasisAI},
			cols: matchflow.ColumnProjection{MatchLevel: "strong"},
			want: smartorder.MethodAI,
		},
		{
			name: "Human knowledge maps to MethodLearnedMapping",
			dec:  matchflow.RowDecision{Basis: matchflow.BasisKnowledgeHuman},
			cols: matchflow.ColumnProjection{MatchLevel: "strong"},
			want: smartorder.MethodLearnedMapping,
		},
		{
			name: "Exact match maps to MethodExactName",
			dec:  matchflow.RowDecision{Basis: matchflow.BasisExact},
			cols: matchflow.ColumnProjection{MatchLevel: "exact"},
			want: smartorder.MethodExactName,
		},
		{
			name: "Strong match maps to MethodIdentityKey",
			dec:  matchflow.RowDecision{Basis: matchflow.BasisSimilarity},
			cols: matchflow.ColumnProjection{MatchLevel: "strong"},
			want: smartorder.MethodIdentityKey,
		},
		{
			name: "Barcode match maps to MethodBarcode",
			dec:  matchflow.RowDecision{Basis: matchflow.BasisIdentifier},
			cols: matchflow.ColumnProjection{MatchLevel: "barcode"},
			want: smartorder.MethodBarcode,
		},
		{
			name: "Code match maps to MethodSKU",
			dec:  matchflow.RowDecision{Basis: matchflow.BasisIdentifier},
			cols: matchflow.ColumnProjection{MatchLevel: "code"},
			want: smartorder.MethodSKU,
		},
		{
			name: "Review or fallback maps to MethodFuzzy",
			dec:  matchflow.RowDecision{State: matchflow.StateNeedsReview},
			cols: matchflow.ColumnProjection{MatchLevel: "review"},
			want: smartorder.MethodFuzzy,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := methodForDecision(tt.dec, tt.cols)
			if got != tt.want {
				t.Errorf("methodForDecision() = %q, want %q", got, tt.want)
			}
			if !got.Valid() {
				t.Errorf("methodForDecision() returned invalid method %q (violates check constraint)", got)
			}
		})
	}
}
