package matching

import (
	"time"

	"github.com/moqasem061-dot/dawa24/internal/shared/matchflow"
)

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
