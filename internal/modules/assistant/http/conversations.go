package http

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/assistant"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/gateway"
)

// ListConversations returns this user's own recent threads.
func (h *Handler) ListConversations(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor := authctx.FromContext(ctx)

	convs, err := h.repo.ListConversations(ctx, actor.OrgID, actor.UserID, 30, 0)
	if err != nil {
		h.log.ErrorContext(ctx, "assistant: list conversations", "error", err)
		writeFailure(w, http.StatusInternalServerError, assistant.Fail(assistant.CodeInternal))
		return
	}
	if convs == nil {
		convs = []*assistant.Conversation{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"conversations": convs})
}

// GetConversation returns one thread's messages, plus any answer still being
// written so a reopened drawer rejoins it instead of showing a dead thread.
func (h *Handler) GetConversation(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor := authctx.FromContext(ctx)
	cfg, _ := assistant.Allowed(actor)

	convID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || convID <= 0 {
		writeFailure(w, http.StatusBadRequest, assistant.Fail(assistant.CodeInvalidRequest))
		return
	}

	conv, err := h.repo.GetOwnedConversation(ctx, convID, actor.OrgID, actor.UserID, string(cfg.Role))
	if err != nil {
		h.log.ErrorContext(ctx, "assistant: get conversation", "error", err)
		writeFailure(w, http.StatusInternalServerError, assistant.Fail(assistant.CodeInternal))
		return
	}
	if conv == nil {
		writeFailure(w, http.StatusNotFound, assistant.Fail(assistant.CodeNotFound))
		return
	}

	msgs, err := h.repo.ListMessages(ctx, convID, 60)
	if err != nil {
		h.log.ErrorContext(ctx, "assistant: list messages", "error", err)
		writeFailure(w, http.StatusInternalServerError, assistant.Fail(assistant.CodeInternal))
		return
	}
	if msgs == nil {
		msgs = []*assistant.Message{}
	}
	for _, m := range msgs {
		h.svc.RefreshProposals(ctx, actor, m.Entities)
	}

	body := map[string]any{"conversation": conv, "messages": msgs}
	if turn, err := h.repo.LatestRunningTurn(ctx, convID, actor.UserID); err == nil && turn != nil {
		body["running_turn"] = map[string]any{
			"turn_id":    turn.PublicID.String(),
			"stream_url": "/api/v1/assistant/turns/" + turn.PublicID.String() + "/stream",
			"question":   turn.Question,
		}
	}
	writeJSON(w, http.StatusOK, body)
}

// DeleteConversation soft-deletes a thread belonging to the caller.
func (h *Handler) DeleteConversation(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor := authctx.FromContext(ctx)

	convID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || convID <= 0 {
		writeFailure(w, http.StatusBadRequest, assistant.Fail(assistant.CodeInvalidRequest))
		return
	}
	// The delete is itself scoped by organisation and user, so a foreign id
	// updates nothing.
	if err := h.repo.DeleteConversation(ctx, convID, actor.OrgID, actor.UserID); err != nil {
		h.log.ErrorContext(ctx, "assistant: delete conversation", "error", err)
		writeFailure(w, http.StatusInternalServerError, assistant.Fail(assistant.CodeInternal))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": true})
}

// Transcribe converts a voice note to text.
//
// Three things are different from what this used to do. The tenant's own
// virtual key authenticates the call, so the cost lands on their usage screen
// and inside their plan window rather than on the platform's. The model is
// discovered at runtime instead of being hardcoded to a name the Gateway ships
// disabled. And a Gateway with no active transcription model produces a clear
// sentence telling the user to type instead of a generic failure.
func (h *Handler) Transcribe(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor := authctx.FromContext(ctx)

	if !h.limiter.Allow(actor.UserID) {
		writeFailure(w, http.StatusTooManyRequests, assistant.Fail(assistant.CodeRateLimited))
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeFailure(w, http.StatusBadRequest, assistant.Fail(assistant.CodeInvalidRequest))
		return
	}
	defer file.Close()

	mime := header.Header.Get("Content-Type")
	model := ""
	if h.modelResolver != nil {
		model, err = h.modelResolver(ctx, mime)
		if err != nil {
			if errors.Is(err, gateway.ErrNoTranscriptionModel) {
				writeFailure(w, http.StatusServiceUnavailable,
					assistant.Fail(assistant.CodeTranscribeUnavail))
				return
			}
			h.log.WarnContext(ctx, "assistant: transcription model discovery failed", "error", err)
		}
	}

	var virtualKey string
	if h.keyResolver != nil && actor.OrgID > 0 {
		if vk, kerr := h.keyResolver(ctx, actor.OrgID); kerr == nil {
			virtualKey = vk
		}
	}

	text, err := h.gw.Transcribe(ctx, gateway.TranscribeRequest{
		Audio:      file,
		Filename:   header.Filename,
		MIMEType:   mime,
		Model:      model,
		OrgID:      actor.OrgID,
		UserID:     actor.UserID,
		VirtualKey: virtualKey,
		Feature:    "التفريغ الصوتي للمساعد",
	})
	if err != nil {
		h.log.WarnContext(ctx, "assistant: transcription failed", "error", err)
		code := assistant.CodeTranscribeFailed
		if errors.Is(err, gateway.ErrDisabled) {
			code = assistant.CodeTranscribeUnavail
		}
		writeFailure(w, http.StatusBadGateway, assistant.Fail(code))
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"text": text})
}
