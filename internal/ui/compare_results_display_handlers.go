package ui

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/muhiya/dawa24-store/internal/modules/compare"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/ui/components"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// CompareResultsPage renders multi-supplier comparison results with full filtering, sorting and metrics.
func (h *UIHandler) CompareResultsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/compare/results", http.StatusSeeOther)
		return
	}

	supParam := r.URL.Query().Get("suppliers")
	var fileIDs []int64
	for _, s := range strings.Split(supParam, ",") {
		if id, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64); err == nil && id > 0 {
			fileIDs = append(fileIDs, id)
		}
	}

	var (
		orgPtr        *int64
		isAdminActing bool
	)
	if actor.OrganizationID > 0 {
		orgPtr = &actor.OrganizationID
	}
	if actor.IsStaff || actor.IsPlatformAdmin() {
		if targetOrg, _ := strconv.ParseInt(r.URL.Query().Get("org_id"), 10, 64); targetOrg > 0 {
			orgPtr = &targetOrg
			isAdminActing = true
		}
	}

	sysCtx := ctx
	effectiveUserID := actor.UserID
	if orgPtr != nil {
		sysCtx = database.WithTenant(database.AsSystem(ctx), *orgPtr)
		if (actor.IsStaff || actor.IsPlatformAdmin()) && h.orgSvc != nil {
			if targetOrg, err := h.orgSvc.GetOrganization(sysCtx, *orgPtr); err == nil && targetOrg != nil && targetOrg.OwnerID > 0 {
				effectiveUserID = targetOrg.OwnerID
			}
		}
	}

	returnURL := "/compare/tool"
	var targetOrgName string
	if isAdminActing {
		returnURL = fmt.Sprintf("/admin/organizations/import/%d/compare", *orgPtr)
		if h.orgSvc != nil {
			targetOrgName, _ = h.resolveTargetOrgInfo(sysCtx, *orgPtr)
		}
	}

	if len(fileIDs) == 0 && h.compareSvc != nil {
		allF, _ := h.compareSvc.ListFiles(sysCtx, effectiveUserID, orgPtr, nil)
		for _, f := range allF {
			if f.Status == compare.FileReady && f.RowCount > 0 {
				fileIDs = append(fileIDs, f.ID)
			}
		}
		if len(fileIDs) == 0 && len(allF) > 0 {
			for _, f := range allF {
				fileIDs = append(fileIDs, f.ID)
			}
		}
	}

	if len(fileIDs) == 0 {
		h.redirectWithNotice(w, r, returnURL, "warning", i18n.T(lang, "compare.results.select_files_warning"))
		return
	}

	var result *compare.ComparisonResultSet
	if h.compareSvc != nil {
		res, err := h.compareSvc.RunMultiSupplierComparison(sysCtx, fileIDs)
		if err == nil {
			result = res
		} else {
			h.redirectWithNotice(w, r, returnURL, "error", fmt.Sprintf(i18n.T(lang, "compare.results.process_error_prefix"), h.safeMessage(err, lang)))
			return
		}
	}

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 {
		limit = 25
	}

	filter := strings.TrimSpace(r.URL.Query().Get("filter"))
	if filter == "" {
		filter = "all"
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	qLower := strings.ToLower(q)

	totalFiltered := 0
	var pagedResult *compare.ComparisonResultSet

	if result != nil {
		var filteredRows []*compare.ProductComparisonRow
		for _, row := range result.Rows {
			if row == nil {
				continue
			}
			if filter == "multi-offer" && row.TotalSuppliers <= 1 {
				continue
			}
			if filter == "single-offer" && row.TotalSuppliers != 1 {
				continue
			}
			if qLower != "" {
				haystack := strings.ToLower(row.ProductName + " " + row.SKU + " " + row.BestSupplier + " " + strings.Join(row.MissingSuppliers, " "))
				if !strings.Contains(haystack, qLower) {
					continue
				}
			}
			filteredRows = append(filteredRows, row)
		}
		totalFiltered = len(filteredRows)

		pagedRows := []*compare.ProductComparisonRow{}
		start := (page - 1) * limit
		if start < totalFiltered {
			end := start + limit
			if end > totalFiltered {
				end = totalFiltered
			}
			pagedRows = filteredRows[start:end]
		}

		resCopy := *result
		resCopy.Rows = pagedRows
		pagedResult = &resCopy
	}

	queryValues := url.Values{}
	if supParam != "" {
		queryValues.Set("suppliers", supParam)
	}
	if filter != "" && filter != "all" {
		queryValues.Set("filter", filter)
	}
	if q != "" {
		queryValues.Set("q", q)
	}
	if isAdminActing {
		queryValues.Set("org_id", strconv.FormatInt(*orgPtr, 10))
	}

	pagination := components.PaginationProps{
		CurrentPage:     page,
		PageSize:        limit,
		TotalCount:      totalFiltered,
		BaseURL:         "/compare/results",
		QueryValues:     queryValues,
		PageSizeOptions: []int{10, 25, 50, 100},
	}

	pageData := pages.CompareResultsPageData{
		Result:             pagedResult,
		ActiveFilter:       filter,
		Query:              q,
		SuppliersParam:     supParam,
		Pagination:         pagination,
		TotalFilteredCount: totalFiltered,
		IsAdmin:            isAdminActing,
		TargetOrgID:        func() int64 { if orgPtr != nil { return *orgPtr }; return 0 }(),
		TargetOrgName:      targetOrgName,
	}

	h.renderPage(ctx, w, "render compare results", pages.CompareResultsPage(lang, dir, pageData))
}

