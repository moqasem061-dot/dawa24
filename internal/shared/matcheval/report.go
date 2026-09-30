package matcheval

// GateResult represents the outcome of an evaluation gate.
type GateResult string

const (
	GatePass               GateResult = "pass"
	GateFail               GateResult = "fail"
	GateInsufficientSample GateResult = "insufficient_sample"
)

// Report contains full evaluation metrics across overall and subgroup cohorts.
type Report struct {
	Engine          string                 `json:"engine"`
	Corpus          string                 `json:"corpus"`
	Split           string                 `json:"split"`
	Policy          string                 `json:"policy,omitempty"`
	Overall         CohortStats            `json:"overall"`
	ByWorkflow      map[string]CohortStats `json:"by_workflow,omitempty"`
	ByLanguage      map[string]CohortStats `json:"by_language,omitempty"`
	ByDifficulty    map[string]CohortStats `json:"by_difficulty,omitempty"`
	ByIdentifier    map[string]CohortStats `json:"by_identifier,omitempty"`
	ByPackSensitive map[string]CohortStats `json:"by_pack_sensitive,omitempty"`
	Gates           map[string]GateResult  `json:"gates"`
}

// CohortStats details metric values for a specific cohort.
type CohortStats struct {
	NCases                      int                     `json:"n_cases"`
	NResolvable                 int                     `json:"n_resolvable"`
	NDisputedExcluded           int                     `json:"n_disputed_excluded"`
	Accepted                    int                     `json:"accepted"`
	AcceptedCorrect             int                     `json:"accepted_correct"`
	Precision                   float64                 `json:"precision"`
	PrecisionLB95               float64                 `json:"precision_lb95"`
	CoverageResolvable          float64                 `json:"coverage_resolvable"`
	CoverageAllRows             float64                 `json:"coverage_all_rows"`
	RecallAt20                  float64                 `json:"recall_at_20"`
	ReviewWorkload              float64                 `json:"review_workload"`
	HardConflictViolations      []HardConflictViolation `json:"hard_conflict_violations"`
	AbsentOrAmbiguousAccepted   int                     `json:"absent_or_ambiguous_accepted"`
	UnavailableVsNoneConfusions int                     `json:"unavailable_vs_none_confusions"`
	AI                          *AIStats                `json:"ai,omitempty"`
}

// HardConflictViolation records an acceptance that violated hard conflict rules.
type HardConflictViolation struct {
	CaseID    string   `json:"case_id"`
	Path      string   `json:"path"`
	Conflicts []string `json:"conflicts"`
}

// AIStats records the net contribution and costs of the AI stage.
type AIStats struct {
	GainNewCorrect      int            `json:"gain_new_correct"`
	NewWrong            int            `json:"new_wrong"`
	LostCorrect         int            `json:"lost_correct"`
	NetGainRate         float64        `json:"net_gain_rate"`
	AIAddedPrecisionLB95 float64       `json:"ai_added_precision_lb95"`
	Attempts            int            `json:"attempts"`
	Tokens              int            `json:"tokens"`
	Cost                float64        `json:"cost"`
	CostKnown           bool           `json:"cost_known"`
	CostPerVerifiedGain float64        `json:"cost_per_verified_gain"`
	ZeroGainReasons     map[string]int `json:"zero_gain_reasons,omitempty"`
}
