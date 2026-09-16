package http

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/assistant"
	"github.com/muhiya/dawa24-store/internal/modules/assistant/stream"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
)

// maxQuestionBytes bounds one question; the limit is shared with every other
// interface to the assistant.
const maxQuestionBytes = assistant.MaxQuestionBytes

// heartbeat is how often an idle stream sends a comment frame.
//
// Fifteen seconds, because a reverse proxy that closes idle connections
// typically does so at thirty or sixty, and because a silent stream is
// indistinguishable from a dead one to the browser.
const heartbeat = 15 * time.Second

type createTurnRequest struct {
	Text           string   `json:"text"`
	ConversationID int64    `json:"conversation_id"`
	Attachments    []string `json:"attachments"`
}

// CreateTurn accepts a question and starts answering it server-side.
//
// It returns as soon as the turn exists. The answer is produced on a context
// detached from this request, so the work — and the money already committed to
// it — survives the browser closing, navigating, or being throttled in a
// background tab.
func (h *Handler) CreateTurn(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor := authctx.FromContext(ctx)
	cfg, _ := assistant.Allowed(actor)

	if !h.limiter.Allow(actor.UserID) {
		writeFailure(w, http.StatusTooManyRequests, assistant.Fail(assistant.CodeRateLimited))
		return
	}

	var req createTurnRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxQuestionBytes*4)).Decode(&req); err != nil {
		writeFailure(w, http.StatusBadRequest, assistant.Fail(assistant.CodeInvalidRequest))
		return
	}
	req.Text = strings.TrimSpace(req.Text)
	if len(req.Text) > maxQuestionBytes {
		req.Text = req.Text[:maxQuestionBytes]
	}
	if req.Text == "" && len(req.Attachments) == 0 {
		writeFailure(w, http.StatusBadRequest, assistant.Fail(assistant.CodeInvalidRequest))
		return
	}

	// Resolve the conversation. A supplied id is only accepted when it belongs
	// to this caller AND was created by the agent they are using now.
	conv, failure := h.svc.OpenConversation(ctx, actor, cfg, req.ConversationID, req.Text)
	if failure != nil {
		writeFailure(w, http.StatusNotFound, *failure)
		return
	}

	atts, digests, parts := h.resolveAttachments(ctx, actor, req.Attachments)

	turn, err := h.svc.BeginTurn(ctx, actor, cfg, conv, req.Text)
	if err != nil {
		h.log.ErrorContext(ctx, "assistant: create turn", "error", err)
		writeFailure(w, http.StatusInternalServerError, assistant.Fail(assistant.CodeInternal))
		return
	}

	turnID := turn.PublicID.String()

	// context.WithoutCancel keeps the request's values — the actor and the
	// tenant GUC that row-level security reads — while dropping its
	// cancellation. That combination is the whole trick: the turn stays fully
	// scoped to the caller but stops caring whether they are still watching.
	workCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	h.track(turnID, cancel)

	emitter := newEmitter(h.buffer, turnID, h.log)
	go func() {
		defer cancel()
		defer h.untrack(turnID)
		defer emitter.close()
		defer func() {
			if r := recover(); r != nil {
				h.log.ErrorContext(workCtx, "assistant run turn panicked", "turn_id", turnID, "panic", r)
			}
		}()
		h.svc.RunTurn(workCtx, actor, cfg, turn, assistant.TurnInput{
			Text:        req.Text,
			Attachments: atts,
			Digests:     digests,
			Parts:       parts,
		}, emitter)
	}()

	writeJSON(w, http.StatusAccepted, map[string]any{
		"turn_id":         turnID,
		"conversation_id": conv.ID,
		"stream_url":      "/api/v1/assistant/turns/" + turnID + "/stream",
		"expires_at":      conv.ExpiresAt,
	})
}

