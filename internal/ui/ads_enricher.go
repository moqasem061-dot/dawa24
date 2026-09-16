package ui

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/modules/promo"
	"github.com/muhiya/dawa24-store/internal/platform/database"
)

// enrichAds fills in each sponsored ad's supplier and pricing.
//
// It used to run two queries per ad, in sequence — one for the organisation,
// one for the variant — before the landing page could render a single byte.
// With every active ad on the platform being enriched, that was hundreds of
// sequential round-trips on the most-visited page on the site, and the reason
// it felt frozen. Both lookups are now one batched query each, whatever the
// number of ads.
func (h *UIHandler) enrichAds(ctx context.Context, ads []*promo.Ad) {
	if len(ads) == 0 {
		return
	}

	sysCtx := database.AsSystem(ctx)

	// Collect every id first, then fetch each kind once.
	orgIDs := make([]int64, 0, len(ads))
	variantIDs := make([]int64, 0, len(ads))
	variantOf := make(map[*promo.Ad]int64, len(ads))
	for _, ad := range ads {
		if ad == nil {
			continue
		}
		if ad.OrganizationID != nil && *ad.OrganizationID > 0 {
			orgIDs = append(orgIDs, *ad.OrganizationID)
		}
		if id := adVariantID(ad); id > 0 {
			variantIDs = append(variantIDs, id)
			variantOf[ad] = id
		}
	}

	orgs := map[int64]*org.Organization{}
	if len(orgIDs) > 0 && h.orgSvc != nil {
		if m, err := h.orgSvc.GetOrganizations(sysCtx, orgIDs); err == nil && m != nil {
			orgs = m
		} else if err != nil {
			h.log.WarnContext(ctx, "enrich ads: batch organizations", "error", err)
		}
	}

	variants := map[int64]*catalog.ProductVariant{}
	if len(variantIDs) > 0 && h.catSvc != nil {
		if m, err := h.catSvc.GetVariantsByIDs(sysCtx, variantIDs); err == nil && m != nil {
			variants = m
		} else if err != nil {
			h.log.WarnContext(ctx, "enrich ads: batch variants", "error", err)
		}
	}

	for _, ad := range ads {
		if ad == nil {
			continue
		}

		if ad.OrganizationID != nil {
			if o := orgs[*ad.OrganizationID]; o != nil {
				name := o.TradeName.Get("ar")
				if name == "" {
					name = o.LegalName
				}
				ad.SupplierName = name
			}
		}

		if variant := variants[variantOf[ad]]; variant != nil {
			if pubMinor := variant.Price.Minor(); pubMinor > 0 {
				ad.PublicPrice = fmt.Sprintf("%.2f ج.م", float64(pubMinor)/100.0)

				discountPct := 15.0
				if variant.CostDiscountPercentage > 0 {
					discountPct = variant.CostDiscountPercentage
				} else if variant.Discount.Minor() > 0 {
					discountPct = (float64(variant.Discount.Minor()) / float64(pubMinor)) * 100.0
				}

				netMinor := int64(float64(pubMinor) * (1.0 - (discountPct / 100.0)))
				if netMinor <= 0 {
					netMinor = pubMinor
				}

				ad.DiscountPercent = fmt.Sprintf("%.0f%%", discountPct)
				ad.SupplyPrice = fmt.Sprintf("%.2f ج.م", float64(netMinor)/100.0)
			}
		}

		// Fallback copy when the ad is not tied to a priced variant.
		if ad.PublicPrice == "" {
			ad.PublicPrice = "سعر رسمي معتمد"
		}
		if ad.DiscountPercent == "" {
			ad.DiscountPercent = "خصم تجاري خاص"
		}
		if ad.SupplyPrice == "" {
			ad.SupplyPrice = "أفضل سعر توريد"
		}
	}
}

// adVariantID reads the variant an ad points at, from the explicit click target
// or from the target URL it was authored with.
func adVariantID(ad *promo.Ad) int64 {
	if ad.ClickTargetID != nil && *ad.ClickTargetID > 0 {
		return *ad.ClickTargetID
	}
	for _, marker := range []string{"variant_id=", "/catalog/"} {
		if !strings.Contains(ad.TargetURL, marker) {
			continue
		}
		parts := strings.Split(ad.TargetURL, marker)
		if len(parts) != 2 {
			continue
		}
		if id, err := strconv.ParseInt(strings.TrimSpace(parts[1]), 10, 64); err == nil && id > 0 {
			return id
		}
	}
	return 0
}
