package matching

import (
	"context"
	"errors"
	"time"

	"github.com/moqasem061-dot/dawa24/internal/shared/matchflow"
	"github.com/moqasem061-dot/dawa24/internal/shared/money"
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

// Applier is implemented by each consumer so the service can apply AI results without
// knowing the consumer's tables (matching-service-port.md).
type Applier interface {
	// ApplyAI applies an AI decision with a revision compare-and-set.
	// It returns false when the row has been modified (obsolete) or human-locked.
	ApplyAI(ctx context.Context, ref RowRef, expectedRev int, runID int64, d matchflow.RowDecision) (bool, error)
}

// AIAttemptRecord captures one AI matching attempt (data-model §3.8).
type AIAttemptRecord struct {
	RunID          int64         `json:"run_id"`
	OrganizationID int64         `json:"organization_id"`
	AttemptNo      int           `json:"attempt_no"`
	Kind           string        `json:"kind"`
	QuestionCount  int           `json:"question_count"`
	StartedAt      time.Time     `json:"started_at"`
	DeadlineAt     time.Time     `json:"deadline_at"`
	EndedAt        *time.Time    `json:"ended_at,omitempty"`
	Outcome        string        `json:"outcome"`
	InputTokens    int           `json:"input_tokens"`
	OutputTokens   int           `json:"output_tokens"`
	Cost           *money.Amount `json:"cost,omitempty"`
	RequestSHA256  string        `json:"request_sha256"`
	InvalidExcerpt *string       `json:"invalid_excerpt,omitempty"`
	Answers        []byte        `json:"answers"`
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
	Kind      string `json:"kind"`
	Status    string `json:"status"`
	Scope     string `json:"scope"`
	Search    string `json:"search"`
	Limit     int    `json:"limit"`
	Offset    int    `json:"offset"`
}

// Knowledge represents a persisted match knowledge record (data-model §3.4).
type Knowledge struct {
	ID                      int64      `json:"id"`
	OrganizationID          *int64     `json:"organization_id,omitempty"`
	SourceNamespace         string     `json:"source_namespace"`
	QuestionSignature       string     `json:"question_signature"`
	Kind                    string     `json:"kind"`
	TargetProductID         *int64     `json:"target_product_id,omitempty"`
	TargetGranularity       string     `json:"target_granularity,omitempty"`
	TargetFactFingerprint   string     `json:"target_fact_fingerprint,omitempty"`
	CandidateSetFingerprint *string    `json:"candidate_set_fingerprint,omitempty"`
	PolicyVersion           string     `json:"policy_version"`
	NormalizerVersion       string     `json:"normalizer_version"`
	AIConfigID              *string    `json:"ai_config_id,omitempty"`
	PromptVersion           *string    `json:"prompt_version,omitempty"`
	DisplayText             string     `json:"display_text"`
	Status                  string     `json:"status"`
	Legacy                  bool       `json:"legacy"`
	CreatedBy               string     `json:"created_by"`
	CreatedAt               time.Time  `json:"created_at"`
	RevokedBy               *string    `json:"revoked_by,omitempty"`
	RevokedAt               *time.Time `json:"revoked_at,omitempty"`
	LastUsedAt              *time.Time `json:"last_used_at,omitempty"`
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

// DiagnosticRowData captures row decision data for diagnostics (US12 T105).
type DiagnosticRowData struct {
	Index       int                   `json:"index"`
	DisplayName string                `json:"display_name"`
	State       string                `json:"state"`
	Basis       string                `json:"basis"`
	Revision    int                   `json:"revision"`
	ProductID   *int64                `json:"product_id,omitempty"`
	Decision    matchflow.RowDecision `json:"decision"`
}

// AIConfiguration represents a qualified AI model configuration (data-model §3.6, US10 T134).
type AIConfiguration struct {
	ID                     string       `json:"id"`
	GatewayRoute           string       `json:"gateway_route"`
	PromptVersion          string       `json:"prompt_version"`
	ContractVersion        string       `json:"contract_version"`
	NativeStructured       bool         `json:"native_structured"`
	BytesPerToken          float64      `json:"bytes_per_token"`
	MaxItemsPerAttempt     int          `json:"max_items_per_attempt"`
	MaxInputTokens         int          `json:"max_input_tokens"`
	MaxOutputTokens        int          `json:"max_output_tokens"`
	MaxCostPerAttempt      money.Amount `json:"max_cost_per_attempt"`
	QualifiedCohorts       []CohortSpec `json:"qualified_cohorts"`
	Status                 string       `json:"status"` // candidate | shadow | qualified | disabled
	QualificationReportURI *string      `json:"qualification_report_uri,omitempty"`
	CreatedBy              string       `json:"created_by"`
	CreatedAt              time.Time    `json:"created_at"`
	UpdatedBy              *string      `json:"updated_by,omitempty"`
	UpdatedAt              time.Time    `json:"updated_at"`
}

// CohortSpec identifies a workflow, language, and row type cohort (FR-068, US10 T137).
type CohortSpec struct {
	Workflow string `json:"workflow"`
	Language string `json:"language"`
	RowType  string `json:"row_type"`
}
