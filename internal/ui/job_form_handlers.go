package ui

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/hr"
	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// ================= Helpers =================

// accessibleBranchesForActor returns the branches of the actor's organization that the user is authorized to manage/select.
func (h *UIHandler) accessibleBranchesForActor(ctx context.Context, actor authctx.Actor) []*org.Branch {
	if h.orgSvc == nil || actor.OrganizationID <= 0 {
		return nil
	}
	allBranches, err := h.orgSvc.ListBranches(ctx, actor.OrganizationID)
	if err != nil || len(allBranches) == 0 {
		return nil
	}

	// Check if the user has permission to manage/view all branches across their organization.
	canManageAll := actor.IsOwner || actor.IsPlatformAdmin() ||
		actor.Can("pharmacy.branch.view") || actor.Can("pharmacy.branch.manage") ||
		actor.Can("pharmacy.branch.update") || actor.Can("vendor.branch.view") ||
		actor.Can("vendor.branch.manage") || actor.Can("vendor.branch.update")

	// If restricted to a specific branch and lacks global branch management permissions, restrict available branches.
	if !canManageAll && actor.BranchID != nil && *actor.BranchID > 0 {
		var filtered []*org.Branch
		for _, b := range allBranches {
			if b != nil && b.ID == *actor.BranchID {
				filtered = append(filtered, b)
			}
		}
		if len(filtered) > 0 {
			return filtered
		}
	}

	return allBranches
}

func (h *UIHandler) handleJobCreateSubmit(w http.ResponseWriter, r *http.Request, redirectURL string) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect="+redirectURL, http.StatusSeeOther)
		return
	}

	if h.hrSvc == nil {
		h.redirectWithNotice(w, r, redirectURL, "error", i18n.T(lang, "job.service_unavailable"))
		return
	}

	_ = r.ParseForm()

	titleAr := strings.TrimSpace(r.PostFormValue("title_ar"))
	titleEn := strings.TrimSpace(r.PostFormValue("title_en"))
	if titleAr == "" {
		h.redirectWithNotice(w, r, redirectURL, "error", i18n.T(lang, "job.title_ar_required"))
		return
	}
	if titleEn == "" {
		titleEn = titleAr
	}

	accessibleBranches := h.accessibleBranchesForActor(ctx, actor)
	branchIDStr := strings.TrimSpace(r.PostFormValue("branch_id"))
	var location string

	if len(accessibleBranches) > 0 {
		if branchIDStr == "" {
			h.redirectWithNotice(w, r, redirectURL, "error", i18n.T(lang, "job.branch_required"))
			return
		}
		branchID, err := strconv.ParseInt(branchIDStr, 10, 64)
		if err != nil || branchID <= 0 {
			h.redirectWithNotice(w, r, redirectURL, "error", i18n.T(lang, "job.branch_id_invalid"))
			return
		}

		var selectedBranch *org.Branch
		for _, b := range accessibleBranches {
			if b != nil && b.ID == branchID {
				selectedBranch = b
				break
			}
		}
		if selectedBranch == nil {
			h.redirectWithNotice(w, r, redirectURL, "error", i18n.T(lang, "job.branch_forbidden"))
			return
		}

		location = selectedBranch.Name.Get(i18n.AR)
		if location == "" {
			location = selectedBranch.Name.Get(i18n.EN)
		}
		if location == "" {
			location = fmt.Sprintf(i18n.T(lang, "job.branch_fallback_prefix"), selectedBranch.ID)
		}
	} else {
		location = strings.TrimSpace(r.PostFormValue("location"))
		if location == "" {
			location = i18n.T(lang, "job.main_branch")
		}
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
		Title:        i18n.New(titleAr, titleEn),
		Description:  desc,
		Requirements: reqs,
		Location:     location,
		SalaryMin:    salMin,
		SalaryMax:    salMax,
		Status:       status,
	}

	if _, err := h.hrSvc.CreateJobOffer(ctx, actor.OrganizationID, j); err != nil {
		h.redirectWithNotice(w, r, redirectURL, "error", h.safeMessage(err, lang))
		return
	}

	h.redirectWithNotice(w, r, redirectURL, "success", i18n.T(lang, "job.published_success"))
}

