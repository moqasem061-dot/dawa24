package pages

import (
	"time"

	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// SavingImportPhase represents the current wizard stage.
type SavingImportPhase string

const (
	SavingPhaseUpload    SavingImportPhase = "upload"
	SavingPhaseMapping   SavingImportPhase = "mapping"
	SavingPhaseReview    SavingImportPhase = "review"
	SavingPhaseCompleted SavingImportPhase = "completed"
)

// SessionState represents the lifecycle status of an import session.
type SessionState string

const (
	SessionStateUploaded   SessionState = "uploaded"
	SessionStateProcessing SessionState = "processing"
	SessionStateReady      SessionState = "ready"
	SessionStateCommitted  SessionState = "committed"
	SessionStateCancelled  SessionState = "cancelled"
	SessionStateFailed     SessionState = "failed"
)

// SavingDetectedCols holds auto-detected column indices.
type SavingDetectedCols struct {
	NameCol      int `json:"name_col"`
	SKUCol       int `json:"sku_col"`
	QtyCol       int `json:"qty_col"`
	PriceCol     int `json:"price_col"`
	ProductIDCol int `json:"product_id_col"`
}

// StagedSavingItem represents a single parsed row in memory during review.
type StagedSavingItem struct {
	Index             int          `json:"index"`
	NameProduct       string       `json:"name_product"`
	SKU               string       `json:"sku"`
	Quantity          float64      `json:"quantity"`
	Price             money.Amount `json:"price"`
	TotalValue        money.Amount `json:"total_value"`
	ProductID         *int64       `json:"product_id,omitempty"`
	MasterProductName string       `json:"master_product_name,omitempty"`
	MasterProductSKU  string       `json:"master_product_sku,omitempty"`
	MatchType         string       `json:"match_type"`
	Confidence        float64      `json:"confidence"`
	Included          bool         `json:"included"`
}

// SavingImportSession represents an active or historical import session.
type SavingImportSession struct {
	Success       bool                `json:"success"`
	ID            string              `json:"id"`
	OrgID         int64               `json:"org_id"`
	UserID        int64               `json:"user_id"`
	Filename      string              `json:"filename"`
	Status        SessionState        `json:"status"`
	Phase         SavingImportPhase   `json:"phase"`
	Progress      int                 `json:"progress"`
	ProgressPhase string              `json:"progress_phase"`
	ErrorMessage  string              `json:"error_message,omitempty"`
	CreatedAt     time.Time           `json:"created_at"`
	ExpiresAt     time.Time           `json:"expires_at"`
	TotalRows     int                 `json:"total_rows"`
	ProcessedRows int                 `json:"processed_rows"`
	MatchedRows   int                 `json:"matched_rows"`
	UnlinkedRows  int                 `json:"unlinked_rows"`
	TotalQuantity float64             `json:"total_quantity"`
	TotalValue    money.Amount        `json:"total_value"`
	InsertedCount int                 `json:"inserted_count"`
	UpdatedCount  int                 `json:"updated_count"`
	Headers       []string            `json:"headers,omitempty"`
	DetectedCols  SavingDetectedCols  `json:"detected_cols"`
	SampleRows    [][]string          `json:"sample_rows,omitempty"`
	RawDataRows   [][]string          `json:"-"`
	Items         []*StagedSavingItem `json:"items,omitempty"`
	ColName       string              `json:"col_name,omitempty"`
	ColSKU        string              `json:"col_sku,omitempty"`
	ColQty        string              `json:"col_qty,omitempty"`
	ColPrice      string              `json:"col_price,omitempty"`
	ColProductID  string              `json:"col_product_id,omitempty"`
	MatchStrategy string              `json:"match_strategy,omitempty"`
	// UseAI is the same switch the other three importers offer, in the same
	// group and the same position on the screen. Off by default: the
	// deterministic tiers are what an import must be judged on, and a feature
	// that spends money on a tenant's behalf is turned on deliberately.
	UseAI bool `json:"use_ai,omitempty"`
}

// IsProcessing reports whether a background run is actually matching this
// session's rows right now.
//
// Status alone is not the answer. A session waiting for its columns to be
// confirmed carries no running goroutine, and treating it as processing is what
// made the wizard skip step 2 and then sit on a progress bar nothing was
// feeding. Phase is the tie-breaker: a session on the mapping screen is waiting
// for the buyer, never for the matcher.
func (s *SavingImportSession) IsProcessing() bool {
	if s == nil {
		return false
	}
	return s.Status == SessionStateProcessing && s.Phase != SavingPhaseMapping
}

// AwaitingMapping reports whether the buyer still has to confirm the columns.
func (s *SavingImportSession) AwaitingMapping() bool {
	return s != nil && s.Phase == SavingPhaseMapping && !s.IsProcessing()
}

// SavingRowFilter contains query parameters for filtering and sorting review table rows.
type SavingRowFilter struct {
	Search      string
	MatchFilter string // "" = all, "matched" = matched, "unmatched" = unlinked
	SortBy      string // "index", "name", "catalog", "score", "qty", "price", "total"
	SortOrder   string // "asc" or "desc"
	Page        int
	Limit       int
}

// SavingImportView is the view model passed into the SavingImportPage template.
type SavingImportView struct {
	Audience      string
	TargetOrgName string
	TargetOrgType string
	BaseURL       string
	ImportURL     string
	Session       *SavingImportSession
	Sessions      []*SavingImportSession
	Filter        SavingRowFilter
	Rows          []*StagedSavingItem
	RowTotal      int
	NoticeType    string
	NoticeMsg     string
	Fatal         string

	// AIAvailable says whether the platform can actually run the AI stage. The
	// switch renders disabled with a reason when it cannot, rather than
	// offering a toggle that ticks and then does nothing — which is what the
	// old strategy dropdown did, promising i18n.TDefault("w4m_ui.s_4_4") from an engine that
	// had none.
	AIAvailable         bool
	AIUnavailableReason string
}

// WizardStep maps the session's phase onto the shared, canonically numbered
// step.
//
// This list has no settings screen — there is nothing to decide beyond the
// mapping — so step 3 is rendered greyed rather than the review being
// renumbered into its place. The number is what a user remembers, and it has to
// mean the same thing in every wizard on the platform.
func (v SavingImportView) WizardStep() Step {
	phase := SavingPhaseUpload
	if v.Session != nil && v.Session.Phase != "" {
		phase = v.Session.Phase
	}
	// A run still matching rows is on the processing step, whatever phase the
	// session was left in. The store sets Phase to review the moment a session
	// is created, because the review screen is where it ends up — but it is not
	// there yet, and rendering the review while the matching is still going is
	// how the wizard came to show an empty table.
	if v.Session.IsProcessing() {
		return StepReview
	}
	switch phase {
	case SavingPhaseMapping:
		return StepColumns
	case SavingPhaseReview:
		return StepReview
	case SavingPhaseCompleted:
		return StepResults
	default:
		return StepFile
	}
}
