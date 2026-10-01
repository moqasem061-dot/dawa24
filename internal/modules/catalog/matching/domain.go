package matching

import (
	"errors"
	"time"

	"github.com/moqasem061-dot/dawa24/internal/shared/matchflow"
	"github.com/moqasem061-dot/dawa24/internal/shared/productmatch"
)

// ErrNotImplemented is returned for port methods not yet implemented in Phase 2D.
var ErrNotImplemented = errors.New("matching: method not implemented")

// ErrRevisionConflict is returned when a review write encounters an optimistic revision mismatch.
var ErrRevisionConflict = errors.New("matching: revision conflict")

// ErrHardConflict is returned when a reviewer attempts to select a candidate with unresolved hard conflicts.
var ErrHardConflict = errors.New("matching: candidate has unresolved hard conflicts")

// Workflow names the matching consumer workflow.
type Workflow = productmatch.Workflow

const (
	WorkflowVendor     = productmatch.WorkflowVendor
	WorkflowSavings    = productmatch.WorkflowSavings
	WorkflowSmartOrder = productmatch.WorkflowSmartOrder
	WorkflowCatalog    = productmatch.WorkflowCatalog
	WorkflowCompare    = productmatch.WorkflowCompare
)

// Scope defines the tenant, workflow, and actor boundary for a matching operation.
type Scope = productmatch.Scope

// PrepareRequest carries source rows into the deterministic preparation stage.
type PrepareRequest = productmatch.PrepareRequest

// PrepareResult holds the outcomes and summary of a Prepare operation.
type PrepareResult = productmatch.PrepareResult

// RowRef identifies an individual row across consumer tables.
type RowRef = productmatch.RowRef

// ReviewAction captures human review decisions.
type ReviewAction struct {
	RowRef      RowRef                 `json:"row_ref"`
	ExpectedRev int                    `json:"expected_rev"`
	Action      string                 `json:"action"` // select, correct, reject, exclude, undo
	TargetID    int64                  `json:"target_id"`
	Corrections []matchflow.Correction `json:"corrections,omitempty"`
	Reason      string                 `json:"reason,omitempty"`
	Row         productmatch.Row       `json:"row,omitempty"`
	CurrentDec  matchflow.RowDecision  `json:"current_dec,omitempty"`
}

// BulkPreview provides impact metrics for a review action across multiple rows.
type BulkPreview struct {
	Total    int `json:"total"`
	Affected int `json:"affected"`
}

// EnhanceRow pairs a row with its decision for AI enhancement.
type EnhanceRow struct {
	RowRef   RowRef
	Decision matchflow.RowDecision
	Row      productmatch.Row
}

// EnhanceHandle tracks an active asynchronous AI enhancement session.
type EnhanceHandle struct {
	RunID     int64     `json:"run_id"`
	StartedAt time.Time `json:"started_at"`
}

// KnowledgeWrite stores an association or rejection in match_knowledge.
type KnowledgeWrite struct {
	SourceNamespace   string `json:"source_namespace"`
	QuestionSignature string `json:"question_signature"`
	Kind              string `json:"kind"`
	TargetProductID   *int64 `json:"target_product_id,omitempty"`
	TargetGranularity string `json:"target_granularity,omitempty"`
	PolicyVersion     string `json:"policy_version"`
	NormalizerVersion string `json:"normalizer_version"`
	DisplayText       string `json:"display_text"`
}

// KnowledgeFilter specifies criteria for querying match knowledge.
type KnowledgeFilter struct {
	Namespace string `json:"namespace"`
	Search    string `json:"search"`
	Limit     int    `json:"limit"`
	Offset    int    `json:"offset"`
}

// Knowledge represents a persisted match knowledge record.
type Knowledge struct {
	ID                int64     `json:"id"`
	OrganizationID    *int64    `json:"organization_id,omitempty"`
	SourceNamespace   string    `json:"source_namespace"`
	QuestionSignature string    `json:"question_signature"`
	Kind              string    `json:"kind"`
	TargetProductID   *int64    `json:"target_product_id,omitempty"`
	TargetGranularity string    `json:"target_granularity,omitempty"`
	DisplayText       string    `json:"display_text"`
	Status            string    `json:"status"`
	CreatedAt         time.Time `json:"created_at"`
}

// Page represents a paginated list of items.
type Page[T any] struct {
	Items []T `json:"items"`
	Total int `json:"total"`
}

// MatchRun tracks a match execution run against the catalog.
type MatchRun struct {
	ID                int64               `json:"id"`
	OrganizationID    int64               `json:"organization_id"`
	Workflow          string              `json:"workflow"`
	EntryPoint        string              `json:"entry_point"`
	ConsumerRef       string              `json:"consumer_ref"`
	SourceRevision    int                 `json:"source_revision"`
	PolicyVersion     string              `json:"policy_version"`
	CatalogRevision   int64               `json:"catalog_revision"`
	NormalizerVersion string              `json:"normalizer_version"`
	State             string              `json:"state"`
	StageTimings      map[string]int64    `json:"stage_timings"`
	Counts            matchflow.RunCounts `json:"counts"`
	AI                map[string]any      `json:"ai"`
	HeartbeatAt       time.Time           `json:"heartbeat_at"`
	CreatedBy         string              `json:"created_by"`
	CreatedAt         time.Time           `json:"created_at"`
	FinishedAt        *time.Time          `json:"finished_at"`
}