func (h *UIHandler) handleJobUpdateSubmit(w http.ResponseWriter, r *http.Request, redirectURL string) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect="+redirectURL, http.StatusSeeOther)
		return
	}

	jobID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || jobID <= 0 {
		h.redirectWithNotice(w, r, redirectURL, "error", i18n.T(lang, "job.invalid_id"))
		return
	}

	if h.hrSvc == nil {
		h.redirectWithNotice(w, r, redirectURL, "error", i18n.T(lang, "job.service_unavailable"))
		return
	}

	_ = r.ParseForm()

	titleAr := strings.TrimSpace(r.PostFormValue("title_ar"))
	titleEn := strings.TrimSpace(r.PostFormValue("title_en"))
	if titleAr == "" {
		h.redirectWithNotice(w, r, redirectURL, "error", i18n.T(lang, "job.title_ar_required"))
		return
	}
	if titleEn == "" {
		titleEn = titleAr
	}

	accessibleBranches := h.accessibleBranchesForActor(ctx, actor)
	branchIDStr := strings.TrimSpace(r.PostFormValue("branch_id"))
	var location string

	if len(accessibleBranches) > 0 {
		if branchIDStr == "" {
			h.redirectWithNotice(w, r, redirectURL, "error", i18n.T(lang, "job.branch_required"))
			return
		}
		branchID, err := strconv.ParseInt(branchIDStr, 10, 64)
		if err != nil || branchID <= 0 {
			h.redirectWithNotice(w, r, redirectURL, "error", i18n.T(lang, "job.branch_id_invalid"))
			return
		}

		var selectedBranch *org.Branch
		for _, b := range accessibleBranches {
			if b != nil && b.ID == branchID {
				selectedBranch = b
				break
			}
		}
		if selectedBranch == nil {
			h.redirectWithNotice(w, r, redirectURL, "error", i18n.T(lang, "job.branch_forbidden"))
			return
		}

		location = selectedBranch.Name.Get(i18n.AR)
		if location == "" {
			location = selectedBranch.Name.Get(i18n.EN)
		}
		if location == "" {
			location = fmt.Sprintf(i18n.T(lang, "job.branch_fallback_prefix"), selectedBranch.ID)
		}
	} else {
		location = strings.TrimSpace(r.PostFormValue("location"))
		if location == "" {
			location = i18n.T(lang, "job.main_branch")
		}
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
		ID:           jobID,
		Title:        i18n.New(titleAr, titleEn),
		Description:  desc,
		Requirements: reqs,
		Location:     location,
		SalaryMin:    salMin,
		SalaryMax:    salMax,
		Status:       status,
	}

	if err := h.hrSvc.UpdateJobOffer(ctx, actor.OrganizationID, j); err != nil {
		h.redirectWithNotice(w, r, redirectURL, "error", h.safeMessage(err, lang))
		return
	}

	h.redirectWithNotice(w, r, redirectURL, "success", i18n.T(lang, "job.updated_success"))
}

func (h *UIHandler) handleJobToggleSubmit(w http.ResponseWriter, r *http.Request, redirectURL string) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect="+redirectURL, http.StatusSeeOther)
		return
	}

	jobID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || jobID <= 0 {
		h.redirectWithNotice(w, r, redirectURL, "error", i18n.T(lang, "job.invalid_id"))
		return
	}

	if h.hrSvc != nil {
		if err := h.hrSvc.ToggleJobOfferStatus(ctx, actor.OrganizationID, jobID); err != nil {
			h.redirectWithNotice(w, r, redirectURL, "error", h.safeMessage(err, lang))
			return
		}
	}

	h.redirectWithNotice(w, r, redirectURL, "success", i18n.T(lang, "job.status_updated_success"))
}

func (h *UIHandler) handleJobDeleteSubmit(w http.ResponseWriter, r *http.Request, redirectURL string) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect="+redirectURL, http.StatusSeeOther)
		return
	}

	jobID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || jobID <= 0 {
		h.redirectWithNotice(w, r, redirectURL, "error", i18n.T(lang, "job.invalid_id"))
		return
	}

	if h.hrSvc != nil {
		if err := h.hrSvc.DeleteJobOffer(ctx, actor.OrganizationID, jobID); err != nil {
			h.redirectWithNotice(w, r, redirectURL, "error", h.safeMessage(err, lang))
			return
		}
	}

	h.redirectWithNotice(w, r, redirectURL, "success", i18n.T(lang, "job.deleted_success"))
}
