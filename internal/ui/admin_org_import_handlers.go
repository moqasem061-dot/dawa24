package ui

import (
	"net/http"
	"strings"

	"github.com/moqasem061-dot/dawa24/internal/modules/org"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
	"github.com/moqasem061-dot/dawa24/internal/shared/pagination"
	"github.com/moqasem061-dot/dawa24/internal/ui/pages"
)

// AdminOrgImportPage renders the organization product import hub.
func (h *UIHandler) AdminOrgImportPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	page := pagination.PageNumber(r)
	limit := pagination.RowsPerPage(r)
	offset := (page - 1) * limit

	tab := strings.TrimSpace(r.URL.Query().Get("tab"))
	if tab != "vendor" {
		tab = "pharmacy"
	}

	q := strings.TrimSpace(r.URL.Query().Get("q"))
	noticeType := r.URL.Query().Get("notice_type")
	noticeMsg := r.URL.Query().Get("notice")

	var orgType org.OrganizationType
	if tab == "vendor" {
		orgType = org.TypeVendor
	} else {
		orgType = org.TypeCustomer
	}

	var orgs []*org.Organization
	var totalCount int
	var totalPharmacies, totalVendors int

	if h.orgSvc != nil {
		sysCtx := database.AsSystem(ctx)
		var err error
		orgs, totalCount, err = h.orgSvc.ListOrganizationsWithTotal(sysCtx, q, &orgType, nil, limit, offset)
		if err != nil {
			h.log.ErrorContext(ctx, "failed to list organizations for import", "error", err, "type", orgType)
		}

		pharmType := org.TypeCustomer
		vendType := org.TypeVendor
		if loaded, err := h.orgSvc.CountOrganizations(sysCtx, &pharmType, nil); err != nil {
			h.logLoadErr(ctx, "orgSvc.CountOrganizations", err)
		} else {
			totalPharmacies = loaded
		}
		if loaded, err := h.orgSvc.CountOrganizations(sysCtx, &vendType, nil); err != nil {
			h.logLoadErr(ctx, "orgSvc.CountOrganizations", err)
		} else {
			totalVendors = loaded
		}
	}

	data := pages.AdminOrgImportPageData{
		Organizations:   orgs,
		TotalOrgs:       totalPharmacies + totalVendors,
		TotalPharmacies: totalPharmacies,
		TotalVendors:    totalVendors,
		ActiveTab:       tab,
		SearchQuery:     q,
		Page:            page,
		PerPage:         limit,
		TotalCount:      totalCount,
		NoticeType:      noticeType,
		NoticeMsg:       noticeMsg,
	}

	h.renderPage(ctx, w, "render admin org import page", pages.AdminOrgImportPage(data, lang, dir))
}
