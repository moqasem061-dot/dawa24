package ui

import (
	"bytes"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/xuri/excelize/v2"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/pagination"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// AdminMatchDecisionsPage renders the central catalog decision memory management page for administrators.
func (h *UIHandler) AdminMatchDecisionsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	filter := parseAdminDecisionFilter(r)
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	limit := pagination.RowsPerPage(r)
	filter.Limit = limit
	filter.Offset = (page - 1) * limit

	decisions, total, err := h.catSvc.ListMatchDecisionsFiltered(ctx, filter)
	if err != nil {
		h.log.ErrorContext(ctx, "failed to list match decisions", "error", err)
	}

	isEnabled := h.catSvc.IsDecisionMemoryEnabled(ctx)

	data := pages.AdminMatchDecisionsData{
		Decisions:     decisions,
		Total:         total,
		Page:          page,
		PerPage:       limit,
		Search:        filter.Search,
		IsEnabled:     isEnabled,
		Scope:         filter.Scope,
		Source:        filter.Source,
		OnlyUnlinked:  filter.OnlyUnlinked,
		PromptVersion: filter.PromptVersion,
		QueryValues:   r.URL.Query(),
	}
	if filter.OrganizationID != nil {
		data.OrgID = strconv.FormatInt(*filter.OrganizationID, 10)
	}
	if filter.UserID != nil {
		data.UserID = strconv.FormatInt(*filter.UserID, 10)
	}
	if filter.MinConfidence != nil {
		data.MinConfidence = fmt.Sprintf("%.2f", *filter.MinConfidence)
	}
	if filter.MaxConfidence != nil {
		data.MaxConfidence = fmt.Sprintf("%.2f", *filter.MaxConfidence)
	}
	if filter.MinHitCount != nil {
		data.MinHitCount = strconv.FormatInt(*filter.MinHitCount, 10)
	}
	if filter.CreatedFrom != nil {
		data.CreatedFrom = filter.CreatedFrom.Format("2006-01-02")
	}
	if filter.CreatedTo != nil {
		data.CreatedTo = filter.CreatedTo.Format("2006-01-02")
	}
	if filter.LastUsedFrom != nil {
		data.UsedFrom = filter.LastUsedFrom.Format("2006-01-02")
	}
	if filter.LastUsedTo != nil {
		data.UsedTo = filter.LastUsedTo.Format("2006-01-02")
	}

	_ = pages.AdminMatchDecisionsPage(lang, dir, data).Render(ctx, w)
}

