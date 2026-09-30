package matchflow_test

import (
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/shared/matchflow"
)

func TestColumns_Table(t *testing.T) {
	prodID := int64(12345)

	tests := []struct {
		name                 string
		decision             matchflow.RowDecision
		wantState            string
		wantBasis            string
		wantGranularity      string
		wantMatchedProductID *int64
		wantMatchLevel       string
		wantAIState          string
	}{
		{
			name: "accepted_identifier_gtin",
			decision: matchflow.RowDecision{
				State:     matchflow.StateAccepted,
				Basis:     matchflow.BasisIdentifier,
				ProductID: &prodID,
				Granularity: matchflow.GranularityPair{
					Required: matchflow.GranularityProduct,
					Achieved: matchflow.GranularityProduct,
				},
				Reasons: []matchflow.Reason{{Code: "identifier_gtin"}},
				AI:      matchflow.AIDecision{State: matchflow.AIStateOff},
			},
			wantState:            "accepted",
			wantBasis:            "identifier",
			wantGranularity:      "product",
			wantMatchedProductID: &prodID,
			wantMatchLevel:       "barcode",
			wantAIState:          "off",
		},
		{
			name: "accepted_identifier_supplier_code",
			decision: matchflow.RowDecision{
				State:     matchflow.StateAccepted,
				Basis:     matchflow.BasisIdentifier,
				ProductID: &prodID,
				Granularity: matchflow.GranularityPair{
					Required: matchflow.GranularityProduct,
					Achieved: matchflow.GranularityProduct,
				},
				Reasons: []matchflow.Reason{{Code: "identifier_supplier_code"}},
				AI:      matchflow.AIDecision{State: matchflow.AIStateOff},
			},
			wantState:            "accepted",
			wantBasis:            "identifier",
			wantGranularity:      "product",
			wantMatchedProductID: &prodID,
			wantMatchLevel:       "code",
			wantAIState:          "off",
		},
		{
			name: "accepted_exact",
			decision: matchflow.RowDecision{
				State:     matchflow.StateAccepted,
				Basis:     matchflow.BasisExact,
				ProductID: &prodID,
				Granularity: matchflow.GranularityPair{
					Required: matchflow.GranularityProduct,
					Achieved: matchflow.GranularityProduct,
				},
				AI: matchflow.AIDecision{State: matchflow.AIStateIneligible},
			},
			wantState:            "accepted",
			wantBasis:            "exact",
			wantGranularity:      "product",
			wantMatchedProductID: &prodID,
			wantMatchLevel:       "exact",
			wantAIState:          "ineligible",
		},
		{
			name: "accepted_similarity",
			decision: matchflow.RowDecision{
				State:     matchflow.StateAccepted,
				Basis:     matchflow.BasisSimilarity,
				ProductID: &prodID,
				Granularity: matchflow.GranularityPair{
					Required: matchflow.GranularityPack,
					Achieved: matchflow.GranularityPack,
				},
				AI: matchflow.AIDecision{State: matchflow.AIStateOff},
			},
			wantState:            "accepted",
			wantBasis:            "similarity",
			wantGranularity:      "pack",
			wantMatchedProductID: &prodID,
			wantMatchLevel:       "strong",
			wantAIState:          "off",
		},
		{
			name: "confirmed_reviewer",
			decision: matchflow.RowDecision{
				State:     matchflow.StateConfirmed,
				Basis:     matchflow.BasisReviewer,
				ProductID: &prodID,
				Granularity: matchflow.GranularityPair{
					Required: matchflow.GranularityProduct,
					Achieved: matchflow.GranularityProduct,
				},
				AI: matchflow.AIDecision{State: matchflow.AIStateCompleted},
			},
			wantState:            "confirmed",
			wantBasis:            "reviewer",
			wantGranularity:      "product",
			wantMatchedProductID: &prodID,
			wantMatchLevel:       "strong",
			wantAIState:          "completed",
		},
		{
			name: "needs_review_product_id_cleared_in_projection",
			decision: matchflow.RowDecision{
				State:     matchflow.StateNeedsReview,
				Basis:     matchflow.BasisSimilarity,
				ProductID: &prodID, // Suggested product in JSON
				Granularity: matchflow.GranularityPair{
					Required: matchflow.GranularityPack,
					Achieved: matchflow.GranularityProduct,
				},
				AI: matchflow.AIDecision{State: matchflow.AIStateQueued},
			},
			wantState:            "needs_review",
			wantBasis:            "similarity",
			wantGranularity:      "product",
			wantMatchedProductID: nil, // Must be nil for needs_review
			wantMatchLevel:       "review",
			wantAIState:          "queued",
		},
		{
			name: "ambiguous_product_id_cleared_in_projection",
			decision: matchflow.RowDecision{
				State:     matchflow.StateAmbiguous,
				Basis:     matchflow.BasisSimilarity,
				ProductID: &prodID,
				Granularity: matchflow.GranularityPair{
					Required: matchflow.GranularityProduct,
					Achieved: matchflow.GranularityNone,
				},
				AI: matchflow.AIDecision{State: matchflow.AIStateRunning},
			},
			wantState:            "ambiguous",
			wantBasis:            "similarity",
			wantGranularity:      "none",
			wantMatchedProductID: nil,
			wantMatchLevel:       "ambiguous",
			wantAIState:          "running",
		},
		{
			name: "no_candidate",
			decision: matchflow.RowDecision{
				State:     matchflow.StateNoCandidate,
				Basis:     matchflow.BasisSimilarity,
				ProductID: nil,
				Granularity: matchflow.GranularityPair{
					Required: matchflow.GranularityProduct,
					Achieved: matchflow.GranularityNone,
				},
				AI: matchflow.AIDecision{State: matchflow.AIStateIneligible},
			},
			wantState:            "no_candidate",
			wantBasis:            "similarity",
			wantGranularity:      "none",
			wantMatchedProductID: nil,
			wantMatchLevel:       "none",
			wantAIState:          "ineligible",
		},
		{
			name: "invalid_input",
			decision: matchflow.RowDecision{
				State: matchflow.StateInvalidInput,
				AI:    matchflow.AIDecision{State: matchflow.AIStateIneligible},
			},
			wantState:            "invalid_input",
			wantMatchedProductID: nil,
			wantMatchLevel:       "none",
			wantAIState:          "ineligible",
		},
		{
			name: "excluded",
			decision: matchflow.RowDecision{
				State: matchflow.StateExcluded,
				AI:    matchflow.AIDecision{State: matchflow.AIStateOff},
			},
			wantState:            "excluded",
			wantMatchedProductID: nil,
			wantMatchLevel:       "none",
			wantAIState:          "off",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			col := matchflow.Columns(tt.decision)
			if col.MatchState != tt.wantState {
				t.Errorf("MatchState = %q, want %q", col.MatchState, tt.wantState)
			}
			if tt.wantBasis != "" && col.MatchBasis != tt.wantBasis {
				t.Errorf("MatchBasis = %q, want %q", col.MatchBasis, tt.wantBasis)
			}
			if tt.wantGranularity != "" && col.MatchGranularity != tt.wantGranularity {
				t.Errorf("MatchGranularity = %q, want %q", col.MatchGranularity, tt.wantGranularity)
			}
			if tt.wantMatchedProductID == nil {
				if col.MatchedProductID != nil {
					t.Errorf("MatchedProductID = %v, want nil", *col.MatchedProductID)
				}
			} else {
				if col.MatchedProductID == nil || *col.MatchedProductID != *tt.wantMatchedProductID {
					t.Errorf("MatchedProductID = %v, want %v", col.MatchedProductID, *tt.wantMatchedProductID)
				}
			}
			if col.MatchLevel != tt.wantMatchLevel {
				t.Errorf("MatchLevel = %q, want %q", col.MatchLevel, tt.wantMatchLevel)
			}
			if tt.wantAIState != "" && col.AIState != tt.wantAIState {
				t.Errorf("AIState = %q, want %q", col.AIState, tt.wantAIState)
			}
		})
	}
}
