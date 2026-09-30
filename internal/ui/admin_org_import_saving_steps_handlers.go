package ui

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/moqasem061-dot/dawa24/internal/modules/catalog"
	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
	"github.com/moqasem061-dot/dawa24/internal/shared/i18n"
	"github.com/moqasem061-dot/dawa24/internal/shared/money"
	"github.com/moqasem061-dot/dawa24/internal/shared/pagination"
	"github.com/moqasem061-dot/dawa24/internal/shared/productmatch"
	"github.com/moqasem061-dot/dawa24/internal/ui/pages"
)

// AdminOrgImportSavingMappingPage renders the persistent column mapping wizard step.
func (h *UIHandler) AdminOrgImportSavingMappingPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	orgID, _ := strconv.ParseInt(chi.URLParam(r, "orgID"), 10, 64)
	runID := chi.URLParam(r, "runID")
	if runID == "" {
		runID = chi.URLParam(r, "id")
	}

	session, ok := globalSavingImportSessionStore.GetSessionForAdmin(runID)
	if !ok {
		h.redirectWithNotice(w, r, "/admin/organizations/import", "error", i18n.T(lang, "customer.saving.import.session_not_found"))
		return
	}
	if orgID <= 0 {
		orgID = session.OrgID
	}

	if session.Phase == SavingPhaseReview {
		http.Redirect(w, r, fmt.Sprintf("/admin/organizations/import/%d/saving/%s/review", orgID, runID), http.StatusSeeOther)
		return
	}

	sysCtx := database.AsSystem(ctx)
	targetOrgName, targetOrgType := h.resolveTargetOrgInfo(sysCtx, orgID)

	view := pages.SavingImportView{
		AIAvailable:         h.matchEnhancer != nil,
		AIUnavailableReason: savingAIUnavailableReason(h.matchEnhancer, lang),
		Audience:            "admin",
		TargetOrgName:       targetOrgName,
		TargetOrgType:       targetOrgType,
		BaseURL:             "/admin/organizations/import",
		ImportURL:           fmt.Sprintf("/admin/organizations/import/%d/saving", orgID),
		Session:             session,
	}

	h.renderPage(ctx, w, "render admin org saving mapping step", pages.SavingImportPage(view, lang, dir))
}

