package ui

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/moqasem061-dot/dawa24/internal/modules/compare"
	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
	"github.com/moqasem061-dot/dawa24/internal/shared/apperr"
	"github.com/moqasem061-dot/dawa24/internal/shared/i18n"
	"github.com/moqasem061-dot/dawa24/internal/ui/components"
	"github.com/moqasem061-dot/dawa24/internal/ui/pages"
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
		allF, loadErr := h.compareSvc.ListFiles(sysCtx, effectiveUserID, orgPtr, nil)
		h.logLoadErr(ctx, "compareSvc.ListFiles", loadErr)
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
		visibleFiles, err := h.compareSvc.ListFiles(sysCtx, effectiveUserID, orgPtr, nil)
		if err != nil {
			h.renderError(w, r, err)
			return
		}
		for _, id := range fileIDs {
			if !containsCompareFile(visibleFiles, id) {
				h.renderError(w, r, apperr.NotFound("compare file"))
				return
			}
		}
		res, err := h.compareSvc.RunMultiSupplierComparison(sysCtx, fileIDs)
		if err == nil { // nolint:errswallow: the else branch reports it to the user
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
	if limit > 100 {
		limit = 100
	}

	filter := strings.TrimSpace(r.URL.Query().Get("filter"))
	if filter == "" {
		filter = "all"
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	qLower := strings.ToLower(q)

	catalogFilter := r.URL.Query().Get("catalog")
	supplierFilter := r.URL.Query().Get("supplier")
	totalFiltered := 0
	var pagedResult *compare.ComparisonResultSet

	if result != nil {
		var filteredRows []*compare.ProductComparisonRow
		for _, row := range result.Rows {
			if row == nil {
				continue
			}
			if catalogFilter == "linked" && !row.InCatalog || catalogFilter == "unlinked" && row.InCatalog {
				continue
			}
			if supplierFilter != "" && row.BestSupplier != supplierFilter {
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
		start := totalFiltered
		if page <= (totalFiltered+limit-1)/limit {
			start = (page - 1) * limit
		}
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

	queryValues.Set("catalog", catalogFilter)
	queryValues.Set("supplier", supplierFilter)
	pagination := components.PaginationProps{
		CurrentPage:     page,
		PageSize:        limit,
		TotalCount:      totalFiltered,
		BaseURL:         "/compare/results",
		QueryValues:     queryValues,
		PageSizeOptions: []int{10, 25, 50, 100},
	}

	pageData := pages.CompareResultsPageData{
		Result:         pagedResult,
		CatalogFilter:  catalogFilter,
		SupplierFilter: supplierFilter,
		Suppliers: func() []string {
			if result != nil {
				return result.Summary.SuppliersList
			}
			return nil
		}(),
		ActiveFilter:       filter,
		Query:              q,
		SuppliersParam:     supParam,
		Pagination:         pagination,
		TotalFilteredCount: totalFiltered,
		IsAdmin:            isAdminActing,
		TargetOrgID: func() int64 {
			if orgPtr != nil {
				return *orgPtr
			}
			return 0
		}(),
		TargetOrgName: targetOrgName,
	}

	h.renderPage(ctx, w, "render compare results", pages.CompareResultsPage(lang, dir, pageData))
}
