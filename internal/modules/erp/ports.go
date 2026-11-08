package erp

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/moqasem061-dot/dawa24/internal/shared/money"
	"github.com/moqasem061-dot/dawa24/internal/shared/productmatch"
)

// Ports are the module-boundary adaptations (plan traps 10, 18): the erp
// module never imports a sibling module. The composition root (cmd/server,
// cmd/worker through internal/composition/erpports) fills them; a nil port
// means the slice that needs it is not wired on this binary.
type Ports struct {
	VariantWriter VariantWriter
	StockWriter   StockWriter
	Matcher       Matcher
	MatchMemory   MatchMemory
	Enqueuer      Enqueuer
}

// VariantWriter writes the catalog side of a synced chunk. Write inserts
// (VariantID 0) and updates in one call, mirroring
// catalog.Service.BulkWriteVariants; SetQuotaLimits mirrors
// catalog.Service.SetVariantQuotaLimits.
type VariantWriter interface {
	Write(ctx context.Context, orgID int64, rows []VariantWrite) (VariantWriteResult, error)
	SetQuotaLimits(ctx context.Context, orgID int64, limits map[int64]*int) error
}

// StockWriter replaces warehouse balances, mirroring
// inventory.Service.BulkWriteStocks(StockReplace). The failure map is keyed by
// the caller's Ref.
type StockWriter interface {
	Replace(ctx context.Context, rows []StockWrite) (map[int]string, error)
}

// Matcher is the deterministic matcher seam. It has exactly one method —
// Prepare — so the ERP can run with the Gateway down (trap 18). TestSyncNeverCallsEnhancement
// proves the shape at compile time.
type Matcher interface {
	Prepare(ctx context.Context, req productmatch.PrepareRequest) (productmatch.PrepareResult, error)
}

// MatchMemory stores confirmed associations in match_knowledge (trap 17:
// catalog.match_knowledge, Kind "supplier_code" from the sync and
// "human_confirm" from supplier review).
type MatchMemory interface {
	Remember(ctx context.Context, sc productmatch.Scope, k KnowledgeWrite) error
}

// Enqueuer inserts River jobs inside the acceptance transaction; satisfied by
// queue.Client.EnqueueTx through a lazy wrapper (trap 20).
type Enqueuer interface {
	EnqueueTx(ctx context.Context, tx pgx.Tx, args river.JobArgs, opts *river.InsertOpts) (*rivertype.JobInsertResult, error)
}

// VariantWrite carries one chunk row to the catalog writer. VariantID 0 means
// insert (the adapter sets the catalog status active); otherwise update. The
// fields mirror catalog.ProductVariant.
type VariantWrite struct {
	Ref         int
	VariantID   int64
	ProductID   int64
	BranchID    int64
	Name        map[string]string
	SKU         string
	Barcode     string
	Unit        string
	BatchNumber string
	Price       money.Amount
	Discount    money.Amount
	ExpiryDate  *time.Time
}

// VariantWriteResult mirrors catalog.VariantWriteResult, Failures keyed by Ref.
type VariantWriteResult struct {
	Inserted     int
	Updated      int
	Failures     map[int]string
	IDs          map[int]int64
	InsertedRefs map[int]bool
}

// StockWrite carries one warehouse balance to the stock writer, mirroring
// inventory.StockWriteRow and its Stock.
type StockWrite struct {
	Ref            int
	OrganizationID int64
	WarehouseID    int64
	ProductID      int64
	VariantID      int64
	Quantity       int
	HasQuantity    bool
}

// KnowledgeWrite mirrors matching.KnowledgeWrite field by field
// (internal/modules/catalog/matching/domain.go:104) so the adapter converts
// without logic.
type KnowledgeWrite struct {
	SourceNamespace   string
	QuestionSignature string
	Kind              string
	TargetProductID   *int64
	TargetGranularity string
	PolicyVersion     string
	NormalizerVersion string
	DisplayText       string
}
