package compare

import (
	"time"

	"github.com/muhiya/dawa24-store/internal/shared/money"
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
	SupplierName string `json:"supplier_name"`
	ProductName  string `json:"product_name"`
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
	Items              []*MarketDiscountRow `json:"items"`
	TotalCount         int64                `json:"total_count"`
	AvailableSuppliers []string             `json:"available_suppliers"`
	Page               int                  `json:"page"`
	Limit              int                  `json:"limit"`
	TotalPages         int                  `json:"total_pages"`
	HasPrev            bool                 `json:"has_prev"`
	HasNext            bool                 `json:"has_next"`
}
