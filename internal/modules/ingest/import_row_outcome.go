package ingest

import (
	"github.com/moqasem061-dot/dawa24/internal/shared/matchflow"
	"github.com/moqasem061-dot/dawa24/internal/shared/productmatch"
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

	// Consumer row columns (Phase 4 / data-model §3.10)
	MatchState       string                 `json:"match_state,omitempty"`
	MatchBasis       string                 `json:"match_basis,omitempty"`
	MatchGranularity string                 `json:"match_granularity,omitempty"`
	MatchRevision    int                    `json:"match_revision"`
	HumanLocked      bool                   `json:"human_locked"`
	AIState          string                 `json:"ai_state,omitempty"`
	MatchRunID       *int64                 `json:"match_run_id,omitempty"`
	MatchDecision    *matchflow.RowDecision `json:"match_decision,omitempty"`
	BaselineDecision *matchflow.RowDecision `json:"baseline_decision,omitempty"`
	ConflictNotice   string                 `json:"conflict_notice,omitempty"`
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
	RowID      int64
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
	codeNS := productmatch.NamespaceSupplierCode
	if s.CodeNamespace == string(productmatch.NamespaceCatalogCode) || s.CodeIsCatalogCode {
		codeNS = productmatch.NamespaceCatalogCode
	}
	barcodeNS := productmatch.NamespaceGTIN
	if s.BarcodeNamespace != "" {
		barcodeNS = productmatch.Namespace(s.BarcodeNamespace)
	}

	return productmatch.IdentifierChoices{
		ByCode:            s.TrustSupplierCode,
		ByBarcode:         s.TrustBarcode,
		CodeIsCatalogCode: codeNS == productmatch.NamespaceCatalogCode,
		Namespaces: map[productmatch.Field]productmatch.Namespace{
			productmatch.FieldSKU:     codeNS,
			productmatch.FieldBarcode: barcodeNS,
		},
	}
}
