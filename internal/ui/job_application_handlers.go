package ui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/hr"
	"github.com/muhiya/dawa24-store/internal/modules/notifications"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

func (h *UIHandler) handleJobApplicationsJSON(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	jobID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || jobID <= 0 {
		http.Error(w, `{"error":"invalid job id"}`, http.StatusBadRequest)
		return
	}

	apps := make([]*hr.JobApplication, 0)
	if h.hrSvc != nil {
		job, err := h.hrSvc.GetJobOffer(ctx, jobID)
		if err != nil || job == nil || (!actor.IsStaff && job.OrganizationID != actor.OrganizationID) {
			http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
			return
		}
		aList, _ := h.hrSvc.ListApplicationsByOffer(ctx, jobID, 100, 0)
		if aList != nil {
			apps = aList
		}
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(apps)
}

func (h *UIHandler) handleJobApplicationAccept(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	appID, err := strconv.ParseInt(chi.URLParam(r, "appId"), 10, 64)
	if err != nil || appID <= 0 {
		http.Error(w, `{"error":"invalid application id"}`, http.StatusBadRequest)
		return
	}

	var req acceptApplicantReq
	if strings.Contains(r.Header.Get("Content-Type"), "application/json") {
		_ = json.NewDecoder(r.Body).Decode(&req)
	} else {
		_ = r.ParseForm()
		req.BranchID, _ = strconv.ParseInt(r.PostFormValue("branch_id"), 10, 64)
		req.RoleKey = r.PostFormValue("role_key")
		req.JobTitle = r.PostFormValue("job_title")
		req.BaseSalary = r.PostFormValue("base_salary")
		req.Notes = r.PostFormValue("notes")
	}

	if req.RoleKey == "" {
		if actor.IsVendor() {
			req.RoleKey = "org_sales_rep"
		} else {
			req.RoleKey = "org_pharmacist"
		}
	}

	var branchPtr *int64
	if req.BranchID > 0 {
		branchPtr = &req.BranchID
	}

	sal, _ := money.Parse(req.BaseSalary)

	if h.hrSvc == nil {
		http.Error(w, `{"error":"hr service unavailable"}`, http.StatusInternalServerError)
		return
	}

	app, err := h.hrSvc.AcceptAndOnboardApplicant(ctx, hr.AcceptApplicantInput{
		ApplicationID:  appID,
		OrganizationID: actor.OrganizationID,
		BranchID:       branchPtr,
		RoleKey:        req.RoleKey,
		JobTitle:       req.JobTitle,
		BaseSalary:     sal,
		Notes:          req.Notes,
	})
	if err != nil {
		h.log.WarnContext(ctx, "failed to accept applicant", "app_id", appID, "error", err)
		http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusBadRequest)
		return
	}

	// Dispatch In-App Notification to the Job Seeker
	if app != nil && app.ApplicantUserID != nil && *app.ApplicantUserID > 0 && h.notifSvc != nil {
		lang := langOf(r)
		orgName := i18n.T(lang, "job.notif.default_org")
		if h.orgSvc != nil {
			if o, err := h.orgSvc.GetOrganization(ctx, actor.OrganizationID); err == nil && o != nil {
				orgName = o.TradeName.Get(i18n.ParseLang(lang))
				if orgName == "" {
					orgName = o.LegalName
				}
			}
		}
		branchName := i18n.T(lang, "job.main_branch")
		if app.BranchName != "" {
			branchName = app.BranchName
		}
		jobTitle := app.JobTitle
		if jobTitle == "" {
			jobTitle = i18n.T(lang, "job.notif.default_job_title")
		}

		_, notifErr := h.notifSvc.Send(ctx, notifications.SendInput{
			UserID:         *app.ApplicantUserID,
			OrganizationID: &actor.OrganizationID,
			Channel:        notifications.ChannelInApp,
			Recipient:      app.ApplicantEmail,
			Title:          i18n.T(lang, "job.notif.accept_title"),
			Body:           fmt.Sprintf(i18n.T(lang, "job.notif.accept_body"), jobTitle, orgName, branchName),
		})
		if notifErr != nil {
			h.log.WarnContext(ctx, "failed to send acceptance notification to seeker", "user_id", *app.ApplicantUserID, "error", notifErr)
		}
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success":     true,
		"application": app,
	})
}

func (h *UIHandler) handleJobApplicationReject(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	appID, err := strconv.ParseInt(chi.URLParam(r, "appId"), 10, 64)
	if err != nil || appID <= 0 {
		http.Error(w, `{"error":"invalid application id"}`, http.StatusBadRequest)
		return
	}

	var req rejectApplicantReq
	if strings.Contains(r.Header.Get("Content-Type"), "application/json") {
		_ = json.NewDecoder(r.Body).Decode(&req)
	} else {
		_ = r.ParseForm()
		req.Notes = r.PostFormValue("notes")
	}

	if h.hrSvc == nil {
		http.Error(w, `{"error":"hr service unavailable"}`, http.StatusInternalServerError)
		return
	}

	app, err := h.hrSvc.RejectApplicant(ctx, actor.OrganizationID, appID, req.Notes)
	if err != nil {
		h.log.WarnContext(ctx, "failed to reject applicant", "app_id", appID, "error", err)
		http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusBadRequest)
		return
	}

	// Dispatch In-App Notification to Job Seeker
	if app != nil && app.ApplicantUserID != nil && *app.ApplicantUserID > 0 && h.notifSvc != nil {
		lang := langOf(r)
		orgName := i18n.T(lang, "job.notif.default_org")
		if h.orgSvc != nil {
			if o, err := h.orgSvc.GetOrganization(ctx, actor.OrganizationID); err == nil && o != nil {
				orgName = o.TradeName.Get(i18n.ParseLang(lang))
				if orgName == "" {
					orgName = o.LegalName
				}
			}
		}
		jobTitle := app.JobTitle
		if jobTitle == "" {
			jobTitle = i18n.T(lang, "job.notif.default_job_title")
		}

		_, _ = h.notifSvc.Send(ctx, notifications.SendInput{
			UserID:         *app.ApplicantUserID,
			OrganizationID: &actor.OrganizationID,
			Channel:        notifications.ChannelInApp,
			Recipient:      app.ApplicantEmail,
			Title:          i18n.T(lang, "job.notif.reject_title"),
			Body:           fmt.Sprintf(i18n.T(lang, "job.notif.reject_body"), jobTitle, orgName),
		})
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success":     true,
		"application": app,
	})
}
