package matchflow

// ColumnProjection represents the flattened columns written alongside the JSON decision record.
type ColumnProjection struct {
	MatchState       string `json:"match_state"`
	MatchBasis       string `json:"match_basis"`
	MatchGranularity string `json:"match_granularity"`
	MatchedProductID *int64 `json:"matched_product_id"`
	MatchLevel       string `json:"match_level"`
	AIState          string `json:"ai_state"`
}

// Columns projects a RowDecision into its database column values.
func Columns(d RowDecision) ColumnProjection {
	var matchedProdID *int64
	if (d.State == StateAccepted || d.State == StateConfirmed) && d.ProductID != nil {
		id := *d.ProductID
		matchedProdID = &id
	}

	level := computeLegacyMatchLevel(d)

	return ColumnProjection{
		MatchState:       string(d.State),
		MatchBasis:       string(d.Basis),
		MatchGranularity: string(d.Granularity.Achieved),
		MatchedProductID: matchedProdID,
		MatchLevel:       level,
		AIState:          string(d.AI.State),
	}
}

func computeLegacyMatchLevel(d RowDecision) string {
	switch d.State {
	case StateAccepted, StateConfirmed:
		switch d.Basis {
		case BasisIdentifier:
			for _, r := range d.Reasons {
				if r.Code == "identifier_gtin" {
					return "barcode"
				}
				if r.Code == "identifier_supplier_code" || r.Code == "identifier_catalog_code" {
					return "code"
				}
			}
			return "barcode"
		case BasisExact:
			return "exact"
		case BasisSimilarity, BasisAI, BasisKnowledgeHuman, BasisKnowledgeCurated, BasisKnowledgePlatform, BasisReviewer:
			return "strong"
		default:
			return "strong"
		}
	case StateNeedsReview:
		return "review"
	case StateAmbiguous:
		return "ambiguous"
	default:
		return "none"
	}
}
