package ui

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/moqasem061-dot/dawa24/internal/modules/compare"
	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
	"github.com/moqasem061-dot/dawa24/internal/ui/components"
	"github.com/moqasem061-dot/dawa24/internal/ui/pages"
)

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
		if loaded, err := h.compareSvc.ListFiles(sysCtx, effectiveUserID, orgPtr, nil); err != nil {
			h.logLoadErr(ctx, "compareSvc.ListFiles", err)
		} else {
			files = loaded
		}
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
	switch tab {
	case "your_better":
		o := compare.OutcomeYourBetter
		outcome = &o
	case "equal":
		o := compare.OutcomeEqual
		outcome = &o
	case "competitor_better":
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
		Result:       result,
		Files:        files,
		SourceFileID: sourceID,
		TargetFileID: targetID,
		Query:        q,
		MinPrice:     minPStr,
		MaxPrice:     maxPStr,
		MinDiscount:  minDStr,
		MaxDiscount:  maxDStr,
		ActiveTab:    tab,
		IsCustomer:   actor.IsCustomer(),
		IsAdmin:      isAdminActing,
		TargetOrgID: func() int64 {
			if orgPtr != nil {
				return *orgPtr
			}
			return 0
		}(),
		TargetOrgName: targetOrgName,
		Pagination:    pagination,
		TotalCount:    totalCount,
	}

	h.renderPage(ctx, w, "render head to head page", pages.CompareHeadToHeadPage(lang, dir, pageData))
}
