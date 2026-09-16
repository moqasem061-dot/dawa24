package smartorder

import "context"

// Repository is the persistence boundary for smart ordering.
//
// Two shapes recur and are deliberate.
//
// The bulk methods (`InsertLines`, `ReplaceCandidates`, `UpsertSelections`) take
// slices rather than single rows. A ten-thousand-row import that saves one row
// at a time spends its whole budget on round trips, and FR-017a forbids per-row
// work. Any caller that finds itself looping over one of the single-row methods
// is doing something the interface is trying to prevent.
//
// The read methods that feed matching (`ResolveByCodes`, `ResolveBySaving`,
// `ResolveByLearned`) take every unresolved key in the file at once and return a
// map. Same reason.
type Repository interface {
	// --- runs -------------------------------------------------------------

	CreateRun(ctx context.Context, r *Run, cfg *Config) error
	GetRunByPublicID(ctx context.Context, orgID int64, publicID string) (*Run, error)
	GetRunByID(ctx context.Context, orgID, id int64) (*Run, error)
	ListRuns(ctx context.Context, orgID int64, limit, offset int) ([]*Run, error)
	UpdateRunStatus(ctx context.Context, id int64, status RunStatus, step int, failureReason string) error
	UpdateRunStats(ctx context.Context, r *Run) error

	// FinalizeRun writes the order id and finalized_at, and MUST refuse when
	// finalized_at is already set. The guard lives in the same statement as the
	// write so a double submit cannot slip between a check and an update.
	FinalizeRun(ctx context.Context, id, orderID int64) error

	// --- configuration ----------------------------------------------------

	GetConfig(ctx context.Context, runID int64) (*Config, error)
	GetProfile(ctx context.Context, orgID int64) (*Profile, error)
	SaveProfile(ctx context.Context, p *Profile) error

	// --- the uploaded file -------------------------------------------------
	//
	// Held only between step 1 and step 2, then deleted. See migration 127.

	SaveRunFile(ctx context.Context, runID, orgID int64, filename string, content []byte) error
	GetRunFile(ctx context.Context, runID, orgID int64) ([]byte, string, error)
	DeleteRunFile(ctx context.Context, runID, orgID int64) error

	// --- column mapping ---------------------------------------------------

	SaveMapping(ctx context.Context, m *Mapping) error
	GetMapping(ctx context.Context, runID int64) (*Mapping, error)

	// --- lines ------------------------------------------------------------

	InsertLines(ctx context.Context, lines []*Line) error
	ListLines(ctx context.Context, runID int64, f LineFilter) ([]*Line, int, error)
	FilterCounts(ctx context.Context, runID int64) (FilterCounts, error)
	// BlockedCounts tallies what will not be ordered, by reason, for the
	// collapsed section the results screen leads with.
	BlockedCounts(ctx context.Context, runID int64) (BlockedCounts, error)
	GetLine(ctx context.Context, orgID, lineID int64) (*Line, error)
	UpdateLines(ctx context.Context, lines []*Line) error
	UpdateLineQuantity(ctx context.Context, orgID, lineID int64, qty float64) error
	SetDefaultQuantity(ctx context.Context, orgID, runID int64, lineIDs []int64, qty float64) error

	// --- matching -----------------------------------------------------------

	// ResolveByCodes matches SKUs and barcodes for the whole file in one query.
	ResolveByCodes(ctx context.Context, skus, barcodes []string) (map[string]int64, error)
	// ResolveBySaving looks up the buyer's Saving Products list by normalised
	// name and SKU. Only called when the toggle is on.
	ResolveBySaving(ctx context.Context, orgID int64, names, skus []string) (map[string]int64, error)
	// ResolveByLearned applies corrections the buyer has confirmed before.
	ResolveByLearned(ctx context.Context, orgID int64, names []string) (map[string]int64, error)
	// ResolveByAlias applies aliases confirmed against the shared catalogue.
	ResolveByAlias(ctx context.Context, names []string) (map[string]int64, error)
	// ResolveByExactName matches normalised names directly against the
	// catalogue's Arabic and English product names, prioritizing the configured
	// language preference. This is the tier that answers "the pharmacy typed the
	// same name the catalogue has" — the most common case.
	ResolveByExactName(ctx context.Context, names []string, matchLang string) (map[string]int64, error)
	// ResolveByFuzzyDB uses PostgreSQL's pg_trgm similarity to match names
	// that share no exact word but have similar character sequences. This
	// catches transliteration variants and typos that the token-based
	// in-memory scorer misses entirely.
	ResolveByFuzzyDB(ctx context.Context, names []string, matchLang string) (map[string]int64, error)
	// ResolveByContains matches lines where the line's name is contained within
	// the catalogue name (or vice versa), provided the name is sufficiently
	// specific and matches uniquely to one product.
	ResolveByContains(ctx context.Context, names []string, matchLang string) (map[string]int64, error)

	// LoadMatchIndex loads the catalogue projection the in-memory matcher scores
	// against. Roughly 30k rows and a few megabytes; see productmatch.Index.
	LoadMatchIndex(ctx context.Context) ([]IndexedProduct, error)

	// ProductNames resolves catalogue product names for display, in one query.
	//
	// A results table that prints "#255741" has told the buyer nothing they can
	// check. They need the catalogue's own name for the product the row was
	// matched to, because reading it beside what they typed is the only way
	// they can tell a right match from a wrong one.
	ProductNames(ctx context.Context, ids []int64) (map[int64]string, error)

	// SaveLearnedMapping records a buyer correction so the same text resolves
	// automatically next time.
	SaveLearnedMapping(ctx context.Context, orgID int64, rawName string, productID int64) error

	// SaveAlias records a confirmed name for a catalogue product.
	//
	// An alias from an AI decision is written with source 'ai_confirmed' and is
	// deliberately NOT consulted by the deterministic alias tier: it is promoted
	// only after a person accepts it. AI output must never become ground truth on
	// its own — that is how one confident mistake propagates to every buyer.
	SaveAlias(ctx context.Context, productID int64, alias, source string, confidence float64) error

	// --- supplier offers ---------------------------------------------------

	// LoadOffers returns every vendor variant of the given products, with price,
	// stock, follow state and the institutional restriction, in ONE query.
	LoadOffers(ctx context.Context, buyerOrgID int64, productIDs []int64) ([]Offer, error)

	ReplaceCandidates(ctx context.Context, lineID int64, candidates []Candidate) error
	// ReplaceRunCandidates rewrites the candidate sets of a whole run in one
	// pass and returns them with their assigned ids, ordered eligible-first then
	// cheapest — the order the selection expects. The per-line method above is
	// for a single correction; the pipeline must never loop over it.
	ReplaceRunCandidates(ctx context.Context, runID int64, byLine map[int64][]Candidate) (map[int64][]Candidate, error)
	ListCandidates(ctx context.Context, orgID, lineID int64) ([]Candidate, error)
	GetCandidate(ctx context.Context, orgID, candidateID int64) (*Candidate, error)

	UpsertSelections(ctx context.Context, selections []*Selection) error
	GetSelection(ctx context.Context, orgID, lineID int64) (*Selection, error)
	// ListSelectionsByRun loads every selection of a run in one query, keyed by
	// line id. Recalculation and finalisation walk every orderable line, and
	// asking per line is one round trip per row of the buyer's file.
	ListSelectionsByRun(ctx context.Context, orgID, runID int64) (map[int64]*Selection, error)
	DeleteSelection(ctx context.Context, orgID, lineID int64) error

	// --- decision cache -----------------------------------------------------

	LookupDecisions(ctx context.Context, keys []string) (map[string]CachedDecision, error)
	SaveDecisions(ctx context.Context, decisions []CachedDecision) error

	// --- progress -----------------------------------------------------------

	AppendEvent(ctx context.Context, e *Event) error
	ListEvents(ctx context.Context, runID int64, afterID int64) ([]*Event, error)
}

