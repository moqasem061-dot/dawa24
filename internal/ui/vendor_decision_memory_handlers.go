package ui

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/moqasem061-dot/dawa24/internal/modules/catalog/matching"
	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
	"github.com/moqasem061-dot/dawa24/internal/shared/i18n"
	"github.com/moqasem061-dot/dawa24/internal/shared/pagination"
	"github.com/moqasem061-dot/dawa24/internal/ui/pages"
)

// VendorDecisionMemoryPage renders decision memories for vendor organizations.
func (h *UIHandler) VendorDecisionMemoryPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/decision-memory", http.StatusSeeOther)
		return
	}

	search := strings.TrimSpace(r.URL.Query().Get("q"))
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	limit := pagination.RowsPerPage(r)
	offset := (page - 1) * limit

	decisions, total := h.loadOrgKnowledgeDecisions(ctx, actor.OrganizationID, search, limit, offset)

	usePlatform, loadErr := h.catSvc.GetDecisionMemoryPreference(ctx, actor.OrganizationID)
	h.logLoadErr(ctx, "catSvc.GetDecisionMemoryPreference", loadErr)
	isEnabled := h.catSvc.IsDecisionMemoryEnabled(ctx)

	data := pages.CustomerDecisionMemoryData{
		Decisions:         decisions,
		Total:             total,
		Page:              page,
		PerPage:           limit,
		Search:            search,
		IsEnabled:         isEnabled,
		IsVendor:          true,
		UsePlatformMemory: usePlatform,
		QueryValues:       r.URL.Query(),
	}

	h.renderPage(ctx, w, "render CustomerDecisionMemoryPage", pages.CustomerDecisionMemoryPage(lang, dir, data))
}

// VendorDecisionMemoryTogglePlatformSubmit toggles platform memory usage for the vendor organization.
func (h *UIHandler) VendorDecisionMemoryTogglePlatformSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/decision-memory", http.StatusSeeOther)
		return
	}

	current, loadErr := h.catSvc.GetDecisionMemoryPreference(ctx, actor.OrganizationID)
	h.logLoadErr(ctx, "catSvc.GetDecisionMemoryPreference", loadErr)
	target := !current
	if val := r.FormValue("use_platform_memory"); val != "" {
		target = (val == "true" || val == "1" || val == "on")
	}

	if err := h.catSvc.SetDecisionMemoryPreference(ctx, actor.OrganizationID, target, actor.UserID); err != nil {
		h.log.ErrorContext(ctx, "failed to update platform memory preference for vendor", "error", err)
		h.redirectWithNotice(w, r, "/vendor/decision-memory", "error", i18n.T(lang, "decision_memory.platform_toggle_error"))
		return
	}

	h.redirectWithNotice(w, r, "/vendor/decision-memory", "success", i18n.T(lang, "decision_memory.platform_toggle_success"))
}

// VendorDecisionMemoryRelinkSubmit allows the vendor to update the catalog product for an org-owned decision.
func (h *UIHandler) VendorDecisionMemoryRelinkSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/decision-memory", http.StatusSeeOther)
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/vendor/decision-memory", "error", i18n.T(lang, "admin.decision_memory.invalid_id"))
		return
	}

	var productID *int64
	prodStr := strings.TrimSpace(r.FormValue("product_id"))
	if prodStr != "" {
		if pid, err := strconv.ParseInt(prodStr, 10, 64); err == nil && pid > 0 {
			productID = &pid
		}
	}

	if err := h.catSvc.RelinkMatchDecisionForOrg(ctx, actor.OrganizationID, id, productID, actor.UserID); err != nil {
		h.log.ErrorContext(ctx, "failed to relink decision for vendor", "id", id, "error", err)
		h.redirectWithNotice(w, r, "/vendor/decision-memory", "error", i18n.T(lang, "admin.decision_memory.relink_error"))
		return
	}

	h.redirectWithNotice(w, r, "/vendor/decision-memory", "success", i18n.T(lang, "admin.decision_memory.relink_success"))
}

// VendorDecisionMemoryAddSubmit adds a new manual match decision for the vendor.
func (h *UIHandler) VendorDecisionMemoryAddSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/decision-memory", http.StatusSeeOther)
		return
	}

	rawName := strings.TrimSpace(r.FormValue("raw_name"))
	productID, err := strconv.ParseInt(strings.TrimSpace(r.FormValue("product_id")), 10, 64)
	if err != nil || productID <= 0 || rawName == "" {
		h.redirectWithNotice(w, r, "/vendor/decision-memory", "error", i18n.T(lang, "decision_memory.item_and_drug_required"))
		return
	}

	reason := strings.TrimSpace(r.FormValue("reason"))
	if reason == "" {
		reason = i18n.T(lang, "decision_memory.vendor_manual_reason")
	}

	if err := h.catSvc.SaveManualDecision(ctx, actor.OrganizationID, actor.UserID, rawName, productID, reason); err != nil {
		h.log.ErrorContext(ctx, "failed to add manual decision for vendor", "error", err)
		h.redirectWithNotice(w, r, "/vendor/decision-memory", "error", i18n.T(lang, "decision_memory.save_error"))
		return
	}

	h.redirectWithNotice(w, r, "/vendor/decision-memory", "success", i18n.T(lang, "decision_memory.vendor_saved_success"))
}

// VendorDecisionMemoryDeleteSubmit deletes a single vendor saved match decision / knowledge.
func (h *UIHandler) VendorDecisionMemoryDeleteSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/decision-memory", http.StatusSeeOther)
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/vendor/decision-memory", "error", i18n.T(lang, "admin.decision_memory.invalid_id"))
		return
	}

	if h.catSvc != nil && h.catSvc.Matching() != nil {
		sc := matching.Scope{
			EffectiveOrg: actor.OrganizationID,
			Workflow:     matching.WorkflowVendor,
			EntryPoint:   "vendor.decision_memory",
			ActorUserID:  actor.UserID,
		}
		_ = h.catSvc.Matching().RevokeKnowledge(ctx, sc, id)
	}

	if err := h.catSvc.DeleteMatchDecisionForOrg(ctx, actor.OrganizationID, id); err != nil {
		h.log.ErrorContext(ctx, "failed to delete match decision for vendor", "id", id, "error", err)
		h.redirectWithNotice(w, r, "/vendor/decision-memory", "error", i18n.T(lang, "admin.decision_memory.delete_error"))
		return
	}

	h.redirectWithNotice(w, r, "/vendor/decision-memory", "success", i18n.T(lang, "admin.decision_memory.delete_success"))
}

// VendorDecisionMemoryClearSubmit clears all match decisions from the vendor organization's memory.
func (h *UIHandler) VendorDecisionMemoryClearSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/decision-memory", http.StatusSeeOther)
		return
	}

	if err := h.catSvc.ClearMatchDecisionsForOrg(ctx, actor.OrganizationID); err != nil {
		h.log.ErrorContext(ctx, "failed to clear match decisions for vendor", "error", err)
		h.redirectWithNotice(w, r, "/vendor/decision-memory", "error", i18n.T(lang, "admin.decision_memory.clear_error"))
		return
	}

	h.redirectWithNotice(w, r, "/vendor/decision-memory", "success", i18n.T(lang, "decision_memory.org_cleared_success"))
}
