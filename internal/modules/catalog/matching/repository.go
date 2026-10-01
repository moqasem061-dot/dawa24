package matching

import (
	"context"
	"time"

	"github.com/moqasem061-dot/dawa24/internal/shared/matchflow"
	"github.com/moqasem061-dot/dawa24/internal/shared/productmatch"
)

// Repository defines data access for catalog revisions, match policies, and match runs.
type Repository interface {
	CurrentRevision(ctx context.Context, scope string) (int64, error)
	ActivePolicy(ctx context.Context, workflow string, orgID int64) (productmatch.Policy, error)
	CreateRun(ctx context.Context, run *MatchRun) error
	GetRun(ctx context.Context, runID int64) (*MatchRun, error)
	ListRunRows(ctx context.Context, runID int64, workflow string, limit int) ([]DiagnosticRowData, error)
	ListEnhanceRows(ctx context.Context, runID int64, workflow string) ([]EnhanceRow, error)
	UpdateCounts(ctx context.Context, runID int64, counts matchflow.RunCounts) error
	SetState(ctx context.Context, runID int64, state string, finished bool) error
	Heartbeat(ctx context.Context, runID int64) error
	CountActiveAIRuns(ctx context.Context, orgID int64) (int, error)
	UpdateAIRunCounters(ctx context.Context, runID int64, aiCounters map[string]any) error
	RecordAIAttempt(ctx context.Context, attempt *AIAttemptRecord) error
	PurgeOldAIAttempts(ctx context.Context, maxAge time.Duration) (int64, error)

	KnowledgeRepository
	AIConfigRepository
	PolicyRepository
}

// PolicyRepository defines data access for policies, assignments, audit logs, and shadow outcomes (US13 T150-T152).
type PolicyRepository interface {
	ActivePolicy(ctx context.Context, workflow string, orgID int64) (productmatch.Policy, error)
	ResolvePolicyAssignment(ctx context.Context, workflow string, orgID int64) (active productmatch.Policy, shadowVersion string, err error)
	GetPolicy(ctx context.Context, version string) (productmatch.Policy, error)
	ListPolicies(ctx context.Context) ([]PolicyRecord, error)
	ListAssignments(ctx context.Context) ([]PolicyAssignment, error)
	UpdateAssignment(ctx context.Context, workflow, cohort, activeVersion string, shadowVersion *string, changedBy, reason string) error
	RollbackAssignment(ctx context.Context, workflow, cohort, changedBy string) error
	RecordShadowOutcomes(ctx context.Context, outcomes []ShadowOutcome) error
	ListShadowOutcomes(ctx context.Context, runID int64) ([]ShadowOutcome, error)
}

// AIConfigRepository defines data access for AI match configurations (US10 T134).
type AIConfigRepository interface {
	ListAIConfigurations(ctx context.Context) ([]AIConfiguration, error)
	GetAIConfiguration(ctx context.Context, id string) (*AIConfiguration, error)
	UpdateAIConfigurationStatus(ctx context.Context, id string, newStatus string, changedBy string, reason string) error
}

// KnowledgeRepository defines data access for scoped match knowledge (US8 T108).
type KnowledgeRepository interface {
	LookupOrg(ctx context.Context, sigs []string) ([]Knowledge, error)
	LookupPlatform(ctx context.Context, sigs []string, orgID int64) ([]Knowledge, error)
	Upsert(ctx context.Context, k Knowledge) error
	Revoke(ctx context.Context, id int64, revokedBy string) error
	MarkStale(ctx context.Context, ids []int64) error
	List(ctx context.Context, f KnowledgeFilter) (Page[Knowledge], error)
	LoadAliases(ctx context.Context, orgID int64) (map[string][]int64, error)
}

// CuratedRepository defines data access for platform-curated identity facts and pack relationships.
type CuratedRepository interface {
	ListActive(ctx context.Context) (map[int64][]productmatch.CuratedFact, map[int64][]productmatch.PackRelationship, error)
}
