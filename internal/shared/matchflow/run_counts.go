package matchflow

// RunCounts aggregates match state counts and metrics for a run (FR-094).
type RunCounts struct {
	Total             int            `json:"total"`
	Accepted          int            `json:"accepted"`
	Confirmed         int            `json:"confirmed"`
	NeedsReview       int            `json:"needs_review"`
	Ambiguous         int            `json:"ambiguous"`
	NoCandidate       int            `json:"no_candidate"`
	InvalidInput      int            `json:"invalid_input"`
	Excluded          int            `json:"excluded"`
	Unavailable       int            `json:"unavailable"`
	DistinctQuestions int            `json:"distinct_questions"`
	KnowledgeReuse    int            `json:"knowledge_reuse"`
	ByState           map[string]int `json:"by_state,omitempty"`
}

// PrimaryTotal returns the sum of all primary states, ensuring 1:1 reconciliation with source rows.
func (rc RunCounts) PrimaryTotal() int {
	return rc.Accepted + rc.Confirmed + rc.NeedsReview + rc.Ambiguous +
		rc.NoCandidate + rc.InvalidInput + rc.Excluded + rc.Unavailable
}

// AddState increments the appropriate state counter in RunCounts.
func (rc *RunCounts) AddState(s State) {
	rc.Total++
	switch s {
	case StateAccepted:
		rc.Accepted++
	case StateConfirmed:
		rc.Confirmed++
	case StateNeedsReview:
		rc.NeedsReview++
	case StateAmbiguous:
		rc.Ambiguous++
	case StateNoCandidate:
		rc.NoCandidate++
	case StateInvalidInput:
		rc.InvalidInput++
	case StateExcluded:
		rc.Excluded++
	case StateUnavailable:
		rc.Unavailable++
	}
}
