// Package importrun defines the domain model for durable import sessions.
//
// Every file import on the platform — saving products, team members,
// compare uploads, temp warehouses, catalogue imports — records its
// lifecycle in platform.import_runs.  Staged rows live in
// platform.import_run_rows.  The in-memory session stores that
// preceded this package held state only while a single web process was
// alive; this package makes imports survive restarts, horizontal
// scaling, and the transition to River-backed background processing.
package importrun

import (
	"context"
	"encoding/json"
	"time"
)

// Import kind constants.  Each names a distinct import pipeline whose
// worker knows how to parse the payload and stage/commit rows.
const (
	KindSavingProducts = "saving_products"
	KindTeam           = "team"
	KindCompare        = "compare"
	KindTempWarehouse  = "temp_warehouse"
	KindCatalog        = "catalog"
	KindCatalogImages  = "catalog_images"
)

// Audience constants.
const (
	AudienceVendor   = "vendor"
	AudienceCustomer = "customer"
	AudienceAdmin    = "admin"
)

// State machine constants.  The state field on an import run follows
// this progression:
//
//	queued → processing → ready → committing → committed
//	                   ↘ failed
//	     any state     → cancelled
const (
	StateQueued     = "queued"
	StateProcessing = "processing"
	StateReady      = "ready"
	StateCommitting = "committing"
	StateCommitted  = "committed"
	StateFailed     = "failed"
	StateCancelled  = "cancelled"
)

// Run is a durable import session.
type Run struct {
	ID             int64           `json:"id"`
	PublicID       string          `json:"public_id"`
	OrganizationID int64           `json:"organization_id"`
	UserID         int64           `json:"user_id"`
	Kind           string          `json:"kind"`
	Audience       string          `json:"audience"`
	Filename       string          `json:"filename"`
	State          string          `json:"state"`
	Phase          string          `json:"phase"`
	Percent        int             `json:"percent"`
	TotalRows      int             `json:"total_rows"`
	ProcessedRows  int             `json:"processed_rows"`
	Payload        json.RawMessage `json:"payload"`
	Result         json.RawMessage `json:"result"`
	ErrorMessage   string          `json:"error_message,omitempty"`
	RiverJobID     *int64          `json:"river_job_id,omitempty"`
	StartedAt      *time.Time      `json:"started_at,omitempty"`
	FinishedAt     *time.Time      `json:"finished_at,omitempty"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
}

// IsDone returns true when the run has reached a terminal state.
func (r *Run) IsDone() bool {
	return r.State == StateCommitted || r.State == StateFailed || r.State == StateCancelled
}

// IsWorking returns true when the run is being processed or committed.
func (r *Run) IsWorking() bool {
	return r.State == StateProcessing || r.State == StateCommitting
}

// Row is a single staged row in an import run.
type Row struct {
	ID               int64           `json:"id"`
	RunID            int64           `json:"run_id"`
	RowNumber        int             `json:"row_number"`
	Data             json.RawMessage `json:"data"`
	Included         bool            `json:"included"`
	MatchedProductID *int64          `json:"matched_product_id,omitempty"`
	CreatedAt        time.Time       `json:"created_at"`
	UpdatedAt        time.Time       `json:"updated_at"`
}

// Progress is the shape polled by the frontend progress bar.
// It matches what import-progress.js already expects.
type Progress struct {
	State     string `json:"state"`
	Phase     string `json:"phase"`
	Percent   int    `json:"percent"`
	Processed int    `json:"processed"`
	Total     int    `json:"total"`
	Error     string `json:"error,omitempty"`
	Done      bool   `json:"done"`
}

// ProgressFromRun builds a Progress from a Run.
func ProgressFromRun(r *Run) Progress {
	return Progress{
		State:     r.State,
		Phase:     r.Phase,
		Percent:   r.Percent,
		Processed: r.ProcessedRows,
		Total:     r.TotalRows,
		Error:     r.ErrorMessage,
		Done:      r.IsDone(),
	}
}

// Repository defines the storage contract for import runs.
type Repository interface {
	// CreateRun inserts a new import run and returns it with its generated
	// fields (id, public_id, created_at) populated.
	CreateRun(ctx context.Context, run *Run) error

	// GetRunByPublicID fetches a run by its public UUID, scoped to an
	// organization (0 = system / admin).
	GetRunByPublicID(ctx context.Context, publicID string, orgID int64) (*Run, error)

	// GetRunByPublicIDSystem fetches a run by its public UUID without
	// tenant scoping (for admin/system callers).
	GetRunByPublicIDSystem(ctx context.Context, publicID string) (*Run, error)

	// GetRunByID fetches a run by its internal ID.
	GetRunByID(ctx context.Context, id int64) (*Run, error)

	// UpdateProgress sets the phase, percent and processed row count.
	UpdateProgress(ctx context.Context, id int64, phase string, percent int, processed int) error

	// TransitionState moves a run to a new state, recording started_at or
	// finished_at as appropriate.
	TransitionState(ctx context.Context, id int64, newState string) error

	// FailRun moves a run to 'failed' with an error message.
	FailRun(ctx context.Context, id int64, errMsg string) error

	// SetResult stores the summary counters on a run.
	SetResult(ctx context.Context, id int64, result json.RawMessage) error

	// SetRiverJobID records the River job ID on the run.
	SetRiverJobID(ctx context.Context, id int64, jobID int64) error

	// UpdatePayload updates the JSONB payload on a run.
	UpdatePayload(ctx context.Context, id int64, payload json.RawMessage) error

	// GetActiveRunForUser finds the most recent unfinished run for a user/kind.
	GetActiveRunForUser(ctx context.Context, userID int64, kind string) (*Run, error)

	// ListRunsByOrg returns recent runs for an organization, newest first.
	ListRunsByOrg(ctx context.Context, orgID int64, kind string, limit, offset int) ([]*Run, int, error)

	// InsertRows bulk-inserts staged rows for a run.
	InsertRows(ctx context.Context, runID int64, rows []Row) error

	// ListRows returns paginated rows for a run.
	ListRows(ctx context.Context, runID int64, onlyIncluded bool, limit, offset int) ([]Row, int, error)

	// UpdateRow updates a single row's data and/or included flag.
	UpdateRow(ctx context.Context, rowID int64, data json.RawMessage, included *bool, matchedProductID *int64) error

	// RecoverStaleRuns fails any run stuck in processing/committing longer
	// than the threshold.  Returns the number of runs recovered.
	RecoverStaleRuns(ctx context.Context) (int, error)
}