// RunClaimer is an optional atomic claim capability. Implementations that
// provide it prevent two dispatchers from starting the same queued run.
type RunClaimer interface {
	ClaimRun(ctx context.Context, orgID, runID int64) (bool, error)
}

// Mapping is the confirmed association between file columns and target fields.
type Mapping struct {
	RunID          int64              `json:"run_id"`
	OrganizationID int64              `json:"organization_id"`
	HeaderRow      int                `json:"header_row"`
	Fields         map[int]string     `json:"mapping"`
	Detected       map[int]string     `json:"detected"`
	Confidence     map[string]float64 `json:"confidence"`
	UserOverridden bool               `json:"user_overridden"`
	Confirmed      bool               `json:"confirmed"`
}

// RequiredFields are the columns without which a run cannot start. Any one of
// them identifies a product; quantity is optional because the default quantity
// exists precisely to cover a file that does not carry one.
var RequiredFields = []string{"product_name", "sku", "barcode"}

// Valid reports whether the mapping identifies products, and what is missing.
func (m *Mapping) Valid() (bool, []string) {
	for _, f := range m.Fields {
		for _, req := range RequiredFields {
			if f == req {
				return true, nil
			}
		}
	}
	return false, RequiredFields
}

// Column returns the file column assigned to a target field.
func (m *Mapping) Column(field string) (int, bool) {
	for col, f := range m.Fields {
		if f == field {
			return col, true
		}
	}
	return 0, false
}

