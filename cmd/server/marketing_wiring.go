package main

import (
	"log/slog"

	"github.com/go-chi/chi/v5"

	"net/http"

	marketingHTTP "github.com/muhiya/dawa24-store/internal/modules/marketing/http"
	promoPostgres "github.com/muhiya/dawa24-store/internal/modules/promo/postgres"
	"github.com/muhiya/dawa24-store/internal/platform/config"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/platform/httpx"
)

// mountMarketing serves the content digest for the social-media workflow when
// MARKETING_BRIDGE_TOKEN is set.
func mountMarketing(r chi.Router, cfg *config.Config, log *slog.Logger, db *database.DB) {
	if cfg.Marketing.BridgeToken == "" {
		log.Info("marketing bridge disabled (MARKETING_BRIDGE_TOKEN not set)")
		r.Route(marketingHTTP.Prefix, func(g chi.Router) {
			disabled := func(w http.ResponseWriter, r *http.Request) {
				httpx.JSON(w, http.StatusServiceUnavailable, map[string]string{
					"error":   "marketing_integration_disabled",
					"message": "Marketing integration is disabled or not configured. Set MARKETING_BRIDGE_TOKEN in server environment variables to enable it.",
				})
			}
			g.HandleFunc("/", disabled)
			g.HandleFunc("/*", disabled)
		})
		return
	}
	marketingHTTP.NewBridge(promoPostgres.NewRepository(db), cfg.BaseURL, cfg.Marketing.BridgeToken, log).RegisterRoutes(r)
	log.Info("marketing bridge mounted", "prefix", marketingHTTP.Prefix)
}
