package ingest

import (
	"context"
	"errors"
)

// ErrImportStoreUnavailable means the catalogue import store was never wired,
// which makes the vendor import screen unavailable rather than broken.
var ErrImportStoreUnavailable = errors.New("ingest: catalogue import store is not configured")

// ImportStore is the persistence the vendor catalogue import needs.
//
// It is a narrow port on purpose. The engine is pure, the service orchestrates,
// and everything that touches Postgres is behind these fourteen calls — which
// is what lets the whole seven-stage flow be tested against a fake without a
// database.
type ImportStore interface {
	// Create opens an import and stores the uploaded bytes with it.
	Create(ctx context.Context, s *Session, file []byte) error
	// Get loads an import by its public id.
	Get(ctx context.Context, publicID string) (*Session, error)
	// File returns the stored upload, so the mapping can be re-derived and the
	// rows streamed without asking the vendor to upload again.
	File(ctx context.Context, id int64) ([]byte, error)
	// SaveDraft persists the vendor's corrections and settings and moves the
	// phase. It never touches the counters or the stored file.
	SaveDraft(ctx context.Context, s *Session) error
	// Begin marks the run started and clears any previous outcome.
	Begin(ctx context.Context, id int64) error
	// BeginCommit moves a reviewed import into 'processing' for the write,
	// KEEPING the staged rows. Begin deletes them, which is right for a
	// re-analysis and catastrophic for a commit: the rows are what it writes.
	BeginCommit(ctx context.Context, id int64) error
	// Progress records how far a run has reached, for the progress screen.
	Progress(ctx context.Context, id int64, percent int, note string) error
	// FinishStaging moves a run out of 'processing' and records what it
	// produced. SaveDraft refuses a session in that phase by design, so a
	// staging pass cannot publish its own outcome through it.
	FinishStaging(ctx context.Context, s *Session) error
	// RecoverStaleRuns releases sessions wedged in 'processing' by a process
	// that died holding them.
	RecoverStaleRuns(ctx context.Context) (int, error)
	// Finish records the outcome of a completed run.
	Finish(ctx context.Context, s *Session) error
	// Fail records a run that stopped on an error.
	Fail(ctx context.Context, id int64, message string) error
	// Cancel discards an import without touching the catalogue.
	Cancel(ctx context.Context, id int64) error
	// List backs the history panel on the upload screen.
	List(ctx context.Context, orgID int64, limit int) ([]*Session, error)

	// AppendRows records the per-row outcome ledger in batches.
	AppendRows(ctx context.Context, importID, orgID int64, rows []RowOutcome) error
	// ClearRows wipes previously staged rows for an import.
	ClearRows(ctx context.Context, importID int64) error
	// Rows reads a page of the results table.
	Rows(ctx context.Context, importID int64, filter RowFilter) ([]*RowOutcome, int, error)
	// RowCounts tallies the ledger by outcome, for the results screen's tabs.
	RowCounts(ctx context.Context, importID int64) (map[string]int, error)
	// ApplyAIMatches folds every answer the AI stage accepted onto the staged
	// rows in one statement. A per-row update here would put a round trip
	// behind each of two thousand matches and undo the whole point of batching
	// the requests that produced them.
	ApplyAIMatches(ctx context.Context, importID int64, matches []AIMatch) error
	// UpdateRow updates fields on a staged row before commit.
	UpdateRow(ctx context.Context, importID, rowID int64, displayName, customVariantName string, price, discount, costPrice, costDiscount *float64, quantity *int, isExcluded *bool) error
	// SetBatchQuantity applies a uniform quantity to all staged rows of an import.
	SetBatchQuantity(ctx context.Context, importID int64, quantity int) error
	// AssignRowMatch links a staged row to a master catalog product manually.
	AssignRowMatch(ctx context.Context, importID, rowID, productID int64, productName, productSKU string) error
	// ToggleRowExclude flips the is_excluded flag of a staged row.
	ToggleRowExclude(ctx context.Context, importID, rowID int64) (bool, error)

	// The bulk review actions. A review screen's unit of work is a decision
	// repeated over a page of rows, and doing that one form post at a time is
	// how a nine-thousand-row import goes unreviewed.

	// ConfirmRowMatches records the vendor accepting the engine's suggestion on
	// each row, which is what makes those rows importable. It returns how many
	// actually carried a product to confirm.
	ConfirmRowMatches(ctx context.Context, importID int64, rowIDs []int64) (int, error)
	// ClearRowMatches unlinks a list of rows.
	ClearRowMatches(ctx context.Context, importID int64, rowIDs []int64) (int, error)
	// SetRowsExcluded includes or excludes a list of rows.
	SetRowsExcluded(ctx context.Context, importID int64, rowIDs []int64, excluded bool) (int, error)
	// PendingRowIDs lists the included rows a commit would refuse, and how many
	// there are in total.
	PendingRowIDs(ctx context.Context, importID int64, limit int) ([]int64, int, error)
	// RowIDsForFilter lists the ids of every row the review screen's current
	// filter selects, so "apply to everything matching this filter" means the
	// same rows the vendor is reading.
	RowIDsForFilter(ctx context.Context, importID int64, filter RowFilter, limit int) ([]int64, error)
	// StagedRowsForCommit returns all non-excluded rows ready to be written to catalog and inventory.
	StagedRowsForCommit(ctx context.Context, importID int64) ([]*RowOutcome, error)
	// MentionedRows lists what every included row of an import refers to,
	// CONFIRMED OR NOT: the catalogue product it resolved to and the codes it
	// carried.
	//
	// It exists for the mode that declares the file to be the whole catalogue.
	// Retiring "everything this run wrote" would delist a product the file does
	// mention but whose row the vendor left in the review queue — the file says
	// they stock it, and the import would take it off sale on the strength of
	// not having settled its match. Retiring "everything the file does not
	// mention" is the promise the mode actually makes.
	MentionedRows(ctx context.Context, importID int64) ([]RowMention, error)
	// UpdateCommittedRows updates rows after final execution.
	UpdateCommittedRows(ctx context.Context, importID int64, rows []RowOutcome) error

	// Sweep collects abandoned imports and the files they hold. It runs when a
	// new import is opened, so no scheduled job is needed.
	Sweep(ctx context.Context) error
}

// RowMention is what one staged row says it is about, before any question of
// whether the vendor confirmed it.
type RowMention struct {
	ProductID  int64
	SourceCode string
	Barcode    string
}
