package ui

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/modules/promo"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

func (h *UIHandler) CustomerCatalogPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	// /catalog is authenticated-only: guests are sent to login instead of
	// receiving a capped public listing.
	actor, ok := authctx.From(ctx)
	if !ok || actor.UserID == 0 {
		http.Redirect(w, r, "/auth/login?redirect="+url.QueryEscape(r.URL.RequestURI()), http.StatusSeeOther)
		return
	}
	lang, dir := h.localeAndDir(r)

	// 1. Security & Anti-Scraping / Bot Defense
	// Honeypot trap check: the filter form carries a hidden field no person can
	// see or tab into, so a value in it means the caller submitted the form by
	// reading the HTML.
	if botTrap := r.URL.Query().Get("company_tax_ref"); botTrap != "" {
		h.scrape.Penalize(r, "honeypot_field")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = pages.CustomerCatalog(pages.CatalogPageData{
			Page:     1,
			PageSize: 24,
			ViewMode: "grid",
		}, lang, dir, h.isHTMX(r)).Render(ctx, w)
		return
	}

	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "SAMEORIGIN")

	// Sanitize and cap search query string to prevent ReDoS / query stuffing
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if len(query) > 80 {
		query = query[:80]
	}

	var categoryID *int64
	if v := r.URL.Query().Get("category_id"); v != "" {
		if id, err := strconv.ParseInt(v, 10, 64); err == nil && id > 0 {
			categoryID = &id
		}
	}

	var brandID *int64
	if v := r.URL.Query().Get("brand_id"); v != "" {
		if id, err := strconv.ParseInt(v, 10, 64); err == nil && id > 0 {
			brandID = &id
		}
	}

	var minPriceMinor, maxPriceMinor *int64
	minPriceStr := r.URL.Query().Get("min_price")
	maxPriceStr := r.URL.Query().Get("max_price")
	if minPriceStr != "" {
		if a, err := money.Parse(minPriceStr); err == nil && a.IsPositive() {
			m := a.Minor()
			minPriceMinor = &m
		}
	}
	if maxPriceStr != "" {
		if a, err := money.Parse(maxPriceStr); err == nil && a.IsPositive() {
			m := a.Minor()
			maxPriceMinor = &m
		}
	}

	dosageForm := strings.TrimSpace(r.URL.Query().Get("dosage_form"))
	sortBy := strings.TrimSpace(r.URL.Query().Get("sort"))

	// Step 5: in_stock defaults to true and is a normal checkbox.
	// Decoupled from filter_applied.
	inStock := true
	if r.URL.Query().Has("in_stock") {
		inStock = r.URL.Query().Get("in_stock") == "true" || r.URL.Query().Get("in_stock") == "1"
	}

	hasDiscount := r.URL.Query().Get("has_discount") == "true"
	viewMode := r.URL.Query().Get("view")
	if viewMode != "table" && viewMode != "grid" {
		viewMode = "grid"
	}

	// 2. Rows per page (PageSize) & Page bounds enforcement
	maxPage, maxPageSize := h.guestListingBounds(r, 200, 96)

	pageSize := 24
	psVal := r.URL.Query().Get("page_size")
	if psVal == "" {
		psVal = r.URL.Query().Get("limit")
	}
	if psVal != "" {
		if ps, err := strconv.Atoi(psVal); err == nil {
			switch ps {
			case 12, 24, 48, 96:
				pageSize = ps
			default:
				pageSize = 24
			}
		}
	}
	if pageSize > maxPageSize {
		pageSize = maxPageSize
	}

	page := 1
	if pVal := r.URL.Query().Get("page"); pVal != "" {
		if p, err := strconv.Atoi(pVal); err == nil && p >= 1 {
			page = p
		}
	}
	if page > maxPage {
		page = maxPage
	}

	if actor.IsStaff {
		var categories []*catalog.Category
		if h.catSvc != nil {
			categories, _ = h.catSvc.ListCategories(ctx)
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = pages.CustomerCatalog(pages.CatalogPageData{
			Query:        query,
			Page:         1,
			PageSize:     pageSize,
			TotalItems:   0,
			Variants:     nil,
			ViewMode:     viewMode,
			IsAdminStaff: true,
			Categories:   categories,
		}, lang, dir, h.isHTMX(r)).Render(ctx, w)
		return
	}

	if h.catSvc == nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = pages.CustomerCatalog(pages.CatalogPageData{
			Query:    query,
			Page:     page,
			PageSize: pageSize,
			ViewMode: viewMode,
		}, lang, dir, h.isHTMX(r)).Render(ctx, w)
		return
	}

	offset := (page - 1) * pageSize

	// 3. Resolve buyer org, branch, and institutional works
	buyerOrg := buyerOrgID(ctx)
	customerBranchID := h.buyingBranchID(ctx, &actor)

	// Products appear ONLY if they are ready for ordering (جاهزة للطلب).
	// Without a selected customer branch, delivery coverage cannot be determined and products cannot be ordered.
	if customerBranchID <= 0 {
		var categories []*catalog.Category
		var brands []*catalog.Brand
		if h.catSvc != nil {
			categories, _ = h.catSvc.ListCategories(ctx)
			brands, _ = h.catSvc.ListBrands(database.AsSystem(ctx))
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = pages.CustomerCatalog(pages.CatalogPageData{
			Query:          query,
			Page:           1,
			PageSize:       pageSize,
			TotalItems:     0,
			Variants:       nil,
			ViewMode:       viewMode,
			RequiresBranch: true,
			Categories:     categories,
			Brands:         brands,
		}, lang, dir, h.isHTMX(r)).Render(ctx, w)
		return
	}

	offers, totalCount, batchResults, err := h.buyerOffersPage(ctx, buyerOrg, customerBranchID, catalog.BuyerOfferQuery{
		Query:          query,
		CategoryID:     categoryID,
		BrandID:        brandID,
		DosageForm:     dosageForm,
		MinPriceMinor:  minPriceMinor,
		MaxPriceMinor:  maxPriceMinor,
		OnlyDiscounted: hasDiscount,
		OnlyInStock:    inStock,
		Sort:           sortBy,
		Limit:          pageSize,
		Offset:         offset,
	})
	if err != nil {
		h.renderError(w, r, err)
		return
	}

	// 6. Build variant cards & filter Hidden
	variantCards, droppedCount := h.buildCatalogVariantCards(ctx, offers, batchResults, customerBranchID, &actor, lang)
	if droppedCount > 0 && len(offers) > 0 && float64(droppedCount)/float64(len(offers)) > 0.20 {
		h.log.WarnContext(ctx, "catalog: more than 20% of page dropped by availability probe",
			"dropped", droppedCount,
			"total_on_page", len(offers),
			"buyer_org", buyerOrg,
			"branch_id", customerBranchID,
		)
	}

	categories, _ := h.catSvc.ListCategories(ctx)
	brands, _ := h.catSvc.ListBrands(database.AsSystem(ctx))
	brandMap := make(map[int64]*catalog.Brand)
	for _, b := range brands {
		if b != nil {
			brandMap[b.ID] = b
		}
	}

	// Active category and brand names for filter pills
	activeCatName := ""
	if categoryID != nil {
		for _, cat := range categories {
			if cat != nil && cat.ID == *categoryID {
				activeCatName = cat.Name["ar"]
				if activeCatName == "" {
					activeCatName = cat.Name["en"]
				}
				break
			}
		}
	}

	activeBrandName := ""
	if brandID != nil {
		if b, ok := brandMap[*brandID]; ok && b != nil {
			activeBrandName = b.Name.Get(i18n.AR)
			if activeBrandName == "" {
				activeBrandName = b.Name.Get(i18n.EN)
			}
		}
	}

	// 7. Compute pagination metrics
	totalPages := 1
	if totalCount > 0 {
		totalPages = (totalCount + pageSize - 1) / pageSize
	}
	if totalPages < 1 {
		totalPages = 1
	}

	startItem := 0
	endItem := 0
	if totalCount > 0 && len(variantCards) > 0 {
		startItem = offset + 1
		endItem = offset + len(variantCards)
		if endItem > totalCount {
			endItem = totalCount
		}
	}

	sponsoredProductIDs := make(map[int64]bool)
	for _, vc := range variantCards {
		if vc != nil && vc.IsSponsored {
			sponsoredProductIDs[vc.ProductID] = true
		}
	}

	var catalogAds []*promo.Ad
	if h.promoSvc != nil {
		if ads, err := h.promoSvc.ListActiveAds(ctx, promo.PositionCatalogTop); err == nil {
			catalogAds = capAds(shuffleAds(ads), 1)
			h.enrichAds(ctx, catalogAds)
		}
	}

	viewData := pages.CatalogPageData{
		Variants:            variantCards,
		Categories:          categories,
		Brands:              brands,
		Query:               query,
		CategoryID:          categoryID,
		BrandID:             brandID,
		MinPrice:            minPriceStr,
		MaxPrice:            maxPriceStr,
		DosageForm:          dosageForm,
		Sort:                sortBy,
		InStock:             inStock,
		HasDiscount:         hasDiscount,
		ViewMode:            viewMode,
		Page:                page,
		PageSize:            pageSize,
		TotalItems:          totalCount,
		TotalPages:          totalPages,
		HasPrev:             page > 1,
		HasNext:             page < totalPages,
		PrevPage:            page - 1,
		NextPage:            page + 1,
		StartItem:           startItem,
		EndItem:             endItem,
		ActiveCategory:      activeCatName,
		ActiveBrand:         activeBrandName,
		SponsoredProductIDs: sponsoredProductIDs,
		CatalogAds:          catalogAds,
	}

	h.renderPage(ctx, w, "render catalog page", pages.CustomerCatalog(viewData, lang, dir, h.isHTMX(r)))
}

