package ui

import (
	"context"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/modules/promo"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// offersForProduct turns the approved vendor variants and promo offers selling the product into
// storefront rows. Every price passes through promo.EffectivePrice when discounts apply.
//
// env carries batch-prefetched org/branch/stock/promo lookups; build it once
// per page with buildOfferEnv so rendering a page costs constant queries
// instead of a few per variant.
func (h *UIHandler) offersForProduct(ctx context.Context, product *catalog.Product, variants []*catalog.ProductVariant, env *offerEnv, langOptional ...string) []pages.SupplierOffer {
	if product == nil {
		return nil
	}
	lang := "ar"
	if len(langOptional) > 0 && langOptional[0] != "" {
		lang = langOptional[0]
	}
	if env == nil {
		env = h.buildOfferEnv(ctx, []int64{product.ID}, map[int64][]*catalog.ProductVariant{
			product.ID: variants,
		})
	}

	offers := make([]pages.SupplierOffer, 0, len(variants)+2)
	seenSuppliers := make(map[int64]int) // supplierID -> index in offers

	actor, hasActor := authctx.From(ctx)
	if hasActor && actor.IsStaff {
		return nil
	}
	isBuyer := hasActor && actor.IsBuyer()
	buyerOrg := buyerOrgID(ctx)
	customerBranchID := int64(0)
	if isBuyer {
		customerBranchID = h.buyingBranchID(ctx, &actor)
	}
	if customerBranchID <= 0 {
		return nil
	}
	custLat, custLng, hasCustCoords := h.buyingBranchCoords(ctx, &actor)

	// 1. Process all direct vendor supply variants
	for _, v := range variants {
		if v == nil || v.OrganizationID <= 0 {
			continue
		}
		// A supplier browsing the catalogue must not be shown its own stock.
		// Dropping the row here rather than marking it unbuyable is deliberate:
		// this is the function every buying screen assembles its offers from,
		// so one refusal keeps own stock out of the catalogue, the product
		// page, the supplier profile and the offer comparison at once.
		if ownedByBuyer(buyerOrg, v.OrganizationID) {
			continue
		}

		orgn := env.org(v.OrganizationID)
		supplierName := orgName(orgn)
		if supplierName == "" {
			supplierName = i18n.T(lang, "offers.default_supplier_name")
		}

		minQty := v.MinOrderQty
		if minQty <= 0 {
			minQty = 1
		}

		expiryStr := ""
		if v.ExpiryDate != nil {
			expiryStr = v.ExpiryDate.Format("2006-01-02")
		}

		price := v.Price
		oldPrice := v.Price
		discountAmt := money.Zero
		var discountBPS int64 = 0

		discPct := 0.0
		if v.Discount.IsPositive() {
			discPct = float64(v.Discount.Minor()) / 100.0
		} else if v.CostDiscountPercentage > 0 && v.CostDiscountPercentage < 100 {
			discPct = v.CostDiscountPercentage
		}

		if discPct > 0 && discPct < 100 && v.Price.IsPositive() {
			oldPrice = v.Price
			if product != nil && product.Price.IsPositive() && product.Price.Minor() > oldPrice.Minor() {
				oldPrice = product.Price
			}
			discountBPS = int64(discPct * 100.0)
			discMinor := int64(float64(oldPrice.Minor()) * (discPct / 100.0))
			discountAmt = money.FromMinor(discMinor)
			price = money.FromMinor(oldPrice.Minor() - discMinor)
		} else if product != nil && product.Price.IsPositive() && v.Price.IsPositive() && product.Price.Minor() > v.Price.Minor() {
			oldPrice = product.Price
			price = v.Price
			diff := product.Price.Minor() - v.Price.Minor()
			discountAmt = money.FromMinor(diff)
			discountBPS = diff * 10000 / int64(product.Price.Minor())
		} else if !price.IsPositive() && product != nil && product.EffectivePrice().IsPositive() {
			price = product.EffectivePrice()
			oldPrice = price
		}

		// Resolve actual stock from inventory.stocks (prefetched).
		stockQty := env.stockQty(v.ID)
		if stockQty == 0 && v.StockQty > 0 {
			stockQty = v.StockQty
		}

		branchNameStr := ""
		cityNameStr := ""
		var distKM float64 = 0
		var distText string = ""

		var venBranch *org.Branch
		if v.BranchID != nil && *v.BranchID > 0 {
			venBranch = env.branch(*v.BranchID)
			if venBranch != nil {
				if venBranch.Name["ar"] != "" {
					branchNameStr = venBranch.Name["ar"]
				} else {
					branchNameStr = venBranch.Name["en"]
				}
				if venBranch.Address != "" {
					cityNameStr = venBranch.Address
				}
			}
		}

		if hasCustCoords && venBranch != nil && venBranch.Latitude != nil && venBranch.Longitude != nil && (*venBranch.Latitude != 0 || *venBranch.Longitude != 0) {
			distKM = calculateHaversineKM(custLat, custLng, *venBranch.Latitude, *venBranch.Longitude)
			distText = formatDistanceKMText(distKM, lang)
		}

		isCovered := false
		canAddToCart := false
		covReason := ""
		// maxOrderQty is what this branch may actually take. It stays zero
		// unless an availability check ran, and the card then falls back to the
		// stock — the behaviour every signed-out or branchless visitor had
		// before quotas existed.
		maxOrderQty := 0

		if isBuyer {
			if h.commSvc != nil && customerBranchID > 0 {
				res, err := h.commSvc.CheckAvailability(ctx, commerce.AvailabilityRequest{
					VariantID:        v.ID,
					VendorOrgID:      v.OrganizationID,
					CustomerOrgID:    actor.OrganizationID,
					CustomerBranchID: customerBranchID,
					Quantity:         minQty,
					When:             time.Now(),
				})
				if err == nil {
					maxOrderQty = res.MaxQuantity
					covReason = res.DisplayReasonAr()
					switch res.Disposition() {
					case commerce.DispositionOrderable:
						isCovered = true
						canAddToCart = (stockQty > 0)
					case commerce.DispositionBlocked:
						isCovered = true
						canAddToCart = false
					default: // commerce.DispositionHidden
						continue
					}
				} else {
					continue
				}
			} else if customerBranchID <= 0 {
				isCovered = false
				canAddToCart = false
				covReason = i18n.T(lang, "buying.select_branch_first")
			}
		} else {
			isCovered = false
			canAddToCart = false
			if !hasActor {
				covReason = i18n.T(lang, "buying.sign_in_to_order")
			} else {
				covReason = i18n.T(lang, "buying.approved_companies_only")
			}
		}

		off := pages.SupplierOffer{
			VariantID:        v.ID,
			SupplierID:       v.OrganizationID,
			SupplierName:     supplierName,
			IsVerified:       orgn == nil || orgn.Status == org.StatusApproved,
			Price:            price,
			OldPrice:         oldPrice,
			DiscountAmount:   discountAmt,
			DiscountBPS:      discountBPS,
			AvailableStock:   stockQty,
			MaxOrderQty:      maxOrderQty,
			MinOrderQty:      minQty,
			BatchNumber:      v.BatchNumber,
			ExpiryDate:       expiryStr,
			DeliveryEstimate: i18n.T(lang, "offers.delivery_estimate_24h"),
			ColdChain:        true,
			BranchName:       branchNameStr,
			CityName:         cityNameStr,
			DistanceKM:       distKM,
			DistanceText:     distText,
			IsCovered:        isCovered,
			CanAddToCart:     canAddToCart,
			CoverageReason:   covReason,
			IsNegotiable:     v.IsNegotiable,
		}

		seenSuppliers[v.OrganizationID] = len(offers)
		offers = append(offers, off)
	}

	// 2. Check for promotional discounts from promo module (prefetched)
	var promoBuyer *offerBuyer
	for _, row := range env.offersFor(product.ID) {
		if row == nil || row.Offer == nil || row.Product == nil {
			continue
		}
		// A promo offer is a second way onto this list, so it needs the same
		// refusal: a supplier must not see its own promotion offered back.
		if ownedByBuyer(buyerOrg, row.Offer.OrganizationID) {
			continue
		}

		listPrice := product.EffectivePrice()
		price, bd := promo.EffectivePrice(listPrice, row.Product, row.Offer)

		matchedExisting := false
		for i := range offers {
			if offers[i].SupplierID != row.Offer.OrganizationID {
				continue
			}
			if row.Product.VariantID != nil && *row.Product.VariantID > 0 && *row.Product.VariantID != offers[i].VariantID {
				continue
			}
			matchedExisting = true

			hasExistingDiscount := offers[i].DiscountBPS > 0 || offers[i].DiscountAmount.IsPositive()
			isPromoDiscounted := bd.DiscountBPS > 0 || bd.DiscountAmount.IsPositive()
			isPromoBetter := bd.DiscountBPS > offers[i].DiscountBPS ||
				(price.IsPositive() && offers[i].Price.IsPositive() && price.Minor() < offers[i].Price.Minor())

			if !hasExistingDiscount || (isPromoDiscounted && isPromoBetter) {
				offers[i].OfferID = row.Offer.ID
				offers[i].Price = price
				offers[i].OldPrice = bd.ListPrice
				offers[i].DiscountAmount = bd.DiscountAmount
				offers[i].DiscountBPS = bd.DiscountBPS
			}
		}

		if !matchedExisting {
			orgn := env.org(row.Offer.OrganizationID)
			sName := orgName(orgn)
			if sName == "" {
				sName = i18n.T(lang, "offers.default_supplier_name")
			}

			promoIsCovered := false
			promoCanAdd := false
			promoCovReason := ""
			if isBuyer {
				if promoBuyer == nil {
					resolved := h.buyerOfferQuery(ctx, actor, 0)
					promoBuyer = &resolved
				}
				promoCovReason = h.offerRefusalFor(ctx, *promoBuyer, row.Offer.ID)
				promoIsCovered = promoCovReason == ""
				promoCanAdd = promoIsCovered
				if !promoIsCovered {
					continue
				}
			} else {
				// Guest or non-buyer browsing the catalog
				promoIsCovered = true
				promoCanAdd = false
			}

			varID := int64(0)
			if row.Product.VariantID != nil && *row.Product.VariantID > 0 {
				varID = *row.Product.VariantID
			}
			if varID <= 0 {
				for _, v := range variants {
					if v != nil && v.OrganizationID == row.Offer.OrganizationID && v.DeletedAt == nil && v.Status == catalog.StatusActive {
						varID = v.ID
						break
					}
				}
			}
			if varID <= 0 {
				promoCanAdd = false
				promoCovReason = "الصنف غير متوفر حالياً لدى المورد"
			}

			newOffer := pages.SupplierOffer{
				VariantID:        varID,
				OfferID:          row.Offer.ID,
				SupplierID:       row.Offer.OrganizationID,
				SupplierName:     sName,
				IsVerified:       orgn == nil || orgn.Status == org.StatusApproved,
				Price:            price,
				OldPrice:         bd.ListPrice,
				DiscountAmount:   bd.DiscountAmount,
				DiscountBPS:      bd.DiscountBPS,
				MinOrderQty:      row.Product.CustomQty,
				DeliveryEstimate: i18n.T(lang, "offers.delivery_estimate_24h"),
				ColdChain:        true,
				IsCovered:        promoIsCovered,
				CanAddToCart:     promoCanAdd,
				CoverageReason:   promoCovReason,
			}
			if newOffer.MinOrderQty <= 0 {
				newOffer.MinOrderQty = 1
			}
			seenSuppliers[row.Offer.OrganizationID] = len(offers)
			offers = append(offers, newOffer)
		}
	}

	sortSupplierOffers(offers)

	return offers
}

// orgName prefers the Arabic trade name, then the English one, then the
// registered legal name.
func orgName(o *org.Organization) string {
	if o == nil {
		return ""
	}
	if o.TradeName["ar"] != "" {
		return o.TradeName["ar"]
	}
	if o.TradeName["en"] != "" {
		return o.TradeName["en"]
	}
	return o.LegalName
}
