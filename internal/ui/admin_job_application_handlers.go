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
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// AdminJobApplicationsJSON returns applicants for a vacancy as JSON.
func (h *UIHandler) AdminJobApplicationsJSON(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	jobID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || jobID <= 0 {
		http.Error(w, `{"error":"invalid job id"}`, http.StatusBadRequest)
		return
	}

	var apps []*hr.JobApplication
	if h.hrSvc != nil {
		aList, _ := h.hrSvc.ListApplicationsByOffer(database.AsSystem(ctx), jobID, 100, 0)
		apps = aList
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(apps)
}

// AdminJobApplicationAcceptSubmit accepts an applicant for a job vacancy.
func (h *UIHandler) AdminJobApplicationAcceptSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, _ := authctx.From(ctx)
	sysCtx := database.AsSystem(database.WithAuditActorID(ctx, actor.UserID))

	isJSON := strings.Contains(r.Header.Get("Content-Type"), "application/json")

	jobID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || jobID <= 0 {
		if isJSON {
			http.Error(w, `{"error":"invalid job id"}`, http.StatusBadRequest)
		} else {
			h.redirectWithNotice(w, r, "/admin/jobs", "error", i18n.T(lang, "admin.jobs.invalid_id"))
		}
		return
	}

	appID, err := strconv.ParseInt(chi.URLParam(r, "appId"), 10, 64)
	if err != nil || appID <= 0 {
		if isJSON {
			http.Error(w, `{"error":"invalid application id"}`, http.StatusBadRequest)
		} else {
			h.redirectWithNotice(w, r, fmt.Sprintf("/admin/jobs/%d", jobID), "error", i18n.T(lang, "admin.jobs.invalid_id"))
		}
		return
	}

	if h.hrSvc == nil {
		if isJSON {
			http.Error(w, `{"error":"hr service unavailable"}`, http.StatusInternalServerError)
		} else {
			h.redirectWithNotice(w, r, fmt.Sprintf("/admin/jobs/%d", jobID), "error", i18n.T(lang, "admin.jobs.service_unavailable"))
		}
		return
	}

	job, err := h.hrSvc.GetJobOffer(sysCtx, jobID)
	if err != nil || job == nil {
		if isJSON {
			http.Error(w, `{"error":"job not found"}`, http.StatusNotFound)
		} else {
			h.redirectWithNotice(w, r, "/admin/jobs", "error", i18n.T(lang, "admin.jobs.not_found"))
		}
		return
	}

	var req acceptApplicantReq
	if isJSON {
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
		req.RoleKey = "org_employee"
	}
	if req.JobTitle == "" {
		req.JobTitle = job.Title.Get(i18n.ParseLang(lang))
	}

	var branchPtr *int64
	if req.BranchID > 0 {
		branchPtr = &req.BranchID
	}
	sal, _ := money.Parse(req.BaseSalary)

	app, err := h.hrSvc.AcceptAndOnboardApplicant(sysCtx, hr.AcceptApplicantInput{
		ApplicationID:  appID,
		OrganizationID: job.OrganizationID,
		BranchID:       branchPtr,
		RoleKey:        req.RoleKey,
		JobTitle:       req.JobTitle,
		BaseSalary:     sal,
		Notes:          req.Notes,
	})
	if err != nil {
		h.log.WarnContext(ctx, "admin accept applicant failed", "app_id", appID, "error", err)
		if isJSON {
			http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusBadRequest)
		} else {
			h.redirectWithNotice(w, r, fmt.Sprintf("/admin/jobs/%d", jobID), "error", h.safeMessage(err, lang))
		}
		return
	}

	// Dispatch In-App Notification if user exists
	if app != nil && app.ApplicantUserID != nil && *app.ApplicantUserID > 0 && h.notifSvc != nil {
		orgName := i18n.T(lang, "job.notif.default_org")
		if h.orgSvc != nil {
			if o, err := h.orgSvc.GetOrganization(sysCtx, job.OrganizationID); err == nil && o != nil {
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

		_, _ = h.notifSvc.Send(sysCtx, notifications.SendInput{
			UserID:         *app.ApplicantUserID,
			OrganizationID: &job.OrganizationID,
			Channel:        notifications.ChannelInApp,
			Recipient:      app.ApplicantEmail,
			Title:          i18n.T(lang, "job.notif.accept_title"),
			Body:           fmt.Sprintf(i18n.T(lang, "job.notif.accept_body"), jobTitle, orgName, branchName),
		})
	}

	if isJSON {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "application": app})
		return
	}

	h.redirectWithNotice(w, r, fmt.Sprintf("/admin/jobs/%d", jobID), "success", i18n.T(lang, "admin.jobs.app_accepted_success"))
}

