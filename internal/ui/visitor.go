package ui

import (
	"crypto/rand"
	"encoding/hex"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"net"
	"net/http"
	"strings"
	"time"

	platformadmin "github.com/muhiya/dawa24-store/internal/modules/platform_admin"
	"github.com/muhiya/dawa24-store/internal/platform/antiscrape"
)

const visitorCookieName = "dawa24_visitor"

// visitorMiddleware records one visitor row per session per day. It never
// writes per request: the cookie carries the day already recorded, so a write
// happens at most once per visitor per day.
func (h *UIHandler) visitorMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Static assets are not page views; skip them.
		if h.adminSvc != nil && !strings.HasPrefix(r.URL.Path, "/static/") {
			h.recordVisitor(w, r)
		}
		next.ServeHTTP(w, r)
	})
}

// recordVisitor records the visitor if this is their first hit today, then
// refreshes the cookie so the next hit is a no-op.
func (h *UIHandler) recordVisitor(w http.ResponseWriter, r *http.Request) {
	// A crawler is not a visitor.
	//
	// The de-duplication below is a cookie, and nothing that is not a browser
	// keeps one, so every request from a bot was a fresh row: Googlebot showed
	// up in the analytics as thousands of daily visitors, and a scraper the
	// anti-scraping guard was about to refuse still cost a write on the way in.
	// The counts are meant to answer "how many pharmacies came today", so this
	// records browsers only.
	if antiscrape.Classify(r) != antiscrape.ClassBrowser {
		return
	}

	today := time.Now().Format("2006-01-02")

	key := newVisitorKey()
	cookie, err := r.Cookie(visitorCookieName)
	if err == nil && cookie != nil {
		if dot := strings.LastIndex(cookie.Value, "."); dot >= 0 && cookie.Value[dot+1:] == today {
			return // already recorded today
		}
		key = cookie.Value
	}

	clientIP := h.clientIP(r)
	browser, device, osName := parseUserAgent(r.UserAgent())
	country, city := detectCountryAndCity(r, clientIP)

	_ = h.adminSvc.RecordVisitor(r.Context(), &platformadmin.Visitor{
		VisitorKey: key,
		IP:         clientIP,
		UserAgent:  r.UserAgent(),
		Browser:    browser,
		Device:     device,
		OS:         osName,
		Country:    country,
		City:       city,
	})

	http.SetCookie(w, &http.Cookie{
		Name:     visitorCookieName,
		Value:    key + "." + today,
		Path:     "/",
		MaxAge:   86400 * 365 * 2,
		SameSite: http.SameSiteLaxMode,
		// Only the server reads it; no script needs it.
		HttpOnly: true,
		Secure:   h.secureCookie,
	})
}

func newVisitorKey() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "v" + time.Now().Format("20060102150405")
	}
	return hex.EncodeToString(b)
}

// detectCountryAndCity determines geographic info from headers and network address.
func detectCountryAndCity(r *http.Request, clientIP string) (country, city string) {
	// 1. Check CDN / Cloudflare / Proxy headers
	if cfCountry := strings.TrimSpace(r.Header.Get("CF-IPCountry")); cfCountry != "" && len(cfCountry) == 2 {
		country = mapCountryCode(cfCountry)
	}
	if country == "" {
		if xCountry := strings.TrimSpace(r.Header.Get("X-Country-Code")); xCountry != "" {
			country = mapCountryCode(xCountry)
		}
	}
	if city == "" {
		city = cleanCityName(strings.TrimSpace(r.Header.Get("CF-IPCity")))
		if city == "" {
			city = cleanCityName(strings.TrimSpace(r.Header.Get("X-City-Name")))
		}
	}

	// 2. Check if private or local address
	ip := net.ParseIP(strings.TrimSpace(clientIP))
	if ip != nil && (ip.IsLoopback() || ip.IsPrivate()) {
		if country == "" {
			country = i18n.T("ar", "geo.country.eg")
		}
		if city == "" {
			city = i18n.T("ar", "geo.city.cairo")
		}
		return country, city
	}

	// 3. Check Accept-Language header if still undetermined
	if country == "" {
		al := strings.ToLower(r.Header.Get("Accept-Language"))
		switch {
		case strings.Contains(al, "ar-eg"), strings.Contains(al, "eg"):
			country = i18n.T("ar", "geo.country.eg")
			if city == "" {
				city = i18n.T("ar", "geo.city.cairo")
			}
		case strings.Contains(al, "ar-sa"), strings.Contains(al, "sa"):
			country = i18n.T("ar", "geo.country.sa")
			if city == "" {
				city = i18n.T("ar", "geo.city.riyadh")
			}
		case strings.Contains(al, "ar-ae"), strings.Contains(al, "ae"):
			country = i18n.T("ar", "geo.country.ae")
			if city == "" {
				city = i18n.T("ar", "geo.city.dubai")
			}
		case strings.Contains(al, "ar-kw"), strings.Contains(al, "kw"):
			country = i18n.T("ar", "geo.country.kw")
			if city == "" {
				city = i18n.T("ar", "geo.city.kuwait")
			}
		case strings.Contains(al, "ar-jo"), strings.Contains(al, "jo"):
			country = i18n.T("ar", "geo.country.jo")
			if city == "" {
				city = i18n.T("ar", "geo.city.amman")
			}
		case strings.Contains(al, "en-us"):
			country = i18n.T("ar", "geo.country.us")
			if city == "" {
				city = i18n.T("ar", "geo.city.newyork")
			}
		case strings.Contains(al, "en-gb"):
			country = i18n.T("ar", "geo.country.gb")
			if city == "" {
				city = i18n.T("ar", "geo.city.london")
			}
		default:
			country = i18n.T("ar", "geo.country.eg")
			if city == "" {
				city = i18n.T("ar", "geo.city.cairo")
			}
		}
	}

	if city == "" {
		if strings.Contains(country, i18n.TDefault("w4_ui.s_176_176")) {
			city = i18n.T("ar", "geo.city.cairo")
		} else if strings.Contains(country, i18n.TDefault("w4_ui.s_177_177")) {
			city = i18n.T("ar", "geo.city.riyadh")
		} else {
			city = i18n.T("ar", "geo.city.main_center")
		}
	}
	return country, city
}