// CompareHeadToHeadPage handles head-to-head comparison between two suppliers.
func (h *UIHandler) CompareHeadToHeadPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/compare/head-to-head", http.StatusSeeOther)
		return
	}

	var (
		orgPtr        *int64
		isAdminActing bool
	)
	if actor.OrganizationID > 0 {
		orgPtr = &actor.OrganizationID
	}
	if actor.IsStaff || actor.IsPlatformAdmin() {
		if targetOrg, _ := strconv.ParseInt(r.URL.Query().Get("org_id"), 10, 64); targetOrg > 0 {
			orgPtr = &targetOrg
			isAdminActing = true
		}
	}

	sysCtx := ctx
	effectiveUserID := actor.UserID
	if orgPtr != nil {
		sysCtx = database.WithTenant(database.AsSystem(ctx), *orgPtr)
		if (actor.IsStaff || actor.IsPlatformAdmin()) && h.orgSvc != nil {
			if targetOrg, err := h.orgSvc.GetOrganization(sysCtx, *orgPtr); err == nil && targetOrg != nil && targetOrg.OwnerID > 0 {
				effectiveUserID = targetOrg.OwnerID
			}
		}
	}

	var files []*compare.CompareFile
	if h.compareSvc != nil {
		files, _ = h.compareSvc.ListFiles(sysCtx, effectiveUserID, orgPtr, nil)
	}

	sourceID, _ := strconv.ParseInt(strings.TrimSpace(r.URL.Query().Get("source")), 10, 64)
	targetID, _ := strconv.ParseInt(strings.TrimSpace(r.URL.Query().Get("target")), 10, 64)

	// Default selection to first two files if not specified and at least 2 files exist
	if sourceID <= 0 && len(files) >= 1 {
		sourceID = files[0].ID
	}
	if targetID <= 0 && len(files) >= 2 {
		targetID = files[1].ID
	} else if targetID <= 0 && len(files) == 1 {
		targetID = files[0].ID
	}

	q := strings.TrimSpace(r.URL.Query().Get("q"))
	minPStr := strings.TrimSpace(r.URL.Query().Get("min_price"))
	maxPStr := strings.TrimSpace(r.URL.Query().Get("max_price"))
	minDStr := strings.TrimSpace(r.URL.Query().Get("min_discount"))
	maxDStr := strings.TrimSpace(r.URL.Query().Get("max_discount"))
	tab := strings.TrimSpace(r.URL.Query().Get("tab"))
	if tab == "" {
		tab = "all"
	}

	var minP, maxP, minD, maxD *float64
	if v, err := strconv.ParseFloat(minPStr, 64); err == nil && v >= 0 {
		minP = &v
	}
	if v, err := strconv.ParseFloat(maxPStr, 64); err == nil && v >= 0 {
		maxP = &v
	}
	if v, err := strconv.ParseFloat(minDStr, 64); err == nil && v >= 0 {
		minD = &v
	}
	if v, err := strconv.ParseFloat(maxDStr, 64); err == nil && v >= 0 {
		maxD = &v
	}

	var outcome *compare.HeadToHeadOutcome
	if tab == "your_better" {
		o := compare.OutcomeYourBetter
		outcome = &o
	} else if tab == "equal" {
		o := compare.OutcomeEqual
		outcome = &o
	} else if tab == "competitor_better" {
		o := compare.OutcomeCompetitorBetter
		outcome = &o
	}

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 {
		limit = 25
	}

	var result *compare.HeadToHeadComparisonResult
	totalCount := 0
	if h.compareSvc != nil && sourceID > 0 && targetID > 0 {
		res, err := h.compareSvc.RunSupplierVsSupplierDetailed(sysCtx, compare.HeadToHeadFilter{
			SourceFileID: sourceID,
			TargetFileID: targetID,
			Query:        q,
			MinPrice:     minP,
			MaxPrice:     maxP,
			MinDiscount:  minD,
			MaxDiscount:  maxD,
			Outcome:      outcome,
		})
		if err != nil {
			h.log.ErrorContext(sysCtx, "failed to run head-to-head comparison", "error", err)
		} else if res != nil {
			totalCount = len(res.Rows)
			pagedRows := []*compare.HeadToHeadRow{}
			start := (page - 1) * limit
			if start < totalCount {
				end := start + limit
				if end > totalCount {
					end = totalCount
				}
				pagedRows = res.Rows[start:end]
			}
			result = res
			result.Rows = pagedRows
		}
	}

	var targetOrgName string
	if isAdminActing && h.orgSvc != nil {
		targetOrgName, _ = h.resolveTargetOrgInfo(sysCtx, *orgPtr)
	}

	queryValues := url.Values{}
	if sourceID > 0 {
		queryValues.Set("source", strconv.FormatInt(sourceID, 10))
	}
	if targetID > 0 {
		queryValues.Set("target", strconv.FormatInt(targetID, 10))
	}
	if q != "" {
		queryValues.Set("q", q)
	}
	if minPStr != "" {
		queryValues.Set("min_price", minPStr)
	}
	if maxPStr != "" {
		queryValues.Set("max_price", maxPStr)
	}
	if minDStr != "" {
		queryValues.Set("min_discount", minDStr)
	}
	if maxDStr != "" {
		queryValues.Set("max_discount", maxDStr)
	}
	if tab != "" && tab != "all" {
		queryValues.Set("tab", tab)
	}
	if isAdminActing {
		queryValues.Set("org_id", strconv.FormatInt(*orgPtr, 10))
	}

	pagination := components.PaginationProps{
		CurrentPage:     page,
		PageSize:        limit,
		TotalCount:      totalCount,
		BaseURL:         "/compare/head-to-head",
		QueryValues:     queryValues,
		PageSizeOptions: []int{10, 25, 50, 100},
	}

	pageData := pages.HeadToHeadPageData{
		Result:        result,
		Files:         files,
		SourceFileID:  sourceID,
		TargetFileID:  targetID,
		Query:         q,
		MinPrice:      minPStr,
		MaxPrice:      maxPStr,
		MinDiscount:   minDStr,
		MaxDiscount:   maxDStr,
		ActiveTab:     tab,
		IsCustomer:    actor.IsCustomer(),
		IsAdmin:       isAdminActing,
		TargetOrgID:   func() int64 { if orgPtr != nil { return *orgPtr }; return 0 }(),
		TargetOrgName: targetOrgName,
		Pagination:    pagination,
		TotalCount:    totalCount,
	}

	h.renderPage(ctx, w, "render head to head page", pages.CompareHeadToHeadPage(lang, dir, pageData))
}
