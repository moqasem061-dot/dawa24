package ingest

import (
	"time"

	"github.com/moqasem061-dot/dawa24/internal/shared/i18n"
	"github.com/moqasem061-dot/dawa24/internal/shared/money"
)

// The types below are what the import pipeline exchanges with the catalogue
// and the warehouse. They belong to ingest, not to catalog or inventory:
// modules do not import each other (AGENTS.md rule 5). cmd/server adapts the
// catalog and inventory services to CatalogPort and InventoryPort.

// VariantKey identifies one of a vendor's existing variants for matching.
type VariantKey struct {
	ID          int64  `json:"id"`
	ProductID   int64  `json:"product_id"`
	SKU         string `json:"sku"`
	Barcode     string `json:"barcode"`
	Unit        string `json:"unit"`
	BatchNumber string `json:"batch_number"`
	BranchID    *int64 `json:"branch_id,omitempty"`
	// Active says whether the variant is currently on sale. The replace mode's
	// preview needs it: an already-inactive variant is not one the run is about
	// to take off sale.
	Active bool `json:"active"`
}

// MatchProduct is one master-catalogue product as the matcher sees it.
type MatchProduct struct {
	ID            int64  `json:"id"`
	NameAR        string `json:"name_ar"`
	NameEN        string `json:"name_en"`
	SKU           string `json:"sku"`
	Barcode       string `json:"barcode"`
	Scientific    string `json:"scientific"`
	DosageForm    string `json:"dosage_form"`
	Concentration string `json:"concentration"`
	Unit          string `json:"unit"`
	Manufacturer  string `json:"manufacturer"`
	PublicPrice   string `json:"public_price"`
}

// ProductRef is the little of a master product a manual match records.
type ProductRef struct {
	ID     int64
	NameAR string
	NameEN string
	SKU    string
}

// RetiredVariant is a variant a replace-mode run took off sale.
type RetiredVariant struct {
	ID        int64
	ProductID int64
}

// VariantStatus is the sale status a new variant is written with. Empty
// leaves an existing variant's status as it is.
type VariantStatus string

const (
	VariantActive   VariantStatus = "active"
	VariantInactive VariantStatus = "inactive"
)

// VariantDraft is the vendor's catalogue entry a reviewed row becomes.
type VariantDraft struct {
	ID             int64
	OrganizationID int64
	ProductID      int64
	Name           i18n.Text
	SKU            string
	Barcode        string
	Unit           string
	BatchNumber    string
	ExpiryDate     *time.Time
	BranchID       *int64
	Price          money.Amount
	Discount       money.Amount
	CostPrice      *money.Amount
	// CostDiscountBps is the cost discount in basis points (1250 = 12.50%).
	CostDiscountBps int64
	MinOrderQty     int
	IsNegotiable    bool
	Status          VariantStatus
	VariantType     string
}

// VariantWriteRow is one variant to write; Ref is the caller's row index.
type VariantWriteRow struct {
	Ref     int
	Variant *VariantDraft
}

// WriteFailure is one row a batched write could not store.
type WriteFailure struct {
	Ref     int    `json:"ref"`
	Message string `json:"message"`
}

// VariantWriteResult accounts for a batched variant write.
type VariantWriteResult struct {
	Inserted int
	Updated  int
	Failures []WriteFailure
	// IDs maps the caller's Ref onto the variant id that was written.
	IDs map[int]int64
	// InsertedRefs marks the refs that created a variant rather than updating one.
	InsertedRefs map[int]bool
}

// Vocabulary is the catalogue's own brand and category names.
type Vocabulary struct {
	Brands     []string
	Categories []string
}

// Warehouse is a vendor warehouse as the import sees it.
type Warehouse struct {
	ID   int64
	Name string
	Code string
}

// StockMode is how an imported quantity is read.
type StockMode string

const (
	// StockReplace sets the balance to the file's figure: a daily stock export.
	StockReplace StockMode = "replace"
	// StockAdd adds the figure to what is there: a goods-received note.
	StockAdd StockMode = "add"
	// StockKeep leaves balances alone and updates only thresholds: a price list.
	StockKeep StockMode = "keep"
)

// Label renders a mode in Arabic.
func (m StockMode) Label() string {
	switch m {
	case StockAdd:
		return i18n.TDefault("w4_mod.s_399_399")
	case StockKeep:
		return i18n.TDefault("w4_mod.s_400_400")
	default:
		return i18n.TDefault("w4_mod.s_401_401")
	}
}

// StockWriteRow is one balance to write.
type StockWriteRow struct {
	Ref            int
	OrganizationID int64
	WarehouseID    int64
	ProductID      int64
	VariantID      int64
	Quantity       int
	MinThreshold   int
	// HasQuantity is false when the file said nothing about this row's
	// balance; the quantity is then left alone whatever the mode says.
	HasQuantity bool
}

// StockWriteResult accounts for a batched stock write.
type StockWriteResult struct {
	Written  int
	Failures []WriteFailure
}
