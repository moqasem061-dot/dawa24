package catalog

import (
	"fmt"
	"time"

	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// The staged import.
//
// An import used to be one request: upload, parse, write, hope. For a
// nine-thousand-row catalogue that is a decision the admin makes blind. The flow
// is now three steps — analyse the file, review what will happen, then commit —
// with the middle step held in catalog.import_sessions so the admin can read a
// full account of every row before anything touches the catalogue.

// ImportMode is the reconciliation strategy applied at commit.
//
// The names match ingest.ImportMode deliberately. A vendor uploading a price
// list and an admin uploading the master catalogue are choosing between the same
// four behaviours, and one vocabulary across both screens is worth more than the
// module independence a second set of names would buy.
type ImportMode string

const (
	// ModeAddNewOnly inserts products the catalogue does not have and leaves
	// every existing row untouched.
	ModeAddNewOnly ImportMode = "add_new_only"
	// ModeUpdateExistingOnly refreshes matched products and inserts nothing.
	ModeUpdateExistingOnly ImportMode = "update_existing_only"
	// ModeClearAndAdd retires the whole catalogue, then inserts the file.
	ModeClearAndAdd ImportMode = "clear_and_add"
	// ModeUpdateAndAdd is the default: update what matches, insert what does not.
	ModeUpdateAndAdd ImportMode = "update_and_add"
)

// ImportModeOption describes one mode for the chooser on the upload screen.
type ImportModeOption struct {
	Mode        ImportMode
	Icon        string
	Title       string
	Description string
	// Destructive marks a mode that removes existing catalogue rows, so the UI
	// can demand a deliberate second confirmation for it.
	Destructive bool
}

// ImportModeOptions are the four strategies, in the order they are offered.
var ImportModeOptions = []ImportModeOption{
	{
		Mode:        ModeUpdateAndAdd,
		Icon:        "⚡",
		Title:       i18n.TDefault("w4_mod.s_289_289"),
		Description: i18n.TDefault("w4_mod.w4str_107_107"),
	},
	{
		Mode:        ModeAddNewOnly,
		Icon:        "➕",
		Title:       i18n.TDefault("w4_mod.s_290_290"),
		Description: i18n.TDefault("w4_mod.w4str_108_108"),
	},
	{
		Mode:        ModeUpdateExistingOnly,
		Icon:        "🔄",
		Title:       i18n.TDefault("w4_mod.s_291_291"),
		Description: i18n.TDefault("w4_mod.w4str_109_109"),
	},
	{
		Mode:        ModeClearAndAdd,
		Icon:        "🗑️",
		Title:       i18n.TDefault("w4_mod.s_292_292"),
		Description: i18n.TDefault("w4_mod.w4str_110_110"),
		Destructive: true,
	},
}

// ParseMode maps a submitted value onto a mode, defaulting to the safe one.
func ParseMode(raw string) ImportMode {
	switch ImportMode(raw) {
	case ModeAddNewOnly:
		return ModeAddNewOnly
	case ModeUpdateExistingOnly:
		return ModeUpdateExistingOnly
	case ModeClearAndAdd:
		return ModeClearAndAdd
	default:
		return ModeUpdateAndAdd
	}
}

// Label renders a mode in the admin's language.
func (m ImportMode) Label() string {
	for _, o := range ImportModeOptions {
		if o.Mode == m {
			return o.Title
		}
	}
	return string(m)
}

// IsDestructive reports whether the mode removes existing catalogue rows.
func (m ImportMode) IsDestructive() bool { return m == ModeClearAndAdd }

// SessionStatus tracks where a staged import has reached.
type SessionStatus string

const (
	// SessionDraft is analysed: the file has been read and its structure
	// worked out, and the admin is on the mapping step deciding how it should
	// be interpreted. Nothing is staged yet.
	SessionDraft SessionStatus = "draft"
	// SessionProcessing is a preparation run in flight. It is durable on
	// purpose: a run whose only record was a map in process memory left a
	// session at 'draft' with no counts and no error when the process went
	// away, which is exactly what an admin reads as "the import returns
	// nothing".
	SessionProcessing SessionStatus = "processing"
	// SessionReady has been processed and is awaiting confirmation.
	SessionReady SessionStatus = "ready"
	// SessionCommitting is an atomic claim: a commit has taken ownership of the
	// session and is writing to the catalogue right now. It is not reviewable,
	// so a second commit, a cancel, or a re-prepare cannot interleave with the
	// run — and a crash mid-write leaves a row the next session-open reaper
	// records as failed rather than a committable zombie.
	SessionCommitting SessionStatus = "committing"
	// SessionCommitted has been written to the catalogue.
	SessionCommitted SessionStatus = "committed"
	// SessionCancelled was discarded by the admin.
	SessionCancelled SessionStatus = "cancelled"
	// SessionFailed hit an error during commit and wrote nothing.
	SessionFailed SessionStatus = "failed"
)

// RowAction is what committing will do with one staged row.
type RowAction string

const (
	// ActionInsert creates a new catalogue product.
	ActionInsert RowAction = "insert"
	// ActionUpdate refreshes a matched product.
	ActionUpdate RowAction = "update"
	// ActionSkip leaves the catalogue untouched for this row, because the
	// chosen mode excludes it or the admin deselected it.
	ActionSkip RowAction = "skip"
)

// Label renders an action in the admin's language.
func (a RowAction) Label() string {
	switch a {
	case ActionInsert:
		return i18n.TDefault("w4_mod.s_293_293")
	case ActionUpdate:
		return i18n.TDefault("w4_mod.s_294_294")
	default:
		return i18n.TDefault("w4_mod.s_295_295")
	}
}

// ImportSession is one staged import awaiting review or already committed.
type ImportSession struct {
	ID             int64           `json:"id"`
	PublicID       string          `json:"public_id"`
	OrganizationID int64           `json:"organization_id"`
	CreatedBy      *int64          `json:"created_by,omitempty"`
	Filename       string          `json:"filename"`
	FileSizeBytes  int64           `json:"file_size_bytes"`
	SourceFormat   string          `json:"source_format"`
	SheetName      string          `json:"sheet_name"`
	Delimiter      string          `json:"delimiter"`
	Status         SessionStatus   `json:"status"`
	Mode           ImportMode      `json:"import_mode"`
	Options        ImportOptions   `json:"options"`
	Overrides      LayoutOverrides `json:"layout_overrides"`
	// Structure is the file as it was read, stored so the mapping and review
	// screens render without decoding the workbook again.
	Structure FileStructure `json:"structure"`
	// Progress is the last phase the background run reported, persisted so a
	// poll still answers after a restart.
	Progress ImportProgress `json:"progress"`

	TotalRows   int `json:"total_rows"`
	ParsedRows  int `json:"parsed_rows"`
	InsertRows  int `json:"insert_rows"`
	UpdateRows  int `json:"update_rows"`
	SkipRows    int `json:"skip_rows"`
	ErrorRows   int `json:"error_rows"`
	WarningRows int `json:"warning_rows"`
	BlockCount  int `json:"block_count"`

	// NewBrands and NewCategories are the taxonomy rows this import would
	// create. They are proposals until the admin confirms; nothing is written
	// during analysis.
	NewBrands []string `json:"new_brands,omitempty"`
	// NewCategories are the category words nothing existing covers. They become
	// real rows at commit, and only when auto-creation is on.
	NewCategories []string `json:"new_categories,omitempty"`

	AICalls int `json:"ai_calls"`
	// AIMatched counts rows tied to an existing product by similarity or by the
	// model, which exact matching alone would have duplicated.
	AIMatched  int    `json:"ai_matched"`
	AIApplied  int    `json:"ai_applied"`
	AINote     string `json:"ai_note,omitempty"`
	AIFallback bool   `json:"ai_fallback"`

	ErrorMessage string     `json:"error_message,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	CommittedAt  *time.Time `json:"committed_at,omitempty"`
	ExpiresAt    time.Time  `json:"expires_at"`
}

// IsReviewable reports whether the admin can still act on this session.
//
// A run in flight is deliberately excluded: re-preparing, cancelling or
// committing while the background pass is writing staging rows would race it.
func (s *ImportSession) IsReviewable() bool {
	return s.Status == SessionDraft || s.Status == SessionReady
}

// IsProcessing reports whether a preparation run owns this session.
func (s *ImportSession) IsProcessing() bool { return s.Status == SessionProcessing }

// IsRetryable reports whether the admin can still correct the mapping and run
// this session again.
//
// A failed run is included deliberately. The upload is still on the row and the
// commonest cause of failure is a mapping that read nothing — so the fix is a
// correction on the mapping screen, not making the admin find and re-upload a
// 30 MB workbook because a checkbox was wrong.
func (s *ImportSession) IsRetryable() bool {
	return s.Status == SessionDraft || s.Status == SessionReady || s.Status == SessionFailed
}

// retryableStatuses are the states a preview or a re-run may transition from.
func retryableStatuses() []SessionStatus {
	return []SessionStatus{SessionDraft, SessionReady, SessionFailed}
}

// IsStaged reports whether there are reviewed rows waiting to be committed.
func (s *ImportSession) IsStaged() bool { return s.Status == SessionReady }

// Affected is how many catalogue rows committing would touch.
func (s *ImportSession) Affected() int { return s.InsertRows + s.UpdateRows }

// AIChange records one field an AI enrichment pass filled, so the preview can
// show the admin exactly what the model decided and why.
type AIChange struct {
	Field  string `json:"field"`
	Label  string `json:"label"`
	Value  string `json:"value"`
	Reason string `json:"reason,omitempty"`
}

// StagingRow is one parsed product held for review.
type StagingRow struct {
	ID        int64     `json:"id"`
	SessionID int64     `json:"session_id"`
	SourceRow int       `json:"source_row"`
	Block     int       `json:"block"`
	Action    RowAction `json:"action"`
	// Included is the admin's per-row switch. A deselected row is skipped at
	// commit whatever its action says.
	Included         bool        `json:"included"`
	MatchedProductID *int64      `json:"matched_product_id,omitempty"`
	MatchReason      MatchReason `json:"match_reason,omitempty"`
	// MatchedProductName and MatchedProductSKU are the catalogue's own labels
	// for the matched product, read by join at list time rather than copied at
	// staging time so they cannot go stale between preparing an import and
	// reviewing it. An admin approving an update needs to see which product is
	// about to be overwritten, and "#255741" does not tell them.
	MatchedProductName string     `json:"matched_product_name,omitempty"`
	MatchedProductSKU  string     `json:"matched_product_sku,omitempty"`
	Product            *Product   `json:"product"`
	Issues             []RowIssue `json:"issues,omitempty"`
	AIChanges          []AIChange `json:"ai_changes,omitempty"`
}

// MatchedLabel is how the review table names the product a row would update.
func (r *StagingRow) MatchedLabel() string {
	if r.MatchedProductID == nil {
		return ""
	}
	if r.MatchedProductName != "" {
		return r.MatchedProductName
	}
	return fmt.Sprintf(i18n.TDefault("w4_mod.d_111"), *r.MatchedProductID)
}

// DisplayName is the product name shown in the review table.
func (r *StagingRow) DisplayName() string {
	if r.Product == nil {
		return ""
	}
	if name := r.Product.Name.Get(i18n.AR); name != "" {
		return name
	}
	return r.Product.Name.Get(i18n.EN)
}

// HasErrors reports whether this row carries a blocking finding.
func (r *StagingRow) HasErrors() bool {
	for _, issue := range r.Issues {
		if issue.Severity == SeverityError {
			return true
		}
	}
	return false
}

// StagingFilter narrows the review table.
type StagingFilter struct {
	// Action limits to one action; empty means all.
	Action RowAction
	// OnlyIssues limits to rows carrying a warning or an error.
	OnlyIssues bool
	// OnlyAI limits to rows an AI pass changed.
	OnlyAI bool
	// Search matches the product name, SKU, or barcode.
	Search string
	Limit  int
	Offset int
}

// StagingCounts is the tally the confirmation screen promises.
type StagingCounts struct {
	Insert    int `json:"insert"`
	Update    int `json:"update"`
	Skip      int `json:"skip"`
	Errors    int `json:"errors"`
	Warnings  int `json:"warnings"`
	AIChanged int `json:"ai_changed"`
	Total     int `json:"total"`
}

// Affected is how many catalogue rows the commit would touch.
func (c StagingCounts) Affected() int { return c.Insert + c.Update }
