package commerce

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// IsOfferLine reports whether this line is an offer bought as a unit rather
// than a catalogue item.
//
// An offer is sold at its own price for whatever it contains; the products
// listed against it are a manifest, not separately purchasable lines. Such a
// line therefore carries an offer and no product reference at all, which is the
// shape migration 155 added to commerce.cart_items and which the check
// constraint there enforces.
func (i *CartItem) IsOfferLine() bool {
	return i != nil && i.OfferID != nil && *i.OfferID > 0 &&
		i.ProductID == 0 && i.ProductVariantID == 0
}

// BranchLocationURL returns the most accurate Google Maps navigation or directions link for the branch.
func (s *OrderShipment) BranchLocationURL() string {
	if s == nil {
		return ""
	}
	if s.CustomerBranchLatitude != nil && s.CustomerBranchLongitude != nil &&
		(*s.CustomerBranchLatitude != 0 || *s.CustomerBranchLongitude != 0) {
		return fmt.Sprintf("https://www.google.com/maps/dir/?api=1&destination=%.6f,%.6f", *s.CustomerBranchLatitude, *s.CustomerBranchLongitude)
	}
	if trimmed := strings.TrimSpace(s.CustomerBranchGoogleMapsURL); trimmed != "" {
		return trimmed
	}
	if trimmedAddr := strings.TrimSpace(s.CustomerBranchAddress); trimmedAddr != "" {
		return "https://www.google.com/maps/search/?api=1&query=" + url.QueryEscape(trimmedAddr)
	}
	return ""
}

// HasExactCoordinates reports whether the shipment recipient branch has valid GPS coordinates.
func (s *OrderShipment) HasExactCoordinates() bool {
	return s != nil && s.CustomerBranchLatitude != nil && s.CustomerBranchLongitude != nil &&
		(*s.CustomerBranchLatitude != 0 || *s.CustomerBranchLongitude != 0)
}

// TotalUnitsCount returns the total quantity sum of all order lines in the shipment.
func (s *OrderShipment) TotalUnitsCount() int {
	if s == nil {
		return 0
	}
	total := 0
	for _, l := range s.Lines {
		if l != nil {
			total += l.Quantity
		}
	}
	return total
}

// HasCost reports whether this line snapshot carried cost information (CostPrice or CostDiscountPercentage).
func (l *OrderLine) HasCost() bool {
	if l == nil {
		return false
	}
	if l.CostDiscountPercentage > 0 && l.EffectivePublicPrice().IsPositive() {
		return true
	}
	return l.CostPrice != nil && l.CostPrice.IsPositive()
}

// HasCostPrice reports whether this line snapshot carried cost information.
func (l *OrderLine) HasCostPrice() bool {
	return l.HasCost()
}

// EffectivePublicPrice returns the official retail price (سعر الجمهور).
func (l *OrderLine) EffectivePublicPrice() money.Amount {
	if l == nil {
		return money.Zero
	}
	if l.ListPrice.IsPositive() {
		return l.ListPrice
	}
	if l.OriginalPrice.IsPositive() {
		return l.OriginalPrice
	}
	if l.UnitPrice.IsPositive() {
		return l.UnitPrice
	}
	if l.CostPrice != nil && l.CostPrice.IsPositive() {
		return *l.CostPrice
	}
	return money.Zero
}

// EffectiveSellingPrice returns the actual unit selling price after discount.
func (l *OrderLine) EffectiveSellingPrice() money.Amount {
	if l == nil || l.Quantity <= 0 {
		return money.Zero
	}
	return money.FromMinor(l.TotalPrice.Minor() / int64(l.Quantity))
}

// EffectiveSellingDiscountPercent returns the selling discount percentage relative to public price.
func (l *OrderLine) EffectiveSellingDiscountPercent() float64 {
	pub := l.EffectivePublicPrice()
	if !pub.IsPositive() {
		return 0
	}
	sell := l.EffectiveSellingPrice()
	if sell.Minor() >= pub.Minor() {
		return 0
	}
	return (float64(pub.Minor()-sell.Minor()) / float64(pub.Minor())) * 100.0
}

