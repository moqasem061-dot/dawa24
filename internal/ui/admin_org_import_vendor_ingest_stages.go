package ui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/ingest"
	"github.com/muhiya/dawa24-store/internal/modules/inventory"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/productmatch"
)

// AdminOrgImportVendorIngestSettingsSubmit records import rules on behalf of vendor org.
func (h *UIHandler) AdminOrgImportVendorIngestSettingsSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	orgID, _ := strconv.ParseInt(chi.URLParam(r, "orgID"), 10, 64)
	publicID := chi.URLParam(r, "id")

	if h.ingSvc == nil {
		h.redirectWithNotice(w, r, "/admin/organizations/import?tab=vendor", "error", i18n.T(lang, "common.import_service_unavailable"))
		return
	}
	if err := r.ParseForm(); err != nil {
		h.redirectWithNotice(w, r, fmt.Sprintf("/admin/organizations/import/%d/ingest/%s", orgID, publicID), "error", i18n.T(lang, "common.invalid_form_data"))
		return
	}

	sysCtx := database.WithTenant(database.AsSystem(ctx), orgID)
	settings := ingest.DefaultSettings()
	settings.WarehouseID, _ = strconv.ParseInt(r.PostFormValue("warehouse_id"), 10, 64)

	if settings.WarehouseID > 0 && h.invSvc != nil {
		if wh, err := h.invSvc.GetWarehouse(sysCtx, settings.WarehouseID); err == nil && wh != nil && wh.BranchID != nil && *wh.BranchID > 0 {
			settings.BranchID = wh.BranchID
		}
	}
	if settings.WarehouseID <= 0 && h.invSvc != nil {
		whs, _ := h.invSvc.ListWarehouses(sysCtx)
		for _, w := range whs {
			if w.OrganizationID == orgID && w.IsActive {
				settings.WarehouseID = w.ID
				if settings.BranchID == nil && w.BranchID != nil && *w.BranchID > 0 {
					settings.BranchID = w.BranchID
				}
				break
			}
		}
	}

	settings.Mode = ingest.ParseMode(r.PostFormValue("mode"))
	settings.StockMode = inventory.StockMode(r.PostFormValue("stock_mode"))
	settings.Duplicates = productmatch.DuplicatePolicy(r.PostFormValue("duplicates"))
	if score, err := strconv.ParseFloat(r.PostFormValue("min_match_score"), 64); err == nil {
		settings.MinMatchScore = score / 100
	}
	settings.TrustSupplierCode = checked(r, "trust_supplier_code")
	settings.CodeIsCatalogCode = checked(r, "code_is_catalog_code")
	settings.TrustBarcode = checked(r, "trust_barcode")
	settings.BlankQuantityIsZero = checked(r, "blank_quantity_is_zero")
	settings.RejectExpired = checked(r, "reject_expired")
	settings.MarkNegotiable = checked(r, "mark_negotiable")
	settings.PublishImmediately = checked(r, "publish_immediately")
	settings.UseAI = checked(r, "use_ai") && h.ingSvc.AIAvailable()
	settings.RecordRows = checked(r, "record_rows")
	if v, err := strconv.Atoi(r.PostFormValue("default_min_order_qty")); err == nil {
		settings.DefaultMinOrderQty = v
	}
	if v, err := strconv.Atoi(r.PostFormValue("default_min_threshold")); err == nil {
		settings.DefaultMinThreshold = v
	}

	if _, err := h.ingSvc.SaveSettings(sysCtx, publicID, settings); err != nil {
		h.redirectWithNotice(w, r, fmt.Sprintf("/admin/organizations/import/%d/ingest/%s", orgID, publicID), "error", h.safeMessage(err, lang))
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/admin/organizations/import/%d/ingest/%s", orgID, publicID), http.StatusSeeOther)
}

