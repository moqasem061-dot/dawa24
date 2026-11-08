package erp

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/moqasem061-dot/dawa24/internal/shared/money"
)

// Cutoffs is the retention sweep's per-table boundaries.
type Cutoffs struct {
	APICalls    time.Time
	SyncItems   time.Time
	SyncBatches time.Time
	Review      time.Time
	MatchRuns   time.Time
	BatchRows   int
}

// NewBatch is one accepted sync batch ready to insert with its items.
type NewBatch struct {
	IntegrationID  int64
	APIKeyID       int64
	WarehouseID    int64
	BranchID       int64
	ClientBatchID  string
	IdempotencyKey string
	BodySHA256     string
	ItemCount      int
	ChunkCount     int
}

// NewItemRow is one pre-normalised row of erp.sync_items.
type NewItemRow struct {
	Position     int
	ChunkNo      int
	ExternalCode string
	Payload      []byte
	State        string
	Code         string // empty on success-shaped rows
	Message      map[string]string
}

// SyncBatch mirrors erp.sync_batches, plus the integration's provider.
type SyncBatch struct {
	ID               int64
	PublicID         string
	OrganizationID   int64
	IntegrationID    int64
	APIKeyID         *int64
	WarehouseID      int64
	BranchID         int64
	ClientBatchID    string
	IdempotencyKey   string
	BodySHA256       string
	State            string
	ItemCount        int
	ChunkCount       int
	ChunksDone       int
	CreatedCount     int
	UpdatedCount     int
	UnchangedCount   int
	NeedsReviewCount int
	FailedCount      int
	ErrorCode        *string
	AcceptedAt       time.Time
	StartedAt        *time.Time
	FinishedAt       *time.Time
	Provider         string
}

// SyncItemRow mirrors erp.sync_items.
type SyncItemRow struct {
	ID             int64
	OrganizationID int64
	BatchID        int64
	Position       int
	ChunkNo        int
	ExternalCode   string
	Payload        []byte
	State          string
	Code           *string
	Message        []byte
	VariantID      *int64
	ProcessedAt    *time.Time
}

// MessageMap decodes the item's bilingual message, empty when none.
func (r SyncItemRow) MessageMap() map[string]string {
	out := map[string]string{}
	_ = json.Unmarshal(r.Message, &out)
	return out
}

// ProductLink mirrors erp.product_links.
type ProductLink struct {
	ID             int64
	OrganizationID int64
	IntegrationID  int64
	ExternalCode   string
	VariantID      int64
	ProductID      *int64
	TaxPercent     *money.Amount
	LinkSource     string
	LastHash       string
	LastSyncedAt   *time.Time
	// VariantDeleted is not a column: the loader computes it for the
	// resolution tier (a link whose variant was soft-deleted goes to review
	// with ERP_ITEM_DELETED_LINK instead of resurrecting the variant), and the
	// tests set it directly against the fake.
	VariantDeleted bool
}

// ReviewItem mirrors erp.review_items.
type ReviewItem struct {
	ID                int64
	OrganizationID    int64
	IntegrationID     int64
	ExternalCode      string
	Payload           []byte
	Candidates        []byte
	ReasonCode        string
	BatchID           *int64
	State             string
	ResolvedProductID *int64
	ResolvedVariantID *int64
	ResolvedBy        *int64
	ResolvedAt        *time.Time
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// ItemResult updates one sync item after processing.
type ItemResult struct {
	ItemID    int64
	State     string
	Code      string
	Message   map[string]string
	VariantID *int64
}

// LinkWrite upserts the code-to-variant association discovered or confirmed by
// a chunk.
type LinkWrite struct {
	IntegrationID int64
	VariantID     int64
	ProductID     *int64
	ExternalCode  string
	LinkSource    string
	LastHash      string
	TaxPercent    *money.Amount
}

// ReviewWrite upserts one open review item for the supplier's screen.
type ReviewWrite struct {
	IntegrationID int64
	BatchID       int64
	ExternalCode  string
	ReasonCode    string
	Payload       []byte
	Candidates    []byte
}

// Page is the shared keyset page (the id is the last row seen; 0 means start).
type Page struct {
	After int64
	Limit int
}

// SyncRepository is the persistence the S2 sync logic needs. Methods that read
// or write other tenants' rows say AsSystem in their name and are the named
// exceptions of plan.md E1-E7.
type SyncRepository interface {
	// InsertBatch writes the batch and its items inside one transaction and
	// calls enqueue (if set) inside that same transaction with the inserted
	// batch, so the chunk jobs can reference its identity.
	InsertBatch(ctx context.Context, orgID int64, b NewBatch, items []NewItemRow, enqueue func(ctx context.Context, tx pgx.Tx, batch SyncBatch) error) (SyncBatch, error)
	FindBatchByIdempotency(ctx context.Context, orgID, integrationID int64, key string) (SyncBatch, bool, error)
	GetBatchByPublicID(ctx context.Context, orgID int64, publicID string) (SyncBatch, error)
	GetBatchByID(ctx context.Context, orgID, batchID int64) (SyncBatch, error)
	ListSyncItems(ctx context.Context, orgID, batchID, afterID int64, state string, limit int) ([]SyncItemRow, error)
	EarlierBatchActive(ctx context.Context, orgID, integrationID, batchID int64) (bool, error)
	MarkBatchProcessing(ctx context.Context, orgID, batchID int64) error
	LoadPendingItems(ctx context.Context, orgID, batchID int64, chunkNo int) ([]SyncItemRow, error)
	LoadLinksByCode(ctx context.Context, orgID, integrationID int64, codes []string) (map[string]ProductLink, error)
	FindVariantsBySKU(ctx context.Context, orgID int64, codes []string) (map[string][]int64, error)
	FindVariantsByBarcode(ctx context.Context, orgID int64, barcodes []string) (map[string][]int64, error)
	WriteResults(ctx context.Context, orgID int64, res []ItemResult, links []LinkWrite, reviews []ReviewWrite) error
	FinishChunk(ctx context.Context, orgID, batchID int64) (done bool, err error)
	ListReview(ctx context.Context, orgID int64, page Page) ([]ReviewItem, error)
	GetReview(ctx context.Context, orgID, id int64) (ReviewItem, error)
	ResolveReviewRow(ctx context.Context, orgID, id, variantID, productID, actorUserID int64, audit AuditRecord) error
	DismissReviewRow(ctx context.Context, orgID, id, actorUserID int64, audit AuditRecord) error
	PurgeExpiredAsSystem(ctx context.Context, cut Cutoffs) error
}

// Interfaces the intake needs from the queue wrapper live in ports.go
// (Enqueuer is one of the Ports of T106).
