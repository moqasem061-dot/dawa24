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

// MatchRequest captures parameters for matching an unstandardized product name against candidate catalogs.
type MatchRequest struct {
	QueryName      string   `json:"query_name"`
	Candidates     []string `json:"candidates"`
	OrganizationID int64    `json:"-"`
	UserID         int64    `json:"-"`
	Feature        string   `json:"-"`
}

// MatchResponse represents the match outcome.
type MatchResponse struct {
	MatchedCandidate string  `json:"matched_candidate,omitempty"`
	ConfidenceScore  float64 `json:"confidence_score"`
	Source           string  `json:"source"` // "ai_gateway" or "deterministic_fallback"
}

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
