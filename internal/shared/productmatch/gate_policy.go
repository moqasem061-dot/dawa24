package productmatch

import "github.com/moqasem061-dot/dawa24/internal/shared/matchflow"

// Granularity indicates product vs pack specificity.
type Granularity uint8

const (
	GranularityNone Granularity = iota
	GranularityProduct
	GranularityPack
)

func (g Granularity) String() string {
	switch g {
	case GranularityNone:
		return "none"
	case GranularityProduct:
		return "product"
	case GranularityPack:
		return "pack"
	default:
		return "none"
	}
}

// MatchflowGranularity converts Granularity to matchflow.Granularity string enum.
func (g Granularity) MatchflowGranularity() matchflow.Granularity {
	switch g {
	case GranularityNone:
		return matchflow.GranularityNone
	case GranularityProduct:
		return matchflow.GranularityProduct
	case GranularityPack:
		return matchflow.GranularityPack
	default:
		return matchflow.GranularityNone
	}
}

// IdentifierPolicy configures declared namespaces per column and enabled lookup tiers.
type IdentifierPolicy struct {
	DeclaredNamespaces map[int]string // column index -> namespace
	AllowGTIN          bool
	AllowCatalogCode   bool
	AllowSupplierCode  bool
}

// Policy governs acceptance thresholds, required granularity, and identifier tiers.
type Policy struct {
	Version            string
	MinStrong          float64 // minimum score for strong acceptance
	MinReview          float64 // minimum score to be considered for review
	TieMargin          float64 // score margin to separate ambiguous siblings
	BlindNearIdentical float64 // threshold for letters-only / blind similarity
	Required           Granularity
	AllowOmittedAttrs  bool             // allow unstated attributes only when uniquely identified
	Identifiers        IdentifierPolicy // namespaces declared and user choices
}

// DefaultPolicy provides baseline configuration copying production numbers with identifier tiers off.
func DefaultPolicy() Policy {
	return Policy{
		Version:            "pm-default",
		MinStrong:          0.60,
		MinReview:          0.25,
		TieMargin:          0.06,
		BlindNearIdentical: 0.90,
		Required:           GranularityProduct,
		AllowOmittedAttrs:  true,
		Identifiers: IdentifierPolicy{
			DeclaredNamespaces: make(map[int]string),
			AllowGTIN:          false,
			AllowCatalogCode:   false,
			AllowSupplierCode:  false,
		},
	}
}
