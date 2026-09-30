package catalog

import (
	"strings"

	"github.com/moqasem061-dot/dawa24/internal/shared/productmatch"
)

// MatchStrategy determines how rows are linked to the master catalogue.
type MatchStrategy string

const (
	StrategyBarcodeOnly MatchStrategy = "barcode_only"
	StrategyIDOnly      MatchStrategy = "id_only"
	StrategyNameOnly    MatchStrategy = "name_only"
	StrategySKUBarcode  MatchStrategy = "sku_barcode"

	// Legacy aliases
	StrategySmartAuto MatchStrategy = "smart_auto"
	StrategySKUOnly   MatchStrategy = "sku_only"
)

// SavingMatchChoice is the unified matching configuration for one import run.
type SavingMatchChoice struct {
	MinScore          float64
	ByProductID       bool
	ByBarcode         bool
	ByCode            bool
	CodeIsCatalogCode bool
}

// DefaultSavingMatchChoice returns the platform default matching configuration.
func DefaultSavingMatchChoice() SavingMatchChoice {
	return SavingMatchChoice{
		MinScore:    productmatch.DefaultMinStrong,
		ByProductID: true,
	}
}

// Normalize clamps a submitted choice into the range the engine accepts.
func (c SavingMatchChoice) Normalize() SavingMatchChoice {
	if c.MinScore <= 0 {
		c.MinScore = productmatch.DefaultMinStrong
	}
	c.MinScore = min(max(c.MinScore, productmatch.DefaultMinReview), 1)
	if !c.ByCode {
		c.CodeIsCatalogCode = false
	}
	return c
}

// SavingMatchResult captures the resolution of a single saving product row.
type SavingMatchResult struct {
	ProductID  *int64  `json:"product_id,omitempty"`
	MatchType  string  `json:"match_type"`
	Confidence float64 `json:"confidence"`
}

// SavingProductMatchEngine links rows to catalogue products using the shared index.
type SavingProductMatchEngine struct {
	Index         *productmatch.Index
	Known         map[int64]bool
	opts          productmatch.MatchOptions
	codeIsBarcode bool
}

// NewSavingProductMatchEngine builds an index for a set of match products.
func NewSavingProductMatchEngine(products []MatchProduct) *SavingProductMatchEngine {
	masters := make([]productmatch.MasterProduct, 0, len(products))
	known := make(map[int64]bool, len(products))
	for _, p := range products {
		if p.ID <= 0 {
			continue
		}
		known[p.ID] = true
		masters = append(masters, productmatch.MasterProduct{
			ID: p.ID, NameAR: p.NameAR, NameEN: p.NameEN, SKU: p.SKU,
			Barcode: p.Barcode, Scientific: p.Scientific, DosageForm: p.DosageForm,
			Concentration: p.Concentration, Unit: p.Unit,
			Manufacturer: p.Manufacturer, PublicPrice: p.PublicPrice,
		})
	}

	return &SavingProductMatchEngine{
		Index: productmatch.NewIndex(masters),
		Known: known,
		opts:  productmatch.DefaultMatchOptions(),
	}
}

// Size reports how many catalogue products are indexed.
func (e *SavingProductMatchEngine) Size() int {
	if e == nil || e.Index == nil {
		return 0
	}
	return e.Index.Size()
}

// UseIdentifiers configures identifier tiers directly without WithIdentifiers.
func (e *SavingProductMatchEngine) UseIdentifiers(
	mapped productmatch.MappedColumns, chosen productmatch.IdentifierChoices,
) {
	if e == nil {
		return
	}
	e.opts.TrustBarcode = mapped.Barcode && chosen.ByBarcode
	e.opts.TrustSupplierCode = mapped.Code && chosen.ByCode
	e.opts.CodeIsAuthoritative = chosen.CodeIsCatalogCode
	e.codeIsBarcode = mapped.Barcode && chosen.ByBarcode
}

// Describe returns a matched product's catalogue name and code.
func (e *SavingProductMatchEngine) Describe(id int64) (name, sku string) {
	if e == nil || e.Index == nil {
		return "", ""
	}
	p, ok := e.Index.Lookup(id)
	if !ok {
		return "", ""
	}
	name = p.NameAR
	if name == "" {
		name = p.NameEN
	}
	return name, p.SKU
}

