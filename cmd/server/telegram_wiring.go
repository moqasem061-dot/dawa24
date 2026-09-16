package main

import (
	"log/slog"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/assistant/actions"
	"github.com/muhiya/dawa24-store/internal/modules/telegram"
	telegramHTTP "github.com/muhiya/dawa24-store/internal/modules/telegram/http"
	telegramPostgres "github.com/muhiya/dawa24-store/internal/modules/telegram/postgres"
	"github.com/muhiya/dawa24-store/internal/platform/config"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/platform/rbac"
)

// mountTelegram wires the bridge and returns its service, or nil when the
// integration is not configured.
func mountTelegram(
	r chi.Router,
	cfg *config.Config,
	log *slog.Logger,
	db *database.DB,
	resolver *rbac.Resolver,
	capsule *capsuleBridge,
) *telegram.Service {
	if !cfg.Telegram.Enabled() {
		log.Info("telegram bridge disabled (TELEGRAM_BOT_USERNAME / TELEGRAM_BRIDGE_TOKEN not set)")
		return nil
	}
	svc := telegram.NewService(telegramPostgres.New(db), resolver, capsule.forChannel(actions.ChannelTelegram), telegram.Config{
		BotUsername: cfg.Telegram.BotUsername,
		BaseURL:     cfg.BaseURL,
		BotToken:    cfg.Telegram.BotToken,
	}, log)
	telegramHTTP.NewBridge(svc, cfg.Telegram.BridgeToken, log).RegisterRoutes(r)
	log.Info("telegram bridge mounted", "bot", cfg.Telegram.BotUsername, "prefix", telegramHTTP.Prefix)
	return svc
}
