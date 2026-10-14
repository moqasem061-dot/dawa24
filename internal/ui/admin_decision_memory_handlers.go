package ui

import (
	"fmt"
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

// AdminMatchDecisionsPage renders the central catalog decision memory and knowledge management page for administrators.
func (h *UIHandler) AdminMatchDecisionsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	limit := pagination.RowsPerPage(r)
	offset := (page - 1) * limit

	q := r.URL.Query()
	search := strings.TrimSpace(q.Get("q"))
	scope := strings.TrimSpace(q.Get("scope"))
	kind := strings.TrimSpace(q.Get("kind"))
	status := strings.TrimSpace(q.Get("status"))

	var decisions []*catalog.MatchDecisionView
	var total int

	if h.catSvc != nil && h.catSvc.Matching() != nil {
		sc := matching.Scope{
			Workflow:   matching.WorkflowCatalog,
			EntryPoint: "admin.match_decisions",
		}
		if actor, ok := authctx.From(ctx); ok {
			sc.ActorUserID = actor.UserID
		}
		kFilter := matching.KnowledgeFilter{
			Search: search,
			Scope:  scope,
			Kind:   kind,
			Status: status,
			Limit:  limit,
			Offset: offset,
		}
		pageRes, err := h.catSvc.Matching().ListKnowledge(ctx, sc, kFilter)
		if err != nil {
			h.log.ErrorContext(ctx, "failed to list match knowledge", "error", err)
		} else {
			total = pageRes.Total
			for _, item := range pageRes.Items {
				decisions = append(decisions, h.knowledgeToDecisionView(ctx, item))
			}
		}
	} else {
		filter := parseAdminDecisionFilter(r)
		filter.Limit = limit
		filter.Offset = offset
		var err error
		decisions, total, err = h.catSvc.ListMatchDecisionsFiltered(ctx, filter)
		if err != nil {
			h.log.ErrorContext(ctx, "failed to list match decisions", "error", err)
		}
	}

	isEnabled := h.catSvc.IsDecisionMemoryEnabled(ctx)

	data := pages.AdminMatchDecisionsData{
		Decisions:   decisions,
		Total:       total,
		Page:        page,
		PerPage:     limit,
		Search:      search,
		IsEnabled:   isEnabled,
		Scope:       scope,
		Source:      kind,
		QueryValues: r.URL.Query(),
	}

	h.renderPage(ctx, w, "render AdminMatchDecisionsPage", pages.AdminMatchDecisionsPage(lang, dir, data))
}

// AdminMatchDecisionPromoteSubmit promotes a decision/knowledge from org to platform scope.
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

	if h.catSvc != nil && h.catSvc.Matching() != nil {
		sc := matching.Scope{
			Workflow:    matching.WorkflowCatalog,
			EntryPoint:  "admin.match_decisions",
			ActorUserID: adminUserID,
		}
		if err := h.catSvc.Matching().PromoteKnowledge(ctx, sc, id); err != nil {
			h.log.ErrorContext(ctx, "failed to promote knowledge", "id", id, "error", err)
			h.redirectWithNotice(w, r, preserveDecisionReturnURL(r), "error", i18n.T(lang, "admin.decision_memory.promote_error"))
			return
		}
	} else {
		if err := h.catSvc.PromoteMatchDecision(ctx, id, adminUserID); err != nil {
			h.log.ErrorContext(ctx, "failed to promote decision", "id", id, "error", err)
			h.redirectWithNotice(w, r, preserveDecisionReturnURL(r), "error", i18n.T(lang, "admin.decision_memory.promote_error"))
			return
		}
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
	if h.catSvc != nil && h.catSvc.Matching() != nil {
		sc := matching.Scope{
			Workflow:    matching.WorkflowCatalog,
			EntryPoint:  "admin.match_decisions",
			ActorUserID: adminUserID,
		}
		switch action {
		case "promote":
			for _, id := range ids {
				if err := h.catSvc.Matching().PromoteKnowledge(ctx, sc, id); err != nil {
					h.renderError(w, r, err)
					return
				}
				affected++
			}
		case "delete", "revoke":
			for _, id := range ids {
				if err := h.catSvc.Matching().RevokeKnowledge(ctx, sc, id); err != nil {
					h.renderError(w, r, err)
					return
				}
				affected++
			}
		default:
			h.redirectWithNotice(w, r, preserveDecisionReturnURL(r), "error", i18n.T(lang, "admin.decision_memory.bulk_error"))
			return
		}
	} else {
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

// AdminMatchDecisionDeleteSubmit revokes/deletes a single match decision / knowledge.
func (h *UIHandler) AdminMatchDecisionDeleteSubmit(w http.ResponseWriter, r *http.Request) {
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

	if h.catSvc != nil && h.catSvc.Matching() != nil {
		sc := matching.Scope{
			Workflow:    matching.WorkflowCatalog,
			EntryPoint:  "admin.match_decisions",
			ActorUserID: adminUserID,
		}
		if err := h.catSvc.Matching().RevokeKnowledge(ctx, sc, id); err != nil {
			h.log.ErrorContext(ctx, "failed to revoke match knowledge", "id", id, "error", err)
			h.redirectWithNotice(w, r, preserveDecisionReturnURL(r), "error", i18n.T(lang, "admin.decision_memory.delete_error"))
			return
		}
	} else {
		if err := h.catSvc.DeleteMatchDecision(ctx, id); err != nil {
			h.log.ErrorContext(ctx, "failed to delete match decision", "id", id, "error", err)
			h.redirectWithNotice(w, r, preserveDecisionReturnURL(r), "error", i18n.T(lang, "admin.decision_memory.delete_error"))
			return
		}
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