// Match resolves one row under the legacy strategy vocabulary.
func (e *SavingProductMatchEngine) Match(strategy MatchStrategy, productID *int64, rawSKU, rawName string) SavingMatchResult {
	unlinked := SavingMatchResult{MatchType: "unlinked"}
	if e == nil || e.Index == nil || e.Index.Size() == 0 {
		return unlinked
	}
	if strategy == "" {
		strategy = StrategyNameOnly
	}

	if productID != nil && *productID > 0 && e.Known[*productID] {
		id := *productID
		return SavingMatchResult{ProductID: &id, MatchType: "direct_id", Confidence: 1.0}
	}
	if strategy == StrategyIDOnly {
		return unlinked
	}

	row := &productmatch.Row{Name: strings.TrimSpace(rawName)}
	opts := e.opts
	code := strings.TrimSpace(rawSKU)

	switch strategy {
	case StrategyBarcodeOnly:
		row.Barcode = code
		opts.TrustBarcode = true
		opts.TrustSupplierCode = false
		opts.MinStrong, opts.MinReview = 1.01, 1.01
	case StrategySKUBarcode, StrategySKUOnly:
		e.bindCode(row, code)
		opts.TrustBarcode = true
		opts.TrustSupplierCode = true
		opts.CodeIsAuthoritative = true
		opts.MinStrong, opts.MinReview = 1.01, 1.01
	case StrategySmartAuto:
		e.bindCode(row, code)
	case StrategyNameOnly:
		opts.TrustSupplierCode = false
		opts.TrustBarcode = false
	default:
		opts.TrustSupplierCode = false
		opts.TrustBarcode = false
	}
	if row.Name == "" && row.SKU == "" && row.Barcode == "" {
		return unlinked
	}

	res := e.Index.Match(row, opts)
	if !res.Matched() || !res.Level.Settled() {
		return unlinked
	}
	return SavingMatchResult{
		ProductID:  &res.ProductID,
		MatchType:  savingMatchType(res.Level),
		Confidence: res.Score,
	}
}

// MatchUnified resolves one row under the platform's single matching order.
func (e *SavingProductMatchEngine) MatchUnified(
	c SavingMatchChoice, productID *int64, rawSKU, rawName string,
) SavingMatchResult {
	unlinked := SavingMatchResult{MatchType: "unlinked"}
	if e == nil || e.Index == nil || e.Index.Size() == 0 {
		return unlinked
	}
	c = c.Normalize()

	if c.ByProductID && productID != nil && *productID > 0 && e.Known[*productID] {
		id := *productID
		return SavingMatchResult{ProductID: &id, MatchType: "direct_id", Confidence: 1.0}
	}

	row := &productmatch.Row{Name: strings.TrimSpace(rawName)}
	opts := e.opts
	opts.MinStrong = c.MinScore
	opts.MinReview = min(productmatch.DefaultMinReview, c.MinScore)
	opts.TrustBarcode = c.ByBarcode
	opts.TrustSupplierCode = c.ByCode
	opts.CodeIsAuthoritative = c.CodeIsCatalogCode

	// T066: A single column is NEVER assigned to both Barcode and SKU.
	if code := strings.TrimSpace(rawSKU); code != "" {
		if c.ByBarcode && !c.ByCode {
			row.Barcode = code
		} else if c.ByCode && !c.ByBarcode {
			row.SKU = code
		} else if c.ByBarcode && c.ByCode {
			if isBarcodeCandidate(code) {
				row.Barcode = code
			} else {
				row.SKU = code
			}
		}
	}
	if row.Name == "" && row.SKU == "" && row.Barcode == "" {
		return unlinked
	}

	res := e.Index.Match(row, opts)
	if !res.Matched() || !res.Level.Settled() {
		return unlinked
	}
	return SavingMatchResult{
		ProductID:  &res.ProductID,
		MatchType:  savingMatchType(res.Level),
		Confidence: res.Score,
	}
}

func isBarcodeCandidate(code string) bool {
	if productmatch.ValidGTIN(code) {
		return true
	}
	if len(code) >= 8 {
		for _, r := range code {
			if r < '0' || r > '9' {
				return false
			}
		}
		return true
	}
	return false
}

func (e *SavingProductMatchEngine) bindCode(row *productmatch.Row, code string) {
	if code == "" {
		return
	}
	if e.codeIsBarcode || isBarcodeCandidate(code) {
		row.Barcode = code
		return
	}
	row.SKU = code
}

func savingMatchType(level productmatch.MatchLevel) string {
	switch level {
	case productmatch.MatchBarcode:
		return "barcode"
	case productmatch.MatchCode:
		return "exact_sku"
	case productmatch.MatchExact:
		return "exact_name"
	default:
		return "fuzzy_name"
	}
}