// LineFilter narrows the results table.
//
// All is for the callers that must see every line of the run rather than a
// page of it: the matching pipeline, the total recalculation, and the
// finalisation that turns lines into an order. Paging those is not a display
// choice, it is a correctness bug — a run of 900 rows whose pipeline reads 200
// reports 200 totals, matches 200 products, and places an order missing 700
// items, with nothing anywhere saying so.
type LineFilter struct {
	Outcome    string
	MatchGroup string // "all", "matched", "unmatched", "review"
	Method     string
	Search     string
	SortBy     string // "row", "name", "matched_name", "method", "confidence", "qty", "outcome"
	SortOrder  string // "asc", "desc"
	Limit      int
	Offset     int
	// All disables paging entirely; Limit and Offset are then ignored.
	All bool
}

// IndexedProduct is the catalogue projection the matcher scores against. It
// mirrors productmatch.MasterProduct without importing it, so the repository
// interface stays free of matching concerns.
type IndexedProduct struct {
	ID            int64
	NameAR        string
	NameEN        string
	SKU           string
	Barcode       string
	Scientific    string
	DosageForm    string
	Concentration string
	Unit          string
	Manufacturer  string
}

// Offer is a vendor's variant as loaded from the authoritative tables.
//
// Read from catalog.product_variants joined to inventory.stocks — never from
// catalog.product_index's stock columns, which were empty for every row in
// production and are the reason the previous attempt at this feature returned
// no suppliers for anything.
type Offer struct {
	ProductID     int64
	VariantID     int64
	VendorOrgID   int64
	BranchID      *int64
	PriceMinor    int64
	DiscountBps   int64
	Unit          string
	MinOrderQty   int
	StockQty      int
	IsFollowed    bool
	VendorActive  bool
	ProductActive bool
	// InstitutionalWorkIDs empty means unrestricted, which in Simple mode is
	// visible to everyone.
	InstitutionalWorkIDs []int64
	// VariantBranchID is the branch the variant itself names, before any
	// fallback. BranchID above is the resolved one — it falls back to the
	// supplier's main branch so a candidate always has somewhere to ship from —
	// and the two differ in exactly the case the Corporate Operations rule cares
	// about: a variant naming no branch is satisfiable from ANY branch of the
	// supplier, so every branch's institutional works count. Using the
	// fallen-back id there would refuse a supplier whose main branch lacks the
	// works while another branch has them.
	VariantBranchID *int64
}

// CachedDecision is a reusable adjudication result.
type CachedDecision struct {
	Key             string
	NormName        string
	ChosenProductID *int64
	Confidence      float64
	Reason          string
	PromptVersion   string
	Scope           string
	Source          string
}
