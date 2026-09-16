package http

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/identity"
	"github.com/muhiya/dawa24-store/internal/platform/config"
	"github.com/muhiya/dawa24-store/internal/platform/httpx"
	"github.com/muhiya/dawa24-store/internal/platform/rbac"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// Handler exposes identity HTTP endpoints.
type Handler struct {
	service          *identity.Service
	cookieName       string
	sessionTTL       time.Duration
	secureCookie     bool
	trustedProxyHops int
	resolver         *rbac.Resolver
	limiter          *httpx.Limiter
	log              *slog.Logger
}

// SetTrustedProxyHops configures the hop count for trusted reverse proxies.
func (h *Handler) SetTrustedProxyHops(n int) { h.trustedProxyHops = n }

// SetResolver supplies the permission resolver. It is optional so that tests
// which only exercise authentication need not stand up a database.
func (h *Handler) SetResolver(r *rbac.Resolver) { h.resolver = r }

// SetLimiter supplies an optional rate limiter for sensitive authentication endpoints.
func (h *Handler) SetLimiter(l *httpx.Limiter) { h.limiter = l }

// NewHandler creates an identity HTTP handler.
func NewHandler(service *identity.Service, cfg config.Session, log *slog.Logger) *Handler {
	return &Handler{
		service:      service,
		cookieName:   cfg.CookieName,
		sessionTTL:   cfg.TTL,
		secureCookie: cfg.SecureOnly,
		log:          log,
	}
}

// RegisterRoutes registers identity routes on a Chi router.
func (h *Handler) RegisterRoutes(r chi.Router) {
	if h.limiter != nil {
		r.With(h.limiter.LimitByIP(5, time.Minute)).Post("/api/v1/auth/register", h.Register)
		r.With(h.limiter.LimitByIP(10, time.Minute)).Post("/api/v1/auth/login", h.Login)
	} else {
		r.Post("/api/v1/auth/register", h.Register)
		r.Post("/api/v1/auth/login", h.Login)
	}
	r.Post("/api/v1/auth/logout", h.Logout)

	r.Group(func(protected chi.Router) {
		protected.Use(httpx.CSRF(h.secureCookie))
		protected.Use(RequireAuth(h.service, h.resolver, h.cookieName, h.log))
		protected.Get("/api/v1/auth/me", h.Me)
		protected.Get("/api/v1/me", h.GetMe)
		protected.Put("/api/v1/me", h.UpdateMe)

		protected.Get("/api/v1/me/addresses", h.ListAddresses)
		protected.Post("/api/v1/me/addresses", h.CreateAddress)
		protected.Put("/api/v1/me/addresses/{id}", h.UpdateAddress)
		protected.Delete("/api/v1/me/addresses/{id}", h.DeleteAddress)

		protected.Get("/api/v1/me/favorites", h.ListFavorites)
		protected.Post("/api/v1/me/favorites", h.AddFavorite)
		protected.Delete("/api/v1/me/favorites/{productId}", h.RemoveFavorite)
	})

	h.RegisterAdminRoutes(r)
}

// Register handles user registration.
func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	var input identity.RegisterInput
	if err := httpx.DecodeJSON(w, r, &input); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	user, sess, err := h.service.Register(r.Context(), input)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	if sess != nil {
		h.setSessionCookie(w, sess.Token)
	}

	httpx.JSON(w, http.StatusCreated, map[string]any{
		"user":    user,
		"session": sess,
	})
}

// Login handles user authentication.
func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var input identity.LoginInput
	if err := httpx.DecodeJSON(w, r, &input); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	input.IP = httpx.ClientIP(r, h.trustedProxyHops)
	input.UserAgent = r.UserAgent()

	result, err := h.service.Login(r.Context(), input)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	if result.Session != nil {
		h.setSessionCookie(w, result.Session.Token)
	}

	httpx.JSON(w, http.StatusOK, result)
}

// Logout clears the user session.
func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	token := extractToken(r, h.cookieName)
	if token != "" {
		_ = h.service.Logout(r.Context(), token)
	}

	h.clearSessionCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

// Me returns the active user session.
func (h *Handler) Me(w http.ResponseWriter, r *http.Request) {
	sess, ok := SessionFrom(r.Context())
	if !ok {
		httpx.Error(w, r, h.log, apperr.Unauthorized())
		return
	}

	httpx.JSON(w, http.StatusOK, map[string]any{
		"session": sess,
	})
}

func (h *Handler) setSessionCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     h.cookieName,
		Value:    token,
		Path:     "/",
		Expires:  time.Now().Add(h.sessionTTL),
		MaxAge:   int(h.sessionTTL.Seconds()),
		HttpOnly: true,
		Secure:   h.secureCookie,
		SameSite: http.SameSiteLaxMode,
	})
}

func (h *Handler) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     h.cookieName,
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   h.secureCookie,
		SameSite: http.SameSiteLaxMode,
	})
}