// StreamTurn replays and then tails a turn's events.
//
// The client reconnects with Last-Event-ID (EventSource does this natively),
// and everything after that sequence number is replayed before live events
// resume. A reader that missed thirty seconds of an answer gets those thirty
// seconds, not a gap.
func (h *Handler) StreamTurn(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor := authctx.FromContext(ctx)
	turnID := chi.URLParam(r, "id")

	turn, err := h.repo.GetTurn(ctx, turnID, actor.OrgID, actor.UserID)
	if err != nil || turn == nil {
		writeFailure(w, http.StatusNotFound, assistant.Fail(assistant.CodeNotFound))
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeFailure(w, http.StatusInternalServerError, assistant.Fail(assistant.CodeInternal))
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	// nginx buffers proxied responses by default, which turns a token stream
	// into one delivery at the end.
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	after := lastEventID(r)

	// Two clocks. The buffer read blocks for up to `heartbeat`, so an idle turn
	// costs one comment frame every fifteen seconds and nothing else — the loop
	// cannot spin, whichever backend is behind it.
	//
	// The turn's status is re-read from the database rather than trusted from
	// the row fetched above, because that row is a snapshot from before the
	// answer was written. Trusting it meant a finished turn whose buffer had
	// already been drained kept the connection open, sending keep-alives to a
	// client waiting for an ending that had already happened.
	const statusEvery = 5 * time.Second
	deadline := time.Now().Add(10 * time.Minute)
	lastCheck := time.Now()

	for {
		if time.Now().After(deadline) {
			writeSSE(w, flusher, 0, "error",
				map[string]any{"code": string(assistant.CodeStreamInterrupted)})
			return
		}

		chunks, err := h.buffer.Read(ctx, turnID, after, heartbeat)
		if err != nil {
			return // the client went away
		}

		if len(chunks) > 0 {
			for _, c := range chunks {
				after = c.Seq
				writeSSE(w, flusher, c.Seq, c.Kind, chunkPayload(c))
				if c.Terminal() {
					return
				}
			}
			continue
		}

		// Nothing new. Ask the database whether there is anything still coming
		// before spending another fifteen seconds waiting for it.
		if time.Since(lastCheck) >= statusEvery {
			lastCheck = time.Now()
			fresh, ferr := h.repo.GetTurn(ctx, turnID, actor.OrgID, actor.UserID)
			if ferr == nil && fresh != nil && fresh.Status != assistant.TurnRunning {
				h.replayFinished(w, flusher, fresh, after)
				return
			}
		}

		// Comment frame: keeps proxies and the browser from deciding the
		// connection is dead while the model is thinking.
		fmt.Fprint(w, ": keep-alive\n\n")
		flusher.Flush()
	}
}

// replayFinished ends a stream from the persisted turn.
//
// It is reached when the answer is complete but the live chunks are gone —
// either the buffer expired, or this reader attached after the fact. `seen` is
// how much the reader already received: if it streamed the answer normally it
// gets only the terminal frame, and never the answer twice.
func (h *Handler) replayFinished(
	w http.ResponseWriter, flusher http.Flusher, turn *assistant.Turn, seen int64,
) {
	if turn.Status == assistant.TurnFailed && turn.Answer == "" {
		f := assistant.Fail(assistant.Code(turn.ErrorCode))
		writeSSE(w, flusher, seen+1, "error", map[string]any{
			"code":      string(f.Code),
			"message":   f.Message,
			"retryable": f.Retryable,
		})
		return
	}
	if seen == 0 && turn.Answer != "" {
		writeSSE(w, flusher, seen+1, "delta", map[string]any{"text": turn.Answer})
		seen++
	}
	// A reader that attached after the turn finished never saw the entities
	// frame, so it is replayed from the persisted turn before the ending.
	if len(turn.Entities) > 0 {
		writeSSE(w, flusher, seen+1, "entities", map[string]any{"entities": turn.Entities})
		seen++
	}
	writeSSE(w, flusher, seen+1, "done", map[string]any{
		"conversation_id": turn.ConversationID,
		"input_tokens":    turn.InputTokens,
		"output_tokens":   turn.OutputTokens,
	})
}

// CancelTurn stops an answer the user no longer wants.
func (h *Handler) CancelTurn(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor := authctx.FromContext(ctx)
	turnID := chi.URLParam(r, "id")

	turn, err := h.repo.GetTurn(ctx, turnID, actor.OrgID, actor.UserID)
	if err != nil || turn == nil {
		writeFailure(w, http.StatusNotFound, assistant.Fail(assistant.CodeNotFound))
		return
	}
	stopped := h.cancel(turnID)
	writeJSON(w, http.StatusOK, map[string]any{"cancelled": stopped})
}

// lastEventID reads the resume point, from the header the browser sends
// automatically or from an explicit query parameter for non-EventSource
// clients.
func lastEventID(r *http.Request) int64 {
	raw := r.Header.Get("Last-Event-ID")
	if raw == "" {
		raw = r.URL.Query().Get("from")
	}
	n, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || n < 0 {
		return 0
	}
	return n
}

func chunkPayload(c stream.Chunk) map[string]any {
	payload := map[string]any{}
	for k, v := range c.Data {
		payload[k] = v
	}
	if c.Text != "" {
		payload["text"] = c.Text
	}
	return payload
}

// writeSSE emits one frame.
//
// The id line is what makes resumption work: the browser stores it and returns
// it as Last-Event-ID on reconnect.
func writeSSE(w http.ResponseWriter, flusher http.Flusher, seq int64, event string, data any) {
	body, err := json.Marshal(data)
	if err != nil {
		return
	}
	if seq > 0 {
		fmt.Fprintf(w, "id: %d\n", seq)
	}
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, body)
	flusher.Flush()
}

// TurnStatus is the answer without the stream.
//
// It exists because Server-Sent Events are the one part of this that depends on
// something outside the application behaving: a reverse proxy that buffers, a
// corporate middlebox, a mobile network that drops a long-lived connection. When
// that happens the turn still runs, still costs money, and still lands in the
// database — and before this endpoint existed the reader had no way to collect
// it except by reopening the conversation from history, which is exactly what
// people reported doing.
//
// The client polls this whenever the stream goes quiet. It is cheap: one
// indexed row by public id, scoped to its owner.
func (h *Handler) TurnStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor := authctx.FromContext(ctx)

	turn, err := h.repo.GetTurn(ctx, chi.URLParam(r, "id"), actor.OrgID, actor.UserID)
	if err != nil || turn == nil {
		writeFailure(w, http.StatusNotFound, assistant.Fail(assistant.CodeNotFound))
		return
	}

	h.svc.RefreshProposals(ctx, actor, turn.Entities)
	body := map[string]any{
		"status":          string(turn.Status),
		"answer":          turn.Answer,
		"conversation_id": turn.ConversationID,
		"input_tokens":    turn.InputTokens,
		"output_tokens":   turn.OutputTokens,
		"entities":        turn.Entities,
	}
	if turn.Status == assistant.TurnFailed {
		f := assistant.Fail(assistant.Code(turn.ErrorCode))
		body["error"] = map[string]any{
			"code": string(f.Code), "message": f.Message, "retryable": f.Retryable,
		}
	}
	writeJSON(w, http.StatusOK, body)
}
