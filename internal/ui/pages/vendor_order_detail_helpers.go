package pages

import (
	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

type VendorOrderDetailData struct {
	Shipment   *commerce.OrderShipment
	History    []*commerce.OrderStatusHistory
	CanAssign  bool
	Couriers   []DeliveryCourierOption
	NoticeType string
	NoticeMsg  string
	Lang       string
	Dir        string
}

type VendorShipmentFinancialSummary struct {
	TotalPublicPrice money.Amount // إجمالي سعر الجمهور
	TotalDiscount    money.Amount // الخصم الممنوح
	ShippingFee      money.Amount // تكاليف الشحن
	NetTotal         money.Amount // السعر الصافي
	TotalCost        money.Amount // التكلفة
	TotalProfit      money.Amount // الربح التقديري
	MarginPct        float64
}

// LineUnitNetPrice returns the net selling price per single unit for an order line.
func LineUnitNetPrice(l *commerce.OrderLine) money.Amount {
	if l == nil {
		return money.Zero
	}
	if l.Quantity > 0 && l.TotalPrice.IsPositive() {
		return money.FromMinor(l.TotalPrice.Minor() / int64(l.Quantity))
	}
	if l.UnitPrice.IsPositive() {
		return l.UnitPrice
	}
	return money.Zero
}

// LinePublicPrice returns the official retail public price per single unit for an order line.
func LinePublicPrice(l *commerce.OrderLine) money.Amount {
	if l == nil {
		return money.Zero
	}
	pub := linePublicPrice(l)
	net := LineUnitNetPrice(l)
	if pub.IsZero() || pub.Minor() < net.Minor() {
		if l.ListPrice.IsPositive() {
			pub = l.ListPrice
		} else if l.OriginalPrice.IsPositive() {
			pub = l.OriginalPrice
		} else if l.Quantity > 0 && l.DiscountAmount.IsPositive() {
			pub = money.FromMinor(net.Minor() + (l.DiscountAmount.Minor() / int64(l.Quantity)))
		} else {
			pub = net
		}
	}
	return pub
}

// LineUnitDiscountPercent calculates the discount percentage on a single unit.
func LineUnitDiscountPercent(l *commerce.OrderLine) float64 {
	if l == nil {
		return 0
	}
	if l.OriginalDiscount.IsPositive() {
		return float64(l.OriginalDiscount.Minor()) / 100.0
	}
	pub := LinePublicPrice(l)
	net := LineUnitNetPrice(l)
	if pub.IsPositive() && pub.Minor() > net.Minor() {
		return (float64(pub.Minor()-net.Minor()) / float64(pub.Minor())) * 100.0
	}
	if l.Quantity > 0 && l.DiscountAmount.IsPositive() && pub.IsPositive() {
		totalRetail := float64(pub.Minor() * int64(l.Quantity))
		if totalRetail > 0 {
			return (float64(l.DiscountAmount.Minor()) / totalRetail) * 100.0
		}
	}
	return 0
}

// LineUnitDiscountValue returns the monetary discount amount per single unit.
func LineUnitDiscountValue(l *commerce.OrderLine) money.Amount {
	if l == nil {
		return money.Zero
	}
	pub := LinePublicPrice(l)
	net := LineUnitNetPrice(l)
	if pub.Minor() > net.Minor() {
		return money.FromMinor(pub.Minor() - net.Minor())
	}
	if l.Quantity > 0 && l.DiscountAmount.IsPositive() {
		return money.FromMinor(l.DiscountAmount.Minor() / int64(l.Quantity))
	}
	return money.Zero
}

// LineUnitCostPrice returns the per-unit purchase cost for the vendor, and reports whether cost is available.
func LineUnitCostPrice(l *commerce.OrderLine) (cost money.Amount, ok bool) {
	if l == nil {
		return money.Zero, false
	}
	effCost := l.EffectivePurchaseCost()
	if effCost.IsPositive() {
		return effCost, true
	}
	if l.CostPrice != nil && l.CostPrice.IsPositive() {
		return *l.CostPrice, true
	}
	return money.Zero, false
}

func computeVendorShipmentFinancialSummary(sh *commerce.OrderShipment) VendorShipmentFinancialSummary {
	if sh == nil {
		return VendorShipmentFinancialSummary{}
	}
	var (
		grossMinor    int64
		netItemsMinor int64
		costMinor     int64
		hasAnyCost    bool
	)
	for _, l := range sh.Lines {
		if l == nil {
			continue
		}
		qty := int64(l.Quantity)
		lineNet := l.TotalPrice.Minor()
		pub := LinePublicPrice(l)
		lineGross := pub.Minor() * qty
		if lineGross <= lineNet && l.DiscountAmount.IsPositive() {
			lineGross = lineNet + l.DiscountAmount.Minor()
		}
		if lineGross < lineNet {
			lineGross = lineNet
		}
		grossMinor += lineGross
		netItemsMinor += lineNet

		if unitCost, ok := LineUnitCostPrice(l); ok {
			costMinor += unitCost.Minor() * qty
			hasAnyCost = true
		}
	}
	if grossMinor == netItemsMinor && sh.Subtotal.IsPositive() && sh.Subtotal.Minor() > netItemsMinor {
		grossMinor = sh.Subtotal.Minor()
	}
	discountMinor := grossMinor - netItemsMinor
	if discountMinor < 0 {
		discountMinor = 0
	}
	shippingMinor := sh.ShippingFee.Minor()
	netTotalMinor := netItemsMinor + shippingMinor
	if sh.TotalAmount.IsPositive() {
		netTotalMinor = sh.TotalAmount.Minor()
	}
	profitMinor := int64(0)
	if hasAnyCost {
		profitMinor = netItemsMinor - costMinor
		if profitMinor < 0 {
			profitMinor = 0
		}
	}
	var marginPct float64
	if netItemsMinor > 0 && hasAnyCost {
		marginPct = (float64(profitMinor) / float64(netItemsMinor)) * 100
	}
	return VendorShipmentFinancialSummary{
		TotalPublicPrice: money.FromMinor(grossMinor),
		TotalDiscount:    money.FromMinor(discountMinor),
		ShippingFee:      sh.ShippingFee,
		NetTotal:         money.FromMinor(netTotalMinor),
		TotalCost:        money.FromMinor(costMinor),
		TotalProfit:      money.FromMinor(profitMinor),
		MarginPct:        marginPct,
	}
}

func computeVendorShipmentFinancials(sh *commerce.OrderShipment) (totalCost money.Amount, totalProfit money.Amount, marginPct float64) {
	s := computeVendorShipmentFinancialSummary(sh)
	return s.TotalCost, s.TotalProfit, s.MarginPct
}
