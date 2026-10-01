package ui

import (
	"bytes"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/xuri/excelize/v2"

	"github.com/moqasem061-dot/dawa24/internal/modules/catalog"
	"github.com/moqasem061-dot/dawa24/internal/shared/i18n"
)

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
	sheet = renameFirstSheet(f, sheet)
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

