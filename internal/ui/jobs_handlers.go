package ui

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/hr"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/platform/features"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/pagination"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// JobsPage renders the job board (public or authenticated in customer/vendor dashboard).
func (h *UIHandler) JobsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !features.Enabled(ctx, "jobs.enabled") {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	lang, dir := h.localeAndDir(r)
	actor, isLoggedIn := authctx.From(ctx)

	page := pagination.PageNumber(r)
	limit := pagination.RowsPerPage(r)
	offset := (page - 1) * limit

	var rawJobs []*hr.JobOffer
	var totalCount int
	if h.hrSvc != nil {
		rawJobs, totalCount, _ = h.hrSvc.ListPublishedJobsWithTotal(ctx, limit, offset)
	}

	// Enrich with organization names.
	orgNames := make(map[int64]string)
	if h.orgSvc != nil && len(rawJobs) > 0 {
		ids := make([]int64, 0, len(rawJobs))
		for _, j := range rawJobs {
			if j.OrganizationID > 0 {
				ids = append(ids, j.OrganizationID)
			}
		}
		orgs, err := h.orgSvc.GetOrganizations(database.AsSystem(ctx), ids)
		if err != nil {
			h.log.WarnContext(ctx, "jobs: resolve organization names", "error", err)
		}
		for id, o := range orgs {
			if o == nil {
				continue
			}
			cName := o.TradeName.Get(i18n.ParseLang(lang))
			if cName == "" {
				cName = o.LegalName
			}
			if cName != "" {
				orgNames[id] = cName
			}
		}
	}

	var jobItems []*pages.JobItemView
	for _, j := range rawJobs {
		compName := orgNames[j.OrganizationID]
		if compName == "" {
			compName = i18n.T(lang, "jobs.verified_pharma_entity")
		}
		jobItems = append(jobItems, &pages.JobItemView{
			Job:         j,
			CompanyName: compName,
			IsVerified:  true,
		})
	}

	// The post job button shows for customers (pharmacies), vendors (suppliers), and staff
	postJobURL := "/customer/jobs"
	if isLoggedIn {
		if actor.IsVendor() {
			postJobURL = "/vendor/jobs"
		} else if actor.IsStaff {
			postJobURL = "/admin/jobs"
		}
	}
	canPost := isLoggedIn && (actor.IsCustomer() || actor.IsVendor() || actor.IsStaff)

	data := pages.JobsPageData{
		Jobs:        jobItems,
		TotalCount:  totalCount,
		Page:        page,
		PerPage:     limit,
		Cities:      h.listCities(ctx),
		Actor:       actor,
		IsLoggedIn:  isLoggedIn,
		IsCustomer:  isLoggedIn && actor.IsCustomer(),
		IsVendor:    isLoggedIn && actor.IsVendor(),
		IsAdmin:     isLoggedIn && actor.IsStaff,
		IsJobSeeker: isLoggedIn && actor.IsJobSeeker(),
		CanPostJob:  canPost,
		PostJobURL:  postJobURL,
	}

	h.renderPage(ctx, w, "render jobs page", pages.JobsPage(data, lang, dir))
}

