package ui

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/moqasem061-dot/dawa24/internal/modules/catalog"
	"github.com/moqasem061-dot/dawa24/internal/modules/catalog/matching"
	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
	"github.com/moqasem061-dot/dawa24/internal/shared/i18n"
	"github.com/moqasem061-dot/dawa24/internal/shared/pagination"
	"github.com/moqasem061-dot/dawa24/internal/ui/pages"
)

// CustomerDecisionMemoryPage renders decision memories for pharmacy customer organizations.
func (h *UIHandler) CustomerDecisionMemoryPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/customer/decision-memory", http.StatusSeeOther)
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
		IsVendor:          false,
		UsePlatformMemory: usePlatform,
		QueryValues:       r.URL.Query(),
	}

	h.renderPage(ctx, w, "render CustomerDecisionMemoryPage", pages.CustomerDecisionMemoryPage(lang, dir, data))
}

// CustomerDecisionMemoryTogglePlatformSubmit toggles platform memory usage for the customer organization.
func (h *UIHandler) CustomerDecisionMemoryTogglePlatformSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/customer/decision-memory", http.StatusSeeOther)
		return
	}

	current, loadErr := h.catSvc.GetDecisionMemoryPreference(ctx, actor.OrganizationID)
	h.logLoadErr(ctx, "catSvc.GetDecisionMemoryPreference", loadErr)
	target := !current
	if val := r.FormValue("use_platform_memory"); val != "" {
		target = (val == "true" || val == "1" || val == "on")
	}

	if err := h.catSvc.SetDecisionMemoryPreference(ctx, actor.OrganizationID, target, actor.UserID); err != nil {
		h.log.ErrorContext(ctx, "failed to update platform memory preference for customer", "error", err)
		h.redirectWithNotice(w, r, "/customer/decision-memory", "error", i18n.T(lang, "decision_memory.platform_toggle_error"))
		return
	}

	h.redirectWithNotice(w, r, "/customer/decision-memory", "success", i18n.T(lang, "decision_memory.platform_toggle_success"))
}

// CustomerDecisionMemoryRelinkSubmit allows the pharmacy to update the catalog product for an org-owned decision.
func (h *UIHandler) CustomerDecisionMemoryRelinkSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/customer/decision-memory", http.StatusSeeOther)
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/customer/decision-memory", "error", i18n.T(lang, "admin.decision_memory.invalid_id"))
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
		h.log.ErrorContext(ctx, "failed to relink decision for customer", "id", id, "error", err)
		h.redirectWithNotice(w, r, "/customer/decision-memory", "error", i18n.T(lang, "admin.decision_memory.relink_error"))
		return
	}

	h.redirectWithNotice(w, r, "/customer/decision-memory", "success", i18n.T(lang, "admin.decision_memory.relink_success"))
}

// CustomerDecisionMemoryAddSubmit adds a new manual match decision for the pharmacy.
func (h *UIHandler) CustomerDecisionMemoryAddSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/customer/decision-memory", http.StatusSeeOther)
		return
	}

	rawName := strings.TrimSpace(r.FormValue("raw_name"))
	productID, err := strconv.ParseInt(strings.TrimSpace(r.FormValue("product_id")), 10, 64)
	if err != nil || productID <= 0 || rawName == "" {
		h.redirectWithNotice(w, r, "/customer/decision-memory", "error", i18n.T(lang, "decision_memory.item_and_drug_required"))
		return
	}

	reason := strings.TrimSpace(r.FormValue("reason"))
	if reason == "" {
		reason = i18n.T(lang, "decision_memory.customer_manual_reason")
	}

	if err := h.catSvc.SaveManualDecision(ctx, actor.OrganizationID, actor.UserID, rawName, productID, reason); err != nil {
		h.log.ErrorContext(ctx, "failed to add manual decision for customer", "error", err)
		h.redirectWithNotice(w, r, "/customer/decision-memory", "error", i18n.T(lang, "decision_memory.save_error"))
		return
	}

	h.redirectWithNotice(w, r, "/customer/decision-memory", "success", i18n.T(lang, "decision_memory.customer_saved_success"))
}

