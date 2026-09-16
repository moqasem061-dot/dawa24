package ui

import (
	"net/http"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/pagination"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// VendorJobsPage renders the supplier's job management surface.
func (h *UIHandler) VendorJobsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/jobs", http.StatusSeeOther)
		return
	}

	page := pagination.PageNumber(r)
	limit := pagination.RowsPerPage(r)
	offset := (page - 1) * limit

	var jobItems []*pages.VendorJobItem
	var totalCount, publishedCount, closedCount, totalApps int

	if h.hrSvc != nil {
		stats, _ := h.hrSvc.GetJobStatsByOrg(ctx, actor.OrganizationID)
		publishedCount = stats.PublishedCount
		closedCount = stats.ClosedCount
		totalApps = stats.TotalApplications

		jobs, total, _ := h.hrSvc.ListOrgJobsWithTotal(ctx, actor.OrganizationID, limit, offset)
		totalCount = total
		for _, j := range jobs {
			if j == nil {
				continue
			}
			cnt := 0
			if appCount, err := h.hrSvc.CountApplications(ctx, j.ID); err == nil {
				cnt = appCount
			}
			jobItems = append(jobItems, &pages.VendorJobItem{
				Job:               j,
				ApplicationsCount: cnt,
			})
		}
	}

	branches := h.accessibleBranchesForActor(ctx, actor)

	noticeType := r.URL.Query().Get("notice")
	if noticeType == "" {
		noticeType = r.URL.Query().Get("notice_type")
	}
	noticeMsg := r.URL.Query().Get("msg")
	if noticeMsg == "" {
		noticeMsg = r.URL.Query().Get("message")
	}

	data := pages.VendorJobsData{
		Jobs:              jobItems,
		Branches:          branches,
		TotalCount:        totalCount,
		PublishedCount:    publishedCount,
		ClosedCount:       closedCount,
		TotalApplications: totalApps,
		Page:              page,
		PerPage:           limit,
		NoticeType:        noticeType,
		NoticeMsg:         noticeMsg,
	}

	h.renderPage(ctx, w, "render vendor jobs", pages.VendorJobs(lang, dir, data))
}

// VendorJobCreateSubmit publishes a vacancy for the vendor organization.
func (h *UIHandler) VendorJobCreateSubmit(w http.ResponseWriter, r *http.Request) {
	h.handleJobCreateSubmit(w, r, "/vendor/jobs")
}

// VendorJobUpdateSubmit modifies an existing vacancy for the vendor organization.
func (h *UIHandler) VendorJobUpdateSubmit(w http.ResponseWriter, r *http.Request) {
	h.handleJobUpdateSubmit(w, r, "/vendor/jobs")
}

// VendorJobToggleSubmit toggles a vacancy between published and closed.
func (h *UIHandler) VendorJobToggleSubmit(w http.ResponseWriter, r *http.Request) {
	h.handleJobToggleSubmit(w, r, "/vendor/jobs")
}

// VendorJobDeleteSubmit removes a vacancy.
func (h *UIHandler) VendorJobDeleteSubmit(w http.ResponseWriter, r *http.Request) {
	h.handleJobDeleteSubmit(w, r, "/vendor/jobs")
}

// VendorJobApplicationsJSON returns applicants for a specific vacancy as JSON.
func (h *UIHandler) VendorJobApplicationsJSON(w http.ResponseWriter, r *http.Request) {
	h.handleJobApplicationsJSON(w, r)
}