// AdminOrgImportSavingMappingSubmit processes column mapping and performs matching for the session.
func (h *UIHandler) AdminOrgImportSavingMappingSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, _ := authctx.From(ctx)

	orgID, _ := strconv.ParseInt(chi.URLParam(r, "orgID"), 10, 64)
	runID := chi.URLParam(r, "runID")
	isRunsPattern := runID != ""
	if runID == "" {
		runID = chi.URLParam(r, "id")
	}

	session, ok := globalSavingImportSessionStore.GetSessionForAdmin(runID)
	if !ok {
		h.redirectWithNotice(w, r, "/admin/organizations/import", "error", i18n.T(lang, "saving.import.session_not_found"))
		return
	}
	if orgID <= 0 {
		orgID = session.OrgID
	}

	mappingURL := fmt.Sprintf("/admin/organizations/import/%d/saving/%s/mapping", orgID, runID)
	if isRunsPattern {
		mappingURL = fmt.Sprintf("/admin/organizations/import/%d/runs/%s/mapping", orgID, runID)
	}

	if err := r.ParseForm(); err != nil {
		h.redirectWithNotice(w, r, mappingURL, "error", i18n.T(lang, "validation.invalid_data"))
		return
	}

	colName := strings.TrimSpace(r.FormValue("col_name"))
	colSKU := strings.TrimSpace(r.FormValue("col_sku"))
	colQty := strings.TrimSpace(r.FormValue("col_qty"))
	colPrice := strings.TrimSpace(r.FormValue("col_price"))
	matchChoice := ParseMatchChoice(r)
	useAI := ParseUseAI(r)

	nCol, sCol, qCol, pCol := -1, -1, -1, -1
	for idx, hName := range session.Headers {
		if hName == colName {
			nCol = idx
		}
		if hName == colSKU {
			sCol = idx
		}
		if hName == colQty {
			qCol = idx
		}
		if hName == colPrice {
			pCol = idx
		}
	}

	if nCol == -1 {
		h.redirectWithNotice(w, r, mappingURL, "error", i18n.T(lang, "customer.saving.import.missing_name_col"))
		return
	}

	matchRows := make([]catalog.SavingMatchRow, 0, len(session.RawDataRows))
	for _, row := range session.RawDataRows {
		if IsJunkOrEmptyRow(row) {
			continue
		}

		var name, sku string
		if nCol >= 0 && nCol < len(row) {
			name = strings.TrimSpace(row[nCol])
		}
		if sCol >= 0 && sCol < len(row) {
			sku = strings.TrimSpace(row[sCol])
		}

		if isAllDigitsOrCode(name) && len(name) >= 4 && isDescriptiveArabicText(sku) {
			name, sku = sku, name
		}
		if name == "" && sku != "" {
			name = sku
		}
		if !IsValidSavingRow(name, sku) {
			continue
		}

		var qty = 1.0
		if qCol >= 0 && qCol < len(row) {
			if parsedQ, ok := ParseFlexibleQuantity(row[qCol]); ok && parsedQ > 0 {
				qty = parsedQ
			}
		}

		var price money.Amount
		if pCol >= 0 && pCol < len(row) {
			price, _ = ParseFlexibleMoney(row[pCol])
		}

		matchRows = append(matchRows, catalog.SavingMatchRow{
			Index:    len(matchRows) + 1,
			Name:     name,
			SKU:      sku,
			Quantity: qty,
			Price:    price,
		})
	}

	scope := productmatch.Scope{
		EffectiveOrg:    session.OrgID,
		ActorUserID:     actor.UserID,
		SourceNamespace: fmt.Sprintf("org:%d", session.OrgID),
		Workflow:        productmatch.WorkflowSavings,
		EntryPoint:      "savings_matching",
	}

	stagedItems := make([]*StagedSavingItem, 0, len(matchRows))
	matchedCount := 0
	unlinkedCount := 0
	var totalQty float64
	var totalValMinor int64
	var matchEngine *catalog.SavingProductMatchEngine

	if h.catSvc != nil {
		matchItems, engine, mErr := h.catSvc.MatchSavingsRows(database.AsSystem(ctx), scope, matchChoice, matchRows)
		if mErr == nil {
			matchEngine = engine
			for i, mi := range matchItems {
				mr := matchRows[i]
				rowTotal, _ := mr.Price.MulQuantity(mr.Quantity)
				rowTotalMinor := rowTotal.Minor()
				totalQty += mr.Quantity
				totalValMinor += rowTotalMinor

				if mi.ProductID != nil {
					matchedCount++
				} else {
					unlinkedCount++
				}

				stagedItems = append(stagedItems, &StagedSavingItem{
					Index:             mi.Index,
					NameProduct:       mr.Name,
					SKU:               mr.SKU,
					Quantity:          mr.Quantity,
					Price:             mr.Price,
					TotalValue:        money.FromMinor(rowTotalMinor),
					ProductID:         mi.ProductID,
					MasterProductName: mi.MasterProductName,
					MasterProductSKU:  mi.MasterProductSKU,
					MatchType:         mi.MatchType,
					Confidence:        mi.Confidence,
					Included:          true,
				})
			}
		}
	}

	// Recount from the rows rather than adjusting by the number changed: the
	// AI stage also checks linked rows, and a disputed one is UNLINKED while a
	// confirmed one changes label but not state — neither is a new match.
	if h.enhanceSaving(database.AsSystem(ctx), useAI, matchEngine, stagedItems) > 0 {
		matchedCount, unlinkedCount = 0, 0
		for _, it := range stagedItems {
			if it.ProductID != nil {
				matchedCount++
			} else {
				unlinkedCount++
			}
		}
	}

	globalSavingImportSessionStore.CompleteProcessing(
		session.ID, stagedItems, matchedCount, unlinkedCount, totalQty, money.FromMinor(totalValMinor),
	)
	session.Phase = SavingPhaseReview

	if isRunsPattern {
		http.Redirect(w, r, fmt.Sprintf("/admin/organizations/import/%d/runs/%s/review", orgID, runID), http.StatusSeeOther)
	} else {
		http.Redirect(w, r, fmt.Sprintf("/admin/organizations/import/%d/saving/%s/review", orgID, runID), http.StatusSeeOther)
	}
}

