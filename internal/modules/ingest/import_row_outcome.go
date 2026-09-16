package ingest

import (
	"github.com/muhiya/dawa24-store/internal/shared/productmatch"
)

// RowOutcome is what the run did with one spreadsheet row.
type RowOutcome struct {
	ID                 int64                         `json:"id"`
	SourceRow          int                           `json:"source_row"`
	Outcome            string                        `json:"outcome"`
	MatchLevel         string                        `json:"match_level"`
	MatchScore         float64                       `json:"match_score"`
	ProductID          *int64                        `json:"product_id,omitempty"`
	MatchedProductName string                        `json:"matched_product_name,omitempty"`
	MatchedProductSKU  string                        `json:"matched_product_sku,omitempty"`
	VariantID          *int64                        `json:"variant_id,omitempty"`
	DisplayName        string                        `json:"display_name"`
	SourceCode         string                        `json:"source_code"`
	CustomVariantName  string                        `json:"custom_variant_name,omitempty"`
	IsExcluded         bool                          `json:"is_excluded"`
	IsManuallyMatched  bool                          `json:"is_manually_matched"`
	InWarehouse        bool                          `json:"in_warehouse"`
	Payload            *productmatch.Row             `json:"payload,omitempty"`
	Candidates         []productmatch.MatchCandidate `json:"candidates,omitempty"`
	Issues             []productmatch.Issue          `json:"issues,omitempty"`
	Message            string                        `json:"message"`
}

// EffectiveVariantName returns the custom variant name if set, or original row name.
func (r *RowOutcome) EffectiveVariantName() string {
	if r.CustomVariantName != "" {
		return r.CustomVariantName
	}
	if r.Payload != nil && r.Payload.Name != "" {
		return r.Payload.Name
	}
	return r.DisplayName
}

// MatchedCatalogName returns the name of the matched master product, or top candidate.
func (r *RowOutcome) MatchedCatalogName() string {
	if r.MatchedProductName != "" {
		return r.MatchedProductName
	}
	if len(r.Candidates) > 0 && r.Candidates[0].Name != "" {
		return r.Candidates[0].Name
	}
	return ""
}

// MatchedCatalogSKU returns the SKU/barcode of the matched master product.
func (r *RowOutcome) MatchedCatalogSKU() string {
	if r.MatchedProductSKU != "" {
		return r.MatchedProductSKU
	}
	return ""
}

// Outcome values recorded against a row.
const (
	OutcomeStaged   = "staged"
	OutcomeInserted = "inserted"
	OutcomeUpdated  = "updated"
	OutcomeSkipped  = "skipped"
	OutcomeError    = "error"
)

// RowFilter narrows the results table.
type RowFilter struct {
	Outcome    string
	MatchLevel string
	Search     string
	SortBy     string
	SortOrder  string
	Limit      int
	Offset     int
}

// identifierChoices is what the vendor switched on, in the shared engine's
// vocabulary. The engine drops any choice whose column was never mapped.
func (s Settings) identifierChoices() productmatch.IdentifierChoices {
	return productmatch.IdentifierChoices{
		ByCode:            s.TrustSupplierCode,
		ByBarcode:         s.TrustBarcode,
		CodeIsCatalogCode: s.CodeIsCatalogCode,
	}
}
