package ui

import (
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/modules/workflow"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/pagination"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// SupplierProfilePage renders a supplier's public profile.
func (h *UIHandler) SupplierProfilePage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, hasActor := authctx.From(ctx)
	if !hasActor || actor.UserID == 0 {
		hasActor = false
		if strings.HasPrefix(r.URL.Path, "/customer/") {
			http.Redirect(w, r, "/auth/login?redirect="+url.QueryEscape(r.URL.RequestURI()), http.StatusSeeOther)
			return
		}
	}
	lang, dir := h.localeAndDir(r)

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || h.orgSvc == nil {
		h.renderError(w, r, err)
		return
	}

	sysCtx := database.AsSystem(ctx)
	o, err := h.orgSvc.GetOrganization(sysCtx, id)
	if err != nil {
		h.renderError(w, r, err)
		return
	}
	// Allow approved or active suppliers
	if o.Status == org.StatusRejected || o.Status == org.StatusSuspended {
		h.renderError(w, r, fmt.Errorf("%s", i18n.T(lang, "suppliers.vendor_unavailable")))
		return
	}
	// A supplier reaching its own profile through the buying surface is sent
	// to its own dashboard. The page is a buying screen — follow, message,
	// review, add to cart — and every one of those is meaningless aimed at
	// yourself. The company's own view of itself is /vendor/organization.
	if hasActor && ownedByBuyer(buyerOrgID(ctx), o.ID) {
		h.redirectHome(w, r, i18n.T(lang, "err.own_organization_supply"))
		return
	}

	branches, _ := h.orgSvc.ListBranches(sysCtx, id)
	var coverages []*workflow.CoverageView
	if h.wfSvc != nil {
		coverages, _ = h.wfSvc.ListCoverageForOrganization(sysCtx, id)
	}
	workingHours, coverageDays, coverageAreas, isOpenNow, statusNote := computeVendorWorkingStatus(branches, coverages)

	q := strings.TrimSpace(r.URL.Query().Get("q"))
	page := pagination.PageNumber(r)
	limit := pagination.RowsPerPage(r)

	tab := strings.TrimSpace(r.URL.Query().Get("tab"))
	if tab == "" {
		tab = "catalog"
	}

	data := pages.SupplierProfileData{
		Org:           o,
		Branches:      branches,
		Coverages:     coverages,
		WorkingHours:  workingHours,
		CoverageDays:  coverageDays,
		CoverageAreas: coverageAreas,
		IsOpenNow:     isOpenNow,
		StatusNote:    statusNote,
		CurrentPage:   page,
		PerPage:       limit,
		SearchQuery:   q,
		ActiveTab:     tab,
	}

	data.VariantMeta = make(map[int64]pages.SupplierVariantMeta)
	isBuyer := hasActor && actor.IsBuyer()
	var buyerOrg int64
	customerBranchID := int64(0)
	var allowedWorkIDs []int64
	if isBuyer {
		buyerOrg = buyerOrgID(ctx)
		customerBranchID = h.buyingBranchID(ctx, &actor)
		if customerBranchID > 0 && h.orgSvc != nil {
			allowedWorkIDs, _ = h.orgSvc.ConnectedWorkIDsForBranch(database.AsSystem(ctx), customerBranchID)
		}
	}

	stockFilter := catalog.StockFilter(r.URL.Query().Get("stock"))
	onlyInStock := (stockFilter == catalog.StockFilterIn)

	if h.catSvc != nil {
		offset := (page - 1) * limit
		// Coverage joins the query rather than filtering its result, for the
		// same reason it does on the catalogue: this is the count the pager
		// shows. See catalog.BuyerOfferQuery.CoveredVendorOrgIDs.
		cov := h.coveringVendorBranchesFor(ctx, customerBranchID)

		buyerOfferQuery := catalog.BuyerOfferQuery{
			BuyerOrgID:     buyerOrg,
			SupplierOrgID:  id,
			BuyerBranchID:  customerBranchID,
			AllowedWorkIDs: allowedWorkIDs,
			Query:          q,
			OnlyInStock:    onlyInStock,
			Limit:          limit,
			Offset:         offset,

			CoveredVendorOrgIDs:        cov.OrgIDs,
			CoveredVendorBranchIDs:     cov.BranchIDs,
			CoveredOrgWideVendorOrgIDs: cov.OrgWideIDs,
			ApplyCoverage:              cov.Evaluated,
		}

		offers, total, err := h.catSvc.ListBuyerOffers(ctx, buyerOfferQuery)
		if err == nil {
			data.TotalVariants = total
			if total > 0 {
				data.TotalPages = int(math.Ceil(float64(total) / float64(limit)))
			} else {
				data.TotalPages = 1
			}

			batchResults := make(map[int64]commerce.AvailabilityResult, len(offers))
			if customerBranchID > 0 && len(offers) > 0 && h.commSvc != nil {
				lines := make([]commerce.AvailabilityLine, len(offers))
				for i, off := range offers {
					qty := off.MinOrderQty
					if qty <= 0 {
						qty = 1
					}
					lines[i] = commerce.AvailabilityLine{
						VariantID:   off.VariantID,
						VendorOrgID: id,
						Quantity:    qty,
					}
				}
				if res, bErr := h.commSvc.CheckAvailabilityBatch(ctx, buyerOrg, customerBranchID, time.Now(), lines); bErr == nil {
					batchResults = res
				}
			}

			variants := make([]*catalog.ProductVariant, 0, len(offers))
			productsMap := make(map[int64]*catalog.Product, len(offers))
			for _, off := range offers {
				if off == nil {
					continue
				}

				availStock := off.AvailableStock
				minQty := off.MinOrderQty
				if minQty <= 0 {
					minQty = 1
				}

				isCovered := true
				canAddToCart := (availStock > 0)
				covReason := ""
				maxOrderQty := availStock

				if isBuyer {
					if customerBranchID <= 0 {
						isCovered = false
						canAddToCart = false
						covReason = i18n.T(lang, "buying.select_branch_first")
					} else {
						res, ok := batchResults[off.VariantID]
						if ok {
							maxOrderQty = res.MaxQuantity
							covReason = res.DisplayReasonAr()
							switch res.Disposition() {
							case commerce.DispositionOrderable:
								isCovered = true
								canAddToCart = (availStock > 0)
								if res.Reason == commerce.ReasonBelowMinimum {
									covReason = res.DisplayReasonAr()
								}
							case commerce.DispositionBlocked:
								isCovered = true
								canAddToCart = false
							default: // commerce.DispositionHidden
								continue
							}
						} else {
							isCovered = false
							canAddToCart = false
							covReason = i18n.T(lang, "offers.cov_reason_verify_failed")
							continue
						}
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

				var quotaLimitPtr *int
				if off.QuotaLimit > 0 {
					q := off.QuotaLimit
					quotaLimitPtr = &q
				}

				v := &catalog.ProductVariant{
					ID:             off.VariantID,
					ProductID:      off.ProductID,
					OrganizationID: off.VendorOrgID,
					BranchID:       off.VendorBranchID,
					Name:           off.VariantName,
					SKU:            off.VariantSKU,
					Price:          off.Price,
					Discount:       off.Discount,
					Status:         catalog.ProductStatus(off.Status),
					ExpiryDate:     off.ExpiryDate,
					StockQty:       availStock,
					MinOrderQty:    minQty,
					QuotaLimit:     quotaLimitPtr,
					IsFeatured:     off.IsFeatured,
					IsNegotiable:   off.IsNegotiable,
					Image:          off.VariantImage,
				}

				p := &catalog.Product{
					ID:                     off.ProductID,
					Name:                   off.ProductName,
					Image:                  off.ProductImage,
					SKU:                    off.ProductSKU,
					Barcode:                off.ProductBarcode,
					Price:                  off.PublicPrice,
					OldPrice:               off.OldPrice,
					ScientificName:         off.ScientificName,
					DosageForm:             off.DosageForm,
					ManufacturingCompanies: off.ManufacturingCompany,
					BrandID:                off.BrandID,
					CategoryID:             off.CategoryID,
				}

				variants = append(variants, v)
				productsMap[off.ProductID] = p
				data.VariantMeta[off.VariantID] = pages.SupplierVariantMeta{
					AvailableStock: availStock,
					MaxOrderQty:    maxOrderQty,
					MinOrderQty:    minQty,
					IsCovered:      isCovered,
					CoverageReason: covReason,
					CanAddToCart:   canAddToCart,
				}
			}

			data.Variants = variants
			data.ProductsMap = productsMap
		}
	}

	if h.promoSvc != nil {
		data.Sections, _ = h.promoSvc.ListHighlightSectionsByOrg(ctx, id)
	}
	if h.orgSvc != nil {
		data.Reviews, _ = h.orgSvc.ListReviews(ctx, id, 20, 0)
		if policies, err := h.orgSvc.ListPolicies(ctx, id); err == nil {
			for _, pol := range policies {
				if pol != nil && pol.PolicyType == org.PolicyTypePrivacy {
					pol.PolicyType = org.PolicyTypeWarranty
				}
			}
			data.Policies = policies
		}
		if actor, ok := authctx.From(ctx); ok {
			data.IsFollowing, _ = h.orgSvc.IsFollowing(ctx, id, actor.UserID)
		}
	}
	data.ReviewCount = len(data.Reviews)
	if data.ReviewCount > 0 {
		var sum int
		for _, rv := range data.Reviews {
			sum += rv.Rating
		}
		data.Rating = float64(sum) / float64(data.ReviewCount)
	} else {
		data.Rating = 0
	}

	h.renderPage(ctx, w, "render supplier profile", pages.SupplierProfile(lang, dir, data))
}

// SupplierFollowSubmit toggles following for the signed-in user.
func (h *UIHandler) SupplierFollowSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID, err := authctx.UserID(ctx)
	if err != nil {
		http.Redirect(w, r, "/auth/login?redirect="+r.Referer(), http.StatusSeeOther)
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	// Following your own company is not a relationship. The directory and the
	// profile both refuse it; this refuses the form post that reaches neither.
	if err == nil && h.orgSvc != nil && !ownedByBuyer(buyerOrgID(ctx), id) {
		_, _ = h.orgSvc.ToggleFollow(ctx, id, userID)
	}

	back := r.Referer()
	if back == "" {
		back = "/suppliers"
	}
	http.Redirect(w, r, back, http.StatusSeeOther)
}
