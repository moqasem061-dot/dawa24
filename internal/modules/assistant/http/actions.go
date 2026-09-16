package http

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/muhiya/dawa24-store/internal/modules/assistant"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
)

// Confirming a proposal from the drawer.
//
// These are ordinary authenticated POSTs under the same session, CSRF check
// and assistant gate as every other assistant route. The request carries only
// the proposal's public id; the arguments, the preview and the authority to
// execute are all re-derived on the server from the live session.

// ConfirmAction executes a pending proposal.
func (h *Handler) ConfirmAction(w http.ResponseWriter, r *http.Request) {
	h.decideAction(w, r, h.svc.ConfirmAction)
}

// CancelAction withdraws a pending proposal.
func (h *Handler) CancelAction(w http.ResponseWriter, r *http.Request) {
	h.decideAction(w, r, h.svc.CancelAction)
}

func (h *Handler) decideAction(
	w http.ResponseWriter, r *http.Request,
	decide func(ctx context.Context, actor authctx.Actor, id uuid.UUID) assistant.ActionResult,
) {
	actor := authctx.FromContext(r.Context())
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeFailure(w, http.StatusNotFound, assistant.Fail(assistant.CodeNotFound))
		return
	}
	res := decide(r.Context(), actor, id)
	writeJSON(w, actionStatus(res.Outcome), map[string]any{
		"outcome":  res.Outcome,
		"message":  res.Message,
		"proposal": res.Card,
	})
}

func actionStatus(o assistant.ActionOutcome) int {
	switch o {
	case assistant.ActionExecuted, assistant.ActionCancelled:
		return http.StatusOK
	case assistant.ActionNotFound:
		return http.StatusNotFound
	case assistant.ActionForbidden:
		return http.StatusForbidden
	case assistant.ActionExpired:
		return http.StatusGone
	case assistant.ActionStale, assistant.ActionRefused:
		return http.StatusConflict
	}
	return http.StatusUnprocessableEntity
}
