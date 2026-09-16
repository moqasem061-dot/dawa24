// Package http serves the endpoints n8n calls for WhatsApp. Authentication is
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
	"github.com/muhiya/dawa24-store/internal/modules/whatsapp"
)

// Prefix is where the bridge lives. It is registered as long-running in
// httpx, because a webhook carrying a question waits for the full answer.
const Prefix = "/api/v1/integrations/whatsapp"

const (
	maxWebhookBytes = 1 << 20
	maxReportBytes  = 256 << 10
	maxReportItems  = 200
)

// Bridge serves n8n.
type Bridge struct {
	svc  *whatsapp.Service
	auth chatHTTP.Authenticator
	log  *slog.Logger
}

// NewBridge constructs the bridge. token must be the configured shared secret;
// an empty token yields a bridge that refuses every call.
func NewBridge(svc *whatsapp.Service, token string, log *slog.Logger) *Bridge {
	if log == nil {
		log = slog.Default()
	}
	log = log.With("handler", "whatsapp_bridge")
	return &Bridge{svc: svc, auth: chatHTTP.NewAuthenticator(token, log), log: log}
}

// RegisterRoutes mounts the bridge. Mount it OUTSIDE the session, CSRF and
// tenant middleware: none of them apply to n8n.
func (b *Bridge) RegisterRoutes(r chi.Router) {
	r.Route(Prefix, func(g chi.Router) {
		g.Use(b.auth.Middleware)
		g.Post("/webhook", b.Webhook)
		g.Post("/outbox/claim", b.Claim)
		g.Post("/outbox/report", b.Report)
		g.Get("/exports/{token}", b.Export)
	})
}

// Webhook receives one WhatsApp webhook and returns the messages to send.
func (b *Bridge) Webhook(w http.ResponseWriter, r *http.Request) {
	var hook whatsapp.Webhook
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxWebhookBytes)).Decode(&hook); err != nil {
		chatHTTP.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_webhook"})
		return
	}
	reply, err := b.svc.HandleWebhook(r.Context(), hook)
	if err != nil {
		b.log.ErrorContext(r.Context(), "whatsapp bridge: handle webhook", "error", err)
		chatHTTP.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	chatHTTP.WriteJSON(w, http.StatusOK, reply)
}

// Export serves a spreadsheet Capsule produced, for n8n to send as a document.
func (b *Bridge) Export(w http.ResponseWriter, r *http.Request) {
	file, err := b.svc.Export(r.Context(), chi.URLParam(r, "token"))
	if err != nil {
		b.log.ErrorContext(r.Context(), "whatsapp bridge: export", "error", err)
		chatHTTP.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	chatHTTP.ServeExport(w, file)
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
		b.log.ErrorContext(r.Context(), "whatsapp bridge: claim outbox", "error", err)
		chatHTTP.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	chatHTTP.WriteJSON(w, http.StatusOK, map[string]any{"deliveries": out})
}

type reportRequest struct {
	Results []whatsapp.DeliveryResult `json:"results"`
}

// Report records the outcome of leased messages.
func (b *Bridge) Report(w http.ResponseWriter, r *http.Request) {
	var req reportRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxReportBytes)).Decode(&req); err != nil || len(req.Results) > maxReportItems {
		chatHTTP.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_request"})
		return
	}
	if err := b.svc.ReportDeliveries(r.Context(), req.Results); err != nil {
		b.log.ErrorContext(r.Context(), "whatsapp bridge: report deliveries", "error", err)
		chatHTTP.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	chatHTTP.WriteJSON(w, http.StatusOK, map[string]int{"recorded": len(req.Results)})
}
