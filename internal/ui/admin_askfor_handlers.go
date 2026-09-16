package ui

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/workflow"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// AdminAskForPage redirects to the unified approvals & requested documents registry.
func (h *UIHandler) AdminAskForPage(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/admin/approvals?tab=requests", http.StatusSeeOther)
}

// AdminAskForDetailPage renders details of an action/document request.
func (h *UIHandler) AdminAskForDetailPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	idStr := chi.URLParam(r, "id")
	reqID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || reqID <= 0 {
		http.Redirect(w, r, "/admin/ask-for", http.StatusSeeOther)
		return
	}

	var req *workflow.Request
	if h.wfSvc != nil {
		reqs, _ := h.wfSvc.ListInbox(database.AsSystem(ctx), 0, "", 100, 0)
		for _, rCand := range reqs {
			if rCand.ID == reqID {
				req = rCand
				break
			}
		}
	}

	if req == nil {
		http.Redirect(w, r, "/admin/ask-for", http.StatusSeeOther)
		return
	}

	h.renderPage(ctx, w, "render admin ask for detail", pages.AdminAskForDetailPage(req, lang, dir))
}

// AdminAskForRespondSubmit handles admin accepting/declining/cancelling request.
func (h *UIHandler) AdminAskForRespondSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	idStr := chi.URLParam(r, "id")
	reqID, _ := strconv.ParseInt(idStr, 10, 64)
	statusStr := r.PostFormValue("status")

	if h.wfSvc != nil && reqID > 0 {
		_ = h.wfSvc.RespondRequest(database.AsSystem(ctx), reqID, workflow.RequestStatus(statusStr))
	}

	http.Redirect(w, r, "/admin/ask-for", http.StatusSeeOther)
}
