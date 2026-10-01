package ui

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/moqasem061-dot/dawa24/internal/shared/i18n"
)

// AdminOrgImportSavingCancelSubmit cancels an import session.
func (h *UIHandler) AdminOrgImportSavingCancelSubmit(w http.ResponseWriter, r *http.Request) {
	orgID, _ := strconv.ParseInt(chi.URLParam(r, "orgID"), 10, 64)
	runID := chi.URLParam(r, "runID")
	if runID == "" {
		runID = chi.URLParam(r, "id")
	}
	if session, ok := globalSavingImportSessionStore.GetSessionForAdmin(runID); ok {
		targetOrgID := orgID
		if targetOrgID <= 0 {
			targetOrgID = session.OrgID
		}
		if session.OrgID == targetOrgID {
			globalSavingImportSessionStore.CancelSession(runID, targetOrgID)
		}
	}
	h.redirectWithNotice(w, r, "/admin/organizations/import", "info", i18n.T(langOf(r), "saving.import.cancelled_success"))
}

// AdminOrgImportSavingSessionPage provides backwards-compatible redirect to persistent step URLs.
func (h *UIHandler) AdminOrgImportSavingSessionPage(w http.ResponseWriter, r *http.Request) {
	orgID, _ := strconv.ParseInt(chi.URLParam(r, "orgID"), 10, 64)
	sessionID := chi.URLParam(r, "id")
	session, ok := globalSavingImportSessionStore.GetSessionForAdmin(sessionID)
	if !ok {
		h.redirectWithNotice(w, r, "/admin/organizations/import", "error", i18n.T(langOf(r), "customer.saving.import.session_not_found"))
		return
	}
	if orgID <= 0 {
		orgID = session.OrgID
	}
	if session.OrgID != orgID {
		h.redirectWithNotice(w, r, "/admin/organizations/import", "error", i18n.T(langOf(r), "customer.saving.import.session_not_found"))
		return
	}

	if session.Phase == SavingPhaseReview {
		http.Redirect(w, r, fmt.Sprintf("/admin/organizations/import/%d/runs/%s/review", orgID, sessionID), http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/admin/organizations/import/%d/runs/%s/mapping", orgID, sessionID), http.StatusSeeOther)
}
