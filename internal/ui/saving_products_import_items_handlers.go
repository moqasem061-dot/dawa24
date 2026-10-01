package ui

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/moqasem061-dot/dawa24/internal/modules/catalog/matching"
	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
	"github.com/moqasem061-dot/dawa24/internal/shared/i18n"
	"github.com/moqasem061-dot/dawa24/internal/shared/matchflow"
	"github.com/moqasem061-dot/dawa24/internal/shared/money"
	"github.com/moqasem061-dot/dawa24/internal/shared/productmatch"
	"github.com/moqasem061-dot/dawa24/internal/ui/pages"
)

// handleSavingProductsImportItemUpdateSubmit updates staged item details for customer or vendor.
func (h *UIHandler) handleSavingProductsImportItemUpdateSubmit(w http.ResponseWriter, r *http.Request, audience string) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}

	sessionID := chi.URLParam(r, "id")
	itemIndex, _ := strconv.Atoi(chi.URLParam(r, "itemIndex"))

	if err := r.ParseForm(); err != nil {
		h.redirectWithNotice(w, r, fmt.Sprintf("/%s/saving-products/import/%s?%s", audience, sessionID, r.URL.RawQuery), "error", i18n.T(langOf(r), "common.invalid_form_data"))
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	var pricePtr *money.Amount
	if pStr := strings.TrimSpace(r.FormValue("price")); pStr != "" {
		if amt, err := money.Parse(pStr); err == nil { // nolint:errswallow: optional input; an unparseable value leaves the default
			pricePtr = &amt
		}
	}
	var qtyPtr *float64
	if qStr := strings.TrimSpace(r.FormValue("quantity")); qStr != "" {
		if q, err := strconv.ParseFloat(qStr, 64); err == nil && q > 0 {
			qtyPtr = &q
		}
	}
	if err := globalSavingImportSessionStore.UpdateStagedItem(sessionID, actor.OrganizationID, itemIndex, name, pricePtr, qtyPtr, nil); err != nil {
		// Silently redirecting on a failed edit is how a row that would not
		// save looked exactly like a row that did.
		h.redirectWithNotice(w, r,
			fmt.Sprintf("/%s/saving-products/import/%s?%s", audience, sessionID, r.URL.RawQuery),
			"error", h.safeMessage(err, langOf(r)))
		return
	}

	redirectURI := fmt.Sprintf("/%s/saving-products/import/%s?%s", audience, sessionID, r.URL.RawQuery)
	http.Redirect(w, r, redirectURI, http.StatusSeeOther)
}

// handleSavingProductsImportItemMatchSubmit handles manual link or unlink for customer or vendor.
func (h *UIHandler) handleSavingProductsImportItemMatchSubmit(w http.ResponseWriter, r *http.Request, audience string) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}

	sessionID := chi.URLParam(r, "id")
	itemIndex, _ := strconv.Atoi(chi.URLParam(r, "itemIndex"))

	if err := r.ParseForm(); err != nil {
		h.redirectWithNotice(w, r, fmt.Sprintf("/%s/saving-products/import/%s?%s", audience, sessionID, r.URL.RawQuery), "error", i18n.T(langOf(r), "common.invalid_form_data"))
		return
	}

	var sess *SavingImportSession
	if audience == "admin" {
		sess, _ = globalSavingImportSessionStore.GetSessionForAdmin(sessionID)
	} else {
		sess, _ = globalSavingImportSessionStore.GetSession(sessionID, actor.OrganizationID)
	}

	var item *StagedSavingItem
	if sess != nil {
		for _, it := range sess.Items {
			if it.Index == itemIndex {
				item = it
				break
			}
		}
	}

	productID, _ := strconv.ParseInt(r.FormValue("product_id"), 10, 64)
	masterName := strings.TrimSpace(r.FormValue("master_name"))
	masterSKU := strings.TrimSpace(r.FormValue("master_sku"))

	curDec := matchflow.RowDecision{}
	if item != nil && item.MatchDecision != nil {
		curDec = *item.MatchDecision
	} else if item != nil {
		curDec.Revision = item.MatchRevision
		curDec.ProductID = item.ProductID
	}

	rowRef := productmatch.Row{Number: itemIndex + 1}
	if item != nil {
		rowRef.Name = item.NameProduct
		rowRef.SKU = item.SKU
	}

	action := parseReviewAction(r, rowRef, curDec)
	scope := productmatch.Scope{
		EffectiveOrg: actor.OrganizationID,
		Workflow:     productmatch.WorkflowSavings,
		EntryPoint:   "savings_review",
	}

	var matchSvc matching.Service
	if h.catSvc != nil {
		matchSvc = h.catSvc.Matching()
	}

	newDec, rErr := executeReviewAction(ctx, matchSvc, scope, action)
	if errors.Is(rErr, matching.ErrRevisionConflict) {
		// T098: Revision conflict notice
		if item != nil {
			item.ConflictNotice = RevisionConflictNoticeBilingual
		}
		if h.isHTMX(r) && item != nil && sess != nil {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			view := pages.SavingImportView{Session: sess, ImportURL: fmt.Sprintf("/%s/saving-products/import", audience), BaseURL: h.baseURL}
			_ = pages.SavingProductRow(view, item).Render(renderContext(ctx), w)
			return
		}
		h.redirectWithNotice(w, r,
			fmt.Sprintf("/%s/saving-products/import/%s?%s", audience, sessionID, r.URL.RawQuery),
			"error", RevisionConflictNoticeBilingual)
		return
	}

	if item != nil {
		item.ConflictNotice = ""
		item.MatchRevision = newDec.Revision
		item.MatchDecision = &newDec
		if newDec.ProductID != nil && *newDec.ProductID > 0 {
			productID = *newDec.ProductID
			if masterName == "" && h.catSvc != nil {
				if p, _, pErr := h.catSvc.GetProduct(ctx, productID); pErr == nil && p != nil {
					masterName = p.Name.Get(i18n.AR)
					masterSKU = p.SKU
				}
			}
		} else if action.Action == "reject" || action.Action == "undo" {
			productID = 0
			masterName = ""
			masterSKU = ""
		}
	}

	if err := globalSavingImportSessionStore.AssignStagedItemMatch(sessionID, actor.OrganizationID, itemIndex, productID, masterName, masterSKU); err != nil {
		h.redirectWithNotice(w, r,
			fmt.Sprintf("/%s/saving-products/import/%s?%s", audience, sessionID, r.URL.RawQuery),
			"error", h.safeMessage(err, langOf(r)))
		return
	}

	if h.isHTMX(r) && item != nil && sess != nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		view := pages.SavingImportView{Session: sess, ImportURL: fmt.Sprintf("/%s/saving-products/import", audience), BaseURL: h.baseURL}
		_ = pages.SavingProductRow(view, item).Render(renderContext(ctx), w)
		return
	}

	noticeMsg := "تم إلغاء ربط الصنف (أصبح غير مرتبط)."
	if productID > 0 {
		noticeMsg = fmt.Sprintf("تم ربط الصنف «%s» بالكتالوج المركزي.", masterName)
	}

	redirectURI := fmt.Sprintf("/%s/saving-products/import/%s?%s", audience, sessionID, r.URL.RawQuery)
	h.redirectWithNotice(w, r, redirectURI, "success", noticeMsg)
}

