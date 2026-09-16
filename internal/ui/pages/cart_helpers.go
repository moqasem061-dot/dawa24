package pages

import (
	"fmt"

	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// CartTotal sums the line totals of a cart. Overflow is impossible for real
// basket sizes, but on the off chance a value is absurd the line is skipped
// rather than panicking.
func CartTotal(cart *commerce.Cart) money.Amount {
	total := money.Zero
	if cart == nil {
		return total
	}
	for _, item := range cart.Items {
		line, err := item.UnitPrice.MulInt(int64(item.Quantity))
		if err != nil {
			continue
		}
		if sum, err := total.Add(line); err == nil {
			total = sum
		}
	}
	return total
}

// CartItemTotal calculates line total for a single cart item.
func CartItemTotal(item *commerce.CartItem) money.Amount {
	if item == nil {
		return money.Zero
	}
	line, err := item.UnitPrice.MulInt(int64(item.Quantity))
	if err != nil {
		return money.Zero
	}
	return line
}

// CartGroup is one supplier's slice of the cart.
type CartGroup struct {
	OrganizationID int64
	SupplierName   i18n.Text
	MinOrderPrice  money.Amount
	BelowMin       bool
	Items          []*commerce.CartItem
	Subtotal       money.Amount
	// Delivery is what this supplier charges to bring this shipment to the
	// pharmacy's branch, measured from their warehouse against their own
	// شرائح ورسوم التوصيل.
	//
	// The cart used to print "رسوم الشحن: مجاني للطلبيات المعتمدة" as a fixed
	// line while the checkout charged the bands, so a pharmacy was told the
	// delivery was free and then billed for it on the next screen.
	Delivery org.DeliveryQuote
}

// DeliveryTotal sums what every supplier in the cart charges for delivery.
func DeliveryTotal(groups []CartGroup) money.Amount {
	total := money.Zero
	for _, g := range groups {
		if sum, err := total.Add(g.Delivery.Fee); err == nil {
			total = sum
		}
	}
	return total
}

// CartGrandTotal is the goods plus every supplier's delivery.
func CartGrandTotal(cart *commerce.Cart, groups []CartGroup) money.Amount {
	total := CartTotal(cart)
	if sum, err := total.Add(DeliveryTotal(groups)); err == nil {
		return sum
	}
	return total
}

// DeliveryNote explains a quote in one phrase, so a pharmacy can see why a fee
// is what it is rather than only that it exists.
func DeliveryNote(q org.DeliveryQuote) string {
	switch q.Basis {
	case org.BasisNoBands:
		return "لم يحدد المورد رسوم توصيل"
	case org.BasisUnknownDistance:
		return "تقديري — لم تُحدَّد إحداثيات الفرع أو مقر المورد"
	case org.BasisAboveRange:
		return fmt.Sprintf("%.1f كم — خارج آخر شريحة، تُطبَّق أعلى شريحة", q.DistanceKM())
	case org.BasisBelowRange, org.BasisGap, org.BasisExact:
		return fmt.Sprintf("%.1f كم من مقر المورد", q.DistanceKM())
	}
	return ""
}

// GroupCartBySupplier partitions the cart by supplier, in order of appearance,
// with a per-supplier subtotal and a minimum-order flag.
func GroupCartBySupplier(cart *commerce.Cart) []CartGroup {
	if cart == nil {
		return nil
	}
	var groups []CartGroup
	byOrg := map[int64]int{}
	for _, item := range cart.Items {
		idx, ok := byOrg[item.OrganizationID]
		if !ok {
			idx = len(groups)
			byOrg[item.OrganizationID] = idx
			groups = append(groups, CartGroup{
				OrganizationID: item.OrganizationID,
				SupplierName:   item.SupplierName,
				MinOrderPrice:  item.MinOrderPrice,
			})
		}
		groups[idx].Items = append(groups[idx].Items, item)
		if line, err := item.UnitPrice.MulInt(int64(item.Quantity)); err == nil {
			if sum, err := groups[idx].Subtotal.Add(line); err == nil {
				groups[idx].Subtotal = sum
			}
		}
	}
	for i := range groups {
		groups[i].BelowMin = groups[i].MinOrderPrice.Minor() > 0 && groups[i].Subtotal.Minor() < groups[i].MinOrderPrice.Minor()
	}
	return groups
}

// HasUncoveredItems reports whether any line item in the cart is outside coverage.
func HasUncoveredItems(cart *commerce.Cart) bool {
	if cart == nil {
		return false
	}
	for _, item := range cart.Items {
		if item != nil && !item.IsCovered && item.CoverageReason != "" {
			return true
		}
	}
	return false
}
