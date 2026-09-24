package main

import (
	"context"
	"log/slog"
	"time"

	"github.com/go-chi/chi/v5"

	"net/http"

	marketingHTTP "github.com/moqasem061-dot/dawa24/internal/modules/marketing/http"
	"github.com/moqasem061-dot/dawa24/internal/modules/promo"
	promoPostgres "github.com/moqasem061-dot/dawa24/internal/modules/promo/postgres"
	"github.com/moqasem061-dot/dawa24/internal/platform/config"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
	"github.com/moqasem061-dot/dawa24/internal/platform/httpx"
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
	marketingHTTP.NewBridge(promoOfferSource{promoPostgres.NewRepository(db)}, cfg.BaseURL, cfg.Marketing.BridgeToken, log).RegisterRoutes(r)
	log.Info("marketing bridge mounted", "prefix", marketingHTTP.Prefix)
}

// promoOfferSource hands promo's published offers to the marketing bridge in
// the bridge's own type; the two modules never import each other.
type promoOfferSource struct {
	repo interface {
		ListPublishedOffers(ctx context.Context, since time.Time, limit int) ([]*promo.PublishedOffer, error)
	}
}

func (s promoOfferSource) ListPublishedOffers(ctx context.Context, since time.Time, limit int) ([]*marketingHTTP.Offer, error) {
	in, err := s.repo.ListPublishedOffers(ctx, since, limit)
	if err != nil {
		return nil, err
	}
	out := make([]*marketingHTTP.Offer, len(in))
	for i, o := range in {
		c := marketingHTTP.Offer(*o)
		out[i] = &c
	}
	return out, nil
}