// handleSavingProductsImportItemToggleSubmit flips included checkbox for customer or vendor.
func (h *UIHandler) handleSavingProductsImportItemToggleSubmit(w http.ResponseWriter, r *http.Request, audience string) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}

	sessionID := chi.URLParam(r, "id")
	itemIndex, _ := strconv.Atoi(chi.URLParam(r, "itemIndex"))

	_, _ = globalSavingImportSessionStore.ToggleStagedItem(sessionID, actor.OrganizationID, itemIndex)

	if h.isHTMX(r) {
		var sess *SavingImportSession
		if audience == "admin" {
			sess, _ = globalSavingImportSessionStore.GetSessionForAdmin(sessionID)
		} else {
			sess, _ = globalSavingImportSessionStore.GetSession(sessionID, actor.OrganizationID)
		}
		if sess != nil {
			for _, it := range sess.Items {
				if it.Index == itemIndex {
					w.Header().Set("Content-Type", "text/html; charset=utf-8")
					view := pages.SavingImportView{Session: sess, ImportURL: fmt.Sprintf("/%s/saving-products/import", audience), BaseURL: h.baseURL}
					_ = pages.SavingProductRow(view, it).Render(renderContext(ctx), w)
					return
				}
			}
		}
	}

	redirectURI := fmt.Sprintf("/%s/saving-products/import/%s?%s", audience, sessionID, r.URL.RawQuery)
	http.Redirect(w, r, redirectURI, http.StatusSeeOther)
}

// handleSavingProductsImportCommitSubmit commits staged items into catalog.saving_products for customer or vendor.
func (h *UIHandler) handleSavingProductsImportCommitSubmit(w http.ResponseWriter, r *http.Request, audience string) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}

	sessionID := chi.URLParam(r, "id")

	// commitSavingImportRun, not the store directly.
	//
	// Two things the store alone does not do: it cannot commit a run whose
	// in-memory session expired or was lost to a restart — the staged rows are
	// in platform.import_run_rows and only this path reads them — and it does
	// not move the durable run to `committed`, so a committed import went on
	// reporting itself as awaiting review in the history list. The JSON commit
	// endpoint has always used this; the form post had drifted.
	_, _, err := h.commitSavingImportRun(ctx, sessionID, actor.OrganizationID, actor.UserID, h.catSvc)
	if err != nil {
		h.redirectWithNotice(w, r, fmt.Sprintf("/%s/saving-products/import/%s", audience, sessionID), "error", i18n.T(langOf(r), "saving.import.save_failed_prefix")+h.safeMessage(err, langOf(r)))
		return
	}

	// The phase and the counters are set inside CommitSession, under the store's
	// lock. Writing them again from here raced every reader of the session —
	// the progress endpoint and the history list both walk it from other
	// goroutines.

	http.Redirect(w, r, fmt.Sprintf("/%s/saving-products/import/%s", audience, sessionID), http.StatusSeeOther)
}

// handleSavingProductsImportCancelSubmit cancels and cleans up a session for customer or vendor.
func (h *UIHandler) handleSavingProductsImportCancelSubmit(w http.ResponseWriter, r *http.Request, audience string) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}

	sessionID := chi.URLParam(r, "id")
	globalSavingImportSessionStore.CancelSession(sessionID, actor.OrganizationID)

	h.redirectWithNotice(w, r, fmt.Sprintf("/%s/saving-products/import", audience), "info", i18n.T(langOf(r), "saving.import.cancelled_success"))
}