// buyerOffersPage is the catalogue as one buying branch sees it: the listings
// the buyer may see (institutional works, delivery coverage) and, for each,
// the availability verdict at the listing's minimum quantity. The catalogue page
// and Capsule's find_offers both read the market through here.
func (h *UIHandler) buyerOffersPage(
	ctx context.Context, buyerOrg, customerBranchID int64, q catalog.BuyerOfferQuery,
) ([]*catalog.BuyerOffer, int, map[int64]commerce.AvailabilityResult, error) {
	var allowedWorkIDs []int64
	if customerBranchID > 0 && h.orgSvc != nil {
		allowedWorkIDs, _ = h.orgSvc.ConnectedWorkIDsForBranch(database.AsSystem(ctx), customerBranchID)
	}
	// Coverage is resolved as a set and pushed into the query, not applied to
	// the rows it returns. See catalog.BuyerOfferQuery.CoveredVendorOrgIDs.
	cov := h.coveringVendorBranchesFor(ctx, customerBranchID)
	q.BuyerOrgID, q.BuyerBranchID, q.AllowedWorkIDs = buyerOrg, customerBranchID, allowedWorkIDs
	q.CoveredVendorOrgIDs, q.CoveredVendorBranchIDs, q.ApplyCoverage = cov.OrgIDs, cov.BranchIDs, cov.Evaluated
	q.CoveredOrgWideVendorOrgIDs = cov.OrgWideIDs

	offers, totalCount, err := h.catSvc.ListBuyerOffers(ctx, q)
	if err != nil {
		return nil, 0, nil, err
	}

	batchResults := make(map[int64]commerce.AvailabilityResult, len(offers))
	if customerBranchID > 0 && len(offers) > 0 && h.commSvc != nil {
		lines := make([]commerce.AvailabilityLine, len(offers))
		for i, off := range offers {
			qty := off.MinOrderQty
			if qty <= 0 {
				qty = 1
			}
			lines[i] = commerce.AvailabilityLine{VariantID: off.VariantID, VendorOrgID: off.VendorOrgID, Quantity: qty}
		}
		res, batchErr := h.commSvc.CheckAvailabilityBatch(ctx, buyerOrg, customerBranchID, time.Now(), lines)
		if batchErr != nil {
			h.log.ErrorContext(ctx, "catalog: batch availability check failed", "error", batchErr)
		} else {
			batchResults = res
		}
	}
	return offers, totalCount, batchResults, nil
}
