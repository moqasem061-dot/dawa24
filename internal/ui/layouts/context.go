package layouts

import (
	"context"

	"github.com/muhiya/dawa24-store/internal/platform/httpx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	platformadmin "github.com/muhiya/dawa24-store/internal/modules/platform_admin"
)

type siteSettingsKey struct{}

// WithSiteSettings embeds SiteSettings into the context for all layout rendering.
func WithSiteSettings(ctx context.Context, s *platformadmin.SiteSettings) context.Context {
	if s == nil {
		return ctx
	}
	return context.WithValue(ctx, siteSettingsKey{}, s)
}

// GetSiteSettings retrieves SiteSettings from the context.
func GetSiteSettings(ctx context.Context) *platformadmin.SiteSettings {
	if ctx != nil {
		if s, ok := ctx.Value(siteSettingsKey{}).(*platformadmin.SiteSettings); ok && s != nil {
			return s
		}
	}
	return &platformadmin.SiteSettings{
		SiteName:        i18n.TDefault("w4_ui.24_28"),
		SiteDescription: i18n.TDefault("w4_ui.s_99_99"),
		LogoURL:         "/static/img/logo.png",
		FaviconURL:      "/static/img/favicon.png",
		ContactEmail:    "info@dawa24.com",
		SupportEmail:    "support@dawa24.com",
		Phone:           "01065397000",
		WhatsApp:        "201065397000",
		Address:         i18n.TDefault("w4_ui.s_100_100"),
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

type requestPathKey struct{}

// WithPath records the path being rendered so the navigation can mark its own
// active item.
//
// The public bar used to have no active state at all: every link looked the
// same on every page, so the header told a reader nothing about where they
// were. Doing it in CSS was not an option — the bar is one template rendered
// for every route — and doing it in JavaScript would have painted the wrong
// link first. It is one string on the context instead.
func WithPath(ctx context.Context, path string) context.Context {
	return context.WithValue(ctx, requestPathKey{}, path)
}

// IsCurrentPath reports whether href is the page currently being rendered.
// Exact match only: /catalog must not light up for /catalog-import, and the
// public bar has no nested sections.
func IsCurrentPath(ctx context.Context, href string) bool {
	if ctx == nil {
		return false
	}
	p, ok := ctx.Value(requestPathKey{}).(string)
	if !ok {
		return false
	}
	if len(p) > 1 && p[len(p)-1] == '/' {
		p = p[:len(p)-1]
	}
	return p == href
}

type authNoticeKey struct{}

// WithAuthNotice embeds an authorization flash message into the request context.
func WithAuthNotice(ctx context.Context, notice string) context.Context {
	if notice == "" {
		return ctx
	}
	return context.WithValue(ctx, authNoticeKey{}, notice)
}

// GetAuthNotice retrieves the authorization flash message from context, if any.
func GetAuthNotice(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if n, ok := ctx.Value(authNoticeKey{}).(string); ok {
		return n
	}
	return ""
}

type seoPageKey struct{}

// WithSEOPage embeds an SEO configuration into the context.
func WithSEOPage(ctx context.Context, s *platformadmin.SEOPage) context.Context {
	if s == nil {
		return ctx
	}
	return context.WithValue(ctx, seoPageKey{}, s)
}

// GetSEOPage retrieves the SEO configuration from context.
func GetSEOPage(ctx context.Context) *platformadmin.SEOPage {
	if ctx != nil {
		if s, ok := ctx.Value(seoPageKey{}).(*platformadmin.SEOPage); ok && s != nil {
			return s
		}
	}
	return nil
}

// WithNonce stores the per-request CSP nonce in the context.
func WithNonce(ctx context.Context, nonce string) context.Context {
	return httpx.WithNonce(ctx, nonce)
}

// Nonce returns the CSP nonce for the current request, or the empty string if none was set.
func Nonce(ctx context.Context) string {
	return httpx.Nonce(ctx)
}


// htmxConfig is the <meta name="htmx-config"> payload. See base.templ.
const htmxConfig = `{"allowEval":false,"includeIndicatorStyles":false,"selfRequestsOnly":true,"historyCacheSize":0}`