// CustomerJobsPage renders the pharmacy customer's job management surface.
func (h *UIHandler) CustomerJobsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/customer/jobs", http.StatusSeeOther)
		return
	}

	page := pagination.PageNumber(r)
	limit := pagination.RowsPerPage(r)
	offset := (page - 1) * limit

	var jobItems []*pages.VendorJobItem
	var totalCount, publishedCount, closedCount, totalApps int

	if h.hrSvc != nil {
		stats, _ := h.hrSvc.GetJobStatsByOrg(ctx, actor.OrganizationID)
		publishedCount = stats.PublishedCount
		closedCount = stats.ClosedCount
		totalApps = stats.TotalApplications

		jobs, total, _ := h.hrSvc.ListOrgJobsWithTotal(ctx, actor.OrganizationID, limit, offset)
		totalCount = total
		for _, j := range jobs {
			if j == nil {
				continue
			}
			cnt := 0
			if appCount, err := h.hrSvc.CountApplications(ctx, j.ID); err == nil {
				cnt = appCount
			}
			jobItems = append(jobItems, &pages.VendorJobItem{
				Job:               j,
				ApplicationsCount: cnt,
			})
		}
	}

	branches := h.accessibleBranchesForActor(ctx, actor)

	noticeType := r.URL.Query().Get("notice")
	if noticeType == "" {
		noticeType = r.URL.Query().Get("notice_type")
	}
	noticeMsg := r.URL.Query().Get("msg")
	if noticeMsg == "" {
		noticeMsg = r.URL.Query().Get("message")
	}

	data := pages.CustomerJobsData{
		Jobs:              jobItems,
		Branches:          branches,
		TotalCount:        totalCount,
		PublishedCount:    publishedCount,
		ClosedCount:       closedCount,
		TotalApplications: totalApps,
		Page:              page,
		PerPage:           limit,
		NoticeType:        noticeType,
		NoticeMsg:         noticeMsg,
		Permissions:       actor.Permissions,
	}

	h.renderPage(ctx, w, "render customer jobs", pages.CustomerJobs(lang, dir, data))
}

// CustomerJobCreateSubmit publishes a vacancy for the pharmacy organization.
func (h *UIHandler) CustomerJobCreateSubmit(w http.ResponseWriter, r *http.Request) {
	h.handleJobCreateSubmit(w, r, "/customer/jobs")
}

// CustomerJobUpdateSubmit modifies an existing vacancy for the pharmacy organization.
func (h *UIHandler) CustomerJobUpdateSubmit(w http.ResponseWriter, r *http.Request) {
	h.handleJobUpdateSubmit(w, r, "/customer/jobs")
}

// CustomerJobToggleSubmit toggles a vacancy between published and closed.
func (h *UIHandler) CustomerJobToggleSubmit(w http.ResponseWriter, r *http.Request) {
	h.handleJobToggleSubmit(w, r, "/customer/jobs")
}

// CustomerJobDeleteSubmit removes a vacancy.
func (h *UIHandler) CustomerJobDeleteSubmit(w http.ResponseWriter, r *http.Request) {
	h.handleJobDeleteSubmit(w, r, "/customer/jobs")
}

// CustomerJobApplicationsJSON returns applicants for a specific vacancy as JSON.
func (h *UIHandler) CustomerJobApplicationsJSON(w http.ResponseWriter, r *http.Request) {
	h.handleJobApplicationsJSON(w, r)
}

// VendorJobApplicationAcceptSubmit accepts and onboards an applicant for vendor.
func (h *UIHandler) VendorJobApplicationAcceptSubmit(w http.ResponseWriter, r *http.Request) {
	h.handleJobApplicationAccept(w, r)
}

// CustomerJobApplicationAcceptSubmit accepts and onboards an applicant for pharmacy customer.
func (h *UIHandler) CustomerJobApplicationAcceptSubmit(w http.ResponseWriter, r *http.Request) {
	h.handleJobApplicationAccept(w, r)
}

// VendorJobApplicationRejectSubmit rejects an application for vendor.
func (h *UIHandler) VendorJobApplicationRejectSubmit(w http.ResponseWriter, r *http.Request) {
	h.handleJobApplicationReject(w, r)
}

// CustomerJobApplicationRejectSubmit rejects an application for pharmacy customer.
func (h *UIHandler) CustomerJobApplicationRejectSubmit(w http.ResponseWriter, r *http.Request) {
	h.handleJobApplicationReject(w, r)
}

type acceptApplicantReq struct {
	BranchID   int64  `json:"branch_id"`
	RoleKey    string `json:"role_key"`
	JobTitle   string `json:"job_title"`
	BaseSalary string `json:"base_salary"`
	Notes      string `json:"notes"`
}

type rejectApplicantReq struct {
	Notes string `json:"notes"`
}
