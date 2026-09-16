package ingest

import (
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/inventory"
	"github.com/muhiya/dawa24-store/internal/shared/productmatch"
	"github.com/muhiya/dawa24-store/internal/shared/sheet"
)

// Phase is where an import has reached. It is the vendor's own progress through
// the wizard, not a job status: 'mapping' is waiting for them, 'processing' is
// waiting for us.
type Phase string

const (
	// PhaseMapping is analysed and awaiting the vendor's column review.
	PhaseMapping Phase = "mapping"
	// PhaseSettings has a confirmed mapping and is awaiting the import rules.
	PhaseSettings Phase = "settings"
	// PhaseReview stages parsed rows for the vendor to review, edit, and manually match.
	PhaseReview Phase = "review"
	// PhaseConfirm has everything and is showing the vendor what will happen.
	PhaseConfirm Phase = "confirm"
	// PhaseProcessing is running.
	PhaseProcessing Phase = "processing"
	// PhaseCompleted finished; the results screen has the account.
	PhaseCompleted Phase = "completed"
	// PhaseFailed stopped on an error and wrote nothing further.
	PhaseFailed Phase = "failed"
	// PhaseCancelled was discarded by the vendor.
	PhaseCancelled Phase = "cancelled"
)

// LabelLang renders a phase in the specified language.
func (p Phase) LabelLang(lang string) string {
	if lang == "" {
		lang = "ar"
	}
	switch p {
	case PhaseMapping:
		return i18n.T(lang, "ingest.phase.mapping")
	case PhaseSettings:
		return i18n.T(lang, "ingest.phase.settings")
	case PhaseReview:
		return i18n.T(lang, "ingest.phase.review")
	case PhaseConfirm:
		return i18n.T(lang, "ingest.phase.confirm")
	case PhaseProcessing:
		return i18n.T(lang, "ingest.phase.processing")
	case PhaseCompleted:
		return i18n.T(lang, "ingest.phase.completed")
	case PhaseFailed:
		return i18n.T(lang, "ingest.phase.failed")
	default:
		return string(p)
	}
}

// Label renders a phase in Arabic.
func (p Phase) Label() string {
	return p.LabelLang("ar")
}

// Open reports whether the vendor can still act on the import.
func (p Phase) Open() bool {
	return p == PhaseMapping || p == PhaseSettings || p == PhaseReview || p == PhaseConfirm
}

// Terminal reports whether the import has finished, one way or another.
func (p Phase) Terminal() bool {
	return p == PhaseCompleted || p == PhaseFailed || p == PhaseCancelled
}

// Mode is the reconciliation strategy applied to the vendor's own catalogue.
type Mode string

const (
	// ModeUpsert updates the variants the file matches and adds the rest.
	ModeUpsert Mode = "update_and_add"
	// ModeAddOnly adds what is new and leaves existing variants untouched.
	ModeAddOnly Mode = "add_new_only"
	// ModeUpdateOnly refreshes what already exists and adds nothing, for a price
	// revision that must not widen the catalogue.
	ModeUpdateOnly Mode = "update_existing_only"
	// ModeReplace additionally deactivates every variant the file does not
	// mention. It is what a vendor means by "this file is my whole catalogue
	// now", and it is the only mode that can remove an offer from sale, so the
	// screen demands a second confirmation for it.
	ModeReplace Mode = "replace_catalog"
)

// ModeOption describes one strategy for the settings screen.
type ModeOption struct {
	Mode        Mode
	Icon        string
	Title       string
	Description string
	// Destructive marks a mode that can take products off sale.
	Destructive bool
}

// ModeOptions are the four strategies, in the order they are offered.
var ModeOptions = ModeOptionsForLang("ar")


// ParseMode maps a submitted value onto a mode, defaulting to the safe one.
func ParseMode(raw string) Mode {
	switch Mode(raw) {
	case ModeAddOnly:
		return ModeAddOnly
	case ModeUpdateOnly:
		return ModeUpdateOnly
	case ModeReplace:
		return ModeReplace
	default:
		return ModeUpsert
	}
}

// ModeOptionsForLang returns the four strategies in the requested language.
func ModeOptionsForLang(lang string) []ModeOption {
	if lang == "" {
		lang = "ar"
	}
	return []ModeOption{
		{
			Mode:        ModeUpsert,
			Icon:        "⚡",
			Title:       i18n.T(lang, "ingest.mode.upsert_title"),
			Description: i18n.T(lang, "ingest.mode.upsert_desc"),
		},
		{
			Mode:        ModeAddOnly,
			Icon:        "➕",
			Title:       i18n.T(lang, "ingest.mode.add_only_title"),
			Description: i18n.T(lang, "ingest.mode.add_only_desc"),
		},
		{
			Mode:        ModeUpdateOnly,
			Icon:        "🔄",
			Title:       i18n.T(lang, "ingest.mode.update_only_title"),
			Description: i18n.T(lang, "ingest.mode.update_only_desc"),
		},
		{
			Mode:        ModeReplace,
			Icon:        "🗂️",
			Title:       i18n.T(lang, "ingest.mode.replace_title"),
			Description: i18n.T(lang, "ingest.mode.replace_desc"),
			Destructive: true,
		},
	}
}

