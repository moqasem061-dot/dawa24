package ui

import (
	"context"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// buildCatalogVariantCards maps a slice of buyer offers into storefront cards,
// applying batch availability decisions and computing presentation badges.
func (h *UIHandler) buildCatalogVariantCards(
	ctx context.Context,
	offers []*catalog.BuyerOffer,
	batchResults map[int64]commerce.AvailabilityResult,
	customerBranchID int64,
	actor *authctx.Actor,
	lang string,
) ([]*pages.SupplierVariantCard, int) {
	if len(offers) == 0 {
		return nil, 0
	}

	favMap := make(map[int64]bool)
	if actor != nil && actor.UserID > 0 && h.idSvc != nil {
		if ids, err := h.idSvc.ListFavorites(ctx, actor.UserID); err == nil {
			for _, id := range ids {
				favMap[id] = true
			}
		}
	}

	custLat, custLng, hasCustCoords := h.buyingBranchCoords(ctx, actor)
	cards := make([]*pages.SupplierVariantCard, 0, len(offers))
	droppedCount := 0

	for _, off := range offers {
		if off == nil {
			continue
		}

		isCovered := false
		canAddToCart := false
		covReason := ""
		maxOrderQty := off.AvailableStock

		if customerBranchID > 0 {
			res, ok := batchResults[off.VariantID]
			if !ok {
				// Variant was not evaluated or failed probe
				droppedCount++
				continue
			}
			switch res.Disposition() {
			case commerce.DispositionOrderable:
				isCovered = true
				canAddToCart = (off.AvailableStock > 0)
				maxOrderQty = res.MaxQuantity
				if res.Reason == commerce.ReasonBelowMinimum {
					covReason = res.DisplayReasonAr()
				}
			case commerce.DispositionBlocked:
				isCovered = true
				canAddToCart = false
				maxOrderQty = res.MaxQuantity
				covReason = res.DisplayReasonAr()
			default: // commerce.DispositionHidden
				droppedCount++
				continue
			}
		} else {
			isCovered = false
			canAddToCart = false
			covReason = i18n.T(lang, "buying.select_branch_first")
		}

		// Products MUST only appear if they are ready for ordering (جاهزة للطلب):
		// 1. Must be covered by delivery to the branch
		// 2. Must be orderable (can add to cart)
		// 3. Must have available stock
		if !isCovered || !canAddToCart || off.AvailableStock <= 0 {
			droppedCount++
			continue
		}

		distKM := 0.0
		distText := ""
		if hasCustCoords && off.VendorLatitude != nil && off.VendorLongitude != nil &&
			(*off.VendorLatitude != 0 || *off.VendorLongitude != 0) {
			distKM = calculateHaversineKM(custLat, custLng, *off.VendorLatitude, *off.VendorLongitude)
			distText = formatDistanceKMText(distKM, lang)
		}

		prodNameAr := off.ProductName.Get(i18n.AR)
		prodNameEn := off.ProductName.Get(i18n.EN)
		if prodNameAr == "" {
			prodNameAr = prodNameEn
		}
		if prodNameEn == "" {
			prodNameEn = prodNameAr
		}

		varUnitName := off.VariantName.Get(i18n.AR)
		if varUnitName == "" {
			varUnitName = off.VariantName.Get(i18n.EN)
		}

		img := off.VariantImage
		if img == "" {
			img = off.ProductImage
		}

		sku := off.VariantSKU
		if sku == "" {
			sku = off.ProductSKU
		}

		expiryStr := ""
		if off.ExpiryDate != nil && !off.ExpiryDate.IsZero() {
			expiryStr = off.ExpiryDate.Format("2006-01-02")
		}

		cards = append(cards, &pages.SupplierVariantCard{
			VariantID:      off.VariantID,
			ProductID:      off.ProductID,
			ProductNameAr:  prodNameAr,
			ProductNameEn:  prodNameEn,
			ProductImage:   img,
			VariantName:    varUnitName,
			SKU:            sku,
			DosageForm:     off.DosageForm,
			Manufacturer:   off.ManufacturingCompany,
			BrandID:        off.BrandID,
			BrandName:      off.BrandName,
			BrandLogo:      off.BrandLogo,
			ScientificName: off.ScientificName,
			PublicPrice:    off.PublicPrice,
			Price:          off.Price,
			OriginalPrice:  off.OldPrice,
			DiscountPercent: func() float64 {
				if off.Discount.IsPositive() {
					return float64(off.Discount.Minor()) / 100.0
				}
				return float64(off.DiscountPercent)
			}(),
			AvailableStock:  off.AvailableStock,
			MinOrderQty:     off.MinOrderQty,
			MaxOrderQty:     maxOrderQty,
			ExpiryDate:      expiryStr,
			SupplierID:      off.VendorOrgID,
			SupplierName:    off.VendorOrgName,
			BranchName:      off.VendorBranchName,
			CityName:        off.CityName,
			GovernorateName: off.GovernorateName,
			DistanceKM:      distKM,
			DistanceText:    distText,
			IsCovered:       isCovered,
			CoverageReason:  covReason,
			CanAddToCart:    canAddToCart,
			IsNegotiable:    off.IsNegotiable,
			IsSponsored:     off.IsSponsored,
			SponsoredTier:   off.SponsoredTier,
			IsFavorite:      favMap[off.ProductID],
		})
	}

	return cards, droppedCount
}
