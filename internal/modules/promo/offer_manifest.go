package promo

import (
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// OrderLineOfferItem represents a single component item inside an offer bundle.
type OrderLineOfferItem struct {
	ProductID             int64        `json:"product_id"`
	ProductName           i18n.Text    `json:"product_name"`
	VariantID             *int64       `json:"variant_id,omitempty"`
	VariantName           string       `json:"variant_name,omitempty"`
	SKU                   string       `json:"sku,omitempty"`
	Quantity              int          `json:"quantity"`
	CustomPrice           money.Amount `json:"custom_price"`
	CustomDiscountPercent float64      `json:"custom_discount_percentage"`
}

// OrderLineOfferDetails carries full manifest information for an offer bundle.
type OrderLineOfferDetails struct {
	OfferID       int64                `json:"offer_id"`
	Title         i18n.Text            `json:"title"`
	Description   i18n.Text            `json:"description"`
	VendorName    string               `json:"vendor_name"`
	DiscountType  string               `json:"discount_type"`
	DiscountValue money.Amount         `json:"discount_value"`
	Items         []OrderLineOfferItem `json:"items"`
}
