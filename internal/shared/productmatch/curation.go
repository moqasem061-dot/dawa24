package productmatch

import (
	"encoding/json"
	"strings"
	"time"
)

// CuratedFact represents an administrative override of a product identity fact.
type CuratedFact struct {
	ID        int64           `json:"id"`
	ProductID int64           `json:"product_id"`
	Attribute string          `json:"attribute"`
	Value     json.RawMessage `json:"value"`
	Reason    string          `json:"reason,omitempty"`
	CreatedBy string          `json:"created_by"`
	CreatedAt time.Time       `json:"created_at"`
	RevokedAt *time.Time      `json:"revoked_at,omitempty"`
	RevokedBy *string         `json:"revoked_by,omitempty"`
}

// PackRelationship represents an approved conversion relationship between packaging units.
type PackRelationship struct {
	ID        int64      `json:"id"`
	ProductID int64      `json:"product_id"`
	FromUnit  string     `json:"from_unit"`
	ToUnit    string     `json:"to_unit"`
	Factor    int        `json:"factor"`
	CreatedBy string     `json:"created_by"`
	CreatedAt time.Time  `json:"created_at"`
	RevokedAt *time.Time `json:"revoked_at,omitempty"`
	RevokedBy *string    `json:"revoked_by,omitempty"`
}

// ApplyCuration applies curated facts and pack relationships to a product's identity facts,
// replacing parsed attributes with Prov=corrected.
func ApplyCuration(facts *IdentityFacts, curations []CuratedFact, rels []PackRelationship) {
	if facts == nil {
		return
	}

	for _, c := range curations {
		switch strings.ToLower(c.Attribute) {
		case "form", "dosage_form":
			var formStr string
			if err := json.Unmarshal(c.Value, &formStr); err == nil && formStr != "" {
				facts.FormFamily = FormFamily(strings.ToLower(strings.TrimSpace(formStr)))
				facts.Prov = ProvCorrected
			}
		case "strength":
			var str string
			if err := json.Unmarshal(c.Value, &str); err == nil && str != "" {
				facts.Strengths = nil
				parseStrengthText(facts, str)
				for i := range facts.Strengths {
					facts.Strengths[i].Prov = ProvCorrected
					facts.Strengths[i].Cert = CertExplicit
				}
			}
		case "brand":
			var brandStr string
			if err := json.Unmarshal(c.Value, &brandStr); err == nil && brandStr != "" {
				facts.Brand = []Token{{Text: brandStr}}
			}
		case "route":
			var r string
			if err := json.Unmarshal(c.Value, &r); err == nil {
				facts.Route = r
			}
		case "release":
			var rel string
			if err := json.Unmarshal(c.Value, &rel); err == nil {
				facts.Release = rel
			}
		case "total_content":
			var q Quantity
			if err := json.Unmarshal(c.Value, &q); err == nil {
				q.Prov = ProvCorrected
				facts.TotalContent = &q
			}
		case "pack":
			var pf PackFacts
			if err := json.Unmarshal(c.Value, &pf); err == nil {
				pf.Prov = ProvCorrected
				facts.Pack = &pf
			} else {
				var count int
				if err := json.Unmarshal(c.Value, &count); err == nil {
					facts.Pack = &PackFacts{Count: count, Prov: ProvCorrected, Cert: CertExplicit}
				}
			}
		}
	}
}

// NewIndexWithCuration creates an Index with curated fact overrides and pack relationships applied.
func NewIndexWithCuration(
	products []MasterProduct,
	facts map[int64][]CuratedFact,
	rels map[int64][]PackRelationship,
) *Index {
	idx := NewIndex(products)
	if idx == nil {
		return nil
	}

	for _, p := range idx.products {
		if p == nil {
			continue
		}
		pCurations := facts[p.ID]
		pRels := rels[p.ID]
		if len(pCurations) > 0 || len(pRels) > 0 {
			f := p.Facts()
			ApplyCuration(&f, pCurations, pRels)
			p.curatedFacts = &f
			p.packRels = pRels
		}
	}

	return idx
}
