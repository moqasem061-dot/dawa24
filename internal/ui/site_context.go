package ui

import (
	"context"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"net/http"
	"sync"

	"golang.org/x/sync/singleflight"
	"time"

	platformadmin "github.com/muhiya/dawa24-store/internal/modules/platform_admin"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/ui/layouts"
)

type siteSettingsKey struct{}

var (
	cachedSettings *platformadmin.SiteSettings
	cachedAt       time.Time
	cacheMu        sync.RWMutex
	// siteSettingsGroup collapses concurrent refreshes into one query. See
	// siteSettingsMiddleware.
	siteSettingsGroup singleflight.Group
)

// InvalidateSiteSettingsCache clears the cached settings so next request fetches fresh from DB.
func InvalidateSiteSettingsCache() {
	cacheMu.Lock()
	defer cacheMu.Unlock()
	cachedSettings = nil
}

// siteSettingsMiddleware injects live SiteSettings from database into every request context.
func (h *UIHandler) siteSettingsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		authNotice := r.URL.Query().Get("auth_notice")
		if c, err := r.Cookie("auth_flash"); err == nil && c != nil && c.Value != "" {
			if authNotice == "" {
				authNotice = c.Value
			}
			http.SetCookie(w, &http.Cookie{
				Name:     "auth_flash",
				Value:    "",
				Path:     "/",
				MaxAge:   -1,
				HttpOnly: false,
				SameSite: http.SameSiteLaxMode,
			})
		}
		if authNotice != "" {
			ctx = layouts.WithAuthNotice(ctx, authNotice)
		}

		if h.adminSvc != nil {
			if seoPage, err := h.adminSvc.GetSEOPageByRoute(database.AsSystem(ctx), r.URL.Path); err == nil && seoPage != nil {
				ctx = layouts.WithSEOPage(ctx, seoPage)
			}
		}

		cacheMu.RLock()
		if cachedSettings != nil && time.Since(cachedAt) < 10*time.Second {
			s := cachedSettings
			cacheMu.RUnlock()
			next.ServeHTTP(w, r.WithContext(layouts.WithPath(WithSiteSettings(ctx, s), r.URL.Path)))
			return
		}
		cacheMu.RUnlock()

		// One reader refreshes; the rest wait for its answer.
		//
		// Without this, every request in flight at the moment the ten-second
		// window expired saw the miss and queried independently. That is a
		// cache stampede, and it is worst exactly when it hurts most: under
		// load there are more concurrent requests to pile on, so the busier the
		// platform the more duplicate queries one expiry produced. singleflight
		// collapses them into a single query whose result they all receive.
		v, _, _ := siteSettingsGroup.Do("site-settings", func() (any, error) {
			// Re-check under the group: by the time a queued caller runs this,
			// the leader may already have refreshed the cache.
			cacheMu.RLock()
			if cachedSettings != nil && time.Since(cachedAt) < 10*time.Second {
				s := cachedSettings
				cacheMu.RUnlock()
				return s, nil
			}
			cacheMu.RUnlock()

			var s *platformadmin.SiteSettings
			if h.adminSvc != nil {
				// Deliberately NOT the request's context. A caller that
				// disconnects mid-flight would otherwise cancel the query every
				// other caller is waiting on, turning one abandoned request into
				// a failure for all of them.
				s, _ = h.adminSvc.GetSiteSettings(context.WithoutCancel(ctx))
			}
			if s == nil {
				s = DefaultSiteSettings()
			}

			cacheMu.Lock()
			cachedSettings = s
			cachedAt = time.Now()
			cacheMu.Unlock()
			return s, nil
		})

		s, _ := v.(*platformadmin.SiteSettings)
		if s == nil {
			s = DefaultSiteSettings()
		}

		if s != nil && s.SessionIdleTimeoutMinutes > 0 && h.idSvc != nil {
			h.idSvc.SetIdleTimeout(time.Duration(s.SessionIdleTimeoutMinutes) * time.Minute)
		}

		next.ServeHTTP(w, r.WithContext(layouts.WithPath(layouts.WithSiteSettings(ctx, s), r.URL.Path)))
	})
}

// WithSiteSettings embeds SiteSettings into context.
func WithSiteSettings(ctx context.Context, s *platformadmin.SiteSettings) context.Context {
	return layouts.WithSiteSettings(ctx, s)
}

// GetSiteSettings retrieves SiteSettings from context or returns default.
func GetSiteSettings(ctx context.Context) *platformadmin.SiteSettings {
	return layouts.GetSiteSettings(ctx)
}

// DefaultSiteSettings returns fallback settings if DB is uninitialized.
func DefaultSiteSettings() *platformadmin.SiteSettings {
	return &platformadmin.SiteSettings{
		SiteName:                  i18n.TDefault("w4_ui.24_28"),
		SiteDescription:           i18n.TDefault("w4_ui.s_99_99"),
		LogoURL:                   "/static/img/logo.png",
		FaviconURL:                "/static/img/favicon.png",
		ContactEmail:              "info@dawa24.com",
		SupportEmail:              "support@dawa24.com",
		Phone:                     "01065397000",
		WhatsApp:                  "201065397000",
		Address:                   i18n.TDefault("w4_ui.s_100_100"),
		SessionIdleTimeoutMinutes: 30,
		SocialLinks: map[string]string{
			"facebook":  "https://facebook.com/dawa24",
			"twitter":   "https://twitter.com/dawa24",
			"instagram": "https://instagram.com/dawa24",
			"linkedin":  "https://linkedin.com/company/dawa24",
			"youtube":   "https://youtube.com/@dawa24",
			"tiktok":    "https://tiktok.com/@dawa24",
			"snapchat":  "https://snapchat.com/add/dawa24",
			"whatsapp":  "https://wa.me/201065397000",
			"telegram":  "https://t.me/dawa24",
		},
	}
}