// JobDetailPage renders one vacancy with an apply form (adapted to dashboard shell).
func (h *UIHandler) JobDetailPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || h.hrSvc == nil {
		h.renderError(w, r, err)
		return
	}

	j, err := h.hrSvc.GetJobOffer(ctx, id)
	if err != nil {
		h.renderError(w, r, err)
		return
	}

	compName := i18n.T(lang, "jobs.verified_pharma_entity")
	if h.orgSvc != nil && j.OrganizationID > 0 {
		if o, err := h.orgSvc.GetOrganization(ctx, j.OrganizationID); err == nil && o != nil {
			compName = o.TradeName.Get(i18n.ParseLang(lang))
			if compName == "" {
				compName = o.LegalName
			}
		}
	}

	actor, isLoggedIn := authctx.From(ctx)
	userEmail, userName, userPhone := "", "", ""
	if isLoggedIn && h.idSvc != nil {
		if u, err := h.idSvc.GetUserByID(ctx, actor.UserID); err == nil && u != nil {
			userEmail = u.Email
			userName = u.Name.Get(i18n.Lang(lang))
			userPhone = u.Phone
		}
	}

	data := pages.JobDetailData{
		Job:         j,
		CompanyName: compName,
		Actor:       actor,
		IsLoggedIn:  isLoggedIn,
		IsCustomer:  isLoggedIn && actor.IsCustomer(),
		IsVendor:    isLoggedIn && actor.IsVendor(),
		IsAdmin:     isLoggedIn && actor.IsStaff,
		IsJobSeeker: isLoggedIn && actor.IsJobSeeker(),
		Submitted:   false,
		UserEmail:   userEmail,
		UserName:    userName,
		UserPhone:   userPhone,
	}

	h.renderPage(ctx, w, "render job detail", pages.JobDetail(data, lang, dir))
}

// JobApplySubmit records an application (requires logged-in job seeker).
func (h *UIHandler) JobApplySubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	offerID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, fmt.Sprintf("/auth/login?redirect=/jobs/%d", offerID), http.StatusSeeOther)
		return
	}

	if !actor.IsJobSeeker() && !actor.IsStaff {
		h.redirectWithNotice(w, r, "/jobs/"+strconv.FormatInt(offerID, 10), "error", i18n.T(lang, "jobs.apply_jobseeker_only"))
		return
	}

	if h.hrSvc == nil {
		h.redirectWithNotice(w, r, "/jobs/"+strconv.FormatInt(offerID, 10), "error", i18n.T(lang, "common.service_unavailable"))
		return
	}

	offer, err := h.hrSvc.GetJobOffer(ctx, offerID)
	if err != nil {
		h.renderError(w, r, err)
		return
	}

	compName := i18n.T(lang, "jobs.verified_pharma_entity")
	if h.orgSvc != nil && offer.OrganizationID > 0 {
		if o, err := h.orgSvc.GetOrganization(ctx, offer.OrganizationID); err == nil && o != nil {
			compName = o.TradeName.Get(i18n.ParseLang(lang))
			if compName == "" {
				compName = o.LegalName
			}
		}
	}

	// Process attached CV
	cvURL, _ := saveUploadedFile(r, "resume_file", "resumes")

	app := &hr.JobApplication{
		JobOfferID:      offerID,
		OrganizationID:  offer.OrganizationID,
		ApplicantUserID: &actor.UserID,
		ApplicantName:   strings.TrimSpace(r.PostFormValue("applicant_name")),
		ApplicantEmail:  strings.TrimSpace(r.PostFormValue("applicant_email")),
		ApplicantPhone:  strings.TrimSpace(r.PostFormValue("applicant_phone")),
		CVStorageKey:    cvURL,
		Notes:           strings.TrimSpace(r.PostFormValue("notes")),
		Status:          "pending",
	}
	if err := h.hrSvc.ApplyToJob(ctx, app); err != nil {
		h.redirectWithNotice(w, r, "/jobs/"+strconv.FormatInt(offerID, 10), "error", h.safeMessage(err, langOf(r)))
		return
	}

	h.notifyJobApplicationReceived(ctx, offer.OrganizationID, offer.Title.Get(i18n.ParseLang(lang)), app.ApplicantName, app.ApplicantPhone)

	// Re-render with the success state so the applicant stays on the page.
	data := pages.JobDetailData{
		Job:         offer,
		CompanyName: compName,
		Actor:       actor,
		IsLoggedIn:  true,
		IsCustomer:  actor.IsCustomer(),
		IsVendor:    actor.IsVendor(),
		IsAdmin:     actor.IsStaff,
		Submitted:   true,
		UserEmail:   app.ApplicantEmail,
		UserName:    app.ApplicantName,
		UserPhone:   app.ApplicantPhone,
	}

	h.renderPage(ctx, w, "render job detail after apply", pages.JobDetail(data, lang, dir))
}