// AdminOrgImportSavingReviewPage renders the persistent review step surviving page refreshes.
func (h *UIHandler) AdminOrgImportSavingReviewPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	orgID, _ := strconv.ParseInt(chi.URLParam(r, "orgID"), 10, 64)
	runID := chi.URLParam(r, "runID")
	if runID == "" {
		runID = chi.URLParam(r, "id")
	}

	session, ok := globalSavingImportSessionStore.GetSessionForAdmin(runID)
	if !ok {
		h.redirectWithNotice(w, r, "/admin/organizations/import", "error", i18n.T(lang, "customer.saving.import.session_not_found"))
		return
	}
	if orgID <= 0 {
		orgID = session.OrgID
	}

	if session.Phase == SavingPhaseMapping {
		http.Redirect(w, r, fmt.Sprintf("/admin/organizations/import/%d/saving/%s/mapping", orgID, runID), http.StatusSeeOther)
		return
	}

	filter := SavingRowFilter{
		Search:      strings.TrimSpace(r.URL.Query().Get("q")),
		MatchFilter: strings.TrimSpace(r.URL.Query().Get("match")),
		SortBy:      strings.TrimSpace(r.URL.Query().Get("sort")),
		SortOrder:   strings.TrimSpace(r.URL.Query().Get("order")),
		Page:        pagination.PageNumber(r),
		Limit:       pagination.RowsPerPage(r),
	}

	rows, total := globalSavingImportSessionStore.FilterItems(session, filter)
	sysCtx := database.AsSystem(ctx)
	targetOrgName, targetOrgType := h.resolveTargetOrgInfo(sysCtx, orgID)

	view := pages.SavingImportView{
		AIAvailable:         h.matchEnhancer != nil,
		AIUnavailableReason: savingAIUnavailableReason(h.matchEnhancer, lang),
		Audience:            "admin",
		TargetOrgName:       targetOrgName,
		TargetOrgType:       targetOrgType,
		BaseURL:             "/admin/organizations/import",
		ImportURL:           fmt.Sprintf("/admin/organizations/import/%d/saving", orgID),
		Session:             session,
		Filter:              filter,
		Rows:                rows,
		RowTotal:            total,
	}

	h.renderPage(ctx, w, "render admin org saving review step", pages.SavingImportPage(view, lang, dir))
}

// AdminOrgImportSavingCommitSubmit saves staged saving products to the target organization.
func (h *UIHandler) AdminOrgImportSavingCommitSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, _ := authctx.From(ctx)

	runID := chi.URLParam(r, "runID")
	if runID == "" {
		runID = chi.URLParam(r, "id")
	}

	session, ok := globalSavingImportSessionStore.GetSessionForAdmin(runID)
	if !ok {
		h.redirectWithNotice(w, r, "/admin/organizations/import", "error", i18n.T(lang, "saving.import.session_not_found"))
		return
	}

	added, updated, err := globalSavingImportSessionStore.CommitSession(database.AsSystem(ctx), runID, session.OrgID, actor.UserID, h.catSvc)
	if err != nil {
		h.log.ErrorContext(ctx, "failed to commit saving products session", "error", err, "run_id", runID, "target_org_id", session.OrgID)
		h.notifyImportRunFailed(ctx, actor.UserID, session.OrgID, 0, err.Error())
		h.redirectWithNotice(w, r, fmt.Sprintf("/admin/organizations/import/%d/saving/%s/review", session.OrgID, runID), "error", h.safeMessage(err, lang))
		return
	}

	h.log.InfoContext(ctx, "admin committed saving products for organization", "actor_id", actor.UserID, "target_org_id", session.OrgID, "added", added, "updated", updated)
	h.notifyImportRunFinished(ctx, actor.UserID, session.OrgID, 0, added+updated)

	successMsg := fmt.Sprintf(i18n.T(lang, "customer.saving.import.commit_success"), added+updated, added, updated)
	h.redirectWithNotice(w, r, "/admin/organizations/import?tab=pharmacy", "success", successMsg)
}

// AdminOrgImportSavingCancelSubmit cancels an import session.
func (h *UIHandler) AdminOrgImportSavingCancelSubmit(w http.ResponseWriter, r *http.Request) {
	runID := chi.URLParam(r, "runID")
	if runID == "" {
		runID = chi.URLParam(r, "id")
	}
	if session, ok := globalSavingImportSessionStore.GetSessionForAdmin(runID); ok {
		globalSavingImportSessionStore.CancelSession(runID, session.OrgID)
	}
	h.redirectWithNotice(w, r, "/admin/organizations/import", "info", i18n.T(langOf(r), "saving.import.cancelled_success"))
}

// AdminOrgImportSavingSessionPage provides backwards-compatible redirect to persistent step URLs.
func (h *UIHandler) AdminOrgImportSavingSessionPage(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "id")
	session, ok := globalSavingImportSessionStore.GetSessionForAdmin(sessionID)
	if !ok {
		h.redirectWithNotice(w, r, "/admin/organizations/import", "error", i18n.T(langOf(r), "customer.saving.import.session_not_found"))
		return
	}

	if session.Phase == SavingPhaseReview {
		http.Redirect(w, r, fmt.Sprintf("/admin/organizations/import/%d/runs/%s/review", session.OrgID, sessionID), http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/admin/organizations/import/%d/runs/%s/mapping", session.OrgID, sessionID), http.StatusSeeOther)
}
