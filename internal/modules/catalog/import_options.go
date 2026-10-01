package catalog

import (
	"os"
	"strconv"
	"strings"

	"github.com/moqasem061-dot/dawa24/internal/shared/productmatch"
)

// What an administrator decides before a main-catalogue import runs.
//
// Split out of import_session.go, which had grown past the 400-line limit
// AGENTS.md sets. The session is a state machine; these are the settings it
// carries, and they are read by import_enrich.go and import_match.go rather
// than by the state machine itself.

// ImportOptions are the enrichment switches the admin sets before processing.
//
// Each one is off-by-default where it writes something the file did not say.
// An importer that invents a category for nine thousand products because a
// checkbox defaulted to on is worse than one that leaves the column empty.
type ImportOptions struct {
	// AutoCreateBrands registers a manufacturer named in the file as a brand
	// when the catalogue has no matching one.
	AutoCreateBrands bool `json:"auto_create_brands"`
	// AssignCategory fills catalog.products.category_id.
	AssignCategory bool `json:"assign_category"`
	// AutoCreateCategories registers a category named in the file that the
	// catalogue does not have. Linking to an existing one never needs it.
	AutoCreateCategories bool `json:"auto_create_categories"`
	// AssignDosageForm fills the pharmaceutical form.
	AssignDosageForm bool `json:"assign_dosage_form"`
	// AssignScientificName fills the generic name.
	AssignScientificName bool `json:"assign_scientific_name"`
	// DownloadImages downloads and processes external image links (watermark, compress, store).
	DownloadImages bool `json:"download_images"`
	// UseAI lets the Gateway settle what the deterministic rules could not:
	// which column is which field, what a category or form word means, and —
	// the one that decides the match rate — which catalogue product an
	// unresolved row is.
	//
	// It is off by default like every other switch here. The deterministic
	// engine — exact identifiers, then similarity scoring — is what an import
	// must be judged on, and an admin diagnosing a bad match rate needs to see
	// that engine's own result first. AI is turned on deliberately, for the
	// file that needs it.
	UseAI bool `json:"use_ai"`
	// MatchRevision tracks the revision of matching decisions across reruns for optimistic locking.
	MatchRevision int `json:"match_revision,omitempty"`
	// DefaultCategoryID is applied to every product that ends without one,
	// including when AI is off. Zero leaves the column null.
	DefaultCategoryID int64 `json:"default_category_id,omitempty"`
	// MergeDuplicateRows folds rows of the uploaded file that describe the same
	// product into one staged row.
	//
	// Off by default, because it was removed on purpose: every line of the
	// administrator's file reaching the review screen is what lets them SEE a
	// distributor's four-lines-per-product export rather than be told about it
	// afterwards.
	//
	// It exists as a switch because leaving it off has a cost at the other end.
	// Two rows the catalogue resolves to one product both write to it, and the
	// later one wins with whatever it happens to carry; two rows the catalogue
	// has never seen both insert, and the master catalogue gains a duplicate
	// that every later import then has to choose between. Turning this on
	// merges only rows that nothing distinguishes — a different strength, form
	// or line extension keeps them apart — and the count is reported on the
	// review screen, so a fold is visible rather than silent.
	MergeDuplicateRows bool `json:"merge_duplicate_rows"`
	// MinMatchScore is the platform-wide "أقل نسبة مطابقة" control, 0–1.
	//
	// Same name, same default and same meaning as the vendor import, the
	// saving-products import and the smart order. Zero means the shared
	// default. What it governs here is the corroborated floor only — see
	// matchFloors in import_match.go, and the reason the bare-name floor is not
	// the administrator's to lower.
	MinMatchScore float64 `json:"min_match_score,omitempty"`
}

// Normalize fills in anything a submitted form left blank or out of range.
func (o ImportOptions) Normalize() ImportOptions {
	if o.MinMatchScore <= 0 {
		o.MinMatchScore = productmatch.DefaultMinStrong
	}
	o.MinMatchScore = min(max(o.MinMatchScore, productmatch.DefaultMinReview), 1)
	if !o.AssignCategory {
		o.AutoCreateCategories = false
	}
	return o
}

// DefaultImportOptions are what the upload screen starts on.
//
// Category assignment and the AI assist are both on, which is a deliberate
// reversal. They were off, on the reasoning that an importer should be judged
// on its deterministic engine first — sound in principle, and in practice what
// it produced was a catalogue of nineteen thousand products with a null
// category, because nobody turns on a switch whose absence is invisible. The
// category IS the catalogue's organising column; leaving it empty is not the
// conservative choice, it is the broken one.
//
// What stays off is everything that invents a row rather than filling a column:
// AutoCreateCategories and AutoCreateBrands. Linking to a category that exists
// is safe and reversible; minting one from a supplier's spelling is neither.
func DefaultImportOptions() ImportOptions {
	return ImportOptions{
		AssignDosageForm:     true,
		AssignCategory:       true,
		AssignScientificName: true,
		DownloadImages:       true,
		UseAI:                false,
		MinMatchScore:        productmatch.DefaultMinStrong,
	}
}

// WantsEnrichment reports whether any field-filling switch is on.
func (o ImportOptions) WantsEnrichment() bool {
	return o.AssignCategory || o.AssignDosageForm || o.AssignScientificName
}

// MaxImportBatch returns the maximum number of items allowed in a single catalog import.
// It reads from the CATALOG_MAX_IMPORT_ROWS (or MAX_IMPORT_ROWS) environment variable.
// If the variable is unset, empty, or <= 0, it returns 0, meaning unlimited imports.
func MaxImportBatch() int {
	val := os.Getenv("CATALOG_MAX_IMPORT_ROWS")
	if val == "" {
		val = os.Getenv("MAX_IMPORT_ROWS")
	}
	if val != "" {
		if n, err := strconv.Atoi(strings.TrimSpace(val)); err == nil { // nolint:errswallow: an unparseable env value keeps the default
			if n > 0 {
				return n
			}
			return 0
		}
	}
	return 0
}
