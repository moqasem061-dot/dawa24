package ui

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"net/http"
	"strings"
	"time"

	platformadmin "github.com/muhiya/dawa24-store/internal/modules/platform_admin"
)

// siteURL is the public origin links are built on: the configured base URL,
// or — in development, where none is configured — the request's own host.
func (h *UIHandler) siteURL(r *http.Request) string {
	if h.baseURL != "" {
		return h.baseURL
	}
	scheme := "https"
	if r.TLS == nil && !strings.HasPrefix(r.Header.Get("X-Forwarded-Proto"), "https") && (strings.HasPrefix(r.Host, "localhost") || strings.HasPrefix(r.Host, "127.0.0.1")) {
		scheme = "http"
	}
	return fmt.Sprintf("%s://%s", scheme, r.Host)
}

// sitemapPrivatePrefixes are areas that are never sitemap pages: dashboards,
// admin, API, account and buying flows.
var sitemapPrivatePrefixes = []string{
	"/admin", "/api", "/vendor", "/customer", "/account", "/auth", "/cart", "/checkout",
	"/orders", "/settings", "/favorites", "/uploads", "/static", "/smart-order", "/compare",
	"/assistant", "/notifications", "/wallet", "/dashboard", "/integrations", "/moderator",
	"/purchase-request", "/reviews", "/documents", "/suppliers/followed",
}

// sitemapPath reports whether a route may be published in the sitemap.
//
// The SEO table marks rows public for page-metadata reasons, and it holds admin
// screens, dashboard pages and API endpoints: 196 of its 321 public rows were
// not pages a visitor can open. The sitemap lists only concrete public pages,
// decided here rather than by whatever the table says.
func sitemapPath(p string) bool {
	if p == "" || p[0] != '/' || strings.ContainsAny(p, "{}*?:") {
		return false
	}
	for _, private := range sitemapPrivatePrefixes {
		if p == private || strings.HasPrefix(p, private+"/") {
			return false
		}
	}
	return true
}

type sitemapEntry struct {
	path       string
	priority   string
	changefreq string
	lastmod    string
}

// SitemapXML generates and returns the canonical XML sitemap per sitemaps.org protocol.
func (h *UIHandler) SitemapXML(w http.ResponseWriter, r *http.Request) {
	baseURL := h.siteURL(r)
	now := time.Now().UTC().Format("2006-01-02")

	var entries []sitemapEntry
	if h.adminSvc != nil {
		isPublic := true
		pgs, _, err := h.adminSvc.ListSEOPages(r.Context(), platformadmin.SEOPagesFilter{
			IsPublic: &isPublic,
			Limit:    1000,
		})
		if err == nil && len(pgs) > 0 {
			for _, p := range pgs {
				if strings.Contains(strings.ToLower(p.RobotsDirectives), "noindex") || !sitemapPath(p.RoutePattern) {
					continue
				}
				cf := p.ChangeFreq
				if cf == "" {
					cf = "monthly"
				}
				prio := "0.7"
				if p.Priority > 0 {
					prio = fmt.Sprintf("%.1f", p.Priority)
				}
				mod := now
				if !p.UpdatedAt.IsZero() {
					mod = p.UpdatedAt.UTC().Format("2006-01-02")
				}
				entries = append(entries, sitemapEntry{
					path:       p.RoutePattern,
					priority:   prio,
					changefreq: cf,
					lastmod:    mod,
				})
			}
		}
	}

	if len(entries) == 0 {
		routes := []struct {
			path       string
			priority   string
			changefreq string
		}{
			{"/", "1.0", "daily"},
			{"/about", "0.8", "monthly"},
			{"/how-it-works", "0.8", "monthly"},
			{"/jobs", "0.7", "weekly"},
			{"/faq", "0.7", "monthly"},
			{"/contact", "0.7", "monthly"},
			{"/terms", "0.5", "monthly"},
			{"/privacy", "0.5", "monthly"},
			{"/llms.txt", "0.8", "weekly"},
		}
		for _, r := range routes {
			entries = append(entries, sitemapEntry{
				path:       r.path,
				priority:   r.priority,
				changefreq: r.changefreq,
				lastmod:    now,
			})
		}
	}

	var sb strings.Builder
	sb.WriteString("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n")
	sb.WriteString("<urlset xmlns=\"http://www.sitemaps.org/schemas/sitemap/0.9\">\n")

	for _, item := range entries {
		sb.WriteString("  <url>\n")
		sb.WriteString("    <loc>")
		_ = xml.EscapeText(&sb, []byte(baseURL+item.path))
		sb.WriteString("</loc>\n")
		sb.WriteString(fmt.Sprintf("    <lastmod>%s</lastmod>\n", item.lastmod))
		sb.WriteString(fmt.Sprintf("    <changefreq>%s</changefreq>\n", item.changefreq))
		sb.WriteString(fmt.Sprintf("    <priority>%s</priority>\n", item.priority))
		sb.WriteString("  </url>\n")
	}

	sb.WriteString("</urlset>\n")

	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(sb.String()))
}

// LLMsTxt serves /llms.txt per https://llmstxt.org providing an LLM-friendly index of the site.
func (h *UIHandler) LLMsTxt(w http.ResponseWriter, r *http.Request) {
	baseURL := h.siteURL(r)
	content := fmt.Sprintf(`# Dawa24 (دوا 24)

> Unified B2B Marketplace connecting licensed pharmacies with verified pharmaceutical suppliers and warehouses in Egypt.

## Core Information
- Platform: Dawa24 B2B Pharmaceutical Marketplace
- Scope: Arab Republic of Egypt
- Website: %s

## Public Sections
- [About Dawa24](%s/about): Platform vision and mission.
- [How It Works](%s/how-it-works): Pharmacy ordering and supplier dispatch workflows.
- [Careers](%s/jobs): Open positions in technology, sales, and logistics.
- [FAQ](%s/faq): Common inquiries about pharmaceutical licensing and fulfillment.
- [Contact](%s/contact): Support hotline and contact information.
- [Terms](%s/terms): Platform terms and conditions.
- [Privacy Policy](%s/privacy): Data privacy policies.
`, baseURL, baseURL, baseURL, baseURL, baseURL, baseURL, baseURL, baseURL)

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(content))
}

// AgentsIndexJSON serves /.well-known/agents-index.json for DNS-AID HTTP discovery.
func (h *UIHandler) AgentsIndexJSON(w http.ResponseWriter, r *http.Request) {
	baseURL := h.siteURL(r)
	resp := map[string]any{
		"$schema":      "https://agents-index.org/schema/v1.json",
		"version":      "1.0",
		"organization": "Dawa24",
		"domain":       strings.TrimPrefix(strings.TrimPrefix(baseURL, "https://"), "http://"),
		"agents": []map[string]any{
			{
				"name":        "dawa24-web",
				"protocol":    "https",
				"endpoint":    baseURL,
				"description": "Dawa24 B2B Pharmaceutical Marketplace Web Interface",
			},
		},
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}