// CustomerDecisionMemoryDeleteSubmit deletes a single customer saved match decision.
func (h *UIHandler) CustomerDecisionMemoryDeleteSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/customer/decision-memory", http.StatusSeeOther)
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/customer/decision-memory", "error", i18n.T(lang, "admin.decision_memory.invalid_id"))
		return
	}

	if h.catSvc != nil && h.catSvc.Matching() != nil {
		sc := matching.Scope{
			EffectiveOrg: actor.OrganizationID,
			Workflow:     matching.WorkflowCatalog,
			EntryPoint:   "customer.decision_memory",
			ActorUserID:  actor.UserID,
		}
		_ = h.catSvc.Matching().RevokeKnowledge(ctx, sc, id)
	}

	if err := h.catSvc.DeleteMatchDecisionForOrg(ctx, actor.OrganizationID, id); err != nil {
		h.log.ErrorContext(ctx, "failed to delete match decision for customer", "id", id, "error", err)
		h.redirectWithNotice(w, r, "/customer/decision-memory", "error", i18n.T(lang, "admin.decision_memory.delete_error"))
		return
	}

	h.redirectWithNotice(w, r, "/customer/decision-memory", "success", i18n.T(lang, "admin.decision_memory.delete_success"))
}

// CustomerDecisionMemoryClearSubmit clears all match decisions from the customer organization's memory.
func (h *UIHandler) CustomerDecisionMemoryClearSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/customer/decision-memory", http.StatusSeeOther)
		return
	}

	if err := h.catSvc.ClearMatchDecisionsForOrg(ctx, actor.OrganizationID); err != nil {
		h.log.ErrorContext(ctx, "failed to clear match decisions for customer", "error", err)
		h.redirectWithNotice(w, r, "/customer/decision-memory", "error", i18n.T(lang, "admin.decision_memory.clear_error"))
		return
	}

	h.redirectWithNotice(w, r, "/customer/decision-memory", "success", i18n.T(lang, "decision_memory.org_cleared_success"))
}

func (h *UIHandler) loadOrgKnowledgeDecisions(ctx context.Context, orgID int64, search string, limit, offset int) ([]*catalog.MatchDecisionView, int) {
	if h.catSvc != nil && h.catSvc.Matching() != nil {
		sc := matching.Scope{
			EffectiveOrg: orgID,
			Workflow:     matching.WorkflowCatalog,
			EntryPoint:   "org.decision_memory",
		}
		kFilter := matching.KnowledgeFilter{
			Search: search,
			Limit:  limit,
			Offset: offset,
		}
		pageRes, err := h.catSvc.Matching().ListKnowledge(ctx, sc, kFilter)
		if err != nil {
			h.log.ErrorContext(ctx, "failed to list match knowledge for org", "org_id", orgID, "error", err)
		} else if len(pageRes.Items) > 0 {
			decisions := make([]*catalog.MatchDecisionView, 0, len(pageRes.Items))
			for _, item := range pageRes.Items {
				decisions = append(decisions, h.knowledgeToDecisionView(ctx, item))
			}
			return decisions, pageRes.Total
		}
	}

	decisions, total, err := h.catSvc.ListMatchDecisionsForOrgWithPlatform(ctx, orgID, search, limit, offset)
	if err != nil {
		h.log.ErrorContext(ctx, "failed to list match decisions for org", "org_id", orgID, "error", err)
	}
	return decisions, total
}

func (h *UIHandler) knowledgeToDecisionView(ctx context.Context, item matching.Knowledge) *catalog.MatchDecisionView {
	d := &catalog.MatchDecisionView{
		ID:              item.ID,
		OrganizationID:  item.OrganizationID,
		NormName:        item.DisplayText,
		ChosenProductID: item.TargetProductID,
		Reason:          item.Status,
		Source:          item.Kind,
		Scope:           "org",
		CreatedAt:       item.CreatedAt,
	}
	if item.OrganizationID == nil {
		d.Scope = "platform"
	}
	if item.LastUsedAt != nil {
		d.LastUsedAt = *item.LastUsedAt
	} else {
		d.LastUsedAt = item.CreatedAt
	}
	if item.PromptVersion != nil {
		d.PromptVersion = *item.PromptVersion
	}
	if item.Status == "active" {
		d.Confidence = 1.0
	} else {
		d.Confidence = 0.5
	}
	if item.TargetProductID != nil && *item.TargetProductID > 0 {
		if prod, _, pErr := h.catSvc.GetProduct(ctx, *item.TargetProductID); pErr == nil && prod != nil {
			d.ChosenProductName = prod.Name.Get("ar")
			if d.ChosenProductName == "" {
				d.ChosenProductName = prod.Name.Get("en")
			}
			d.ChosenProductSKU = prod.SKU
		}
	}
	return d
}
