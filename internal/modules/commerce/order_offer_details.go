package commerce

import (
	"fmt"
	"time"

	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// OrderLineOfferItem represents a product included in an offer bundle.
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

// OrderLineOfferDetails carries full manifest and financial reconciliation
// information for an order line that was purchased under a promotional offer (WO-16).
type OrderLineOfferDetails struct {
	OfferID        int64                `json:"offer_id"`
	LineID         int64                `json:"line_id"`
	Title          i18n.Text            `json:"title"`
	Description    i18n.Text            `json:"description"`
	VendorName     string               `json:"vendor_name"`
	DiscountType   string               `json:"discount_type"`
	DiscountValue  money.Amount         `json:"discount_value"`
	ListPrice      money.Amount         `json:"list_price"`
	UnitPrice      money.Amount         `json:"unit_price"`
	Quantity       int                  `json:"quantity"`
	DiscountAmount money.Amount         `json:"discount_amount"`
	TotalPrice     money.Amount         `json:"total_price"`
	StartsAt       *time.Time           `json:"starts_at,omitempty"`
	ExpiresAt      *time.Time           `json:"expires_at,omitempty"`
	Items          []OrderLineOfferItem `json:"items"`
}

// EffectiveDiscountPercent calculates the percentage discount applied to this line.
func (d *OrderLineOfferDetails) EffectiveDiscountPercent() float64 {
	if d == nil {
		return 0
	}
	totalListMinor := d.ListPrice.Minor() * int64(d.Quantity)
	if totalListMinor > 0 && d.DiscountAmount.IsPositive() {
		return (float64(d.DiscountAmount.Minor()) / float64(totalListMinor)) * 100.0
	}
	if d.ListPrice.IsPositive() && d.UnitPrice.IsPositive() && d.ListPrice.Minor() > d.UnitPrice.Minor() {
		diff := d.ListPrice.Minor() - d.UnitPrice.Minor()
		return (float64(diff) / float64(d.ListPrice.Minor())) * 100.0
	}
	if d.DiscountType == "percentage" && d.DiscountValue.Minor() > 0 {
		return float64(d.DiscountValue.Minor()) / 100.0
	}
	return 0
}

// ValidityText formats the offer validity window for UI display.
func (d *OrderLineOfferDetails) ValidityText(lang string) string {
	if d == nil {
		return ""
	}
	isAr := lang != "en"
	if d.StartsAt != nil && d.ExpiresAt != nil {
		if isAr {
			return fmt.Sprintf("من %s إلى %s", d.StartsAt.Format("2006-01-02"), d.ExpiresAt.Format("2006-01-02"))
		}
		return fmt.Sprintf("From %s to %s", d.StartsAt.Format("2006-01-02"), d.ExpiresAt.Format("2006-01-02"))
	}
	if d.ExpiresAt != nil {
		if isAr {
			return fmt.Sprintf("سارٍ حتى %s", d.ExpiresAt.Format("2006-01-02"))
		}
		return fmt.Sprintf("Valid until %s", d.ExpiresAt.Format("2006-01-02"))
	}
	if d.StartsAt != nil {
		if isAr {
			return fmt.Sprintf("بدأ في %s", d.StartsAt.Format("2006-01-02"))
		}
		return fmt.Sprintf("Started %s", d.StartsAt.Format("2006-01-02"))
	}
	if isAr {
		return "عرض دائم"
	}
	return "Permanent Offer"
}

// MechanicsText returns a human-readable description of the offer mechanics.
func (d *OrderLineOfferDetails) MechanicsText(lang string) string {
	if d == nil {
		return ""
	}
	isAr := lang != "en"
	if len(d.Items) > 0 {
		if isAr {
			return fmt.Sprintf("باقة ترويجية مجمعة تشمل %d أصناف", len(d.Items))
		}
		return fmt.Sprintf("Promotional bundle containing %d items", len(d.Items))
	}
	pct := d.EffectiveDiscountPercent()
	if pct > 0 {
		if isAr {
			return fmt.Sprintf("خصم بنسبة %.1f%% على سعر الجمهور", pct)
		}
		return fmt.Sprintf("%.1f%% discount on list price", pct)
	}
	if d.DiscountAmount.IsPositive() {
		if isAr {
			return fmt.Sprintf("خصم بمقدار %s ج.م", d.DiscountAmount.String())
		}
		return fmt.Sprintf("Discount of %s EGP", d.DiscountAmount.String())
	}
	if isAr {
		return "عرض ترويجي خاص"
	}
	return "Special promotional offer"
}