// LabelLang renders a mode in the requested language.
func (m Mode) LabelLang(lang string) string {
	for _, o := range ModeOptionsForLang(lang) {
		if o.Mode == m {
			return o.Title
		}
	}
	return string(m)
}

// Label renders a mode in Arabic.
func (m Mode) Label() string {
	return m.LabelLang("ar")
}

// Destructive reports whether the mode can take products off sale.
func (m Mode) Destructive() bool { return m == ModeReplace }

// A vendor's file can no longer introduce a product to the shared catalogue.
//
// It used to, as `UnmatchedCreate`: a row the catalogue did not recognise was
// registered as a new master product and the vendor's variant was linked to it.
// The intent was that a new supplier's first upload should not be mostly
// refused. The result was that the shared catalogue became whatever any
// supplier happened to type — thousands of near-duplicate entries, each one a
// product no other vendor's row could ever match, which is the opposite of what
// a shared catalogue is for.
//
// The catalogue is now the administrator's alone. A vendor's import matches
// against it and prices what it finds; what it cannot match is reported back to
// the vendor, with the candidates the engine considered, so they can correct
// their file or ask for the product to be added. Nothing is invented.

// Settings are the rules the vendor sets before processing starts.
//
// Every default is the conservative reading. An importer whose defaults widen
// the blast radius — treating a blank cell as zero stock, accepting a weak
// match, overwriting balances a price list said nothing about — is one whose
// first run has to be undone by hand.
type Settings struct {
	WarehouseID int64  `json:"warehouse_id"`
	BranchID    *int64 `json:"branch_id,omitempty"`

	Mode      Mode                `json:"mode"`
	StockMode inventory.StockMode `json:"stock_mode"`

	// Duplicates decides what a repeated identity inside one file means.
	Duplicates productmatch.DuplicatePolicy `json:"duplicates"`

	// MinMatchScore is the similarity at or above which a match is applied
	// without asking. Below it the row is held for review and is NOT written by
	// the commit, however well it scored — a suggestion the engine would not
	// stand behind is the vendor's to accept, one row at a time or in bulk from
	// the review screen, and never by default.
	MinMatchScore float64 `json:"min_match_score"`
	// UseAI lets a model settle the rows the deterministic engine could not.
	//
	// It is a tier, not a mode: everything the exact and similarity tiers
	// resolved is already decided before this runs, and the model only ever
	// chooses among candidates the engine retrieved. That is what keeps it
	// cheap — a nine-thousand-row file reaches it with tens of rows, not nine
	// thousand — and what makes switching it off change how much is matched
	// rather than whether the import works.
	UseAI bool `json:"use_ai"`
	// TrustSupplierCode lets the vendor's own item code match the shared
	// catalogue's. Off by default: a vendor's "951" is their internal numbering.
	//
	// It has no effect unless a كود صنف column was mapped in step one. The two
	// are set in different steps, so a vendor can switch this on and then go
	// back and unmap the column; the run resolves both together rather than
	// trusting the toggle alone.
	TrustSupplierCode bool
	// TrustBarcode lets the file's barcode settle a match on its own.
	//
	// Off by default, and it used not to be a choice: the barcode tier ran on
	// every import, ahead of the name and the dose, and any eight-digit value
	// with a single catalogue hit won outright. A vendor whose "barcode" column
	// holds their own warehouse numbering got confident links to unrelated
	// medicines with nothing in the review screen to mark them as doubtful.
	TrustBarcode bool
	// CodeIsCatalogCode says the mapped code column holds دوا 24's own codes
	// rather than the vendor's internal numbering. Only then is a code hit
	// accepted without the name agreeing too.
	CodeIsCatalogCode bool `json:"trust_supplier_code"`

	BlankQuantityIsZero bool `json:"blank_quantity_is_zero"`
	RejectExpired       bool `json:"reject_expired"`

	// The pharmaceutical-form and concentration inference switches used to live
	// here. They were main-catalogue vocabulary offered on a vendor screen: a
	// vendor import writes catalog.product_variants, which has no column for
	// either, so whichever way the vendor set them nothing was written. They
	// still happen — inside the matcher, which reads the strength and the form
	// straight out of the product name in order to veto a 500 mg row matched to
	// a 1 g product — but that is the engine's business, not a setting.
	//
	// The default-quantity box went the same way. A vendor whose file states no
	// quantity is telling us the balance is unknown; inventing a uniform 50 for
	// nine thousand rows is how a supplier's catalogue comes to advertise stock
	// nobody holds. BlankQuantityIsZero is the honest version of that question
	// and it is still here.

	DefaultMinOrderQty  int  `json:"default_min_order_qty"`
	DefaultMinThreshold int  `json:"default_min_threshold"`
	MarkNegotiable      bool `json:"mark_negotiable"`
	// PublishImmediately puts newly imported variants on sale at once. On by
	// default so vendor imports are ready and visible for ordering upon completion.
	//
	// It governs NEW variants only. An update leaves the status alone, so a
	// vendor running a routine price refresh does not delist their live catalogue.
	PublishImmediately bool `json:"publish_immediately"`
	// RecordRows keeps a per-row outcome ledger. On by default; a vendor
	// importing a hundred thousand rows may turn it off.
	RecordRows bool `json:"record_rows"`
}

