package pages

import (
	"strings"

	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

func reviewScoreOf(rev *org.Review) int {
	if rev == nil {
		return 5
	}
	return rev.Rating
}

func isStepDone(status commerce.OrderStatus, step int) bool {
	switch step {
	case 1:
		return true
	case 2:
		return status == commerce.StatusConfirmed || status == commerce.StatusProcessing || status == commerce.StatusShipped || status == commerce.StatusInTransit || status == commerce.StatusOutForDelivery || status == commerce.StatusDelivered || status == commerce.StatusCompleted
	case 3:
		return status == commerce.StatusShipped || status == commerce.StatusInTransit || status == commerce.StatusOutForDelivery || status == commerce.StatusDelivered || status == commerce.StatusCompleted
	case 4:
		return status == commerce.StatusDelivered || status == commerce.StatusCompleted
	default:
		return false
	}
}

func isStepActive(status commerce.OrderStatus, step int) bool {
	switch step {
	case 1:
		return status == commerce.StatusPending
	case 2:
		return status == commerce.StatusConfirmed || status == commerce.StatusProcessing
	case 3:
		return status == commerce.StatusShipped || status == commerce.StatusInTransit || status == commerce.StatusOutForDelivery
	case 4:
		return status == commerce.StatusDelivered || status == commerce.StatusCompleted
	default:
		return false
	}
}

func orderProgressClass(status commerce.OrderStatus) string {
	switch status {
	case commerce.StatusPending:
		return "w-10"
	case commerce.StatusConfirmed, commerce.StatusProcessing:
		return "w-40"
	case commerce.StatusShipped, commerce.StatusInTransit, commerce.StatusOutForDelivery:
		return "w-75"
	case commerce.StatusDelivered, commerce.StatusCompleted:
		return "w-100"
	default:
		return "w-10"
	}
}

func getAllOrderLines(order *commerce.Order) []*commerce.OrderLine {
	if order == nil {
		return nil
	}
	if len(order.Lines) > 0 {
		return order.Lines
	}
	var res []*commerce.OrderLine
	for _, sh := range order.Shipments {
		if sh != nil {
			res = append(res, sh.Lines...)
		}
	}
	return res
}

func unitDiscountMinor(l *commerce.OrderLine) int64 {
	if l == nil || l.Quantity <= 0 {
		return 0
	}
	if l.DiscountAmount.IsPositive() {
		return l.DiscountAmount.Minor() / int64(l.Quantity)
	}
	pub := linePublicPrice(l)
	if pub.IsPositive() && pub.Minor() > l.UnitPrice.Minor() {
		return pub.Minor() - l.UnitPrice.Minor()
	}
	return 0
}

func minAllowedQty(l *commerce.OrderLine) int {
	if l == nil || l.MinOrderQty <= 1 {
		return 1
	}
	return l.MinOrderQty
}

func maxAllowedQty(l *commerce.OrderLine) int {
	if l == nil {
		return 99999
	}
	maxVal := 99999
	if l.MaxQtyPerOrder > 0 {
		maxVal = l.MaxQtyPerOrder
	}
	if l.AvailableStock > 0 && l.AvailableStock < maxVal {
		maxVal = l.AvailableStock
	}
	return maxVal
}

func orderTaxRate(order *commerce.Order) float64 {
	if order == nil || order.TaxAmount.IsZero() {
		return 0.0
	}
	taxable, _ := order.Subtotal.Sub(order.DiscountAmount)
	if taxable.IsPositive() {
		return float64(order.TaxAmount.Minor()) / float64(taxable.Minor())
	}
	return 0.0
}

func linePublicPrice(l *commerce.OrderLine) money.Amount {
	if l == nil {
		return money.Zero
	}
	if l.ListPrice.IsPositive() {
		return l.ListPrice
	}
	if l.OriginalPrice.IsPositive() {
		return l.OriginalPrice
	}
	if l.OriginalDiscount.IsPositive() && l.UnitPrice.IsPositive() && l.Quantity > 0 {
		if l.TotalPrice.Minor() == l.UnitPrice.Minor()*int64(l.Quantity) {
			discPct := float64(l.OriginalDiscount.Minor()) / 100.0
			rate := 1.0 - (discPct / 100.0)
			if rate > 0.001 {
				return money.FromMinor(int64(float64(l.UnitPrice.Minor()) / rate))
			}
		}
	}
	if l.DiscountAmount.IsPositive() && l.Quantity > 0 {
		unitDisc := l.DiscountAmount.Minor() / int64(l.Quantity)
		if unitDisc > 0 {
			res, _ := l.UnitPrice.Add(money.FromMinor(unitDisc))
			return res
		}
	}
	if l.UnitPrice.IsPositive() {
		return l.UnitPrice
	}
	return money.Zero
}

func orderTotalDiscount(order *commerce.Order) money.Amount {
	if order == nil {
		return money.Zero
	}
	if order.TotalDiscount.IsPositive() {
		return order.TotalDiscount
	}
	if order.DiscountAmount.IsPositive() {
		return order.DiscountAmount
	}
	var sum money.Amount
	for _, l := range getAllOrderLines(order) {
		if l == nil || l.Quantity <= 0 {
			continue
		}
		if l.DiscountAmount.IsPositive() {
			sum, _ = sum.Add(l.DiscountAmount)
		} else {
			pub := linePublicPrice(l)
			if pub.Minor() > l.UnitPrice.Minor() {
				unitDisc, _ := pub.Sub(l.UnitPrice)
				lineDisc, _ := unitDisc.MulInt(int64(l.Quantity))
				sum, _ = sum.Add(lineDisc)
			}
		}
	}
	return sum
}

func orderPublicSubtotal(order *commerce.Order) money.Amount {
	if order == nil {
		return money.Zero
	}
	disc := orderTotalDiscount(order)
	if order.Subtotal.IsPositive() && disc.IsPositive() && order.Subtotal.Minor() > order.TotalAmount.Minor() {
		return order.Subtotal
	}
	var sum money.Amount
	for _, l := range getAllOrderLines(order) {
		if l != nil && l.Quantity > 0 {
			pub := linePublicPrice(l)
			lineGross, _ := pub.MulInt(int64(l.Quantity))
			sum, _ = sum.Add(lineGross)
		}
	}
	if sum.IsPositive() {
		return sum
	}
	if order.Subtotal.IsPositive() {
		return order.Subtotal
	}
	return order.TotalAmount
}

func lineSupplyPrice(l *commerce.OrderLine) money.Amount {
	if l == nil || l.Quantity <= 0 {
		return money.Zero
	}
	return money.FromMinor(l.TotalPrice.Minor() / int64(l.Quantity))
}

func lineDiscountPercent(l *commerce.OrderLine) float64 {
	if l == nil {
		return 0
	}
	if l.OriginalDiscount.IsPositive() {
		return float64(l.OriginalDiscount.Minor()) / 100.0
	}
	pub := linePublicPrice(l)
	if l.Quantity > 0 && pub.IsPositive() && l.DiscountAmount.IsPositive() {
		totalRetail := float64(pub.Minor() * int64(l.Quantity))
		if totalRetail > 0 {
			return (float64(l.DiscountAmount.Minor()) / totalRetail) * 100.0
		}
	}
	if l.CostDiscountPercentage > 0 {
		return l.CostDiscountPercentage
	}
	if pub.Minor() > l.UnitPrice.Minor() {
		return (float64(pub.Minor()-l.UnitPrice.Minor()) / float64(pub.Minor())) * 100.0
	}
	return 0
}

func isOrderLineAnOffer(l *commerce.OrderLine) bool {
	if l == nil {
		return false
	}
	if l.OfferProductID != nil && *l.OfferProductID > 0 {
		return true
	}
	ar := l.ProductName.Get("ar")
	en := l.ProductName.Get("en")
	return strings.Contains(ar, "عرض") || strings.Contains(ar, "باقة") || strings.Contains(en, "Offer") || strings.Contains(en, "Bundle")
}