// AdminMatchDecisionsExportXLSX exports the filtered decision memories to Excel.
func (h *UIHandler) AdminMatchDecisionsExportXLSX(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, _ := h.localeAndDir(r)
	filter := parseAdminDecisionFilter(r)
	filter.Limit = 5000
	filter.Offset = 0

	decisions, _, err := h.catSvc.ListMatchDecisionsFiltered(ctx, filter)
	if err != nil {
		h.log.ErrorContext(ctx, "failed to export match decisions", "error", err)
		http.Error(w, "export failed", http.StatusInternalServerError)
		return
	}

	f := excelize.NewFile()
	defer func() { _ = f.Close() }()
	sheet := i18n.T(lang, "admin.catalog.match_decisions")
	f.SetSheetName("Sheet1", sheet)
	_ = f.SetSheetView(sheet, 0, &excelize.ViewOptions{RightToLeft: boolPtr(lang == "ar")})

	headers := []string{
		"#",
		i18n.T(lang, "admin.catalog.col_scope"),
		i18n.T(lang, "admin.catalog.col_source"),
		i18n.T(lang, "admin.catalog.incoming_product_name"),
		i18n.T(lang, "admin.catalog.approved_catalog_product"),
		i18n.T(lang, "admin.catalog.sku"),
		i18n.T(lang, "admin.catalog.confidence"),
		i18n.T(lang, "admin.catalog.hit_count"),
		i18n.T(lang, "admin.catalog.reason"),
		i18n.T(lang, "admin.catalog.col_org"),
		i18n.T(lang, "admin.catalog.col_user"),
		i18n.T(lang, "admin.catalog.col_created_at"),
		i18n.T(lang, "admin.catalog.col_last_used"),
	}
	for i, head := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		_ = f.SetCellValue(sheet, cell, head)
	}

	for rIdx, d := range decisions {
		rowNum := rIdx + 2
		scopeLabel := i18n.T(lang, "admin.catalog.scope_org")
		if d.Scope == "platform" {
			scopeLabel = i18n.T(lang, "admin.catalog.scope_platform")
		}
		sourceLabel := d.Source
		switch d.Source {
		case "ai":
			sourceLabel = i18n.T(lang, "admin.catalog.source_ai")
		case "manual":
			sourceLabel = i18n.T(lang, "admin.catalog.source_manual")
		case "admin":
			sourceLabel = i18n.T(lang, "admin.catalog.source_admin")
		case "import":
			sourceLabel = i18n.T(lang, "admin.catalog.source_import")
		}
		confStr := fmt.Sprintf("%.0f%%", d.Confidence*100)
		orgStr := d.OrganizationName
		if orgStr == "" && d.OrganizationID != nil {
			orgStr = fmt.Sprintf(i18n.T(lang, "admin.catalog.org_hash"), *d.OrganizationID)
		} else if d.OrganizationID == nil {
			orgStr = i18n.T(lang, "admin.catalog.platform_general")
		}

		vals := []any{
			d.ID, scopeLabel, sourceLabel, d.NormName, d.ChosenProductName,
			d.ChosenProductSKU, confStr, d.HitCount, d.Reason,
			orgStr, d.UserName, d.CreatedAt.Format("2006-01-02 15:04"),
			d.LastUsedAt.Format("2006-01-02 15:04"),
		}
		for cIdx, val := range vals {
			cell, _ := excelize.CoordinatesToCellName(cIdx+1, rowNum)
			_ = f.SetCellValue(sheet, cell, val)
		}
	}

	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		http.Error(w, "excel write error", http.StatusInternalServerError)
		return
	}

	filename := fmt.Sprintf("match_decisions_%s.xlsx", time.Now().Format("20060102_150405"))
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))
	_, _ = w.Write(buf.Bytes())
}

// AdminMatchDecisionPromoteSubmit promotes a decision from org to platform scope.
func (h *UIHandler) AdminMatchDecisionPromoteSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, preserveDecisionReturnURL(r), "error", i18n.T(lang, "admin.decision_memory.invalid_id"))
		return
	}

	var adminUserID int64
	if actor, ok := authctx.From(ctx); ok {
		adminUserID = actor.UserID
	}

	if err := h.catSvc.PromoteMatchDecision(ctx, id, adminUserID); err != nil {
		h.log.ErrorContext(ctx, "failed to promote decision", "id", id, "error", err)
		h.redirectWithNotice(w, r, preserveDecisionReturnURL(r), "error", i18n.T(lang, "admin.decision_memory.promote_error"))
		return
	}

	h.redirectWithNotice(w, r, preserveDecisionReturnURL(r), "success", i18n.T(lang, "admin.decision_memory.promote_success"))
}

// AdminMatchDecisionDemoteSubmit demotes a decision back to org scope.
func (h *UIHandler) AdminMatchDecisionDemoteSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, preserveDecisionReturnURL(r), "error", i18n.T(lang, "admin.decision_memory.invalid_id"))
		return
	}

	if err := h.catSvc.DemoteMatchDecision(ctx, id); err != nil {
		h.log.ErrorContext(ctx, "failed to demote decision", "id", id, "error", err)
		h.redirectWithNotice(w, r, preserveDecisionReturnURL(r), "error", i18n.T(lang, "admin.decision_memory.demote_error"))
		return
	}

	h.redirectWithNotice(w, r, preserveDecisionReturnURL(r), "success", i18n.T(lang, "admin.decision_memory.demote_success"))
}

