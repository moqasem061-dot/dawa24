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

// MarshalJSON serializes Granularity as a string.
func (g Granularity) MarshalJSON() ([]byte, error) {
	return []byte(`"` + g.String() + `"`), nil
}

// UnmarshalJSON deserializes Granularity from string or int.
func (g *Granularity) UnmarshalJSON(b []byte) error {
	s := string(b)
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		s = s[1 : len(s)-1]
	}
	switch s {
	case "product", "1":
		*g = GranularityProduct
	case "pack", "2":
		*g = GranularityPack
	default:
		*g = GranularityNone
	}
	return nil
}

// IdentifierPolicy configures declared namespaces per column and enabled lookup tiers.
type IdentifierPolicy struct {
	DeclaredNamespaces map[int]string `json:"declared_namespaces,omitempty"` // column index -> namespace
	AllowGTIN          bool           `json:"allow_gtin"`
	AllowCatalogCode   bool           `json:"allow_catalog_code"`
	AllowSupplierCode  bool           `json:"allow_supplier_code"`
}

// Policy governs acceptance thresholds, required granularity, and identifier tiers.
type Policy struct {
	Version            string           `json:"version"`
	MinStrong          float64          `json:"min_strong"`           // minimum score for strong acceptance
	MinReview          float64          `json:"min_review"`           // minimum score to be considered for review
	TieMargin          float64          `json:"tie_margin"`           // score margin to separate ambiguous siblings
	BlindNearIdentical float64          `json:"blind_near_identical"` // threshold for letters-only / blind similarity
	Required           Granularity      `json:"required"`
	AllowOmittedAttrs  bool             `json:"allow_omitted_attrs"` // allow unstated attributes only when uniquely identified
	Identifiers        IdentifierPolicy `json:"identifiers"`         // namespaces declared and user choices
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
