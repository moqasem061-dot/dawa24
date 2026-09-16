// Package http serves the endpoints n8n calls for Telegram. Authentication is
// the shared chat-bridge secret check (chatbridge/http).
package http

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	chatHTTP "github.com/muhiya/dawa24-store/internal/modules/chatbridge/http"
	"github.com/muhiya/dawa24-store/internal/modules/telegram"
)

// Prefix is where the bridge lives. It is registered as long-running in
// httpx, because an update carrying a question waits for the full answer.
const Prefix = "/api/v1/integrations/telegram"

const (
	maxUpdateBytes = 1 << 20
	maxReportBytes = 256 << 10
	maxReportItems = 200
)

// Bridge serves n8n.
type Bridge struct {
	svc  *telegram.Service
	auth chatHTTP.Authenticator
	log  *slog.Logger
}

// NewBridge constructs the bridge. token must be the configured shared secret;
// an empty token yields a bridge that refuses every call.
func NewBridge(svc *telegram.Service, token string, log *slog.Logger) *Bridge {
	if log == nil {
		log = slog.Default()
	}
	log = log.With("handler", "telegram_bridge")
	return &Bridge{svc: svc, auth: chatHTTP.NewAuthenticator(token, log), log: log}
}

// RegisterRoutes mounts the bridge. Mount it OUTSIDE the session, CSRF and
// tenant middleware: none of them apply to n8n, and the CSRF check would
// refuse every call.
func (b *Bridge) RegisterRoutes(r chi.Router) {
	r.Route(Prefix, func(g chi.Router) {
		g.Use(b.auth.Middleware)
		g.Post("/updates", b.Updates)
		g.Post("/outbox/claim", b.Claim)
		g.Post("/outbox/report", b.Report)
		g.Get("/exports/{token}", b.Export)
	})
}

// Export serves a spreadsheet Capsule produced, for n8n to send as a document.
func (b *Bridge) Export(w http.ResponseWriter, r *http.Request) {
	file, err := b.svc.Export(r.Context(), chi.URLParam(r, "token"))
	if err != nil {
		b.log.ErrorContext(r.Context(), "telegram bridge: export", "error", err)
		chatHTTP.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	chatHTTP.ServeExport(w, file)
}

// Updates receives one raw Telegram update and returns the messages to send.
func (b *Bridge) Updates(w http.ResponseWriter, r *http.Request) {
	var u telegram.Update
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxUpdateBytes)).Decode(&u); err != nil || u.UpdateID <= 0 {
		chatHTTP.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_update"})
		return
	}
	reply, err := b.svc.HandleUpdate(r.Context(), u)
	if err != nil {
		b.log.ErrorContext(r.Context(), "telegram bridge: handle update", "error", err, "update_id", u.UpdateID)
		chatHTTP.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	if reply.Messages == nil {
		reply.Messages = []telegram.OutMessage{}
	}
	chatHTTP.WriteJSON(w, http.StatusOK, reply)
}

type claimRequest struct {
	Limit int `json:"limit"`
}

// Claim leases notification messages for n8n to send.
func (b *Bridge) Claim(w http.ResponseWriter, r *http.Request) {
	var req claimRequest
	if r.ContentLength != 0 {
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
			chatHTTP.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_request"})
			return
		}
	}
	out, err := b.svc.ClaimOutbox(r.Context(), req.Limit)
	if err != nil {
		b.log.ErrorContext(r.Context(), "telegram bridge: claim outbox", "error", err)
		chatHTTP.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	if out == nil {
		out = []telegram.Outgoing{}
	}
	chatHTTP.WriteJSON(w, http.StatusOK, map[string]any{"deliveries": out})
}

type reportRequest struct {
	Results []telegram.DeliveryResult `json:"results"`
}

// Report records the outcome of leased messages.
func (b *Bridge) Report(w http.ResponseWriter, r *http.Request) {
	var req reportRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxReportBytes)).Decode(&req); err != nil || len(req.Results) > maxReportItems {
		chatHTTP.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_request"})
		return
	}
	if err := b.svc.ReportDeliveries(r.Context(), req.Results); err != nil {
		b.log.ErrorContext(r.Context(), "telegram bridge: report deliveries", "error", err)
		chatHTTP.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	chatHTTP.WriteJSON(w, http.StatusOK, map[string]int{"recorded": len(req.Results)})
}
