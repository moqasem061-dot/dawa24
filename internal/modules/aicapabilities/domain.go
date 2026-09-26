// Package aicapabilities provides AI-augmented intelligence (column detection, product matching,
// search query expansion) through the platform gateway with strict deterministic fallbacks.
package aicapabilities

// Capability names registered in the platform gateway.
const (
	CapImportDetectColumns = "import.detect_columns"
	CapProductMatch        = "product.match"
	CapSearchExpand        = "search.expand"
	CapSupportClassify     = "support.classify"
)

// QueryExpansionRequest represents a customer search phrase to expand with pharmaceutical synonyms.
type QueryExpansionRequest struct {
	Query string `json:"query"`
}

// QueryExpansionResponse represents expanded search terms.
type QueryExpansionResponse struct {
	OriginalTerms string   `json:"original_terms"`
	ExpandedTerms []string `json:"expanded_terms"`
	Source        string   `json:"source"`
}
