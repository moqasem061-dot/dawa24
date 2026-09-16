package main

import (
	"log/slog"

	"github.com/go-chi/chi/v5"

	marketingHTTP "github.com/muhiya/dawa24-store/internal/modules/marketing/http"
	promoPostgres "github.com/muhiya/dawa24-store/internal/modules/promo/postgres"
	"github.com/muhiya/dawa24-store/internal/platform/config"
	"github.com/muhiya/dawa24-store/internal/platform/database"
)

// mountMarketing serves the content digest for the social-media workflow when
// MARKETING_BRIDGE_TOKEN is set.
func mountMarketing(r chi.Router, cfg *config.Config, log *slog.Logger, db *database.DB) {
	if cfg.Marketing.BridgeToken == "" {
		log.Info("marketing bridge disabled (MARKETING_BRIDGE_TOKEN not set)")
		return
	}
	marketingHTTP.NewBridge(promoPostgres.NewRepository(db), cfg.BaseURL, cfg.Marketing.BridgeToken, log).RegisterRoutes(r)
	log.Info("marketing bridge mounted", "prefix", marketingHTTP.Prefix)
}
