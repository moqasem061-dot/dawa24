package compare

import (
	"time"

	"github.com/moqasem061-dot/dawa24/internal/shared/money"
)

// خصومات السوق العامة — the shapes the temporary-warehouse price board reads
// and returns. Split from domain.go for the 400-line rule.

// MarketDiscountRow represents a single discount item displayed on the market discounts page.
type MarketDiscountRow struct {
	// ID is the compare.file_rows line this card renders.
	ID int64 `json:"id"`
	// FileID is the temporary warehouse the line came from.
	FileID int64 `json:"file_id"`
	// SupplierName is the warehouse's own name. A temporary warehouse is a
	// moderator's upload and has no organization behind it, so there is no
	// trade name to read.
	SupplierName  string `json:"supplier_name"`
	ProductName   string `json:"product_name"`
	CatalogNameAR string `json:"catalog_name_ar"`
	CatalogNameEN string `json:"catalog_name_en"`
	// OriginalPrice is سعر الجمهور as the sheet quoted it, and the number the
	// card leads with.
	OriginalPrice      money.Amount `json:"original_price"`
	DiscountPercent    float64      `json:"discount_percent"`
	DiscountValue      money.Amount `json:"discount_value"`
	PriceAfterDiscount money.Amount `json:"price_after_discount"`
	// MatchedProductID is the catalogue product this line was matched to, when
	// the matching pipeline has reached it. Nil means the line is a quoted
	// price and nothing more.
	MatchedProductID *int64 `json:"matched_product_id,omitempty"`
	// InCatalog reports whether the line can be opened in the catalogue. It is
	// false for every unmatched row, and the card shows that rather than
	// offering a button that goes nowhere.
	InCatalog bool `json:"in_catalog"`
	// UploadedAt is when the warehouse was uploaded — the "date of adding" the
	// cards show.
	UploadedAt time.Time `json:"uploaded_at"`
}

// MarketDiscountsResult contains the paginated result of market discounts.
type MarketDiscountsResult struct {
	Items      []*MarketDiscountRow `json:"items"`
	TotalCount int64                `json:"total_count"`
	// TotalKnown is false when the exact count could not be read inside the
	// page's budget. The rows are still the answer; the pager is the part that
	// is missing, and it says so rather than printing a zero it would have to
	// invent.
	TotalKnown bool `json:"total_known"`
	// MaxDiscount is the highest discount on the whole filtered board, not on
	// this page: it comes from the same cached scan as TotalCount and is only
	// meaningful while TotalKnown is true and TotalCount is above zero.
	MaxDiscount float64 `json:"max_discount"`
	// ProductNameAR and ProductNameEN name the catalogue product the board is
	// filtered to (MarketDiscountsFilter.ProductID); both are empty otherwise,
	// or when the product no longer exists.
	ProductNameAR      string   `json:"product_name_ar,omitempty"`
	ProductNameEN      string   `json:"product_name_en,omitempty"`
	AvailableSuppliers []string `json:"available_suppliers"`
	Page               int      `json:"page"`
	Limit              int      `json:"limit"`
	TotalPages         int      `json:"total_pages"`
	HasPrev            bool     `json:"has_prev"`
	HasNext            bool     `json:"has_next"`
}

// MarketCount is what one scan of the filtered board yields: how many rows
// there are, and the highest discount among them. They are read together so the
// header's "highest discount" describes the whole filter, not the page on show,
// at the price of one extra aggregate on a scan the count already makes.
type MarketCount struct {
	Total       int64   `json:"total"`
	MaxDiscount float64 `json:"max_discount"`
}

// MarketSuggestion is one line of the search box's dropdown: a product, or an
// unmatched file name, with what the board holds for it.
//
// ProductID is set when the offers were matched to a catalogue product; the
// board can then be filtered to exactly that product. An unmatched line has no
// ID and is grouped by its normalised file name instead.
type MarketSuggestion struct {
	ProductID    *int64       `json:"product_id,omitempty"`
	NameAR       string       `json:"name_ar,omitempty"`
	NameEN       string       `json:"name_en,omitempty"`
	FileName     string       `json:"file_name"`
	Offers       int          `json:"offers"`
	Warehouses   int          `json:"warehouses"`
	BestDiscount float64      `json:"best_discount"`
	LowestPrice  money.Amount `json:"lowest_price"`
}

// DisplayName is the catalogue name in the reader's language, else the other
// catalogue language, else the name as the sheet spelled it.
func (s MarketSuggestion) DisplayName(lang string) string {
	if lang == "en" && s.NameEN != "" {
		return s.NameEN
	}
	if s.NameAR != "" {
		return s.NameAR
	}
	if s.NameEN != "" {
		return s.NameEN
	}
	return s.FileName
}
