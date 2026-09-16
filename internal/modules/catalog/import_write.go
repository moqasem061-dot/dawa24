package catalog

import "fmt"
import "github.com/muhiya/dawa24-store/internal/shared/i18n"

// Result types for the bulk write, shared between the repository that produces
// them and the admin screen that renders them.

// WriteAction records what happened to one product in the catalogue.
type WriteAction string

const (
	// ActionInserted means a new catalogue row was created.
	ActionInserted WriteAction = "inserted"
	// ActionUpdated means an existing row was matched and refreshed.
	ActionUpdated WriteAction = "updated"
)

// MatchReason records how an incoming row was tied to an existing product. It
// is shown in the report because "matched by name" deserves more scrutiny from
// the admin than "matched by barcode".
type MatchReason string

const (
	MatchNone    MatchReason = ""
	MatchSKU     MatchReason = "sku"
	MatchBarcode MatchReason = "barcode"
	MatchName    MatchReason = "name"
	// MatchAI is a match a model adjudicated between similar candidates.
	MatchAI MatchReason = "ai"
	// MatchDisputed is a match the similarity tier applied and the model would
	// not confirm.
	//
	// The product is kept — the administrator needs to see which entry was
	// proposed in order to judge it — and the row is excluded from the commit.
	// This importer overwrites the catalogue entry every pharmacy on the
	// platform reads, so where two independent methods disagree the right
	// answer is neither of them: it is a person.
	MatchDisputed MatchReason = "disputed"
)

// MatchLabels renders a match reason in the admin's language.
var MatchLabels = map[MatchReason]string{
	MatchSKU:      i18n.TDefault("w4_mod.s_332_332"),
	MatchBarcode:  i18n.TDefault("w4_mod.s_333_333"),
	MatchName:     i18n.TDefault("w4_mod.s_334_334"),
	MatchSimilar:  i18n.TDefault("w4_mod.s_335_335"),
	MatchAI:       i18n.TDefault("w4_mod.s_336_336"),
	MatchDisputed: "تعارض بين المطابقة الآلية والمراجعة الذكية — يلزم القرار يدوياً",
}

// WriteFailure identifies one product the database refused, by its position in
// the batch the caller submitted, so the caller can name the spreadsheet row.
type WriteFailure struct {
	Index  int    `json:"index"`
	Name   string `json:"name"`
	SKU    string `json:"sku,omitempty"`
	Reason string `json:"reason"`
}

// BulkWriteResult reports what a bulk import did to the catalogue.
type BulkWriteResult struct {
	Inserted int
	Updated  int
	// BrandsCreated and CategoriesCreated count the taxonomy rows this import
	// added. Both are zero on a re-import of the same file: an existing row is
	// always reused, never duplicated.
	BrandsCreated     int
	CategoriesCreated int
	// Matches maps a batch index to how that product was matched, for the rows
	// that updated an existing product rather than inserting a new one.
	Matches map[int]MatchReason
	// Failures lists the rows the database refused. When it is non-empty the
	// whole transaction was rolled back and nothing was written.
	Failures []WriteFailure
}

// Total is the number of catalogue rows the import touched.
func (r BulkWriteResult) Total() int { return r.Inserted + r.Updated }

// Error renders the failures as a single message. It is deliberately concrete —
// the row, the product, and what PostgreSQL objected to — because the previous
// importer surfaced "bulk batch close failed" with no indication of which of
// nine thousand rows was at fault.
func (r BulkWriteResult) Error() string {
	if len(r.Failures) == 0 {
		return ""
	}
	msg := fmt.Sprintf(i18n.TDefault("w4_mod.d_337"), len(r.Failures))
	shown := r.Failures
	if len(shown) > 3 {
		shown = shown[:3]
	}
	for _, f := range shown {
		msg += fmt.Sprintf(i18n.TDefault("w4_mod.s_s_113"), f.Name, f.Reason)
	}
	if len(r.Failures) > len(shown) {
		msg += fmt.Sprintf(i18n.TDefault("w4_mod.d_114"), len(r.Failures)-len(shown))
	}
	return msg
}

// ExistingMatch ties an incoming row to a product already in the catalogue.
type ExistingMatch struct {
	ProductID int64       `json:"product_id"`
	Reason    MatchReason `json:"reason"`
}

// BulkWriteOptions govern what a bulk write may create alongside the products.
//
// They exist because "link to an existing manufacturer" and "invent a new one"
// are different permissions. An import always reuses a taxonomy row it finds;
// these say whether it may add one it does not.
type BulkWriteOptions struct {
	CreateBrands     bool
	CreateCategories bool
}
