package main

import (
	"log/slog"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/assistant/actions"
	"github.com/muhiya/dawa24-store/internal/modules/whatsapp"
	whatsappHTTP "github.com/muhiya/dawa24-store/internal/modules/whatsapp/http"
	whatsappPostgres "github.com/muhiya/dawa24-store/internal/modules/whatsapp/postgres"
	"github.com/muhiya/dawa24-store/internal/platform/config"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/platform/rbac"
	"net/http"

	"github.com/muhiya/dawa24-store/internal/platform/httpx"
)

// mountWhatsApp wires the WhatsApp bridge and returns its service, or nil when
// the integration is not configured.
func mountWhatsApp(
	r chi.Router,
	cfg *config.Config,
	log *slog.Logger,
	db *database.DB,
	resolver *rbac.Resolver,
	capsule *capsuleBridge,
) *whatsapp.Service {
	if !cfg.WhatsApp.Enabled() {
		log.Info("whatsapp bridge disabled (WHATSAPP_BUSINESS_NUMBER / WHATSAPP_BRIDGE_TOKEN not set)")
		r.Route(whatsappHTTP.Prefix, func(g chi.Router) {
			disabled := func(w http.ResponseWriter, r *http.Request) {
				httpx.JSON(w, http.StatusServiceUnavailable, map[string]string{
					"error":   "whatsapp_integration_disabled",
					"message": "WhatsApp integration is disabled or not configured. Set WHATSAPP_BUSINESS_NUMBER and WHATSAPP_BRIDGE_TOKEN in server environment variables to enable it.",
				})
			}
			g.HandleFunc("/", disabled)
			g.HandleFunc("/*", disabled)
		})
		return nil
	}
	svc := whatsapp.NewService(whatsappPostgres.New(db), resolver, capsule.forChannel(actions.ChannelWhatsApp), whatsapp.Config{
		BusinessNumber:       cfg.WhatsApp.BusinessNumber,
		BaseURL:              cfg.BaseURL,
		NotificationTemplate: cfg.WhatsApp.NotificationTemplate,
		TemplateLanguage:     cfg.WhatsApp.TemplateLanguage,
	}, log)
	whatsappHTTP.NewBridge(svc, cfg.WhatsApp.BridgeToken, log).RegisterRoutes(r)
	log.Info("whatsapp bridge mounted", "prefix", whatsappHTTP.Prefix,
		"notification_template", cfg.WhatsApp.NotificationTemplate != "")
	return svc
}
