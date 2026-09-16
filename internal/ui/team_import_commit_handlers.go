package ui

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/identity"
	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

func (h *UIHandler) handleTeamImportMapSubmit(w http.ResponseWriter, r *http.Request, audience string) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}

	baseImportURL := fmt.Sprintf("/%s/team/import", audience)
	sessionID := chi.URLParam(r, "id")
	session, ok := globalTeamImportSessionStore.GetSession(sessionID, actor.OrganizationID)
	if !ok {
		h.redirectWithNotice(w, r, baseImportURL, "error", i18n.T(langOf(r), "customer.saving.import.session_not_found"))
		return
	}

	if err := r.ParseForm(); err != nil {
		h.redirectWithNotice(w, r, fmt.Sprintf("/%s/team/import/%s", audience, sessionID), "error", i18n.T(langOf(r), "customer.team.import.data_invalid"))
		return
	}

	parseInt := func(name string, defVal int) int {
		if v, err := strconv.Atoi(r.PostFormValue(name)); err == nil {
			return v
		}
		return defVal
	}

	session.DetectedCols = pages.TeamDetectedCols{
		NameCol:     parseInt("col_name", -1),
		EmailCol:    parseInt("col_email", -1),
		PhoneCol:    parseInt("col_phone", -1),
		RoleCol:     parseInt("col_role", -1),
		JobTitleCol: parseInt("col_job_title", -1),
		BranchCol:   parseInt("col_branch", -1),
		CodeCol:     parseInt("col_code", -1),
	}

	if session.DetectedCols.NameCol == -1 || session.DetectedCols.EmailCol == -1 {
		h.redirectWithNotice(w, r, fmt.Sprintf("/%s/team/import/%s", audience, sessionID), "error", i18n.T(langOf(r), "customer.team.import.columns_required"))
		return
	}

	if defRoleID, err := strconv.ParseInt(r.PostFormValue("default_role_id"), 10, 64); err == nil && defRoleID > 0 {
		session.DefaultRoleID = defRoleID
	}

	roleMap := make(map[string]int64)
	for _, er := range session.ExcelRoles {
		formKey := "role_map_" + er.RawName
		if val := r.PostFormValue(formKey); val != "" {
			if rID, err := strconv.ParseInt(val, 10, 64); err == nil && rID > 0 {
				roleMap[er.RawName] = rID
				er.MatchedRoleID = rID
			}
		}
	}

	parsedRows := ParseAndValidateTeamRows(
		session.RawDataRows,
		session.DetectedCols,
		roleMap,
		session.DefaultRoleID,
		session.CompanyRoles,
		session.Branches,
		langOf(r),
	)

	// Check existing user accounts
	if h.idSvc != nil {
		sysCtx := database.AsSystem(ctx)
		for _, row := range parsedRows {
			if row.IsValid && row.Email != "" {
				if existingUser, err := h.idSvc.GetUserByEmail(sysCtx, row.Email); err == nil && existingUser != nil {
					row.IsExistingUser = true
				}
			}
		}
	}

	session.Rows = parsedRows
	session.Phase = pages.TeamPhaseReview
	session.UpdatedAt = time.Now()

	http.Redirect(w, r, fmt.Sprintf("/%s/team/import/%s", audience, sessionID), http.StatusSeeOther)
}

func (h *UIHandler) handleTeamImportCommitSubmit(w http.ResponseWriter, r *http.Request, audience, defaultRoleKey string) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}

	baseImportURL := fmt.Sprintf("/%s/team/import", audience)
	sessionID := chi.URLParam(r, "id")
	session, ok := globalTeamImportSessionStore.GetSession(sessionID, actor.OrganizationID)
	if !ok || session.Phase != pages.TeamPhaseReview {
		h.redirectWithNotice(w, r, baseImportURL, "error", i18n.T(langOf(r), "customer.team.import.session_invalid"))
		return
	}

	if h.idSvc == nil || h.orgSvc == nil {
		h.redirectWithNotice(w, r, fmt.Sprintf("/%s/team/import/%s", audience, sessionID), "error", i18n.T(langOf(r), "customer.team.import.system_unavailable"))
		return
	}

	sysCtx := database.AsSystem(ctx)
	importedCount := 0
	skippedCount := 0
	failedCount := 0

	for _, row := range session.Rows {
		if !row.IsValid {
			skippedCount++
			row.ImportStatus = "skipped"
			continue
		}

		// 1. Check or register user
		var targetUserID int64
		existingUser, err := h.idSvc.GetUserByEmail(sysCtx, row.Email)
		if err == nil && existingUser != nil {
			targetUserID = existingUser.ID
		} else {
			randBytes := make([]byte, 6)
			_, _ = rand.Read(randBytes)
			defaultPass := fmt.Sprintf("Dawa24!%s", hex.EncodeToString(randBytes))

			newUser, _, regErr := h.idSvc.Register(sysCtx, identity.RegisterInput{
				Email:    row.Email,
				Password: defaultPass,
				NameAr:   row.RawName,
				NameEn:   row.RawName,
				Phone:    row.Phone,
				Role:     "user",
			})
			if regErr != nil {
				h.log.ErrorContext(ctx, "failed to register imported employee user", "email", row.Email, "error", regErr)
				failedCount++
				row.ImportStatus = "failed"
				continue
			}
			targetUserID = newUser.ID
		}

		// 2. Add member to organization
		roleKey := row.AssignedRoleKey
		if roleKey == "" {
			roleKey = defaultRoleKey
		}

		member := &org.Member{
			OrganizationID: actor.OrganizationID,
			UserID:         targetUserID,
			BranchID:       row.AssignedBranchID,
			RoleID:         row.AssignedRoleID,
			RoleKey:        roleKey,
			JobTitle:       row.JobTitle,
			EmployeeCode:   row.EmployeeCode,
			IsActive:       true,
		}
		if row.AssignedRoleID > 0 {
			member.OrgRoleID = &row.AssignedRoleID
		}

		if err := h.orgSvc.AddMemberDirect(sysCtx, member); err != nil {
			h.log.ErrorContext(ctx, "failed to link imported employee to org", "org_id", actor.OrganizationID, "user_id", targetUserID, "error", err)
			failedCount++
			row.ImportStatus = "failed"
			continue
		}

		importedCount++
		row.ImportStatus = "imported"
	}

	session.ImportedCount = importedCount
	session.SkippedCount = skippedCount
	session.FailedCount = failedCount
	session.Phase = pages.TeamPhaseCompleted

	http.Redirect(w, r, fmt.Sprintf("/%s/team/import/%s", audience, sessionID), http.StatusSeeOther)
}

func (h *UIHandler) handleTeamImportCancelSubmit(w http.ResponseWriter, r *http.Request, audience string) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}

	sessionID := chi.URLParam(r, "id")
	globalTeamImportSessionStore.DeleteSession(sessionID, actor.OrganizationID)
	h.redirectWithNotice(w, r, fmt.Sprintf("/%s/team/import", audience), "info", i18n.T(langOf(r), "customer.team.import.cancelled"))
}