// AdminMatchDecisionRelinkSubmit relinks a decision to a chosen catalog product.
func (h *UIHandler) AdminMatchDecisionRelinkSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, preserveDecisionReturnURL(r), "error", i18n.T(lang, "admin.decision_memory.invalid_id"))
		return
	}

	var productID *int64
	prodStr := strings.TrimSpace(r.FormValue("product_id"))
	if prodStr != "" {
		if pid, err := strconv.ParseInt(prodStr, 10, 64); err == nil && pid > 0 {
			productID = &pid
		}
	}

	var adminUserID int64
	if actor, ok := authctx.From(ctx); ok {
		adminUserID = actor.UserID
	}

	if err := h.catSvc.RelinkMatchDecision(ctx, id, productID, adminUserID); err != nil {
		h.log.ErrorContext(ctx, "failed to relink decision", "id", id, "error", err)
		h.redirectWithNotice(w, r, preserveDecisionReturnURL(r), "error", i18n.T(lang, "admin.decision_memory.relink_error"))
		return
	}

	h.redirectWithNotice(w, r, preserveDecisionReturnURL(r), "success", i18n.T(lang, "admin.decision_memory.relink_success"))
}

// AdminMatchDecisionBulkSubmit executes bulk actions (promote, delete) across selected decisions.
func (h *UIHandler) AdminMatchDecisionBulkSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	action := r.FormValue("action")
	idStrs := r.Form["ids"]
	if len(idStrs) == 0 {
		h.redirectWithNotice(w, r, preserveDecisionReturnURL(r), "error", i18n.T(lang, "admin.decision_memory.invalid_id"))
		return
	}

	var ids []int64
	for _, s := range idStrs {
		if id, err := strconv.ParseInt(s, 10, 64); err == nil && id > 0 {
			ids = append(ids, id)
		}
	}

	var adminUserID int64
	if actor, ok := authctx.From(ctx); ok {
		adminUserID = actor.UserID
	}

	var affected int64
	var err error
	switch action {
	case "promote":
		affected, err = h.catSvc.BulkPromoteMatchDecisions(ctx, ids, adminUserID)
	case "delete":
		affected, err = h.catSvc.BulkDeleteMatchDecisions(ctx, ids)
	default:
		h.redirectWithNotice(w, r, preserveDecisionReturnURL(r), "error", i18n.T(lang, "admin.decision_memory.bulk_error"))
		return
	}

	if err != nil {
		h.log.ErrorContext(ctx, "bulk action failed", "action", action, "error", err)
		h.redirectWithNotice(w, r, preserveDecisionReturnURL(r), "error", i18n.T(lang, "admin.decision_memory.bulk_error"))
		return
	}

	h.redirectWithNotice(w, r, preserveDecisionReturnURL(r), "success", fmt.Sprintf(i18n.T(lang, "admin.decision_memory.bulk_success"), affected))
}

// AdminMatchDecisionToggleStateSubmit toggles the global Decision Memory active switch.
func (h *UIHandler) AdminMatchDecisionToggleStateSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	currentlyEnabled := h.catSvc.IsDecisionMemoryEnabled(ctx)
	targetState := !currentlyEnabled
	if stateStr := r.FormValue("enabled"); stateStr != "" {
		targetState = (stateStr == "true" || stateStr == "1" || stateStr == "on")
	}

	if err := h.catSvc.SetDecisionMemoryEnabled(ctx, targetState); err != nil {
		h.log.ErrorContext(ctx, "failed to toggle decision memory state", "error", err)
		h.redirectWithNotice(w, r, "/admin/match-decisions", "error", i18n.T(lang, "admin.decision_memory.toggle_error"))
		return
	}

	stateLabel := i18n.T(lang, "admin.decision_memory.state_enabled")
	if !targetState {
		stateLabel = i18n.T(lang, "admin.decision_memory.state_disabled")
	}
	h.redirectWithNotice(w, r, "/admin/match-decisions", "success", fmt.Sprintf(i18n.T(lang, "admin.decision_memory.toggle_success"), stateLabel))
}

