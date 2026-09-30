package matchflow

import (
	"encoding/json"
	"fmt"
	"time"
)

// State represents the primary match verdict for a row.
type State string

const (
	StateAccepted     State = "accepted"
	StateConfirmed    State = "confirmed"
	StateNeedsReview  State = "needs_review"
	StateAmbiguous    State = "ambiguous"
	StateNoCandidate  State = "no_candidate"
	StateInvalidInput State = "invalid_input"
	StateExcluded     State = "excluded"
	StateUnavailable  State = "unavailable"
)

// Basis identifies the path that established the candidate association.
type Basis string

const (
	BasisIdentifier        Basis = "identifier"
	BasisKnowledgeHuman    Basis = "knowledge_human"
	BasisKnowledgeCurated  Basis = "knowledge_curated"
	BasisKnowledgePlatform Basis = "knowledge_platform"
	BasisExact             Basis = "exact"
	BasisSimilarity        Basis = "similarity"
	BasisAI                Basis = "ai"
	BasisReviewer          Basis = "reviewer"
)

// Granularity indicates product vs pack specificity.
type Granularity string

const (
	GranularityNone    Granularity = "none"
	GranularityProduct Granularity = "product"
	GranularityPack    Granularity = "pack"
)

// GranularityPair holds required vs achieved resolution.
type GranularityPair struct {
	Required Granularity `json:"required"`
	Achieved Granularity `json:"achieved"`
}

// PackOutcome indicates pack comparison result.
type PackOutcome string

const (
	PackOutcomeAgree             PackOutcome = "agree"
	PackOutcomeConflict          PackOutcome = "conflict"
	PackOutcomeUnstated          PackOutcome = "unstated"
	PackOutcomeNeedsRelationship PackOutcome = "needs_relationship"
)

// PackDecision holds pack-level outcome and optional relationship.
type PackDecision struct {
	Outcome        PackOutcome `json:"outcome"`
	RelationshipID *int64      `json:"relationship_id,omitempty"`
}

// CandidateDecision captures evaluation details of a top candidate.
type CandidateDecision struct {
	ProductID int64             `json:"product_id"`
	Rank      int               `json:"rank"`
	Eligible  bool              `json:"eligible"`
	Diff      map[string]string `json:"diff,omitempty"`
	Reasons   []string          `json:"reasons,omitempty"`
}

// Reason explains a gate action or decision finding.
type Reason struct {
	Code   string            `json:"code"`
	Params map[string]string `json:"params,omitempty"`
}

// Versions captures the components and policy versions that produced the decision.
type Versions struct {
	Policy          string `json:"policy"`
	Normalizer      string `json:"normalizer"`
	CatalogRevision int64  `json:"catalog_revision"`
	Prompt          string `json:"prompt,omitempty"`
	AIConfig        string `json:"ai_config,omitempty"`
}

// AIState represents the lifecycle of AI enhancement for a row.
type AIState string

const (
	AIStateOff        AIState = "off"
	AIStateIneligible AIState = "ineligible"
	AIStateQueued     AIState = "queued"
	AIStateRunning    AIState = "running"
	AIStateCompleted  AIState = "completed"
	AIStateStopped    AIState = "stopped"
	AIStateFailed     AIState = "failed"
	AIStateObsolete   AIState = "obsolete"
)

// AIDecision records AI run status and outcome.
type AIDecision struct {
	State    AIState  `json:"state"`
	Proposal *int64   `json:"proposal,omitempty"`
	Outcome  string   `json:"outcome,omitempty"`
	Attempts []int64  `json:"attempts,omitempty"`
}

// DecisionActor captures human review authorship.
type DecisionActor struct {
	UserID      string       `json:"user_id"`
	At          time.Time    `json:"at"`
	Corrections []Correction `json:"corrections,omitempty"`
}

// Correction records a reviewer attribute override.
type Correction struct {
	Attribute string `json:"attribute"`
	Value     string `json:"value"`
	Reason    string `json:"reason,omitempty"`
}

// RowDecision is the complete persisted decision record (JSON v1).
type RowDecision struct {
	V           int                 `json:"v"`
	State       State               `json:"state"`
	Basis       Basis               `json:"basis"`
	ProductID   *int64              `json:"product_id"`
	Granularity GranularityPair     `json:"granularity"`
	Pack        PackDecision        `json:"pack"`
	Candidates  []CandidateDecision `json:"candidates,omitempty"`
	Reasons     []Reason            `json:"reasons,omitempty"`
	Flags       []string            `json:"flags,omitempty"`
	Versions    Versions            `json:"versions"`
	Revision    int                 `json:"revision"`
	AI          AIDecision          `json:"ai"`
	Actor       *DecisionActor      `json:"actor"`
}

// MarshalJSON implements json.Marshaler, ensuring V=1.
func (d RowDecision) MarshalJSON() ([]byte, error) {
	type alias RowDecision
	rec := alias(d)
	rec.V = 1
	return json.Marshal(rec)
}

// ParseRowDecision parses a JSON decision record, rejecting major version != 1.
func ParseRowDecision(data []byte) (RowDecision, error) {
	var d RowDecision
	if err := json.Unmarshal(data, &d); err != nil {
		return RowDecision{}, fmt.Errorf("failed to unmarshal decision record: %w", err)
	}
	if d.V != 1 {
		return RowDecision{}, fmt.Errorf("unsupported decision record version: %d (expected 1)", d.V)
	}
	return d, nil
}
