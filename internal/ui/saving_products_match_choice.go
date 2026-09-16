package ui

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/muhiya/dawa24-store/internal/shared/productmatch"
)

// The unified matching choice, shared with the other three importers.
//
// The four exclusive "strategies" above are what this screen used to offer:
// pick الاسم فقط or الباركود فقط or الكود والباركود, and picking an identifier
// switched name scoring off entirely (MinStrong 1.01). That is not a priority
// order, it is four different engines, and it is why the same file matched
// differently here than in the vendor import.
//
// The platform's rule is one order everywhere: the name decides, the AI tier
// settles what the name could not, and an identifier is an optional extra tier
// the user switches on when they know their codes are the platform's codes.
// MatchChoice is that rule expressed as data.

// MatchChoice is the unified matching configuration for one import run.
type MatchChoice struct {
	// MinScore is the similarity at or above which a name match is applied
	// without asking. Zero means productmatch.DefaultMinStrong.
	MinScore float64
	// ByProductID accepts a دوا 24 product id stated in the file. It is on by
	// default because an id the file states is not a guess: it is validated
	// against the catalogue before it is believed, and a stale id simply falls
	// through to the name.
	ByProductID bool
	// ByBarcode lets a mapped GTIN settle the match on its own. Off by default.
	ByBarcode bool
	// ByCode lets the file's item code settle the match. Off by default; it
	// still needs the name to agree unless CodeIsCatalogCode says the column
	// holds دوا 24's own codes.
	ByCode bool
	// CodeIsCatalogCode lifts the name corroboration on a code hit.
	CodeIsCatalogCode bool
}

// DefaultMatchChoice is what every import screen starts on: name first, AI
// second, no identifier tier but the file's own stated product id.
func DefaultMatchChoice() MatchChoice {
	return MatchChoice{
		MinScore:    productmatch.DefaultMinStrong,
		ByProductID: true,
	}
}

// Normalize clamps a submitted choice into the range the engine accepts.
func (c MatchChoice) Normalize() MatchChoice {
	if c.MinScore <= 0 {
		c.MinScore = productmatch.DefaultMinStrong
	}
	c.MinScore = min(max(c.MinScore, productmatch.DefaultMinReview), 1)
	if !c.ByCode {
		c.CodeIsCatalogCode = false
	}
	return c
}

// MatchUnified resolves one row under the platform's single matching order.
func (e *SavingProductMatchEngine) MatchUnified(
	c MatchChoice, productID *int64, rawSKU, rawName string,
) MatchResult {
	unlinked := MatchResult{MatchType: "unlinked"}
	if e == nil || e.index == nil || e.index.Size() == 0 {
		return unlinked
	}
	c = c.Normalize()

	// A product id the file states outright, checked against the catalogue
	// rather than trusted. A stale export names ids that no longer exist, and
	// those fall through to the name rather than resolving to nothing.
	if c.ByProductID && productID != nil && *productID > 0 && e.known[*productID] {
		id := *productID
		return MatchResult{ProductID: &id, MatchType: "direct_id", Confidence: 1.0}
	}

	row := &productmatch.Row{Name: strings.TrimSpace(rawName)}
	opts := e.opts
	opts.MinStrong = c.MinScore
	opts.MinReview = min(productmatch.DefaultMinReview, c.MinScore)
	opts.TrustBarcode = c.ByBarcode
	opts.TrustSupplierCode = c.ByCode
	opts.CodeIsAuthoritative = c.CodeIsCatalogCode

	// One column, two possible readings. The import screen offers a single
	// "كود الصنف / SKU / الباركود" column, so which tier may see it is the
	// user's answer, not a guess: a pharmacy's internal item number in the
	// barcode slot is how a row gets a confident link to an unrelated medicine.
	if code := strings.TrimSpace(rawSKU); code != "" {
		if c.ByBarcode {
			row.Barcode = code
		}
		if c.ByCode {
			row.SKU = code
		}
	}
	if row.Name == "" && row.SKU == "" && row.Barcode == "" {
		return unlinked
	}

	res := e.index.Match(row, opts)
	if !res.Matched() || !res.Level.Settled() {
		return unlinked
	}
	return MatchResult{
		ProductID:  &res.ProductID,
		MatchType:  savingMatchType(res.Level),
		Confidence: res.Score,
	}
}

// ParseMatchChoice reads the unified matching settings off a submitted form.
//
// Absent fields mean the defaults, not "off": a client that posts only a file —
// the drag-and-drop path, or an old open tab — gets name matching at the
// platform default score with the AI tier available, which is the behaviour
// every screen describes.
func ParseMatchChoice(r *http.Request) MatchChoice {
	c := DefaultMatchChoice()
	if raw := strings.TrimSpace(r.FormValue("min_match_score")); raw != "" {
		if pct, err := strconv.ParseFloat(raw, 64); err == nil && pct > 0 {
			if pct > 1 {
				pct /= 100
			}
			c.MinScore = pct
		}
	}
	c.ByBarcode = matchFlag(r, "match_by_barcode")
	c.ByCode = matchFlag(r, "match_by_code")
	c.CodeIsCatalogCode = matchFlag(r, "code_is_catalog_code")
	if r.FormValue("match_by_product_id") != "" {
		c.ByProductID = matchFlag(r, "match_by_product_id")
	}

	// Legacy: an open tab still posting one of the four exclusive strategies.
	switch MatchStrategy(strings.TrimSpace(r.FormValue("match_strategy"))) {
	case StrategyBarcodeOnly:
		c.ByBarcode = true
	case StrategySKUBarcode, StrategySKUOnly:
		c.ByBarcode, c.ByCode = true, true
	case StrategyIDOnly:
		c.ByProductID = true
	}
	return c.Normalize()
}

// ParseUseAI reads the AI switch. Absent means on: the AI tier is a default of
// the platform's matching order, and a form that forgot the field must not
// silently downgrade the match rate.
func ParseUseAI(r *http.Request) bool {
	v := strings.TrimSpace(r.FormValue("use_ai"))
	if v == "" {
		return true
	}
	return v == "1" || v == "on" || v == "true"
}

// ParseUseAIFromWizard reads the AI switch off a form that actually drew it.
//
// An unticked checkbox posts nothing at all. ParseUseAI reads "nothing" as "the
// client never mentioned it" and defaults the tier on, which is right for the
// drag-and-drop path — that posts a file and little else — and wrong for a
// screen that rendered the switch, because there the silence IS the buyer's
// answer, and the answer is no. A form that leaves the AI toggle off and then
// gets billed for an AI pass has been overruled by its own screen.
//
// The wizard posts a hidden ai_choice marker to say it drew the switch; without
// it this behaves exactly as ParseUseAI, so no other caller changes.
func ParseUseAIFromWizard(r *http.Request) bool {
	if strings.TrimSpace(r.FormValue("ai_choice")) == "" {
		return ParseUseAI(r)
	}
	return matchFlag(r, "use_ai")
}

// matchFlag reads a checkbox from either a urlencoded or a multipart body.
// The import screens post both kinds, and PostFormValue does not see the
// multipart one.
func matchFlag(r *http.Request, name string) bool {
	v := strings.TrimSpace(r.FormValue(name))
	return v == "1" || v == "on" || v == "true"
}
