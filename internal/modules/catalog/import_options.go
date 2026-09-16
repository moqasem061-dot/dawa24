package catalog

import "github.com/muhiya/dawa24-store/internal/shared/productmatch"

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
	// DefaultCategoryID is applied to every product that ends without one,
	// including when AI is off. Zero leaves the column null.
	DefaultCategoryID int64 `json:"default_category_id,omitempty"`
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
		UseAI:                false,
		MinMatchScore:        productmatch.DefaultMinStrong,
	}
}

// WantsEnrichment reports whether any field-filling switch is on.
func (o ImportOptions) WantsEnrichment() bool {
	return o.AssignCategory || o.AssignDosageForm || o.AssignScientificName
}