// AdminMatchDecisionDeleteSubmit deletes a single match decision.
func (h *UIHandler) AdminMatchDecisionDeleteSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, preserveDecisionReturnURL(r), "error", i18n.T(lang, "admin.decision_memory.invalid_id"))
		return
	}

	if err := h.catSvc.DeleteMatchDecision(ctx, id); err != nil {
		h.log.ErrorContext(ctx, "failed to delete match decision", "id", id, "error", err)
		h.redirectWithNotice(w, r, preserveDecisionReturnURL(r), "error", i18n.T(lang, "admin.decision_memory.delete_error"))
		return
	}

	h.redirectWithNotice(w, r, preserveDecisionReturnURL(r), "success", i18n.T(lang, "admin.decision_memory.delete_success"))
}

// AdminMatchDecisionsClearSubmit purges the entire decision cache.
func (h *UIHandler) AdminMatchDecisionsClearSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	if err := h.catSvc.ClearMatchDecisions(ctx); err != nil {
		h.log.ErrorContext(ctx, "failed to clear match decisions", "error", err)
		h.redirectWithNotice(w, r, "/admin/match-decisions", "error", i18n.T(lang, "admin.decision_memory.clear_error"))
		return
	}
	h.redirectWithNotice(w, r, "/admin/match-decisions", "success", i18n.T(lang, "admin.decision_memory.clear_success"))
}

func parseAdminDecisionFilter(r *http.Request) catalog.DecisionMemoryFilter {
	q := r.URL.Query()
	f := catalog.DecisionMemoryFilter{
		Search:        strings.TrimSpace(q.Get("q")),
		Scope:         strings.TrimSpace(q.Get("scope")),
		Source:        strings.TrimSpace(q.Get("source")),
		PromptVersion: strings.TrimSpace(q.Get("prompt_version")),
		OnlyUnlinked:  q.Get("unlinked") == "true" || q.Get("unlinked") == "1",
	}
	if oid, err := strconv.ParseInt(q.Get("org_id"), 10, 64); err == nil && oid > 0 {
		f.OrganizationID = &oid
	}
	if uid, err := strconv.ParseInt(q.Get("user_id"), 10, 64); err == nil && uid > 0 {
		f.UserID = &uid
	}
	if minC, err := strconv.ParseFloat(q.Get("min_confidence"), 64); err == nil && minC >= 0 {
		f.MinConfidence = &minC
	}
	if maxC, err := strconv.ParseFloat(q.Get("max_confidence"), 64); err == nil && maxC >= 0 {
		f.MaxConfidence = &maxC
	}
	if minH, err := strconv.ParseInt(q.Get("min_hits"), 10, 64); err == nil && minH >= 0 {
		f.MinHitCount = &minH
	}
	if cf, err := time.Parse("2006-01-02", q.Get("created_from")); err == nil {
		f.CreatedFrom = &cf
	}
	if ct, err := time.Parse("2006-01-02", q.Get("created_to")); err == nil {
		endDay := ct.Add(24*time.Hour - time.Nanosecond)
		f.CreatedTo = &endDay
	}
	if uf, err := time.Parse("2006-01-02", q.Get("used_from")); err == nil {
		f.LastUsedFrom = &uf
	}
	if ut, err := time.Parse("2006-01-02", q.Get("used_to")); err == nil {
		endDay := ut.Add(24*time.Hour - time.Nanosecond)
		f.LastUsedTo = &endDay
	}
	return f
}

func preserveDecisionReturnURL(r *http.Request) string {
	ret := r.FormValue("return_url")
	if ret != "" && strings.HasPrefix(ret, "/admin/match-decisions") {
		return ret
	}
	return "/admin/match-decisions"
}