// DefaultSettings are what the settings screen starts on.
func DefaultSettings() Settings {
	return Settings{
		Mode:                ModeUpsert,
		StockMode:           inventory.StockReplace,
		Duplicates:          productmatch.DuplicateLastWins,
		MinMatchScore:       productmatch.DefaultMinStrong,
		UseAI:               false,
		BlankQuantityIsZero: true,
		RejectExpired:       false,
		DefaultMinOrderQty:  1,
		DefaultMinThreshold: 0,
		PublishImmediately:  true,
		RecordRows:          true,
	}
}

// Normalize fills in anything a submitted form left blank or out of range.
func (s Settings) Normalize() Settings {
	if s.Mode == "" {
		s.Mode = ModeUpsert
	}
	if s.StockMode == "" {
		s.StockMode = inventory.StockReplace
	}
	if s.Duplicates == "" {
		s.Duplicates = productmatch.DuplicateLastWins
	}
	if s.MinMatchScore <= 0 {
		s.MinMatchScore = productmatch.DefaultMinStrong
	} else {
		// The floor is the shared review floor rather than an arbitrary 0.05.
		// A vendor may widen what counts as a confident match, but not below
		// the point at which the engine stops believing its own answer — that
		// is not a preference, it is a way of importing the review queue.
		s.MinMatchScore = min(max(s.MinMatchScore, productmatch.DefaultMinReview), 1)
	}
	if s.DefaultMinOrderQty <= 0 {
		s.DefaultMinOrderQty = 1
	}
	if s.DefaultMinThreshold < 0 {
		s.DefaultMinThreshold = 0
	}
	return s
}

// Session is one vendor catalogue import.
type Session struct {
	ID             int64  `json:"id"`
	PublicID       string `json:"public_id"`
	OrganizationID int64  `json:"organization_id"`
	CreatedBy      *int64 `json:"created_by,omitempty"`

	Filename      string `json:"filename"`
	FileSizeBytes int64  `json:"file_size_bytes"`

	Phase    Phase        `json:"phase"`
	Source   sheet.Source `json:"source"`
	Settings Settings     `json:"settings"`
	// Overrides are the vendor's column corrections, keyed by zero-based column
	// index. They are the only part of the analysis that is persisted: the rest
	// is re-derived from the file, which is safe because the engine is
	// deterministic and means a stored mapping can never drift from the one the
	// processing run actually uses.
	Overrides map[int]productmatch.Field `json:"overrides,omitempty"`
	// Mapping is the confirmed reading, stored once the vendor accepts it, for
	// the results screen and the audit trail.
	Mapping *MappingSnapshot `json:"mapping,omitempty"`

	Stats    productmatch.Stats   `json:"stats"`
	Findings []productmatch.Issue `json:"findings,omitempty"`
	// AI is what the enhancement stage did, and it is the vendor's answer to
	// "was the smart matching worth switching on?". Zero on an import that
	// never ran it.
	AI AIStats `json:"ai"`

	TotalRows     int `json:"total_rows"`
	InsertedRows  int `json:"inserted_rows"`
	UpdatedRows   int `json:"updated_rows"`
	SkippedRows   int `json:"skipped_rows"`
	ErrorRows     int `json:"error_rows"`
	MatchedRows   int `json:"matched_rows"`
	ReviewRows    int `json:"review_rows"`
	UnmatchedRows int `json:"unmatched_rows"`
	// CreatedProducts is history only. Imports no longer add anything to the
	// shared catalogue, so it is zero for every run from now on; older runs keep
	// the count they actually made.
	CreatedProducts int `json:"created_products"`

	ProgressPercent int    `json:"progress_percent"`
	ProgressNote    string `json:"progress_note"`
	ErrorMessage    string `json:"error_message,omitempty"`

	StartedAt   *time.Time `json:"started_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	ExpiresAt   time.Time  `json:"expires_at"`
}

// Affected is how many of the vendor's variants the run touched.
func (s *Session) Affected() int { return s.InsertedRows + s.UpdatedRows }
