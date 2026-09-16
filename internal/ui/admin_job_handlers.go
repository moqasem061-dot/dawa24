package ui

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/hr"
	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
	"github.com/muhiya/dawa24-store/internal/shared/pagination"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// AdminJobsPage renders all job vacancies across the platform with filtering and control options.
func (h *UIHandler) AdminJobsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	page := pagination.PageNumber(r)
	limit := pagination.RowsPerPage(r)
	offset := (page - 1) * limit

	var selectedOrgID int64
	if s := r.URL.Query().Get("org_id"); s != "" {
		selectedOrgID, _ = strconv.ParseInt(s, 10, 64)
	}
	selectedStatus := strings.TrimSpace(r.URL.Query().Get("status"))
	searchQuery := strings.TrimSpace(r.URL.Query().Get("q"))

	filter := hr.AdminJobFilter{
		OrganizationID: selectedOrgID,
		Status:         selectedStatus,
		Search:         searchQuery,
		Limit:          limit,
		Offset:         offset,
	}

	var jobViews []*pages.AdminJobView
	var totalCount int
	if h.hrSvc != nil {
		offers, total, err := h.hrSvc.ListAllJobsFiltered(ctx, filter)
		if err != nil {
			h.log.WarnContext(ctx, "admin jobs: list filtered jobs", "error", err)
		} else {
			totalCount = total
			orgs := map[int64]*org.Organization{}
			if h.orgSvc != nil && len(offers) > 0 {
				ids := make([]int64, 0, len(offers))
				for _, j := range offers {
					if j.OrganizationID > 0 {
						ids = append(ids, j.OrganizationID)
					}
				}
				resolved, orgErr := h.orgSvc.GetOrganizations(ctx, ids)
				if orgErr != nil {
					h.log.WarnContext(ctx, "admin jobs: resolve owning organizations", "error", orgErr)
				} else {
					orgs = resolved
				}
			}

			for _, j := range offers {
				companyName := i18n.T(lang, "admin.jobs_unknown_company")
				companyType := "vendor"
				if o := orgs[j.OrganizationID]; o != nil {
					if o.TradeName["ar"] != "" {
						companyName = o.TradeName["ar"]
					} else {
						companyName = o.LegalName
					}
					companyType = string(o.Type)
				}
				appsCount := 0
				if cnt, err := h.hrSvc.CountApplications(ctx, j.ID); err == nil {
					appsCount = cnt
				}
				jobViews = append(jobViews, &pages.AdminJobView{
					Job:               j,
					CompanyName:       companyName,
					CompanyType:       companyType,
					ApplicationsCount: appsCount,
				})
			}
		}
	}

	var orgOptions []*pages.AdminJobOrgOption
	if h.orgSvc != nil {
		if allOrgs, err := h.orgSvc.ListOrganizations(database.AsSystem(ctx), nil, nil, 1000, 0); err == nil {
			for _, o := range allOrgs {
				if o == nil {
					continue
				}
				name := o.LegalName
				if o.TradeName["ar"] != "" {
					name = o.TradeName["ar"]
				}
				orgOptions = append(orgOptions, &pages.AdminJobOrgOption{
					ID:   o.ID,
					Name: name,
				})
			}
		}
	}

	noticeType := r.URL.Query().Get("notice")
	noticeMsg := r.URL.Query().Get("msg")

	data := pages.AdminJobsData{
		Jobs:           jobViews,
		Organizations:  orgOptions,
		SelectedOrgID:  selectedOrgID,
		SelectedStatus: selectedStatus,
		SearchQuery:    searchQuery,
		Page:           page,
		PerPage:        limit,
		TotalCount:     totalCount,
		NoticeType:     noticeType,
		NoticeMsg:      noticeMsg,
	}

	h.renderPage(ctx, w, "render admin jobs", pages.AdminJobs(lang, dir, data))
}

