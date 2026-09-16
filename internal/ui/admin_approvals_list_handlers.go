package ui

import (
	"net/http"

	"github.com/muhiya/dawa24-store/internal/modules/attachments"
	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/pagination"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

func (h *UIHandler) AdminApprovalsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, _ := authctx.From(ctx)
	lang, dir := h.localeAndDir(r)

	tab := r.URL.Query().Get("tab")
	if tab == "" {
		tab = "organizations"
	}
	statusParam := r.URL.Query().Get("status")

	limit := pagination.RowsPerPage(r)
	page := pagination.PageNumber(r)
	offset := (page - 1) * limit

	data := &pages.AdminApprovalsData{
		ActiveTab:    tab,
		StatusFilter: statusParam,
		OrgDocs:      make(map[int64][]*attachments.Document),
		OrgNames:     make(map[int64]string),
		OrgPage:      page,
		OrgPerPage:   limit,
	}

	sysCtx := database.AsSystem(ctx)

	if h.orgSvc != nil {
		allList, _ := h.orgSvc.ListOrganizations(sysCtx, nil, nil, 500, 0)
		data.AllOrganizations = allList
		for _, o := range allList {
			if o != nil {
				data.OrgNames[o.ID] = o.LegalName
			}
		}

		var filterStatus *org.OrganizationStatus
		if statusParam != "" {
			st := org.OrganizationStatus(statusParam)
			filterStatus = &st
		} else if tab == "organizations" {
			st := org.StatusPending
			filterStatus = &st
		}
		list, total, err := h.orgSvc.ListOrganizationsWithTotal(sysCtx, "", nil, filterStatus, limit, offset)
		if err != nil {
			h.log.WarnContext(ctx, "admin approvals: list organizations", "error", err)
		} else {
			data.Organizations = list
			data.OrgTotalCount = total
		}
	}

	if h.attSvc != nil {
		for _, o := range data.Organizations {
			if o != nil {
				docs, _ := h.attSvc.ListByOrganization(sysCtx, o.ID)
				if len(docs) > 0 {
					data.OrgDocs[o.ID] = docs
				}
			}
		}

		docs, _, err := h.attSvc.ListAll(sysCtx, attachments.DocumentFilter{Limit: 200})
		if err == nil {
			data.UploadedDocs = docs
		}

		reqs, err := h.attSvc.ListDocumentRequests(sysCtx, actor, nil)
		if err == nil {
			data.DocRequests = reqs
		}
	}

	if h.idSvc != nil {
		filterSeekerStatus := statusParam
		if filterSeekerStatus == "all" {
			filterSeekerStatus = ""
		} else if filterSeekerStatus == "" && tab == "job_seekers" {
			// default to all or pending
		}
		seekers, err := h.idSvc.AdminListUsers(sysCtx, "job_seeker", filterSeekerStatus)
		if err == nil {
			for _, u := range seekers {
				if u == nil {
					continue
				}
				item := pages.JobSeekerApprovalItem{
					User: u,
				}
				if h.hrSvc != nil {
					prof, _ := h.hrSvc.GetJobSeekerProfile(sysCtx, u.ID)
					item.Profile = prof
				}
				data.JobSeekers = append(data.JobSeekers, item)
			}
		}
	}

	h.renderPage(ctx, w, "render admin approvals page", pages.AdminApprovals(data, lang, dir))
}