// UnitSellingDiscountAmount returns the unit discount value conceded on the public price.
func (l *OrderLine) UnitSellingDiscountAmount() money.Amount {
	if l == nil {
		return money.Zero
	}
	pub := l.EffectivePublicPrice()
	sell := l.EffectiveSellingPrice()
	if pub.Minor() > sell.Minor() {
		return money.FromMinor(pub.Minor() - sell.Minor())
	}
	return money.Zero
}

// TotalSellingDiscount returns the total discount value conceded on the public price for this line.
func (l *OrderLine) TotalSellingDiscount() money.Amount {
	if l == nil || l.Quantity <= 0 {
		return money.Zero
	}
	pub := l.EffectivePublicPrice()
	lineGross := money.FromMinor(pub.Minor() * int64(l.Quantity))
	if lineGross.Minor() > l.TotalPrice.Minor() {
		return money.FromMinor(lineGross.Minor() - l.TotalPrice.Minor())
	}
	if l.DiscountAmount.IsPositive() {
		return l.DiscountAmount
	}
	return money.Zero
}

// EffectivePurchaseCost calculates unit purchase cost:
// In Egyptian pharmaceutical trade, cost discount % is applied directly to Public Price (سعر الجمهور).
// Effective Cost = Public Price * (1 - CostDiscountPercentage / 100).
// If cost discount % is not given, CostPrice is used directly.
// If neither is present, returns money.Zero.
func (l *OrderLine) EffectivePurchaseCost() money.Amount {
	if l == nil || !l.HasCost() {
		return money.Zero
	}
	pub := l.EffectivePublicPrice()
	if l.CostDiscountPercentage > 0 && pub.IsPositive() {
		discMinor := int64(float64(pub.Minor()) * (l.CostDiscountPercentage / 100.0))
		return money.FromMinor(pub.Minor() - discMinor)
	}
	if l.CostPrice != nil && l.CostPrice.IsPositive() {
		if l.CostDiscountPercentage > 0 {
			discMinor := int64(float64(l.CostPrice.Minor()) * (l.CostDiscountPercentage / 100.0))
			return money.FromMinor(l.CostPrice.Minor() - discMinor)
		}
		return *l.CostPrice
	}
	return money.Zero
}

// UnitDiscountedCost calculates the discounted unit cost price.
func (l *OrderLine) UnitDiscountedCost() money.Amount {
	return l.EffectivePurchaseCost()
}

// TotalPurchaseCost calculates the total purchase cost for this order line (Discounted Cost * Quantity).
func (l *OrderLine) TotalPurchaseCost() money.Amount {
	if l == nil || l.Quantity <= 0 || !l.HasCost() {
		return money.Zero
	}
	unitCost := l.EffectivePurchaseCost()
	if !unitCost.IsPositive() {
		return money.Zero
	}
	return money.FromMinor(unitCost.Minor() * int64(l.Quantity))
}

// TotalCost calculates the total cost for this order line strictly from purchase cost:
// Total Cost = Total Purchase Cost (سعر التكلفة بعد خصم التكلفة × الكمية).
// Selling discounts conceded to customers on public price are NEVER included in cost.
func (l *OrderLine) TotalCost() money.Amount {
	if l == nil || l.Quantity <= 0 || !l.HasCost() {
		return money.Zero
	}
	return l.TotalPurchaseCost()
}

// TotalNetProfit computes the vendor's net profit for this line.
// Net Profit = Total Price (سعر البيع الفعلي المحقق) - Total Cost (تكلفة الشراء الفعلية).
// If no cost exists, net profit is Zero.
func (l *OrderLine) TotalNetProfit() money.Amount {
	if l == nil || !l.HasCost() {
		return money.Zero
	}
	totCost := l.TotalCost()
	return money.FromMinor(l.TotalPrice.Minor() - totCost.Minor())
}

// CalculateAverageRating computes the exact 2-decimal scalar average of review criteria (audit §3.3).
func CalculateAverageRating(ratings ...int) float64 {
	if len(ratings) == 0 {
		return 0
	}
	sum := 0
	for _, r := range ratings {
		if r < 1 {
			r = 1
		} else if r > 5 {
			r = 5
		}
		sum += r
	}
	cents := (sum*100 + len(ratings)/2) / len(ratings)
	return float64(cents) / 100.0
}