// AdminJobDetailPage renders details and applicant records for a single job vacancy.
func (h *UIHandler) AdminJobDetailPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	jobID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || jobID <= 0 {
		h.redirectWithNotice(w, r, "/admin/jobs", "error", i18n.T(lang, "admin.jobs.invalid_id"))
		return
	}

	if h.hrSvc == nil {
		h.redirectWithNotice(w, r, "/admin/jobs", "error", i18n.T(lang, "admin.jobs.service_unavailable"))
		return
	}

	job, err := h.hrSvc.GetJobOffer(database.AsSystem(ctx), jobID)
	if err != nil || job == nil {
		h.redirectWithNotice(w, r, "/admin/jobs", "error", i18n.T(lang, "admin.jobs.not_found"))
		return
	}

	companyName := i18n.T(lang, "admin.jobs_unknown_company")
	companyType := "vendor"
	if h.orgSvc != nil && job.OrganizationID > 0 {
		if o, err := h.orgSvc.GetOrganization(database.AsSystem(ctx), job.OrganizationID); err == nil && o != nil {
			if o.TradeName["ar"] != "" {
				companyName = o.TradeName["ar"]
			} else {
				companyName = o.LegalName
			}
			companyType = string(o.Type)
		}
	}

	apps, err := h.hrSvc.ListApplicationsByOffer(database.AsSystem(ctx), jobID, 100, 0)
	if err != nil {
		h.log.WarnContext(ctx, "admin job detail: list applications", "error", err)
	}

	noticeType := r.URL.Query().Get("notice")
	noticeMsg := r.URL.Query().Get("msg")

	data := pages.AdminJobDetailData{
		Job:          job,
		CompanyName:  companyName,
		CompanyType:  companyType,
		Applications: apps,
		NoticeType:   noticeType,
		NoticeMsg:    noticeMsg,
	}

	h.renderPage(ctx, w, "render admin job detail", pages.AdminJobDetail(lang, dir, data))
}

// AdminJobCreateSubmit creates a new vacancy for any chosen organization.
func (h *UIHandler) AdminJobCreateSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, _ := authctx.From(ctx)
	sysCtx := database.AsSystem(database.WithAuditActorID(ctx, actor.UserID))

	if h.hrSvc == nil {
		h.redirectWithNotice(w, r, "/admin/jobs", "error", i18n.T(lang, "admin.jobs.service_unavailable"))
		return
	}

	_ = r.ParseForm()

	orgID, err := strconv.ParseInt(strings.TrimSpace(r.PostFormValue("organization_id")), 10, 64)
	if err != nil || orgID <= 0 {
		h.redirectWithNotice(w, r, "/admin/jobs", "error", i18n.T(lang, "admin.jobs.org_required"))
		return
	}

	titleAr := strings.TrimSpace(r.PostFormValue("title_ar"))
	titleEn := strings.TrimSpace(r.PostFormValue("title_en"))
	if titleAr == "" {
		h.redirectWithNotice(w, r, "/admin/jobs", "error", i18n.T(lang, "job.title_ar_required"))
		return
	}
	if titleEn == "" {
		titleEn = titleAr
	}

	location := strings.TrimSpace(r.PostFormValue("location"))
	if location == "" {
		location = i18n.T(lang, "job.main_branch")
	}

	desc := strings.TrimSpace(r.PostFormValue("description"))
	reqs := strings.TrimSpace(r.PostFormValue("requirements"))

	salMin, _ := money.Parse(strings.TrimSpace(r.PostFormValue("salary_min")))
	salMax, _ := money.Parse(strings.TrimSpace(r.PostFormValue("salary_max")))

	status := strings.TrimSpace(r.PostFormValue("status"))
	if status == "" {
		status = "published"
	}

	j := &hr.JobOffer{
		OrganizationID: orgID,
		Title:          i18n.New(titleAr, titleEn),
		Description:    desc,
		Requirements:   reqs,
		Location:       location,
		SalaryMin:      salMin,
		SalaryMax:      salMax,
		Status:         status,
	}

	if _, err := h.hrSvc.CreateJobOffer(sysCtx, orgID, j); err != nil {
		h.redirectWithNotice(w, r, "/admin/jobs", "error", h.safeMessage(err, lang))
		return
	}

	h.redirectWithNotice(w, r, "/admin/jobs", "success", i18n.T(lang, "admin.jobs.created_success"))
}