// AdminOrgImportVendorIngestBackSubmit reopens column mapping for vendor ingest session.
func (h *UIHandler) AdminOrgImportVendorIngestBackSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	orgID, _ := strconv.ParseInt(chi.URLParam(r, "orgID"), 10, 64)
	publicID := chi.URLParam(r, "id")

	if h.ingSvc != nil {
		sysCtx := database.WithTenant(database.AsSystem(ctx), orgID)
		if _, err := h.ingSvc.BackToMapping(sysCtx, publicID); err != nil {
			h.redirectWithNotice(w, r, fmt.Sprintf("/admin/organizations/import/%d/ingest/%s", orgID, publicID), "error", h.safeMessage(err, lang))
			return
		}
	}
	http.Redirect(w, r, fmt.Sprintf("/admin/organizations/import/%d/ingest/%s", orgID, publicID), http.StatusSeeOther)
}

// AdminOrgImportVendorIngestBackToSettingsSubmit reopens settings stage from review.
func (h *UIHandler) AdminOrgImportVendorIngestBackToSettingsSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	orgID, _ := strconv.ParseInt(chi.URLParam(r, "orgID"), 10, 64)
	publicID := chi.URLParam(r, "id")

	if h.ingSvc != nil {
		sysCtx := database.WithTenant(database.AsSystem(ctx), orgID)
		if _, err := h.ingSvc.BackToSettings(sysCtx, publicID); err != nil {
			h.redirectWithNotice(w, r, fmt.Sprintf("/admin/organizations/import/%d/ingest/%s", orgID, publicID), "error", h.safeMessage(err, lang))
			return
		}
	}
	http.Redirect(w, r, fmt.Sprintf("/admin/organizations/import/%d/ingest/%s", orgID, publicID), http.StatusSeeOther)
}

// AdminOrgImportVendorIngestConfirmSubmit starts background commit run on behalf of vendor org.
func (h *UIHandler) AdminOrgImportVendorIngestConfirmSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	orgID, _ := strconv.ParseInt(chi.URLParam(r, "orgID"), 10, 64)
	publicID := chi.URLParam(r, "id")

	if h.ingSvc == nil {
		h.redirectWithNotice(w, r, "/admin/organizations/import?tab=vendor", "error", i18n.T(lang, "common.import_service_unavailable"))
		return
	}
	sysCtx := database.WithTenant(database.AsSystem(ctx), orgID)
	if _, err := h.ingSvc.CommitInBackground(sysCtx, publicID); err != nil {
		h.redirectWithNotice(w, r, fmt.Sprintf("/admin/organizations/import/%d/ingest/%s", orgID, publicID), "error", h.safeMessage(err, lang))
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/admin/organizations/import/%d/ingest/%s", orgID, publicID), http.StatusSeeOther)
}

// AdminOrgImportVendorIngestProgress reports progress for admin polling.
func (h *UIHandler) AdminOrgImportVendorIngestProgress(w http.ResponseWriter, r *http.Request) {
	orgID, _ := strconv.ParseInt(chi.URLParam(r, "orgID"), 10, 64)
	publicID := chi.URLParam(r, "id")
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if h.ingSvc == nil {
		http.Error(w, `{"error":"unavailable"}`, http.StatusServiceUnavailable)
		return
	}
	sysCtx := database.WithTenant(database.AsSystem(r.Context()), orgID)
	session, err := h.ingSvc.LoadImport(sysCtx, publicID)
	if err != nil {
		http.Error(w, `{"error":"not_found"}`, http.StatusNotFound)
		return
	}
	percent := session.ProgressPercent
	running := session.Phase == ingest.PhaseProcessing
	if running && percent >= 100 {
		percent = 99
	}
	if !running {
		percent = 100
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"phase":    session.Phase,
		"percent":  percent,
		"note":     session.ProgressNote,
		"message":  session.ProgressNote,
		"done":     session.Phase != ingest.PhaseProcessing,
		"inserted": session.InsertedRows,
		"updated":  session.UpdatedRows,
		"skipped":  session.SkippedRows,
		"errors":   session.ErrorRows,
	})
}