func cleanCityName(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == i18n.TDefault("w4_ui.s_178_178") || strings.Contains(raw, "Local") {
		return ""
	}
	switch strings.ToLower(raw) {
	case "cairo", "al qahirah", "el qahira":
		return i18n.T("ar", "geo.city.cairo")
	case "giza", "al jizah":
		return i18n.T("ar", "geo.city.giza")
	case "alexandria", "al iskandariyah":
		return i18n.T("ar", "geo.city.alexandria")
	case "mansoura", "al mansurah":
		return i18n.T("ar", "geo.city.mansoura")
	case "tanta":
		return i18n.T("ar", "geo.city.tanta")
	case "asyut", "assiut":
		return i18n.T("ar", "geo.city.asyut")
	case "zagazig", "az zaqaziq":
		return i18n.T("ar", "geo.city.zagazig")
	case "ismailia", "al ismailiyah":
		return i18n.T("ar", "geo.city.ismailia")
	case "suez", "as suways":
		return i18n.T("ar", "geo.city.suez")
	case "port said", "bur said":
		return i18n.T("ar", "geo.city.portsaid")
	case "damietta", "dumyat":
		return i18n.T("ar", "geo.city.damietta")
	case "aswan":
		return i18n.T("ar", "geo.city.aswan")
	case "luxor", "al uqsur":
		return i18n.T("ar", "geo.city.luxor")
	case "hurghada", "al ghardaqah":
		return i18n.T("ar", "geo.city.hurghada")
	case "sharm el-sheikh", "sharm ash shaykh":
		return i18n.T("ar", "geo.city.sharm")
	case "riyadh", "ar riyad":
		return i18n.T("ar", "geo.city.riyadh")
	case "jeddah":
		return i18n.T("ar", "geo.city.jeddah")
	case "mecca", "makkah":
		return i18n.T("ar", "geo.city.mecca")
	case "medina", "madinah":
		return i18n.T("ar", "geo.city.medina")
	case "dammam", "ad dammam":
		return i18n.T("ar", "geo.city.dammam")
	case "dubai":
		return i18n.T("ar", "geo.city.dubai")
	case "abu dhabi":
		return i18n.T("ar", "geo.city.abudhabi")
	case "doha":
		return i18n.T("ar", "geo.city.doha")
	case "kuwait", "kuwait city":
		return i18n.T("ar", "geo.city.kuwait")
	case "amman":
		return i18n.T("ar", "geo.city.amman")
	default:
		return raw
	}
}

func mapCountryCode(code string) string {
	switch strings.ToUpper(code) {
	case "EG":
		return i18n.T("ar", "geo.country.eg")
	case "SA":
		return i18n.T("ar", "geo.country.sa")
	case "AE":
		return i18n.T("ar", "geo.country.ae")
	case "KW":
		return i18n.T("ar", "geo.country.kw")
	case "JO":
		return i18n.T("ar", "geo.country.jo")
	case "OM":
		return i18n.T("ar", "geo.country.om")
	case "QA":
		return i18n.T("ar", "geo.country.qa")
	case "BH":
		return i18n.T("ar", "geo.country.bh")
	case "IQ":
		return i18n.T("ar", "geo.country.iq")
	case "LY":
		return i18n.T("ar", "geo.country.ly")
	case "SD":
		return i18n.T("ar", "geo.country.sd")
	case "US":
		return i18n.T("ar", "geo.country.us")
	case "GB", "UK":
		return i18n.T("ar", "geo.country.gb")
	case "DE":
		return i18n.T("ar", "geo.country.de")
	default:
		return strings.ToUpper(code)
	}
}

// parseUserAgent does a lightweight, dependency-free split of the user agent.
func parseUserAgent(ua string) (browser, device, osName string) {
	u := strings.ToLower(ua)
	switch {
	case strings.Contains(u, "chrome") || strings.Contains(u, "crios"):
		browser = "chrome"
	case strings.Contains(u, "firefox") || strings.Contains(u, "fxios"):
		browser = "firefox"
	case strings.Contains(u, "safari"):
		browser = "safari"
	case strings.Contains(u, "edg"):
		browser = "edge"
	default:
		browser = "other"
	}
	switch {
	case strings.Contains(u, "ipad"):
		device = "tablet"
	case strings.Contains(u, "mobile") || strings.Contains(u, "android") || strings.Contains(u, "iphone"):
		device = "mobile"
	default:
		device = "desktop"
	}
	switch {
	case strings.Contains(u, "windows"):
		osName = "windows"
	case strings.Contains(u, "mac os") || strings.Contains(u, "iphone") || strings.Contains(u, "ipad"):
		osName = "ios/macos"
	case strings.Contains(u, "android"):
		osName = "android"
	case strings.Contains(u, "linux"):
		osName = "linux"
	default:
		osName = "other"
	}
	return browser, device, osName
}
