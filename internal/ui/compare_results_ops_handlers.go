package ui

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/muhiya/dawa24-store/internal/modules/compare"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// CompareRunSubmit processes selection of suppliers and redirects to results view (Plan V5 Phase 2 §2.5.1).
func (h *UIHandler) CompareRunSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/compare/tool", http.StatusSeeOther)
		return
	}
	if actor.IsCustomer() {
		h.redirectWithNotice(w, r, "/customer/dashboard", "error", i18n.T(lang, "compare.run.vendors_only"))
		return
	}

	targetOrgID, _ := strconv.ParseInt(r.FormValue("org_id"), 10, 64)
	isAdminActing := (actor.IsStaff || actor.IsPlatformAdmin()) && targetOrgID > 0

	returnURL := "/compare/tool"
	if isAdminActing {
		returnURL = fmt.Sprintf("/admin/organizations/import/%d/compare", targetOrgID)
	}

	if err := r.ParseForm(); err != nil {
		h.redirectWithNotice(w, r, returnURL, "error", i18n.T(lang, "compare.run.request_failed"))
		return
	}

	supplierIDs := r.Form["supplier_ids"]
	if len(supplierIDs) == 0 {
		h.redirectWithNotice(w, r, returnURL, "error", i18n.T(lang, "compare.run.select_at_least_one"))
		return
	}

	if len(supplierIDs) > 10 {
		h.redirectWithNotice(w, r, returnURL, "error", i18n.T(lang, "compare.run.max_suppliers_exceeded"))
		return
	}

	// Validate all selected files are ready (have mapping applied)
	sysCtx := ctx
	if isAdminActing {
		sysCtx = database.WithTenant(database.AsSystem(ctx), targetOrgID)
	}
	if h.compareSvc != nil {
		for _, idStr := range supplierIDs {
			if id, err := strconv.ParseInt(strings.TrimSpace(idStr), 10, 64); err == nil && id > 0 {
				file, errGet := h.compareSvc.GetFile(sysCtx, id)
				if errGet == nil && file != nil && file.Status != compare.FileReady {
					// A failed file is not an unmapped one. Telling the user to
					// finish the mapping when parsing actually broke sends them
					// to a screen that cannot fix anything.
					msg := fmt.Sprintf(i18n.T(lang, "compare.run.mapping_needed"), file.SupplierName)
					if file.Status == compare.FileFailed {
						reason := strings.TrimSpace(file.ErrorMessage)
						if reason == "" {
							reason = i18n.T(lang, "compare.run.read_content_failed")
						}
						msg = fmt.Sprintf(i18n.T(lang, "compare.run.file_failed_format"), file.SupplierName, reason)
					}
					h.redirectWithNotice(w, r, returnURL, "error", msg)
					return
				}
			}
		}
	}

	queryParam := strings.Join(supplierIDs, ",")
	redirectURL := "/compare/results?suppliers=" + queryParam
	if isAdminActing {
		redirectURL += fmt.Sprintf("&org_id=%d", targetOrgID)
	}
	http.Redirect(w, r, redirectURL, http.StatusSeeOther)
}

// CompareMarketBenchmarkPage compares one of the caller's lists against the
// whole public market.
func (h *UIHandler) CompareMarketBenchmarkPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/compare/market-benchmark", http.StatusSeeOther)
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

	// The list selector offers the caller's own files. Falling back to every
	// file on the platform, as this used to, meant a supplier with no uploads
	// was silently shown somebody else's list as "قائمتي".
	var files []*compare.CompareFile
	if h.compareSvc != nil {
		files, _ = h.compareSvc.ListFiles(sysCtx, effectiveUserID, orgPtr, nil)
	}

	fileID, _ := strconv.ParseInt(strings.TrimSpace(r.URL.Query().Get("file")), 10, 64)
	if fileID <= 0 && len(files) > 0 {
		fileID = files[0].ID
	}

	filter := compare.BenchmarkFilter{
		FileID:         fileID,
		Query:          strings.TrimSpace(r.URL.Query().Get("q")),
		Tab:            strings.TrimSpace(r.URL.Query().Get("tab")),
		Sort:           strings.TrimSpace(r.URL.Query().Get("sort")),
		OrganizationID: orgPtr,
		MinPrice:       optionalFloat(r.URL.Query().Get("min_price")),
		MaxPrice:       optionalFloat(r.URL.Query().Get("max_price")),
		MinDiscount:    optionalFloat(r.URL.Query().Get("min_discount")),
		MaxDiscount:    optionalFloat(r.URL.Query().Get("max_discount")),
	}
	if filter.Tab == "" {
		filter.Tab = "all"
	}

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page <= 0 {
		page = 1
	}

	limit := 25
	if lStr := strings.TrimSpace(r.URL.Query().Get("limit")); lStr != "" {
		if l, err := strconv.Atoi(lStr); err == nil && (l == 10 || l == 25 || l == 50 || l == 100) {
			limit = l
		}
	}

	var (
		result *compare.BenchmarkResult
		failed bool
	)
	if h.compareSvc != nil && fileID > 0 {
		res, err := h.compareSvc.RunMarketBenchmark(database.AsSystem(ctx), filter)
		if err != nil {
			h.log.ErrorContext(ctx, "failed to run market benchmark", "error", err, "file", fileID)
			failed = true
		} else {
			result = res
		}
	}

	var (
		pagedRows  []*compare.BenchmarkRow
		totalCount int
		totalPages int
	)
	if result != nil {
		totalCount = len(result.Rows)
		if totalCount > 0 {
			totalPages = (totalCount + limit - 1) / limit
			if page > totalPages {
				page = totalPages
			}
			start := (page - 1) * limit
			end := start + limit
			if end > totalCount {
				end = totalCount
			}
			pagedRows = result.Rows[start:end]
		} else {
			totalPages = 1
		}
	} else {
		totalPages = 1
	}

	var targetOrgName string
	if isAdminActing && h.orgSvc != nil {
		targetOrgName, _ = h.resolveTargetOrgInfo(sysCtx, *orgPtr)
	}

	pageData := pages.MarketBenchmarkPageData{
		Result:        result,
		PagedRows:     pagedRows,
		Failed:        failed,
		Files:         files,
		FileID:        fileID,
		Query:         filter.Query,
		MinPrice:      strings.TrimSpace(r.URL.Query().Get("min_price")),
		MaxPrice:      strings.TrimSpace(r.URL.Query().Get("max_price")),
		MinDiscount:   strings.TrimSpace(r.URL.Query().Get("min_discount")),
		MaxDiscount:   strings.TrimSpace(r.URL.Query().Get("max_discount")),
		ActiveTab:     filter.Tab,
		Sort:          filter.Sort,
		IsCustomer:    actor.IsCustomer(),
		IsAdmin:       isAdminActing,
		TargetOrgID:   func() int64 { if orgPtr != nil { return *orgPtr }; return 0 }(),
		TargetOrgName: targetOrgName,
		Page:          page,
		Limit:         limit,
		TotalCount:    totalCount,
		TotalPages:    totalPages,
	}

	h.renderPage(ctx, w, "render market benchmark page", pages.CompareMarketBenchmarkPage(lang, dir, pageData))
}

// optionalFloat reads a query parameter that may legitimately be absent.
func optionalFloat(raw string) *float64 {
	v, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil || v < 0 {
		return nil
	}
	return &v
}
