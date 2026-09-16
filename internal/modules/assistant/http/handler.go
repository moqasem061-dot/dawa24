package http

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/assistant"
	"github.com/muhiya/dawa24-store/internal/modules/assistant/stream"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/gateway"
	"github.com/muhiya/dawa24-store/internal/platform/storage"
)

// The assistant's HTTP surface.
//
// It is transport only: authenticate, gate, hand to the service, stream the
// result. Nothing here decides what data anybody may see; that lives entirely
// in assistant/tools and assistant/postgres, so a mistake in a handler cannot
// widen access.
//
// The route shape is deliberate. Asking a question and reading the answer are
// two different requests:
//
//	POST   /turns                → 202 {turn_id}; the work starts server-side
//	GET    /turns/{id}/stream    → SSE, resumable with Last-Event-ID
//	DELETE /turns/{id}           → stop generating
//
// That split is what makes an answer survive a dropped connection. The old
// design ran the whole turn inside the streaming POST, so closing the tab both
// discarded the answer and kept billing for it.

// Handler serves the assistant API.
type Handler struct {
	svc     *assistant.Service
	repo    assistant.Repository
	gw      gateway.Client
	buffer  stream.Buffer
	storage *storage.Client
	limiter *RateLimiter
	log     *slog.Logger

	exports       ExportReader
	keyResolver   assistant.KeyResolver
	modelResolver gateway.TranscriptionModelResolver

	// running lets a client stop an answer it no longer wants.
	//
	// Process-local on purpose. A turn runs on the replica that accepted it, so
	// only that replica can cancel it; a cancel that lands elsewhere is a no-op
	// and the turn finishes normally, which is a wasted completion rather than
	// a correctness problem. Coordinating cancellation across replicas would
	// cost a pub/sub channel per turn to save a few seconds of generation.
	runningMu sync.Mutex
	running   map[string]context.CancelFunc
}

// NewHandler constructs the assistant HTTP handler.
func NewHandler(
	svc *assistant.Service,
	repo assistant.Repository,
	gw gateway.Client,
	buffer stream.Buffer,
	log *slog.Logger,
) *Handler {
	if log == nil {
		log = slog.Default()
	}
	if buffer == nil {
		buffer = stream.NewMemoryBuffer()
	}
	return &Handler{
		svc:     svc,
		repo:    repo,
		gw:      gw,
		buffer:  buffer,
		limiter: NewRateLimiter(20, time.Minute),
		log:     log.With("handler", "assistant"),
		running: make(map[string]context.CancelFunc),
	}
}

// SetStorage installs the object store attachments are written to.
func (h *Handler) SetStorage(c *storage.Client) { h.storage = c }

// SetKeyResolver installs the tenant virtual-key lookup.
func (h *Handler) SetKeyResolver(k assistant.KeyResolver) { h.keyResolver = k }

// SetTranscriptionModelResolver installs runtime transcription-model discovery.
func (h *Handler) SetTranscriptionModelResolver(r gateway.TranscriptionModelResolver) {
	h.modelResolver = r
}

// AllowQuestion applies the per-user question limit. Exposed so every
// interface to the assistant spends from the same budget.
func (h *Handler) AllowQuestion(userID int64) bool { return h.limiter.Allow(userID) }