// AdminJobUpdateSubmit updates an existing job vacancy.
func (h *UIHandler) AdminJobUpdateSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, _ := authctx.From(ctx)
	sysCtx := database.AsSystem(database.WithAuditActorID(ctx, actor.UserID))

	jobID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || jobID <= 0 {
		h.redirectWithNotice(w, r, "/admin/jobs", "error", i18n.T(lang, "admin.jobs.invalid_id"))
		return
	}

	if h.hrSvc == nil {
		h.redirectWithNotice(w, r, "/admin/jobs", "error", i18n.T(lang, "admin.jobs.service_unavailable"))
		return
	}

	existing, err := h.hrSvc.GetJobOffer(sysCtx, jobID)
	if err != nil || existing == nil {
		h.redirectWithNotice(w, r, "/admin/jobs", "error", i18n.T(lang, "admin.jobs.not_found"))
		return
	}

	_ = r.ParseForm()

	titleAr := strings.TrimSpace(r.PostFormValue("title_ar"))
	titleEn := strings.TrimSpace(r.PostFormValue("title_en"))
	if titleAr == "" {
		h.redirectWithNotice(w, r, fmt.Sprintf("/admin/jobs/%d", jobID), "error", i18n.T(lang, "job.title_ar_required"))
		return
	}
	if titleEn == "" {
		titleEn = titleAr
	}

	location := strings.TrimSpace(r.PostFormValue("location"))
	if location == "" {
		location = existing.Location
	}

	desc := strings.TrimSpace(r.PostFormValue("description"))
	reqs := strings.TrimSpace(r.PostFormValue("requirements"))

	salMin, _ := money.Parse(strings.TrimSpace(r.PostFormValue("salary_min")))
	salMax, _ := money.Parse(strings.TrimSpace(r.PostFormValue("salary_max")))

	status := strings.TrimSpace(r.PostFormValue("status"))
	if status == "" {
		status = existing.Status
	}

	j := &hr.JobOffer{
		ID:             jobID,
		OrganizationID: existing.OrganizationID,
		Title:          i18n.New(titleAr, titleEn),
		Description:    desc,
		Requirements:   reqs,
		Location:       location,
		SalaryMin:      salMin,
		SalaryMax:      salMax,
		Status:         status,
	}

	if err := h.hrSvc.UpdateJobOffer(sysCtx, existing.OrganizationID, j); err != nil {
		h.redirectWithNotice(w, r, fmt.Sprintf("/admin/jobs/%d", jobID), "error", h.safeMessage(err, lang))
		return
	}

	redirectURL := r.Header.Get("Referer")
	if redirectURL == "" {
		redirectURL = fmt.Sprintf("/admin/jobs/%d", jobID)
	}
	h.redirectWithNotice(w, r, redirectURL, "success", i18n.T(lang, "admin.jobs.updated_success"))
}

// AdminJobToggleSubmit switches status between published and closed.
func (h *UIHandler) AdminJobToggleSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, _ := authctx.From(ctx)
	sysCtx := database.AsSystem(database.WithAuditActorID(ctx, actor.UserID))

	jobID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || jobID <= 0 {
		h.redirectWithNotice(w, r, "/admin/jobs", "error", i18n.T(lang, "admin.jobs.invalid_id"))
		return
	}

	if h.hrSvc == nil {
		h.redirectWithNotice(w, r, "/admin/jobs", "error", i18n.T(lang, "admin.jobs.service_unavailable"))
		return
	}

	existing, err := h.hrSvc.GetJobOffer(sysCtx, jobID)
	if err != nil || existing == nil {
		h.redirectWithNotice(w, r, "/admin/jobs", "error", i18n.T(lang, "admin.jobs.not_found"))
		return
	}

	if err := h.hrSvc.ToggleJobOfferStatus(sysCtx, existing.OrganizationID, jobID); err != nil {
		h.redirectWithNotice(w, r, "/admin/jobs", "error", h.safeMessage(err, lang))
		return
	}

	redirectURL := r.Header.Get("Referer")
	if redirectURL == "" {
		redirectURL = "/admin/jobs"
	}
	h.redirectWithNotice(w, r, redirectURL, "success", i18n.T(lang, "admin.jobs.status_toggled_success"))
}

// AdminJobDeleteSubmit removes a job vacancy.
func (h *UIHandler) AdminJobDeleteSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, _ := authctx.From(ctx)
	sysCtx := database.AsSystem(database.WithAuditActorID(ctx, actor.UserID))

	jobID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || jobID <= 0 {
		h.redirectWithNotice(w, r, "/admin/jobs", "error", i18n.T(lang, "admin.jobs.invalid_id"))
		return
	}

	if h.hrSvc == nil {
		h.redirectWithNotice(w, r, "/admin/jobs", "error", i18n.T(lang, "admin.jobs.service_unavailable"))
		return
	}

	existing, err := h.hrSvc.GetJobOffer(sysCtx, jobID)
	if err != nil || existing == nil {
		h.redirectWithNotice(w, r, "/admin/jobs", "error", i18n.T(lang, "admin.jobs.not_found"))
		return
	}

	if err := h.hrSvc.DeleteJobOffer(sysCtx, existing.OrganizationID, jobID); err != nil {
		h.redirectWithNotice(w, r, "/admin/jobs", "error", h.safeMessage(err, lang))
		return
	}

	h.redirectWithNotice(w, r, "/admin/jobs", "success", i18n.T(lang, "admin.jobs.deleted_success"))
}
