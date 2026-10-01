package matcheval

// Case represents an evaluation case in the corpus.
type Case struct {
	ID                  string   `json:"id"`
	Split               string   `json:"split"` // tuning | heldout | adversarial
	TemplateFamily      string   `json:"template_family,omitempty"`
	ExpressionFamily    string   `json:"expression_family,omitempty"`
	Workflow            []string `json:"workflow"`
	Language            string   `json:"language"` // ar | en | mixed
	Row                 CaseRow  `json:"row"`
	RequiredGranularity string   `json:"required_granularity"` // product | pack
	Label               Label    `json:"label"`
	CatalogSnapshot     string   `json:"catalog_snapshot,omitempty"`
}

// CaseRow defines the row data, mapping, and identifier declarations.
type CaseRow struct {
	Cells       map[string]string `json:"cells"`
	Mapping     map[string]int    `json:"mapping,omitempty"`
	Identifiers []CaseIdentifier  `json:"identifiers,omitempty"`
}

// CaseIdentifier declares a column's identifier namespace.
type CaseIdentifier struct {
	Column    string `json:"column"`
	Namespace string `json:"namespace"` // gtin | supplier_code | catalog_code
}

// Label specifies the ground-truth expectation for a case.
type Label struct {
	Resolvable  bool     `json:"resolvable"`
	Targets     []int64  `json:"targets"`
	Granularity string   `json:"granularity"` // product | pack
	Expect      string   `json:"expect"`      // accept | abstain | conflict | ambiguous
	Difficulty  []string `json:"difficulty,omitempty"`
	Labellers   int      `json:"labellers,omitempty"`
	Disputed    bool     `json:"disputed,omitempty"`
}

// Outcome represents the actual result produced by the engine on a case.
type Outcome struct {
	CaseID                 string   `json:"case_id"`
	State                  string   `json:"state"` // accepted | confirmed | needs_review | ambiguous | no_candidate | invalid_input | excluded | unavailable
	Basis                  string   `json:"basis,omitempty"`
	MatchedProductID       *int64   `json:"matched_product_id,omitempty"`
	AchievedGranularity    string   `json:"achieved_granularity,omitempty"`
	TopCandidates          []int64  `json:"top_candidates,omitempty"` // up to 20 for recall_at_20
	HardConflictViolations []string `json:"hard_conflict_violations,omitempty"`
	Path                   string   `json:"path,omitempty"` // identifier, exact, similarity, reviewer, ai
	Reasons                []string `json:"reasons,omitempty"`

	// AI specific diagnostics
	AIAttempts  int     `json:"ai_attempts,omitempty"`
	AITokens    int     `json:"ai_tokens,omitempty"`
	AICost      float64 `json:"ai_cost,omitempty"`
	AICostKnown bool    `json:"ai_cost_known,omitempty"`
}