// RegisterRoutes mounts the assistant endpoints.
//
// Every one of them sits behind requireAssistant, so the owner's per-role
// switch governs the whole feature and not just the chat box.
func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Group(func(g chi.Router) {
		g.Use(h.requireAssistant)

		g.Post("/api/v1/assistant/turns", h.CreateTurn)
		g.Get("/api/v1/assistant/turns/{id}/stream", h.StreamTurn)
		// The stream's safety net: when SSE is blocked or buffered by something
		// between us and the browser, the client polls this instead and the
		// answer still arrives.
		g.Get("/api/v1/assistant/turns/{id}", h.TurnStatus)
		g.Delete("/api/v1/assistant/turns/{id}", h.CancelTurn)

		g.Post("/api/v1/assistant/attachments", h.Upload)
		// Serving an upload back is what lets a reopened conversation still
		// show the photograph that was sent with the question, instead of a
		// paperclip where the picture used to be.
		g.Get("/api/v1/assistant/attachments/{ref}", h.Download)
		g.Post("/api/v1/assistant/transcribe", h.Transcribe)
		g.Get("/api/v1/assistant/exports/{token}", h.DownloadExport)
		g.Post("/api/v1/assistant/actions/{id}/confirm", h.ConfirmAction)
		g.Post("/api/v1/assistant/actions/{id}/cancel", h.CancelAction)

		g.Get("/api/v1/assistant/session", h.Session)
		g.Get("/api/v1/assistant/conversations", h.ListConversations)
		g.Get("/api/v1/assistant/conversations/{id}", h.GetConversation)
		g.Delete("/api/v1/assistant/conversations/{id}", h.DeleteConversation)

		g.Get("/api/v1/assistant/memories", h.ListMemories)
		g.Post("/api/v1/assistant/memories", h.CreateMemory)
		g.Delete("/api/v1/assistant/memories/{id}", h.DeleteMemory)
	})
}

// requireAssistant refuses callers who may not use the assistant.
//
// Two conditions: they belong to a dashboard, and they hold that dashboard's
// assistant permission. A pharmacy owner grants the permission to an employee
// role in the roles screen; owners hold their whole scope and always pass.
func (h *Handler) requireAssistant(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authctx.From(r.Context())
		if !ok || actor.UserID <= 0 {
			writeFailure(w, http.StatusUnauthorized, assistant.Fail(assistant.CodeForbidden))
			return
		}
		if _, allowed := assistant.Allowed(actor); !allowed {
			// 403 and not 404: the user can see the launcher in the shell, so
			// pretending the endpoint does not exist would just look broken.
			// The message names the fix — ask the owner.
			writeFailure(w, http.StatusForbidden, assistant.Fail(assistant.CodeForbidden))
			return
		}
		next.ServeHTTP(w, r)
	})
}

// Session tells the drawer what this user's assistant is and what it can do.
func (h *Handler) Session(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor := authctx.FromContext(ctx)
	cfg, _ := assistant.Allowed(actor)

	status := "connected"
	if h.gw == nil || !h.gw.Enabled() {
		status = "disabled"
	} else if err := h.gw.Health(ctx); err != nil {
		status = "degraded"
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status": status,
		"agent": map[string]any{
			"role":  string(cfg.Role),
			"label": cfg.Label,
		},
		"context_window": h.svc.ContextWindow(ctx),
		// The drawer states the retention rule in words; this is the number
		// behind it, so the two cannot drift.
		"retention_months": 6,
		"read_only":        true,
	})
}

// writeJSON sends a JSON body.
func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// writeFailure sends a user-facing failure.
//
// Every error the assistant returns goes through here, which is what keeps
// gateway messages, SQL and stack traces out of the browser. The previous
// handlers hand-wrote JSON in string literals, and five of them were malformed
// — the body read `{"error":"rate_limited","message":i18n.TDefault("…")}`, so
// the client's parse failed and the user was told nothing at all.
func writeFailure(w http.ResponseWriter, status int, f assistant.Failure) {
	writeJSON(w, status, f)
}

// track registers a cancel function for a running turn.
func (h *Handler) track(turnID string, cancel context.CancelFunc) {
	h.runningMu.Lock()
	h.running[turnID] = cancel
	h.runningMu.Unlock()
}

func (h *Handler) untrack(turnID string) {
	h.runningMu.Lock()
	delete(h.running, turnID)
	h.runningMu.Unlock()
}

func (h *Handler) cancel(turnID string) bool {
	h.runningMu.Lock()
	cancel, ok := h.running[turnID]
	delete(h.running, turnID)
	h.runningMu.Unlock()
	if ok {
		cancel()
	}
	return ok
}
