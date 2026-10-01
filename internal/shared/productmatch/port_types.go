package productmatch

import (
	"github.com/moqasem061-dot/dawa24/internal/shared/matchflow"
)

// Workflow names the matching consumer workflow.
type Workflow string

const (
	WorkflowVendor     Workflow = "vendor"
	WorkflowSavings    Workflow = "savings"
	WorkflowSmartOrder Workflow = "smartorder"
	WorkflowCatalog    Workflow = "catalog"
	WorkflowCompare    Workflow = "compare"
)

// Scope defines the tenant, workflow, and actor boundary for a matching operation.
type Scope struct {
	EffectiveOrg    int64    `json:"effective_org"`
	ActorUserID     int64    `json:"actor_user_id"`
	SourceNamespace string   `json:"source_namespace"`
	Workflow        Workflow `json:"workflow"`
	EntryPoint      string   `json:"entry_point"`
}

// PrepareRequest carries source rows into the deterministic preparation stage.
type PrepareRequest struct {
	Scope       Scope
	ConsumerRef string
	SourceRev   int
	Rows        []Row
	Required    Granularity
	Identifiers *IdentifierPolicy
}

// PrepareResult holds the outcomes and summary of a Prepare operation.
type PrepareResult struct {
	RunID     int64
	Decisions []matchflow.RowDecision
	Summary   matchflow.RunCounts
}

// RowRef identifies an individual row across consumer tables.
type RowRef struct {
	ID                 int64  `json:"id"`
	Table              string `json:"table"`
	RowNumber          int    `json:"row_number"`
	QuestionSignature  string `json:"question_signature,omitempty"`
	State              string `json:"state,omitempty"`
	ExplicitlyRejected bool   `json:"explicitly_rejected,omitempty"`
}
