package promo

import (
	"strings"
	"time"

	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// IsApproved reports whether the platform cleared the offer for commerce.
func (o *Offer) IsApproved() bool { return o.AdminStatus == "approved" }

type OfferProduct struct {
	ID                    int64         `json:"id"`
	OfferID               int64         `json:"offer_id"`
	ProductID             int64         `json:"product_id"`
	VariantID             *int64        `json:"variant_id,omitempty"`
	CustomPrice           *money.Amount `json:"custom_price,omitempty"`               // full override of the list price
	CustomDiscountPercent *float64      `json:"custom_discount_percentage,omitempty"` // percent, e.g. 15.00 = 15%
	CustomDiscountAmount  *money.Amount `json:"custom_discount_amount,omitempty"`
	CustomQty             int           `json:"custom_qty"`
	MaxQtyPerOrder        *int          `json:"max_qty_per_order,omitempty"`
	CreatedAt             time.Time     `json:"created_at"`
}

// HasPendingEdit reports whether this ad has an unmoderated edit request.
func (a *Ad) HasPendingEdit() bool { return a.PendingChanges != nil }

// IsVideo reports whether this ad displays video media (either by MediaType or by file extension).
func (a *Ad) IsVideo() bool {
	if a == nil {
		return false
	}
	if a.MediaType == MediaVideo || a.MediaType == "video" {
		return true
	}
	lower := strings.ToLower(a.MediaURL)
	return strings.HasSuffix(lower, ".mp4") || strings.HasSuffix(lower, ".webm") || strings.HasSuffix(lower, ".mov")
}

// IsApproved reports whether the admin cleared this ad for display.
func (a *Ad) IsApproved() bool { return a.AdminStatus == AdminApproved }

// DisplayTitle returns the title in the requested language, falling back to the
// legacy single-language Title field.
func (a *Ad) DisplayTitle(lang string) string {
	if lang == "en" {
		if a.TitleEn != "" {
			return a.TitleEn
		}
	}
	if a.TitleAr != "" {
		return a.TitleAr
	}
	return a.Title
}

// DisplayText returns the advertising copy in the requested language.
func (a *Ad) DisplayText(lang string) string {
	if lang == "en" && a.AdTextEn != "" {
		return a.AdTextEn
	}
	return a.AdTextAr
}

// ResolveClickURL builds the destination URL based on the click target type.
func (a *Ad) ResolveClickURL() string {
	if a.TargetURL != "" {
		if strings.Contains(a.TargetURL, "/products?variant_id=") {
			parts := strings.Split(a.TargetURL, "=")
			if len(parts) == 2 && parts[1] != "" {
				return "/catalog/" + parts[1]
			}
		}
		return a.TargetURL
	}
	switch a.ClickTargetType {
	case ClickTargetProduct:
		if a.ClickTargetID != nil {
			return "/catalog/" + fmtInt64(*a.ClickTargetID)
		}
	case ClickTargetOffer:
		if a.ClickTargetID != nil {
			return "/offers/" + fmtInt64(*a.ClickTargetID)
		}
	default:
		if a.OrganizationID != nil {
			return "/suppliers/" + fmtInt64(*a.OrganizationID)
		}
	}
	return "/"
}

// CalculateDiscount computes the discount amount for a given order subtotal.
func (o *Offer) CalculateDiscount(subtotal money.Amount) (money.Amount, error) {
	if !o.IsActive || o.DiscountValue.IsZero() {
		return money.Zero, nil
	}
	if o.MinOrderAmount.IsPositive() && subtotal.Minor() < o.MinOrderAmount.Minor() {
		return money.Zero, nil
	}

	if o.DiscountType == DiscountPercentage {
		// Value stored in minor units as basis points or percent: e.g. 15.00 -> 1500 bps
		bps := o.DiscountValue.Minor()
		discount := subtotal.ApplyPercent(bps)
		return discount, nil
	}

	// Fixed discount
	if o.DiscountValue.Minor() > subtotal.Minor() {
		return subtotal, nil
	}
	return o.DiscountValue, nil
}

// Validate ensures dates and discount amounts are sound.
func (o *Offer) Validate() error {
	if o.OrganizationID <= 0 {
		return apperr.Validation("offer.org_required", "Organization ID is required.", nil)
	}
	if o.Title.IsEmpty() {
		return apperr.Validation("offer.title_required", "Offer title is required.", nil)
	}
	if o.ExpiresAt.Before(o.StartsAt) {
		return apperr.Validation("offer.dates_invalid", "Expiration date must be after start date.", nil)
	}
	return nil
}
