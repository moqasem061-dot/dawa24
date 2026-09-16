// Package http serves the content digest the social-media workflow in n8n
// reads before it writes a post.
//
// Like the Telegram bridge it is machine-to-machine: no session, no CSRF, one
// shared secret presented as a Bearer token and compared in constant time
// against its hash. It is read-only and returns only what buyers can already
// see on the public offers board.
package http

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/promo"
)

// Prefix is where the bridge lives.
const Prefix = "/api/v1/integrations/marketing"

const (
	defaultWindowHours = 24
	maxWindowHours     = 72
	maxOffers          = 10
)

// OfferSource lists offers that became visible to buyers recently.
type OfferSource interface {
	ListPublishedOffers(ctx context.Context, since time.Time, limit int) ([]*promo.PublishedOffer, error)
}

// Bridge serves n8n.
type Bridge struct {
	offers    OfferSource
	baseURL   string
	tokenHash [32]byte
	log       *slog.Logger
}

// NewBridge constructs the bridge. An empty token yields a bridge that refuses
// every call.
func NewBridge(offers OfferSource, baseURL, token string, log *slog.Logger) *Bridge {
	b := &Bridge{offers: offers, baseURL: strings.TrimRight(baseURL, "/"), log: log.With("handler", "marketing_bridge")}
	if token != "" {
		b.tokenHash = sha256.Sum256([]byte(token))
	}
	return b
}

// RegisterRoutes mounts the bridge outside the session and CSRF middleware.
func (b *Bridge) RegisterRoutes(r chi.Router) {
	r.Route(Prefix, func(g chi.Router) {
		g.Use(b.authenticate)
		g.Get("/digest", b.Digest)
	})
}

// digest is the response: the facts a post may state, and the links and image
// it may use. Posts must not state anything that is not here.
type digest struct {
	GeneratedAt time.Time               `json:"generated_at"`
	WindowHours int                     `json:"window_hours"`
	NewOffers   []*promo.PublishedOffer `json:"new_offers"`
	SiteURL     string                  `json:"site_url"`
	OffersURL   string                  `json:"offers_url"`
	ImageURL    string                  `json:"image_url"`
}

// Digest returns offers published within the last `hours` (default 24, at most
// 72) that are still live now.
func (b *Bridge) Digest(w http.ResponseWriter, r *http.Request) {
	hours := defaultWindowHours
	if v := r.URL.Query().Get("hours"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > maxWindowHours {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "hours must be between 1 and 72"})
			return
		}
		hours = n
	}
	now := time.Now().UTC()
	offers, err := b.offers.ListPublishedOffers(r.Context(), now.Add(-time.Duration(hours)*time.Hour), maxOffers)
	if err != nil {
		b.log.ErrorContext(r.Context(), "marketing bridge: list published offers", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	if offers == nil {
		offers = []*promo.PublishedOffer{}
	}
	writeJSON(w, http.StatusOK, digest{
		GeneratedAt: now,
		WindowHours: hours,
		NewOffers:   offers,
		SiteURL:     b.baseURL,
		OffersURL:   b.baseURL + "/offers",
		ImageURL:    b.baseURL + "/static/img/doctor-capsule-social.jpg",
	})
}

func (b *Bridge) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !b.authorized(r.Header.Get("Authorization")) {
			b.log.WarnContext(r.Context(), "marketing bridge: refused unauthenticated call",
				"path", r.URL.Path, "remote", r.RemoteAddr)
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (b *Bridge) authorized(header string) bool {
	var zero [32]byte
	if b.tokenHash == zero {
		return false
	}
	scheme, token, ok := strings.Cut(strings.TrimSpace(header), " ")
	if !ok || !strings.EqualFold(scheme, "bearer") {
		return false
	}
	got := sha256.Sum256([]byte(strings.TrimSpace(token)))
	return subtle.ConstantTimeCompare(got[:], b.tokenHash[:]) == 1
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
