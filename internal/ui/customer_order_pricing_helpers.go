package ui

import (
	"context"
	"strings"

	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

func (h *UIHandler) enrichOrderPricing(ctx context.Context, order *commerce.Order) {
	if order == nil {
		return
	}
	var allLines []*commerce.OrderLine
	allLines = append(allLines, order.Lines...)
	for _, sh := range order.Shipments {
		if sh != nil {
			allLines = append(allLines, sh.Lines...)
		}
	}

	for _, l := range allLines {
		if l == nil {
			continue
		}

		// 1. Resolve offer link if missing
		if l.OfferProductID == nil || *l.OfferProductID <= 0 {
			if order.OfferID > 0 {
				l.OfferProductID = &order.OfferID
			} else if strings.Contains(l.ProductName.Get("ar"), "عرض") || strings.Contains(l.ProductName.Get("en"), "Offer") {
				if h.promoSvc != nil {
					if offList, err := h.promoSvc.ListActiveOffers(ctx, 50, 0); err == nil {
						for _, off := range offList {
							if off.Title.Get("ar") == l.ProductName.Get("ar") || off.Title.Get("en") == l.ProductName.Get("en") {
								offID := off.ID
								l.OfferProductID = &offID
								if off.DiscountValue.IsPositive() && l.CostDiscountPercentage <= 0 {
									l.CostDiscountPercentage = float64(off.DiscountValue.Minor()) / 100.0
								}
								break
							}
						}
					}
				}
			}
		}

		// 2. Resolve variant & public retail price if list price missing
		if (l.ListPrice.IsZero() || l.ListPrice.Minor() <= l.UnitPrice.Minor()) && l.ProductID != nil && *l.ProductID > 0 && h.catSvc != nil {
			if prod, variants, err := h.catSvc.GetProduct(ctx, *l.ProductID); err == nil && prod != nil {
				var targetListPrice money.Amount
				if prod.Price.IsPositive() {
					targetListPrice = prod.Price
				}
				if l.ProductVariantID != nil && *l.ProductVariantID > 0 {
					for _, v := range variants {
						if v != nil && v.ID == *l.ProductVariantID {
							if v.Price.IsPositive() {
								targetListPrice = v.Price
							}
							if v.CostDiscountPercentage > 0 && l.CostDiscountPercentage <= 0 {
								l.CostDiscountPercentage = v.CostDiscountPercentage
							}
							break
						}
					}
				}
				if targetListPrice.IsPositive() {
					l.ListPrice = targetListPrice
					if l.OriginalPrice.IsZero() {
						l.OriginalPrice = targetListPrice
					}
					if targetListPrice.Minor() > l.UnitPrice.Minor() {
						diff := targetListPrice.Minor() - l.UnitPrice.Minor()
						if l.CostDiscountPercentage <= 0 {
							l.CostDiscountPercentage = (float64(diff) / float64(targetListPrice.Minor())) * 100.0
						}
						if l.DiscountAmount.IsZero() {
							l.DiscountAmount = money.FromMinor(diff * int64(l.Quantity))
						}
					}
				}
			}
		}

		// 3. Fallback: derive cost discount percentage from list price and unit price
		if l.CostDiscountPercentage <= 0 && l.ListPrice.Minor() > l.UnitPrice.Minor() {
			diff := l.ListPrice.Minor() - l.UnitPrice.Minor()
			l.CostDiscountPercentage = (float64(diff) / float64(l.ListPrice.Minor())) * 100.0
		}
	}
}

// CustomerOrderLineOfferDetails returns the HTML modal partial containing the offer bundle
// manifest and all included items.
