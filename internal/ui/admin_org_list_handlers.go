package ui

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/moqasem061-dot/dawa24/internal/modules/org"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
	"github.com/moqasem061-dot/dawa24/internal/shared/pagination"
	"github.com/moqasem061-dot/dawa24/internal/ui/pages"
)

// AdminOrganizationsPage renders the dedicated organizations management screen.
func (h *UIHandler) AdminOrganizationsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sysCtx := database.AsSystem(ctx)
	lang, dir := h.localeAndDir(r)

	searchQuery := strings.TrimSpace(r.URL.Query().Get("q"))
	typeParam := strings.TrimSpace(r.URL.Query().Get("type"))
	statusParam := strings.TrimSpace(r.URL.Query().Get("status"))

	page := pagination.PageNumber(r)
	limit := pagination.RowsPerPage(r)
	offset := (page - 1) * limit

	var orgs []*org.Organization
	var totalMatching int
	var stats org.AdminOrgStatsResult
	var branchCounts map[int64]int
	userCounts := make(map[int64]int)

	if h.orgSvc != nil {
		var filterType *org.OrganizationType
		if typeParam != "" {
			t := org.OrganizationType(typeParam)
			filterType = &t
		}
		var filterStatus *org.OrganizationStatus
		if statusParam != "" {
			s := org.OrganizationStatus(statusParam)
			filterStatus = &s
		}

		if loaded, err := h.orgSvc.AdminOrgStats(sysCtx); err != nil {
			h.logLoadErr(ctx, "orgSvc.AdminOrgStats", err)
		} else {
			stats = loaded
		}
		if loaded, err := h.orgSvc.CountBranchesByOrg(sysCtx); err != nil {
			h.logLoadErr(ctx, "orgSvc.CountBranchesByOrg", err)
		} else {
			branchCounts = loaded
		}
		if loaded, loaded2, err := h.orgSvc.ListOrganizationsWithTotal(sysCtx, searchQuery, filterType, filterStatus, limit, offset); err != nil {
			h.logLoadErr(ctx, "orgSvc.ListOrganizationsWithTotal", err)
		} else {
			orgs, totalMatching = loaded, loaded2
		}
	}

	data := pages.AdminOrganizationsPageData{
		Organizations:   orgs,
		TotalOrgs:       stats.TotalOrgs,
		TotalPharmacies: stats.TotalPharmacies,
		TotalVendors:    stats.TotalVendors,
		PendingCount:    stats.PendingCount,
		ApprovedCount:   stats.ApprovedCount,
		BranchCounts:    branchCounts,
		UserCounts:      userCounts,
		SearchQuery:     searchQuery,
		TypeFilter:      typeParam,
		StatusFilter:    statusParam,
		Page:            page,
		PerPage:         limit,
		TotalCount:      totalMatching,
	}

	h.renderPage(ctx, w, "render admin organizations page", pages.AdminOrganizationsPage(data, lang, dir))
}

// AdminBranchesPage renders the dedicated branches and warehouses directory.
func (h *UIHandler) AdminBranchesPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sysCtx := database.AsSystem(ctx)
	lang, dir := h.localeAndDir(r)

	searchQuery := strings.TrimSpace(r.URL.Query().Get("q"))
	statusFilter := strings.TrimSpace(r.URL.Query().Get("status"))
	orgIDFilter, _ := strconv.ParseInt(r.URL.Query().Get("org_id"), 10, 64)

	page := pagination.PageNumber(r)
	limit := pagination.RowsPerPage(r)
	offset := (page - 1) * limit

	var branches []*org.Branch
	var allOrgs []*org.Organization
	orgNames := make(map[int64]string)
	orgTypes := make(map[int64]string)
	var instWorks []*org.InstitutionalWork
	var totalBranches, activeBranches, pharmacyBranches, vendorWarehouses, filteredCount int

	if h.orgSvc != nil {
		allOrgsList, loadErr := h.orgSvc.ListOrganizations(sysCtx, nil, nil, 500, 0)
		h.logLoadErr(ctx, "orgSvc.ListOrganizations", loadErr)
		allOrgs = allOrgsList
		for _, o := range allOrgsList {
			if o != nil {
				orgNames[o.ID] = o.LegalName
				orgTypes[o.ID] = string(o.Type)
			}
		}

		stats, loadErr := h.orgSvc.AdminBranchStats(sysCtx)
		h.logLoadErr(ctx, "orgSvc.AdminBranchStats", loadErr)
		totalBranches = stats.TotalBranches
		activeBranches = stats.ActiveBranches
		pharmacyBranches = stats.PharmacyBranches
		vendorWarehouses = stats.VendorWarehouses

		filter := org.BranchFilter{
			SearchQuery:    searchQuery,
			OrganizationID: orgIDFilter,
			Status:         statusFilter,
		}

		bList, total, err := h.orgSvc.ListBranchesWithTotal(sysCtx, filter, limit, offset)
		if err != nil {
			h.log.ErrorContext(ctx, "admin list branches failed", "error", err)
		} else {
			branches = bList
			filteredCount = total
		}

		if loaded, err := h.orgSvc.ListAllFlatInstitutionalWorks(sysCtx, true); err != nil {
			h.logLoadErr(ctx, "orgSvc.ListAllFlatInstitutionalWorks", err)
		} else {
			instWorks = loaded
		}
	}

	noticeType := r.URL.Query().Get("notice")
	noticeMsg := r.URL.Query().Get("msg")
	if noticeType == "" {
		noticeType = r.URL.Query().Get("notice_type")
	}
	if noticeMsg == "" {
		noticeMsg = r.URL.Query().Get("notice_msg")
	}

	data := pages.AdminBranchesPageData{
		Branches:           branches,
		Organizations:      allOrgs,
		OrgNames:           orgNames,
		OrgTypes:           orgTypes,
		Governorates:       h.listGovernorates(ctx),
		Cities:             h.listCities(ctx),
		InstitutionalWorks: instWorks,
		TotalBranches:      totalBranches,
		FilteredCount:      filteredCount,
		ActiveBranches:     activeBranches,
		PharmacyBranches:   pharmacyBranches,
		VendorWarehouses:   vendorWarehouses,
		Page:               page,
		PerPage:            limit,
		SearchQuery:        searchQuery,
		SelectedOrgID:      orgIDFilter,
		StatusFilter:       statusFilter,
		NoticeType:         noticeType,
		NoticeMsg:          noticeMsg,
	}

	h.renderPage(ctx, w, "render admin branches page", pages.AdminBranchesPage(data, lang, dir))
}