// AdminJobApplicationRejectSubmit rejects an application for a job vacancy.
func (h *UIHandler) AdminJobApplicationRejectSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, _ := authctx.From(ctx)
	sysCtx := database.AsSystem(database.WithAuditActorID(ctx, actor.UserID))

	isJSON := strings.Contains(r.Header.Get("Content-Type"), "application/json")

	jobID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || jobID <= 0 {
		if isJSON {
			http.Error(w, `{"error":"invalid job id"}`, http.StatusBadRequest)
		} else {
			h.redirectWithNotice(w, r, "/admin/jobs", "error", i18n.T(lang, "admin.jobs.invalid_id"))
		}
		return
	}

	appID, err := strconv.ParseInt(chi.URLParam(r, "appId"), 10, 64)
	if err != nil || appID <= 0 {
		if isJSON {
			http.Error(w, `{"error":"invalid application id"}`, http.StatusBadRequest)
		} else {
			h.redirectWithNotice(w, r, fmt.Sprintf("/admin/jobs/%d", jobID), "error", i18n.T(lang, "admin.jobs.invalid_id"))
		}
		return
	}

	if h.hrSvc == nil {
		if isJSON {
			http.Error(w, `{"error":"hr service unavailable"}`, http.StatusInternalServerError)
		} else {
			h.redirectWithNotice(w, r, fmt.Sprintf("/admin/jobs/%d", jobID), "error", i18n.T(lang, "admin.jobs.service_unavailable"))
		}
		return
	}

	job, err := h.hrSvc.GetJobOffer(sysCtx, jobID)
	if err != nil || job == nil {
		if isJSON {
			http.Error(w, `{"error":"job not found"}`, http.StatusNotFound)
		} else {
			h.redirectWithNotice(w, r, "/admin/jobs", "error", i18n.T(lang, "admin.jobs.not_found"))
		}
		return
	}

	var req rejectApplicantReq
	if isJSON {
		_ = json.NewDecoder(r.Body).Decode(&req)
	} else {
		_ = r.ParseForm()
		req.Notes = r.PostFormValue("notes")
	}

	app, err := h.hrSvc.RejectApplicant(sysCtx, job.OrganizationID, appID, req.Notes)
	if err != nil {
		h.log.WarnContext(ctx, "admin reject applicant failed", "app_id", appID, "error", err)
		if isJSON {
			http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusBadRequest)
		} else {
			h.redirectWithNotice(w, r, fmt.Sprintf("/admin/jobs/%d", jobID), "error", h.safeMessage(err, lang))
		}
		return
	}

	// Dispatch In-App Notification if user exists
	if app != nil && app.ApplicantUserID != nil && *app.ApplicantUserID > 0 && h.notifSvc != nil {
		orgName := i18n.T(lang, "job.notif.default_org")
		if h.orgSvc != nil {
			if o, err := h.orgSvc.GetOrganization(sysCtx, job.OrganizationID); err == nil && o != nil {
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

		_, _ = h.notifSvc.Send(sysCtx, notifications.SendInput{
			UserID:         *app.ApplicantUserID,
			OrganizationID: &job.OrganizationID,
			Channel:        notifications.ChannelInApp,
			Recipient:      app.ApplicantEmail,
			Title:          i18n.T(lang, "job.notif.reject_title"),
			Body:           fmt.Sprintf(i18n.T(lang, "job.notif.reject_body"), jobTitle, orgName),
		})
	}

	if isJSON {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "application": app})
		return
	}

	h.redirectWithNotice(w, r, fmt.Sprintf("/admin/jobs/%d", jobID), "success", i18n.T(lang, "admin.jobs.app_rejected_success"))
}
