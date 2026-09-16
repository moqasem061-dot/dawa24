package ui

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// AdminOrgImportSavingItemUpdateSubmit updates staged item details for an admin importing on behalf of an org.
func (h *UIHandler) AdminOrgImportSavingItemUpdateSubmit(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "id")
	if sessionID == "" {
		sessionID = chi.URLParam(r, "runID")
	}
	itemIndex, _ := strconv.Atoi(chi.URLParam(r, "itemIndex"))

	session, ok := globalSavingImportSessionStore.GetSessionForAdmin(sessionID)
	if !ok {
		h.redirectWithNotice(w, r, "/admin/organizations/import", "error", "جلسة الاستيراد غير موجودة أو انتهت صلاحيتها")
		return
	}

	redirectURI := fmt.Sprintf("/admin/organizations/import/%d/saving/%s/review", session.OrgID, sessionID)
	if r.URL.RawQuery != "" {
		redirectURI += "?" + r.URL.RawQuery
	}

	if err := r.ParseForm(); err == nil {
		name := strings.TrimSpace(r.FormValue("name"))
		var pricePtr *money.Amount
		if pStr := strings.TrimSpace(r.FormValue("price")); pStr != "" {
			if amt, err := money.Parse(pStr); err == nil {
				pricePtr = &amt
			}
		}
		var qtyPtr *float64
		if qStr := strings.TrimSpace(r.FormValue("quantity")); qStr != "" {
			if q, err := strconv.ParseFloat(qStr, 64); err == nil && q > 0 {
				qtyPtr = &q
			}
		}
		if err := globalSavingImportSessionStore.UpdateStagedItem(sessionID, session.OrgID, itemIndex, name, pricePtr, qtyPtr, nil); err != nil {
			h.redirectWithNotice(w, r, redirectURI, "error", h.safeMessage(err, langOf(r)))
			return
		}
	}

	http.Redirect(w, r, redirectURI, http.StatusSeeOther)
}

// AdminOrgImportSavingItemMatchSubmit handles manual link or unlink for an admin importing on behalf of an org.
func (h *UIHandler) AdminOrgImportSavingItemMatchSubmit(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "id")
	if sessionID == "" {
		sessionID = chi.URLParam(r, "runID")
	}
	itemIndex, _ := strconv.Atoi(chi.URLParam(r, "itemIndex"))

	session, ok := globalSavingImportSessionStore.GetSessionForAdmin(sessionID)
	if !ok {
		h.redirectWithNotice(w, r, "/admin/organizations/import", "error", "جلسة الاستيراد غير موجودة أو انتهت صلاحيتها")
		return
	}

	redirectURI := fmt.Sprintf("/admin/organizations/import/%d/saving/%s/review", session.OrgID, sessionID)
	if r.URL.RawQuery != "" {
		redirectURI += "?" + r.URL.RawQuery
	}

	noticeMsg := "تم إلغاء ربط الصنف (أصبح غير مرتبط)."
	if err := r.ParseForm(); err == nil {
		productID, _ := strconv.ParseInt(r.FormValue("product_id"), 10, 64)
		masterName := strings.TrimSpace(r.FormValue("master_name"))
		masterSKU := strings.TrimSpace(r.FormValue("master_sku"))
		if err := globalSavingImportSessionStore.AssignStagedItemMatch(sessionID, session.OrgID, itemIndex, productID, masterName, masterSKU); err != nil {
			h.redirectWithNotice(w, r, redirectURI, "error", h.safeMessage(err, langOf(r)))
			return
		}
		if productID > 0 {
			noticeMsg = fmt.Sprintf("تم ربط الصنف «%s» بالكتالوج المركزي.", masterName)
		}
	}

	h.redirectWithNotice(w, r, redirectURI, "success", noticeMsg)
}

// AdminOrgImportSavingItemToggleSubmit flips included checkbox for an admin importing on behalf of an org.
func (h *UIHandler) AdminOrgImportSavingItemToggleSubmit(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "id")
	if sessionID == "" {
		sessionID = chi.URLParam(r, "runID")
	}
	itemIndex, _ := strconv.Atoi(chi.URLParam(r, "itemIndex"))

	session, ok := globalSavingImportSessionStore.GetSessionForAdmin(sessionID)
	if !ok {
		h.redirectWithNotice(w, r, "/admin/organizations/import", "error", "جلسة الاستيراد غير موجودة أو انتهت صلاحيتها")
		return
	}

	_, _ = globalSavingImportSessionStore.ToggleStagedItem(sessionID, session.OrgID, itemIndex)

	redirectURI := fmt.Sprintf("/admin/organizations/import/%d/saving/%s/review", session.OrgID, sessionID)
	if r.URL.RawQuery != "" {
		redirectURI += "?" + r.URL.RawQuery
	}
	http.Redirect(w, r, redirectURI, http.StatusSeeOther)
}
